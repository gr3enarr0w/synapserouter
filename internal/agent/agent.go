package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"

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

// Run processes a user message through the agent loop and returns the final text response.
func (a *Agent) Run(ctx context.Context, userMessage string) (string, error) {
	a.conversation.Add(providers.Message{
		Role:    "user",
		Content: userMessage,
	})

	return a.loop(ctx)
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
		messages := a.buildMessages()
		toolDefs := a.registry.OpenAIToolDefinitions()

		req := providers.ChatRequest{
			Model:    a.config.Model,
			Messages: messages,
			Tools:    toolDefs,
		}

		resp, err := a.executor.ChatCompletion(ctx, req, a.sessionID)
		if err != nil {
			return "", fmt.Errorf("LLM call failed: %w", err)
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

			result, execErr := a.registry.ExecuteChecked(ctx, name, args, a.config.WorkDir, a.permissions)
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
