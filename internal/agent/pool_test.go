package agent

import (
	"bytes"
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/providers"
	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/tools"
)

func TestPoolAcquireRelease(t *testing.T) {
	pool := NewPool(2)

	exec := &mockExecutor{
		responses: []providers.ChatResponse{{
			Choices: []providers.Choice{{
				Message: providers.Message{Role: "assistant", Content: "ok"},
			}},
		}},
	}
	ag := New(exec, tools.NewRegistry(), NewRenderer(&bytes.Buffer{}), DefaultConfig())

	release, err := pool.Acquire(context.Background(), ag, "tester")
	if err != nil {
		t.Fatal(err)
	}

	metrics := pool.Metrics()
	if metrics.Active != 1 {
		t.Errorf("active = %d, want 1", metrics.Active)
	}
	if metrics.TotalSpawned != 1 {
		t.Errorf("total spawned = %d, want 1", metrics.TotalSpawned)
	}

	release()

	metrics = pool.Metrics()
	if metrics.Active != 0 {
		t.Errorf("active after release = %d, want 0", metrics.Active)
	}
	if metrics.Completed != 1 {
		t.Errorf("completed = %d, want 1", metrics.Completed)
	}
}

func TestPoolConcurrencyLimit(t *testing.T) {
	pool := NewPool(1) // Only 1 concurrent agent

	var running int64
	var maxRunning int64

	exec := &mockExecutor{}
	for i := 0; i < 5; i++ {
		exec.responses = append(exec.responses, providers.ChatResponse{
			Choices: []providers.Choice{{
				Message: providers.Message{Role: "assistant", Content: "ok"},
			}},
		})
	}

	done := make(chan struct{})
	go func() {
		ag1 := New(exec, tools.NewRegistry(), NewRenderer(&bytes.Buffer{}), DefaultConfig())
		release, _ := pool.Acquire(context.Background(), ag1, "a")
		cur := atomic.AddInt64(&running, 1)
		if cur > atomic.LoadInt64(&maxRunning) {
			atomic.StoreInt64(&maxRunning, cur)
		}
		time.Sleep(50 * time.Millisecond)
		atomic.AddInt64(&running, -1)
		release()
		close(done)
	}()

	// Give first goroutine time to acquire
	time.Sleep(10 * time.Millisecond)

	ag2 := New(exec, tools.NewRegistry(), NewRenderer(&bytes.Buffer{}), DefaultConfig())
	release, err := pool.Acquire(context.Background(), ag2, "b")
	if err != nil {
		t.Fatal(err)
	}

	cur := atomic.AddInt64(&running, 1)
	if cur > atomic.LoadInt64(&maxRunning) {
		atomic.StoreInt64(&maxRunning, cur)
	}
	release()
	atomic.AddInt64(&running, -1)

	<-done

	if maxRunning > 1 {
		t.Errorf("max concurrent = %d, want at most 1", maxRunning)
	}
}

func TestPoolRunInPool(t *testing.T) {
	pool := NewPool(3)
	exec := &mockExecutor{
		responses: []providers.ChatResponse{{
			Choices: []providers.Choice{{
				Message: providers.Message{Role: "assistant", Content: "pooled result"},
			}},
		}},
	}

	ag := New(exec, tools.NewRegistry(), NewRenderer(&bytes.Buffer{}), DefaultConfig())
	result, err := pool.RunInPool(context.Background(), ag, "tester", "run tests")
	if err != nil {
		t.Fatal(err)
	}
	if result != "pooled result" {
		t.Errorf("result = %q, want 'pooled result'", result)
	}

	metrics := pool.Metrics()
	if metrics.Completed != 1 {
		t.Errorf("completed = %d, want 1", metrics.Completed)
	}
}

func TestPoolContextCancellation(t *testing.T) {
	pool := NewPool(1)

	exec := &mockExecutor{
		responses: []providers.ChatResponse{{
			Choices: []providers.Choice{{
				Message: providers.Message{Role: "assistant", Content: "ok"},
			}},
		}},
	}

	// Fill the pool
	ag1 := New(exec, tools.NewRegistry(), NewRenderer(&bytes.Buffer{}), DefaultConfig())
	release, _ := pool.Acquire(context.Background(), ag1, "blocker")

	// Try to acquire with cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ag2 := New(exec, tools.NewRegistry(), NewRenderer(&bytes.Buffer{}), DefaultConfig())
	_, err := pool.Acquire(ctx, ag2, "waiter")
	if err == nil {
		t.Error("expected error from cancelled context")
	}

	release()
}

func TestPoolDefaultConcurrency(t *testing.T) {
	pool := NewPool(0) // Should default to 5
	if pool.maxConcurrent != 5 {
		t.Errorf("default maxConcurrent = %d, want 5", pool.maxConcurrent)
	}
}
