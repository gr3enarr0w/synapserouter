package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// FileEditTool performs in-place search/replace on a file.
type FileEditTool struct{}

func (t *FileEditTool) Name() string        { return "file_edit" }
func (t *FileEditTool) Description() string { return "Edit a file by replacing an exact string match" }
func (t *FileEditTool) Category() ToolCategory { return CategoryWrite }

func (t *FileEditTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "File path (absolute or relative to work directory)",
			},
			"old_string": map[string]interface{}{
				"type":        "string",
				"description": "The exact string to find and replace",
			},
			"new_string": map[string]interface{}{
				"type":        "string",
				"description": "The replacement string",
			},
			"replace_all": map[string]interface{}{
				"type":        "boolean",
				"description": "Replace all occurrences (default false, fails if not unique)",
			},
		},
		"required": []string{"path", "old_string", "new_string"},
	}
}

func (t *FileEditTool) Execute(ctx context.Context, args map[string]interface{}, workDir string) (*ToolResult, error) {
	path, err := resolveToolPathChecked(stringArg(args, "path"), workDir)
	if err != nil {
		return &ToolResult{Error: err.Error()}, nil
	}

	// Spec file protection
	if IsProtectedPath(path) {
		return &ToolResult{Error: "Cannot overwrite spec file — spec is read-only during implementation."}, nil
	}

	oldStr := stringArg(args, "old_string")
	newStr := stringArg(args, "new_string")
	replaceAll := boolArg(args, "replace_all")

	if oldStr == "" {
		return &ToolResult{Error: "old_string is required"}, nil
	}
	if oldStr == newStr {
		return &ToolResult{Error: "old_string and new_string are identical"}, nil
	}

	info, err := os.Stat(path)
	if err != nil {
		return &ToolResult{Error: err.Error()}, nil
	}
	fileMode := info.Mode()

	data, err := os.ReadFile(path)
	if err != nil {
		return &ToolResult{Error: err.Error()}, nil
	}

	content := string(data)
	count := strings.Count(content, oldStr)

	if count == 0 {
		return &ToolResult{Error: "old_string not found in file"}, nil
	}

	if count > 1 && !replaceAll {
		return &ToolResult{
			Error: fmt.Sprintf("old_string found %d times — use replace_all=true or provide a more specific match", count),
		}, nil
	}

	var result string
	if replaceAll {
		result = strings.ReplaceAll(content, oldStr, newStr)
	} else {
		result = strings.Replace(content, oldStr, newStr, 1)
	}

	// Dry-run mode: show diff without applying
	if DryRunMode {
		diff := generateUnifiedDiff(path, content, result, oldStr, newStr)
		return &ToolResult{Output: fmt.Sprintf("DRY RUN: Would replace %d occurrence(s) in %s\n\n%s", count, path, diff)}, nil
	}

	if err := os.WriteFile(path, []byte(result), fileMode); err != nil { //nolint:G703 // path validated by agent tool permission system
		return &ToolResult{Error: err.Error()}, nil
	}

	// Auto-format Go files with goimports
	if strings.HasSuffix(path, ".go") {
		if _, err := exec.LookPath("goimports"); err == nil {
			cmd := exec.Command("goimports", "-w", path)
			cmd.Run() // ignore errors, fail silently
		}
	}

	replacements := 1
	if replaceAll {
		replacements = count
	}
	return &ToolResult{Output: fmt.Sprintf("replaced %d occurrence(s) in %s", replacements, path)}, nil
}

// generateUnifiedDiff creates a simple unified diff preview
func generateUnifiedDiff(path, oldContent, newContent, oldStr, newStr string) string {
	lines := strings.Split(newContent, "\n")
	preview := ""
	startLine := 0
	for i, line := range lines {
		if strings.Contains(line, newStr) && startLine == 0 {
			startLine = max(0, i-3)
			endLine := min(len(lines), i+4)
			for j := startLine; j < endLine; j++ {
				prefix := " "
				if j == i {
					prefix = "+"
				}
				preview += fmt.Sprintf("%s%s\n", prefix, lines[j])
			}
			break
		}
	}
	if preview == "" {
		preview = strings.Join(lines[:min(10, len(lines))], "\n")
	}
	return fmt.Sprintf("--- %s (old)\n+++ %s (new)\n%s", path, path, preview)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
