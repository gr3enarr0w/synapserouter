package agent

import (
	"sync"
	"time"
)

// AgentBudget defines resource limits for an agent.
type AgentBudget struct {
	MaxTurns    int           `json:"max_turns,omitempty"`
	MaxTokens   int64         `json:"max_tokens,omitempty"`
	MaxDuration time.Duration `json:"max_duration,omitempty"`
}

// BudgetTracker tracks resource consumption against a budget.
type BudgetTracker struct {
	mu        sync.Mutex
	budget    AgentBudget
	turns     int
	tokens    int64
	startTime time.Time
}

// NewBudgetTracker creates a tracker for the given budget.
func NewBudgetTracker(budget AgentBudget) *BudgetTracker {
	return &BudgetTracker{
		budget:    budget,
		startTime: time.Now(),
	}
}

// RecordTurn increments the turn counter.
func (bt *BudgetTracker) RecordTurn() {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	bt.turns++
}

// RecordTokens adds token usage.
func (bt *BudgetTracker) RecordTokens(n int64) {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	bt.tokens += n
}

// Exceeded returns a non-empty reason string if any budget limit is exceeded.
func (bt *BudgetTracker) Exceeded() string {
	bt.mu.Lock()
	defer bt.mu.Unlock()

	if bt.budget.MaxTurns > 0 && bt.turns >= bt.budget.MaxTurns {
		return "turn limit exceeded"
	}
	if bt.budget.MaxTokens > 0 && bt.tokens >= bt.budget.MaxTokens {
		return "token limit exceeded"
	}
	if bt.budget.MaxDuration > 0 && time.Since(bt.startTime) >= bt.budget.MaxDuration {
		return "duration limit exceeded"
	}
	return ""
}

// Snapshot returns current usage stats.
func (bt *BudgetTracker) Snapshot() BudgetSnapshot {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	return BudgetSnapshot{
		Turns:    bt.turns,
		Tokens:   bt.tokens,
		Elapsed:  time.Since(bt.startTime),
		Budget:   bt.budget,
	}
}

// BudgetSnapshot captures a point-in-time view of resource usage.
type BudgetSnapshot struct {
	Turns   int           `json:"turns"`
	Tokens  int64         `json:"tokens"`
	Elapsed time.Duration `json:"elapsed"`
	Budget  AgentBudget   `json:"budget"`
}
