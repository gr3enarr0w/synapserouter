package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// GlobTool finds files matching a glob pattern.
type GlobTool struct{}

func (t *GlobTool) Name() string        { return "glob" }
func (t *GlobTool) Description() string { return "Find files matching a glob pattern" }
func (t *GlobTool) Category() ToolCategory { return CategoryReadOnly }

func (t *GlobTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"pattern": map[string]interface{}{
				"type":        "string",
				"description": "Glob pattern (e.g. '**/*.go', 'src/*.ts')",
			},
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Base directory to search from (default: work directory)",
			},
		},
		"required": []string{"pattern"},
	}
}

func (t *GlobTool) Execute(ctx context.Context, args map[string]interface{}, workDir string) (*ToolResult, error) {
	pattern := stringArg(args, "pattern")
	if pattern == "" {
		return &ToolResult{Error: "pattern is required"}, nil
	}

	basePath := workDir
	if p := stringArg(args, "path"); p != "" {
		basePath = resolveToolPath(p, workDir)
	}

	type fileEntry struct {
		path    string
		modTime int64
	}
	var matches []fileEntry

	// Check if the pattern uses ** (recursive glob)
	isRecursive := strings.Contains(pattern, "**")

	err := filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip inaccessible
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// Skip hidden directories (but not the root)
		if info.IsDir() && strings.HasPrefix(info.Name(), ".") && path != basePath {
			return filepath.SkipDir
		}
		// Skip common large directories
		if info.IsDir() {
			switch info.Name() {
			case "node_modules", "vendor", ".git", "__pycache__", ".cache":
				return filepath.SkipDir
			}
		}
		if info.IsDir() {
			return nil
		}

		relPath, _ := filepath.Rel(basePath, path)

		var matched bool
		if isRecursive {
			// For ** patterns, match the filename against the non-recursive part
			nonRecursive := strings.TrimPrefix(pattern, "**/")
			matched, _ = filepath.Match(nonRecursive, info.Name())
			// Also try matching the full relative path
			if !matched {
				matched, _ = filepath.Match(pattern, relPath)
			}
		} else {
			matched, _ = filepath.Match(pattern, relPath)
			if !matched {
				matched, _ = filepath.Match(pattern, info.Name())
			}
		}

		if matched {
			matches = append(matches, fileEntry{path: relPath, modTime: info.ModTime().Unix()})
		}
		return nil
	})

	if err != nil {
		return &ToolResult{Error: err.Error()}, nil
	}

	// Sort by modification time (newest first)
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].modTime > matches[j].modTime
	})

	if len(matches) == 0 {
		return &ToolResult{Output: "no files matched"}, nil
	}

	var b strings.Builder
	for _, m := range matches {
		fmt.Fprintln(&b, m.path)
	}
	return &ToolResult{Output: strings.TrimRight(b.String(), "\n")}, nil
}
