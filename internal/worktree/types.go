package worktree

import "time"

// WorktreeStatus represents the lifecycle state of a worktree.
type WorktreeStatus string

const (
	StatusActive  WorktreeStatus = "active"
	StatusStale   WorktreeStatus = "stale"
	StatusDeleted WorktreeStatus = "deleted"
)

// Worktree represents a managed git worktree with lifecycle tracking.
type Worktree struct {
	ID         string         `json:"id"`
	Path       string         `json:"path"`
	SourceRepo string         `json:"source_repo"`
	Branch     string         `json:"branch"`
	SessionID  string         `json:"session_id"`
	Status     WorktreeStatus `json:"status"`
	SizeBytes  int64          `json:"size_bytes"`
	CreatedAt  time.Time      `json:"created_at"`
	LastUsedAt time.Time      `json:"last_used_at"`
	ExpiresAt  time.Time      `json:"expires_at"`
}

// Config controls worktree lifecycle behavior.
type Config struct {
	BaseDir         string        // base directory for worktrees (default: ~/.mcp/synapse/worktrees/)
	DefaultTTL      time.Duration // how long before expiry (default: 24h)
	MaxTotalBytes   int64         // total disk cap (default: 10GB)
	MaxPerTreeBytes int64         // per-worktree cap (default: 2GB)
	MaxWorktrees    int           // max concurrent worktrees (default: 20)
	CleanupInterval time.Duration // how often cleanup runs (default: 5m)
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		DefaultTTL:      24 * time.Hour,
		MaxTotalBytes:   10 * 1024 * 1024 * 1024, // 10GB
		MaxPerTreeBytes: 2 * 1024 * 1024 * 1024,  // 2GB
		MaxWorktrees:    20,
		CleanupInterval: 5 * time.Minute,
	}
}
