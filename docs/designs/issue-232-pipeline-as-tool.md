# Technical Design: Pipeline as a Tool the Frontier Model Calls

**Issue:** [#232](https://github.com/gr3enarr0w/synapserouter/issues/232) — Pipeline as a tool the frontier model calls, not a forced mode

**Status:** Design Proposal
**Date:** 2026-03-28

---

## Problem Statement

Currently, every `synroute chat` session with `AutoOrchestrate=true` unconditionally forces a 6-phase pipeline (plan, implement, self-check, code-review, acceptance-test, deploy) on every user message. This creates three concrete problems:

1. **Wasted tokens on simple tasks.** Asking "what does this function do?" forces the agent through plan phase, injects phase prompts, runs verification gates, and spawns sub-agents for code review — all for a question that needs a single file read.

2. **No conversational iteration.** Each message restarts pipeline state. Users cannot have a back-and-forth discussion that evolves into structured work. The pipeline is all-or-nothing.

3. **The model is smarter than the harness.** The frontier model (Claude, GPT-5.3, Gemini) is capable of determining when structured phases are needed. The current design second-guesses it with rigid phase sequencing, stall detection, loop counters, and force-advance logic — 600+ lines of orchestration code in `agent.go` that exists only because the pipeline cannot be invoked selectively.

---

## Research Findings

### How Other Tools Handle "Chat vs. Structured Work"

| Tool | Approach | Who Decides? | Pipeline Structure |
|------|----------|-------------|-------------------|
| **Claude Code** | Single agentic loop with tool calling. No forced phases. The model decides when to search, edit, test, or delegate to sub-agents. Structured work emerges from the model's own reasoning. | The model | None — emergent from tool use |
| **Cursor** | Three explicit modes: **Ask** (read-only, no edits), **Agent** (autonomous with tools), **Custom** (user-defined tool sets). The Composer model was RL-trained to decide which tools to use. | User picks mode; model picks tools within mode | None — ReAct loop with 25-tool-call checkpoints |
| **Aider** | User toggles between `/ask` (discuss), `/code` (edit), `/architect` (two-model plan+edit). No autonomous pipeline. Planning is a manual user-initiated mode, not auto-detected. | The user | None — mode-based, user-controlled |
| **OpenAI Codex** | App Server protocol with Items, Turns, and Threads. Three approval levels (Suggest, Auto Edit, Full Auto). No forced pipeline — the model proposes actions, the harness executes and pauses for approval when needed. | The model, with approval gates | None — approval-gated ReAct loop |
| **Amazon Kiro** | Spec-driven phases (Requirements, Design, Tasks) generate `.md` files. Tasks are executed **one at a time on user trigger**, not auto-sequenced. Agent hooks provide pre/post tool-use automation. | User triggers each phase; agent hooks automate within phases | Explicit phases, but user-triggered, not auto-sequenced |

### Key Takeaways

1. **No major coding agent forces a multi-phase pipeline.** Claude Code, Cursor Agent, and Codex all use a simple agentic loop where the model decides what to do. Structured phases (plan, test, review) emerge from the model's reasoning, not from harness-level state machines.

2. **The model should be the router.** Cursor's Composer model was specifically RL-trained inside real codebases to learn when to search, edit, test, and navigate. The lesson: train/prompt the model to invoke structured work, don't force it from the harness.

3. **Kiro is the closest analog — and it still lets the user trigger phases.** Kiro generates specs and task lists but lets the developer trigger implementation task-by-task. Even the most structured tool in the market does not auto-sequence phases.

4. **The industry consensus is "workflow for predictable tasks, agent for complex ones."** Multiple sources (Towards Data Science, ZenML, Vellum) recommend: use deterministic workflows when the steps are known; use autonomous agents when the steps depend on intermediate results. Synapserouter's pipeline tries to be both and ends up being neither.

5. **Tool-call limits provide natural checkpoints.** Cursor stops every 25 tool calls for review. Codex pauses for approval on write operations. These lightweight checkpoints replace the need for rigid phase gates.

---

## Technical Design

### Core Idea

Replace the `AutoOrchestrate` flag and the `advancePipeline()` state machine with **pipeline phase tools** that the frontier model can invoke when it determines structured work is appropriate. The agent loop stays simple (message -> LLM -> tool calls -> repeat). Pipeline phases become tools like `bash`, `file_read`, and `grep` — available but not forced.

### New Tools

Six new tools, registered in the tool registry alongside existing tools:

#### 1. `pipeline_plan`

```json
{
  "name": "pipeline_plan",
  "description": "Create a structured plan with acceptance criteria for a complex task. Use this when the task requires multiple files, has a spec to follow, or needs coordinated changes. Do NOT use for simple questions, single-file edits, or information lookups.",
  "parameters": {
    "type": "object",
    "properties": {
      "task": {
        "type": "string",
        "description": "The task to plan. Include any spec content or requirements."
      },
      "scope": {
        "type": "string",
        "enum": ["small", "medium", "large"],
        "description": "small: 1-3 files, medium: 4-10 files, large: 10+ files or multi-component"
      }
    },
    "required": ["task"]
  }
}
```

**Execution:** Runs the planning logic currently in `DefaultPipeline.Phases[0]`. For `medium`/`large` scope, spawns parallel planning sub-agents (existing `runParallelPhase` logic). Returns structured plan + acceptance criteria. Stores criteria in agent state for later verification tools to reference.

#### 2. `pipeline_implement`

```json
{
  "name": "pipeline_implement",
  "description": "Execute an implementation plan by delegating to a sub-agent. Use after pipeline_plan when the implementation is complex enough to benefit from dedicated focus. The sub-agent works in the same directory with the same tools.",
  "parameters": {
    "type": "object",
    "properties": {
      "plan": {
        "type": "string",
        "description": "The implementation plan to execute (output from pipeline_plan)"
      },
      "parallel": {
        "type": "boolean",
        "description": "Split work across parallel sub-agents (for large scope)"
      }
    },
    "required": ["plan"]
  }
}
```

**Execution:** Spawns a sub-agent (existing `SpawnChild` + `RunChild`) with the plan as the task. Uses existing provider escalation chain. Returns implementation summary. Optional parallel mode uses existing `RunChildrenParallel`.

#### 3. `pipeline_verify`

```json
{
  "name": "pipeline_verify",
  "description": "Run programmatic verification: build, test, lint, and spec compliance checks. Use after making code changes to verify correctness. Returns pass/fail with details for each check.",
  "parameters": {
    "type": "object",
    "properties": {
      "checks": {
        "type": "array",
        "items": {"type": "string", "enum": ["build", "test", "lint", "spec"]},
        "description": "Which checks to run. Default: all applicable checks."
      }
    }
  }
}
```

**Execution:** Wraps existing `RunVerificationGate()` logic. Runs build/test/lint commands from environment detection. Runs spec compliance checks if `specConstraints` are set. Returns structured results (pass/fail per check with output).

#### 4. `pipeline_review`

```json
{
  "name": "pipeline_review",
  "description": "Request an independent code review from a fresh sub-agent with no shared conversation context. The reviewer uses a larger model than the current session. Use after implementation is complete and pipeline_verify passes.",
  "parameters": {
    "type": "object",
    "properties": {
      "criteria": {
        "type": "string",
        "description": "Acceptance criteria to review against (output from pipeline_plan)"
      },
      "scope": {
        "type": "string",
        "description": "What to review: file paths, component names, or 'all' for full review"
      }
    },
    "required": ["criteria"]
  }
}
```

**Execution:** Spawns a fresh sub-agent (`UseSubAgent=true` logic) with `Escalate=true` (forces bigger model). Injects acceptance criteria and original spec. Returns review findings (pass/fail with specifics). This is the existing `runSubAgentPhase` logic wrapped as a tool.

#### 5. `pipeline_test`

```json
{
  "name": "pipeline_test",
  "description": "Run end-to-end acceptance testing from the user's perspective. Spawns a fresh sub-agent that executes/calls/opens the output as a user would, checking every aspect of the end-user experience.",
  "parameters": {
    "type": "object",
    "properties": {
      "criteria": {
        "type": "string",
        "description": "Acceptance criteria to test against"
      },
      "spec": {
        "type": "string",
        "description": "Original spec/request for scope verification"
      }
    },
    "required": ["criteria"]
  }
}
```

**Execution:** Spawns an acceptance-test sub-agent (existing logic from `acceptance-test` phase). Escalated model. Returns pass/fail with user-perspective findings.

#### 6. `pipeline_status`

```json
{
  "name": "pipeline_status",
  "description": "Check the current state of pipeline work in this session: what has been planned, implemented, verified, and reviewed. Use to track progress on complex tasks.",
  "parameters": {}
}
```

**Execution:** Returns the agent's current pipeline state: stored acceptance criteria, verification results, review outcomes, tool call counts. Read-only, no side effects.

### Tool Implementation Architecture

All pipeline tools live in a new file: `internal/agent/pipeline_tools.go`. They are **agent-aware tools** (like the existing `DelegateTool` and `HandoffTool`) — they need access to agent state (executor, config, escalation chain, spec constraints) that regular tools in `internal/tools/` don't have.

```go
// internal/agent/pipeline_tools.go

// PipelinePlanTool implements the pipeline_plan tool.
type PipelinePlanTool struct {
    agent *Agent
}

func (t *PipelinePlanTool) Name() string        { return "pipeline_plan" }
func (t *PipelinePlanTool) Category() ToolCategory { return tools.CategoryWrite }
func (t *PipelinePlanTool) Description() string  { return "Create a structured plan..." }
func (t *PipelinePlanTool) InputSchema() map[string]interface{} { /* schema above */ }

func (t *PipelinePlanTool) Execute(ctx context.Context, args map[string]interface{}, workDir string) (*tools.ToolResult, error) {
    task, _ := args["task"].(string)
    scope, _ := args["scope"].(string)

    // Reuse existing planning logic
    if scope == "large" || scope == "medium" {
        // Spawn parallel planners (existing runParallelPhase logic)
        result := t.agent.runPlanningPhase(task)
        t.agent.acceptanceCriteria = result
        return &tools.ToolResult{Output: result}, nil
    }

    // For small scope, the model plans inline — just store criteria
    t.agent.acceptanceCriteria = task
    return &tools.ToolResult{
        Output: "Plan registered. Acceptance criteria stored for verification tools.",
    }, nil
}
```

### Registration

Pipeline tools are registered in `Agent.Run()` after the agent is initialized, since they need the `*Agent` reference:

```go
// In Agent.Run(), after existing tool setup:
if !a.config.AutoOrchestrate {  // new model: tools, not forced pipeline
    a.registry.Register(&PipelinePlanTool{agent: a})
    a.registry.Register(&PipelineImplementTool{agent: a})
    a.registry.Register(&PipelineVerifyTool{agent: a})
    a.registry.Register(&PipelineReviewTool{agent: a})
    a.registry.Register(&PipelineTestTool{agent: a})
    a.registry.Register(&PipelineStatusTool{agent: a})
}
```

### The Frontier Model Loop (New)

The new loop is dramatically simpler than the current one. Here is the diff conceptually:

**Current loop (600+ lines):**
```
loop:
  LLM call
  if no tool calls:
    stall detection (3 turns → escalate)
    advancePipeline (signal parsing, quality gates, sub-agents, fail-back)
    force continuation if phases remain
  if tool calls:
    execute tools
    loop detection (fingerprints, warnings, escalation)
    pipeline signal detection in content
```

**New loop (~80 lines):**
```
loop:
  LLM call
  if no tool calls:
    return response  // the model is done talking
  execute tool calls (including pipeline_* tools)
  // no pipeline state machine, no stall detection, no force-advance
```

The agent loop in `agent.go` function `loop()` strips out:
- All `AutoOrchestrate` conditional blocks (~200 lines)
- `advancePipeline()` and its helpers (~300 lines)
- Stall detection / `noToolTurns` tracking
- Loop detection / `toolFingerprints` tracking
- Phase signal parsing (`IsPassSignal`, `IsFailSignal`)
- `forceToolsMessage` / forced continuation
- Phase turn caps (`maxPhaseTurns`, `phaseTurns`)

These behaviors move into the pipeline tools or become unnecessary:
- **Quality gates** → built into `pipeline_verify` tool
- **Sub-agent spawning** → built into `pipeline_review` and `pipeline_test` tools
- **Provider escalation** → built into `pipeline_review` (auto-escalates) and available via existing escalation chain
- **Stall detection** → unnecessary; the model decides when to stop
- **Loop detection** → the model sees its own tool results and self-corrects; budget tracker remains as a hard safety net

### System Prompt Changes

The system prompt gains a **pipeline tools section** that guides the model on when to use them:

```
## Pipeline Tools (use when appropriate)

You have access to pipeline tools for structured software development work.
Use them when the task warrants it — NOT for every message.

WHEN TO USE PIPELINE TOOLS:
- Multi-file changes with dependencies between files
- Tasks with explicit specs or acceptance criteria
- New feature implementation (not bug fixes or tweaks)
- When the user says "build", "implement", "create" something substantial

WHEN NOT TO USE PIPELINE TOOLS:
- Questions about code ("what does X do?", "explain Y")
- Single-file edits or bug fixes
- Configuration changes
- Research or exploration tasks
- Quick scripts or one-off commands

PIPELINE TOOL WORKFLOW (when warranted):
1. pipeline_plan → create plan + acceptance criteria
2. Implement directly (or pipeline_implement for large scope)
3. pipeline_verify → check build/test/lint
4. pipeline_review → independent code review (optional, for large changes)
5. pipeline_test → end-to-end acceptance test (optional, for user-facing deliverables)

You can use pipeline_verify at any point during implementation — you don't need
to follow the full sequence. Use your judgment.
```

### Migration Strategy: Coexistence Period

Both modes coexist behind `AutoOrchestrate`:

| `AutoOrchestrate` | Behavior | Pipeline |
|---|---|---|
| `true` (current default) | Forced 6-phase pipeline | State machine in `loop()` |
| `false` (new default) | Frontier model with pipeline tools | Model invokes tools as needed |

This allows A/B testing and gradual migration. The CLI flag `--pipeline` can force the old behavior:

```bash
synroute chat                     # new: frontier model with pipeline tools
synroute chat --pipeline          # legacy: forced 6-phase pipeline
synroute chat --pipeline=false    # explicit: frontier model (default)
```

### State Management

Pipeline tools store state on the `*Agent` struct (reusing existing fields):

| Field | Set By | Read By |
|---|---|---|
| `acceptanceCriteria` | `pipeline_plan` | `pipeline_verify`, `pipeline_review`, `pipeline_test` |
| `specConstraints` | `Run()` init (unchanged) | `pipeline_verify`, `pipeline_review` |
| `originalRequest` | `Run()` init (unchanged) | `pipeline_review`, `pipeline_test` |

No new state is needed. The pipeline tools reuse the agent's existing state fields.

### Provider Escalation in Pipeline Tools

Pipeline tools that spawn sub-agents (`pipeline_implement`, `pipeline_review`, `pipeline_test`) use the existing escalation chain:

- `pipeline_review` and `pipeline_test` always escalate (`Escalate: true`) — the reviewer/tester must be a bigger model than the current session
- `pipeline_implement` uses the current provider level — no escalation needed for implementation
- The parent agent's `providerIdx` is never mutated by pipeline tools — escalation is only for sub-agents

### Event Bus Integration

Pipeline tools emit the same events as the current pipeline state machine:

```go
func (t *PipelineReviewTool) Execute(ctx context.Context, args map[string]interface{}, workDir string) (*tools.ToolResult, error) {
    t.agent.emit(EventPhaseStart, "", map[string]any{
        "phase_name": "code-review",
        "triggered_by": "pipeline_review_tool",
    })
    // ... spawn sub-agent ...
    t.agent.emit(EventPhaseComplete, "", map[string]any{
        "phase_name": "code-review",
        "passed": passed,
    })
}
```

This preserves observability — the event bus, tracing, and metrics all continue working.

---

## Architecture Comparison

### Current: Harness-Driven Pipeline

```
User Message
    │
    ▼
┌─────────────────────────────────────────┐
│  Agent.Run()                            │
│  ├── Initialize pipeline (always)       │
│  ├── Detect intent entry point          │
│  ├── Inject phase prompt                │
│  └── Enter loop()                       │
│       ├── LLM call                      │
│       ├── Tool execution                │
│       ├── Phase signal detection        │
│       ├── Quality gates                 │
│       ├── Stall detection               │
│       ├── Loop detection                │
│       ├── Sub-agent spawning            │
│       ├── advancePipeline()             │
│       │    ├── Signal parsing           │
│       │    ├── Verify gate              │
│       │    ├── Phase advance            │
│       │    ├── Conversation compact     │
│       │    ├── Provider escalation      │
│       │    ├── Parallel sub-agents      │
│       │    ├── Review sub-agents        │
│       │    ├── Divergence detection     │
│       │    ├── Stability detection      │
│       │    └── Fail-back cycles         │
│       └── Force continuation            │
└─────────────────────────────────────────┘
```

### New: Model-Driven with Pipeline Tools

```
User Message
    │
    ▼
┌─────────────────────────────────────────┐
│  Agent.Run()                            │
│  ├── Register pipeline tools            │
│  └── Enter loop()                       │
│       ├── LLM call                      │
│       ├── If no tool calls → return     │
│       └── Execute tool calls            │
│            ├── bash, file_read, etc.    │
│            ├── pipeline_plan            │
│            │    └── Parallel planners   │
│            ├── pipeline_implement       │
│            │    └── Sub-agent           │
│            ├── pipeline_verify          │
│            │    └── Build/test/lint     │
│            ├── pipeline_review          │
│            │    └── Escalated sub-agent │
│            ├── pipeline_test            │
│            │    └── Escalated sub-agent │
│            └── pipeline_status          │
└─────────────────────────────────────────┘
```

The new architecture moves complexity from the harness (~600 lines of state machine) into tool implementations (~300 lines of focused tool code) that the model invokes when needed.

---

## Implementation Plan

### Phase 1: Pipeline Tools (non-breaking)

**Files to create:**
- `internal/agent/pipeline_tools.go` — all 6 pipeline tool implementations
- `internal/agent/pipeline_tools_test.go` — unit tests

**Files to modify:**
- `internal/agent/agent.go` — register pipeline tools in `Run()` when `AutoOrchestrate=false`
- `internal/agent/config.go` — change `AutoOrchestrate` default to `false`
- `commands.go` — add `--pipeline` flag to `synroute chat`; set `AutoOrchestrate=true` only when `--pipeline` is passed

**Existing code reused (not modified):**
- `internal/agent/pipeline.go` — phase definitions (used by pipeline tools for prompts)
- `internal/agent/verify.go` — `RunVerificationGate()` (called by `pipeline_verify`)
- `internal/agent/subagent.go` — `SpawnChild`, `RunChild` (called by pipeline tools)
- `internal/agent/spec_constraints.go` — constraint extraction (unchanged)
- `internal/tools/registry.go` — `Register()`, `OpenAIToolDefinitions()` (unchanged)
- `internal/tools/tool.go` — `Tool` interface (unchanged)

### Phase 2: System Prompt Update

**Files to modify:**
- `internal/agent/agent.go` — update `defaultSystemPrompt()` to include pipeline tool guidance when tools are registered
- `internal/agent/agent.go` — update `buildMessages()` to skip phase prompt injection when `AutoOrchestrate=false`

### Phase 3: Simplify the Loop

**Files to modify:**
- `internal/agent/agent.go` — `loop()` function: wrap all `AutoOrchestrate` blocks in conditionals (they already are, but some leak). Clean path for `AutoOrchestrate=false` is just: LLM call → tool execution → repeat.

### Phase 4: Deprecation Path

**Files to modify:**
- `internal/agent/agent.go` — add deprecation log warning when `AutoOrchestrate=true`
- `commands.go` — `--pipeline` flag emits deprecation notice

### Phase 5: Cleanup (future, after validation)

**Files to remove/reduce:**
- `internal/agent/intent.go` — `DetectPipelineEntry()` only needed for forced pipeline
- `internal/agent/review_stability.go` — divergence/stability detection moves into `pipeline_review` tool
- `internal/agent/agent.go` — remove `advancePipeline()`, stall detection, loop detection, force-advance (~400 lines)

---

## Acceptance Criteria

### Core Behavior

1. **AC-1: Direct answers work.** `synroute chat --message "what does the Router struct do?"` returns a direct answer without invoking any pipeline tools. No plan phase, no verification gate, no sub-agent review.

2. **AC-2: Pipeline tools are available.** The model's tool list includes `pipeline_plan`, `pipeline_implement`, `pipeline_verify`, `pipeline_review`, `pipeline_test`, and `pipeline_status` alongside existing tools (bash, file_read, etc.).

3. **AC-3: Model invokes pipeline_plan for complex tasks.** Given a message like "implement a REST API with CRUD operations for a todo app", the model calls `pipeline_plan` before starting implementation. Verified by checking tool call history in agent trace.

4. **AC-4: Model skips pipeline tools for simple tasks.** Given a message like "add a comment to line 5 of main.go", the model uses `file_read` + `file_edit` directly without calling any pipeline tools. Verified by checking tool call history.

5. **AC-5: pipeline_verify runs build and test commands.** Calling `pipeline_verify` executes the environment-detected build and test commands (e.g., `go build`, `go test`) and returns structured pass/fail results with output.

6. **AC-6: pipeline_review spawns an escalated sub-agent.** Calling `pipeline_review` creates a fresh sub-agent with no shared conversation context that uses a higher escalation level than the current session. Verified by checking sub-agent's `providerIdx > parent.providerIdx`.

7. **AC-7: pipeline_test spawns an escalated sub-agent.** Same as AC-6 but for acceptance testing.

### Backward Compatibility

8. **AC-8: Legacy pipeline still works.** `synroute chat --pipeline` enables the old `AutoOrchestrate=true` behavior with the 6-phase forced pipeline. All existing pipeline tests pass.

9. **AC-9: Default behavior changes.** `synroute chat` (no flags) uses the new frontier model loop with pipeline tools available but not forced. `AutoOrchestrate` defaults to `false`.

### State Management

10. **AC-10: Acceptance criteria persist across tool calls.** After `pipeline_plan` stores criteria, `pipeline_verify`, `pipeline_review`, and `pipeline_test` can access them. Verified by calling `pipeline_status` after `pipeline_plan`.

11. **AC-11: Pipeline tools emit events.** All pipeline tools emit `EventPhaseStart` and `EventPhaseComplete` events on the event bus, preserving observability.

### Quality

12. **AC-12: Token savings on simple tasks.** A simple question (e.g., "list all Go files") completes in fewer than 5 tool calls and under 3 LLM turns. Currently, the forced pipeline adds 10+ injected messages and multiple phase transitions.

13. **AC-13: Complex tasks still get structured treatment.** A spec-driven implementation task (e.g., "implement this spec: [500-line spec]") results in the model calling `pipeline_plan`, then implementing, then calling `pipeline_verify` at least once. The model self-selects the structured workflow.

14. **AC-14: No regression in eval scores.** Running `synroute eval run --count 20` produces pass rates within 5% of the forced-pipeline baseline on coding tasks (exercism, ds1000).

---

## Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Model doesn't invoke pipeline tools when it should | Complex tasks get no planning/review | System prompt guidance + eval benchmarking. If model consistently under-uses tools, add stronger prompting. Worst case: `--pipeline` flag restores forced mode. |
| Model over-invokes pipeline tools | Simple tasks waste tokens on unnecessary planning | System prompt explicitly lists "when NOT to use". Tool descriptions include negative examples. Monitor via event bus. |
| Sub-agent costs increase | Review/test sub-agents called more often than needed | Budget tracker still enforces token/turn limits. Pipeline tools respect existing `AgentBudget`. |
| Small models (Level 0) can't use pipeline tools effectively | 20-30B models may not reason about when to invoke pipeline tools | Conditional registration: only register pipeline tools for Level 1+ models. Level 0 gets the simplified system prompt (existing behavior). |
| State loss between tool calls | Acceptance criteria not available to later tools | Criteria stored on `*Agent` struct (in-memory), same as current design. Persisted to `ToolOutputStore` for cross-session recall. |

---

## What We Gain

1. **Simpler agent loop.** The `loop()` function drops from ~200 lines to ~40 lines. No state machine, no signal parsing, no stall/loop detection.

2. **Conversational flexibility.** Users can ask questions, discuss approaches, and then say "ok build it" — the model invokes pipeline tools when the conversation shifts to implementation.

3. **Token efficiency.** Simple tasks skip pipeline overhead entirely. No injected phase prompts, no verification gates, no sub-agent reviews for questions.

4. **Model-appropriate behavior.** The frontier model's reasoning ability determines when structured work is needed, rather than regex-based intent detection and hardcoded phase sequencing.

5. **Preserved quality for complex tasks.** Pipeline tools retain all the existing quality mechanisms: verification gates, escalated reviewers, spec compliance checks, sub-agent isolation. They're just invoked by the model instead of the harness.

6. **Alignment with industry practice.** Every major coding agent (Claude Code, Cursor, Codex, Aider) uses a model-driven loop where the model decides what to do. None forces a multi-phase pipeline.

---

## Research Sources

- [How Claude Code Works](https://code.claude.com/docs/en/how-claude-code-works) — Claude Code's agentic loop architecture
- [How Cursor Shipped its Coding Agent to Production](https://blog.bytebytego.com/p/how-cursor-shipped-its-coding-agent) — Cursor's per-model agent harness and ReAct loop
- [Cursor 2.0: Agent-First Architecture](https://www.digitalapplied.com/blog/cursor-2-0-agent-first-architecture-guide) — Composer model, mode separation, tool-call limits
- [Best Practices for Coding with Agents (Cursor)](https://cursor.com/blog/agent-best-practices) — When to use Ask vs Agent mode
- [Unrolling the Codex Agent Loop](https://openai.com/index/unrolling-the-codex-agent-loop/) — OpenAI's agent loop design
- [Codex CLI Documentation](https://developers.openai.com/codex/cli) — Approval levels, sandbox architecture
- [Codex App Server Architecture (InfoQ)](https://www.infoq.com/news/2026/02/opanai-codex-app-server/) — Items, Turns, Threads protocol
- [Aider Chat Modes](https://aider.chat/docs/usage/modes.html) — Ask/Code/Architect mode separation
- [Amazon Kiro: Spec-Driven Agentic IDE (InfoQ)](https://www.infoq.com/news/2025/08/aws-kiro-spec-driven-agent/) — Spec-driven phases, user-triggered execution
- [Kiro vs Cursor Comparison](https://www.augmentcode.com/tools/kiro-vs-cursor) — Pipeline approaches compared
- [Workflows vs Agents: A Developer's Guide (Towards Data Science)](https://towardsdatascience.com/a-developers-guide-to-building-scalable-ai-workflows-vs-agents/) — When to use workflows vs autonomous agents
- [LLM Agents in Production (ZenML)](https://www.zenml.io/blog/llm-agents-in-production-architectures-challenges-and-best-practices) — Production best practices
- [The Ultimate LLM Agent Build Guide (Vellum)](https://vellum.ai/blog/the-ultimate-llm-agent-build-guide) — Tool calling patterns
- [AI Agents and Tool Calling: A Complete Technical Guide (Spartner)](https://spartner.software/kennisbank/ai-agents-tool-calling) — Tool calling architecture patterns
