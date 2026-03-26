---
title: Requirements & Roadmap
project: synapserouter
tags: [requirements, roadmap, epics, stories]
created: 2026-03-26
updated: 2026-03-26
status: active
---

# Requirements & Roadmap

All epics, stories, and acceptance criteria for the synapserouter project. For implementation patterns, see [[Dev Guide]].

**Legend:** DONE = shipped and verified | IN PROGRESS = partially complete | PLANNED = not started

---

## Project Status

Synapserouter is an active, early-stage project. The core router and agent are functional, but most of the roadmap is still ahead. This document covers the full vision -- what works today, what's next, and what's further out.

**Current Phase: Phase 1 -- Core Router + Agent**

### Epic Progress

| Epic | Name | Status | Progress | Phase |
|---|---|---|---|---|
| 0 | Foundation | IN PROGRESS | 1 of 7 stories done | Phase 1 |
| 1 | Pipeline Architecture | PLANNED | 0 of 3 stories done | Phase 1 |
| 2 | Code Quality | PLANNED | 0 of 2 stories done | Phase 1 |
| 3 | Skill System | PLANNED | 0 of 2 stories done | Phase 1 |
| 4 | DevOps | PLANNED | 0 of 2 stories done | Phase 2 |
| 5 | Continuous Improvement | PLANNED | 0 of 1 stories done | Phase 2 |
| 6 | Future Infrastructure | PLANNED | 0 of 9 stories done | Phase 3 |
| 7 | Chat Backend API | PLANNED | 0 of 3 stories done | Phase 3 |
| 8 | Rich Content | PLANNED | 0 of 2 stories done | Future |
| 9 | CLI Terminal UI | PLANNED | 0 of 3 stories done | Future |
| 10 | Scale | PLANNED | 0 of 2 stories done | Future |

**Overall: ~1 of 36 stories complete. Most work is ahead.**

### What Works Today

- 7-level Ollama Cloud provider chain with 19+ models and automatic escalation
- Circuit breakers with rate-limit-aware cooldowns
- Interactive agent REPL with tool execution (bash, file I/O, grep, glob, git)
- Skill system with 38+ embedded skills parsed from YAML frontmatter
- Worktree isolation for safe code changes
- MCP server mode for tool exposure over HTTP
- Recall tool and conversation compaction (basic, with known bugs)
- Two profiles: personal (Ollama Cloud + subscriptions) and work (Vertex AI)
- Eval framework with 11 benchmark sources and 4 eval modes

### What's Next (Phase 1 -- Active)

Phase 1 focuses on fixing known bugs and stabilizing the foundation before building new features:

- **Wave 1 (now):** Bug fixes (memory pollution, data loss, secret scrubbing, trigger quality)
- **Wave 2:** Recall scoping for sub-agents, compaction improvements, skill budget, thread safety
- **Wave 3:** Review pipeline, cycle detection, skill verification, DevOps basics
- **Wave 4:** Test generation, test coverage, documentation cleanup
- **Wave 5:** Full test suite and merge

### What's Further Out

- **Phase 2:** DevOps pipeline, CI/CD, self-improving agent patterns
- **Phase 3:** Real embedding model, MCP client management, chat backend API, smart routing

---

## Execution Waves

Stories are grouped into waves based on dependencies. With [[Dev Guide#Adding a Pipeline Phase|worktree isolation]], waves can run in parallel.

| Wave | Stories | Focus |
|---|---|---|
| Wave 1 | 0.1, 0.2, 0.3, 0.4, 3.2 | Bug fixes, secret scrubbing, skill install |
| Wave 2 | 0.5, 0.6, 0.7, 2.2 | Recall scoping, compaction, skill budget, thread safety |
| Wave 3 | 1.1, 1.2, 3.1, 4.1 | Review pipeline, cycle detection, skill verify, DevOps |
| Wave 4 | 1.3, 2.1, 4.2, 5.1 | Test generation, test coverage, docs, self-improvement |
| Wave 5 | Full test suite + merge | Final integration |

---

## Epic 0: Foundation

> MVP: Stories 0.2, 0.3, 0.4 (non-negotiable). Defer: 0.1, 0.5, 0.6, 0.7.

### Story 0.1: Install missing skills from community catalogs

**Status:** PLANNED

**As a** developer improving synapserouter,
**I want** all relevant community skills installed and available,
**So that** the agent has access to quality gates, TDD enforcement, and self-improvement patterns.

**Acceptance Criteria:**
- [ ] Superpowers framework installed (TDD, brainstorming, plans, subagent dev)
- [ ] levnikolaevich pipeline installed (quality gates, task reviewer, docs auditor)
- [ ] alirezarezvani engineering skills installed (senior-qa, self-improving-agent, senior-devops, senior-secops, senior-architect, ci-cd-pipeline-builder, database-designer, agenthub, codebase-onboarding)
- [ ] wshobson conductor installed
- [ ] All skills appear in `npx skillfish list`
- [ ] `go build` still compiles (skills don't break embedded skilldata)

**Dependencies:** None
**Effort:** Small

---

### Story 0.2: Fix router variable shadowing causing memory pollution

**Status:** PLANNED

**As a** user running any project through synapserouter,
**I want** memory injection to not re-store already-injected messages,
**So that** the DB doesn't fill with duplicates and semantic search stays accurate.

**Acceptance Criteria:**
- [ ] `router.go:190` inner `injectedCount` removed, outer variable used
- [ ] Unit test: store 3 messages, inject 2 from memory, verify only 3 stored (not 5)
- [ ] `go test -race ./internal/router/...` passes

**Dependencies:** None
**Effort:** Trivial
**Reference:** Bug B1

---

### Story 0.3: Fix data loss bugs in tool store and compaction

**Status:** IN PROGRESS (1 of 6 bugs fixed)

**As a** user relying on recall and conversation compaction,
**I want** no silent data loss when DB operations fail,
**So that** tool outputs and compacted messages are reliably retrievable.

**Acceptance Criteria:**
- [ ] `tool_store.go:111` -- `rows.Err()` checked after iteration
- [ ] `tool_store.go:25` -- CREATE TABLE failure logged and returns nil store
- [ ] `agent.go compactConversation` -- VectorMemory.Store error checked, compaction skipped on failure
- [ ] `recall.go:86` -- truncation reports original size, not post-truncation size
- [ ] `recall.go:78` -- dead int64 case removed
- [x] `circuit.go Open()/SetState()` -- INSERT OR IGNORE guard added *(done in commit ad0b841)*
- [ ] Remaining 5 bugs have regression tests
- [ ] `go test -race ./...` passes

**Dependencies:** None
**Effort:** Small
**Reference:** Bugs B2-B7

---

### Story 0.4: Scrub secrets from tool output storage

**Status:** PLANNED

**As a** user whose bash commands may contain API keys,
**I want** secrets redacted before storing to the DB,
**So that** the SQLite database doesn't become a credential dump.

**Acceptance Criteria:**
- [ ] `FormatArgsSummary` scrubs patterns: `Bearer `, `token=`, `password`, `api_key=`, `secret=`
- [ ] `ToolOutputStore.Store` scrubs `fullOutput` for the same patterns
- [ ] Reuses patterns from existing `SecretPatternGuardrail` in `guardrails.go`
- [ ] Test: `FormatArgsSummary("bash", {"command": "curl -H 'Authorization: Bearer sk-abc123'"})` returns redacted output
- [ ] Existing tool outputs in DB are not retroactively scrubbed (acceptable)

**Dependencies:** None
**Effort:** Medium
**Reference:** Issue H1

---

### Story 0.5: Fix recall tool session scoping for sub-agents

**Status:** PLANNED

**As a** code reviewer sub-agent,
**I want** to recall tool outputs from the parent agent's session,
**So that** I can access full file contents that were compacted from conversation.

**Acceptance Criteria:**
- [ ] Recall tool accepts optional `parent_session` parameter
- [ ] When called from a sub-agent, searches parent's session by default
- [ ] Parent session ID passed to child via `SpawnChild` config
- [ ] Test: parent stores tool output, child recalls it by query

**Dependencies:** Story 0.3
**Effort:** Medium
**Reference:** Issue H2

---

### Story 0.6: Improve compaction summary with key context

**Status:** PLANNED

**As an** agent entering a new pipeline phase,
**I want** the compaction summary to include what was built and what passed,
**So that** I have enough context to continue without recalling everything.

**Acceptance Criteria:**
- [ ] Summary includes: files created/modified (from glob), acceptance criteria, phase result
- [ ] Summary is 500-1000 chars (not just 1 line)
- [ ] Test: compact 50 messages -> summary contains file list + criteria reference

**Dependencies:** None
**Effort:** Small
**Reference:** Issue H3

---

### Story 0.7: Add token budget to skill injection

**Status:** PLANNED

**As a** developer scaling to 100+ skills,
**I want** skill context injection capped at 8K tokens,
**So that** the system prompt doesn't consume half the context window.

**Acceptance Criteria:**
- [ ] `computeSkillContext()` enforces 8K token budget
- [ ] Skills ordered by relevance (trigger match count), highest first
- [ ] When budget exhausted, remaining skills get description only (not full instructions)
- [ ] Test: 20 matched skills -> total injection <= 8K tokens

**Dependencies:** None
**Effort:** Medium
**Reference:** Issue H4

---

## Epic 1: Pipeline Architecture

> MVP: Story 1.2 (stops 8-cycle spinning). Defer: 1.1, 1.3.

### Story 1.1: Review mode with 4 phases

**Status:** PLANNED

**As a** user running synapserouter on existing code,
**I want** the agent to analyze and report findings before making changes,
**So that** I can approve the plan before code is modified.

**Acceptance Criteria:**
- [ ] Existing project detected -> 4-phase pipeline: analyze -> report -> plan+approve -> implement+verify
- [ ] Analyze phase: read all source files, run tests, check against spec
- [ ] Report phase: output structured findings (bugs, gaps, improvements)
- [ ] Approve phase: wait for user input (or `--auto-approve` for batch)
- [ ] Implement phase: make approved changes only
- [ ] Self-review test: synapserouter reviews itself -> reports findings -> waits for approval

**Dependencies:** Story 0.1
**Effort:** Large
**Reference:** Pipeline gap P1, P2

---

### Story 1.2: Review cycle "no improvement" detection

**Status:** PLANNED

**As a** user waiting for a project to complete,
**I want** the review cycle to stop when no improvement is being made,
**So that** the agent doesn't spin for 8 cycles on the same unchanged code.

**Acceptance Criteria:**
- [ ] Track LOC diff between review cycles
- [ ] If LOC unchanged for 2 consecutive cycles -> accept and advance
- [ ] If same issues reported 2 cycles -> accept and advance
- [ ] Log: `[Agent] review cycle stable -- no changes in 2 cycles, accepting`

**Dependencies:** None
**Effort:** Medium
**Reference:** Pipeline gap P3

---

### Story 1.3: Functional test generation from acceptance criteria

**Status:** PLANNED

**As a** user with a spec containing acceptance criteria,
**I want** the verification gate to test actual functionality,
**So that** "server starts and serves files" is verified, not just "code compiles."

**Acceptance Criteria:**
- [ ] Parse acceptance criteria from spec or plan phase output
- [ ] Generate shell commands that verify each criterion
- [ ] Run generated tests in verification gate alongside build/test
- [ ] Test: caddy spec criterion "server starts on configured port" -> generates `curl localhost:8080` check

**Dependencies:** Story 0.1
**Effort:** Large
**Reference:** Pipeline gap P4, P5

---

## Epic 2: Code Quality

> MVP: Story 2.2 (fixes race condition crashes). Defer: 2.1.

### Story 2.1: Test coverage for critical paths

**Status:** PLANNED

**As a** developer maintaining synapserouter,
**I want** the 9 untested critical functions to have test coverage,
**So that** regressions are caught before they reach users.

**Acceptance Criteria:**
- [ ] `advancePipeline` -- table-driven tests for phase transitions, gate failures, escalation
- [ ] `callLLMWithRetry` -- tests for retry, escalation, context overflow
- [ ] `compactConversation` -- tests for threshold, DB storage, summary
- [ ] `RecallTool.Execute` -- tests for search, retrieve, nil searcher, empty results
- [ ] `runSubAgentPhase` / `runParallelPhase` -- integration tests with mock providers
- [ ] `isContextOverflowError` / `isRetryableError` -- table-driven string matching tests
- [ ] `TrimOldest` -- edge cases (n > len, orphan cleanup)
- [ ] Total: 20+ new tests

**Dependencies:** None
**Effort:** Large

---

### Story 2.2: Fix context propagation and thread safety

**Status:** PLANNED

**As a** developer running concurrent sub-agents,
**I want** proper context propagation and mutex protection,
**So that** cancelled operations actually stop and shared state doesn't race.

**Acceptance Criteria:**
- [ ] `invokeMCPToolsForSkills`, `runSubAgentPhase`, `runParallelPhase` accept parent context
- [ ] `providerIdx` protected by mutex for goroutine access
- [ ] TF-IDF embedding input capped at 32KB
- [ ] `go test -race ./...` passes with zero race warnings

**Dependencies:** None
**Effort:** Medium

---

## Epic 3: Skill System

> MVP: Story 3.2 (reduces false-positive matching). Defer: 3.1.

### Story 3.1: Add verify commands to untestable skills

**Status:** PLANNED

**As a** developer relying on the verification gate,
**I want** all skills to have at least one verify command,
**So that** the gate can enforce quality regardless of which skills match.

**Acceptance Criteria:**
- [ ] 29 skills currently without verify commands each get at least 1
- [ ] Verify commands are language-appropriate
- [ ] Total verify commands across all skills >= 60

**Dependencies:** Story 0.1
**Effort:** Large

---

### Story 3.2: Fix trigger quality

**Status:** PLANNED

**As a** user whose message shouldn't match irrelevant skills,
**I want** triggers to be specific enough to avoid false positives,
**So that** the system prompt isn't bloated with unrelated skill instructions.

**Acceptance Criteria:**
- [ ] `code-implement` -- remove "fix", "add", "create", "update" (too generic)
- [ ] `go-patterns` -- change "go" trigger to require co-occurrence with code terms
- [ ] Merge: research + deep-research + search-first -> 1 skill with modes
- [ ] Merge: jira-manage + jira-project-config -> 1 skill
- [ ] Test: "fix the Go auth handler" matches go-patterns + code-review, NOT code-implement + python-testing

**Dependencies:** None
**Effort:** Medium

---

## Epic 4: DevOps

> MVP: Story 4.1 (Go upgrade + mux replacement). Defer: 4.2.

### Story 4.1: Production-ready build pipeline

**Status:** PLANNED

**As a** developer deploying synapserouter,
**I want** a Dockerfile, CI pipeline, and release process,
**So that** the project can be built, tested, and released automatically.

**Risk Assessment:**
- Go upgrade (1.21 -> 1.23): LOW RISK -- 3 direct deps, all compatible
- Replace gorilla/mux: MEDIUM RISK -- 7 files, 47 `mux.Vars()` calls, 106 `.Methods()` calls. Recommend stdlib `net/http` (Go 1.22+)

**Acceptance Criteria:**
- [ ] Go updated from 1.21 to 1.23
- [ ] gorilla/mux replaced with stdlib net/http (Go 1.22+ routing)
- [ ] Dockerfile (multi-stage, CGo for go-sqlite3)
- [ ] `.github/workflows/ci.yml` -- vet, test -race, build on push/PR
- [ ] Makefile `test` target uses `-race`
- [ ] `make docker` builds container image
- [ ] Version set via ldflags in CI

**Dependencies:** None
**Effort:** Large

---

### Story 4.2: Documentation cleanup

**Status:** PLANNED

**As a** new contributor reading the project docs,
**I want** accurate, non-contradictory documentation,
**So that** I understand the current architecture without confusion.

**Acceptance Criteria:**
- [ ] README.md rewritten for current architecture (Ollama Cloud primary)
- [ ] NanoGPT removed from all 10 markdown files + 9 test files
- [ ] ARCHITECTURE.md either deleted or updated to match CLAUDE.md
- [ ] Historical docs archived or deleted
- [ ] Stale branches deleted, worktree dirs cleaned
- [ ] Uncommitted files (chunk.ts, main.go.bak) removed

**Dependencies:** None
**Effort:** Medium

---

## Epic 5: Continuous Improvement

> MVP: None -- defer entirely until Epics 0-2 are solid.

### Story 5.1: Self-improving agent pattern

**Status:** PLANNED

**As a** system that runs projects repeatedly,
**I want** to learn from failures and successes,
**So that** each run is better than the last.

**Acceptance Criteria:**
- [ ] synroute.md captures: failed verification results, error messages, providers used, time per phase
- [ ] On re-run, agent reads previous failure diagnostics and avoids same mistakes
- [ ] Self-improving-agent skill pattern: discover -> memory -> review -> promote to CLAUDE.md
- [ ] Test: run project, fail on build, re-run -> agent references previous error in plan

**Dependencies:** Story 0.1, Story 0.6
**Effort:** Large

---

## Epic 6: Future -- Infrastructure

### Story 6.1: Real embedding model (ONNX bundled)

**Status:** PLANNED

**As a** user relying on recall and memory search,
**I want** a real embedding model (all-MiniLM-L6-v2) bundled in the binary,
**So that** semantic search returns relevant results instead of TF-IDF hash approximations.

**Acceptance Criteria:**
- [ ] all-MiniLM-L6-v2 ONNX model (~80MB) embedded or downloaded with checksum
- [ ] `ONNXEmbedding` struct implements `EmbeddingProvider` interface
- [ ] Uses `onnxruntime-go` bindings (pure CGo, no Python)
- [ ] Dimensions: 384 (matching current LocalHashEmbedding)
- [ ] Embedding latency < 50ms for typical summaries
- [ ] Batch embedding support
- [ ] Graceful fallback to LocalHashEmbedding if ONNX unavailable
- [ ] Build tag `onnx` controls inclusion
- [ ] Cosine similarity test: "fix authentication" vs "repair login" > 0.7

**Dependencies:** None
**Effort:** Large

---

### Story 6.2: MCP client runtime management

**Status:** PLANNED

Synapserouter discovers, installs, configures, and manages MCP server lifecycles. Auto-start servers when skills reference their tools, auto-stop after 5min idle.

**Key Criteria:**
- [ ] `internal/mcpclient/` package with Connect, ListTools, CallTool
- [ ] MCP server registry: `~/.synroute/mcp-servers.json`
- [ ] CLI: `synroute mcp add|list|remove`
- [ ] Auto-start/stop with health checks and restart (max 3)
- [ ] Tool namespace: `github.create_issue` vs `bash`
- [ ] `invokeMCPToolsForSkills` wired to MCP client (currently no-op)

**Dependencies:** None
**Effort:** Large

---

### Story 6.3: Freeform input to spec generation

**Status:** PLANNED

Detect freeform input and generate structured spec with acceptance criteria before entering the pipeline. Clarification mode for vague requests.

**Dependencies:** Story 1.1
**Effort:** Medium

---

### Story 6.4: Provider backup strategies

**Status:** PLANNED

Health scoring (0.0-1.0) per provider, health-weighted routing, level-skip for degraded providers, warm standby probing, latency tracking, error classification.

**Dependencies:** None (enhances existing router)
**Effort:** Large

---

### Story 6.5: Bash tool sandboxing

**Status:** PLANNED

Filesystem restrictions (allowlist writable dirs), denied paths (~/.ssh, ~/.aws), command denylist, resource limits (CPU/memory/filesize), macOS sandbox-exec / Linux bwrap.

**Dependencies:** None
**Effort:** Large

---

### Story 6.6: Wave-based parallel story execution

**Status:** PLANNED

`synroute wave --stories stories.md --concurrency 4` -- parse dependency graph, spawn agents in worktrees per wave, merge back, advance to next wave.

**Dependencies:** Epics 0-1 complete, worktree system working
**Effort:** Large

---

### Story 6.7: Persistent task queue

**Status:** PLANNED

`synroute queue add|status|worker|retry` -- SQLite-backed job queue with priority ordering, per-job timeout, multiple concurrent workers.

**Dependencies:** Story 6.6
**Effort:** Large

---

### Story 6.8: Batch project execution

**Status:** PLANNED

`synroute batch --specs benchmarks/reconstruction/ --concurrency 3 --report` -- scan for spec.md files, create queue jobs, generate summary table.

**Dependencies:** Story 6.7
**Effort:** Medium

---

### Story 6.9: Full integration test (LibreChat backend)

**Status:** PLANNED

Synapserouter as LibreChat backend with smart model selection, skill injection, memory, concurrency reservation, streaming.

**Dependencies:** Provider backup strategies (Story 6.4)
**Effort:** Medium

---

## Epic 7: Chat Backend API

> MVP: Stories 7.1, 7.2. Defer: 7.3.

### Story 7.1: Stateful chat sessions via API

**Status:** PLANNED

**As a** chat client user,
**I want** synapserouter to maintain conversation state across requests,
**So that** I get multi-turn chat with memory, not stateless proxy calls.

**Acceptance Criteria:**
- [ ] `/v1/chat/completions` accepts `session_id` header or generates one
- [ ] Conversation history persisted in VectorMemory per session
- [ ] Previous messages retrieved and injected
- [ ] Skill preprocessing fires on every message
- [ ] Streaming works end-to-end
- [ ] Session timeout: configurable idle expiry

**Dependencies:** None
**Effort:** Medium

---

### Story 7.2: Smart model selection for chat

**Status:** PLANNED

Query complexity scoring -> simple queries use L0-L1 (fast), complex queries use L3-L4 (thorough), code generation routes to code-specialized models.

**Dependencies:** Story 7.1
**Effort:** Medium

---

### Story 7.3: N-1 concurrency reservation

**Status:** PLANNED

When agent runs, reserve N-1 model slots for agent, keep 1 for chat. Queue chat requests to reserved models rather than rejecting.

**Dependencies:** Story 7.1
**Effort:** Medium

---

## Epic 8: Rich Content (Documents & Files)

> MVP: None -- defer until Epic 7 and MCP client (6.2) are built.

### Story 8.1: Document creation (presentations, docs, spreadsheets)

**Status:** PLANNED

Chat-driven creation of PPTX, DOCX, XLSX, and diagrams via document-mcp. AI-generated images, Google Drive upload, multi-file output.

**Dependencies:** Epic 7 (Story 7.1), MCP client (Story 6.2)
**Effort:** Large

---

### Story 8.2: File uploads and processing

**Status:** PLANNED

`/v1/files/upload` endpoint for PDF, DOCX, XLSX, CSV, images, code files. Text extraction, summarization, format conversion.

**Dependencies:** Story 8.1
**Effort:** Medium

---

## Epic 9: CLI Terminal UI

> MVP: None -- defer until Epic 7 API is stable.

### Story 9.1: Chat mode -- conversational terminal interface

**Status:** PLANNED

Clean TUI with status bar, keyboard shortcuts (^H history, ^S sessions, ^N new, ^F files), no TUI framework (raw ANSI + Go stdlib).

**Dependencies:** Epic 7 (Story 7.1)
**Effort:** Large

---

### Story 9.2: Code mode -- pipeline-aware agent interface

**Status:** PLANNED

Dedicated code mode showing pipeline phase, tool calls, ^P pipeline status, ^R recall, ^E escalate. No shared conversation with chat mode.

**Dependencies:** Story 9.1
**Effort:** Medium

---

### Story 9.3: Session management and mode switching

**Status:** PLANNED

Inline session browser (^S), sessions tagged chat/code, ^M mode switch, auto-naming, CLI commands for list/search/archive.

**Dependencies:** Stories 9.1, 9.2
**Effort:** Medium

---

## Epic 10: Scale

> MVP: None -- defer until multi-user deployment.

### Story 10.1: Postgres migration

**Status:** PLANNED

Build tag `postgres` for `pgx` driver. Schema migration for all 8 migration files. Connection pooling. Data migration tool: `synroute migrate --from sqlite --to postgres`.

**Dependencies:** Epic 7
**Effort:** Large

---

### Story 10.2: Branding + polish

**Status:** PLANNED

ASCII art banner, version info (git commit, build date, Go version, profile, embedding provider), color theme with `NO_COLOR` support, spinners, progress indicators, timing, provider attribution, token usage display.

**Dependencies:** None
**Effort:** Medium

---

## Session Progress (2026-03-24)

### What Was Implemented

The following were completed across 15 commits in the A-phase session:

| Commit | Change |
|---|---|
| A1 | ProjectLanguage wired end-to-end (spec detection -> pipeline -> verify -> sub-agents) |
| A2 | Dynamic pipeline detection from skill metadata (no hardcoded lists) |
| A3 | Loop detection via intent-based fingerprints + warning counter |
| A4 | Memory corruption fixed (tool call summaries on store, filter empty on retrieve) |
| A5-A10 | 7 diagnostic bugs fixed (sprintf panic, DefaultSkills caching, global pipeline race, etc.) |
| A9 | Recall tool + conversation compaction + agent-DB bridge |
| A10 | CLAUDE.md cleanup -- removed NanoGPT, updated architecture |
| ad0b841 | Circuit breaker fix for dynamic providers + integration tests |
| f803f95 | Tool store integration tests (6 tests including auto-create table) |
| f1ff39e | Fix tool_outputs table not created in CLI mode |

### Full Audit Results

75 issues found across 9 skill audits + 3 deep reviews + self-review:
- **Priority 1:** 7 confirmed bugs (B1-B7)
- **Priority 2:** 7 high-impact design fixes (H1-H7)
- **Priority 3:** 8 pipeline architecture gaps (P1-P8)
- **Priority 4-5:** Code quality + test coverage gaps
- **Priority 6:** Skill system overhaul (71% untestable -- no verify commands)
- **Priority 7:** DevOps (no CI, no Dockerfile, Go EOL, mux archived)
- **Priority 8:** Continuous improvement (feedback loops)

### What Remains

All stories in this document except the session progress items above are still PLANNED or IN PROGRESS. The next wave of work should start with **Wave 1** (Stories 0.2, 0.3, 0.4, 3.2) -- all independent, no dependencies between them.
