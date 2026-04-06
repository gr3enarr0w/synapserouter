package router

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ConcurrencyManager tracks in-flight requests per provider and enforces
// per-provider concurrency limits. Uses semaphores (not global locks)
// so different providers can process requests independently.
type ConcurrencyManager struct {
	mu       sync.RWMutex
	sems     map[string]chan struct{} // provider name → buffered channel (semaphore)
	defaults int                     // default max concurrent per provider
}

// NewConcurrencyManager creates a concurrency manager with a default limit.
// Individual providers can have custom limits set via SetLimit().
func NewConcurrencyManager(defaultMaxConcurrent int) *ConcurrencyManager {
	if defaultMaxConcurrent <= 0 {
		defaultMaxConcurrent = 10 // generous default
	}
	return &ConcurrencyManager{
		sems:     make(map[string]chan struct{}),
		defaults: defaultMaxConcurrent,
	}
}

// SetLimit sets the concurrency limit for a specific provider.
func (cm *ConcurrencyManager) SetLimit(provider string, maxConcurrent int) {
	if maxConcurrent <= 0 {
		return
	}
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.sems[provider] = make(chan struct{}, maxConcurrent)
}

func (cm *ConcurrencyManager) getSem(provider string) chan struct{} {
	cm.mu.RLock()
	sem, ok := cm.sems[provider]
	cm.mu.RUnlock()
	if ok {
		return sem
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()
	// Double-check after acquiring write lock
	if sem, ok := cm.sems[provider]; ok {
		return sem
	}
	sem = make(chan struct{}, cm.defaults)
	cm.sems[provider] = sem
	return sem
}

// Acquire acquires a concurrency slot for a provider. Blocks until a slot
// is available or the context expires. Returns a release function that MUST
// be called when the request completes (including streaming body reads).
func (cm *ConcurrencyManager) Acquire(ctx context.Context, provider string) (release func(), err error) {
	sem := cm.getSem(provider)

	select {
	case sem <- struct{}{}:
		// Acquired slot
		released := false
		return func() {
			if !released {
				released = true
				<-sem
			}
		}, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("concurrency limit reached for provider %s: %w", provider, ctx.Err())
	}
}

// AcquireWithTimeout is like Acquire but with an explicit timeout for queuing.
// This prevents requests from waiting indefinitely when the semaphore is full.
func (cm *ConcurrencyManager) AcquireWithTimeout(ctx context.Context, provider string, queueTimeout time.Duration) (release func(), err error) {
	sem := cm.getSem(provider)

	// Create a deadline for queuing (separate from request context)
	queueCtx, cancel := context.WithTimeout(ctx, queueTimeout)
	defer cancel()

	select {
	case sem <- struct{}{}:
		released := false
		return func() {
			if !released {
				released = true
				<-sem
			}
		}, nil
	case <-queueCtx.Done():
		if ctx.Err() != nil {
			return nil, fmt.Errorf("request cancelled while waiting for provider %s slot: %w", provider, ctx.Err())
		}
		return nil, fmt.Errorf("timed out waiting for provider %s slot (queue timeout %s, %d/%d in use)",
			provider, queueTimeout, len(sem), cap(sem))
	}
}

// InFlight returns the number of in-flight requests for a provider.
func (cm *ConcurrencyManager) InFlight(provider string) int {
	sem := cm.getSem(provider)
	return len(sem)
}

// Stats returns concurrency stats for all tracked providers.
func (cm *ConcurrencyManager) Stats() map[string]ConcurrencyStats {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	stats := make(map[string]ConcurrencyStats, len(cm.sems))
	for name, sem := range cm.sems {
		stats[name] = ConcurrencyStats{
			InFlight: len(sem),
			Limit:    cap(sem),
		}
	}
	return stats
}

// ConcurrencyStats holds per-provider concurrency info.
type ConcurrencyStats struct {
	InFlight int `json:"in_flight"`
	Limit    int `json:"limit"`
}

