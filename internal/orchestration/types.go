package orchestration

import "time"

type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusQueued    TaskStatus = "queued"
	TaskStatusAssigned  TaskStatus = "assigned"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusPaused    TaskStatus = "paused"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusCancelled TaskStatus = "cancelled"
)

type StepStatus string

const (
	StepStatusPending   StepStatus = "pending"
	StepStatusRunning   StepStatus = "running"
	StepStatusCompleted StepStatus = "completed"
	StepStatusFailed    StepStatus = "failed"
)

// Role describes a focused worker identity derived from the ruflo merge path.
type Role struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Capabilities []string `json:"capabilities,omitempty"`
}

type TaskRequest struct {
	Goal      string   `json:"goal"`
	SessionID string   `json:"session_id,omitempty"`
	Roles     []string `json:"roles,omitempty"`
	Execute   bool     `json:"execute,omitempty"`
	MaxSteps  int      `json:"max_steps,omitempty"`
}

type Task struct {
	ID           string     `json:"id"`
	ParentTaskID string     `json:"parent_task_id,omitempty"`
	Goal         string     `json:"goal"`
	SessionID    string     `json:"session_id"`
	Status       TaskStatus `json:"status"`
	AssignedTo   string     `json:"assigned_to,omitempty"`
	Iteration    int        `json:"iteration"`
	Feedback     string     `json:"feedback,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
	Error        string     `json:"error,omitempty"`
	Steps        []TaskStep `json:"steps"`
	FinalOutput  string     `json:"final_output,omitempty"`

	// Dependency graph fields for parallel execution
	DependsOn    []string   `json:"depends_on,omitempty"`    // Task IDs this task must wait for
	Blocks       []string   `json:"blocks,omitempty"`        // Task IDs blocked by this task
	Priority     int        `json:"priority,omitempty"`      // Higher priority tasks run first
}

type TaskStep struct {
	ID          string     `json:"id"`
	Role        string     `json:"role"`
	Prompt      string     `json:"prompt"`
	Status      StepStatus `json:"status"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	Output      string     `json:"output,omitempty"`
	Error       string     `json:"error,omitempty"`
}

type RefineRequest struct {
	Feedback string `json:"feedback"`
	Execute  bool   `json:"execute,omitempty"`
}

type SessionForkRequest struct {
	Goal      string `json:"goal,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	Execute   bool   `json:"execute,omitempty"`
}

type AgentStatus string

const (
	AgentStatusIdle    AgentStatus = "idle"
	AgentStatusBusy    AgentStatus = "busy"
	AgentStatusStopped AgentStatus = "stopped"
)

type SwarmStatus string

const (
	SwarmStatusReady   SwarmStatus = "ready"
	SwarmStatusRunning SwarmStatus = "running"
	SwarmStatusStopped SwarmStatus = "stopped"
)

type Agent struct {
	ID           string      `json:"id"`
	Type         string      `json:"type"`
	Name         string      `json:"name"`
	Description  string      `json:"description,omitempty"`
	Capabilities []string    `json:"capabilities,omitempty"`
	Status       AgentStatus `json:"status"`
	SwarmID      string      `json:"swarm_id,omitempty"`
	CreatedAt    time.Time   `json:"created_at"`
	LastSeenAt   time.Time   `json:"last_seen_at"`
}

type AgentSpawnRequest struct {
	Type    string `json:"type"`
	Name    string `json:"name,omitempty"`
	SwarmID string `json:"swarm_id,omitempty"`
}

type AgentMetrics struct {
	AgentID             string `json:"agent_id"`
	Status              string `json:"status"`
	AssignedTaskCount   int    `json:"assigned_task_count"`
	CompletedTaskCount  int    `json:"completed_task_count"`
	FailedTaskCount     int    `json:"failed_task_count"`
	ParticipatingSwarms int    `json:"participating_swarms"`
}

type AgentHealthSnapshot struct {
	Total   int `json:"total"`
	Idle    int `json:"idle"`
	Busy    int `json:"busy"`
	Stopped int `json:"stopped"`
}

type Swarm struct {
	ID         string      `json:"id"`
	Objective  string      `json:"objective"`
	Topology   string      `json:"topology"`
	Strategy   string      `json:"strategy"`
	Status     SwarmStatus `json:"status"`
	MaxAgents  int         `json:"max_agents"`
	AgentIDs   []string    `json:"agent_ids,omitempty"`
	TaskIDs    []string    `json:"task_ids,omitempty"`
	CreatedAt  time.Time   `json:"created_at"`
	StartedAt  *time.Time  `json:"started_at,omitempty"`
	FinishedAt *time.Time  `json:"finished_at,omitempty"`
}

type SwarmMetrics struct {
	AgentCount     int `json:"agent_count"`
	TaskCount      int `json:"task_count"`
	BusyAgents     int `json:"busy_agents"`
	IdleAgents     int `json:"idle_agents"`
	StoppedAgents  int `json:"stopped_agents"`
	CompletedTasks int `json:"completed_tasks"`
	FailedTasks    int `json:"failed_tasks"`
	RunningTasks   int `json:"running_tasks"`
}

type AgentLoadInfo struct {
	AgentID             string    `json:"agent_id"`
	AgentType           string    `json:"agent_type"`
	TaskCount           int       `json:"task_count"`
	MaxTasks            int       `json:"max_tasks"`
	Utilization         float64   `json:"utilization"`
	AvgCompletionTimeMS int64     `json:"avg_completion_time_ms"`
	CurrentBlockedCount int       `json:"current_blocked_count"`
	ActiveTaskIDs       []string  `json:"active_task_ids,omitempty"`
	LastActivityAt      time.Time `json:"last_activity_at"`
}

type SwarmLoadInfo struct {
	SwarmID           string          `json:"swarm_id"`
	TotalAgents       int             `json:"total_agents"`
	ActiveAgents      int             `json:"active_agents"`
	TotalTasks        int             `json:"total_tasks"`
	AvgUtilization    float64         `json:"avg_utilization"`
	BalanceScore      float64         `json:"balance_score"`
	OverloadedAgents  []string        `json:"overloaded_agents,omitempty"`
	UnderloadedAgents []string        `json:"underloaded_agents,omitempty"`
	Agents            []AgentLoadInfo `json:"agents"`
}

type RebalanceMove struct {
	TaskID string `json:"task_id"`
	From   string `json:"from"`
	To     string `json:"to"`
	Reason string `json:"reason,omitempty"`
}

type RebalanceResult struct {
	Moved   []RebalanceMove    `json:"moved,omitempty"`
	Preview []RebalanceMove    `json:"preview,omitempty"`
	Stats   map[string]float64 `json:"stats,omitempty"`
	Load    *SwarmLoadInfo     `json:"load,omitempty"`
}

type ImbalanceReport struct {
	SwarmID         string          `json:"swarm_id"`
	IsBalanced      bool            `json:"is_balanced"`
	BalanceScore    float64         `json:"balance_score"`
	AvgLoad         float64         `json:"avg_load"`
	Overloaded      []AgentLoadInfo `json:"overloaded,omitempty"`
	Underloaded     []AgentLoadInfo `json:"underloaded,omitempty"`
	Recommendations []string        `json:"recommendations,omitempty"`
}

type StealableTaskInfo struct {
	TaskID      string     `json:"task_id"`
	FromAgentID string     `json:"from_agent_id,omitempty"`
	Status      TaskStatus `json:"status"`
	Progress    float64    `json:"progress"`
	Reason      string     `json:"reason"`
}

type TaskStealRequest struct {
	SwarmID   string `json:"swarm_id,omitempty"`
	StealerID string `json:"stealer_id,omitempty"`
}

type TaskStealResult struct {
	TaskID      string `json:"task_id"`
	FromAgentID string `json:"from_agent_id,omitempty"`
	ToAgentID   string `json:"to_agent_id"`
	Reason      string `json:"reason,omitempty"`
}

type TaskStealContest struct {
	TaskID          string     `json:"task_id"`
	OriginalAgentID string     `json:"original_agent_id"`
	CurrentAgentID  string     `json:"current_agent_id"`
	Reason          string     `json:"reason"`
	Resolution      string     `json:"resolution,omitempty"`
	WinnerAgentID   string     `json:"winner_agent_id,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	ResolvedAt      *time.Time `json:"resolved_at,omitempty"`
}

type TaskContestRequest struct {
	OriginalAgentID string `json:"original_agent_id"`
	Reason          string `json:"reason"`
}

type TaskContestResolutionRequest struct {
	WinnerAgentID string `json:"winner_agent_id"`
	Reason        string `json:"reason"`
}

type SwarmStatusView struct {
	Swarm   *Swarm       `json:"swarm"`
	Agents  []*Agent     `json:"agents"`
	Tasks   []*Task      `json:"tasks"`
	Metrics SwarmMetrics `json:"metrics"`
}

type SwarmConsensusView struct {
	SwarmID          string   `json:"swarm_id"`
	Objective        string   `json:"objective"`
	Status           string   `json:"status"`
	CompletedOutputs []string `json:"completed_outputs,omitempty"`
	PendingTasks     []string `json:"pending_tasks,omitempty"`
	Risks            []string `json:"risks,omitempty"`
	Recommendation   string   `json:"recommendation,omitempty"`
}

type WorkflowStateView struct {
	ID             string     `json:"id"`
	Name           string     `json:"name,omitempty"`
	Status         string     `json:"status"`
	Tasks          []string   `json:"tasks,omitempty"`
	CompletedTasks []string   `json:"completed_tasks,omitempty"`
	CurrentTask    string     `json:"current_task,omitempty"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
	Paused         bool       `json:"paused"`
	SwarmID        string     `json:"swarm_id,omitempty"`
	Topology       string     `json:"topology,omitempty"`
	Strategy       string     `json:"strategy,omitempty"`
}

type WorkflowMetrics struct {
	TasksTotal          int     `json:"tasks_total"`
	TasksCompleted      int     `json:"tasks_completed"`
	TotalDurationMS     int64   `json:"total_duration_ms"`
	AverageTaskDuration int64   `json:"average_task_duration_ms"`
	SuccessRate         float64 `json:"success_rate"`
}

type WorkflowDebugTrace struct {
	TaskID    string    `json:"task_id"`
	Timestamp time.Time `json:"timestamp"`
	Action    string    `json:"action"`
}

type WorkflowDebugInfo struct {
	ExecutionTrace  []WorkflowDebugTrace     `json:"execution_trace,omitempty"`
	TaskTimings     map[string]int64         `json:"task_timings_ms,omitempty"`
	EventLog        []AutopilotEvent         `json:"event_log,omitempty"`
	MemorySnapshots []map[string]interface{} `json:"memory_snapshots,omitempty"`
}

type SwarmRequest struct {
	Objective  string   `json:"objective"`
	Topology   string   `json:"topology,omitempty"`
	Strategy   string   `json:"strategy,omitempty"`
	MaxAgents  int      `json:"max_agents,omitempty"`
	AgentTypes []string `json:"agent_types,omitempty"`
	Execute    bool     `json:"execute,omitempty"`
	SessionID  string   `json:"session_id,omitempty"`
}

type SwarmScaleRequest struct {
	Count int `json:"count"`
}

type SwarmCoordinateRequest struct {
	Agents int `json:"agents"`
}

type TaskAssignRequest struct {
	AgentID string `json:"agent_id"`
}

type AutopilotEvent struct {
	Type       string                 `json:"type"`
	TaskID     string                 `json:"taskId,omitempty"`
	GroupID    string                 `json:"groupId,omitempty"`
	Step       int                    `json:"step,omitempty"`
	MaxSteps   int                    `json:"maxSteps,omitempty"`
	TotalSteps int                    `json:"totalSteps,omitempty"`
	TotalTasks int                    `json:"totalTasks,omitempty"`
	Duration   int64                  `json:"duration,omitempty"`
	Status     string                 `json:"status,omitempty"`
	Summary    string                 `json:"summary,omitempty"`
	Content    string                 `json:"content,omitempty"`
	Reason     string                 `json:"reason,omitempty"`
	Error      string                 `json:"error,omitempty"`
	Tools      []string               `json:"tools,omitempty"`
	Tasks      []map[string]any       `json:"tasks,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}
