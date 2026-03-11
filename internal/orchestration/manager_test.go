package orchestration

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/memory"
	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/providers"
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
	steps := BuildPlan("Implement and test a Go orchestration workflow", nil, 0)
	if len(steps) < 4 {
		t.Fatalf("expected at least 4 steps, got %d", len(steps))
	}
	if steps[0].Role != "researcher" {
		t.Fatalf("expected first role researcher, got %s", steps[0].Role)
	}
	if steps[len(steps)-1].Role != "reviewer" {
		t.Fatalf("expected last role reviewer, got %s", steps[len(steps)-1].Role)
	}
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
