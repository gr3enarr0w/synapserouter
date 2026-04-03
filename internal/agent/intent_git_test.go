package agent

import (
	"os"
	"strings"
	"testing"
)

func TestDetectGitContext(t *testing.T) {
	// Test with the current repo (synapserouter is a git repo)
	cwd, _ := os.Getwd()
	gc := DetectGitContext(cwd)

	if gc == nil {
		t.Fatal("expected non-nil git context for synapserouter repo")
	}
	if !gc.IsRepo {
		t.Fatal("expected IsRepo=true")
	}
	// Branch may be empty in detached HEAD (e.g., CI tag checkout)
	// Just verify it doesn't crash — branch is optional
	if len(gc.RecentCommits) == 0 {
		t.Error("expected recent commits")
	}
}

func TestDetectGitContextNonRepo(t *testing.T) {
	gc := DetectGitContext(os.TempDir())
	// TempDir might or might not be a git repo, so just check it doesn't crash
	if gc != nil && !gc.IsRepo {
		t.Error("if returned, should have IsRepo=true")
	}
}

func TestFormatGitContext(t *testing.T) {
	gc := &GitContext{
		IsRepo:         true,
		Branch:         "main",
		HasUncommitted: true,
		RecentCommits:  []string{"abc1234 Fix bug", "def5678 Add feature"},
	}

	formatted := FormatGitContext(gc)
	if !strings.Contains(formatted, "main") {
		t.Error("expected branch name")
	}
	if !strings.Contains(formatted, "uncommitted") {
		t.Error("expected uncommitted status")
	}
	if !strings.Contains(formatted, "Fix bug") {
		t.Error("expected commit messages")
	}
}

func TestFormatGitContextNil(t *testing.T) {
	result := FormatGitContext(nil)
	if result != "" {
		t.Fatal("expected empty for nil context")
	}
}
