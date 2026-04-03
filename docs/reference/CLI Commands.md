---
title: CLI Commands Reference
created: 2026-03-26
tags:
  - synroute
  - reference
  - cli
aliases:
  - CLI Reference
  - Commands
---

# CLI Commands Reference

Complete reference for all `synroute` CLI commands. For a guided introduction, see [[User Guide]].

All examples below assume `synroute` is on your PATH. If not, replace `synroute` with `./synroute` (run from the project directory) or the full path to the binary. See the [[User Guide#Add to your PATH]] section for setup.

---

## synroute (code mode)

The default command starts the code mode TUI -- a full terminal interface with status bar, scroll regions, and keyboard shortcuts.

```bash
synroute              # Start code mode TUI (default)
synroute code         # Start code mode TUI (explicit)
```

### Keyboard Shortcuts

| Shortcut | Action |
| --- | --- |
| `^P` | Show pipeline status |
| `^T` | Show recent tool calls |
| `^L` | Cycle verbosity level |
| `^E` | Force provider escalation |
| `^/` | Show help and shortcuts |
| `Ctrl-C` | Multi-mode: cancels LLM call during generation; at idle, press twice to exit |

### Environment Variables

| Variable | Default | Description |
| --- | --- | --- |
| `SYNROUTE_CONVERSATION_TIER` | `frontier` (personal) / `mid` (work) | Starting conversation tier for the parent agent |

---

## synroute serve

Start the HTTP server.

```bash
synroute serve        # Start HTTP server
```

The server listens on `:8090` by default and exposes:

- `/v1/chat/completions` -- OpenAI-compatible chat completions
- `/v1/responses` -- OpenAI responses endpoint
- `/v1/agent/chat` -- Agent API
- `/v1/models` -- List models
- `/v1/skills` -- List skills
- `/v1/skills/match?q=...` -- Preview skill chain for a goal
- `/v1/tools` -- List agent tools
- `/v1/agent/pool` -- Agent pool metrics
- `/v1/profile` -- Show profile info
- `/v1/doctor` -- Run diagnostics
- `/v1/eval/*` -- Eval framework endpoints
- `/health` -- Health check
- `/v1/test/providers` -- Smoke test providers (POST)
- `/v1/circuit-breakers/reset` -- Reset circuit breakers (POST)

When `SYNROUTE_MCP_SERVER=true` is set, the server also exposes MCP endpoints:

- `/mcp/initialize` (POST)
- `/mcp/tools/list` (POST)
- `/mcp/tools/call` (POST)

---

## synroute chat

Interactive agent REPL or one-shot message execution. The agent has access to 10 built-in tools (bash, file_read, file_write, file_edit, grep, glob, git, web_search, web_fetch, notebook_edit) plus 2 agent tools (delegate, handoff). Supports file attachments via `@file` and `@dir/` references.

### Flags

| Flag | Default | Description |
| --- | --- | --- |
| `--model` | `auto` | Model to use. `auto` lets the router pick. |
| `--message` | | One-shot message (non-interactive). Agent works in current directory. |
| `--spec-file` | | Read a spec file and use as message. Auto-detects language and existing project. |
| `--system` | | Custom system prompt. |
| `--project` | | Project name. Creates `~/Development/<name>/` and works there. |
| `--worktree` | `false` | Run in an isolated git worktree (24h TTL, 2GB cap). |
| `--max-agents` | `5` | Max concurrent sub-agents. |
| `--budget` | `0` | Max total tokens budget. `0` means unlimited. |
| `--resume` | `false` | Resume the most recent saved session. |
| `--session` | | Resume a specific session by ID. |
| `--verbose` | `0` | Verbosity: `0`=compact, `1`=normal, `2`=verbose. Also supports `-v` / `-vv`. |
| `--json-events` | `false` | Emit events as JSON lines to stderr. |

### Examples

```bash
# Interactive REPL
synroute chat

# One-shot: fix tests in the current directory
synroute chat --message "fix the failing tests"

# Use a specific model
synroute chat --model claude-sonnet-4-6

# Build from a spec file
synroute chat --spec-file ~/specs/api-redesign.md

# Work in a new project directory
synroute chat --project my-api

# Isolated worktree with limited concurrency
synroute chat --worktree --max-agents 3

# Resume previous session
synroute chat --resume
synroute chat --session abc123

# Verbose output
synroute chat -vv
```

### REPL Commands

When running interactively, these slash commands are available:

| Command | Description |
| --- | --- |
| `/exit` | Exit the session |
| `/clear` | Clear conversation history |
| `/model` | Show or change active model |
| `/tools` | List available tools |
| `/history` | Show conversation history |
| `/agents` | Show sub-agent status |
| `/budget` | Show token budget usage |
| `/plan` | Enter plan mode |
| `/review` | Enter review mode |
| `/check` | Run self-check |
| `/fix` | Enter fix mode |
| `/help` | Show help and keyboard shortcuts |

---

## synroute test

Smoke test all configured providers. Each provider receives a simple prompt and the result is displayed as a table.

### Flags

| Flag | Default | Description |
| --- | --- | --- |
| `--provider` | | Test only this provider (by name). |
| `--timeout` | `30s` | Per-provider timeout. |
| `--verbose` | `false` | Show detailed output including error messages. |
| `--json` | `false` | Output as JSON. |

### Examples

```bash
# Test all providers
synroute test

# Test a single provider
synroute test --provider ollama-chain-1

# JSON output with extended timeout
synroute test --json --timeout 60s

# Verbose output to see errors
synroute test --verbose
```

### Output

```
PROVIDER             STATUS   MODEL                          TOKENS   LATENCY
--------------------------------------------------------------------------------
ollama-chain-0       PASS     ministral-3:14b                42       312ms
ollama-chain-1       PASS     gpt-oss:20b                    38       445ms
...
--------------------------------------------------------------------------------
Results: 21 passed, 0 failed
```

---

## synroute profile

Show or switch the active profile.

### Subcommands

#### profile show

Display the active profile and its providers.

```bash
synroute profile show
synroute profile show --json
```

#### profile list

List all available profiles.

```bash
synroute profile list
synroute profile list --json
```

#### profile switch

Switch to a different profile. Requires a server restart to take effect.

```bash
synroute profile switch personal
synroute profile switch work
```

### Flags

| Flag | Default | Description |
| --- | --- | --- |
| `--json` | `false` | Output as JSON (available on `show` and `list`). |

---

## synroute doctor

Run comprehensive diagnostics on configuration, providers, and environment.

### Flags

| Flag | Default | Description |
| --- | --- | --- |
| `--json` | `false` | Output as JSON. |

### Examples

```bash
synroute doctor
synroute doctor --json
```

### Output

Checks are grouped by category with status indicators:

```
[Configuration]
  OK    profile                   personal
  OK    env_file                  .env loaded
[Providers]
  OK    ollama-chain-0            healthy (312ms)
  WARN  gemini                    rate limited
[Environment]
  OK    go_version                go1.24.1

Summary: 8 ok, 1 warnings, 0 failures
```

---

## synroute models

List available models across all configured providers.

### Flags

| Flag | Default | Description |
| --- | --- | --- |
| `--provider` | | Filter by provider name. |
| `--json` | `false` | Output as JSON. |

### Examples

```bash
synroute models
synroute models --provider ollama-chain-0
synroute models --json
```

---

## synroute version

Show version, commit hash, build date, active profile, and Go runtime version.

```bash
synroute version
```

```
synroute v1.2.0 (ad0b841) built 2026-03-24 | profile: personal | go1.24.1
```

---

## synroute mcp-serve

Start a standalone MCP (Model Context Protocol) tool server. Exposes the agent's tool registry over HTTP.

### Flags

| Flag | Default | Description |
| --- | --- | --- |
| `--addr` | `127.0.0.1:8091` | Listen address. |
| `--token` | | Bearer token for auth. Auto-generated if empty. Falls back to `SYNROUTE_MCP_TOKEN` env var. |
| `--no-auth` | `false` | Disable authentication (not recommended). |

### Examples

```bash
# Default (localhost:8091, auto-generated token)
synroute mcp-serve

# Custom port, no auth
synroute mcp-serve --addr :9090 --no-auth

# With explicit token
synroute mcp-serve --token my-secret-token
```

### Endpoints

| Method | Path | Description |
| --- | --- | --- |
| POST | `/mcp/initialize` | Initialize MCP session |
| POST | `/mcp/tools/list` | List available tools |
| POST | `/mcp/tools/call` | Call a tool |

### Example: Call a Tool

```bash
curl -X POST localhost:8091/mcp/tools/call \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "tools/call",
    "params": {
      "name": "bash",
      "arguments": {"command": "ls -la"}
    }
  }'
```

---

## synroute eval

Multi-language evaluation framework for benchmarking LLM code generation. Supports 11 benchmark sources and 4 eval modes.

### Subcommands

- `import` -- Import exercises from a benchmark repository
- `import-all` -- Clone and import all benchmark sources
- `exercises` -- List imported exercises
- `run` -- Run an evaluation
- `results` -- Show results from a run
- `compare` -- Compare two runs

---

### eval import

Import exercises from a single benchmark source.

#### Flags

| Flag | Default | Description |
| --- | --- | --- |
| `--source` | *required* | Source name (see table below). |
| `--path` | *required* | Path to the cloned benchmark repository. |
| `--language` | | Language filter (required for `exercism`). |
| `--count` | `0` | Max exercises to import (`codecontests` only, `0`=all). |
| `--json` | `false` | Output as JSON. |

#### Sources

| Source | Description | Approximate Count |
| --- | --- | --- |
| `polyglot` | Aider polyglot-benchmark | ~225 |
| `roocode` | Roo-Code-Evals | ~170 |
| `exercism` | Exercism tracks (per language) | 100-159 per language |
| `multiple` | MultiPL-E HumanEval translations | ~984 across 6 languages |
| `evalplus` | EvalPlus HumanEval+/MBPP+ (Python) | ~563 |
| `codecontests` | Google CodeContests | variable (use `--count`) |
| `ds1000` | DS-1000 data science (Python) | ~1000 |
| `birdsql` | BIRD-SQL Mini-Dev text-to-SQL | ~500 |
| `dare-bench` | DARE-bench ML modeling | ~162 |
| `writingbench` | WritingBench business writing | ~1000 |
| `pptarena` | PPTArena slide editing | ~100 |

#### Examples

```bash
synroute eval import --source polyglot --path ~/polyglot-benchmark
synroute eval import --source exercism --path ~/exercism-go --language go
synroute eval import --source codecontests --path ~/code_contests --count 500
synroute eval import --source ds1000 --path benchmarks/ds1000
synroute eval import --source birdsql --path benchmarks/birdsql
```

---

### eval import-all

Clone all benchmark repositories and import them in one step.

#### Flags

| Flag | Default | Description |
| --- | --- | --- |
| `--dir` | *required* | Directory to clone repos into. |
| `--codecontests-count` | `500` | Max CodeContests exercises to import. |
| `--json` | `false` | Output as JSON. |

#### Example

```bash
synroute eval import-all --dir ~/eval-benchmarks
```

This clones 16 repositories (polyglot, roocode, exercism for 6 languages, MultiPL-E, evalplus, codecontests, DS-1000, DARE-bench, BIRD-SQL, WritingBench, PPTArena) and imports all exercises.

---

### eval exercises

List imported exercises.

#### Flags

| Flag | Default | Description |
| --- | --- | --- |
| `--language` | | Filter by language. |
| `--suite` | | Filter by suite. |
| `--json` | `false` | Output as JSON. |

#### Examples

```bash
synroute eval exercises --language go
synroute eval exercises --suite polyglot --json
```

---

### eval run

Run an evaluation against imported exercises.

#### Flags

| Flag | Default | Description |
| --- | --- | --- |
| `--language` | | Languages, comma-separated. |
| `--suite` | | Filter by suite. |
| `--provider` | | Specific provider for direct mode. |
| `--model` | | Specific model. |
| `--mode` | `direct` | Mode: `direct` (single provider) or `routing` (use router). Auto-switches to `routing` if no provider specified. |
| `--count` | `0` | Total exercise cap (`0`=no cap). |
| `--per-suite` | `40` | Exercises per suite (`0`=all). |
| `--seed` | `0` | Random seed for reproducibility. |
| `--two-pass` | `false` | Enable two-pass with error feedback. |
| `--agent` | `false` | Agent mode: iterative test-fix loop with test file context. |
| `--max-turns` | `5` | Max fix iterations in agent mode. |
| `--timeout` | `120` | Per-exercise Docker timeout in seconds. |
| `--concurrency` | `4` | Parallel exercises (`1`=sequential, max `10`). |
| `--json` | `false` | Output as JSON. |

#### Examples

```bash
# Run Go exercises with two-pass
synroute eval run --language go --count 10 --two-pass

# Run via a specific provider
synroute eval run --provider ollama-chain-1 --count 20

# Routing mode (auto-escalation)
synroute eval run --mode routing --count 15

# Per-suite limit (default is 40 per suite)
synroute eval run --per-suite 40

# No per-suite limit, 100 total
synroute eval run --per-suite 0 --count 100

# Agent mode with iterative fixing
synroute eval run --language python --agent --max-turns 5
```

---

### eval results

Show results from an evaluation run.

#### Flags

| Flag | Default | Description |
| --- | --- | --- |
| `--run-id` | | Run ID. Defaults to most recent run if omitted. |
| `--json` | `false` | Output as JSON. |

#### Examples

```bash
# Most recent run
synroute eval results

# Specific run as JSON
synroute eval results --run-id eval-abc123 --json
```

---

### eval compare

Compare two evaluation runs side by side.

#### Flags

| Flag | Default | Description |
| --- | --- | --- |
| `--run-a` | *required* | First run ID. |
| `--run-b` | *required* | Second run ID. |
| `--json` | `false` | Output as JSON. |

#### Example

```bash
synroute eval compare --run-a eval-abc --run-b eval-def
```

#### Output

```
Comparison: eval-abc vs eval-def

  Pass@1:    +5.2%
  Pass@2:    +3.1%
  Latency:   -120ms
  Tokens:    +1500
  Fallback:  -2.0%
```

---

## API Endpoints (Quick Reference)

These endpoints are available when the server is running (`synroute serve`).

```bash
# Diagnostics
curl -X POST localhost:8090/v1/test/providers
curl -X POST localhost:8090/v1/circuit-breakers/reset
curl localhost:8090/v1/doctor
curl localhost:8090/health

# Profile
curl localhost:8090/v1/profile
curl -X POST localhost:8090/v1/profile/switch -d '{"profile":"work"}'

# Models and Skills
curl localhost:8090/v1/models
curl localhost:8090/v1/skills
curl "localhost:8090/v1/skills/match?q=fix+the+Go+auth"
curl localhost:8090/v1/tools

# Agent
curl -X POST localhost:8090/v1/agent/chat -d '{"message":"fix the tests","model":"auto"}'
curl localhost:8090/v1/agent/pool

# Eval
curl localhost:8090/v1/eval/exercises?language=go
curl -X POST localhost:8090/v1/eval/runs -d '{"languages":["go"],"count":5}'
curl localhost:8090/v1/eval/runs
curl localhost:8090/v1/eval/runs/<run_id>
curl localhost:8090/v1/eval/runs/<run_id>/results
curl -X POST localhost:8090/v1/eval/compare -d '{"run_a":"<id>","run_b":"<id>"}'

# MCP (requires SYNROUTE_MCP_SERVER=true or use mcp-serve)
curl -X POST localhost:8090/mcp/tools/list
curl -X POST localhost:8090/mcp/tools/call -d '{
  "jsonrpc":"2.0","id":1,"method":"tools/call",
  "params":{"name":"bash","arguments":{"command":"ls"}}
}'
```

---

## synroute recommend (v1.05)

Detects available providers and suggests optimal tier configurations based on bundled benchmark scores and pricing data.

```bash
synroute recommend              # Show tier recommendations
synroute recommend --json       # JSON output
```

Output includes per-tier model suggestions with coding scores, planning scores, cost per query, and a suggested OLLAMA_CHAIN string.

---

## synroute config (v1.05)

Manage YAML tier configuration as an alternative to OLLAMA_CHAIN env var.

```bash
synroute config show            # Show current effective config (YAML or env)
synroute config generate        # Generate ~/.synroute/config.yaml from OLLAMA_CHAIN
```

### Config File Locations (priority order)
1. `.synroute.yaml` in current directory (project-level)
2. `~/.synroute/config.yaml` (user-level)
3. `OLLAMA_CHAIN` env var (fallback)

### YAML Format
```yaml
tiers:
  cheap:
    - ministral-3:14b-cloud
    - gemma3:12b-cloud
  mid:
    - nemotron-3-super:cloud
    - qwen3.5:cloud
  frontier:
    - deepseek-v3.1:671b-cloud
    - kimi-k2.5:cloud
```

---

## REPL Commands (v1.05)

Available in code mode and chat mode:

```
/research [quick|standard|deep] <query>  — Multi-round web research with citations
/plan <description>                      — Generate plan with acceptance criteria
/review                                  — Code review current changes
/check                                   — Self-check against criteria
/fix <description>                       — Targeted fix
/exit                                    — Exit REPL
/clear                                   — Clear conversation
/model [name]                            — Show or switch model
/tools                                   — List available tools
/history                                 — Show conversation history
/agents                                  — Show sub-agent pool status
/budget                                  — Show budget usage
/help                                    — Show available commands
```

### /research Depth Tiers

| Tier | Rounds | Backends | Max API Calls | Est. Cost |
|------|--------|----------|---------------|-----------|
| quick | 1 | Free only | 3 | $0 |
| standard | 2-3 | Free + cheap paid | 15-30 | <$0.05 |
| deep | 3-5 | All configured | 50-100 | <$0.50 |

---

## Related Pages

- [[User Guide]] -- Getting started and daily usage
- [[Architecture]] -- System architecture and provider chain
