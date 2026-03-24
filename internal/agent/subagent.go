package agent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/tools"
)

// SpawnConfig configures a child agent.
type SpawnConfig struct {
	Role        string       // agent role (e.g., "tester", "researcher")
	Model       string       // model override (empty = inherit parent)
	Provider    string       // target specific provider by name (empty = default routing)
	Tools       *tools.Registry // tool registry (nil = inherit parent)
	Budget      *AgentBudget // resource limits for child
	WorkDir     string       // working directory (empty = inherit parent)
	System      string       // system prompt override
}

// ChildRef tracks a spawned child agent.
type ChildRef struct {
	ID        string    `json:"id"`
	ParentID  string    `json:"parent_id"`
	Role      string    `json:"role"`
	Status    string    `json:"status"` // "running", "completed", "failed"
	StartedAt time.Time `json:"started_at"`
	Result    string    `json:"result,omitempty"`
	Error     string    `json:"error,omitempty"`
}

// SpawnChild creates and returns a child agent configured from the parent.
func (a *Agent) SpawnChild(cfg SpawnConfig) *Agent {
	model := cfg.Model
	if model == "" {
		model = a.config.Model
	}

	registry := cfg.Tools
	if registry == nil {
		registry = a.registry
	}

	workDir := cfg.WorkDir
	if workDir == "" {
		workDir = a.config.WorkDir
	}

	maxTurns := a.config.MaxTurns
	if cfg.Budget != nil && cfg.Budget.MaxTurns > 0 {
		maxTurns = cfg.Budget.MaxTurns
	}

	sysPrompt := cfg.System
	if sysPrompt == "" {
		// Build skill-aware system prompt: base + dynamically matched skills
		sysPrompt = a.buildChildSystemPrompt(cfg.Role, workDir)
	}

	// Inherit escalation chain so sub-agents use the same provider ordering.
	// Without this, sub-agents fall through to default routing which may
	// pick providers (gemini, codex) that should only be reached via escalation.
	escalationChain := a.config.EscalationChain

	childConfig := Config{
		Model:           model,
		SystemPrompt:    sysPrompt,
		MaxTurns:        maxTurns,
		WorkDir:         workDir,
		TargetProvider:  cfg.Provider,
		EscalationChain: escalationChain,
		Skills:          a.config.Skills,          // inherit parent's skill registry for dynamic matching
		EventBus:        a.config.EventBus,
		ProjectLanguage: a.config.ProjectLanguage, // inherit so sub-agents use correct language context
		ToolStore:       a.config.ToolStore,       // inherit so sub-agents store tool outputs in same DB
		VectorMemory:    a.config.VectorMemory,    // inherit for recall tool access
	}

	child := New(a.executor, registry, a.renderer, childConfig)
	child.parentID = a.sessionID

	// Sub-agents start at the parent's current escalation level (or higher).
	// This prevents reviewers from using Level 0 small models when the parent
	// has already escalated past them.
	if len(escalationChain) > 0 && a.providerIdx > 0 {
		child.setMinProviderLevel(a.providerIdx)
	}

	if cfg.Budget != nil {
		child.budget = NewBudgetTracker(*cfg.Budget)
	}

	a.emit(EventSubAgentSpawn, cfg.Provider, map[string]any{
		"child_id": child.sessionID,
		"role":     cfg.Role,
	})

	// Track child in parent
	a.mu.Lock()
	a.children = append(a.children, &ChildRef{
		ID:        child.sessionID,
		ParentID:  a.sessionID,
		Role:      cfg.Role,
		Status:    "running",
		StartedAt: time.Now(),
	})
	a.mu.Unlock()

	return child
}

// RunChild spawns a child agent, runs a task, and returns the result.
// This is the convenience method for single-shot delegation.
func (a *Agent) RunChild(ctx context.Context, cfg SpawnConfig, task string) (string, error) {
	child := a.SpawnChild(cfg)
	childStart := time.Now()
	result, err := child.Run(ctx, task)
	childDuration := time.Since(childStart)

	// Update child ref status
	status := "completed"
	a.mu.Lock()
	for _, ref := range a.children {
		if ref.ID == child.sessionID {
			if err != nil {
				ref.Status = "failed"
				ref.Error = err.Error()
				status = "failed"
			} else {
				ref.Status = "completed"
				ref.Result = result
			}
		}
	}
	a.mu.Unlock()

	resultPreview := result
	if len(resultPreview) > 200 {
		resultPreview = resultPreview[:200] + "..."
	}
	a.emit(EventSubAgentComplete, cfg.Provider, map[string]any{
		"child_id":       child.sessionID,
		"role":           cfg.Role,
		"status":         status,
		"duration":       childDuration.String(),
		"result_preview": resultPreview,
	})

	return result, err
}

// Children returns the list of child agent references.
func (a *Agent) Children() []*ChildRef {
	a.mu.Lock()
	defer a.mu.Unlock()
	refs := make([]*ChildRef, len(a.children))
	copy(refs, a.children)
	return refs
}

// ParentID returns the parent agent's session ID, if this is a child.
func (a *Agent) ParentID() string {
	return a.parentID
}

// RunChildrenParallel spawns multiple child agents and runs them concurrently.
// Returns results indexed by role. Respects the provided concurrency limit.
func (a *Agent) RunChildrenParallel(ctx context.Context, tasks []DelegateTask, maxConcurrent int) []DelegateResult {
	if maxConcurrent <= 0 {
		maxConcurrent = 5
	}

	results := make([]DelegateResult, len(tasks))
	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup

	for i, task := range tasks {
		wg.Add(1)
		go func(idx int, dt DelegateTask) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			result, err := a.RunChild(ctx, dt.Config, dt.Task)
			results[idx] = DelegateResult{
				Role:   dt.Config.Role,
				Task:   dt.Task,
				Result: result,
			}
			if err != nil {
				results[idx].Error = err.Error()
			}
		}(i, task)
	}

	wg.Wait()
	return results
}

// DelegateTask pairs a spawn config with a task prompt.
type DelegateTask struct {
	Config SpawnConfig
	Task   string
}

// DelegateResult holds the outcome of a delegated task.
type DelegateResult struct {
	Role   string `json:"role"`
	Task   string `json:"task"`
	Result string `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

// buildChildSystemPrompt creates a system prompt for a child agent.
// Uses embedded child-agent.md for base instructions, then loads role-specific
// instructions from .claude/agents/{role}.md if available.
// Skills are NOT injected here — they go in the task prompt instead,
// where they can be tailored per role (planner vs coder vs reviewer).
func (a *Agent) buildChildSystemPrompt(role, workDir string) string {
	base := LoadPrompt("child-agent.md")
	if base == "" {
		base = "You have been delegated a specific task. Focus on completing it."
	}
	prompt := fmt.Sprintf("You are a %s agent working in: %s\n\n%s", role, workDir, base)

	// Load role-specific instructions from .claude/agents/{role}.md
	if roleInstr := LoadRoleInstructions(workDir, role); roleInstr != "" {
		prompt += "\n\n# Role Instructions\n" + roleInstr
	}
	return prompt
}
