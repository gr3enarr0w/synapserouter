package agent

import (
	"io"
	"strings"
	"testing"

	"github.com/gr3enarr0w/synapserouter/internal/tools"
)

func TestFormatPermissionSummary_FileWritePreview(t *testing.T) {
	summary := formatPermissionSummary("file_write", map[string]interface{}{
		"path":    "notes.txt",
		"content": "line one\nline two\nline three",
	})

	if !strings.Contains(summary, "create/overwrite notes.txt") {
		t.Fatalf("expected path in summary, got %q", summary)
	}
	if !strings.Contains(summary, "line one") || !strings.Contains(summary, "line two") {
		t.Fatalf("expected preview in summary, got %q", summary)
	}
}

func TestFormatPermissionSummary_FileEditPreview(t *testing.T) {
	summary := formatPermissionSummary("file_edit", map[string]interface{}{
		"path":       "main.go",
		"old_string": "old line",
		"new_string": "new line",
	})

	if !strings.Contains(summary, "edit main.go") {
		t.Fatalf("expected path in summary, got %q", summary)
	}
	if !strings.Contains(summary, "old line") || !strings.Contains(summary, "new line") {
		t.Fatalf("expected replace preview in summary, got %q", summary)
	}
}

func TestFormatPermissionSummary_BashNotTruncated(t *testing.T) {
	cmd := strings.Repeat("x", 120)
	summary := formatPermissionSummary("bash", map[string]interface{}{"command": cmd})
	if summary != cmd {
		t.Fatalf("expected full command, got %q", summary)
	}
}

func TestSetPermissionPromptEmitsEventAndDelegates(t *testing.T) {
	b := NewEventBus()
	events := b.Subscribe()
	a := New(nil, tools.NewRegistry(), nil, Config{EventBus: b})
	called := false
	a.SetPermissionPrompt(func(toolName string, category tools.ToolCategory, args map[string]interface{}) bool {
		called = true
		return toolName == "bash" && category == tools.CategoryDangerous && args["command"] == "rm -rf /tmp/demo"
	})

	if a.permissionPrompt == nil {
		t.Fatal("expected wrapped permission prompt")
	}
	approved := a.permissionPrompt("bash", tools.CategoryDangerous, map[string]interface{}{"command": "rm -rf /tmp/demo"})
	if !approved {
		t.Fatal("expected wrapped permission prompt result")
	}
	if !called {
		t.Fatal("expected original permission prompt to be called")
	}

	e := <-events
	if e.Type != EventPermissionRequest {
		t.Fatalf("expected permission request event, got %v", e.Type)
	}
	if e.Data["tool_name"] != "bash" {
		t.Fatalf("expected tool_name bash, got %#v", e.Data["tool_name"])
	}
	if e.Data["category"] != string(tools.CategoryDangerous) {
		t.Fatalf("expected dangerous category, got %#v", e.Data["category"])
	}
}

func TestParsePermissionString(t *testing.T) {
	approveAll := false
	approved, ok := parsePermissionString("all", &approveAll)
	if !ok || !approved || !approveAll {
		t.Fatalf("expected all to approve and set approveAll, got approved=%v ok=%v approveAll=%v", approved, ok, approveAll)
	}

	approved, ok = parsePermissionString("no", &approveAll)
	if !ok || approved {
		t.Fatalf("expected no to deny, got approved=%v ok=%v", approved, ok)
	}
}

func TestCodeModePermissionPromptUsesStdin(t *testing.T) {
	renderer := NewCodeRenderer(io.Discard, 80, 24, "repo", "model", "go")
	prompt := CodeModePermissionPrompt(renderer, strings.NewReader("y\n"))
	if !prompt("bash", tools.CategoryDangerous, map[string]interface{}{"command": "rm -rf tmp"}) {
		t.Fatal("expected approval from stdin-backed code mode prompt")
	}
}
