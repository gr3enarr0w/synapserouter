package agent

import (
	"embed"
	"encoding/json"
	"log"
	"sort"
	"strings"
)

//go:embed model_catalog.json
var catalogData embed.FS

// ModelEntry represents a model in the bundled catalog with benchmark scores and pricing.
type ModelEntry struct {
	ID            string             `json:"id"`
	Provider      string             `json:"provider"`
	ContextWindow int                `json:"context"`
	InputPrice    float64            `json:"input_price"`  // $/1M tokens
	OutputPrice   float64            `json:"output_price"` // $/1M tokens
	Scores        map[string]float64 `json:"scores"`       // "coding": 79.6, "planning": 85.0, etc.
	Tags          []string           `json:"tags"`          // "frontier", "fast", "cheap", "open-source"
	Released      string             `json:"released"`      // "2026-01"
}

// LoadModelCatalog loads the embedded model catalog.
func LoadModelCatalog() []ModelEntry {
	data, err := catalogData.ReadFile("model_catalog.json")
	if err != nil {
		log.Printf("[ModelCatalog] failed to read embedded catalog: %v", err)
		return nil
	}

	var entries []ModelEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		log.Printf("[ModelCatalog] failed to parse catalog: %v", err)
		return nil
	}

	return entries
}

// LookupModel finds a model by ID with fuzzy matching.
// "claude-sonnet" matches "claude-sonnet-4-6", "deepseek-v3" matches "deepseek-v3.1".
func LookupModel(catalog []ModelEntry, id string) *ModelEntry {
	lower := strings.ToLower(id)

	// Exact match first
	for i, e := range catalog {
		if strings.ToLower(e.ID) == lower {
			return &catalog[i]
		}
	}

	// Fuzzy: check if catalog ID contains the query or vice versa
	for i, e := range catalog {
		eLower := strings.ToLower(e.ID)
		if strings.Contains(eLower, lower) || strings.Contains(lower, eLower) {
			return &catalog[i]
		}
	}

	// Fuzzy: strip version suffixes and try again
	// "qwen3.5:cloud" → "qwen3.5", "gemma3:12b-cloud" → "gemma3"
	stripped := lower
	if idx := strings.Index(stripped, ":"); idx > 0 {
		stripped = stripped[:idx]
	}
	if idx := strings.Index(stripped, "-cloud"); idx > 0 {
		stripped = stripped[:idx]
	}

	for i, e := range catalog {
		eLower := strings.ToLower(e.ID)
		if strings.Contains(eLower, stripped) || strings.Contains(stripped, eLower) {
			return &catalog[i]
		}
	}

	return nil
}

// ModelsForProvider returns all catalog entries for a given provider.
func ModelsForProvider(catalog []ModelEntry, provider string) []ModelEntry {
	lower := strings.ToLower(provider)
	var filtered []ModelEntry
	for _, e := range catalog {
		if strings.ToLower(e.Provider) == lower {
			filtered = append(filtered, e)
		}
	}
	return filtered
}

// RankByCategory sorts models by score in a given category (descending).
// Models without a score for the category are ranked last.
func RankByCategory(models []ModelEntry, category string) []ModelEntry {
	sorted := make([]ModelEntry, len(models))
	copy(sorted, models)

	sort.Slice(sorted, func(i, j int) bool {
		scoreI := sorted[i].Scores[category]
		scoreJ := sorted[j].Scores[category]
		if scoreI != scoreJ {
			return scoreI > scoreJ
		}
		// Tiebreak: cheaper first
		return sorted[i].InputPrice < sorted[j].InputPrice
	})

	return sorted
}

// HasTag checks if a model entry has a specific tag.
func (e ModelEntry) HasTag(tag string) bool {
	for _, t := range e.Tags {
		if t == tag {
			return true
		}
	}
	return false
}

// CostPerQuery estimates cost for a typical coding query (~2K input, ~1K output tokens).
func (e ModelEntry) CostPerQuery() float64 {
	return (e.InputPrice * 2000 / 1_000_000) + (e.OutputPrice * 1000 / 1_000_000)
}
