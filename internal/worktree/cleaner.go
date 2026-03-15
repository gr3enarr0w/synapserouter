package worktree

import (
	"context"
	"log"
	"time"
)

// StartCleaner runs a background goroutine that periodically cleans up expired worktrees.
// Returns a cancel function to stop the cleaner.
func StartCleaner(ctx context.Context, mgr *Manager, interval time.Duration) context.CancelFunc {
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				removed := mgr.Cleanup()
				if removed > 0 {
					log.Printf("worktree cleanup: removed %d expired worktrees", removed)
				}
			}
		}
	}()
	return cancel
}
