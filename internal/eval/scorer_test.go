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
