package tools

import (
	"path/filepath"
	"strings"
)

// PermissionMode controls how tool execution is authorized.
type PermissionMode string

const (
	ModeInteractive PermissionMode = "interactive" // prompt user for write ops
	ModeAutoApprove PermissionMode = "auto_approve" // approve all
	ModeReadOnly    PermissionMode = "read_only"    // deny all writes
)

// PermissionChecker determines whether a tool call should be allowed.
type PermissionChecker struct {
	Mode            PermissionMode
	AutoApproveGlob []string // file patterns to auto-approve (e.g., "*.go", "*.md")
	DenyPatterns    []string // file patterns to always deny
}

// NewPermissionChecker creates a checker with the given mode.
func NewPermissionChecker(mode PermissionMode) *PermissionChecker {
	return &PermissionChecker{Mode: mode}
}

// PermissionResult describes whether a tool call is allowed.
type PermissionResult struct {
	Allowed bool
	Reason  string
	Prompt  bool // if true, the caller should prompt the user
}

// Check evaluates whether a tool call should be allowed.
func (pc *PermissionChecker) Check(tool Tool, args map[string]interface{}) PermissionResult {
	category := tool.Category()
	// Use dynamic category if the tool supports it (e.g., git status=read_only, git push=dangerous)
	if dyn, ok := tool.(DynamicCategoryTool); ok {
		category = dyn.CategoryForArgs(args)
	}

	// Read-only tools are always allowed
	if category == CategoryReadOnly {
		return PermissionResult{Allowed: true}
	}

	switch pc.Mode {
	case ModeReadOnly:
		return PermissionResult{
			Allowed: false,
			Reason:  "read-only mode: write operations are disabled",
		}

	case ModeAutoApprove:
		// Check deny patterns first
		if pc.matchesDenyPattern(args) {
			return PermissionResult{
				Allowed: false,
				Reason:  "path matches deny pattern",
			}
		}
		// Dangerous tools still prompt in auto-approve
		if category == CategoryDangerous {
			return PermissionResult{
				Allowed: false,
				Prompt:  true,
				Reason:  "dangerous operation requires confirmation",
			}
		}
		return PermissionResult{Allowed: true}

	case ModeInteractive:
		// Check auto-approve patterns
		if pc.matchesAutoApprovePattern(args) {
			return PermissionResult{Allowed: true}
		}
		// Check deny patterns
		if pc.matchesDenyPattern(args) {
			return PermissionResult{
				Allowed: false,
				Reason:  "path matches deny pattern",
			}
		}
		// Prompt for all write operations
		return PermissionResult{
			Allowed: false,
			Prompt:  true,
			Reason:  "write operation requires approval",
		}

	default:
		return PermissionResult{
			Allowed: false,
			Reason:  "unknown permission mode",
		}
	}
}

func (pc *PermissionChecker) matchesAutoApprovePattern(args map[string]interface{}) bool {
	return matchesAnyGlob(pc.AutoApproveGlob, extractPath(args))
}

func (pc *PermissionChecker) matchesDenyPattern(args map[string]interface{}) bool {
	return matchesAnyGlob(pc.DenyPatterns, extractPath(args))
}

func extractPath(args map[string]interface{}) string {
	for _, key := range []string{"path", "file", "command"} {
		if v, ok := args[key].(string); ok {
			return v
		}
	}
	return ""
}

func matchesAnyGlob(patterns []string, target string) bool {
	if target == "" || len(patterns) == 0 {
		return false
	}
	base := filepath.Base(target)
	for _, p := range patterns {
		if matched, _ := filepath.Match(p, base); matched {
			return true
		}
		if strings.Contains(target, p) {
			return true
		}
	}
	return false
}
