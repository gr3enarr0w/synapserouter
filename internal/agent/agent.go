package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/environment"
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
	permissions       *tools.PermissionChecker
	permissionPrompt tools.PermissionPromptFunc
	conversation *Conversation
	renderer     TerminalRenderer
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

	// Provider escalation — providerIdx is MONOTONICALLY INCREASING.
	// Once escalated to a level, never goes back to a lower one.
	// All mutations MUST go through setMinProviderLevel().
	providerIdx      int // index into config.EscalationChain levels
	levelRotationIdx int // tracks provider rotation within current escalation level

	// Pipeline state
	originalRequest    string // first user message
	toolCallCount      int    // total tool calls this session
	wroteCodeFiles     bool   // true if file_write or file_edit was used on code files
	completionVerifyDone bool // true after completion verification ran (prevents retry loops)
	pipeline           *Pipeline
	pipelinePhase      int    // current phase index
	phaseToolCalls     int    // tool calls in current phase
	pipelineCycles     int    // how many times we've failed back to implement (cap at 3)
	acceptanceCriteria string // generated in plan phase
	cachedSystemPrompt string // built once, reused
	cachedSkillContext string // computed once from originalRequest, injected into all sub-agents
	skillContextOnce   sync.Once
	noToolTurns        int // consecutive turns without tool calls (stall detection)
	textContinuations  int // how many times we've continued past text-only turns (cap at 2)
	reviewTracker      *ReviewCycleTracker // detects stable review cycles (no progress)
	phaseRetries       int // consecutive quality gate rejections in current phase
	phaseTurns         int // turns spent in current phase (hard cap at maxPhaseTurns)
	lastGateScore      int // verification gate passed count from previous retry (plateau detection)
	plateauCount       int // consecutive retries with no score improvement
	cachedPromptLevel  int      // provider level when system prompt was cached (-1 = uncached)
	toolFingerprints   []string // sliding window of recent tool call fingerprints (loop detection)
	loopWarningCount   int      // consecutive loop warnings without resolution (escalation trigger)

	// Context retrieval after compaction
	hasCompacted bool // set true after compactConversation; triggers auto-context injection

	// Hallucination detection
	factTracker              *FactTracker // in-memory ground-truth accumulator
	hallucinationRecallCount int          // consecutive auto-corrections (rate limited at 3)

	// Toolchain detection
	toolchainSetup    string // install instructions for missing tools
	resolvedBuildCmds string // resolved build/test/install commands for detected language

	// File read dedup cache — avoids re-reading unchanged files within a session.
	// Invalidated when file_write or file_edit modifies a cached path.
	fileReadCache map[string]string

	// Spec constraint extraction
	specConstraints *SpecConstraints

	// Regression detection
	regressionTracker *RegressionTracker

	// Intent-based pipeline routing
	intentPromptAdjustment string // mode-specific system prompt addition (review, implement, etc.)

	// Event bus for real-time observability
	bus *EventBus
}

// New creates an agent with the given executor, tool registry, and config.
func New(executor ChatExecutor, registry *tools.Registry, renderer TerminalRenderer, config Config) *Agent {
	return &Agent{
		executor:          executor,
		registry:          registry,
		permissions:       tools.NewPermissionChecker(tools.ModeAutoApprove),
		conversation:      NewConversation(),
		renderer:          renderer,
		config:            config,
		sessionID:         fmt.Sprintf("agent-%d", uniqueID()),
		bus:               config.EventBus,
		cachedPromptLevel: -1, // force rebuild on first call
		reviewTracker:     &ReviewCycleTracker{},
	}
}

// SetPermissions sets the permission checker for tool execution.
func (a *Agent) SetPermissions(pc *tools.PermissionChecker) {
	a.permissions = pc
}

// SetPermissionPrompt sets the callback for interactive permission prompting.
func (a *Agent) SetPermissionPrompt(fn tools.PermissionPromptFunc) {
	a.permissionPrompt = fn
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

// SetEventBus attaches an event bus for real-time observability.
func (a *Agent) SetEventBus(bus *EventBus) {
	a.bus = bus
}

// emit publishes an event to the bus if one is attached.
func (a *Agent) emit(eventType EventType, provider string, data map[string]any) {
	if a.bus == nil {
		return
	}
	phase := ""
	if a.pipeline != nil && a.pipelinePhase < len(a.pipeline.Phases) {
		phase = a.pipeline.Phases[a.pipelinePhase].Name
	}
	a.bus.Publish(AgentEvent{
		AgentID:  a.sessionID,
		ParentID: a.parentID,
		Type:     eventType,
		Phase:    phase,
		Provider: provider,
		Data:     data,
	})
}

// SetMetrics attaches a metrics tracker to this agent.
func (a *Agent) SetMetrics(m *AgentMetrics) {
	a.metrics = m
}

// Metrics returns the agent's metrics tracker.
func (a *Agent) Metrics() *AgentMetrics {
	return a.metrics
}

// setupTrimHook wires the conversation's BeforeTrimHook to persist dropped
// messages to VectorMemory before they are lost. Must be called after the
// agent has its sessionID set (which happens in New()).
func (a *Agent) setupTrimHook() {
	if a.config.VectorMemory == nil {
		return
	}
	a.conversation.BeforeTrimHook = func(dropped []providers.Message) {
		for _, m := range dropped {
			// Store all roles including tool messages. For assistant messages
			// with empty Content but ToolCalls, serialize the tool call structure.
			content := m.Content
			if content == "" && len(m.ToolCalls) > 0 {
				if b, err := json.Marshal(m.ToolCalls); err == nil {
					content = fmt.Sprintf("[tool_calls] %s", string(b))
				}
			}
			if content == "" {
				continue
			}
			if err := a.config.VectorMemory.Store(content, m.Role, a.sessionID, nil); err != nil {
				log.Printf("[Agent] trim hook: failed to store dropped message to DB: %v", err)
			}
		}
	}
}

// Run processes a user message through the agent loop and returns the final text response.
func (a *Agent) Run(ctx context.Context, userMessage string) (string, error) {
	// Protect spec file from agent overwrite (tool-layer enforcement)
	if a.config.SpecFilePath != "" {
		tools.SetProtectedPaths([]string{a.config.SpecFilePath})
	}

	// Wire trim hook so messages are persisted to DB before being dropped
	a.setupTrimHook()

	// Initialize hallucination detection
	a.factTracker = NewFactTracker()

	// Set model-aware conversation limit so large-context models compact less aggressively
	a.conversation.MaxMessages = modelMaxMessages(a.config.Model)

	// Register recall tool with unified searcher that covers all ancestor sessions.
	// UnifiedSearcher queries both ToolOutputStore (tool outputs) and VectorMemory
	// (compacted conversation messages) across the current session and all parent sessions.
	if a.config.ToolStore != nil || a.config.VectorMemory != nil {
		allSessionIDs := append([]string{a.sessionID}, a.config.ParentSessionIDs...)
		searcher := NewUnifiedSearcher(a.config.ToolStore, a.config.VectorMemory, allSessionIDs)
		recall := tools.NewRecallTool(searcher, a.sessionID)
		if a.config.VectorMemory != nil {
			recall.WithSemanticSearcher(searcher)
		}
		a.registry.Register(recall)
	}

	// Detect missing build tools and prepare install instructions for the system prompt
	if a.config.WorkDir != "" {
		env := environment.Detect(a.config.WorkDir)
		if env != nil {
			// Auto-set ProjectLanguage from environment detection (before pipeline init)
			if a.config.ProjectLanguage == "" && env.Language != "" {
				a.config.ProjectLanguage = env.Language
				log.Printf("[Agent] auto-detected project language: %s", env.Language)
			}
			missing := environment.MissingTools(env, a.config.WorkDir)
			if len(missing) > 0 {
				a.toolchainSetup = environment.SetupInstructions(env, a.config.WorkDir)
				log.Printf("[Agent] missing tools detected: %v", missing)
			}
			// Also resolve dynamic build commands (e.g., Maven vs Gradle for Java)
			install, test, build := environment.ResolveBuildCommands(env.Language, a.config.WorkDir)
			if install != "" || test != "" || build != "" {
				a.resolvedBuildCmds = fmt.Sprintf("Build: %s | Test: %s | Install: %s", build, test, install)
			}
			// Initialize regression tracker with the resolved build command
			if build != "" {
				a.regressionTracker = NewRegressionTracker(build)
			}
		}
	}

	// Input guardrails
	if a.inputGuardrails != nil {
		if result := a.inputGuardrails.Validate(userMessage); !result.Passed {
			return "", fmt.Errorf("input guardrail: %s", result.Reason)
		}
	}

	// Parse file attachments from @references and absolute paths in the message
	cleanedMessage, attachments := ParseAttachments(userMessage, a.config.WorkDir)
	if len(attachments) > 0 {
		userMessage = cleanedMessage + FormatAttachments(attachments)
		log.Printf("[Agent] attached %d file(s) to message", len(attachments))
	}

	a.conversation.Add(providers.Message{
		Role:    "user",
		Content: userMessage,
	})

	// Reset per-message state
	a.completionVerifyDone = false

	// Capture original request for domain-specific review
	if a.originalRequest == "" {
		a.originalRequest = userMessage
	}

	// Store large specs in ToolOutputStore for recall after compaction
	if a.config.ToolStore != nil && a.originalRequest == userMessage && len(userMessage) > 2048 {
		summary := "Project specification loaded (" + strconv.Itoa(len(userMessage)) + " bytes)"
		_, storeErr := a.config.ToolStore.Store(
			a.sessionID, "spec_load", "initial_request", summary,
			userMessage, 0, len(userMessage))
		if storeErr != nil {
			log.Printf("[Agent] warning: failed to store spec: %v", storeErr)
		} else {
			log.Printf("[Agent] stored spec (%d bytes) as spec_load for recall", len(userMessage))
		}
	}

	// Extract spec constraints (package structure, scope, prohibited patterns)
	// for injection into all agent prompts — runs once at session start.
	if a.specConstraints == nil && len(a.originalRequest) > 100 {
		a.specConstraints = ExtractSpecConstraints(a.originalRequest)
		if !a.specConstraints.IsEmpty() {
			log.Printf("[Agent] extracted spec constraints: package=%s, in_scope=%d, out_of_scope=%d, prohibited=%d",
				a.specConstraints.PackageStructure, len(a.specConstraints.InScope),
				len(a.specConstraints.OutOfScope), len(a.specConstraints.Prohibited))
		}
	}

	// Initialize pipeline immediately on first message (if AutoOrchestrate enabled)
	// This fires parallel planners BEFORE the main agent starts making tool calls
	if a.config.AutoOrchestrate && a.pipeline == nil {
		matched := orchestration.MatchSkillsForLanguage(a.originalRequest, a.config.Skills, a.config.ProjectLanguage)
		a.pipeline = DetectPipelineType(matched, a.config.ProjectLanguage)

		// Adaptive pipeline: assess task complexity and reduce pipeline for trivial/simple tasks.
		// Must run before intent detection and phase initialization since it may nil out the pipeline.
		complexity := AssessComplexity(userMessage, a.config.SpecFilePath != "")
		a.pipeline = AdaptPipeline(a.pipeline, complexity)
		log.Printf("[Agent] adaptive pipeline: complexity=%s", complexity)

		if a.pipeline == nil {
			// Trivial task — no pipeline needed, just answer directly.
			log.Printf("[Agent] skipping pipeline for trivial task")
		}

		if a.pipeline != nil {
			a.pipelinePhase = 0
			a.initializeImplementPhase()

			// Intent-based phase routing: determine starting phase from project state.
			// The pipeline becomes a menu of capabilities, not a forced sequence.
			intentEntry := DetectPipelineEntry(userMessage, a.config.WorkDir, a.pipeline, false)
			a.ApplyIntentEntry(intentEntry)

			skillNames := make([]string, len(matched))
			for i, s := range matched {
				skillNames[i] = s.Name
			}
			log.Printf("[Agent] pipeline: %s (%d phases, entry: %d/%s) | language: %s | skills: %v",
				a.pipeline.Name, len(a.pipeline.Phases), intentEntry.Phase, intentEntry.Mode,
				a.config.ProjectLanguage, skillNames)

			a.emit(EventPipelineStart, "", map[string]any{
				"pipeline_name":  a.pipeline.Name,
				"phase_count":    len(a.pipeline.Phases),
				"matched_skills": skillNames,
				"intent_phase":   intentEntry.Phase,
				"intent_mode":    intentEntry.Mode,
				"intent_reason":  intentEntry.Reason,
			})
			a.emit(EventSkillMatch, "", map[string]any{
				"skill_names":   skillNames,
				"trigger_count": len(matched),
			})

			// Fire parallel plan phase immediately if configured AND intent didn't skip past it
			firstPhase := a.pipeline.Phases[a.pipelinePhase]
			if a.pipelinePhase == 0 && firstPhase.Name == "plan" &&
				firstPhase.ParallelSubAgents > 0 && len(firstPhase.CoderProviders) > 0 && a.hasProviders(firstPhase.CoderProviders) {
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
						parallelResult, a.pipeline.PhasePrompt(a.pipelinePhase, a.acceptanceCriteria, a.originalRequest)),
				})
			} else {
				// No parallel plan phase (or intent skipped it) — inject the current
				// phase prompt so the pipeline actively steers the agent from turn 1.
				var phasePrompt string
				if intentEntry.Phase > 0 {
					phasePrompt = phasePromptForEntry(a.pipeline, intentEntry, a.acceptanceCriteria)
				} else {
					phasePrompt = a.pipeline.PhasePrompt(a.pipelinePhase, a.acceptanceCriteria, a.originalRequest)
				}
				if phasePrompt != "" {
					currentPhase := a.pipeline.Phases[a.pipelinePhase]
					log.Printf("[Agent] pipeline: injecting phase %d/%d prompt: %s",
						a.pipelinePhase+1, len(a.pipeline.Phases), currentPhase.Name)
					a.conversation.Add(providers.Message{
						Role:    "user",
						Content: fmt.Sprintf("You are working in phases. Current phase: %s\n\n%s\n\nComplete this phase using tools, then say %s_COMPLETE before moving on.",
							currentPhase.Name, phasePrompt, strings.ToUpper(strings.ReplaceAll(currentPhase.Name, "-", "_"))),
					})
				}
			}
		} // end if a.pipeline != nil (adaptive pipeline)
	}

	start := time.Now()
	response, err := a.loop(ctx)

	// Record metrics
	if a.metrics != nil {
		turns := len(a.conversation.Messages()) / 2 // rough estimate
		a.metrics.RecordRequest(time.Since(start), 0, turns, err != nil)
	}

	// Write project state file (synroute.md) — the agent's CLAUDE.md equivalent.
	// Written regardless of success/failure so the next run knows what happened.
	a.writeSynrouteMD()

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

// GetAcceptanceCriteria returns the stored acceptance criteria.
func (a *Agent) GetAcceptanceCriteria() string {
	return a.acceptanceCriteria
}

// SetAcceptanceCriteria stores acceptance criteria for pipeline tools.
func (a *Agent) SetAcceptanceCriteria(criteria string) {
	a.acceptanceCriteria = criteria
}

// GetOriginalRequest returns the original user request.
func (a *Agent) GetOriginalRequest() string {
	return a.originalRequest
}

// GetConfig returns the agent's config.
func (a *Agent) GetConfig() Config {
	return a.config
}

// Emit publishes an event to the bus (exported wrapper for pipeline tools).
func (a *Agent) Emit(eventType EventType, provider string, data map[string]any) {
	a.emit(eventType, provider, data)
}

// RunPhase runs the agent with a single pipeline phase. Used by REPL slash
// commands (/plan, /review, /check, /fix). The message is composed with
// phase-appropriate context.
func (a *Agent) RunPhase(ctx context.Context, phaseName string, userMessage string) (string, error) {
	// If no pipeline exists (e.g., trivial task skipped it), create a default
	// pipeline so we can extract the requested phase from it.
	sourcePipeline := a.pipeline
	if sourcePipeline == nil {
		sourcePipeline = &DefaultPipeline
	}

	idx := findPhaseByName(sourcePipeline, phaseName)
	if idx < 0 {
		return "", fmt.Errorf("unknown phase: %s", phaseName)
	}

	// Save and restore pipeline state
	origPipeline := a.pipeline
	origPhase := a.pipelinePhase
	origPrompt := a.intentPromptAdjustment
	defer func() {
		a.pipeline = origPipeline
		a.pipelinePhase = origPhase
		a.intentPromptAdjustment = origPrompt
	}()

	// Set up single-phase mode
	a.ApplyIntentEntry(IntentEntry{
		Phase:       0,
		Mode:        "single",
		SinglePhase: phaseName,
		Reason:      fmt.Sprintf("user invoked /%s command", phaseName),
	})

	return a.Run(ctx, userMessage)
}

func (a *Agent) loop(ctx context.Context) (string, error) {
	for turn := 0; a.config.MaxTurns <= 0 || turn < a.config.MaxTurns; turn++ {
		// Budget check
		if a.budget != nil {
			a.budget.RecordTurn()
			if reason := a.budget.Exceeded(); reason != "" {
				return "", &BudgetExhaustedError{Reason: reason}
			}
		}

		// Per-phase turn cap: force-advance if a single phase consumes too many turns.
		// This prevents infinite loops in self-check/review phases that can't converge.
		if a.config.AutoOrchestrate && a.pipeline != nil && a.pipelinePhase < len(a.pipeline.Phases) {
			a.phaseTurns++
			if a.phaseTurns > a.maxPhaseTurns() {
				phaseName := a.pipeline.Phases[a.pipelinePhase].Name
				log.Printf("[Agent] phase %s exceeded %d turn cap — force-advancing to next phase",
					phaseName, a.maxPhaseTurns())
				a.emit(EventPhaseComplete, "", map[string]any{
					"phase_name":    phaseName,
					"passed":        false,
					"force_advance": true,
					"turns_used":    a.phaseTurns,
				})
				a.advancePipeline("PHASE_SKIPPED_TURN_CAP")
			}
		}

		// LLM call
		req := providers.ChatRequest{
			Model:      a.config.Model,
			Messages:   a.buildMessages(),
			Tools:      a.registry.OpenAIToolDefinitions(),
			ToolChoice: "auto", // Required for many open-source models to use function calling
		}

		// Resolve which provider will handle this call for the event
		startProvider := a.config.TargetProvider
		startModel := a.config.Model
		if startProvider == "" && len(a.config.EscalationChain) > 0 && a.providerIdx < len(a.config.EscalationChain) {
			level := a.config.EscalationChain[a.providerIdx]
			if len(level.Providers) > 0 {
				startProvider = level.Providers[0]
			}
		}
		a.emit(EventLLMStart, startProvider, map[string]any{
			"model": startModel,
			"turn":  turn,
			"role":  a.sessionID,
		})

		var endSpan func(error)
		if a.trace != nil {
			endSpan = a.trace.StartSpan("llm_call", "llm_call", map[string]interface{}{"turn": turn})
		}
		llmStart := time.Now()
		resp, err := a.callLLMWithStreaming(ctx, req)
		llmDuration := time.Since(llmStart)
		if endSpan != nil {
			endSpan(err)
		}
		if err != nil {
			a.emit(EventError, a.config.TargetProvider, map[string]any{
				"source":  "llm_call",
				"message": err.Error(),
			})
			return "", fmt.Errorf("LLM call failed: %w", err)
		}

		respProvider := ""
		if resp.XProxyMetadata != nil {
			respProvider = resp.XProxyMetadata.Provider
		}
		a.emit(EventLLMComplete, respProvider, map[string]any{
			"model":       resp.Model,
			"tokens_used": resp.Usage.TotalTokens,
			"duration":    llmDuration.String(),
		})

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
		// Text-based tool call fallback: some models (especially open-source via Ollama)
		// output tool calls as text markers instead of structured JSON. Parse them.
		if len(msg.ToolCalls) == 0 && msg.Content != "" {
			if textToolCalls, cleanedContent := extractTextToolCalls(msg.Content); len(textToolCalls) > 0 {
				msg.ToolCalls = textToolCalls
				msg.Content = cleanedContent
			}
		}
		// Skip empty assistant messages (no content, no tool calls).
		// Some models return these when confused or context is too large.
		// Adding them to conversation causes 400 errors on strict models.
		if msg.Content == "" && len(msg.ToolCalls) == 0 {
			log.Printf("[Agent] skipping empty assistant message (no content, no tool_calls)")
			a.noToolTurns++
			continue
		}
		a.conversation.Add(msg)

		// Hallucination check: verify LLM claims against ground-truth facts.
		// Only check text-only responses (tool-call-only messages have nothing to hallucinate about).
		// Skip first 3 turns (not enough facts accumulated).
		if msg.Content != "" && len(msg.ToolCalls) == 0 && a.factTracker != nil && a.toolCallCount > 3 {
			checkResult := CheckForHallucinations(msg.Content, a.factTracker)
			if checkResult.Detected {
				corrective := a.autoRecall(checkResult)
				if corrective != "" {
					log.Printf("[Agent] hallucination detected (confidence %.2f, %d signals) — injecting correction",
						checkResult.Confidence, len(checkResult.Signals))
					a.conversation.Add(providers.Message{
						Role:    "user",
						Content: corrective,
					})
					continue // re-run LLM with corrective context
				}
			}
		}

		// Reset hallucination recall count when agent makes tool calls (it moved on)
		if len(msg.ToolCalls) > 0 {
			a.hallucinationRecallCount = 0
		}

		// No tool calls → stall detection + pipeline advancement
		if len(msg.ToolCalls) == 0 {
			a.noToolTurns++
			// Stall detection: if model hasn't made tool calls in 2 consecutive turns,
			// escalate to a bigger model regardless of mode (pipeline or direct).
			if a.noToolTurns >= 2 {
				phaseName := "direct"
				if a.pipeline != nil && a.pipelinePhase < len(a.pipeline.Phases) {
					phaseName = a.pipeline.Phases[a.pipelinePhase].Name
				}
				log.Printf("[Agent] stall detected: %d turns without tools at level %d (phase: %s)",
					a.noToolTurns, a.providerIdx, phaseName)
				a.escalateProvider()
				a.noToolTurns = 0
				a.conversation.Add(forceToolsMessage(phaseName))
				continue
			}
			// Pipeline-specific advancement stays inside AutoOrchestrate guard
			if a.config.AutoOrchestrate {
				// Try to advance pipeline (plan phase may produce text-only output)
				if a.advancePipeline(msg.Content) {
					continue
				}
				// If pipeline has remaining phases, don't exit — force the agent
				// to keep working. Without this, the agent exits after implement
				// phase without running self-check/code-review/acceptance-test.
				if a.pipeline != nil && a.pipelinePhase < len(a.pipeline.Phases) {
					phaseName := a.pipeline.Phases[a.pipelinePhase].Name
					log.Printf("[Agent] pipeline has %d phases remaining (current: %s) — forcing continuation",
						len(a.pipeline.Phases)-a.pipelinePhase, phaseName)
					a.conversation.Add(forceToolsMessage(phaseName))
					continue
				}
			}
			// Final compile verification: if the agent wrote code files,
			// check that the project still builds before declaring success.
			// Only runs once to prevent destructive retry loops, and only when
			// the written files match the project's detected language.
			if a.toolCallCount > 0 && a.hasWrittenCode() && !a.completionVerifyDone {
				a.completionVerifyDone = true
				passed, results := a.RunVerificationGate("deploy")
				if !passed {
					failMsg := FormatVerifyFailures(results)
					log.Printf("[Agent] compile verification failed at completion — sending back for fixes (one attempt)")
					a.conversation.Add(providers.Message{
						Role:    "user",
						Content: failMsg + "\n\nFix these issues before completing. Only fix files you created or modified — do NOT modify other project files.",
					})
					continue // re-enter loop for one fix attempt
				}
			}
			// Don't exit on first text-only turn if:
			// 1. Agent was mid-work (toolCallCount > 0), OR
			// 2. Agent is in non-interactive mode (must try harder, no user fallback)
			// Only continue if escalation chain exists (not tests). Cap at 2.
			shouldContinue := a.noToolTurns == 1 && len(a.config.EscalationChain) > 1 && a.textContinuations < 2 &&
				(a.toolCallCount > 0 || a.config.NonInteractive)
			if shouldContinue {
				a.textContinuations++
				log.Printf("[Agent] text-only turn — continuing (%d/2 chances, tools=%d)", a.textContinuations, a.toolCallCount)
				continue
			}
			return msg.Content, nil
		}

		// Execute tool calls — reset stall and continuation counters
		a.noToolTurns = 0
		a.textContinuations = 0
		a.toolCallCount += len(msg.ToolCalls)
		a.phaseToolCalls += len(msg.ToolCalls)
		a.executeToolCalls(ctx, msg.ToolCalls)

		// Action repetition detection: fingerprint each tool call and detect loops.
		// Uses intent-based fingerprinting (path-only for file ops, normalized bash)
		// plus a cumulative warning counter that escalates even when per-window
		// repeat counts stay low (e.g., 7-file rotation keeps repeats at 3).
		// Always active regardless of pipeline mode.
		for _, tc := range msg.ToolCalls {
			name, args := extractToolCallNameArgs(tc)
			a.toolFingerprints = append(a.toolFingerprints, toolCallFingerprint(name, args))
		}
		if len(a.toolFingerprints) > 40 {
			a.toolFingerprints = a.toolFingerprints[len(a.toolFingerprints)-40:]
		}

		repeats := maxRepeatCount(a.toolFingerprints)
		if repeats >= 3 {
			a.loopWarningCount++
			log.Printf("[Agent] loop warning #%d: same tool call %d times in window",
				a.loopWarningCount, repeats)

			if a.loopWarningCount >= 6 || repeats >= 6 {
				log.Printf("[Agent] action loop: %d warnings, %d repeats — breaking",
					a.loopWarningCount, repeats)
				a.toolFingerprints = nil
				a.loopWarningCount = 0
				if a.config.AutoOrchestrate {
					a.advancePipeline("PHASE_SKIPPED_LOOP")
				} else {
					break // exit loop in direct mode
				}
			} else if a.loopWarningCount >= 3 || repeats >= 4 {
				log.Printf("[Agent] action loop: %d warnings, %d repeats — escalating",
					a.loopWarningCount, repeats)
				a.escalateProvider()
				a.toolFingerprints = nil
				a.conversation.Add(loopDetectedMessage(repeats))
			} else {
				a.conversation.Add(loopDetectedMessage(repeats))
			}
		} else {
			a.loopWarningCount = 0
		}

		// Check for phase signals in the LLM's text content even when tool calls
		// are present. Models often say "EDA_COMPLETE" or "IMPLEMENT_COMPLETE"
		// alongside their tool calls — detect and advance the pipeline mid-work.
		if a.config.AutoOrchestrate && msg.Content != "" {
			if IsPassSignal(msg.Content) || IsFailSignal(msg.Content) {
				a.advancePipeline(msg.Content)
			}
		}
	}

	return "", &BudgetExhaustedError{Reason: fmt.Sprintf("max turns (%d) exceeded", a.config.MaxTurns)}
}

// executeToolCalls runs each tool call and adds results to conversation.
func (a *Agent) executeToolCalls(ctx context.Context, toolCalls []map[string]interface{}) {
	for _, toolCall := range toolCalls {
		callID := extractToolCallID(toolCall)
		name, args := extractToolCallNameArgs(toolCall)

		// Track code file writes for compile verification
		if (name == "file_write" || name == "file_edit") && isCodeFilePath(args) {
			a.wroteCodeFiles = true
		}

		if a.renderer != nil {
			a.renderer.ToolCall(name, args)
		}
		a.emit(EventToolStart, "", map[string]any{
			"tool_name":    name,
			"args_summary": formatToolCallSummary(name, args),
		})

		// File read dedup: return cached content if the file hasn't been modified
		if name == "file_read" {
			readPath, _ := args["path"].(string)
			if readPath != "" {
				resolvedPath := resolvePathForCache(readPath, a.config.WorkDir)
				if cached, ok := a.fileReadCache[resolvedPath]; ok {
					log.Printf("[Agent] file_read cache hit: %s", resolvedPath)
					a.conversation.Add(providers.Message{
						Role:       "tool",
						ToolCallID: callID,
						Content:    cached,
					})
					continue
				}
			}
		}

		// Per-tool-call timeout as a safety net. Even if the bash tool's internal
		// timeout fails, the agent won't hang forever on a single tool call.
		toolCtx, toolCancel := context.WithTimeout(ctx, 5*time.Minute)
		toolStart := time.Now()
		result, execErr := a.registry.ExecuteWithPrompt(toolCtx, name, args, a.config.WorkDir, a.permissions, a.permissionPrompt)
		toolCancel()
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
		isError := false
		if execErr != nil {
			resultContent = fmt.Sprintf("error: %v\n%s", execErr, toolErrorHint(name))
			isError = true
			if a.renderer != nil {
				a.renderer.ToolResult(name, resultContent, true)
			}
		} else {
			resultContent = result.Output
			if result.Error != "" {
				resultContent = result.Error + "\n" + resultContent
				isError = true
			}
			if a.renderer != nil {
				// Show colored diff for file edits
				if name == "file_edit" && !isError {
					oldText, _ := args["old_text"].(string)
					newText, _ := args["new_text"].(string)
					path, _ := args["path"].(string)
					if oldText != "" && newText != "" {
						a.renderer.ToolDiff(path, oldText, newText)
					} else {
						a.renderer.ToolResult(name, resultContent, false)
					}
				} else {
					a.renderer.ToolResult(name, resultContent, result.Error != "")
				}
			}
		}

		toolEventData := map[string]any{
			"tool_name": name,
			"duration":  toolDuration.String(),
			"is_error":  isError,
		}
		if isError || a.bus != nil {
			lines := strings.Count(resultContent, "\n") + 1
			toolEventData["output_lines"] = lines
			if len(resultContent) <= 500 {
				toolEventData["output"] = resultContent
			}
		}
		a.emit(EventToolComplete, "", toolEventData)

		// Summarize large tool outputs and store full output in DB.
		// Small outputs (<2KB) are kept verbatim in conversation.
		conversationContent := resultContent
		exitCode := 0
		if result != nil {
			exitCode = result.ExitCode
		}
		if execErr != nil {
			exitCode = -1
		}
		argsSummary := FormatArgsSummary(name, args)

		// Store ALL tool outputs in DB regardless of size (if configured).
		// This ensures nothing is lost — even small outputs are persisted for recall.
		var storedOutputID int64
		if a.config.ToolStore != nil {
			summary := SummarizeToolOutput(name, args, resultContent, exitCode)
			outputID, storeErr := a.config.ToolStore.Store(
				a.sessionID, name, argsSummary, summary, resultContent,
				exitCode, len(resultContent))
			if storeErr != nil {
				log.Printf("[Agent] warning: failed to store tool output: %v", storeErr)
			} else {
				storedOutputID = outputID
			}
		}

		// Summarize large outputs for conversation context; keep small outputs verbatim.
		if ShouldSummarize(name, resultContent) {
			summary := SummarizeToolOutput(name, args, resultContent, exitCode)
			if storedOutputID > 0 {
				summary += fmt.Sprintf("\n[full output: ref:%d]", storedOutputID)
			}
			conversationContent = summary
		} else {
			// Still apply safety truncation for very large outputs that slip through
			conversationContent = truncateToolOutput(resultContent, 32*1024)
		}

		// Record ground-truth facts for hallucination detection
		if a.factTracker != nil {
			a.factTracker.RecordToolOutput(name, args, resultContent, exitCode, storedOutputID)
		}

		a.conversation.Add(providers.Message{
			Role:       "tool",
			ToolCallID: callID,
			Content:    conversationContent,
		})

		// File read cache: populate on successful read, invalidate on write/edit
		if name == "file_read" && !isError {
			readPath, _ := args["path"].(string)
			if readPath != "" {
				resolvedPath := resolvePathForCache(readPath, a.config.WorkDir)
				if a.fileReadCache == nil {
					a.fileReadCache = make(map[string]string)
				}
				a.fileReadCache[resolvedPath] = conversationContent
			}
		}
		if (name == "file_write" || name == "file_edit") && !isError {
			writePath, _ := args["path"].(string)
			if writePath != "" {
				resolvedPath := resolvePathForCache(writePath, a.config.WorkDir)
				delete(a.fileReadCache, resolvedPath)
			}
		}

		// Regression detection: check if a bash command produced MORE compilation
		// errors than the previous build. Injects a warning into conversation so
		// the LLM knows to stop and fix instead of continuing to create files.
		if name == "bash" && a.regressionTracker != nil && exitCode != 0 {
			if warning := a.regressionTracker.Check(resultContent, exitCode); warning != "" {
				a.conversation.Add(providers.Message{
					Role:    "user",
					Content: warning,
				})
			}
		}
	}
}

func (a *Agent) buildMessages() []providers.Message {
	// Build system prompt and cache it. Invalidate when provider level changes
	// so small models get simpler prompts and large models get full instructions.
	if a.cachedSystemPrompt == "" || a.cachedPromptLevel != a.providerIdx {
		sysPrompt := a.config.SystemPrompt
		if sysPrompt == "" {
			sysPrompt = defaultSystemPrompt(a.config.WorkDir, a.providerIdx, a.config.ProjectLanguage)
		}

		// Inject project-level instructions FIRST (CLAUDE.md / AGENTS.md from working directory)
		// Project instructions take priority over skill patterns.
		if projectInstr := LoadProjectInstructions(a.config.WorkDir); projectInstr != "" {
			sysPrompt += "\n\n# Project Instructions\n" + projectInstr
		}

		// Inject matched skill instructions (reference patterns, not overrides)
		if skillCtx := a.matchedSkillContext(); skillCtx != "" {
			sysPrompt += "\n\n" + skillCtx
			sysPrompt += "\n\nNOTE: Skill patterns are reference examples. When they conflict with the original request, the request takes priority."
		}


		// Inject extracted spec constraints prominently — these override skill patterns
		if a.specConstraints != nil {
			if formatted := a.specConstraints.FormatConstraints(); formatted != "" {
				sysPrompt += "\n\n" + formatted
			}
		}

		// Inject toolchain setup instructions if missing tools detected
		if a.toolchainSetup != "" {
			sysPrompt += "\n\n# TOOLCHAIN SETUP REQUIRED\n" + a.toolchainSetup +
				"\nInstall these tools FIRST before attempting to build or test."
		}
		if a.resolvedBuildCmds != "" {
			sysPrompt += "\n\n# Resolved Build Commands\n" + a.resolvedBuildCmds
		}

		// Inject intent-based mode adjustment (review, implement, single)
		if a.intentPromptAdjustment != "" {
			sysPrompt += "\n\n" + a.intentPromptAdjustment
		}

		// Non-interactive mode: suppress follow-up questions (#315)
		if a.config.NonInteractive {
			sysPrompt += "\n\nNON-INTERACTIVE MODE: Complete the full task. Do not ask follow-up questions — there is no user to answer them. If requirements are ambiguous, make reasonable assumptions and document them."
		}

		a.cachedSystemPrompt = sysPrompt
		a.cachedPromptLevel = a.providerIdx
	}

	msgs := []providers.Message{{
		Role:    "system",
		Content: a.cachedSystemPrompt,
	}}

	// After compaction, auto-inject relevant past context from VectorMemory.
	// This surfaces earlier conversation that was compacted to DB, so the LLM
	// has continuity without the agent needing explicit recall tool calls.
	if a.hasCompacted && a.config.VectorMemory != nil {
		lastUser := a.conversation.LastUserMessage()
		if lastUser != "" {
			retrieved, err := a.config.VectorMemory.RetrieveRelevant(lastUser, a.sessionID, 2048)
			if err == nil && len(retrieved) > 0 {
				var contextBuf strings.Builder
				contextBuf.WriteString("[Retrieved context from earlier:]\n")
				for _, m := range retrieved {
					contextBuf.WriteString(fmt.Sprintf("[%s] %s\n", m.Role, m.Content))
				}
				msgs = append(msgs, providers.Message{
					Role:    "user",
					Content: contextBuf.String(),
				})
				msgs = append(msgs, providers.Message{
					Role:    "assistant",
					Content: "Understood, I have the retrieved context from earlier in the session.",
				})
			}
		}
	}

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

// skillContextBudget returns the max bytes allowed for skill injection
// based on the current provider level (smaller models get less context).
func (a *Agent) skillContextBudget() int {
	switch {
	case a.providerIdx == 0:
		return 4096 // ~1K tokens for small models (14-30B)
	case a.providerIdx <= 2:
		return 16384 // ~4K tokens for mid models (120B)
	default:
		return 65536 // ~16K tokens for frontier models (480B+)
	}
}

// computeSkillContext does the actual skill matching and formatting.
// Respects a size budget based on the current provider level — small models
// get abbreviated skill context to avoid overwhelming their context window.
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

	matched := orchestration.MatchSkillsForLanguage(query, a.config.Skills, a.config.ProjectLanguage)
	if len(matched) == 0 {
		return ""
	}

	chain := orchestration.BuildSkillChain(matched)
	budget := a.skillContextBudget()

	var b strings.Builder
	b.WriteString("=== Active Skills ===\n")
	used := b.Len()

	for _, skill := range chain {
		content := skill.Description
		if skill.Instructions != "" && a.providerIdx > 0 {
			// Only inject full instructions for mid+ models
			content = skill.Instructions
		}

		entry := "\n## " + skill.Name + "\n" + content + "\n"
		if used+len(entry) > budget {
			// Over budget — add abbreviated entry
			abbreviated := "\n## " + skill.Name + " (abbreviated)\n" + skill.Description + "\n"
			if used+len(abbreviated) <= budget {
				b.WriteString(abbreviated)
				used += len(abbreviated)
			}
			// Skip remaining skills if we're out of budget
			if used >= budget {
				b.WriteString(fmt.Sprintf("\n[%d more skills truncated for model capacity]\n",
					len(chain)-countBefore(chain, skill.Name)))
				break
			}
			continue
		}
		b.WriteString(entry)
		used += len(entry)
	}
	b.WriteString("=== End Skills ===")

	// Auto-invoke MCP tools bound to matched skills
	if mcpCtx := a.invokeMCPToolsForSkills(chain, query); mcpCtx != "" {
		b.WriteString("\n\n" + mcpCtx)
	}

	return b.String()
}

// countBefore returns the index of skill with given name in the chain.
func countBefore(chain []orchestration.Skill, name string) int {
	for i, s := range chain {
		if s.Name == name {
			return i
		}
	}
	return len(chain)
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

// maxPhaseTurns returns the dynamic hard cap on LLM calls per pipeline phase.
// Auto-detects from spec complexity when Config.MaxPhaseTurns is 0:
//   >20KB spec = 40 turns, 5-20KB = 25 turns, <5KB = 15 turns.
func (a *Agent) maxPhaseTurns() int {
	if a.config.MaxPhaseTurns > 0 {
		return a.config.MaxPhaseTurns
	}
	// Auto-detect from spec complexity
	specLen := len(a.originalRequest)
	switch {
	case specLen > 20000: // complex spec (>20KB)
		return 40
	case specLen > 5000: // medium spec (5-20KB)
		return 25
	default: // simple spec or no spec
		return 15
	}
}

// maxPipelineCycles is the hard cap on fail-back cycles (review → implement → review).
// 3 cycles is enough: initial attempt + 2 fix iterations. Beyond that, accept the result
// and let later pipeline phases still run.
const maxPipelineCycles = 3

// toolBlock is the canonical tool reference shared across all prompt levels.
const toolBlock = `AVAILABLE TOOLS (use exact names):
- bash: Run shell commands. Args: command (string).
- file_read: Read file. Args: path (string), offset (int, optional), limit (int, optional).
- file_write: Create/overwrite file. Args: path (string), content (string).
- file_edit: Edit text in file. Args: path (string), old_text (string), new_text (string).
- grep: Search files. Args: pattern (string), path (string, optional), include (string, optional).
- glob: Find files. Args: pattern (string), path (string, optional).
- git: Git ops. Args: subcommand (string), args ([]string).

DO NOT call: str_replace_editor, read_file, write_file, execute_command, list_files,
search_files, browser, computer, text_editor, or any unlisted tool.

EXECUTION RULES:
- Use bash ONLY for: mkdir, pip/npm install, quick smoke tests (< 10 seconds).
- Do NOT run full training, data processing, or long computations via bash.
- Do NOT use "python -c" or "node -e" for inline code — write it to a file with file_write first.
- Build the PROGRAM. Run it once briefly to verify it starts. Then deliver.
- If a command takes more than 10 seconds, it is too long — the user will run it themselves.`

func defaultSystemPrompt(workDir string, providerLevel int, projectLanguage string) string {
	// Language directive — prevents wrong-language file creation
	langDirective := ""
	if projectLanguage != "" {
		langDirective = fmt.Sprintf("\nPROJECT LANGUAGE: %s — all source files MUST be in this language. "+
			"Do NOT create config files or source files for other languages "+
			"(no go.mod for JS projects, no setup.py for Go projects, no package.json for Python projects).\n", projectLanguage)
	}

	// Level 0 (small models ~20-30B): shorter, more forceful prompt focused on tool calling
	if providerLevel == 0 {
		return fmt.Sprintf(`You are a coding assistant working in: %s
%s
YOU MUST USE TOOLS TO COMPLETE TASKS. Do not describe what you would do — actually do it by calling tools.

%s

SPEC COMPLIANCE:
- If the request defines IN SCOPE / OUT OF SCOPE: follow strictly. Do NOT add out-of-scope features.
- Match the spec's directory structure exactly. Do NOT reorganize packages.
- Do NOT add layers (service, controller, repository) unless the spec requires them.

RULES:
- To fix existing files: file_read THEN file_edit. NEVER create duplicate files.
- After writing tests: ALWAYS run them. If they fail, fix and re-run until all pass.
- Never claim success without running the actual command and seeing output.
- If a build tool is missing (mvn, go, npm, etc.), install it first: bash(brew install <tool>).
- WORKING DIRECTORY: All files MUST be created directly in the current working directory — do NOT
  create subdirectories like "petclinic/", "myapp/", or "project/" and build inside them.
- When bash returns a compilation error, READ THE ERROR carefully — it tells you the exact
  file and line number. Use file_edit to fix that specific line, then rebuild.

WORKFLOW — start immediately with tool calls:
1. bash: create directories WITHIN the working directory (mkdir -p src/main/java/...)
2. file_write: create NEW source files with full content
3. bash: install dependencies, build, and run tests
4. If tests fail: READ the error output, file_edit to fix the exact file:line, then re-run

Do NOT output plans, descriptions, or JSON without tool calls. Every response must include at least one tool call.`, workDir, langDirective, toolBlock)
	}

	// Level 1+ (larger models ~120B+): full prompt with methodology
	return fmt.Sprintf(`You are a coding assistant that BUILDS tools and programs. You are working in: %s
%s
SPEC COMPLIANCE:
- If the request defines IN SCOPE / OUT OF SCOPE: follow strictly. Do NOT add out-of-scope features.
- Match the spec's directory structure exactly. Do NOT reorganize packages.
- Do NOT add layers (service, controller, repository) unless the spec requires them.

TOOL BUILDER (DEFAULT MODE):
- By default, BUILD programs and tools — do not do the work yourself.
- When a task involves operations (API calls, data processing, sync, transforms):
  write a PROGRAM that does it. Do not do it manually with bash/curl.
- The user CAN ask you to run, monitor, or execute programs. When asked, do so.
- Without explicit instruction to run: build it, verify it starts briefly, deliver it.
- Use bash/curl for research (testing APIs, inspecting responses) and
  quick verification (running the built tool once to check it works).

RESEARCH BEFORE CODING:
- When working with an unfamiliar API, library, or format: read docs, test with curl,
  inspect a real response BEFORE writing code against it.
- Never guess at auth methods, endpoints, or payload formats.
- When you encounter unknowns: STOP → RESEARCH → APPLY. Never guess and ship.

EDITING EXISTING FILES:
- When fixing bugs or modifying existing code, ALWAYS use file_edit on the existing file.
- NEVER create a new file with corrections when the original file already exists.
- Read the file first with file_read, then edit it in place with file_edit.
- Only use file_write for genuinely NEW files that don't exist yet.

ENVIRONMENT SETUP:
- Before building, check that required tools are installed (e.g., bash: which mvn, which go, which python3).
- If a required tool is missing, install it before proceeding:
  - macOS: use brew install (e.g., brew install maven, brew install go, brew install node)
  - If brew is not available, try the tool's official installer
  - For Java projects: ensure JDK 17+ is installed (brew install openjdk@17)
  - For Node projects: ensure node/npm are installed (brew install node)
  - For Python projects: ensure python3 and pip are available
- Do NOT skip the build step because a tool is missing — install it first.

WORKING DIRECTORY:
- All project files MUST be created directly in the working directory (%s).
- Do NOT create a wrapper subdirectory (e.g., "petclinic/", "myapp/") and build inside it.
- Source trees go directly in the working directory: src/, pom.xml, package.json, etc.
- When running bash commands, do NOT use "cd <subdir> && ..." — run from the working directory.

SELF-CORRECTION ON BUILD ERRORS:
- When a build command (mvn, go build, npm run build, cargo build) fails, READ THE ERROR OUTPUT.
- Compilation errors include the exact file path and line number — use file_edit to fix that line.
- Do NOT create new files to work around compilation errors — fix the existing file.
- After fixing, re-run the SAME build command to verify. Repeat until it compiles.

VERIFY YOUR WORK:
- After writing code, ALWAYS run the build/compile command to verify it compiles.
- After writing tests, ALWAYS run them. If any test fails, fix it before reporting success.
- Never claim "tests pass" without actually running them and seeing the output.
- If a test fails, read the error, fix the code or test, and re-run until all pass.

PRODUCTION QUALITY:
- Would a senior engineer ship this? No stubs, no flat structures, no missing edge cases.
- Show math for calculated values. Never approximate when exact values are available.
- Document assumptions. Flag ambiguous decisions for review.

%s`, workDir, langDirective, workDir, toolBlock)
}

// forceToolsMessage returns a phase-appropriate message demanding tool calls.
// Reads phase-specific messages from the embedded force-tools.md file.
func forceToolsMessage(phaseName string) providers.Message {
	return providers.Message{
		Role:    "user",
		Content: ParseForceToolsPrompt(phaseName),
	}
}

// writeSynrouteMD writes the agent's project state to synroute.md with YAML
// frontmatter for cross-session continuity. Uses the continuity system so
// the format is consistent between in-session writes and session-end saves.
func (a *Agent) writeSynrouteMD() {
	c := BuildContinuityFromAgent(a)
	if err := writeSynrouteMD(c); err != nil {
		log.Printf("[Agent] warning: could not write synroute.md: %v", err)
	}
}

// executeForCurrentProvider routes the LLM call to the current provider in the
// escalation chain. If not escalated (providerIdx == 0), uses default routing
// which lets the router pick the best provider. If escalated, targets a specific
// provider so the agent can use a different model for review/retry.
func (a *Agent) executeForCurrentProvider(ctx context.Context, req providers.ChatRequest) (providers.ChatResponse, error) {
	// If a target provider is set (sub-agent targeting specific Ollama model), try it first
	if a.config.TargetProvider != "" {
		if pae, ok := a.executor.(ProviderAwareExecutor); ok {
			resp, err := pae.ChatCompletionForProvider(ctx, req, a.sessionID, a.config.TargetProvider, false)
			if err == nil {
				return resp, nil
			}
			// Target provider failed (circuit-open, timeout, etc.) — fall through to escalation chain
			log.Printf("[Agent] target provider %s failed: %v — falling through to chain", a.config.TargetProvider, err)
		}
	}

	// Try providers at the CURRENT escalation level only.
	// NEVER mutates providerIdx — the pipeline/caller decides escalation.
	// NEVER falls through to default routing — stays on current level.
	levelProvs := a.currentLevelProviders()
	if len(levelProvs) > 0 {
		if pae, ok := a.executor.(ProviderAwareExecutor); ok {
			var lastErr error
			for _, provider := range levelProvs {
				resp, err := pae.ChatCompletionForProvider(ctx, req, a.sessionID, provider, false)
				if err == nil {
					return resp, nil
				}
				log.Printf("[Agent] provider %s at level %d failed (%v), trying next at same level",
					provider, a.providerIdx, err)
				lastErr = err
			}
			// All providers at this level failed — return error, let caller decide
			return providers.ChatResponse{}, fmt.Errorf("all providers at level %d failed: %w", a.providerIdx, lastErr)
		}
	}
	// No escalation chain configured — use default routing
	return a.executor.ChatCompletion(ctx, req, a.sessionID)
}

// advancePipeline checks the LLM's response and advances to the next pipeline
// phase, or sends back for fixes if the current phase failed. Returns true if
// a new phase prompt was injected (loop should continue), false if done.
func (a *Agent) advancePipeline(content string) bool {
	// Initialize pipeline on first call
	if a.pipeline == nil {
		matched := orchestration.MatchSkillsForLanguage(a.originalRequest, a.config.Skills, a.config.ProjectLanguage)
		a.pipeline = DetectPipelineType(matched, a.config.ProjectLanguage)
		a.pipelinePhase = 0
		log.Printf("[Agent] pipeline: %s (%d phases) | language: %s", a.pipeline.Name, len(a.pipeline.Phases), a.config.ProjectLanguage)
	}

	if a.pipelinePhase >= len(a.pipeline.Phases) {
		return false // all phases done
	}

	currentPhase := a.pipeline.Phases[a.pipelinePhase]

	// Turn cap force-advance: when a phase exceeds maxPhaseTurns, skip it entirely.
	// This MUST be handled before normal signal detection to prevent the "first time
	// entering" fallback from re-injecting the same phase prompt.
	if content == "PHASE_SKIPPED_TURN_CAP" || content == "PHASE_SKIPPED_LOOP" {
		a.pipelineCycles++
		log.Printf("[Agent] force-skipping phase %s (cycle %d/%d)",
			currentPhase.Name, a.pipelineCycles, maxPipelineCycles)
		if a.pipelineCycles > maxPipelineCycles {
			log.Printf("[Agent] max cycles reached (%d) — force-advancing past all review phases",
				a.pipelineCycles)
			a.pipelineCycles = 0
		}
		// Fall through to the normal advance logic below (shouldAdvance = true)
	}

	// shouldContinue only applies to implement phase — prevents false advances in review phases
	shouldAdvance := IsPassSignal(content) || content == "PHASE_SKIPPED_TURN_CAP" || content == "PHASE_SKIPPED_LOOP"
	if !shouldAdvance && (currentPhase.Name == "implement" || currentPhase.Name == "data-prep" || currentPhase.Name == "model") {
		shouldAdvance = a.shouldContinue(content)
	}

	// Quality gate: reject phase transition if minimum tool calls not met
	if shouldAdvance && currentPhase.MinToolCalls > 0 && a.phaseToolCalls < currentPhase.MinToolCalls {
		a.phaseRetries++

		// After 5 retries: escalate provider and try again
		if a.phaseRetries == 5 {
			log.Printf("[Agent] phase %s stuck after %d retries at level %d — escalating",
				currentPhase.Name, a.phaseRetries, a.providerIdx)
			a.escalateProvider()
			a.conversation.Add(forceToolsMessage(currentPhase.Name))
			return true
		}

		// After 10 retries: skip phase entirely (deadlock prevention)
		if a.phaseRetries >= 10 {
			log.Printf("[Agent] phase %s deadlocked after %d retries — skipping",
				currentPhase.Name, a.phaseRetries)
			a.phaseRetries = 0
			// Fall through to normal advance below
		} else {
			log.Printf("[Agent] quality gate: phase %s needs %d tool calls, has %d (retry %d) — rejecting",
				currentPhase.Name, currentPhase.MinToolCalls, a.phaseToolCalls, a.phaseRetries)
			a.emit(EventQualityGate, "", map[string]any{
				"phase":    currentPhase.Name,
				"required": currentPhase.MinToolCalls,
				"actual":   a.phaseToolCalls,
				"rejected": true,
				"retry":    a.phaseRetries,
			})
			a.conversation.Add(providers.Message{
				Role: "user",
				Content: fmt.Sprintf("You claimed phase '%s' is complete, but you only made %d tool calls (minimum %d required). You MUST use tools to gather evidence — fetch real data, inspect actual output, run tests. Do not state opinions without evidence. Use tools now, then re-assess.",
					currentPhase.Name, a.phaseToolCalls, currentPhase.MinToolCalls),
			})
			return true
		}
	}

	// Check if current phase passed or failed
	if shouldAdvance {
		// Programmatic verification gate — the LLM claims PASS,
		// now prove it with actual build/test/verify exit codes.
		// Exit codes can't be hallucinated.
		allPassed, verifyResults := a.RunVerificationGate(currentPhase.Name)
		if !allPassed {
			score := countVerifyPassed(verifyResults)
			total := len(verifyResults)
			a.phaseRetries++

			// Plateau detection: track whether score is improving
			if score > a.lastGateScore {
				a.plateauCount = 0 // progress — reset plateau counter
			} else {
				a.plateauCount++
			}
			a.lastGateScore = score

			log.Printf("[Agent] verification gate FAILED for phase %s: %d/%d passed (retry %d, plateau %d)",
				currentPhase.Name, score, total, a.phaseRetries, a.plateauCount)
			a.emit(EventQualityGate, "", map[string]any{
				"phase":         currentPhase.Name,
				"gate":          "verification",
				"checks_total":  total,
				"checks_passed": score,
				"checks_failed": countVerifyFailed(verifyResults),
				"plateau":       a.plateauCount,
			})

			// Plateau for 2+ retries: escalate to bigger model
			if a.plateauCount == 2 {
				log.Printf("[Agent] verification plateau at %d/%d for %d retries — escalating",
					score, total, a.plateauCount)
				a.escalateProvider()
			}

			// Deadlock: plateau 4+ retries or hard cap at 10 total — skip phase
			if a.plateauCount >= 4 || a.phaseRetries >= 10 {
				log.Printf("[Agent] verification deadlock (plateau %d, retries %d) — skipping phase %s",
					a.plateauCount, a.phaseRetries, currentPhase.Name)
				// Fall through to advance below
			} else {
				a.conversation.Add(providers.Message{
					Role:    "user",
					Content: FormatVerifyFailures(verifyResults),
				})
				return true // agent must fix before phase can advance
			}
		}

		// Store acceptance criteria if this phase produces them
		if currentPhase.StoreAs == "criteria" {
			a.acceptanceCriteria = content
		}

		// Advance to next phase
		a.pipelinePhase++
		a.phaseToolCalls = 0      // reset for new phase
		a.phaseTurns = 0          // reset per-phase turn counter
		a.phaseRetries = 0        // reset retry counter
		a.lastGateScore = 0       // reset plateau tracking
		a.plateauCount = 0
		a.toolFingerprints = nil  // reset loop detection
		a.loopWarningCount = 0
		a.reviewTracker.Reset()   // reset review stability detection for new phase

		// Compact conversation between phases: store old messages to DB,
		// keep only recent context + a phase summary. Prevents context overflow
		// in long multi-phase sessions.
		a.compactConversation(currentPhase.Name)

		if a.pipelinePhase >= len(a.pipeline.Phases) {
			log.Printf("[Agent] pipeline complete: all %d phases passed", len(a.pipeline.Phases))
			return false
		}

		nextPhase := a.pipeline.Phases[a.pipelinePhase]

		// Tier-based provider routing: set provider level to match the next phase's
		// preferred tier. providerIdx is monotonically increasing, so this only
		// escalates UP, never back down to cheaper models.
		if nextPhase.Tier != "" && len(a.config.EscalationChain) > 1 {
			tierLevel := a.ProviderLevelForTier(nextPhase.Tier)
			if tierLevel > a.providerIdx {
				a.SetMinProviderLevel(tierLevel)
				log.Printf("[Agent] tier routing: phase %s wants %s tier → level %d: %v",
					nextPhase.Name, nextPhase.Tier, a.providerIdx,
					a.config.EscalationChain[a.providerIdx].Providers)
			}
		} else if currentPhase.Name == "implement" && a.providerIdx == 0 && len(a.config.EscalationChain) > 1 {
			// Legacy fallback: advance past Level 0 coders after implement
			a.SetMinProviderLevel(1)
			log.Printf("[Agent] advanced past coder level to review level %d: %v",
				a.providerIdx, a.config.EscalationChain[a.providerIdx].Providers)
		}

		// Escalate provider if this phase requires it (stays escalated permanently)
		if nextPhase.Escalate {
			a.escalateProvider()
		}

		log.Printf("[Agent] pipeline: advancing to phase %d/%d: %s",
			a.pipelinePhase+1, len(a.pipeline.Phases), nextPhase.Name)

		// Update project state file on each phase transition
		a.writeSynrouteMD()

		a.emit(EventPhaseComplete, "", map[string]any{
			"phase_name": currentPhase.Name,
			"passed":     true,
			"cycle":      a.pipelineCycles,
		})
		a.emit(EventPhaseStart, "", map[string]any{
			"phase_name":   nextPhase.Name,
			"phase_index":  a.pipelinePhase,
			"total_phases": len(a.pipeline.Phases),
		})

		// Parallel implement: spawn N coders working concurrently on split tasks
		// For plan phase, check hardcoded providers. For others, use current level.
		canParallel := nextPhase.ParallelSubAgents > 0 && len(a.currentLevelProviders()) > 0
		if nextPhase.Name == "plan" {
			canParallel = nextPhase.ParallelSubAgents > 0 && len(nextPhase.CoderProviders) > 0 && a.hasProviders(nextPhase.CoderProviders)
		}
		if canParallel {
			parallelResult := a.runParallelPhase(nextPhase)
			a.conversation.Add(providers.Message{
				Role:    "user",
				Content: fmt.Sprintf("Parallel implementation complete. Results from %d agents:\n%s\nReview the combined output and say IMPLEMENT_COMPLETE if everything looks good, or fix any issues.", nextPhase.ParallelSubAgents, parallelResult),
			})
			return true
		}

		// Sub-agent phases: spawn a fresh agent with NO shared conversation
		if nextPhase.UseSubAgent {
			// Parallel verification: if the next phase after this one is ALSO
			// UseSubAgent, run both simultaneously. Both are independent (fresh
			// agents, no shared context) so they can safely execute in parallel.
			// This cuts wall-clock time for code-review + acceptance-test in half.
			followingIdx := a.pipelinePhase + 1
			canParallelVerify := followingIdx < len(a.pipeline.Phases) &&
				a.pipeline.Phases[followingIdx].UseSubAgent

			if canParallelVerify {
				followingPhase := a.pipeline.Phases[followingIdx]
				log.Printf("[Agent] parallel verification: running %s + %s simultaneously",
					nextPhase.Name, followingPhase.Name)

				// Escalate for the following phase too if needed
				if followingPhase.Escalate {
					a.escalateProvider()
				}

				reviewResult, acceptResult := a.runParallelSubAgentPhases(nextPhase, followingPhase)

				bothPassed := IsPassSignal(reviewResult) && IsPassSignal(acceptResult)
				if bothPassed {
					// Both passed — advance past both phases
					a.pipelinePhase++ // skip past the following phase too
					a.conversation.Add(providers.Message{
						Role: "user",
						Content: fmt.Sprintf("Parallel verification complete — both passed:\n\n--- %s ---\n%s\n\n--- %s ---\n%s",
							nextPhase.Name, reviewResult, followingPhase.Name, acceptResult),
					})
					return a.advancePipeline("PHASE_PASSED")
				}

				// At least one failed — combine failures for the fix cycle
				var failures []string
				if !IsPassSignal(reviewResult) {
					failures = append(failures, fmt.Sprintf("=== %s FAILED ===\n%s", nextPhase.Name, reviewResult))
				}
				if !IsPassSignal(acceptResult) {
					failures = append(failures, fmt.Sprintf("=== %s FAILED ===\n%s", followingPhase.Name, acceptResult))
				}
				combinedFailures := strings.Join(failures, "\n\n")

				a.pipelineCycles++

				if a.reviewTracker.CheckDivergence(combinedFailures) {
					log.Printf("[Agent] pipeline: parallel review findings INCREASING — force-advancing after %d cycles", a.pipelineCycles)
					a.pipelinePhase++ // skip past following phase
					a.conversation.Add(providers.Message{
						Role:    "user",
						Content: fmt.Sprintf("Review findings are increasing each cycle (diverging). Accepting current state to stop cost growth:\n%s", combinedFailures),
					})
					return a.advancePipeline("PHASE_PASSED")
				}

				if a.reviewTracker.CheckStability(a.config.WorkDir, combinedFailures) {
					log.Printf("[Agent] pipeline: parallel review stable — accepting after %d cycles", a.pipelineCycles)
					a.pipelinePhase++ // skip past following phase
					a.conversation.Add(providers.Message{
						Role:    "user",
						Content: fmt.Sprintf("Review found issues but no improvement detected across cycles. Accepting current state:\n%s", combinedFailures),
					})
					return a.advancePipeline("PHASE_PASSED")
				}

				if a.pipelineCycles > maxPipelineCycles {
					log.Printf("[Agent] pipeline: max review cycles reached (%d), accepting result", a.pipelineCycles)
					a.pipelineCycles = 0
					a.pipelinePhase++ // skip past following phase
					a.conversation.Add(providers.Message{
						Role:    "user",
						Content: fmt.Sprintf("Review found issues but max cycles reached. Delivering current state:\n%s", combinedFailures),
					})
					return a.advancePipeline("PHASE_PASSED")
				}

				escalated := a.escalateProvider()
				providerName := "default"
				if a.providerIdx < len(a.config.EscalationChain) {
					level := a.config.EscalationChain[a.providerIdx]
					if len(level.Providers) > 0 {
						providerName = level.Providers[0]
					}
				}
				log.Printf("[Agent] pipeline: parallel review cycle %d/%d — fixing on %s (provider idx %d, escalated=%v)",
					a.pipelineCycles, maxPipelineCycles, providerName, a.providerIdx, escalated)
				a.conversation.Add(providers.Message{
					Role: "user",
					Content: fmt.Sprintf("Parallel verification found issues (cycle %d/%d). Fix ALL these issues using tools, then say IMPLEMENT_COMPLETE:\n---\n%s",
						a.pipelineCycles, maxPipelineCycles, combinedFailures),
				})
				a.pipelinePhase = a.findPhaseIndex("self-check", a.pipelinePhase-1)
				a.phaseToolCalls = 0
				a.phaseTurns = 0
				return true
			}

			// Single sub-agent phase (no parallel peer follows)
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

			// Check for divergence — if findings are increasing, stop cycling (cost is growing)
			if a.reviewTracker.CheckDivergence(reviewResult) {
				log.Printf("[Agent] pipeline: review findings INCREASING — cost diverging, force-advancing after %d cycles", a.pipelineCycles)
				a.conversation.Add(providers.Message{
					Role:    "user",
					Content: fmt.Sprintf("Review findings are increasing each cycle (diverging). Accepting current state to stop cost growth:\n%s", reviewResult),
				})
				return a.advancePipeline("PHASE_PASSED")
			}

			// Check for stability — if code and issues unchanged for 2 cycles, accept
			if a.reviewTracker.CheckStability(a.config.WorkDir, reviewResult) {
				log.Printf("[Agent] pipeline: review stable — accepting result after %d cycles", a.pipelineCycles)
				a.conversation.Add(providers.Message{
					Role:    "user",
					Content: fmt.Sprintf("Review found issues but no improvement detected across cycles. Accepting current state:\n%s", reviewResult),
				})
				return a.advancePipeline("PHASE_PASSED")
			}

			if a.pipelineCycles > maxPipelineCycles {
				log.Printf("[Agent] pipeline: max review cycles reached (%d), accepting result", a.pipelineCycles)
				a.pipelineCycles = 0
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
			log.Printf("[Agent] pipeline: review cycle %d/%d — fixing on %s (provider idx %d, escalated=%v)",
				a.pipelineCycles, maxPipelineCycles, providerName, a.providerIdx, escalated)
			a.conversation.Add(providers.Message{
				Role: "user",
				Content: fmt.Sprintf("The %s review found issues (cycle %d/%d). Fix ALL these issues using tools, then say IMPLEMENT_COMPLETE:\n---\n%s", nextPhase.Name, a.pipelineCycles, maxPipelineCycles, reviewResult),
			})
			// Go back to self-check — after the fix, self-check re-runs, then code-review with next reviewer
			a.pipelinePhase = a.findPhaseIndex("self-check", a.pipelinePhase-1)
			a.phaseToolCalls = 0
			a.phaseTurns = 0 // reset turn cap for the new phase
			return true
		}

		prompt := a.pipeline.PhasePrompt(a.pipelinePhase, a.acceptanceCriteria, a.originalRequest)
		a.conversation.Add(providers.Message{
			Role:    "user",
			Content: prompt,
		})
		return true
	}

	if IsFailSignal(content) {
		a.pipelineCycles++

		// Check for divergence — if findings are increasing, stop cycling (cost is growing)
		if a.reviewTracker.CheckDivergence(content) {
			log.Printf("[Agent] pipeline: review findings INCREASING — cost diverging, force-advancing after %d cycles", a.pipelineCycles)
			a.pipelinePhase++
			a.phaseToolCalls = 0
			a.phaseTurns = 0
			a.phaseRetries = 0
			a.lastGateScore = 0
			a.plateauCount = 0
			a.toolFingerprints = nil
			a.loopWarningCount = 0
			a.reviewTracker.Reset()
			if a.pipelinePhase >= len(a.pipeline.Phases) {
				return false
			}
			nextPhase := a.pipeline.Phases[a.pipelinePhase]
			log.Printf("[Agent] pipeline: force-advancing to phase %d/%d: %s (diverging reviews)",
				a.pipelinePhase+1, len(a.pipeline.Phases), nextPhase.Name)
			prompt := a.pipeline.PhasePrompt(a.pipelinePhase, a.acceptanceCriteria, a.originalRequest)
			a.conversation.Add(providers.Message{
				Role:    "user",
				Content: prompt,
			})
			return true
		}

		// Check for stability — if code and issues unchanged for 2 cycles, force-advance
		if a.reviewTracker.CheckStability(a.config.WorkDir, content) {
			log.Printf("[Agent] pipeline: review stable — force-advancing past phase after %d cycles", a.pipelineCycles)
			a.pipelinePhase++
			a.phaseToolCalls = 0
			a.phaseTurns = 0
			a.phaseRetries = 0
			a.lastGateScore = 0
			a.plateauCount = 0
			a.toolFingerprints = nil
			a.loopWarningCount = 0
			a.reviewTracker.Reset()
			if a.pipelinePhase >= len(a.pipeline.Phases) {
				return false
			}
			nextPhase := a.pipeline.Phases[a.pipelinePhase]
			log.Printf("[Agent] pipeline: force-advancing to phase %d/%d: %s",
				a.pipelinePhase+1, len(a.pipeline.Phases), nextPhase.Name)
			prompt := a.pipeline.PhasePrompt(a.pipelinePhase, a.acceptanceCriteria, a.originalRequest)
			a.conversation.Add(providers.Message{
				Role:    "user",
				Content: prompt,
			})
			return true
		}

		if a.pipelineCycles > maxPipelineCycles {
			log.Printf("[Agent] pipeline: max review cycles reached, force-advancing past failing phase")
			a.pipelineCycles = 0
			a.pipelinePhase++
			a.phaseToolCalls = 0
			a.phaseTurns = 0
			a.phaseRetries = 0
			a.lastGateScore = 0
			a.plateauCount = 0
			a.toolFingerprints = nil
			a.loopWarningCount = 0
			a.reviewTracker.Reset()
			if a.pipelinePhase >= len(a.pipeline.Phases) {
				return false
			}
			nextPhase := a.pipeline.Phases[a.pipelinePhase]
			log.Printf("[Agent] pipeline: force-advancing to phase %d/%d: %s",
				a.pipelinePhase+1, len(a.pipeline.Phases), nextPhase.Name)
			prompt := a.pipeline.PhasePrompt(a.pipelinePhase, a.acceptanceCriteria, a.originalRequest)
			a.conversation.Add(providers.Message{
				Role:    "user",
				Content: prompt,
			})
			return true
		}
		a.escalateProvider()
		log.Printf("[Agent] pipeline: phase %s FAILED (cycle %d/%d), escalated to provider idx %d",
			currentPhase.Name, a.pipelineCycles, maxPipelineCycles, a.providerIdx)
		a.phaseToolCalls = 0
		a.phaseTurns = 0 // reset turn cap for retry

		a.conversation.Add(providers.Message{
			Role: "user",
			Content: fmt.Sprintf("The %s phase found issues (cycle %d/%d). Fix them now using tools, then say IMPLEMENT_COMPLETE.", currentPhase.Name, a.pipelineCycles, maxPipelineCycles),
		})
		return true
	}

	// Not a clear pass/fail — this is the first time entering the pipeline
	log.Printf("[Agent] pipeline: starting phase %d/%d: %s",
		a.pipelinePhase+1, len(a.pipeline.Phases), currentPhase.Name)

	// Parallel phase starting: spawn sub-agents immediately
	canRunParallel := currentPhase.ParallelSubAgents > 0 && len(a.currentLevelProviders()) > 0
	if currentPhase.Name == "plan" {
		canRunParallel = currentPhase.ParallelSubAgents > 0 && len(currentPhase.CoderProviders) > 0 && a.hasProviders(currentPhase.CoderProviders)
	}
	if canRunParallel {
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

	prompt := a.pipeline.PhasePrompt(a.pipelinePhase, a.acceptanceCriteria, a.originalRequest)
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
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	model := "auto"

	// Dynamically match skills and extract verification commands (single matching pass)
	query := a.originalRequest
	if query == "" {
		query = a.conversation.LastUserMessage()
	}
	matched := orchestration.MatchSkillsForLanguage(query, a.config.Skills, a.config.ProjectLanguage)
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
			// Budget exhaustion means the sub-agent ran out of turns/tokens — escalate
			// to a bigger model and retry once before giving up.
			if IsBudgetExhausted(err) {
				log.Printf("[Agent] sub-agent %s hit budget — escalating provider and retrying", phase.Name)
				if a.escalateProvider() {
					retryProvider := ""
					if provs := a.currentLevelProviders(); len(provs) > 0 {
						retryProvider = provs[0]
					}
					retryResult, retryErr := a.runSingleReviewer(ctx, phase, model, retryProvider, skillContext, verifySection)
					if retryErr == nil {
						return retryResult
					}
					log.Printf("[Agent] escalated retry also failed: %v", retryErr)
				}
			}
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
EXTRACTED SPEC CONSTRAINTS (MANDATORY — flag any violations):
%s

Your job:
1. Run EVERY verification command listed above. Report the output and PASS/FAIL for each.
2. Use tools (file_read, grep, bash) to inspect ALL actual outputs. Trust NOTHING without evidence.
3. Compare each output against the original request and acceptance criteria.
4. For [MANUAL] checks: read the relevant code and verify the stated condition.
5. Check for: null values, zero values, empty fields, missing structure.
6. Check all spec constraints above — any violation is a FAIL.
7. Say VERIFIED_CORRECT only if ALL verification commands pass AND all criteria are met.
   Otherwise say NEEDS_FIX with every specific issue listed.`,
				step+1, n, a.originalRequest, a.acceptanceCriteria, skillContext, verifySection, a.formatConstraintsBlock())
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

EXTRACTED SPEC CONSTRAINTS (MANDATORY — fixes must respect these):
%s

Fix every issue. Run verification commands to confirm fixes work.
Say VERIFIED_CORRECT if all fixed, or NEEDS_FIX if you couldn't fix something.`,
				step+1, n, a.originalRequest, a.acceptanceCriteria, prevReview, skillContext, a.formatConstraintsBlock())
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
EXTRACTED SPEC CONSTRAINTS (MANDATORY — flag any violations):
%s

Verify the fixes are correct. Run ALL verification commands.
Check all spec constraints above — any violation is a FAIL.
Say VERIFIED_CORRECT if everything passes, or NEEDS_FIX with remaining issues.`,
				step+1, n, a.originalRequest, a.acceptanceCriteria, prevFix, skillContext, verifySection, a.formatConstraintsBlock())
		}

		log.Printf("[Agent] sub-agent %s step %d/%d: provider %s (%s)",
			phase.Name, step+1, n, provider,
			map[bool]string{true: "review", false: "fix"}[step%2 == 0])

		result, err := a.RunChild(ctx, SpawnConfig{
			Role:     fmt.Sprintf("%s-step-%d", phase.Name, step+1),
			Model:    model,
			Provider: provider,
			Tier:     phase.Tier,
			Budget:   &AgentBudget{MaxTurns: a.maxPhaseTurns()},
		}, task)

		// Budget exhaustion: escalate provider and retry this step once
		if err != nil && IsBudgetExhausted(err) {
			log.Printf("[Agent] sub-agent step %d hit budget — escalating and retrying", step+1)
			if a.escalateProvider() {
				retryProviders := a.currentLevelProviders()
				retryProvider := provider
				if len(retryProviders) > 0 {
					retryProvider = retryProviders[0]
				}
				result, err = a.RunChild(ctx, SpawnConfig{
					Role:     fmt.Sprintf("%s-step-%d-retry", phase.Name, step+1),
					Model:    model,
					Provider: retryProvider,
					Tier:     phase.Tier,
					Budget:   &AgentBudget{MaxTurns: a.maxPhaseTurns()},
				}, task)
				if err != nil {
					log.Printf("[Agent] escalated retry step %d also failed: %v", step+1, err)
				}
			}
		}

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

// cloneForSubAgent creates a lightweight clone of the agent with its own
// copy of mutable escalation state (providerIdx, levelRotationIdx).
// Shared immutable state (config, executor, registry, tools) is referenced,
// not copied. This prevents data races in runParallelSubAgentPhases.
func (a *Agent) cloneForSubAgent() *Agent {
	clone := &Agent{
		executor:           a.executor,
		registry:           a.registry,
		permissions:        a.permissions,
		conversation:       a.conversation,
		renderer:           a.renderer,
		config:             a.config,
		sessionID:          a.sessionID,
		pool:               a.pool,
		trace:              a.trace,
		metrics:            a.metrics,
		bus:                a.bus,
		providerIdx:        a.providerIdx,
		levelRotationIdx:   a.levelRotationIdx,
		originalRequest:    a.originalRequest,
		acceptanceCriteria: a.acceptanceCriteria,
		cachedSkillContext: a.cachedSkillContext,
		cachedPromptLevel:  a.cachedPromptLevel,
		specConstraints:    a.specConstraints,
		reviewTracker:      &ReviewCycleTracker{},
	}
	return clone
}

// runParallelSubAgentPhases runs two UseSubAgent phases simultaneously.
// Both phases spawn independent sub-agents with no shared context, so they
// can safely execute in parallel. Each goroutine gets a snapshot of the
// provider state to avoid racing on providerIdx and levelRotationIdx.
func (a *Agent) runParallelSubAgentPhases(phase1, phase2 PipelinePhase) (string, string) {
	// Snapshot mutable escalation state before spawning goroutines.
	// Each sub-agent clone gets its own copy so concurrent escalation
	// calls don't race on the parent's fields.
	snapshot1 := a.cloneForSubAgent()
	snapshot2 := a.cloneForSubAgent()

	var result1, result2 string
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		result1 = snapshot1.runSubAgentPhase(phase1)
	}()
	go func() {
		defer wg.Done()
		result2 = snapshot2.runSubAgentPhase(phase2)
	}()

	wg.Wait()
	log.Printf("[Agent] parallel verification complete: %s=%v, %s=%v",
		phase1.Name, IsPassSignal(result1), phase2.Name, IsPassSignal(result2))
	return result1, result2
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
EXTRACTED SPEC CONSTRAINTS (MANDATORY — flag any violations):
%s

Your job:
1. Run EVERY verification command listed above. Report the output and PASS/FAIL for each.
2. Use tools (file_read, grep, bash) to inspect ALL actual outputs. Trust NOTHING without evidence.
3. Compare each output against the original request and acceptance criteria.
4. For [MANUAL] checks: read the relevant code and verify the stated condition.
5. Check for: null values, zero values, empty fields, missing structure.
6. Check all spec constraints above — any violation is a FAIL.
7. Say VERIFIED_CORRECT only if ALL verification commands pass AND all criteria are met.
   Otherwise say NEEDS_FIX with every specific issue listed.`,
		a.originalRequest, a.acceptanceCriteria, skillContext, verifySection, a.formatConstraintsBlock())

	log.Printf("[Agent] spawning independent %s sub-agent (no shared context)", phase.Name)

	result, err := a.RunChild(ctx, SpawnConfig{
		Role:     phase.Name,
		Model:    model,
		Provider: provider,
		Tier:     phase.Tier,
		Budget:   &AgentBudget{MaxTurns: a.maxPhaseTurns()},
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

// SetMinProviderLevel enforces monotonic escalation. The provider level can
// only go UP, never down. This is the ONLY way to change providerIdx.
func (a *Agent) SetMinProviderLevel(level int) {
	if level <= a.providerIdx {
		return // already at or above this level
	}
	if len(a.config.EscalationChain) == 0 {
		return
	}
	if level >= len(a.config.EscalationChain) {
		level = len(a.config.EscalationChain) - 1
	}
	fromLevel := a.providerIdx
	a.providerIdx = level
	a.levelRotationIdx = 0
	providers := a.config.EscalationChain[a.providerIdx].Providers
	log.Printf("[Agent] escalating to level %d/%d: %v",
		a.providerIdx+1, len(a.config.EscalationChain), providers)
	tier := ""
	if a.providerIdx < len(a.config.EscalationChain) {
		tier = string(a.config.EscalationChain[a.providerIdx].Tier)
	}
	a.emit(EventEscalation, "", map[string]any{
		"from_level":  fromLevel,
		"to_level":    a.providerIdx,
		"total_levels": len(a.config.EscalationChain),
		"tier":        tier,
		"providers":   fmt.Sprintf("%v", providers),
	})
}

// SetNonInteractive marks the agent as running in one-shot mode (--message).
// The system prompt will instruct the agent not to ask follow-up questions.
func (a *Agent) SetNonInteractive(v bool) {
	a.config.NonInteractive = v
}

// ForceEscalate is the public API for manually triggering provider escalation
// (e.g., from the code mode ^E shortcut).
func (a *Agent) ForceEscalate() bool {
	return a.escalateProvider()
}

// escalateProvider moves to the next provider level. Returns true if escalated.
func (a *Agent) escalateProvider() bool {
	if len(a.config.EscalationChain) == 0 {
		return false
	}
	if a.providerIdx >= len(a.config.EscalationChain)-1 {
		log.Printf("[Agent] escalation chain exhausted — staying on level %d", a.providerIdx)
		return false
	}
	a.SetMinProviderLevel(a.providerIdx + 1)
	return true
}

// currentLevelProviders returns the providers at the current escalation level.
// This is the ONLY way to get providers for non-plan phases — never use cached
// CoderProviders for implement/review phases.
func (a *Agent) currentLevelProviders() []string {
	if len(a.config.EscalationChain) == 0 {
		return nil
	}
	idx := a.providerIdx
	if idx >= len(a.config.EscalationChain) {
		idx = len(a.config.EscalationChain) - 1
	}
	return a.config.EscalationChain[idx].Providers
}

// ProviderLevelForTier returns the first escalation chain index that matches
// the given tier. Returns 0 if no tier is set or no matching level is found.
// Tier classification: cheap = bottom third, mid = middle third, frontier = top third.
// If OLLAMA_CHAIN_TIERS was parsed, uses explicit tier assignments.
// Otherwise auto-classifies from chain position.
func (a *Agent) ProviderLevelForTier(tier ModelTier) int {
	if tier == "" || len(a.config.EscalationChain) == 0 {
		return 0
	}

	// First pass: look for an explicit tier assignment on a chain level.
	for i, level := range a.config.EscalationChain {
		if level.Tier == tier {
			return i
		}
	}

	// Second pass: auto-classify by position (bottom=cheap, middle=mid, top=frontier).
	n := len(a.config.EscalationChain)
	switch tier {
	case TierCheap:
		return 0 // bottom of chain
	case TierMid:
		return n / 3 // start of middle third
	case TierFrontier:
		return (2 * n) / 3 // start of top third
	}
	return 0
}

// initializeImplementPhase sets the initial parallel agent count from Level 0.
// CoderProviders is NOT cached — currentLevelProviders() is used at runtime.
func (a *Agent) initializeImplementPhase() {
	if a.pipeline == nil || len(a.config.EscalationChain) == 0 {
		return
	}
	level0 := a.config.EscalationChain[0]
	for i := range a.pipeline.Phases {
		phase := &a.pipeline.Phases[i]
		if phase.Name == "implement" && phase.ParallelSubAgents == 0 {
			if len(level0.Providers) > 0 {
				phase.ParallelSubAgents = len(level0.Providers)
				log.Printf("[Agent] implement phase: %d parallel coders from chain level 0: %v",
					phase.ParallelSubAgents, level0.Providers)
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

// hasProviders checks if all named providers exist in the escalation chain
// OR the full registered provider list. Planner providers (e.g., ollama-planner-1)
// are registered as standalone providers but not in the escalation chain, so both
// must be checked.
func (a *Agent) hasProviders(names []string) bool {
	knownSet := make(map[string]bool)
	// Check escalation chain
	for _, level := range a.config.EscalationChain {
		for _, p := range level.Providers {
			knownSet[p] = true
		}
	}
	// Check full provider list (includes planners and other standalone providers)
	for _, p := range a.config.Providers {
		knownSet[p] = true
	}
	for _, name := range names {
		if !knownSet[name] {
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

// hasWrittenCode returns true if the agent has written code files in this session.
func (a *Agent) hasWrittenCode() bool {
	return a.wroteCodeFiles
}

// isCodeFilePath checks if a tool call's args reference a code file.
func isCodeFilePath(args map[string]interface{}) bool {
	path, _ := args["path"].(string)
	if path == "" {
		path, _ = args["file_path"].(string)
	}
	if path == "" {
		return false
	}
	codeExtensions := []string{
		".go", ".py", ".js", ".ts", ".java", ".rs", ".rb",
		".cpp", ".c", ".cs", ".swift", ".kt", ".scala",
		".sh", ".bash", ".sql",
	}
	lower := strings.ToLower(path)
	for _, ext := range codeExtensions {
		if strings.HasSuffix(lower, ext) {
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

// compactConversation stores old messages to DB and keeps only recent context.
// Called between pipeline phases to prevent context overflow in long sessions.
func (a *Agent) compactConversation(completedPhase string) {
	msgs := a.conversation.Messages()
	if len(msgs) <= 30 {
		return // not enough to compact
	}

	keepCount := 20 // keep last 20 messages
	dropCount := len(msgs) - keepCount

	// Store dropped messages to DB for later recall (all roles including tool).
	a.storeMessagesToDB(msgs[:dropCount], "compaction")

	// Replace conversation with summary + recent messages
	recent := make([]providers.Message, keepCount)
	copy(recent, msgs[dropCount:])

	a.conversation.Clear()
	a.conversation.Add(providers.Message{
		Role:    "user",
		Content: fmt.Sprintf("[Phase %s completed. %d earlier messages compacted to DB. Use recall tool to access past context.]", completedPhase, dropCount),
	})
	for _, m := range recent {
		a.conversation.Add(m)
	}

	a.hasCompacted = true
	log.Printf("[Agent] compacted conversation: dropped %d messages, kept %d + summary", dropCount, keepCount)
}

// toolIdentityKeys defines which argument keys constitute the "identity" of a tool call.
// Only these keys are hashed for loop detection. Payload keys (file content, edit text)
// are excluded so agents can't evade detection by varying file content.
var toolIdentityKeys = map[string][]string{
	"file_write": {"path"},
	"file_edit":  {"path"},
	"file_read":  {"path"},
	"grep":       {"pattern", "path"},
	"glob":       {"pattern", "path"},
	"git":        {"subcommand"},
}

// toolCallFingerprint returns a short hash capturing the INTENT of a tool call.
// For file tools, only the path is hashed (not content). For bash, the command
// is normalized to first two tokens. For unknown tools, all args are hashed.
func toolCallFingerprint(name string, args map[string]interface{}) string {
	var data string

	if name == "bash" {
		cmd, _ := args["command"].(string)
		data = name + "|" + normalizeBashCommand(cmd)
	} else if keys, ok := toolIdentityKeys[name]; ok {
		data = name
		for _, k := range keys {
			if v, exists := args[k]; exists {
				data += "|" + fmt.Sprintf("%v", v)
			}
		}
	} else {
		b, _ := json.Marshal(args)
		data = name + string(b)
	}

	h := sha256.Sum256([]byte(data))
	return hex.EncodeToString(h[:8])
}

// normalizeBashCommand extracts the first two tokens of a shell command
// (command + subcommand), ignoring flags and subsequent args.
// "npm install --legacy-peer-deps" → "npm|install"
// "go test -race ./..." → "go|test"
func normalizeBashCommand(cmd string) string {
	fields := strings.Fields(strings.TrimSpace(cmd))
	if len(fields) == 0 {
		return ""
	}
	result := fields[0]
	if len(fields) > 1 && !strings.HasPrefix(fields[1], "-") {
		result += "|" + fields[1]
	}
	return result
}

// maxRepeatCount returns the highest occurrence count of any fingerprint in the window.
func maxRepeatCount(fps []string) int {
	counts := make(map[string]int)
	mx := 0
	for _, fp := range fps {
		counts[fp]++
		if counts[fp] > mx {
			mx = counts[fp]
		}
	}
	return mx
}

// loopDetectedMessage returns a message telling the agent it's repeating itself.
func loopDetectedMessage(repeats int) providers.Message {
	return providers.Message{
		Role: "user",
		Content: fmt.Sprintf(`LOOP DETECTED: You have called the same tool with the same arguments %d times.
This is NOT making progress. You MUST try a DIFFERENT approach:
- If a command keeps failing, investigate WHY (read the error output carefully)
- If a dependency won't install, try a different version or skip it
- If a file won't compile, read the specific error and fix it
- Do NOT retry the same command — it will produce the same result
Change your approach NOW.`, repeats),
	}
}

// runParallelPhase spawns N parallel sub-agents working concurrently.
// For plan phases: each produces a plan, then MergeProvider synthesizes.
// For implement phases: dynamic role assignment — agent 1 implements, agent 2 tests,
// agent 3+ bug-reviews. Stage 2 cross-review: each model reviews another's output.
func (a *Agent) runParallelPhase(phase PipelinePhase) string {
	// Use a timeout context rather than background to prevent runaway sub-agents
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	// For plan phases, use the hardcoded planner providers.
	// For ALL other phases, use currentLevelProviders() — never stale cached values.
	levelProviders := phase.CoderProviders // plan phase keeps its own
	if phase.Name != "plan" {
		levelProviders = a.currentLevelProviders()
	}

	n := phase.ParallelSubAgents
	if n > len(levelProviders) {
		n = len(levelProviders)
	}

	// Build task descriptions based on phase type
	type taskDef struct {
		role, provider, task, workDir string
	}
	var tasks []taskDef

	// Dynamically match skills against the original request — same skills
	// the parent agent has, passed to every sub-agent for domain awareness
	skillContext := a.matchedSkillContext()

	constraintsBlock := ""
	if a.specConstraints != nil {
		constraintsBlock = a.specConstraints.FormatConstraints()
	}

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

EXTRACTED SPEC CONSTRAINTS (MANDATORY — your plan must respect these):
%s

Your plan MUST begin with a SPEC CONSTRAINTS section that explicitly lists:
- Required package/directory structure
- OUT OF SCOPE items (things you must NOT build)
- Prohibited patterns (e.g., "no service layer")
If you skip this section, the plan will be rejected.

0. SPEC PERCEPTION (do this FIRST):
   Before planning, restate the spec's key architectural decisions:
   - Required package/directory structure?
   - IN SCOPE and OUT OF SCOPE?
   - Mandated/prohibited design patterns?
   - Technology constraints?
   If the spec has an "Acceptance Criteria" section, EXTRACT those criteria verbatim — do not generate your own.
   If no spec is provided, state "No spec provided" in the SPEC CONSTRAINTS section and proceed.

Produce:
1. TASK DECOMPOSITION: ordered subtasks with dependencies
2. ACCEPTANCE CRITERIA for each subtask AND overall deliverable
   - Reference the skill documentation above for correct formats, APIs, and patterns
3. UNKNOWNS and ASSUMPTIONS
4. DEFINITION OF DONE

Be thorough and specific. Output your complete plan, then say PLAN_COMPLETE.`, a.originalRequest, skillContext, constraintsBlock),
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

SKILL REFERENCE (use as reference patterns — spec requirements and acceptance criteria take priority):
%s

EXTRACTED SPEC CONSTRAINTS (MANDATORY — your implementation must respect these):
%s

BEFORE WRITING ANY CODE, verify these spec requirements:
1. What package name does the spec require? (Use ONLY this, not defaults)
2. What is OUT OF SCOPE? (Do NOT create these)
3. What directory structure does the spec mandate?
If acceptance criteria contradict the original spec, FOLLOW THE SPEC.

Focus on: main implementation files, data structures, core logic, API integration.
Use skill patterns as references — the original request takes priority over examples.
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

SKILL REFERENCE (use as reference patterns — spec requirements and acceptance criteria take priority):
%s

EXTRACTED SPEC CONSTRAINTS (MANDATORY — your tests must validate these):
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

EXTRACTED SPEC CONSTRAINTS (MANDATORY — flag any violations):
%s

Focus on: reading all code written so far, finding bugs, logic errors, missing edge cases,
violations of the skill reference patterns and spec constraints, and potential runtime failures.
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
				provider: levelProviders[i],
				task:     fmt.Sprintf(rolePrompts[roleIdx], i+1, n, a.originalRequest, a.acceptanceCriteria, skillContext, constraintsBlock),
			})
		}
	}

	log.Printf("[Agent] spawning %d parallel %s sub-agents", len(tasks), phase.Name)

	providerNames := make([]string, 0, len(tasks))
	for _, t := range tasks {
		providerNames = append(providerNames, t.provider)
	}
	a.emit(EventParallelStart, "", map[string]any{
		"agent_count": len(tasks),
		"phase":       phase.Name,
		"providers":   fmt.Sprintf("%v", providerNames),
	})

	// For implement phases, give each agent its own temp directory to prevent file conflicts
	// Cleanup function defined first, then used after temp dirs are created
	var tempDirs []string
	cleanup := func() {
		for _, dir := range tempDirs {
			os.RemoveAll(dir)
		}
	}
	if phase.Name != "plan" {
		for i := range tasks {
			subDir, err := os.MkdirTemp("", fmt.Sprintf("synroute-parallel-%s-*", tasks[i].role))
			if err != nil {
				log.Printf("[Agent] failed to create temp dir for %s: %v, using shared dir", tasks[i].role, err)
				continue
			}
			copyDirContents(a.config.WorkDir, subDir)
			tasks[i].workDir = subDir
			tempDirs = append(tempDirs, subDir)
			log.Printf("[Agent] parallel agent %s using isolated dir: %s", tasks[i].role, subDir)
		}
		defer cleanup()
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
				Budget:   &AgentBudget{MaxTurns: a.maxPhaseTurns()},
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
	budgetExhaustedCount := 0
	for i := 0; i < len(tasks); i++ {
		r := <-results
		combined.WriteString(fmt.Sprintf("\n=== %s ===\n", r.role))
		if r.err != nil {
			if IsBudgetExhausted(r.err) {
				budgetExhaustedCount++
			}
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

	// If any sub-agent hit its budget, escalate the provider so subsequent
	// phases (review, acceptance) use a bigger model that can finish in fewer turns.
	if budgetExhaustedCount > 0 {
		log.Printf("[Agent] %d/%d parallel sub-agents hit budget — escalating provider",
			budgetExhaustedCount, len(tasks))
		a.escalateProvider()
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
			a.emit(EventCrossReview, levelProviders[i], map[string]any{
				"reviewer": fmt.Sprintf("agent-%d", i+1),
				"target":   reviewTarget,
				"step":     i + 1,
			})
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

EXTRACTED SPEC CONSTRAINTS (MANDATORY — flag any violations):
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
				skillContext, constraintsBlock,
				reviewTarget, reviewOutput)

			go func(idx int, task string) {
				defer func() {
					if r := recover(); r != nil {
						crossResults <- result{role: fmt.Sprintf("cross-review-%d", idx+1), err: fmt.Errorf("panic: %v", r)}
					}
				}()
				out, err := a.RunChild(ctx, SpawnConfig{
					Role:     fmt.Sprintf("cross-review-%d", idx+1),
					Provider: levelProviders[idx],
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

EXTRACTED SPEC CONSTRAINTS (MANDATORY — the merged plan must respect these):
%s

Output the MERGED plan with complete acceptance criteria that reference the skill specs. Say PLAN_COMPLETE.`, a.originalRequest, combined.String(), skillContext, constraintsBlock)

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

// callLLMWithStreaming tries to use streaming if the executor supports it.
// Emits EventTokenStream for each token so the renderer can display them inline.
// Falls back to callLLMWithRetry if streaming isn't available.
func (a *Agent) callLLMWithStreaming(ctx context.Context, req providers.ChatRequest) (providers.ChatResponse, error) {
	// Check if the executor supports streaming via the router
	if streamer, ok := a.executor.(interface {
		ChatCompletionStreamForProvider(ctx context.Context, req providers.ChatRequest, sessionID, provider string, onToken providers.TokenCallback) (providers.ChatResponse, error)
	}); ok && a.config.Streaming {
		provider := ""
		if len(a.config.EscalationChain) > 0 && a.providerIdx < len(a.config.EscalationChain) {
			level := a.config.EscalationChain[a.providerIdx]
			if len(level.Providers) > 0 {
				provider = level.Providers[a.levelRotationIdx%len(level.Providers)]
			}
		}
		if provider != "" {
			onToken := func(token string) {
				a.emit(EventTokenStream, provider, map[string]any{"token": token})
			}
			resp, err := streamer.ChatCompletionStreamForProvider(ctx, req, a.sessionID, provider, onToken)
			if err == nil {
				return resp, nil
			}
			log.Printf("[Agent] streaming failed, falling back to non-streaming: %v", err)
		}
	}
	return a.callLLMWithRetry(ctx, req)
}

// callLLMWithRetry attempts the LLM call with retry and automatic escalation.
// If all providers at the current level fail, escalates to the next level and retries.
// Only returns error if all levels are exhausted or a non-retryable error occurs.
func (a *Agent) callLLMWithRetry(ctx context.Context, req providers.ChatRequest) (providers.ChatResponse, error) {
	backoffs := []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second}
	var lastErr error

	for attempt := 0; attempt <= len(backoffs); attempt++ {
		resp, err := a.executeForCurrentProvider(ctx, req)
		if err == nil {
			return resp, nil
		}
		lastErr = err

		// All providers at current level failed — escalate instead of retrying same level
		if strings.Contains(err.Error(), "all providers at level") {
			if a.escalateProvider() {
				log.Printf("[Agent] all providers at level %d failed — escalated to level %d",
					a.providerIdx-1, a.providerIdx)
				continue
			}
			return providers.ChatResponse{}, fmt.Errorf("all escalation levels exhausted: %w", err)
		}

		// Context overflow — persist messages to DB, then trim and retry
		if isContextOverflowError(err) {
			// Store messages to DB before they are lost
			allMsgs := a.conversation.Messages()
			if len(allMsgs) > 20 {
				a.storeMessagesToDB(allMsgs[:20], "context_overflow_trim")
			}
			trimmed := a.conversation.TrimOldest(20)
			if trimmed > 0 {
				log.Printf("[Agent] context overflow — stored and trimmed %d old messages, retrying", trimmed)
				req.Messages = a.buildMessages()
				continue
			}
			return providers.ChatResponse{}, fmt.Errorf("context overflow, cannot trim further: %w", err)
		}

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

// storeMessagesToDB persists messages to VectorMemory before they are dropped.
// Used by emergency trim and compaction to ensure zero information loss.
func (a *Agent) storeMessagesToDB(msgs []providers.Message, source string) {
	if a.config.VectorMemory == nil {
		return
	}
	for _, m := range msgs {
		content := m.Content

		// For assistant messages with empty Content but ToolCalls, serialize the tool calls
		if content == "" && len(m.ToolCalls) > 0 {
			b, err := json.Marshal(m.ToolCalls)
			if err == nil {
				content = fmt.Sprintf("[tool_calls: %s]", string(b))
			}
		}

		if content == "" {
			continue
		}

		metadata := map[string]interface{}{"source": source}
		if m.ToolCallID != "" {
			metadata["tool_call_id"] = m.ToolCallID
		}

		if err := a.config.VectorMemory.Store(content, m.Role, a.sessionID, metadata); err != nil {
			log.Printf("[Agent] storeMessagesToDB: failed to store message: %v", err)
		}
	}
}

// isContextOverflowError returns true if the error indicates the request exceeds
// the model's context window. Previously dead code — now wired into callLLMWithRetry.
func isContextOverflowError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "too long") ||
		strings.Contains(msg, "context length") ||
		strings.Contains(msg, "maximum context") ||
		strings.Contains(msg, "token limit") ||
		strings.Contains(msg, "request too large")
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

// modelMaxMessages returns the conversation message limit based on the model's
// context window. Large-context models can hold more messages before compaction.
func modelMaxMessages(model string) int {
	lower := strings.ToLower(model)
	switch {
	case strings.Contains(lower, "gemini"):
		return 500 // Gemini models have 1M+ context
	case strings.Contains(lower, "claude"):
		return 400 // Claude models have 200K context
	case strings.Contains(lower, "deepseek"):
		return 300 // DeepSeek models have 128K context
	default:
		return 0 // 0 = use default (maxConversationMessages = 200)
	}
}

func uniqueID() int64 {
	return atomic.AddInt64(&idCounter, 1)
}

// resolvePathForCache resolves a tool path to an absolute canonical form for cache keying.
// Mirrors the logic in tools.resolveToolPath but is agent-local to avoid exporting internals.
func resolvePathForCache(path, workDir string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, path[2:])
		}
	}
	resolved := filepath.Clean(path)
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(workDir, resolved)
	}
	return resolved
}
