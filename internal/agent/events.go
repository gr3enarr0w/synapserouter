package agent

import (
	"sync"
	"time"
)

// EventType identifies the kind of agent event.
type EventType int

const (
	EventPipelineStart  EventType = iota // Pipeline initialized
	EventPhaseStart                      // Pipeline phase begins
	EventPhaseComplete                   // Pipeline phase ends
	EventLLMStart                        // LLM call begins
	EventLLMComplete                     // LLM call ends
	EventToolStart                       // Tool execution begins
	EventToolComplete                    // Tool execution ends
	EventSubAgentSpawn                   // Child agent spawned
	EventSubAgentComplete                // Child agent finished
	EventEscalation                      // Provider escalation triggered
	EventSkillMatch                      // Skills matched against request
	EventQualityGate                     // Pipeline quality gate check
	EventParallelStart                   // Parallel phase begins
	EventCrossReview                     // Cross-review step
	EventBudgetUpdate                    // Budget state change
	EventError                           // Error occurred
	EventTokenStream                     // Streamed token chunk from LLM
	EventKReviewStart                    // K-LLM parallel review phase begins
	EventKReviewMerge                    // K-LLM findings merged
	EventPermissionRequest               // Permission request for tool execution
	EventTaskComplete                    // Task completed with summary
)

var eventTypeNames = map[EventType]string{
	EventPipelineStart:    "pipeline_start",
	EventPhaseStart:       "phase_start",
	EventPhaseComplete:    "phase_complete",
	EventLLMStart:         "llm_start",
	EventLLMComplete:      "llm_complete",
	EventToolStart:        "tool_start",
	EventToolComplete:     "tool_complete",
	EventSubAgentSpawn:    "subagent_spawn",
	EventSubAgentComplete: "subagent_complete",
	EventEscalation:       "escalation",
	EventSkillMatch:       "skill_match",
	EventQualityGate:      "quality_gate",
	EventParallelStart:    "parallel_start",
	EventCrossReview:      "cross_review",
	EventBudgetUpdate:     "budget_update",
	EventError:            "error",
	EventTokenStream:      "token_stream",
	EventKReviewStart:     "k_review_start",
	EventKReviewMerge:     "k_review_merge",
	EventPermissionRequest: "permission_request",
	EventTaskComplete:      "task_complete",
}

// String returns the event type name.
func (e EventType) String() string {
	if name, ok := eventTypeNames[e]; ok {
		return name
	}
	return "unknown"
}

// AgentEvent is a structured event emitted by the agent during execution.
type AgentEvent struct {
	Timestamp time.Time      `json:"timestamp"`
	AgentID   string         `json:"agent_id"`
	ParentID  string         `json:"parent_id,omitempty"`
	Type      EventType      `json:"type"`
	TypeName  string         `json:"type_name"`
	Phase     string         `json:"phase,omitempty"`
	Provider  string         `json:"provider,omitempty"`
	Data      map[string]any `json:"data,omitempty"`
}

// ActiveModel tracks a currently-running model call.
type ActiveModel struct {
	AgentID  string    `json:"agent_id"`
	Role     string    `json:"role,omitempty"` // e.g., "planner-1", "cross-review-2"
	Model    string    `json:"model"`
	Provider string    `json:"provider,omitempty"`
	StartedAt time.Time `json:"started_at"`
}

// EventBus provides a channel-based pub/sub for agent events.
// Producers call Publish; subscribers receive via Subscribe channels.
// Thread-safe: multiple goroutines can Publish concurrently.
// Also tracks active models for real-time "what's running?" queries.
type EventBus struct {
	mu          sync.RWMutex
	subscribers []chan AgentEvent
	closed      bool

	// Active model tracking
	modelMu      sync.RWMutex
	activeModels map[string]ActiveModel // keyed by agentID
}

// NewEventBus creates an event bus ready for publishing and subscribing.
func NewEventBus() *EventBus {
	return &EventBus{
		activeModels: make(map[string]ActiveModel),
	}
}

// Subscribe returns a channel that receives all published events.
// The channel is buffered to prevent slow subscribers from blocking producers.
func (b *EventBus) Subscribe() <-chan AgentEvent {
	b.mu.Lock()
	defer b.mu.Unlock()
	ch := make(chan AgentEvent, 256)
	b.subscribers = append(b.subscribers, ch)
	return ch
}

// Publish sends an event to all subscribers. Non-blocking: if a subscriber's
// buffer is full, the event is dropped for that subscriber (prevents deadlock).
// Also updates active model tracking for LLM start/complete events.
func (b *EventBus) Publish(event AgentEvent) {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	event.TypeName = event.Type.String()

	// Track active models
	switch event.Type {
	case EventLLMStart:
		b.modelMu.Lock()
		b.activeModels[event.AgentID] = ActiveModel{
			AgentID:   event.AgentID,
			Role:      str(event.Data, "role"),
			Model:     str(event.Data, "model"),
			Provider:  event.Provider,
			StartedAt: event.Timestamp,
		}
		b.modelMu.Unlock()
	case EventLLMComplete:
		b.modelMu.Lock()
		delete(b.activeModels, event.AgentID)
		b.modelMu.Unlock()
	}

	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.closed {
		return
	}
	for _, ch := range b.subscribers {
		select {
		case ch <- event:
		default:
			// subscriber buffer full, drop event
		}
	}
}

// ActiveModels returns a snapshot of all currently-running model calls.
func (b *EventBus) ActiveModels() []ActiveModel {
	b.modelMu.RLock()
	defer b.modelMu.RUnlock()
	models := make([]ActiveModel, 0, len(b.activeModels))
	for _, m := range b.activeModels {
		models = append(models, m)
	}
	return models
}

// Close closes all subscriber channels. Call once when the agent session ends.
func (b *EventBus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	for _, ch := range b.subscribers {
		close(ch)
	}
}
