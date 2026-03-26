package tools

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// GrepTool performs recursive content search using grep.
type GrepTool struct{}

func (t *GrepTool) Name() string        { return "grep" }
func (t *GrepTool) Description() string { return "Search file contents recursively using grep" }
func (t *GrepTool) Category() ToolCategory { return CategoryReadOnly }

func (t *GrepTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"pattern": map[string]interface{}{
				"type":        "string",
				"description": "Search pattern (regex supported)",
			},
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Directory or file to search (default: work directory)",
			},
			"include": map[string]interface{}{
				"type":        "string",
				"description": "File glob pattern to include (e.g. '*.go')",
			},
			"ignore_case": map[string]interface{}{
				"type":        "boolean",
				"description": "Case-insensitive search",
			},
			"max_results": map[string]interface{}{
				"type":        "integer",
				"description": "Maximum number of matching lines (default 100)",
			},
		},
		"required": []string{"pattern"},
	}
}

func (t *GrepTool) Execute(ctx context.Context, args map[string]interface{}, workDir string) (*ToolResult, error) {
	pattern := stringArg(args, "pattern")
	if pattern == "" {
		return &ToolResult{Error: "pattern is required"}, nil
	}

	searchPath := workDir
	if p := stringArg(args, "path"); p != "" {
		searchPath = resolveToolPath(p, workDir)
	}

	maxResults := intArg(args, "max_results", 100)

	grepArgs := []string{"-rn", "--color=never"}
	if boolArg(args, "ignore_case") {
		grepArgs = append(grepArgs, "-i")
	}
	if include := stringArg(args, "include"); include != "" {
		grepArgs = append(grepArgs, "--include="+include)
	}
	grepArgs = append(grepArgs, "-m", fmt.Sprintf("%d", maxResults))
	grepArgs = append(grepArgs, "-e", pattern, searchPath)

	cmd := exec.CommandContext(ctx, "grep", grepArgs...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &limitWriter{w: &stdout, max: maxOutputBytes}
	cmd.Stderr = &stderr

	err := cmd.Run()
	output := strings.TrimRight(stdout.String(), "\n")

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 1 {
				return &ToolResult{Output: "no matches found"}, nil
			}
		}
		if errStr := stderr.String(); errStr != "" {
			return &ToolResult{Error: errStr, ExitCode: 2}, nil
		}
	}

	return &ToolResult{Output: output}, nil
}
