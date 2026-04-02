package agent

import (
	"strings"
	"testing"
)

func TestParseFindings_Structured(t *testing.T) {
	output := `Review complete.

[FILE: cmd/main.go:42] error-handling: Missing error check on database connection
[FILE: internal/router/router.go:15-20] spec-violation: Router does not implement fallback chain
[FILE: internal/agent/agent.go:100] null-value: Nil pointer dereference when config is empty

Overall: NEEDS_FIX`

	findings := ParseFindings(0, output)
	if len(findings) != 3 {
		t.Fatalf("expected 3 findings, got %d", len(findings))
	}

	// Check first finding
	f := findings[0]
	if f.File != "cmd/main.go" {
		t.Errorf("expected file cmd/main.go, got %q", f.File)
	}
	if f.LineRange[0] != 42 {
		t.Errorf("expected line 42, got %d", f.LineRange[0])
	}
	if f.Category != "error-handling" {
		t.Errorf("expected category error-handling, got %q", f.Category)
	}
	if f.ReviewerID != 0 {
		t.Errorf("expected reviewerID 0, got %d", f.ReviewerID)
	}

	// Check range finding
	f2 := findings[1]
	if f2.LineRange[0] != 15 || f2.LineRange[1] != 20 {
		t.Errorf("expected line range 15-20, got %d-%d", f2.LineRange[0], f2.LineRange[1])
	}
}

func TestParseFindings_FallbackBullets(t *testing.T) {
	output := `I found several issues:

- FAIL: The handler at server.go:55 does not validate input
- Issue: Missing authentication check
- NEEDS_FIX: Null pointer in parser.go:12

NEEDS_FIX`

	findings := ParseFindings(1, output)
	if len(findings) != 3 {
		t.Fatalf("expected 3 findings, got %d", len(findings))
	}

	// First should have file reference
	if findings[0].File != "server.go" {
		t.Errorf("expected file server.go, got %q", findings[0].File)
	}
	if findings[0].LineRange[0] != 55 {
		t.Errorf("expected line 55, got %d", findings[0].LineRange[0])
	}
	if findings[0].ReviewerID != 1 {
		t.Errorf("expected reviewerID 1, got %d", findings[0].ReviewerID)
	}
}

func TestParseFindings_Empty(t *testing.T) {
	findings := ParseFindings(0, "")
	if len(findings) != 0 {
		t.Errorf("expected 0 findings from empty output, got %d", len(findings))
	}

	findings = ParseFindings(0, "All looks good! VERIFIED_CORRECT")
	if len(findings) != 0 {
		t.Errorf("expected 0 findings from passing review, got %d", len(findings))
	}
}

func TestParseFindings_Dedup(t *testing.T) {
	output := `[FILE: main.go:10] error: Missing check
[FILE: main.go:10] error: Missing check`

	findings := ParseFindings(0, output)
	if len(findings) != 1 {
		t.Errorf("expected dedup to 1 finding, got %d", len(findings))
	}
}

func TestClusterFindings_SameFileSameIssue(t *testing.T) {
	findings := []ReviewFinding{
		{File: "main.go", LineRange: [2]int{10, 10}, Category: "error-handling", Summary: "Missing error check", ReviewerID: 0},
		{File: "main.go", LineRange: [2]int{12, 12}, Category: "error-handling", Summary: "Missing error check on return", ReviewerID: 1},
	}

	clusters := ClusterFindings(findings, 2, 0.5)
	if len(clusters) != 1 {
		t.Fatalf("expected 1 cluster (same file, proximate lines, same category), got %d", len(clusters))
	}
	if clusters[0].Agreement != 1.0 {
		t.Errorf("expected agreement 1.0, got %f", clusters[0].Agreement)
	}
	if clusters[0].Confidence != "high" {
		t.Errorf("expected confidence high, got %q", clusters[0].Confidence)
	}
}

func TestClusterFindings_DifferentFiles(t *testing.T) {
	findings := []ReviewFinding{
		{File: "main.go", LineRange: [2]int{10, 10}, Category: "error", Summary: "Missing check", ReviewerID: 0},
		{File: "server.go", LineRange: [2]int{10, 10}, Category: "error", Summary: "Missing check", ReviewerID: 1},
	}

	clusters := ClusterFindings(findings, 2, 0.5)
	if len(clusters) != 2 {
		t.Fatalf("expected 2 clusters (different files), got %d", len(clusters))
	}
}

func TestClusterFindings_AgreementCalculation(t *testing.T) {
	findings := []ReviewFinding{
		{File: "main.go", LineRange: [2]int{10, 10}, Category: "error", Summary: "Missing null check", ReviewerID: 0},
		{File: "main.go", LineRange: [2]int{10, 10}, Category: "error", Summary: "Missing null check", ReviewerID: 1},
		{File: "main.go", LineRange: [2]int{10, 10}, Category: "error", Summary: "Missing null check", ReviewerID: 2},
	}

	clusters := ClusterFindings(findings, 3, 0.5)
	if len(clusters) != 1 {
		t.Fatalf("expected 1 cluster, got %d", len(clusters))
	}
	if clusters[0].Agreement != 1.0 {
		t.Errorf("expected agreement 1.0, got %f", clusters[0].Agreement)
	}
}

func TestClusterFindings_SingleReviewer(t *testing.T) {
	findings := []ReviewFinding{
		{File: "main.go", LineRange: [2]int{10, 10}, Category: "error", Summary: "Issue A", ReviewerID: 0},
	}

	clusters := ClusterFindings(findings, 1, 0.5)
	if len(clusters) != 1 {
		t.Fatalf("expected 1 cluster, got %d", len(clusters))
	}
	if clusters[0].Agreement != 1.0 {
		t.Errorf("K=1: expected agreement 1.0, got %f", clusters[0].Agreement)
	}
}

func TestClusterFindings_Empty(t *testing.T) {
	clusters := ClusterFindings(nil, 2, 0.5)
	if clusters != nil {
		t.Errorf("expected nil clusters from empty findings, got %d", len(clusters))
	}
}

func TestClusterFindings_DistantLines(t *testing.T) {
	findings := []ReviewFinding{
		{File: "main.go", LineRange: [2]int{10, 10}, Category: "error", Summary: "Issue at top", ReviewerID: 0},
		{File: "main.go", LineRange: [2]int{100, 100}, Category: "error", Summary: "Issue at bottom", ReviewerID: 1},
	}

	clusters := ClusterFindings(findings, 2, 0.5)
	if len(clusters) != 2 {
		t.Fatalf("expected 2 clusters (distant lines), got %d", len(clusters))
	}
}

func TestFormatMergedReview_AllPass(t *testing.T) {
	result := KReviewResult{
		K:               2,
		ReviewerResults: []string{"All good. VERIFIED_CORRECT", "Looks correct. verified_correct"},
		AllPassed:       true,
		MajorityPassed:  true,
	}

	output := FormatMergedReview(result)
	if !strings.Contains(output, "VERIFIED_CORRECT") {
		t.Error("expected VERIFIED_CORRECT in all-pass output")
	}
	if !strings.Contains(output, "2/2 reviewers passed") {
		t.Error("expected 2/2 count")
	}
}

func TestFormatMergedReview_AllFail(t *testing.T) {
	result := KReviewResult{
		K:               2,
		ReviewerResults: []string{"NEEDS_FIX: bad code", "NEEDS_FIX: terrible code"},
		HighConfidence: []FindingCluster{
			{RootCause: "Bad error handling", File: "main.go", LineRange: [2]int{10, 10}, Confidence: "high",
				Findings: []ReviewFinding{{ReviewerID: 0}, {ReviewerID: 1}}},
		},
		AllPassed:      false,
		MajorityPassed: false,
	}

	output := FormatMergedReview(result)
	if !strings.Contains(output, "NEEDS_FIX") {
		t.Error("expected NEEDS_FIX in all-fail output")
	}
	if !strings.Contains(output, "HIGH-CONFIDENCE") {
		t.Error("expected HIGH-CONFIDENCE section")
	}
}

func TestFormatMergedReview_SplitDecision(t *testing.T) {
	result := KReviewResult{
		K:               3,
		ReviewerResults: []string{"VERIFIED_CORRECT", "VERIFIED_CORRECT", "NEEDS_FIX: minor issue"},
		Disagreements: []FindingCluster{
			{RootCause: "Minor style issue", File: "util.go", Confidence: "low",
				Findings: []ReviewFinding{{ReviewerID: 2}}},
		},
		AllPassed:      false,
		MajorityPassed: true,
	}

	output := FormatMergedReview(result)
	if !strings.Contains(output, "DISAGREEMENTS") {
		t.Error("expected DISAGREEMENTS section in split decision")
	}
}

func TestShuffleFileOrder_Deterministic(t *testing.T) {
	files := []string{"a.go", "b.go", "c.go", "d.go", "e.go"}

	s1 := ShuffleFileOrder(files, 42)
	s2 := ShuffleFileOrder(files, 42)

	for i := range s1 {
		if s1[i] != s2[i] {
			t.Errorf("same seed should produce same order: s1[%d]=%s, s2[%d]=%s", i, s1[i], i, s2[i])
		}
	}
}

func TestShuffleFileOrder_DifferentSeeds(t *testing.T) {
	files := []string{"a.go", "b.go", "c.go", "d.go", "e.go"}

	s1 := ShuffleFileOrder(files, 0)
	s2 := ShuffleFileOrder(files, 1)

	different := false
	for i := range s1 {
		if s1[i] != s2[i] {
			different = true
			break
		}
	}
	if !different {
		t.Error("different seeds should (very likely) produce different orders")
	}
}

func TestShuffleFileOrder_DoesNotMutateInput(t *testing.T) {
	files := []string{"a.go", "b.go", "c.go"}
	original := make([]string, len(files))
	copy(original, files)

	ShuffleFileOrder(files, 99)

	for i := range files {
		if files[i] != original[i] {
			t.Error("ShuffleFileOrder should not mutate the input slice")
		}
	}
}

func TestShuffleFileOrder_SingleFile(t *testing.T) {
	result := ShuffleFileOrder([]string{"only.go"}, 0)
	if len(result) != 1 || result[0] != "only.go" {
		t.Error("single file should return unchanged")
	}
}

func TestShuffleFileOrder_Empty(t *testing.T) {
	result := ShuffleFileOrder(nil, 0)
	if len(result) != 0 {
		t.Error("empty input should return empty")
	}
}

func TestJaccardSimilarity(t *testing.T) {
	tests := []struct {
		a, b string
		want float64
	}{
		{"missing error check", "missing error check", 1.0},
		{"missing error check", "completely different words", 0.0},
		{"missing null check on return", "missing nil check on return value", 0.5}, // ~approx
		{"", "", 1.0},
	}

	for _, tt := range tests {
		got := jaccardSimilarity(tt.a, tt.b)
		// Allow some tolerance for approximate cases
		if tt.want == 1.0 && got != 1.0 {
			t.Errorf("jaccardSimilarity(%q, %q) = %f, want %f", tt.a, tt.b, got, tt.want)
		}
		if tt.want == 0.0 && got != 0.0 {
			t.Errorf("jaccardSimilarity(%q, %q) = %f, want %f", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestCheckDivergenceCount(t *testing.T) {
	tracker := &ReviewCycleTracker{}

	// First call: no previous data, should not diverge
	if tracker.CheckDivergenceCount(5) {
		t.Error("first call should not detect divergence")
	}

	// Second call: findings increased (5 → 8), divergeCount = 1
	if tracker.CheckDivergenceCount(8) {
		t.Error("single increase should not trigger divergence")
	}

	// Third call: findings increased again (8 → 10), divergeCount = 2 → diverging
	if !tracker.CheckDivergenceCount(10) {
		t.Error("two consecutive increases should trigger divergence")
	}

	// Fourth call: findings decreased (10 → 3), divergeCount resets
	if tracker.CheckDivergenceCount(3) {
		t.Error("decrease should reset divergence")
	}
}

func TestFormatMergedReview_MajorityPassNoHighConfidence(t *testing.T) {
	result := KReviewResult{
		K:               3,
		ReviewerResults: []string{"VERIFIED_CORRECT", "VERIFIED_CORRECT", "NEEDS_FIX: minor"},
		AllPassed:       false,
		MajorityPassed:  true,
		// No HighConfidence findings
		Disagreements: []FindingCluster{
			{RootCause: "Minor issue", File: "util.go", Confidence: "low",
				Findings: []ReviewFinding{{ReviewerID: 2}}},
		},
	}

	output := FormatMergedReview(result)
	if !strings.Contains(output, "VERIFIED_CORRECT (majority passed") {
		t.Errorf("expected VERIFIED_CORRECT signal when majority passed with no high-confidence issues, got:\n%s", output)
	}
}

func TestClassifyCategory(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Null pointer dereference", "null-value"},
		{"Missing spec requirement", "spec-violation"},
		{"Unhandled error return", "error-handling"},
		{"No test coverage", "testing"},
		{"SQL injection vulnerability", "security"},
		{"Memory leak detected", "performance"},
		{"Variable naming convention", "style"},
		{"Some random issue", "general"},
	}

	for _, tt := range tests {
		got := classifyCategory(tt.input)
		if got != tt.want {
			t.Errorf("classifyCategory(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
