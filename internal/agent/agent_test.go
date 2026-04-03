package agent

import (
	"bytes"
	"context"
	"testing"

	"github.com/gr3enarr0w/synapserouter/internal/providers"
	"github.com/gr3enarr0w/synapserouter/internal/tools"
)

// mockExecutor simulates LLM responses for testing.
type mockExecutor struct {
	responses []providers.ChatResponse
	callIndex int
}

func (m *mockExecutor) ChatCompletion(ctx context.Context, req providers.ChatRequest, sessionID string) (providers.ChatResponse, error) {
	if m.callIndex >= len(m.responses) {
		return providers.ChatResponse{
			Choices: []providers.Choice{{
				Message: providers.Message{Role: "assistant", Content: "fallback response"},
			}},
		}, nil
	}
	resp := m.responses[m.callIndex]
	m.callIndex++
	return resp, nil
}

func TestAgentSimpleResponse(t *testing.T) {
	exec := &mockExecutor{
		responses: []providers.ChatResponse{{
			Choices: []providers.Choice{{
				Message: providers.Message{Role: "assistant", Content: "Hello! How can I help?"},
			}},
		}},
	}

	registry := tools.NewRegistry()
	renderer := NewRenderer(&bytes.Buffer{})
	config := DefaultConfig()
	config.WorkDir = t.TempDir()

	agent := New(exec, registry, renderer, config)
	result, err := agent.Run(context.Background(), "hi")
	if err != nil {
		t.Fatal(err)
	}
	if result != "Hello! How can I help?" {
		t.Errorf("expected greeting, got %q", result)
	}
}

func TestAgentToolCallLoop(t *testing.T) {
	dir := t.TempDir()

	exec := &mockExecutor{
		responses: []providers.ChatResponse{
			// First response: tool call
			{Choices: []providers.Choice{{
				Message: providers.Message{
					Role: "assistant",
					ToolCalls: []map[string]interface{}{
						{
							"id":   "call_1",
							"type": "function",
							"function": map[string]interface{}{
								"name":      "bash",
								"arguments": `{"command": "echo test-output"}`,
							},
						},
					},
				},
			}}},
			// Second response: final text
			{Choices: []providers.Choice{{
				Message: providers.Message{Role: "assistant", Content: "I ran the command. Output was: test-output"},
			}}},
		},
	}

	registry := tools.DefaultRegistry()
	var output bytes.Buffer
	renderer := NewRenderer(&output)
	config := DefaultConfig()
	config.WorkDir = dir

	agent := New(exec, registry, renderer, config)
	result, err := agent.Run(context.Background(), "run echo test-output")
	if err != nil {
		t.Fatal(err)
	}
	if result != "I ran the command. Output was: test-output" {
		t.Errorf("unexpected result: %q", result)
	}
	if exec.callIndex != 2 {
		t.Errorf("expected 2 LLM calls, got %d", exec.callIndex)
	}
}

func TestConversation(t *testing.T) {
	conv := NewConversation()

	conv.Add(providers.Message{Role: "user", Content: "hello"})
	conv.Add(providers.Message{Role: "assistant", Content: "hi"})

	if len(conv.Messages()) != 2 {
		t.Errorf("expected 2 messages, got %d", len(conv.Messages()))
	}

	conv.Clear()
	if len(conv.Messages()) != 0 {
		t.Errorf("expected 0 messages after clear, got %d", len(conv.Messages()))
	}
}

func TestConversationTrim(t *testing.T) {
	conv := NewConversation()
	for i := 0; i < maxConversationMessages+50; i++ {
		conv.Add(providers.Message{Role: "user", Content: "msg"})
	}
	if len(conv.Messages()) > maxConversationMessages {
		t.Errorf("expected at most %d messages, got %d", maxConversationMessages, len(conv.Messages()))
	}
}

func TestRendererToolCall(t *testing.T) {
	var buf bytes.Buffer
	r := NewRenderer(&buf)

	r.ToolCall("bash", map[string]interface{}{"command": "ls -la"})
	output := buf.String()
	if output == "" {
		t.Error("expected output from ToolCall")
	}
	if !bytes.Contains([]byte(output), []byte("bash")) {
		t.Errorf("expected 'bash' in output, got %q", output)
	}
}

func TestRendererToolResult(t *testing.T) {
	var buf bytes.Buffer
	r := NewRenderer(&buf)

	r.ToolResult("bash", "hello world", false)
	if !bytes.Contains(buf.Bytes(), []byte("hello world")) {
		t.Errorf("expected 'hello world' in output, got %q", buf.String())
	}
}

func TestREPLCommands(t *testing.T) {
	exec := &mockExecutor{}
	registry := tools.DefaultRegistry()
	var output bytes.Buffer
	renderer := NewRenderer(&output)
	config := DefaultConfig()
	config.WorkDir = t.TempDir()
	agent := New(exec, registry, renderer, config)

	repl := NewREPL(agent, renderer)
	var replOut bytes.Buffer
	repl.out = &replOut

	// Test /help command
	done := repl.handleCommand(context.Background(), "/help")
	if done {
		t.Error("/help should not exit")
	}
	if !bytes.Contains(replOut.Bytes(), []byte("/exit")) {
		t.Error("help output should mention /exit")
	}

	// Test /model command
	replOut.Reset()
	repl.handleCommand(context.Background(), "/model claude-sonnet-4-6")
	if agent.config.Model != "claude-sonnet-4-6" {
		t.Errorf("expected model claude-sonnet-4-6, got %s", agent.config.Model)
	}

	// Test /tools command
	replOut.Reset()
	repl.handleCommand(context.Background(), "/tools")
	if !bytes.Contains(replOut.Bytes(), []byte("bash")) {
		t.Error("tools output should list bash")
	}

	// Test /clear command
	agent.conversation.Add(providers.Message{Role: "user", Content: "test"})
	repl.handleCommand(context.Background(), "/clear")
	if len(agent.conversation.Messages()) != 0 {
		t.Error("clear should empty conversation")
	}

	// Test /exit command
	done = repl.handleCommand(context.Background(), "/exit")
	if !done {
		t.Error("/exit should return true")
	}
}

func TestExtractToolCallNameArgs(t *testing.T) {
	tc := map[string]interface{}{
		"id":   "call_1",
		"type": "function",
		"function": map[string]interface{}{
			"name":      "bash",
			"arguments": `{"command": "ls"}`,
		},
	}

	name, args := extractToolCallNameArgs(tc)
	if name != "bash" {
		t.Errorf("expected 'bash', got %q", name)
	}
	if args["command"] != "ls" {
		t.Errorf("expected command='ls', got %v", args["command"])
	}
}
