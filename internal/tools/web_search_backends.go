package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// --- Perplexity Sonar Backend (perplexity.ai — AI-grounded search) ---

type PerplexityBackend struct{ apiKey string }

func (b *PerplexityBackend) Name() string { return "perplexity" }

func (b *PerplexityBackend) Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	body := fmt.Sprintf(`{"model":"sonar","messages":[{"role":"user","content":%q}],"search_recency_filter":"month","return_related_questions":false}`, query)
	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.perplexity.ai/chat/completions", strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+b.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := ssrfSafeClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("perplexity API error %d: %s", resp.StatusCode, string(body))
	}

	var pResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Citations []string `json:"citations"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&pResp); err != nil {
		return nil, fmt.Errorf("parse perplexity response: %w", err)
	}

	var results []SearchResult
	content := ""
	if len(pResp.Choices) > 0 {
		content = pResp.Choices[0].Message.Content
	}
	for i, citation := range pResp.Citations {
		if i >= maxResults {
			break
		}
		results = append(results, SearchResult{
			Title:   fmt.Sprintf("Source %d", i+1),
			URL:     citation,
			Snippet: content,
		})
	}
	if len(results) == 0 && content != "" {
		results = append(results, SearchResult{Title: "Perplexity Answer", URL: "", Snippet: content})
	}
	return results, nil
}

// --- You.com Backend (you.com — multi-step AI search) ---

type YouBackend struct{ apiKey string }

func (b *YouBackend) Name() string { return "you" }

func (b *YouBackend) Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	u, _ := url.Parse("https://api.ydc-index.io/search")
	params := url.Values{}
	params.Set("query", query)
	params.Set("num_web_results", fmt.Sprintf("%d", maxResults))
	u.RawQuery = params.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-API-Key", b.apiKey)

	resp, err := ssrfSafeClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("you.com API error %d: %s", resp.StatusCode, string(body))
	}

	var yResp struct {
		Hits []struct {
			Title       string   `json:"title"`
			URL         string   `json:"url"`
			Description string   `json:"description"`
			Snippets    []string `json:"snippets"`
		} `json:"hits"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&yResp); err != nil {
		return nil, fmt.Errorf("parse you.com response: %w", err)
	}

	var results []SearchResult
	for _, hit := range yResp.Hits {
		snippet := hit.Description
		if snippet == "" && len(hit.Snippets) > 0 {
			snippet = hit.Snippets[0]
		}
		results = append(results, SearchResult{Title: hit.Title, URL: hit.URL, Snippet: snippet})
	}
	return results, nil
}

// --- SerpAPI Backend (serpapi.com — 80+ search engines) ---

type SerpAPIBackend struct{ apiKey string }

func (b *SerpAPIBackend) Name() string { return "serpapi" }

func (b *SerpAPIBackend) Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	u, _ := url.Parse("https://serpapi.com/search.json")
	params := url.Values{}
	params.Set("q", query)
	params.Set("api_key", b.apiKey)
	params.Set("engine", "google")
	params.Set("num", fmt.Sprintf("%d", maxResults))
	u.RawQuery = params.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := ssrfSafeClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("serpapi error %d: %s", resp.StatusCode, string(body))
	}

	var sResp struct {
		OrganicResults []struct {
			Title   string `json:"title"`
			Link    string `json:"link"`
			Snippet string `json:"snippet"`
		} `json:"organic_results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&sResp); err != nil {
		return nil, fmt.Errorf("parse serpapi response: %w", err)
	}

	var results []SearchResult
	for _, r := range sResp.OrganicResults {
		results = append(results, SearchResult{Title: r.Title, URL: r.Link, Snippet: r.Snippet})
	}
	return results, nil
}

// --- Google Custom Search Backend (programmablesearchengine.google.com) ---

type GoogleCSEBackend struct {
	apiKey string
	cx     string // Custom Search Engine ID
}

func (b *GoogleCSEBackend) Name() string { return "google-cse" }

func (b *GoogleCSEBackend) Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	u, _ := url.Parse("https://www.googleapis.com/customsearch/v1")
	params := url.Values{}
	params.Set("q", query)
	params.Set("key", b.apiKey)
	params.Set("cx", b.cx)
	params.Set("num", fmt.Sprintf("%d", maxResults))
	u.RawQuery = params.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := ssrfSafeClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("google CSE error %d: %s", resp.StatusCode, string(body))
	}

	var gResp struct {
		Items []struct {
			Title   string `json:"title"`
			Link    string `json:"link"`
			Snippet string `json:"snippet"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&gResp); err != nil {
		return nil, fmt.Errorf("parse google CSE response: %w", err)
	}

	var results []SearchResult
	for _, item := range gResp.Items {
		results = append(results, SearchResult{Title: item.Title, URL: item.Link, Snippet: item.Snippet})
	}
	return results, nil
}

// --- Jina Reader Backend (jina.ai — search + content extraction) ---

type JinaBackend struct{ apiKey string }

func (b *JinaBackend) Name() string { return "jina" }

func (b *JinaBackend) Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	searchURL := "https://s.jina.ai/" + url.QueryEscape(query)
	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+b.apiKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Retain-Images", "none")

	resp, err := ssrfSafeClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("jina API error %d: %s", resp.StatusCode, string(body))
	}

	var jResp struct {
		Data []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&jResp); err != nil {
		return nil, fmt.Errorf("parse jina response: %w", err)
	}

	var results []SearchResult
	for i, d := range jResp.Data {
		if i >= maxResults {
			break
		}
		snippet := d.Content
		if len(snippet) > 500 {
			snippet = snippet[:497] + "..."
		}
		results = append(results, SearchResult{Title: d.Title, URL: d.URL, Snippet: snippet})
	}
	return results, nil
}

// --- Kagi Backend (kagi.com — ad-free, privacy-focused) ---

type KagiBackend struct{ apiKey string }

func (b *KagiBackend) Name() string { return "kagi" }

func (b *KagiBackend) Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	u, _ := url.Parse("https://kagi.com/api/v0/search")
	params := url.Values{}
	params.Set("q", query)
	params.Set("limit", fmt.Sprintf("%d", maxResults))
	u.RawQuery = params.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bot "+b.apiKey)

	resp, err := ssrfSafeClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("kagi API error %d: %s", resp.StatusCode, string(body))
	}

	var kResp struct {
		Data []struct {
			T       string `json:"t"`       // title
			URL     string `json:"url"`
			Snippet string `json:"snippet"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&kResp); err != nil {
		return nil, fmt.Errorf("parse kagi response: %w", err)
	}

	var results []SearchResult
	for _, d := range kResp.Data {
		if d.URL == "" {
			continue
		}
		results = append(results, SearchResult{Title: d.T, URL: d.URL, Snippet: d.Snippet})
	}
	return results, nil
}

// --- Yandex Backend (yandex.cloud — Russia/CIS/Turkey) ---

type YandexBackend struct{ apiKey string }

func (b *YandexBackend) Name() string { return "yandex" }

func (b *YandexBackend) Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	u, _ := url.Parse("https://yandex.com/search/xml")
	params := url.Values{}
	params.Set("query", query)
	params.Set("apikey", b.apiKey)
	params.Set("format", "json")
	params.Set("numdoc", fmt.Sprintf("%d", maxResults))
	u.RawQuery = params.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := ssrfSafeClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("yandex API error %d: %s", resp.StatusCode, string(body))
	}

	var yResp struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Snippet string `json:"snippet"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&yResp); err != nil {
		return nil, fmt.Errorf("parse yandex response: %w", err)
	}

	var results []SearchResult
	for _, r := range yResp.Results {
		results = append(results, SearchResult{Title: r.Title, URL: r.URL, Snippet: r.Snippet})
	}
	return results, nil
}

// --- Linkup Backend (linkup.so — AI-native search) ---

type LinkupBackend struct{ apiKey string }

func (b *LinkupBackend) Name() string { return "linkup" }

func (b *LinkupBackend) Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	body := fmt.Sprintf(`{"q":%q,"depth":"standard","outputType":"searchResults"}`, query)
	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.linkup.so/v1/search", strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+b.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := ssrfSafeClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("linkup API error %d: %s", resp.StatusCode, string(body))
	}

	var lResp struct {
		Results []struct {
			Name    string `json:"name"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&lResp); err != nil {
		return nil, fmt.Errorf("parse linkup response: %w", err)
	}

	var results []SearchResult
	for i, r := range lResp.Results {
		if i >= maxResults {
			break
		}
		snippet := r.Content
		if len(snippet) > 500 {
			snippet = snippet[:497] + "..."
		}
		results = append(results, SearchResult{Title: r.Name, URL: r.URL, Snippet: snippet})
	}
	return results, nil
}

// --- Semantic Scholar Backend (semanticscholar.org — 200M academic papers, free) ---

type SemanticScholarBackend struct{}

func (b *SemanticScholarBackend) Name() string { return "semantic-scholar" }

func (b *SemanticScholarBackend) Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	u, _ := url.Parse("https://api.semanticscholar.org/graph/v1/paper/search")
	params := url.Values{}
	params.Set("query", query)
	params.Set("limit", fmt.Sprintf("%d", maxResults))
	params.Set("fields", "title,url,abstract")
	u.RawQuery = params.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, err
	}
	// Optional API key for higher rate limits
	if key := strings.TrimSpace(strings.ToLower(fmt.Sprintf("%s", ctx.Value("semantic_scholar_key")))); key != "" {
		req.Header.Set("x-api-key", key)
	}

	resp, err := ssrfSafeClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("semantic scholar error %d: %s", resp.StatusCode, string(body))
	}

	var ssResp struct {
		Data []struct {
			Title    string `json:"title"`
			URL      string `json:"url"`
			Abstract string `json:"abstract"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ssResp); err != nil {
		return nil, fmt.Errorf("parse semantic scholar response: %w", err)
	}

	var results []SearchResult
	for _, p := range ssResp.Data {
		paperURL := p.URL
		if paperURL == "" {
			paperURL = fmt.Sprintf("https://www.semanticscholar.org/paper/%s", url.QueryEscape(p.Title))
		}
		snippet := p.Abstract
		if len(snippet) > 500 {
			snippet = snippet[:497] + "..."
		}
		results = append(results, SearchResult{Title: p.Title, URL: paperURL, Snippet: snippet})
	}
	return results, nil
}

// --- OpenAlex Backend (openalex.org — 260M academic works, fully open) ---

type OpenAlexBackend struct{}

func (b *OpenAlexBackend) Name() string { return "openalex" }

func (b *OpenAlexBackend) Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	u, _ := url.Parse("https://api.openalex.org/works")
	params := url.Values{}
	params.Set("search", query)
	params.Set("per_page", fmt.Sprintf("%d", maxResults))
	params.Set("select", "title,doi,primary_location")
	u.RawQuery = params.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "synapserouter/1.0 (mailto:contact@synapserouter.dev)")

	resp, err := ssrfSafeClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("openalex error %d: %s", resp.StatusCode, string(body))
	}

	var oResp struct {
		Results []struct {
			Title           string `json:"title"`
			DOI             string `json:"doi"`
			PrimaryLocation struct {
				LandingPageURL string `json:"landing_page_url"`
			} `json:"primary_location"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&oResp); err != nil {
		return nil, fmt.Errorf("parse openalex response: %w", err)
	}

	var results []SearchResult
	for _, w := range oResp.Results {
		pageURL := w.PrimaryLocation.LandingPageURL
		if pageURL == "" {
			pageURL = w.DOI
		}
		results = append(results, SearchResult{Title: w.Title, URL: pageURL, Snippet: ""})
	}
	return results, nil
}

// --- GitHub Search Backend (github.com — code search across public repos) ---

type GitHubSearchBackend struct{ token string }

func (b *GitHubSearchBackend) Name() string { return "github" }

func (b *GitHubSearchBackend) Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	u, _ := url.Parse("https://api.github.com/search/code")
	params := url.Values{}
	params.Set("q", query)
	params.Set("per_page", fmt.Sprintf("%d", maxResults))
	u.RawQuery = params.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+b.token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := ssrfSafeClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("github search error %d: %s", resp.StatusCode, string(body))
	}

	var ghResp struct {
		Items []struct {
			Name       string `json:"name"`
			HTMLURL    string `json:"html_url"`
			Repository struct {
				FullName string `json:"full_name"`
			} `json:"repository"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ghResp); err != nil {
		return nil, fmt.Errorf("parse github response: %w", err)
	}

	var results []SearchResult
	for _, item := range ghResp.Items {
		results = append(results, SearchResult{
			Title:   fmt.Sprintf("%s — %s", item.Name, item.Repository.FullName),
			URL:     item.HTMLURL,
			Snippet: item.Repository.FullName,
		})
	}
	return results, nil
}

// --- Sourcegraph Backend (sourcegraph.com — code search, 2M+ repos) ---

type SourcegraphBackend struct{ token string }

func (b *SourcegraphBackend) Name() string { return "sourcegraph" }

func (b *SourcegraphBackend) Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	body := fmt.Sprintf(`{"query":"type:file %s count:%d"}`, query, maxResults)
	req, err := http.NewRequestWithContext(ctx, "POST", "https://sourcegraph.com/.api/search/stream", strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "token "+b.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := ssrfSafeClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("sourcegraph error %d: %s", resp.StatusCode, string(respBody))
	}

	// Sourcegraph stream API returns newline-delimited JSON events
	var results []SearchResult
	decoder := json.NewDecoder(resp.Body)
	for decoder.More() && len(results) < maxResults {
		var event struct {
			Type string `json:"type"`
			Data []struct {
				Repository string `json:"repository"`
				Path       string `json:"path"`
			} `json:"data"`
		}
		if err := decoder.Decode(&event); err != nil {
			break
		}
		if event.Type != "content" {
			continue
		}
		for _, match := range event.Data {
			results = append(results, SearchResult{
				Title:   match.Path,
				URL:     fmt.Sprintf("https://sourcegraph.com/%s/-/blob/%s", match.Repository, match.Path),
				Snippet: match.Repository,
			})
		}
	}
	return results, nil
}

// --- NewsAPI Backend (newsapi.org — 150K+ news sources, 55 countries) ---

type NewsAPIBackend struct{ apiKey string }

func (b *NewsAPIBackend) Name() string { return "newsapi" }

func (b *NewsAPIBackend) Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	u, _ := url.Parse("https://newsapi.org/v2/everything")
	params := url.Values{}
	params.Set("q", query)
	params.Set("apiKey", b.apiKey)
	params.Set("pageSize", fmt.Sprintf("%d", maxResults))
	params.Set("sortBy", "relevancy")
	u.RawQuery = params.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := ssrfSafeClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("newsapi error %d: %s", resp.StatusCode, string(respBody))
	}

	var nResp struct {
		Articles []struct {
			Title       string `json:"title"`
			URL         string `json:"url"`
			Description string `json:"description"`
		} `json:"articles"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&nResp); err != nil {
		return nil, fmt.Errorf("parse newsapi response: %w", err)
	}

	var results []SearchResult
	for _, a := range nResp.Articles {
		results = append(results, SearchResult{Title: a.Title, URL: a.URL, Snippet: a.Description})
	}
	return results, nil
}
