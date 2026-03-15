package agent

import (
	"testing"
	"time"
)

func TestBudgetTrackerTurns(t *testing.T) {
	bt := NewBudgetTracker(AgentBudget{MaxTurns: 3})

	bt.RecordTurn()
	bt.RecordTurn()
	if reason := bt.Exceeded(); reason != "" {
		t.Errorf("should not be exceeded at 2/3 turns, got %q", reason)
	}

	bt.RecordTurn()
	if reason := bt.Exceeded(); reason == "" {
		t.Error("should be exceeded at 3/3 turns")
	}
}

func TestBudgetTrackerTokens(t *testing.T) {
	bt := NewBudgetTracker(AgentBudget{MaxTokens: 1000})

	bt.RecordTokens(500)
	if reason := bt.Exceeded(); reason != "" {
		t.Errorf("should not be exceeded at 500/1000, got %q", reason)
	}

	bt.RecordTokens(600)
	if reason := bt.Exceeded(); reason == "" {
		t.Error("should be exceeded at 1100/1000")
	}
}

func TestBudgetTrackerDuration(t *testing.T) {
	bt := NewBudgetTracker(AgentBudget{MaxDuration: 50 * time.Millisecond})

	if reason := bt.Exceeded(); reason != "" {
		t.Errorf("should not be exceeded immediately, got %q", reason)
	}

	time.Sleep(60 * time.Millisecond)
	if reason := bt.Exceeded(); reason == "" {
		t.Error("should be exceeded after duration")
	}
}

func TestBudgetTrackerNoBudget(t *testing.T) {
	bt := NewBudgetTracker(AgentBudget{}) // no limits

	bt.RecordTurn()
	bt.RecordTokens(999999)
	if reason := bt.Exceeded(); reason != "" {
		t.Errorf("unlimited budget should never be exceeded, got %q", reason)
	}
}

func TestBudgetSnapshot(t *testing.T) {
	bt := NewBudgetTracker(AgentBudget{MaxTurns: 10, MaxTokens: 5000})

	bt.RecordTurn()
	bt.RecordTurn()
	bt.RecordTokens(150)

	snap := bt.Snapshot()
	if snap.Turns != 2 {
		t.Errorf("turns = %d, want 2", snap.Turns)
	}
	if snap.Tokens != 150 {
		t.Errorf("tokens = %d, want 150", snap.Tokens)
	}
	if snap.Budget.MaxTurns != 10 {
		t.Errorf("budget.MaxTurns = %d, want 10", snap.Budget.MaxTurns)
	}
}
