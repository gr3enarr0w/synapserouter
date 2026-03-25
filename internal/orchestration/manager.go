package orchestration

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/mcp"
	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/memory"
	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/providers"
	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/tools"
)

type ChatExecutor interface {
	ChatCompletion(ctx context.Context, req providers.ChatRequest, sessionID string) (providers.ChatResponse, error)
}

type Manager struct {
	executor     ChatExecutor
	memory       *memory.VectorMemory
	mcpClient    *mcp.MCPClient
	toolRegistry *tools.Registry
	workDir      string
	roles        []Role
	skills       []Skill
	db           *sql.DB
	dag          *DAGScheduler

	mu                      sync.RWMutex
	counter                 int
	tasks                   map[string]*Task
	agents                  map[string]*Agent
	swarms                  map[string]*Swarm
	customWorkflowTemplates []WorkflowTemplate
	taskContests            map[string]*TaskStealContest

	listeners map[string][]chan AutopilotEvent
}

func NewManager(executor ChatExecutor, vm *memory.VectorMemory) *Manager {
	return NewManagerWithStore(executor, vm, nil)
}

func NewManagerWithStore(executor ChatExecutor, vm *memory.VectorMemory, db *sql.DB) *Manager {
	m := &Manager{
		executor:     executor,
		memory:       vm,
		roles:        DefaultRoles(),
		skills:       DefaultSkills(),
		db:           db,
		dag:          NewDAGScheduler(),
		tasks:        make(map[string]*Task),
		agents:       make(map[string]*Agent),
		swarms:       make(map[string]*Swarm),
		taskContests: make(map[string]*TaskStealContest),
		listeners:    make(map[string][]chan AutopilotEvent),
	}
	m.bootstrapDefaults()
	return m
}

// SetMCPClient attaches an MCP client for automatic tool invocation during skill execution.
func (m *Manager) SetMCPClient(client *mcp.MCPClient) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mcpClient = client
}

// SetToolRegistry attaches a tool registry for agent tool execution during orchestration.
func (m *Manager) SetToolRegistry(registry *tools.Registry) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.toolRegistry = registry
}

// SetWorkDir sets the working directory for tool execution.
func (m *Manager) SetWorkDir(dir string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.workDir = dir
}

// ToolRegistry returns the attached tool registry, or nil.
func (m *Manager) ToolRegistry() *tools.Registry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.toolRegistry
}

// Skills returns a copy of the registered skill list.
func (m *Manager) Skills() []Skill {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Skill, len(m.skills))
	copy(out, m.skills)
	return out
}

// MatchSkillsForGoal returns matched skills for a goal (for dry-run / preview).
func (m *Manager) MatchSkillsForGoal(goal string) *DispatchResult {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return DispatchWithDetails(goal, m.skills)
}

// invokeMCPTools auto-invokes MCP tools bound to a skill chain and returns
// a context string with the results. If no MCP client is set or no tools
// are bound, returns empty string.
func (m *Manager) invokeMCPTools(ctx context.Context, chain []Skill, goal string) string {
	m.mu.RLock()
	client := m.mcpClient
	m.mu.RUnlock()

	if client == nil {
		return ""
	}

	tools := MCPToolsForChain(chain)
	if len(tools) == 0 {
		return ""
	}

	var results []string
	for _, toolName := range tools {
		result, err := client.CallTool(ctx, mcp.ToolCall{
			ToolName:  toolName,
			Arguments: map[string]interface{}{"query": goal},
		})
		if err != nil {
			log.Printf("[dispatch] MCP tool %s failed: %v", toolName, err)
			continue
		}
		if result.Success {
			outputStr := fmt.Sprintf("[MCP:%s] %v", toolName, result.Output)
			results = append(results, outputStr)
		}
	}

	return strings.Join(results, "\n\n")
}

// invokeMCPToolsForTask detects skill-dispatched steps and invokes their MCP tools.
func (m *Manager) invokeMCPToolsForTask(ctx context.Context, task *Task) string {
	// Reconstruct matched skills from step prompts to find MCP tools
	m.mu.RLock()
	skills := m.skills
	m.mu.RUnlock()

	matched := MatchSkills(task.Goal, skills)
	if len(matched) == 0 {
		return ""
	}

	chain := BuildSkillChain(matched)
	return m.invokeMCPTools(ctx, chain, task.Goal)
}

func (m *Manager) bootstrapDefaults() {
	if m.db != nil {
		if err := m.loadPersistedState(); err != nil {
			log.Printf("[orchestration] failed to load persisted state: %v", err)
		}
	}
	if len(m.agents) == 0 {
		for _, agentType := range DefaultSwarmAgentTypes() {
			m.counter++
			agent := NewAgent(fmt.Sprintf("agent-%d", m.counter), agentType, "", "")
			m.agents[agent.ID] = &agent
			m.persistAgent(&agent)
		}
	}
}

func (m *Manager) loadPersistedState() error {
	if err := m.loadPersistedAgents(); err != nil {
		return err
	}
	if err := m.loadPersistedSwarms(); err != nil {
		return err
	}
	if err := m.loadPersistedTasks(); err != nil {
		return err
	}
	return nil
}

func (m *Manager) loadPersistedAgents() error {
	rows, err := m.db.Query(`
		SELECT id, type, name, description, capabilities, status, swarm_id, created_at, last_seen_at
		FROM orchestration_agents
		ORDER BY created_at ASC
	`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	for rows.Next() {
		var agent Agent
		var capabilitiesJSON string
		if err := rows.Scan(&agent.ID, &agent.Type, &agent.Name, &agent.Description, &capabilitiesJSON, &agent.Status, &agent.SwarmID, &agent.CreatedAt, &agent.LastSeenAt); err != nil {
			continue
		}
		_ = json.Unmarshal([]byte(capabilitiesJSON), &agent.Capabilities)
		m.agents[agent.ID] = &agent
		m.bumpCounter(agent.ID)
	}
	return nil
}

func (m *Manager) loadPersistedSwarms() error {
	rows, err := m.db.Query(`
		SELECT id, objective, topology, strategy, status, max_agents, agent_ids, task_ids, created_at, started_at, finished_at
		FROM orchestration_swarms
		ORDER BY created_at ASC
	`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	for rows.Next() {
		var swarm Swarm
		var agentIDsJSON, taskIDsJSON string
		if err := rows.Scan(&swarm.ID, &swarm.Objective, &swarm.Topology, &swarm.Strategy, &swarm.Status, &swarm.MaxAgents, &agentIDsJSON, &taskIDsJSON, &swarm.CreatedAt, &swarm.StartedAt, &swarm.FinishedAt); err != nil {
			continue
		}
		_ = json.Unmarshal([]byte(agentIDsJSON), &swarm.AgentIDs)
		_ = json.Unmarshal([]byte(taskIDsJSON), &swarm.TaskIDs)
		m.swarms[swarm.ID] = &swarm
		m.bumpCounter(swarm.ID)
	}
	return nil
}

func (m *Manager) loadPersistedTasks() error {
	rows, err := m.db.Query(`
		SELECT id, parent_task_id, goal, session_id, status, assigned_to, iteration, feedback, created_at, started_at, completed_at, error, final_output
		FROM orchestration_tasks
		ORDER BY created_at ASC
	`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	for rows.Next() {
		var task Task
		if err := rows.Scan(&task.ID, &task.ParentTaskID, &task.Goal, &task.SessionID, &task.Status, &task.AssignedTo, &task.Iteration, &task.Feedback, &task.CreatedAt, &task.StartedAt, &task.CompletedAt, &task.Error, &task.FinalOutput); err != nil {
			continue
		}
		task.Steps = m.loadPersistedSteps(task.ID)
		m.tasks[task.ID] = &task
		m.bumpCounter(task.ID)
	}
	return nil
}

func (m *Manager) loadPersistedSteps(taskID string) []TaskStep {
	rows, err := m.db.Query(`
		SELECT step_id, role, prompt, status, started_at, completed_at, output, error
		FROM orchestration_steps
		WHERE task_id = ?
		ORDER BY rowid ASC
	`, taskID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	steps := []TaskStep{}
	for rows.Next() {
		var step TaskStep
		if err := rows.Scan(&step.ID, &step.Role, &step.Prompt, &step.Status, &step.StartedAt, &step.CompletedAt, &step.Output, &step.Error); err != nil {
			continue
		}
		steps = append(steps, step)
	}
	return steps
}

func (m *Manager) bumpCounter(id string) {
	var value int
	if _, err := fmt.Sscanf(id, "orch-%d", &value); err == nil && value > m.counter {
		m.counter = value
		return
	}
	if _, err := fmt.Sscanf(id, "agent-%d", &value); err == nil && value > m.counter {
		m.counter = value
		return
	}
	if _, err := fmt.Sscanf(id, "swarm-%d", &value); err == nil && value > m.counter {
		m.counter = value
	}
}

func (m *Manager) Roles() []Role {
	out := make([]Role, len(m.roles))
	copy(out, m.roles)
	return out
}

func (m *Manager) CreateTask(ctx context.Context, req TaskRequest) (*Task, error) {
	goal := strings.TrimSpace(req.Goal)
	if goal == "" {
		return nil, fmt.Errorf("goal is required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.counter++
	taskID := fmt.Sprintf("orch-%d", m.counter)
	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" {
		sessionID = taskID
	}

	task := &Task{
		ID:        taskID,
		Goal:      goal,
		SessionID: sessionID,
		Status:    TaskStatusQueued,
		Iteration: 1,
		CreatedAt: time.Now(),
		Steps:     BuildPlan(goal, req.Roles, req.MaxSteps),
	}
	m.tasks[task.ID] = task
	m.persistTask(task)

	if req.Execute {
		go m.runTask(context.Background(), task.ID)
	}

	copyTask := *task
	copyTask.Steps = append([]TaskStep(nil), task.Steps...)
	return &copyTask, nil
}

func (m *Manager) RefineTask(ctx context.Context, taskID string, req RefineRequest) (*Task, error) {
	feedback := strings.TrimSpace(req.Feedback)
	if feedback == "" {
		return nil, fmt.Errorf("feedback is required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	parent, ok := m.tasks[taskID]
	if !ok {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}

	m.counter++
	childID := fmt.Sprintf("orch-%d", m.counter)
	task := &Task{
		ID:           childID,
		ParentTaskID: parent.ID,
		Goal:         parent.Goal,
		SessionID:    parent.SessionID,
		Status:       TaskStatusQueued,
		Iteration:    parent.Iteration + 1,
		Feedback:     feedback,
		CreatedAt:    time.Now(),
		Steps:        BuildRefinementPlan(parent.Goal, feedback, *parent, len(parent.Steps)),
	}
	m.tasks[task.ID] = task
	m.persistTask(task)

	if req.Execute {
		go m.runTask(context.Background(), task.ID)
	}

	return cloneTask(task), nil
}

func (m *Manager) ResumeSessionTask(ctx context.Context, sessionID, goal string, execute bool) (*Task, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, fmt.Errorf("session id is required")
	}

	resumeGoal := strings.TrimSpace(goal)
	if resumeGoal == "" {
		tasks := m.ListTasksBySession(sessionID)
		if len(tasks) == 0 {
			return nil, fmt.Errorf("session not found: %s", sessionID)
		}
		resumeGoal = tasks[len(tasks)-1].Goal
	}

	return m.CreateTask(ctx, TaskRequest{
		Goal:      resumeGoal,
		SessionID: sessionID,
		Execute:   execute,
	})
}

func (m *Manager) ForkSessionTask(ctx context.Context, sourceSessionID string, req SessionForkRequest) (*Task, error) {
	sourceSessionID = strings.TrimSpace(sourceSessionID)
	if sourceSessionID == "" {
		return nil, fmt.Errorf("source session id is required")
	}

	sourceTasks := m.ListTasksBySession(sourceSessionID)
	if len(sourceTasks) == 0 {
		return nil, fmt.Errorf("session not found: %s", sourceSessionID)
	}

	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" {
		sessionID = sourceSessionID + "-fork"
	}
	goal := strings.TrimSpace(req.Goal)
	if goal == "" {
		goal = sourceTasks[len(sourceTasks)-1].Goal
	}

	return m.CreateTask(ctx, TaskRequest{
		Goal:      goal,
		SessionID: sessionID,
		Execute:   req.Execute,
	})
}

func (m *Manager) GetTask(taskID string) (*Task, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	task, ok := m.tasks[taskID]
	if !ok {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}
	return cloneTask(task), nil
}

func (m *Manager) ListTasks() []*Task {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tasks := make([]*Task, 0, len(m.tasks))
	for _, task := range m.tasks {
		tasks = append(tasks, cloneTask(task))
	}
	return tasks
}

func (m *Manager) ListTasksBySession(sessionID string) []*Task {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	tasks := make([]*Task, 0)
	for _, task := range m.tasks {
		if task.SessionID != sessionID {
			continue
		}
		tasks = append(tasks, cloneTask(task))
	}
	return tasks
}

func (m *Manager) StartTask(taskID string) error {
	m.mu.RLock()
	_, ok := m.tasks[taskID]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("task not found: %s", taskID)
	}
	go m.runTask(context.Background(), taskID)
	return nil
}

func (m *Manager) CancelTask(taskID string) (*Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	task, ok := m.tasks[taskID]
	if !ok {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}
	if task.Status == TaskStatusCompleted || task.Status == TaskStatusFailed {
		return nil, fmt.Errorf("task cannot be cancelled from status %s", task.Status)
	}

	now := time.Now()
	task.Status = TaskStatusCancelled
	task.CompletedAt = &now
	m.persistTask(task)
	m.syncAgentsForTask(task, AgentStatusIdle)
	return cloneTask(task), nil
}

func (m *Manager) PauseTask(taskID string) (*Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	task, ok := m.tasks[taskID]
	if !ok {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}
	if task.Status == TaskStatusCompleted || task.Status == TaskStatusFailed || task.Status == TaskStatusCancelled {
		return nil, fmt.Errorf("task cannot be paused from status %s", task.Status)
	}
	task.Status = TaskStatusPaused
	m.persistTask(task)
	return cloneTask(task), nil
}

func (m *Manager) ResumeTask(taskID string) (*Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	task, ok := m.tasks[taskID]
	if !ok {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}
	if task.Status != TaskStatusPaused {
		return nil, fmt.Errorf("task is not paused: %s", taskID)
	}
	task.Status = TaskStatusRunning
	m.persistTask(task)
	return cloneTask(task), nil
}

func (m *Manager) AssignTask(taskID, agentID string) (*Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	task, ok := m.tasks[taskID]
	if !ok {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}
	agent, ok := m.agents[agentID]
	if !ok {
		return nil, fmt.Errorf("agent not found: %s", agentID)
	}

	task.AssignedTo = agentID
	if task.Status == TaskStatusPending || task.Status == TaskStatusQueued {
		task.Status = TaskStatusAssigned
	}
	agent.Status = AgentStatusBusy
	agent.LastSeenAt = time.Now()
	m.persistAgent(agent)
	m.persistTask(task)
	return cloneTask(task), nil
}

func (m *Manager) ListAgents() []*Agent {
	m.mu.RLock()
	defer m.mu.RUnlock()

	agents := make([]*Agent, 0, len(m.agents))
	for _, agent := range m.agents {
		copyAgent := *agent
		copyAgent.Capabilities = append([]string(nil), agent.Capabilities...)
		agents = append(agents, &copyAgent)
	}
	return agents
}

func (m *Manager) ListAgentsFiltered(status string) []*Agent {
	status = strings.TrimSpace(strings.ToLower(status))
	if status == "" {
		return m.ListAgents()
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	agents := make([]*Agent, 0, len(m.agents))
	for _, agent := range m.agents {
		if strings.ToLower(string(agent.Status)) != status {
			continue
		}
		copyAgent := *agent
		copyAgent.Capabilities = append([]string(nil), agent.Capabilities...)
		agents = append(agents, &copyAgent)
	}
	return agents
}

func (m *Manager) GetAgent(agentID string) (*Agent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	agent, ok := m.agents[agentID]
	if !ok {
		return nil, fmt.Errorf("agent not found: %s", agentID)
	}
	copyAgent := *agent
	copyAgent.Capabilities = append([]string(nil), agent.Capabilities...)
	return &copyAgent, nil
}

func (m *Manager) StopAgent(agentID string) (*Agent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	agent, ok := m.agents[agentID]
	if !ok {
		return nil, fmt.Errorf("agent not found: %s", agentID)
	}
	agent.Status = AgentStatusStopped
	agent.LastSeenAt = time.Now()
	m.persistAgent(agent)
	return cloneAgent(agent), nil
}

func (m *Manager) AgentMetrics(agentID string) (*AgentMetrics, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	agent, ok := m.agents[agentID]
	if !ok {
		return nil, fmt.Errorf("agent not found: %s", agentID)
	}

	metrics := &AgentMetrics{
		AgentID: string(agent.ID),
		Status:  string(agent.Status),
	}
	for _, swarm := range m.swarms {
		for _, swarmAgentID := range swarm.AgentIDs {
			if swarmAgentID == agentID {
				metrics.ParticipatingSwarms++
				break
			}
		}
	}
	for _, task := range m.tasks {
		if task.AssignedTo != agentID {
			continue
		}
		metrics.AssignedTaskCount++
		switch task.Status {
		case TaskStatusCompleted:
			metrics.CompletedTaskCount++
		case TaskStatusFailed:
			metrics.FailedTaskCount++
		}
	}
	return metrics, nil
}

func (m *Manager) AgentHealth() AgentHealthSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	snapshot := AgentHealthSnapshot{Total: len(m.agents)}
	for _, agent := range m.agents {
		switch agent.Status {
		case AgentStatusIdle:
			snapshot.Idle++
		case AgentStatusBusy:
			snapshot.Busy++
		case AgentStatusStopped:
			snapshot.Stopped++
		}
	}
	return snapshot
}

func (m *Manager) AgentLogs(agentID string) ([]AutopilotEvent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	agent, ok := m.agents[agentID]
	if !ok {
		return nil, fmt.Errorf("agent not found: %s", agentID)
	}

	logs := make([]AutopilotEvent, 0, 8)
	for _, task := range m.tasks {
		if task.AssignedTo != agentID {
			continue
		}
		logs = append(logs, AutopilotEvent{
			Type:    "agent_task",
			TaskID:  task.ID,
			Status:  string(task.Status),
			Summary: task.Goal,
			Metadata: map[string]interface{}{
				"agent_id":   agentID,
				"agent_type": agent.Type,
				"iteration":  task.Iteration,
			},
		})
	}
	if len(logs) == 0 {
		logs = append(logs, AutopilotEvent{
			Type:    "agent_status",
			Status:  string(agent.Status),
			Summary: "no task history available",
			Metadata: map[string]interface{}{
				"agent_id":   agentID,
				"agent_type": agent.Type,
			},
		})
	}
	return logs, nil
}

func (m *Manager) SpawnAgent(ctx context.Context, req AgentSpawnRequest) (*Agent, error) {
	agentType := strings.TrimSpace(strings.ToLower(req.Type))
	if agentType == "" {
		return nil, fmt.Errorf("type is required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if req.SwarmID != "" {
		if _, ok := m.swarms[req.SwarmID]; !ok {
			return nil, fmt.Errorf("swarm not found: %s", req.SwarmID)
		}
	}

	m.counter++
	agent := NewAgent(fmt.Sprintf("agent-%d", m.counter), agentType, req.Name, req.SwarmID)
	m.agents[agent.ID] = &agent
	if req.SwarmID != "" {
		swarm := m.swarms[req.SwarmID]
		swarm.AgentIDs = append(swarm.AgentIDs, agent.ID)
		m.persistSwarm(swarm)
	}
	m.persistAgent(&agent)
	return cloneAgent(&agent), nil
}

func (m *Manager) ListSwarms() []*Swarm {
	m.mu.RLock()
	defer m.mu.RUnlock()

	swarms := make([]*Swarm, 0, len(m.swarms))
	for _, swarm := range m.swarms {
		swarms = append(swarms, cloneSwarm(swarm))
	}
	return swarms
}

func (m *Manager) GetSwarm(swarmID string) (*Swarm, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	swarm, ok := m.swarms[swarmID]
	if !ok {
		return nil, fmt.Errorf("swarm not found: %s", swarmID)
	}
	return cloneSwarm(swarm), nil
}

func (m *Manager) SwarmStatus(swarmID string) (*SwarmStatusView, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	swarm, ok := m.swarms[swarmID]
	if !ok {
		return nil, fmt.Errorf("swarm not found: %s", swarmID)
	}

	view := &SwarmStatusView{
		Swarm:  cloneSwarm(swarm),
		Agents: make([]*Agent, 0, len(swarm.AgentIDs)),
		Tasks:  make([]*Task, 0, len(swarm.TaskIDs)),
	}

	for _, agentID := range swarm.AgentIDs {
		agent, ok := m.agents[agentID]
		if !ok {
			continue
		}
		view.Agents = append(view.Agents, cloneAgent(agent))
		switch agent.Status {
		case AgentStatusBusy:
			view.Metrics.BusyAgents++
		case AgentStatusIdle:
			view.Metrics.IdleAgents++
		case AgentStatusStopped:
			view.Metrics.StoppedAgents++
		}
	}
	view.Metrics.AgentCount = len(view.Agents)

	for _, taskID := range swarm.TaskIDs {
		task, ok := m.tasks[taskID]
		if !ok {
			continue
		}
		view.Tasks = append(view.Tasks, cloneTask(task))
		switch task.Status {
		case TaskStatusCompleted:
			view.Metrics.CompletedTasks++
		case TaskStatusFailed:
			view.Metrics.FailedTasks++
		case TaskStatusRunning, TaskStatusAssigned, TaskStatusQueued:
			view.Metrics.RunningTasks++
		}
	}
	view.Metrics.TaskCount = len(view.Tasks)
	return view, nil
}

func (m *Manager) SwarmLoad(swarmID string) (*SwarmLoadInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	swarm, ok := m.swarms[swarmID]
	if !ok {
		return nil, fmt.Errorf("swarm not found: %s", swarmID)
	}

	load := &SwarmLoadInfo{
		SwarmID: swarm.ID,
		Agents:  make([]AgentLoadInfo, 0, len(swarm.AgentIDs)),
	}
	totalUtilization := 0.0
	for _, agentID := range swarm.AgentIDs {
		agent, ok := m.agents[agentID]
		if !ok {
			continue
		}
		if agent.Status != AgentStatusStopped {
			load.ActiveAgents++
		}
		info := m.agentLoadInfoLocked(agent, swarm)
		load.Agents = append(load.Agents, info)
		totalUtilization += info.Utilization
	}
	load.TotalAgents = len(load.Agents)
	load.TotalTasks = len(swarm.TaskIDs)
	if load.TotalAgents > 0 {
		load.AvgUtilization = totalUtilization / float64(load.TotalAgents)
	}
	load.BalanceScore = 1
	if load.TotalAgents > 0 {
		var variance float64
		for _, agent := range load.Agents {
			diff := agent.Utilization - load.AvgUtilization
			variance += diff * diff
			if agent.Utilization > load.AvgUtilization*1.5 && agent.Utilization > 0 {
				load.OverloadedAgents = append(load.OverloadedAgents, agent.AgentID)
			}
			if agent.Utilization < load.AvgUtilization*0.5 {
				load.UnderloadedAgents = append(load.UnderloadedAgents, agent.AgentID)
			}
		}
		load.BalanceScore = 1 / (1 + variance)
	}
	return load, nil
}

func (m *Manager) DetectImbalance(swarmID string) (*ImbalanceReport, error) {
	load, err := m.SwarmLoad(swarmID)
	if err != nil {
		return nil, err
	}

	report := &ImbalanceReport{
		SwarmID:      swarmID,
		BalanceScore: load.BalanceScore,
		AvgLoad:      load.AvgUtilization,
	}
	overloadedSet := make(map[string]struct{}, len(load.OverloadedAgents))
	for _, id := range load.OverloadedAgents {
		overloadedSet[id] = struct{}{}
	}
	underloadedSet := make(map[string]struct{}, len(load.UnderloadedAgents))
	for _, id := range load.UnderloadedAgents {
		underloadedSet[id] = struct{}{}
	}
	for _, agent := range load.Agents {
		if _, ok := overloadedSet[agent.AgentID]; ok {
			report.Overloaded = append(report.Overloaded, agent)
		}
		if _, ok := underloadedSet[agent.AgentID]; ok {
			report.Underloaded = append(report.Underloaded, agent)
		}
	}
	report.IsBalanced = len(report.Overloaded) == 0 && len(report.Underloaded) == 0
	if report.IsBalanced {
		report.Recommendations = append(report.Recommendations, "swarm is balanced")
		return report, nil
	}
	if len(report.Overloaded) > 0 {
		report.Recommendations = append(report.Recommendations, fmt.Sprintf("rebalance %d overloaded agents", len(report.Overloaded)))
	}
	if len(report.Underloaded) > 0 {
		report.Recommendations = append(report.Recommendations, fmt.Sprintf("route new work to %d underloaded agents", len(report.Underloaded)))
	}
	if stealable, _ := m.ListStealableTasks(swarmID); len(stealable) > 0 {
		report.Recommendations = append(report.Recommendations, fmt.Sprintf("%d tasks are suitable for work stealing", len(stealable)))
	}
	return report, nil
}

func (m *Manager) PreviewRebalance(swarmID string) (*RebalanceResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	swarm, ok := m.swarms[swarmID]
	if !ok {
		return nil, fmt.Errorf("swarm not found: %s", swarmID)
	}
	load, err := m.SwarmLoad(swarmID)
	if err != nil {
		return nil, err
	}
	preview := &RebalanceResult{
		Preview: make([]RebalanceMove, 0),
		Load:    load,
		Stats: map[string]float64{
			"candidate_tasks": 0,
			"projected_moves": 0,
		},
	}
	for _, taskID := range swarm.TaskIDs {
		task, ok := m.tasks[taskID]
		if !ok || !m.isTaskStealableLocked(task) {
			continue
		}
		targetID := m.selectAgentForTaskLocked(swarm, task)
		if targetID == "" || targetID == task.AssignedTo {
			continue
		}
		preview.Preview = append(preview.Preview, RebalanceMove{
			TaskID: task.ID,
			From:   task.AssignedTo,
			To:     targetID,
			Reason: "load rebalance preview",
		})
	}
	preview.Stats["candidate_tasks"] = float64(len(preview.Preview))
	preview.Stats["projected_moves"] = float64(len(preview.Preview))
	return preview, nil
}

func (m *Manager) ListStealableTasks(swarmID string) ([]StealableTaskInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	swarm, ok := m.swarms[swarmID]
	if !ok {
		return nil, fmt.Errorf("swarm not found: %s", swarmID)
	}

	stealable := make([]StealableTaskInfo, 0)
	for _, taskID := range swarm.TaskIDs {
		task, ok := m.tasks[taskID]
		if !ok || !m.isTaskStealableLocked(task) {
			continue
		}
		stealable = append(stealable, StealableTaskInfo{
			TaskID:      task.ID,
			FromAgentID: task.AssignedTo,
			Status:      task.Status,
			Progress:    taskProgress(task),
			Reason:      taskStealableReason(task),
		})
	}
	return stealable, nil
}

func (m *Manager) StealTask(ctx context.Context, taskID, swarmID, stealerID string) (*TaskStealResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	swarm, ok := m.swarms[swarmID]
	if !ok {
		return nil, fmt.Errorf("swarm not found: %s", swarmID)
	}
	task, ok := m.tasks[taskID]
	if !ok {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}
	if !m.isTaskStealableLocked(task) {
		return nil, fmt.Errorf("task is not stealable: %s", taskID)
	}
	if stealerID == "" {
		stealerID = m.selectAgentForTaskLocked(swarm, task)
	}
	agent, ok := m.agents[stealerID]
	if !ok {
		return nil, fmt.Errorf("agent not found: %s", stealerID)
	}
	if agent.Status == AgentStatusStopped {
		return nil, fmt.Errorf("agent is stopped: %s", stealerID)
	}

	fromAgentID := task.AssignedTo
	task.AssignedTo = stealerID
	if task.Status == TaskStatusQueued || task.Status == TaskStatusPending {
		task.Status = TaskStatusAssigned
	}
	agent.Status = AgentStatusBusy
	agent.LastSeenAt = time.Now()
	m.persistAgent(agent)
	if fromAgentID != "" && fromAgentID != stealerID {
		if fromAgent, ok := m.agents[fromAgentID]; ok {
			fromAgent.LastSeenAt = time.Now()
			if m.agentActiveTaskCountLocked(fromAgentID) <= 1 {
				fromAgent.Status = AgentStatusIdle
			}
			m.persistAgent(fromAgent)
		}
	}
	m.persistTask(task)
	return &TaskStealResult{
		TaskID:      task.ID,
		FromAgentID: fromAgentID,
		ToAgentID:   stealerID,
		Reason:      taskStealableReason(task),
	}, nil
}

func (m *Manager) ContestTaskSteal(taskID, originalAgentID, reason string) (*TaskStealContest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	task, ok := m.tasks[taskID]
	if !ok {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}
	if task.AssignedTo == "" {
		return nil, fmt.Errorf("task is unassigned: %s", taskID)
	}
	if strings.TrimSpace(originalAgentID) == "" {
		originalAgentID = task.AssignedTo
	}
	contest := &TaskStealContest{
		TaskID:          taskID,
		OriginalAgentID: originalAgentID,
		CurrentAgentID:  task.AssignedTo,
		Reason:          firstNonEmptyString(strings.TrimSpace(reason), "contest requested"),
		CreatedAt:       time.Now(),
	}
	m.taskContests[taskID] = contest
	return cloneContest(contest), nil
}

func (m *Manager) ResolveTaskStealContest(taskID, winnerAgentID, reason string) (*TaskStealContest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	contest, ok := m.taskContests[taskID]
	if !ok {
		return nil, fmt.Errorf("task contest not found: %s", taskID)
	}
	task, ok := m.tasks[taskID]
	if !ok {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}
	if _, ok := m.agents[winnerAgentID]; !ok {
		return nil, fmt.Errorf("agent not found: %s", winnerAgentID)
	}
	task.AssignedTo = winnerAgentID
	if task.Status == TaskStatusQueued || task.Status == TaskStatusPending {
		task.Status = TaskStatusAssigned
	}
	m.persistTask(task)
	contest.WinnerAgentID = winnerAgentID
	contest.Resolution = firstNonEmptyString(strings.TrimSpace(reason), "contest resolved")
	resolvedAt := time.Now()
	contest.ResolvedAt = &resolvedAt
	return cloneContest(contest), nil
}

func (m *Manager) GetTaskContest(taskID string) (*TaskStealContest, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	contest, ok := m.taskContests[taskID]
	if !ok {
		return nil, false
	}
	return cloneContest(contest), true
}

func (m *Manager) DetectStaleTasks(swarmID string, staleAfter time.Duration) ([]StealableTaskInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	swarm, ok := m.swarms[swarmID]
	if !ok {
		return nil, fmt.Errorf("swarm not found: %s", swarmID)
	}
	if staleAfter <= 0 {
		staleAfter = 30 * time.Minute
	}
	now := time.Now()
	stealable := make([]StealableTaskInfo, 0)
	for _, taskID := range swarm.TaskIDs {
		task, ok := m.tasks[taskID]
		if !ok || task.AssignedTo == "" {
			continue
		}
		lastActivity := task.CreatedAt
		if task.StartedAt != nil {
			lastActivity = *task.StartedAt
		}
		if task.CompletedAt != nil {
			lastActivity = *task.CompletedAt
		}
		if now.Sub(lastActivity) < staleAfter {
			continue
		}
		stealable = append(stealable, StealableTaskInfo{
			TaskID:      task.ID,
			FromAgentID: task.AssignedTo,
			Status:      task.Status,
			Progress:    taskProgress(task),
			Reason:      "stale",
		})
	}
	return stealable, nil
}

func (m *Manager) ProposeConsensus(swarmID string) (*SwarmConsensusView, error) {
	status, err := m.SwarmStatus(swarmID)
	if err != nil {
		return nil, err
	}

	view := &SwarmConsensusView{
		SwarmID:   status.Swarm.ID,
		Objective: status.Swarm.Objective,
		Status:    string(status.Swarm.Status),
	}

	for _, task := range status.Tasks {
		switch task.Status {
		case TaskStatusCompleted:
			if strings.TrimSpace(task.FinalOutput) != "" {
				view.CompletedOutputs = append(view.CompletedOutputs, trimSummary(task.FinalOutput))
			} else {
				for _, step := range task.Steps {
					if strings.TrimSpace(step.Output) != "" {
						view.CompletedOutputs = append(view.CompletedOutputs, trimSummary(step.Output))
					}
				}
			}
		case TaskStatusFailed:
			view.Risks = append(view.Risks, firstNonEmptyString(task.Error, "task failed: "+task.ID))
		case TaskStatusQueued, TaskStatusAssigned, TaskStatusRunning:
			view.PendingTasks = append(view.PendingTasks, firstNonEmptyString(task.Goal, task.ID))
		}
	}

	if status.Metrics.FailedTasks > 0 {
		view.Risks = append(view.Risks, fmt.Sprintf("%d tasks failed", status.Metrics.FailedTasks))
	}
	if status.Metrics.RunningTasks > 0 {
		view.Risks = append(view.Risks, fmt.Sprintf("%d tasks still running or queued", status.Metrics.RunningTasks))
	}
	if len(view.Risks) == 0 && len(view.PendingTasks) == 0 && len(view.CompletedOutputs) > 0 {
		view.Recommendation = "consensus reached: completed outputs are ready for review or handoff"
	} else if len(view.Risks) > 0 {
		view.Recommendation = "consensus blocked: resolve risks before proceeding"
	} else {
		view.Recommendation = "consensus pending: more execution is required"
	}
	return view, nil
}

func (m *Manager) WorkflowState(workflowID string) (*WorkflowStateView, error) {
	status, err := m.SwarmStatus(workflowID)
	if err != nil {
		return nil, err
	}
	view := &WorkflowStateView{
		ID:          status.Swarm.ID,
		Name:        status.Swarm.Objective,
		Status:      string(status.Swarm.Status),
		StartedAt:   status.Swarm.StartedAt,
		CompletedAt: status.Swarm.FinishedAt,
		SwarmID:     status.Swarm.ID,
		Topology:    status.Swarm.Topology,
		Strategy:    status.Swarm.Strategy,
	}
	for _, task := range status.Tasks {
		view.Tasks = append(view.Tasks, task.ID)
		if task.Status == TaskStatusCompleted {
			view.CompletedTasks = append(view.CompletedTasks, task.ID)
		}
		if task.Status == TaskStatusRunning || task.Status == TaskStatusPaused {
			view.CurrentTask = task.ID
		}
		if task.Status == TaskStatusPaused {
			view.Paused = true
			view.Status = "paused"
		}
	}
	return view, nil
}

func (m *Manager) WorkflowMetrics(workflowID string) (*WorkflowMetrics, error) {
	status, err := m.SwarmStatus(workflowID)
	if err != nil {
		return nil, err
	}
	metrics := &WorkflowMetrics{
		TasksTotal:     len(status.Tasks),
		TasksCompleted: status.Metrics.CompletedTasks,
	}
	var totalDuration int64
	var durationCount int64
	for _, task := range status.Tasks {
		if task.StartedAt != nil && task.CompletedAt != nil {
			totalDuration += task.CompletedAt.Sub(*task.StartedAt).Milliseconds()
			durationCount++
		}
	}
	if durationCount > 0 {
		metrics.TotalDurationMS = totalDuration
		metrics.AverageTaskDuration = totalDuration / durationCount
	}
	if metrics.TasksTotal > 0 {
		metrics.SuccessRate = float64(metrics.TasksCompleted) / float64(metrics.TasksTotal)
	}
	return metrics, nil
}

func (m *Manager) WorkflowDebugInfo(workflowID string) (*WorkflowDebugInfo, error) {
	status, err := m.SwarmStatus(workflowID)
	if err != nil {
		return nil, err
	}
	debug := &WorkflowDebugInfo{
		ExecutionTrace:  make([]WorkflowDebugTrace, 0, len(status.Tasks)),
		TaskTimings:     make(map[string]int64, len(status.Tasks)),
		EventLog:        make([]AutopilotEvent, 0, len(status.Tasks)),
		MemorySnapshots: make([]map[string]interface{}, 0),
	}
	for _, task := range status.Tasks {
		ts := task.CreatedAt
		if task.StartedAt != nil {
			ts = *task.StartedAt
		}
		debug.ExecutionTrace = append(debug.ExecutionTrace, WorkflowDebugTrace{
			TaskID:    task.ID,
			Timestamp: ts,
			Action:    "execute",
		})
		if task.StartedAt != nil && task.CompletedAt != nil {
			debug.TaskTimings[task.ID] = task.CompletedAt.Sub(*task.StartedAt).Milliseconds()
		}
		debug.EventLog = append(debug.EventLog, AutopilotEvent{
			Type:    "workflow_task",
			TaskID:  task.ID,
			Status:  string(task.Status),
			Summary: task.Goal,
			Metadata: map[string]interface{}{
				"assigned_to": task.AssignedTo,
				"iteration":   task.Iteration,
			},
		})
		if m.memory != nil {
			debug.MemorySnapshots = append(debug.MemorySnapshots, map[string]interface{}{
				"task_id":    task.ID,
				"session_id": task.SessionID,
			})
		}
	}
	return debug, nil
}

func (m *Manager) PauseSwarm(swarmID string) (*Swarm, error) {
	m.mu.RLock()
	swarm, ok := m.swarms[swarmID]
	if !ok {
		m.mu.RUnlock()
		return nil, fmt.Errorf("swarm not found: %s", swarmID)
	}
	taskIDs := append([]string(nil), swarm.TaskIDs...)
	m.mu.RUnlock()
	for _, taskID := range taskIDs {
		_, _ = m.PauseTask(taskID)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	swarm = m.swarms[swarmID]
	swarm.Status = SwarmStatusReady
	m.persistSwarm(swarm)
	return cloneSwarm(swarm), nil
}

func (m *Manager) ResumeSwarm(swarmID string) (*Swarm, error) {
	m.mu.RLock()
	swarm, ok := m.swarms[swarmID]
	if !ok {
		m.mu.RUnlock()
		return nil, fmt.Errorf("swarm not found: %s", swarmID)
	}
	taskIDs := append([]string(nil), swarm.TaskIDs...)
	m.mu.RUnlock()
	for _, taskID := range taskIDs {
		_, _ = m.ResumeTask(taskID)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	swarm = m.swarms[swarmID]
	swarm.Status = SwarmStatusRunning
	now := time.Now()
	swarm.StartedAt = &now
	m.persistSwarm(swarm)
	return cloneSwarm(swarm), nil
}

func (m *Manager) StopSwarm(swarmID string) (*Swarm, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	swarm, ok := m.swarms[swarmID]
	if !ok {
		return nil, fmt.Errorf("swarm not found: %s", swarmID)
	}

	swarm.Status = SwarmStatusStopped
	finishedAt := time.Now()
	swarm.FinishedAt = &finishedAt
	for _, agentID := range swarm.AgentIDs {
		if agent, ok := m.agents[agentID]; ok {
			agent.Status = AgentStatusStopped
			agent.LastSeenAt = finishedAt
			m.persistAgent(agent)
		}
	}
	m.persistSwarm(swarm)
	return cloneSwarm(swarm), nil
}

func (m *Manager) InitSwarm(ctx context.Context, req SwarmRequest) (*Swarm, error) {
	objective := strings.TrimSpace(req.Objective)
	if objective == "" {
		return nil, fmt.Errorf("objective is required")
	}

	topology := strings.TrimSpace(req.Topology)
	if topology == "" {
		topology = DefaultSwarmTopology()
	}
	strategy := strings.TrimSpace(req.Strategy)
	if strategy == "" {
		strategy = DefaultSwarmStrategy()
	}
	maxAgents := req.MaxAgents
	if maxAgents <= 0 {
		maxAgents = len(DefaultSwarmAgentTypes())
	}

	m.mu.Lock()
	m.counter++
	swarmID := fmt.Sprintf("swarm-%d", m.counter)
	swarm := &Swarm{
		ID:        swarmID,
		Objective: objective,
		Topology:  topology,
		Strategy:  strategy,
		Status:    SwarmStatusReady,
		MaxAgents: maxAgents,
		CreatedAt: time.Now(),
	}
	m.swarms[swarm.ID] = swarm
	m.persistSwarm(swarm)

	agentTypes := req.AgentTypes
	if len(agentTypes) == 0 {
		agentTypes = AgentTypesForTopology(topology)
	}
	if len(agentTypes) > maxAgents {
		agentTypes = agentTypes[:maxAgents]
	}
	for _, agentType := range agentTypes {
		m.counter++
		agent := NewAgent(fmt.Sprintf("agent-%d", m.counter), agentType, "", swarm.ID)
		m.agents[agent.ID] = &agent
		swarm.AgentIDs = append(swarm.AgentIDs, agent.ID)
		m.persistAgent(&agent)
	}
	m.persistSwarm(swarm)
	m.mu.Unlock()

	if req.Execute {
		if _, err := m.StartSwarm(ctx, swarm.ID, req.SessionID); err != nil {
			return nil, err
		}
	}

	return cloneSwarm(swarm), nil
}

func (m *Manager) StartSwarm(ctx context.Context, swarmID, sessionID string) (*Task, error) {
	m.mu.Lock()
	swarm, ok := m.swarms[swarmID]
	if !ok {
		m.mu.Unlock()
		return nil, fmt.Errorf("swarm not found: %s", swarmID)
	}
	now := time.Now()
	swarm.Status = SwarmStatusRunning
	swarm.StartedAt = &now

	roles := make([]string, 0, len(swarm.AgentIDs))
	for _, agentID := range swarm.AgentIDs {
		agent, ok := m.agents[agentID]
		if !ok {
			continue
		}
		agent.Status = AgentStatusBusy
		agent.LastSeenAt = now
		m.persistAgent(agent)
		role := mapAgentTypeToRole(agent.Type)
		if role != "" {
			roles = append(roles, role)
		}
	}
	m.persistSwarm(swarm)
	m.mu.Unlock()

	task, err := m.CreateTask(ctx, TaskRequest{
		Goal:      swarm.Objective,
		SessionID: sessionID,
		Roles:     dedupe(roles),
		Execute:   true,
		MaxSteps:  swarm.MaxAgents,
	})
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	swarm.TaskIDs = append(swarm.TaskIDs, task.ID)
	if assignedAgentID := m.selectAgentForTaskLocked(swarm, task); assignedAgentID != "" {
		task.AssignedTo = assignedAgentID
		task.Status = TaskStatusAssigned
		if agent, ok := m.agents[assignedAgentID]; ok {
			agent.Status = AgentStatusBusy
			agent.LastSeenAt = time.Now()
			m.persistAgent(agent)
		}
		m.persistTask(task)
	}
	m.persistSwarm(swarm)
	m.mu.Unlock()

	return task, nil
}

func (m *Manager) ScaleSwarm(ctx context.Context, swarmID string, count int) (*Swarm, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	swarm, ok := m.swarms[swarmID]
	if !ok {
		return nil, fmt.Errorf("swarm not found: %s", swarmID)
	}
	if count <= 0 {
		return nil, fmt.Errorf("count must be greater than zero")
	}

	current := len(swarm.AgentIDs)
	if count == current {
		return cloneSwarm(swarm), nil
	}

	if count < current {
		removed := swarm.AgentIDs[count:]
		swarm.AgentIDs = append([]string(nil), swarm.AgentIDs[:count]...)
		for _, agentID := range removed {
			if agent, ok := m.agents[agentID]; ok {
				agent.Status = AgentStatusStopped
				agent.LastSeenAt = time.Now()
				m.persistAgent(agent)
			}
		}
		swarm.MaxAgents = count
		m.persistSwarm(swarm)
		return cloneSwarm(swarm), nil
	}

	additionalTypes := AgentTypesForTopology(swarm.Topology)
	for len(swarm.AgentIDs) < count {
		agentType := "coder"
		if len(additionalTypes) > 0 {
			agentType = additionalTypes[len(swarm.AgentIDs)%len(additionalTypes)]
		}
		m.counter++
		agent := NewAgent(fmt.Sprintf("agent-%d", m.counter), agentType, "", swarm.ID)
		m.agents[agent.ID] = &agent
		swarm.AgentIDs = append(swarm.AgentIDs, agent.ID)
		m.persistAgent(&agent)
	}
	swarm.MaxAgents = count
	m.persistSwarm(swarm)
	return cloneSwarm(swarm), nil
}

func (m *Manager) CoordinateSwarm(ctx context.Context, swarmID string, agentCount int) (*Swarm, error) {
	if agentCount > 0 {
		return m.ScaleSwarm(ctx, swarmID, agentCount)
	}
	return m.GetSwarm(swarmID)
}

func (m *Manager) RebalanceSwarm(ctx context.Context, swarmID string) ([]*Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	swarm, ok := m.swarms[swarmID]
	if !ok {
		return nil, fmt.Errorf("swarm not found: %s", swarmID)
	}

	updated := make([]*Task, 0)
	for _, taskID := range swarm.TaskIDs {
		task, ok := m.tasks[taskID]
		if !ok {
			continue
		}
		if task.Status == TaskStatusCompleted || task.Status == TaskStatusFailed || task.Status == TaskStatusCancelled {
			continue
		}
		agentID := m.selectAgentForTaskLocked(swarm, task)
		if agentID == "" {
			continue
		}
		task.AssignedTo = agentID
		if task.Status == TaskStatusPending || task.Status == TaskStatusQueued {
			task.Status = TaskStatusAssigned
		}
		if agent, ok := m.agents[agentID]; ok && agent.Status != AgentStatusStopped {
			agent.Status = AgentStatusBusy
			agent.LastSeenAt = time.Now()
			m.persistAgent(agent)
		}
		m.persistTask(task)
		updated = append(updated, cloneTask(task))
	}
	m.persistSwarm(swarm)
	return updated, nil
}

func (m *Manager) SubscribeTask(taskID string) (<-chan AutopilotEvent, func(), error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.tasks[taskID]; !ok {
		return nil, nil, fmt.Errorf("task not found: %s", taskID)
	}

	ch := make(chan AutopilotEvent, 32)
	m.listeners[taskID] = append(m.listeners[taskID], ch)
	cancel := func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		listeners := m.listeners[taskID]
		filtered := listeners[:0]
		for _, candidate := range listeners {
			if candidate != ch {
				filtered = append(filtered, candidate)
				continue
			}
			close(candidate)
		}
		if len(filtered) == 0 {
			delete(m.listeners, taskID)
			return
		}
		m.listeners[taskID] = filtered
	}
	return ch, cancel, nil
}

func (m *Manager) runTask(ctx context.Context, taskID string) {
	task, err := m.markTaskRunning(taskID)
	if err != nil {
		return
	}
	started := time.Now()
	plannedTasks := make([]map[string]any, 0, len(task.Steps))
	for idx := range task.Steps {
		plannedTasks = append(plannedTasks, map[string]any{
			"taskId": fmt.Sprintf("%s:%s", task.ID, task.Steps[idx].ID),
			"tool":   task.Steps[idx].Role,
			"step":   idx + 1,
			"status": string(task.Steps[idx].Status),
		})
	}
	m.publish(task.ID, AutopilotEvent{
		Type:     "autopilot_start",
		TaskID:   task.ID,
		MaxSteps: len(task.Steps),
	})
	m.publish(task.ID, AutopilotEvent{
		Type:       "autopilot_plan",
		TaskID:     task.ID,
		MaxSteps:   len(task.Steps),
		TotalSteps: len(task.Steps),
		TotalTasks: len(task.Steps),
		Tasks:      plannedTasks,
	})

	// Auto-invoke MCP tools if the task was skill-dispatched
	var previousOutputs []string
	if mcpContext := m.invokeMCPToolsForTask(ctx, task); mcpContext != "" {
		previousOutputs = append(previousOutputs, mcpContext)
		log.Printf("[dispatch] MCP context injected for task %s (%d bytes)", task.ID, len(mcpContext))
	}

	for idx := range task.Steps {
		for m.isTaskPaused(task.ID) {
			m.publish(task.ID, AutopilotEvent{
				Type:   "autopilot_pause",
				TaskID: task.ID,
				Status: string(TaskStatusPaused),
			})
			time.Sleep(25 * time.Millisecond)
		}
		groupID := fmt.Sprintf("%s:%s", task.ID, task.Steps[idx].ID)
		m.publish(task.ID, AutopilotEvent{
			Type:    "task_group_start",
			TaskID:  task.ID,
			GroupID: groupID,
			Step:    idx + 1,
			Tasks: []map[string]any{
				{
					"taskId": groupID,
					"tool":   task.Steps[idx].Role,
					"status": "queued",
				},
			},
		})
		m.publish(task.ID, AutopilotEvent{
			Type:    "task_group_update",
			TaskID:  task.ID,
			GroupID: groupID,
			Step:    idx + 1,
			Status:  string(StepStatusPending),
			Summary: task.Steps[idx].Prompt,
		})
		if err := m.runStep(ctx, task, idx, previousOutputs); err != nil {
			m.failTask(task.ID, err)
			m.publish(task.ID, AutopilotEvent{
				Type:    "autopilot_error",
				TaskID:  task.ID,
				GroupID: groupID,
				Error:   err.Error(),
			})
			return
		}
		m.publish(task.ID, AutopilotEvent{
			Type:     "task_group_end",
			TaskID:   task.ID,
			GroupID:  groupID,
			Step:     idx + 1,
			Status:   string(StepStatusCompleted),
			Duration: time.Since(started).Milliseconds(),
		})
		previousOutputs = append(previousOutputs, task.Steps[idx].Output)
		m.publish(task.ID, AutopilotEvent{
			Type:       "autopilot_progress",
			TaskID:     task.ID,
			GroupID:    groupID,
			Step:       idx + 1,
			MaxSteps:   len(task.Steps),
			TotalSteps: len(task.Steps),
			TotalTasks: len(task.Steps),
			Status:     string(task.Status),
			Summary:    trimSummary(task.Steps[idx].Output),
			Metadata: map[string]interface{}{
				"role": task.Steps[idx].Role,
			},
		})
	}

	m.completeTask(task.ID, strings.Join(previousOutputs, "\n\n"))
	m.publish(task.ID, AutopilotEvent{
		Type:       "autopilot_end",
		TaskID:     task.ID,
		TotalSteps: len(task.Steps),
		TotalTasks: len(task.Steps),
		Duration:   time.Since(started).Milliseconds(),
	})
	m.finishTaskListeners(task.ID)
}

func (m *Manager) runStep(ctx context.Context, task *Task, index int, previousOutputs []string) error {
	now := time.Now()

	m.mu.Lock()
	task.Steps[index].Status = StepStatusRunning
	task.Steps[index].StartedAt = &now
	m.persistTask(task)
	m.mu.Unlock()
	m.publish(task.ID, AutopilotEvent{
		Type:    "task_update",
		TaskID:  task.ID,
		GroupID: fmt.Sprintf("%s:%s", task.ID, task.Steps[index].ID),
		Status:  string(StepStatusRunning),
		Summary: task.Steps[index].Prompt,
	})
	m.publish(task.ID, AutopilotEvent{
		Type:    "task_step_start",
		TaskID:  task.ID,
		GroupID: fmt.Sprintf("%s:%s", task.ID, task.Steps[index].ID),
		Step:    index + 1,
		Status:  string(StepStatusRunning),
		Summary: task.Steps[index].Prompt,
		Metadata: map[string]interface{}{
			"role": task.Steps[index].Role,
		},
	})

	var relevant []memory.Message
	if m.memory != nil {
		relevant, _ = m.memory.RetrieveRelevant(task.Goal, task.SessionID, 2000)
	}
	toolDefs := builtInOrchestrationTools()
	if m.toolRegistry != nil {
		toolDefs = append(toolDefs, m.toolRegistry.OpenAIToolDefinitions()...)
	}
	request := providers.ChatRequest{
		Model:     "auto",
		MaxTokens: 1500,
		Messages:  buildMessages(task, task.Steps[index], relevant, previousOutputs),
		Tools:     toolDefs,
	}

	resp, err := m.completeToolCallingStep(ctx, task, index, request)
	if err != nil {
		m.mu.Lock()
		task.Steps[index].Status = StepStatusFailed
		task.Steps[index].Error = err.Error()
		completedAt := time.Now()
		task.Steps[index].CompletedAt = &completedAt
		m.persistTask(task)
		m.mu.Unlock()
		return err
	}

	output := firstChoice(resp)
	if output == "" {
		output = "(empty orchestration response)"
	}
	if m.memory != nil {
		_ = m.memory.Store(output, "assistant", task.SessionID, map[string]interface{}{
			"orchestration_task_id": task.ID,
			"orchestration_role":    task.Steps[index].Role,
		})
	}

	m.mu.Lock()
	task.Steps[index].Status = StepStatusCompleted
	task.Steps[index].Output = output
	completedAt := time.Now()
	task.Steps[index].CompletedAt = &completedAt
	m.persistTask(task)
	m.mu.Unlock()
	m.publish(task.ID, AutopilotEvent{
		Type:    "task_update",
		TaskID:  task.ID,
		GroupID: fmt.Sprintf("%s:%s", task.ID, task.Steps[index].ID),
		Status:  string(StepStatusCompleted),
		Summary: trimSummary(output),
		Content: output,
	})
	m.publish(task.ID, AutopilotEvent{
		Type:    "task_step_complete",
		TaskID:  task.ID,
		GroupID: fmt.Sprintf("%s:%s", task.ID, task.Steps[index].ID),
		Step:    index + 1,
		Status:  string(StepStatusCompleted),
		Summary: trimSummary(output),
		Content: output,
		Metadata: map[string]interface{}{
			"role": task.Steps[index].Role,
		},
	})
	m.publish(task.ID, AutopilotEvent{
		Type:    "autopilot_text",
		TaskID:  task.ID,
		Content: output,
	})

	return nil
}

func (m *Manager) completeToolCallingStep(ctx context.Context, task *Task, stepIndex int, request providers.ChatRequest) (providers.ChatResponse, error) {
	messages := append([]providers.Message(nil), request.Messages...)
	var resp providers.ChatResponse
	for round := 0; round < 4; round++ {
		request.Messages = messages
		currentResp, err := m.executor.ChatCompletion(ctx, request, task.SessionID)
		if err != nil {
			return providers.ChatResponse{}, err
		}
		resp = currentResp
		if len(currentResp.Choices) == 0 || len(currentResp.Choices[0].Message.ToolCalls) == 0 {
			return currentResp, nil
		}

		assistantMessage := currentResp.Choices[0].Message
		if strings.TrimSpace(assistantMessage.Role) == "" {
			assistantMessage.Role = "assistant"
		}
		messages = append(messages, assistantMessage)

		for _, toolCall := range currentResp.Choices[0].Message.ToolCalls {
			callID := firstNonEmptyString(orchestrationStringValue(toolCall["id"]), orchestrationStringValue(toolCall["call_id"]))
			toolName := builtInToolName(toolCall)
			m.publish(task.ID, AutopilotEvent{
				Type:    "tool_call_start",
				TaskID:  task.ID,
				GroupID: fmt.Sprintf("%s:%s", task.ID, task.Steps[stepIndex].ID),
				Step:    stepIndex + 1,
				Status:  "running",
				Summary: toolName,
				Metadata: map[string]interface{}{
					"tool_call_id": callID,
				},
			})

			toolName, result, err := m.executeBuiltInToolCall(ctx, toolCall)
			if err != nil {
				result = err.Error()
			}
			m.publish(task.ID, AutopilotEvent{
				Type:    "tool_call_result",
				TaskID:  task.ID,
				GroupID: fmt.Sprintf("%s:%s", task.ID, task.Steps[stepIndex].ID),
				Step:    stepIndex + 1,
				Status:  "completed",
				Summary: toolName,
				Content: result,
				Metadata: map[string]interface{}{
					"tool_call_id": callID,
				},
			})
			messages = append(messages, providers.Message{
				Role:       "tool",
				ToolCallID: callID,
				Content:    result,
			})
		}
	}
	return resp, nil
}

func builtInToolName(toolCall map[string]interface{}) string {
	function, _ := toolCall["function"].(map[string]interface{})
	return firstNonEmptyString(orchestrationStringValue(function["name"]), orchestrationStringValue(toolCall["name"]))
}

func builtInOrchestrationTools() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "create_task",
				"description": "Create a new orchestration task.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"goal":       map[string]interface{}{"type": "string"},
						"session_id": map[string]interface{}{"type": "string"},
						"roles": map[string]interface{}{
							"type":  "array",
							"items": map[string]interface{}{"type": "string"},
						},
						"execute":   map[string]interface{}{"type": "boolean"},
						"max_steps": map[string]interface{}{"type": "number"},
					},
					"required": []string{"goal"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "init_swarm",
				"description": "Initialize a new orchestration swarm.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"objective": map[string]interface{}{"type": "string"},
						"topology":  map[string]interface{}{"type": "string"},
						"strategy":  map[string]interface{}{"type": "string"},
						"max_agents": map[string]interface{}{
							"type": "number",
						},
						"session_id": map[string]interface{}{"type": "string"},
						"execute":    map[string]interface{}{"type": "boolean"},
						"agent_types": map[string]interface{}{
							"type":  "array",
							"items": map[string]interface{}{"type": "string"},
						},
					},
					"required": []string{"objective"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "run_workflow_template",
				"description": "Run an orchestration workflow template.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"template_id": map[string]interface{}{"type": "string"},
						"objective":   map[string]interface{}{"type": "string"},
						"session_id":  map[string]interface{}{"type": "string"},
						"execute":     map[string]interface{}{"type": "boolean"},
					},
					"required": []string{"template_id"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "get_workflow_state",
				"description": "Return workflow state for a workflow or swarm execution.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"workflow_id": map[string]interface{}{"type": "string"},
					},
					"required": []string{"workflow_id"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "get_workflow_metrics",
				"description": "Return workflow execution metrics.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"workflow_id": map[string]interface{}{"type": "string"},
					},
					"required": []string{"workflow_id"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "get_workflow_debug_info",
				"description": "Return workflow execution trace and debug information.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"workflow_id": map[string]interface{}{"type": "string"},
					},
					"required": []string{"workflow_id"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "get_workflow_template",
				"description": "Fetch a single orchestration workflow template by id.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"template_id": map[string]interface{}{"type": "string"},
					},
					"required": []string{"template_id"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "save_workflow_template",
				"description": "Create or update a custom orchestration workflow template.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id":          map[string]interface{}{"type": "string"},
						"name":        map[string]interface{}{"type": "string"},
						"description": map[string]interface{}{"type": "string"},
						"topology":    map[string]interface{}{"type": "string"},
						"strategy":    map[string]interface{}{"type": "string"},
						"agent_types": map[string]interface{}{
							"type":  "array",
							"items": map[string]interface{}{"type": "string"},
						},
						"roles": map[string]interface{}{
							"type":  "array",
							"items": map[string]interface{}{"type": "string"},
						},
					},
					"required": []string{"id"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "delete_workflow_template",
				"description": "Delete a custom orchestration workflow template.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"template_id": map[string]interface{}{"type": "string"},
					},
					"required": []string{"template_id"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "list_workflow_templates",
				"description": "List available orchestration workflow templates.",
				"parameters": map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "list_tasks",
				"description": "List orchestration tasks, optionally filtered by session_id or status.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"session_id": map[string]interface{}{"type": "string"},
						"status":     map[string]interface{}{"type": "string"},
					},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "assign_task",
				"description": "Assign an orchestration task to an agent.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"task_id":  map[string]interface{}{"type": "string"},
						"agent_id": map[string]interface{}{"type": "string"},
					},
					"required": []string{"task_id", "agent_id"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "start_task",
				"description": "Start execution of an orchestration task.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"task_id": map[string]interface{}{"type": "string"},
					},
					"required": []string{"task_id"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "cancel_task",
				"description": "Cancel an orchestration task.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"task_id": map[string]interface{}{"type": "string"},
					},
					"required": []string{"task_id"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "get_task",
				"description": "Fetch a single orchestration task by id.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"task_id": map[string]interface{}{"type": "string"},
					},
					"required": []string{"task_id"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "list_agents",
				"description": "List orchestration agents, optionally filtered by status.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"status": map[string]interface{}{"type": "string"},
					},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "spawn_agent",
				"description": "Spawn a new orchestration agent.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"type":     map[string]interface{}{"type": "string"},
						"name":     map[string]interface{}{"type": "string"},
						"swarm_id": map[string]interface{}{"type": "string"},
					},
					"required": []string{"type"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "stop_agent",
				"description": "Stop an orchestration agent.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"agent_id": map[string]interface{}{"type": "string"},
					},
					"required": []string{"agent_id"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "list_swarms",
				"description": "List orchestration swarms.",
				"parameters": map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "propose_consensus",
				"description": "Build a consensus view from swarm state, outputs, and risks.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"swarm_id": map[string]interface{}{"type": "string"},
					},
					"required": []string{"swarm_id"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "delegate_task",
				"description": "Assign a task to the best matching agent in a swarm.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"task_id":  map[string]interface{}{"type": "string"},
						"swarm_id": map[string]interface{}{"type": "string"},
					},
					"required": []string{"task_id", "swarm_id"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "start_swarm",
				"description": "Start a swarm and create an execution task.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"swarm_id":   map[string]interface{}{"type": "string"},
						"session_id": map[string]interface{}{"type": "string"},
					},
					"required": []string{"swarm_id"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "stop_swarm",
				"description": "Stop a running swarm.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"swarm_id": map[string]interface{}{"type": "string"},
					},
					"required": []string{"swarm_id"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "refine_task",
				"description": "Create a refinement task from an existing orchestration task.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"task_id":  map[string]interface{}{"type": "string"},
						"feedback": map[string]interface{}{"type": "string"},
						"execute":  map[string]interface{}{"type": "boolean"},
					},
					"required": []string{"task_id", "feedback"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "resume_session",
				"description": "Resume work in an existing orchestration session.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"session_id": map[string]interface{}{"type": "string"},
						"goal":       map[string]interface{}{"type": "string"},
						"execute":    map[string]interface{}{"type": "boolean"},
					},
					"required": []string{"session_id"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "fork_session",
				"description": "Fork work from an existing orchestration session into a new session.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"source_session_id": map[string]interface{}{"type": "string"},
						"session_id":        map[string]interface{}{"type": "string"},
						"goal":              map[string]interface{}{"type": "string"},
						"execute":           map[string]interface{}{"type": "boolean"},
					},
					"required": []string{"source_session_id"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "get_swarm_status",
				"description": "Fetch swarm status, agents, tasks, and metrics for a swarm.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"swarm_id": map[string]interface{}{"type": "string"},
					},
					"required": []string{"swarm_id"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "scale_swarm",
				"description": "Scale a swarm to the requested agent count.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"swarm_id": map[string]interface{}{"type": "string"},
						"count":    map[string]interface{}{"type": "number"},
					},
					"required": []string{"swarm_id", "count"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "coordinate_swarm",
				"description": "Coordinate a swarm to a target active agent count.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"swarm_id": map[string]interface{}{"type": "string"},
						"agents":   map[string]interface{}{"type": "number"},
					},
					"required": []string{"swarm_id", "agents"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "rebalance_swarm",
				"description": "Rebalance pending swarm tasks across the best matching agents.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"swarm_id": map[string]interface{}{"type": "string"},
					},
					"required": []string{"swarm_id"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "get_swarm_load",
				"description": "Return swarm-wide load distribution and balance metrics.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"swarm_id": map[string]interface{}{"type": "string"},
					},
					"required": []string{"swarm_id"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "detect_swarm_imbalance",
				"description": "Detect overloaded and underloaded agents in a swarm.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"swarm_id": map[string]interface{}{"type": "string"},
					},
					"required": []string{"swarm_id"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "preview_rebalance",
				"description": "Preview swarm rebalancing without moving work.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"swarm_id": map[string]interface{}{"type": "string"},
					},
					"required": []string{"swarm_id"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "list_stealable_tasks",
				"description": "List swarm tasks that are good candidates for work stealing.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"swarm_id": map[string]interface{}{"type": "string"},
					},
					"required": []string{"swarm_id"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "steal_task",
				"description": "Steal a task from one agent to another within a swarm.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"task_id":    map[string]interface{}{"type": "string"},
						"swarm_id":   map[string]interface{}{"type": "string"},
						"stealer_id": map[string]interface{}{"type": "string"},
					},
					"required": []string{"task_id", "swarm_id"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "contest_task_steal",
				"description": "Contest a previously stolen task.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"task_id":           map[string]interface{}{"type": "string"},
						"original_agent_id": map[string]interface{}{"type": "string"},
						"reason":            map[string]interface{}{"type": "string"},
					},
					"required": []string{"task_id"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "resolve_task_contest",
				"description": "Resolve a task steal contest and declare a winner.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"task_id":         map[string]interface{}{"type": "string"},
						"winner_agent_id": map[string]interface{}{"type": "string"},
						"reason":          map[string]interface{}{"type": "string"},
					},
					"required": []string{"task_id", "winner_agent_id"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "detect_stale_tasks",
				"description": "Detect stale tasks within a swarm that are candidates for stealing.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"swarm_id":            map[string]interface{}{"type": "string"},
						"stale_after_minutes": map[string]interface{}{"type": "number"},
					},
					"required": []string{"swarm_id"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "get_agent_status",
				"description": "Fetch an agent and its metrics.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"agent_id": map[string]interface{}{"type": "string"},
					},
					"required": []string{"agent_id"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "search_memory",
				"description": "Search relevant memory within a session using lexical matching.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"session_id": map[string]interface{}{"type": "string"},
						"query":      map[string]interface{}{"type": "string"},
						"max_tokens": map[string]interface{}{"type": "number"},
					},
					"required": []string{"session_id", "query"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "get_session_history",
				"description": "Return full session history from memory.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"session_id": map[string]interface{}{"type": "string"},
					},
					"required": []string{"session_id"},
				},
			},
		},
	}
}

func (m *Manager) executeBuiltInToolCall(ctx context.Context, toolCall map[string]interface{}) (string, string, error) {
	function, _ := toolCall["function"].(map[string]interface{})
	name := firstNonEmptyString(orchestrationStringValue(function["name"]), orchestrationStringValue(toolCall["name"]))
	args := parseToolArguments(function["arguments"])

	switch name {
	case "get_workflow_template":
		templateID := orchestrationStringValue(args["template_id"])
		template, ok := m.GetWorkflowTemplate(templateID)
		if !ok {
			return name, "", fmt.Errorf("workflow template not found: %s", templateID)
		}
		result, err := json.Marshal(template)
		return name, string(result), err
	case "save_workflow_template":
		template, err := m.SaveWorkflowTemplate(WorkflowTemplate{
			ID:          orchestrationStringValue(args["id"]),
			Name:        orchestrationStringValue(args["name"]),
			Description: orchestrationStringValue(args["description"]),
			Topology:    orchestrationStringValue(args["topology"]),
			Strategy:    orchestrationStringValue(args["strategy"]),
			AgentTypes:  stringSliceValue(args["agent_types"]),
			Roles:       stringSliceValue(args["roles"]),
		})
		if err != nil {
			return name, "", err
		}
		result, err := json.Marshal(template)
		return name, string(result), err
	case "delete_workflow_template":
		templateID := orchestrationStringValue(args["template_id"])
		if err := m.DeleteWorkflowTemplate(templateID); err != nil {
			return name, "", err
		}
		result, err := json.Marshal(map[string]interface{}{
			"deleted":     true,
			"template_id": templateID,
		})
		return name, string(result), err
	case "create_task":
		task, err := m.CreateTask(ctx, TaskRequest{
			Goal:      orchestrationStringValue(args["goal"]),
			SessionID: orchestrationStringValue(args["session_id"]),
			Roles:     stringSliceValue(args["roles"]),
			Execute:   boolValue(args["execute"]),
			MaxSteps:  intValue(args["max_steps"], 0),
		})
		if err != nil {
			return name, "", err
		}
		result, err := json.Marshal(task)
		return name, string(result), err
	case "init_swarm":
		swarm, err := m.InitSwarm(ctx, SwarmRequest{
			Objective:  orchestrationStringValue(args["objective"]),
			Topology:   orchestrationStringValue(args["topology"]),
			Strategy:   orchestrationStringValue(args["strategy"]),
			MaxAgents:  intValue(args["max_agents"], 0),
			SessionID:  orchestrationStringValue(args["session_id"]),
			Execute:    boolValue(args["execute"]),
			AgentTypes: stringSliceValue(args["agent_types"]),
		})
		if err != nil {
			return name, "", err
		}
		result, err := json.Marshal(swarm)
		return name, string(result), err
	case "run_workflow_template":
		swarm, task, err := m.RunWorkflowTemplate(ctx, orchestrationStringValue(args["template_id"]), WorkflowRunRequest{
			Objective: orchestrationStringValue(args["objective"]),
			SessionID: orchestrationStringValue(args["session_id"]),
			Execute:   boolValue(args["execute"]),
		})
		if err != nil {
			return name, "", err
		}
		result, err := json.Marshal(map[string]interface{}{
			"swarm": swarm,
			"task":  task,
		})
		return name, string(result), err
	case "get_workflow_state":
		workflowID := orchestrationStringValue(args["workflow_id"])
		state, err := m.WorkflowState(workflowID)
		if err != nil {
			return name, "", err
		}
		result, err := json.Marshal(state)
		return name, string(result), err
	case "get_workflow_metrics":
		workflowID := orchestrationStringValue(args["workflow_id"])
		metrics, err := m.WorkflowMetrics(workflowID)
		if err != nil {
			return name, "", err
		}
		result, err := json.Marshal(metrics)
		return name, string(result), err
	case "get_workflow_debug_info":
		workflowID := orchestrationStringValue(args["workflow_id"])
		debugInfo, err := m.WorkflowDebugInfo(workflowID)
		if err != nil {
			return name, "", err
		}
		result, err := json.Marshal(debugInfo)
		return name, string(result), err
	case "list_workflow_templates":
		result, err := json.Marshal(m.ListWorkflowTemplates())
		return name, string(result), err
	case "list_tasks":
		sessionID := orchestrationStringValue(args["session_id"])
		status := strings.ToLower(orchestrationStringValue(args["status"]))
		var tasks []*Task
		if sessionID != "" {
			tasks = m.ListTasksBySession(sessionID)
		} else {
			tasks = m.ListTasks()
		}
		if status != "" {
			filtered := make([]*Task, 0, len(tasks))
			for _, task := range tasks {
				if strings.ToLower(string(task.Status)) == status {
					filtered = append(filtered, task)
				}
			}
			tasks = filtered
		}
		result, err := json.Marshal(tasks)
		return name, string(result), err
	case "assign_task":
		taskID := orchestrationStringValue(args["task_id"])
		agentID := orchestrationStringValue(args["agent_id"])
		task, err := m.AssignTask(taskID, agentID)
		if err != nil {
			return name, "", err
		}
		result, err := json.Marshal(task)
		return name, string(result), err
	case "start_task":
		taskID := orchestrationStringValue(args["task_id"])
		if err := m.StartTask(taskID); err != nil {
			return name, "", err
		}
		task, err := m.GetTask(taskID)
		if err != nil {
			return name, "", err
		}
		result, err := json.Marshal(task)
		return name, string(result), err
	case "cancel_task":
		taskID := orchestrationStringValue(args["task_id"])
		task, err := m.CancelTask(taskID)
		if err != nil {
			return name, "", err
		}
		result, err := json.Marshal(task)
		return name, string(result), err
	case "get_task":
		taskID := orchestrationStringValue(args["task_id"])
		task, err := m.GetTask(taskID)
		if err != nil {
			return name, "", err
		}
		result, err := json.Marshal(task)
		return name, string(result), err
	case "list_agents":
		status := orchestrationStringValue(args["status"])
		var agents []*Agent
		if status != "" {
			agents = m.ListAgentsFiltered(status)
		} else {
			agents = m.ListAgents()
		}
		result, err := json.Marshal(agents)
		return name, string(result), err
	case "spawn_agent":
		agent, err := m.SpawnAgent(ctx, AgentSpawnRequest{
			Type:    orchestrationStringValue(args["type"]),
			Name:    orchestrationStringValue(args["name"]),
			SwarmID: orchestrationStringValue(args["swarm_id"]),
		})
		if err != nil {
			return name, "", err
		}
		result, err := json.Marshal(agent)
		return name, string(result), err
	case "stop_agent":
		agentID := orchestrationStringValue(args["agent_id"])
		agent, err := m.StopAgent(agentID)
		if err != nil {
			return name, "", err
		}
		result, err := json.Marshal(agent)
		return name, string(result), err
	case "list_swarms":
		result, err := json.Marshal(m.ListSwarms())
		return name, string(result), err
	case "propose_consensus":
		swarmID := orchestrationStringValue(args["swarm_id"])
		consensus, err := m.ProposeConsensus(swarmID)
		if err != nil {
			return name, "", err
		}
		result, err := json.Marshal(consensus)
		return name, string(result), err
	case "delegate_task":
		taskID := orchestrationStringValue(args["task_id"])
		swarmID := orchestrationStringValue(args["swarm_id"])
		swarm, err := m.GetSwarm(swarmID)
		if err != nil {
			return name, "", err
		}
		task, err := m.GetTask(taskID)
		if err != nil {
			return name, "", err
		}
		m.mu.RLock()
		internalSwarm := m.swarms[swarm.ID]
		internalTask := m.tasks[task.ID]
		agentID := m.selectAgentForTaskLocked(internalSwarm, internalTask)
		m.mu.RUnlock()
		if agentID == "" {
			return name, "", fmt.Errorf("no eligible agent found for task %s in swarm %s", taskID, swarmID)
		}
		assigned, err := m.AssignTask(taskID, agentID)
		if err != nil {
			return name, "", err
		}
		result, err := json.Marshal(assigned)
		return name, string(result), err
	case "start_swarm":
		swarmID := orchestrationStringValue(args["swarm_id"])
		sessionID := orchestrationStringValue(args["session_id"])
		task, err := m.StartSwarm(ctx, swarmID, sessionID)
		if err != nil {
			return name, "", err
		}
		result, err := json.Marshal(task)
		return name, string(result), err
	case "stop_swarm":
		swarmID := orchestrationStringValue(args["swarm_id"])
		swarm, err := m.StopSwarm(swarmID)
		if err != nil {
			return name, "", err
		}
		result, err := json.Marshal(swarm)
		return name, string(result), err
	case "refine_task":
		taskID := orchestrationStringValue(args["task_id"])
		feedback := orchestrationStringValue(args["feedback"])
		execute := boolValue(args["execute"])
		task, err := m.RefineTask(ctx, taskID, RefineRequest{
			Feedback: feedback,
			Execute:  execute,
		})
		if err != nil {
			return name, "", err
		}
		result, err := json.Marshal(task)
		return name, string(result), err
	case "resume_session":
		sessionID := orchestrationStringValue(args["session_id"])
		goal := orchestrationStringValue(args["goal"])
		execute := boolValue(args["execute"])
		task, err := m.ResumeSessionTask(ctx, sessionID, goal, execute)
		if err != nil {
			return name, "", err
		}
		result, err := json.Marshal(task)
		return name, string(result), err
	case "fork_session":
		sourceSessionID := orchestrationStringValue(args["source_session_id"])
		task, err := m.ForkSessionTask(ctx, sourceSessionID, SessionForkRequest{
			SessionID: orchestrationStringValue(args["session_id"]),
			Goal:      orchestrationStringValue(args["goal"]),
			Execute:   boolValue(args["execute"]),
		})
		if err != nil {
			return name, "", err
		}
		result, err := json.Marshal(task)
		return name, string(result), err
	case "get_swarm_status":
		swarmID := orchestrationStringValue(args["swarm_id"])
		status, err := m.SwarmStatus(swarmID)
		if err != nil {
			return name, "", err
		}
		result, err := json.Marshal(status)
		return name, string(result), err
	case "scale_swarm":
		swarmID := orchestrationStringValue(args["swarm_id"])
		count := intValue(args["count"], 0)
		swarm, err := m.ScaleSwarm(ctx, swarmID, count)
		if err != nil {
			return name, "", err
		}
		result, err := json.Marshal(swarm)
		return name, string(result), err
	case "coordinate_swarm":
		swarmID := orchestrationStringValue(args["swarm_id"])
		agents := intValue(args["agents"], 0)
		swarm, err := m.CoordinateSwarm(ctx, swarmID, agents)
		if err != nil {
			return name, "", err
		}
		result, err := json.Marshal(swarm)
		return name, string(result), err
	case "rebalance_swarm":
		swarmID := orchestrationStringValue(args["swarm_id"])
		tasks, err := m.RebalanceSwarm(ctx, swarmID)
		if err != nil {
			return name, "", err
		}
		result, err := json.Marshal(map[string]interface{}{
			"swarm_id":       swarmID,
			"rebalanced":     len(tasks),
			"assigned_tasks": tasks,
		})
		return name, string(result), err
	case "get_swarm_load":
		swarmID := orchestrationStringValue(args["swarm_id"])
		load, err := m.SwarmLoad(swarmID)
		if err != nil {
			return name, "", err
		}
		result, err := json.Marshal(load)
		return name, string(result), err
	case "detect_swarm_imbalance":
		swarmID := orchestrationStringValue(args["swarm_id"])
		report, err := m.DetectImbalance(swarmID)
		if err != nil {
			return name, "", err
		}
		result, err := json.Marshal(report)
		return name, string(result), err
	case "preview_rebalance":
		swarmID := orchestrationStringValue(args["swarm_id"])
		preview, err := m.PreviewRebalance(swarmID)
		if err != nil {
			return name, "", err
		}
		result, err := json.Marshal(preview)
		return name, string(result), err
	case "list_stealable_tasks":
		swarmID := orchestrationStringValue(args["swarm_id"])
		stealable, err := m.ListStealableTasks(swarmID)
		if err != nil {
			return name, "", err
		}
		result, err := json.Marshal(stealable)
		return name, string(result), err
	case "steal_task":
		taskID := orchestrationStringValue(args["task_id"])
		swarmID := orchestrationStringValue(args["swarm_id"])
		stealerID := orchestrationStringValue(args["stealer_id"])
		resultValue, err := m.StealTask(ctx, taskID, swarmID, stealerID)
		if err != nil {
			return name, "", err
		}
		result, err := json.Marshal(resultValue)
		return name, string(result), err
	case "contest_task_steal":
		taskID := orchestrationStringValue(args["task_id"])
		originalAgentID := orchestrationStringValue(args["original_agent_id"])
		reason := orchestrationStringValue(args["reason"])
		contest, err := m.ContestTaskSteal(taskID, originalAgentID, reason)
		if err != nil {
			return name, "", err
		}
		result, err := json.Marshal(contest)
		return name, string(result), err
	case "resolve_task_contest":
		taskID := orchestrationStringValue(args["task_id"])
		winnerAgentID := orchestrationStringValue(args["winner_agent_id"])
		reason := orchestrationStringValue(args["reason"])
		contest, err := m.ResolveTaskStealContest(taskID, winnerAgentID, reason)
		if err != nil {
			return name, "", err
		}
		result, err := json.Marshal(contest)
		return name, string(result), err
	case "detect_stale_tasks":
		swarmID := orchestrationStringValue(args["swarm_id"])
		staleAfterMinutes := intValue(args["stale_after_minutes"], 30)
		stale, err := m.DetectStaleTasks(swarmID, time.Duration(staleAfterMinutes)*time.Minute)
		if err != nil {
			return name, "", err
		}
		result, err := json.Marshal(stale)
		return name, string(result), err
	case "get_agent_status":
		agentID := orchestrationStringValue(args["agent_id"])
		agent, err := m.GetAgent(agentID)
		if err != nil {
			return name, "", err
		}
		metrics, err := m.AgentMetrics(agentID)
		if err != nil {
			return name, "", err
		}
		result, err := json.Marshal(map[string]interface{}{
			"agent":   agent,
			"metrics": metrics,
			"health":  m.AgentHealth(),
		})
		return name, string(result), err
	case "search_memory":
		if m.memory == nil {
			return name, "", fmt.Errorf("memory unavailable")
		}
		sessionID := orchestrationStringValue(args["session_id"])
		query := orchestrationStringValue(args["query"])
		maxTokens := intValue(args["max_tokens"], 2000)
		results, err := m.memory.RetrieveRelevant(query, sessionID, maxTokens)
		if err != nil {
			return name, "", err
		}
		result, err := json.Marshal(results)
		return name, string(result), err
	case "get_session_history":
		if m.memory == nil {
			return name, "", fmt.Errorf("memory unavailable")
		}
		sessionID := orchestrationStringValue(args["session_id"])
		results, err := m.memory.GetSessionHistory(sessionID)
		if err != nil {
			return name, "", err
		}
		result, err := json.Marshal(results)
		return name, string(result), err
	default:
		// Delegate to tool registry if available
		if m.toolRegistry != nil {
			if _, ok := m.toolRegistry.Get(name); ok {
				workDir := m.workDir
				if workDir == "" {
					workDir = "."
				}
				result, err := m.toolRegistry.Execute(ctx, name, args, workDir)
				if err != nil {
					return name, "", err
				}
				if result.Error != "" {
					return name, result.Error, nil
				}
				return name, result.Output, nil
			}
		}
		return name, "", fmt.Errorf("unsupported tool: %s", name)
	}
}

func parseToolArguments(raw interface{}) map[string]interface{} {
	switch typed := raw.(type) {
	case map[string]interface{}:
		return typed
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil
		}
		var decoded map[string]interface{}
		if err := json.Unmarshal([]byte(typed), &decoded); err == nil {
			return decoded
		}
	}
	return nil
}

func orchestrationStringValue(value interface{}) string {
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	return ""
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func intValue(value interface{}, fallback int) int {
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	case int64:
		return int(typed)
	case json.Number:
		if parsed, err := typed.Int64(); err == nil {
			return int(parsed)
		}
	case string:
		if parsed, err := strconv.Atoi(strings.TrimSpace(typed)); err == nil {
			return parsed
		}
	}
	return fallback
}

func boolValue(value interface{}) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(typed))
		return err == nil && parsed
	default:
		return false
	}
}

func stringSliceValue(value interface{}) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []interface{}:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := orchestrationStringValue(item); text != "" {
				result = append(result, text)
			}
		}
		return result
	default:
		return nil
	}
}

func (m *Manager) markTaskRunning(taskID string) (*Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	task, ok := m.tasks[taskID]
	if !ok {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}
	if task.Status == TaskStatusRunning || task.Status == TaskStatusCompleted {
		return task, nil
	}
	if task.Status == TaskStatusPaused {
		task.Status = TaskStatusRunning
		m.persistTask(task)
		return task, nil
	}
	now := time.Now()
	task.Status = TaskStatusRunning
	task.StartedAt = &now
	m.persistTask(task)
	return task, nil
}

func (m *Manager) isTaskPaused(taskID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	task, ok := m.tasks[taskID]
	return ok && task.Status == TaskStatusPaused
}

func (m *Manager) completeTask(taskID, output string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	task := m.tasks[taskID]
	now := time.Now()
	task.Status = TaskStatusCompleted
	task.CompletedAt = &now
	task.FinalOutput = output
	m.persistTask(task)
	m.syncAgentsForTask(task, AgentStatusIdle)
}

func (m *Manager) failTask(taskID string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	task := m.tasks[taskID]
	now := time.Now()
	task.Status = TaskStatusFailed
	task.CompletedAt = &now
	task.Error = err.Error()
	m.persistTask(task)
	m.syncAgentsForTask(task, AgentStatusIdle)
}

func buildMessages(task *Task, step TaskStep, relevant []memory.Message, previousOutputs []string) []providers.Message {
	messages := []providers.Message{
		{
			Role:    "system",
			Content: systemPrompt(step.Role),
		},
	}

	if len(relevant) > 0 {
		var memoryContext strings.Builder
		memoryContext.WriteString("Relevant prior context:\n")
		for _, msg := range relevant {
			memoryContext.WriteString("- ")
			memoryContext.WriteString(msg.Role)
			memoryContext.WriteString(": ")
			memoryContext.WriteString(msg.Content)
			memoryContext.WriteString("\n")
		}
		messages = append(messages, providers.Message{Role: "system", Content: memoryContext.String()})
	}

	if len(previousOutputs) > 0 {
		var prior strings.Builder
		prior.WriteString("Previous workflow outputs:\n")
		for i, output := range previousOutputs {
			prior.WriteString(fmt.Sprintf("Step %d:\n%s\n\n", i+1, output))
		}
		messages = append(messages, providers.Message{Role: "system", Content: prior.String()})
	}

	messages = append(messages, providers.Message{
		Role:    "user",
		Content: step.Prompt + "\n\nGoal:\n" + task.Goal,
	})
	if strings.TrimSpace(task.Feedback) != "" {
		messages = append(messages, providers.Message{
			Role:    "user",
			Content: "Refinement feedback:\n" + task.Feedback,
		})
	}
	return messages
}

func systemPrompt(role string) string {
	switch role {
	case "researcher":
		return "You are the researcher in a local orchestration workflow. Be concise, factual, and extract only the context needed for implementation."
	case "architect":
		return "You are the architect in a local orchestration workflow. Define a practical implementation approach with clear tradeoffs."
	case "coder":
		return "You are the coder in a local orchestration workflow. Produce implementation-ready output, favoring direct execution details."
	case "tester":
		return "You are the tester in a local orchestration workflow. Focus on verification, regressions, and edge cases."
	case "reviewer":
		return "You are the reviewer in a local orchestration workflow. Focus on bugs, missing cases, and residual risks."
	case "debugger":
		return "You are the debugger in a local orchestration workflow. Focus on root causes, not superficial symptoms."
	case "documenter":
		return "You are the documenter in a local orchestration workflow. Write concise technical summaries."
	default:
		return "You are a specialist worker in a local orchestration workflow."
	}
}

func firstChoice(resp providers.ChatResponse) string {
	if len(resp.Choices) == 0 {
		return ""
	}
	return strings.TrimSpace(resp.Choices[0].Message.Content)
}

func cloneTask(task *Task) *Task {
	copyTask := *task
	copyTask.Steps = append([]TaskStep(nil), task.Steps...)
	return &copyTask
}

func cloneAgent(agent *Agent) *Agent {
	copyAgent := *agent
	copyAgent.Capabilities = append([]string(nil), agent.Capabilities...)
	return &copyAgent
}

func cloneSwarm(swarm *Swarm) *Swarm {
	copySwarm := *swarm
	copySwarm.AgentIDs = append([]string(nil), swarm.AgentIDs...)
	copySwarm.TaskIDs = append([]string(nil), swarm.TaskIDs...)
	return &copySwarm
}

func cloneContest(contest *TaskStealContest) *TaskStealContest {
	if contest == nil {
		return nil
	}
	copyContest := *contest
	return &copyContest
}

func trimSummary(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 160 {
		return value
	}
	return value[:157] + "..."
}

func mapAgentTypeToRole(agentType string) string {
	switch strings.ToLower(strings.TrimSpace(agentType)) {
	case "coordinator", "task-orchestrator", "hierarchical-coordinator", "adaptive-coordinator", "collective-intelligence-coordinator":
		return "architect"
	case "architect", "system-architect", "repo-architect", "backend-architect", "frontend-architect", "database-architect", "security-architect", "api-designer":
		return "architect"
	case "coder", "integration-engineer", "toolsmith", "migration-specialist", "workflow-automation":
		return "coder"
	case "tester", "qa-engineer", "production-validator", "tdd-london-swarm":
		return "tester"
	case "reviewer", "code-review-swarm", "compliance-auditor":
		return "reviewer"
	case "researcher":
		return "researcher"
	case "debugger", "issue-tracker":
		return "debugger"
	case "documenter", "tech-writer":
		return "documenter"
	case "memory-coordinator", "swarm-memory-manager":
		return "researcher"
	case "perf-analyzer", "performance-benchmarker", "observability-engineer", "sre", "devops-engineer":
		return "tester"
	default:
		return "coder"
	}
}

func (m *Manager) publish(taskID string, event AutopilotEvent) {
	m.mu.RLock()
	listeners := append([]chan AutopilotEvent(nil), m.listeners[taskID]...)
	m.mu.RUnlock()
	for _, listener := range listeners {
		select {
		case listener <- event:
		default:
		}
	}
}

func (m *Manager) finishTaskListeners(taskID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, listener := range m.listeners[taskID] {
		close(listener)
	}
	delete(m.listeners, taskID)
}

func (m *Manager) syncAgentsForTask(task *Task, status AgentStatus) {
	for _, swarm := range m.swarms {
		for _, taskID := range swarm.TaskIDs {
			if taskID != task.ID {
				continue
			}
			for _, agentID := range swarm.AgentIDs {
				if agent, ok := m.agents[agentID]; ok {
					if task.AssignedTo != "" && status == AgentStatusIdle && agent.ID != task.AssignedTo {
						continue
					}
					agent.Status = status
					agent.LastSeenAt = time.Now()
					m.persistAgent(agent)
				}
			}
			if status == AgentStatusIdle {
				swarm.Status = SwarmStatusReady
				finishedAt := time.Now()
				swarm.FinishedAt = &finishedAt
				m.persistSwarm(swarm)
			}
		}
	}
}

func (m *Manager) selectAgentForTaskLocked(swarm *Swarm, task *Task) string {
	if swarm == nil || task == nil {
		return ""
	}

	preferredRole := ""
	if len(task.Steps) > 0 {
		preferredRole = task.Steps[0].Role
	}

	bestID := ""
	bestScore := -1
	for _, agentID := range swarm.AgentIDs {
		agent, ok := m.agents[agentID]
		if !ok {
			continue
		}
		score := m.agentSuitabilityScoreLocked(agent, task, preferredRole, swarm.Topology, swarm.Strategy)
		if score > bestScore {
			bestScore = score
			bestID = agent.ID
		}
	}
	return bestID
}

func (m *Manager) agentSuitabilityScoreLocked(agent *Agent, task *Task, preferredRole, topology, strategy string) int {
	if agent == nil {
		return -1
	}
	if agent.Status == AgentStatusStopped {
		return -1
	}

	score := 0
	switch strings.ToLower(strings.TrimSpace(topology)) {
	case "hierarchical", "star":
		if strings.Contains(agent.Type, "coordinator") {
			score += 3
		}
	case "mesh":
		if strings.Contains(agent.Type, "mesh") {
			score += 3
		}
	case "hierarchical-mesh":
		if strings.Contains(agent.Type, "hierarchical") || strings.Contains(agent.Type, "mesh") {
			score += 2
		}
	case "adaptive":
		if strings.Contains(agent.Type, "adaptive") {
			score += 3
		}
	}

	if preferredRole != "" && mapAgentTypeToRole(agent.Type) == preferredRole {
		score += 5
	}
	if agent.Status == AgentStatusIdle {
		score += 2
	}
	if task != nil {
		goal := strings.ToLower(strings.TrimSpace(task.Goal))
		for _, capability := range agent.Capabilities {
			if strings.Contains(goal, strings.ToLower(strings.TrimSpace(capability))) {
				score += 2
			}
		}
		for _, keyword := range roleKeywords(preferredRole) {
			for _, capability := range agent.Capabilities {
				if strings.Contains(strings.ToLower(capability), keyword) {
					score += 2
				}
			}
		}
	}
	switch strings.ToLower(strings.TrimSpace(strategy)) {
	case "research":
		if strings.Contains(agent.Type, "research") || strings.Contains(agent.Type, "document") {
			score += 3
		}
	case "security":
		if strings.Contains(agent.Type, "security") || strings.Contains(agent.Type, "review") {
			score += 3
		}
	case "debugging":
		if strings.Contains(agent.Type, "debug") || strings.Contains(agent.Type, "tester") {
			score += 3
		}
	case "development", "sparc":
		if strings.Contains(agent.Type, "architect") || strings.Contains(agent.Type, "coder") || strings.Contains(agent.Type, "tester") {
			score += 2
		}
	case "release", "shipping", "delivery":
		if strings.Contains(agent.Type, "deploy") || strings.Contains(agent.Type, "release") || strings.Contains(agent.Type, "devops") {
			score += 3
		}
	}
	score -= m.agentActiveTaskCountLocked(agent.ID)
	return score
}

func (m *Manager) agentActiveTaskCountLocked(agentID string) int {
	count := 0
	for _, task := range m.tasks {
		if task.AssignedTo != agentID {
			continue
		}
		switch task.Status {
		case TaskStatusAssigned, TaskStatusRunning, TaskStatusQueued:
			count++
		}
	}
	return count
}

func (m *Manager) agentLoadInfoLocked(agent *Agent, swarm *Swarm) AgentLoadInfo {
	info := AgentLoadInfo{
		AgentID:        agent.ID,
		AgentType:      agent.Type,
		MaxTasks:       max(1, swarm.MaxAgents),
		LastActivityAt: agent.LastSeenAt,
	}
	var completionDurations []int64
	for _, task := range m.tasks {
		if task.AssignedTo != agent.ID {
			continue
		}
		if task.Status != TaskStatusCompleted && task.Status != TaskStatusAssigned && task.Status != TaskStatusRunning && task.Status != TaskStatusQueued && task.Status != TaskStatusPaused {
			continue
		}
		info.TaskCount++
		info.ActiveTaskIDs = append(info.ActiveTaskIDs, task.ID)
		if task.Status == TaskStatusPaused {
			info.CurrentBlockedCount++
		}
		if task.StartedAt != nil && task.CompletedAt != nil {
			completionDurations = append(completionDurations, task.CompletedAt.Sub(*task.StartedAt).Milliseconds())
		}
	}
	if info.MaxTasks > 0 {
		info.Utilization = float64(info.TaskCount) / float64(info.MaxTasks)
	}
	if len(completionDurations) > 0 {
		var total int64
		for _, duration := range completionDurations {
			total += duration
		}
		info.AvgCompletionTimeMS = total / int64(len(completionDurations))
	}
	return info
}

func (m *Manager) isTaskStealableLocked(task *Task) bool {
	if task == nil {
		return false
	}
	if task.AssignedTo == "" {
		return false
	}
	switch task.Status {
	case TaskStatusCompleted, TaskStatusFailed, TaskStatusCancelled:
		return false
	}
	return taskProgress(task) <= 0.25
}

func taskProgress(task *Task) float64 {
	if task == nil || len(task.Steps) == 0 {
		return 0
	}
	completed := 0
	for _, step := range task.Steps {
		if step.Status == StepStatusCompleted {
			completed++
		}
	}
	return float64(completed) / float64(len(task.Steps))
}

func taskStealableReason(task *Task) string {
	progress := taskProgress(task)
	switch task.Status {
	case TaskStatusPaused:
		return "paused task can be stolen"
	case TaskStatusQueued, TaskStatusAssigned:
		return "queued or assigned task is low progress"
	default:
		if progress <= 0.25 {
			return "task has low progress and can be reassigned"
		}
		return "task is eligible for reassignment"
	}
}

func roleKeywords(role string) []string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "architect":
		return []string{"design", "architecture", "interfaces"}
	case "coder":
		return []string{"implementation", "integration", "refactor"}
	case "tester":
		return []string{"testing", "verification", "coverage"}
	case "reviewer":
		return []string{"review", "risk", "quality"}
	case "researcher":
		return []string{"research", "compare", "summarize"}
	case "debugger":
		return []string{"debug", "triage", "root"}
	case "documenter":
		return []string{"documentation", "guides", "reference"}
	default:
		return nil
	}
}

func (m *Manager) persistTask(task *Task) {
	if m.db == nil || task == nil {
		return
	}
	if _, err := m.db.Exec(`
		INSERT INTO orchestration_tasks (
			id, parent_task_id, goal, session_id, status, assigned_to, iteration, feedback, created_at, started_at, completed_at, error, final_output
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			parent_task_id = excluded.parent_task_id,
			goal = excluded.goal,
			session_id = excluded.session_id,
			status = excluded.status,
			assigned_to = excluded.assigned_to,
			iteration = excluded.iteration,
			feedback = excluded.feedback,
			created_at = excluded.created_at,
			started_at = excluded.started_at,
			completed_at = excluded.completed_at,
			error = excluded.error,
			final_output = excluded.final_output
	`, task.ID, task.ParentTaskID, task.Goal, task.SessionID, task.Status, task.AssignedTo, task.Iteration, task.Feedback, task.CreatedAt, task.StartedAt, task.CompletedAt, task.Error, task.FinalOutput); err != nil {
		log.Printf("[orchestration] persist task failed: %v", err)
		return
	}

	if _, err := m.db.Exec(`DELETE FROM orchestration_steps WHERE task_id = ?`, task.ID); err != nil {
		log.Printf("[orchestration] clear task steps failed: %v", err)
		return
	}
	for _, step := range task.Steps {
		if _, err := m.db.Exec(`
			INSERT INTO orchestration_steps (
				task_id, step_id, role, prompt, status, started_at, completed_at, output, error
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, task.ID, step.ID, step.Role, step.Prompt, step.Status, step.StartedAt, step.CompletedAt, step.Output, step.Error); err != nil {
			log.Printf("[orchestration] persist task step failed: %v", err)
		}
	}
}

func (m *Manager) persistAgent(agent *Agent) {
	if m.db == nil || agent == nil {
		return
	}
	capabilitiesJSON, _ := json.Marshal(agent.Capabilities)
	if _, err := m.db.Exec(`
		INSERT INTO orchestration_agents (
			id, type, name, description, capabilities, status, swarm_id, created_at, last_seen_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			type = excluded.type,
			name = excluded.name,
			description = excluded.description,
			capabilities = excluded.capabilities,
			status = excluded.status,
			swarm_id = excluded.swarm_id,
			created_at = excluded.created_at,
			last_seen_at = excluded.last_seen_at
	`, agent.ID, agent.Type, agent.Name, agent.Description, string(capabilitiesJSON), agent.Status, agent.SwarmID, agent.CreatedAt, agent.LastSeenAt); err != nil {
		log.Printf("[orchestration] persist agent failed: %v", err)
	}
}

func (m *Manager) persistSwarm(swarm *Swarm) {
	if m.db == nil || swarm == nil {
		return
	}
	agentIDsJSON, _ := json.Marshal(swarm.AgentIDs)
	taskIDsJSON, _ := json.Marshal(swarm.TaskIDs)
	if _, err := m.db.Exec(`
		INSERT INTO orchestration_swarms (
			id, objective, topology, strategy, status, max_agents, agent_ids, task_ids, created_at, started_at, finished_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			objective = excluded.objective,
			topology = excluded.topology,
			strategy = excluded.strategy,
			status = excluded.status,
			max_agents = excluded.max_agents,
			agent_ids = excluded.agent_ids,
			task_ids = excluded.task_ids,
			created_at = excluded.created_at,
			started_at = excluded.started_at,
			finished_at = excluded.finished_at
	`, swarm.ID, swarm.Objective, swarm.Topology, swarm.Strategy, swarm.Status, swarm.MaxAgents, string(agentIDsJSON), string(taskIDsJSON), swarm.CreatedAt, swarm.StartedAt, swarm.FinishedAt); err != nil {
		log.Printf("[orchestration] persist swarm failed: %v", err)
	}
}
