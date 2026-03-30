# Pipeline Speed Optimization: Technical Design

**Epic:** #289 -- 110min to 30min without losing accuracy
**Issues:** #290-#299
**Date:** 2026-03-28

---

## Table of Contents

1. [Current State Analysis](#current-state-analysis)
2. [Priority Matrix](#priority-matrix)
3. [Dependency Graph](#dependency-graph)
4. [Quick Wins (Phase 1)](#quick-wins-phase-1)
5. [Context Engineering (Phase 2)](#context-engineering-phase-2)
6. [Advanced Optimizations (Phase 3)](#advanced-optimizations-phase-3)
7. [Research-Grade (Phase 4)](#research-grade-phase-4)
8. [Research Findings](#research-findings)
9. [Acceptance Criteria](#acceptance-criteria)
10. [Risk Analysis](#risk-analysis)

---

## Current State Analysis

### Where Time Goes

The 6-phase pipeline (plan, implement, self-check, code-review, acceptance-test, deploy) for the Spring PetClinic benchmark consumes ~110 minutes with 395 LLM calls and 13M tokens. Based on code analysis, the time distributes approximately:

| Phase | Est. Minutes | LLM Calls | Key Cost Driver |
|-------|-------------|-----------|-----------------|
| Plan (parallel) | ~5 | 10-15 | 2 parallel planners + merge |
| Implement (parallel) | ~25 | 80-120 | Main coding, tool execution |
| Self-check | ~10 | 30-40 | Re-reads all files, runs tests |
| Code-review (sub-agent) | ~20 | 50-70 | Fresh agent, reads everything from scratch |
| Acceptance-test (sub-agent) | ~20 | 50-70 | Fresh agent, runs everything from scratch |
| Fix cycles (1-3x) | ~25 | 80-100 | Back to self-check, re-review, re-test |
| Deploy | ~5 | 10-15 | Cleanup |

### Structural Bottlenecks Identified in Code

1. **Sequential review phases** (`advancePipeline`, lines 1477-1539): code-review runs, then acceptance-test runs. Both are `UseSubAgent: true` with no shared context -- they are already independent but run sequentially.

2. **Full context on every call** (`buildMessages`, line 471-473): every LLM call in the agent loop sends `a.registry.OpenAIToolDefinitions()` (all 11 tools) and the full system prompt (skills, spec constraints, project instructions). The system prompt alone is 3-8K tokens; tool schemas add ~2K more.

3. **No prompt caching**: the `ChatRequest` struct and `OllamaCloudProvider.ChatCompletion` use raw OpenAI-compatible JSON. No `cache_control` fields, no prefix ordering optimization.

4. **Blunt compaction** (`compactConversation`, line 2132): drops oldest messages, keeps last 20. No structured summary. The replacement message is just `"[Phase X completed. N messages compacted to DB.]"` -- loses semantic value.

5. **Full codebase re-read per review**: `runSubAgentPhase` (line 1672) spawns fresh agents with the full spec + criteria + skill context. Each reviewer starts from zero and re-reads every file.

6. **No early termination**: the loop runs until either `MaxTurns` is hit or the LLM produces a text-only response with no pipeline phases remaining. There is no "all tests pass, build succeeds, spec checklist done -> stop" shortcut.

7. **Same model for everything**: within a phase, every LLM call uses the same provider level. Simple classification decisions ("is this file relevant?") use the same heavyweight model as complex code generation.

---

## Priority Matrix

Improvements ordered by `(estimated minutes saved) / (implementation effort)`:

| Rank | Issue | Improvement | Est. Savings | Effort | Risk |
|------|-------|-------------|-------------|--------|------|
| 1 | #290 | Parallel verification | ~20 min | Low (1-2 days) | Very Low |
| 2 | #291 | Prompt caching | ~5 min latency + 60-80% cost | Low (2-3 days) | Very Low |
| 3 | #292 | Adaptive pipeline | ~10 min | Medium (3-5 days) | Low |
| 4 | #293 | Phase summaries | ~10 min | Medium (3-5 days) | Medium |
| 5 | #296 | Early termination | ~10 min | Medium (3-5 days) | Low |
| 6 | #295 | Diff-based re-review | ~10 min | Medium (5-7 days) | Medium |
| 7 | #294 | Dynamic tool definitions | ~5 min | High (5-7 days) | High |
| 8 | #297 | Speculative phase overlap | ~5 min | High (7-10 days) | Medium |
| 9 | #298 | ACON context compression | ~5 min | Very High (2-3 weeks) | High |
| 10 | #299 | Intra-agent smart routing | ~5 min | Very High (2-3 weeks) | High |

**Recommended quick wins (do first, minimal risk): #290, #291, #292.**
Combined estimated savings: ~35 minutes, reducing runtime from 110 to ~75 minutes.

---

## Dependency Graph

```
                    #291 Prompt Caching (independent)

#290 Parallel Verify ─────────────────────────────┐
                                                   │
#292 Adaptive Pipeline ──────────┐                 │
                                 │                 │
#296 Early Termination ──────────┤                 │
                                 ▼                 ▼
                    #293 Phase Summaries     #295 Diff-based Re-review
                         │                         │
                         ▼                         │
                    #298 ACON Compression ◄────────┘

#294 Dynamic Tool Defs (independent, but benefits from #291)

#297 Speculative Overlap (requires #290 parallel infra)

#299 Intra-agent Routing (requires #294 tool routing concept)
```

### Hard Dependencies

- **#297 requires #290**: speculative overlap builds on the parallel sub-agent infrastructure.
- **#298 benefits from #293**: ACON compression is most effective when phase summaries already provide structured boundaries.
- **#299 benefits from #294**: intra-agent model routing extends the dynamic tool selection pattern.
- **#295 requires #290 or sequential review**: diff-based re-review only matters when reviews happen (parallel or sequential).

### Soft Dependencies

- #293 (phase summaries) makes #295 (diff-based re-review) more effective -- reviewers get concise summaries instead of raw history.
- #296 (early termination) reduces the impact of #292 (adaptive pipeline) -- if tasks terminate early, skipping phases matters less.
- #291 (prompt caching) amplifies #293 (phase summaries) -- cached prefix + compact context = maximum token savings.

---

## Quick Wins (Phase 1)

### #290: Parallel Verification

**Goal:** Run code-review and acceptance-test simultaneously instead of sequentially.

**Why it works now:** Both phases already have `UseSubAgent: true` in `pipeline.go`. They spawn fresh agents with no shared conversation. The only thing making them sequential is the `advancePipeline` control flow, which advances one phase at a time.

**Technical Design:**

1. Add a `Parallel` field to `PipelinePhase`:

```go
// pipeline.go
type PipelinePhase struct {
    // ... existing fields ...
    ParallelWith []string // phase names to run concurrently with this one
}
```

2. Mark code-review and acceptance-test as parallel:

```go
{
    Name:         "code-review",
    Escalate:     true,
    MinToolCalls: 2,
    UseSubAgent:  true,
    ParallelWith: []string{"acceptance-test"},
    // ...
},
{
    Name:         "acceptance-test",
    Escalate:     true,
    MinToolCalls: 1,
    UseSubAgent:  true,
    ParallelWith: []string{"code-review"},  // mutual reference
    // ...
},
```

3. Modify `advancePipeline` in `agent.go` (~line 1477). When advancing to a phase that has `ParallelWith` entries, collect all parallel phases and run them concurrently:

```go
// In advancePipeline, when nextPhase.UseSubAgent && len(nextPhase.ParallelWith) > 0:
func (a *Agent) runParallelVerification(phases []PipelinePhase) (allPassed bool, mergedFindings string) {
    type phaseResult struct {
        name   string
        result string
        passed bool
    }

    results := make(chan phaseResult, len(phases))
    ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
    defer cancel()

    for _, phase := range phases {
        go func(p PipelinePhase) {
            result := a.runSubAgentPhase(p)
            results <- phaseResult{
                name:   p.Name,
                result: result,
                passed: IsPassSignal(result),
            }
        }(phase)
    }

    var findings []string
    allPassed = true
    for range phases {
        r := <-results
        if !r.passed {
            allPassed = false
            findings = append(findings, fmt.Sprintf("=== %s ===\n%s", r.name, r.result))
        }
    }
    mergedFindings = strings.Join(findings, "\n\n")
    return
}
```

4. In `advancePipeline`, replace the sequential sub-agent block with:

```go
if nextPhase.UseSubAgent {
    // Check if this phase should run in parallel with others
    parallelPhases := a.collectParallelPhases(a.pipelinePhase)
    if len(parallelPhases) > 1 {
        allPassed, mergedFindings := a.runParallelVerification(parallelPhases)
        // Skip past all parallel phases
        a.pipelinePhase += len(parallelPhases)
        if allPassed {
            // All passed -- continue to next non-parallel phase
            return a.advancePipeline("PHASE_PASSED")
        }
        // Merge findings and cycle back to self-check
        a.pipelineCycles++
        a.conversation.Add(providers.Message{
            Role:    "user",
            Content: fmt.Sprintf("Parallel review found issues:\n%s\nFix all issues.", mergedFindings),
        })
        a.pipelinePhase = a.findPhaseIndex("self-check", a.pipelinePhase-2)
        return true
    }
    // ... existing single sub-agent logic ...
}
```

**Files changed:** `internal/agent/pipeline.go`, `internal/agent/agent.go`

**Worktree isolation:** Both sub-agents read from the same working directory. Since they are read-only reviewers (they only run verification commands and read files), there is no write conflict. If the multi-provider rotation pattern is active (fix stage), the parallel approach must serialize the fix steps -- only the initial review step can parallelize.

**Estimated savings:** ~20 minutes (the two 20-minute phases overlap instead of running sequentially).

---

### #291: Prompt Caching

**Goal:** Enable provider-level prompt caching for system prompts, spec content, and tool definitions.

**Current state:** No caching support exists. The `ChatRequest` struct has no cache control fields. The `OllamaCloudProvider` sends raw JSON to the OpenAI-compatible endpoint.

**Provider caching landscape:**

| Provider | Mechanism | Min Tokens | TTL | Cost |
|----------|-----------|-----------|-----|------|
| Anthropic (Claude) | `cache_control: {type: "ephemeral"}` on content blocks | 1024 (Sonnet) / 4096 (Opus) | 5min default, 1hr optional | Write: 1.25x, Read: 0.1x |
| OpenAI | Automatic prefix matching | 1024 | 5-10min (auto-evict) | Free (automatic) |
| Google Gemini | Implicit (auto) + Explicit (context cache API) | 2048 | Implicit: auto, Explicit: 60min default | 90% discount on cached tokens (Gemini 2.5+) |
| Ollama Cloud | OpenAI-compatible -- automatic if provider supports it | Varies | Varies | Depends on upstream |

**Technical Design:**

1. Extend `ChatRequest` with cache control:

```go
// providers/provider.go
type Message struct {
    Role         string                   `json:"role"`
    Content      interface{}              `json:"content"`      // string or []ContentBlock
    // ... existing fields ...
}

// ContentBlock supports cache_control for Anthropic-style caching
type ContentBlock struct {
    Type         string                 `json:"type"`
    Text         string                 `json:"text,omitempty"`
    CacheControl map[string]string      `json:"cache_control,omitempty"`
}
```

2. Add a `CachePolicy` to `ChatRequest`:

```go
type ChatRequest struct {
    // ... existing fields ...
    CacheBreakpoints []int `json:"-"` // indices of messages to mark as cache breakpoints
}
```

3. Modify `buildMessages()` in `agent.go` to structure for caching:

```go
func (a *Agent) buildMessages() []providers.Message {
    // ... existing system prompt building ...

    // Structure: [system (cached)] [spec/criteria (cached)] [conversation (dynamic)]
    // This ordering maximizes prefix cache hits because:
    // - System prompt is identical across all calls in a session
    // - Spec content changes only between phases
    // - Conversation changes every turn

    msgs := []providers.Message{{
        Role: "system",
        Content: []ContentBlock{
            {
                Type: "text",
                Text: a.cachedSystemPrompt,
                CacheControl: map[string]string{"type": "ephemeral"},
            },
        },
    }}
    // ... rest of messages ...
}
```

4. Provider-specific serialization in each provider's `ChatCompletion`:

```go
// providers/vertex.go - Anthropic path
// Convert ContentBlock cache_control into Anthropic API format

// providers/ollama.go - OpenAI-compatible path
// OpenAI caching is automatic based on prefix matching.
// Ensure system prompt comes first (already does).
// Flatten ContentBlock back to string for non-Anthropic providers.
```

5. Add a helper to flatten `ContentBlock` to string for providers that do not support cache_control:

```go
func flattenContent(content interface{}) string {
    switch v := content.(type) {
    case string:
        return v
    case []ContentBlock:
        var parts []string
        for _, block := range v {
            parts = append(parts, block.Text)
        }
        return strings.Join(parts, "\n")
    }
    return fmt.Sprintf("%v", content)
}
```

**Prompt ordering strategy for maximum cache hits:**

```
Position 1: System prompt (static per session)         <- CACHE BREAKPOINT 1
Position 2: Tool definitions (static per session)       <- CACHE BREAKPOINT 2
Position 3: Spec constraints (static per session)       <- CACHE BREAKPOINT 3
Position 4: Phase prompt + criteria (changes per phase) <- CACHE BREAKPOINT 4
Position 5+: Conversation messages (changes every turn) <- NOT CACHED
```

This ordering ensures that even when conversation changes, the first 5-10K tokens of every request hit the cache. With 395 LLM calls, the cumulative savings are substantial.

**Files changed:** `internal/providers/provider.go`, `internal/providers/ollama.go`, `internal/providers/vertex.go`, `internal/agent/agent.go`

**Estimated savings:** ~5 minutes latency + 60-80% cost reduction on cached prefix tokens.

---

### #292: Adaptive Pipeline

**Goal:** Classify tasks by complexity and skip unnecessary phases for trivial work.

**Technical Design:**

1. Add a complexity classifier:

```go
// internal/agent/complexity.go
type TaskComplexity int

const (
    ComplexityTrivial  TaskComplexity = iota // typo fix, config change, comment update
    ComplexityStandard                        // feature, refactor, bug fix
    ComplexityCritical                        // auth, security, API, schema migration
)

type ComplexitySignals struct {
    FileCount     int
    DiffLineCount int
    TouchesPaths  []string
    HasTests      bool
    SpecLength    int
}

func ClassifyComplexity(signals ComplexitySignals) TaskComplexity {
    // Critical paths: always full pipeline
    criticalPatterns := []string{"auth", "security", "credential", "password",
        "token", "migration", "schema", "config/prod", ".env"}
    for _, path := range signals.TouchesPaths {
        for _, pattern := range criticalPatterns {
            if strings.Contains(strings.ToLower(path), pattern) {
                return ComplexityCritical
            }
        }
    }

    // Trivial: single file, <50 lines changed, no tests needed
    if signals.FileCount <= 2 && signals.DiffLineCount < 50 && !signals.HasTests {
        return ComplexityTrivial
    }

    // Standard: everything else
    return ComplexityStandard
}
```

2. Add phase skip logic based on complexity:

```go
// SkippablePhases returns phases that can be skipped for this complexity level.
func SkippablePhases(complexity TaskComplexity) map[string]bool {
    switch complexity {
    case ComplexityTrivial:
        return map[string]bool{
            "code-review":     true,
            "acceptance-test": true,
            // Keep: plan, implement, self-check, deploy
        }
    case ComplexityCritical:
        return nil // never skip anything
    default:
        return nil // standard: full pipeline
    }
}
```

3. Integrate into `advancePipeline`:

```go
// When advancing to a new phase, check if it should be skipped
if a.skipPhases != nil && a.skipPhases[nextPhase.Name] {
    log.Printf("[Agent] adaptive pipeline: skipping %s (complexity: %s)",
        nextPhase.Name, a.taskComplexity)
    a.pipelinePhase++
    return a.advancePipeline("PHASE_PASSED")
}
```

4. Collect complexity signals after the implement phase completes:

```go
// After implement completes, measure what was actually changed
func (a *Agent) collectComplexitySignals() ComplexitySignals {
    // Run: git diff --stat HEAD
    // Count files changed, lines changed
    // Check which paths were touched
    // Return signals for classification
}
```

**Key design decision:** Complexity is assessed AFTER implementation, not before. The plan phase always runs (it is fast). After implement completes, the system measures what was actually changed and decides whether code-review and acceptance-test are needed. This avoids pre-judging complexity incorrectly.

**Safety valve:** If any verification gate check fails during self-check, complexity is upgraded to Standard regardless of initial classification.

**Files changed:** `internal/agent/complexity.go` (new), `internal/agent/agent.go`, `internal/agent/pipeline.go`

**Estimated savings:** ~10 minutes for trivial tasks (skips 2 phases entirely). No savings for standard/critical tasks.

---

## Context Engineering (Phase 2)

### #293: Phase Summaries

**Goal:** Replace blunt message dropping with structured summaries that preserve semantic value.

**Current state:** `compactConversation` (line 2132) drops all but the last 20 messages and replaces them with a one-line notice. This loses information about what was built, what decisions were made, and what was tested.

**Technical Design:**

1. Generate a structured summary before dropping messages:

```go
// internal/agent/summary.go
type PhaseSummary struct {
    Phase        string   `json:"phase"`
    FilesCreated []string `json:"files_created"`
    FilesModified []string `json:"files_modified"`
    TestsRun     bool     `json:"tests_run"`
    TestsPassed  bool     `json:"tests_passed"`
    KeyDecisions []string `json:"key_decisions"`
    OpenIssues   []string `json:"open_issues"`
}

func ExtractPhaseSummary(messages []providers.Message, phaseName string) PhaseSummary {
    summary := PhaseSummary{Phase: phaseName}

    for _, msg := range messages {
        // Extract file operations from tool call messages
        if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
            for _, tc := range msg.ToolCalls {
                name, args := extractToolCallNameArgs(tc)
                switch name {
                case "file_write":
                    if path, ok := args["path"].(string); ok {
                        summary.FilesCreated = append(summary.FilesCreated, path)
                    }
                case "file_edit":
                    if path, ok := args["path"].(string); ok {
                        summary.FilesModified = append(summary.FilesModified, path)
                    }
                }
            }
        }
        // Extract test results from tool output messages
        if msg.Role == "tool" && strings.Contains(msg.Content, "PASS") {
            summary.TestsRun = true
            if !strings.Contains(msg.Content, "FAIL") {
                summary.TestsPassed = true
            }
        }
    }

    return summary
}
```

2. Replace the compaction message with a structured summary:

```go
func (a *Agent) compactConversation(completedPhase string) {
    msgs := a.conversation.Messages()
    if len(msgs) <= 30 {
        return
    }

    keepCount := 10  // reduced from 20 -- summaries preserve context
    dropCount := len(msgs) - keepCount

    // Extract structured summary before dropping
    summary := ExtractPhaseSummary(msgs[:dropCount], completedPhase)

    // Store to DB for recall
    a.storeMessagesToDB(msgs[:dropCount], "compaction")

    recent := make([]providers.Message, keepCount)
    copy(recent, msgs[dropCount:])

    a.conversation.Clear()
    a.conversation.Add(providers.Message{
        Role: "user",
        Content: fmt.Sprintf(`[Phase %s completed — summary:]
Files created: %s
Files modified: %s
Tests: run=%v passed=%v
Key state: %d messages compacted. Use recall tool for detailed history.`,
            completedPhase,
            strings.Join(summary.FilesCreated, ", "),
            strings.Join(summary.FilesModified, ", "),
            summary.TestsRun, summary.TestsPassed,
            dropCount),
    })
    for _, m := range recent {
        a.conversation.Add(m)
    }
    a.hasCompacted = true
}
```

3. **Delta entries pattern** (from ACON research): instead of rewriting the entire summary each phase, append incremental updates:

```go
// Each phase appends a delta, not a full rewrite
a.phaseSummaries = append(a.phaseSummaries, summary)
// The compaction message includes ALL phase summaries as a running log
```

**Files changed:** `internal/agent/summary.go` (new), `internal/agent/agent.go`

**Estimated savings:** ~10 minutes (smaller context = faster LLM responses, fewer wasted tokens re-reading old outputs).

---

### #294: Dynamic Tool Definitions

**Goal:** Only send tool schemas relevant to the current action instead of all 11 tools on every call.

**Current state:** Every LLM call includes `a.registry.OpenAIToolDefinitions()` which serializes all registered tools (bash, file_read, file_write, file_edit, grep, glob, git, web_search, web_fetch, recall, delegate/handoff). This adds ~2-3K tokens per call.

**Technical Design:**

1. Add phase-aware tool filtering:

```go
// internal/tools/registry.go
func (r *Registry) ToolDefinitionsForPhase(phase string) []map[string]interface{} {
    r.mu.RLock()
    defer r.mu.RUnlock()

    allowed := phaseTools[phase]
    if allowed == nil {
        return r.OpenAIToolDefinitions() // fallback: all tools
    }

    var defs []map[string]interface{}
    for _, tool := range r.tools {
        if allowed[tool.Name()] {
            defs = append(defs, map[string]interface{}{
                "type": "function",
                "function": map[string]interface{}{
                    "name":        tool.Name(),
                    "description": tool.Description(),
                    "parameters":  tool.InputSchema(),
                },
            })
        }
    }
    return defs
}

var phaseTools = map[string]map[string]bool{
    "plan":            {"bash": true, "file_read": true, "grep": true, "glob": true, "web_search": true, "web_fetch": true, "recall": true},
    "implement":       {"bash": true, "file_read": true, "file_write": true, "file_edit": true, "grep": true, "glob": true, "git": true, "recall": true},
    "self-check":      {"bash": true, "file_read": true, "file_edit": true, "grep": true, "glob": true, "recall": true},
    "code-review":     {"bash": true, "file_read": true, "grep": true, "glob": true, "recall": true},
    "acceptance-test": {"bash": true, "file_read": true, "grep": true, "glob": true, "recall": true},
    "deploy":          {"bash": true, "file_read": true, "file_write": true, "git": true, "glob": true},
}
```

2. Modify the agent loop to use phase-aware tools:

```go
// In loop(), line 471
phaseName := ""
if a.pipeline != nil && a.pipelinePhase < len(a.pipeline.Phases) {
    phaseName = a.pipeline.Phases[a.pipelinePhase].Name
}
req := providers.ChatRequest{
    Model:    a.config.Model,
    Messages: a.buildMessages(),
    Tools:    a.registry.ToolDefinitionsForPhase(phaseName),
}
```

**Risk:** If the LLM needs a tool not in the phase set, it cannot call it. Mitigation: keep `bash` in every phase (it can do anything), and treat the phase tool sets as a "fast path" with a fallback to all tools if the model explicitly requests an unavailable tool (detected by a tool_call to an unknown name).

**Alternative approach (search_tools pattern from Speakeasy):** Instead of pre-filtering, add a `search_tools` meta-tool that lets the LLM discover available tools. This is more flexible but adds an extra LLM roundtrip. Not recommended for our use case where phases are well-defined.

**Files changed:** `internal/tools/registry.go`, `internal/agent/agent.go`

**Estimated savings:** ~5 minutes (2-3K fewer tokens per call x 395 calls = 800K-1.2M fewer tokens).

---

### #295: Diff-Based Re-Review

**Goal:** When a review cycle finds issues and the agent fixes them, the next review only examines changed files with surrounding context instead of re-reading the entire codebase.

**Current state:** `runSubAgentPhase` (line 1672) gives each reviewer the full spec + criteria + skill context. The reviewer then reads EVERY file from scratch. On fix cycles (pipeline cycles 2-3), this is wasteful because only a few files changed.

**Technical Design:**

1. Capture a diff snapshot after fixes:

```go
// internal/agent/diff.go
func CaptureChangeSummary(workDir string) (string, error) {
    // Get diff since last review cycle
    // Uses git diff if available, falls back to file modification times
    cmd := exec.Command("git", "diff", "--stat", "HEAD~1")
    cmd.Dir = workDir
    output, err := cmd.Output()
    if err != nil {
        return "", err
    }
    return string(output), nil
}

func CaptureDetailedDiff(workDir string, maxLines int) (string, error) {
    cmd := exec.Command("git", "diff", "HEAD~1")
    cmd.Dir = workDir
    output, err := cmd.Output()
    if err != nil {
        return "", err
    }
    diff := string(output)
    if len(strings.Split(diff, "\n")) > maxLines {
        diff = truncateDiff(diff, maxLines)
    }
    return diff, nil
}
```

2. Modify the fix-cycle reviewer task to be diff-focused:

```go
// In runSubAgentPhase, when pipelineCycles > 0:
if a.pipelineCycles > 0 {
    diffSummary, _ := CaptureChangeSummary(a.config.WorkDir)
    detailedDiff, _ := CaptureDetailedDiff(a.config.WorkDir, 500)

    task = fmt.Sprintf(`You are reviewing CHANGES made to fix previous review findings.

CHANGES SINCE LAST REVIEW:
---
%s
---

DETAILED DIFF:
---
%s
---

PREVIOUS FINDINGS THAT WERE FIXED:
---
%s
---

Focus on:
1. Were ALL previous findings addressed?
2. Did the fixes introduce new issues? (check surrounding code, not just the diff)
3. Run verification commands to confirm fixes work.

Say VERIFIED_CORRECT if all fixes look good, or NEEDS_FIX with specifics.`,
        diffSummary, detailedDiff, previousFindings)
}
```

3. **Context-aware, not diff-only:** The reviewer still gets the full spec and criteria, but is directed to focus on changes. This avoids the 40-60% cross-file issue miss rate that pure diff-only review produces (CodeRabbit data). The diff is the primary input; the full codebase is available via tools if the reviewer needs broader context.

**Files changed:** `internal/agent/diff.go` (new), `internal/agent/agent.go`

**Estimated savings:** ~10 minutes on fix cycles (reviewer reads 50-100 lines of diff instead of 500+ lines of full files).

---

## Advanced Optimizations (Phase 3)

### #296: Early Termination

**Goal:** Replace subjective LLM self-assessment with machine-verifiable completion criteria.

**Current state:** The pipeline advances when the LLM outputs a pass signal like `IMPLEMENT_COMPLETE` or `SELF_CHECK_PASS`. The verification gate (`RunVerificationGate`) already runs programmatic checks (build, test, spec compliance), but it only runs AFTER the LLM claims completion. There is no mechanism to proactively check "are we done?" without waiting for the LLM to declare it.

**Technical Design:**

1. Add a completion checker that runs periodically:

```go
// internal/agent/completion.go
type CompletionCriteria struct {
    BuildPasses     bool
    TestsPassed     bool
    SpecChecklist   map[string]bool  // derived from acceptance criteria
    FilesExist      []string         // required output files
}

func (cc *CompletionCriteria) AllMet() bool {
    if !cc.BuildPasses || !cc.TestsPassed {
        return false
    }
    for _, met := range cc.SpecChecklist {
        if !met {
            return false
        }
    }
    return true
}

// CheckCompletion runs objective checks without asking the LLM
func (a *Agent) CheckCompletion() *CompletionCriteria {
    criteria := &CompletionCriteria{
        SpecChecklist: make(map[string]bool),
    }

    // Run build
    allPassed, results := a.RunVerificationGate(a.currentPhaseName())
    criteria.BuildPasses = allPassed

    for _, r := range results {
        if strings.HasPrefix(r.Name, "test/") {
            criteria.TestsPassed = r.Passed
        }
    }

    // Check required files exist (from acceptance criteria)
    for _, f := range a.requiredOutputFiles() {
        criteria.FilesExist = append(criteria.FilesExist, f)
        _, err := os.Stat(filepath.Join(a.config.WorkDir, f))
        criteria.SpecChecklist[f] = err == nil
    }

    return criteria
}
```

2. Integrate into the agent loop as a periodic check:

```go
// In loop(), after tool execution:
// Every 5 tool calls during implement phase, check if we're already done
if a.phaseToolCalls > 0 && a.phaseToolCalls % 5 == 0 &&
    a.currentPhaseName() == "implement" {
    criteria := a.CheckCompletion()
    if criteria.AllMet() {
        log.Printf("[Agent] early termination: all completion criteria met at tool call %d",
            a.phaseToolCalls)
        a.conversation.Add(providers.Message{
            Role: "user",
            Content: "All objective completion criteria are met (build passes, tests pass, required files exist). Say IMPLEMENT_COMPLETE to proceed to review.",
        })
    }
}
```

3. **Ralph Loop pattern:** Instead of checking periodically, hook into the LLM's "I'm done" signal and verify before accepting:

```go
// Already partially implemented via RunVerificationGate.
// Enhancement: also check a parsed spec checklist.
```

**Files changed:** `internal/agent/completion.go` (new), `internal/agent/agent.go`

**Estimated savings:** ~10 minutes (catches completion 5-10 turns earlier than LLM self-assessment, avoids unnecessary additional tool calls after the work is actually done).

---

### #297: Speculative Phase Overlap

**Goal:** Start loading context for the next phase while the current phase finishes its last iterations.

**Technical Design:**

1. When the implement phase has been running for >70% of its turn budget, speculatively prepare review context:

```go
// internal/agent/speculative.go
type SpeculativeContext struct {
    mu          sync.Mutex
    ready       bool
    fileList    string    // pre-computed file listing
    buildResult string    // pre-run build check
    diffStat    string    // pre-computed diff
    codeHash    string    // hash of code state when context was built
}

func (a *Agent) startSpeculativePrep() {
    go func() {
        ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
        defer cancel()

        sc := &SpeculativeContext{}

        // Pre-compute file listing
        sc.fileList, _ = a.runVerifyCommand("speculative/files",
            fmt.Sprintf("find %s -name '*.go' -o -name '*.java' -o -name '*.py' | head -50",
                a.config.WorkDir)).Output

        // Pre-run build
        env := environment.Detect(a.config.WorkDir)
        if buildCmd := environment.BuildCommand(env); buildCmd != "" {
            sc.buildResult = a.runVerifyCommand("speculative/build", buildCmd).Output
        }

        // Hash current code state
        sc.codeHash = hashWorkDir(a.config.WorkDir)

        sc.ready = true
        a.speculativeCtx = sc
    }()
}
```

2. When the review sub-agent starts, check if speculative context is still valid:

```go
// In runSubAgentPhase:
if a.speculativeCtx != nil && a.speculativeCtx.ready {
    currentHash := hashWorkDir(a.config.WorkDir)
    if currentHash == a.speculativeCtx.codeHash {
        // Context is still valid -- inject pre-computed results
        task += fmt.Sprintf("\nPRE-COMPUTED CONTEXT:\nFiles: %s\nBuild: %s",
            a.speculativeCtx.fileList, a.speculativeCtx.buildResult)
    } else {
        log.Printf("[Agent] speculative context invalidated (code changed)")
    }
}
```

3. Trigger speculative prep at 70% of turn budget:

```go
// In loop(), after tool execution:
if a.currentPhaseName() == "implement" &&
    a.phaseTurns > int(float64(a.maxPhaseTurns()) * 0.7) &&
    a.speculativeCtx == nil {
    a.startSpeculativePrep()
}
```

**Risk:** Wasted computation if implementation changes after speculative prep. Mitigation: the `codeHash` check ensures stale context is discarded. The speculative work is lightweight (file listing + build check), so waste is minimal.

**Files changed:** `internal/agent/speculative.go` (new), `internal/agent/agent.go`

**Estimated savings:** ~5 minutes (review sub-agents skip initial file discovery and build checks).

---

## Research-Grade (Phase 4)

### #298: ACON Context Compression

**Goal:** Apply the ACON framework to compress both environment observations (tool outputs) and interaction histories (conversation) using LLM-optimized compression guidelines.

**Background (from research):**

The [ACON paper](https://arxiv.org/abs/2510.00615) (Kang et al., 2025) introduces gradient-free optimization of natural language compression guidelines. Key insight: rather than compressing with a fixed strategy, ACON iteratively refines compression instructions by analyzing failure cases -- "full context succeeded but compressed context failed, why?"

**Technical Design:**

1. **Compression guideline store:**

```go
// internal/agent/acon.go
type CompressionGuideline struct {
    ToolOutputRules   map[string]string // tool name -> compression instruction
    HistoryRules      string            // how to compress conversation history
    Version           int
    LastUpdated       time.Time
}

// Default guidelines (refined through production usage)
var DefaultGuideline = CompressionGuideline{
    ToolOutputRules: map[string]string{
        "bash":      "Keep exit code, first 20 lines of output, last 10 lines. Drop middle.",
        "file_read": "Keep file path and first 50 lines. Summarize rest as '[N more lines]'.",
        "grep":      "Keep all match lines. Drop context lines if >20 matches.",
        "glob":      "Keep all file paths.",
    },
    HistoryRules: "Keep: tool calls with results, decisions, errors. Drop: exploratory reads that led nowhere, repeated attempts at the same fix.",
}
```

2. **Compress tool outputs at storage time** (already partially implemented in `tool_store.go`):

```go
// Enhance existing tool output storage to apply compression guidelines
func CompressToolOutput(toolName, output string, guideline CompressionGuideline) string {
    rule, ok := guideline.ToolOutputRules[toolName]
    if !ok {
        // Default: keep first 100 lines, truncate rest
        lines := strings.Split(output, "\n")
        if len(lines) > 100 {
            return strings.Join(lines[:100], "\n") + fmt.Sprintf("\n[...%d more lines]", len(lines)-100)
        }
        return output
    }
    return applyRule(output, rule)
}
```

3. **Failure-driven refinement** (the ACON innovation):

```go
// After a review cycle failure, analyze whether compression lost critical info
func (a *Agent) analyzeCompressionFailure(fullCtx, compressedCtx, failureReason string) {
    // This would require an LLM call to analyze the failure
    // "What information in the full context was missing from the compressed version
    //  that caused this failure?"
    // Then update the compression guideline accordingly.

    // For initial implementation: log failures for manual review
    log.Printf("[ACON] compression may have lost info: %s", failureReason)
}
```

**Phased rollout:**
- **v1 (immediate):** Rule-based compression of tool outputs at storage time. No LLM calls needed. Reduces token count by 30-40%.
- **v2 (later):** LLM-generated summaries at phase boundaries (builds on #293).
- **v3 (research):** Failure-driven guideline refinement per ACON paper.

**Files changed:** `internal/agent/acon.go` (new), `internal/agent/agent.go`, `internal/tools/registry.go`

**Estimated savings:** ~5 minutes (26-54% token reduction per ACON benchmarks, but our phases are already compacted).

---

### #299: Intra-Agent Smart Model Routing

**Goal:** Use cheap/fast models for simple decisions within the agent loop, reserving heavyweight models for actual code generation and reasoning.

**Current state:** The agent uses a single provider level for all calls within a phase. A simple decision like "which file should I read next?" uses the same model as "implement this complex algorithm."

**Technical Design:**

1. Classify LLM call intent:

```go
// internal/agent/model_routing.go
type CallIntent int

const (
    IntentClassify  CallIntent = iota // "which file is relevant?" -- cheap model
    IntentNavigate                     // "what should I do next?" -- cheap model
    IntentGenerate                     // "write this code" -- heavyweight model
    IntentReason                       // "analyze this error" -- heavyweight model
    IntentReview                       // "is this correct?" -- heavyweight model
)

func ClassifyCallIntent(messages []providers.Message) CallIntent {
    lastUser := ""
    for i := len(messages) - 1; i >= 0; i-- {
        if messages[i].Role == "user" {
            lastUser = messages[i].Content
            break
        }
    }

    // Heuristic classification based on conversation state
    if strings.Contains(lastUser, "which file") || strings.Contains(lastUser, "list") {
        return IntentClassify
    }
    if strings.Contains(lastUser, "PHASE:") {
        return IntentGenerate
    }
    return IntentGenerate // default to heavyweight
}
```

2. Route to appropriate model tier:

```go
// In loop(), before LLM call:
intent := ClassifyCallIntent(msgs)
model := a.config.Model
if intent == IntentClassify || intent == IntentNavigate {
    // Use Level 0 (cheapest) model for simple decisions
    if len(a.config.EscalationChain) > 0 {
        model = a.config.EscalationChain[0].Providers[0]
    }
}
```

**Risk:** This is the highest-risk optimization. Misclassifying a complex task as simple leads to bad output that wastes time in later phases. The classifier must be conservative -- when in doubt, use the heavyweight model.

**Practical concern:** Ollama Cloud (the primary provider) has a flat pricing model. Intra-agent routing only saves money/time if the cheap model is significantly faster. Given that Ollama Cloud latency is dominated by network round-trip and queue wait (not model size), the savings may be smaller than expected.

**Recommendation:** Defer this until after Phase 1-2 optimizations are measured. The heuristic classifier is brittle and the savings are speculative.

**Files changed:** `internal/agent/model_routing.go` (new), `internal/agent/agent.go`

**Estimated savings:** ~5 minutes (highly uncertain, depends on provider latency characteristics).

---

## Research Findings

### Prompt Caching Across Providers

| Feature | Anthropic Claude | OpenAI | Google Gemini |
|---------|-----------------|--------|---------------|
| Activation | Explicit (`cache_control`) or automatic | Automatic (no code change) | Implicit (auto) + Explicit (context cache API) |
| Min tokens | 1024 (Sonnet), 4096 (Opus/Haiku) | 1024 | 2048 |
| TTL | 5min (default), 1hr (paid) | 5-10min (auto-evict) | Implicit: auto, Explicit: 60min |
| Cost model | Write: 1.25x, Read: 0.1x | Free | Read: 90% discount (Gemini 2.5+) |
| Scope | System, tools, messages | Prefix-based (automatic) | System, tools, messages |
| Cache key | Content hash (org-isolated) | Prefix hash + optional `prompt_cache_key` | Content-based |

**Key takeaway for synapserouter:** Since the primary provider (Ollama Cloud) uses OpenAI-compatible endpoints, prompt caching may already be partially automatic. The explicit `cache_control` approach is needed for Vertex AI (Claude path) and would be a no-op for OpenAI-compatible endpoints. The implementation should be provider-aware: add `cache_control` fields for Anthropic/Vertex, and ensure correct prefix ordering for OpenAI-compatible endpoints.

Sources: [Claude Prompt Caching](https://platform.claude.com/docs/en/build-with-claude/prompt-caching), [OpenAI Prompt Caching](https://developers.openai.com/api/docs/guides/prompt-caching), [Vertex AI Context Caching](https://docs.cloud.google.com/vertex-ai/generative-ai/docs/context-cache/context-cache-overview)

### ACON Framework

The [ACON paper](https://arxiv.org/abs/2510.00615) (arXiv:2510.00615) demonstrates:

- **26-54% peak token reduction** with >95% accuracy preserved across AppWorld, OfficeBench, and Multi-objective QA benchmarks.
- **Gradient-free optimization**: uses LLMs as optimizers to refine compression guidelines through failure analysis. No fine-tuning needed -- works with any API model.
- **Distillable**: the optimized compressor can be distilled into smaller models (Qwen3-14B achieves 95% of teacher accuracy).
- **Universal**: supports both observation compression (tool outputs) and history compression (conversation).

**Applicability to synapserouter:** ACON's core insight maps directly to our pipeline. Our phases already create natural compression boundaries. The failure-driven refinement loop (full context succeeds, compressed fails -> analyze why -> update guidelines) can be implemented as a background learning process using pipeline failure data we already track.

Source: [ACON Paper](https://arxiv.org/abs/2510.00615), [HuggingFace Paper Page](https://huggingface.co/papers/2510.00615)

### Speculative Execution in Agent Frameworks

Key papers and systems (2025-2026):

1. **PASTE** (arXiv:2603.18897): Pattern-Aware Speculative Tool Execution. Exploits recurring tool-call sequences to predict and pre-execute likely next tools. 48.5% latency reduction, 1.8x throughput improvement.

2. **Speculative Actions** (arXiv:2510.04371): Generalizes speculative execution from CPU microarchitecture to agent environments. Agents predict and tentatively pursue likely next actions using faster models, with validation (not waiting) as the critical path. Supports rollback for incorrect speculations.

3. **M1-Parallel** (arXiv:2507.08944): Schedules multiple agent teams to solve tasks independently and concurrently, terminating early when the first team completes. Up to 2.2x speedup with early termination.

4. **SPAgent**: Runs speculation alongside normal reasoning. Candidate actions are executed speculatively in parallel; if they match the main path's eventual action, results are reused. 1.08-1.65x speedup.

**Applicability to synapserouter:** The most relevant pattern is speculative context preparation (not speculative execution of the full next phase). Starting file listing, build checks, and test runs while implementation is finishing is low-risk and provides free parallelism. Full speculative phase execution (starting a review while implementation continues) is higher risk due to invalidation when code changes.

Sources: [PASTE Paper](https://arxiv.org/abs/2603.18897), [Speculative Actions](https://arxiv.org/pdf/2510.04371), [M1-Parallel](https://arxiv.org/html/2507.08944v1)

### Early Termination Patterns

The **Ralph Loop** pattern (from Claude Code community) intercepts the agent's "I'm done" signal with a verification prompt. Instead of trusting the LLM's self-assessment, a stop hook runs objective checks and only allows termination when criteria are met.

SynapseRouter's `RunVerificationGate` already implements the verification half of this pattern. The missing piece is proactive completion checking -- detecting that all criteria are met BEFORE the LLM declares completion, and nudging it to finish early.

---

## Acceptance Criteria

### Epic-Level (#289)

1. **Runtime target:** Median pipeline runtime for a standard task (Spring PetClinic complexity) drops from 110 minutes to 30 minutes or less.
2. **Accuracy preserved:** Eval pass rate on the exercism benchmark does not decrease by more than 2% compared to the pre-optimization baseline.
3. **Cost reduction:** Total token consumption per pipeline run decreases by at least 50%.
4. **No regression in quality gates:** Verification gate pass rate remains stable. Code-review and acceptance-test catch rates do not decrease.

### Per-Issue Acceptance Criteria

| Issue | Metric | Target |
|-------|--------|--------|
| #290 | Review phase wall-clock time | Reduced by 40-50% (two phases overlap) |
| #291 | Cached token percentage per run | >60% of input tokens served from cache |
| #292 | Trivial task runtime | <15 minutes (vs current ~60 for short tasks) |
| #293 | Context size at phase N | <50% of current size at same phase |
| #294 | Tool definition tokens per call | <500 tokens (vs current ~2-3K) |
| #295 | Fix-cycle review time | <50% of initial review time |
| #296 | Turns after objective completion | <3 turns between "all tests pass" and phase advance |
| #297 | Review startup latency | <30% reduction (pre-computed context available) |
| #298 | Peak token count per session | 26-54% reduction (per ACON benchmarks) |
| #299 | Cheap-model call percentage | >30% of total calls routed to fast model |

### Measurement Framework

```go
// Add timing instrumentation to each phase
type PhaseMetrics struct {
    PhaseName    string
    StartTime    time.Time
    EndTime      time.Time
    LLMCalls     int
    TokensIn     int
    TokensOut    int
    CachedTokens int
    ToolCalls    int
}
```

Emit `PhaseMetrics` as structured logs. Build a comparison dashboard:
- Before/after runtime per phase
- Before/after token consumption per phase
- Cache hit rate over time
- Accuracy on eval benchmarks (exercism, reconstruction)

---

## Risk Analysis

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Parallel review misses cross-file issues | Low | Medium | Both reviewers still get full spec + tools; diff-only applies to fix cycles |
| Adaptive pipeline skips review for non-trivial task | Low | High | Safety valve: any self-check failure upgrades complexity to Standard |
| Prompt caching TTL misses (provider-specific) | Medium | Low | Ordering optimization helps even without explicit caching |
| Phase summaries lose critical context | Medium | Medium | Store full messages to DB; recall tool available; keep more messages (20 vs 10) initially |
| Dynamic tool defs prevent needed tool call | Low | Medium | Keep bash in all phases; fallback to full tool set on unknown tool request |
| Speculative prep wastes compute on changing code | Medium | Low | Code hash validation; lightweight prep work; discard on mismatch |
| ACON compression too aggressive | Medium | High | Start with conservative rule-based compression; add failure tracking |
| Intra-agent routing misclassifies complex task | High | High | Conservative default (heavyweight); defer until other optimizations measured |

### Implementation Order (Recommended)

```
Week 1-2:  #290 (parallel verify) + #291 (prompt caching)     -> ~25 min saved
Week 3:    #292 (adaptive pipeline)                            -> ~10 min saved
Week 4-5:  #293 (phase summaries) + #296 (early termination)  -> ~20 min saved
Week 6-7:  #295 (diff-based re-review) + #294 (dynamic tools) -> ~15 min saved
Week 8-9:  #297 (speculative overlap)                          -> ~5 min saved
Week 10+:  #298 (ACON) + #299 (smart routing)                 -> ~10 min saved

Cumulative projected: 110 -> ~25 minutes
```

The first three items (#290, #291, #292) are the **quick wins** -- they can be implemented in 1-2 weeks with minimal risk and provide the largest per-effort savings. Start there, measure, then proceed.
