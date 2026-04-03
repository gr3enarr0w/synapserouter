package eval

import (
	"math"
	"sort"
)

// Percentile computes the p-th percentile (0-100) from a sorted slice.
func Percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if len(sorted) == 1 {
		return sorted[0]
	}

	rank := p / 100.0 * float64(len(sorted)-1)
	lower := int(math.Floor(rank))
	upper := int(math.Ceil(rank))

	if lower == upper || upper >= len(sorted) {
		return sorted[lower]
	}

	// Linear interpolation
	weight := rank - float64(lower)
	return sorted[lower]*(1-weight) + sorted[upper]*weight
}

// ComputeSummary aggregates results into an EvalSummary.
func ComputeSummary(results []EvalResult) *EvalSummary {
	if len(results) == 0 {
		return &EvalSummary{
			ByProvider: make(map[string]*ProviderStats),
			ByLanguage: make(map[string]*LanguageStats),
		}
	}

	summary := &EvalSummary{
		TotalExercises: len(results),
		ByProvider:     make(map[string]*ProviderStats),
		ByLanguage:     make(map[string]*LanguageStats),
	}

	var totalLatency int64
	var pass1Count, pass2Count, fallbackCount int
	var totalMetricScore float64
	var totalCost float64
	var metricCount int
	var latencies []float64

	for _, r := range results {
		if r.Pass1 {
			pass1Count++
		}
		if r.Pass2 {
			pass2Count++
		}
		if r.FallbackUsed {
			fallbackCount++
		}
		totalLatency += r.LatencyMs
		latencies = append(latencies, float64(r.LatencyMs))
		summary.TotalTokens += r.TotalTokens
		totalCost += r.CostUSD

		if r.MetricScore != 0 || r.MetricName != "" {
			totalMetricScore += r.MetricScore
			metricCount++
		}

		// Per-provider stats
		ps, ok := summary.ByProvider[r.Provider]
		if !ok {
			ps = &ProviderStats{}
			summary.ByProvider[r.Provider] = ps
		}
		ps.Total++
		if r.Pass1 {
			ps.Pass1++
		}
		if r.Pass2 {
			ps.Pass2++
		}
		ps.AvgLatency += r.LatencyMs
		ps.Tokens += r.TotalTokens
		if r.MetricScore != 0 || r.MetricName != "" {
			ps.AvgMetricScore += r.MetricScore
			ps.MetricResults++
		}

		// Per-language stats — extract from exercise ID (suite/lang/slug)
		lang := extractLanguage(r.ExerciseID)
		ls, ok := summary.ByLanguage[lang]
		if !ok {
			ls = &LanguageStats{}
			summary.ByLanguage[lang] = ls
		}
		ls.Total++
		if r.Pass1 {
			ls.Pass1++
		}
		if r.Pass2 {
			ls.Pass2++
		}
		if r.MetricScore != 0 || r.MetricName != "" {
			ls.AvgMetricScore += r.MetricScore
			ls.MetricResults++
		}
	}

	n := float64(len(results))
	summary.Pass1Rate = float64(pass1Count) / n
	summary.Pass2Rate = float64(pass2Count) / n
	summary.AvgLatencyMs = totalLatency / int64(len(results))
	summary.FallbackRate = float64(fallbackCount) / n
	if metricCount > 0 {
		summary.AvgMetricScore = totalMetricScore / float64(metricCount)
		summary.MetricResults = metricCount
	}

	// Latency percentiles
	sort.Float64s(latencies)
	summary.LatencyP50 = Percentile(latencies, 50)
	summary.LatencyP95 = Percentile(latencies, 95)
	summary.LatencyP99 = Percentile(latencies, 99)

	// Throughput: tokens per second (across all results)
	if totalLatency > 0 {
		summary.TokensPerSec = float64(summary.TotalTokens) / (float64(totalLatency) / 1000.0)
	}

	// Cost metrics
	summary.TotalCost = totalCost
	if len(results) > 0 {
		summary.AvgCostPerEx = totalCost / n
	}
	if summary.AvgCostPerEx > 0 {
		summary.CostEfficiency = summary.Pass1Rate / summary.AvgCostPerEx
	}

	for _, ps := range summary.ByProvider {
		if ps.Total > 0 {
			ps.Pass1Rate = float64(ps.Pass1) / float64(ps.Total)
			ps.Pass2Rate = float64(ps.Pass2) / float64(ps.Total)
			ps.AvgLatency = ps.AvgLatency / int64(ps.Total)
		}
		if ps.MetricResults > 0 {
			ps.AvgMetricScore = ps.AvgMetricScore / float64(ps.MetricResults)
		}
	}

	for _, ls := range summary.ByLanguage {
		if ls.Total > 0 {
			ls.Pass1Rate = float64(ls.Pass1) / float64(ls.Total)
			ls.Pass2Rate = float64(ls.Pass2) / float64(ls.Total)
		}
		if ls.MetricResults > 0 {
			ls.AvgMetricScore = ls.AvgMetricScore / float64(ls.MetricResults)
		}
	}

	return summary
}

// CompareRuns compares two run summaries.
func CompareRuns(a, b *EvalRun) *RunComparison {
	comp := &RunComparison{
		RunA: a.ID,
		RunB: b.ID,
	}

	if a.Summary != nil && b.Summary != nil {
		comp.Pass1Delta = b.Summary.Pass1Rate - a.Summary.Pass1Rate
		comp.Pass2Delta = b.Summary.Pass2Rate - a.Summary.Pass2Rate
		comp.LatencyDelta = b.Summary.AvgLatencyMs - a.Summary.AvgLatencyMs
		comp.LatencyP50Delta = b.Summary.LatencyP50 - a.Summary.LatencyP50
		comp.LatencyP95Delta = b.Summary.LatencyP95 - a.Summary.LatencyP95
		comp.CostDelta = b.Summary.TotalCost - a.Summary.TotalCost
		comp.EfficiencyDelta = b.Summary.CostEfficiency - a.Summary.CostEfficiency
		comp.TokensDelta = b.Summary.TotalTokens - a.Summary.TotalTokens
		comp.FallbackDelta = b.Summary.FallbackRate - a.Summary.FallbackRate
		comp.SampleCountA = a.Summary.TotalExercises
		comp.SampleCountB = b.Summary.TotalExercises
	}

	return comp
}

func extractLanguage(exerciseID string) string {
	// Exercise IDs are formatted as "suite/language/slug"
	parts := splitExerciseID(exerciseID)
	if len(parts) >= 2 {
		return parts[1]
	}
	return "unknown"
}

func splitExerciseID(id string) []string {
	result := make([]string, 0, 3)
	start := 0
	for i, c := range id {
		if c == '/' {
			result = append(result, id[start:i])
			start = i + 1
		}
	}
	result = append(result, id[start:])
	return result
}
