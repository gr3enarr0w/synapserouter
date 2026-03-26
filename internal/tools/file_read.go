package tools

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
)

// FileReadTool reads file contents with optional line offset/limit.
type FileReadTool struct{}

func (t *FileReadTool) Name() string        { return "file_read" }
func (t *FileReadTool) Description() string { return "Read a file's contents with optional line range" }
func (t *FileReadTool) Category() ToolCategory { return CategoryReadOnly }

func (t *FileReadTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "File path (absolute or relative to work directory)",
			},
			"offset": map[string]interface{}{
				"type":        "integer",
				"description": "Starting line number (1-based, default 1)",
			},
			"limit": map[string]interface{}{
				"type":        "integer",
				"description": "Maximum number of lines to read (default: all)",
			},
		},
		"required": []string{"path"},
	}
}

func (t *FileReadTool) Execute(ctx context.Context, args map[string]interface{}, workDir string) (*ToolResult, error) {
	const defaultLineLimit = 2000

	path := resolveToolPath(stringArg(args, "path"), workDir)
	offset := intArg(args, "offset", 1)
	limit := intArg(args, "limit", 0)
	userSetLimit := limit > 0

	if offset < 1 {
		offset = 1
	}
	if limit <= 0 {
		limit = defaultLineLimit
	}

	f, err := os.Open(path)
	if err != nil {
		return &ToolResult{Error: err.Error()}, nil
	}
	defer f.Close()

	var b strings.Builder
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNum := 0
	linesRead := 0

	for scanner.Scan() {
		lineNum++
		if lineNum < offset {
			continue
		}
		if linesRead >= limit {
			// Keep counting total lines
			for scanner.Scan() {
				lineNum++
			}
			break
		}
		fmt.Fprintf(&b, "%6d\t%s\n", lineNum, scanner.Text())
		linesRead++
	}

	if err := scanner.Err(); err != nil {
		return &ToolResult{Error: fmt.Sprintf("read error: %v", err)}, nil
	}

	if linesRead == 0 {
		if lineNum == 0 {
			return &ToolResult{Output: "(empty file)"}, nil
		}
		return &ToolResult{Output: fmt.Sprintf("(no lines in range, file has %d lines)", lineNum)}, nil
	}

	// Notify if output was capped by default limit
	if !userSetLimit && lineNum > limit {
		b.WriteString(fmt.Sprintf("\n(showing first %d of %d lines, use offset/limit for more)\n", linesRead, lineNum))
	}

	return &ToolResult{Output: b.String()}, nil
}
