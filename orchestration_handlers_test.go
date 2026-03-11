package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/orchestration"
	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/providers"
)

func TestOrchestrationTasksHandlerCreatesTask(t *testing.T) {
	orchestrator = orchestration.NewManager(stubChatExecutor{}, nil)

	body := `{"goal":"Plan and review a refactor","roles":["architect","reviewer"],"execute":false}`
	req := httptest.NewRequest(http.MethodPost, "/v1/orchestration/tasks", strings.NewReader(body))
	rr := httptest.NewRecorder()

	orchestrationTasksHandler(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected status 202, got %d", rr.Code)
	}

	var task orchestration.Task
	if err := json.Unmarshal(rr.Body.Bytes(), &task); err != nil {
		t.Fatal(err)
	}
	if task.ID == "" {
		t.Fatal("expected task id")
	}
	if len(task.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(task.Steps))
	}
}

func TestOrchestrationTaskRefineHandlerCreatesRefinementTask(t *testing.T) {
	orchestrator = orchestration.NewManager(stubChatExecutor{}, nil)
	parent, err := orchestrator.CreateTask(context.Background(), orchestration.TaskRequest{
		Goal: "Improve a router implementation",
	})
	if err != nil {
		t.Fatal(err)
	}

	body := `{"feedback":"Add a stronger review and testing pass","execute":false}`
	req := httptest.NewRequest(http.MethodPost, "/v1/orchestration/tasks/"+parent.ID+"/refine", strings.NewReader(body))
	req = muxSetVars(req, map[string]string{"task_id": parent.ID})
	rr := httptest.NewRecorder()

	orchestrationTaskRefineHandler(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected status 202, got %d", rr.Code)
	}

	var task orchestration.Task
	if err := json.Unmarshal(rr.Body.Bytes(), &task); err != nil {
		t.Fatal(err)
	}
	if task.ParentTaskID != parent.ID {
		t.Fatalf("expected parent id %s, got %s", parent.ID, task.ParentTaskID)
	}
}

func TestOrchestrationSwarmsHandlerCreatesSwarm(t *testing.T) {
	orchestrator = orchestration.NewManager(stubChatExecutor{}, nil)

	body := `{"objective":"Build a swarm workflow","topology":"hierarchical","strategy":"specialized","execute":false}`
	req := httptest.NewRequest(http.MethodPost, "/v1/orchestration/swarms", strings.NewReader(body))
	rr := httptest.NewRecorder()

	orchestrationSwarmsHandler(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected status 202, got %d", rr.Code)
	}

	var swarm orchestration.Swarm
	if err := json.Unmarshal(rr.Body.Bytes(), &swarm); err != nil {
		t.Fatal(err)
	}
	if swarm.ID == "" {
		t.Fatal("expected swarm id")
	}
	if len(swarm.AgentIDs) == 0 {
		t.Fatal("expected default agents")
	}
}

func TestOrchestrationAgentsHandlerSpawnsAgent(t *testing.T) {
	orchestrator = orchestration.NewManager(stubChatExecutor{}, nil)

	body := `{"type":"coder","name":"worker-1"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/orchestration/agents", strings.NewReader(body))
	rr := httptest.NewRecorder()

	orchestrationAgentsHandler(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected status 202, got %d", rr.Code)
	}

	var agent orchestration.Agent
	if err := json.Unmarshal(rr.Body.Bytes(), &agent); err != nil {
		t.Fatal(err)
	}
	if agent.Type != "coder" {
		t.Fatalf("expected coder type, got %s", agent.Type)
	}
}

func TestOrchestrationTaskAssignHandlerAssignsAgent(t *testing.T) {
	orchestrator = orchestration.NewManager(stubChatExecutor{}, nil)
	swarm, err := orchestrator.InitSwarm(context.Background(), orchestration.SwarmRequest{
		Objective: "Assign work",
	})
	if err != nil {
		t.Fatal(err)
	}
	task, err := orchestrator.CreateTask(context.Background(), orchestration.TaskRequest{
		Goal: "Implement routing",
	})
	if err != nil {
		t.Fatal(err)
	}

	body := `{"agent_id":"` + swarm.AgentIDs[0] + `"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/orchestration/tasks/"+task.ID+"/assign", strings.NewReader(body))
	req = muxSetVars(req, map[string]string{"task_id": task.ID})
	rr := httptest.NewRecorder()

	orchestrationTaskAssignHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var updated orchestration.Task
	if err := json.Unmarshal(rr.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.AssignedTo != swarm.AgentIDs[0] {
		t.Fatalf("expected assignment to %s, got %s", swarm.AgentIDs[0], updated.AssignedTo)
	}
}

func TestOrchestrationTaskPauseAndResumeHandlers(t *testing.T) {
	orchestrator = orchestration.NewManager(stubChatExecutor{}, nil)
	task, err := orchestrator.CreateTask(context.Background(), orchestration.TaskRequest{
		Goal: "Pause this task",
	})
	if err != nil {
		t.Fatal(err)
	}

	pauseReq := httptest.NewRequest(http.MethodPost, "/v1/orchestration/tasks/"+task.ID+"/pause", nil)
	pauseReq = muxSetVars(pauseReq, map[string]string{"task_id": task.ID})
	pauseRR := httptest.NewRecorder()
	orchestrationTaskPauseHandler(pauseRR, pauseReq)
	if pauseRR.Code != http.StatusOK {
		t.Fatalf("expected pause status 200, got %d", pauseRR.Code)
	}

	resumeReq := httptest.NewRequest(http.MethodPost, "/v1/orchestration/tasks/"+task.ID+"/resume", nil)
	resumeReq = muxSetVars(resumeReq, map[string]string{"task_id": task.ID})
	resumeRR := httptest.NewRecorder()
	orchestrationTaskResumeHandler(resumeRR, resumeReq)
	if resumeRR.Code != http.StatusOK {
		t.Fatalf("expected resume status 200, got %d", resumeRR.Code)
	}

	current, err := orchestrator.GetTask(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != orchestration.TaskStatusRunning {
		t.Fatalf("expected running status after resume, got %s", current.Status)
	}
}

func TestOrchestrationWorkflowsHandlerListsTemplates(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/orchestration/workflows", nil)
	rr := httptest.NewRecorder()

	orchestrationWorkflowsHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var payload struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Count == 0 {
		t.Fatal("expected at least one workflow template")
	}
}

func TestOrchestrationWorkflowRunHandlerCreatesSwarm(t *testing.T) {
	orchestrator = orchestration.NewManager(stubChatExecutor{}, nil)

	req := httptest.NewRequest(http.MethodPost, "/v1/orchestration/workflows/development/run", strings.NewReader(`{"objective":"Ship feature","execute":false}`))
	req = muxSetVars(req, map[string]string{"template_id": "development"})
	rr := httptest.NewRecorder()

	orchestrationWorkflowRunHandler(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected status 202, got %d", rr.Code)
	}

	var payload struct {
		Swarm orchestration.Swarm `json:"swarm"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Swarm.ID == "" {
		t.Fatal("expected workflow to create a swarm")
	}
}

func TestOrchestrationWorkflowHandlerReturnsTemplate(t *testing.T) {
	orchestrator = orchestration.NewManager(stubChatExecutor{}, nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/orchestration/workflows/development", nil)
	req = muxSetVars(req, map[string]string{"template_id": "development"})
	rr := httptest.NewRecorder()

	orchestrationWorkflowHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var template orchestration.WorkflowTemplate
	if err := json.Unmarshal(rr.Body.Bytes(), &template); err != nil {
		t.Fatal(err)
	}
	if template.ID != "development" {
		t.Fatalf("expected development template, got %s", template.ID)
	}
}

func TestOrchestrationWorkflowsHandlerCreatesTemplate(t *testing.T) {
	orchestrator = orchestration.NewManager(stubChatExecutor{}, nil)

	req := httptest.NewRequest(http.MethodPost, "/v1/orchestration/workflows", strings.NewReader(`{"id":"custom-flow","name":"Custom Flow","topology":"mesh","strategy":"research","agent_types":["mesh-coordinator","researcher"]}`))
	rr := httptest.NewRecorder()

	orchestrationWorkflowsHandler(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected status 202, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), `"id":"custom-flow"`) {
		t.Fatalf("expected created template in response: %s", rr.Body.String())
	}
}

func TestOrchestrationWorkflowHandlerUpdatesAndDeletesTemplate(t *testing.T) {
	orchestrator = orchestration.NewManager(stubChatExecutor{}, nil)

	createReq := httptest.NewRequest(http.MethodPost, "/v1/orchestration/workflows", strings.NewReader(`{"id":"custom-edit","name":"Custom Edit","topology":"mesh","strategy":"research"}`))
	createRR := httptest.NewRecorder()
	orchestrationWorkflowsHandler(createRR, createReq)
	if createRR.Code != http.StatusAccepted {
		t.Fatalf("expected create status 202, got %d", createRR.Code)
	}

	updateReq := httptest.NewRequest(http.MethodPut, "/v1/orchestration/workflows/custom-edit", strings.NewReader(`{"name":"Updated Flow","topology":"hierarchical","strategy":"development"}`))
	updateReq = muxSetVars(updateReq, map[string]string{"template_id": "custom-edit"})
	updateRR := httptest.NewRecorder()
	orchestrationWorkflowHandler(updateRR, updateReq)
	if updateRR.Code != http.StatusOK {
		t.Fatalf("expected update status 200, got %d", updateRR.Code)
	}
	if !strings.Contains(updateRR.Body.String(), `"name":"Updated Flow"`) {
		t.Fatalf("expected updated template: %s", updateRR.Body.String())
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/v1/orchestration/workflows/custom-edit", nil)
	deleteReq = muxSetVars(deleteReq, map[string]string{"template_id": "custom-edit"})
	deleteRR := httptest.NewRecorder()
	orchestrationWorkflowHandler(deleteRR, deleteReq)
	if deleteRR.Code != http.StatusNoContent {
		t.Fatalf("expected delete status 204, got %d", deleteRR.Code)
	}
}

func TestOrchestrationSwarmHandlerIncludesMetrics(t *testing.T) {
	orchestrator = orchestration.NewManager(stubChatExecutor{}, nil)
	swarm, err := orchestrator.InitSwarm(context.Background(), orchestration.SwarmRequest{
		Objective: "Metrics view",
	})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/orchestration/swarms/"+swarm.ID+"?include_metrics=true", nil)
	req = muxSetVars(req, map[string]string{"swarm_id": swarm.ID})
	rr := httptest.NewRecorder()

	orchestrationSwarmHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var payload struct {
		Metrics orchestration.SwarmMetrics `json:"metrics"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Metrics.AgentCount == 0 {
		t.Fatal("expected non-zero agent count")
	}
}

func TestOrchestrationAgentHandlerIncludesMetrics(t *testing.T) {
	orchestrator = orchestration.NewManager(stubChatExecutor{}, nil)
	agents := orchestrator.ListAgents()
	if len(agents) == 0 {
		t.Fatal("expected default agents")
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/orchestration/agents/"+agents[0].ID+"?include_metrics=true", nil)
	req = muxSetVars(req, map[string]string{"agent_id": agents[0].ID})
	rr := httptest.NewRecorder()

	orchestrationAgentHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "\"metrics\"") {
		t.Fatalf("expected metrics in response: %s", rr.Body.String())
	}
}

func TestOrchestrationSessionHandlersResumeAndFork(t *testing.T) {
	orchestrator = orchestration.NewManager(stubChatExecutor{}, nil)
	_, err := orchestrator.CreateTask(context.Background(), orchestration.TaskRequest{
		Goal:      "Session work",
		SessionID: "session-z",
	})
	if err != nil {
		t.Fatal(err)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/v1/orchestration/sessions/session-z/tasks", nil)
	listReq = muxSetVars(listReq, map[string]string{"session_id": "session-z"})
	listRR := httptest.NewRecorder()
	orchestrationSessionTasksHandler(listRR, listReq)
	if listRR.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", listRR.Code)
	}

	resumeReq := httptest.NewRequest(http.MethodPost, "/v1/orchestration/sessions/session-z/resume", strings.NewReader(`{}`))
	resumeReq = muxSetVars(resumeReq, map[string]string{"session_id": "session-z"})
	resumeRR := httptest.NewRecorder()
	orchestrationSessionResumeHandler(resumeRR, resumeReq)
	if resumeRR.Code != http.StatusAccepted {
		t.Fatalf("expected status 202, got %d", resumeRR.Code)
	}

	forkReq := httptest.NewRequest(http.MethodPost, "/v1/orchestration/sessions/session-z/fork", strings.NewReader(`{"session_id":"session-z-branch","goal":"Branch task"}`))
	forkReq = muxSetVars(forkReq, map[string]string{"session_id": "session-z"})
	forkRR := httptest.NewRecorder()
	orchestrationSessionForkHandler(forkRR, forkReq)
	if forkRR.Code != http.StatusAccepted {
		t.Fatalf("expected status 202, got %d", forkRR.Code)
	}
}

func TestOrchestrationTasksHandlerFiltersBySessionAndStatus(t *testing.T) {
	orchestrator = orchestration.NewManager(stubChatExecutor{}, nil)
	_, err := orchestrator.CreateTask(context.Background(), orchestration.TaskRequest{
		Goal:      "Queued session task",
		SessionID: "session-filter",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = orchestrator.CreateTask(context.Background(), orchestration.TaskRequest{
		Goal:      "Other session task",
		SessionID: "session-other",
	})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/orchestration/tasks?session_id=session-filter&status=queued", nil)
	rr := httptest.NewRecorder()

	orchestrationTasksHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var payload struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Count != 1 {
		t.Fatalf("expected 1 filtered task, got %d", payload.Count)
	}
}

func TestOrchestrationTaskCancelHandlerCancelsTask(t *testing.T) {
	orchestrator = orchestration.NewManager(stubChatExecutor{}, nil)
	task, err := orchestrator.CreateTask(context.Background(), orchestration.TaskRequest{
		Goal: "Cancel via handler",
	})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/orchestration/tasks/"+task.ID+"/cancel", nil)
	req = muxSetVars(req, map[string]string{"task_id": task.ID})
	rr := httptest.NewRecorder()

	orchestrationTaskCancelHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var cancelled orchestration.Task
	if err := json.Unmarshal(rr.Body.Bytes(), &cancelled); err != nil {
		t.Fatal(err)
	}
	if cancelled.Status != orchestration.TaskStatusCancelled {
		t.Fatalf("expected cancelled status, got %s", cancelled.Status)
	}
}

func TestOrchestrationSwarmScaleHandlerScalesSwarm(t *testing.T) {
	orchestrator = orchestration.NewManager(stubChatExecutor{}, nil)
	swarm, err := orchestrator.InitSwarm(context.Background(), orchestration.SwarmRequest{
		Objective: "Scale work",
		MaxAgents: 3,
	})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/orchestration/swarms/"+swarm.ID+"/scale", strings.NewReader(`{"count":5}`))
	req = muxSetVars(req, map[string]string{"swarm_id": swarm.ID})
	rr := httptest.NewRecorder()

	orchestrationSwarmScaleHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var updated orchestration.Swarm
	if err := json.Unmarshal(rr.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if len(updated.AgentIDs) != 5 {
		t.Fatalf("expected 5 agents, got %d", len(updated.AgentIDs))
	}
}

func TestOrchestrationAgentHealthHandlerReturnsSnapshot(t *testing.T) {
	orchestrator = orchestration.NewManager(stubChatExecutor{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/orchestration/agents/health", nil)
	rr := httptest.NewRecorder()

	orchestrationAgentHealthHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var snapshot orchestration.AgentHealthSnapshot
	if err := json.Unmarshal(rr.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Total == 0 {
		t.Fatal("expected at least one default agent")
	}
}

func TestOrchestrationAgentStatusHandlerReturnsAgentStatus(t *testing.T) {
	orchestrator = orchestration.NewManager(stubChatExecutor{}, nil)
	agent, err := orchestrator.SpawnAgent(context.Background(), orchestration.AgentSpawnRequest{
		Type: "coder",
		Name: "status-agent",
	})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/orchestration/agents/"+agent.ID+"/status", nil)
	req = muxSetVars(req, map[string]string{"agent_id": agent.ID})
	rr := httptest.NewRecorder()

	orchestrationAgentStatusHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var payload struct {
		Agent   orchestration.Agent               `json:"agent"`
		Metrics orchestration.AgentMetrics        `json:"metrics"`
		Health  orchestration.AgentHealthSnapshot `json:"health"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Agent.ID != agent.ID {
		t.Fatalf("expected agent id %s, got %s", agent.ID, payload.Agent.ID)
	}
	if payload.Health.Total == 0 {
		t.Fatal("expected health snapshot data")
	}
}

func TestOrchestrationAgentLogsHandlerReturnsAssignedTaskHistory(t *testing.T) {
	orchestrator = orchestration.NewManager(stubChatExecutor{}, nil)
	swarm, err := orchestrator.InitSwarm(context.Background(), orchestration.SwarmRequest{
		Objective: "Review history",
	})
	if err != nil {
		t.Fatal(err)
	}
	task, err := orchestrator.CreateTask(context.Background(), orchestration.TaskRequest{
		Goal: "Inspect logs",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := orchestrator.AssignTask(task.ID, swarm.AgentIDs[0]); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/orchestration/agents/"+swarm.AgentIDs[0]+"/logs", nil)
	req = muxSetVars(req, map[string]string{"agent_id": swarm.AgentIDs[0]})
	rr := httptest.NewRecorder()

	orchestrationAgentLogsHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var payload struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Count == 0 {
		t.Fatal("expected at least one log entry")
	}
}

func TestOrchestrationSwarmStatusHandlerReturnsSwarmStatus(t *testing.T) {
	orchestrator = orchestration.NewManager(stubChatExecutor{}, nil)
	swarm, err := orchestrator.InitSwarm(context.Background(), orchestration.SwarmRequest{
		Objective: "Status swarm",
		Topology:  "hierarchical",
		Strategy:  "specialized",
	})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/orchestration/swarms/"+swarm.ID+"/status", nil)
	req = muxSetVars(req, map[string]string{"swarm_id": swarm.ID})
	rr := httptest.NewRecorder()

	orchestrationSwarmStatusHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var status orchestration.SwarmStatusView
	if err := json.Unmarshal(rr.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.Swarm.ID != swarm.ID {
		t.Fatalf("expected swarm id %s, got %s", swarm.ID, status.Swarm.ID)
	}
	if len(status.Agents) == 0 {
		t.Fatal("expected swarm agents in status view")
	}
}

func TestOrchestrationSwarmLoadAndStealHandlers(t *testing.T) {
	orchestrator = orchestration.NewManager(stubChatExecutor{}, nil)
	swarm, err := orchestrator.InitSwarm(context.Background(), orchestration.SwarmRequest{
		Objective: "Balance work",
		Topology:  "delivery",
		MaxAgents: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	task, err := orchestrator.CreateTask(context.Background(), orchestration.TaskRequest{
		Goal: "Prepare rollout notes",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := orchestrator.AssignTask(task.ID, swarm.AgentIDs[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := orchestrator.StartSwarm(context.Background(), swarm.ID, "load-session"); err != nil {
		t.Fatal(err)
	}

	queuedTask, err := orchestrator.CreateTask(context.Background(), orchestration.TaskRequest{
		Goal: "Queue for stealing",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := orchestrator.AssignTask(queuedTask.ID, swarm.AgentIDs[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := orchestrator.StealTask(context.Background(), queuedTask.ID, swarm.ID, swarm.AgentIDs[0]); err == nil {
	}

	loadReq := httptest.NewRequest(http.MethodGet, "/v1/orchestration/swarms/"+swarm.ID+"/load", nil)
	loadReq = muxSetVars(loadReq, map[string]string{"swarm_id": swarm.ID})
	loadRR := httptest.NewRecorder()
	orchestrationSwarmLoadHandler(loadRR, loadReq)
	if loadRR.Code != http.StatusOK {
		t.Fatalf("expected load status 200, got %d", loadRR.Code)
	}

	imbalanceReq := httptest.NewRequest(http.MethodGet, "/v1/orchestration/swarms/"+swarm.ID+"/imbalance", nil)
	imbalanceReq = muxSetVars(imbalanceReq, map[string]string{"swarm_id": swarm.ID})
	imbalanceRR := httptest.NewRecorder()
	orchestrationSwarmImbalanceHandler(imbalanceRR, imbalanceReq)
	if imbalanceRR.Code != http.StatusOK {
		t.Fatalf("expected imbalance status 200, got %d", imbalanceRR.Code)
	}

	previewReq := httptest.NewRequest(http.MethodPost, "/v1/orchestration/swarms/"+swarm.ID+"/rebalance/preview", nil)
	previewReq = muxSetVars(previewReq, map[string]string{"swarm_id": swarm.ID})
	previewRR := httptest.NewRecorder()
	orchestrationSwarmRebalancePreviewHandler(previewRR, previewReq)
	if previewRR.Code != http.StatusOK {
		t.Fatalf("expected preview status 200, got %d", previewRR.Code)
	}

	stealableReq := httptest.NewRequest(http.MethodGet, "/v1/orchestration/swarms/"+swarm.ID+"/stealable", nil)
	stealableReq = muxSetVars(stealableReq, map[string]string{"swarm_id": swarm.ID})
	stealableRR := httptest.NewRecorder()
	orchestrationSwarmStealableTasksHandler(stealableRR, stealableReq)
	if stealableRR.Code != http.StatusOK {
		t.Fatalf("expected stealable status 200, got %d", stealableRR.Code)
	}
	if !strings.Contains(stealableRR.Body.String(), `"count":`) {
		t.Fatalf("expected stealable task in response: %s", stealableRR.Body.String())
	}

	stealReq := httptest.NewRequest(http.MethodPost, "/v1/orchestration/tasks/"+queuedTask.ID+"/steal", strings.NewReader(`{"swarm_id":"`+swarm.ID+`","stealer_id":"`+swarm.AgentIDs[1]+`"}`))
	stealReq = muxSetVars(stealReq, map[string]string{"task_id": queuedTask.ID})
	stealRR := httptest.NewRecorder()
	orchestrationTaskStealHandler(stealRR, stealReq)
	if stealRR.Code != http.StatusOK {
		t.Fatalf("expected steal status 200, got %d", stealRR.Code)
	}
	if !strings.Contains(stealRR.Body.String(), `"to_agent_id":"`+swarm.AgentIDs[1]+`"`) {
		t.Fatalf("expected steal result body, got %s", stealRR.Body.String())
	}
}

func TestOrchestrationExecutionAndSwarmPauseResumeHandlers(t *testing.T) {
	orchestrator = orchestration.NewManager(stubChatExecutor{}, nil)
	swarm, _, err := orchestrator.RunWorkflowTemplate(context.Background(), "development", orchestration.WorkflowRunRequest{
		Objective: "Handler workflow",
		Execute:   false,
	})
	if err != nil {
		t.Fatal(err)
	}

	stateReq := httptest.NewRequest(http.MethodGet, "/v1/orchestration/executions/"+swarm.ID+"/state", nil)
	stateReq = muxSetVars(stateReq, map[string]string{"workflow_id": swarm.ID})
	stateRR := httptest.NewRecorder()
	orchestrationExecutionStateHandler(stateRR, stateReq)
	if stateRR.Code != http.StatusOK {
		t.Fatalf("expected execution state status 200, got %d", stateRR.Code)
	}

	metricsReq := httptest.NewRequest(http.MethodGet, "/v1/orchestration/executions/"+swarm.ID+"/metrics", nil)
	metricsReq = muxSetVars(metricsReq, map[string]string{"workflow_id": swarm.ID})
	metricsRR := httptest.NewRecorder()
	orchestrationExecutionMetricsHandler(metricsRR, metricsReq)
	if metricsRR.Code != http.StatusOK {
		t.Fatalf("expected execution metrics status 200, got %d", metricsRR.Code)
	}

	debugReq := httptest.NewRequest(http.MethodGet, "/v1/orchestration/executions/"+swarm.ID+"/debug", nil)
	debugReq = muxSetVars(debugReq, map[string]string{"workflow_id": swarm.ID})
	debugRR := httptest.NewRecorder()
	orchestrationExecutionDebugHandler(debugRR, debugReq)
	if debugRR.Code != http.StatusOK {
		t.Fatalf("expected execution debug status 200, got %d", debugRR.Code)
	}

	pauseReq := httptest.NewRequest(http.MethodPost, "/v1/orchestration/swarms/"+swarm.ID+"/pause", nil)
	pauseReq = muxSetVars(pauseReq, map[string]string{"swarm_id": swarm.ID})
	pauseRR := httptest.NewRecorder()
	orchestrationSwarmPauseHandler(pauseRR, pauseReq)
	if pauseRR.Code != http.StatusOK {
		t.Fatalf("expected swarm pause status 200, got %d", pauseRR.Code)
	}

	resumeReq := httptest.NewRequest(http.MethodPost, "/v1/orchestration/swarms/"+swarm.ID+"/resume", nil)
	resumeReq = muxSetVars(resumeReq, map[string]string{"swarm_id": swarm.ID})
	resumeRR := httptest.NewRecorder()
	orchestrationSwarmResumeHandler(resumeRR, resumeReq)
	if resumeRR.Code != http.StatusOK {
		t.Fatalf("expected swarm resume status 200, got %d", resumeRR.Code)
	}
}

func TestOrchestrationTaskContestHandlers(t *testing.T) {
	orchestrator = orchestration.NewManager(stubChatExecutor{}, nil)
	swarm, err := orchestrator.InitSwarm(context.Background(), orchestration.SwarmRequest{
		Objective: "Contest handler",
	})
	if err != nil {
		t.Fatal(err)
	}
	task, err := orchestrator.CreateTask(context.Background(), orchestration.TaskRequest{
		Goal: "Steal and contest",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := orchestrator.AssignTask(task.ID, swarm.AgentIDs[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := orchestrator.StealTask(context.Background(), task.ID, swarm.ID, swarm.AgentIDs[1]); err != nil {
		t.Fatal(err)
	}

	contestReq := httptest.NewRequest(http.MethodPost, "/v1/orchestration/tasks/"+task.ID+"/contest", strings.NewReader(`{"original_agent_id":"`+swarm.AgentIDs[0]+`","reason":"want it back"}`))
	contestReq = muxSetVars(contestReq, map[string]string{"task_id": task.ID})
	contestRR := httptest.NewRecorder()
	orchestrationTaskContestHandler(contestRR, contestReq)
	if contestRR.Code != http.StatusOK {
		t.Fatalf("expected contest status 200, got %d", contestRR.Code)
	}

	resolveReq := httptest.NewRequest(http.MethodPost, "/v1/orchestration/tasks/"+task.ID+"/contest/resolve", strings.NewReader(`{"winner_agent_id":"`+swarm.AgentIDs[0]+`","reason":"approved"}`))
	resolveReq = muxSetVars(resolveReq, map[string]string{"task_id": task.ID})
	resolveRR := httptest.NewRecorder()
	orchestrationTaskContestResolveHandler(resolveRR, resolveReq)
	if resolveRR.Code != http.StatusOK {
		t.Fatalf("expected resolve status 200, got %d", resolveRR.Code)
	}
	if !strings.Contains(resolveRR.Body.String(), `"winner_agent_id":"`+swarm.AgentIDs[0]+`"`) {
		t.Fatalf("expected resolution body, got %s", resolveRR.Body.String())
	}
}

type stubChatExecutor struct{}

func (stubChatExecutor) ChatCompletion(ctx context.Context, req providers.ChatRequest, sessionID string) (providers.ChatResponse, error) {
	return providers.ChatResponse{}, nil
}
