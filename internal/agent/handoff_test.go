package agent

import (
	"bytes"
	"context"
	"testing"

	"github.com/gr3enarr0w/synapserouter/internal/providers"
	"github.com/gr3enarr0w/synapserouter/internal/tools"
)

func TestExecuteHandoff(t *testing.T) {
	exec := &mockExecutor{
		responses: []providers.ChatResponse{{
			Choices: []providers.Choice{{
				Message: providers.Message{Role: "assistant", Content: "security audit complete: no issues found"},
			}},
		}},
	}

	parent := New(exec, tools.NewRegistry(), NewRenderer(&bytes.Buffer{}), DefaultConfig())
	// Add some conversation context
	parent.conversation.Add(providers.Message{Role: "user", Content: "review the auth module"})
	parent.conversation.Add(providers.Message{Role: "assistant", Content: "I'll hand this off to a security specialist."})

	h := Handoff{
		TargetRole: "security-audit",
		Context:    "Review the authentication module for vulnerabilities",
		Files:      []string{"auth.go", "middleware.go"},
	}

	result, err := parent.ExecuteHandoff(context.Background(), h)
	if err != nil {
		t.Fatal(err)
	}

	if result.ToRole != "security-audit" {
		t.Errorf("to_role = %q, want security-audit", result.ToRole)
	}
	if result.Result != "security audit complete: no issues found" {
		t.Errorf("result = %q, unexpected", result.Result)
	}
	if result.FromAgent != parent.SessionID() {
		t.Errorf("from_agent = %q, want %q", result.FromAgent, parent.SessionID())
	}
}

func TestHandoffContextBuilding(t *testing.T) {
	messages := []providers.Message{
		{Role: "user", Content: "fix the auth bug"},
		{Role: "assistant", Content: "I see the issue in the token validation."},
		{Role: "user", Content: "also check for XSS"},
	}

	h := Handoff{
		TargetRole: "reviewer",
		Context:    "Security review needed",
		Files:      []string{"handler.go"},
	}

	ctx := buildHandoffContext(h, messages)

	if ctx == "" {
		t.Fatal("context should not be empty")
	}
	if !bytes.Contains([]byte(ctx), []byte("Security review needed")) {
		t.Error("context should contain handoff context")
	}
	if !bytes.Contains([]byte(ctx), []byte("handler.go")) {
		t.Error("context should contain file references")
	}
}

func TestRecentUserMessages(t *testing.T) {
	messages := []providers.Message{
		{Role: "user", Content: "msg1"},
		{Role: "assistant", Content: "reply1"},
		{Role: "user", Content: "msg2"},
		{Role: "assistant", Content: "", ToolCalls: []map[string]interface{}{{"id": "c1"}}},
		{Role: "tool", Content: "tool output"},
		{Role: "user", Content: "msg3"},
		{Role: "assistant", Content: "reply3"},
	}

	recent := recentUserMessages(messages, 2)
	if len(recent) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(recent))
	}
	// Should be in chronological order
	if recent[0].Content != "msg3" {
		t.Errorf("recent[0] = %q, want msg3", recent[0].Content)
	}
	if recent[1].Content != "reply3" {
		t.Errorf("recent[1] = %q, want reply3", recent[1].Content)
	}
}

func TestDelegateToolExecute(t *testing.T) {
	exec := &mockExecutor{
		responses: []providers.ChatResponse{{
			Choices: []providers.Choice{{
				Message: providers.Message{Role: "assistant", Content: "delegated result"},
			}},
		}},
	}

	parent := New(exec, tools.NewRegistry(), NewRenderer(&bytes.Buffer{}), DefaultConfig())
	delegateTool := NewDelegateTool(parent)

	result, err := delegateTool.Execute(context.Background(), map[string]interface{}{
		"role": "tester",
		"task": "run unit tests",
	}, t.TempDir())

	if err != nil {
		t.Fatal(err)
	}
	if result.Output != "delegated result" {
		t.Errorf("output = %q, want 'delegated result'", result.Output)
	}
}

func TestDelegateToolMissingArgs(t *testing.T) {
	parent := New(&mockExecutor{}, tools.NewRegistry(), NewRenderer(&bytes.Buffer{}), DefaultConfig())
	delegateTool := NewDelegateTool(parent)

	result, _ := delegateTool.Execute(context.Background(), map[string]interface{}{
		"role": "tester",
		// missing "task"
	}, t.TempDir())

	if result.Error == "" {
		t.Error("expected error for missing task")
	}
}

func TestHandoffToolExecute(t *testing.T) {
	exec := &mockExecutor{
		responses: []providers.ChatResponse{{
			Choices: []providers.Choice{{
				Message: providers.Message{Role: "assistant", Content: "handoff complete"},
			}},
		}},
	}

	parent := New(exec, tools.NewRegistry(), NewRenderer(&bytes.Buffer{}), DefaultConfig())
	handoffTool := NewHandoffTool(parent)

	result, err := handoffTool.Execute(context.Background(), map[string]interface{}{
		"role":    "security-audit",
		"context": "review auth code",
		"files":   []interface{}{"auth.go"},
	}, t.TempDir())

	if err != nil {
		t.Fatal(err)
	}
	if result.Output != "handoff complete" {
		t.Errorf("output = %q, want 'handoff complete'", result.Output)
	}
}

func TestTruncate(t *testing.T) {
	if truncate("hello", 10) != "hello" {
		t.Error("short string should not be truncated")
	}
	if truncate("hello world", 5) != "hello..." {
		t.Errorf("truncated = %q, want 'hello...'", truncate("hello world", 5))
	}
}
