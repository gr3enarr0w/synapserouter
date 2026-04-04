package tools

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
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
		return &ToolResult{Error: "Cannot overwrite spec file — spec is read-only during implementation."}, nil
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

	// Auto-format Go files with goimports
	if strings.HasSuffix(path, ".go") {
		if _, err := exec.LookPath("goimports"); err == nil {
			cmd := exec.Command("goimports", "-w", path)
			cmd.Run() // ignore errors, fail silently
		}
	}

	output := fmt.Sprintf("wrote %d bytes to %s", len(content), path)
	if warningMsg != "" {
		output += warningMsg
	}

	// Run language-appropriate verification
	if verifyOutput := verifyWrittenFile(path, workDir); verifyOutput != "" {
		output += "\n\n⚠️ VERIFICATION FAILED:\n" + verifyOutput + "\nFix the errors above before proceeding."
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

// runGoBuild runs 'go build ./...' in the given directory and returns any error output.
// Returns empty string if build succeeds.
func runGoBuild(workDir string) string {
	cmd := exec.Command("go", "build", "./...")
	cmd.Dir = workDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output)
	}
	return ""
}

// verifyWrittenFile runs language-appropriate syntax verification on the written file.
// Returns empty string if verification succeeds or if language is not supported.
func verifyWrittenFile(path, workDir string) string {
	ext := strings.ToLower(filepath.Ext(path))
	
	switch ext {
	case ".go":
		return runGoBuild(workDir)
	
	case ".py":
		cmd := exec.Command("python", "-m", "py_compile", path)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return string(output)
		}
		return ""
	
	case ".js":
		cmd := exec.Command("node", "--check", path)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return string(output)
		}
		return ""
	
	case ".ts", ".tsx":
		// Only run if tsconfig.json exists
		if _, err := os.Stat(filepath.Join(workDir, "tsconfig.json")); err == nil {
			cmd := exec.Command("npx", "tsc", "--noEmit")
			cmd.Dir = workDir
			output, err := cmd.CombinedOutput()
			if err != nil {
				return string(output)
			}
		}
		return ""
	
	case ".rs":
		// Only run if Cargo.toml exists
		if _, err := os.Stat(filepath.Join(workDir, "Cargo.toml")); err == nil {
			cmd := exec.Command("cargo", "check")
			cmd.Dir = workDir
			output, err := cmd.CombinedOutput()
			if err != nil {
				return string(output)
			}
		}
		return ""
	
	case ".java":
		cmd := exec.Command("javac", path)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return string(output)
		}
		return ""
	
	case ".rb":
		cmd := exec.Command("ruby", "-c", path)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return string(output)
		}
		return ""
	
	case ".cpp", ".h", ".hpp", ".cc":
		cmd := exec.Command("g++", "-fsyntax-only", path)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return string(output)
		}
		return ""
	
	default:
		return ""
	}
}
