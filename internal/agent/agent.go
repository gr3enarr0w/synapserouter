package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/providers"
	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/tools"
)

// ChatExecutor can execute chat completions against the LLM provider chain.
type ChatExecutor interface {
	ChatCompletion(ctx context.Context, req providers.ChatRequest, sessionID string) (providers.ChatResponse, error)
}

// Agent implements the core agent loop: message -> LLM -> tool calls -> LLM -> response.
type Agent struct {
	executor     ChatExecutor
	registry     *tools.Registry
	permissions  *tools.PermissionChecker
	conversation *Conversation
	renderer     *Renderer
	config       Config
	sessionID    string

	// Sub-agent hierarchy
	mu       sync.Mutex
	parentID string
	children []*ChildRef
	budget   *BudgetTracker
	pool     *Pool

	// Guardrails
	inputGuardrails  *GuardrailChain
	outputGuardrails *GuardrailChain

	// Observability
	trace   *Trace
	metrics *AgentMetrics
}

// New creates an agent with the given executor, tool registry, and config.
func New(executor ChatExecutor, registry *tools.Registry, renderer *Renderer, config Config) *Agent {
	return &Agent{
		executor:     executor,
		registry:     registry,
		permissions:  tools.NewPermissionChecker(tools.ModeAutoApprove),
		conversation: NewConversation(),
		renderer:     renderer,
		config:       config,
		sessionID:    fmt.Sprintf("agent-%d", uniqueID()),
	}
}

// SetPermissions sets the permission checker for tool execution.
func (a *Agent) SetPermissions(pc *tools.PermissionChecker) {
	a.permissions = pc
}

// SetPool sets the agent pool for concurrency management.
func (a *Agent) SetPool(pool *Pool) {
	a.pool = pool
}

// SetInputGuardrails sets guardrails applied to user input.
func (a *Agent) SetInputGuardrails(gc *GuardrailChain) {
	a.inputGuardrails = gc
}

// SetOutputGuardrails sets guardrails applied to agent output.
func (a *Agent) SetOutputGuardrails(gc *GuardrailChain) {
	a.outputGuardrails = gc
}

// EnableTracing starts recording structured trace spans.
func (a *Agent) EnableTracing() {
	a.trace = NewTrace(a.sessionID, a.parentID)
}

// Trace returns the agent's trace, or nil if tracing is disabled.
func (a *Agent) Trace() *Trace {
	return a.trace
}

// SetMetrics attaches a metrics tracker to this agent.
func (a *Agent) SetMetrics(m *AgentMetrics) {
	a.metrics = m
}

// Metrics returns the agent's metrics tracker.
func (a *Agent) Metrics() *AgentMetrics {
	return a.metrics
}

// Run processes a user message through the agent loop and returns the final text response.
func (a *Agent) Run(ctx context.Context, userMessage string) (string, error) {
	// Input guardrails
	if a.inputGuardrails != nil {
		if result := a.inputGuardrails.Validate(userMessage); !result.Passed {
			return "", fmt.Errorf("input guardrail: %s", result.Reason)
		}
	}

	a.conversation.Add(providers.Message{
		Role:    "user",
		Content: userMessage,
	})

	start := time.Now()
	response, err := a.loop(ctx)

	// Record metrics
	if a.metrics != nil {
		turns := len(a.conversation.Messages()) / 2 // rough estimate
		a.metrics.RecordRequest(time.Since(start), 0, turns, err != nil)
	}

	if err != nil {
		return "", err
	}

	// Output guardrails
	if a.outputGuardrails != nil {
		if result := a.outputGuardrails.Validate(response); !result.Passed {
			return "", fmt.Errorf("output guardrail: %s", result.Reason)
		}
	}

	return response, nil
}

// Clear resets the conversation history.
func (a *Agent) Clear() {
	a.conversation.Clear()
}

// SessionID returns the agent's session ID.
func (a *Agent) SessionID() string {
	return a.sessionID
}

func (a *Agent) loop(ctx context.Context) (string, error) {
	for turn := 0; turn < a.config.MaxTurns; turn++ {
		// Budget check
		if a.budget != nil {
			a.budget.RecordTurn()
			if reason := a.budget.Exceeded(); reason != "" {
				return "", fmt.Errorf("agent budget exceeded: %s", reason)
			}
		}

		messages := a.buildMessages()
		toolDefs := a.registry.OpenAIToolDefinitions()

		req := providers.ChatRequest{
			Model:    a.config.Model,
			Messages: messages,
			Tools:    toolDefs,
		}

		// Trace LLM call
		var endLLMSpan func(error)
		if a.trace != nil {
			endLLMSpan = a.trace.StartSpan("llm_call", "llm_call", map[string]interface{}{
				"model": a.config.Model,
				"turn":  turn,
			})
		}

		resp, err := a.executor.ChatCompletion(ctx, req, a.sessionID)
		if endLLMSpan != nil {
			endLLMSpan(err)
		}
		if err != nil {
			return "", fmt.Errorf("LLM call failed: %w", err)
		}

		// Track token usage
		if a.budget != nil && resp.Usage.TotalTokens > 0 {
			a.budget.RecordTokens(int64(resp.Usage.TotalTokens))
		}

		if len(resp.Choices) == 0 {
			return "", fmt.Errorf("empty response from LLM")
		}

		choice := resp.Choices[0]
		assistantMsg := choice.Message
		if strings.TrimSpace(assistantMsg.Role) == "" {
			assistantMsg.Role = "assistant"
		}
		a.conversation.Add(assistantMsg)

		// If no tool calls, we're done
		if len(assistantMsg.ToolCalls) == 0 {
			return assistantMsg.Content, nil
		}

		// Execute tool calls
		for _, toolCall := range assistantMsg.ToolCalls {
			callID := extractToolCallID(toolCall)
			name, args := extractToolCallNameArgs(toolCall)

			if a.renderer != nil {
				a.renderer.ToolCall(name, args)
			}

			toolStart := time.Now()
			result, execErr := a.registry.ExecuteChecked(ctx, name, args, a.config.WorkDir, a.permissions)
			toolDuration := time.Since(toolStart)

			// Trace and metrics for tool call
			if a.trace != nil {
				a.trace.AddSpan(Span{
					Name:      name,
					Type:      "tool_call",
					StartTime: toolStart,
					Duration:  toolDuration,
					Metadata:  map[string]interface{}{"args": args},
				})
			}
			if a.metrics != nil {
				a.metrics.RecordToolCall(name, toolDuration)
			}
			var resultContent string
			if execErr != nil {
				resultContent = fmt.Sprintf("error: %v", execErr)
				if a.renderer != nil {
					a.renderer.ToolResult(name, resultContent, true)
				}
			} else {
				resultContent = result.Output
				if result.Error != "" {
					resultContent = result.Error + "\n" + resultContent
				}
				if a.renderer != nil {
					a.renderer.ToolResult(name, resultContent, result.Error != "")
				}
			}

			a.conversation.Add(providers.Message{
				Role:       "tool",
				ToolCallID: callID,
				Content:    resultContent,
			})
		}
	}

	return "", fmt.Errorf("agent exceeded max turns (%d)", a.config.MaxTurns)
}

func (a *Agent) buildMessages() []providers.Message {
	var msgs []providers.Message

	// System prompt
	sysPrompt := a.config.SystemPrompt
	if sysPrompt == "" {
		sysPrompt = defaultSystemPrompt(a.config.WorkDir)
	}
	msgs = append(msgs, providers.Message{
		Role:    "system",
		Content: sysPrompt,
	})

	// Conversation history
	msgs = append(msgs, a.conversation.Messages()...)
	return msgs
}

func defaultSystemPrompt(workDir string) string {
	return fmt.Sprintf(`You are a coding assistant with access to tools for reading files, editing code, running commands, and managing git repositories. You are working in: %s

Use the available tools to help the user with their request. Be concise and direct.
When making changes to code, read the relevant files first to understand the context.
After making changes, verify them by running appropriate tests or commands.`, workDir)
}

func extractToolCallID(tc map[string]interface{}) string {
	if id, ok := tc["id"].(string); ok && id != "" {
		return id
	}
	if id, ok := tc["call_id"].(string); ok && id != "" {
		return id
	}
	return ""
}

func extractToolCallNameArgs(tc map[string]interface{}) (string, map[string]interface{}) {
	fn, _ := tc["function"].(map[string]interface{})
	name := ""
	if fn != nil {
		name, _ = fn["name"].(string)
	}
	if name == "" {
		name, _ = tc["name"].(string)
	}

	var args map[string]interface{}
	if fn != nil {
		args = parseArguments(fn["arguments"])
	}
	if args == nil {
		args = parseArguments(tc["arguments"])
	}
	if args == nil {
		args = make(map[string]interface{})
	}
	return name, args
}

func parseArguments(raw interface{}) map[string]interface{} {
	switch v := raw.(type) {
	case map[string]interface{}:
		return v
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}
		var decoded map[string]interface{}
		if err := json.Unmarshal([]byte(v), &decoded); err == nil {
			return decoded
		}
	}
	return nil
}

var idCounter int64

func uniqueID() int64 {
	return atomic.AddInt64(&idCounter, 1)
}
