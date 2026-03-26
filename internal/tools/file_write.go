package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// FileWriteTool creates or overwrites a file.
type FileWriteTool struct{}

func (t *FileWriteTool) Name() string        { return "file_write" }
func (t *FileWriteTool) Description() string { return "Create or overwrite a file with the given content" }
func (t *FileWriteTool) Category() ToolCategory { return CategoryWrite }

func (t *FileWriteTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "File path (absolute or relative to work directory)",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "Content to write to the file",
			},
		},
		"required": []string{"path", "content"},
	}
}

func (t *FileWriteTool) Execute(ctx context.Context, args map[string]interface{}, workDir string) (*ToolResult, error) {
	path, err := resolveToolPathChecked(stringArg(args, "path"), workDir)
	if err != nil {
		return &ToolResult{Error: err.Error()}, nil
	}
	content := stringArg(args, "content")

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return &ToolResult{Error: fmt.Sprintf("cannot create directory %s: %v", dir, err)}, nil
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return &ToolResult{Error: err.Error()}, nil
	}

	return &ToolResult{Output: fmt.Sprintf("wrote %d bytes to %s", len(content), path)}, nil
}
