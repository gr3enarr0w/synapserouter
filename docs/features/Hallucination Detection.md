---
title: Hallucination Detection
area: agent
status: implemented
created: 2026-03-26
tags:
  - agent
  - hallucination
  - fact-checking
  - auto-recall
related:
  - "[[Agent Loop]]"
  - "[[Tool Store]]"
  - "[[Auto Recall]]"
  - "[[Guardrails]]"
---

# Hallucination Detection

Synapserouter's agent includes a built-in hallucination detection system that cross-checks LLM text responses against ground-truth facts accumulated from tool executions. When the LLM makes claims that contradict observed reality (e.g., "all tests pass" when the last test run failed), the system automatically injects a corrective message and re-runs the LLM.

All checks are pure string matching -- no LLM calls, sub-1ms latency.

## Architecture

```
Tool Execution
      │
      ▼
┌─────────────┐     Records facts from every tool call
│ FactTracker │◄─── (bash exit codes, test results, file paths)
└─────┬───────┘
      │  Provides ground truth
      ▼
┌─────────────────────────┐     Compares LLM text against facts
│ CheckForHallucinations  │◄─── 4 detection rules, regex-based
└─────┬───────────────────┘
      │  HallucinationCheckResult (signals + confidence)
      ▼
┌────────────┐     Builds corrective message with evidence
│ autoRecall │◄─── Rate-limited (max 3), secret-scrubbed, 4KB cap
└─────┬──────┘
      │  Corrective user message injected into conversation
      ▼
┌────────────┐
│ Agent Loop │──── continue (re-runs LLM with correction context)
└────────────┘
```

## FactProvider Interface

The `FactProvider` interface defines the contract for querying ground-truth facts. It is implemented by `FactTracker` and kept small for testability.

```go
type FactProvider interface {
    KnownPaths()                map[string]bool
    LastTestResult()            *TestFact
    LastBashResult(prefix string) *BashFact
    RecentBashFacts(n int)      []BashFact
}
```

### What Gets Tracked

The `FactTracker` is an in-memory, zero-latency accumulator that records facts from every tool execution. It is initialized once per `Agent.Run()` call and populated inside `executeToolCalls()`.

| Tool | What is recorded |
|------|-----------------|
| `bash` | Command string, exit code, last 5 lines of output, output ID, timestamp. If the command matches a test prefix (`go test`, `pytest`, `npm test`, `cargo test`, etc.), a `TestFact` is also parsed. |
| `file_read`, `grep`, `glob` | File paths from arguments and from output (via regex extraction). |
| `file_write`, `file_edit` | File paths from arguments. |

Key implementation details:

- **Bash facts** are capped at the most recent 20 entries (sliding window).
- **Path extraction** uses a regex that matches common source file extensions (`.go`, `.py`, `.js`, `.ts`, `.rs`, `.java`, `.rb`, `.c`, `.cpp`, `.h`, `.yaml`, `.yml`, `.json`, `.toml`, `.md`, `.sql`, `.sh`), limited to 50 matches per output.
- **Test result parsing** supports Go (`ok <pkg>` / `FAIL <pkg>`) and pytest (`N passed` / `N failed`) output formats.
- **Concurrency**: writes happen only from the agent loop (single-threaded). Reads are protected by `sync.RWMutex` and return copies to prevent races.

## Detection Rules

`CheckForHallucinations` runs four independent rules against the LLM's text response. Each rule produces zero or more `HallucinationSignal` values with a type, description, evidence string, and severity score (0.0--1.0).

### Rule 1: Test Pass/Fail Contradiction

**Severity: 0.9**

Detects when the LLM claims tests pass but the most recent test run failed (non-zero exit code).

Trigger patterns: `all tests pass`, `tests pass`, `test suite passes`, `tests succeed`, `tests are passing`, `PASS`

Example:
- LLM says: "All tests pass successfully."
- Fact: Last `go test ./...` exited with code 1, with `FAIL internal/agent` in output.
- Signal: `SignalFalseSuccess` -- "Claims tests pass but most recent test run failed"

### Rule 2: Build Success Contradiction

**Severity: 0.9**

Detects when the LLM claims a build succeeded but the most recent build command failed. Checks `go build`, `npm run build`, and `cargo build` prefixes in order.

Trigger patterns: `builds successfully`, `compilation succeeds`, `build passes`, `compiles without errors`, `compiles cleanly`

Example:
- LLM says: "The project compiles cleanly now."
- Fact: Last `go build -o synroute .` exited with code 2.
- Signal: `SignalFalseSuccess` -- "Claims build succeeds but most recent build failed"

### Rule 3: Unknown File Paths

**Severity: 0.5**

Detects when the LLM references file paths that were never seen in any tool output. This catches fabricated file names and hallucinated project structures.

The checker extracts up to 20 file paths from the LLM response using the same regex pattern as `FactTracker`. For each path:

1. Skip if it is a [[#Common Paths|common path]] (see below).
2. Check exact match against `KnownPaths()`.
3. Check if any known path ends with `/<this_path>` (suffix match for relative references).
4. If no match found, count as unknown.

Up to 3 unknown path examples are included in the evidence string.

Example:
- LLM says: "I've updated `internal/router/balancer.go` to fix the issue."
- Fact: No tool has ever read, written, or referenced `internal/router/balancer.go`.
- Signal: `SignalUnknownPath` -- "References file paths never seen in tool outputs"

### Rule 4: Exit Code Contradictions

**Severity: 0.7**

Two sub-checks against the 3 most recent bash facts:

**4a -- Claims success when last command failed:**
- Trigger patterns: `exit 0`, `succeeded`, `completed successfully`, `no errors`, `ran successfully`
- Fires when the most recent bash command had a non-zero exit code.

**4b -- Claims failure when last command succeeded:**
- Trigger patterns: `failed`, `error occurred`, `doesn't work`, `broken`, `exit code [1-9]`
- Fires when the most recent bash command exited with code 0.
- Suppressed if test-pass patterns also match (to avoid false positives on messages discussing both pass and fail scenarios).

Example:
- LLM says: "The command ran successfully with no errors."
- Fact: Last bash command `go vet ./...` exited with code 1.
- Signal: `SignalContradiction` -- "Claims success but most recent command had non-zero exit code"

## Confidence Calculation and Threshold

Confidence is the sum of all signal severities, capped at 1.0:

```
confidence = min(1.0, sum(signal.Severity for each signal))
```

The detection threshold is **0.7** (`hallucinationThreshold`). Detection fires (`Detected = true`) when `confidence >= 0.7`.

This means:
- A single test/build contradiction (severity 0.9) crosses the threshold alone.
- A single exit code contradiction (severity 0.7) crosses the threshold alone.
- An unknown path signal (severity 0.5) alone does NOT cross the threshold.
- An unknown path (0.5) + an exit code contradiction (0.7) = 1.0, which crosses.
- Two unknown path signals are not possible (the rule emits at most one signal per check).

## False Positive Mitigations

### Hedged Language Detection

Rules 1, 2, and 4 are suppressed when the LLM's response is "heavily hedged." The system counts hedge words and compares to total word count:

```
hedgeRatio = hedgeWordCount / totalWordCount
heavilyHedged = hedgeRatio > 0.1  (more than 10% hedge words)
```

Recognized hedge words: `likely`, `probably`, `should`, `might`, `could`, `possibly`, `may`, `appears to`, `seems to`.

When heavily hedged, only Rule 3 (unknown paths) still runs, since path existence is a factual check regardless of hedging.

### Common Paths Allowlist

Rule 3 skips these common file names that frequently appear in system prompts and general discussion without being tool outputs:

`go.mod`, `go.sum`, `Makefile`, `README.md`, `CLAUDE.md`, `package.json`, `Cargo.toml`, `requirements.txt`, `main.go`, `main.py`, `index.js`, `index.ts`

### Suffix Matching

Before flagging a path as unknown, the checker tests whether any known path ends with `/<path>`. This allows the LLM to reference files by relative name (e.g., `agent.go`) when the full path (`internal/agent/agent.go`) is known.

### Minimum Tool Call Count

Hallucination checking is skipped entirely when `toolCallCount <= 3`. Early in a session there are not enough accumulated facts for meaningful cross-referencing, and false positives would be likely.

### Text-Only Responses Only

Checking only runs on responses where `msg.Content != ""` and `len(msg.ToolCalls) == 0`. Tool-call-only messages have no natural language claims to hallucinate about.

## AutoRecall Flow

When hallucination is detected, `autoRecall` builds a corrective message that is injected into the conversation as a `user` message, causing the agent loop to `continue` and re-run the LLM.

### Corrective Message Structure

```
CORRECTION: Your previous response contains claims that contradict actual tool outputs.
Please re-assess.

- FALSE CLAIM: Claims tests pass but most recent test run failed
  ACTUAL: Last test: exit code non-zero, failed
  ACTUAL OUTPUT:
  ```
  <first 512 bytes>
  ...
  <last 512 bytes>
  ```

- UNKNOWN PATH: References file paths never seen in tool outputs
  internal/router/balancer.go
  KNOWN PATHS: internal/agent/agent.go, internal/router/router.go, ...

Please provide a corrected response based on the actual data above.
```

### Evidence Retrieval

For `SignalFalseSuccess` signals, `autoRecall` attempts to retrieve the full tool output from the [[Tool Store]] using the `OutputID` stored in the `TestFact`. The output is truncated to 1024 bytes (first 512 + last 512) via `truncateForCorrection`.

For `SignalUnknownPath` signals, up to 10 known paths are listed so the LLM can self-correct to valid file references.

### Rate Limiting

A counter `hallucinationRecallCount` tracks consecutive corrections. After **3 consecutive corrections** (`maxHallucinationRecalls`), `autoRecall` returns an empty string and the hallucinated response passes through uncorrected. This prevents infinite correction loops where the LLM keeps hallucinating despite corrections.

The counter resets to 0 whenever the LLM makes tool calls (indicating it has moved on to doing actual work rather than making text claims).

### Secret Scrubbing

Before the corrective message is returned, it passes through `scrubSecrets()` (defined in `tool_summarize.go`). This prevents credentials or API keys from tool outputs being echoed back into the conversation via corrective messages.

### Size Cap

The entire corrective message is capped at **4KB** (`maxCorrectiveMessageSize = 4 * 1024`). If the message exceeds this, it is truncated with a `...(truncated)` suffix.

## Integration in the Agent Loop

The hallucination detection system is wired into the agent loop in `agent.go` at three points:

### 1. Initialization (`Agent.Run`)

```go
a.factTracker = NewFactTracker()
```

A fresh `FactTracker` is created at the start of every `Run()` call.

### 2. Fact Recording (`executeToolCalls`)

After each tool call completes and its result is computed, the fact tracker records the output:

```go
if a.factTracker != nil {
    a.factTracker.RecordToolOutput(name, args, resultContent, exitCode, storedOutputID)
}
```

This happens for every tool call, using the full (unsummarized) `resultContent` and the actual `exitCode`.

### 3. Hallucination Check (`loop`)

After the LLM produces a text-only response (no tool calls), and after at least 3 tool calls have been made:

```go
if msg.Content != "" && len(msg.ToolCalls) == 0 && a.factTracker != nil && a.toolCallCount > 3 {
    checkResult := CheckForHallucinations(msg.Content, a.factTracker)
    if checkResult.Detected {
        corrective := a.autoRecall(checkResult)
        if corrective != "" {
            // Log and inject corrective message
            a.conversation.Add(providers.Message{
                Role:    "user",
                Content: corrective,
            })
            continue // re-run LLM with correction
        }
    }
}
```

If the LLM then makes tool calls on a subsequent turn, the recall counter resets:

```go
if len(msg.ToolCalls) > 0 {
    a.hallucinationRecallCount = 0
}
```

## Signal Types

| Signal Type | Value | Meaning |
|---|---|---|
| `SignalFalseSuccess` | 0 | Claims success when tool showed failure |
| `SignalUnknownPath` | 1 | References file never seen in tool outputs |
| `SignalContradiction` | 2 | Claim contradicts tool output |
| `SignalFabricatedData` | 3 | Quotes output that does not match stored (defined but not yet used by any rule) |

## Source Files

- `internal/agent/fact_tracker.go` -- `FactProvider` interface, `FactTracker`, `TestFact`, `BashFact`
- `internal/agent/hallucination.go` -- `CheckForHallucinations`, 4 detection rules, signal types
- `internal/agent/auto_recall.go` -- `autoRecall`, corrective message builder, rate limiting
- `internal/agent/agent.go` -- Integration points in `Run()`, `loop()`, `executeToolCalls()`
