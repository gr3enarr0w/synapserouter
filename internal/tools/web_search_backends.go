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

// --- You.com Backend (you.com — multi-step AI search) ---

type YouBackend struct{ apiKey string }

func (b *YouBackend) CostPer1K() float64 { return 5.00 }

func (b *YouBackend) Name() string { return "you" }

func (b *YouBackend) Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	u, _ := url.Parse("https://api.you.com/v1/search")
	params := url.Values{}
	params.Set("query", query)
	params.Set("count", fmt.Sprintf("%d", maxResults))
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
		Results struct {
			Web []struct {
				Title       string   `json:"title"`
				URL         string   `json:"url"`
				Description string   `json:"description"`
				Snippets    []string `json:"snippets"`
			} `json:"web"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&yResp); err != nil {
		return nil, fmt.Errorf("parse you.com response: %w", err)
	}

	var results []SearchResult
	for _, hit := range yResp.Results.Web {
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

func (b *SerpAPIBackend) CostPer1K() float64 { return 10.00 }

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

// --- Jina Reader Backend (jina.ai — search + content extraction) ---

type JinaBackend struct{ apiKey string }

func (b *JinaBackend) CostPer1K() float64 { return 5.00 }

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

func (b *KagiBackend) CostPer1K() float64 { return 25.00 }

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

// --- Linkup Backend (linkup.so — AI-native search) ---

type LinkupBackend struct{ apiKey string }

func (b *LinkupBackend) CostPer1K() float64 { return 5.50 }

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

type SemanticScholarBackend struct {
	apiKey string // optional — higher rate limits with key
}

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
	if b.apiKey != "" {
		req.Header.Set("x-api-key", b.apiKey)
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

func (b *SemanticScholarBackend) CostPer1K() float64 { return 0 }

// --- OpenAlex Backend (openalex.org — 260M academic works, fully open) ---

type OpenAlexBackend struct{}

func (b *OpenAlexBackend) CostPer1K() float64 { return 0 }

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

func (b *GitHubSearchBackend) CostPer1K() float64 { return 0 }

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

func (b *SourcegraphBackend) CostPer1K() float64 { return 0 }

func (b *SourcegraphBackend) Name() string { return "sourcegraph" }

func (b *SourcegraphBackend) Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	u, _ := url.Parse("https://sourcegraph.com/.api/search/stream")
	params := url.Values{}
	params.Set("q", fmt.Sprintf("type:file %s count:%d", query, maxResults))
	params.Set("v", "V3")
	u.RawQuery = params.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "token "+b.token)
	req.Header.Set("Accept", "text/event-stream")

	resp, err := ssrfSafeClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("sourcegraph error %d: %s", resp.StatusCode, string(respBody))
	}

	// SSE stream: parse "event: matches" followed by "data: [...]"
	var results []SearchResult
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	lines := strings.Split(string(body), "\n")
	for i, line := range lines {
		// Look for "event: matches" then grab the next "data: " line
		if line != "event: matches" {
			continue
		}
		if i+1 >= len(lines) || !strings.HasPrefix(lines[i+1], "data: ") {
			continue
		}
		data := strings.TrimPrefix(lines[i+1], "data: ")
		var matches []struct {
			Type       string `json:"type"`
			Repository string `json:"repository"`
			Path       string `json:"path"`
		}
		if json.Unmarshal([]byte(data), &matches) != nil {
			continue
		}
		for _, m := range matches {
			if len(results) >= maxResults {
				break
			}
			results = append(results, SearchResult{
				Title:   m.Path,
				URL:     fmt.Sprintf("https://sourcegraph.com/%s/-/blob/%s", m.Repository, m.Path),
				Snippet: m.Repository,
			})
		}
	}
	return results, nil
}

// --- NewsAPI Backend (newsapi.org — 150K+ news sources, 55 countries) ---

type NewsAPIBackend struct{ apiKey string }

func (b *NewsAPIBackend) CostPer1K() float64 { return 0 }

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

// --- SearchAPI.io Backend (searchapi.io — Google, Bing, Baidu, Naver, DuckDuckGo) ---

type SearchAPIBackend struct{ apiKey string }

func (b *SearchAPIBackend) CostPer1K() float64 { return 5.00 }

func (b *SearchAPIBackend) Name() string { return "searchapi" }

func (b *SearchAPIBackend) Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	u, _ := url.Parse("https://www.searchapi.io/api/v1/search")
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
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("searchapi error %d: %s", resp.StatusCode, string(respBody))
	}

	var sResp struct {
		OrganicResults []struct {
			Title   string `json:"title"`
			Link    string `json:"link"`
			Snippet string `json:"snippet"`
		} `json:"organic_results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&sResp); err != nil {
		return nil, fmt.Errorf("parse searchapi response: %w", err)
	}

	var results []SearchResult
	for _, r := range sResp.OrganicResults {
		results = append(results, SearchResult{Title: r.Title, URL: r.Link, Snippet: r.Snippet})
	}
	return results, nil
}

// --- Newsdata.io Backend (newsdata.io — 79K+ news sources, 206 countries) ---

type NewsdataBackend struct{ apiKey string }

func (b *NewsdataBackend) CostPer1K() float64 { return 0 }

func (b *NewsdataBackend) Name() string { return "newsdata" }

func (b *NewsdataBackend) Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	u, _ := url.Parse("https://newsdata.io/api/1/latest")
	params := url.Values{}
	params.Set("q", query)
	params.Set("apikey", b.apiKey)
	params.Set("language", "en")
	params.Set("size", fmt.Sprintf("%d", maxResults))
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
		return nil, fmt.Errorf("newsdata error %d: %s", resp.StatusCode, string(respBody))
	}

	var nResp struct {
		Results []struct {
			Title       string `json:"title"`
			Link        string `json:"link"`
			Description string `json:"description"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&nResp); err != nil {
		return nil, fmt.Errorf("parse newsdata response: %w", err)
	}

	var results []SearchResult
	for _, r := range nResp.Results {
		results = append(results, SearchResult{Title: r.Title, URL: r.Link, Snippet: r.Description})
	}
	return results, nil
}

// --- TheNewsAPI Backend (thenewsapi.com — structured global news) ---

type TheNewsAPIBackend struct{ apiKey string }

func (b *TheNewsAPIBackend) CostPer1K() float64 { return 0 }

func (b *TheNewsAPIBackend) Name() string { return "thenewsapi" }

func (b *TheNewsAPIBackend) Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	u, _ := url.Parse("https://api.thenewsapi.com/v1/news/all")
	params := url.Values{}
	params.Set("search", query)
	params.Set("api_token", b.apiKey)
	params.Set("language", "en")
	params.Set("limit", fmt.Sprintf("%d", maxResults))
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
		return nil, fmt.Errorf("thenewsapi error %d: %s", resp.StatusCode, string(respBody))
	}

	var tResp struct {
		Data []struct {
			Title       string `json:"title"`
			URL         string `json:"url"`
			Description string `json:"description"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tResp); err != nil {
		return nil, fmt.Errorf("parse thenewsapi response: %w", err)
	}

	var results []SearchResult
	for _, d := range tResp.Data {
		results = append(results, SearchResult{Title: d.Title, URL: d.URL, Snippet: d.Description})
	}
	return results, nil
}
