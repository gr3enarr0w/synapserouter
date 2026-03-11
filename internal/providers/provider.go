package providers

import (
	"context"
	"time"
)

// ChatRequest represents an OpenAI-compatible chat request
type ChatRequest struct {
	Model           string                   `json:"model"`
	Messages        []Message                `json:"messages"`
	Temperature     float64                  `json:"temperature,omitempty"`
	MaxTokens       int                      `json:"max_tokens,omitempty"`
	Stream          bool                     `json:"stream,omitempty"`
	Tools           []map[string]interface{} `json:"tools,omitempty"`
	ToolChoice      interface{}              `json:"tool_choice,omitempty"`
	Functions       []map[string]interface{} `json:"functions,omitempty"`
	FunctionCall    interface{}              `json:"function_call,omitempty"`
	ReasoningEffort string                   `json:"reasoning_effort,omitempty"`
	Thinking        map[string]interface{}   `json:"thinking,omitempty"`

	// Memory control fields (not part of OpenAI spec)
	SkipMemory bool `json:"skip_memory,omitempty"`
	ForceStore bool `json:"force_store,omitempty"`
}

// Message represents a chat message
type Message struct {
	Role       string                   `json:"role"`
	Content    string                   `json:"content"`
	Name       string                   `json:"name,omitempty"`
	ToolCallID string                   `json:"tool_call_id,omitempty"`
	ToolCalls  []map[string]interface{} `json:"tool_calls,omitempty"`
}

// ChatResponse represents an OpenAI-compatible chat response
type ChatResponse struct {
	ID             string         `json:"id"`
	Object         string         `json:"object"`
	Created        int64          `json:"created"`
	Model          string         `json:"model"`
	Choices        []Choice       `json:"choices"`
	Usage          Usage          `json:"usage,omitempty"`
	XProxyMetadata *ProxyMetadata `json:"x_proxy_metadata,omitempty"`
}

// Choice represents a response choice
type Choice struct {
	Index        int         `json:"index"`
	Message      Message     `json:"message"`
	FinishReason string      `json:"finish_reason"`
	Delta        interface{} `json:"delta,omitempty"`
}

// Usage represents token usage stats
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type ProxyMetadata struct {
	Provider             string    `json:"provider,omitempty"`
	SessionID            string    `json:"session_id,omitempty"`
	MemoryQuery          string    `json:"memory_query,omitempty"`
	MemoryCandidateCount int       `json:"memory_candidate_count,omitempty"`
	MemoryCandidates     []Message `json:"memory_candidates,omitempty"`
}

// Provider interface that all LLM providers must implement
type Provider interface {
	Name() string
	ChatCompletion(ctx context.Context, req ChatRequest, sessionID string) (ChatResponse, error)
	IsHealthy(ctx context.Context) bool
	MaxContextTokens() int
	SupportsModel(model string) bool
}

// BaseProvider contains common fields for all providers
type BaseProvider struct {
	name       string
	baseURL    string
	apiKey     string
	maxContext int
	timeout    time.Duration
}

func (bp *BaseProvider) Name() string {
	return bp.name
}

func (bp *BaseProvider) MaxContextTokens() int {
	return bp.maxContext
}
