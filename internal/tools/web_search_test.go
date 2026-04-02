package tools

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestParseDDGResults(t *testing.T) {
	html := `
<html><body>
<div class="result__body">
	<a class="result__a" href="https://duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com%2Fpage1&amp;rut=abc">Example Page One</a>
	<a class="result__snippet" href="#">This is the <b>first</b> snippet.</a>
</div>
<div class="result__body">
	<a class="result__a" href="https://duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com%2Fpage2&amp;rut=def">Example &amp; Page Two</a>
	<a class="result__snippet" href="#">Second snippet here.</a>
</div>
<div class="result__body">
	<a class="result__a" href="https://duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com%2Fpage3&amp;rut=ghi">Third Result</a>
	<a class="result__snippet" href="#">Third snippet.</a>
</div>
</body></html>`

	results := parseDDGResults(html, 10)
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// Check first result
	if results[0].Title != "Example Page One" {
		t.Errorf("title[0] = %q, want %q", results[0].Title, "Example Page One")
	}
	if results[0].URL != "https://example.com/page1" {
		t.Errorf("url[0] = %q, want %q", results[0].URL, "https://example.com/page1")
	}
	if results[0].Snippet != "This is the first snippet." {
		t.Errorf("snippet[0] = %q, want %q", results[0].Snippet, "This is the first snippet.")
	}

	// Check HTML entity decoding in title
	if results[1].Title != "Example & Page Two" {
		t.Errorf("title[1] = %q, want %q", results[1].Title, "Example & Page Two")
	}
}

func TestParseDDGResults_MaxResults(t *testing.T) {
	html := `
<div class="result__body">
	<a class="result__a" href="https://duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com%2Fa">A</a>
	<a class="result__snippet" href="#">Snippet A</a>
</div>
<div class="result__body">
	<a class="result__a" href="https://duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com%2Fb">B</a>
	<a class="result__snippet" href="#">Snippet B</a>
</div>
<div class="result__body">
	<a class="result__a" href="https://duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com%2Fc">C</a>
	<a class="result__snippet" href="#">Snippet C</a>
</div>`

	results := parseDDGResults(html, 2)
	if len(results) != 2 {
		t.Errorf("expected 2 results (max), got %d", len(results))
	}
}

func TestParseDDGResults_Empty(t *testing.T) {
	results := parseDDGResults("<html><body>no results</body></html>", 10)
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestExtractDDGURL(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "uddg parameter",
			raw:  "https://duckduckgo.com/l/?uddg=https%3A%2F%2Fgolang.org%2Fdoc&rut=abc",
			want: "https://golang.org/doc",
		},
		{
			name: "direct https url",
			raw:  "https://golang.org/doc",
			want: "https://golang.org/doc",
		},
		{
			name: "direct http url",
			raw:  "http://example.com",
			want: "http://example.com",
		},
		{
			name: "relative or empty",
			raw:  "/some/path",
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractDDGURL(tt.raw)
			if got != tt.want {
				t.Errorf("extractDDGURL(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestStripHTML(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"<b>bold</b> text", "bold text"},
		{"no tags", "no tags"},
		{"&amp; &lt; &gt;", "& < >"},
		{"<span class=\"x\">inner</span>", "inner"},
		{"", ""},
	}
	for _, tt := range tests {
		got := stripHTML(tt.input)
		if got != tt.want {
			t.Errorf("stripHTML(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFormatSearchResults(t *testing.T) {
	results := []SearchResult{
		{Title: "Go Programming", URL: "https://golang.org", Snippet: "The Go language"},
		{Title: "No Snippet", URL: "https://example.com", Snippet: ""},
	}
	output := formatSearchResults(results)
	if output == "" {
		t.Fatal("expected non-empty output")
	}
	if !contains(output, "[1] Go Programming") {
		t.Error("missing first result header")
	}
	if !contains(output, "https://golang.org") {
		t.Error("missing first result URL")
	}
	if !contains(output, "The Go language") {
		t.Error("missing first result snippet")
	}
	if !contains(output, "[2] No Snippet") {
		t.Error("missing second result header")
	}
}

func TestWebSearchTool_EmptyQuery(t *testing.T) {
	tool := &WebSearchTool{}
	result, err := tool.Execute(context.Background(), map[string]interface{}{}, ".")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error == "" {
		t.Error("expected error for empty query")
	}
}

func TestWebSearchTool_Schema(t *testing.T) {
	tool := &WebSearchTool{}
	if tool.Name() != "web_search" {
		t.Errorf("name = %q, want web_search", tool.Name())
	}
	if tool.Category() != CategoryReadOnly {
		t.Errorf("category = %v, want read_only", tool.Category())
	}
	schema := tool.InputSchema()
	props, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("missing properties in schema")
	}
	if _, ok := props["query"]; !ok {
		t.Error("missing 'query' in schema properties")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && containsStr(s, substr)))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// --- Search Fusion Tests ---

type mockSearchBackend struct {
	name    string
	results []SearchResult
	err     error
	delay   time.Duration
}

func (m *mockSearchBackend) Name() string { return m.name }
func (m *mockSearchBackend) Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return m.results, m.err
}

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"https://example.com/page", "https://example.com/page"},
		{"https://example.com/page/", "https://example.com/page"},
		{"https://www.example.com/page", "https://example.com/page"},
		{"HTTPS://EXAMPLE.COM/page", "https://example.com/page"},
		{"https://example.com/page?utm_source=google&utm_medium=cpc", "https://example.com/page"},
		{"https://example.com/page#section", "https://example.com/page"},
		{"https://example.com/page?q=test&utm_campaign=spring", "https://example.com/page?q=test"},
		{"https://example.com/", "https://example.com/"},
		{"https://example.com", "https://example.com/"},
		{"not-a-url", "not-a-url"},
	}

	for _, tt := range tests {
		got := normalizeURL(tt.input)
		if got != tt.want {
			t.Errorf("normalizeURL(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestMergeRRF_BasicMerge(t *testing.T) {
	backendResults := []backendResult{
		{
			backendName: "backend-a",
			results: []SearchResult{
				{Title: "Shared Result", URL: "https://example.com/shared", Snippet: "A's version"},
				{Title: "A Only", URL: "https://example.com/a-only", Snippet: "Only in A"},
			},
		},
		{
			backendName: "backend-b",
			results: []SearchResult{
				{Title: "Shared Result", URL: "https://example.com/shared", Snippet: "B's version"},
				{Title: "B Only", URL: "https://example.com/b-only", Snippet: "Only in B"},
			},
		},
	}

	merged := mergeRRF(backendResults, 10)

	if len(merged) != 3 {
		t.Fatalf("expected 3 merged results, got %d", len(merged))
	}

	// Shared result should be first (highest RRF score)
	if merged[0].URL != "https://example.com/shared" {
		t.Errorf("expected shared result first, got %q", merged[0].URL)
	}
}

func TestMergeRRF_SingleBackend(t *testing.T) {
	backendResults := []backendResult{
		{backendName: "ok", results: []SearchResult{
			{Title: "Result 1", URL: "https://example.com/1"},
		}},
		{backendName: "failed", err: errors.New("timeout")},
	}

	merged := mergeRRF(backendResults, 10)
	if len(merged) != 1 {
		t.Fatalf("expected 1 result, got %d", len(merged))
	}
}

func TestMergeRRF_AllFailed(t *testing.T) {
	backendResults := []backendResult{
		{backendName: "a", err: errors.New("fail")},
		{backendName: "b", err: errors.New("fail")},
	}

	merged := mergeRRF(backendResults, 10)
	if len(merged) != 0 {
		t.Errorf("expected 0 results, got %d", len(merged))
	}
}

func TestMergeRRF_DuplicateURLs(t *testing.T) {
	backendResults := []backendResult{
		{backendName: "a", results: []SearchResult{
			{Title: "Page", URL: "https://www.example.com/page/", Snippet: "A"},
		}},
		{backendName: "b", results: []SearchResult{
			{Title: "Page Better", URL: "https://example.com/page", Snippet: "B"},
		}},
	}

	merged := mergeRRF(backendResults, 10)
	if len(merged) != 1 {
		t.Fatalf("expected 1 deduped result, got %d", len(merged))
	}
}

func TestMergeRRF_MaxResults(t *testing.T) {
	var results []SearchResult
	for i := 0; i < 20; i++ {
		results = append(results, SearchResult{
			Title: "Result",
			URL:   "https://example.com/" + string(rune('a'+i)),
		})
	}

	merged := mergeRRF([]backendResult{{backendName: "a", results: results}}, 5)
	if len(merged) != 5 {
		t.Errorf("expected 5 results, got %d", len(merged))
	}
}

func TestSearchAllBackends_Parallel(t *testing.T) {
	backends := []SearchBackend{
		&mockSearchBackend{name: "fast", results: []SearchResult{{Title: "Fast"}}, delay: 100 * time.Millisecond},
		&mockSearchBackend{name: "slow", results: []SearchResult{{Title: "Slow"}}, delay: 100 * time.Millisecond},
	}

	start := time.Now()
	results := searchAllBackends(context.Background(), backends, "test", 5)
	elapsed := time.Since(start)

	if elapsed > 300*time.Millisecond {
		t.Errorf("parallel search took %v, expected < 300ms", elapsed)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
}

func TestSearchAllBackends_GracefulDegradation(t *testing.T) {
	backends := []SearchBackend{
		&mockSearchBackend{name: "ok", results: []SearchResult{{Title: "Good"}}},
		&mockSearchBackend{name: "broken", err: errors.New("network error")},
	}

	results := searchAllBackends(context.Background(), backends, "test", 5)
	if results[0].err != nil {
		t.Errorf("ok backend should succeed: %v", results[0].err)
	}
	if results[1].err == nil {
		t.Error("broken backend should fail")
	}
}

func TestSearchAllBackends_Timeout(t *testing.T) {
	backends := []SearchBackend{
		&mockSearchBackend{name: "fast", results: []SearchResult{{Title: "Fast"}}, delay: 50 * time.Millisecond},
		&mockSearchBackend{name: "very-slow", results: []SearchResult{{Title: "Slow"}}, delay: 5 * time.Second},
	}

	start := time.Now()
	results := searchAllBackends(context.Background(), backends, "test", 5)
	elapsed := time.Since(start)

	if elapsed > 3*time.Second {
		t.Errorf("timeout should cap at ~2s, took %v", elapsed)
	}
	if results[0].err != nil {
		t.Errorf("fast backend should succeed: %v", results[0].err)
	}
	if results[1].err == nil {
		t.Error("slow backend should have timed out")
	}
}

func TestIsFusionEnabled(t *testing.T) {
	tests := []struct {
		envVal       string
		backendCount int
		want         bool
	}{
		{"", 1, false},
		{"", 2, true},
		{"", 5, true},
		{"false", 3, false},
		{"0", 3, false},
		{"no", 3, false},
		{"true", 1, false},
		{"true", 2, true},
		{"yes", 3, true},
	}

	for _, tt := range tests {
		t.Setenv("SYNROUTE_SEARCH_FUSION", tt.envVal)
		got := isFusionEnabled(tt.backendCount)
		if got != tt.want {
			t.Errorf("isFusionEnabled(%d) with env=%q = %v, want %v", tt.backendCount, tt.envVal, got, tt.want)
		}
	}
}

func TestLLMRerank_NilCompleter(t *testing.T) {
	results := []SearchResult{
		{Title: "A", URL: "https://a.com"},
		{Title: "B", URL: "https://b.com"},
	}

	reranked := llmRerank(context.Background(), nil, "test", results)
	if len(reranked) != 2 || reranked[0].Title != "A" || reranked[1].Title != "B" {
		t.Error("nil completer should return results unchanged")
	}
}

func TestLLMRerank_SingleResult(t *testing.T) {
	results := []SearchResult{{Title: "Only", URL: "https://only.com"}}
	reranked := llmRerank(context.Background(), nil, "test", results)
	if len(reranked) != 1 || reranked[0].Title != "Only" {
		t.Error("single result should pass through unchanged")
	}
}

func TestExecuteFusion_BackwardCompat(t *testing.T) {
	tool := &WebSearchTool{
		backend: &mockSearchBackend{name: "single", results: []SearchResult{
			{Title: "Single Backend", URL: "https://example.com"},
		}},
		fusion: false,
	}

	result, err := tool.Execute(context.Background(), map[string]interface{}{"query": "test"}, ".")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("unexpected tool error: %s", result.Error)
	}
	if !contains(result.Output, "Single Backend") {
		t.Error("expected single backend result in output")
	}
}

func TestExecuteFusion_MultiBackend(t *testing.T) {
	tool := &WebSearchTool{
		backends: []SearchBackend{
			&mockSearchBackend{name: "a", results: []SearchResult{
				{Title: "Shared", URL: "https://example.com/shared"},
				{Title: "A Only", URL: "https://example.com/a"},
			}},
			&mockSearchBackend{name: "b", results: []SearchResult{
				{Title: "Shared", URL: "https://example.com/shared"},
				{Title: "B Only", URL: "https://example.com/b"},
			}},
		},
		fusion: true,
	}

	result, err := tool.Execute(context.Background(), map[string]interface{}{"query": "test"}, ".")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("unexpected tool error: %s", result.Error)
	}
	// Shared result should appear (consensus via RRF)
	if !contains(result.Output, "Shared") {
		t.Error("expected shared result in fused output")
	}
}
