package agent

import (
	"context"
	"testing"
	"time"

	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/tools"
)

func TestPredictNextTools_GrepToFileRead(t *testing.T) {
	history := []toolCallRecord{
		{
			Name:   "grep",
			Output: "internal/agent/agent.go:42: func main() {\ninternal/tools/registry.go:10: type Registry struct {\n",
		},
	}

	predictions := PredictNextTools(history)
	if len(predictions) == 0 {
		t.Fatal("expected predictions after grep")
	}

	// Should predict file_read on matched files
	if predictions[0].Name != "file_read" {
		t.Errorf("expected file_read, got %s", predictions[0].Name)
	}
	path, _ := predictions[0].Args["file_path"].(string)
	if path != "internal/agent/agent.go" {
		t.Errorf("expected agent.go path, got %q", path)
	}
}

func TestPredictNextTools_GlobToFileRead(t *testing.T) {
	history := []toolCallRecord{
		{
			Name:   "glob",
			Output: "main.go\ninternal/agent/agent.go\ninternal/tools/tool.go\nREADME.md\n",
		},
	}

	predictions := PredictNextTools(history)
	if len(predictions) != 3 {
		t.Fatalf("expected 3 predictions (capped), got %d", len(predictions))
	}

	for _, p := range predictions {
		if p.Name != "file_read" {
			t.Errorf("expected file_read, got %s", p.Name)
		}
	}
}

func TestPredictNextTools_SearchToFetch(t *testing.T) {
	history := []toolCallRecord{
		{
			Name:   "web_search",
			Output: "[1] Go Error Handling\n    https://go.dev/doc/effective_go#errors\n    Learn about error handling\n\n[2] Error Types\n    https://pkg.go.dev/errors\n    Package errors",
		},
	}

	predictions := PredictNextTools(history)
	if len(predictions) != 2 {
		t.Fatalf("expected 2 predictions (web_fetch), got %d", len(predictions))
	}

	if predictions[0].Name != "web_fetch" {
		t.Errorf("expected web_fetch, got %s", predictions[0].Name)
	}
	url, _ := predictions[0].Args["url"].(string)
	if url != "https://go.dev/doc/effective_go#errors" {
		t.Errorf("expected go.dev URL, got %q", url)
	}
}

func TestPredictNextTools_Empty(t *testing.T) {
	predictions := PredictNextTools(nil)
	if len(predictions) != 0 {
		t.Error("empty history should produce no predictions")
	}

	predictions = PredictNextTools([]toolCallRecord{
		{Name: "file_write", Output: "wrote 100 bytes"}, // not a pattern trigger
	})
	if len(predictions) != 0 {
		t.Error("file_write should not trigger predictions")
	}
}

func TestSpeculativeCache_HitAndMiss(t *testing.T) {
	sc := NewSpeculativeCache()

	// Create a mock registry with a read-only tool
	registry := tools.NewRegistry()
	registry.Register(&mockReadOnlyTool{name: "file_read"})

	predictions := []ToolPrediction{
		{Name: "file_read", Args: map[string]interface{}{"file_path": "test.go"}},
	}

	sc.Speculate(context.Background(), registry, ".", predictions)

	// Wait a bit for goroutine to complete
	time.Sleep(50 * time.Millisecond)

	// Hit: same tool + args
	result, ok := sc.Get("file_read", map[string]interface{}{"file_path": "test.go"})
	if !ok {
		t.Error("expected cache hit")
	}
	if result == nil {
		t.Error("expected non-nil result on hit")
	}

	// Miss: different args
	_, ok = sc.Get("file_read", map[string]interface{}{"file_path": "other.go"})
	if ok {
		t.Error("expected cache miss for different args")
	}

	// Miss: different tool
	_, ok = sc.Get("grep", map[string]interface{}{"pattern": "test"})
	if ok {
		t.Error("expected cache miss for different tool")
	}
}

func TestSpeculativeCache_SafetyFilter(t *testing.T) {
	sc := NewSpeculativeCache()

	registry := tools.NewRegistry()
	registry.Register(&mockWriteTool{name: "file_write"})

	// Predict a write tool — should be rejected
	predictions := []ToolPrediction{
		{Name: "file_write", Args: map[string]interface{}{"path": "evil.go", "content": "rm -rf /"}},
	}

	sc.Speculate(context.Background(), registry, ".", predictions)

	time.Sleep(50 * time.Millisecond)

	// Should not have been speculated
	_, ok := sc.Get("file_write", map[string]interface{}{"path": "evil.go", "content": "rm -rf /"})
	if ok {
		t.Error("write tool should NOT be speculatively executed")
	}
}

func TestSpeculativeCache_Clear(t *testing.T) {
	sc := NewSpeculativeCache()

	registry := tools.NewRegistry()
	registry.Register(&mockReadOnlyTool{name: "file_read"})

	sc.Speculate(context.Background(), registry, ".", []ToolPrediction{
		{Name: "file_read", Args: map[string]interface{}{"file_path": "test.go"}},
	})

	time.Sleep(50 * time.Millisecond)

	sc.Clear()

	_, ok := sc.Get("file_read", map[string]interface{}{"file_path": "test.go"})
	if ok {
		t.Error("clear should discard all cached results")
	}
}

func TestSpeculativeCache_Nil(t *testing.T) {
	var sc *SpeculativeCache
	sc.Speculate(context.Background(), nil, ".", nil)    // should not panic
	sc.Clear()                                            // should not panic
	_, ok := sc.Get("test", nil)                         // should not panic
	if ok {
		t.Error("nil cache should always miss")
	}
}

func TestIsSpeculationEnabled(t *testing.T) {
	t.Setenv("SYNROUTE_SPECULATE", "")
	if !isSpeculationEnabled() {
		t.Error("default should be enabled")
	}

	t.Setenv("SYNROUTE_SPECULATE", "false")
	if isSpeculationEnabled() {
		t.Error("false should disable")
	}

	t.Setenv("SYNROUTE_SPECULATE", "true")
	if !isSpeculationEnabled() {
		t.Error("true should enable")
	}
}

// Mock tools for testing

type mockReadOnlyTool struct{ name string }

func (m *mockReadOnlyTool) Name() string                       { return m.name }
func (m *mockReadOnlyTool) Description() string                { return "mock" }
func (m *mockReadOnlyTool) Category() tools.ToolCategory       { return tools.CategoryReadOnly }
func (m *mockReadOnlyTool) InputSchema() map[string]interface{} { return nil }
func (m *mockReadOnlyTool) Execute(ctx context.Context, args map[string]interface{}, workDir string) (*tools.ToolResult, error) {
	return &tools.ToolResult{Output: "mock output"}, nil
}

type mockWriteTool struct{ name string }

func (m *mockWriteTool) Name() string                       { return m.name }
func (m *mockWriteTool) Description() string                { return "mock write" }
func (m *mockWriteTool) Category() tools.ToolCategory       { return tools.CategoryWrite }
func (m *mockWriteTool) InputSchema() map[string]interface{} { return nil }
func (m *mockWriteTool) Execute(ctx context.Context, args map[string]interface{}, workDir string) (*tools.ToolResult, error) {
	return &tools.ToolResult{Output: "wrote"}, nil
}
