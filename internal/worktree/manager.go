package worktree

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Manager handles creation, tracking, and cleanup of git worktrees.
type Manager struct {
	config Config
	mu     sync.Mutex
	trees  map[string]*Worktree
}

// NewManager creates a worktree manager with the given config.
func NewManager(config Config) (*Manager, error) {
	if config.BaseDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("cannot determine home directory: %w", err)
		}
		config.BaseDir = filepath.Join(home, ".mcp", "synapse", "worktrees")
	}

	if err := os.MkdirAll(config.BaseDir, 0755); err != nil {
		return nil, fmt.Errorf("cannot create worktree base dir: %w", err)
	}

	return &Manager{
		config: config,
		trees:  make(map[string]*Worktree),
	}, nil
}

// Create creates a new git worktree from the given source repository.
func (m *Manager) Create(sourceRepo, sessionID string) (*Worktree, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Validate source is a git repository
	checkCmd := exec.Command("git", "rev-parse", "--git-dir")
	checkCmd.Dir = sourceRepo
	if err := checkCmd.Run(); err != nil {
		return nil, fmt.Errorf("source %q is not a git repository", sourceRepo)
	}

	// Enforce worktree count limit
	activeCount := 0
	for _, wt := range m.trees {
		if wt.Status == StatusActive {
			activeCount++
		}
	}
	if m.config.MaxWorktrees > 0 && activeCount >= m.config.MaxWorktrees {
		return nil, fmt.Errorf("maximum worktree count (%d) reached", m.config.MaxWorktrees)
	}

	// Enforce total size cap at creation time
	if m.config.MaxTotalBytes > 0 {
		var totalSize int64
		for _, wt := range m.trees {
			if wt.Status == StatusActive {
				totalSize += dirSize(wt.Path)
			}
		}
		if totalSize >= m.config.MaxTotalBytes {
			return nil, fmt.Errorf("total worktree disk usage (%d bytes) exceeds cap (%d bytes)", totalSize, m.config.MaxTotalBytes)
		}
	}

	id := fmt.Sprintf("wt-%d", time.Now().UnixNano())
	branch := fmt.Sprintf("worktree/%s", id)
	wtPath := filepath.Join(m.config.BaseDir, id)

	cmd := exec.Command("git", "worktree", "add", "-b", branch, wtPath)
	cmd.Dir = sourceRepo
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git worktree add failed: %s", strings.TrimSpace(stderr.String()))
	}

	now := time.Now()
	wt := &Worktree{
		ID:         id,
		Path:       wtPath,
		SourceRepo: sourceRepo,
		Branch:     branch,
		SessionID:  sessionID,
		Status:     StatusActive,
		CreatedAt:  now,
		LastUsedAt: now,
		ExpiresAt:  now.Add(m.config.DefaultTTL),
	}

	m.trees[id] = wt
	return wt, nil
}

// Touch updates the LastUsedAt timestamp and resets the TTL.
func (m *Manager) Touch(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	wt, ok := m.trees[id]
	if !ok {
		return fmt.Errorf("worktree not found: %s", id)
	}

	now := time.Now()
	wt.LastUsedAt = now
	wt.ExpiresAt = now.Add(m.config.DefaultTTL)
	return nil
}

// Get returns a worktree by ID.
func (m *Manager) Get(id string) (*Worktree, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	wt, ok := m.trees[id]
	if !ok {
		return nil, fmt.Errorf("worktree not found: %s", id)
	}
	return wt, nil
}

// List returns all tracked worktrees.
func (m *Manager) List() []*Worktree {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make([]*Worktree, 0, len(m.trees))
	for _, wt := range m.trees {
		result = append(result, wt)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	return result
}

// Delete removes a worktree and cleans up its files.
func (m *Manager) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	wt, ok := m.trees[id]
	if !ok {
		return fmt.Errorf("worktree not found: %s", id)
	}

	return m.removeLocked(wt)
}

// Cleanup removes expired worktrees and enforces size caps.
func (m *Manager) Cleanup() (removed int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()

	// Phase 1: Remove expired worktrees
	for _, wt := range m.trees {
		if wt.Status == StatusActive && now.After(wt.ExpiresAt) {
			if err := m.removeLocked(wt); err == nil {
				removed++
			}
		}
	}

	// Phase 2: Enforce total size cap
	var totalSize int64
	for _, wt := range m.trees {
		if wt.Status == StatusActive {
			wt.SizeBytes = dirSize(wt.Path)
			totalSize += wt.SizeBytes
		}
	}

	if totalSize > m.config.MaxTotalBytes {
		// Sort by LastUsedAt, remove oldest first
		active := make([]*Worktree, 0)
		for _, wt := range m.trees {
			if wt.Status == StatusActive {
				active = append(active, wt)
			}
		}
		sort.Slice(active, func(i, j int) bool {
			return active[i].LastUsedAt.Before(active[j].LastUsedAt)
		})

		for _, wt := range active {
			if totalSize <= m.config.MaxTotalBytes {
				break
			}
			totalSize -= wt.SizeBytes
			if err := m.removeLocked(wt); err == nil {
				removed++
			}
		}
	}

	return removed
}

// TotalSize returns the total disk usage of all active worktrees.
func (m *Manager) TotalSize() int64 {
	m.mu.Lock()
	defer m.mu.Unlock()

	var total int64
	for _, wt := range m.trees {
		if wt.Status == StatusActive {
			total += dirSize(wt.Path)
		}
	}
	return total
}

// ActiveCount returns the number of active worktrees.
func (m *Manager) ActiveCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	count := 0
	for _, wt := range m.trees {
		if wt.Status == StatusActive {
			count++
		}
	}
	return count
}

func (m *Manager) removeLocked(wt *Worktree) error {
	// Try git worktree remove first
	cmd := exec.Command("git", "worktree", "remove", "--force", wt.Path)
	cmd.Dir = wt.SourceRepo
	_ = cmd.Run() // best effort

	// Force remove the directory
	os.RemoveAll(wt.Path)

	// Try to delete the branch
	cmd = exec.Command("git", "branch", "-d", wt.Branch)
	cmd.Dir = wt.SourceRepo
	_ = cmd.Run() // best effort

	wt.Status = StatusDeleted
	delete(m.trees, wt.ID)
	return nil
}

// dirSize calculates directory size recursively.
func dirSize(path string) int64 {
	var size int64
	filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size
}
