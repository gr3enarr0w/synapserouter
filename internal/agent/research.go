package agent

import (
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/gr3enarr0w/synapserouter/internal/tools"
)

// ResearchConfig controls research depth, rounds, and budget.
type ResearchConfig struct {
	Depth       string // "quick", "standard", "deep"
	MaxRounds   int
	MaxQueries  int // per round
	MaxAPICalls int // hard budget ceiling
}

// ResearchHit is a single search result from one backend in one round.
type ResearchHit struct {
	URL          string
	Title        string
	Snippet      string
	Source       string  // backend name
	Round        int
	QualityScore float64 // 0-1 quality score
}

// ResearchRound holds the results of one search round.
type ResearchRound struct {
	Queries  []string
	Hits     []ResearchHit
	NewURLs  int // URLs not seen in prior rounds
	APICalls int
}

// ResearchReport is the final output of a research session.
type ResearchReport struct {
	Query      string
	Depth      string
	Rounds     []ResearchRound
	Findings   string   // synthesized report with [1], [2] citations
	Citations  []string // URL list matching citation numbers
	TotalHits  int
	UniqueURLs int
	APICalls   int
}

// BackendTag associates a search backend with capability tags and cost tier.
type BackendTag struct {
	Backend  tools.SearchBackend
	Name     string
	Tags     []string // "web", "code", "academic", "news"
	CostTier string   // "free", "cheap", "mid", "expensive"
}

// Cost tier sort order (lower = preferred).
var costTierOrder = map[string]int{
	"free":      0,
	"cheap":     1,
	"mid":       2,
	"expensive": 3,
}

// Backend tag registry — maps backend Name() to tags + cost tier.
var backendTagRegistry = map[string]struct {
	tags     []string
	costTier string
}{
	"duckduckgo":       {[]string{"web"}, "free"},
	"searxng":          {[]string{"web"}, "free"},
	"semantic-scholar": {[]string{"academic"}, "free"},
	"openalex":         {[]string{"academic"}, "free"},
	"github":           {[]string{"code"}, "free"},
	"serper":           {[]string{"web"}, "cheap"},
	"brave":            {[]string{"web"}, "cheap"},
	"newsapi":          {[]string{"news"}, "cheap"},
	"newsdata":         {[]string{"news"}, "cheap"},
	"thenewsapi":       {[]string{"news"}, "cheap"},
	"tavily":           {[]string{"web"}, "mid"},
	"exa":              {[]string{"web", "academic"}, "mid"},
	"serpapi":           {[]string{"web"}, "mid"},
	"searchapi":        {[]string{"web"}, "mid"},
	"you":              {[]string{"web"}, "mid"},
	"linkup":           {[]string{"web"}, "mid"},
	"jina":             {[]string{"web"}, "mid"},
	"sourcegraph":      {[]string{"code"}, "mid"},
	"kagi":             {[]string{"web"}, "expensive"},
}

// Query classification keywords.
var queryTypeKeywords = map[string][]string{
	"code":     {"function", "func", "class", "struct", "api", "library", "package", "import", "error", "bug", "test", "compile", "runtime", "debug", "syntax", "golang", "python", "javascript", "rust", "java", "typescript", "react", "node", "npm", "pip", "cargo", "module", "interface", "method", "variable", "type"},
	"academic": {"paper", "research", "study", "journal", "published", "theory", "algorithm", "arxiv", "citation", "peer-reviewed", "hypothesis", "methodology", "abstract", "proceedings", "conference", "survey", "benchmark", "dataset", "evaluation", "analysis"},
	"news":     {"latest", "today", "yesterday", "announced", "released", "update", "breaking", "2026", "2025", "launch", "acquisition", "funding", "startup", "cve", "vulnerability", "incident", "outage", "deprecated", "end-of-life"},
}

// DefaultResearchConfig returns config for a depth tier.
func DefaultResearchConfig(depth string) ResearchConfig {
	budget := 50
	if envBudget := os.Getenv("SYNROUTE_RESEARCH_BUDGET"); envBudget != "" {
		if n, err := strconv.Atoi(envBudget); err == nil && n > 0 {
			budget = n
		}
	}

	switch depth {
	case "quick":
		return ResearchConfig{Depth: "quick", MaxRounds: 1, MaxQueries: 3, MaxAPICalls: minInt(3, budget)}
	case "deep":
		return ResearchConfig{Depth: "deep", MaxRounds: 5, MaxQueries: 10, MaxAPICalls: minInt(100, budget)}
	default: // "standard"
		return ResearchConfig{Depth: "standard", MaxRounds: 3, MaxQueries: 5, MaxAPICalls: minInt(30, budget)}
	}
}

// TagBackends assigns capability tags to each configured backend.
func TagBackends(backends []tools.SearchBackend) []BackendTag {
	var tagged []BackendTag
	for _, b := range backends {
		name := b.Name()
		reg, ok := backendTagRegistry[name]
		if !ok {
			// Unknown backend — tag as general web, mid cost
			reg = struct {
				tags     []string
				costTier string
			}{[]string{"web"}, "mid"}
		}
		tagged = append(tagged, BackendTag{
			Backend:  b,
			Name:     name,
			Tags:     reg.tags,
			CostTier: reg.costTier,
		})
	}
	return tagged
}

// ClassifyQuery detects query type(s) from keywords. Multi-label.
// Returns ["web"] if no specific type matches.
func ClassifyQuery(query string) []string {
	lower := strings.ToLower(query)
	words := strings.Fields(lower)

	typeScores := make(map[string]int)
	for qType, keywords := range queryTypeKeywords {
		for _, word := range words {
			word = strings.Trim(word, ".,;:!?()[]{}\"'`-")
			for _, kw := range keywords {
				if word == kw {
					typeScores[qType]++
				}
			}
		}
	}

	var types []string
	for qType, score := range typeScores {
		if score >= 1 {
			types = append(types, qType)
		}
	}

	if len(types) == 0 {
		return []string{"web"}
	}

	// Sort for determinism
	sort.Strings(types)
	return types
}

// SelectBackends picks the right backends from what's available based on
// query type and depth tier. Free backends first, caps by depth.
func SelectBackends(tagged []BackendTag, queryTypes []string, depth string) []tools.SearchBackend {
	if len(tagged) == 0 {
		return nil
	}

	// Filter: backends whose tags overlap with query types
	var matched []BackendTag
	for _, bt := range tagged {
		if tagsOverlap(bt.Tags, queryTypes) {
			matched = append(matched, bt)
		}
	}

	// Fallback: if no specialized match, use all web-tagged backends
	if len(matched) == 0 {
		for _, bt := range tagged {
			if containsTag(bt.Tags, "web") {
				matched = append(matched, bt)
			}
		}
	}

	// Still nothing? Use everything available
	if len(matched) == 0 {
		matched = tagged
	}

	// Sort by cost tier (free first)
	sort.Slice(matched, func(i, j int) bool {
		return costTierOrder[matched[i].CostTier] < costTierOrder[matched[j].CostTier]
	})

	// Cap by depth
	maxBackends := len(matched)
	switch depth {
	case "quick":
		// Free only
		var freeOnly []BackendTag
		for _, bt := range matched {
			if bt.CostTier == "free" {
				freeOnly = append(freeOnly, bt)
			}
		}
		if len(freeOnly) > 0 {
			matched = freeOnly
		}
		if maxBackends > 3 {
			maxBackends = 3
		}
	case "standard":
		if maxBackends > 5 {
			maxBackends = 5
		}
	case "deep":
		// Cap at 7 backends for deep research
		if maxBackends > 7 {
			maxBackends = 7
		}
	}

	if len(matched) > maxBackends {
		matched = matched[:maxBackends]
	}

	backends := make([]tools.SearchBackend, len(matched))
	for i, bt := range matched {
		backends[i] = bt.Backend
	}
	return backends
}

// DecomposeQuery splits a complex query into sub-queries for broader coverage.
// Keyword-based, no LLM calls, <1ms.
func DecomposeQuery(query string, maxQueries int) []string {
	if maxQueries <= 0 {
		maxQueries = 5
	}

	queries := []string{query} // always include original

	// Extract key terms (>3 chars, not stop words)
	var terms []string
	for _, word := range strings.Fields(strings.ToLower(query)) {
		word = strings.Trim(word, ".,;:!?()[]{}\"'`-")
		if len(word) > 3 && !researchStopWords[word] {
			terms = append(terms, word)
		}
	}

	if len(terms) >= 2 {
		// Generate variations
		mainTopic := strings.Join(terms, " ")
		variations := []string{
			mainTopic + " best practices",
			mainTopic + " tutorial guide",
			mainTopic + " vs alternatives comparison",
			mainTopic + " examples",
			mainTopic + " common issues problems",
		}
		for _, v := range variations {
			if len(queries) >= maxQueries {
				break
			}
			queries = append(queries, v)
		}
	}

	if len(queries) > maxQueries {
		queries = queries[:maxQueries]
	}
	return queries
}

// IsSaturated returns true when new results are <10% of total (diminishing returns).
func IsSaturated(newURLs, totalURLs int) bool {
	if totalURLs == 0 {
		return false
	}
	return float64(newURLs)/float64(totalURLs) < 0.1
}

// --- helpers ---

func tagsOverlap(a, b []string) bool {
	for _, tagA := range a {
		for _, tagB := range b {
			if tagA == tagB {
				return true
			}
		}
	}
	return false
}

func containsTag(tags []string, target string) bool {
	for _, t := range tags {
		if t == target {
			return true
		}
	}
	return false
}

var researchStopWords = map[string]bool{
	"what": true, "how": true, "does": true, "this": true, "that": true,
	"with": true, "from": true, "about": true, "into": true, "through": true,
	"have": true, "been": true, "being": true, "would": true, "could": true,
	"should": true, "will": true, "just": true, "also": true, "very": true,
	"some": true, "more": true, "most": true, "other": true, "each": true,
	"make": true, "like": true, "when": true, "where": true, "which": true,
	"their": true, "there": true, "they": true, "them": true, "then": true,
	"than": true, "these": true, "those": true, "your": true, "help": true,
	"please": true, "want": true, "need": true,
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
