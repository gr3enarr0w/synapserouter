package agent

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gr3enarr0w/synapserouter/internal/providers"
	"github.com/gr3enarr0w/synapserouter/internal/tools"
)

// --- Scenario: Multi-turn tool-calling agent session ---

// sequenceExecutor returns pre-defined responses in order, simulating an LLM
// that decides to use tools and then responds with a summary.
type sequenceExecutor struct {
	responses []providers.ChatResponse
	calls     []providers.ChatRequest // captured requests
	idx       int
}

func (s *sequenceExecutor) ChatCompletion(ctx context.Context, req providers.ChatRequest, sessionID string) (providers.ChatResponse, error) {
	s.calls = append(s.calls, req)
	if s.idx >= len(s.responses) {
		return providers.ChatResponse{
			Choices: []providers.Choice{{
				Message: providers.Message{Role: "assistant", Content: "(fallback)"},
			}},
		}, nil
	}
	resp := s.responses[s.idx]
	s.idx++
	return resp, nil
}

func TestScenarioReadEditVerify(t *testing.T) {
	// Scenario: User asks agent to fix a typo in a file.
	// Agent should: 1) read file, 2) edit file, 3) verify with read, 4) respond.
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "hello.go"), []byte(`package main
func main() {
	fmt.Println("Helo World")
}
`), 0644)

	exec := &sequenceExecutor{
		responses: []providers.ChatResponse{
			// Turn 1: Agent reads the file
			{Choices: []providers.Choice{{Message: providers.Message{
				Role: "assistant",
				ToolCalls: []map[string]interface{}{
					{"id": "c1", "type": "function", "function": map[string]interface{}{
						"name": "file_read", "arguments": `{"path":"hello.go"}`,
					}},
				},
			}}}},
			// Turn 2: Agent edits the file
			{Choices: []providers.Choice{{Message: providers.Message{
				Role: "assistant",
				ToolCalls: []map[string]interface{}{
					{"id": "c2", "type": "function", "function": map[string]interface{}{
						"name": "file_edit", "arguments": `{"path":"hello.go","old_string":"Helo","new_string":"Hello"}`,
					}},
				},
			}}}},
			// Turn 3: Agent reads again to verify
			{Choices: []providers.Choice{{Message: providers.Message{
				Role: "assistant",
				ToolCalls: []map[string]interface{}{
					{"id": "c3", "type": "function", "function": map[string]interface{}{
						"name": "file_read", "arguments": `{"path":"hello.go"}`,
					}},
				},
			}}}},
			// Turn 4: Final response
			{Choices: []providers.Choice{{Message: providers.Message{
				Role:    "assistant",
				Content: "Fixed the typo: changed 'Helo' to 'Hello' in hello.go.",
			}}}},
		},
	}

	registry := tools.DefaultRegistry()
	var output bytes.Buffer
	renderer := NewRenderer(&output)
	config := DefaultConfig()
	config.WorkDir = dir

	agent := New(exec, registry, renderer, config)
	result, err := agent.Run(context.Background(), "Fix the typo in hello.go")
	if err != nil {
		t.Fatal(err)
	}

	// Verify final response
	if !strings.Contains(result, "Fixed the typo") {
		t.Errorf("expected fix confirmation, got %q", result)
	}

	// Verify 4 LLM calls were made
	if exec.idx != 4 {
		t.Errorf("expected 4 LLM calls, got %d", exec.idx)
	}

	// Verify file was actually fixed
	data, _ := os.ReadFile(filepath.Join(dir, "hello.go"))
	if !strings.Contains(string(data), "Hello World") {
		t.Errorf("file should contain 'Hello World', got %q", string(data))
	}

	// Verify renderer showed tool calls
	rendered := output.String()
	if !strings.Contains(rendered, "[file_read]") {
		t.Error("renderer should show file_read tool call")
	}
	if !strings.Contains(rendered, "[file_edit]") {
		t.Error("renderer should show file_edit tool call")
	}
}

func TestScenarioBashAndGlob(t *testing.T) {
	// Scenario: User asks to list Go files and run go vet.
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0644)
	os.WriteFile(filepath.Join(dir, "lib.go"), []byte("package main\n"), 0644)

	exec := &sequenceExecutor{
		responses: []providers.ChatResponse{
			// Agent uses glob to find files
			{Choices: []providers.Choice{{Message: providers.Message{
				Role: "assistant",
				ToolCalls: []map[string]interface{}{
					{"id": "c1", "type": "function", "function": map[string]interface{}{
						"name": "glob", "arguments": `{"pattern":"**/*.go"}`,
					}},
				},
			}}}},
			// Agent uses bash to run a command
			{Choices: []providers.Choice{{Message: providers.Message{
				Role: "assistant",
				ToolCalls: []map[string]interface{}{
					{"id": "c2", "type": "function", "function": map[string]interface{}{
						"name": "bash", "arguments": `{"command":"echo 'all clear'"}`,
					}},
				},
			}}}},
			// Final response
			{Choices: []providers.Choice{{Message: providers.Message{
				Role:    "assistant",
				Content: "Found 2 Go files: main.go and lib.go. All checks pass.",
			}}}},
		},
	}

	registry := tools.DefaultRegistry()
	var output bytes.Buffer
	renderer := NewRenderer(&output)
	config := DefaultConfig()
	config.WorkDir = dir

	agent := New(exec, registry, renderer, config)
	result, err := agent.Run(context.Background(), "list all Go files and check them")
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "2 Go files") {
		t.Errorf("expected summary, got %q", result)
	}
	if exec.idx != 3 {
		t.Errorf("expected 3 LLM calls, got %d", exec.idx)
	}
}

func TestScenarioParallelToolCalls(t *testing.T) {
	// Scenario: LLM issues multiple tool calls in a single turn.
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("file a"), 0644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("file b"), 0644)

	exec := &sequenceExecutor{
		responses: []providers.ChatResponse{
			// Single turn with two tool calls
			{Choices: []providers.Choice{{Message: providers.Message{
				Role: "assistant",
				ToolCalls: []map[string]interface{}{
					{"id": "c1", "type": "function", "function": map[string]interface{}{
						"name": "file_read", "arguments": `{"path":"a.txt"}`,
					}},
					{"id": "c2", "type": "function", "function": map[string]interface{}{
						"name": "file_read", "arguments": `{"path":"b.txt"}`,
					}},
				},
			}}}},
			// Final response
			{Choices: []providers.Choice{{Message: providers.Message{
				Role:    "assistant",
				Content: "Both files read successfully.",
			}}}},
		},
	}

	registry := tools.DefaultRegistry()
	config := DefaultConfig()
	config.WorkDir = dir
	agent := New(exec, registry, NewRenderer(&bytes.Buffer{}), config)

	result, err := agent.Run(context.Background(), "read both files")
	if err != nil {
		t.Fatal(err)
	}

	if result != "Both files read successfully." {
		t.Errorf("unexpected result: %q", result)
	}

	// Verify both tool results were added to conversation
	msgs := agent.conversation.Messages()
	toolMsgCount := 0
	for _, m := range msgs {
		if m.Role == "tool" {
			toolMsgCount++
		}
	}
	if toolMsgCount != 2 {
		t.Errorf("expected 2 tool messages, got %d", toolMsgCount)
	}
}

func TestScenarioMaxTurnsExceeded(t *testing.T) {
	// Scenario: Agent keeps calling tools and hits the max turns limit.
	dir := t.TempDir()

	// Create executor that always returns tool calls
	exec := &sequenceExecutor{}
	for i := 0; i < 30; i++ {
		exec.responses = append(exec.responses, providers.ChatResponse{
			Choices: []providers.Choice{{Message: providers.Message{
				Role: "assistant",
				ToolCalls: []map[string]interface{}{
					{"id": fmt.Sprintf("c%d", i), "type": "function", "function": map[string]interface{}{
						"name": "bash", "arguments": `{"command":"echo loop"}`,
					}},
				},
			}}},
		})
	}

	registry := tools.DefaultRegistry()
	config := DefaultConfig()
	config.WorkDir = dir
	config.MaxTurns = 5 // Low limit for test

	agent := New(exec, registry, NewRenderer(&bytes.Buffer{}), config)
	_, err := agent.Run(context.Background(), "keep going")

	if err == nil {
		t.Fatal("expected error for max turns exceeded")
	}
	if !strings.Contains(err.Error(), "max turns") {
		t.Errorf("expected max turns error, got: %v", err)
	}
	if exec.idx != 5 {
		t.Errorf("expected exactly 5 LLM calls, got %d", exec.idx)
	}
}

func TestScenarioToolCallError(t *testing.T) {
	// Scenario: Agent calls a tool that fails — error should be passed back as tool result.
	dir := t.TempDir()

	exec := &sequenceExecutor{
		responses: []providers.ChatResponse{
			// Agent tries to read a nonexistent file
			{Choices: []providers.Choice{{Message: providers.Message{
				Role: "assistant",
				ToolCalls: []map[string]interface{}{
					{"id": "c1", "type": "function", "function": map[string]interface{}{
						"name": "file_read", "arguments": `{"path":"nonexistent.txt"}`,
					}},
				},
			}}}},
			// Agent acknowledges the error
			{Choices: []providers.Choice{{Message: providers.Message{
				Role:    "assistant",
				Content: "The file doesn't exist.",
			}}}},
		},
	}

	registry := tools.DefaultRegistry()
	config := DefaultConfig()
	config.WorkDir = dir

	agent := New(exec, registry, NewRenderer(&bytes.Buffer{}), config)
	result, err := agent.Run(context.Background(), "read nonexistent.txt")
	if err != nil {
		t.Fatal(err)
	}

	if result != "The file doesn't exist." {
		t.Errorf("unexpected result: %q", result)
	}

	// Verify the error was passed back as a tool message
	msgs := agent.conversation.Messages()
	var toolMsg providers.Message
	for _, m := range msgs {
		if m.Role == "tool" {
			toolMsg = m
		}
	}
	if toolMsg.Content == "" {
		t.Error("expected tool error message in conversation")
	}
}

func TestScenarioConversationContext(t *testing.T) {
	// Scenario: Multi-turn conversation preserves context.
	dir := t.TempDir()

	callCount := 0
	exec := &sequenceExecutor{}
	// First exchange
	exec.responses = append(exec.responses, providers.ChatResponse{
		Choices: []providers.Choice{{Message: providers.Message{
			Role: "assistant", Content: "I'll remember that your name is Alice.",
		}}},
	})
	// Second exchange — verify message history includes prior messages
	exec.responses = append(exec.responses, providers.ChatResponse{
		Choices: []providers.Choice{{Message: providers.Message{
			Role: "assistant", Content: "Your name is Alice!",
		}}},
	})
	// Third response: stall detection fires after 2 consecutive no-tool turns,
	// escalates and sends forceToolsMessage, requiring a third LLM response.
	exec.responses = append(exec.responses, providers.ChatResponse{
		Choices: []providers.Choice{{Message: providers.Message{
			Role: "assistant", Content: "(fallback)",
		}}},
	})

	registry := tools.NewRegistry()
	config := DefaultConfig()
	config.WorkDir = dir

	agent := New(exec, registry, NewRenderer(&bytes.Buffer{}), config)

	// First message
	_, err := agent.Run(context.Background(), "My name is Alice")
	if err != nil {
		t.Fatal(err)
	}

	// Reset noToolTurns so second Run() starts fresh (simulates interactive session)
	agent.noToolTurns = 0

	// Second message — should include full history
	result, err := agent.Run(context.Background(), "What's my name?")
	if err != nil {
		t.Fatal(err)
	}
	_ = result
	_ = callCount

	// Verify the second LLM call included all prior messages
	if len(exec.calls) < 2 {
		t.Fatalf("expected at least 2 LLM calls, got %d", len(exec.calls))
	}
	secondCall := exec.calls[1]
	// Should have: system + user1 + assistant1 + user2
	if len(secondCall.Messages) < 4 {
		t.Errorf("second call should include full history, got %d messages", len(secondCall.Messages))
	}
}

func TestScenarioSystemPromptInjected(t *testing.T) {
	exec := &sequenceExecutor{
		responses: []providers.ChatResponse{{
			Choices: []providers.Choice{{Message: providers.Message{
				Role: "assistant", Content: "ok",
			}}},
		}},
	}

	config := DefaultConfig()
	config.WorkDir = t.TempDir()
	config.SystemPrompt = "You are a Go expert."

	agent := New(exec, tools.NewRegistry(), NewRenderer(&bytes.Buffer{}), config)
	agent.Run(context.Background(), "hi")

	if len(exec.calls) == 0 {
		t.Fatal("expected at least one call")
	}

	firstMsg := exec.calls[0].Messages[0]
	if firstMsg.Role != "system" {
		t.Errorf("first message should be system, got %s", firstMsg.Role)
	}
	if firstMsg.Content != "You are a Go expert." {
		t.Errorf("expected custom system prompt, got %q", firstMsg.Content)
	}
}

func TestScenarioToolDefsPassedToLLM(t *testing.T) {
	exec := &sequenceExecutor{
		responses: []providers.ChatResponse{{
			Choices: []providers.Choice{{Message: providers.Message{
				Role: "assistant", Content: "ok",
			}}},
		}},
	}

	registry := tools.DefaultRegistry()
	config := DefaultConfig()
	config.WorkDir = t.TempDir()

	agent := New(exec, registry, NewRenderer(&bytes.Buffer{}), config)
	agent.Run(context.Background(), "write a function that adds two numbers")

	if len(exec.calls) == 0 {
		t.Fatal("expected at least one call")
	}

	toolDefs := exec.calls[0].Tools
	if len(toolDefs) != 10 {
		t.Errorf("expected 10 tool definitions, got %d", len(toolDefs))
	}
}
