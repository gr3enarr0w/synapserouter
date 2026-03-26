package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestRegistryToolDelegationFlow verifies the flow that orchestration uses:
// registry.Get(name) -> tool found -> registry.Execute() -> ToolResult
func TestRegistryToolDelegationFlow(t *testing.T) {
	dir := t.TempDir()
	registry := DefaultRegistry()

	// Simulate what orchestration manager does in executeBuiltInToolCall default case
	tests := []struct {
		name    string
		tool    string
		args    map[string]interface{}
		setup   func()
		wantErr bool
		check   func(t *testing.T, result *ToolResult)
	}{
		{
			name: "bash via registry delegation",
			tool: "bash",
			args: map[string]interface{}{"command": "echo delegated"},
			check: func(t *testing.T, result *ToolResult) {
				if result.ExitCode != 0 {
					t.Errorf("expected exit 0, got %d", result.ExitCode)
				}
				if result.Output == "" {
					t.Error("expected output from echo")
				}
			},
		},
		{
			name: "file_read via registry delegation",
			tool: "file_read",
			args: map[string]interface{}{"path": "testfile.txt"},
			setup: func() {
				os.WriteFile(filepath.Join(dir, "testfile.txt"), []byte("hello from integration"), 0644)
			},
			check: func(t *testing.T, result *ToolResult) {
				if result.Error != "" {
					t.Errorf("unexpected error: %s", result.Error)
				}
			},
		},
		{
			name: "file_write + file_read roundtrip",
			tool: "file_write",
			args: map[string]interface{}{"path": "roundtrip.txt", "content": "roundtrip content"},
			check: func(t *testing.T, result *ToolResult) {
				if result.Error != "" {
					t.Errorf("write error: %s", result.Error)
				}
				// Now read it back
				readResult, err := registry.Execute(context.Background(), "file_read",
					map[string]interface{}{"path": "roundtrip.txt"}, dir)
				if err != nil {
					t.Fatal(err)
				}
				if readResult.Error != "" {
					t.Errorf("read error: %s", readResult.Error)
				}
			},
		},
		{
			name: "file_edit via registry",
			tool: "file_edit",
			args: map[string]interface{}{
				"path": "editable.txt", "old_string": "before", "new_string": "after",
			},
			setup: func() {
				os.WriteFile(filepath.Join(dir, "editable.txt"), []byte("value=before"), 0644)
			},
			check: func(t *testing.T, result *ToolResult) {
				if result.Error != "" {
					t.Errorf("edit error: %s", result.Error)
				}
				data, _ := os.ReadFile(filepath.Join(dir, "editable.txt"))
				if string(data) != "value=after" {
					t.Errorf("expected 'value=after', got %q", string(data))
				}
			},
		},
		{
			name: "unknown tool returns error",
			tool: "nonexistent_tool",
			args: map[string]interface{}{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				tt.setup()
			}

			// Simulate orchestration delegation: check registry, then execute
			tool, ok := registry.Get(tt.tool)
			if !ok {
				if tt.wantErr {
					return // expected
				}
				t.Fatalf("tool %q not found in registry", tt.tool)
			}
			_ = tool // registry.Get found it

			result, err := registry.Execute(context.Background(), tt.tool, tt.args, dir)
			if err != nil {
				if tt.wantErr {
					return
				}
				t.Fatal(err)
			}

			if tt.check != nil {
				tt.check(t, result)
			}
		})
	}
}

// TestOpenAIToolDefsSchemaValidity verifies schemas are well-formed for LLM consumption.
func TestOpenAIToolDefsSchemaValidity(t *testing.T) {
	registry := DefaultRegistry()
	defs := registry.OpenAIToolDefinitions()

	for _, def := range defs {
		typ, _ := def["type"].(string)
		if typ != "function" {
			t.Errorf("tool def type should be 'function', got %q", typ)
		}

		fn, ok := def["function"].(map[string]interface{})
		if !ok {
			t.Fatal("missing function key in tool def")
		}

		name, _ := fn["name"].(string)
		if name == "" {
			t.Error("tool def missing name")
		}

		desc, _ := fn["description"].(string)
		if desc == "" {
			t.Errorf("tool %q has empty description", name)
		}

		params, ok := fn["parameters"].(map[string]interface{})
		if !ok {
			t.Errorf("tool %q has no parameters schema", name)
			continue
		}

		schemaType, _ := params["type"].(string)
		if schemaType != "object" {
			t.Errorf("tool %q parameters.type should be 'object', got %q", name, schemaType)
		}

		props, ok := params["properties"].(map[string]interface{})
		if !ok || len(props) == 0 {
			t.Errorf("tool %q has no properties", name)
		}

		required, ok := params["required"].([]string)
		if !ok || len(required) == 0 {
			t.Errorf("tool %q has no required fields", name)
		}
	}
}

// TestPermissionCheckerWithRealTools verifies permissions work with actual tool instances.
func TestPermissionCheckerWithRealTools(t *testing.T) {
	registry := DefaultRegistry()

	tests := []struct {
		name     string
		mode     PermissionMode
		tool     string
		args     map[string]interface{}
		wantOK   bool
		wantProm bool
	}{
		{"read always allowed in readonly", ModeReadOnly, "file_read", nil, true, false},
		{"write blocked in readonly", ModeReadOnly, "file_write", nil, false, false},
		{"bash blocked in readonly", ModeReadOnly, "bash", nil, false, false},
		{"read allowed in interactive", ModeInteractive, "grep", nil, true, false},
		{"write prompts in interactive", ModeInteractive, "file_edit", map[string]interface{}{"path": "x.go"}, false, true},
		{"read allowed in auto_approve", ModeAutoApprove, "glob", nil, true, false},
		{"write allowed in auto_approve", ModeAutoApprove, "file_write", nil, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc := NewPermissionChecker(tt.mode)
			tool, ok := registry.Get(tt.tool)
			if !ok {
				t.Fatalf("tool %q not found", tt.tool)
			}

			result := pc.Check(tool, tt.args)
			if result.Allowed != tt.wantOK {
				t.Errorf("allowed=%v, want %v (reason: %s)", result.Allowed, tt.wantOK, result.Reason)
			}
			if result.Prompt != tt.wantProm {
				t.Errorf("prompt=%v, want %v", result.Prompt, tt.wantProm)
			}
		})
	}
}

// TestConcurrentRegistryAccess verifies the registry is safe for concurrent use.
func TestConcurrentRegistryAccess(t *testing.T) {
	registry := DefaultRegistry()
	dir := t.TempDir()
	ctx := context.Background()

	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for j := 0; j < 50; j++ {
				registry.List()
				registry.Get("bash")
				registry.OpenAIToolDefinitions()
				registry.ToolInfo()
				registry.Execute(ctx, "bash", map[string]interface{}{"command": "true"}, dir)
			}
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}
