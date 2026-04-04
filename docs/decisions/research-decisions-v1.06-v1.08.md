# Research-Backed Decisions: v1.06 – v1.08

All technical decisions made during v1.06 through v1.08 development, with cited research and rationale. For the full intent routing analysis, see `intent-routing-design.md`.

**Date range:** 2026-04-02 to 2026-04-04
**Authors:** Clark Everson, Claude Opus 4.6

---

## 1. Pure Go SQLite Driver

**Decision:** Replace `github.com/mattn/go-sqlite3` (CGo) with `modernc.org/sqlite` (pure Go).

**Problem:** CGo dependency prevented cross-compilation. Release workflow couldn't build Linux binaries on macOS, Windows builds impossible without cross-compilers. Building for 6 platforms required 6 separate runners.

**Research:**
- DataStation benchmark (2022): modernc.org/sqlite is ~2x slower on INSERTs, 10-20% slower on SELECTs vs mattn/go-sqlite3. For small datasets (<100K rows), difference is negligible.
- Grafana (issue #87327): Actively migrating from mattn/go-sqlite3 to modernc.org/sqlite. Reason: "eliminate cross-compilation considerations."
- Gogs (issue #7882): Migrated to modernc.org/sqlite, running in production 2+ years. "No identified issues." Concurrency performance can be better without CGo overhead.
- mattn/go-sqlite3 status: Archived/unmaintained as of 2025.

**Why this choice:** synroute's SQLite usage is agent sessions, circuit breakers, search metrics — small datasets, ~100-1000 rows. The 2x INSERT slowdown is irrelevant. Eliminating CGo enables `CGO_ENABLED=0` builds for all 6 platforms with zero cross-compiler dependencies. Database files are binary-compatible — no migration needed.

**Alternatives considered:**
- Keep CGo, use zig as cross-compiler — adds toolchain complexity for every contributor.
- Use PostgreSQL only — eliminates SQLite advantage (zero-config single-user).
- Use BoltDB/BadgerDB — would require rewriting all SQL queries.

**Sources:**
- [SQLite in Go, with and without cgo](https://datastation.multiprocess.io/blog/2022-05-12-sqlite-in-go-with-and-without-cgo.html) (DataStation, 2022)
- [Grafana #87327](https://github.com/grafana/grafana/issues/87327)
- [Gogs #7882](https://github.com/gogs/gogs/issues/7882)
- [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite)

---

## 2. Search Fusion Backend Caps

**Decision:** Cap regular web_search at 3 backends (2 free + 1 paid). Cap `/research deep` at 7. Previously fired all 19 configured backends on every search.

**Problem:** Every search query fired 19 parallel API requests. 8 deep research sessions burned through SerpAPI and SearchAPI monthly quotas. Budget of 100 API calls gave only ~5 actual searches (100 ÷ 19 backends per search).

**Research:**
- Rackauckas (2024): "When the number of queries m > 5, informational redundancy outweighs recall gains."
- Yazdani et al. (2023), Chang et al. (2025): "Limit ensemble size to 2-7 lists to avoid retrieval cost explosion and diminishing returns."
- RRF (Reciprocal Rank Fusion) best practices: 3-5 well-crafted ranked lists optimal. Beyond 5, poorly aligned sub-queries risk introducing off-topic content.

**Implementation:**
- Regular search: 2 free backends + 1 cheap/mid backend (selected by CostTier)
- `/research deep`: up to 7 backends, smart-routed by query type (code → GitHub+Sourcegraph, academic → Semantic Scholar+OpenAlex)
- `/research standard`: 5 backends (unchanged)
- `/research quick`: 3 free backends (unchanged)
- Falls back to all free if no paid backends configured

**Sources:**
- Rackauckas, 2024 (RRF sub-query analysis)
- Yazdani et al., 2023 (ensemble size recommendations)
- Chang et al., 2025 (retrieval cost analysis)
- [Azure AI Search: Hybrid Search Scoring (RRF)](https://learn.microsoft.com/en-us/azure/search/hybrid-search-ranking)
- [OpenSearch: Introducing Reciprocal Rank Fusion](https://opensearch.org/blog/introducing-reciprocal-rank-fusion-hybrid-search/)

---

## 3. Search Backend Quality Matrix

**Decision:** Track per-backend reliability, cost, and unique contribution metrics. Recommend backend combinations with combined reliability scores.

**Problem:** Users configure 19 backends without knowing which are reliable or cost-effective. No data-driven way to choose.

**Data collected (8 research sessions, 2026-04-03):**

| Backend | Reliability | Cost/1K | Calls |
|---------|-----------|---------|-------|
| Tavily | 100.00% | $5.00 | 112 |
| Brave | 100.00% | $5.00 | 109 |
| You.com | 100.00% | $5.00 | 109 |
| Exa | 99.09% | $7.00 | 111 |
| Serper | 97.32% | $0.30 | 112 |
| Linkup | 95.23% | $5.50 | 105 |
| DuckDuckGo | 64.21% | Free | 95 |
| SearXNG | 0% | Free | 130 (all failed — not configured) |
| Kagi | 0% | $25 | 128 (all failed — closed beta) |
| Jina | 0.78% | ~$5 | 129 (effectively broken) |

**Pricing sources (April 2026):**
- Serper: $0.30-$1.00/1K queries (google.serper.dev, pay-as-you-go)
- Brave Search API: $5.00/1K requests, $5/mo free credit
- Tavily: $5-$8/1K (credit-based, acquired by Nebius Feb 2026)
- You.com: $5.00/1K calls (reduced March 2026)
- Exa: $7-$15/1K (semantic/neural search)
- Linkup: €5/1K standard, €50/1K deep

**Recommendation for users:**

| Combination | Reliability | Cost/1K |
|-------------|-----------|---------|
| Serper only | 97.32% | $0.30 |
| Serper + Brave | 100.00% | $5.30 |
| Serper + Tavily | 100.00% | $5.30 |
| Serper + Brave + Tavily | 100.00% | $10.30 |

**Sources:**
- [Tavily pricing](https://www.tavily.com/pricing)
- [Serper pricing](https://serper.dev/)
- [Brave Search API](https://brave.com/search/api/)
- [You.com API pricing](https://home.you.com/pricing/api)
- [Exa pricing](https://exa.ai/pricing)
- Direct measurement from 8 research sessions

---

## 4. Per-Language Build Verification

**Decision:** After writing code files, run language-appropriate syntax verification automatically.

**Problem:** Agent writes files that don't compile/parse. Previously only Go files got `go build` verification. Python, JavaScript, Ruby, Rust files had zero verification.

**Research:**
- Claude Code (Dec 2025): Shipped native LSP support for per-language diagnostics after every file edit. Supports 20+ languages.
- Anthropic blog "Writing effective tools for agents": Tools should validate their output. Verification closes the feedback loop.
- Industry standard: All modern editors run linters/compilers on save.

**Commands per language:**

| Language | Syntax Check | Source |
|----------|-------------|--------|
| Go | `go build ./...` + `go vet ./...` | Go toolchain |
| Python | `python -m py_compile <file>` | Python stdlib |
| JavaScript | `node --check <file>` | Node.js (also `node -c`) |
| TypeScript | `npx tsc --noEmit` | TypeScript compiler |
| Rust | `cargo check` | Cargo (faster than `cargo build`) |
| Java | `javac <file>` | JDK |
| Ruby | `ruby -c <file>` | Ruby interpreter |
| C++ | `g++ -fsyntax-only <file>` | GCC/Clang |

**Why not LSP:** LSP requires running a language server per language, significant infrastructure. Shell commands are simpler, no dependencies beyond the language runtime. LSP is a future enhancement (v1.10+).

**Sources:**
- [Claude Code LSP support announcement](https://code.claude.com/docs/en/) (Dec 2025)
- [Anthropic: Writing effective tools for agents](https://www.anthropic.com/engineering/writing-tools-for-agents)
- Python: `python -m py_compile` is built-in since Python 2
- Node.js: `--check` flag added in Node.js 5.0 (github.com/nodejs/node-v0.x-archive/issues/9426)
- Ruby: `ruby -c` is the standard syntax check (ruby-forum.com)
- Rust: `cargo check` is documented as faster alternative to `cargo build` (doc.rust-lang.org)
- Modern Python toolchain: Ruff replaces Flake8+Black+isort; mypy for type checking (community consensus 2025-2026)

---

## 5. Agent Loop Detection

**Decision:** Three-signal loop detection: tool output hashing, hard cap, progress tracking.

**Problem:** Agent gets stuck reading the same files and running the same bash commands repeatedly. Existing fingerprint-based detection fires warnings but doesn't break loops.

**Research:**
- Claude Code: Uses tool call fingerprinting with sliding window.
- Codex CLI: Implements stall detection with automatic escalation.
- LangChain: ToolCallTracker pattern with severity escalation (NONE → WARNING → CRITICAL → TERMINAL).
- Anthropic "Context Engineering for AI Agents": Agents need progress metrics — not just "is it looping" but "is it making progress."

**Implementation (three signals):**
1. **Output hash tracking** — Hash tool output alongside the fingerprint. Same tool + same output 3x = stuck (not just same tool called repeatedly).
2. **Hard cap** — 5 loop warnings forces context reset with progress summary. Previously just escalated to bigger model (which doesn't fix loops).
3. **Progress metrics** — Track files modified count. 10 turns without file changes triggers "are you making progress?" prompt.

**Sources:**
- [Claude Code architecture analysis](https://arxiv.org/html/2603.05344v2) (2026)
- [LangChain context engineering blog](https://blog.langchain.com/context-engineering-for-agents/)
- [Anthropic effective context engineering](https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents)
- Direct observation: Agent made 17 bash calls + 7 file_reads on same 2 files without producing output (Atlassian project test, 2026-04-03)

---

## 6. Bash Tool State Persistence

**Decision:** Add explicit "fresh shell" instruction to bash tool description and system prompt.

**Problem:** Agent loops trying to set environment variables across bash calls. Each `bash` tool call runs in a fresh shell — env vars, cd, aliases don't persist. Agent doesn't know this and tries `export VAR=value` in one call, then uses `$VAR` in the next.

**Evidence:** Work profile test (2026-04-03) — agent called `EMAIL="ceverson@redhat.com"` 8+ times before loop detection kicked in. Never got to actual API calls.

**Research:**
- All CLI AI tools have this constraint. Claude Code, Codex, Aider all run each tool call in an isolated process.
- The fix is prompt-based, not architectural. The model needs to know the constraint.

**Implementation:**
- Bash tool description: "Each call runs in a fresh shell — environment variables, cd, and aliases do NOT persist between calls. To use env vars from .env: `source .env && curl...`"
- System prompt (both level 0 and level 1+): BASH section explaining inline sourcing.
- Validated on both personal (Ollama Cloud) and work (Vertex AI) profiles.

**Sources:**
- Direct observation from synroute agent testing
- Claude Code uses same pattern (isolated tool execution)
- Codex CLI documentation confirms per-call isolation

---

## 7. Auto-Worktree Isolation for Parallel Agents

**Decision:** Automatically create git worktrees when parallel agents run on the same directory.

**Problem:** Running 3 synroute agents in parallel on the same repo causes file clobber. Last writer wins, earlier agents' changes silently lost.

**Research:**
- Windsurf: Uses git worktrees for parallel agents (validated pattern).
- v1.06 research: "each unit → sub-agent in own worktree" (research_v106_broad.md).
- Spec Section 2.16: "Named volumes for workspace sharing" — worktrees are the single-user equivalent.

**Implementation:**
- Lock file (`.synroute.lock` with PID) in working directory.
- On agent start: check lock. If held by alive process, auto-create worktree via `internal/worktree/manager.go`.
- On agent exit: remove lock, cleanup worktree.
- No user action required — fully automatic.

**Sources:**
- [Windsurf parallel agent architecture](https://www.codeium.com/blog) (2025)
- synroute v1.06 multi-agent research (memory: research_v106_multiagent.md)
- Git worktree documentation

---

## 8. Provider-Aware YAML Tier Routing

**Decision:** YAML tier config routes models to their correct provider by name pattern matching.

**Problem:** YAML config (`~/.synroute/config.yaml`) treated all model names as Ollama Cloud models. `chatgpt-nano:latest` should route through codex subscription, `gemini-3-flash-preview` through gemini subscription.

**Implementation:**
Three-step resolution in `resolveYAMLProviders()`:
1. Pattern matching: `chatgpt/gpt/o1/o3` → codex, `gemini` → gemini, `claude` → anthropic
2. `DefaultModel()` interface match against registered providers
3. `SupportsModel()` fallback

**Sources:**
- User requirement (2026-04-02): "chatgpt-nano and gemini-flash should route through their subscription providers"
- Spec Section 1: Provider System table

---

## 9. Search Backend Circuit Breakers

**Decision:** Add persistent (SQLite-backed) circuit breakers for search backends, reusing the provider circuit breaker pattern.

**Problem:** When a search backend returns 429/402/rate-limit, synroute keeps calling it on every subsequent search. 8 research sessions caused SerpAPI and SearchAPI to exhaust quotas, and every search after that wasted time on guaranteed failures.

**Research:**
- synroute already has circuit breakers for LLM providers (`internal/router/circuit.go`) with states: closed → open → half-open.
- Same pattern applies to search backends.
- In-memory counters don't persist across CLI invocations (each `synroute chat --message` is a new process).

**Implementation:**
- `search_circuit_breakers` SQLite table (migration 012)
- On 429/402/rate-limit: open circuit, cooldown 1 hour
- Before selecting backends: query table, skip circuit-open backends
- On success: reset circuit

**Sources:**
- `internal/router/circuit.go` (existing provider pattern)
- Direct observation: SearchAPI hit monthly limits, continued being called 100+ times after

---

## 10. CLI UX Design Principles

**Decision:** Add 5 user experience principles to the spec (principles 12-16).

**Problem:** synroute dumped 40+ lines of internal logs to the user's terminal for a simple "hello" query. No status bar, no thinking indicator, no trust dialog. Competitor tools (Claude Code, Codex, Gemini CLI) showed clean, professional interfaces.

**Research (8 deep research sessions, 50+ sources):**
- Claude Code: Clean output, collapsible tool calls, status bar, `ctrl+o` to expand.
- Codex CLI: Header box with model info, suggested prompts, bottom status bar with budget remaining.
- Gemini CLI: Trust dialog, thinking spinner with elapsed time, workspace/branch/sandbox status bar.
- User sentiment (Reddit/HN/GitHub): "#1 reason users switch tools is trust erosion from hallucinations and unexpected behavior." Transparency into what the agent is doing is the most praised feature across all tools.

**Principles added to spec:**
12. The user comes first — usable without reading docs.
13. Zero noise by default — internal logs never in user terminal.
14. Show state, not mechanics — model/status/cancel, not provider indices.
15. Progressive disclosure — tips for new users, clean for experienced.
16. Accessibility is not optional — NO_COLOR, screen reader, colorblind as baseline.

**Sources:**
- `docs/research/v108-ui-research.md` (compiled from 8 sessions)
- `tests/ui/competitor-screenshots/` (Claude Code, Codex, Gemini CLI real captures)
- [Claude Code documentation](https://code.claude.com/docs/en/)
- [Codex CLI](https://github.com/openai/codex)
- [Gemini CLI](https://github.com/google-gemini/gemini-cli)
- User sentiment: Reddit r/LocalLLaMA, r/ChatGPT, HN threads, GitHub issues

---

## 11. Competitor Visual Comparison

**Decision:** Capture real screenshots of competitor tools for design reference.

**Method:** Used macOS `screencapture` via `osascript` to open each tool in Terminal and capture actual interactive output. VHS virtual terminal couldn't render tools using alternate screen buffers.

**Tools captured:**
- Claude Code: Tool call display, expandable results, cascading indicator, status bar
- Codex CLI: Header box, model info, suggested prompts, bottom status bar with budget
- Gemini CLI: Trust dialog, thinking spinner, workspace/branch/sandbox status bar, response with ✦ prefix

**Key findings:**
- Every competitor has a status bar (model, project, branch). synroute has none.
- Every competitor suppresses internal logs. synroute showed 174 log lines for "hello".
- Gemini CLI's thinking indicator (`⠋ Thinking... (esc to cancel, 4s)`) is the UX standard.
- Claude Code's `ctrl+o to expand` for tool results is the collapsibility standard.

**Screenshots stored:** `tests/ui/competitor-screenshots/`

**Sources:**
- Direct captures (2026-04-03)
- Claude Code v2.1.91, Codex CLI v0.118.0, Gemini CLI v0.36.0

---

## Summary

| # | Decision | Version | Research Sources |
|---|----------|---------|-----------------|
| 1 | Pure Go SQLite | v1.08 | DataStation, Grafana, Gogs |
| 2 | Search fusion caps | v1.08 | Rackauckas, Yazdani, Chang |
| 3 | Backend quality matrix | v1.08 | Direct measurement + pricing research |
| 4 | Per-language verification | v1.08 | Claude Code LSP, Anthropic blog |
| 5 | Loop detection | v1.08.1 | LangChain, Anthropic, direct observation |
| 6 | Bash state persistence | v1.08.1 | Direct observation + tool isolation pattern |
| 7 | Auto-worktree isolation | v1.08.1 | Windsurf, v1.06 research |
| 8 | Provider-aware tier routing | v1.06 | User requirement |
| 9 | Search circuit breakers | v1.08 | Existing provider pattern |
| 10 | CLI UX principles | v1.08 | 8 research sessions, competitor analysis |
| 11 | Competitor visual comparison | v1.08 | Direct captures |
| 12 | Intent routing (9 intents, 3 layers) | v1.08.4-7 | See intent-routing-design.md |
