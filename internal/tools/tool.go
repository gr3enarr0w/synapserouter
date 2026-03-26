package tools

import "context"

// ToolCategory classifies tools by their risk level for permission checks.
type ToolCategory string

const (
	CategoryReadOnly  ToolCategory = "read_only"  // always allowed
	CategoryWrite     ToolCategory = "write"       // needs approval in interactive mode
	CategoryDangerous ToolCategory = "dangerous"   // extra scrutiny (rm -rf, force push)
)

// Tool is the interface that all agent tools must implement.
type Tool interface {
	Name() string
	Description() string
	Category() ToolCategory
	InputSchema() map[string]interface{}
	Execute(ctx context.Context, args map[string]interface{}, workDir string) (*ToolResult, error)
}

// ToolResult is the outcome of executing a tool.
type ToolResult struct {
	Output   string `json:"output"`
	Error    string `json:"error,omitempty"`
	ExitCode int    `json:"exit_code,omitempty"`
}
