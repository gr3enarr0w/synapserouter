package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/mcp"
	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/orchestration"
	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/providers"
	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/tools"
)

// ChatExecutor can execute chat completions against the LLM provider chain.
type ChatExecutor interface {
	ChatCompletion(ctx context.Context, req providers.ChatRequest, sessionID string) (providers.ChatResponse, error)
}

// ProviderAwareExecutor extends ChatExecutor with the ability to target a specific provider.
type ProviderAwareExecutor interface {
	ChatExecutor
	ChatCompletionForProvider(ctx context.Context, req providers.ChatRequest, sessionID, provider string, includeDebug bool) (providers.ChatResponse, error)
}

// Agent implements the core agent loop: message -> LLM -> tool calls -> LLM -> response.
type Agent struct {
	executor     ChatExecutor
	registry     *tools.Registry
	permissions  *tools.PermissionChecker
	conversation *Conversation
	renderer     *Renderer
	config       Config
	sessionID    string

	// Sub-agent hierarchy
	mu       sync.Mutex
	parentID string
	children []*ChildRef
	budget   *BudgetTracker
	pool     *Pool

	// Guardrails
	inputGuardrails  *GuardrailChain
	outputGuardrails *GuardrailChain

	// Observability
	trace   *Trace
	metrics *AgentMetrics

	// Provider escalation
	providerIdx      int // index into config.EscalationChain levels
	levelRotationIdx int // tracks provider rotation within current escalation level

	// Pipeline state
	originalRequest    string // first user message
	toolCallCount      int    // total tool calls this session
	pipeline           *Pipeline
	pipelinePhase      int    // current phase index
	phaseToolCalls     int    // tool calls in current phase
	pipelineCycles     int    // how many times we've failed back to implement (cap at 3)
	acceptanceCriteria string // generated in plan phase
	cachedSystemPrompt string // built once, reused
	cachedSkillContext string // computed once from originalRequest, injected into all sub-agents
	skillContextOnce   sync.Once
}

// New creates an agent with the given executor, tool registry, and config.
func New(executor ChatExecutor, registry *tools.Registry, renderer *Renderer, config Config) *Agent {
	return &Agent{
		executor:     executor,
		registry:     registry,
		permissions:  tools.NewPermissionChecker(tools.ModeAutoApprove),
		conversation: NewConversation(),
		renderer:     renderer,
		config:       config,
		sessionID:    fmt.Sprintf("agent-%d", uniqueID()),
	}
}

// SetPermissions sets the permission checker for tool execution.
func (a *Agent) SetPermissions(pc *tools.PermissionChecker) {
	a.permissions = pc
}

// SetPool sets the agent pool for concurrency management.
func (a *Agent) SetPool(pool *Pool) {
	a.pool = pool
}

// SetInputGuardrails sets guardrails applied to user input.
func (a *Agent) SetInputGuardrails(gc *GuardrailChain) {
	a.inputGuardrails = gc
}

// SetOutputGuardrails sets guardrails applied to agent output.
func (a *Agent) SetOutputGuardrails(gc *GuardrailChain) {
	a.outputGuardrails = gc
}

// EnableTracing starts recording structured trace spans.
func (a *Agent) EnableTracing() {
	a.trace = NewTrace(a.sessionID, a.parentID)
}

// Trace returns the agent's trace, or nil if tracing is disabled.
func (a *Agent) Trace() *Trace {
	return a.trace
}

// SetMetrics attaches a metrics tracker to this agent.
func (a *Agent) SetMetrics(m *AgentMetrics) {
	a.metrics = m
}

// Metrics returns the agent's metrics tracker.
func (a *Agent) Metrics() *AgentMetrics {
	return a.metrics
}

// Run processes a user message through the agent loop and returns the final text response.
func (a *Agent) Run(ctx context.Context, userMessage string) (string, error) {
	// Input guardrails
	if a.inputGuardrails != nil {
		if result := a.inputGuardrails.Validate(userMessage); !result.Passed {
			return "", fmt.Errorf("input guardrail: %s", result.Reason)
		}
	}

	a.conversation.Add(providers.Message{
		Role:    "user",
		Content: userMessage,
	})

	// Capture original request for domain-specific review
	if a.originalRequest == "" {
		a.originalRequest = userMessage
	}

	// Initialize pipeline immediately on first message (if AutoOrchestrate enabled)
	// This fires parallel planners BEFORE the main agent starts making tool calls
	if a.config.AutoOrchestrate && a.pipeline == nil {
		matched := orchestration.MatchSkills(a.originalRequest, a.config.Skills)
		skillNames := make([]string, len(matched))
		for i, s := range matched {
			skillNames[i] = s.Name
		}
		a.pipeline = DetectPipelineType(skillNames)
		a.pipelinePhase = 0
		a.initializeImplementPhase()
		log.Printf("[Agent] pipeline: %s (%d phases)", a.pipeline.Name, len(a.pipeline.Phases))

		// Fire parallel plan phase immediately if configured
		firstPhase := a.pipeline.Phases[0]
		if firstPhase.ParallelSubAgents > 0 && len(firstPhase.CoderProviders) > 0 && a.hasProviders(firstPhase.CoderProviders) {
			log.Printf("[Agent] pipeline: firing parallel %s phase immediately", firstPhase.Name)
			parallelResult := a.runParallelPhase(firstPhase)
			if firstPhase.StoreAs == "criteria" {
				a.acceptanceCriteria = parallelResult
			}
			// Advance past the plan phase
			a.pipelinePhase = 1
			a.phaseToolCalls = 0
			// Inject the plan result + next phase prompt into conversation
			a.conversation.Add(providers.Message{
				Role:    "user",
				Content: fmt.Sprintf("The planning phase is complete. Here is the merged plan and acceptance criteria:\n\n%s\n\nNow proceed to implement. %s",
					parallelResult, a.pipeline.PhasePrompt(a.pipelinePhase, a.acceptanceCriteria)),
			})
		}
	}

	start := time.Now()
	response, err := a.loop(ctx)

	// Record metrics
	if a.metrics != nil {
		turns := len(a.conversation.Messages()) / 2 // rough estimate
		a.metrics.RecordRequest(time.Since(start), 0, turns, err != nil)
	}

	if err != nil {
		return "", err
	}

	// Output guardrails
	if a.outputGuardrails != nil {
		if result := a.outputGuardrails.Validate(response); !result.Passed {
			return "", fmt.Errorf("output guardrail: %s", result.Reason)
		}
	}

	return response, nil
}

// Clear resets the conversation history.
func (a *Agent) Clear() {
	a.conversation.Clear()
}

// SessionID returns the agent's session ID.
func (a *Agent) SessionID() string {
	return a.sessionID
}

func (a *Agent) loop(ctx context.Context) (string, error) {
	for turn := 0; a.config.MaxTurns <= 0 || turn < a.config.MaxTurns; turn++ {
		// Budget check
		if a.budget != nil {
			a.budget.RecordTurn()
			if reason := a.budget.Exceeded(); reason != "" {
				return "", fmt.Errorf("agent budget exceeded: %s", reason)
			}
		}

		// LLM call
		req := providers.ChatRequest{
			Model:    a.config.Model,
			Messages: a.buildMessages(),
			Tools:    a.registry.OpenAIToolDefinitions(),
		}

		var endSpan func(error)
		if a.trace != nil {
			endSpan = a.trace.StartSpan("llm_call", "llm_call", map[string]interface{}{"turn": turn})
		}
		resp, err := a.callLLMWithRetry(ctx, req)
		if endSpan != nil {
			endSpan(err)
		}
		if err != nil {
			return "", fmt.Errorf("LLM call failed: %w", err)
		}
		if a.budget != nil && resp.Usage.TotalTokens > 0 {
			a.budget.RecordTokens(int64(resp.Usage.TotalTokens))
		}
		if len(resp.Choices) == 0 {
			return "", fmt.Errorf("empty response from LLM")
		}

		msg := resp.Choices[0].Message
		if strings.TrimSpace(msg.Role) == "" {
			msg.Role = "assistant"
		}
		a.conversation.Add(msg)

		// No tool calls → pipeline decides next step
		if len(msg.ToolCalls) == 0 {
			if a.config.AutoOrchestrate && a.toolCallCount >= 3 {
				if a.advancePipeline(msg.Content) {
					continue
				}
			}
			return msg.Content, nil
		}

		// Execute tool calls
		a.toolCallCount += len(msg.ToolCalls)
		a.phaseToolCalls += len(msg.ToolCalls)
		a.executeToolCalls(ctx, msg.ToolCalls)
	}

	return "", fmt.Errorf("agent exceeded max turns (%d)", a.config.MaxTurns)
}

// executeToolCalls runs each tool call and adds results to conversation.
func (a *Agent) executeToolCalls(ctx context.Context, toolCalls []map[string]interface{}) {
	for _, toolCall := range toolCalls {
		callID := extractToolCallID(toolCall)
		name, args := extractToolCallNameArgs(toolCall)

		if a.renderer != nil {
			a.renderer.ToolCall(name, args)
		}

		toolStart := time.Now()
		result, execErr := a.registry.ExecuteChecked(ctx, name, args, a.config.WorkDir, a.permissions)
		toolDuration := time.Since(toolStart)

		if a.trace != nil {
			a.trace.AddSpan(Span{
				Name: name, Type: "tool_call",
				StartTime: toolStart, Duration: toolDuration,
				Metadata: map[string]interface{}{"args": args},
			})
		}
		if a.metrics != nil {
			a.metrics.RecordToolCall(name, toolDuration)
		}

		var resultContent string
		if execErr != nil {
			resultContent = fmt.Sprintf("error: %v\n%s", execErr, toolErrorHint(name))
			if a.renderer != nil {
				a.renderer.ToolResult(name, resultContent, true)
			}
		} else {
			resultContent = result.Output
			if result.Error != "" {
				resultContent = result.Error + "\n" + resultContent
			}
			if a.renderer != nil {
				a.renderer.ToolResult(name, resultContent, result.Error != "")
			}
		}

		// Truncate tool output for conversation (keep full output in renderer)
		conversationContent := truncateToolOutput(resultContent, 32*1024)

		a.conversation.Add(providers.Message{
			Role:       "tool",
			ToolCallID: callID,
			Content:    conversationContent,
		})
	}
}

func (a *Agent) buildMessages() []providers.Message {
	// Build system prompt once and cache it
	if a.cachedSystemPrompt == "" {
		sysPrompt := a.config.SystemPrompt
		if sysPrompt == "" {
			sysPrompt = defaultSystemPrompt(a.config.WorkDir)
		}

		// Inject matched skill instructions
		if skillCtx := a.matchedSkillContext(); skillCtx != "" {
			sysPrompt += "\n\n" + skillCtx
		}

		a.cachedSystemPrompt = sysPrompt
	}

	msgs := []providers.Message{{
		Role:    "system",
		Content: a.cachedSystemPrompt,
	}}
	msgs = append(msgs, a.conversation.Messages()...)
	return msgs
}

// matchedSkillContext matches the latest user message against skill triggers
// and returns the full instructions for all matched skills plus any MCP tool
// results, formatted for injection into the system prompt.
// Computed once from originalRequest and cached — all sub-agents get the same
// skill context regardless of how their task prompt is worded.
func (a *Agent) matchedSkillContext() string {
	a.skillContextOnce.Do(func() {
		a.cachedSkillContext = a.computeSkillContext()
	})
	return a.cachedSkillContext
}

// computeSkillContext does the actual skill matching and formatting.
func (a *Agent) computeSkillContext() string {
	if len(a.config.Skills) == 0 {
		return ""
	}

	// Use originalRequest for consistent skill matching throughout the session.
	// LastUserMessage() changes as the pipeline injects phase prompts, but skills
	// should always match against what the user actually asked for.
	query := a.originalRequest
	if query == "" {
		query = a.conversation.LastUserMessage()
	}
	if query == "" {
		return ""
	}

	matched := orchestration.MatchSkills(query, a.config.Skills)
	if len(matched) == 0 {
		return ""
	}

	chain := orchestration.BuildSkillChain(matched)

	var b strings.Builder
	b.WriteString("=== Active Skills ===\n")
	for _, skill := range chain {
		b.WriteString("\n## " + skill.Name + "\n")
		if skill.Instructions != "" {
			b.WriteString(skill.Instructions)
		} else {
			b.WriteString(skill.Description)
		}
		b.WriteString("\n")
	}
	b.WriteString("=== End Skills ===")

	// Auto-invoke MCP tools bound to matched skills
	if mcpCtx := a.invokeMCPToolsForSkills(chain, query); mcpCtx != "" {
		b.WriteString("\n\n" + mcpCtx)
	}

	return b.String()
}

// invokeMCPToolsForSkills calls MCP tools bound to the matched skill chain
// and returns formatted results. Gracefully skips if MCPClient is nil or
// individual tool calls fail.
func (a *Agent) invokeMCPToolsForSkills(chain []orchestration.Skill, query string) string {
	if a.config.MCPClient == nil {
		return ""
	}

	mcpTools := orchestration.MCPToolsForChain(chain)
	if len(mcpTools) == 0 {
		return ""
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var b strings.Builder
	b.WriteString("=== MCP Tool Results ===\n")
	hasResults := false

	for _, toolName := range mcpTools {
		result, err := a.config.MCPClient.CallTool(ctx, mcp.ToolCall{
			ToolName:  toolName,
			Arguments: map[string]interface{}{"query": query},
		})
		if err != nil {
			continue
		}
		if result == nil || !result.Success {
			continue
		}

		hasResults = true
		output, _ := json.Marshal(result.Output)
		b.WriteString(fmt.Sprintf("\n[MCP:%s]\n%s\n", toolName, string(output)))
	}

	b.WriteString("=== End MCP Results ===")

	if !hasResults {
		return ""
	}
	return b.String()
}

func defaultSystemPrompt(workDir string) string {
	return fmt.Sprintf(`You are a coding assistant that BUILDS tools and programs. You are working in: %s

TOOL BUILDER, NOT TOOL RUNNER:
- Your job is to BUILD programs and tools — not to DO the work yourself.
- When a task involves operations (API calls, data processing, sync, transforms):
  write a PROGRAM that does it. Do not do it manually with bash/curl.
- Use bash/curl ONLY for research (testing APIs, inspecting responses) and
  verification (running the built tool once to check it works).
- The deliverable is ALWAYS a runnable program, not a series of manual operations.
- After building: compile, test, run once to verify, deliver.

RESEARCH BEFORE CODING:
- When working with an unfamiliar API, library, or format: read docs, test with curl,
  inspect a real response BEFORE writing code against it.
- Never guess at auth methods, endpoints, or payload formats.
- When you encounter unknowns: STOP → RESEARCH → APPLY. Never guess and ship.

PRODUCTION QUALITY:
- Would a senior engineer ship this? No stubs, no flat structures, no missing edge cases.
- Show math for calculated values. Never approximate when exact values are available.
- Document assumptions. Flag ambiguous decisions for review.

AVAILABLE TOOLS (use exact argument names):
- bash: Run shell commands. Args: command (string, required), timeout (int, ms, optional).
  Use for: curl, compilation, running programs, system commands.
- file_read: Read file contents. Args: path (string, required), offset (int, line number), limit (int, max lines).
  Use for: reading source code, configs, logs. Prefer over bash+cat.
- file_write: Create or overwrite a file. Args: path (string, required), content (string, required).
  Use for: creating new files. Writes entire file content.
- file_edit: Edit specific text in a file. Args: path (string, required), old_text (string), new_text (string).
  Use for: modifying existing files. Prefer over file_write for small changes.
- grep: Search file contents recursively. Args: pattern (string, required), path (string), include (glob filter).
  Use for: finding code, imports, function definitions.
- glob: Find files matching a pattern. Args: pattern (string, required), path (string, base directory).
  Use for: discovering files, checking if files exist.
- git: Git operations. Args: subcommand (string, required), args ([]string).
  Use for: status, diff, log, add, commit. Blocked: push --force, branch -D.`, workDir)
}

// executeForCurrentProvider routes the LLM call to the current provider in the
// escalation chain. If not escalated (providerIdx == 0), uses default routing
// which lets the router pick the best provider. If escalated, targets a specific
// provider so the agent can use a different model for review/retry.
func (a *Agent) executeForCurrentProvider(ctx context.Context, req providers.ChatRequest) (providers.ChatResponse, error) {
	// If a target provider is set (sub-agent targeting specific Ollama model), use it directly
	if a.config.TargetProvider != "" {
		if pae, ok := a.executor.(ProviderAwareExecutor); ok {
			return pae.ChatCompletionForProvider(ctx, req, a.sessionID, a.config.TargetProvider, false)
		}
	}

	// When escalated, try providers at the current level, then walk up
	for a.providerIdx > 0 && a.providerIdx < len(a.config.EscalationChain) {
		level := a.config.EscalationChain[a.providerIdx]
		if pae, ok := a.executor.(ProviderAwareExecutor); ok {
			// Try each provider at this level
			for _, provider := range level.Providers {
				resp, err := pae.ChatCompletionForProvider(ctx, req, a.sessionID, provider, false)
				if err == nil {
					return resp, nil
				}
				log.Printf("[Agent] provider %s failed (%v), trying next", provider, err)
			}
			// All providers at this level failed, advance to next level
			a.providerIdx++
			continue
		}
		break
	}
	// Not escalated or all escalated providers exhausted — use default routing
	return a.executor.ChatCompletion(ctx, req, a.sessionID)
}

// advancePipeline checks the LLM's response and advances to the next pipeline
// phase, or sends back for fixes if the current phase failed. Returns true if
// a new phase prompt was injected (loop should continue), false if done.
func (a *Agent) advancePipeline(content string) bool {
	// Initialize pipeline on first call
	if a.pipeline == nil {
		matched := orchestration.MatchSkills(a.originalRequest, a.config.Skills)
		skillNames := make([]string, len(matched))
		for i, s := range matched {
			skillNames[i] = s.Name
		}
		a.pipeline = DetectPipelineType(skillNames)
		a.pipelinePhase = 0
		log.Printf("[Agent] pipeline: %s (%d phases)", a.pipeline.Name, len(a.pipeline.Phases))
	}

	if a.pipelinePhase >= len(a.pipeline.Phases) {
		return false // all phases done
	}

	currentPhase := a.pipeline.Phases[a.pipelinePhase]

	// shouldContinue only applies to implement phase — prevents false advances in review phases
	shouldAdvance := IsPassSignal(content)
	if !shouldAdvance && (currentPhase.Name == "implement" || currentPhase.Name == "data-prep" || currentPhase.Name == "model") {
		shouldAdvance = a.shouldContinue(content)
	}

	// Quality gate: reject phase transition if minimum tool calls not met
	if shouldAdvance && currentPhase.MinToolCalls > 0 && a.phaseToolCalls < currentPhase.MinToolCalls {
		log.Printf("[Agent] quality gate: phase %s needs %d tool calls, has %d — rejecting",
			currentPhase.Name, currentPhase.MinToolCalls, a.phaseToolCalls)
		a.conversation.Add(providers.Message{
			Role: "user",
			Content: fmt.Sprintf("You claimed phase '%s' is complete, but you only made %d tool calls (minimum %d required). You MUST use tools to gather evidence — fetch real data, inspect actual output, run tests. Do not state opinions without evidence. Use tools now, then re-assess.",
				currentPhase.Name, a.phaseToolCalls, currentPhase.MinToolCalls),
		})
		return true
	}

	// Check if current phase passed or failed
	if shouldAdvance {
		// Store acceptance criteria if this phase produces them
		if currentPhase.StoreAs == "criteria" {
			a.acceptanceCriteria = content
		}

		// Advance to next phase
		a.pipelinePhase++
		a.phaseToolCalls = 0 // reset for new phase
		if a.pipelinePhase >= len(a.pipeline.Phases) {
			log.Printf("[Agent] pipeline complete: all %d phases passed", len(a.pipeline.Phases))
			return false
		}

		nextPhase := a.pipeline.Phases[a.pipelinePhase]

		// After implement phase passes, advance providerIdx past Level 0 (coders)
		// to Level 1 (first review level). This ensures reviews use bigger models.
		if currentPhase.Name == "implement" && a.providerIdx == 0 && len(a.config.EscalationChain) > 1 {
			a.providerIdx = 1
			a.levelRotationIdx = 0
			log.Printf("[Agent] advanced past coder level to review level %d: %v",
				a.providerIdx, a.config.EscalationChain[a.providerIdx].Providers)
		}

		// Escalate provider if this phase requires it (stays escalated permanently)
		if nextPhase.Escalate {
			a.escalateProvider()
		}

		log.Printf("[Agent] pipeline: advancing to phase %d/%d: %s",
			a.pipelinePhase+1, len(a.pipeline.Phases), nextPhase.Name)

		// Parallel implement: spawn N coders working concurrently on split tasks
		// Only if the named providers exist in the escalation chain
		if nextPhase.ParallelSubAgents > 0 && len(nextPhase.CoderProviders) > 0 && a.hasProviders(nextPhase.CoderProviders) {
			parallelResult := a.runParallelPhase(nextPhase)
			a.conversation.Add(providers.Message{
				Role:    "user",
				Content: fmt.Sprintf("Parallel implementation complete. Results from %d agents:\n%s\nReview the combined output and say IMPLEMENT_COMPLETE if everything looks good, or fix any issues.", nextPhase.ParallelSubAgents, parallelResult),
			})
			return true
		}

		// Sub-agent phases: spawn a fresh agent with NO shared conversation
		if nextPhase.UseSubAgent {
			reviewResult := a.runSubAgentPhase(nextPhase)
			if IsPassSignal(reviewResult) {
				// Sub-agent approved — advance to next phase recursively
				a.conversation.Add(providers.Message{
					Role:    "user",
					Content: fmt.Sprintf("Independent %s passed:\n%s", nextPhase.Name, reviewResult),
				})
				return a.advancePipeline("PHASE_PASSED") // advance again
			}
			// Sub-agent found issues — escalate and fix on the CURRENT (bigger) model.
			// Never go back to small coders. Each cycle escalates further.
			a.pipelineCycles++
			if a.pipelineCycles > 8 {
				log.Printf("[Agent] pipeline: max review cycles reached (%d), accepting result", a.pipelineCycles)
				a.conversation.Add(providers.Message{
					Role: "user",
					Content: fmt.Sprintf("Review found issues but max cycles reached. Delivering current state:\n%s", reviewResult),
				})
				return a.advancePipeline("PHASE_PASSED")
			}
			// Escalate to next bigger model for the fix
			escalated := a.escalateProvider()
			providerName := "default"
			if a.providerIdx < len(a.config.EscalationChain) {
				level := a.config.EscalationChain[a.providerIdx]
				if len(level.Providers) > 0 {
					providerName = level.Providers[0]
				}
			}
			log.Printf("[Agent] pipeline: review cycle %d/8 — fixing on %s (provider idx %d, escalated=%v)",
				a.pipelineCycles, providerName, a.providerIdx, escalated)
			a.conversation.Add(providers.Message{
				Role: "user",
				Content: fmt.Sprintf("The %s review found issues (cycle %d/8). Fix ALL these issues using tools, then say IMPLEMENT_COMPLETE:\n---\n%s", nextPhase.Name, a.pipelineCycles, reviewResult),
			})
			// Go back to self-check — after the fix, self-check re-runs, then code-review with next reviewer
			a.pipelinePhase = a.findPhaseIndex("self-check", a.pipelinePhase-1)
			a.phaseToolCalls = 0
			return true
		}

		prompt := a.pipeline.PhasePrompt(a.pipelinePhase, a.acceptanceCriteria)
		a.conversation.Add(providers.Message{
			Role:    "user",
			Content: prompt,
		})
		return true
	}

	if IsFailSignal(content) {
		a.pipelineCycles++
		if a.pipelineCycles > 8 {
			log.Printf("[Agent] pipeline: max review cycles reached, accepting result")
			return false
		}
		a.escalateProvider()
		log.Printf("[Agent] pipeline: phase %s FAILED (cycle %d/8), escalated to provider idx %d",
			currentPhase.Name, a.pipelineCycles, a.providerIdx)
		a.phaseToolCalls = 0

		a.conversation.Add(providers.Message{
			Role: "user",
			Content: fmt.Sprintf("The %s phase found issues (cycle %d/8). Fix them now using tools, then say IMPLEMENT_COMPLETE.", currentPhase.Name, a.pipelineCycles),
		})
		return true
	}

	// Not a clear pass/fail — this is the first time entering the pipeline
	log.Printf("[Agent] pipeline: starting phase %d/%d: %s",
		a.pipelinePhase+1, len(a.pipeline.Phases), currentPhase.Name)

	// Parallel phase starting: spawn sub-agents immediately (only if providers exist)
	if currentPhase.ParallelSubAgents > 0 && len(currentPhase.CoderProviders) > 0 && a.hasProviders(currentPhase.CoderProviders) {
		parallelResult := a.runParallelPhase(currentPhase)
		if currentPhase.StoreAs == "criteria" {
			a.acceptanceCriteria = parallelResult
		}
		a.conversation.Add(providers.Message{
			Role:    "user",
			Content: fmt.Sprintf("Parallel %s phase complete. Results from %d agents:\n%s\nReview and say %s_COMPLETE.", currentPhase.Name, currentPhase.ParallelSubAgents, parallelResult, strings.ToUpper(currentPhase.Name)),
		})
		return true
	}

	prompt := a.pipeline.PhasePrompt(a.pipelinePhase, a.acceptanceCriteria)
	a.conversation.Add(providers.Message{
		Role:    "user",
		Content: prompt,
	})
	return true
}

// runSubAgentPhase spawns fresh sub-agent(s) with NO shared conversation context
// to independently review the work. When a level has multiple providers, runs
// sequential review→fix stages: A reviews → B fixes → C reviews B's fix.
// Returns the final sub-agent's result text.
func (a *Agent) runSubAgentPhase(phase PipelinePhase) string {
	ctx := context.Background()
	model := "auto"

	// Dynamically match skills and extract verification commands (single matching pass)
	query := a.originalRequest
	if query == "" {
		query = a.conversation.LastUserMessage()
	}
	matched := orchestration.MatchSkills(query, a.config.Skills)
	chain := orchestration.BuildSkillChain(matched)
	skillContext := a.matchedSkillContext()
	verifyCommands := orchestration.VerifyCommandsForChain(chain)

	verifySection := ""
	if verifyCommands != "" {
		verifySection = fmt.Sprintf(`
VERIFICATION COMMANDS — Run ALL of these using bash and report PASS/FAIL for each.
Commands marked [MANUAL] require reading code instead of running a command.
%s`, verifyCommands)
	}

	// Determine providers for this level
	var levelProviders []string
	if a.providerIdx < len(a.config.EscalationChain) {
		levelProviders = a.config.EscalationChain[a.providerIdx].Providers
	}

	// Single provider (or no chain): original behavior — one reviewer
	if len(levelProviders) <= 1 {
		targetProvider := ""
		if len(levelProviders) == 1 {
			targetProvider = levelProviders[0]
		}
		log.Printf("[Agent] targeting chain level %d: %s (single provider)",
			a.providerIdx, targetProvider)

		result, err := a.runSingleReviewer(ctx, phase, model, targetProvider, skillContext, verifySection)
		if err != nil {
			return fmt.Sprintf("NEEDS_FIX: sub-agent error: %v", err)
		}
		return result
	}

	// Multi-provider level: sequential review→fix rotation
	// Stage pattern: A reviews → B fixes A's issues → C reviews B's fix → ...
	// Uses levelRotationIdx to start from where we left off last cycle
	n := len(levelProviders)
	startIdx := a.levelRotationIdx % n
	log.Printf("[Agent] multi-provider %s: level %d has %d providers, starting at rotation %d",
		phase.Name, a.providerIdx, n, startIdx)

	var lastResult string

	for step := 0; step < n; step++ {
		provIdx := (startIdx + step) % n
		provider := levelProviders[provIdx]

		var task string
		if step == 0 {
			// First provider: review (same as single reviewer)
			task = fmt.Sprintf(`You are an INDEPENDENT reviewer (agent %d/%d) with NO context from the implementation.
You must evaluate the work FRESH — do not assume anything is correct.

ORIGINAL REQUEST:
---
%s
---

ACCEPTANCE CRITERIA:
---
%s
---

SKILLS TO CHECK AGAINST:
%s
%s
Your job:
1. Run EVERY verification command listed above. Report the output and PASS/FAIL for each.
2. Use tools (file_read, grep, bash) to inspect ALL actual outputs. Trust NOTHING without evidence.
3. Compare each output against the original request and acceptance criteria.
4. For [MANUAL] checks: read the relevant code and verify the stated condition.
5. Check for: null values, zero values, empty fields, missing structure.
6. Say VERIFIED_CORRECT only if ALL verification commands pass AND all criteria are met.
   Otherwise say NEEDS_FIX with every specific issue listed.`,
				step+1, n, a.originalRequest, a.acceptanceCriteria, skillContext, verifySection)
		} else if step%2 == 1 {
			// Odd steps: fix issues found by previous reviewer
			prevReview := lastResult
			if len(prevReview) > 3000 {
				prevReview = prevReview[:3000] + "\n[...truncated]"
			}
			task = fmt.Sprintf(`You are agent %d/%d. The previous agent REVIEWED the code and found issues.
Fix ALL issues listed below.

ORIGINAL REQUEST:
---
%s
---

ACCEPTANCE CRITERIA:
---
%s
---

REVIEW FINDINGS TO FIX:
---
%s
---

SKILL REFERENCE:
%s

Fix every issue. Run verification commands to confirm fixes work.
Say VERIFIED_CORRECT if all fixed, or NEEDS_FIX if you couldn't fix something.`,
				step+1, n, a.originalRequest, a.acceptanceCriteria, prevReview, skillContext)
		} else {
			// Even steps (2, 4, ...): review the previous agent's fixes
			prevFix := lastResult
			if len(prevFix) > 3000 {
				prevFix = prevFix[:3000] + "\n[...truncated]"
			}
			task = fmt.Sprintf(`You are agent %d/%d. The previous agent attempted FIXES. Review their work independently.

ORIGINAL REQUEST:
---
%s
---

ACCEPTANCE CRITERIA:
---
%s
---

PREVIOUS AGENT'S FIX REPORT:
---
%s
---

SKILLS TO CHECK AGAINST:
%s
%s
Verify the fixes are correct. Run ALL verification commands.
Say VERIFIED_CORRECT if everything passes, or NEEDS_FIX with remaining issues.`,
				step+1, n, a.originalRequest, a.acceptanceCriteria, prevFix, skillContext, verifySection)
		}

		log.Printf("[Agent] sub-agent %s step %d/%d: provider %s (%s)",
			phase.Name, step+1, n, provider,
			map[bool]string{true: "review", false: "fix"}[step%2 == 0])

		result, err := a.RunChild(ctx, SpawnConfig{
			Role:     fmt.Sprintf("%s-step-%d", phase.Name, step+1),
			Model:    model,
			Provider: provider,
		}, task)

		if err != nil {
			log.Printf("[Agent] sub-agent step %d failed: %v", step+1, err)
			lastResult = fmt.Sprintf("NEEDS_FIX: agent %d error: %v", step+1, err)
			continue
		}

		preview := result
		if len(preview) > 100 {
			preview = preview[:100]
		}
		log.Printf("[Agent] sub-agent step %d completed: %s", step+1, preview)
		lastResult = result

		// If a reviewer says VERIFIED_CORRECT, no need to continue — exit early
		if step%2 == 0 && IsPassSignal(result) {
			log.Printf("[Agent] step %d verified correct, skipping remaining steps", step+1)
			break
		}
	}

	a.levelRotationIdx += n // advance rotation for next cycle
	return lastResult
}

// runSingleReviewer runs one independent reviewer sub-agent (used when level has 1 provider).
func (a *Agent) runSingleReviewer(ctx context.Context, phase PipelinePhase, model, provider, skillContext, verifySection string) (string, error) {
	task := fmt.Sprintf(`You are an INDEPENDENT reviewer with NO context from the implementation.
You must evaluate the work FRESH — do not assume anything is correct.

ORIGINAL REQUEST:
---
%s
---

ACCEPTANCE CRITERIA:
---
%s
---

SKILLS TO CHECK AGAINST:
%s
%s
Your job:
1. Run EVERY verification command listed above. Report the output and PASS/FAIL for each.
2. Use tools (file_read, grep, bash) to inspect ALL actual outputs. Trust NOTHING without evidence.
3. Compare each output against the original request and acceptance criteria.
4. For [MANUAL] checks: read the relevant code and verify the stated condition.
5. Check for: null values, zero values, empty fields, missing structure.
6. Say VERIFIED_CORRECT only if ALL verification commands pass AND all criteria are met.
   Otherwise say NEEDS_FIX with every specific issue listed.`,
		a.originalRequest, a.acceptanceCriteria, skillContext, verifySection)

	log.Printf("[Agent] spawning independent %s sub-agent (no shared context)", phase.Name)

	result, err := a.RunChild(ctx, SpawnConfig{
		Role:     phase.Name,
		Model:    model,
		Provider: provider,
	}, task)

	if err != nil {
		log.Printf("[Agent] sub-agent %s failed: %v", phase.Name, err)
		return "", err
	}

	preview := result
	if len(preview) > 100 {
		preview = preview[:100]
	}
	log.Printf("[Agent] sub-agent %s completed: %s", phase.Name, preview)
	return result, nil
}

// escalateProvider moves to the next provider in the escalation chain.
// Returns true if a new provider was selected, false if already at the end.
// Never over-increments past the last provider — stays capped at the most capable.
func (a *Agent) escalateProvider() bool {
	if len(a.config.EscalationChain) == 0 {
		return false
	}
	if a.providerIdx >= len(a.config.EscalationChain)-1 {
		level := a.config.EscalationChain[a.providerIdx]
		log.Printf("[Agent] escalation chain exhausted — staying on %v (level %d)",
			level.Providers, a.providerIdx)
		return false
	}

	a.providerIdx++
	a.levelRotationIdx = 0
	level := a.config.EscalationChain[a.providerIdx]
	log.Printf("[Agent] escalating to level %d/%d: %v",
		a.providerIdx+1, len(a.config.EscalationChain), level.Providers)
	return true
}

// initializeImplementPhase populates the implement phase's CoderProviders
// from the escalation chain Level 0. Called after pipeline init so that
// hardcoded values aren't needed in pipeline.go.
func (a *Agent) initializeImplementPhase() {
	if a.pipeline == nil || len(a.config.EscalationChain) == 0 {
		return
	}
	level0 := a.config.EscalationChain[0]
	for i := range a.pipeline.Phases {
		phase := &a.pipeline.Phases[i]
		if phase.Name == "implement" && phase.ParallelSubAgents == 0 && len(phase.CoderProviders) == 0 {
			if len(level0.Providers) > 0 {
				phase.ParallelSubAgents = len(level0.Providers)
				phase.CoderProviders = level0.Providers
				log.Printf("[Agent] implement phase: %d parallel coders from chain level 0: %v",
					phase.ParallelSubAgents, phase.CoderProviders)
			}
		}
	}
}

// findPhaseIndex returns the index of a phase by name, or fallback if not found.
func (a *Agent) findPhaseIndex(name string, fallback int) int {
	for i, p := range a.pipeline.Phases {
		if p.Name == name {
			return i
		}
	}
	if fallback >= 0 && fallback < len(a.pipeline.Phases) {
		return fallback
	}
	return 0
}

// hasProviders checks if all named providers exist in the escalation chain.
// Returns false if any are missing, causing parallel phases to fall back to sequential.
// hasProviders checks if all named providers exist in any escalation level.
func (a *Agent) hasProviders(names []string) bool {
	chainSet := make(map[string]bool)
	for _, level := range a.config.EscalationChain {
		for _, p := range level.Providers {
			chainSet[p] = true
		}
	}
	for _, name := range names {
		if !chainSet[name] {
			return false
		}
	}
	return true
}

// shouldContinue detects if the LLM's text response signals intent to
// keep working (e.g. "Let me start implementing" or "I'll begin by").
// This prevents premature exit when the model outputs a plan before acting.
func (a *Agent) shouldContinue(content string) bool {
	lower := strings.ToLower(content)
	continuationSignals := []string{
		"let me start", "let me begin", "i'll start", "i'll begin",
		"i will start", "i will begin", "let's start", "let's begin",
		"now i'll", "now let me", "starting with", "beginning with",
		"first, i'll", "first, let me", "let me implement",
		"i'll implement", "let me create", "i'll create",
		"let me build", "i'll build", "moving to phase",
		"proceeding with", "let me proceed",
	}
	for _, signal := range continuationSignals {
		if strings.Contains(lower, signal) {
			return true
		}
	}
	return false
}

func extractToolCallID(tc map[string]interface{}) string {
	if id, ok := tc["id"].(string); ok && id != "" {
		return id
	}
	if id, ok := tc["call_id"].(string); ok && id != "" {
		return id
	}
	return ""
}

func extractToolCallNameArgs(tc map[string]interface{}) (string, map[string]interface{}) {
	fn, _ := tc["function"].(map[string]interface{})
	name := ""
	if fn != nil {
		name, _ = fn["name"].(string)
	}
	if name == "" {
		name, _ = tc["name"].(string)
	}

	var args map[string]interface{}
	if fn != nil {
		args = parseArguments(fn["arguments"])
	}
	if args == nil {
		args = parseArguments(tc["arguments"])
	}
	if args == nil {
		args = make(map[string]interface{})
	}
	return name, args
}

// runParallelPhase spawns N parallel sub-agents working concurrently.
// For plan phases: each produces a plan, then MergeProvider synthesizes.
// For implement phases: dynamic role assignment — agent 1 implements, agent 2 tests,
// agent 3+ bug-reviews. Stage 2 cross-review: each model reviews another's output.
func (a *Agent) runParallelPhase(phase PipelinePhase) string {
	// Use a timeout context rather than background to prevent runaway sub-agents
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	n := phase.ParallelSubAgents
	if n > len(phase.CoderProviders) {
		n = len(phase.CoderProviders)
	}

	// Build task descriptions based on phase type
	type taskDef struct {
		role, provider, task, workDir string
	}
	var tasks []taskDef

	// Dynamically match skills against the original request — same skills
	// the parent agent has, passed to every sub-agent for domain awareness
	skillContext := a.matchedSkillContext()

	if phase.Name == "plan" {
		for i := 0; i < n; i++ {
			tasks = append(tasks, taskDef{
				role:     fmt.Sprintf("planner-%d", i+1),
				provider: phase.CoderProviders[i],
				task: fmt.Sprintf(`You are a planning agent. Create a detailed implementation plan for this task.

TASK:
---
%s
---

SKILL REFERENCE (use these as authoritative guides for formats, APIs, patterns):
%s

Produce:
1. TASK DECOMPOSITION: ordered subtasks with dependencies
2. ACCEPTANCE CRITERIA for each subtask AND overall deliverable
   - Reference the skill documentation above for correct formats, APIs, and patterns
3. UNKNOWNS and ASSUMPTIONS
4. DEFINITION OF DONE

Be thorough and specific. Output your complete plan, then say PLAN_COMPLETE.`, a.originalRequest, skillContext),
			})
		}
	} else {
		// Dynamic role assignment: roles rotate by index, same pattern at every level
		rolePrompts := []string{
			// Role 0: implement
			`You are agent %d of %d working in parallel. Your role: IMPLEMENT the main code.

ORIGINAL REQUEST:
---
%s
---

ACCEPTANCE CRITERIA:
---
%s
---

SKILL REFERENCE (follow these exactly for formats, APIs, patterns):
%s

Focus on: main implementation files, data structures, core logic, API integration.
Follow the skill reference documentation precisely.
Do NOT write tests — another agent handles that concurrently.
When done, say IMPLEMENT_COMPLETE.`,
			// Role 1: test
			`You are agent %d of %d working in parallel. Your role: Write comprehensive TESTS.

ORIGINAL REQUEST:
---
%s
---

ACCEPTANCE CRITERIA:
---
%s
---

SKILL REFERENCE (follow these for testing patterns and expected formats):
%s

Focus on: unit tests, edge cases, integration tests, test fixtures.
The main implementation is being written by another agent concurrently.
Write tests based on the requirements and skill reference, not the implementation.
When done, say IMPLEMENT_COMPLETE.`,
			// Role 2+: review
			`You are agent %d of %d working in parallel. Your role: BUG REVIEW.

ORIGINAL REQUEST:
---
%s
---

ACCEPTANCE CRITERIA:
---
%s
---

SKILL REFERENCE (check code against these patterns):
%s

Focus on: reading all code written so far, finding bugs, logic errors, missing edge cases,
violations of the skill reference patterns, and potential runtime failures.
List every issue found with file path and line. Do NOT fix — just report.
When done, say IMPLEMENT_COMPLETE.`,
		}

		for i := 0; i < n; i++ {
			roleIdx := i
			if roleIdx >= len(rolePrompts) {
				roleIdx = 2 // additional agents get review role
			}
			tasks = append(tasks, taskDef{
				role:     fmt.Sprintf("agent-%d", i+1),
				provider: phase.CoderProviders[i],
				task:     fmt.Sprintf(rolePrompts[roleIdx], i+1, n, a.originalRequest, a.acceptanceCriteria, skillContext),
			})
		}
	}

	log.Printf("[Agent] spawning %d parallel %s sub-agents", len(tasks), phase.Name)

	// For implement phases, give each agent its own temp directory to prevent file conflicts
	if phase.Name != "plan" {
		for i := range tasks {
			subDir, err := os.MkdirTemp("", fmt.Sprintf("synroute-parallel-%s-*", tasks[i].role))
			if err != nil {
				log.Printf("[Agent] failed to create temp dir for %s: %v, using shared dir", tasks[i].role, err)
				continue
			}
			copyDirContents(a.config.WorkDir, subDir)
			tasks[i].workDir = subDir
			log.Printf("[Agent] parallel agent %s using isolated dir: %s", tasks[i].role, subDir)
		}
		// Cleanup temp dirs after merge (defer only for cleanup, merge happens below)
		defer func() {
			for _, t := range tasks {
				if t.workDir != "" {
					os.RemoveAll(t.workDir)
				}
			}
		}()
	}

	// Run in parallel
	type result struct {
		role, output string
		err          error
	}
	results := make(chan result, len(tasks))

	for _, t := range tasks {
		go func(role, provider, task, workDir string) {
			defer func() {
				if r := recover(); r != nil {
					results <- result{role: role, err: fmt.Errorf("panic in sub-agent: %v", r)}
				}
			}()
			cfg := SpawnConfig{
				Role:     role,
				Provider: provider,
			}
			if workDir != "" {
				cfg.WorkDir = workDir
			}
			out, err := a.RunChild(ctx, cfg, task)
			results <- result{role: role, output: out, err: err}
		}(t.role, t.provider, t.task, t.workDir)
	}

	// Collect results
	var combined strings.Builder
	for i := 0; i < len(tasks); i++ {
		r := <-results
		combined.WriteString(fmt.Sprintf("\n=== %s ===\n", r.role))
		if r.err != nil {
			combined.WriteString(fmt.Sprintf("ERROR: %v\n", r.err))
			log.Printf("[Agent] parallel sub-agent %s failed: %v", r.role, r.err)
		} else {
			if len(r.output) > 4000 {
				combined.WriteString(r.output[:4000] + "\n[...truncated]")
			} else {
				combined.WriteString(r.output)
			}
			log.Printf("[Agent] parallel sub-agent %s completed", r.role)
		}
	}

	// Merge files from temp dirs back to parent WorkDir (before returning results)
	for _, t := range tasks {
		if t.workDir != "" {
			mergeParallelDir(t.workDir, a.config.WorkDir)
		}
	}

	// Stage 2: Cross-review — each model reviews a different model's output.
	// Only for non-plan phases with 2+ models. Models rotate: agent[i] reviews agent[(i-1+N)%N].
	if phase.Name != "plan" && n >= 2 {
		log.Printf("[Agent] Stage 2: cross-review with %d agents (rotated)", n)

		// Collect Stage 1 outputs keyed by role for rotation
		stage1Outputs := make(map[string]string)
		for _, t := range tasks {
			// Find this task's output in combined (parse it back out)
			stage1Outputs[t.role] = t.task // fallback
		}
		// Re-parse combined output by role sections
		combinedStr := combined.String()
		for i, t := range tasks {
			start := strings.Index(combinedStr, fmt.Sprintf("=== %s ===", t.role))
			if start < 0 {
				continue
			}
			start += len(fmt.Sprintf("=== %s ===\n", t.role))
			end := len(combinedStr)
			if i+1 < len(tasks) {
				nextMarker := fmt.Sprintf("\n=== %s ===", tasks[(i+1)%len(tasks)].role)
				if idx := strings.Index(combinedStr[start:], nextMarker); idx >= 0 {
					end = start + idx
				}
			}
			stage1Outputs[t.role] = combinedStr[start:end]
		}

		crossResults := make(chan result, n)
		for i := 0; i < n; i++ {
			reviewIdx := (i - 1 + n) % n // agent[i] reviews agent[i-1]'s work
			reviewTarget := tasks[reviewIdx].role
			reviewOutput := stage1Outputs[reviewTarget]
			if len(reviewOutput) > 3000 {
				reviewOutput = reviewOutput[:3000] + "\n[...truncated]"
			}

			crossTask := fmt.Sprintf(`You are agent %d of %d in a CROSS-REVIEW round. Review the work of %s.

ORIGINAL REQUEST:
---
%s
---

ACCEPTANCE CRITERIA:
---
%s
---

SKILL REFERENCE (check code against these patterns and formats):
%s

WORK TO REVIEW (from %s):
---
%s
---

Your job:
1. Find bugs, logic errors, missing edge cases, incorrect patterns
2. Check against acceptance criteria AND skill reference — does this work meet them?
3. List every issue with specifics (file, line, what's wrong)
4. If you find issues, fix them directly in the codebase
5. Say IMPLEMENT_COMPLETE when done.`, i+1, n, reviewTarget,
				a.originalRequest, a.acceptanceCriteria,
				skillContext,
				reviewTarget, reviewOutput)

			go func(idx int, task string) {
				defer func() {
					if r := recover(); r != nil {
						crossResults <- result{role: fmt.Sprintf("cross-review-%d", idx+1), err: fmt.Errorf("panic: %v", r)}
					}
				}()
				out, err := a.RunChild(ctx, SpawnConfig{
					Role:     fmt.Sprintf("cross-review-%d", idx+1),
					Provider: phase.CoderProviders[idx],
				}, task)
				crossResults <- result{role: fmt.Sprintf("cross-review-%d", idx+1), output: out, err: err}
			}(i, crossTask)
		}

		// Collect cross-review results
		combined.WriteString("\n\n--- STAGE 2: CROSS-REVIEW ---\n")
		for i := 0; i < n; i++ {
			r := <-crossResults
			combined.WriteString(fmt.Sprintf("\n=== %s ===\n", r.role))
			if r.err != nil {
				combined.WriteString(fmt.Sprintf("ERROR: %v\n", r.err))
				log.Printf("[Agent] cross-review %s failed: %v", r.role, r.err)
			} else {
				if len(r.output) > 4000 {
					combined.WriteString(r.output[:4000] + "\n[...truncated]")
				} else {
					combined.WriteString(r.output)
				}
				log.Printf("[Agent] cross-review %s completed", r.role)
			}
		}
	}

	// Merge via MergeProvider if configured (e.g., Codex synthesizes 2 plans)
	if phase.MergeProvider != "" {
		log.Printf("[Agent] merging parallel results via %s", phase.MergeProvider)
		mergeTask := fmt.Sprintf(`Multiple models produced independent plans for the same task. Synthesize the BEST plan by:
1. Taking the strongest acceptance criteria from each
2. Combining the most thorough task decomposition
3. Including ALL unknowns and assumptions from both
4. Resolving any contradictions by picking the more detailed/correct approach
5. Ensuring the plan references the skill documentation below for correct formats and patterns

ORIGINAL TASK:
---
%s
---

PLANS FROM MULTIPLE MODELS:
%s

SKILL REFERENCE (the merged plan MUST incorporate these):
%s

Output the MERGED plan with complete acceptance criteria that reference the skill specs. Say PLAN_COMPLETE.`, a.originalRequest, combined.String(), skillContext)

		merged, err := a.RunChild(ctx, SpawnConfig{
			Role:     "plan-merger",
			Provider: phase.MergeProvider,
		}, mergeTask)
		if err != nil {
			log.Printf("[Agent] merge via %s failed: %v, using combined output", phase.MergeProvider, err)
			return combined.String()
		}
		log.Printf("[Agent] plan merge completed via %s", phase.MergeProvider)
		return merged
	}

	return combined.String()
}

// callLLMWithRetry attempts the LLM call up to 3 times with exponential backoff
// for transient errors (network, 429, 500). Non-retryable errors fail immediately.
func (a *Agent) callLLMWithRetry(ctx context.Context, req providers.ChatRequest) (providers.ChatResponse, error) {
	backoffs := []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second}
	var lastErr error

	for attempt := 0; attempt <= len(backoffs); attempt++ {
		resp, err := a.executeForCurrentProvider(ctx, req)
		if err == nil {
			return resp, nil
		}
		lastErr = err

		if !isRetryableError(err) {
			return resp, err
		}

		if attempt < len(backoffs) {
			log.Printf("[Agent] LLM call failed (attempt %d/%d): %v — retrying in %v",
				attempt+1, len(backoffs)+1, err, backoffs[attempt])
			select {
			case <-ctx.Done():
				return providers.ChatResponse{}, ctx.Err()
			case <-time.After(backoffs[attempt]):
			}
		}
	}
	return providers.ChatResponse{}, lastErr
}

// isRetryableError returns true for transient errors worth retrying.
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	// Network errors
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	// Rate limits and server errors
	for _, signal := range []string{"429", "rate limit", "too many requests", "500", "502", "503", "504", "timeout", "connection reset", "connection refused", "eof"} {
		if strings.Contains(msg, signal) {
			return true
		}
	}
	return false
}

// truncateToolOutput caps tool output at maxBytes for conversation context.
// The full output is still shown to the user via the renderer.
func truncateToolOutput(content string, maxBytes int) string {
	if len(content) <= maxBytes {
		return content
	}
	lines := strings.Split(content, "\n")
	var b strings.Builder
	lineCount := 0
	for _, line := range lines {
		if b.Len()+len(line)+1 > maxBytes {
			break
		}
		if lineCount > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(line)
		lineCount++
	}
	b.WriteString(fmt.Sprintf("\n\n[...truncated, showing first %d of %d total lines]", lineCount, len(lines)))
	return b.String()
}

// toolErrorHint returns a recovery hint for failed tool calls.
func toolErrorHint(toolName string) string {
	switch toolName {
	case "bash":
		return "Hint: Try a different command, check the path, or verify the program exists."
	case "file_read":
		return "Hint: Check if the file exists using glob first. Verify the path is correct."
	case "file_write":
		return "Hint: Ensure the parent directory exists. Check permissions."
	case "file_edit":
		return "Hint: Read the file first to verify the exact text to replace. old_text must match exactly."
	case "grep":
		return "Hint: Try a broader pattern or check the search path. Use glob to find files first."
	case "glob":
		return "Hint: Try a broader pattern (e.g., **/*.go) or check the base directory."
	case "git":
		return "Hint: Check git status first. Ensure you're in a git repository."
	default:
		return "Hint: Try a different approach or use a different tool."
	}
}

func parseArguments(raw interface{}) map[string]interface{} {
	switch v := raw.(type) {
	case map[string]interface{}:
		return v
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}
		var decoded map[string]interface{}
		if err := json.Unmarshal([]byte(v), &decoded); err == nil {
			return decoded
		}
	}
	return nil
}

var idCounter int64

// copyDirContents copies files from src to dst, skipping .git and heavy directories.
func copyDirContents(src, dst string) {
	skipDirs := map[string]bool{".git": true, "node_modules": true, "vendor": true, "__pycache__": true, ".build": true, "target": true}
	filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil || path == src {
			return nil
		}
		// Skip symlinks
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		rel, relErr := filepath.Rel(src, path)
		if relErr != nil {
			return nil
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			if mkErr := os.MkdirAll(filepath.Join(dst, rel), 0755); mkErr != nil {
				log.Printf("[Agent] copyDir: failed to create dir %s: %v", rel, mkErr)
			}
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			log.Printf("[Agent] copyDir: failed to read %s: %v", rel, readErr)
			return nil
		}
		if writeErr := os.WriteFile(filepath.Join(dst, rel), data, 0644); writeErr != nil {
			log.Printf("[Agent] copyDir: failed to write %s: %v", rel, writeErr)
		}
		return nil
	})
}

// mergeParallelDir copies new/modified files from subDir back to parentDir.
// Validates that output paths stay within parentDir (path traversal protection).
func mergeParallelDir(subDir, parentDir string) {
	cleanParent := filepath.Clean(parentDir) + string(os.PathSeparator)
	merged := 0
	filepath.WalkDir(subDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || path == subDir {
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		rel, relErr := filepath.Rel(subDir, path)
		if relErr != nil {
			return nil
		}
		dstPath := filepath.Join(parentDir, rel)
		// Path traversal protection: ensure dst stays within parentDir
		if !strings.HasPrefix(filepath.Clean(dstPath), cleanParent) {
			log.Printf("[Agent] mergeDir: skipping path traversal attempt: %s", rel)
			return nil
		}
		if mkErr := os.MkdirAll(filepath.Dir(dstPath), 0755); mkErr != nil {
			log.Printf("[Agent] mergeDir: failed to create dir for %s: %v", rel, mkErr)
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			log.Printf("[Agent] mergeDir: failed to read %s: %v", rel, readErr)
			return nil
		}
		if writeErr := os.WriteFile(dstPath, data, 0644); writeErr != nil {
			log.Printf("[Agent] mergeDir: failed to write %s: %v", rel, writeErr)
			return nil
		}
		merged++
		return nil
	})
	if merged > 0 {
		log.Printf("[Agent] merged %d files from %s to %s", merged, filepath.Base(subDir), filepath.Base(parentDir))
	}
}

func uniqueID() int64 {
	return atomic.AddInt64(&idCounter, 1)
}
