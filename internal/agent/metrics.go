package agent

import (
	"sync"
	"time"
)

// AgentMetrics tracks performance metrics for agent executions.
type AgentMetrics struct {
	mu sync.Mutex

	TotalRequests  int64         `json:"total_requests"`
	TotalTokens    int64         `json:"total_tokens"`
	TotalTurns     int64         `json:"total_turns"`
	TotalToolCalls int64         `json:"total_tool_calls"`
	TotalErrors    int64         `json:"total_errors"`
	TotalDuration  time.Duration `json:"total_duration"`

	// Per-tool metrics
	ToolCalls map[string]int64         `json:"tool_calls"`
	ToolTime  map[string]time.Duration `json:"tool_time"`

	// Sub-agent metrics
	SubAgentsSpawned int64 `json:"sub_agents_spawned"`
	HandoffsExecuted int64 `json:"handoffs_executed"`
}

// NewAgentMetrics creates a new metrics tracker.
func NewAgentMetrics() *AgentMetrics {
	return &AgentMetrics{
		ToolCalls: make(map[string]int64),
		ToolTime:  make(map[string]time.Duration),
	}
}

// RecordRequest records a complete agent request.
func (m *AgentMetrics) RecordRequest(duration time.Duration, tokens int64, turns int, err bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.TotalRequests++
	m.TotalDuration += duration
	m.TotalTokens += tokens
	m.TotalTurns += int64(turns)
	if err {
		m.TotalErrors++
	}
}

// RecordToolCall records a tool invocation.
func (m *AgentMetrics) RecordToolCall(toolName string, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.TotalToolCalls++
	m.ToolCalls[toolName]++
	m.ToolTime[toolName] += duration
}

// RecordSubAgent records a sub-agent spawn.
func (m *AgentMetrics) RecordSubAgent() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.SubAgentsSpawned++
}

// RecordHandoff records a handoff execution.
func (m *AgentMetrics) RecordHandoff() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.HandoffsExecuted++
}

// MetricsSnapshot is a copy of AgentMetrics without the mutex.
type MetricsSnapshot struct {
	TotalRequests    int64                    `json:"total_requests"`
	TotalTokens      int64                    `json:"total_tokens"`
	TotalTurns       int64                    `json:"total_turns"`
	TotalToolCalls   int64                    `json:"total_tool_calls"`
	TotalErrors      int64                    `json:"total_errors"`
	TotalDuration    time.Duration            `json:"total_duration"`
	SubAgentsSpawned int64                    `json:"sub_agents_spawned"`
	HandoffsExecuted int64                    `json:"handoffs_executed"`
	ToolCalls        map[string]int64         `json:"tool_calls"`
	ToolTime         map[string]time.Duration `json:"tool_time"`
}

// Snapshot returns a copy of current metrics.
func (m *AgentMetrics) Snapshot() MetricsSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	snap := MetricsSnapshot{
		TotalRequests:    m.TotalRequests,
		TotalTokens:      m.TotalTokens,
		TotalTurns:       m.TotalTurns,
		TotalToolCalls:   m.TotalToolCalls,
		TotalErrors:      m.TotalErrors,
		TotalDuration:    m.TotalDuration,
		SubAgentsSpawned: m.SubAgentsSpawned,
		HandoffsExecuted: m.HandoffsExecuted,
		ToolCalls:        make(map[string]int64, len(m.ToolCalls)),
		ToolTime:         make(map[string]time.Duration, len(m.ToolTime)),
	}
	for k, v := range m.ToolCalls {
		snap.ToolCalls[k] = v
	}
	for k, v := range m.ToolTime {
		snap.ToolTime[k] = v
	}
	return snap
}

// AvgRequestDuration returns average duration per request.
func (m *AgentMetrics) AvgRequestDuration() time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.TotalRequests == 0 {
		return 0
	}
	return m.TotalDuration / time.Duration(m.TotalRequests)
}

// AvgTokensPerRequest returns average tokens consumed per request.
func (m *AgentMetrics) AvgTokensPerRequest() int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.TotalRequests == 0 {
		return 0
	}
	return m.TotalTokens / m.TotalRequests
}
