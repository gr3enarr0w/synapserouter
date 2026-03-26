package agent

import (
	"testing"
)

// mockFactProvider implements FactProvider for testing.
type mockFactProvider struct {
	paths      map[string]bool
	testResult *TestFact
	bashFacts  []BashFact
}

func (m *mockFactProvider) KnownPaths() map[string]bool { return m.paths }
func (m *mockFactProvider) LastTestResult() *TestFact    { return m.testResult }
func (m *mockFactProvider) LastBashResult(prefix string) *BashFact {
	for i := len(m.bashFacts) - 1; i >= 0; i-- {
		if len(m.bashFacts[i].Command) >= len(prefix) && m.bashFacts[i].Command[:len(prefix)] == prefix {
			return &m.bashFacts[i]
		}
	}
	return nil
}
func (m *mockFactProvider) RecentBashFacts(n int) []BashFact {
	if n > len(m.bashFacts) {
		n = len(m.bashFacts)
	}
	return m.bashFacts[len(m.bashFacts)-n:]
}

func TestHallucination_TestsPassWhenFailed(t *testing.T) {
	facts := &mockFactProvider{
		testResult: &TestFact{Passed: false, FailCount: 3, PassCount: 10},
	}
	result := CheckForHallucinations("All tests pass and the code looks good.", facts)
	if !result.Detected {
		t.Error("should detect hallucination: claims tests pass but they failed")
	}
	if len(result.Signals) == 0 || result.Signals[0].Type != SignalFalseSuccess {
		t.Error("should have SignalFalseSuccess")
	}
}

func TestHallucination_TestsPassWhenPassed(t *testing.T) {
	facts := &mockFactProvider{
		testResult: &TestFact{Passed: true, PassCount: 10},
	}
	result := CheckForHallucinations("All tests pass and the code looks good.", facts)
	if result.Detected {
		t.Error("should NOT detect hallucination: tests actually pass")
	}
}

func TestHallucination_BuildSuccessWhenFailed(t *testing.T) {
	facts := &mockFactProvider{
		bashFacts: []BashFact{
			{Command: "go build ./...", ExitCode: 1, LastLines: "undefined: foo"},
		},
	}
	result := CheckForHallucinations("The code builds successfully with no errors.", facts)
	if !result.Detected {
		t.Error("should detect hallucination: claims build succeeds but it failed")
	}
}

func TestHallucination_BuildSuccessWhenPassed(t *testing.T) {
	facts := &mockFactProvider{
		bashFacts: []BashFact{
			{Command: "go build ./...", ExitCode: 0, LastLines: ""},
		},
	}
	result := CheckForHallucinations("The code builds successfully.", facts)
	if result.Detected {
		t.Error("should NOT detect hallucination: build actually succeeded")
	}
}

func TestHallucination_UnknownFilePath(t *testing.T) {
	facts := &mockFactProvider{
		paths: map[string]bool{
			"internal/handler.go": true,
			"internal/server.go":  true,
		},
	}
	result := CheckForHallucinations("I've updated internal/router.go to fix the issue.", facts)
	found := false
	for _, s := range result.Signals {
		if s.Type == SignalUnknownPath {
			found = true
		}
	}
	if !found {
		t.Error("should detect unknown path: internal/router.go not in known paths")
	}
}

func TestHallucination_KnownFilePath(t *testing.T) {
	facts := &mockFactProvider{
		paths: map[string]bool{
			"internal/handler.go": true,
		},
	}
	result := CheckForHallucinations("I've updated internal/handler.go to fix the issue.", facts)
	for _, s := range result.Signals {
		if s.Type == SignalUnknownPath {
			t.Error("should NOT flag known path internal/handler.go")
		}
	}
}

func TestHallucination_ExitCodeContradiction(t *testing.T) {
	facts := &mockFactProvider{
		bashFacts: []BashFact{
			{Command: "curl https://api.example.com", ExitCode: 1, LastLines: "connection refused"},
		},
	}
	result := CheckForHallucinations("The command completed successfully with no errors.", facts)
	found := false
	for _, s := range result.Signals {
		if s.Type == SignalContradiction {
			found = true
		}
	}
	if !found {
		t.Error("should detect contradiction: claims success but exit code was 1")
	}
}

func TestHallucination_HedgedLanguageSuppresses(t *testing.T) {
	facts := &mockFactProvider{
		testResult: &TestFact{Passed: false, FailCount: 3},
	}
	result := CheckForHallucinations("The tests should probably pass after these changes likely fix the issue.", facts)
	if result.Detected {
		t.Error("should NOT detect hallucination when language is heavily hedged")
	}
}

func TestHallucination_ConfidenceThreshold(t *testing.T) {
	// Single low-severity signal should NOT cross threshold
	facts := &mockFactProvider{
		paths: map[string]bool{"known.go": true},
	}
	result := CheckForHallucinations("Check out unknown_file.py for details.", facts)
	if result.Confidence >= hallucinationThreshold {
		t.Errorf("single low-severity signal should not cross threshold, got confidence %f", result.Confidence)
	}
}

func TestHallucination_EmptyContent(t *testing.T) {
	facts := &mockFactProvider{
		testResult: &TestFact{Passed: false},
	}
	result := CheckForHallucinations("", facts)
	if result.Detected {
		t.Error("empty content should not trigger detection")
	}
}

func TestHallucination_NilFacts(t *testing.T) {
	result := CheckForHallucinations("All tests pass.", nil)
	if result.Detected {
		t.Error("nil facts should not trigger detection")
	}
}

func TestHallucination_NoFactsYet(t *testing.T) {
	facts := &mockFactProvider{
		paths: map[string]bool{},
	}
	result := CheckForHallucinations("I'll update the handler.go file.", facts)
	if result.Detected {
		t.Error("no known paths = can't judge, should not detect")
	}
}
