package worktree

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func setupTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	commands := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
	}
	for _, args := range commands {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if err := cmd.Run(); err != nil {
			t.Fatalf("git setup failed: %v", err)
		}
	}

	// Create a file and initial commit
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test"), 0644)
	cmd := exec.Command("git", "add", ".")
	cmd.Dir = dir
	cmd.Run()
	cmd = exec.Command("git", "commit", "-m", "initial")
	cmd.Dir = dir
	cmd.Run()

	return dir
}

func TestCreateAndDelete(t *testing.T) {
	repo := setupTestRepo(t)
	baseDir := t.TempDir()

	mgr, err := NewManager(Config{
		BaseDir:    baseDir,
		DefaultTTL: 1 * time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}

	wt, err := mgr.Create(repo, "session-1")
	if err != nil {
		t.Fatal(err)
	}

	if wt.Status != StatusActive {
		t.Errorf("expected status active, got %s", wt.Status)
	}
	if wt.SessionID != "session-1" {
		t.Errorf("expected session-1, got %s", wt.SessionID)
	}

	// Verify the worktree directory exists
	if _, err := os.Stat(wt.Path); err != nil {
		t.Errorf("worktree directory should exist: %v", err)
	}

	// Verify list returns it
	list := mgr.List()
	if len(list) != 1 {
		t.Errorf("expected 1 worktree, got %d", len(list))
	}

	// Delete it
	if err := mgr.Delete(wt.ID); err != nil {
		t.Fatal(err)
	}

	// Should be gone
	list = mgr.List()
	if len(list) != 0 {
		t.Errorf("expected 0 worktrees after delete, got %d", len(list))
	}
}

func TestTouch(t *testing.T) {
	repo := setupTestRepo(t)
	baseDir := t.TempDir()

	mgr, err := NewManager(Config{
		BaseDir:    baseDir,
		DefaultTTL: 1 * time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}

	wt, err := mgr.Create(repo, "session-1")
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Delete(wt.ID)

	originalExpiry := wt.ExpiresAt
	time.Sleep(10 * time.Millisecond)

	if err := mgr.Touch(wt.ID); err != nil {
		t.Fatal(err)
	}

	updated, _ := mgr.Get(wt.ID)
	if !updated.ExpiresAt.After(originalExpiry) {
		t.Error("touch should extend expiry")
	}
}

func TestCleanupExpired(t *testing.T) {
	repo := setupTestRepo(t)
	baseDir := t.TempDir()

	mgr, err := NewManager(Config{
		BaseDir:    baseDir,
		DefaultTTL: 1 * time.Millisecond, // expire immediately
	})
	if err != nil {
		t.Fatal(err)
	}

	wt, err := mgr.Create(repo, "session-1")
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(10 * time.Millisecond)

	removed := mgr.Cleanup()
	if removed != 1 {
		t.Errorf("expected 1 removed, got %d", removed)
	}

	// Verify directory is cleaned up
	if _, err := os.Stat(wt.Path); !os.IsNotExist(err) {
		t.Error("expired worktree directory should be removed")
	}
}

func TestActiveCount(t *testing.T) {
	repo := setupTestRepo(t)
	baseDir := t.TempDir()

	mgr, err := NewManager(Config{
		BaseDir:    baseDir,
		DefaultTTL: 1 * time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}

	if mgr.ActiveCount() != 0 {
		t.Error("expected 0 active worktrees initially")
	}

	wt1, _ := mgr.Create(repo, "s1")
	wt2, _ := mgr.Create(repo, "s2")

	if mgr.ActiveCount() != 2 {
		t.Errorf("expected 2 active, got %d", mgr.ActiveCount())
	}

	mgr.Delete(wt1.ID)
	if mgr.ActiveCount() != 1 {
		t.Errorf("expected 1 active after delete, got %d", mgr.ActiveCount())
	}

	mgr.Delete(wt2.ID)
}

func TestGetNotFound(t *testing.T) {
	baseDir := t.TempDir()
	mgr, _ := NewManager(Config{BaseDir: baseDir})

	_, err := mgr.Get("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent worktree")
	}
}

func TestCleanerStops(t *testing.T) {
	baseDir := t.TempDir()
	mgr, _ := NewManager(Config{
		BaseDir:    baseDir,
		DefaultTTL: 1 * time.Hour,
	})

	cancel := StartCleaner(context.Background(), mgr, 50*time.Millisecond)
	time.Sleep(100 * time.Millisecond)
	cancel() // should not panic or deadlock
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.DefaultTTL != 24*time.Hour {
		t.Errorf("expected 24h TTL, got %v", cfg.DefaultTTL)
	}
	if cfg.MaxTotalBytes != 10*1024*1024*1024 {
		t.Errorf("expected 10GB max, got %d", cfg.MaxTotalBytes)
	}
	if cfg.CleanupInterval != 5*time.Minute {
		t.Errorf("expected 5m cleanup, got %v", cfg.CleanupInterval)
	}
}
