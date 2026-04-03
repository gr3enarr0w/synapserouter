package agent

import (
	"fmt"
	"math/rand"
	"regexp"
	"sort"
	"strings"
)

// KReviewConfig controls K-LLM parallel verification.
// When K > 1, a review phase spawns K independent reviewers in parallel,
// each seeing a shuffled file inspection order. Findings are merged by
// file region + root cause, with agreement scoring.
type KReviewConfig struct {
	K               int     // number of parallel reviewers (default 2)
	AgreementThresh float64 // fraction of reviewers that must agree (default 0.5)
}

// ReviewFinding is a single issue extracted from a reviewer's output.
type ReviewFinding struct {
	File       string // file path (normalized, relative to workdir)
	LineRange  [2]int // approximate start/end lines (0,0 if unknown)
	Category   string // e.g. "null-value", "missing-check", "spec-violation"
	Summary    string // one-line description
	RawText    string // verbatim excerpt from the reviewer
	ReviewerID int    // which reviewer (0..K-1) reported this
}

// FindingCluster groups similar findings from multiple reviewers.
type FindingCluster struct {
	RootCause  string          // canonical description of the issue
	File       string          // file (or "multiple" if cross-file)
	LineRange  [2]int          // representative line range
	Findings   []ReviewFinding // all findings in this cluster
	Agreement  float64         // fraction of K reviewers that reported this
	Confidence string          // "high" (>threshold), "low", or "disputed"
}

// KReviewResult is the merged output of K parallel reviewers.
type KReviewResult struct {
	K               int
	ReviewerResults []string         // raw output from each reviewer
	Clusters        []FindingCluster // all merged findings
	HighConfidence  []FindingCluster // agreement > threshold
	Disagreements   []FindingCluster // reported by minority only
	AllPassed       bool             // true only if ALL K reviewers passed
	MajorityPassed  bool             // true if >50% of reviewers passed
}

// Regex patterns for parsing structured findings from reviewer output.
var (
	// Matches: [FILE: path/to/file.go:42] CATEGORY: description
	structuredFindingRe = regexp.MustCompile(`\[FILE:\s*([^\]:]+?)(?::(\d+)(?:-(\d+))?)?\]\s*(\w[\w-]*):\s*(.+)`)

	// Fallback: matches lines with file:line references like "path/file.go:42 — some issue"
	fileRefRe = regexp.MustCompile(`(\S+\.\w{1,5}):(\d+)\b`)

	// Matches bullet-pointed issues: "- FAIL:", "* Issue:", "- NEEDS_FIX:"
	bulletIssueRe = regexp.MustCompile(`^[\s]*[-*]\s+(?:FAIL|NEEDS_FIX|Issue|Problem|Bug|Error|Warning|ISSUE):\s*(.+)`)
)

// ParseFindings extracts structured findings from a reviewer's free-text output.
// It tries the structured format first, then falls back to heuristic extraction.
func ParseFindings(reviewerID int, output string) []ReviewFinding {
	var findings []ReviewFinding
	seen := make(map[string]bool) // dedup by file+summary

	// Pass 1: structured format [FILE: path:LINE] CATEGORY: desc
	for _, match := range structuredFindingRe.FindAllStringSubmatch(output, -1) {
		file := strings.TrimSpace(match[1])
		startLine := atoiSafe(match[2])
		endLine := atoiSafe(match[3])
		if endLine == 0 {
			endLine = startLine
		}
		category := strings.ToLower(strings.TrimSpace(match[4]))
		summary := strings.TrimSpace(match[5])

		key := file + "|" + summary
		if seen[key] {
			continue
		}
		seen[key] = true

		findings = append(findings, ReviewFinding{
			File:       file,
			LineRange:  [2]int{startLine, endLine},
			Category:   category,
			Summary:    summary,
			RawText:    match[0],
			ReviewerID: reviewerID,
		})
	}

	// Pass 2: fallback — bullet issues with file references
	if len(findings) == 0 {
		lines := strings.Split(output, "\n")
		for _, line := range lines {
			if bulletIssueRe.MatchString(line) {
				summary := bulletIssueRe.FindStringSubmatch(line)[1]
				file := ""
				startLine := 0

				if fileMatch := fileRefRe.FindStringSubmatch(line); fileMatch != nil {
					file = fileMatch[1]
					startLine = atoiSafe(fileMatch[2])
				}

				category := classifyCategory(summary)
				key := file + "|" + summary
				if seen[key] {
					continue
				}
				seen[key] = true

				findings = append(findings, ReviewFinding{
					File:       file,
					LineRange:  [2]int{startLine, startLine},
					Category:   category,
					Summary:    summary,
					RawText:    strings.TrimSpace(line),
					ReviewerID: reviewerID,
				})
			}
		}
	}

	return findings
}

// ClusterFindings groups findings from K reviewers by file + root cause similarity.
// Returns clusters sorted by agreement (descending), then by file path.
func ClusterFindings(allFindings []ReviewFinding, k int, threshold float64) []FindingCluster {
	if len(allFindings) == 0 || k <= 0 {
		return nil
	}

	// Group by file first
	byFile := make(map[string][]ReviewFinding)
	for _, f := range allFindings {
		key := f.File
		if key == "" {
			key = "_unknown"
		}
		byFile[key] = append(byFile[key], f)
	}

	var clusters []FindingCluster

	for file, findings := range byFile {
		// Sort by start line for proximity grouping
		sort.Slice(findings, func(i, j int) bool {
			return findings[i].LineRange[0] < findings[j].LineRange[0]
		})

		// Cluster by line proximity + category/summary similarity
		used := make([]bool, len(findings))
		for i := 0; i < len(findings); i++ {
			if used[i] {
				continue
			}
			cluster := []ReviewFinding{findings[i]}
			used[i] = true

			for j := i + 1; j < len(findings); j++ {
				if used[j] {
					continue
				}
				if areSimilarFindings(findings[i], findings[j]) {
					cluster = append(cluster, findings[j])
					used[j] = true
				}
			}

			// Count unique reviewers in this cluster
			reviewers := make(map[int]bool)
			for _, f := range cluster {
				reviewers[f.ReviewerID] = true
			}
			agreement := float64(len(reviewers)) / float64(k)

			confidence := "low"
			if agreement > threshold {
				confidence = "high"
			}
			if len(reviewers) > 1 && agreement <= threshold {
				confidence = "disputed"
			}

			// Use the first finding as the representative
			representative := cluster[0]
			displayFile := file
			if displayFile == "_unknown" {
				displayFile = ""
			}

			clusters = append(clusters, FindingCluster{
				RootCause:  representative.Summary,
				File:       displayFile,
				LineRange:  representative.LineRange,
				Findings:   cluster,
				Agreement:  agreement,
				Confidence: confidence,
			})
		}
	}

	// Sort: high agreement first, then by file for determinism
	sort.Slice(clusters, func(i, j int) bool {
		if clusters[i].Agreement != clusters[j].Agreement {
			return clusters[i].Agreement > clusters[j].Agreement
		}
		return clusters[i].File < clusters[j].File
	})

	return clusters
}

// FormatMergedReview formats a KReviewResult as a string compatible with
// IsPassSignal/IsFailSignal detection.
func FormatMergedReview(result KReviewResult) string {
	if result.K <= 0 {
		return "NEEDS_FIX: invalid K-LLM config (K=0)\n"
	}

	var b strings.Builder

	passCount := 0
	for _, output := range result.ReviewerResults {
		if IsPassSignal(output) {
			passCount++
		}
	}

	fmt.Fprintf(&b, "K-LLM REVIEW (K=%d, %d/%d reviewers passed)\n\n", result.K, passCount, result.K)

	if result.AllPassed {
		b.WriteString("All reviewers agree: VERIFIED_CORRECT\n")
		return b.String()
	}

	// High confidence findings
	if len(result.HighConfidence) > 0 {
		b.WriteString("=== HIGH-CONFIDENCE FINDINGS (agreement > threshold) ===\n\n")
		for _, c := range result.HighConfidence {
			formatCluster(&b, c)
		}
		b.WriteString("\n")
	}

	// Disagreements
	if len(result.Disagreements) > 0 {
		b.WriteString("=== DISAGREEMENTS (reported by minority) ===\n\n")
		for _, c := range result.Disagreements {
			formatCluster(&b, c)
		}
		b.WriteString("\n")
	}

	// Individual summaries (truncated)
	b.WriteString("=== INDIVIDUAL REVIEWER SUMMARIES ===\n\n")
	for i, output := range result.ReviewerResults {
		status := "NEEDS_FIX"
		if IsPassSignal(output) {
			status = "VERIFIED_CORRECT"
		}
		// Extract first meaningful line as summary
		summary := extractFirstMeaningfulLine(output)
		fmt.Fprintf(&b, "Reviewer %d: %s — %s\n", i+1, status, summary)
	}

	// Emit the appropriate signal for pipeline consumption
	if result.MajorityPassed && len(result.HighConfidence) == 0 {
		b.WriteString("\nVERIFIED_CORRECT (majority passed, no high-confidence issues)\n")
	} else {
		b.WriteString("\nNEEDS_FIX\n")
	}

	return b.String()
}

// ShuffleFileOrder returns a deterministic shuffle of file paths.
// Each reviewer index produces a different ordering.
func ShuffleFileOrder(files []string, seed int64) []string {
	if len(files) <= 1 {
		result := make([]string, len(files))
		copy(result, files)
		return result
	}

	result := make([]string, len(files))
	copy(result, files)

	r := rand.New(rand.NewSource(seed)) //nolint:G404 // used for shuffling review order, not security
	r.Shuffle(len(result), func(i, j int) {
		result[i], result[j] = result[j], result[i]
	})

	return result
}

// --- helpers ---

// areSimilarFindings checks if two findings are about the same issue.
// Uses line proximity (within 10 lines) AND category/summary similarity.
func areSimilarFindings(a, b ReviewFinding) bool {
	// Must be same file (already grouped by file before calling)
	hasLineInfo := a.LineRange[0] > 0 && b.LineRange[0] > 0

	// Check line proximity when both have line info
	if hasLineInfo {
		dist := a.LineRange[0] - b.LineRange[0]
		if dist < 0 {
			dist = -dist
		}
		if dist > 10 {
			return false
		}
	}

	// Exact category match
	if a.Category != "" && b.Category != "" && a.Category == b.Category {
		return true
	}

	// Jaccard similarity on summary words.
	// Require higher threshold when line info is missing to avoid over-merging.
	threshold := 0.4
	if !hasLineInfo {
		threshold = 0.6
	}
	return jaccardSimilarity(a.Summary, b.Summary) >= threshold
}

// jaccardSimilarity computes Jaccard index between word sets of two strings.
func jaccardSimilarity(a, b string) float64 {
	wordsA := wordSet(strings.ToLower(a))
	wordsB := wordSet(strings.ToLower(b))

	if len(wordsA) == 0 && len(wordsB) == 0 {
		return 1.0
	}

	intersection := 0
	for w := range wordsA {
		if wordsB[w] {
			intersection++
		}
	}

	union := len(wordsA)
	for w := range wordsB {
		if !wordsA[w] {
			union++
		}
	}

	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

// wordSet splits a string into a set of words.
func wordSet(s string) map[string]bool {
	set := make(map[string]bool)
	for _, w := range strings.Fields(s) {
		// Strip punctuation
		w = strings.Trim(w, ".,;:!?()[]{}\"'`")
		if len(w) > 1 {
			set[w] = true
		}
	}
	return set
}

// classifyCategory assigns a category based on keyword matching in the summary.
func classifyCategory(summary string) string {
	lower := strings.ToLower(summary)
	switch {
	case strings.Contains(lower, "null") || strings.Contains(lower, "nil") || strings.Contains(lower, "empty"):
		return "null-value"
	case strings.Contains(lower, "spec") || strings.Contains(lower, "scope") || strings.Contains(lower, "requirement"):
		return "spec-violation"
	case strings.Contains(lower, "error") || strings.Contains(lower, "handle") || strings.Contains(lower, "panic"):
		return "error-handling"
	case strings.Contains(lower, "test") || strings.Contains(lower, "assert"):
		return "testing"
	case strings.Contains(lower, "security") || strings.Contains(lower, "auth") || strings.Contains(lower, "inject"):
		return "security"
	case strings.Contains(lower, "performance") || strings.Contains(lower, "slow") || strings.Contains(lower, "leak"):
		return "performance"
	case strings.Contains(lower, "style") || strings.Contains(lower, "naming") || strings.Contains(lower, "convention"):
		return "style"
	default:
		return "general"
	}
}

// atoiSafe converts a string to int, returning 0 on error.
func atoiSafe(s string) int {
	if s == "" {
		return 0
	}
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// formatCluster writes a single finding cluster to a builder.
func formatCluster(b *strings.Builder, c FindingCluster) {
	loc := c.File
	if c.LineRange[0] > 0 {
		if c.LineRange[1] > c.LineRange[0] {
			loc = fmt.Sprintf("%s:%d-%d", c.File, c.LineRange[0], c.LineRange[1])
		} else {
			loc = fmt.Sprintf("%s:%d", c.File, c.LineRange[0])
		}
	}
	if loc == "" {
		loc = "(no file)"
	}

	reviewerCount := len(uniqueReviewers(c.Findings))
	fmt.Fprintf(b, "[%s] (%d reviewers, %.0f%% agreement, %s)\n", loc, reviewerCount, c.Agreement*100, c.Confidence)
	fmt.Fprintf(b, "  Root cause: %s\n", c.RootCause)
}

// uniqueReviewers counts distinct reviewer IDs in a set of findings.
func uniqueReviewers(findings []ReviewFinding) map[int]bool {
	m := make(map[int]bool)
	for _, f := range findings {
		m[f.ReviewerID] = true
	}
	return m
}

// extractFirstMeaningfulLine returns the first non-empty, non-boilerplate line.
func extractFirstMeaningfulLine(output string) string {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "---") || strings.HasPrefix(line, "===") {
			continue
		}
		if len(line) > 120 {
			return line[:117] + "..."
		}
		return line
	}
	return "(no output)"
}
