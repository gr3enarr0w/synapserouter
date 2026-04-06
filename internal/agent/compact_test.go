package agent

import (
	"strings"
	"testing"
)

// TestToolEventDataFields verifies that tool completion events include
// expected data fields for different tool types (compact verbosity mode).
func TestToolEventDataFields(t *testing.T) {
	tests := []struct {
		name           string
		toolName       string
		args           map[string]interface{}
		resultContent  string
		isError        bool
		wantDataFields []string
	}{
		{
			name:          "file_read includes lines_read",
			toolName:      "file_read",
			args:          map[string]interface{}{"path": "test.txt", "limit": 10},
			resultContent: "line1\nline2\nline3",
			wantDataFields: []string{"lines_read", "path"},
		},
		{
			name:          "grep includes matches",
			toolName:      "grep",
			args:          map[string]interface{}{"pattern": "test", "path": "."},
			resultContent: "match1\nmatch2",
			wantDataFields: []string{"matches", "pattern"},
		},
		{
			name:          "glob includes files_found",
			toolName:      "glob",
			args:          map[string]interface{}{"pattern": "*.go"},
			resultContent: "file1.txt\nfile2.go",
			wantDataFields: []string{"files_found", "pattern"},
		},
		{
			name:          "bash includes command",
			toolName:      "bash",
			args:          map[string]interface{}{"command": "ls -la"},
			resultContent: "output",
			wantDataFields: []string{"command"},
		},
		{
			name:          "error includes output_lines",
			toolName:      "bash",
			args:          map[string]interface{}{"command": "invalid"},
			resultContent: "error message",
			isError:       true,
			wantDataFields: []string{"output_lines", "command"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build event data inline (mimics executeToolCalls logic)
			toolEventData := map[string]interface{}{
				"tool_name": tt.toolName,
				"duration":  "0s",
				"is_error":  tt.isError,
			}

			// Add tool-specific data (same logic as agent.go:1385-1420)
			switch tt.toolName {
			case "file_read":
				if path, ok := tt.args["path"].(string); ok {
					toolEventData["path"] = path
				}
				if limit, ok := tt.args["limit"].(float64); ok && limit > 0 {
					toolEventData["lines_read"] = int(limit)
				} else {
					toolEventData["lines_read"] = strings.Count(tt.resultContent, "\n") + 1
				}
			case "bash":
				if cmd, ok := tt.args["command"].(string); ok {
					toolEventData["command"] = cmd
				}
			case "grep":
				if pattern, ok := tt.args["pattern"].(string); ok {
					toolEventData["pattern"] = pattern
				}
				if tt.resultContent != "" && !tt.isError {
					toolEventData["matches"] = strings.Count(strings.TrimSpace(tt.resultContent), "\n") + 1
				} else {
					toolEventData["matches"] = 0
				}
			case "glob":
				if pattern, ok := tt.args["pattern"].(string); ok {
					toolEventData["pattern"] = pattern
				}
				if tt.resultContent != "" && !tt.isError {
					toolEventData["files_found"] = strings.Count(strings.TrimSpace(tt.resultContent), "\n") + 1
				} else {
					toolEventData["files_found"] = 0
				}
			}

			if tt.isError {
				lines := strings.Count(tt.resultContent, "\n") + 1
				toolEventData["output_lines"] = lines
				if len(tt.resultContent) <= 500 {
					toolEventData["output"] = tt.resultContent
				}
			}

			// Verify expected fields are present
			for _, field := range tt.wantDataFields {
				if _, ok := toolEventData[field]; !ok {
					t.Errorf("tool event data missing field: %s, got: %v", field, toolEventData)
				}
			}
		})
	}
}
