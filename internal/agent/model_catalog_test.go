package agent

import (
	"strings"
	"testing"

	"github.com/gr3enarr0w/synapserouter/internal/subscriptions"
)

func TestLoadModelCatalog(t *testing.T) {
	catalog := LoadModelCatalog()
	if len(catalog) == 0 {
		t.Fatal("catalog should have entries")
	}
	if len(catalog) < 20 {
		t.Errorf("expected at least 20 models, got %d", len(catalog))
	}

	// Verify structure
	for _, e := range catalog {
		if e.ID == "" {
			t.Error("model ID should not be empty")
		}
		if e.Provider == "" {
			t.Errorf("model %s: provider should not be empty", e.ID)
		}
		if e.ContextWindow == 0 {
			t.Errorf("model %s: context window should not be 0", e.ID)
		}
	}
}

func TestLookupModel_Exact(t *testing.T) {
	catalog := LoadModelCatalog()
	entry := LookupModel(catalog, "claude-sonnet-4-6")
	if entry == nil {
		t.Fatal("expected to find claude-sonnet-4-6")
	}
	if entry.Provider != "anthropic" {
		t.Errorf("provider = %q, want anthropic", entry.Provider)
	}
}

func TestLookupModel_Fuzzy(t *testing.T) {
	catalog := LoadModelCatalog()

	// "claude-sonnet" should match "claude-sonnet-4-6"
	entry := LookupModel(catalog, "claude-sonnet")
	if entry == nil {
		t.Fatal("fuzzy match should find claude-sonnet")
	}

	// "deepseek-v3" should match "deepseek-v3.1"
	entry = LookupModel(catalog, "deepseek-v3")
	if entry == nil {
		t.Fatal("fuzzy match should find deepseek-v3")
	}

	// Ollama-style names: "qwen3.5:cloud" should match "qwen3.5"
	entry = LookupModel(catalog, "qwen3.5:cloud")
	if entry == nil {
		t.Fatal("fuzzy match should find qwen3.5 from qwen3.5:cloud")
	}
}

func TestLookupModel_NotFound(t *testing.T) {
	catalog := LoadModelCatalog()
	entry := LookupModel(catalog, "nonexistent-model-xyz")
	if entry != nil {
		t.Error("should not find nonexistent model")
	}
}

func TestRankByCategory(t *testing.T) {
	models := []ModelEntry{
		{ID: "low", Scores: map[string]float64{"coding": 50.0}},
		{ID: "high", Scores: map[string]float64{"coding": 90.0}},
		{ID: "mid", Scores: map[string]float64{"coding": 70.0}},
	}

	ranked := RankByCategory(models, "coding")
	if ranked[0].ID != "high" {
		t.Errorf("first should be highest score, got %s", ranked[0].ID)
	}
	if ranked[2].ID != "low" {
		t.Errorf("last should be lowest score, got %s", ranked[2].ID)
	}
}

func TestModelsForProvider(t *testing.T) {
	catalog := LoadModelCatalog()
	anthropic := ModelsForProvider(catalog, "anthropic")
	if len(anthropic) < 2 {
		t.Errorf("expected at least 2 anthropic models, got %d", len(anthropic))
	}
	for _, m := range anthropic {
		if m.Provider != "anthropic" {
			t.Errorf("expected anthropic, got %s", m.Provider)
		}
	}
}

func TestGenerateRecommendation(t *testing.T) {
	catalog := LoadModelCatalog()
	available := []subscriptions.ModelInfo{
		{ID: "claude-sonnet-4-6"},
		{ID: "claude-haiku-4-5"},
		{ID: "deepseek-v3.1"},
		{ID: "gemma3"},
		{ID: "gemini-3-flash"},
	}

	report := GenerateRecommendation(available, catalog, "personal")

	if report.MatchedModels == 0 {
		t.Fatal("should match at least some models")
	}
	if len(report.Tiers) == 0 {
		t.Fatal("should produce at least 1 tier")
	}
	if report.SuggestedChain == "" {
		t.Error("should generate suggested chain")
	}
}

func TestGenerateRecommendation_NoMatch(t *testing.T) {
	catalog := LoadModelCatalog()
	available := []subscriptions.ModelInfo{
		{ID: "totally-unknown-model"},
	}

	report := GenerateRecommendation(available, catalog, "personal")
	if report.MatchedModels != 0 {
		t.Errorf("should match 0, got %d", report.MatchedModels)
	}
	if len(report.Warnings) == 0 {
		t.Error("should have warnings about unmatched models")
	}
}

func TestModelEntry_CostPerQuery(t *testing.T) {
	entry := ModelEntry{InputPrice: 3.0, OutputPrice: 15.0}
	cost := entry.CostPerQuery()
	// 3.0 * 2000/1M + 15.0 * 1000/1M = 0.006 + 0.015 = 0.021
	if cost < 0.02 || cost > 0.03 {
		t.Errorf("cost per query = %f, expected ~0.021", cost)
	}

	free := ModelEntry{InputPrice: 0, OutputPrice: 0}
	if free.CostPerQuery() != 0 {
		t.Error("free model should have 0 cost")
	}
}

func TestFormatRecommendation(t *testing.T) {
	report := &RecommendationReport{
		Profile:     "personal",
		AvailModels: 5,
		MatchedModels: 3,
		Tiers: []Recommendation{
			{Tier: "cheap", Models: []ModelEntry{{ID: "gemma3", Scores: map[string]float64{"coding": 60}, Tags: []string{"free"}}}, Reasoning: "test"},
		},
		SuggestedChain: "gemma3",
	}

	output := FormatRecommendation(report)
	if output == "" {
		t.Error("should produce non-empty output")
	}
	if !strings.Contains(output, "CHEAP TIER") {
		t.Error("should contain tier header")
	}
	if !strings.Contains(output, "gemma3") {
		t.Error("should contain model name")
	}
}
