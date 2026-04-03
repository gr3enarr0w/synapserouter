package eval

import (
	"testing"
	"time"
)

func TestComputeSummaryEmpty(t *testing.T) {
	summary := ComputeSummary(nil)
	if summary.TotalExercises != 0 {
		t.Fatalf("expected 0 exercises, got %d", summary.TotalExercises)
	}
}

func TestComputeSummary(t *testing.T) {
	results := []EvalResult{
		{ID: "r1", ExerciseID: "polyglot/go/hello", Provider: "nanogpt-sub", Pass1: true, Pass2: true, LatencyMs: 1000, TotalTokens: 100},
		{ID: "r2", ExerciseID: "polyglot/go/world", Provider: "nanogpt-sub", Pass1: false, Pass2: true, LatencyMs: 2000, TotalTokens: 200, FallbackUsed: true},
		{ID: "r3", ExerciseID: "polyglot/python/hello", Provider: "gemini", Pass1: true, Pass2: true, LatencyMs: 1500, TotalTokens: 150},
		{ID: "r4", ExerciseID: "polyglot/python/world", Provider: "gemini", Pass1: false, Pass2: false, LatencyMs: 3000, TotalTokens: 300},
	}

	summary := ComputeSummary(results)

	if summary.TotalExercises != 4 {
		t.Fatalf("expected 4, got %d", summary.TotalExercises)
	}
	if summary.Pass1Rate != 0.5 {
		t.Fatalf("expected pass1 rate 0.5, got %f", summary.Pass1Rate)
	}
	if summary.Pass2Rate != 0.75 {
		t.Fatalf("expected pass2 rate 0.75, got %f", summary.Pass2Rate)
	}
	if summary.AvgLatencyMs != 1875 {
		t.Fatalf("expected avg latency 1875, got %d", summary.AvgLatencyMs)
	}
	if summary.TotalTokens != 750 {
		t.Fatalf("expected 750 tokens, got %d", summary.TotalTokens)
	}
	if summary.FallbackRate != 0.25 {
		t.Fatalf("expected fallback rate 0.25, got %f", summary.FallbackRate)
	}

	// Per-provider
	nanoStats := summary.ByProvider["nanogpt-sub"]
	if nanoStats == nil || nanoStats.Total != 2 || nanoStats.Pass1 != 1 {
		t.Fatalf("unexpected nanogpt-sub stats: %+v", nanoStats)
	}
	geminiStats := summary.ByProvider["gemini"]
	if geminiStats == nil || geminiStats.Total != 2 || geminiStats.Pass1 != 1 {
		t.Fatalf("unexpected gemini stats: %+v", geminiStats)
	}

	// Per-language
	goStats := summary.ByLanguage["go"]
	if goStats == nil || goStats.Total != 2 {
		t.Fatalf("unexpected go stats: %+v", goStats)
	}
	pyStats := summary.ByLanguage["python"]
	if pyStats == nil || pyStats.Total != 2 {
		t.Fatalf("unexpected python stats: %+v", pyStats)
	}
}

func TestCompareRuns(t *testing.T) {
	now := time.Now()
	runA := &EvalRun{
		ID: "run-a",
		Summary: &EvalSummary{
			Pass1Rate:    0.5,
			Pass2Rate:    0.7,
			AvgLatencyMs: 2000,
			TotalTokens:  1000,
			FallbackRate: 0.2,
		},
	}
	runB := &EvalRun{
		ID:          "run-b",
		CompletedAt: &now,
		Summary: &EvalSummary{
			Pass1Rate:    0.6,
			Pass2Rate:    0.8,
			AvgLatencyMs: 1800,
			TotalTokens:  900,
			FallbackRate: 0.1,
		},
	}

	comp := CompareRuns(runA, runB)
	if comp.RunA != "run-a" || comp.RunB != "run-b" {
		t.Fatalf("unexpected run IDs: %s, %s", comp.RunA, comp.RunB)
	}

	// pass1 delta should be +0.1
	delta := comp.Pass1Delta - 0.1
	if delta > 0.001 || delta < -0.001 {
		t.Fatalf("expected pass1 delta ~0.1, got %f", comp.Pass1Delta)
	}

	if comp.LatencyDelta != -200 {
		t.Fatalf("expected latency delta -200, got %d", comp.LatencyDelta)
	}
}

func TestPercentile(t *testing.T) {
	data := []float64{100, 200, 300, 400, 500}

	p50 := Percentile(data, 50)
	if p50 != 300 {
		t.Errorf("p50 of [100-500] = %f, want 300", p50)
	}

	p95 := Percentile(data, 95)
	if p95 < 480 || p95 > 500 {
		t.Errorf("p95 of [100-500] = %f, want ~480-500", p95)
	}

	// Edge cases
	if Percentile(nil, 50) != 0 {
		t.Error("p50 of empty = 0")
	}
	if Percentile([]float64{42}, 50) != 42 {
		t.Error("p50 of [42] = 42")
	}
}

func TestComputeSummary_Percentiles(t *testing.T) {
	results := []EvalResult{
		{ID: "r1", ExerciseID: "s/go/a", Provider: "p1", Pass1: true, LatencyMs: 100, TotalTokens: 50},
		{ID: "r2", ExerciseID: "s/go/b", Provider: "p1", Pass1: true, LatencyMs: 200, TotalTokens: 60},
		{ID: "r3", ExerciseID: "s/go/c", Provider: "p1", Pass1: true, LatencyMs: 300, TotalTokens: 70},
		{ID: "r4", ExerciseID: "s/go/d", Provider: "p1", Pass1: false, LatencyMs: 400, TotalTokens: 80},
		{ID: "r5", ExerciseID: "s/go/e", Provider: "p1", Pass1: true, LatencyMs: 500, TotalTokens: 90},
	}

	summary := ComputeSummary(results)

	if summary.LatencyP50 != 300 {
		t.Errorf("p50 = %f, want 300", summary.LatencyP50)
	}
	if summary.LatencyP95 < 450 {
		t.Errorf("p95 = %f, want >= 450", summary.LatencyP95)
	}
	if summary.LatencyP99 < 490 {
		t.Errorf("p99 = %f, want >= 490", summary.LatencyP99)
	}
	if summary.TokensPerSec <= 0 {
		t.Error("TokensPerSec should be > 0")
	}
}

func TestComputeSummary_Cost(t *testing.T) {
	results := []EvalResult{
		{ID: "r1", ExerciseID: "s/go/a", Provider: "p1", Pass1: true, LatencyMs: 100, CostUSD: 0.01},
		{ID: "r2", ExerciseID: "s/go/b", Provider: "p1", Pass1: true, LatencyMs: 200, CostUSD: 0.02},
		{ID: "r3", ExerciseID: "s/go/c", Provider: "p1", Pass1: false, LatencyMs: 300, CostUSD: 0.03},
	}

	summary := ComputeSummary(results)

	if summary.TotalCost < 0.059 || summary.TotalCost > 0.061 {
		t.Errorf("TotalCost = %f, want ~0.06", summary.TotalCost)
	}
	if summary.AvgCostPerEx < 0.019 || summary.AvgCostPerEx > 0.021 {
		t.Errorf("AvgCostPerEx = %f, want ~0.02", summary.AvgCostPerEx)
	}
	if summary.CostEfficiency <= 0 {
		t.Error("CostEfficiency should be > 0 when cost > 0 and pass rate > 0")
	}
}

func TestCompareRuns_Extended(t *testing.T) {
	runA := &EvalRun{
		ID: "a",
		Summary: &EvalSummary{
			TotalExercises: 40,
			Pass1Rate:      0.5,
			AvgLatencyMs:   2000,
			LatencyP50:     1500,
			LatencyP95:     3000,
			TotalCost:      1.0,
			CostEfficiency: 20.0,
			TotalTokens:    1000,
			FallbackRate:   0.2,
		},
	}
	runB := &EvalRun{
		ID: "b",
		Summary: &EvalSummary{
			TotalExercises: 35,
			Pass1Rate:      0.6,
			AvgLatencyMs:   1800,
			LatencyP50:     1200,
			LatencyP95:     2500,
			TotalCost:      0.8,
			CostEfficiency: 26.25,
			TotalTokens:    900,
			FallbackRate:   0.1,
		},
	}

	comp := CompareRuns(runA, runB)

	if comp.LatencyP50Delta != -300 {
		t.Errorf("p50 delta = %f, want -300", comp.LatencyP50Delta)
	}
	if comp.LatencyP95Delta != -500 {
		t.Errorf("p95 delta = %f, want -500", comp.LatencyP95Delta)
	}
	if comp.CostDelta > -0.19 || comp.CostDelta < -0.21 {
		t.Errorf("cost delta = %f, want ~-0.2", comp.CostDelta)
	}
	if comp.SampleCountA != 40 || comp.SampleCountB != 35 {
		t.Errorf("sample counts = %d/%d, want 40/35", comp.SampleCountA, comp.SampleCountB)
	}
}

func TestExtractLanguage(t *testing.T) {
	tests := []struct {
		id   string
		want string
	}{
		{"polyglot/go/hello", "go"},
		{"roocode/python/world", "python"},
		{"unknown", "unknown"},
	}
	for _, tt := range tests {
		got := extractLanguage(tt.id)
		if got != tt.want {
			t.Errorf("extractLanguage(%q) = %q, want %q", tt.id, got, tt.want)
		}
	}
}
