# Technical Design: Stateful REPL Conversation (#235)

**Issue**: [#235 — REPL conversation should be stateful — each message triggers different phases](https://github.com/gr3enarr0w/synapserouter/issues/235)
**Depends on**: [#232 — Pipeline as a tool the frontier model calls, not a forced mode](https://github.com/gr3enarr0w/synapserouter/issues/232)
**Author**: Technical design for comment on #235
**Date**: 2026-03-28

---

## Problem Statement

Currently, every message sent through `synroute chat` triggers a full pipeline run. The REPL calls `a.agent.Run(reqCtx, input)` on each message, and `Run()` reinitializes the pipeline from scratch every time (`a.pipeline == nil` check at line 332 of `agent.go`). The `originalRequest` is captured once (first message only), but pipeline state (`pipelinePhase`, `phaseToolCalls`, `acceptanceCriteria`, etc.) is all scoped to a single `Run()` invocation.

This means:
- "Fix the auth bug" triggers plan -> implement -> self-check -> code-review -> acceptance-test -> deploy
- A follow-up "now review that" triggers the same 6-phase pipeline again from scratch
- A simple question like "what does this function do?" gets forced through the pipeline
- There is no way to say "looks good, now test it" and have only the test phase run

The REPL has conversation continuity (messages persist in `a.conversation`), but no pipeline state continuity between `Run()` calls.

---

## Research Findings

### How Claude Code Handles This

Claude Code uses a **single-agent loop** architecture where the model controls what happens on each turn. There is no forced pipeline — the model receives user messages and decides whether to use tools, answer directly, or orchestrate multi-step work. Key design principles:

- **"Model controls the Loop" (TAOR)**: The runtime is minimal; the model decides what actions to take. There is no rigid phase sequence imposed by the harness.
- **Primitive tools over workflow plugins**: Instead of pipeline phases, the model composes workflows from primitive tools (bash, edit, grep, etc.).
- **Context compaction**: When conversations get long, older turns are summarized to free context window space. Sessions are persistent and resumable.
- **Sub-agents for isolation**: Heavy tasks can be delegated to sub-agents that don't pollute the main conversation context.

The critical insight: **Claude Code has no pipeline at all.** Every message is handled by the same agent loop. The model's intelligence determines whether a message needs code changes, testing, or a direct answer. This is exactly what #232 proposes — making the pipeline optional and model-invoked.

### How Cursor Handles This

Cursor 2.0 uses an **agent-first architecture** where intent detection is delegated to the LLM:

- **No explicit intent classifier**: The LLM itself determines whether a message requires code editing, test execution, or a conversational answer. Cursor's result handler routes the output to the appropriate UI.
- **Agent loop for complex tasks**: Planning -> executing -> applying changes -> verifying -> adjusting. The loop stops when the goal is met or clarification is needed.
- **Planning/execution separation**: Different models can handle planning vs. implementation.
- **Context management**: Earlier messages are summarized by smaller models to maintain speed. When the context limit approaches, a new conversation is suggested with reference to the current one.

### Letta (MemGPT) — Stateful Agent Architecture

Letta provides the most relevant model for inter-turn state management:

- **Memory blocks**: Structured text segments pinned to the system prompt (always visible). Agents can read/write their own memory blocks. This is analogous to persisting pipeline state between turns.
- **No sessions — perpetual history**: All interactions are part of one persistent memory stream, not ephemeral sessions.
- **Self-editing memory**: The agent uses tools (`memory_insert`, `memory_replace`) to update its own state. This maps directly to a pipeline tool that updates phase state.
- **Tiered storage**: Core memory (in-context) + archival memory (searchable) + recall memory (full history). This mirrors synroute's existing Conversation + VectorMemory + ToolOutputStore.

### Key Takeaway from Research

**The industry consensus is: let the model decide.** No coding agent forces a fixed pipeline on every message. The agent runtime maintains state, and the model invokes structured workflows (like pipeline phases) as tools when appropriate. Simple questions get direct answers. Complex tasks trigger multi-phase work. Follow-up messages reference accumulated state.

---

## Technical Design

### Architecture: Per-Message Intent with Persistent Pipeline State

The design has three layers:

1. **Conversation Layer** (exists today) — messages persist between REPL turns
2. **Session State Layer** (new) — pipeline position, accumulated artifacts, and working context persist between turns
3. **Intent Layer** (new) — each message is classified before deciding what action to take

```
User message
    |
    v
[Intent Classification] -----> question  --> direct LLM answer (no pipeline)
    |                   -----> followup  --> resume pipeline at current/specified phase
    |                   -----> new_task  --> reset pipeline, start fresh
    |                   -----> phase_cmd --> run specific phase only
    v
[Session State] <-- reads/writes pipeline position, criteria, modified files, test results
    |
    v
[Agent.Run()] -- now a PARTIAL run, not always full pipeline
```

### 1. Session State (`REPLSessionState`)

A new struct that persists between REPL turns, carried by the REPL and injected into each `Agent.Run()` call:

```go
// REPLSessionState tracks pipeline and project state across REPL messages.
// This is NOT persisted to SQLite between process restarts (SessionState handles that).
// This is in-memory state that lives for the duration of a REPL session.
type REPLSessionState struct {
    // Pipeline position
    Pipeline           *Pipeline  // current pipeline (nil = no active pipeline)
    PipelinePhase      int        // current phase index
    AcceptanceCriteria string     // generated during plan phase
    OriginalTask       string     // the message that started the current pipeline

    // Accumulated artifacts
    ModifiedFiles      []string   // files changed during this pipeline run
    TestResults        []TestResult // last test run output
    ReviewFindings     []string   // findings from most recent review phase
    CompletedPhases    []string   // names of phases already run

    // Working context
    ProjectLanguage    string     // detected or declared language
    SpecConstraints    *SpecConstraints // extracted constraints (persists across turns)
    ProviderLevel      int        // current escalation level (persists — never regresses)

    // History
    TaskHistory        []TaskRecord // completed tasks in this session
}

type TaskRecord struct {
    Message    string
    Intent     MessageIntent
    Phases     []string  // which phases ran
    StartedAt  time.Time
    Duration   time.Duration
}
```

### 2. Per-Message Intent Detection (`MessageIntent`)

Intent classification determines what happens with each REPL message. This is done **before** calling `Agent.Run()` and controls how the agent loop behaves.

```go
type MessageIntent int

const (
    IntentQuestion    MessageIntent = iota // "what does this do?" -> direct answer
    IntentNewTask                          // "fix the auth bug" -> new pipeline from plan
    IntentContinue                         // "looks good, implement it" -> advance to next phase
    IntentPhaseCmd                         // "review that" / "test it" -> run specific phase
    IntentFollowUp                         // "also add logging" -> resume current phase with amendment
    IntentMeta                             // "what phase are we in?" -> REPL info, no LLM call
)

type IntentResult struct {
    Intent      MessageIntent
    TargetPhase string // for IntentPhaseCmd: which phase to run
    Reason      string // human-readable explanation
    Confidence  float64
}
```

**Classification approach**: Use a lightweight two-stage classifier:

**Stage 1 — Rule-based fast path** (no LLM call, ~0ms):
```go
func ClassifyIntentFast(message string, state *REPLSessionState) *IntentResult {
    lower := strings.ToLower(message)

    // Meta commands
    if strings.HasPrefix(lower, "what phase") || strings.HasPrefix(lower, "where are we") {
        return &IntentResult{Intent: IntentMeta, Reason: "meta query"}
    }

    // Phase commands — explicit phase references
    phaseKeywords := map[string]string{
        "review":    "code-review",
        "test":      "acceptance-test",
        "deploy":    "deploy",
        "plan":      "plan",
        "implement": "implement",
        "check":     "self-check",
    }
    for keyword, phase := range phaseKeywords {
        // Match patterns like "review that", "now test it", "run the tests"
        if matchesPhaseCommand(lower, keyword) {
            return &IntentResult{Intent: IntentPhaseCmd, TargetPhase: phase}
        }
    }

    // Continue signals — short affirmatives after a pipeline pause
    if state.Pipeline != nil && isAffirmative(lower) {
        return &IntentResult{Intent: IntentContinue, Reason: "affirmative with active pipeline"}
    }

    // Questions — interrogative patterns with no action verbs
    if isQuestion(lower) && !hasActionVerb(lower) {
        return &IntentResult{Intent: IntentQuestion, Reason: "interrogative without action"}
    }

    return nil // fall through to stage 2
}
```

**Stage 2 — LLM-assisted classification** (only when stage 1 returns nil):

Instead of a separate classification LLM call (expensive), inject a classification preamble into the system prompt that asks the model to prefix its response with a structured intent tag. The agent loop parses this tag before proceeding:

```
When responding to a message in an ongoing REPL session, first output a single
line indicating your intent classification:

INTENT: question | new_task | continue | phase:<phase-name> | followup

Then proceed with your response. The harness will parse this line and route
your response accordingly.
```

This approach uses the same LLM call that would happen anyway, adding minimal overhead. The harness strips the INTENT line before displaying the response.

### 3. Modified REPL Flow

The REPL loop changes from:

```go
// Current: every message → full Run()
response, err := r.agent.Run(reqCtx, input)
```

To:

```go
// New: classify → configure → partial Run()
intent := ClassifyIntent(input, r.sessionState)

switch intent.Intent {
case IntentMeta:
    r.handleMetaQuery(input)
    continue

case IntentQuestion:
    // Disable pipeline for this message
    r.agent.SetPipelineMode(PipelineModeOff)
    response, err := r.agent.Run(reqCtx, input)

case IntentNewTask:
    // Reset pipeline state, start fresh
    r.sessionState.StartNewTask(input)
    r.agent.SetPipelineMode(PipelineModeFull)
    r.agent.ResetPipeline()
    response, err := r.agent.Run(reqCtx, input)

case IntentPhaseCmd:
    // Run only the specified phase
    r.agent.SetPipelineMode(PipelineModeSingle)
    r.agent.SetTargetPhase(intent.TargetPhase)
    r.agent.InjectSessionState(r.sessionState)
    response, err := r.agent.Run(reqCtx, input)

case IntentContinue:
    // Resume pipeline at current position
    r.agent.SetPipelineMode(PipelineModeContinue)
    r.agent.InjectSessionState(r.sessionState)
    response, err := r.agent.Run(reqCtx, input)

case IntentFollowUp:
    // Resume current phase with additional instructions
    r.agent.SetPipelineMode(PipelineModeContinue)
    r.agent.InjectSessionState(r.sessionState)
    response, err := r.agent.Run(reqCtx, input)
}

// After Run(), capture updated state back
r.sessionState.CaptureFrom(r.agent)
```

### 4. Agent.Run() Changes

The key change to `Agent.Run()` is making pipeline initialization **conditional** rather than unconditional:

```go
type PipelineMode int

const (
    PipelineModeOff      PipelineMode = iota // no pipeline — direct conversation
    PipelineModeFull                          // full pipeline from plan phase
    PipelineModeSingle                        // single specified phase
    PipelineModeContinue                      // resume from saved position
)
```

In `Run()`, the current block:

```go
if a.config.AutoOrchestrate && a.pipeline == nil {
    // ... initialize pipeline from scratch every time
}
```

Becomes:

```go
switch a.pipelineMode {
case PipelineModeOff:
    // No pipeline — just run the agent loop for direct conversation
case PipelineModeFull:
    // Current behavior — initialize from scratch
case PipelineModeSingle:
    // Wrap the target phase in SinglePhase() pipeline
    // Inject accumulated criteria/context from session state
case PipelineModeContinue:
    // Restore pipeline + phase position from session state
    // Don't re-run completed phases
}
```

### 5. Relationship to #232 (Pipeline as a Tool)

If #232 is implemented first (recommended), the design simplifies dramatically:

- Pipeline phases become tools: `plan()`, `implement()`, `self_check()`, `code_review()`, `accept_test()`
- The REPL is **always** in direct conversation mode (no `PipelineMode` needed)
- The model decides per-message whether to invoke pipeline tools
- "Review that" causes the model to call `code_review()` with context from previous turns
- "What does this do?" gets a direct answer — no tools invoked
- Session state is still needed (to carry criteria, modified files, etc. between tool invocations)

With #232:
```
User: "fix the auth bug"
Model: [calls plan()] → [calls implement()] → [calls self_check()] → "Done. Auth bug fixed."

User: "now review that"
Model: [calls code_review(criteria=<from plan>)] → "Review complete: 2 minor issues found..."

User: "what's the package structure?"
Model: "The package structure is..." (no tools needed)

User: "fix those 2 issues"
Model: [calls implement()] → [calls self_check()] → "Fixed both issues."
```

This is the recommended path. The `REPLSessionState` struct is still needed to carry context between tool invocations, but the intent classification becomes the model's job, not the harness's.

### 6. State Persistence for `--resume`

The existing `SessionState` (in `state.go`) persists conversation messages to SQLite. For stateful REPL, extend it to also persist pipeline state:

```go
type SessionState struct {
    // ... existing fields ...

    // Pipeline state (new)
    PipelineName       string `json:"pipeline_name,omitempty"`
    PipelinePhase      int    `json:"pipeline_phase"`
    AcceptanceCriteria string `json:"acceptance_criteria,omitempty"`
    CompletedPhases    []string `json:"completed_phases,omitempty"`
    ModifiedFiles      []string `json:"modified_files,omitempty"`
    OriginalTask       string `json:"original_task,omitempty"`
}
```

Migration:
```sql
-- migrations/007_session_pipeline_state.sql
ALTER TABLE agent_sessions ADD COLUMN pipeline_name TEXT DEFAULT '';
ALTER TABLE agent_sessions ADD COLUMN pipeline_phase INTEGER DEFAULT 0;
ALTER TABLE agent_sessions ADD COLUMN acceptance_criteria TEXT DEFAULT '';
ALTER TABLE agent_sessions ADD COLUMN completed_phases TEXT DEFAULT '[]';
ALTER TABLE agent_sessions ADD COLUMN modified_files TEXT DEFAULT '[]';
ALTER TABLE agent_sessions ADD COLUMN original_task TEXT DEFAULT '';
```

When `--resume` restores a session, it also restores pipeline position so the user can continue exactly where they left off.

### 7. New REPL Commands

```
/phase          Show current pipeline phase and state
/phases         Show all phases with completion status
/reset          Reset pipeline state (start fresh on next message)
/criteria       Show current acceptance criteria
/files          Show files modified in this session
/task           Show the original task that started the current pipeline
```

### 8. Context Injection Between Turns

When the REPL sends a follow-up message that triggers a specific phase, the session state context must be injected so the LLM knows what happened before:

```go
func (s *REPLSessionState) ContextSummary() string {
    if s.Pipeline == nil {
        return ""
    }
    var parts []string
    parts = append(parts, fmt.Sprintf("Active pipeline: %s (phase %d/%d: %s)",
        s.Pipeline.Name, s.PipelinePhase+1, len(s.Pipeline.Phases),
        s.Pipeline.Phases[s.PipelinePhase].Name))
    if s.AcceptanceCriteria != "" {
        parts = append(parts, "Acceptance criteria from plan phase:\n"+s.AcceptanceCriteria)
    }
    if len(s.CompletedPhases) > 0 {
        parts = append(parts, "Completed phases: "+strings.Join(s.CompletedPhases, ", "))
    }
    if len(s.ModifiedFiles) > 0 {
        parts = append(parts, fmt.Sprintf("Modified files (%d): %s",
            len(s.ModifiedFiles), strings.Join(s.ModifiedFiles, ", ")))
    }
    return strings.Join(parts, "\n")
}
```

This summary is prepended to the user message or injected as a system prompt addition, giving the LLM full context about where the pipeline stands.

---

## State Management: What Persists Between Messages

| State | Persists between turns? | Persists on `--resume`? | Notes |
|---|---|---|---|
| Conversation messages | Yes (existing) | Yes (existing) | `Conversation.messages` |
| Pipeline type | Yes (new) | Yes (new) | software vs data-science |
| Pipeline phase position | Yes (new) | Yes (new) | Which phase we're in |
| Acceptance criteria | Yes (new) | Yes (new) | Generated during plan |
| Modified files list | Yes (new) | Yes (new) | Tracked from file_write/edit tools |
| Test results (last run) | Yes (new) | No | Too large; regenerate on resume |
| Review findings | Yes (new) | No | Regenerate if needed |
| Spec constraints | Yes (existing) | No | Re-extracted from original request |
| Provider escalation level | Yes (existing) | No | Monotonically increasing per session |
| Tool call fingerprints | No | No | Reset per Run() — loop detection is per-task |
| Original task message | Yes (new) | Yes (new) | What started the current pipeline |

---

## Acceptance Criteria

### AC-1: Questions get direct answers
**Scenario**: User sends "what does the Router interface look like?" in the REPL.
**Expected**: The agent reads the file and responds directly. No pipeline phases are triggered. No "PHASE: PLAN" or "PHASE: IMPLEMENT" prompts appear in the conversation.
**Verify**: Check that `pipelineMode == PipelineModeOff` for this message. Response contains no phase signal keywords.

### AC-2: First coding task triggers full pipeline
**Scenario**: User sends "add rate limiting to the /v1/chat endpoint" as the first message.
**Expected**: Full pipeline triggers: plan -> implement -> self-check -> code-review -> acceptance-test -> deploy.
**Verify**: All 6 phase names appear in logs. `AcceptanceCriteria` is populated after plan phase.

### AC-3: "Review that" triggers only code-review
**Scenario**: After AC-2 completes, user sends "review that again."
**Expected**: Only the code-review phase runs. It uses the `AcceptanceCriteria` from the original plan. No plan or implement phase runs.
**Verify**: Only `code-review` phase appears in logs. `AcceptanceCriteria` matches what was generated in AC-2.

### AC-4: "Now test it" triggers only acceptance-test
**Scenario**: After AC-2 completes, user sends "now test it."
**Expected**: Only the acceptance-test phase runs, using persisted criteria.
**Verify**: Only `acceptance-test` phase in logs. Modified files from AC-2 are still tracked.

### AC-5: "Fix the auth part" triggers targeted implement
**Scenario**: After a pipeline run completes, user sends "fix the auth validation — it should reject expired tokens."
**Expected**: A new implement cycle starts (not full pipeline). The agent has context about what was previously built (via conversation history and session state).
**Verify**: Pipeline starts at implement phase, not plan. Previous acceptance criteria are available. Modified files from previous run are known.

### AC-6: Affirmative continues pipeline
**Scenario**: Pipeline pauses after plan phase (future: pipeline-as-tool pauses for approval). User sends "looks good, go ahead."
**Expected**: Pipeline advances to implement phase without re-planning.
**Verify**: `IntentContinue` classified. `pipelinePhase` advances from 0 to 1.

### AC-7: Pipeline state persists through conversation clearing
**Scenario**: User runs `/clear` during an active pipeline, then sends "continue implementing."
**Expected**: Conversation messages are cleared, but pipeline state (phase, criteria) persists. The agent can resume with fresh context.
**Verify**: `REPLSessionState.Pipeline` is not nil after `/clear`. `AcceptanceCriteria` still populated.

### AC-8: --resume restores pipeline position
**Scenario**: User exits REPL mid-pipeline (after plan + implement, before review). Restarts with `synroute chat --resume`.
**Expected**: Pipeline state is restored. User can say "now review it" and get the code-review phase with original criteria.
**Verify**: `SessionState` loaded from SQLite includes `pipeline_phase`, `acceptance_criteria`, `completed_phases`.

### AC-9: New task resets pipeline
**Scenario**: After completing a pipeline run, user sends "now build a CLI for the new rate limiter."
**Expected**: Pipeline resets to phase 0 (plan). Previous task's criteria are archived in `TaskHistory`. Fresh plan phase begins.
**Verify**: `pipelinePhase` resets to 0. `OriginalTask` updates. Previous task appears in `TaskHistory`.

### AC-10: /phase command shows current state
**Scenario**: User types `/phase` during an active pipeline.
**Expected**: Output shows current phase name, index, completed phases, and whether criteria exist.
**Verify**: Output includes phase name, "2/6", list of completed phases, and criteria summary.

### AC-11: Mixed conversation flow
**Scenario**: Multi-turn sequence:
1. "Fix the login bug" (new task -> pipeline)
2. "What's in the auth middleware?" (question -> direct answer)
3. "OK, looks like the token check is wrong. Fix that specific line." (followup -> implement continues)
4. "Review the fix" (phase command -> code-review only)
5. "Ship it" (continue -> deploy phase)
**Expected**: Each message triggers the appropriate behavior. Pipeline state is maintained throughout. Questions don't reset or affect pipeline state.
**Verify**: Intent classification matches expected for each message. Pipeline phase advances correctly through the sequence.

### AC-12: Intent classification has no false positives on coding messages
**Scenario**: User sends "Can you implement the caching layer?" (interrogative form, but coding task).
**Expected**: Classified as `IntentNewTask`, not `IntentQuestion`. The "can you" is a request, not a question.
**Verify**: Rule-based classifier correctly identifies action verbs ("implement") even in interrogative syntax.

### AC-13: No regression in one-shot mode
**Scenario**: `synroute chat --message "fix the tests"` (non-interactive, single message).
**Expected**: Full pipeline runs as it does today. No intent classification overhead. `RunOneShot()` behavior is unchanged.
**Verify**: `RunOneShot()` does not use `REPLSessionState` or intent classification. Output matches current behavior.

---

## Implementation Plan

### Phase 1: Session State Infrastructure (prerequisite)

**Files to modify:**
- `internal/agent/session_state.go` (new) — `REPLSessionState` struct, `ContextSummary()`, `CaptureFrom()`, `StartNewTask()`
- `internal/agent/state.go` — extend `SessionState` with pipeline fields for `--resume`
- `migrations/007_session_pipeline_state.sql` (new) — schema migration

**Tests:**
- `internal/agent/session_state_test.go` (new) — state capture/restore, context summary formatting

### Phase 2: Intent Classification

**Files to modify:**
- `internal/agent/intent.go` — add `MessageIntent` type, `ClassifyIntentFast()`, helper functions (`isQuestion`, `isAffirmative`, `matchesPhaseCommand`, `hasActionVerb`)
- `internal/agent/intent_test.go` — table-driven tests covering all intent types, edge cases (interrogative coding requests, affirmatives without pipeline)

**Tests:** 30+ test cases covering the classification matrix:
| Message | Expected Intent |
|---|---|
| "what does this function do?" | Question |
| "fix the auth bug" | NewTask |
| "looks good, go ahead" | Continue |
| "now review that" | PhaseCmd(code-review) |
| "also add logging to that" | FollowUp |
| "can you implement caching?" | NewTask (not Question) |
| "test it" | PhaseCmd(acceptance-test) |
| "what phase are we in?" | Meta |
| "yes" | Continue (if pipeline active) |
| "yes" | NewTask (if no pipeline) |

### Phase 3: Pipeline Mode in Agent

**Files to modify:**
- `internal/agent/agent.go` — add `PipelineMode` field, modify `Run()` to respect mode, add `SetPipelineMode()`, `SetTargetPhase()`, `InjectSessionState()`, `ResetPipeline()` methods
- `internal/agent/config.go` — no changes (pipeline mode is per-Run, not per-Config)

**Key change in `Run()`:**
The `if a.config.AutoOrchestrate && a.pipeline == nil` block becomes a switch on `pipelineMode`. The `originalRequest` field is no longer set unconditionally — it only updates on `IntentNewTask`.

### Phase 4: REPL Integration

**Files to modify:**
- `internal/agent/repl.go` — add `sessionState *REPLSessionState` field, modify `Run()` loop to classify intent before each `agent.Run()`, add new commands (`/phase`, `/phases`, `/reset`, `/criteria`, `/files`, `/task`)
- `internal/agent/coderepl.go` — same changes for the code mode REPL (both REPLs share the same agent, so they share session state)

### Phase 5: Modified File Tracking

**Files to modify:**
- `internal/tools/file_write.go` — emit event or callback when a file is written
- `internal/tools/file_edit.go` — emit event or callback when a file is edited
- `internal/agent/session_state.go` — `TrackModifiedFile(path)` method with dedup

This enables the session state to know which files were changed, which is critical for targeted review phases.

### Phase 6: State Persistence for --resume

**Files to modify:**
- `internal/agent/state.go` — `SaveState()` includes pipeline fields, `LoadState()` / `RestoreAgent()` restores them
- `internal/agent/repl.go` — on startup with `--resume`, populate `REPLSessionState` from loaded `SessionState`

### Phase 7 (if #232 lands first): Pipeline-as-Tool Simplification

If #232 is implemented, phases 2-4 simplify:
- Remove `PipelineMode` enum — always direct conversation
- Remove `ClassifyIntentFast()` — model does its own intent classification
- Keep `REPLSessionState` — tools still need shared state
- Pipeline tools read/write `REPLSessionState` for criteria, modified files, etc.
- The REPL loop reverts to the current simple form (just calls `agent.Run()`)

---

## Migration Path

**Without #232** (standalone implementation):
1. Implement phases 1-6 above
2. Intent classification lives in the harness (REPL code)
3. More code, but works immediately with current architecture

**With #232** (preferred):
1. Implement #232 first — pipeline phases become tools
2. Implement phase 1 (session state) and phase 5 (file tracking)
3. Phase 2-4 are unnecessary — the model handles intent naturally
4. Phase 6 still needed for `--resume`

**Recommendation**: Implement #232 first. It eliminates the need for an explicit intent classifier in the harness, which is inherently fragile (the research shows that multi-turn intent classification is the hardest part — Zendesk found >90% accuracy for single-turn but significantly worse for multi-turn). Letting the frontier model handle intent is both simpler and more accurate.

---

## Risks and Mitigations

| Risk | Mitigation |
|---|---|
| Intent classifier misclassifies coding requests as questions | Stage 1 rule-based classifier checks for action verbs before classifying as question. If #232 lands, this risk disappears. |
| Pipeline state grows stale between turns (user changes files externally) | Modified file list is advisory, not authoritative. Review/test phases always re-read actual file state via tools. |
| Long REPL sessions exceed context window | Existing compaction mechanism handles this. Session state is separate from conversation messages, so compaction doesn't lose pipeline position. |
| `--resume` restores stale pipeline state | Include a timestamp in persisted state. On resume, warn if state is >24h old. Let user `/reset` if needed. |
| Rule-based intent classifier doesn't generalize to non-English | Acceptable limitation. The REPL is a developer tool used in English. Phase commands can also be issued explicitly via `/phase review`. |

---

## Open Questions

1. **Should intent classification use an LLM call?** The design above avoids it (rule-based fast path + inline INTENT tag). An alternative is a cheap classification call to a small model before the main LLM call. This adds latency but improves accuracy. If #232 lands, this question is moot.

2. **Should `/clear` preserve pipeline state?** The design says yes (clear conversation, keep pipeline). An alternative is `/clear` resets everything and `/clear --keep-pipeline` preserves it. User feedback will determine the right default.

3. **How should the REPL indicate which mode it's in?** Options: colored prompt prefix (`[plan]>`, `[review]>`), status bar in code mode REPL, or only on explicit `/phase` command. The code mode REPL already has `Ctrl-P` for pipeline status.

4. **Should sub-agent review phases access session state?** Currently sub-agents are intentionally context-free (fresh perspective). Session state should be available for acceptance criteria and spec constraints, but NOT for conversation history or implementation details.
