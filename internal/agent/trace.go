package agent

import (
	"encoding/json"
	"sync"
	"time"
)

// Trace captures structured events from an agent execution.
type Trace struct {
	mu      sync.Mutex
	ID      string    `json:"id"`
	AgentID string    `json:"agent_id"`
	ParentID string   `json:"parent_id,omitempty"`
	Spans   []Span    `json:"spans"`
	StartTime time.Time `json:"start_time"`
}

// Span is a timed event within a trace.
type Span struct {
	Name      string                 `json:"name"`
	Type      string                 `json:"type"` // "llm_call", "tool_call", "handoff", "delegate"
	StartTime time.Time              `json:"start_time"`
	Duration  time.Duration          `json:"duration"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	Error     string                 `json:"error,omitempty"`
	Children  []Span                 `json:"children,omitempty"`
}

// NewTrace creates a new trace for the given agent.
func NewTrace(agentID, parentID string) *Trace {
	return &Trace{
		ID:        "trace-" + agentID,
		AgentID:   agentID,
		ParentID:  parentID,
		StartTime: time.Now(),
	}
}

// StartSpan begins a new span and returns a function to end it.
func (t *Trace) StartSpan(name, spanType string, metadata map[string]interface{}) func(err error) {
	start := time.Now()
	return func(err error) {
		span := Span{
			Name:      name,
			Type:      spanType,
			StartTime: start,
			Duration:  time.Since(start),
			Metadata:  metadata,
		}
		if err != nil {
			span.Error = err.Error()
		}
		t.mu.Lock()
		t.Spans = append(t.Spans, span)
		t.mu.Unlock()
	}
}

// AddSpan directly adds a completed span.
func (t *Trace) AddSpan(span Span) {
	t.mu.Lock()
	t.Spans = append(t.Spans, span)
	t.mu.Unlock()
}

// JSON returns the trace as JSON bytes.
func (t *Trace) JSON() ([]byte, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return json.MarshalIndent(t, "", "  ")
}

// SpanCount returns the number of recorded spans.
func (t *Trace) SpanCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.Spans)
}

// SpansByType returns all spans of a given type.
func (t *Trace) SpansByType(spanType string) []Span {
	t.mu.Lock()
	defer t.mu.Unlock()
	var matched []Span
	for _, s := range t.Spans {
		if s.Type == spanType {
			matched = append(matched, s)
		}
	}
	return matched
}

// TotalDuration returns the total trace duration.
func (t *Trace) TotalDuration() time.Duration {
	return time.Since(t.StartTime)
}
