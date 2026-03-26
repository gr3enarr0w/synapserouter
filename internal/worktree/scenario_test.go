package worktree

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestScenarioFullWorktreeLifecycle simulates a complete agent session:
// create worktree → use it (touch) → verify isolation → cleanup
func TestScenarioFullWorktreeLifecycle(t *testing.T) {
	repo := setupTestRepo(t)
	baseDir := t.TempDir()

	mgr, err := NewManager(Config{
		BaseDir:         baseDir,
		DefaultTTL:      1 * time.Hour,
		MaxTotalBytes:   10 * 1024 * 1024 * 1024,
		MaxPerTreeBytes: 2 * 1024 * 1024 * 1024,
		CleanupInterval: 5 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Step 1: Create worktree
	wt, err := mgr.Create(repo, "agent-session-1")
	if err != nil {
		t.Fatal(err)
	}
	if wt.Status != StatusActive {
		t.Errorf("expected active status, got %s", wt.Status)
	}
	if wt.SessionID != "agent-session-1" {
		t.Errorf("expected session agent-session-1, got %s", wt.SessionID)
	}

	// Step 2: Verify worktree is isolated — has its own branch
	cmd := exec.Command("git", "branch", "--show-current")
	cmd.Dir = wt.Path
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	branch := string(out)
	if branch == "" {
		t.Error("worktree should be on a branch")
	}

	// Step 3: Make changes in worktree — should NOT affect source repo
	testFile := filepath.Join(wt.Path, "worktree-only.txt")
	os.WriteFile(testFile, []byte("this is worktree-only content"), 0644)

	// Verify file exists in worktree
	if _, err := os.Stat(testFile); err != nil {
		t.Errorf("file should exist in worktree: %v", err)
	}
	// Verify file does NOT exist in source repo
	sourceFile := filepath.Join(repo, "worktree-only.txt")
	if _, err := os.Stat(sourceFile); !os.IsNotExist(err) {
		t.Error("file should NOT exist in source repo")
	}

	// Step 4: Touch to simulate activity
	originalExpiry := wt.ExpiresAt
	originalLastUsed := wt.LastUsedAt
	time.Sleep(10 * time.Millisecond)
	if err := mgr.Touch(wt.ID); err != nil {
		t.Fatal(err)
	}
	updated, _ := mgr.Get(wt.ID)
	if !updated.ExpiresAt.After(originalExpiry) {
		t.Error("touch should extend expiry")
	}
	if !updated.LastUsedAt.After(originalLastUsed) {
		t.Error("touch should update LastUsedAt")
	}

	// Step 5: List — should show one active worktree
	list := mgr.List()
	if len(list) != 1 {
		t.Errorf("expected 1 worktree, got %d", len(list))
	}
	if list[0].ID != wt.ID {
		t.Errorf("expected ID %s, got %s", wt.ID, list[0].ID)
	}

	// Step 6: Check metrics
	if mgr.ActiveCount() != 1 {
		t.Errorf("expected 1 active, got %d", mgr.ActiveCount())
	}
	size := mgr.TotalSize()
	if size <= 0 {
		t.Error("expected positive total size")
	}

	// Step 7: Cleanup should not remove (not expired)
	removed := mgr.Cleanup()
	if removed != 0 {
		t.Errorf("cleanup should not remove active worktrees, removed %d", removed)
	}

	// Step 8: Delete
	if err := mgr.Delete(wt.ID); err != nil {
		t.Fatal(err)
	}
	if mgr.ActiveCount() != 0 {
		t.Errorf("expected 0 active after delete, got %d", mgr.ActiveCount())
	}

	// Verify directory is removed
	if _, err := os.Stat(wt.Path); !os.IsNotExist(err) {
		t.Error("worktree directory should be removed after delete")
	}
}

// TestScenarioMultipleWorktreesSizeCap simulates hitting the disk cap.
func TestScenarioMultipleWorktreesSizeCap(t *testing.T) {
	repo := setupTestRepo(t)
	baseDir := t.TempDir()

	// Create with a large initial cap so both worktrees can be created
	mgr, err := NewManager(Config{
		BaseDir:       baseDir,
		DefaultTTL:    1 * time.Hour,
		MaxTotalBytes: 100 * 1024 * 1024, // 100MB — allows creation
	})
	if err != nil {
		t.Fatal(err)
	}

	wt1, err := mgr.Create(repo, "s1")
	if err != nil {
		t.Fatal(err)
	}
	wt2, err := mgr.Create(repo, "s2")
	if err != nil {
		t.Fatal(err)
	}

	if mgr.ActiveCount() != 2 {
		t.Errorf("expected 2 active, got %d", mgr.ActiveCount())
	}

	// Touch wt2 to make it newer — wt1 should be cleaned up first
	time.Sleep(10 * time.Millisecond)
	mgr.Touch(wt2.ID)

	// Now lower the cap to force cleanup
	mgr.config.MaxTotalBytes = 1

	// Cleanup should remove wt1 (oldest by LastUsedAt) to get under cap
	removed := mgr.Cleanup()
	if removed == 0 {
		t.Error("cleanup should have removed at least one worktree")
	}

	_, err = mgr.Get(wt1.ID)
	if err == nil {
		t.Error("wt1 should be removed (oldest)")
	}

	mgr.Delete(wt2.ID)
}

// TestScenarioCreateBlockedByCountLimit verifies max worktree count.
func TestScenarioCreateBlockedByCountLimit(t *testing.T) {
	repo := setupTestRepo(t)
	baseDir := t.TempDir()

	mgr, err := NewManager(Config{
		BaseDir:      baseDir,
		DefaultTTL:   1 * time.Hour,
		MaxWorktrees: 2,
	})
	if err != nil {
		t.Fatal(err)
	}

	wt1, err := mgr.Create(repo, "s1")
	if err != nil {
		t.Fatal(err)
	}
	wt2, err := mgr.Create(repo, "s2")
	if err != nil {
		t.Fatal(err)
	}

	// Third should be blocked
	_, err = mgr.Create(repo, "s3")
	if err == nil {
		t.Error("expected error when max worktree count reached")
	}

	mgr.Delete(wt1.ID)
	mgr.Delete(wt2.ID)
}

// TestScenarioCreateBlockedByNonGitRepo verifies source validation.
func TestScenarioCreateBlockedByNonGitRepo(t *testing.T) {
	baseDir := t.TempDir()
	nonGitDir := t.TempDir() // not a git repo

	mgr, err := NewManager(Config{
		BaseDir:    baseDir,
		DefaultTTL: 1 * time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = mgr.Create(nonGitDir, "s1")
	if err == nil {
		t.Error("expected error for non-git source directory")
	}
}

// TestScenarioWorktreeWithAgentWorkDir verifies the worktree path works as tool workDir.
func TestScenarioWorktreeWithAgentWorkDir(t *testing.T) {
	repo := setupTestRepo(t)
	baseDir := t.TempDir()

	mgr, err := NewManager(Config{
		BaseDir:    baseDir,
		DefaultTTL: 1 * time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}

	wt, err := mgr.Create(repo, "tool-test")
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Delete(wt.ID)

	// Simulate what the agent would do: use wt.Path as workDir for file operations
	// Write a file using the worktree path
	testPath := filepath.Join(wt.Path, "agent-created.txt")
	if err := os.WriteFile(testPath, []byte("agent was here"), 0644); err != nil {
		t.Fatal(err)
	}

	// Read it back
	data, err := os.ReadFile(testPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "agent was here" {
		t.Errorf("expected 'agent was here', got %q", string(data))
	}

	// Verify it's NOT in the source repo
	if _, err := os.Stat(filepath.Join(repo, "agent-created.txt")); !os.IsNotExist(err) {
		t.Error("agent file should not be in source repo")
	}

	// Verify git status in worktree shows the new file
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = wt.Path
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(wt.Path) {
		t.Error("worktree path should be absolute")
	}
	if len(out) == 0 {
		t.Error("git status should show untracked file")
	}
}
