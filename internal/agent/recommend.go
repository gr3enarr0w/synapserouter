package agent

import (
	"fmt"
	"strings"

	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/subscriptions"
)

// Recommendation holds model suggestions for one tier.
type Recommendation struct {
	Tier      string       // "cheap", "mid", "frontier"
	Models    []ModelEntry // recommended models, ranked by coding score
	Reasoning string       // why these were picked
}

// RecommendationReport is the full output of the recommendation engine.
type RecommendationReport struct {
	Profile        string
	AvailModels    int
	MatchedModels  int
	UnmatchedNames []string // models with no catalog data
	Tiers          []Recommendation
	SuggestedChain string   // OLLAMA_CHAIN format
	Warnings       []string
}

// GenerateRecommendation matches available models against the catalog and suggests tiers.
func GenerateRecommendation(available []subscriptions.ModelInfo, catalog []ModelEntry, profile string) *RecommendationReport {
	report := &RecommendationReport{
		Profile:     profile,
		AvailModels: len(available),
	}

	// Match available models against catalog
	var matched []ModelEntry
	for _, avail := range available {
		entry := LookupModel(catalog, avail.ID)
		if entry != nil {
			matched = append(matched, *entry)
		} else {
			report.UnmatchedNames = append(report.UnmatchedNames, avail.ID)
		}
	}
	report.MatchedModels = len(matched)

	if len(matched) == 0 {
		report.Warnings = append(report.Warnings, "No available models matched the catalog. Run synroute eval to benchmark your models.")
		return report
	}

	// Rank by coding score
	ranked := RankByCategory(matched, "coding")

	// Split into tiers based on scores
	var cheap, mid, frontier []ModelEntry
	for _, m := range ranked {
		switch {
		case m.HasTag("frontier") || m.Scores["coding"] >= 78:
			frontier = append(frontier, m)
		case m.Scores["coding"] >= 65:
			mid = append(mid, m)
		default:
			cheap = append(cheap, m)
		}
	}

	// If a tier is empty, redistribute
	if len(cheap) == 0 && len(mid) > 2 {
		cheap = mid[len(mid)-2:]
		mid = mid[:len(mid)-2]
	}
	if len(frontier) == 0 && len(mid) > 1 {
		frontier = mid[:1]
		mid = mid[1:]
	}

	// Build tier recommendations
	if len(cheap) > 0 {
		report.Tiers = append(report.Tiers, Recommendation{
			Tier:      "cheap",
			Models:    capModels(cheap, 5),
			Reasoning: "Fast, low-cost models for coding tasks and first-pass implementation",
		})
	}
	if len(mid) > 0 {
		report.Tiers = append(report.Tiers, Recommendation{
			Tier:      "mid",
			Models:    capModels(mid, 5),
			Reasoning: "Balanced models for self-check, planning, and code review",
		})
	}
	if len(frontier) > 0 {
		report.Tiers = append(report.Tiers, Recommendation{
			Tier:      "frontier",
			Models:    capModels(frontier, 5),
			Reasoning: "Highest-quality models for complex planning, review, and conversation",
		})
	}

	// Generate suggested OLLAMA_CHAIN
	report.SuggestedChain = generateChain(report.Tiers)

	// Warnings
	if len(report.UnmatchedNames) > 0 {
		report.Warnings = append(report.Warnings,
			fmt.Sprintf("%d models not in catalog (no benchmark data): %s",
				len(report.UnmatchedNames), strings.Join(report.UnmatchedNames[:minInt(3, len(report.UnmatchedNames))], ", ")))
	}
	if len(frontier) == 0 {
		report.Warnings = append(report.Warnings, "No frontier-tier models detected. Consider adding Claude, GPT, or Gemini Pro.")
	}
	if len(cheap) == 0 {
		report.Warnings = append(report.Warnings, "No cheap-tier models detected. Adding small models (Phi, Gemma, Ministral) would reduce costs.")
	}

	return report
}

// FormatRecommendation produces a human-readable report.
func FormatRecommendation(r *RecommendationReport) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "MODEL RECOMMENDATION (%s profile)\n", r.Profile)
	fmt.Fprintf(&sb, "Available: %d models | Matched: %d | Unmatched: %d\n\n",
		r.AvailModels, r.MatchedModels, len(r.UnmatchedNames))

	for _, tier := range r.Tiers {
		fmt.Fprintf(&sb, "=== %s TIER — %s ===\n", strings.ToUpper(tier.Tier), tier.Reasoning)
		fmt.Fprintf(&sb, "%-30s %-10s %-10s %-12s %s\n", "MODEL", "CODING", "PLANNING", "COST/QUERY", "TAGS")
		fmt.Fprintf(&sb, "%s\n", strings.Repeat("-", 85))

		for _, m := range tier.Models {
			codingScore := fmt.Sprintf("%.1f", m.Scores["coding"])
			planningScore := fmt.Sprintf("%.1f", m.Scores["planning"])
			cost := "free"
			if m.CostPerQuery() > 0 {
				cost = fmt.Sprintf("$%.4f", m.CostPerQuery())
			}
			tags := strings.Join(m.Tags, ", ")
			fmt.Fprintf(&sb, "%-30s %-10s %-10s %-12s %s\n", m.ID, codingScore, planningScore, cost, tags)
		}
		sb.WriteString("\n")
	}

	if r.SuggestedChain != "" {
		sb.WriteString("SUGGESTED OLLAMA_CHAIN:\n")
		fmt.Fprintf(&sb, "  %s\n\n", r.SuggestedChain)
	}

	for _, w := range r.Warnings {
		fmt.Fprintf(&sb, "WARNING: %s\n", w)
	}

	return sb.String()
}

// generateChain builds an OLLAMA_CHAIN string from tier recommendations.
func generateChain(tiers []Recommendation) string {
	var levels []string
	for _, tier := range tiers {
		var modelNames []string
		for _, m := range tier.Models {
			modelNames = append(modelNames, m.ID)
		}
		if len(modelNames) > 0 {
			levels = append(levels, strings.Join(modelNames, ","))
		}
	}
	return strings.Join(levels, "|")
}

func capModels(models []ModelEntry, n int) []ModelEntry {
	if len(models) <= n {
		return models
	}
	return models[:n]
}
