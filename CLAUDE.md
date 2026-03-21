# SynapseRouter (synroute)

Go-based LLM proxy router and coding agent that distributes requests across subscription providers (Claude Code, Codex, Gemini) and direct providers (NanoGPT, Ollama, Vertex AI). Includes interactive agent REPL with tool execution (bash, file I/O, grep, glob, git), worktree isolation, and MCP server mode. Two profiles: `personal` (OAuth subscriptions) and `work` (Vertex AI). Auto-discovers 159+ models across all active providers.

## Key Files

- `main.go` — Server setup, CLI dispatch, provider initialization, HTTP handlers
- `commands.go` — CLI command implementations (chat, test, profile, doctor, models, version, mcp-serve)
- `eval_commands.go` — CLI: `synroute eval {import,import-all,exercises,run,results,compare}`
- `eval_handlers.go` — API: `/v1/eval/*` endpoints
- `internal/eval/` — Eval framework (types, store, importer, docker, runner, scorer) — 11 sources, 4 eval modes
- `benchmarks/` — Eval benchmark data (ds1000, dare-bench, birdsql, writingbench, pptarena, exercism, multiple, evalplus)
- `diagnostic_handlers.go` — API endpoints for testing, diagnostics, circuit breaker reset, skill dispatch
- `internal/orchestration/skills.go` — Skill registry with trigger-based matching
- `internal/orchestration/dispatch.go` — Auto-dispatch engine: goal → skill chain → task steps
- `compat_handlers.go` — OpenAI-compatible `/v1/chat/completions` and `/v1/responses` endpoints
- `internal/tools/` — Agent tool interface, registry, and implementations (bash, file_read/write/edit, grep, glob, git, permissions)
- `internal/agent/` — Agent loop, REPL, sub-agents, handoffs, guardrails, state persistence, tracing, streaming
- `internal/environment/` — Project environment detection, version resolution, best practices engine
- `internal/worktree/` — Git worktree isolation with TTL, size caps, background cleanup
- `internal/mcpserver/` — MCP server: expose tools over HTTP (tools/list, tools/call)
- `internal/app/` — Shared logic for CLI and API (smoketest, diagnostics, profile, models)
- `internal/router/router.go` — Provider selection, fallback chain, circuit breakers, health caching
- `internal/providers/provider.go` — Provider interface (`Name`, `ChatCompletion`, `IsHealthy`, `SupportsModel`)
- `internal/providers/vertex.go` — Vertex AI provider (Claude via rawPredict, Gemini via generateContent)
- `internal/providers/nanogpt.go` — NanoGPT provider with tiered model routing
- `internal/subscriptions/providers.go` — OAuth subscription provider management
- `internal/subscriptions/credential_store.go` — Credential storage and OAuth refresh
- `internal/router/circuit.go` — Circuit breaker with smart rate-limit cooldowns and reset

## Architecture

### Provider Chain (Personal Profile)
Cost-optimized escalation: subscription models first, paid API last.

`NanoGPT-Sub → Gemini → Codex → Claude Code → Ollama → NanoGPT-Paid`

- `nanogpt-sub`: NanoGPT subscription models (qwen, deepseek, etc. — included in subscription)
- `gemini`, `codex`, `claude-code`: CLI subscription providers (included in subscriptions)
- `nanogpt-paid`: NanoGPT paid API models (claude/gpt/gemini via NanoGPT — costs money per call)
- List order = priority (`GetBestProvider()` returns first under threshold)
- Circuit breakers open on failures, parse "reset after Ns" from 429 errors
- Health checks cached (2min TTL) to avoid burning API quota

### NanoGPT Split
Two provider instances from one `NANOGPT_API_KEY`:
- `nanogpt-sub`: baseURL `https://nano-gpt.com/api/subscription/v1`, handles auto/subscription models
- `nanogpt-paid`: baseURL `https://nano-gpt.com/api/paid/v1`, handles paid API models only
- `SupportsModel` routes by tier; `nanogpt-paid` returns false for auto (only reachable via fallback)

### Profiles
- `personal`: NanoGPT-Sub + OAuth subscription providers + NanoGPT-Paid
- `work`: Vertex AI only (Claude + Gemini via native GCP auth) — no NanoGPT
- Controlled by `ACTIVE_PROFILE` in `.env`

### Skill Auto-Dispatch
When a task/goal is submitted to orchestration, the dispatch engine automatically:
1. Matches goal text against skill triggers (keyword matching)
2. Orders matched skills by phase: `analyze → implement → verify → review`
3. Converts skill chain to TaskSteps with skill-aware prompts
4. Auto-invokes bound MCP tools and injects results as context
5. Falls back to role-based inference (`inferRoles`) if no skills match

Built-in skills: `go-patterns`, `python-patterns`, `security-review`, `code-implement`, `go-testing`, `python-testing`, `code-review`, `api-design`, `docker-expert`, `research`

### Agent Execution Layer
- **Tool Registry** (`internal/tools/`): 7 built-in tools + 2 agent tools (delegate, handoff)
- **Tool Categories**: `read_only` (always allowed), `write` (needs approval), `dangerous` (extra scrutiny)
- **Agent Loop** (`internal/agent/`): message → LLM → tool calls → pipeline check → repeat (unlimited turns)
- **Sub-Agent SDK** (`internal/agent/subagent.go`): parent-child agent spawning with config inheritance
  - `SpawnChild(SpawnConfig)` — create child agent (inherits model, tools, workdir)
  - `RunChild(ctx, cfg, task)` — spawn + run + collect result
  - `RunChildrenParallel(ctx, tasks, maxConcurrent)` — parallel delegation
- **Handoffs** (`internal/agent/handoff.go`): OpenAI Swarm-style agent-to-agent context transfer
  - `ExecuteHandoff(ctx, Handoff)` — spawn specialist with context summary
  - `DelegateTool` / `HandoffTool` — LLM-invocable tools for delegation
- **Agent Pool** (`internal/agent/pool.go`): concurrency-limited agent management
  - Default 5 concurrent agents, semaphore-based, resource tracking
- **Guardrails** (`internal/agent/guardrails.go`): input/output validation
  - `MaxLengthGuardrail`, `SecretPatternGuardrail`, `BlocklistGuardrail`
  - `GuardrailChain` — compose multiple guardrails
- **State Persistence** (`internal/agent/state.go`): SQLite-backed session save/load/resume
  - `SaveState(db)`, `LoadState(db, id)`, `LoadLatestState(db)`, `RestoreAgent()`
  - Migration: `migrations/006_agent_sessions.sql`
- **Budget Tracking** (`internal/agent/budget.go`): turn/token/duration limits
  - Per-agent `AgentBudget` with `BudgetTracker` enforcement
- **Tracing** (`internal/agent/trace.go`): structured event spans (llm_call, tool_call, handoff)
- **Metrics** (`internal/agent/metrics.go`): request/tool/sub-agent performance tracking
- **Streaming** (`internal/agent/streaming.go`): line-by-line output via `StreamWriter`
- **REPL**: `synroute chat` — interactive with `/exit`, `/clear`, `/model`, `/tools`, `/history`, `/agents`, `/budget`
- **Worktree Isolation** (`internal/worktree/`): `synroute chat --worktree` creates managed git worktree
  - TTL-based expiry (default 24h), size caps (10GB total, 2GB per tree), background cleanup (every 5m)
- **Permission Model**: `interactive` (prompt), `auto_approve` (allow all), `read_only` (deny writes)
- **MCP Server** (`internal/mcpserver/`): `synroute mcp-serve` or `SYNROUTE_MCP_SERVER=true` on main server
  - Endpoints: `/mcp/initialize`, `/mcp/tools/list`, `/mcp/tools/call`
- **Git Safety**: `git push --force`, `git branch -D`, `git checkout --force` blocked by git tool — use bash with explicit approval

### Environment Best Practices Engine
- **Detection** (`internal/environment/detector.go`): auto-detect language from config files
  - Supports: Go (go.mod), Python (requirements.txt, pyproject.toml), JS (package.json), Rust (Cargo.toml), Java (pom.xml), Ruby (Gemfile), C++ (CMakeLists.txt)
- **Version Resolution** (`internal/environment/resolver.go`): match installed runtime vs project requirements
  - Known Python version constraints for ML packages (tensorflow→3.12, torch→3.12, etc.)
- **Best Practices** (`internal/environment/best_practices.go`): per-language checks
  - Go: go.mod, go.sum, .gitignore
  - Python: virtual environment, pinned deps, version compatibility
  - JS: lockfile, Node version pinning
  - Rust: Cargo.lock, edition
  - Java: build wrapper
- **Command Wrapping** (`internal/environment/setup.go`): auto-activate venv, generate setup/test/build commands

### Key Patterns
- Gemini 2.5+ models: thinking tokens from output budget, min 1024 maxOutputTokens enforced
- Codex: SSE streaming via `/responses` endpoint (NOT `/responses/compact`)
- NanoGPT-Sub: defaults to "qwen/qwen3.5-397b-a17b" for auto (paid subscription, not free)
- NanoGPT-Paid: defaults to "chatgpt-4o-latest" for auto (only via fallback)
- Gemini personal (subscription): defaults to "gemini-3-flash-preview" for auto
- Vertex Gemini (work): defaults to "gemini-3.1-pro-preview" for auto
- Vertex Claude: use model names without dates (e.g. `claude-sonnet-4-6`)
- Default subscription provider order: `gemini,openai,anthropic` (configurable via env)

### Agent Pipeline (`internal/agent/pipeline.go`)
- **Software pipeline**: Plan → Implement → Self-Check → Code Review → Acceptance Test → Deploy
- **Data science pipeline**: EDA → Data Prep → Model → Review → Deploy → Verify
- Pipeline detected automatically from matched skills
- Quality gates: verification phases require minimum tool calls (can't rubber-stamp)
- Sub-agent reviews: code-review and acceptance-test phases spawn FRESH agents with no shared context
- Provider escalation triggered by pipeline phases (Escalate flag), not by separate mechanisms
- Max 3 fail-back cycles before accepting result

### Skills System (`internal/orchestration/skilldata/`)
- 38+ embedded skills parsed from YAML frontmatter in `.md` files via `go:embed`
- `ParseSkillsFromFS()` — no hardcoded Go registry, everything from frontmatter
- Adding a skill: create `.md` file with frontmatter (name, triggers, role, phase, mcp_tools), rebuild
- Skills injected into system prompt when triggers match the user's message

## Dev Commands

```bash
go build -o synroute .                    # Build
go test ./...                              # Run all tests
go vet ./...                               # Lint
```

## CLI Commands

```bash
./synroute                                 # Start server (default)
./synroute serve                           # Start server (explicit)
./synroute test                            # Smoke test all providers
./synroute test --provider nanogpt         # Test single provider
./synroute test --json                     # JSON output
./synroute eval import --source polyglot --path ~/polyglot-benchmark
./synroute eval import --source roocode --path ~/Roo-Code-Evals
./synroute eval import --source exercism --path ~/exercism-go --language go
./synroute eval import --source multiple --path ~/MultiPL-E
./synroute eval import --source evalplus --path ~/evalplus
./synroute eval import --source codecontests --path ~/code_contests --count 500
./synroute eval import --source ds1000 --path benchmarks/ds1000
./synroute eval import --source birdsql --path benchmarks/birdsql
./synroute eval import --source dare-bench --path benchmarks/dare-bench
./synroute eval import --source writingbench --path benchmarks/writingbench
./synroute eval import --source pptarena --path benchmarks/pptarena
./synroute eval import-all --dir ~/eval-benchmarks
./synroute eval exercises --language go    # List imported exercises
./synroute eval run --language go --count 10 --two-pass
./synroute eval run --provider nanogpt-sub --count 20
./synroute eval run --mode routing --count 15
./synroute eval run --per-suite 40              # 40 per suite (default), pipeline validation
./synroute eval run --per-suite 0 --count 100   # no per-suite limit, 100 total
./synroute eval results --json             # Most recent run
./synroute eval compare --run-a <id1> --run-b <id2>
./synroute profile show                    # Show active profile
./synroute profile list                    # List available profiles
./synroute profile switch work             # Switch to work profile
./synroute doctor                          # Run diagnostics
./synroute doctor --json                   # JSON diagnostics
./synroute models                          # List available models
./synroute version                         # Show version info
./synroute chat                            # Interactive agent REPL
./synroute chat --model claude-sonnet-4-6 # Specific model
./synroute chat --message "fix the bug"    # One-shot (non-interactive)
./synroute chat --system "You are a Go expert"
./synroute chat --worktree                 # Run in isolated worktree
./synroute chat --max-agents 3             # Limit concurrent sub-agents
./synroute chat --budget 10000             # Max total tokens budget
./synroute chat --project my-app            # Create ~/Development/my-app/ and work there
./synroute chat --resume                   # Resume most recent session
./synroute chat --session <id>             # Resume specific session
./synroute mcp-serve                       # Start standalone MCP server
./synroute mcp-serve --addr :9090          # Custom port
```

## API Endpoints (Diagnostic)

```bash
curl -X POST localhost:8090/v1/test/providers          # Smoke test providers
curl -X POST localhost:8090/v1/circuit-breakers/reset   # Reset all circuit breakers
curl localhost:8090/v1/profile                          # Show profile info
curl -X POST localhost:8090/v1/profile/switch -d '{"profile":"work"}'
curl localhost:8090/v1/doctor                           # Run diagnostics
curl localhost:8090/health                              # Health check
curl localhost:8090/v1/models                           # List models
curl localhost:8090/v1/skills                           # List registered skills
curl "localhost:8090/v1/skills/match?q=fix+the+Go+auth"  # Preview skill chain for a goal
curl localhost:8090/v1/tools                             # List agent tools
# Agent SDK API
curl -X POST localhost:8090/v1/agent/chat -d '{"message":"fix the tests","model":"auto"}'
curl localhost:8090/v1/agent/pool                        # Agent pool metrics
# MCP server (requires SYNROUTE_MCP_SERVER=true)
curl -X POST localhost:8090/mcp/tools/list               # MCP tools/list
curl -X POST localhost:8090/mcp/tools/call -d '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"bash","arguments":{"command":"ls"}}}'
curl localhost:8090/v1/eval/exercises?language=go          # List eval exercises
curl -X POST localhost:8090/v1/eval/runs -d '{"languages":["go"],"count":5}'
curl localhost:8090/v1/eval/runs                           # List recent eval runs
curl localhost:8090/v1/eval/runs/<run_id>                  # Run status + summary
curl localhost:8090/v1/eval/runs/<run_id>/results          # Individual results
curl -X POST localhost:8090/v1/eval/compare -d '{"run_a":"<id>","run_b":"<id>"}'
```

## Auto-Dispatch Rules (MANDATORY)

Skills are a chain, not a menu. For ANY task, scan ALL available skills (user-level + project-level) and auto-invoke every one whose trigger matches. A single user request like "clean the code" should cascade through 3-5+ skills automatically:

1. **Detect language/domain** → invoke the matching pattern skill (go-patterns, python-patterns, etc.)
2. **Detect changes were made** → invoke code-review
3. **Detect tests exist** → invoke the testing skill (go-testing, python-testing, etc.)
4. **Detect security surface** → invoke security-review if auth/credentials/API keys are touched
5. **Detect verification needed** → invoke the project test workflow (synroute-test)

Never ask "should I run the tests?" or "want me to review?" — just do it. The skills exist to be used automatically.

### Skill Triggers — Auto-invoke when condition matches

| Condition | Skill | How to invoke |
|---|---|---|
| Writing or modifying Go code | `go-patterns` | Skill tool |
| Writing or modifying Go tests | `go-testing` | Skill tool |
| Testing, verifying, or checking | `synroute-test` | Read `.claude/skills/synroute-test.md` and execute its instructions |
| Reviewing code or changes | `code-review` | Skill tool |
| Security, credentials, or auth | `security-review` | Skill tool |
| Researching an API, library, or concept | `synroute-research` | Read `.claude/skills/synroute-research.md` and execute its instructions |
| Switching or checking profile | `synroute-profile` | Read `.claude/skills/synroute-profile.md` and execute its instructions |
| Docker or containers | `docker-expert` | Skill tool |
| Designing or modifying API endpoints | `api-design` | Skill tool |
| GitHub PRs, issues, or CI | `github-workflows` | Skill tool |

**Project skills** (`.claude/skills/*.md`) are not invocable via the Skill tool — read the file and follow its instructions directly.
**User-level skills** (`~/.claude/skills/`) are invocable via the Skill tool.

### Subagent Delegation — Auto-delegate when condition matches

| Condition | Agent | File |
|---|---|---|
| E2E provider testing | `@provider-tester` | `.claude/agents/provider-tester.md` |
| Code review of changes | `@code-reviewer` | `.claude/agents/code-reviewer.md` |
| Profile switching | `@profile-manager` | `.claude/agents/profile-manager.md` |
| Technical research | `@research-assistant` | `.claude/agents/research-assistant.md` |

### Standard Post-Change Pipeline

After ANY code change, automatically run this pipeline in order:
1. `go vet ./...` — catch issues early
2. `go test -race ./...` — unit tests with race detection
3. `./synroute test` — E2E provider smoke test (or delegate to `@provider-tester`)
4. Verify build: `go build -o synroute .`

Do NOT skip steps or ask whether to run them. The pipeline runs automatically after changes are complete.

## Documentation Guardrails

- Use precise language for parity claims
- Say "implemented slice" for partial ports
- Say "targeted parity bucket complete" only for the exact verified subset
- Do not claim full parity without an explicit audit
- Do not describe synroute as an MCP server — it is a standalone LLM router

## Post-Run Audit (MANDATORY for every `synroute chat` run)

After every agent run completes, perform this audit before reporting results. This is how we verify what works and what doesn't — every project, every session.

### 1. Analyze the Work
- Read the full agent output — don't just check if it exited cleanly
- List every tool call the agent made and what it produced
- Identify which providers were used and whether escalation fired

### 2. Verify Output Matches Intent
- Compare what the agent produced against what was requested
- Check every specific detail: distances, values, formats, field names, auth methods
- If the agent said "I did X" — verify X actually happened (fetch API state, read files, run the code)

### 3. Check for Duplicates / Side Effects
- Verify no duplicate resources were created (API entries, files, DB records)
- Check that cleanup happened if requested
- Verify old/stale state was removed

### 4. Code Review (if code was generated)
- Does it compile? Did the agent run `go build`?
- Are there tests? Did they pass?
- Is the code idiomatic for the language?
- Are there hardcoded values that should be configurable?

### 5. Identify Missing Steps
For each project, consider which of these the agent SHOULD have done but didn't:
- **Planning** — did it plan before implementing?
- **Research** — did it look up APIs/docs before guessing?
- **Implementation** — did it build all required components?
- **Testing** — did it run tests or verify results?
- **Review** — did it compare output vs input for completeness?
- **Documentation** — did it document what it built?
- **Escalation** — when stuck, did it escalate to research/different provider?
- **Self-correction** — when results were wrong, did it detect and fix?

### 6. Record Findings
- Document what worked and what didn't
- Identify patterns (e.g., "agent always misses rest periods in workout parsing")
- Update skills, prompts, or agent behavior based on findings
- Feed learnings into the next run

### 7. Provider Chain Verification
- Which providers were actually used? (check logs for "Success with X")
- Did escalation happen when it should have?
- Did auto-review use a different provider than implementation?
- Were all subscription providers reachable?

This audit is how we iteratively improve synapserouter. Each project run teaches us what the agent gets right and wrong, and we fix the gaps before the next project.

## Workflow Rules (learned from real usage)

### Tool Builder, Not Tool Runner
- The agent BUILDS programs, scripts, and tools — it does NOT do the work itself
- When a task involves repeated operations (API calls, data processing, sync): write a PROGRAM that does it
- The user should end up with a runnable tool, not just completed side effects
- Use bash/curl ONLY for research (testing APIs, inspecting responses) and verification (running the built tool)
- The deliverable is ALWAYS a program the user can run, not a series of manual operations
- This is how Claude Code works: it writes code, the user runs it

### Plan Before Execute
- ANY new feature or non-trivial change MUST be planned first (enter plan mode automatically)
- Do NOT jump to implementation — always plan, get approval, then execute

### The Router IS the Escalation
- Synapserouter's entire purpose is cost-optimized escalation across providers
- The agent uses the router's existing provider chain — no parallel escalation systems
- Provider chain: `NanoGPT-Sub → Gemini → Codex → Claude Code → Ollama → NanoGPT-Paid`
- Agent sends `model: "auto"` and lets the router pick
- NanoGPT subscription is a PAID subscription, not free

### Project Lifecycle Pipeline
- Every substantial task goes through: Plan → Implement → Self-Check → Code Review → Acceptance Test → Deploy
- Plan phase produces acceptance criteria that ALL later phases check against
- Code review and acceptance test use FRESH sub-agents with no shared conversation (independent review)
- Quality gates: verification phases must make tool calls (can't rubber-stamp with text)
- Pipeline is the ONLY control flow mechanism — no bolted-on stall detection, mode switching, or failure tracking

### Keep the Agent Simple
- The agent loop is: call LLM → execute tools → check pipeline → repeat
- ONE system per responsibility — no overlapping mechanisms
- Pipeline handles all phase transitions and provider escalation
- No inline prompt injection during tool execution
- System prompt generated once per session, not every loop iteration

### Skills Are Self-Contained
- Adding a skill = drop one `.md` file in `internal/orchestration/skilldata/`, rebuild
- No Go code changes needed — frontmatter defines triggers, role, phase, MCP tools
- Skills compiled into binary via `go:embed`

### Agent Working Directory
- One-shot `--message` mode works in the CURRENT directory, not a temp dir
- Files must persist after the agent exits
- `--project <name>` flag creates `~/Development/<name>/` and works there

## Do Not

- Put code style rules here (use linters and `go vet`)
- Add comments, docstrings, or type annotations to code you didn't change
- Over-engineer solutions beyond what was asked
- Commit `.env` or credential files
- Refer to NanoGPT subscription as "free" — it is a paid subscription
- Hardcode model names in escalation paths
- Build parallel systems that duplicate the router's job
- Bolt on mechanisms to the agent loop — use the pipeline
- Have the agent DO the work instead of BUILD the tool that does the work
- Skip planning for non-trivial changes
- Declare success without verifying the actual result
