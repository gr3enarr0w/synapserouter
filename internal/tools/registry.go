package tools

import (
	"context"
	"fmt"
	"sync"
)

// Registry manages a collection of tools and provides lookup/execution.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// NewRegistry creates an empty tool registry.
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

// DefaultRegistry creates a registry pre-populated with all built-in tools.
func DefaultRegistry() *Registry {
	r := NewRegistry()
	r.Register(&BashTool{})
	r.Register(&FileReadTool{})
	r.Register(&FileWriteTool{})
	r.Register(&FileEditTool{})
	r.Register(&GrepTool{})
	r.Register(&GlobTool{})
	r.Register(&GitTool{})
	r.Register(&WebSearchTool{})
	r.Register(&WebFetchTool{})
	return r
}

// Register adds a tool to the registry. Overwrites if name already exists.
func (r *Registry) Register(tool Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[tool.Name()] = tool
}

// Get returns a tool by name.
func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

// List returns all registered tool names.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	return names
}

// Execute runs a tool by name with the given arguments.
func (r *Registry) Execute(ctx context.Context, name string, args map[string]interface{}, workDir string) (*ToolResult, error) {
	tool, ok := r.Get(name)
	if !ok {
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
	return tool.Execute(ctx, args, workDir)
}

// ExecuteChecked runs a tool after verifying it passes permission checks.
func (r *Registry) ExecuteChecked(ctx context.Context, name string, args map[string]interface{}, workDir string, pc *PermissionChecker) (*ToolResult, error) {
	tool, ok := r.Get(name)
	if !ok {
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
	if pc != nil {
		perm := pc.Check(tool, args)
		if !perm.Allowed {
			return &ToolResult{Error: fmt.Sprintf("permission denied: %s", perm.Reason)}, nil
		}
	}
	return tool.Execute(ctx, args, workDir)
}

// OpenAIToolDefinitions returns tool schemas in OpenAI function-calling format
// suitable for inclusion in ChatRequest.Tools.
func (r *Registry) OpenAIToolDefinitions() []map[string]interface{} {
	r.mu.RLock()
	tools := r.tools
	r.mu.RUnlock()
	defs := make([]map[string]interface{}, 0, len(tools))
	for _, tool := range tools {
		defs = append(defs, map[string]interface{}{
			"type": "function",
			"function": map[string]interface{}{
				"name":        tool.Name(),
				"description": tool.Description(),
				"parameters":  tool.InputSchema(),
			},
		})
	}
	return defs
}

// ToolInfo returns metadata about all registered tools.
func (r *Registry) ToolInfo() []map[string]interface{} {
	r.mu.RLock()
	tools := r.tools
	r.mu.RUnlock()
	info := make([]map[string]interface{}, 0, len(tools))
	for _, tool := range tools {
		info = append(info, map[string]interface{}{
			"name":        tool.Name(),
			"description": tool.Description(),
			"category":    string(tool.Category()),
		})
	}
	return info
}
