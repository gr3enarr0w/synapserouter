package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/providers"
	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/tools"
)

// Handoff represents a request to transfer control to a specialist agent.
// Inspired by OpenAI Swarm's handoff pattern: when an agent determines
// another specialist is better suited, it returns a Handoff instead of text.
type Handoff struct {
	TargetRole string   `json:"target_role"` // agent type to hand off to
	Context    string   `json:"context"`     // summary for the target
	Files      []string `json:"files,omitempty"`
	Model      string   `json:"model,omitempty"`
}

// HandoffResult holds the outcome of a completed handoff.
type HandoffResult struct {
	FromAgent string `json:"from_agent"`
	ToRole    string `json:"to_role"`
	Context   string `json:"context"`
	Result    string `json:"result"`
	Error     string `json:"error,omitempty"`
}

// ExecuteHandoff creates a specialist agent for the handoff target and runs it.
// The specialist receives the handoff context plus relevant conversation history.
func (a *Agent) ExecuteHandoff(ctx context.Context, h Handoff) (*HandoffResult, error) {
	model := h.Model
	if model == "" {
		model = a.config.Model
	}

	// Build context summary from handoff + recent conversation
	contextSummary := buildHandoffContext(h, a.conversation.Messages())

	child := a.SpawnChild(SpawnConfig{
		Role:  h.TargetRole,
		Model: model,
		WorkDir: a.config.WorkDir,
		System: handoffSystemPrompt(h.TargetRole, h.Files),
	})

	result, err := child.Run(ctx, contextSummary)
	hr := &HandoffResult{
		FromAgent: a.sessionID,
		ToRole:    h.TargetRole,
		Context:   h.Context,
		Result:    result,
	}
	if err != nil {
		hr.Error = err.Error()
		return hr, err
	}
	return hr, nil
}

// buildHandoffContext creates a prompt for the target agent that includes
// the handoff context and a summary of relevant conversation history.
func buildHandoffContext(h Handoff, messages []providers.Message) string {
	var sb strings.Builder

	sb.WriteString("## Handoff Context\n")
	sb.WriteString(h.Context)
	sb.WriteString("\n\n")

	if len(h.Files) > 0 {
		sb.WriteString("## Relevant Files\n")
		for _, f := range h.Files {
			sb.WriteString("- ")
			sb.WriteString(f)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	// Include last few meaningful messages for context
	relevant := recentUserMessages(messages, 3)
	if len(relevant) > 0 {
		sb.WriteString("## Recent Conversation\n")
		for _, msg := range relevant {
			sb.WriteString(fmt.Sprintf("[%s]: %s\n", msg.Role, truncate(msg.Content, 500)))
		}
	}

	return sb.String()
}

func handoffSystemPrompt(role string, files []string) string {
	prompt := fmt.Sprintf(`You are a specialist %s agent. You have received a handoff from another agent because your expertise is needed.

Focus on the specific task described in the handoff context. Use the available tools as needed.
When done, provide a clear summary of what you accomplished.`, role)

	if len(files) > 0 {
		prompt += "\n\nRelevant files to examine: " + strings.Join(files, ", ")
	}
	return prompt
}

func recentUserMessages(messages []providers.Message, n int) []providers.Message {
	var relevant []providers.Message
	for i := len(messages) - 1; i >= 0 && len(relevant) < n; i-- {
		msg := messages[i]
		if msg.Role == "user" || (msg.Role == "assistant" && len(msg.ToolCalls) == 0 && msg.Content != "") {
			relevant = append(relevant, msg)
		}
	}
	// Reverse to preserve chronological order
	for i, j := 0, len(relevant)-1; i < j; i, j = i+1, j-1 {
		relevant[i], relevant[j] = relevant[j], relevant[i]
	}
	return relevant
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// DelegateTool is an agent tool that enables sub-agent delegation from within the agent loop.
type DelegateTool struct {
	spawner AgentSpawner
}

// AgentSpawner is the interface used by DelegateTool to create child agents.
type AgentSpawner interface {
	RunChild(ctx context.Context, cfg SpawnConfig, task string) (string, error)
}

func NewDelegateTool(spawner AgentSpawner) *DelegateTool {
	return &DelegateTool{spawner: spawner}
}

func (t *DelegateTool) Name() string        { return "delegate" }
func (t *DelegateTool) Description() string  { return "Delegate a subtask to a specialist sub-agent" }
func (t *DelegateTool) Category() tools.ToolCategory { return tools.CategoryWrite }

func (t *DelegateTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"role": map[string]interface{}{
				"type":        "string",
				"description": "Specialist role (e.g., tester, researcher, security-audit, reviewer)",
			},
			"task": map[string]interface{}{
				"type":        "string",
				"description": "The task to delegate to the sub-agent",
			},
			"max_turns": map[string]interface{}{
				"type":        "integer",
				"description": "Maximum turns for the sub-agent (default: 10)",
			},
		},
		"required": []string{"role", "task"},
	}
}

func (t *DelegateTool) Execute(ctx context.Context, args map[string]interface{}, workDir string) (*tools.ToolResult, error) {
	role, _ := args["role"].(string)
	task, _ := args["task"].(string)
	if role == "" || task == "" {
		return &tools.ToolResult{Error: "role and task are required"}, nil
	}

	maxTurns := 10
	if mt, ok := args["max_turns"].(float64); ok && mt > 0 {
		maxTurns = int(mt)
	}

	cfg := SpawnConfig{
		Role:    role,
		WorkDir: workDir,
		Budget:  &AgentBudget{MaxTurns: maxTurns},
	}

	result, err := t.spawner.RunChild(ctx, cfg, task)
	if err != nil {
		return &tools.ToolResult{Error: fmt.Sprintf("sub-agent failed: %v", err)}, nil
	}

	return &tools.ToolResult{Output: result}, nil
}

// HandoffTool is an agent tool that enables handoff to a specialist agent.
type HandoffTool struct {
	agent *Agent
}

func NewHandoffTool(agent *Agent) *HandoffTool {
	return &HandoffTool{agent: agent}
}

func (t *HandoffTool) Name() string        { return "handoff" }
func (t *HandoffTool) Description() string  { return "Hand off to a specialist agent with full context transfer" }
func (t *HandoffTool) Category() tools.ToolCategory { return tools.CategoryWrite }

func (t *HandoffTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"role": map[string]interface{}{
				"type":        "string",
				"description": "Specialist role to hand off to",
			},
			"context": map[string]interface{}{
				"type":        "string",
				"description": "Summary of what the specialist should focus on",
			},
			"files": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "Relevant file paths for the specialist",
			},
		},
		"required": []string{"role", "context"},
	}
}

func (t *HandoffTool) Execute(ctx context.Context, args map[string]interface{}, workDir string) (*tools.ToolResult, error) {
	role, _ := args["role"].(string)
	ctxStr, _ := args["context"].(string)
	if role == "" || ctxStr == "" {
		return &tools.ToolResult{Error: "role and context are required"}, nil
	}

	var files []string
	if f, ok := args["files"].([]interface{}); ok {
		for _, v := range f {
			if s, ok := v.(string); ok {
				files = append(files, s)
			}
		}
	}

	h := Handoff{
		TargetRole: role,
		Context:    ctxStr,
		Files:      files,
	}

	result, err := t.agent.ExecuteHandoff(ctx, h)
	if err != nil {
		return &tools.ToolResult{Error: fmt.Sprintf("handoff failed: %v", err)}, nil
	}

	return &tools.ToolResult{Output: result.Result}, nil
}
