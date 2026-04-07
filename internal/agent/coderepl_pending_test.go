package agent

import (
	"testing"

	"github.com/gr3enarr0w/synapserouter/internal/tools"
)

func TestPendingMessageQueue(t *testing.T) {
	exec := &mockExecutor{}
	registry := tools.DefaultRegistry()
	renderer := NewCodeRenderer(nil, 120, 40, "test", "auto", "")
	config := DefaultConfig()
	config.WorkDir = t.TempDir()
	agent := New(exec, registry, renderer, config)

	cr := NewCodeREPL(agent, renderer, nil)

	if got := cr.dequeuePendingMessage(); got != "" {
		t.Fatalf("expected empty queue, got %q", got)
	}

	cr.enqueuePendingMessage("first queued message")
	cr.enqueuePendingMessage("second queued message")
	cr.enqueuePendingMessage("   ")

	if len(cr.pendingMessages) != 2 {
		t.Fatalf("expected 2 queued messages, got %d", len(cr.pendingMessages))
	}
	if got := cr.dequeuePendingMessage(); got != "first queued message" {
		t.Fatalf("unexpected first dequeued message: %q", got)
	}
	if got := cr.dequeuePendingMessage(); got != "second queued message" {
		t.Fatalf("unexpected second dequeued message: %q", got)
	}
	if got := cr.dequeuePendingMessage(); got != "" {
		t.Fatalf("expected queue to be empty, got %q", got)
	}
}
