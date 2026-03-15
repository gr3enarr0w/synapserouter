package agent

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Pool manages a set of concurrent agents with resource limits.
type Pool struct {
	mu            sync.Mutex
	maxConcurrent int
	semaphore     chan struct{}
	active        map[string]*Agent
	completed     []*PoolEntry
	totalSpawned  int
}

// PoolEntry records metadata about an agent in the pool.
type PoolEntry struct {
	AgentID   string    `json:"agent_id"`
	ParentID  string    `json:"parent_id,omitempty"`
	Role      string    `json:"role"`
	Status    string    `json:"status"` // "active", "completed", "failed"
	StartedAt time.Time `json:"started_at"`
	Duration  time.Duration `json:"duration,omitempty"`
}

// PoolMetrics reports pool utilization.
type PoolMetrics struct {
	MaxConcurrent int `json:"max_concurrent"`
	Active        int `json:"active"`
	TotalSpawned  int `json:"total_spawned"`
	Completed     int `json:"completed"`
}

// NewPool creates an agent pool with the given concurrency limit.
func NewPool(maxConcurrent int) *Pool {
	if maxConcurrent <= 0 {
		maxConcurrent = 5
	}
	return &Pool{
		maxConcurrent: maxConcurrent,
		semaphore:     make(chan struct{}, maxConcurrent),
		active:        make(map[string]*Agent),
	}
}

// Acquire reserves a slot in the pool for an agent. Blocks if at capacity.
// Returns a release function that must be called when the agent is done.
func (p *Pool) Acquire(ctx context.Context, agent *Agent, role string) (func(), error) {
	select {
	case p.semaphore <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	p.mu.Lock()
	p.active[agent.sessionID] = agent
	p.totalSpawned++
	entry := &PoolEntry{
		AgentID:   agent.sessionID,
		ParentID:  agent.parentID,
		Role:      role,
		Status:    "active",
		StartedAt: time.Now(),
	}
	p.mu.Unlock()

	release := func() {
		p.mu.Lock()
		delete(p.active, agent.sessionID)
		entry.Duration = time.Since(entry.StartedAt)
		entry.Status = "completed"
		p.completed = append(p.completed, entry)
		p.mu.Unlock()
		<-p.semaphore
	}

	return release, nil
}

// Metrics returns current pool utilization stats.
func (p *Pool) Metrics() PoolMetrics {
	p.mu.Lock()
	defer p.mu.Unlock()
	return PoolMetrics{
		MaxConcurrent: p.maxConcurrent,
		Active:        len(p.active),
		TotalSpawned:  p.totalSpawned,
		Completed:     len(p.completed),
	}
}

// ActiveAgents returns the IDs of currently running agents.
func (p *Pool) ActiveAgents() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	ids := make([]string, 0, len(p.active))
	for id := range p.active {
		ids = append(ids, id)
	}
	return ids
}

// RunInPool acquires a pool slot, runs the agent, and releases the slot.
func (p *Pool) RunInPool(ctx context.Context, agent *Agent, role, task string) (string, error) {
	release, err := p.Acquire(ctx, agent, role)
	if err != nil {
		return "", fmt.Errorf("pool acquire failed: %w", err)
	}
	defer release()

	return agent.Run(ctx, task)
}
