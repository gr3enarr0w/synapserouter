package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

const (
	defaultSearchTimeout = 15 * time.Second
	maxSearchResults     = 10
	duckDuckGoURL        = "https://html.duckduckgo.com/html/"
)

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
// Default: DuckDuckGo. Set TAVILY_API_KEY for Tavily. Set SEARXNG_URL for SearXNG.
type WebSearchTool struct {
	backend SearchBackend
}

// NewWebSearchTool creates a web search tool with auto-detected backend.
func NewWebSearchTool() *WebSearchTool {
	return &WebSearchTool{backend: detectSearchBackend()}
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
	return "Search the web and return top results. Supports Serper, Brave, Exa, Tavily, SearXNG, and DuckDuckGo backends."
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
