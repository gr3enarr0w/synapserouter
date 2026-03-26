package agent

import (
	"strings"
	"testing"
)

func TestAutoRecall_FormatsCorrectiveMessage(t *testing.T) {
	a := &Agent{
		factTracker: NewFactTracker(),
		config:      Config{},
	}
	check := &HallucinationCheckResult{
		Detected:   true,
		Confidence: 0.9,
		Signals: []HallucinationSignal{
			{
				Type:        SignalFalseSuccess,
				Description: "Claims tests pass but they failed",
				Evidence:    "exit code 1",
				Severity:    0.9,
			},
		},
	}

	result := a.autoRecall(check)
	if result == "" {
		t.Fatal("expected corrective message")
	}
	if !strings.Contains(result, "CORRECTION") {
		t.Error("should contain CORRECTION header")
	}
	if !strings.Contains(result, "FALSE CLAIM") {
		t.Error("should contain FALSE CLAIM for SignalFalseSuccess")
	}
	if !strings.Contains(result, "exit code 1") {
		t.Error("should contain the evidence")
	}
}

func TestAutoRecall_RateLimits(t *testing.T) {
	a := &Agent{
		factTracker:              NewFactTracker(),
		hallucinationRecallCount: maxHallucinationRecalls,
	}
	check := &HallucinationCheckResult{
		Detected:   true,
		Confidence: 0.9,
		Signals:    []HallucinationSignal{{Type: SignalFalseSuccess, Severity: 0.9}},
	}
	result := a.autoRecall(check)
	if result != "" {
		t.Error("should return empty after max recalls reached")
	}
}

func TestAutoRecall_IncrementsCount(t *testing.T) {
	a := &Agent{
		factTracker: NewFactTracker(),
	}
	check := &HallucinationCheckResult{
		Detected: true,
		Signals:  []HallucinationSignal{{Type: SignalContradiction, Severity: 0.7}},
	}
	a.autoRecall(check)
	if a.hallucinationRecallCount != 1 {
		t.Errorf("expected count 1, got %d", a.hallucinationRecallCount)
	}
	a.autoRecall(check)
	if a.hallucinationRecallCount != 2 {
		t.Errorf("expected count 2, got %d", a.hallucinationRecallCount)
	}
}

func TestAutoRecall_ScrubsSecrets(t *testing.T) {
	a := &Agent{
		factTracker: NewFactTracker(),
	}
	check := &HallucinationCheckResult{
		Detected: true,
		Signals: []HallucinationSignal{
			{
				Type:        SignalContradiction,
				Description: "test",
				Evidence:    "curl -H 'Authorization: Bearer sk-secret123'",
				Severity:    0.7,
			},
		},
	}
	result := a.autoRecall(check)
	if strings.Contains(result, "sk-secret123") {
		t.Error("should scrub Bearer token from corrective message")
	}
	if !strings.Contains(result, "[REDACTED]") {
		t.Error("should contain [REDACTED] placeholder")
	}
}

func TestAutoRecall_CapsAt4KB(t *testing.T) {
	a := &Agent{
		factTracker: NewFactTracker(),
	}
	// Create a signal with very long evidence
	longEvidence := strings.Repeat("x", 10000)
	check := &HallucinationCheckResult{
		Detected: true,
		Signals: []HallucinationSignal{
			{Type: SignalContradiction, Description: "test", Evidence: longEvidence, Severity: 0.7},
		},
	}
	result := a.autoRecall(check)
	if len(result) > maxCorrectiveMessageSize+20 { // +20 for truncation suffix
		t.Errorf("corrective message should be capped at ~4KB, got %d bytes", len(result))
	}
}

func TestAutoRecall_NilCheck(t *testing.T) {
	a := &Agent{factTracker: NewFactTracker()}
	if a.autoRecall(nil) != "" {
		t.Error("nil check should return empty")
	}
}

func TestAutoRecall_NotDetected(t *testing.T) {
	a := &Agent{factTracker: NewFactTracker()}
	check := &HallucinationCheckResult{Detected: false}
	if a.autoRecall(check) != "" {
		t.Error("non-detected check should return empty")
	}
}
