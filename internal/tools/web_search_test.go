package tools

import (
	"context"
	"testing"
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
