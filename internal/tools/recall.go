package tools

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ToolOutputSearcher is the interface the recall tool uses to query past tool outputs.
// Implemented by agent.ToolOutputStore.
type ToolOutputSearcher interface {
	Search(sessionID, toolName string, limit int) ([]ToolOutputResult, error)
	Retrieve(sessionID string, id int64) (string, error)
}

// ToolOutputResult represents a stored tool output entry.
type ToolOutputResult struct {
	ID          int64
	ToolName    string
	ArgsSummary string
	Summary     string
	ExitCode    int
	OutputSize  int
	CreatedAt   time.Time
}

// RecallTool allows the agent to search and retrieve past tool outputs from the DB.
type RecallTool struct {
	searcher  ToolOutputSearcher
	sessionID string
}

// NewRecallTool creates a recall tool bound to a session.
func NewRecallTool(searcher ToolOutputSearcher, sessionID string) *RecallTool {
	return &RecallTool{searcher: searcher, sessionID: sessionID}
}

func (t *RecallTool) Name() string        { return "recall" }
func (t *RecallTool) Description() string  { return "Search and retrieve past tool outputs from this session" }
func (t *RecallTool) Category() ToolCategory { return CategoryReadOnly }

func (t *RecallTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "Search term to find relevant past tool outputs",
			},
			"tool_name": map[string]interface{}{
				"type":        "string",
				"description": "Filter by tool name (bash, grep, file_read, etc.)",
			},
			"id": map[string]interface{}{
				"type":        "number",
				"description": "Retrieve full output for a specific tool output ID (from a previous recall search)",
			},
			"limit": map[string]interface{}{
				"type":        "number",
				"description": "Maximum number of results to return (default 5)",
			},
		},
	}
}

func (t *RecallTool) Execute(ctx context.Context, args map[string]interface{}, workDir string) (*ToolResult, error) {
	if t.searcher == nil {
		return &ToolResult{Output: "recall not available (no tool output store configured)"}, nil
	}

	// Mode 1: Retrieve full output by ID
	if idVal, ok := args["id"]; ok {
		id := int64(0)
		switch v := idVal.(type) {
		case float64:
			id = int64(v)
		case int64:
			id = v
		}
		if id > 0 {
			output, err := t.searcher.Retrieve(t.sessionID, id)
			if err != nil {
				return &ToolResult{Output: fmt.Sprintf("not found: %v", err), ExitCode: 1}, nil
			}
			// Truncate very large outputs for conversation
			if len(output) > 16*1024 {
				output = output[:16*1024] + "\n...(truncated, full output is " + fmt.Sprintf("%d", len(output)) + " bytes)"
			}
			return &ToolResult{Output: output}, nil
		}
	}

	// Mode 2: Search by tool name and/or query
	toolName, _ := args["tool_name"].(string)
	limit := 5
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}

	results, err := t.searcher.Search(t.sessionID, toolName, limit)
	if err != nil {
		return &ToolResult{Output: fmt.Sprintf("search error: %v", err), ExitCode: 1}, nil
	}

	if len(results) == 0 {
		return &ToolResult{Output: "no past tool outputs found for this session"}, nil
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Found %d past tool outputs:\n\n", len(results)))
	for _, r := range results {
		b.WriteString(fmt.Sprintf("  [ref:%d] %s (%s) — %d bytes, exit %d\n",
			r.ID, r.ToolName, r.ArgsSummary, r.OutputSize, r.ExitCode))
		b.WriteString(fmt.Sprintf("    %s\n\n", r.Summary))
	}
	b.WriteString("Use recall(id=N) to retrieve the full output of any result.")
	return &ToolResult{Output: b.String()}, nil
}
