package orchestration

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/mcp"
	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/memory"
	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/providers"
	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/tools"
)

type stubExecutor struct{}

func (stubExecutor) ChatCompletion(ctx context.Context, req providers.ChatRequest, sessionID string) (providers.ChatResponse, error) {
	content := ""
	if len(req.Messages) > 0 {
		content = req.Messages[len(req.Messages)-1].Content
	}
	return providers.ChatResponse{
		ID:     "stub",
		Object: "chat.completion",
		Model:  "stub-model",
		Choices: []providers.Choice{
			{
				Index: 0,
				Message: providers.Message{
					Role:    "assistant",
					Content: fmt.Sprintf("executed:%s:%s", sessionID, content),
				},
				FinishReason: "stop",
			},
		},
	}, nil
}

type toolLoopExecutor struct {
	calls int
}

func (e *toolLoopExecutor) ChatCompletion(ctx context.Context, req providers.ChatRequest, sessionID string) (providers.ChatResponse, error) {
	e.calls++
	if e.calls == 1 {
		return providers.ChatResponse{
			ID:     "tool-loop-1",
			Object: "chat.completion",
			Model:  "stub-model",
			Choices: []providers.Choice{
				{
					Index: 0,
					Message: providers.Message{
						Role:    "assistant",
						Content: "Checking workflows first.",
						ToolCalls: []map[string]interface{}{
							{
								"id":   "call-1",
								"type": "function",
								"function": map[string]interface{}{
									"name":      "search_memory",
									"arguments": `{"session_id":"orch-tool-session","query":"workflow templates","max_tokens":1000}`,
								},
							},
						},
					},
					FinishReason: "tool_calls",
				},
			},
		}, nil
	}
	last := req.Messages[len(req.Messages)-1]
	return providers.ChatResponse{
		ID:     "tool-loop-2",
		Object: "chat.completion",
		Model:  "stub-model",
		Choices: []providers.Choice{
			{
				Index: 0,
				Message: providers.Message{
					Role:    "assistant",
					Content: "finalized with tool data: " + last.Content,
				},
				FinishReason: "stop",
			},
		},
	}, nil
}

func TestBuildPlanIncludesCoreExecutionRoles(t *testing.T) {
	// Skill dispatch is the primary path when no explicit roles are given.
	// "Implement and test a Go orchestration workflow" matches go-patterns,
	// code-implement, go-testing — ordered by phase (analyze → implement → verify).
	steps := BuildPlan("Implement and test a Go orchestration workflow", nil, 0)
	if len(steps) < 3 {
		t.Fatalf("expected at least 3 steps from skill dispatch, got %d", len(steps))
	}

	// First step should be analyze-phase (go-patterns → coder role)
	if steps[0].Role != "coder" {
		t.Fatalf("expected first role coder (from go-patterns skill), got %s", steps[0].Role)
	}
	if !strings.Contains(steps[0].Prompt, "go-patterns") {
		t.Fatalf("expected first step to be go-patterns skill, got prompt: %s", steps[0].Prompt)
	}

	// When explicit roles are provided, it falls back to role-based planning
	roleSteps := BuildPlan("something generic", []string{"researcher", "coder", "reviewer"}, 0)
	if roleSteps[0].Role != "researcher" {
		t.Fatalf("with explicit roles, expected first role researcher, got %s", roleSteps[0].Role)
	}
	if roleSteps[len(roleSteps)-1].Role != "reviewer" {
		t.Fatalf("with explicit roles, expected last role reviewer, got %s", roleSteps[len(roleSteps)-1].Role)
	}
}

// TestCreateTask_SkillDispatchIntegration verifies that CreateTask uses skill
// dispatch when no explicit roles are given, producing skill-aware steps.
func TestCreateTask_SkillDispatchIntegration(t *testing.T) {
	db := newOrchestrationTestDB(t)
	vm := memory.NewVectorMemory(db)
	mgr := NewManagerWithStore(stubExecutor{}, vm, db)

	tests := []struct {
		name       string
		goal       string
		wantSkill  string // at least one step prompt must contain this skill name
		wantPhases int    // minimum distinct phases
	}{
		{
			name:       "Go implementation task",
			goal:       "implement Go error handling for the router",
			wantSkill:  "go-patterns",
			wantPhases: 2,
		},
		{
			name:       "security fix",
			goal:       "fix the OAuth credential leak",
			wantSkill:  "security-review",
			wantPhases: 2,
		},
		{
			name:       "Docker task",
			goal:       "create a Dockerfile for the service",
			wantSkill:  "docker-expert",
			wantPhases: 1,
		},
		{
			name:       "full pipeline",
			goal:       "refactor the Go API handler, add tests, and review the code",
			wantSkill:  "go-patterns",
			wantPhases: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task, err := mgr.CreateTask(context.Background(), TaskRequest{
				Goal: tt.goal,
			})
			if err != nil {
				t.Fatalf("CreateTask failed: %v", err)
			}

			if task.Status != TaskStatusQueued {
				t.Errorf("expected queued status, got %s", task.Status)
			}
			if len(task.Steps) == 0 {
				t.Fatal("expected steps from skill dispatch")
			}

			// Verify wanted skill is present in prompts
			found := false
			for _, step := range task.Steps {
				if strings.Contains(step.Prompt, tt.wantSkill) {
					found = true
					break
				}
			}
			if !found {
				prompts := make([]string, len(task.Steps))
				for i, s := range task.Steps {
					prompts[i] = s.Prompt[:min(80, len(prompts[i]))]
				}
				t.Errorf("expected skill %q in step prompts, got: %v", tt.wantSkill, prompts)
			}

			// Verify step prompts contain the goal
			for i, step := range task.Steps {
				if !strings.Contains(step.Prompt, tt.goal) {
					t.Errorf("step %d prompt missing goal text", i)
				}
			}
		})
	}
}

// TestCreateTask_FallbackToRoles verifies unmatched goals still produce role-based plans.
func TestCreateTask_FallbackToRoles(t *testing.T) {
	db := newOrchestrationTestDB(t)
	vm := memory.NewVectorMemory(db)
	mgr := NewManagerWithStore(stubExecutor{}, vm, db)

	task, err := mgr.CreateTask(context.Background(), TaskRequest{
		Goal: "hello world",
	})
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	// Should fall back to role-based planning (researcher, architect, coder, tester, reviewer)
	if len(task.Steps) < 4 {
		t.Errorf("expected at least 4 role-based steps, got %d", len(task.Steps))
	}

	// First step should be researcher (from inferRoles)
	if task.Steps[0].Role != "researcher" {
		t.Errorf("fallback first role should be researcher, got %s", task.Steps[0].Role)
	}
}

// TestCreateTask_ExplicitRolesOverrideSkills verifies explicit roles bypass skill dispatch.
func TestCreateTask_ExplicitRolesOverrideSkills(t *testing.T) {
	db := newOrchestrationTestDB(t)
	vm := memory.NewVectorMemory(db)
	mgr := NewManagerWithStore(stubExecutor{}, vm, db)

	task, err := mgr.CreateTask(context.Background(), TaskRequest{
		Goal:  "implement Go handler with tests",
		Roles: []string{"coder", "reviewer"},
	})
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	if len(task.Steps) != 2 {
		t.Fatalf("expected 2 steps from explicit roles, got %d", len(task.Steps))
	}
	if task.Steps[0].Role != "coder" {
		t.Errorf("expected first role coder, got %s", task.Steps[0].Role)
	}
	if task.Steps[1].Role != "reviewer" {
		t.Errorf("expected second role reviewer, got %s", task.Steps[1].Role)
	}
	// Prompts should NOT contain skill names (role-based, not skill-based)
	for _, step := range task.Steps {
		if strings.Contains(step.Prompt, "[go-patterns]") {
			t.Error("explicit roles should not produce skill-based prompts")
		}
	}
}

// TestCreateTaskAndExecute_SkillDispatch runs a skill-dispatched task through
// the full execution pipeline with stub executor.
func TestCreateTaskAndExecute_SkillDispatch(t *testing.T) {
	db := newOrchestrationTestDB(t)
	vm := memory.NewVectorMemory(db)
	mgr := NewManagerWithStore(stubExecutor{}, vm, db)

	task, err := mgr.CreateTask(context.Background(), TaskRequest{
		Goal:    "fix the Go auth token handler",
		Execute: true,
	})
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	// Wait for async execution
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		got, _ := mgr.GetTask(task.ID)
		if got != nil && (got.Status == TaskStatusCompleted || got.Status == TaskStatusFailed) {
			task = got
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if task.Status != TaskStatusCompleted {
		t.Fatalf("expected completed, got %s (error: %s)", task.Status, task.Error)
	}

	// All steps should be completed
	for i, step := range task.Steps {
		if step.Status != StepStatusCompleted {
			t.Errorf("step %d (%s) status: expected completed, got %s", i, step.Role, step.Status)
		}
		if step.Output == "" {
			t.Errorf("step %d (%s) has empty output", i, step.Role)
		}
	}

	// Final output should contain all step outputs
	if task.FinalOutput == "" {
		t.Error("expected non-empty final output")
	}

	// Verify skill-based steps were used (not generic role prompts)
	hasSkillPrompt := false
	for _, step := range task.Steps {
		if strings.Contains(step.Prompt, "[") && strings.Contains(step.Prompt, "]") {
			hasSkillPrompt = true
			break
		}
	}
	if !hasSkillPrompt {
		t.Error("expected skill-based prompts with [skill-name] markers")
	}
}

// TestManagerSkillsMethod verifies the Skills() accessor returns the registry.
func TestManagerSkillsMethod(t *testing.T) {
	db := newOrchestrationTestDB(t)
	vm := memory.NewVectorMemory(db)
	mgr := NewManagerWithStore(stubExecutor{}, vm, db)

	skills := mgr.Skills()
	if len(skills) == 0 {
		t.Fatal("expected non-empty skill registry")
	}
	if len(skills) != len(DefaultSkills()) {
		t.Errorf("expected %d skills, got %d", len(DefaultSkills()), len(skills))
	}
}

// TestManagerMatchSkillsForGoal verifies the dry-run match endpoint.
func TestManagerMatchSkillsForGoal(t *testing.T) {
	db := newOrchestrationTestDB(t)
	vm := memory.NewVectorMemory(db)
	mgr := NewManagerWithStore(stubExecutor{}, vm, db)

	result := mgr.MatchSkillsForGoal("fix the Go auth handler and verify tests")
	if result.FallbackToRole {
		t.Fatal("should not fall back for this goal")
	}
	if len(result.MatchedSkills) < 3 {
		t.Errorf("expected at least 3 matched skills, got %d: %v",
			len(result.MatchedSkills), skillNames(result.MatchedSkills))
	}
	if len(result.Steps) == 0 {
		t.Error("expected steps in result")
	}

	// Test fallback case
	fallback := mgr.MatchSkillsForGoal("just chatting")
	if !fallback.FallbackToRole {
		t.Error("expected fallback for unmatched goal")
	}
}

// === MCP Auto-Invocation Tests ===
// These prove that MCP tools are automatically called by the skill dispatch
// system during task execution — no user action needed.

// newMockMCPServer creates an httptest server that fakes the MCP protocol.
// It returns the server, and an atomic counter of how many tool calls it received.
func newMockMCPServer(t *testing.T, serverName string, toolNames []string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var callCount atomic.Int32

	mux := http.NewServeMux()

	// GET /health — always healthy
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// GET /tools/list — return registered tools
	mux.HandleFunc("/tools/list", func(w http.ResponseWriter, r *http.Request) {
		tools := make([]map[string]interface{}, len(toolNames))
		for i, name := range toolNames {
			tools[i] = map[string]interface{}{
				"name":        name,
				"description": "mock tool " + name,
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"tools": tools})
	})

	// POST /tools/call — record the call and return mock result
	mux.HandleFunc("/tools/call", func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)

		var req struct {
			Tool      string                 `json:"tool"`
			Arguments map[string]interface{} `json:"arguments"`
		}
		json.NewDecoder(r.Body).Decode(&req)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"output":  fmt.Sprintf("mock-result-from-%s: query=%v", req.Tool, req.Arguments["query"]),
		})
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server, &callCount
}

// setupMockMCPClient creates an MCP client connected to mock servers matching
// the tool names used in DefaultSkills (context7 and research-mcp).
func setupMockMCPClient(t *testing.T) (*mcp.MCPClient, *atomic.Int32, *atomic.Int32) {
	t.Helper()

	// Mock context7 server (serves query-docs)
	ctx7Server, ctx7Calls := newMockMCPServer(t, "context7", []string{"query-docs"})

	// Mock research-mcp server (serves research_search)
	researchServer, researchCalls := newMockMCPServer(t, "research-mcp", []string{"research_search"})

	client := mcp.NewMCPClient()

	if err := client.AddServer("context7", ctx7Server.URL, ""); err != nil {
		t.Fatal(err)
	}
	if err := client.AddServer("research-mcp", researchServer.URL, ""); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	if err := client.Connect(ctx, "context7"); err != nil {
		t.Fatal(err)
	}
	if err := client.Connect(ctx, "research-mcp"); err != nil {
		t.Fatal(err)
	}

	return client, ctx7Calls, researchCalls
}

// TestMCPAutoInvoke_GoTask verifies that a Go-related task auto-invokes
// context7.query-docs without any user action.
func TestMCPAutoInvoke_GoTask(t *testing.T) {
	db := newOrchestrationTestDB(t)
	vm := memory.NewVectorMemory(db)
	mgr := NewManagerWithStore(stubExecutor{}, vm, db)

	client, ctx7Calls, researchCalls := setupMockMCPClient(t)
	mgr.SetMCPClient(client)

	task, err := mgr.CreateTask(context.Background(), TaskRequest{
		Goal:    "implement a Go HTTP handler with proper error handling",
		Execute: true,
	})
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	// Wait for execution
	waitForTaskDone(t, mgr, task.ID, 5*time.Second)

	// context7.query-docs should have been called (go-patterns skill binds it)
	if ctx7Calls.Load() == 0 {
		t.Error("context7.query-docs was NOT auto-invoked — expected automatic call from go-patterns skill")
	}

	// research-mcp should NOT have been called (no research/security trigger)
	if researchCalls.Load() > 0 {
		t.Error("research-mcp.research_search should not fire for a plain Go task")
	}

	// Verify task completed and MCP context was injected
	got, _ := mgr.GetTask(task.ID)
	if got.Status != TaskStatusCompleted {
		t.Fatalf("task should be completed, got %s (error: %s)", got.Status, got.Error)
	}

	// The first step output should contain the goal (stub executor echoes it)
	// and MCP context should have been fed as previousOutputs[0]
	if got.FinalOutput == "" {
		t.Error("expected non-empty final output")
	}
}

// TestMCPAutoInvoke_SecurityTask verifies that a security-related task
// auto-invokes research-mcp.research_search.
func TestMCPAutoInvoke_SecurityTask(t *testing.T) {
	db := newOrchestrationTestDB(t)
	vm := memory.NewVectorMemory(db)
	mgr := NewManagerWithStore(stubExecutor{}, vm, db)

	client, _, researchCalls := setupMockMCPClient(t)
	mgr.SetMCPClient(client)

	task, err := mgr.CreateTask(context.Background(), TaskRequest{
		Goal:    "fix the OAuth credential handling vulnerability",
		Execute: true,
	})
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	waitForTaskDone(t, mgr, task.ID, 5*time.Second)

	// research-mcp should have been called (security-review binds it)
	if researchCalls.Load() == 0 {
		t.Error("research-mcp.research_search was NOT auto-invoked — expected automatic call from security-review skill")
	}
}

// TestMCPAutoInvoke_ResearchTask verifies that research tasks invoke BOTH
// research-mcp and context7 automatically.
func TestMCPAutoInvoke_ResearchTask(t *testing.T) {
	db := newOrchestrationTestDB(t)
	vm := memory.NewVectorMemory(db)
	mgr := NewManagerWithStore(stubExecutor{}, vm, db)

	client, ctx7Calls, researchCalls := setupMockMCPClient(t)
	mgr.SetMCPClient(client)

	task, err := mgr.CreateTask(context.Background(), TaskRequest{
		Goal:    "research how gRPC streaming works in Go",
		Execute: true,
	})
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	waitForTaskDone(t, mgr, task.ID, 5*time.Second)

	// Both should fire — research skill binds both
	if ctx7Calls.Load() == 0 {
		t.Error("context7.query-docs was NOT auto-invoked for research task")
	}
	if researchCalls.Load() == 0 {
		t.Error("research-mcp.research_search was NOT auto-invoked for research task")
	}
}

// TestMCPAutoInvoke_NoMatchDoesNotCallMCP verifies that tasks with no
// matching skills don't invoke any MCP tools.
func TestMCPAutoInvoke_NoMatchDoesNotCallMCP(t *testing.T) {
	db := newOrchestrationTestDB(t)
	vm := memory.NewVectorMemory(db)
	mgr := NewManagerWithStore(stubExecutor{}, vm, db)

	client, ctx7Calls, researchCalls := setupMockMCPClient(t)
	mgr.SetMCPClient(client)

	task, err := mgr.CreateTask(context.Background(), TaskRequest{
		Goal:    "hello world",
		Execute: true,
	})
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	waitForTaskDone(t, mgr, task.ID, 5*time.Second)

	if ctx7Calls.Load() > 0 {
		t.Error("context7 should NOT be called for unmatched goal")
	}
	if researchCalls.Load() > 0 {
		t.Error("research-mcp should NOT be called for unmatched goal")
	}
}

// TestMCPAutoInvoke_NilClientDoesNotPanic verifies that tasks execute fine
// without an MCP client (graceful no-op).
func TestMCPAutoInvoke_NilClientDoesNotPanic(t *testing.T) {
	db := newOrchestrationTestDB(t)
	vm := memory.NewVectorMemory(db)
	mgr := NewManagerWithStore(stubExecutor{}, vm, db)
	// Deliberately NOT setting an MCP client

	task, err := mgr.CreateTask(context.Background(), TaskRequest{
		Goal:    "implement Go handler with auth tokens",
		Execute: true,
	})
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	waitForTaskDone(t, mgr, task.ID, 5*time.Second)

	got, _ := mgr.GetTask(task.ID)
	if got.Status != TaskStatusCompleted {
		t.Errorf("task should complete even without MCP client, got %s", got.Status)
	}
}

// TestMCPAutoInvoke_MCPFailureDoesNotBlockTask verifies that if an MCP tool
// returns an error, the task still completes (MCP is best-effort).
func TestMCPAutoInvoke_MCPFailureDoesNotBlockTask(t *testing.T) {
	db := newOrchestrationTestDB(t)
	vm := memory.NewVectorMemory(db)
	mgr := NewManagerWithStore(stubExecutor{}, vm, db)

	// Create a failing MCP server
	failServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/tools/list" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"tools": []map[string]interface{}{
					{"name": "query-docs", "description": "docs"},
				},
			})
			return
		}
		// /tools/call always fails
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"success":false,"error":"server on fire"}`))
	}))
	t.Cleanup(failServer.Close)

	client := mcp.NewMCPClient()
	client.AddServer("context7", failServer.URL, "")
	client.Connect(context.Background(), "context7")
	mgr.SetMCPClient(client)

	task, err := mgr.CreateTask(context.Background(), TaskRequest{
		Goal:    "implement a Go handler",
		Execute: true,
	})
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	waitForTaskDone(t, mgr, task.ID, 5*time.Second)

	got, _ := mgr.GetTask(task.ID)
	if got.Status != TaskStatusCompleted {
		t.Errorf("task should complete despite MCP failure, got %s (error: %s)", got.Status, got.Error)
	}
}

// TestMCPAutoInvoke_ContextInjectedIntoSteps verifies that MCP tool results
// appear in the final output (they get injected as previousOutputs[0]).
func TestMCPAutoInvoke_ContextInjectedIntoSteps(t *testing.T) {
	db := newOrchestrationTestDB(t)
	vm := memory.NewVectorMemory(db)
	mgr := NewManagerWithStore(stubExecutor{}, vm, db)

	client, _, _ := setupMockMCPClient(t)
	mgr.SetMCPClient(client)

	task, err := mgr.CreateTask(context.Background(), TaskRequest{
		Goal:    "implement a Go handler",
		Execute: true,
	})
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	waitForTaskDone(t, mgr, task.ID, 5*time.Second)

	got, _ := mgr.GetTask(task.ID)
	// The stub executor echoes the prompt content. Since MCP context is injected
	// as the first previousOutput, the second step's messages will contain it.
	// The final output joins all step outputs.
	if !strings.Contains(got.FinalOutput, "mock-result-from-query-docs") {
		t.Errorf("expected MCP mock result in final output, got:\n%s", got.FinalOutput)
	}
}

// TestMCPAutoInvoke_DeduplicatesToolCalls verifies that when multiple skills
// bind the same MCP tool, it's only called once.
func TestMCPAutoInvoke_DeduplicatesToolCalls(t *testing.T) {
	db := newOrchestrationTestDB(t)
	vm := memory.NewVectorMemory(db)
	mgr := NewManagerWithStore(stubExecutor{}, vm, db)

	client, ctx7Calls, _ := setupMockMCPClient(t)
	mgr.SetMCPClient(client)

	// "implement Go tests" matches go-patterns + code-implement + go-testing
	// Both go-patterns and go-testing bind context7.query-docs
	task, err := mgr.CreateTask(context.Background(), TaskRequest{
		Goal:    "implement Go tests for the router",
		Execute: true,
	})
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	waitForTaskDone(t, mgr, task.ID, 5*time.Second)

	// context7.query-docs should be called exactly once despite multiple skills binding it
	if ctx7Calls.Load() != 1 {
		t.Errorf("context7.query-docs should be called exactly 1 time (deduplicated), got %d", ctx7Calls.Load())
	}
}

func waitForTaskDone(t *testing.T, mgr *Manager, taskID string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		got, _ := mgr.GetTask(taskID)
		if got != nil && (got.Status == TaskStatusCompleted || got.Status == TaskStatusFailed) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("task %s did not finish within %v", taskID, timeout)
}

func TestResolveAgentTemplateSupportsExpandedAgentCatalog(t *testing.T) {
	template := ResolveAgentTemplate("queen-coordinator")
	if template.Type != "queen-coordinator" {
		t.Fatalf("expected queen-coordinator template, got %s", template.Type)
	}
	if len(template.Capabilities) == 0 {
		t.Fatal("expected queen-coordinator capabilities")
	}

	hybrid := AgentTypesForTopology("hybrid")
	if !slices.Contains(hybrid, "queen-coordinator") || !slices.Contains(hybrid, "deployer") {
		t.Fatalf("expected hybrid topology to include expanded agent set, got %v", hybrid)
	}
}

func TestManagerExecutesWorkflowTask(t *testing.T) {
	db := newOrchestrationTestDB(t)
	vm := memory.NewVectorMemory(db)
	manager := NewManager(stubExecutor{}, vm)

	task, err := manager.CreateTask(context.Background(), TaskRequest{
		Goal:    "Implement and test orchestration",
		Execute: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		current, err := manager.GetTask(task.ID)
		if err != nil {
			t.Fatal(err)
		}
		if current.Status == TaskStatusCompleted {
			if current.FinalOutput == "" {
				t.Fatal("expected final output")
			}
			for _, step := range current.Steps {
				if step.Status != StepStatusCompleted {
					t.Fatalf("expected completed step, got %s", step.Status)
				}
				if step.Output == "" {
					t.Fatal("expected step output")
				}
			}
			return
		}
		time.Sleep(25 * time.Millisecond)
	}

	t.Fatal("workflow task did not complete before deadline")
}

func TestManagerPublishesStructuredAutopilotEvents(t *testing.T) {
	db := newOrchestrationTestDB(t)
	vm := memory.NewVectorMemory(db)
	manager := NewManager(&toolLoopExecutor{}, vm)
	if err := vm.Store("workflow templates include development and sparc", "assistant", "orch-tool-session", map[string]interface{}{"source": "test"}); err != nil {
		t.Fatal(err)
	}

	task, err := manager.CreateTask(context.Background(), TaskRequest{
		Goal:      "Inspect workflow templates",
		SessionID: "orch-tool-session",
		Roles:     []string{"researcher"},
		Execute:   false,
	})
	if err != nil {
		t.Fatal(err)
	}

	stream, cancel, err := manager.SubscribeTask(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()

	go manager.runTask(task.ID)

	var eventTypes []string
	deadline := time.After(2 * time.Second)
	for {
		select {
		case event, ok := <-stream:
			if !ok {
				if !slices.Contains(eventTypes, "autopilot_plan") {
					t.Fatalf("expected autopilot_plan event, got %v", eventTypes)
				}
				if !slices.Contains(eventTypes, "task_step_start") {
					t.Fatalf("expected task_step_start event, got %v", eventTypes)
				}
				if !slices.Contains(eventTypes, "task_step_complete") {
					t.Fatalf("expected task_step_complete event, got %v", eventTypes)
				}
				if !slices.Contains(eventTypes, "autopilot_progress") {
					t.Fatalf("expected autopilot_progress event, got %v", eventTypes)
				}
				if !slices.Contains(eventTypes, "tool_call_start") {
					t.Fatalf("expected tool_call_start event, got %v", eventTypes)
				}
				if !slices.Contains(eventTypes, "tool_call_result") {
					t.Fatalf("expected tool_call_result event, got %v", eventTypes)
				}
				return
			}
			eventTypes = append(eventTypes, event.Type)
		case <-deadline:
			t.Fatalf("timed out waiting for structured events: %v", eventTypes)
		}
	}
}

func TestManagerExecutesToolCallsAndContinuesStep(t *testing.T) {
	db := newOrchestrationTestDB(t)
	vm := memory.NewVectorMemory(db)
	exec := &toolLoopExecutor{}
	manager := NewManager(exec, vm)
	if err := vm.Store("workflow templates include development and sparc", "assistant", "orch-tool-session", map[string]interface{}{"source": "test"}); err != nil {
		t.Fatal(err)
	}

	task, err := manager.CreateTask(context.Background(), TaskRequest{
		Goal:      "Inspect workflow templates",
		SessionID: "orch-tool-session",
		Roles:     []string{"researcher"},
		Execute:   true,
	})
	if err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		current, err := manager.GetTask(task.ID)
		if err != nil {
			t.Fatal(err)
		}
		if current.Status == TaskStatusCompleted {
			if exec.calls < 2 {
				t.Fatalf("expected at least 2 model calls, got %d", exec.calls)
			}
			if !strings.Contains(current.FinalOutput, "development") {
				t.Fatalf("expected tool result content in final output, got %q", current.FinalOutput)
			}
			return
		}
		time.Sleep(25 * time.Millisecond)
	}

	t.Fatal("tool loop task did not complete before deadline")
}

func TestExecuteBuiltInToolCallSupportsRefineTask(t *testing.T) {
	db := newOrchestrationTestDB(t)
	vm := memory.NewVectorMemory(db)
	manager := NewManager(stubExecutor{}, vm)

	task, err := manager.CreateTask(context.Background(), TaskRequest{
		Goal: "Refine me",
	})
	if err != nil {
		t.Fatal(err)
	}

	name, result, err := manager.executeBuiltInToolCall(map[string]interface{}{
		"function": map[string]interface{}{
			"name":      "refine_task",
			"arguments": fmt.Sprintf(`{"task_id":"%s","feedback":"Add testing","execute":false}`, task.ID),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if name != "refine_task" {
		t.Fatalf("expected refine_task, got %s", name)
	}
	if !strings.Contains(result, `"parent_task_id":"`+task.ID+`"`) {
		t.Fatalf("expected refinement result to reference parent task, got %s", result)
	}
}

func TestExecuteBuiltInToolCallSupportsCoordinateSwarm(t *testing.T) {
	db := newOrchestrationTestDB(t)
	vm := memory.NewVectorMemory(db)
	manager := NewManager(stubExecutor{}, vm)

	swarm, err := manager.InitSwarm(context.Background(), SwarmRequest{
		Objective: "Coordinate me",
		MaxAgents: 2,
	})
	if err != nil {
		t.Fatal(err)
	}

	name, result, err := manager.executeBuiltInToolCall(map[string]interface{}{
		"function": map[string]interface{}{
			"name":      "coordinate_swarm",
			"arguments": fmt.Sprintf(`{"swarm_id":"%s","agents":4}`, swarm.ID),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if name != "coordinate_swarm" {
		t.Fatalf("expected coordinate_swarm, got %s", name)
	}
	if !strings.Contains(result, `"max_agents":4`) {
		t.Fatalf("expected scaled swarm result, got %s", result)
	}
}

func TestExecuteBuiltInToolCallSupportsResumeAndForkSession(t *testing.T) {
	db := newOrchestrationTestDB(t)
	vm := memory.NewVectorMemory(db)
	manager := NewManager(stubExecutor{}, vm)

	if _, err := manager.CreateTask(context.Background(), TaskRequest{
		Goal:      "Session baseline",
		SessionID: "sess-a",
	}); err != nil {
		t.Fatal(err)
	}

	_, resumeResult, err := manager.executeBuiltInToolCall(map[string]interface{}{
		"function": map[string]interface{}{
			"name":      "resume_session",
			"arguments": `{"session_id":"sess-a","goal":"Resume session work","execute":false}`,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resumeResult, `"session_id":"sess-a"`) {
		t.Fatalf("expected resumed session result, got %s", resumeResult)
	}

	_, forkResult, err := manager.executeBuiltInToolCall(map[string]interface{}{
		"function": map[string]interface{}{
			"name":      "fork_session",
			"arguments": `{"source_session_id":"sess-a","session_id":"sess-b","goal":"Forked branch","execute":false}`,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(forkResult, `"session_id":"sess-b"`) {
		t.Fatalf("expected forked session result, got %s", forkResult)
	}
}

func TestExecuteBuiltInToolCallSupportsAssignStartAndCancelTask(t *testing.T) {
	db := newOrchestrationTestDB(t)
	vm := memory.NewVectorMemory(db)
	manager := NewManager(stubExecutor{}, vm)

	task, err := manager.CreateTask(context.Background(), TaskRequest{
		Goal: "Control task lifecycle",
	})
	if err != nil {
		t.Fatal(err)
	}
	agents := manager.ListAgents()
	if len(agents) == 0 {
		t.Fatal("expected default agents")
	}

	_, assignResult, err := manager.executeBuiltInToolCall(map[string]interface{}{
		"function": map[string]interface{}{
			"name":      "assign_task",
			"arguments": fmt.Sprintf(`{"task_id":"%s","agent_id":"%s"}`, task.ID, agents[0].ID),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(assignResult, `"assigned_to":"`+agents[0].ID+`"`) {
		t.Fatalf("expected assigned task result, got %s", assignResult)
	}

	_, startResult, err := manager.executeBuiltInToolCall(map[string]interface{}{
		"function": map[string]interface{}{
			"name":      "start_task",
			"arguments": fmt.Sprintf(`{"task_id":"%s"}`, task.ID),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(startResult, `"id":"`+task.ID+`"`) {
		t.Fatalf("expected started task result, got %s", startResult)
	}

	cancelTask, err := manager.CreateTask(context.Background(), TaskRequest{
		Goal: "Cancel me",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, cancelResult, err := manager.executeBuiltInToolCall(map[string]interface{}{
		"function": map[string]interface{}{
			"name":      "cancel_task",
			"arguments": fmt.Sprintf(`{"task_id":"%s"}`, cancelTask.ID),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(cancelResult, `"status":"cancelled"`) {
		t.Fatalf("expected cancelled task result, got %s", cancelResult)
	}
}

func TestExecuteBuiltInToolCallSupportsAgentAndSwarmManagement(t *testing.T) {
	db := newOrchestrationTestDB(t)
	vm := memory.NewVectorMemory(db)
	manager := NewManager(stubExecutor{}, vm)

	_, spawnResult, err := manager.executeBuiltInToolCall(map[string]interface{}{
		"function": map[string]interface{}{
			"name":      "spawn_agent",
			"arguments": `{"type":"designer","name":"ui-worker"}`,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(spawnResult, `"type":"designer"`) {
		t.Fatalf("expected spawned designer agent, got %s", spawnResult)
	}

	agents := manager.ListAgents()
	agentID := agents[len(agents)-1].ID
	_, stopResult, err := manager.executeBuiltInToolCall(map[string]interface{}{
		"function": map[string]interface{}{
			"name":      "stop_agent",
			"arguments": fmt.Sprintf(`{"agent_id":"%s"}`, agentID),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stopResult, `"status":"stopped"`) {
		t.Fatalf("expected stopped agent result, got %s", stopResult)
	}

	if _, listAgentsResult, err := manager.executeBuiltInToolCall(map[string]interface{}{
		"function": map[string]interface{}{
			"name":      "list_agents",
			"arguments": `{"status":"stopped"}`,
		},
	}); err != nil {
		t.Fatal(err)
	} else if !strings.Contains(listAgentsResult, `"status":"stopped"`) {
		t.Fatalf("expected stopped agents in result, got %s", listAgentsResult)
	}

	if _, listSwarmsResult, err := manager.executeBuiltInToolCall(map[string]interface{}{
		"function": map[string]interface{}{
			"name":      "list_swarms",
			"arguments": `{}`,
		},
	}); err != nil {
		t.Fatal(err)
	} else if !strings.HasPrefix(strings.TrimSpace(listSwarmsResult), "[") {
		t.Fatalf("expected swarm list json array, got %s", listSwarmsResult)
	}
}

func TestExecuteBuiltInToolCallSupportsDelegationAndSwarmLifecycle(t *testing.T) {
	db := newOrchestrationTestDB(t)
	vm := memory.NewVectorMemory(db)
	manager := NewManager(stubExecutor{}, vm)

	swarm, err := manager.InitSwarm(context.Background(), SwarmRequest{
		Objective: "Delegation target",
		Topology:  "hierarchical",
		MaxAgents: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	task, err := manager.CreateTask(context.Background(), TaskRequest{
		Goal:  "Implement delegation logic",
		Roles: []string{"coder"},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, delegatedResult, err := manager.executeBuiltInToolCall(map[string]interface{}{
		"function": map[string]interface{}{
			"name":      "delegate_task",
			"arguments": fmt.Sprintf(`{"task_id":"%s","swarm_id":"%s"}`, task.ID, swarm.ID),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(delegatedResult, `"assigned_to":"`) {
		t.Fatalf("expected delegated task assignment, got %s", delegatedResult)
	}

	_, startResult, err := manager.executeBuiltInToolCall(map[string]interface{}{
		"function": map[string]interface{}{
			"name":      "start_swarm",
			"arguments": fmt.Sprintf(`{"swarm_id":"%s","session_id":"swarm-session"}`, swarm.ID),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(startResult, `"session_id":"swarm-session"`) {
		t.Fatalf("expected started swarm task, got %s", startResult)
	}

	_, stopResult, err := manager.executeBuiltInToolCall(map[string]interface{}{
		"function": map[string]interface{}{
			"name":      "stop_swarm",
			"arguments": fmt.Sprintf(`{"swarm_id":"%s"}`, swarm.ID),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stopResult, `"status":"stopped"`) {
		t.Fatalf("expected stopped swarm result, got %s", stopResult)
	}
}

func TestExecuteBuiltInToolCallSupportsCreationFlows(t *testing.T) {
	db := newOrchestrationTestDB(t)
	vm := memory.NewVectorMemory(db)
	manager := NewManagerWithStore(stubExecutor{}, vm, db)

	_, createResult, err := manager.executeBuiltInToolCall(map[string]interface{}{
		"function": map[string]interface{}{
			"name":      "create_task",
			"arguments": `{"goal":"Plan a release","session_id":"sess-create","roles":["architect","reviewer"],"execute":false,"max_steps":2}`,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(createResult, `"goal":"Plan a release"`) || !strings.Contains(createResult, `"session_id":"sess-create"`) {
		t.Fatalf("expected created task result, got %s", createResult)
	}

	_, initResult, err := manager.executeBuiltInToolCall(map[string]interface{}{
		"function": map[string]interface{}{
			"name":      "init_swarm",
			"arguments": `{"objective":"Ship a feature","topology":"delivery","strategy":"shipping","max_agents":3,"execute":false,"agent_types":["queen-coordinator","coder","tester"]}`,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(initResult, `"objective":"Ship a feature"`) || !strings.Contains(initResult, `"topology":"delivery"`) {
		t.Fatalf("expected initialized swarm result, got %s", initResult)
	}

	_, runResult, err := manager.executeBuiltInToolCall(map[string]interface{}{
		"function": map[string]interface{}{
			"name":      "run_workflow_template",
			"arguments": `{"template_id":"development","objective":"Build a release candidate","session_id":"wf-dev","execute":false}`,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(runResult, `"swarm":`) || !strings.Contains(runResult, `"objective":"Build a release candidate"`) {
		t.Fatalf("expected workflow run result, got %s", runResult)
	}
}

func TestExecuteBuiltInToolCallSupportsWorkflowTemplateLifecycle(t *testing.T) {
	db := newOrchestrationTestDB(t)
	vm := memory.NewVectorMemory(db)
	manager := NewManagerWithStore(stubExecutor{}, vm, db)

	_, saveResult, err := manager.executeBuiltInToolCall(map[string]interface{}{
		"function": map[string]interface{}{
			"name":      "save_workflow_template",
			"arguments": `{"id":"release-train","name":"Release Train","description":"Ship in coordinated stages","topology":"delivery","strategy":"release","agent_types":["queen-coordinator","coder","tester","deployer"],"roles":["architect","coder","tester","documenter"]}`,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(saveResult, `"id":"release-train"`) {
		t.Fatalf("expected saved workflow template, got %s", saveResult)
	}

	_, getResult, err := manager.executeBuiltInToolCall(map[string]interface{}{
		"function": map[string]interface{}{
			"name":      "get_workflow_template",
			"arguments": `{"template_id":"release-train"}`,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(getResult, `"name":"Release Train"`) {
		t.Fatalf("expected fetched workflow template, got %s", getResult)
	}

	_, deleteResult, err := manager.executeBuiltInToolCall(map[string]interface{}{
		"function": map[string]interface{}{
			"name":      "delete_workflow_template",
			"arguments": `{"template_id":"release-train"}`,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(deleteResult, `"deleted":true`) {
		t.Fatalf("expected deleted workflow template result, got %s", deleteResult)
	}
}

func TestExecuteBuiltInToolCallSupportsSwarmRebalancing(t *testing.T) {
	db := newOrchestrationTestDB(t)
	vm := memory.NewVectorMemory(db)
	manager := NewManagerWithStore(stubExecutor{}, vm, db)

	swarm, err := manager.InitSwarm(context.Background(), SwarmRequest{
		Objective: "Ship a release safely",
		Topology:  "delivery",
		Strategy:  "release",
		MaxAgents: 4,
		AgentTypes: []string{
			"queen-coordinator",
			"deployer",
			"release-manager",
			"tester",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	task, err := manager.CreateTask(context.Background(), TaskRequest{
		Goal:      "Prepare deployment rollout and release checks",
		SessionID: "rebalance-session",
		Roles:     []string{"coder"},
	})
	if err != nil {
		t.Fatal(err)
	}

	manager.mu.Lock()
	manager.swarms[swarm.ID].TaskIDs = append(manager.swarms[swarm.ID].TaskIDs, task.ID)
	manager.mu.Unlock()

	_, result, err := manager.executeBuiltInToolCall(map[string]interface{}{
		"function": map[string]interface{}{
			"name":      "rebalance_swarm",
			"arguments": fmt.Sprintf(`{"swarm_id":"%s"}`, swarm.ID),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, `"rebalanced":1`) {
		t.Fatalf("expected rebalance result, got %s", result)
	}

	current, err := manager.GetTask(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.AssignedTo == "" {
		t.Fatal("expected task assignment after rebalance")
	}
}

func TestSwarmLoadPreviewAndWorkStealing(t *testing.T) {
	db := newOrchestrationTestDB(t)
	vm := memory.NewVectorMemory(db)
	manager := NewManagerWithStore(stubExecutor{}, vm, db)

	swarm, err := manager.InitSwarm(context.Background(), SwarmRequest{
		Objective:  "Balance a swarm",
		Topology:   "delivery",
		Strategy:   "release",
		MaxAgents:  3,
		AgentTypes: []string{"queen-coordinator", "coder", "tester"},
	})
	if err != nil {
		t.Fatal(err)
	}
	task, err := manager.CreateTask(context.Background(), TaskRequest{
		Goal:      "Prepare release validation",
		SessionID: "steal-session",
		Roles:     []string{"coder", "tester"},
	})
	if err != nil {
		t.Fatal(err)
	}
	assigned, err := manager.AssignTask(task.ID, swarm.AgentIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	manager.mu.Lock()
	manager.swarms[swarm.ID].TaskIDs = append(manager.swarms[swarm.ID].TaskIDs, task.ID)
	manager.mu.Unlock()

	load, err := manager.SwarmLoad(swarm.ID)
	if err != nil {
		t.Fatal(err)
	}
	if load.TotalAgents == 0 || len(load.Agents) == 0 {
		t.Fatalf("expected load overview, got %+v", load)
	}

	preview, err := manager.PreviewRebalance(swarm.ID)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Load == nil {
		t.Fatal("expected preview to include load snapshot")
	}

	stealable, err := manager.ListStealableTasks(swarm.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stealable) == 0 {
		t.Fatal("expected at least one stealable task")
	}

	stealResult, err := manager.StealTask(context.Background(), task.ID, swarm.ID, swarm.AgentIDs[1])
	if err != nil {
		t.Fatal(err)
	}
	if stealResult.FromAgentID != assigned.AssignedTo || stealResult.ToAgentID != swarm.AgentIDs[1] {
		t.Fatalf("unexpected steal result: %+v", stealResult)
	}

	if _, result, err := manager.executeBuiltInToolCall(map[string]interface{}{
		"function": map[string]interface{}{
			"name":      "get_swarm_load",
			"arguments": fmt.Sprintf(`{"swarm_id":"%s"}`, swarm.ID),
		},
	}); err != nil {
		t.Fatal(err)
	} else if !strings.Contains(result, `"swarm_id":"`+swarm.ID+`"`) {
		t.Fatalf("expected swarm load result, got %s", result)
	}

	if _, result, err := manager.executeBuiltInToolCall(map[string]interface{}{
		"function": map[string]interface{}{
			"name":      "detect_swarm_imbalance",
			"arguments": fmt.Sprintf(`{"swarm_id":"%s"}`, swarm.ID),
		},
	}); err != nil {
		t.Fatal(err)
	} else if !strings.Contains(result, `"swarm_id":"`+swarm.ID+`"`) {
		t.Fatalf("expected imbalance result, got %s", result)
	}

	if _, result, err := manager.executeBuiltInToolCall(map[string]interface{}{
		"function": map[string]interface{}{
			"name":      "preview_rebalance",
			"arguments": fmt.Sprintf(`{"swarm_id":"%s"}`, swarm.ID),
		},
	}); err != nil {
		t.Fatal(err)
	} else if !strings.Contains(result, `"load":`) {
		t.Fatalf("expected preview result, got %s", result)
	}

	if _, result, err := manager.executeBuiltInToolCall(map[string]interface{}{
		"function": map[string]interface{}{
			"name":      "list_stealable_tasks",
			"arguments": fmt.Sprintf(`{"swarm_id":"%s"}`, swarm.ID),
		},
	}); err != nil {
		t.Fatal(err)
	} else if !strings.Contains(result, task.ID) {
		t.Fatalf("expected stealable task listing, got %s", result)
	}

	task2, err := manager.CreateTask(context.Background(), TaskRequest{
		Goal:      "Steal via tool",
		SessionID: "steal-session-2",
		Roles:     []string{"coder"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.AssignTask(task2.ID, swarm.AgentIDs[0]); err != nil {
		t.Fatal(err)
	}
	manager.mu.Lock()
	manager.swarms[swarm.ID].TaskIDs = append(manager.swarms[swarm.ID].TaskIDs, task2.ID)
	manager.mu.Unlock()
	if _, result, err := manager.executeBuiltInToolCall(map[string]interface{}{
		"function": map[string]interface{}{
			"name":      "steal_task",
			"arguments": fmt.Sprintf(`{"task_id":"%s","swarm_id":"%s","stealer_id":"%s"}`, task2.ID, swarm.ID, swarm.AgentIDs[2]),
		},
	}); err != nil {
		t.Fatal(err)
	} else if !strings.Contains(result, `"to_agent_id":"`+swarm.AgentIDs[2]+`"`) {
		t.Fatalf("expected steal result, got %s", result)
	}
}

func TestWorkflowStateMetricsDebugAndSwarmPauseResume(t *testing.T) {
	db := newOrchestrationTestDB(t)
	vm := memory.NewVectorMemory(db)
	manager := NewManagerWithStore(stubExecutor{}, vm, db)

	swarm, err := manager.InitSwarm(context.Background(), SwarmRequest{
		Objective: "Workflow execution",
		Topology:  "hierarchical",
		Strategy:  "development",
	})
	if err != nil {
		t.Fatal(err)
	}
	task, err := manager.StartSwarm(context.Background(), swarm.ID, "workflow-session")
	if err != nil {
		t.Fatal(err)
	}
	manager.mu.Lock()
	internalTask := manager.tasks[task.ID]
	now := time.Now()
	internalTask.Status = TaskStatusPaused
	internalTask.StartedAt = &now
	internalTask.CompletedAt = &now
	internalTask.FinalOutput = "done"
	manager.persistTask(internalTask)
	manager.mu.Unlock()

	state, err := manager.WorkflowState(swarm.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.ID != swarm.ID || !state.Paused {
		t.Fatalf("unexpected workflow state: %+v", state)
	}

	metrics, err := manager.WorkflowMetrics(swarm.ID)
	if err != nil {
		t.Fatal(err)
	}
	if metrics.TasksTotal == 0 {
		t.Fatalf("expected workflow metrics, got %+v", metrics)
	}

	debugInfo, err := manager.WorkflowDebugInfo(swarm.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(debugInfo.ExecutionTrace) == 0 {
		t.Fatal("expected workflow debug trace")
	}

	if _, result, err := manager.executeBuiltInToolCall(map[string]interface{}{
		"function": map[string]interface{}{
			"name":      "get_workflow_state",
			"arguments": fmt.Sprintf(`{"workflow_id":"%s"}`, swarm.ID),
		},
	}); err != nil {
		t.Fatal(err)
	} else if !strings.Contains(result, `"id":"`+swarm.ID+`"`) {
		t.Fatalf("expected workflow state result, got %s", result)
	}

	if _, result, err := manager.executeBuiltInToolCall(map[string]interface{}{
		"function": map[string]interface{}{
			"name":      "get_workflow_metrics",
			"arguments": fmt.Sprintf(`{"workflow_id":"%s"}`, swarm.ID),
		},
	}); err != nil {
		t.Fatal(err)
	} else if !strings.Contains(result, `"tasks_total":`) {
		t.Fatalf("expected workflow metrics result, got %s", result)
	}

	if _, result, err := manager.executeBuiltInToolCall(map[string]interface{}{
		"function": map[string]interface{}{
			"name":      "get_workflow_debug_info",
			"arguments": fmt.Sprintf(`{"workflow_id":"%s"}`, swarm.ID),
		},
	}); err != nil {
		t.Fatal(err)
	} else if !strings.Contains(result, `"execution_trace":`) {
		t.Fatalf("expected workflow debug result, got %s", result)
	}

	paused, err := manager.PauseSwarm(swarm.ID)
	if err != nil {
		t.Fatal(err)
	}
	if paused.Status != SwarmStatusReady {
		t.Fatalf("expected paused swarm to be ready, got %s", paused.Status)
	}

	resumed, err := manager.ResumeSwarm(swarm.ID)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.Status != SwarmStatusRunning {
		t.Fatalf("expected resumed swarm to be running, got %s", resumed.Status)
	}
}

func TestTaskStealContestAndStaleDetection(t *testing.T) {
	db := newOrchestrationTestDB(t)
	vm := memory.NewVectorMemory(db)
	manager := NewManagerWithStore(stubExecutor{}, vm, db)

	swarm, err := manager.InitSwarm(context.Background(), SwarmRequest{
		Objective: "Contest stealing",
		Topology:  "hierarchical",
		MaxAgents: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	task, err := manager.CreateTask(context.Background(), TaskRequest{
		Goal: "Reassign contested task",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.AssignTask(task.ID, swarm.AgentIDs[0]); err != nil {
		t.Fatal(err)
	}
	manager.mu.Lock()
	manager.swarms[swarm.ID].TaskIDs = append(manager.swarms[swarm.ID].TaskIDs, task.ID)
	oldTime := time.Now().Add(-2 * time.Hour)
	manager.tasks[task.ID].StartedAt = &oldTime
	manager.persistTask(manager.tasks[task.ID])
	manager.mu.Unlock()

	if _, err := manager.StealTask(context.Background(), task.ID, swarm.ID, swarm.AgentIDs[1]); err != nil {
		t.Fatal(err)
	}
	contest, err := manager.ContestTaskSteal(task.ID, swarm.AgentIDs[0], "original owner wants it back")
	if err != nil {
		t.Fatal(err)
	}
	if contest.OriginalAgentID != swarm.AgentIDs[0] {
		t.Fatalf("unexpected contest: %+v", contest)
	}
	resolved, err := manager.ResolveTaskStealContest(task.ID, swarm.AgentIDs[0], "queen restored ownership")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.WinnerAgentID != swarm.AgentIDs[0] {
		t.Fatalf("unexpected resolution: %+v", resolved)
	}

	stale, err := manager.DetectStaleTasks(swarm.ID, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(stale) == 0 {
		t.Fatal("expected stale task detection")
	}

	if _, result, err := manager.executeBuiltInToolCall(map[string]interface{}{
		"function": map[string]interface{}{
			"name":      "contest_task_steal",
			"arguments": fmt.Sprintf(`{"task_id":"%s","original_agent_id":"%s","reason":"contest again"}`, task.ID, swarm.AgentIDs[0]),
		},
	}); err != nil {
		t.Fatal(err)
	} else if !strings.Contains(result, `"task_id":"`+task.ID+`"`) {
		t.Fatalf("expected contest tool result, got %s", result)
	}

	if _, result, err := manager.executeBuiltInToolCall(map[string]interface{}{
		"function": map[string]interface{}{
			"name":      "resolve_task_contest",
			"arguments": fmt.Sprintf(`{"task_id":"%s","winner_agent_id":"%s","reason":"resolved"}`, task.ID, swarm.AgentIDs[1]),
		},
	}); err != nil {
		t.Fatal(err)
	} else if !strings.Contains(result, `"winner_agent_id":"`+swarm.AgentIDs[1]+`"`) {
		t.Fatalf("expected contest resolution tool result, got %s", result)
	}

	if _, result, err := manager.executeBuiltInToolCall(map[string]interface{}{
		"function": map[string]interface{}{
			"name":      "detect_stale_tasks",
			"arguments": fmt.Sprintf(`{"swarm_id":"%s","stale_after_minutes":1}`, swarm.ID),
		},
	}); err != nil {
		t.Fatal(err)
	} else if !strings.Contains(result, `"reason":"stale"`) {
		t.Fatalf("expected stale detection tool result, got %s", result)
	}
}

func TestProposeConsensusSummarizesSwarmState(t *testing.T) {
	db := newOrchestrationTestDB(t)
	vm := memory.NewVectorMemory(db)
	manager := NewManager(stubExecutor{}, vm)

	swarm, err := manager.InitSwarm(context.Background(), SwarmRequest{
		Objective: "Reach consensus",
	})
	if err != nil {
		t.Fatal(err)
	}
	task, err := manager.CreateTask(context.Background(), TaskRequest{
		Goal: "Consensus task",
	})
	if err != nil {
		t.Fatal(err)
	}

	manager.mu.Lock()
	internalTask := manager.tasks[task.ID]
	internalTask.Status = TaskStatusCompleted
	internalTask.FinalOutput = "Consensus output from completed task."
	manager.swarms[swarm.ID].TaskIDs = append(manager.swarms[swarm.ID].TaskIDs, task.ID)
	manager.mu.Unlock()

	view, err := manager.ProposeConsensus(swarm.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.CompletedOutputs) == 0 {
		t.Fatal("expected completed outputs in consensus view")
	}
	if !strings.Contains(view.Recommendation, "consensus") {
		t.Fatalf("expected consensus recommendation, got %q", view.Recommendation)
	}
}

func TestExecuteBuiltInToolCallSupportsConsensus(t *testing.T) {
	db := newOrchestrationTestDB(t)
	vm := memory.NewVectorMemory(db)
	manager := NewManager(stubExecutor{}, vm)

	swarm, err := manager.InitSwarm(context.Background(), SwarmRequest{
		Objective: "Consensus tool",
	})
	if err != nil {
		t.Fatal(err)
	}

	_, result, err := manager.executeBuiltInToolCall(map[string]interface{}{
		"function": map[string]interface{}{
			"name":      "propose_consensus",
			"arguments": fmt.Sprintf(`{"swarm_id":"%s"}`, swarm.ID),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, `"swarm_id":"`+swarm.ID+`"`) {
		t.Fatalf("expected consensus result for swarm, got %s", result)
	}
}

func TestManagerRefinesExistingTask(t *testing.T) {
	db := newOrchestrationTestDB(t)
	vm := memory.NewVectorMemory(db)
	manager := NewManager(&toolLoopExecutor{}, vm)

	task, err := manager.CreateTask(context.Background(), TaskRequest{
		Goal: "Refactor a workflow",
	})
	if err != nil {
		t.Fatal(err)
	}

	refined, err := manager.RefineTask(context.Background(), task.ID, RefineRequest{
		Feedback: "The first result missed testing and risk review.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if refined.ParentTaskID != task.ID {
		t.Fatalf("expected parent task %s, got %s", task.ID, refined.ParentTaskID)
	}
	if refined.Iteration != 2 {
		t.Fatalf("expected iteration 2, got %d", refined.Iteration)
	}
	if len(refined.Steps) == 0 {
		t.Fatal("expected refinement steps")
	}
}

func TestManagerInitializesAndStartsSwarm(t *testing.T) {
	db := newOrchestrationTestDB(t)
	vm := memory.NewVectorMemory(db)
	manager := NewManagerWithStore(stubExecutor{}, vm, db)

	swarm, err := manager.InitSwarm(context.Background(), SwarmRequest{
		Objective: "Implement and validate a routing feature",
	})
	if err != nil {
		t.Fatal(err)
	}
	if swarm.ID == "" {
		t.Fatal("expected swarm id")
	}
	if len(swarm.AgentIDs) == 0 {
		t.Fatal("expected default agents in swarm")
	}

	task, err := manager.StartSwarm(context.Background(), swarm.ID, "session-swarm")
	if err != nil {
		t.Fatal(err)
	}
	if task.ID == "" {
		t.Fatal("expected task from swarm start")
	}
}

func TestManagerListsTasksBySessionAndForksSession(t *testing.T) {
	db := newOrchestrationTestDB(t)
	vm := memory.NewVectorMemory(db)
	manager := NewManager(stubExecutor{}, vm)

	_, err := manager.CreateTask(context.Background(), TaskRequest{
		Goal:      "Original session work",
		SessionID: "session-a",
	})
	if err != nil {
		t.Fatal(err)
	}

	tasks := manager.ListTasksBySession("session-a")
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}

	forked, err := manager.ForkSessionTask(context.Background(), "session-a", SessionForkRequest{
		SessionID: "session-b",
		Goal:      "Forked branch",
	})
	if err != nil {
		t.Fatal(err)
	}
	if forked.SessionID != "session-b" {
		t.Fatalf("expected forked session-b, got %s", forked.SessionID)
	}
}

func TestManagerResumesSessionTask(t *testing.T) {
	db := newOrchestrationTestDB(t)
	vm := memory.NewVectorMemory(db)
	manager := NewManager(stubExecutor{}, vm)

	_, err := manager.CreateTask(context.Background(), TaskRequest{
		Goal:      "Continue implementation",
		SessionID: "session-r",
	})
	if err != nil {
		t.Fatal(err)
	}

	resumed, err := manager.ResumeSessionTask(context.Background(), "session-r", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.SessionID != "session-r" {
		t.Fatalf("expected resumed session-r, got %s", resumed.SessionID)
	}
	if resumed.Goal != "Continue implementation" {
		t.Fatalf("unexpected resumed goal: %s", resumed.Goal)
	}
}

func TestManagerCancelsTask(t *testing.T) {
	db := newOrchestrationTestDB(t)
	vm := memory.NewVectorMemory(db)
	manager := NewManager(stubExecutor{}, vm)

	task, err := manager.CreateTask(context.Background(), TaskRequest{
		Goal: "Cancel this task",
	})
	if err != nil {
		t.Fatal(err)
	}

	cancelled, err := manager.CancelTask(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Status != TaskStatusCancelled {
		t.Fatalf("expected cancelled status, got %s", cancelled.Status)
	}
}

func TestManagerSwarmStatusIncludesMetrics(t *testing.T) {
	db := newOrchestrationTestDB(t)
	vm := memory.NewVectorMemory(db)
	manager := NewManagerWithStore(stubExecutor{}, vm, db)

	swarm, err := manager.InitSwarm(context.Background(), SwarmRequest{
		Objective: "Status view",
	})
	if err != nil {
		t.Fatal(err)
	}
	task, err := manager.CreateTask(context.Background(), TaskRequest{
		Goal:      "Status task",
		SessionID: "swarm-status",
	})
	if err != nil {
		t.Fatal(err)
	}
	swarm.TaskIDs = append(swarm.TaskIDs, task.ID)

	status, err := manager.SwarmStatus(swarm.ID)
	if err != nil {
		t.Fatal(err)
	}
	if status.Metrics.AgentCount == 0 {
		t.Fatal("expected agent metrics")
	}
}

func TestManagerSavesCustomWorkflowTemplate(t *testing.T) {
	db := newOrchestrationTestDB(t)
	vm := memory.NewVectorMemory(db)
	manager := NewManagerWithStore(stubExecutor{}, vm, db)

	template, err := manager.SaveWorkflowTemplate(WorkflowTemplate{
		ID:          "custom-dev",
		Name:        "Custom Dev",
		Description: "Custom workflow",
		Topology:    "mesh",
		Strategy:    "development",
		AgentTypes:  []string{"mesh-coordinator", "coder"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if template.ID != "custom-dev" {
		t.Fatalf("unexpected template id: %s", template.ID)
	}
	if _, ok := manager.GetWorkflowTemplate("custom-dev"); !ok {
		t.Fatal("expected custom template to be retrievable")
	}
}

func TestManagerDeletesCustomWorkflowTemplate(t *testing.T) {
	db := newOrchestrationTestDB(t)
	vm := memory.NewVectorMemory(db)
	manager := NewManagerWithStore(stubExecutor{}, vm, db)

	if _, err := manager.SaveWorkflowTemplate(WorkflowTemplate{
		ID:       "custom-delete",
		Name:     "Delete Me",
		Topology: "mesh",
		Strategy: "research",
	}); err != nil {
		t.Fatal(err)
	}

	if err := manager.DeleteWorkflowTemplate("custom-delete"); err != nil {
		t.Fatal(err)
	}
	if _, ok := manager.GetWorkflowTemplate("custom-delete"); ok {
		t.Fatal("expected template to be deleted")
	}
}

func TestRunWorkflowTemplateUsesTemplateRoles(t *testing.T) {
	db := newOrchestrationTestDB(t)
	vm := memory.NewVectorMemory(db)
	manager := NewManagerWithStore(stubExecutor{}, vm, db)

	template, err := manager.SaveWorkflowTemplate(WorkflowTemplate{
		ID:          "doc-flow",
		Name:        "Doc Flow",
		Description: "Document-heavy template",
		Topology:    "mesh",
		Strategy:    "documentation",
		AgentTypes:  []string{"mesh-coordinator", "documenter"},
		Roles:       []string{"documenter", "reviewer"},
	})
	if err != nil {
		t.Fatal(err)
	}

	swarm, task, err := manager.RunWorkflowTemplate(context.Background(), template.ID, WorkflowRunRequest{
		Objective: "Write operator docs",
		SessionID: "workflow-docs",
		Execute:   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if swarm.ID == "" || task == nil {
		t.Fatal("expected swarm and task")
	}
	if len(task.Steps) != 2 {
		t.Fatalf("expected 2 template-driven steps, got %d", len(task.Steps))
	}
	if task.Steps[0].Role != "documenter" {
		t.Fatalf("expected first role documenter, got %s", task.Steps[0].Role)
	}
	if task.Steps[1].Role != "reviewer" {
		t.Fatalf("expected second role reviewer, got %s", task.Steps[1].Role)
	}
}

func newOrchestrationTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec(`
		CREATE TABLE memory (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			content TEXT,
			embedding BLOB,
			timestamp DATETIME,
			session_id TEXT,
			role TEXT,
			metadata TEXT
		)
	`)
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec(`
		CREATE TABLE runtime_settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec(`
		CREATE TABLE orchestration_tasks (
			id TEXT PRIMARY KEY,
			parent_task_id TEXT,
			goal TEXT NOT NULL,
			session_id TEXT NOT NULL,
			status TEXT NOT NULL,
			assigned_to TEXT,
			iteration INTEGER NOT NULL DEFAULT 1,
			feedback TEXT,
			created_at DATETIME NOT NULL,
			started_at DATETIME,
			completed_at DATETIME,
			error TEXT,
			final_output TEXT
		)
	`)
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec(`
		CREATE TABLE orchestration_steps (
			task_id TEXT NOT NULL,
			step_id TEXT NOT NULL,
			role TEXT NOT NULL,
			prompt TEXT NOT NULL,
			status TEXT NOT NULL,
			started_at DATETIME,
			completed_at DATETIME,
			output TEXT,
			error TEXT
		)
	`)
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec(`
		CREATE TABLE orchestration_agents (
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL,
			name TEXT NOT NULL,
			description TEXT,
			capabilities TEXT,
			status TEXT NOT NULL,
			swarm_id TEXT,
			created_at DATETIME NOT NULL,
			last_seen_at DATETIME NOT NULL
		)
	`)
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec(`
		CREATE TABLE orchestration_swarms (
			id TEXT PRIMARY KEY,
			objective TEXT NOT NULL,
			topology TEXT NOT NULL,
			strategy TEXT NOT NULL,
			status TEXT NOT NULL,
			max_agents INTEGER NOT NULL,
			agent_ids TEXT,
			task_ids TEXT,
			created_at DATETIME NOT NULL,
			started_at DATETIME,
			finished_at DATETIME
		)
	`)
	if err != nil {
		t.Fatal(err)
	}

	return db
}

// --- Tool Registry Integration Tests ---

func TestToolRegistryDelegation(t *testing.T) {
	db := newOrchestrationTestDB(t)
	vm := memory.NewVectorMemory(db)
	manager := NewManager(stubExecutor{}, vm)

	dir := t.TempDir()
	registry := tools.DefaultRegistry()
	manager.SetToolRegistry(registry)
	manager.SetWorkDir(dir)

	t.Run("bash tool delegation", func(t *testing.T) {
		name, result, err := manager.executeBuiltInToolCall(map[string]interface{}{
			"function": map[string]interface{}{
				"name":      "bash",
				"arguments": `{"command": "echo tool-registry-works"}`,
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		if name != "bash" {
			t.Errorf("expected name 'bash', got %q", name)
		}
		if !strings.Contains(result, "tool-registry-works") {
			t.Errorf("expected 'tool-registry-works' in result, got %q", result)
		}
	})

	t.Run("file_write delegation", func(t *testing.T) {
		name, result, err := manager.executeBuiltInToolCall(map[string]interface{}{
			"function": map[string]interface{}{
				"name":      "file_write",
				"arguments": `{"path": "delegated.txt", "content": "hello from orchestration"}`,
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		if name != "file_write" {
			t.Errorf("expected name 'file_write', got %q", name)
		}
		if result == "" {
			t.Error("expected non-empty result")
		}
	})

	t.Run("file_read delegation", func(t *testing.T) {
		name, result, err := manager.executeBuiltInToolCall(map[string]interface{}{
			"function": map[string]interface{}{
				"name":      "file_read",
				"arguments": `{"path": "delegated.txt"}`,
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		if name != "file_read" {
			t.Errorf("expected name 'file_read', got %q", name)
		}
		if !strings.Contains(result, "hello from orchestration") {
			t.Errorf("expected file content in result, got %q", result)
		}
	})

	t.Run("glob delegation", func(t *testing.T) {
		name, result, err := manager.executeBuiltInToolCall(map[string]interface{}{
			"function": map[string]interface{}{
				"name":      "glob",
				"arguments": `{"pattern": "*.txt"}`,
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		if name != "glob" {
			t.Errorf("expected name 'glob', got %q", name)
		}
		if !strings.Contains(result, "delegated.txt") {
			t.Errorf("expected 'delegated.txt' in glob results, got %q", result)
		}
	})

	t.Run("unknown tool still fails", func(t *testing.T) {
		_, _, err := manager.executeBuiltInToolCall(map[string]interface{}{
			"function": map[string]interface{}{
				"name":      "totally_unknown_tool",
				"arguments": `{}`,
			},
		})
		if err == nil {
			t.Error("expected error for unknown tool")
		}
	})

	t.Run("builtin tools still take precedence", func(t *testing.T) {
		// create_task is a built-in — should NOT delegate to registry
		name, result, err := manager.executeBuiltInToolCall(map[string]interface{}{
			"function": map[string]interface{}{
				"name":      "create_task",
				"arguments": `{"goal": "test precedence", "execute": false}`,
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		if name != "create_task" {
			t.Errorf("expected name 'create_task', got %q", name)
		}
		if !strings.Contains(result, "test precedence") {
			t.Errorf("expected goal in result, got %q", result)
		}
	})
}

func TestToolRegistryNilDoesNotPanic(t *testing.T) {
	db := newOrchestrationTestDB(t)
	vm := memory.NewVectorMemory(db)
	manager := NewManager(stubExecutor{}, vm)
	// Don't set tool registry — should fall through to error

	_, _, err := manager.executeBuiltInToolCall(map[string]interface{}{
		"function": map[string]interface{}{
			"name":      "bash",
			"arguments": `{"command": "echo test"}`,
		},
	})
	if err == nil {
		t.Error("expected error when no registry set and tool not built-in")
	}
}

func TestToolRegistryAccessors(t *testing.T) {
	db := newOrchestrationTestDB(t)
	vm := memory.NewVectorMemory(db)
	manager := NewManager(stubExecutor{}, vm)

	if manager.ToolRegistry() != nil {
		t.Error("expected nil registry initially")
	}

	registry := tools.DefaultRegistry()
	manager.SetToolRegistry(registry)

	if manager.ToolRegistry() == nil {
		t.Error("expected non-nil registry after set")
	}
	if manager.ToolRegistry() != registry {
		t.Error("expected same registry instance")
	}
}

func TestToolDefsIncludedInRunStep(t *testing.T) {
	db := newOrchestrationTestDB(t)
	vm := memory.NewVectorMemory(db)

	// Custom executor that captures the request to verify tool defs
	var capturedTools []map[string]interface{}
	executor := &toolCapturingExecutor{
		onChat: func(req providers.ChatRequest) {
			capturedTools = req.Tools
		},
	}

	manager := NewManagerWithStore(executor, vm, db)
	registry := tools.DefaultRegistry()
	manager.SetToolRegistry(registry)
	manager.SetWorkDir(t.TempDir())

	// Create a task to trigger runStep
	task, err := manager.CreateTask(context.Background(), TaskRequest{
		Goal:    "Test tool defs in runStep",
		Execute: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Wait for task to complete
	for i := 0; i < 50; i++ {
		time.Sleep(50 * time.Millisecond)
		task, err = manager.GetTask(task.ID)
		if err != nil {
			t.Fatal(err)
		}
		if task.Status == TaskStatusCompleted || task.Status == TaskStatusFailed {
			break
		}
	}

	// Verify tool definitions were passed to LLM
	if len(capturedTools) == 0 {
		t.Fatal("no tools were passed to LLM request")
	}

	// Should have built-in orchestration tools + registry tools
	builtInCount := len(builtInOrchestrationTools())
	registryCount := len(registry.OpenAIToolDefinitions())
	expectedCount := builtInCount + registryCount

	if len(capturedTools) != expectedCount {
		t.Errorf("expected %d tools (builtin=%d + registry=%d), got %d",
			expectedCount, builtInCount, registryCount, len(capturedTools))
	}

	// Verify registry tools are present by checking for known tool names
	toolNames := make(map[string]bool)
	for _, td := range capturedTools {
		fn, _ := td["function"].(map[string]interface{})
		if fn != nil {
			name, _ := fn["name"].(string)
			toolNames[name] = true
		}
	}
	for _, expected := range []string{"bash", "file_read", "file_write", "file_edit", "grep", "glob", "git"} {
		if !toolNames[expected] {
			t.Errorf("missing expected tool %q in LLM request", expected)
		}
	}
}

type toolCapturingExecutor struct {
	onChat func(req providers.ChatRequest)
}

func (e *toolCapturingExecutor) ChatCompletion(ctx context.Context, req providers.ChatRequest, sessionID string) (providers.ChatResponse, error) {
	if e.onChat != nil {
		e.onChat(req)
	}
	return providers.ChatResponse{
		ID:    "capture",
		Model: "stub",
		Choices: []providers.Choice{{
			Message: providers.Message{
				Role:    "assistant",
				Content: "I used the tools to complete the task.",
			},
			FinishReason: "stop",
		}},
	}, nil
}
