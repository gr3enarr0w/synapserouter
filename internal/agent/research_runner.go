package agent

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gr3enarr0w/synapserouter/internal/tools"
)

// RunResearch executes a multi-round research session with dynamic backend
// selection, budget enforcement, and saturation detection.
func (a *Agent) RunResearch(ctx context.Context, query, depth string) (*ResearchReport, error) {
	config := DefaultResearchConfig(depth)
	log.Printf("[Research] starting: depth=%s, maxRounds=%d, maxQueries=%d, budget=%d",
		config.Depth, config.MaxRounds, config.MaxQueries, config.MaxAPICalls)

	// Tag all configured backends
	allBackends := tools.DetectAllBackends()
	tagged := TagBackends(allBackends)
	log.Printf("[Research] %d backends configured", len(tagged))

	// Classify query type for backend routing
	queryTypes := ClassifyQuery(query)
	log.Printf("[Research] query types: %v", queryTypes)

	// Select backends for this query + depth
	selected := SelectBackends(tagged, queryTypes, config.Depth)
	if len(selected) == 0 {
		return nil, fmt.Errorf("no search backends available")
	}

	backendNames := make([]string, len(selected))
	for i, b := range selected {
		backendNames[i] = b.Name()
	}
	log.Printf("[Research] selected %d backends: %s", len(selected), strings.Join(backendNames, ", "))

	// Track state across rounds
	report := &ResearchReport{
		Query: query,
		Depth: config.Depth,
	}
	seenURLs := make(map[string]bool)
	totalAPICalls := 0

	for round := 0; round < config.MaxRounds; round++ {
		// Budget check
		if totalAPICalls >= config.MaxAPICalls {
			log.Printf("[Research] budget exhausted (%d/%d calls) — stopping", totalAPICalls, config.MaxAPICalls)
			break
		}

		// Decompose query for this round
		var subQueries []string
		if round == 0 {
			subQueries = DecomposeQuery(query, config.MaxQueries)
		} else {
			// Round 2+: generate follow-up queries based on gaps
			subQueries = generateFollowUpQueries(query, report.Rounds, config.MaxQueries)
		}

		roundResult := ResearchRound{
			Queries: subQueries,
		}

		// Execute searches for each sub-query
		for _, sq := range subQueries {
			if totalAPICalls >= config.MaxAPICalls {
				break
			}

			searchCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			results := tools.SearchSelectedBackends(searchCtx, selected, sq, 5)
			cancel()

			roundResult.APICalls += len(selected) // each backend = 1 API call
			totalAPICalls += len(selected)

			// Collect hits, dedup
			for _, br := range results {
				if br.Err != nil || len(br.Results) == 0 {
					continue
				}
				for _, sr := range br.Results {
					normURL := tools.NormalizeURL(sr.URL)
					if normURL == "" || seenURLs[normURL] {
						continue
					}
					seenURLs[normURL] = true
					roundResult.NewURLs++
					roundResult.Hits = append(roundResult.Hits, ResearchHit{
						URL:     sr.URL,
						Title:   sr.Title,
						Snippet: sr.Snippet,
						Source:  br.BackendName,
						Round:   round + 1,
					})
				}
			}
		}

		report.Rounds = append(report.Rounds, roundResult)
		report.TotalHits += len(roundResult.Hits)
		report.UniqueURLs = len(seenURLs)
		report.APICalls = totalAPICalls

		log.Printf("[Research] round %d: %d new URLs, %d total unique, %d/%d API calls",
			round+1, roundResult.NewURLs, len(seenURLs), totalAPICalls, config.MaxAPICalls)

		// Saturation check (skip on first round — need at least 2 to compare)
		if round > 0 && IsSaturated(roundResult.NewURLs, len(seenURLs)) {
			log.Printf("[Research] saturated (<10%% new URLs) — stopping early")
			break
		}
	}

	// Synthesize findings with citations
	report.Findings, report.Citations = SynthesizeFindings(report)

	// Store in tool output DB if available
	if a.config.ToolStore != nil {
		a.config.ToolStore.Store(
			a.sessionID, "research", query,
			fmt.Sprintf("Research: %s (%s, %d rounds, %d sources)", query, depth, len(report.Rounds), report.UniqueURLs),
			report.Findings, 0, len(report.Findings))
	}

	return report, nil
}

// generateFollowUpQueries creates queries for round 2+ based on gaps.
func generateFollowUpQueries(originalQuery string, previousRounds []ResearchRound, maxQueries int) []string {
	// Collect all titles/snippets from previous rounds to identify themes
	var coveredTerms []string
	for _, round := range previousRounds {
		for _, hit := range round.Hits {
			for _, word := range strings.Fields(strings.ToLower(hit.Title)) {
				word = strings.Trim(word, ".,;:!?()[]{}\"'`-")
				if len(word) > 4 {
					coveredTerms = append(coveredTerms, word)
				}
			}
		}
	}

	// Generate follow-up queries that explore different angles
	queries := []string{
		originalQuery + " advanced",
		originalQuery + " common mistakes pitfalls",
		originalQuery + " real world examples case study",
	}

	if len(queries) > maxQueries {
		queries = queries[:maxQueries]
	}
	return queries
}

// SynthesizeFindings formats research hits into a report with inline citations.
func SynthesizeFindings(report *ResearchReport) (string, []string) {
	if report == nil || report.TotalHits == 0 {
		return "No results found.", nil
	}

	var sb strings.Builder
	var citations []string
	citationMap := make(map[string]int) // URL → citation number

	fmt.Fprintf(&sb, "# Research: %s\n\n", report.Query)
	fmt.Fprintf(&sb, "**Depth:** %s | **Rounds:** %d | **Sources:** %d | **API calls:** %d\n\n",
		report.Depth, len(report.Rounds), report.UniqueURLs, report.APICalls)

	sb.WriteString("## Findings\n\n")

	// Group hits by round
	for _, round := range report.Rounds {
		for _, hit := range round.Hits {
			if hit.URL == "" || hit.Title == "" {
				continue
			}

			// Assign citation number
			citNum, exists := citationMap[hit.URL]
			if !exists {
				citNum = len(citations) + 1
				citationMap[hit.URL] = citNum
				citations = append(citations, hit.URL)
			}

			// Cap at 20 citations
			if citNum > 20 {
				continue
			}

			snippet := hit.Snippet
			if len(snippet) > 300 {
				snippet = snippet[:297] + "..."
			}

			fmt.Fprintf(&sb, "- **%s** [%d] — %s _(via %s)_\n", hit.Title, citNum, snippet, hit.Source)
		}
	}

	// Citations section
	if len(citations) > 0 {
		sb.WriteString("\n## Sources\n\n")
		for i, url := range citations {
			if i >= 20 {
				fmt.Fprintf(&sb, "_(+%d more sources)_\n", len(citations)-20)
				break
			}
			fmt.Fprintf(&sb, "[%d] %s\n", i+1, url)
		}
	}

	return sb.String(), citations
}
