package agent

import (
	"testing"

	"github.com/gr3enarr0w/synapserouter/internal/tools"
)

func TestPendingMessageField(t *testing.T) {
	// Create minimal dependencies for CodeREPL
	exec := &mockExecutor{}
	registry := tools.DefaultRegistry()
	renderer := NewCodeRenderer(nil, 120, 40, "test", "auto", "")
	config := DefaultConfig()
	config.WorkDir = t.TempDir()
	agent := New(exec, registry, renderer, config)
	
	cr := NewCodeREPL(agent, renderer, nil)

	// Verify pendingMessage field exists and initializes empty
	if cr.pendingMessage != "" {
		t.Errorf("CodeREPL.pendingMessage should initialize empty, got: %q", cr.pendingMessage)
	}

	// Set a pending message
	cr.pendingMessage = "test message from user"
	
	// Verify it was set
	if cr.pendingMessage != "test message from user" {
		t.Errorf("CodeREPL.pendingMessage = %q, want 'test message from user'", cr.pendingMessage)
	}

	// Clear it
	cr.pendingMessage = ""
	if cr.pendingMessage != "" {
		t.Errorf("CodeREPL.pendingMessage should be empty after clearing, got: %q", cr.pendingMessage)
	}
}
