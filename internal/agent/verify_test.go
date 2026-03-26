package agent

import (
	"strings"
	"testing"
)

func TestShouldRunBuild(t *testing.T) {
	tests := []struct {
		phase    string
		expected bool
	}{
		{"plan", false},
		{"eda", false},
		{"data-prep", false},
		{"implement", true},
		{"self-check", true},
		{"code-review", true},
		{"acceptance-test", true},
		{"deploy", true},
		{"model", true},
		{"review", true},
		{"unknown", false},
	}
	for _, tt := range tests {
		t.Run(tt.phase, func(t *testing.T) {
			if got := shouldRunBuild(tt.phase); got != tt.expected {
				t.Errorf("shouldRunBuild(%q) = %v, want %v", tt.phase, got, tt.expected)
			}
		})
	}
}

func TestShouldRunTests(t *testing.T) {
	tests := []struct {
		phase    string
		expected bool
	}{
		{"plan", false},
		{"implement", false},
		{"self-check", true},
		{"code-review", true},
		{"acceptance-test", true},
		{"model", true},
		{"review", true},
		{"deploy", false},
		{"eda", false},
	}
	for _, tt := range tests {
		t.Run(tt.phase, func(t *testing.T) {
			if got := shouldRunTests(tt.phase); got != tt.expected {
				t.Errorf("shouldRunTests(%q) = %v, want %v", tt.phase, got, tt.expected)
			}
		})
	}
}

func TestShouldRunSkillVerify(t *testing.T) {
	tests := []struct {
		phase    string
		expected bool
	}{
		{"code-review", true},
		{"acceptance-test", true},
		{"review", true},
		{"implement", false},
		{"self-check", false},
		{"plan", false},
		{"deploy", false},
	}
	for _, tt := range tests {
		t.Run(tt.phase, func(t *testing.T) {
			if got := shouldRunSkillVerify(tt.phase); got != tt.expected {
				t.Errorf("shouldRunSkillVerify(%q) = %v, want %v", tt.phase, got, tt.expected)
			}
		})
	}
}

func TestCountVerifyFailed(t *testing.T) {
	results := []VerifyResult{
		{Name: "build", Passed: true, ExitCode: 0},
		{Name: "test", Passed: false, ExitCode: 1},
		{Name: "lint", Passed: true, ExitCode: 0},
		{Name: "verify", Passed: false, ExitCode: 2},
	}
	if got := countVerifyFailed(results); got != 2 {
		t.Errorf("countVerifyFailed() = %d, want 2", got)
	}

	// All pass
	allPass := []VerifyResult{
		{Name: "a", Passed: true},
		{Name: "b", Passed: true},
	}
	if got := countVerifyFailed(allPass); got != 0 {
		t.Errorf("countVerifyFailed(allPass) = %d, want 0", got)
	}

	// Empty
	if got := countVerifyFailed(nil); got != 0 {
		t.Errorf("countVerifyFailed(nil) = %d, want 0", got)
	}
}

func TestFormatVerifyFailures(t *testing.T) {
	results := []VerifyResult{
		{Name: "build/go", Passed: true, ExitCode: 0, Output: "ok"},
		{Name: "test/go", Passed: false, ExitCode: 1, Output: "FAIL: TestFoo"},
		{Name: "skill/readme", Passed: false, ExitCode: 1, Output: "MISSING"},
	}

	msg := FormatVerifyFailures(results)

	if !strings.Contains(msg, "VERIFICATION GATE FAILED") {
		t.Error("expected VERIFICATION GATE FAILED header")
	}
	if !strings.Contains(msg, "test/go: FAIL") {
		t.Error("expected test/go FAIL")
	}
	if !strings.Contains(msg, "skill/readme: FAIL") {
		t.Error("expected skill/readme FAIL")
	}
	if !strings.Contains(msg, "build/go: PASS") {
		t.Error("expected build/go PASS")
	}
	if !strings.Contains(msg, "FAIL: TestFoo") {
		t.Error("expected failure output included")
	}
	if !strings.Contains(msg, "Fix these issues") {
		t.Error("expected fix instruction")
	}
}

func TestFormatVerifyFailures_TruncatesLongOutput(t *testing.T) {
	longOutput := strings.Repeat("x", 1000)
	results := []VerifyResult{
		{Name: "test", Passed: false, ExitCode: 1, Output: longOutput},
	}

	msg := FormatVerifyFailures(results)

	if !strings.Contains(msg, "truncated") {
		t.Error("expected long output to be truncated")
	}
	if len(msg) > 1500 {
		t.Errorf("formatted message too long: %d chars", len(msg))
	}
}

func TestVerifyResult_Struct(t *testing.T) {
	r := VerifyResult{
		Name:     "build/rust",
		Passed:   true,
		Output:   "Compiling fd v0.1.0",
		ExitCode: 0,
	}
	if r.Name != "build/rust" {
		t.Error("wrong name")
	}
	if !r.Passed {
		t.Error("should be passed")
	}
	if r.ExitCode != 0 {
		t.Error("should be exit 0")
	}
}
