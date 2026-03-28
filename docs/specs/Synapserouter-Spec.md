# Synapserouter Product Specification

**Version:** 1.1
**Date:** 2026-03-27
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

Synapserouter (binary: `synroute`) is a Go-based LLM platform with four capabilities:

1. **LLM Chat Interface** -- a CLI conversational AI, like ChatGPT or Claude Desktop but terminal-native. Ask questions, get answers, have conversations. Minimalistic, keyboard-driven, color-coded. *(Core REPL implemented; rich UI/UX planned -- #229, #231)*

2. **Autonomous Coding Agent** -- when you ask it to build something, it plans, writes code, runs tests, reviews its own work, and iterates. The frontier model talks to you; smaller models do parallel coding work. You talk to one agent; it orchestrates many.

3. **LLM Proxy Router** -- an OpenAI-compatible API server that routes raw LLM requests across any configured provider. Applications like LibreChat, Open WebUI, custom scripts, or any OpenAI-compatible client send requests to one endpoint; the router picks the best model.

4. **Agent Tool Backend** -- AI coding tools (Claude Code, OpenCode, Cursor, Continue, etc.) can use synapserouter as their model provider. Unlike raw proxy routing, the agent backend understands tool-calling patterns, manages context, and optimizes model selection for agent workloads.

### Who It Is For

- **Anyone who uses LLMs** -- one router for all providers. Chat from the terminal.
- **Developers** -- autonomous coding agent. Spec in, working project out.
- **Users of other AI tools** -- backend for Claude Code, OpenCode, LibreChat, etc.
- **Enterprise teams** -- pluggable provider backends (Vertex AI, Azure OpenAI, Amazon Bedrock, Red Hat AI Inference / models.corp, OpenRouter, any OpenAI-compatible endpoint). Container-isolated sessions. *(Planned -- Epic #211)*
- **Self-hosters** -- runs on Unraid, bare metal, or cloud. Docker and Podman support as independent runtimes.

### Why It Exists

1. **One interface, all models.** Any LLM provider that exposes an API can be added as a backend. No vendor lock-in. Swap providers by changing config, not code.

2. **Cost-optimized intelligence.** Start with the cheapest model. Escalate to bigger models only when needed. The user configures the escalation chain for their budget and providers.

3. **Multi-model collaboration.** Different models plan, code, and review. Code written by one model is reviewed by a different model with different training data -- catching blind spots neither would find alone.

4. **Terminal-native AI.** No browser, no Electron. CLI with colors, streaming, file references, session history. *(Rich chat UI planned -- #229, #231)*

5. **Unlimited context.** SQLite (or PostgreSQL) persistence with local TF-IDF embeddings. Cross-session recall at zero API cost. *(ONNX embeddings planned -- #68)*

6. **Container isolation.** Each user session can run in its own Docker or Podman container. Sub-agents get their own containers. Full capability inside, zero escape outside. *(Planned -- Epic #211)*

### Operating Modes

| Mode | Command | What It Does | Status |
|------|---------|-------------|--------|
| **Chat** | `synroute chat` | Conversational AI + coding agent | Implemented |
| **Proxy** | `synroute serve` | OpenAI-compatible API router | Implemented |
| **Agent Backend** | `synroute serve` (agent-aware routing) | Model provider for coding tools | Implemented |
| **MCP Server** | `synroute mcp-serve` | Model Context Protocol -- exposes tools over HTTP | Implemented |
| **Eval** | `synroute eval run` | Benchmark evaluation framework (11 sources, 4 modes) | Implemented |

### Provider System

Synapserouter supports any number of provider backends. Users configure which providers are available and how they're ordered for escalation.

**Supported provider types:**

| Type | Examples | Status |
|------|---------|--------|
| Ollama-compatible | Ollama Cloud, local Ollama (`ollama serve`) | Implemented |
| Google | Vertex AI (Claude + Gemini), Gemini API | Implemented |
| OpenAI-compatible | OpenAI, Codex, Azure OpenAI, any compatible endpoint | Implemented |
| Anthropic | Claude API, Claude Code (OAuth) | Implemented |
| OpenRouter | Any model on OpenRouter marketplace | Planned |
| Red Hat AI | RHAI Inference / models.corp | Planned |
| Amazon | Bedrock | Planned |
| Custom | Any HTTP endpoint with OpenAI-compatible API (configurable base URL + API key) | Planned |

**Escalation chain** is user-configured, not hardcoded. Users define their own levels based on available providers and budget. The product provides the escalation engine; the user provides the provider list.

### Profiles

Profiles are named configurations mapping a set of providers + escalation chain. Not limited to preset names -- users can define any number of profiles.

| Profile | Typical Use | Example Config |
|---------|------------|----------------|
| `personal` | Home dev | Example: Ollama Cloud primary, subscription fallback |
| `work` | Enterprise | Example: Vertex AI, RHAI, Bedrock -- whatever the org uses |
| Custom | Any | Any combination of supported providers |

Switch at runtime: `synroute profile switch <name>`.

### Database System

Two independent database backends. User picks based on environment.

| Backend | Use Case | Status |
|---------|----------|--------|
| **SQLite** | Single user, local, self-hosted. Zero config. | Implemented |
| **PostgreSQL** | Multi-user, team, production, cloud. Connection pooling, concurrent writes. | Planned (#85) |

### Design Principles

1. **The router IS the escalation.** One system routes requests. No parallel escalation mechanisms.
2. **Frontier talks, cheap models work.** The best available model handles conversation with the user. Smaller models handle parallel coding and builds.
3. **Pipeline is a tool, not a mode.** The coding pipeline is triggered when the agent decides work is needed. Conversational messages don't trigger it.
4. **Intent-based routing.** The LLM reads the user's message and knows what to do. No slash commands or mode flags needed.
5. **Multi-model verification.** Code written by Model A is reviewed by Model B. Architectural diversity for quality.
6. **Spec is a contract.** When a spec is provided, it's binding. Skills are suggestions; spec directives are mandatory.
7. **Provider-agnostic.** Any LLM provider can be added. The product doesn't prefer any vendor.
8. **Runtime-agnostic isolation.** Docker and Podman are equal, independent container runtimes. Neither is primary.
9. **Database-agnostic.** SQLite and PostgreSQL are equal, independent database backends.
10. **Skills are self-contained.** Adding a skill means dropping one `.md` file and rebuilding. No Go code changes.

### Agent Intents

The agent detects user intent from natural language and routes to the appropriate behavior:

| Intent | Example | Behavior |
|--------|---------|----------|
| **Chat** | "What does this function do?" | Answer directly. No pipeline. |
| **Plan** | "Help me design an auth system" | Plan phase only. Architecture, decisions, criteria. |
| **Build** | "Build this from the spec" | Full 6-phase pipeline. |
| **Fix** | "Debug this auth bug" | Targeted implement -- read code, diagnose, fix. No plan. |
| **Review** | "Review my code" | Code-review phase only. Independent assessment. |
| **Test** | "Write tests for this" | Test generation + verification. |
| **Refactor** | "Clean this up" | Modify existing code. No new features. |
| **Research** | "What's the best approach for X?" | Web search + analysis. No code. |
| **Explain** | "How does the auth middleware work?" | Read code, explain. No changes. |
| **Deploy** | "Package for production" | Dockerfile, Containerfile (Podman), CI, release pipeline. |
| **Migrate** | "Port this from Python to Go" | Cross-language translation with verification. |
| **Generate Spec** | "I want an app that manages recipes" | Agent asks questions, generates spec.md. *(Implemented -- spec_generate.go)* |

---

## 2. Architecture

### System Diagram

```
                    +------------------+
                    |   User / CLI     |
                    |  synroute chat   |
                    +--------+---------+
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
                    +--------+---------+
                    |     Router       |
                    | model selection  |
                    | circuit breaker  |
                    | health cache     |
                    +--------+---------+
                             |
         +-------------------+-------------------+
         |                   |                   |
  +------+------+    +------+------+    +-------+------+
  | Provider A  |    | Provider B  |    | Provider N   |
  | (any type)  |    | (any type)  |    | (any type)   |
  +-------------+    +-------------+    +--------------+
```

Providers are pluggable. Any number of backends can be configured. See Provider System in Section 1 for supported types.

### 2.1 Provider Chain

#### Escalation Engine

The router maintains a user-configured escalation chain -- an ordered list of provider levels. Each level has one or more providers. On failure at any level, the router escalates to the next.

```
Level 0: [provider-a, provider-b, provider-c]    (cheapest)
Level 1: [provider-d, provider-e]
...
Level N: [provider-x, provider-y]                 (most capable)
-> optional subscription/fallback providers
```

- Each level's providers rotate for cross-review and load distribution
- Escalation is monotonic -- once escalated, never downgrades within a session
- Circuit breakers, health checks, and rate limits trigger automatic escalation
- Sub-agents start at the parent's current escalation level (never below)

**Configuration:** Users define levels via environment variables:

```env
OLLAMA_CHAIN=model-a:14b,model-b:8b|model-c:120b,model-d|model-e:671b
#            ^--- Level 0 ---^      ^--- Level 1 ---^     ^-- Level 2 --^
```

Pipe (`|`) separates levels. Comma (`,`) separates models within a level.

#### Planner Slots

Two optional dedicated planner providers for the plan phase. These run the best available models for architectural planning -- separate from the coding escalation chain.

```env
OLLAMA_PLANNER_1=qwen3-coder:480b-cloud
OLLAMA_PLANNER_2=kimi-k2-thinking:cloud
```

Both planners run in parallel, and a third model merges their plans. *(#218)*

#### Multiple API Keys

```env
OLLAMA_API_KEYS=key1,key2,key3
```

Keys distributed round-robin across providers. Enables multiple paid subscriptions running concurrently without per-key rate limits.

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

Adding a new provider backend = implement this interface + register in startup.

### 2.2 Router Layer

#### Provider Selection Algorithm

1. **Filter candidates** -- only providers that support the requested model (or all if `auto`)
2. **Usage-based ranking** -- select provider with lowest usage percentage
3. **Circuit breaker check** -- skip providers with open circuit breakers
4. **Health check** -- verify provider is reachable (cached, with retry on failure)
5. **Fallback chain** -- if primary fails, try remaining healthy providers ordered by usage

#### Request Flow

```
Client Request
  -> routeChatRequest()
  -> Router.ChatCompletionWithDebug()
      |-- Refine vague prompts using conversation context
      |-- Touch session tracker
      |-- Retrieve memory (vector search, cross-session)
      |-- Inject retrieved memory into request
      |-- Preprocess: inject matched skill context
      |-- Store new messages in vector memory
      +-- Select provider (usage + circuit + health)
  -> tryProvidersWithFallback()
      |-- Try primary provider
      |-- On failure: record in circuit breaker, check rate limit
      |-- On success: reset circuit, update health cache, track usage
      +-- If primary fails: try fallback providers in usage order
  -> handleStall() (retry if response looks stalled)
  -> Store assistant response in memory
  -> Persist audit trail
```

### 2.3 Circuit Breakers

Each provider gets a circuit breaker instance backed by the database for persistence across restarts.

| State | Meaning |
|-------|---------|
| `closed` | Normal operation |
| `open` | Provider blocked until cooldown expires |
| `half_open` | Cooldown expired, next request probes -- success closes, failure re-opens |

**Thresholds (configurable via env):**

| Setting | Default | Env Var |
|---------|---------|---------|
| Failure threshold | 5 consecutive | `CIRCUIT_FAILURE_THRESHOLD` |
| Default cooldown | 5 minutes | `CIRCUIT_COOLDOWN_SECONDS` |

**Rate limit detection:** Checks error messages for `"rate limit"`, `"429"`, `"quota exceeded"`. When detected, circuit opens immediately with provider-specific cooldown.

### 2.4 Health Caching

Health checks cached to avoid burning API quota. Background monitor probes circuit-open providers for recovery.

| Setting | Default | Env Var |
|---------|---------|---------|
| Cache TTL | 2 minutes | `HEALTH_CACHE_TTL_SECONDS` |
| Monitor interval | 30 seconds | `HEALTH_MONITOR_INTERVAL_SECONDS` |

```
Request arrives
  -> Circuit breaker open?  -> Return unhealthy (no API call)
  -> Cache valid (< TTL)?   -> Return cached result
  -> Cache stale/miss?      -> Call provider.IsHealthy()
      -> If unhealthy: retry once with 6s timeout before marking unhealthy
      -> Update cache with result
```

Background monitor runs independently, resetting circuit breakers when providers recover.

### 2.5 Usage Tracking

Every successful request records token usage in the database. The usage tracker enforces a configurable auto-switch threshold -- when a provider exceeds the threshold of its daily quota, the router prefers other providers. Default threshold: 80%. Quotas are configurable per-provider via environment variables (e.g., `<PROVIDER>_DAILY_LIMIT`). No built-in defaults -- users set limits based on their subscriptions.

### 2.6 Stall Detection

If a provider takes longer than `STALL_TIMEOUT_SECONDS` (default: 180) and returns fewer than 50 characters, the router retries with a continuation prompt.

### 2.7 Audit Trail

Every request generates audit records in the database:

- **`request_audit`** -- session, selected/final provider, model, memory stats, success/error
- **`provider_attempt_audit`** -- each provider attempt with index, success, error details

### 2.8 Memory Layer

Three subsystems provide persistent, searchable memory:

#### VectorMemory

Database-backed store for compacted conversation messages with embeddings for semantic search.

- **Hybrid search:** vector similarity (cosine distance, min 0.15 threshold) + lexical fallback (TF-IDF)
- **Always fetches 4 most recent messages** for conversational continuity
- **Token budget:** default 4096, max 8192 for semantic results
- **Local embeddings:** TF-IDF feature hashing -- zero API cost, works offline
- **Optional:** OpenAI embeddings when API key configured
- **Planned:** ONNX bundled model (all-MiniLM-L6-v2) for better semantic quality without API (#68)

#### ToolOutputStore

Preserves full tool outputs while conversation gets summaries.

- **All tool outputs stored** regardless of size; secrets scrubbed before storage
- **Conversation gets summary** if output > 2KB, with `[full output: ref:N]` back-reference
- **Spec storage:** large initial messages (>2KB, like spec files) stored as `tool_name="spec_load"` for sub-agent recall

#### UnifiedSearcher

Merges both stores behind a single interface for the recall tool.

- **Three retrieval modes:**
  - `recall(id=47)` -- fetch full output by ID
  - `recall(query="auth middleware")` -- semantic search across VectorMemory + ToolOutputStore
  - `recall(tool_name="bash")` -- filter by tool name
- **Cross-session:** searches current session, then all parent sessions via `ParentSessionIDs`

### 2.9 Skill System

54+ embedded skills parsed from YAML frontmatter in Markdown files, compiled via `go:embed`.

#### Trigger Matching

Three strategies, in precedence:

1. **Compound triggers** (`+` syntax): `go+handler` requires both in text
2. **Substring matching**: for multi-word triggers and special characters
3. **Word-boundary matching**: for short ambiguous words (2 chars) -- requires exact word match

#### Language-Aware Filtering

When `ProjectLanguage` is set (from environment detection or spec), skills with a `Language` field that doesn't match are excluded. Language-agnostic skills (empty Language field) always pass.

#### Spec Deferral

Skills are reference patterns, not mandates. When a spec defines architecture, package structure, or scope, spec directives override conflicting skill patterns. All architecture-prescribing skills include spec-deferral headers.

#### Skill Phases

| Phase | Order | Purpose |
|-------|-------|---------|
| `analyze` | 0 | Understand the problem, detect patterns |
| `implement` | 1 | Write code, build features |
| `verify` | 2 | Run tests, validate output |
| `review` | 3 | Code review, quality audit |

When multiple skills match, `BuildSkillChain()` sorts by phase order, then alphabetically within phase.

### 2.10 Database Layer

Two independent database backends. User selects via configuration.

| Backend | Driver | Use Case | Status |
|---------|--------|----------|--------|
| SQLite | `go-sqlite3` (CGo) | Single user, local, zero config | Implemented |
| PostgreSQL | `pgx` | Multi-user, team, cloud, concurrent writes | Planned (#85) |

Both backends implement the same schema (8 tables -- see Appendix D). WAL mode enabled for SQLite concurrent access.

### 2.11 Environment Detection

Automatic language and toolchain detection from project files:

| Config File | Language | Build Commands |
|-------------|----------|---------------|
| `go.mod` | Go | `go build`, `go test` |
| `pom.xml` / `build.gradle` | Java | `mvn compile`, `mvn test` (or gradle) |
| `package.json` | JavaScript/TypeScript | `npm install`, `npm test` |
| `Cargo.toml` | Rust | `cargo build`, `cargo test` |
| `requirements.txt` / `pyproject.toml` | Python | `pip install`, `pytest` |
| `Gemfile` | Ruby | `bundle install`, `rake test` |
| `CMakeLists.txt` | C++ | `cmake`, `make` |
| + 13 more | Various | Auto-detected |

Detection sets `ProjectLanguage`, resolves build commands, checks for missing tools, and injects setup instructions into the system prompt.

### 2.12 Chat Architecture

*(Core implemented in `internal/agent/repl.go`; rich UI planned -- #229, #231)*

The chat mode operates as a conversational interface where the frontier model responds directly to the user. No pipeline is triggered unless the user's intent requires work.

```
User Input
  -> Frontier model reads message
  -> Intent detection (LLM-native, not keyword matching)
  -> If conversational (chat/explain/research): respond directly
  -> If work needed (build/fix/review/deploy): trigger pipeline
  -> Stream response to terminal
  -> Persist to session (database)
```

**Session persistence:** Each conversation is a session with a unique ID. Messages, tool outputs, and agent state persist to the database. Sessions can be resumed (`--resume`) or listed.

**Streaming:** Responses stream token-by-token to the terminal. Tool calls render inline with semantic colors. *(Rich rendering planned -- #231)*

**REPL commands (current):** `/exit`, `/clear`, `/model`, `/tools`, `/history`, `/agents`, `/budget`

**Planned:**
- File attachment via `@filename` references (#228)
- Semantic color system with accessibility presets (#231)
- Session list/switch/delete (#84)
- Stateful per-message phase routing (#235)

### 2.13 Three-Tier Model Routing

*(Planned -- #233)*

Three distinct routing tiers within a single session:

| Tier | Purpose | Model Selection | When |
|------|---------|----------------|------|
| **Frontier** | Talk to user, understand intent, explain, ask questions | Best available (most capable) | Every user-facing turn |
| **Mid** | Code review, complex fixes, architecture decisions | Large coders at mid-range levels | When independent review is needed |
| **Cheap** | Write code, run builds, file ops, parallel sub-agents | Cheapest adequate (escalation chain) | When pipeline is triggered |

The user always talks to the frontier model. When work is needed, the frontier model delegates to the appropriate tier based on task complexity.

**Current state:** Not yet implemented. All turns use the same provider selection. See Section 3.3 for the full three-tier design.

### 2.14 Intent Detection

*(Planned -- #232, #235; partial implementation in `internal/agent/intent.go`)*

The agent uses the LLM itself to determine what phase to run -- not keyword matching. The frontier model reads the user's message and naturally decides what to do.

**Heuristic fallbacks** (when LLM intent isn't clear):
- Project has spec.md + no code -> full pipeline (build from spec)
- Project has existing code + tests -> start at review/implement, skip plan
- Project has existing code, no tests -> implement tests
- Empty project, no spec -> conversational (ask what to build)

These heuristics are in `DetectPipelineEntry()`, used as a starting point. The LLM overrides them naturally.

### 2.15 Pipeline System

The 6-phase pipeline is triggered when the agent decides work is needed. Not every message runs the pipeline.

#### Software Pipeline

| Phase | Name | What It Does | MinToolCalls | Sub-Agent |
|-------|------|-------------|:---:|:---:|
| 1 | **plan** | Spec perception, task decomposition, acceptance criteria | 1 | Parallel (2 planners + merge) |
| 2 | **implement** | Write code, create files, run builds | 1 | Parallel (3 coders + cross-review) |
| 3 | **self-check** | Verify own work against acceptance criteria | 2 | No |
| 4 | **code-review** | Independent reviewer (fresh agent, no shared context) | 2 | Yes |
| 5 | **acceptance-test** | End-to-end test from user's perspective | 1 | Yes |
| 6 | **deploy** | Package, document, finalize | 0 | No |

#### Data Science Pipeline

| Phase | Name | What It Does | Key Tools |
|-------|------|-------------|-----------|
| 1 | **eda** | Exploratory data analysis | bash (run scripts), file_write (notebooks/reports) |
| 2 | **data-prep** | Feature engineering, cleaning | bash (data processing), file_write (pipeline code) |
| 3 | **model** | Model training, evaluation | bash (training runs), file_read (metrics) |
| 4 | **review** | Results review, findings documentation | file_read (outputs), recall (prior experiments) |
| 5 | **deploy** | Model deployment, containerization | bash (build), file_write (Dockerfile/config) |
| 6 | **verify** | Production verification | bash (inference tests), grep (log analysis) |

**Pipeline customization (planned):** Both pipelines are defaults. Users should be able to define custom pipelines with their own phases, tool restrictions, and quality gates. *(No issue filed yet)*

#### Phase Transitions

- **Pass signals:** LLM says `IMPLEMENT_COMPLETE`, `SELF_CHECK_PASS`, etc.
- **Fail signals:** `NEEDS_FIX`, `SELF_CHECK_FAIL` -> fail-back to previous phase
- **Turn cap:** 25 turns per phase maximum -> force-advance
- **Cycle cap:** 3 fail-back cycles per review phase -> force-advance
- **Escalation:** Provider escalated on budget exhaustion, stall, or loop detection

#### Quality Gates

- **MinToolCalls:** Phase can't pass without minimum tool calls (prevents rubber-stamping)
- **Verification gate:** Runs actual build/test commands; exit code = 0 required
- **Plateau detection:** If verification score stops improving for 2+ retries -> escalate
- **Regression detection:** If compilation errors increase after changes -> inject warning

### 2.16 Container Isolation Architecture

*(Planned -- Epic #211, Stories #212-217)*

See Section 7 for full details. Key architectural decisions:

- **Docker and Podman** as two independent, equal container runtime options
- **Sibling container pattern** -- orchestrator creates containers via host socket, not nested
- **Podman:** pod model (shared network namespace, localhost communication)
- **Docker:** `--network=container:<primary>` for shared namespace
- **Named volumes** for workspace sharing between session and sub-agent containers
- **Warm container pool** for sub-second startup

### 2.17 Session Continuity Architecture

*(Planned -- Epic #220, Stories #221-226)*

Cross-session state persistence using the hybrid approach (DB + synroute.md):

```
Session A ends
  -> Write project_continuity row to DB (phase, build status, file manifest)
  -> Write synroute.md with YAML frontmatter (session_id, phase, status)

Session B starts on same project
  -> Query DB for last session on this project_dir
  -> If found: inject summary into system prompt, set ParentSessionIDs for recall
  -> If DB unavailable: read synroute.md frontmatter as fallback
  -> Skip completed phases, start where previous session left off
```

- **DB** is the primary store -- queryable, relational, supports multi-user
- **synroute.md** is the portable fallback -- travels with the project between machines
- **ParentSessionIDs** links sessions for cross-session recall via UnifiedSearcher
- **Local embeddings** make semantic search across sessions free (zero API cost)

### 2.18 Fallback Strategy

Fallback applies to runtime failures — when something that was working stops working. This is separate from configuration options (choosing Docker vs Podman is a config choice, not a fallback).

#### Runtime Fallbacks (automatic)

| Layer | Primary | Fallback | Condition |
|-------|---------|----------|-----------|
| **Provider** | Current level providers | Next escalation level | All providers at level fail |
| **Model** | Cheapest adequate | Bigger model at higher level | Task too complex, budget exhausted |
| **Health check** | Single 3s check | Retry with 6s timeout | First check times out |
| **Circuit breaker** | Closed (normal) | Half-open probe after cooldown | Configurable failures open circuit |
| **Sub-agent** | Run at current level | Escalate parent, retry higher | Budget exhausted |
| **Target provider** | Specific provider | Fall through to escalation chain | Circuit open or failure |
| **Session continuity** | DB query | synroute.md file | DB unavailable |
| **Embeddings** | Configured embedding provider | Local TF-IDF hash | Provider unavailable |
| **Skill filtering** | Language-filtered set | Unfiltered set | Filtering removes all skills |
| **Spec recall** | ToolOutputStore (explicit) | VectorMemory (semantic) | Explicit search misses |

#### Configuration Options (user choice, not fallback)

These are independent options the user selects based on their environment — not automatic degradation:

| System | Options | Configured Via |
|--------|---------|---------------|
| **Container runtime** | Podman, Docker, none (worktrees) | `CONTAINER_RUNTIME` or guided setup |
| **Database** | SQLite, PostgreSQL | `DB_TYPE` or guided setup |
| **Embeddings** | ONNX (planned), OpenAI API, local TF-IDF | API key presence or config |
| **Isolation** | Containers, worktrees, temp directories | `CONTAINER_RUNTIME` + `--worktree` flag |

No layer silently fails. Every fallback is logged.

---

## 3. Agent Capabilities

*Section 2 describes **how** these capabilities are architecturally designed. This section describes **what** they do from the user's perspective and their current implementation status.*

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

See Section 2.15 for the pipeline architecture. From the user's perspective, every substantial task runs through a multi-phase pipeline. Two pipeline types are auto-detected from matched skills:

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

### 3.3 Model Tier Routing (Planned, #233)

See Section 2.13 for the routing architecture. From the user's perspective, the planned design splits work across three tiers:

| Tier | Escalation Levels | Models | Handles |
|------|:---:|--------|---------|
| **Frontier** | Top of user's chain | Most capable models available | User conversation, planning, orchestration, final review |
| **Mid** | Middle of user's chain | Capable but not most expensive | Code review by different model, complex fixes, architecture |
| **Cheap** | Bottom of user's chain | Fast, inexpensive | Parallel coding, file generation, builds, tests |

Tier-to-level mapping is user-defined based on their escalation chain configuration. The product assigns tiers based on position in the chain, not hardcoded model sizes.

- Parent agent always uses frontier tier for user-facing turns
- Child implement agents default to cheap tier
- Code-review and acceptance-test use mid or frontier tier (different model than implementer)
- Pipeline phases map to tiers: plan=frontier, implement=cheap, self-check=cheap, code-review=mid/frontier, acceptance-test=frontier
- Extend `SpawnChild` config to accept `ModelTier` (`cheap`, `mid`, `frontier`, `auto`)

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

See Section 2.17 for the session continuity architecture. From the user's perspective:

When a new agent session starts on a project where a previous session left work, the agent picks up where it left off. The agent loads prior phase progress, build status, and file context so it can skip completed phases and resume from the last checkpoint. A portable `synroute.md` file travels with the project to bootstrap context on new machines when the database is unavailable.

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

**Current:** Turn and token budgets work well for fixed-subscription providers.

**Planned:** Cost-based budgets for pay-per-token APIs (OpenAI, Anthropic, etc.) -- set a dollar limit (`--cost-limit 5.00`) instead of token count. Requires per-provider cost tracking based on model pricing. *(No issue filed yet)*

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

MaxMessages should be auto-detected from the model's reported context window, not hardcoded per family. The system queries the model's capabilities and sets limits accordingly.

**Current (hardcoded defaults):**

| Model Family | MaxMessages | Context Window |
|-------------|-------------|----------------|
| Gemini | 500 | 1M+ tokens |
| Claude | 400 | 200K tokens |
| DeepSeek | 300 | 128K tokens |
| Default | 200 | Conservative |

**Planned:** Auto-detect context window from model metadata (via `/models` endpoint or provider config), calculate MaxMessages dynamically. Users can override via config. *(No issue filed yet)*

### 3.14 Auto-Context Injection

After any compaction event, `buildMessages()` automatically injects retrieved context before conversation messages. Query is the last user message. Budget capped at 2048 tokens.

**When the budget cap is reached**, the injected context is only a subset of what's in the database. The agent must be told what else is available so it knows to use the `recall` tool for deeper queries. The injection should include:

- The 2048-token context sample (most relevant messages)
- A summary of what's in the DB but NOT injected: "N prior messages, M tool outputs, K files read — use `recall(query=...)` to access"
- The recall tool's capabilities: search by keyword, tool name, or semantic query

Without this, the model assumes the injected context is complete and never calls recall. *(Current implementation does not include the DB summary — needs fix)*

### 3.14b Memory Flow: How State, Compaction, and Auto-Context Interact

These three systems (3.12, 3.13, 3.14) form a pipeline that preserves context across long sessions:

```
Session starts
  -> LoadState (3.12): restore prior conversation if --resume
  -> Agent runs, conversation grows
  -> MaxMessages exceeded (model-aware limit)
      -> BeforeTrimHook (3.13): store oldest messages in VectorMemory BEFORE dropping
      -> Trim conversation to fit context window
  -> Pipeline phase completes
      -> Phase compaction (3.13): store N-20 oldest to DB, keep 20 recent + summary
  -> Next LLM turn
      -> Auto-Context Injection (3.14): query VectorMemory for relevant prior context
      -> Inject as synthetic messages before conversation
      -> LLM sees: [system] [injected context] [recent conversation]
  -> Session ends
      -> SaveState (3.12): persist full agent state for future --resume
      -> Write synroute.md: portable state summary
```

**Key insight:** Messages are never permanently lost. They move from conversation -> VectorMemory (with embeddings) -> queryable via recall tool or auto-injection. The local TF-IDF embedding model makes all searches free.

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
- LLM-generated criteria supplement spec criteria -- they do not replace them
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

### 3.16 Intent-to-Phase Mapping

How agent intents (from Section 1) map to pipeline behavior. Intents are NOT single phases -- each triggers a mini-pipeline appropriate to the task:

| Intent | Pipeline Flow | Frontier Model | Notes |
|--------|-------------|:-:|-------|
| **Chat** | respond directly | Yes | No tools, no pipeline |
| **Explain** | read code -> explain | Yes | Uses file_read/grep but no pipeline phases |
| **Research** | search -> analyze -> report | Yes | Web search + local analysis. *(Needs #227)* |
| **Plan** | perceive spec -> decompose -> criteria -> output | Yes | Generates plan + acceptance criteria |
| **Generate Spec** | converse -> clarify -> draft -> refine -> output | Yes | Back-and-forth with user. *(Implemented)* |
| **Build** | plan -> implement -> self-check -> code-review -> acceptance-test -> deploy | Plan=frontier, code=cheap | Full 6-phase pipeline |
| **Fix** | diagnose -> implement fix -> verify -> update tickets | Cheap | Reads errors, patches specific files, updates issue tracker |
| **Review** | read code -> assess -> report findings -> create issues/tickets | Frontier | Produces actionable findings, not just pass/fail. May trigger Fix intent for found issues |
| **Test** | analyze code -> write tests -> run -> iterate -> verify | Cheap | Iterates until tests pass |
| **Refactor** | read code -> plan changes -> implement -> self-check | Cheap | No new features. Preserves behavior. |
| **Deploy** | detect platform -> generate config -> verify | Cheap | Dockerfile, Containerfile (Podman), CI, release |
| **Migrate** | analyze source -> plan target -> implement -> cross-verify | Plan=frontier, code=cheap | Verifies against both source and target languages |

**Key:** Intents are not rigidly mapped. The frontier model reads the user's message and naturally selects the appropriate flow. The table above is the intended design -- the LLM isn't given this table, it behaves this way through system prompt guidance. *(Full implementation planned -- #232, #235)*

**Issue tracker integration (planned):** Review and Fix intents should interact with external issue trackers (GitHub Issues, Jira) to create/update tickets for findings and fixes.

### 3.17 Spec Generation

*(Implemented -- `internal/agent/spec_generate.go`)*

When the user describes what they want in natural language, the agent has a **conversation** to understand requirements before generating a structured spec.md:

```
User: "I want an app that manages recipes with ingredients and categories"
  -> Agent: "What tech stack? (Go, Python, Java, etc.)"
  -> User: "Go"
  -> Agent: "Do you need authentication?"
  -> User: "Not sure yet, TBA"
  -> Agent: "API or web UI?"
  -> User: "REST API for now"
  -> Agent generates spec.md with:
    - Overview, Scope (IN/OUT)
    - Architecture (package structure, design patterns)
    - Data model (entities, relationships)
    - API specification
    - Acceptance criteria
    - Build & run instructions
    - TBA items clearly marked for future decisions
```

The agent handles uncertainty gracefully -- "idk", "not sure", "TBA" are valid answers that get marked in the spec as open decisions. The spec is a living document, not a final contract until the user confirms.

The generated spec becomes the input for a Build intent -- full pipeline from spec to working project.

### 3.18 Worktree Isolation

*(Implemented -- `internal/worktree/`)*

`synroute chat --worktree` creates a managed git worktree for the session, isolating changes from the main working tree.

| Setting | Default | Notes |
|---------|---------|-------|
| TTL | 24 hours | Auto-cleanup on expiry |
| Size cap (total) | 10 GB | All worktrees combined |
| Size cap (per tree) | 2 GB | Single worktree |
| Cleanup interval | 5 minutes | Background goroutine |

Changes can be merged back to main branch or discarded. Worktrees are git-native -- full history, branches, commits available.

**Relationship to container isolation (Section 7):** Worktree isolation and container isolation coexist:
- **Worktrees** -- lightweight, git-native, for local single-user development. No overhead.
- **Containers** -- heavyweight, full OS isolation, for multi-user or untrusted code. *(Planned -- Epic #211)*
- In containerized mode, each container gets its own worktree. The worktree system works inside containers.
- Users don't choose between them -- local dev uses worktrees automatically; hosted/multi-user mode adds containers on top.

### 3.19 Permission Model

*(Implemented -- `internal/tools/permissions.go`)*

Three permission modes control what the agent can do without asking:

| Mode | Behavior | Use Case |
|------|----------|----------|
| `interactive` | Prompt before write/dangerous ops | Default for CLI chat |
| `auto_approve` | Allow all without prompting | Unattended one-shot |
| `read_only` | Deny all write operations | Safe exploration, review |

Tool categories and approval:

| Category | `interactive` | `auto_approve` | `read_only` |
|----------|:---:|:---:|:---:|
| `read_only` (file_read, grep, glob, recall) | Auto | Auto | Auto |
| `write` (file_write, file_edit, git) | Prompt | Auto | Deny |
| `dangerous` (bash) | Prompt + confirm | Auto | Deny |
| `agent` (delegate, handoff) | Auto | Auto | Auto |

### 3.20 MCP Server Mode

*(Implemented -- `internal/mcpserver/`)*

Exposes synapserouter's tools as an MCP (Model Context Protocol) server for external clients.

```bash
synroute mcp-serve                       # Standalone MCP server
synroute mcp-serve --addr :9090          # Custom port
SYNROUTE_MCP_SERVER=true synroute serve  # MCP on main server
```

Endpoints: `/mcp/initialize`, `/mcp/tools/list`, `/mcp/tools/call`

Enables other AI agents or IDE extensions to use synapserouter's tool execution without running the full agent loop.

### 3.21 Environment Best Practices

*(Implemented -- `internal/environment/best_practices.go`)*

Per-language checks applied automatically when a project is detected:

| Language | Checks |
|----------|--------|
| Go | go.mod, go.sum, .gitignore |
| Python | Virtual env, pinned deps, version compatibility |
| JavaScript | Lockfile, Node version |
| Rust | Cargo.lock, edition |
| Java | Build wrapper (mvnw/gradlew), JDK version |

Best practices injected as warnings. Agent fixes violations before proceeding.

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

Query complexity scoring: simple queries use cheap-tier models (fast), complex queries use mid/frontier-tier (thorough), code generation routes to code-specialized models.

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
| `ACTIVE_PROFILE` | `personal` | Active profile name |
| `PORT` | `8090` | HTTP server port |
| `DB_PATH` | `synroute.db` | SQLite database path |

#### Provider Configuration (Examples)

The product does not assume which providers you use or what limits apply. Configure based on your environment. Any supported provider type works in any profile. The examples below show Ollama-compatible and Vertex AI configurations.

**Ollama-Compatible Provider:**

| Variable | Default | Purpose |
|----------|---------|---------|
| `OLLAMA_API_KEY` | -- | Single API key for Ollama-compatible provider |
| `OLLAMA_API_KEYS` | -- | Multiple API keys (comma-separated, round-robin) |
| `OLLAMA_BASE_URL` | -- | Base URL for Ollama-compatible provider |
| `OLLAMA_CHAIN` | -- | Model chain (levels separated by `\|`, models by `,`) |
| `OLLAMA_PLANNER_1` | -- | Planner model slot 1 |
| `OLLAMA_PLANNER_2` | -- | Planner model slot 2 |

**Subscription Providers (Optional):**

Any subscription provider can be added as a fallback. These are optional and work with any profile configuration.

| Variable | Default | Purpose |
|----------|---------|---------|
| `SUBSCRIPTIONS_DISABLED` | `false` | Disable all subscription providers |
| `SUBSCRIPTION_PROVIDER_ORDER` | `gemini,openai,anthropic` | Fallback order after primary providers |

**Vertex AI Provider (Example):**

| Variable | Default | Purpose |
|----------|---------|---------|
| `VERTEX_CLAUDE_PROJECT` | -- | GCP project for Claude |
| `VERTEX_CLAUDE_REGION` | `us-east5` | Vertex AI region for Claude |
| `VERTEX_GEMINI_PROJECT` | -- | GCP project for Gemini |
| `VERTEX_GEMINI_LOCATION` | `global` | Vertex AI location for Gemini |
| `VERTEX_GEMINI_SA_KEY` | -- | Service account key file for Gemini |

#### Usage Limits

User-configured. Set based on your subscription limits. No built-in defaults -- users set limits based on their provider subscriptions.

| Variable | Default | Purpose |
|----------|---------|---------|
| `<PROVIDER>_DAILY_LIMIT` | -- | Daily token limit per provider (e.g., `OLLAMA_CLOUD_DAILY_LIMIT`, `GEMINI_DAILY_LIMIT`) |

#### Router

| Variable | Default | Env Var |
|---------|---------|---------|
| Health cache TTL | 2 minutes | `HEALTH_CACHE_TTL_SECONDS` |
| Health monitor interval | 30 seconds | `HEALTH_MONITOR_INTERVAL_SECONDS` |
| Circuit failure threshold | 5 consecutive | `CIRCUIT_FAILURE_THRESHOLD` |
| Circuit cooldown | 5 minutes | `CIRCUIT_COOLDOWN_SECONDS` |

#### Agent

| Variable | Default | Purpose |
|----------|---------|---------|
| `STALL_TIMEOUT_SECONDS` | `180` | Stall detection timeout |

**Planned:** `--cost-limit <dollars>` flag for API cost budgets on pay-per-token providers (see Section 3.10). Sets a dollar ceiling instead of token count. Requires per-provider cost tracking based on model pricing. *(No issue filed yet)*

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

Switching profiles reinitializes providers based on the target profile's configuration. Profiles are user-defined -- any provider type can be configured in any profile.

### 8.4 Model Defaults

| Context | Default Model |
|---------|---------------|
| Ollama-compatible | Per level, from `OLLAMA_CHAIN` |
| Gemini (subscription) | `gemini-3-flash-preview` |
| Vertex Gemini | `gemini-3.1-pro-preview` |
| Vertex Claude | Model names without dates (e.g., `claude-sonnet-4-6`) |
| Codex | SSE streaming via `/responses` endpoint |
| Gemini 2.5+ | Thinking tokens from output budget, min 1024 maxOutputTokens |

### 8.5 Guided Setup (Planned, Epic #247)

First-time configuration via interactive wizard instead of manual `.env` editing:

```bash
synroute setup
```

**Scope:** Covers ALL aspects of configuration in a single guided session:

| Step | What It Configures |
|------|-------------------|
| **LLM Providers** | Select providers, enter API keys/credentials, configure escalation chain |
| **MCP Servers & Skills** | Discover and connect MCP servers. OAuth flow or API key per service. Skills auto-discovered. |
| **Integrations** | GitHub, Jira, Slack, etc. — user picks auth method (OAuth, API key, PAT) |
| **Container Runtime** | Auto-detect Docker/Podman socket, test container creation |
| **Database** | SQLite (default) or PostgreSQL (connection string) |

**UX requirements:**
- Single restart at end of all configuration — NOT per-change
- Re-runnable (`synroute setup` on existing config shows current settings, lets user modify)
- Validates connections before saving
- Applies to both chat and code modes
- Respects `NO_COLOR` for accessibility
- Config stored in `.env` (env vars) + `settings.json` (MCP/integration config)

**Stories:** #248 (LLM providers), #249 (MCP/skills), #250 (integrations), #251 (containers/DB), #252 (re-configuration), #253 (terminal UI)

---

## 9. Acceptance Criteria

### 9.1 Provider Routing

- [ ] **AC-R1:** Request with `model: "auto"` routes to lowest-usage healthy provider
- [ ] **AC-R2:** When L0 providers all fail, request automatically escalates to L1 within same API call
- [ ] **AC-R3:** Full escalation L0 through L6 then subscription fallback works end-to-end
- [ ] **AC-R4:** Circuit breaker opens after configurable consecutive failures (default 5); closes on successful probe after cooldown
- [ ] **AC-R5:** Rate limit error with "reset after Ns" sets cooldown to N+5 seconds
- [ ] **AC-R6:** Health check results cached for configurable TTL (default 2 minutes via `HEALTH_CACHE_TTL_SECONDS`); no API call during cache validity
- [ ] **AC-R7:** Multiple API keys distribute across providers in round-robin
- [ ] **AC-R8:** Profile switch reinitializes providers based on target profile configuration
- [ ] **AC-R9:** Stall detection retries when response <50 chars after 180s
- [ ] **AC-R10:** Usage tracking enforces configurable auto-switch threshold (default 80%)

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

- [ ] **AC-S1:** `ParseSkillsFromFS` loads all skills from embedded `.md` files at startup
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
- [ ] **AC-A5 (planned):** Three-tier model routing: parent=frontier, children=cheap, reviewers=mid (#233)

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
