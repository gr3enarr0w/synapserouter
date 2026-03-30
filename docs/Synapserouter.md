---
title: Synapserouter
aliases: [synroute, home]
updated: 2026-03-26
status: active-development
---

# Synapserouter

A Go-based LLM proxy router and autonomous coding agent. It routes requests across multiple LLM providers using a cost-optimized escalation chain — starting with the cheapest adequate model and escalating only when needed. It includes a full coding agent that can plan, implement, test, and review code using built-in tools.

> **This is an active project, not a finished product.** Core routing, agent execution, and code mode TUI work. Several major features (chat backend, real embeddings) are still ahead. See [[Known Gaps]] for what's broken and [[Requirements]] for where it's going.

---

## Why This Exists

1. **No vendor lock-in.** Use Ollama Cloud, Gemini, Codex, Claude Code, and Vertex AI through a single API. Swap providers without changing client code.
2. **Cost-optimized model selection.** A 14B model handles most tasks. A 671B model handles the hard ones. The router figures out which is which — you don't pay for frontier models when a small one suffices.
3. **Autonomous coding agent.** Give it a task. It plans, writes code, runs tests, reviews its own work, and iterates. It builds programs and tools — it does not just describe what it would do.
4. **Unlimited context.** Conversation history and tool outputs persist to SQLite. Long sessions don't lose early context. A recall tool retrieves relevant history on demand.

---

## How It Works

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
    | 21 models     |   | claude-code   |   | Claude+Gemini  |
    +---------------+   +---------------+   +----------------+
```

### The Escalation Chain

The router sends each request to the cheapest provider level first. If that level fails (model too small, rate-limited, circuit breaker open), it escalates to the next level automatically. No user intervention required.

Levels are configured dynamically via `OLLAMA_CHAIN` env var (pipe-separated levels, comma-separated models within a level). Escalation proceeds L0 → L1 → ... → LN → subscription fallback.

See [[Provider Chain]] for the full model list and escalation logic.

### The Agent

The agent is a **tool builder, not a tool runner**. When given a task that involves repeated operations, it writes a program that does the work — the user gets a runnable tool, not a series of side effects.

Every substantial task runs through the **pipeline**: Plan -> Implement -> Self-Check -> Code Review -> Acceptance Test. Code review and acceptance testing use fresh sub-agents with no shared context, so they can catch mistakes the implementing agent is blind to.

The agent has 10 built-in tools (bash, file read/write/edit, grep, glob, git, web_search, web_fetch, notebook_edit) plus agent-to-agent delegation (delegate, handoff). Web search supports DuckDuckGo, Tavily, and SearXNG backends. Web fetch is SSRF-safe. Notebook support renders `.ipynb` cells on read and edits by cell index. See [[User Guide]] for CLI usage and [[Architecture Overview]] for internals.

### Skills

54 embedded skills across 20+ languages, parsed from YAML frontmatter in Markdown files. When a user's message matches a skill's triggers, that skill's domain expertise is injected into the system prompt automatically. No configuration needed. 13 high-risk skills include spec-deferral headers so project specs override skill defaults.

Adding a skill means dropping one `.md` file into `internal/orchestration/skilldata/` and rebuilding. See [[Skill System]].

### Memory

Tool outputs and conversation history persist to SQLite. The agent uses a recall tool to retrieve relevant context from earlier in the session. Hallucination detection tracks facts the LLM asserts and flags contradictions.

See [[Memory System]] and [[Hallucination Detection]].

---

## Current State (March 2026)

| Component | Status |
|---|---|
| Provider routing (Ollama Cloud dynamic chain) | Working |
| Circuit breakers + health caching | Working |
| Agent tool execution (10 built-in tools + 2 agent tools) | Working |
| Web search (DuckDuckGo, Tavily, SearXNG) + web fetch (SSRF-safe) | Working |
| Notebook support (file_read renders cells, notebook_edit by index) | Working |
| File attachments (@file, @dir/ with path traversal protection) | Working |
| Agent pipeline (plan/implement/verify/review) | Working |
| Token streaming via SSE (StreamingProvider, Ollama) | Working |
| Text-based tool call parser (5 formats for Ollama models) | Working |
| Loop/stall detection (all modes) + completion signal detection | Working |
| Response truncation at 4000 chars | Working |
| Memory system (zero-loss, unified recall) | Implemented |
| Hallucination detection (fact tracking) | Implemented |
| Skills (54 embedded, auto-matching) | Working |
| Spec compliance (constraint extraction, tool protection) | Working |
| Dynamic turn caps (15/25/40 based on spec complexity) | Working |
| Review divergence detection (ReviewStabilityTracker) | Working |
| Regression tracking (RegressionTracker, compilation errors) | Working |
| Background health monitor (auto-recovery) | Working |
| Sub-agent delegation + handoffs | Working |
| Worktree isolation | Working |
| MCP server mode | Working |
| OpenAI-compatible API | Working |
| Code mode TUI (status bar, scroll regions, keyboard shortcuts) | Working |
| Permission prompting (y/n/a via /dev/tty, chat mode) | Working |
| Work profile 3-tier chain (haiku→sonnet+gemini→opus+gemini) | Working |
| Configurable conversation tier (SYNROUTE_CONVERSATION_TIER) | Working |
| models.corp OpenAI-compatible provider (work profile, optional) | Working |
| Multi-mode Ctrl-C (cancel LLM call / double-press exit) | Working |
| Chat backend for LibreChat (Epic 7) | Not started |
| Real embeddings (ONNX) | Not started -- using TF-IDF hash vectors |
| MCP client | Not started |
| Multi-user / Postgres | Not started |

See [[Known Gaps]] for specific issues found during testing.

---

## Roadmap

| Phase | Focus | Status |
|---|---|---|
| 1 | Core router + agent — route requests, build projects, code mode TUI | **Current** |
| 2 | Chat backend API — LibreChat integration, smart model selection | Next |
| 3 | Rich content — document creation, file processing | Planned |
| 4 | Scale — Postgres, multi-user, branding | Planned |
| Beyond | Real embeddings, MCP client, wave-based parallel execution, job queue | Future |

See [[Requirements]] for detailed acceptance criteria per phase.

---

## Documentation Map

### Architecture
- [[Architecture Overview]] — Components, data flow, system design
- [[Provider Chain]] — Dynamic escalation chain, circuit breakers, health caching

### Features
- [[Memory System]] — SQLite persistence, recall tool, conversation compaction
- [[Hallucination Detection]] — Fact tracking, contradiction detection, auto-correction
- [[Skill System]] — 54 embedded skills, trigger matching, YAML frontmatter
- [[Known Gaps]] — What's broken, what's missing, what needs work

### Guides
- [[User Guide]] — CLI commands, profiles, agent usage, worktrees
- [[Dev Guide]] — Building, testing, adding skills, contributing

### Reference
- [[API Endpoints]] — HTTP API, diagnostic endpoints, MCP server
- [[CLI Commands]] — All commands with flags and examples

### Specs
- [[Requirements]] — Acceptance criteria per epic and phase
