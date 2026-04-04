package tools

import (
	"testing"
)

func TestResultFilter_GetDomainScore(t *testing.T) {
	rf := NewResultFilter(nil)

	tests := []struct {
		name     string
		url      string
		wantMin  float64
		wantMax  float64
	}{
		{
			name:    "org_domain",
			url:     "https://example.org/page",
			wantMin: 0.75,
			wantMax: 0.75,
		},
		{
			name:    "edu_domain",
			url:     "https://example.edu/page",
			wantMin: 0.85,
			wantMax: 0.85,
		},
		{
			name:    "gov_domain",
			url:     "https://example.gov/page",
			wantMin: 0.85,
			wantMax: 0.85,
		},
		{
			name:    "github_domain",
			url:     "https://github.com/user/repo",
			wantMin: 0.95,
			wantMax: 0.95,
		},
		{
			name:    "unknown_domain",
			url:     "https://unknown.com/page",
			wantMin: 0.5,
			wantMax: 0.5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rf.getDomainScore(tt.url)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("getDomainScore(%q) = %v, want between %v and %v", tt.url, got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestResultFilter_GetRelevanceScore(t *testing.T) {
	rf := NewResultFilter(nil)

	tests := []struct {
		name    string
		result  SearchResult
		query   string
		wantMin float64
		wantMax float64
	}{
		{
			name: "low_relevance_-_no_match",
			result: SearchResult{
				Title:   "Some unrelated title",
				Snippet: "Some unrelated content",
				URL:     "https://example.com/page",
			},
			query:   "golang",
			wantMin: 0.0,
			wantMax: 0.333, // URL contains query term, so score is 1/3
		},
		{
			name: "high_relevance_-_title_match",
			result: SearchResult{
				Title:   "Golang Tutorial",
				Snippet: "Learn golang programming",
				URL:     "https://example.com/golang",
			},
			query:   "golang",
			wantMin: 0.66,
			wantMax: 1.0,
		},
		{
			name: "medium_relevance_-_snippet_match",
			result: SearchResult{
				Title:   "Programming Tutorial",
				Snippet: "Learn golang here",
				URL:     "https://example.com/page",
			},
			query:   "golang",
			wantMin: 0.33,
			wantMax: 0.34,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rf.getRelevanceScore(tt.result, tt.query)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("getRelevanceScore(%v, %q) = %v, want between %v and %v", tt.result, tt.query, got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestResultFilter_GetRecencyScore(t *testing.T) {
	rf := NewResultFilter(nil)

	tests := []struct {
		name    string
		snippet string
		wantMin float64
		wantMax float64
	}{
		{
			name:    "old_year",
			snippet: "This article from 2020 discusses programming",
			wantMin: 0.5,
			wantMax: 0.5, // 2020 not in recency patterns, returns neutral 0.5
		},
		{
			name:    "recent_year_2024",
			snippet: "This article from 2024 discusses programming",
			wantMin: 0.8,
			wantMax: 0.8,
		},
		{
			name:    "recent_year_2025",
			snippet: "This article from 2025 discusses programming",
			wantMin: 1.0,
			wantMax: 1.0,
		},
		{
			name:    "no_date",
			snippet: "This article discusses programming",
			wantMin: 0.5,
			wantMax: 0.5, // No date info, returns neutral 0.5
		},
		{
			name:    "relative_recent",
			snippet: "This article was published recently",
			wantMin: 0.8,
			wantMax: 0.8,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rf.getRecencyScore(tt.snippet)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("getRecencyScore(%q) = %v, want between %v and %v", tt.snippet, got, tt.wantMin, tt.wantMax)
			}
		})
	}
}
