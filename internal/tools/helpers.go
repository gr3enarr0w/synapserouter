package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// stringArg extracts a string argument from the args map.
func stringArg(args map[string]interface{}, key string) string {
	if v, ok := args[key].(string); ok {
		return v
	}
	return ""
}

// intArg extracts an integer argument from the args map.
func intArg(args map[string]interface{}, key string, defaultVal int) int {
	switch v := args[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	}
	return defaultVal
}

// boolArg extracts a boolean argument from the args map.
func boolArg(args map[string]interface{}, key string) bool {
	if v, ok := args[key].(bool); ok {
		return v
	}
	return false
}

// resolveToolPath resolves a path relative to the work directory and enforces
// containment — the resolved path must be within workDir. Returns an error
// if the path escapes the work directory.
func resolveToolPath(path, workDir string) string {
	resolved := filepath.Clean(path)
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(workDir, resolved)
	}
	return resolved
}

// resolveToolPathChecked resolves and validates that the path stays within workDir.
func resolveToolPathChecked(path, workDir string) (string, error) {
	resolved := resolveToolPath(path, workDir)

	// Resolve symlinks on the work directory
	realWorkDir := filepath.Clean(workDir)
	if r, err := filepath.EvalSymlinks(workDir); err == nil {
		realWorkDir = r
	}

	// For the resolved path, walk up to find the first existing ancestor
	// and resolve symlinks from there
	realResolved := resolved
	if _, err := os.Stat(resolved); err == nil {
		if r, err := filepath.EvalSymlinks(resolved); err == nil {
			realResolved = r
		}
	} else {
		// File doesn't exist yet — find nearest existing ancestor
		ancestor := resolved
		suffix := ""
		for {
			parent := filepath.Dir(ancestor)
			if parent == ancestor {
				break // reached root
			}
			suffix = filepath.Join(filepath.Base(ancestor), suffix)
			ancestor = parent
			if _, err := os.Stat(ancestor); err == nil {
				if r, err := filepath.EvalSymlinks(ancestor); err == nil {
					realResolved = filepath.Join(r, suffix)
				}
				break
			}
		}
	}

	// Verify containment
	if !strings.HasPrefix(realResolved, realWorkDir+string(filepath.Separator)) && realResolved != realWorkDir {
		return "", fmt.Errorf("path %q resolves outside work directory", path)
	}

	return resolved, nil
}
