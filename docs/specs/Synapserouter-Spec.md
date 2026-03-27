# Synapserouter Product Specification

**Version:** 1.0
**Date:** 2026-03-26
**Status:** Living document -- updated as features ship

---

## Table of Contents

1. [Product Overview](#1-product-overview)
2. [Architecture](#2-architecture)
3. [Agent Capabilities](#3-agent-capabilities)
4. [CLI Interface](#4-cli-interface)
5. [HTTP API](#5-http-api)
6. [Eval Framework](#6-eval-framework)
7. [Container Isolation](#7-container-isolation-planned)
8. [Configuration](#8-configuration)
9. [Acceptance Criteria](#9-acceptance-criteria)

---

## 1. Product Overview

### What It Is

Synapserouter (binary name: `synroute`) is a Go-based LLM proxy router and autonomous coding agent. It operates in two modes:

1. **HTTP Proxy Router** -- an OpenAI-compatible API server that distributes LLM requests across multiple providers using cost-optimized escalation. Clients (LibreChat, curl, any OpenAI-compatible SDK) send requests to a single endpoint; the router selects the cheapest adequate provider and falls back through a 7-level chain on failure.

2. **Interactive Coding Agent** -- a CLI tool (`synroute chat`) that plans, implements, tests, and reviews code using built-in tools (bash, file I/O, grep, glob, git). It builds programs and tools -- it does not do the work itself.

### Who It Is For

- **Individual developers** who want cost-optimized access to 19+ LLM models through a single API, with automatic fallback when any provider is rate-limited or down.
- **Coding agent users** who want an autonomous agent that can take a spec, plan the work, write the code, run the tests, review the output, and iterate -- all driven by the provider chain.
- **Teams** (future) who need multi-user sessions with container isolation and shared project continuity.

### Why It Exists

1. **No vendor lock-in.** Route requests across Ollama Cloud, Gemini, Codex, Claude Code, and Vertex AI through a single API. Swap providers without changing client code.
2. **Cost-optimized model selection.** A 14B model handles most tasks. A 671B model handles the hard ones. The router figures out which is which -- you do not pay for frontier models when a small one suffices.
3. **Autonomous coding agent.** Give it a task. It plans, writes code, runs tests, reviews its own work, and iterates. The agent builds programs -- it does not just describe what it would do.
4. **Unlimited context.** Conversation history and tool outputs persist to SQLite. Long sessions do not lose early context. A recall tool retrieves relevant history on demand.

### Two Profiles

| Profile | Primary Provider | Fallback | Auth | Use Case |
|---------|-----------------|----------|------|----------|
| `personal` | Ollama Cloud (7-level, 19+ models) | Gemini, Codex, Claude Code (OAuth subscriptions) | API keys + OAuth | Personal development, home lab |
| `work` | Vertex AI (Claude + Gemini) | None | GCP Application Default Credentials | Enterprise, managed GCP environments |

Controlled by `ACTIVE_PROFILE` in `.env`. Switch at runtime via `synroute profile switch work`.

### Design Principles

- **The router IS the escalation.** No parallel escalation systems. The agent sends `model: "auto"` and the router picks.
- **Tool builder, not tool runner.** The agent builds programs, scripts, and tools. The deliverable is always a runnable program, not a series of manual operations.
- **Plan before execute.** Any new feature or non-trivial change is planned first. No jumping to implementation.
- **Pipeline is the only control flow.** Phase transitions, provider escalation, and quality gates are all handled by the pipeline. No bolted-on mechanisms.
- **Skills are self-contained.** Adding a skill means dropping one `.md` file and rebuilding. No Go code changes.
- **One system per responsibility.** No overlapping mechanisms. The agent loop is: call LLM, execute tools, check pipeline, repeat.

---

## 2. Architecture

### System Diagram

```
                        +-----------------+
                        |   User / CLI    |
                        |  synroute chat  |
                        +--------+--------+
                                 |
                    +------------+------------+
                    |                         |
             +------+------+          +------+------+
             | Agent Loop  |          |  HTTP API   |
             | tools, plan |          | /v1/chat/*  |
             | pipeline    |          | /v1/models  |
             +------+------+          +------+------+
                    |                         |
                    +------------+------------+
                                 |
                        +--------+--------+
                        |     Router      |
                        | model selection |
                        | circuit breaker |
                        | health cache    |
                        +--------+--------+
                                 |
            +--------------------+--------------------+
            |                    |                    |
    +-------+-------+   +-------+-------+   +--------+-------+
    | Ollama Cloud  |   | Subscriptions |   |   Vertex AI    |
    | 7 levels      |   | gemini, codex |   | (work profile) |
    | 19+ models    |   | claude-code   |   | Claude+Gemini  |
    +---------------+   +---------------+   +----------------+
```

### 2.1 Provider Chain

#### 7-Level Ollama Cloud Escalation (Personal Profile)

The router sends each request to the cheapest provider level first. If that level fails (model too small, rate-limited, circuit breaker open), it escalates to the next level automatically. No user intervention required.

| Level | Models | Class | Typical Context |
|-------|--------|-------|-----------------|
| L0 | `ministral-3:14b`, `rnj-1:8b`, `nemotron-3-nano:30b` | Fast/cheap | Simple completions, short answers |
| L1 | `gpt-oss:20b`, `devstral-small-2:24b`, `qwen3.5` | Small coders | Basic code generation, edits |
| L2 | `nemotron-3-super`, `gpt-oss:120b`, `minimax-m2.7` | Medium | Multi-file code, moderate reasoning |
| L3 | `devstral-2:123b`, `qwen3-coder:480b` | Large coders | Complex code, architecture |
| L4 | `qwen3.5:397b`, `kimi-k2.5`, `minimax-m2.5` | XL general | Long-form, multi-step reasoning |
| L5 | `deepseek-v3.1:671b`, `cogito-2.1:671b` | XXL reasoning | Deep analysis, hard problems |
| L6 | `glm-5`, `kimi-k2-thinking`, `glm-4.7` | Frontier | Most capable, last resort before subscriptions |

After all Ollama Cloud levels are exhausted:
```
L0-L6 (Ollama Cloud) --> gemini --> codex --> claude-code
```

Subscription providers can be disabled entirely with `SUBSCRIPTIONS_DISABLED=true`.

#### Planner Slots

Two optional planner slots for planning-phase tasks: `OLLAMA_PLANNER_1`, `OLLAMA_PLANNER_2`. Registered as `ollama-planner-1` and `ollama-planner-2`.

#### Multiple API Keys

```env
OLLAMA_API_KEYS=key1,key2,key3
```

Keys distributed across providers in round-robin fashion. Enables multiple paid subscriptions ($20/mo each) running concurrently without per-key rate limits.

#### Vertex AI (Work Profile)

| Provider | Backend | Auth | Config |
|----------|---------|------|--------|
| `vertex-claude` | Claude via Vertex rawPredict | ADC | `VERTEX_CLAUDE_PROJECT`, `VERTEX_CLAUDE_REGION` (default: `us-east5`) |
| `vertex-gemini` | Gemini via Vertex generateContent | ADC or SA key | `VERTEX_GEMINI_PROJECT`, `VERTEX_GEMINI_LOCATION` (default: `global`), `VERTEX_GEMINI_SA_KEY` |

#### Provider Interface

All providers implement:

```go
type Provider interface {
    Name() string
    ChatCompletion(ctx context.Context, req ChatRequest) (ChatResponse, error)
    IsHealthy(ctx context.Context) bool
    SupportsModel(model string) bool
}
```

### 2.2 Router Layer

#### Provider Selection Algorithm

1. **Filter candidates** -- only providers that support the requested model (or all if model is `auto`/empty)
2. **Usage-based ranking** -- select provider with lowest usage percentage via `usageTracker.GetBestProvider`
3. **Circuit breaker check** -- skip providers with open circuit breakers
4. **Health check** -- verify provider is reachable (cached with 2-minute TTL)
5. **Fallback chain** -- if primary fails, try remaining healthy providers ordered by usage

#### Request Flow

```
Client Request
  --> routeChatRequest()
  --> Router.ChatCompletionWithDebug()
      |-- Refine vague prompts using conversation context
      |-- Touch session tracker
      |-- Retrieve memory (vector search, cross-session fallback)
      |-- Inject retrieved memory into request
      |-- Preprocess: inject matched skill context
      |-- Store new messages in vector memory
      |-- Select provider (usage + circuit + health)
  --> tryProvidersWithFallback()
      |-- Try primary provider
      |-- On failure: record in circuit breaker, check rate limit
      |-- On success: reset circuit, update health cache, track usage
      |-- If primary fails: try fallback providers in usage order
  --> handleStall() (retry if response looks stalled)
  --> Store assistant response in memory
  --> Persist audit trail
```

### 2.3 Circuit Breakers

Each provider gets a circuit breaker instance backed by SQLite for persistence across restarts.

#### States

| State | Meaning |
|-------|---------|
| `closed` | Normal operation, requests flow through |
| `open` | Provider blocked, no requests until cooldown expires |
| `half_open` | Cooldown expired, next request is a probe -- success closes, failure re-opens |

#### Thresholds

- **Failure threshold:** 5 consecutive failures triggers open state
- **Default cooldown:** 5 minutes
- **Rate limit cooldowns (dynamic):**
  - Error contains `"reset after Ns"` -- uses N+5 seconds
  - `gemini` -- 30 seconds
  - `claude-code` -- 60 seconds
  - All others -- 2 minutes

#### Rate Limit Detection

Checks error messages for: `"rate limit"`, `"429"`, `"quota exceeded"`, `"too many requests"`. When detected, circuit opens with provider-specific cooldown immediately (does not wait for 5 failures).

### 2.4 Health Caching

Health checks cached for 2 minutes (`healthCacheTTL`) to avoid burning API quota.

```
Request arrives
  --> Circuit breaker open?  --> Return unhealthy (no API call)
  --> Cache valid (< 2min)?  --> Return cached result
  --> Cache stale/miss?      --> Call provider.IsHealthy(), update cache
```

Protected by read-write mutex for concurrent access.

### 2.5 Usage Tracking

Every successful request records token usage (reported or estimated at ~4 chars/token) in SQLite. The usage tracker enforces an 80% auto-switch threshold -- when a provider exceeds 80% of its daily quota, the router prefers other providers.

| Provider | Daily Limit | Monthly Limit |
|----------|-------------|---------------|
| `ollama-cloud` | 1,000,000 | 30,000,000 |
| `claude-code` | 500,000 | 15,000,000 |
| `gemini` | 500,000 | 15,000,000 |
| `codex` | 300,000 | 9,000,000 |

Quotas configurable via environment variables (e.g., `OLLAMA_CLOUD_DAILY_LIMIT`).

### 2.6 Stall Detection

If a provider takes longer than `STALL_TIMEOUT_SECONDS` (default: 180) and returns fewer than 50 characters, the router retries with a continuation prompt. Handles providers that hang or return empty responses.

### 2.7 Audit Trail

Every request generates audit records in SQLite:

- **`request_audit`** -- session, selected/final provider, model, memory stats, success/error
- **`provider_attempt_audit`** -- each provider attempt with index, success, error details

### 2.8 Memory Layer

Three subsystems provide persistent, searchable memory:

#### VectorMemory (`internal/memory/vector.go`)

SQLite-backed store for compacted conversation messages with embeddings for semantic search.

- **Table:** `memory` -- content, embedding (float32 BLOB), session_id, role, metadata
- **Hybrid search:** vector similarity (cosine distance, min 0.15 threshold) + lexical fallback (TF-IDF term frequency)
- **Always fetches 4 most recent messages** for conversational continuity
- **Token budget:** default 4096, max 8192 for semantic results

#### ToolOutputStore (`internal/agent/tool_store.go`)

Preserves full tool outputs while conversation gets summaries.

- **Table:** `tool_outputs` -- tool_name, args_summary, summary, full_output, exit_code
- **All tool outputs stored** regardless of size; secrets scrubbed before storage
- **Conversation gets summary** if output exceeds 2KB threshold, with `[full output: ref:N]` back-reference

#### UnifiedSearcher (`internal/agent/unified_recall.go`)

Merges both stores behind a single interface for the recall tool.

- **Three retrieval modes:**
  - `recall(id=47)` -- fetch full output by ID from tool_outputs
  - `recall(query="auth middleware")` -- semantic search across VectorMemory + tool_outputs
  - `recall(tool_name="bash")` -- search tool_outputs filtered by tool name
- **Cross-session:** searches current session, then all parent sessions via `ParentSessionIDs`

#### Embedding Providers

| Provider | Dimensions | When Used | Quality |
|----------|-----------|-----------|---------|
| `LocalHashEmbedding` | 384 | Default (no API key) | TF-IDF weighted feature hashing: word unigrams, char 3-grams, word bigrams. Pure Go. |
| `OpenAIEmbedding` | 1536 | When `OPENAI_API_KEY` set | `text-embedding-3-small` via API. Results cached. |
| ONNX (planned) | 384 | Build tag `onnx` | all-MiniLM-L6-v2 bundled, <50ms latency. See [Story 6.1](#story-61-real-embedding-model). |

### 2.9 Skill System

52 embedded skills parsed from YAML frontmatter in Markdown files, compiled into the binary via `go:embed`. No hardcoded Go registries.

#### Trigger Matching

Three strategies, in precedence:

1. **Compound triggers** (`+` syntax): `go+handler` requires both "go" AND "handler" in the text. Parts matched recursively.
2. **Substring matching**: for multi-word triggers and triggers with special characters (`.`, `-`, `_`, `/`).
3. **Word-boundary matching**: for short ambiguous words (2 chars: `go`, `do`, `is`, etc.) -- requires exact word match in tokenized text.

#### Skill Phases

| Phase | Order | Purpose |
|-------|-------|---------|
| `analyze` | 0 | Understand the problem, detect patterns, research |
| `implement` | 1 | Write code, build features |
| `verify` | 2 | Run tests, validate output |
| `review` | 3 | Code review, quality audit |

When multiple skills match, `BuildSkillChain()` sorts by phase order, then alphabetically within phase.

#### Complete Skill Inventory (52 Skills)

**Language Patterns (14 skills -- analyze phase):**
go-patterns, python-patterns, rust-patterns, typescript-patterns, javascript-patterns, swift-patterns, kotlin-patterns, java-patterns, csharp-patterns, sql-expert, fastapi-patterns, java-spring, ml-patterns, node-toolchain

**Testing (10 skills -- verify phase):**
go-testing, python-testing, rust-testing, typescript-testing, swift-testing, kotlin-testing, java-testing, csharp-testing (plus 2 more)

**Implementation (13 skills -- implement phase):**
code-implement, docker-expert, git-expert, github-workflows, devops-engineer, python-venv, data-scrubber, doc-coauthoring, task-orchestrator, predictive-modeler, feature-engineer, slack-integration, intervals-icu

**Project Management (3 skills -- implement phase):**
jira-manage, jira-project-config, document-mcp

**Research and Analysis (6 skills -- analyze phase):**
research, deep-research, context7, search-first, eda-explorer, prompt-engineer

**Architecture and Design (3 skills -- analyze phase):**
api-design, spec-workflow, dbt-modeler

**Database (1 skill -- analyze phase):**
snowflake-query

**Review and Quality (3 skills -- review phase):**
code-review, security-review, continuous-improvement

**Meta (1 skill -- review phase):**
skill-stocktake

**Project-Level Skills (not embedded, read at runtime):**
synroute-test, synroute-research, synroute-profile (in `.claude/skills/`)

#### Verification Commands

Skills can define executable checks via the `verify` field in frontmatter:

```yaml
verify:
  - name: "go vet passes"
    command: "go vet ./... 2>&1 || echo 'VET_FAILED'"
    expect_not: "VET_FAILED"
```

`VerifyCommandsForChain()` collects all verify commands from matched skills for the reviewer phase.

#### Adding a New Skill

1. Create `.md` file in `internal/orchestration/skilldata/`
2. Add YAML frontmatter with `name` (required), `phase` (required), `triggers`, `role`, `language`
3. Write Markdown body with instructions
4. Rebuild: `go build -o synroute .`

No Go code changes required. The `go:embed` directive picks up new files on rebuild.

#### Injection into System Prompt

Two paths inject skill context:

1. **Orchestration dispatch** (`MatchSkills`): scans triggers against goal text, injects instructions for agent loop
2. **Router preprocessor** (`buildInjections`): detects programming languages in conversation, injects `analyze`-phase skills by `language` field, with 500-token budget

### 2.10 Environment Best Practices Engine

Auto-detects project language from config files and applies per-language checks.

**Detection:** Go (go.mod), Python (requirements.txt, pyproject.toml), JS (package.json), Rust (Cargo.toml), Java (pom.xml), Ruby (Gemfile), C++ (CMakeLists.txt) -- 20+ languages total via `internal/environment/detector.go`.

**Version Resolution:** matches installed runtime vs project requirements. Known Python version constraints for ML packages (tensorflow requires <=3.12, torch requires <=3.12, etc.).

**Best Practices:**
- Go: go.mod, go.sum, .gitignore
- Python: virtual environment, pinned deps, version compatibility
- JS: lockfile, Node version pinning
- Rust: Cargo.lock, edition
- Java: build wrapper

**Command Wrapping:** auto-activates venv, generates setup/test/build commands per detected language.

---

## 3. Agent Capabilities

### 3.1 Tool Registry

9 built-in tools plus 2 agent tools:

| Tool | Category | Arguments | Purpose |
|------|----------|-----------|---------|
| `bash` | dangerous | `command` (string), `timeout` (int, optional) | Execute shell commands. Timeout default 120s, max 600s. Child process killing on timeout. |
| `file_read` | read_only | `path` (string), `offset` (int, optional), `limit` (int, optional) | Read file contents with optional line range |
| `file_write` | write | `path` (string), `content` (string) | Write/overwrite file |
| `file_edit` | write | `path` (string), `old_string` (string), `new_string` (string), `replace_all` (bool, optional) | Exact string replacement in files |
| `grep` | read_only | `pattern` (string), `path` (string, optional), `glob` (string, optional), `type` (string, optional) | Regex search across files via ripgrep |
| `glob` | read_only | `pattern` (string), `path` (string, optional) | File pattern matching |
| `git` | write | `args` (string) | Git operations. Safety: blocks `push --force`, `branch -D`, `checkout --force`. Use bash for explicit dangerous git ops. |
| `recall` | read_only | `id` (int, optional), `query` (string, optional), `tool_name` (string, optional) | Retrieve from memory. Three modes: by ID, semantic search, tool output search. |
| `web_search` | read_only | `query` (string) | *Planned (#227).* Search external information. |
| `web_fetch` | read_only | `url` (string) | *Planned (#227).* Read URL content. |
| `delegate` | agent | `task` (string), `model` (string, optional) | Spawn sub-agent for parallel work |
| `handoff` | agent | `target` (string), `context` (string) | Transfer context to specialist agent (Swarm-style) |

**Tool Categories:**
- `read_only` -- always allowed, no approval needed
- `write` -- needs approval in interactive mode
- `dangerous` -- extra scrutiny, explicit approval

### 3.2 Agent Pipeline

Every substantial task runs through a multi-phase pipeline. Two pipeline types are auto-detected from matched skills:

#### Software Pipeline (default)

```
Plan --> Implement --> Self-Check --> Code-Review --> Acceptance-Test --> Deploy
```

#### Data Science Pipeline

```
EDA --> Data-Prep --> Model --> Review --> Deploy --> Verify
```

**Pipeline behavior:**

- **Quality gates:** verification phases require minimum tool calls. An agent cannot rubber-stamp with text alone.
- **Sub-agent reviews:** code-review and acceptance-test phases spawn FRESH agents with no shared context. This provides independent review -- the reviewer cannot be biased by the implementer's conversation.
- **Provider escalation:** triggered by pipeline phases (via Escalate flag), not by separate mechanisms.
- **Max fail-back cycles:** 3 cycles before accepting result and advancing.
- **Phase compaction:** conversation is compacted between phases to prevent context overflow.

#### Pipeline as a Tool (Planned, #232)

Currently the pipeline runs on every `synroute chat` session. The planned design exposes pipeline phases as tools the frontier model calls on demand:

- The frontier model handles conversation directly
- Pipeline phases exposed as tools: `plan()`, `implement()`, `self_check()`, `code_review()`, `test()`
- Simple questions get direct answers; complex coding tasks trigger the pipeline
- Related: standalone phase commands (#230), stateful REPL (#235)

### 3.3 Two-Tier Model Routing (Planned, #233)

Currently the same model/escalation chain handles everything. The planned design splits work by tier:

| Tier | Models | Handles |
|------|--------|---------|
| Frontier (L5-L6 or subscription) | Expensive, highly capable | User conversation, planning, orchestration decisions |
| Cheap (L0-L2) | Fast, inexpensive | Coding sub-tasks, file generation, test execution |

- Parent agent always uses frontier tier; child agents default to cheap tier
- Pipeline phases map to tiers: plan=frontier, implement=cheap, self-check=cheap, code-review=frontier
- Extend `SpawnChild` config to accept `ModelTier` (`cheap`, `frontier`, `auto`)

### 3.4 Sub-Agent SDK

The agent can spawn child agents for parallel work and handoff.

#### SpawnChild / RunChild

```go
SpawnChild(SpawnConfig) (*Agent, error)        // create child agent
RunChild(ctx, cfg, task) (string, error)       // spawn + run + collect result
RunChildrenParallel(ctx, tasks, max) ([]Result) // parallel delegation
```

- Child agents inherit model, tools, and work directory from parent
- `ParentSessionIDs` carries lineage for cross-session recall
- Configurable max concurrent agents via `--max-agents` (default 5)

#### Handoffs (Swarm-Style)

```go
ExecuteHandoff(ctx, Handoff) (string, error)  // spawn specialist with context summary
```

- `DelegateTool` and `HandoffTool` are LLM-invocable for agent-to-agent context transfer
- Context summary is passed to the new agent; no shared conversation

#### Agent Pool

Concurrency-limited agent management:
- Default 5 concurrent agents
- Semaphore-based resource tracking
- Queryable via `/v1/agent/pool` API

### 3.5 Hallucination Detection

- **FactTracker** -- accumulates ground truth from tool outputs (file paths, exit codes, test results, compilation status)
- **HallucinationChecker** -- 5 pattern-based rules, <1ms execution, no LLM calls
- **AutoRecall** -- retrieves contradicting evidence from memory, injects corrective message
- Rate limited at 3 corrections per session
- All corrective messages pass through `scrubSecrets()`

### 3.6 Loop Detection

Intent-based fingerprinting detects when the agent is stuck:
- Fingerprints built from tool calls + arguments
- Warning counter increments on repeated fingerprints
- After threshold: force phase advance or terminate

### 3.7 Regression Detection (Planned, #201)

Currently no detection when changes make things worse (e.g., compilation errors go from 2 to 332). Planned:
- Track build/test metrics between iterations
- Flag when metrics regress beyond threshold
- Revert or escalate on regression

### 3.8 Session Continuity (Planned, Epic #220)

When a new agent session starts on a project where a previous session left work, the agent should pick up where it left off.

**Design: Hybrid DB + synroute.md**

- **Central SQLite DB (primary):**
  - `project_continuity` table: session_id, project_dir, phase_reached, build_status, test_status, files_modified, acceptance_criteria, summary
  - At session start: query last session for this project_dir
  - Inject prior session_id into ParentSessionIDs for cross-session recall

- **synroute.md (secondary/portable):**
  - Human-readable summary + structured YAML frontmatter
  - Frontmatter: session_id, phase, build status, file list, timestamp
  - Travels with the project, bootstraps context on new machines
  - Links to DB via session_id when DB is available

**Implementation scope:** ~250 lines Go, 1 SQL migration, no new dependencies. All existing infrastructure reused (VectorMemory, ToolOutputStore, UnifiedSearcher, recall tool).

**Stories:** #221 (migration), #222 (write side), #223 (read side), #224 (synroute.md YAML), #225 (ParentSessionIDs wiring), #226 (integration tests)

### 3.9 Guardrails

Input/output validation via composable guardrail chain:

| Guardrail | Purpose |
|-----------|---------|
| `MaxLengthGuardrail` | Reject inputs/outputs exceeding length limit |
| `SecretPatternGuardrail` | Detect API keys, tokens, passwords in content |
| `BlocklistGuardrail` | Block specific words or phrases |
| `GuardrailChain` | Compose multiple guardrails |

### 3.10 Budget Tracking

Per-agent resource limits:

- **Turn budget:** maximum LLM calls per agent
- **Token budget:** maximum total tokens (set via `--budget`)
- **Duration budget:** maximum wall-clock time
- `BudgetTracker` enforces limits and terminates agent when exceeded

### 3.11 Tracing and Metrics

- **Tracing** (`internal/agent/trace.go`): structured event spans for llm_call, tool_call, handoff
- **Metrics** (`internal/agent/metrics.go`): request/tool/sub-agent performance tracking
- **Streaming** (`internal/agent/streaming.go`): line-by-line output via `StreamWriter`

### 3.12 State Persistence

SQLite-backed session save/load/resume:

```go
SaveState(db)                    // serialize current agent state
LoadState(db, id)                // load specific session
LoadLatestState(db)              // load most recent session
RestoreAgent()                   // rebuild agent from loaded state
```

Migration: `migrations/006_agent_sessions.sql`

Resume via CLI: `synroute chat --resume` or `synroute chat --session <id>`

### 3.13 Compaction

Three mechanisms move old messages from in-memory conversation to the database:

1. **BeforeTrimHook (automatic):** when conversation exceeds `MaxMessages`, stores messages about to be trimmed to VectorMemory. Respects tool-call boundaries.
2. **Phase compaction:** between pipeline phases, stores oldest `N-20` messages to DB, rebuilds with summary marker + 20 most recent.
3. **Emergency trim (context overflow):** on LLM context overflow error, stores first 20 messages to DB, trims, retries.

#### Model-Aware MaxMessages

| Model Family | MaxMessages | Context Window |
|-------------|-------------|----------------|
| Gemini | 500 | 1M+ tokens |
| Claude | 400 | 200K tokens |
| DeepSeek | 300 | 128K tokens |
| Default (Ollama, etc.) | 200 | Conservative |

### 3.14 Auto-Context Injection

After any compaction event, `buildMessages()` automatically injects retrieved context before conversation messages. Query is the last user message. Budget capped at 2048 tokens.

### 3.15 Spec Compliance System

When a user provides a spec file (`--spec-file`), the agent treats it as a binding contract, not a suggestion. Every phase of the pipeline enforces spec directives.

#### Requirements

**R1: All Model Levels**
- ALL model levels (L0 small models through L6 frontier) must receive spec compliance instructions in their system prompt
- Level 0 gets a condensed version (~50 tokens); Level 1+ gets the full version
- No model level is exempt from spec compliance

**R2: Spec Perception (Plan Phase)**
- Before generating acceptance criteria, the plan phase MUST include a "Spec Perception" step
- The LLM restates the spec's key architectural decisions in its own words: package structure, IN/OUT scope, mandated/prohibited patterns, technology constraints
- This prevents cascading errors from misperceived specs (research shows 29.6% improvement)

**R3: Criteria Extraction**
- If the spec contains an explicit "Acceptance Criteria" section, those criteria MUST be extracted verbatim as the baseline
- LLM-generated criteria supplement spec criteria — they do not replace them
- This prevents the LLM from generating wrong criteria that all subsequent phases verify against

**R4: Spec Visibility in Review Phases**
- Self-check, code-review, and acceptance-test phases MUST receive the original spec (not just acceptance criteria summaries)
- Reviewers must verify: implementation matches spec's architecture, package structure, directory layout, and scope
- Reviewers must check for OUT OF SCOPE violations (features added that spec excludes)

**R5: Skill Precedence**
- Skill patterns are suggestions and defaults
- When a spec defines architecture, package structure, directory layout, or scope (IN/OUT), those spec directives are MANDATORY
- Spec directives override any conflicting skill pattern
- Skills must include spec-deferral language: "If a project spec defines different architecture, follow the spec"

**R6: Sub-Agent Spec Awareness**
- Child agents (parallel coders, reviewers, fixers) must be instructed that spec architecture overrides skill patterns
- The child-agent base prompt must reference spec compliance
- Review sub-agents must check against the original spec, not just skill patterns

#### Acceptance Criteria

1. Level 0 system prompt contains SPEC COMPLIANCE section
2. Level 1+ SPEC COMPLIANCE section appears in the top third of the prompt (not buried at line 80+)
3. Plan phase prompt includes Spec Perception step (step 0)
4. Plan phase extracts acceptance criteria from spec when available
5. Code-review and acceptance-test prompts include the original spec text
6. PhasePrompt function supports both criteria and spec parameters
7. Skill precedence note explicitly says spec directives are MANDATORY
8. child-agent.md references spec compliance
9. 13 high-risk skill files include spec-deferral headers
10. Spring PetClinic reconstruction test produces correct package structure (org.springframework.samples.petclinic, not com.example)
11. Spring PetClinic test produces no service layer (spec says "no service layer")
12. Spring PetClinic test produces 12 Thymeleaf templates

---

## 4. CLI Interface

### 4.1 Commands

```bash
synroute                                    # Start HTTP server (default)
synroute serve                              # Start HTTP server (explicit)
synroute chat                               # Interactive agent REPL
synroute chat --model claude-sonnet-4-6     # Specific model
synroute chat --message "fix the bug"       # One-shot (non-interactive)
synroute chat --system "You are a Go expert" # Custom system prompt
synroute chat --worktree                    # Run in isolated git worktree
synroute chat --max-agents 3               # Limit concurrent sub-agents
synroute chat --budget 10000               # Max total tokens
synroute chat --project my-app             # Create ~/Development/my-app/ and work there
synroute chat --resume                     # Resume most recent session
synroute chat --session <id>               # Resume specific session
synroute test                              # Smoke test all providers
synroute test --provider ollama-chain-1    # Test single provider
synroute test --json                       # JSON output
synroute profile show                      # Show active profile
synroute profile list                      # List available profiles
synroute profile switch work               # Switch to work profile
synroute doctor                            # Run diagnostics
synroute doctor --json                     # JSON diagnostics
synroute models                            # List available models
synroute version                           # Show version info
synroute mcp-serve                         # Start standalone MCP server
synroute mcp-serve --addr :9090            # Custom port
```

#### Eval Commands

```bash
synroute eval import --source <source> --path <path>   # Import benchmark
synroute eval import --source exercism --path ~/exercism-go --language go
synroute eval import-all --dir ~/eval-benchmarks       # Import all benchmarks
synroute eval exercises --language go                   # List imported exercises
synroute eval run --language go --count 10 --two-pass   # Run eval
synroute eval run --provider ollama-chain-1 --count 20  # Run on specific provider
synroute eval run --mode routing --count 15             # Run in routing mode
synroute eval run --per-suite 40                        # 40 per suite (default)
synroute eval run --per-suite 0 --count 100             # No per-suite limit
synroute eval results --json                            # Most recent run
synroute eval compare --run-a <id1> --run-b <id2>       # Compare two runs
```

#### Standalone Phase Commands (Planned, #230)

```bash
synroute plan "build a REST API"           # Plan only, output plan
synroute code --spec-file spec.md          # Implement only
synroute review                            # Review current code in working dir
synroute fix "the auth middleware is broken" # Targeted fix
```

Each command persists state via project_continuity so the next command picks up context.

### 4.2 REPL Slash Commands

| Command | Purpose |
|---------|---------|
| `/exit` | Exit the REPL |
| `/clear` | Clear conversation history |
| `/model` | Show or switch current model |
| `/tools` | List available tools |
| `/history` | Show conversation history |
| `/agents` | Show active sub-agents |
| `/budget` | Show remaining budget |

### 4.3 Worktree Isolation

`synroute chat --worktree` creates a managed git worktree for safe code changes:

- TTL-based expiry: default 24 hours
- Size caps: 10GB total across all worktrees, 2GB per individual worktree
- Background cleanup: every 5 minutes
- Changes can be merged back or discarded

### 4.4 Permission Model

| Mode | Behavior |
|------|----------|
| `interactive` | Prompt user before write/dangerous tool execution |
| `auto_approve` | Allow all tool executions without prompting |
| `read_only` | Deny all write operations |

### 4.5 File Attachment (Planned, #228)

Accept file paths or URLs in messages. The agent reads them and includes content in conversation context.

- Images: base64 encode for multimodal models
- PDFs: extract text
- Large files: chunk and summarize
- Syntax: `@file` references in messages

### 4.6 CLI Color System (Planned, #231)

Semantic color mapping for REPL elements:

| Element | Default Color |
|---------|--------------|
| User input | White/default |
| Agent response | Cyan/blue |
| / commands | Yellow |
| @file references | Green |
| Tool calls | Magenta |
| Tool output | Dim/gray |
| Errors | Red |
| Phase transitions | Bold blue |
| Escalation | Bold yellow |
| Success | Green |
| Failure | Red |

**Accessibility:**
- Built-in presets: default, high-contrast, deuteranopia, protanopia, tritanopia
- Never relies on color alone -- always paired with symbols/text
- WCAG 2.1 contrast ratios
- Respects `NO_COLOR` env var (https://no-color.org/)
- Per-element overrides via env vars (`CLI_COLOR_AGENT=cyan`)

### 4.7 CLI Terminal UI (Planned, Epic 9)

Polished terminal interface beyond basic REPL:

- **Chat mode** (Story 9.1): status bar, keyboard shortcuts (Ctrl-H history, Ctrl-S sessions, Ctrl-N new, Ctrl-F files), raw ANSI + Go stdlib
- **Code mode** (Story 9.2): pipeline phase display, tool call visualization, Ctrl-P pipeline status, Ctrl-R recall, Ctrl-E escalate
- **Session management** (Story 9.3): inline session browser, auto-naming, list/search/archive

---

## 5. HTTP API

### 5.1 OpenAI-Compatible Endpoints

#### Chat Completions

```
POST /v1/chat/completions
```

Standard OpenAI chat completion format. Supports `model: "auto"` for router-selected model, or specific model names.

#### Responses

```
POST /v1/responses
```

OpenAI Responses API format (used by Codex). SSE streaming via this endpoint (NOT `/responses/compact`).

#### Provider-Specific Routing

```
POST /api/provider/{provider}/v1/chat/completions
```

Route directly to a named provider (e.g., `ollama-chain-3`).

### 5.2 Stateful Chat Sessions (Planned, Story 7.1)

```
POST /v1/chat/completions
Header: X-Session-ID: <session_id>
```

- Conversation history persisted in VectorMemory per session
- Previous messages retrieved and injected
- Skill preprocessing fires on every message
- Session timeout: configurable idle expiry

### 5.3 Smart Model Selection for Chat (Planned, Story 7.2)

Query complexity scoring: simple queries use L0-L1 (fast), complex queries use L3-L4 (thorough), code generation routes to code-specialized models.

### 5.4 Agent API

```
POST /v1/agent/chat
Body: {"message": "fix the tests", "model": "auto"}
```

Runs the agent loop via HTTP. Returns agent output.

```
GET /v1/agent/pool
```

Agent pool metrics: active agents, capacity, resource usage.

### 5.5 Diagnostic Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/health` | Health check |
| GET | `/v1/models` | List available models |
| GET | `/v1/profile` | Show active profile |
| POST | `/v1/profile/switch` | Switch profile |
| GET | `/v1/doctor` | Run diagnostics |
| POST | `/v1/test/providers` | Smoke test all providers |
| POST | `/v1/circuit-breakers/reset` | Reset circuit breakers (all or specific) |
| GET | `/v1/skills` | List registered skills |
| GET | `/v1/skills/match?q=...` | Preview skill chain for a goal |
| GET | `/v1/tools` | List agent tools |

### 5.6 Eval API

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/v1/eval/exercises?language=go` | List imported exercises |
| POST | `/v1/eval/runs` | Start eval run |
| GET | `/v1/eval/runs` | List recent runs |
| GET | `/v1/eval/runs/<id>` | Run status + summary |
| GET | `/v1/eval/runs/<id>/results` | Individual results |
| POST | `/v1/eval/compare` | Compare two runs |

### 5.7 MCP Server Mode

Expose agent tools over HTTP using the Model Context Protocol.

Start standalone: `synroute mcp-serve` or enable on main server with `SYNROUTE_MCP_SERVER=true`.

| Method | Path | Purpose |
|--------|------|---------|
| POST | `/mcp/initialize` | Initialize MCP session |
| POST | `/mcp/tools/list` | List available tools |
| POST | `/mcp/tools/call` | Call a tool (JSON-RPC format) |

### 5.8 N-1 Concurrency Reservation (Planned, Story 7.3)

When agent runs, reserve N-1 model slots for agent, keep 1 for chat. Queue chat requests to reserved models rather than rejecting.

---

## 6. Eval Framework

### 6.1 Overview

The eval framework benchmarks LLM coding capabilities across multiple languages and problem types. It imports exercises from external benchmark suites, runs them through the router, and scores results.

**Source files:** `internal/eval/` (types, store, importer, docker, runner, scorer), `eval_commands.go` (CLI), `eval_handlers.go` (API)

### 6.2 Benchmark Sources (11)

| Source | Description | Import Command |
|--------|-------------|----------------|
| `polyglot` | Polyglot benchmark exercises | `eval import --source polyglot` |
| `roocode` | Roo-Code evaluation exercises | `eval import --source roocode` |
| `exercism` | Exercism exercises (per language) | `eval import --source exercism --language go` |
| `multiple` | MultiPL-E multilingual benchmark | `eval import --source multiple` |
| `evalplus` | EvalPlus benchmark suite | `eval import --source evalplus` |
| `codecontests` | Code Contests problems | `eval import --source codecontests --count 500` |
| `ds1000` | DS-1000 data science problems | `eval import --source ds1000` |
| `birdsql` | BIRD-SQL database queries | `eval import --source birdsql` |
| `dare-bench` | DARE-Bench difficulty assessment | `eval import --source dare-bench` |
| `writingbench` | WritingBench creative writing | `eval import --source writingbench` |
| `pptarena` | PPTArena presentation generation | `eval import --source pptarena` |

### 6.3 Eval Modes (4)

| Mode | Description |
|------|-------------|
| Language-specific | Run exercises for one language (`--language go`) |
| Provider-specific | Run on a specific provider (`--provider ollama-chain-1`) |
| Routing | Test the router's model selection (`--mode routing`) |
| Two-pass | Run each exercise twice for consistency (`--two-pass`) |

### 6.4 Reconstruction Test Suite

75 projects across 15 categories used to find and fix weaknesses in the agent. Each project that fails reveals a gap in skills, pipeline, escalation, tools, or prompts.

#### Structure

Each project gets its own folder with a `spec.md` (written by Claude Code after analyzing the original repo). Synapserouter builds from the spec without seeing the original repo.

#### Categories

| Category | Projects | Languages |
|----------|----------|-----------|
| Wave 1: Calibration | 5 | Java, TypeScript, Rust, ML Python, Go |
| Wave 2: Skill Gaps | 5 | C#, ML Python, Python, Java, Rust |
| Wave 3: Architecture | 10 | Mixed (Go, TS, Python, Java, C#) |
| Wave 4: Scale | 10 | Mixed |
| Wave 5: Boss Level | 5 | Python, C#, Java, PowerShell |
| Jupyter Notebooks | 5 | Python (.ipynb) |
| R Markdown | 5 | R (.Rmd) |
| SQL/Database | 5 | SQL |
| Wave 6: New Languages | 25 | Kotlin (5), Swift (5), C++ (5), Ruby (5), PHP (5) |

#### Failure Categories

| Category | Description | Example Fix |
|----------|-------------|-------------|
| `skill_gap` | No skill for language/framework | Create new skill |
| `architecture` | Wrong patterns chosen | Add patterns to skill |
| `tool_usage` | Tools used incorrectly | Fix tool implementation |
| `escalation` | Stuck on weak model | Fix escalation thresholds |
| `pipeline` | Skipped phases | Fix pipeline enforcement |
| `scope` | Overwhelmed by project size | Improve task decomposition |
| `context` | Lost context mid-task | Fix context management |
| `dependency` | Cannot set up build env | Improve environment detection |
| `output_format` | Wrong file format | Add format-specific skill |
| `destructive` | Agent deleted/destroyed work | Add safety guards |

#### Router Bugs Found (11 fixed)

Models calling wrong tool names, agent exiting after plan phase, memory injecting 163 messages (overflow), L0 models unable to function-call, pipeline infinite loop on quality gate, sub-agent using wrong provider, phase signals ignored during tool calls, agent running full ML training, PhasePrompt returning raw %s, pipeline phases never engaging, bash timeout not killing children.

---

## 7. Container Isolation (Planned)

Epic #211. Docker and Podman as two separate, equal container runtime options.

### 7.1 Docker Runtime (#213)

- Native Docker Go SDK (`github.com/docker/docker/client`)
- `--network=container:<primary>` for shared namespace (pod-like localhost communication)
- Primary session container holds the namespace, sub-agents attach to it
- Socket: `/var/run/docker.sock`
- Individual container lifecycle management

### 7.2 Podman Runtime (#212)

- Native Podman REST API via `net/http` (not the heavy podman/v5 Go module)
- Pod model: session pod contains workspace + sub-agent containers sharing network namespace
- Containers communicate via localhost (shared namespace)
- Socket: `/run/user/{uid}/podman/podman.sock` or `/run/podman/podman.sock`
- Atomic lifecycle: pod rm removes all containers

### 7.3 Session Containers (#214)

Named volumes for workspace sharing (`workspace-{sessionID}`). Sub-agents work in subdirectories of shared volume.

### 7.4 Sub-Agent Containers (#215)

Each sub-agent runs in its own container within the session pod/network. Inherits workspace volume.

### 7.5 Warm Pool (#216)

Pre-warm 5-10 idle containers for <100ms claim time. Container pooling for sub-second startup.

### 7.6 Security Hardening (#217)

- `cap-drop=ALL` -- drop all Linux capabilities
- Read-only root filesystem
- `no-new-privileges` flag
- Resource limits per container (memory, CPU)
- Filesystem restrictions (planned, Story 6.5): allowlist writable dirs, denied paths (~/.ssh, ~/.aws), command denylist

### 7.7 Configuration

```env
CONTAINER_RUNTIME=podman|docker|none    # none = current behavior (no containers)
```

Socket auto-detection at startup.

### 7.8 Scaling Path

- **Single server** (Unraid/bare metal): Docker socket or Podman socket
- **Cloud:** Same socket pattern, swap for Kubernetes API
- **Stronger isolation:** Sysbox runtime (no code changes) or Firecracker microVMs
- **RAM budget:** 5 users x 6 containers x 512MB = ~19GB on 64GB Unraid

---

## 8. Configuration

### 8.1 Environment Variables

#### Core

| Variable | Default | Purpose |
|----------|---------|---------|
| `ACTIVE_PROFILE` | `personal` | Active profile: `personal` or `work` |
| `PORT` | `8090` | HTTP server port |
| `DB_PATH` | `synroute.db` | SQLite database path |

#### Ollama Cloud (Personal Profile)

| Variable | Default | Purpose |
|----------|---------|---------|
| `OLLAMA_API_KEY` | -- | Single Ollama Cloud API key |
| `OLLAMA_API_KEYS` | -- | Multiple API keys (comma-separated, round-robin) |
| `OLLAMA_BASE_URL` | -- | Ollama Cloud base URL |
| `OLLAMA_CHAIN` | -- | Model chain (levels separated by `\|`, models by `,`) |
| `OLLAMA_PLANNER_1` | -- | Planner model slot 1 |
| `OLLAMA_PLANNER_2` | -- | Planner model slot 2 |

#### Subscription Providers

| Variable | Default | Purpose |
|----------|---------|---------|
| `SUBSCRIPTIONS_DISABLED` | `false` | Disable all subscription providers |
| `SUBSCRIPTION_PROVIDER_ORDER` | `gemini,openai,anthropic` | Fallback order after Ollama Cloud |

#### Vertex AI (Work Profile)

| Variable | Default | Purpose |
|----------|---------|---------|
| `VERTEX_CLAUDE_PROJECT` | -- | GCP project for Claude |
| `VERTEX_CLAUDE_REGION` | `us-east5` | Vertex AI region for Claude |
| `VERTEX_GEMINI_PROJECT` | -- | GCP project for Gemini |
| `VERTEX_GEMINI_LOCATION` | `global` | Vertex AI location for Gemini |
| `VERTEX_GEMINI_SA_KEY` | -- | Service account key file for Gemini |

#### Usage Limits

| Variable | Default | Purpose |
|----------|---------|---------|
| `OLLAMA_CLOUD_DAILY_LIMIT` | `1000000` | Daily token limit |
| `CLAUDE_CODE_DAILY_LIMIT` | `500000` | Daily token limit |
| `GEMINI_DAILY_LIMIT` | `500000` | Daily token limit |
| `CODEX_DAILY_LIMIT` | `300000` | Daily token limit |

#### Agent

| Variable | Default | Purpose |
|----------|---------|---------|
| `STALL_TIMEOUT_SECONDS` | `180` | Stall detection timeout |

#### MCP Server

| Variable | Default | Purpose |
|----------|---------|---------|
| `SYNROUTE_MCP_SERVER` | `false` | Enable MCP server on main HTTP server |

#### Container Runtime (Planned)

| Variable | Default | Purpose |
|----------|---------|---------|
| `CONTAINER_RUNTIME` | `none` | Container runtime: `podman`, `docker`, `none` |

#### CLI Appearance (Planned)

| Variable | Default | Purpose |
|----------|---------|---------|
| `CLI_THEME` | `default` | Color theme preset |
| `CLI_NO_COLOR` | `false` | Disable all colors |
| `NO_COLOR` | -- | Standard no-color flag |

### 8.2 .env File

All configuration is loaded from a `.env` file in the project root. The `.env` file should never be committed (listed in `.gitignore`).

### 8.3 Profile Switching

```bash
# CLI
synroute profile switch work

# API
curl -X POST localhost:8090/v1/profile/switch -d '{"profile":"work"}'
```

Switching profiles reinitializes providers. Personal profile loads Ollama Cloud + optional subscriptions. Work profile loads Vertex AI only.

### 8.4 Model Defaults

| Context | Default Model |
|---------|---------------|
| Ollama Cloud | Per level, from `OLLAMA_CHAIN` |
| Gemini personal (subscription) | `gemini-3-flash-preview` |
| Vertex Gemini (work) | `gemini-3.1-pro-preview` |
| Vertex Claude (work) | Model names without dates (e.g., `claude-sonnet-4-6`) |
| Codex | SSE streaming via `/responses` endpoint |
| Gemini 2.5+ | Thinking tokens from output budget, min 1024 maxOutputTokens |

---

## 9. Acceptance Criteria

### 9.1 Provider Routing

- [ ] **AC-R1:** Request with `model: "auto"` routes to lowest-usage healthy provider
- [ ] **AC-R2:** When L0 providers all fail, request automatically escalates to L1 within same API call
- [ ] **AC-R3:** Full escalation L0 through L6 then subscription fallback works end-to-end
- [ ] **AC-R4:** Circuit breaker opens after 5 consecutive failures; closes on successful probe after cooldown
- [ ] **AC-R5:** Rate limit error with "reset after Ns" sets cooldown to N+5 seconds
- [ ] **AC-R6:** Health check results cached for 2 minutes; no API call during cache validity
- [ ] **AC-R7:** Multiple API keys distribute across providers in round-robin
- [ ] **AC-R8:** Profile switch from personal to work replaces all providers with Vertex AI
- [ ] **AC-R9:** Stall detection retries when response <50 chars after 180s
- [ ] **AC-R10:** Usage tracking enforces 80% auto-switch threshold

### 9.2 Agent Pipeline

- [ ] **AC-P1:** Every substantial task runs through Plan, Implement, Self-Check, Code-Review, Acceptance-Test phases
- [ ] **AC-P2:** Verification phases (self-check, code-review, acceptance-test) require minimum tool calls to advance
- [ ] **AC-P3:** Code-review and acceptance-test phases use fresh sub-agents with no shared context
- [ ] **AC-P4:** Max 3 fail-back cycles before accepting result
- [ ] **AC-P5:** Conversation compacted between phases; no context overflow on long sessions
- [ ] **AC-P6:** Data science tasks auto-detect and use EDA pipeline
- [ ] **AC-P7 (planned):** Pipeline phases available as tools the frontier model calls on demand (#232)
- [ ] **AC-P8 (planned):** Simple questions get direct answers without triggering pipeline (#235)
- [ ] **AC-P9 (planned):** Review cycle stops when no improvement detected for 2 consecutive cycles (Story 1.2)

### 9.3 Agent Tools

- [ ] **AC-T1:** `bash` tool kills child processes on timeout; timeout configurable up to 600s
- [ ] **AC-T2:** `file_edit` fails if `old_string` not found (exact match required)
- [ ] **AC-T3:** `git` tool blocks `push --force`, `branch -D`, `checkout --force`
- [ ] **AC-T4:** `recall` tool returns results from current session and all parent sessions
- [ ] **AC-T5:** Tool outputs >2KB stored in DB and summarized in conversation with back-reference
- [ ] **AC-T6:** All tool outputs pass through `scrubSecrets()` before DB storage
- [ ] **AC-T7 (planned):** `web_search` and `web_fetch` tools available for external research (#227)

### 9.4 Memory System

- [ ] **AC-M1:** No information loss: every message and tool output reaches DB before being dropped from conversation
- [ ] **AC-M2:** `recall(id=N)` returns full tool output from any ancestor session
- [ ] **AC-M3:** `recall(query="...")` returns semantically relevant results from VectorMemory + tool_outputs
- [ ] **AC-M4:** BeforeTrimHook stores messages before they are dropped; respects tool-call boundaries
- [ ] **AC-M5:** Emergency trim on context overflow stores messages then retries
- [ ] **AC-M6:** Auto-context injection after compaction injects relevant prior context (2048 token budget)
- [ ] **AC-M7:** Model-aware MaxMessages: Gemini=500, Claude=400, DeepSeek=300, default=200
- [ ] **AC-M8 (planned):** Real embedding model (ONNX) with cosine similarity >0.7 for semantically similar queries (Story 6.1)
- [ ] **AC-M9 (planned):** Memory injection does not re-store already-injected messages (Story 0.2)

### 9.5 Skill System

- [ ] **AC-S1:** `ParseSkillsFromFS` loads all 52+ skills from embedded `.md` files at startup
- [ ] **AC-S2:** Compound trigger `go+handler` matches "write a go handler" but not "going to handle this"
- [ ] **AC-S3:** Ambiguous word `go` (standalone trigger) matches "write go code" but not "going to fix"
- [ ] **AC-S4:** Skills sorted by phase order (analyze, implement, verify, review) in chain
- [ ] **AC-S5:** New skill added by dropping `.md` file and rebuilding -- no Go code changes
- [ ] **AC-S6:** Router preprocessor injects skills within 500-token budget
- [ ] **AC-S7 (planned):** Skill context injection capped at 8K tokens total (Story 0.7)
- [ ] **AC-S8 (planned):** System prompt tiered by model context window (#237)

### 9.6 Sub-Agents

- [ ] **AC-A1:** `RunChild` spawns child agent that inherits model, tools, work directory
- [ ] **AC-A2:** `RunChildrenParallel` respects `--max-agents` concurrency limit (default 5)
- [ ] **AC-A3:** Child agent can recall tool outputs from parent session via ParentSessionIDs
- [ ] **AC-A4:** Handoff transfers context summary to specialist agent
- [ ] **AC-A5 (planned):** Two-tier model routing: parent=frontier, children=cheap (#233)

### 9.7 Session Continuity (Planned)

- [ ] **AC-SC1:** New session on same project_dir loads prior session's phase, build status, file list
- [ ] **AC-SC2:** Prior session_id injected into ParentSessionIDs for cross-session recall
- [ ] **AC-SC3:** synroute.md YAML frontmatter contains session_id, phase, build status, timestamp
- [ ] **AC-SC4:** synroute.md bootstraps context when DB is unavailable (different machine)

### 9.8 CLI

- [ ] **AC-C1:** `--message` mode works in current directory; files persist after exit
- [ ] **AC-C2:** `--project <name>` creates `~/Development/<name>/` and works there
- [ ] **AC-C3:** `--worktree` creates isolated git worktree with 24h TTL and 2GB size cap
- [ ] **AC-C4:** `--resume` restores most recent session with full conversation history
- [ ] **AC-C5:** All REPL slash commands functional: /exit, /clear, /model, /tools, /history, /agents, /budget
- [ ] **AC-C6 (planned):** @file references attach file content to conversation (#228)
- [ ] **AC-C7 (planned):** Standalone phase commands work independently with session continuity (#230)
- [ ] **AC-C8 (planned):** Color system respects NO_COLOR and provides accessibility presets (#231)

### 9.9 HTTP API

- [ ] **AC-H1:** `/v1/chat/completions` returns OpenAI-compatible responses
- [ ] **AC-H2:** `/v1/responses` supports SSE streaming for Codex
- [ ] **AC-H3:** `/health` returns 200 when server is running
- [ ] **AC-H4:** Circuit breaker reset via API resets specific or all providers
- [ ] **AC-H5:** MCP server exposes all agent tools via JSON-RPC
- [ ] **AC-H6 (planned):** Stateful chat sessions via session_id header (Story 7.1)

### 9.10 Eval Framework

- [ ] **AC-E1:** All 11 benchmark sources importable without errors
- [ ] **AC-E2:** Eval run executes exercises, scores results, stores in DB
- [ ] **AC-E3:** Compare command shows diff between two eval runs
- [ ] **AC-E4:** Per-suite limit controls exercises per benchmark (default 40)
- [ ] **AC-E5:** Docker-based execution isolates eval runs

### 9.11 Container Isolation (Planned)

- [ ] **AC-CI1:** Docker runtime creates session containers with shared network namespace
- [ ] **AC-CI2:** Podman runtime creates session pods with shared network namespace
- [ ] **AC-CI3:** Sub-agent containers share workspace volume with session container
- [ ] **AC-CI4:** Warm pool provides <100ms container claim time
- [ ] **AC-CI5:** Security hardening: cap-drop=ALL, read-only root, no-new-privileges
- [ ] **AC-CI6:** `CONTAINER_RUNTIME=none` preserves current behavior (no containers)

### 9.12 Hallucination Detection

- [ ] **AC-HD1:** FactTracker accumulates ground truth from tool outputs (paths, exit codes, test results)
- [ ] **AC-HD2:** HallucinationChecker detects contradictions in <1ms (no LLM calls)
- [ ] **AC-HD3:** AutoRecall injects corrective message when contradiction detected
- [ ] **AC-HD4:** Maximum 3 corrections per session (rate limited)

### 9.13 Quality and Safety

- [ ] **AC-Q1:** Agent MUST run build command before declaring implement phase complete (#236)
- [ ] **AC-Q2:** Regression detection flags when build/test metrics worsen between iterations (#201)
- [ ] **AC-Q3:** Secrets scrubbed from tool output storage (Story 0.4)
- [ ] **AC-Q4:** `SecretPatternGuardrail` detects Bearer tokens, API keys, passwords
- [ ] **AC-Q5:** Agent does not create duplicate project structures (#200)
- [ ] **AC-Q6:** Agent does not read same files multiple times (#205)
- [ ] **AC-Q7 (planned):** Bash tool sandboxing: filesystem restrictions, command denylist, resource limits (Story 6.5)

### 9.14 DevOps (Planned)

- [ ] **AC-D1:** Go updated to 1.23+; gorilla/mux replaced with stdlib net/http (Story 4.1)
- [ ] **AC-D2:** Multi-stage Dockerfile with CGo for go-sqlite3
- [ ] **AC-D3:** GitHub Actions CI: vet, test -race, build on push/PR
- [ ] **AC-D4:** Version set via ldflags in CI

---

## Appendix A: Known Bugs (Open)

Summary of key open bugs from GitHub issues:

| Issue | Title | Severity |
|-------|-------|----------|
| #237 | System prompt 82KB too large for small models | High |
| #236 | Agent declares victory without verifying code compiles | High |
| #219 | Self-check/code-review infinite fail-back loop | High |
| #218 | Plan phase runs on L0 small models | High |
| #205 | Agent reads same files multiple times | Medium |
| #204 | Sub-agents cannot resolve ~/ paths from temp dirs | Medium |
| #203 | No research-only pipeline mode | Medium |
| #202 | Verification gates never execute (phases 3-6 never reached) | High |
| #201 | No regression detection | High |
| #200 | Agent creates 26 duplicate Java classes | Medium |
| #199 | Sub-agent TargetProvider has no fallback on circuit-open | Medium |
| #198 | Skill matching is language-agnostic (go-testing on Java) | Medium |
| #197 | ProjectLanguage never set despite detection | Medium |
| #196 | Pipeline stuck in implement (9 cycles, doc says max 3) | High |
| #195 | Escalation never fires (25-turn budget before stall detection) | High |
| #194 | Agent ignores spec scope | Medium |
| #193 | Agent creates duplicate project structure | Medium |
| #192 | Agent cannot self-correct compilation errors | High |
| #191 | Self-check phase infinite loop | High |
| #241 | Spec compliance: L0 models receive no spec instructions | High |
| #242 | Spec compliance: no Spec Perception step in plan phase | High |
| #243 | Spec compliance: review phases receive criteria summary, not original spec | High |
| #244 | Spec compliance: skills override spec architecture directives | Medium |
| #245 | Spec compliance: sub-agents unaware of spec constraints | Medium |
| #246 | Spec compliance: PhasePrompt lacks spec parameter | Medium |

## Appendix B: Roadmap Phases

| Phase | Focus | Status |
|-------|-------|--------|
| Phase 1 | Core router + agent (bug fixes, stabilization) | **Current** |
| Phase 2 | DevOps pipeline, CI/CD, self-improving patterns | Next |
| Phase 3 | Real embeddings, MCP client, chat backend API, smart routing | Planned |
| Phase 4 | CLI TUI, polished terminal interface | Planned |
| Phase 5 | Scale: Postgres, multi-user, branding | Future |
| Beyond | Wave-based parallel execution, job queue, batch projects | Future |

## Appendix C: Source File Map

| File | Purpose |
|------|---------|
| `main.go` | Server setup, CLI dispatch, provider initialization, HTTP handlers |
| `commands.go` | CLI command implementations |
| `eval_commands.go` | CLI eval commands |
| `eval_handlers.go` | API eval endpoints |
| `compat_handlers.go` | OpenAI-compatible API endpoints |
| `diagnostic_handlers.go` | Test, diagnostics, circuit breaker, skill dispatch endpoints |
| `internal/router/router.go` | Provider selection, fallback chain, memory injection |
| `internal/router/circuit.go` | Circuit breaker with rate-limit cooldowns |
| `internal/providers/provider.go` | Provider interface |
| `internal/providers/vertex.go` | Vertex AI provider |
| `internal/agent/agent.go` | Agent loop, tool execution, pipeline |
| `internal/agent/pipeline.go` | Pipeline phases, quality gates |
| `internal/agent/conversation.go` | Message management, trim hooks |
| `internal/agent/subagent.go` | Sub-agent SDK |
| `internal/agent/handoff.go` | Swarm-style handoffs |
| `internal/agent/pool.go` | Agent pool concurrency |
| `internal/agent/guardrails.go` | Input/output validation |
| `internal/agent/state.go` | Session persistence |
| `internal/agent/budget.go` | Budget tracking |
| `internal/agent/trace.go` | Structured tracing |
| `internal/agent/metrics.go` | Performance metrics |
| `internal/agent/streaming.go` | Stream output |
| `internal/agent/tool_store.go` | DB-backed tool output storage |
| `internal/agent/tool_summarize.go` | Tool output summarization |
| `internal/agent/unified_recall.go` | Unified cross-store search |
| `internal/agent/fact_tracker.go` | Ground truth accumulation |
| `internal/agent/hallucination.go` | Hallucination detection rules |
| `internal/tools/` | Tool interface, registry, implementations |
| `internal/memory/vector.go` | VectorMemory, embedding search |
| `internal/memory/embeddings.go` | Embedding providers |
| `internal/orchestration/skills.go` | Skill registry, trigger matching |
| `internal/orchestration/dispatch.go` | Auto-dispatch engine |
| `internal/orchestration/skilldata/` | 52+ embedded skill Markdown files |
| `internal/environment/` | Language detection, version resolution, best practices |
| `internal/worktree/` | Git worktree isolation |
| `internal/mcpserver/` | MCP server endpoints |
| `internal/subscriptions/` | OAuth subscription provider management |
| `internal/eval/` | Eval framework |
| `internal/app/` | Shared CLI/API logic |
| `benchmarks/` | Eval benchmark data |
| `migrations/` | SQLite schema migrations |

## Appendix D: Database Schema

### Tables

| Table | Migration | Purpose |
|-------|-----------|---------|
| `memory` | `001_init.sql` | Conversation messages with vector embeddings |
| `usage_tracking` | `002_usage.sql` | Provider token usage |
| `request_audit` | `003_audit.sql` | Request audit trail |
| `provider_attempt_audit` | `003_audit.sql` | Per-provider attempt records |
| `circuit_breaker_state` | `004_circuit.sql` | Persistent circuit breaker state |
| `session_tracking` | `005_sessions.sql` | Session metadata |
| `agent_sessions` | `006_agent_sessions.sql` | Serialized agent state for resume |
| `tool_outputs` | `007_tool_outputs.sql` | Full tool output storage |
| `project_continuity` | Planned (#221) | Cross-session project state |
