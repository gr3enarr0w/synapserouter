package tools

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
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

	// Spec file protection — enforced at tool layer, not prompt layer
	if IsProtectedPath(path) {
		return &ToolResult{Error: fmt.Sprintf("Cannot overwrite protected file '%s'. This is the source spec — write plan output to synroute.md instead.", filepath.Base(path))}, nil
	}

	content := stringArg(args, "content")

	// Check for duplicate files with the same basename in the work directory
	var warningMsg string
	basename := filepath.Base(path)
	if dupes := findDuplicateFiles(basename, workDir, path); len(dupes) > 0 {
		warningMsg = fmt.Sprintf("\nWARNING: '%s' already exists at: %s. If you meant to edit the existing file, use file_edit instead.", basename, strings.Join(dupes, ", "))
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return &ToolResult{Error: fmt.Sprintf("cannot create directory %s: %v", dir, err)}, nil
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return &ToolResult{Error: err.Error()}, nil
	}

	output := fmt.Sprintf("wrote %d bytes to %s", len(content), path)
	if warningMsg != "" {
		output += warningMsg
	}
	return &ToolResult{Output: output}, nil
}

// findDuplicateFiles scans rootDir (max 3 levels deep, 100ms timeout) for files
// with the given basename, excluding excludePath. Returns paths of duplicates found.
func findDuplicateFiles(basename, rootDir, excludePath string) []string {
	var dupes []string
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_ = filepath.WalkDir(rootDir, func(p string, d fs.DirEntry, err error) error {
		select {
		case <-ctx.Done():
			return filepath.SkipAll
		default:
		}
		if err != nil {
			return nil
		}
		if d.IsDir() {
			// Limit depth to 3 levels
			rel, _ := filepath.Rel(rootDir, p)
			if rel != "." && strings.Count(rel, string(filepath.Separator)) >= 3 {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Base(p) == basename && p != excludePath {
			dupes = append(dupes, p)
		}
		return nil
	})
	return dupes
}
