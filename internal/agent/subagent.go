package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gr3enarr0w/synapserouter/internal/tools"
	"github.com/gr3enarr0w/synapserouter/internal/worktree"
)

// SpawnConfig configures a child agent.
type SpawnConfig struct {
	Role        string       // agent role (e.g., "tester", "researcher")
	Model       string       // model override (empty = inherit parent)
	Provider    string       // target specific provider by name (empty = default routing)
	Tier        ModelTier    // preferred model tier (cheap, mid, frontier) — sets initial provider level
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

	// Tell sub-agents about parent's work and how to access it
	sysPrompt += "\n\nPARENT SESSION CONTEXT: You were spawned by a parent agent. " +
		"Use the recall tool to access parent session context:\n" +
		"- recall(query=\"search term\") — semantic search over past outputs\n" +
		"- recall(tool_name=\"bash\") — filter by specific tool\n" +
		"- recall(id=N) — retrieve full output by ID (from a previous recall search)\n" +
		"Do NOT re-read files the parent already read — use recall instead."

	// Inherit escalation chain so sub-agents use the same provider ordering.
	// Without this, sub-agents fall through to default routing which may
	// pick providers (gemini, codex) that should only be reached via escalation.
	escalationChain := a.config.EscalationChain

	// Build parent session chain: current agent's session prepended to its own parent chain.
	// This gives the child visibility into all ancestor sessions for cross-session recall.
	parentChain := append([]string{a.sessionID}, a.config.ParentSessionIDs...)

	childConfig := Config{
		Model:            model,
		SystemPrompt:     sysPrompt,
		MaxTurns:         maxTurns,
		WorkDir:          workDir,
		TargetProvider:   cfg.Provider,
		EscalationChain:  escalationChain,
		// NOTE: AutoOrchestrate intentionally NOT inherited. Sub-agents should
		// just execute their task (write code, review, etc.), not run their own
		// 6-phase pipeline. The PARENT orchestrates phases; children execute.
		Skills:           a.config.Skills,           // inherit parent's skill registry for dynamic matching
		EventBus:         a.config.EventBus,
		SpecFilePath:     a.config.SpecFilePath,      // inherit so sub-agents can't overwrite spec
		ProjectLanguage:  a.config.ProjectLanguage,  // inherit so sub-agents use correct language context
		ToolStore:        a.config.ToolStore,         // inherit so sub-agents store tool outputs in same DB
		VectorMemory:     a.config.VectorMemory,      // inherit for recall tool access
		ParentSessionIDs: parentChain,                // pass full ancestor chain for cross-session recall
	}

	child := New(a.executor, registry, a.renderer, childConfig)
	child.parentID = a.sessionID

	// Set initial provider level based on tier preference first, then parent level.
	// Tier-based routing: cheap=bottom third, mid=middle third, frontier=top third.
	// Parent's current level is the floor — never go below it (monotonic).
	if len(escalationChain) > 0 {
		tierLevel := a.ProviderLevelForTier(cfg.Tier)
		if tierLevel > 0 {
			child.SetMinProviderLevel(tierLevel)
		}
		// Parent's current level is the absolute minimum
		if a.providerIdx > 0 {
			child.SetMinProviderLevel(a.providerIdx)
		}
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
// Each child automatically gets its own git worktree for isolation.
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

			// Create worktree for child isolation
			var worktreeID string
			var worktreePath string
			var wtManager *worktree.Manager
			var err error

			// Try to create worktree, fall back to parent WorkDir if it fails
			if wtManager, err = worktree.NewManager(worktree.Config{}); err == nil {
				wt, wtErr := wtManager.Create(a.config.WorkDir, fmt.Sprintf("child-%d-%s", idx, dt.Config.Role))
				if wtErr == nil {
					worktreeID = wt.ID
					worktreePath = wt.Path
					// Override child's WorkDir with worktree path
					dt.Config.WorkDir = worktreePath
				} else {
					// Worktree creation failed, fall back to parent WorkDir with warning
					fmt.Printf("[WARN] Worktree creation failed for child %d (%s): %v. Using parent WorkDir.\n", idx, dt.Config.Role, wtErr)
				}
			} else {
				fmt.Printf("[WARN] Worktree manager creation failed: %v. Using parent WorkDir.\n", err)
			}

			result, err := a.RunChild(ctx, dt.Config, dt.Task)

			// After child completes: sync files back and cleanup worktree
			if worktreePath != "" && worktreeID != "" {
				// Copy changed files from worktree back to parent WorkDir
				if syncErr := syncWorktreeToParent(worktreePath, a.config.WorkDir); syncErr != nil {
					fmt.Printf("[WARN] Failed to sync worktree files back to parent: %v\n", syncErr)
				}

				// Remove the worktree
				if rmErr := wtManager.Delete(worktreeID); rmErr != nil {
					fmt.Printf("[WARN] Failed to remove worktree %s: %v\n", worktreeID, rmErr)
				}
			}

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

// syncWorktreeToParent copies changed files from worktree back to parent directory.
func syncWorktreeToParent(worktreePath, parentPath string) error {
	return filepath.Walk(worktreePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip .git directory
		if info.Name() == ".git" && info.IsDir() {
			return filepath.SkipDir
		}

		// Calculate relative path from worktree
		relPath, err := filepath.Rel(worktreePath, path)
		if err != nil {
			return err
		}

		destPath := filepath.Join(parentPath, relPath)

		if info.IsDir() {
			// Create directory if it doesn't exist
			if _, err := os.Stat(destPath); os.IsNotExist(err) {
				return os.MkdirAll(destPath, 0755)
			}
			return nil
		}

		// Copy file
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		// Ensure parent directory exists
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return err
		}

		return os.WriteFile(destPath, data, info.Mode())
	})
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

	// Inject language directive so children know the project language
	if a.config.ProjectLanguage != "" {
		prompt += fmt.Sprintf("\n\nPROJECT LANGUAGE: %s — all source files MUST be in this language.", a.config.ProjectLanguage)
	}

	// Inject spec compliance — children MUST follow spec architecture
	prompt += `

SPEC COMPLIANCE:
- If the ORIGINAL REQUEST defines IN SCOPE / OUT OF SCOPE: follow strictly.
- Match the spec's package/directory structure EXACTLY. Do NOT reorganize or rename packages.
- Do NOT add layers, services, controllers, DTOs, or components the spec excludes.
- Do NOT use default package names (com.example, main) — use what the spec defines.
- Skill patterns are reference examples. Spec directives override any conflicting pattern.

WORKING DIRECTORY: All files MUST be created in ` + workDir + `. Do NOT create wrapper subdirectories.`

	// Load role-specific instructions from .claude/agents/{role}.md
	if roleInstr := LoadRoleInstructions(workDir, role); roleInstr != "" {
		prompt += "\n\n# Role Instructions\n" + roleInstr
	}

	// Inject extracted spec constraints into child agents
	if a.specConstraints != nil {
		if formatted := a.specConstraints.FormatConstraints(); formatted != "" {
			prompt += "\n\n" + formatted
		}
	}

	return prompt
}
