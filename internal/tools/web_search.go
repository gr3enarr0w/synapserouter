package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/providers"
)

const (
	defaultSearchTimeout    = 15 * time.Second
	fusionTimeoutPerBackend = 2 * time.Second
	maxSearchResults        = 10
	rrfK                    = 60 // standard RRF constant (Cormack et al. 2009)
	duckDuckGoURL           = "https://html.duckduckgo.com/html/"
)

// BackendResult holds one backend's ranked results alongside metadata.
type BackendResult struct {
	BackendName string
	Results     []SearchResult
	Err         error
}

// backendResult is the unexported alias used internally.
type backendResult = BackendResult

// ChatCompleter is a minimal interface for LLM re-ranking.
// Defined locally to avoid import cycles (tools ↔ agent/orchestration).
type ChatCompleter interface {
	ChatCompletion(ctx context.Context, req providers.ChatRequest, sessionID string) (providers.ChatResponse, error)
}

// SearchResult represents a single search result.
type SearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

// SearchBackend is the interface for pluggable search providers.
type SearchBackend interface {
	Name() string
	Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error)
}

// WebSearchTool searches the web using a configurable backend.
// When multiple backends are configured, queries all in parallel and merges
// results via Reciprocal Rank Fusion (RRF) for better coverage.
type WebSearchTool struct {
	backend   SearchBackend   // single-backend mode (backward compat)
	backends  []SearchBackend // all configured backends for fusion mode
	fusion    bool            // whether fusion is active
	completer ChatCompleter   // optional, for LLM re-ranking; nil = skip
}

// NewWebSearchTool creates a web search tool with auto-detected backend(s).
// Pass an optional ChatCompleter to enable LLM re-ranking of fused results.
func NewWebSearchTool(completer ...ChatCompleter) *WebSearchTool {
	backends := detectAllBackends()
	fusion := isFusionEnabled(len(backends))

	var comp ChatCompleter
	if len(completer) > 0 {
		comp = completer[0]
	}

	if fusion {
		names := make([]string, len(backends))
		for i, b := range backends {
			names[i] = b.Name()
		}
		log.Printf("[SearchFusion] enabled with %d backends: %s", len(backends), strings.Join(names, ", "))
		return &WebSearchTool{
			backends:  backends,
			fusion:    true,
			completer: comp,
		}
	}
	return &WebSearchTool{
		backend: detectSearchBackend(),
		fusion:  false,
	}
}

// detectSearchBackend selects the search backend by quality ranking.
// Priority based on AIMultiple agentic search benchmark (2026):
// Brave (14.89 score, 669ms) > Tavily (LangChain default) >
// Serper ($0.30/1K, Google) > Exa (semantic) > SearXNG > DuckDuckGo.
func detectSearchBackend() SearchBackend {
	if key := os.Getenv("BRAVE_API_KEY"); key != "" {
		return &BraveBackend{apiKey: key}
	}
	if key := os.Getenv("TAVILY_API_KEY"); key != "" {
		return &TavilyBackend{apiKey: key}
	}
	if key := os.Getenv("SERPER_API_KEY"); key != "" {
		return &SerperBackend{apiKey: key}
	}
	if key := os.Getenv("EXA_API_KEY"); key != "" {
		return &ExaBackend{apiKey: key}
	}
	if url := os.Getenv("SEARXNG_URL"); url != "" {
		return &SearXNGBackend{baseURL: url}
	}
	return &DuckDuckGoBackend{}
}

func (t *WebSearchTool) Name() string     { return "web_search" }
func (t *WebSearchTool) Description() string {
	return "Search the web and return top results. 19 backends: Brave, Tavily, Serper, Exa, SearXNG, DuckDuckGo, You.com, SerpAPI, SearchAPI.io, Jina, Kagi, Linkup, Semantic Scholar, OpenAlex, GitHub, Sourcegraph, NewsAPI, Newsdata.io, TheNewsAPI. Multiple backends fused via RRF."
}
func (t *WebSearchTool) Category() ToolCategory { return CategoryReadOnly }

func (t *WebSearchTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "The search query",
			},
			"max_results": map[string]interface{}{
				"type":        "integer",
				"description": "Maximum number of results to return (default 10, max 10)",
			},
		},
		"required": []string{"query"},
	}
}

func (t *WebSearchTool) Execute(ctx context.Context, args map[string]interface{}, workDir string) (*ToolResult, error) {
	query := stringArg(args, "query")
	if query == "" {
		return &ToolResult{Error: "query is required"}, nil
	}

	maxResults := intArg(args, "max_results", maxSearchResults)
	if maxResults > maxSearchResults {
		maxResults = maxSearchResults
	}
	if maxResults < 1 {
		maxResults = 1
	}

	// Fusion mode: query all configured backends in parallel, merge via RRF
	if t.fusion && len(t.backends) >= 2 {
		return t.executeFusion(ctx, query, maxResults)
	}

	// Single-backend mode (backward compatible)
	ctx, cancel := context.WithTimeout(ctx, defaultSearchTimeout)
	defer cancel()

	backend := t.backend
	if backend == nil {
		backend = &DuckDuckGoBackend{}
	}
	results, err := backend.Search(ctx, query, maxResults)
	if err != nil {
		return &ToolResult{Error: fmt.Sprintf("search failed: %v", err)}, nil
	}

	if len(results) == 0 {
		return &ToolResult{Output: "no results found"}, nil
	}

	return &ToolResult{Output: formatSearchResults(results)}, nil
}

// executeFusion queries all backends in parallel and merges results via RRF.
func (t *WebSearchTool) executeFusion(ctx context.Context, query string, maxResults int) (*ToolResult, error) {
	backendResults := searchAllBackends(ctx, t.backends, query, maxResults)

	// Count successes
	successCount := 0
	for _, br := range backendResults {
		if br.Err == nil && len(br.Results) > 0 {
			successCount++
			log.Printf("[SearchFusion] %s: %d results", br.BackendName, len(br.Results))
		} else if br.Err != nil {
			log.Printf("[SearchFusion] %s: failed: %v", br.BackendName, br.Err)
		}
	}

	if successCount == 0 {
		var errs []string
		for _, br := range backendResults {
			if br.Err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", br.BackendName, br.Err))
			}
		}
		if len(errs) > 0 {
			return &ToolResult{Error: fmt.Sprintf("all search backends failed: %s", strings.Join(errs, "; "))}, nil
		}
		return &ToolResult{Output: "no results found"}, nil
	}

	// Single success: skip RRF, return directly
	if successCount == 1 {
		for _, br := range backendResults {
			if br.Err == nil && len(br.Results) > 0 {
				trimmed := br.Results
				if len(trimmed) > maxResults {
					trimmed = trimmed[:maxResults]
				}
				return &ToolResult{Output: formatSearchResults(trimmed)}, nil
			}
		}
	}

	// Merge via RRF
	merged := mergeRRF(backendResults, maxResults)

	// Optional LLM re-rank when 2+ backends contributed
	if t.completer != nil && successCount >= 2 && len(merged) > 1 {
		merged = llmRerank(ctx, t.completer, query, merged)
	}

	if len(merged) == 0 {
		return &ToolResult{Output: "no results found"}, nil
	}

	log.Printf("[SearchFusion] merged %d results from %d backends", len(merged), successCount)
	return &ToolResult{Output: formatSearchResults(merged)}, nil
}

// --- DuckDuckGo Backend ---

// DuckDuckGoBackend searches via DuckDuckGo's HTML interface (no API key needed).
type DuckDuckGoBackend struct{}

func (b *DuckDuckGoBackend) Name() string { return "duckduckgo" }
func (b *DuckDuckGoBackend) Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	return duckDuckGoSearch(ctx, query, maxResults)
}

func duckDuckGoSearch(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	form := url.Values{}
	form.Set("q", query)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, duckDuckGoURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; SynapseRouter/1.0)")

	resp, err := ssrfSafeClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB limit
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	return parseDDGResults(string(body), maxResults), nil
}

// resultBlockRe matches each DuckDuckGo result block.
var resultBlockRe = regexp.MustCompile(`(?s)<div[^>]*class="[^"]*result__body[^"]*"[^>]*>(.*?)</div>`)

// resultLinkRe extracts the result link and title from a result snippet link.
var resultLinkRe = regexp.MustCompile(`<a[^>]*class="[^"]*result__a[^"]*"[^>]*href="([^"]*)"[^>]*>(.*?)</a>`)

// resultSnippetRe extracts the snippet text.
var resultSnippetRe = regexp.MustCompile(`(?s)<a[^>]*class="[^"]*result__snippet[^"]*"[^>]*>(.*?)</a>`)

// uddgRe extracts the actual URL from DuckDuckGo's redirect wrapper.
var uddgRe = regexp.MustCompile(`uddg=([^&]+)`)

// parseDDGResults parses DuckDuckGo HTML search results.
func parseDDGResults(html string, maxResults int) []SearchResult {
	blocks := resultBlockRe.FindAllStringSubmatch(html, maxResults*2) // over-fetch in case some are ads
	var results []SearchResult
	for _, block := range blocks {
		if len(results) >= maxResults {
			break
		}
		content := block[1]

		linkMatch := resultLinkRe.FindStringSubmatch(content)
		if linkMatch == nil {
			continue
		}

		rawURL := linkMatch[1]
		title := stripHTML(linkMatch[2])

		// Extract actual URL from DuckDuckGo redirect
		actualURL := extractDDGURL(rawURL)
		if actualURL == "" {
			continue
		}

		snippet := ""
		snippetMatch := resultSnippetRe.FindStringSubmatch(content)
		if snippetMatch != nil {
			snippet = stripHTML(snippetMatch[1])
		}

		if title == "" {
			continue
		}

		results = append(results, SearchResult{
			Title:   strings.TrimSpace(title),
			URL:     actualURL,
			Snippet: strings.TrimSpace(snippet),
		})
	}
	return results
}

// extractDDGURL extracts the real URL from a DuckDuckGo redirect link.
func extractDDGURL(rawURL string) string {
	match := uddgRe.FindStringSubmatch(rawURL)
	if match != nil {
		decoded, err := url.QueryUnescape(match[1])
		if err == nil {
			return decoded
		}
	}
	// If it's already a direct URL, return as-is
	if strings.HasPrefix(rawURL, "http://") || strings.HasPrefix(rawURL, "https://") {
		return rawURL
	}
	return ""
}

// htmlTagRe matches HTML tags for stripping.
var htmlTagRe = regexp.MustCompile(`<[^>]*>`)

// htmlEntityMap covers common HTML entities.
var htmlEntityMap = map[string]string{
	"&amp;":  "&",
	"&lt;":   "<",
	"&gt;":   ">",
	"&quot;": `"`,
	"&#39;":  "'",
	"&apos;": "'",
	"&nbsp;": " ",
}

// stripHTML removes HTML tags and decodes common entities.
func stripHTML(s string) string {
	s = htmlTagRe.ReplaceAllString(s, "")
	for entity, replacement := range htmlEntityMap {
		s = strings.ReplaceAll(s, entity, replacement)
	}
	return strings.TrimSpace(s)
}

// --- Tavily Backend ---

// TavilyBackend uses Tavily's AI-optimized search API.
type TavilyBackend struct {
	apiKey string
}

func (b *TavilyBackend) Name() string { return "tavily" }

func (b *TavilyBackend) Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	reqBody, _ := json.Marshal(map[string]interface{}{
		"api_key":            b.apiKey,
		"query":              query,
		"max_results":        maxResults,
		"search_depth":       "basic",
		"include_answer":     false,
		"include_raw_content": false,
	})

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.tavily.com/search", strings.NewReader(string(reqBody)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := ssrfSafeClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("tavily API error %d: %s", resp.StatusCode, string(body))
	}

	var tavilyResp struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tavilyResp); err != nil {
		return nil, fmt.Errorf("parse tavily response: %w", err)
	}

	results := make([]SearchResult, 0, len(tavilyResp.Results))
	for _, r := range tavilyResp.Results {
		results = append(results, SearchResult{
			Title:   r.Title,
			URL:     r.URL,
			Snippet: r.Content,
		})
	}
	return results, nil
}

// --- SearXNG Backend ---

// SearXNGBackend uses a self-hosted SearXNG instance.
type SearXNGBackend struct {
	baseURL string
}

func (b *SearXNGBackend) Name() string { return "searxng" }

func (b *SearXNGBackend) Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	u, err := url.Parse(b.baseURL + "/search")
	if err != nil {
		return nil, err
	}
	params := url.Values{}
	params.Set("q", query)
	params.Set("format", "json")
	params.Set("categories", "general")
	u.RawQuery = params.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := ssrfSafeClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("searxng error: %d", resp.StatusCode)
	}

	var searxResp struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&searxResp); err != nil {
		return nil, fmt.Errorf("parse searxng response: %w", err)
	}

	results := make([]SearchResult, 0, maxResults)
	for i, r := range searxResp.Results {
		if i >= maxResults {
			break
		}
		results = append(results, SearchResult{
			Title:   r.Title,
			URL:     r.URL,
			Snippet: r.Content,
		})
	}
	return results, nil
}

// --- Serper Backend (serper.dev — Google Search results) ---

type SerperBackend struct{ apiKey string }

func (b *SerperBackend) Name() string { return "serper" }

func (b *SerperBackend) Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	reqBody, _ := json.Marshal(map[string]interface{}{"q": query, "num": maxResults})
	req, err := http.NewRequestWithContext(ctx, "POST", "https://google.serper.dev/search", strings.NewReader(string(reqBody)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", b.apiKey)

	resp, err := ssrfSafeClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("serper API error %d: %s", resp.StatusCode, string(body))
	}

	var serperResp struct {
		Organic []struct {
			Title   string `json:"title"`
			Link    string `json:"link"`
			Snippet string `json:"snippet"`
		} `json:"organic"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&serperResp); err != nil {
		return nil, fmt.Errorf("parse serper response: %w", err)
	}
	results := make([]SearchResult, 0, len(serperResp.Organic))
	for _, r := range serperResp.Organic {
		results = append(results, SearchResult{Title: r.Title, URL: r.Link, Snippet: r.Snippet})
	}
	return results, nil
}

// --- Brave Search Backend (brave.com — independent web index) ---

type BraveBackend struct{ apiKey string }

func (b *BraveBackend) Name() string { return "brave" }

func (b *BraveBackend) Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	u, _ := url.Parse("https://api.search.brave.com/res/v1/web/search")
	params := url.Values{}
	params.Set("q", query)
	params.Set("count", fmt.Sprintf("%d", maxResults))
	u.RawQuery = params.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", b.apiKey)

	resp, err := ssrfSafeClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("brave API error %d: %s", resp.StatusCode, string(body))
	}

	var braveResp struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&braveResp); err != nil {
		return nil, fmt.Errorf("parse brave response: %w", err)
	}
	results := make([]SearchResult, 0, len(braveResp.Web.Results))
	for _, r := range braveResp.Web.Results {
		results = append(results, SearchResult{Title: r.Title, URL: r.URL, Snippet: r.Description})
	}
	return results, nil
}

// --- Exa AI Backend (exa.ai — semantic/neural search) ---

type ExaBackend struct{ apiKey string }

func (b *ExaBackend) Name() string { return "exa" }

func (b *ExaBackend) Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	reqBody, _ := json.Marshal(map[string]interface{}{"query": query, "numResults": maxResults, "type": "auto"})
	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.exa.ai/search", strings.NewReader(string(reqBody)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+b.apiKey)

	resp, err := ssrfSafeClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("exa API error %d: %s", resp.StatusCode, string(body))
	}

	var exaResp struct {
		Results []struct {
			Title string `json:"title"`
			URL   string `json:"url"`
			Text  string `json:"text"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&exaResp); err != nil {
		return nil, fmt.Errorf("parse exa response: %w", err)
	}
	results := make([]SearchResult, 0, len(exaResp.Results))
	for _, r := range exaResp.Results {
		results = append(results, SearchResult{Title: r.Title, URL: r.URL, Snippet: r.Text})
	}
	return results, nil
}

// formatSearchResults formats results for display.
func formatSearchResults(results []SearchResult) string {
	var sb strings.Builder
	for i, r := range results {
		if i > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString(fmt.Sprintf("[%d] %s\n    %s", i+1, r.Title, r.URL))
		if r.Snippet != "" {
			sb.WriteString(fmt.Sprintf("\n    %s", r.Snippet))
		}
	}
	return sb.String()
}

// --- Search Fusion: RRF Multi-Backend Merge ---

// DetectAllBackends returns all configured search backends.
// Exported for use by the research pipeline.
func DetectAllBackends() []SearchBackend {
	return detectAllBackends()
}

// detectAllBackends returns all configured search backends.
// DuckDuckGo is always included as the last fallback (no API key needed).
func detectAllBackends() []SearchBackend {
	var backends []SearchBackend
	if key := os.Getenv("BRAVE_API_KEY"); key != "" {
		backends = append(backends, &BraveBackend{apiKey: key})
	}
	if key := os.Getenv("TAVILY_API_KEY"); key != "" {
		backends = append(backends, &TavilyBackend{apiKey: key})
	}
	if key := os.Getenv("SERPER_API_KEY"); key != "" {
		backends = append(backends, &SerperBackend{apiKey: key})
	}
	if key := os.Getenv("EXA_API_KEY"); key != "" {
		backends = append(backends, &ExaBackend{apiKey: key})
	}
	if u := os.Getenv("SEARXNG_URL"); u != "" {
		backends = append(backends, &SearXNGBackend{baseURL: u})
	}
	// Tier 1: Large user bases
	if key := os.Getenv("YOU_API_KEY"); key != "" {
		backends = append(backends, &YouBackend{apiKey: key})
	}
	if key := os.Getenv("SERPAPI_KEY"); key != "" {
		backends = append(backends, &SerpAPIBackend{apiKey: key})
	}
	if key := os.Getenv("JINA_API_KEY"); key != "" {
		backends = append(backends, &JinaBackend{apiKey: key})
	}
	// Tier 2: Regional/niche
	if key := os.Getenv("KAGI_API_KEY"); key != "" {
		backends = append(backends, &KagiBackend{apiKey: key})
	}
	if key := os.Getenv("LINKUP_API_KEY"); key != "" {
		backends = append(backends, &LinkupBackend{apiKey: key})
	}
	if key := os.Getenv("SEARCHAPI_KEY"); key != "" {
		backends = append(backends, &SearchAPIBackend{apiKey: key})
	}
	// Tier 3: Specialized (academic — keys optional for higher rate limits)
	backends = append(backends, &SemanticScholarBackend{apiKey: os.Getenv("SEMANTIC_SCHOLAR_API_KEY")})
	backends = append(backends, &OpenAlexBackend{})
	// Tier 3: Code (optional keys)
	if key := os.Getenv("GITHUB_TOKEN"); key != "" {
		backends = append(backends, &GitHubSearchBackend{token: key})
	}
	if key := os.Getenv("SOURCEGRAPH_TOKEN"); key != "" {
		backends = append(backends, &SourcegraphBackend{token: key})
	}
	// Tier 3: News
	if key := os.Getenv("NEWSAPI_KEY"); key != "" {
		backends = append(backends, &NewsAPIBackend{apiKey: key})
	}
	if key := os.Getenv("NEWSDATA_API_KEY"); key != "" {
		backends = append(backends, &NewsdataBackend{apiKey: key})
	}
	if key := os.Getenv("THENEWSAPI_KEY"); key != "" {
		backends = append(backends, &TheNewsAPIBackend{apiKey: key})
	}
	// DuckDuckGo always last (free fallback)
	backends = append(backends, &DuckDuckGoBackend{})
	return backends
}

// isFusionEnabled checks whether search fusion should be active.
// Auto-enables when 2+ backends are configured, unless SYNROUTE_SEARCH_FUSION=false.
func isFusionEnabled(backendCount int) bool {
	switch strings.ToLower(os.Getenv("SYNROUTE_SEARCH_FUSION")) {
	case "false", "0", "no":
		return false
	case "true", "1", "yes":
		return backendCount >= 2
	default:
		return backendCount >= 2
	}
}

// SearchSelectedBackends queries a specific set of backends in parallel.
// Used by the research pipeline to query only the backends matching the query type.
func SearchSelectedBackends(ctx context.Context, backends []SearchBackend, query string, maxResults int) []backendResult {
	return searchAllBackends(ctx, backends, query, maxResults)
}

// NormalizeURL is the exported version of normalizeURL for use by the research pipeline.
func NormalizeURL(rawURL string) string {
	return normalizeURL(rawURL)
}

// searchAllBackends queries all backends in parallel with per-backend timeouts.
func searchAllBackends(ctx context.Context, backends []SearchBackend, query string, maxResults int) []backendResult {
	results := make([]backendResult, len(backends))
	var wg sync.WaitGroup

	for i, b := range backends {
		wg.Add(1)
		go func(idx int, backend SearchBackend) {
			defer wg.Done()
			bctx, cancel := context.WithTimeout(ctx, fusionTimeoutPerBackend)
			defer cancel()
			res, err := backend.Search(bctx, query, maxResults)
			results[idx] = backendResult{
				BackendName: backend.Name(),
				Results:     res,
				Err:         err,
			}
		}(i, b)
	}

	wg.Wait()
	return results
}

// normalizeURL produces a canonical form for URL deduplication.
// Lowercases scheme+host, strips www., trailing slash, fragments, and tracking params.
func normalizeURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	u.Host = strings.TrimPrefix(u.Host, "www.")
	u.Fragment = ""

	// Strip tracking parameters
	q := u.Query()
	for key := range q {
		lower := strings.ToLower(key)
		if strings.HasPrefix(lower, "utm_") || lower == "ref" || lower == "source" {
			q.Del(key)
		}
	}
	u.RawQuery = q.Encode()
	u.Path = strings.TrimRight(u.Path, "/")
	if u.Path == "" {
		u.Path = "/"
	}
	return u.String()
}

// mergeRRF merges results from multiple backends using Reciprocal Rank Fusion.
// score(doc) = sum(1 / (k + rank_i)) for each backend that returned the doc.
// Documents appearing in multiple backends naturally rise to the top.
func mergeRRF(backendResults []backendResult, maxResults int) []SearchResult {
	type candidate struct {
		bestResult SearchResult
		bestRank   int
		score      float64
	}
	urlMap := make(map[string]*candidate)

	for _, br := range backendResults {
		if br.Err != nil || len(br.Results) == 0 {
			continue
		}
		for rank, sr := range br.Results {
			normURL := normalizeURL(sr.URL)
			rrfScore := 1.0 / float64(rrfK+rank+1) // rank+1 for 1-based

			if c, exists := urlMap[normURL]; exists {
				c.score += rrfScore
				if rank < c.bestRank {
					c.bestResult = sr
					c.bestRank = rank
				}
			} else {
				urlMap[normURL] = &candidate{
					bestResult: sr,
					bestRank:   rank,
					score:      rrfScore,
				}
			}
		}
	}

	sorted := make([]*candidate, 0, len(urlMap))
	for _, c := range urlMap {
		sorted = append(sorted, c)
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].score > sorted[j].score
	})

	results := make([]SearchResult, 0, maxResults)
	for i, c := range sorted {
		if i >= maxResults {
			break
		}
		results = append(results, c.bestResult)
	}
	return results
}

// llmRerank optionally re-ranks search results using the router's cheapest model.
// Uses model "auto" so the router picks from whatever's configured (provider-agnostic).
// On any failure (timeout, parse error, nil completer), returns results unchanged.
func llmRerank(ctx context.Context, completer ChatCompleter, query string, results []SearchResult) []SearchResult {
	if completer == nil || len(results) <= 1 {
		return results
	}

	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Rank these search results by relevance to: %q\n\n", query))
	for i, r := range results {
		sb.WriteString(fmt.Sprintf("%d. [%s] %s\n", i, r.Title, r.Snippet))
	}
	sb.WriteString("\nReturn ONLY a JSON array of indices in order of relevance, e.g. [2,0,1,3]")

	resp, err := completer.ChatCompletion(ctx, providers.ChatRequest{
		Model:       "auto",
		Messages:    []providers.Message{{Role: "user", Content: sb.String()}},
		Temperature: 0,
		MaxTokens:   100,
		SkipMemory:  true,
	}, "")
	if err != nil || len(resp.Choices) == 0 {
		return results
	}

	content := strings.TrimSpace(resp.Choices[0].Message.Content)
	// Extract JSON array from response (may have surrounding text)
	start := strings.Index(content, "[")
	end := strings.LastIndex(content, "]")
	if start < 0 || end < 0 || end <= start {
		return results
	}
	content = content[start : end+1]

	var indices []int
	if err := json.Unmarshal([]byte(content), &indices); err != nil {
		return results
	}

	seen := make(map[int]bool)
	reranked := make([]SearchResult, 0, len(results))
	for _, idx := range indices {
		if idx >= 0 && idx < len(results) && !seen[idx] {
			reranked = append(reranked, results[idx])
			seen[idx] = true
		}
	}
	// Append any results the LLM missed
	for i, r := range results {
		if !seen[i] {
			reranked = append(reranked, r)
		}
	}
	return reranked
}
