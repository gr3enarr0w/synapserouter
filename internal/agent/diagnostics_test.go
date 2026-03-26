package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWriteDiagnostics_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	a := &Agent{
		sessionID:     "test-session-1",
		config:        Config{WorkDir: dir},
		toolCallCount: 5,
		providerIdx:   0,
	}

	a.writeDiagnostics(time.Now().Add(-10 * time.Second))

	path := filepath.Join(dir, diagnosticsFile)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected synroute.md to exist: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "## Session test-session-1") {
		t.Error("diagnostics should contain session ID")
	}
	if !strings.Contains(content, "Tool calls:** 5") {
		t.Error("diagnostics should contain tool call count")
	}
}

func TestWriteDiagnostics_AppendsMultipleSessions(t *testing.T) {
	dir := t.TempDir()
	a1 := &Agent{
		sessionID: "session-1",
		config:    Config{WorkDir: dir},
	}
	a2 := &Agent{
		sessionID: "session-2",
		config:    Config{WorkDir: dir},
	}

	a1.writeDiagnostics(time.Now())
	a2.writeDiagnostics(time.Now())

	data, _ := os.ReadFile(filepath.Join(dir, diagnosticsFile))
	content := string(data)
	if strings.Count(content, "## Session") != 2 {
		t.Errorf("expected 2 session blocks, got %d", strings.Count(content, "## Session"))
	}
}

func TestWriteDiagnostics_EmptyWorkDir(t *testing.T) {
	a := &Agent{
		sessionID: "test",
		config:    Config{WorkDir: ""},
	}
	// Should not panic
	a.writeDiagnostics(time.Now())
}

func TestReadPreviousDiagnostics_ReturnsLastSession(t *testing.T) {
	dir := t.TempDir()
	content := `## Session session-1
- **Result:** failed at implement

---

## Session session-2
- **Result:** all phases passed

---
`
	os.WriteFile(filepath.Join(dir, diagnosticsFile), []byte(content), 0644)

	result := readPreviousDiagnostics(dir)
	if !strings.Contains(result, "session-2") {
		t.Error("should return the most recent session")
	}
	if strings.Contains(result, "session-1") {
		t.Error("should NOT contain earlier sessions")
	}
}

func TestReadPreviousDiagnostics_MissingFile(t *testing.T) {
	result := readPreviousDiagnostics(t.TempDir())
	if result != "" {
		t.Error("should return empty string when file doesn't exist")
	}
}

func TestReadPreviousDiagnostics_EmptyWorkDir(t *testing.T) {
	result := readPreviousDiagnostics("")
	if result != "" {
		t.Error("should return empty string for empty workdir")
	}
}

func TestFormatDiagnosticsReport_AllFields(t *testing.T) {
	report := DiagnosticsReport{
		Timestamp:       time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC),
		SessionID:       "test-123",
		PhasesCompleted: []string{"plan", "implement", "self-check"},
		FailedPhase:     "code-review",
		ToolCallCount:   42,
		TotalDuration:   2 * time.Minute,
		ProvidersUsed:   []string{"devstral-2:123b", "qwen3-coder:480b"},
		ErrorMessages:   []string{"build failed: undefined foo"},
		VerifyResults:   []string{"go vet: PASS", "go test: FAIL"},
	}

	content := formatDiagnosticsReport(report)

	checks := []string{
		"## Session test-123",
		"plan → implement → self-check",
		"Failed at:** code-review",
		"Tool calls:** 42",
		"devstral-2:123b, qwen3-coder:480b",
		"build failed: undefined foo",
		"go vet: PASS",
	}
	for _, check := range checks {
		if !strings.Contains(content, check) {
			t.Errorf("report should contain %q", check)
		}
	}
}

func TestFormatDiagnosticsReport_AllPassed(t *testing.T) {
	report := DiagnosticsReport{
		SessionID:       "success-1",
		PhasesCompleted: []string{"plan", "implement", "verify"},
		TotalDuration:   30 * time.Second,
	}
	content := formatDiagnosticsReport(report)
	if !strings.Contains(content, "all phases passed") {
		t.Error("should indicate all phases passed when no failed phase")
	}
}
