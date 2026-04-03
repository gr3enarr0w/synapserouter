package agent

import (
	"bytes"
	"context"
	"testing"

	"github.com/gr3enarr0w/synapserouter/internal/providers"
	"github.com/gr3enarr0w/synapserouter/internal/tools"
)

func TestSpawnChild(t *testing.T) {
	exec := &mockExecutor{
		responses: []providers.ChatResponse{{
			Choices: []providers.Choice{{
				Message: providers.Message{Role: "assistant", Content: "child result"},
			}},
		}},
	}

	parent := New(exec, tools.NewRegistry(), NewRenderer(&bytes.Buffer{}), DefaultConfig())
	child := parent.SpawnChild(SpawnConfig{
		Role:    "tester",
		WorkDir: t.TempDir(),
	})

	if child.ParentID() != parent.SessionID() {
		t.Errorf("child parent ID = %q, want %q", child.ParentID(), parent.SessionID())
	}

	children := parent.Children()
	if len(children) != 1 {
		t.Fatalf("expected 1 child ref, got %d", len(children))
	}
	if children[0].Role != "tester" {
		t.Errorf("child role = %q, want tester", children[0].Role)
	}
	if children[0].Status != "running" {
		t.Errorf("child status = %q, want running", children[0].Status)
	}
}

func TestRunChild(t *testing.T) {
	exec := &mockExecutor{
		responses: []providers.ChatResponse{{
			Choices: []providers.Choice{{
				Message: providers.Message{Role: "assistant", Content: "tests pass"},
			}},
		}},
	}

	parent := New(exec, tools.NewRegistry(), NewRenderer(&bytes.Buffer{}), DefaultConfig())
	result, err := parent.RunChild(context.Background(), SpawnConfig{
		Role:    "tester",
		WorkDir: t.TempDir(),
	}, "run the tests")

	if err != nil {
		t.Fatal(err)
	}
	if result != "tests pass" {
		t.Errorf("result = %q, want 'tests pass'", result)
	}

	children := parent.Children()
	if len(children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(children))
	}
	if children[0].Status != "completed" {
		t.Errorf("child status = %q, want completed", children[0].Status)
	}
	if children[0].Result != "tests pass" {
		t.Errorf("child result = %q, want 'tests pass'", children[0].Result)
	}
}

func TestRunChildInheritsConfig(t *testing.T) {
	exec := &mockExecutor{
		responses: []providers.ChatResponse{{
			Choices: []providers.Choice{{
				Message: providers.Message{Role: "assistant", Content: "ok"},
			}},
		}},
	}

	parentConfig := DefaultConfig()
	parentConfig.Model = "claude-sonnet-4-6"
	parentConfig.WorkDir = t.TempDir()

	parent := New(exec, tools.NewRegistry(), NewRenderer(&bytes.Buffer{}), parentConfig)
	child := parent.SpawnChild(SpawnConfig{Role: "coder"})

	// Child should inherit parent model
	if child.config.Model != "claude-sonnet-4-6" {
		t.Errorf("child model = %q, want claude-sonnet-4-6", child.config.Model)
	}
	// Child should inherit parent workdir
	if child.config.WorkDir != parentConfig.WorkDir {
		t.Errorf("child workdir = %q, want %q", child.config.WorkDir, parentConfig.WorkDir)
	}
}

func TestRunChildModelOverride(t *testing.T) {
	exec := &mockExecutor{
		responses: []providers.ChatResponse{{
			Choices: []providers.Choice{{
				Message: providers.Message{Role: "assistant", Content: "ok"},
			}},
		}},
	}

	parent := New(exec, tools.NewRegistry(), NewRenderer(&bytes.Buffer{}), DefaultConfig())
	child := parent.SpawnChild(SpawnConfig{
		Role:  "researcher",
		Model: "gemini-2.5-pro",
	})

	if child.config.Model != "gemini-2.5-pro" {
		t.Errorf("child model = %q, want gemini-2.5-pro", child.config.Model)
	}
}

func TestRunChildWithBudget(t *testing.T) {
	exec := &mockExecutor{
		responses: []providers.ChatResponse{{
			Choices: []providers.Choice{{
				Message: providers.Message{Role: "assistant", Content: "ok"},
			}},
		}},
	}

	parent := New(exec, tools.NewRegistry(), NewRenderer(&bytes.Buffer{}), DefaultConfig())
	child := parent.SpawnChild(SpawnConfig{
		Role:   "tester",
		Budget: &AgentBudget{MaxTurns: 3},
	})

	if child.budget == nil {
		t.Fatal("child budget should be set")
	}
	if child.config.MaxTurns != 3 {
		t.Errorf("child maxTurns = %d, want 3", child.config.MaxTurns)
	}
}

func TestRunChildrenParallel(t *testing.T) {
	// Use a thread-safe executor for parallel test
	exec := &safeExecutor{
		response: providers.ChatResponse{
			Choices: []providers.Choice{{
				Message: providers.Message{Role: "assistant", Content: "done"},
			}},
		},
	}

	parent := New(exec, tools.NewRegistry(), NewRenderer(&bytes.Buffer{}), DefaultConfig())
	tasks := []DelegateTask{
		{Config: SpawnConfig{Role: "tester"}, Task: "test A"},
		{Config: SpawnConfig{Role: "reviewer"}, Task: "review B"},
		{Config: SpawnConfig{Role: "researcher"}, Task: "research C"},
	}

	results := parent.RunChildrenParallel(context.Background(), tasks, 2)
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	for i, r := range results {
		if r.Error != "" {
			t.Errorf("result %d has error: %s", i, r.Error)
		}
	}
}

// safeExecutor is a thread-safe mock executor for parallel tests.
type safeExecutor struct {
	response providers.ChatResponse
}

func (s *safeExecutor) ChatCompletion(ctx context.Context, req providers.ChatRequest, sessionID string) (providers.ChatResponse, error) {
	return s.response, nil
}
