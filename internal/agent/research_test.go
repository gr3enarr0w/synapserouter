package agent

import (
	"context"
	"testing"

	"github.com/gr3enarr0w/synapserouter/internal/tools"
)

// mockBackend for testing backend selection
type mockTagBackend struct {
	name string
}

func (m *mockTagBackend) Name() string { return m.name }
func (m *mockTagBackend) Search(ctx context.Context, query string, maxResults int) ([]tools.SearchResult, error) {
	return nil, nil
}
func (m *mockTagBackend) CostPer1K() float64 { return 0 }

func TestClassifyQuery_Code(t *testing.T) {
	types := ClassifyQuery("fix the Go test function error")
	found := false
	for _, tp := range types {
		if tp == "code" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'code' in types, got %v", types)
	}
}

func TestClassifyQuery_Academic(t *testing.T) {
	types := ClassifyQuery("research papers on transformer algorithm")
	found := false
	for _, tp := range types {
		if tp == "academic" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'academic' in types, got %v", types)
	}
}

func TestClassifyQuery_News(t *testing.T) {
	types := ClassifyQuery("latest Kubernetes release announced 2026")
	found := false
	for _, tp := range types {
		if tp == "news" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'news' in types, got %v", types)
	}
}

func TestClassifyQuery_Multi(t *testing.T) {
	types := ClassifyQuery("latest research papers on Go runtime algorithm 2026")
	if len(types) < 2 {
		t.Errorf("expected multi-label, got %v", types)
	}
}

func TestClassifyQuery_General(t *testing.T) {
	types := ClassifyQuery("how does the weather work")
	if len(types) != 1 || types[0] != "web" {
		t.Errorf("expected ['web'], got %v", types)
	}
}

func TestTagBackends(t *testing.T) {
	backends := []tools.SearchBackend{
		&mockTagBackend{name: "duckduckgo"},
		&mockTagBackend{name: "serper"},
		&mockTagBackend{name: "semantic-scholar"},
		&mockTagBackend{name: "github"},
	}

	tagged := TagBackends(backends)
	if len(tagged) != 4 {
		t.Fatalf("expected 4 tagged, got %d", len(tagged))
	}

	// DDG should be free + web
	if tagged[0].CostTier != "free" {
		t.Errorf("DDG should be free, got %s", tagged[0].CostTier)
	}
	if !containsTag(tagged[0].Tags, "web") {
		t.Error("DDG should have 'web' tag")
	}

	// Semantic Scholar should be academic + free
	if !containsTag(tagged[2].Tags, "academic") {
		t.Error("Semantic Scholar should have 'academic' tag")
	}

	// GitHub should be code + free
	if !containsTag(tagged[3].Tags, "code") {
		t.Error("GitHub should have 'code' tag")
	}
}

func TestSelectBackends_QuickFreeOnly(t *testing.T) {
	tagged := []BackendTag{
		{Backend: &mockTagBackend{name: "ddg"}, Tags: []string{"web"}, CostTier: "free"},
		{Backend: &mockTagBackend{name: "serper"}, Tags: []string{"web"}, CostTier: "cheap"},
		{Backend: &mockTagBackend{name: "tavily"}, Tags: []string{"web"}, CostTier: "mid"},
	}

	selected := SelectBackends(tagged, []string{"web"}, "quick")

	// Quick = free only
	for _, b := range selected {
		if b.Name() != "ddg" {
			t.Errorf("quick should only select free backends, got %s", b.Name())
		}
	}
}

func TestSelectBackends_StandardCapped(t *testing.T) {
	var tagged []BackendTag
	for i := 0; i < 10; i++ {
		tagged = append(tagged, BackendTag{
			Backend:  &mockTagBackend{name: "backend"},
			Tags:     []string{"web"},
			CostTier: "mid",
		})
	}

	selected := SelectBackends(tagged, []string{"web"}, "standard")
	if len(selected) > 5 {
		t.Errorf("standard should cap at 5, got %d", len(selected))
	}
}

func TestSelectBackends_Fallback(t *testing.T) {
	tagged := []BackendTag{
		{Backend: &mockTagBackend{name: "ddg"}, Tags: []string{"web"}, CostTier: "free"},
		{Backend: &mockTagBackend{name: "serper"}, Tags: []string{"web"}, CostTier: "cheap"},
	}

	// Query is "code" but no code backends — should fallback to web
	selected := SelectBackends(tagged, []string{"code"}, "standard")
	if len(selected) == 0 {
		t.Error("should fallback to web backends when no code backends available")
	}
	if selected[0].Name() != "ddg" {
		t.Errorf("fallback should prefer free, got %s", selected[0].Name())
	}
}

func TestSelectBackends_FreeFirst(t *testing.T) {
	tagged := []BackendTag{
		{Backend: &mockTagBackend{name: "tavily"}, Tags: []string{"web"}, CostTier: "mid"},
		{Backend: &mockTagBackend{name: "ddg"}, Tags: []string{"web"}, CostTier: "free"},
		{Backend: &mockTagBackend{name: "kagi"}, Tags: []string{"web"}, CostTier: "expensive"},
		{Backend: &mockTagBackend{name: "serper"}, Tags: []string{"web"}, CostTier: "cheap"},
	}

	selected := SelectBackends(tagged, []string{"web"}, "deep")
	if selected[0].Name() != "ddg" {
		t.Errorf("first should be free (ddg), got %s", selected[0].Name())
	}
	if selected[len(selected)-1].Name() != "kagi" {
		t.Errorf("last should be expensive (kagi), got %s", selected[len(selected)-1].Name())
	}
}

func TestSelectBackends_Empty(t *testing.T) {
	selected := SelectBackends(nil, []string{"web"}, "quick")
	if selected != nil {
		t.Error("nil tagged should return nil")
	}
}

func TestDecomposeQuery(t *testing.T) {
	queries := DecomposeQuery("Go error handling patterns", 5)
	if len(queries) == 0 {
		t.Fatal("expected at least 1 query")
	}
	if queries[0] != "Go error handling patterns" {
		t.Errorf("first query should be original, got %q", queries[0])
	}
	if len(queries) < 3 {
		t.Errorf("expected at least 3 variations, got %d", len(queries))
	}
}

func TestDecomposeQuery_Cap(t *testing.T) {
	queries := DecomposeQuery("Go error handling patterns", 2)
	if len(queries) > 2 {
		t.Errorf("should cap at 2, got %d", len(queries))
	}
}

func TestDecomposeQuery_Short(t *testing.T) {
	queries := DecomposeQuery("Go", 5)
	// Very short query — might not generate many variations
	if len(queries) == 0 {
		t.Fatal("should return at least the original")
	}
}

func TestIsSaturated(t *testing.T) {
	if IsSaturated(0, 0) {
		t.Error("0/0 should not be saturated")
	}
	if IsSaturated(5, 10) {
		t.Error("5/10 = 50% should not be saturated")
	}
	if !IsSaturated(1, 100) {
		t.Error("1/100 = 1% should be saturated")
	}
	if IsSaturated(10, 100) {
		t.Error("10/100 = 10% should NOT be saturated (threshold is <10%)")
	}
	if !IsSaturated(9, 100) {
		t.Error("9/100 = 9% should be saturated")
	}
}

func TestDefaultResearchConfig(t *testing.T) {
	quick := DefaultResearchConfig("quick")
	if quick.MaxRounds != 1 || quick.MaxQueries != 3 {
		t.Errorf("quick: rounds=%d queries=%d", quick.MaxRounds, quick.MaxQueries)
	}

	standard := DefaultResearchConfig("standard")
	if standard.MaxRounds != 3 || standard.MaxQueries != 5 {
		t.Errorf("standard: rounds=%d queries=%d", standard.MaxRounds, standard.MaxQueries)
	}

	deep := DefaultResearchConfig("deep")
	if deep.MaxRounds != 5 || deep.MaxQueries != 10 {
		t.Errorf("deep: rounds=%d queries=%d", deep.MaxRounds, deep.MaxQueries)
	}
}
