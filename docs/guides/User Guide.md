---
title: SynapseRouter User Guide
created: 2026-03-26
tags:
  - synroute
  - guide
  - getting-started
aliases:
  - User Guide
  - Getting Started
---

# SynapseRouter User Guide

SynapseRouter (synroute) is a Go-based LLM proxy router and coding agent. It distributes requests across Ollama Cloud, subscription providers (Gemini, Codex, Claude Code), and Vertex AI, with automatic escalation when providers fail or rate-limit.

This guide covers daily usage. For the full CLI reference, see [[CLI Commands]].

---

## Getting Started

### Build

```bash
cd ~/Development/synapserouter
go build -o synroute .
```

### Configure

Create a `.env` file in the project root. The minimum configuration requires a profile and at least one provider.

**Personal profile** (Ollama Cloud primary):

```env
ACTIVE_PROFILE=personal
OLLAMA_API_KEY=your-ollama-key
# Multiple keys for concurrent subscriptions (round-robin):
# OLLAMA_API_KEYS=key1,key2,key3

# Optional subscription providers (disable with SUBSCRIPTIONS_DISABLED=true):
# GEMINI_API_KEY=your-gemini-key
# SUBSCRIPTIONS_DISABLED=true
```

**Work profile** (Vertex AI only):

```env
ACTIVE_PROFILE=work
GCP_PROJECT_ID=your-project
GCP_REGION=us-central1
```

### Verify Setup

```bash
./synroute doctor         # Check configuration and provider health
./synroute test           # Smoke test all providers
./synroute models         # List available models
```

### Start the Server

```bash
./synroute                # Starts HTTP server on :8090 (default)
./synroute serve          # Same, explicit command
```

The server exposes the OpenAI-compatible API at `/v1/chat/completions` and diagnostic endpoints. See [[CLI Commands#synroute serve]] for details.

---

## Using synroute chat

The `chat` command is the primary interface for the coding agent. It supports two modes: interactive REPL and one-shot.

### Interactive REPL

```bash
./synroute chat
```

This opens an interactive session where you can converse with the agent. The agent has access to tools (bash, file read/write/edit, grep, glob, git) and will execute them to complete your tasks.

**REPL commands:**

| Command     | Description                    |
| ----------- | ------------------------------ |
| `/exit`     | Exit the session               |
| `/clear`    | Clear conversation history     |
| `/model`    | Show or change the active model |
| `/tools`    | List available tools           |
| `/history`  | Show conversation history      |
| `/agents`   | Show sub-agent status          |
| `/budget`   | Show token budget usage        |

### One-Shot Mode

Send a single message and get a result. The agent works in your current directory and files persist after it exits.

```bash
./synroute chat --message "fix the failing tests in internal/router/"
```

### Spec File Mode

Pass a specification file. The agent detects whether you have an existing project and adjusts its behavior (build from scratch vs. review/fix existing).

```bash
./synroute chat --spec-file ~/specs/my-feature.md
```

### Common Flags

```bash
./synroute chat --model claude-sonnet-4-6   # Use a specific model
./synroute chat --system "You are a Go expert"  # Custom system prompt
./synroute chat --project my-app                 # Create ~/Development/my-app/ and work there
./synroute chat --worktree                       # Isolated git worktree
./synroute chat --max-agents 3                   # Limit concurrent sub-agents
./synroute chat --budget 10000                   # Max total tokens
./synroute chat --resume                         # Resume most recent session
./synroute chat --session <id>                   # Resume a specific session
./synroute chat -v                               # Normal verbosity
./synroute chat -vv                              # Verbose output
```

### Session Persistence

Sessions are automatically saved to SQLite on exit. Resume them later:

```bash
./synroute chat --resume              # Most recent session
./synroute chat --session abc123      # Specific session by ID
```

### Worktree Isolation

The `--worktree` flag creates a managed git worktree so the agent works on an isolated branch. Changes do not affect your main branch until you merge.

```bash
./synroute chat --worktree
# On exit, synroute prints the worktree path and branch name.
# Merge the branch to keep changes, or discard:
# synroute worktree delete <id>
```

Worktrees have a 24-hour TTL, a 2GB per-tree size cap, and a 10GB total cap. Background cleanup runs every 5 minutes.

---

## Using the Agent API

The server exposes an agent endpoint for programmatic access.

### POST /v1/agent/chat

```bash
curl -X POST localhost:8090/v1/agent/chat \
  -H "Content-Type: application/json" \
  -d '{
    "message": "fix the tests",
    "model": "auto",
    "work_dir": "/path/to/project"
  }'
```

The `model` field defaults to `"auto"`, which lets the router pick the best provider. You can specify a model name to target a specific provider.

The `work_dir` field sets the agent's working directory. If omitted, the agent uses the server's working directory.

### Agent Pool Metrics

```bash
curl localhost:8090/v1/agent/pool    # Show active/available agents
```

---

## Profiles

Synroute supports two profiles that control which providers are available.

### Personal

- **Primary:** Ollama Cloud with 7-level escalation chain (19+ models from fast/cheap to frontier)
- **Fallback:** Optional subscription providers (Gemini, Codex, Claude Code)
- Supports multiple API keys for concurrent requests
- Disable subscriptions with `SUBSCRIPTIONS_DISABLED=true`

### Work

- **Vertex AI only** with native GCP authentication
- Claude models via `rawPredict`, Gemini models via `generateContent`
- Uses model names without dates (e.g., `claude-sonnet-4-6`)

### Switching Profiles

```bash
./synroute profile show                # Show active profile and providers
./synroute profile list                # List available profiles
./synroute profile switch work         # Switch to work profile
```

After switching, restart the server for changes to take effect.

---

## Skill Auto-Dispatch

When you submit a task, synroute automatically matches it against 38+ built-in skills based on keyword triggers. Skills are chained, not selected one at a time.

### How It Works

1. **Trigger matching** -- your message text is scanned against skill triggers
2. **Phase ordering** -- matched skills are sorted: `analyze` then `implement` then `verify` then `review`
3. **Prompt injection** -- each matched skill's instructions are injected into the system prompt
4. **MCP tool binding** -- skills can auto-invoke bound MCP tools and inject results as context
5. **Fallback** -- if no skills match, the system falls back to role-based inference

### Example

A message like "fix the Go authentication bug" would trigger:

- `go-patterns` (Go code detected)
- `code-implement` (fix/implement detected)
- `security-review` (auth detected)
- `go-testing` (verification phase)
- `code-review` (review phase)

All five skills fire automatically. You do not need to select them.

### Viewing Skills

```bash
# List all registered skills
curl localhost:8090/v1/skills

# Preview which skills would match a given goal
curl "localhost:8090/v1/skills/match?q=fix+the+Go+auth+bug"
```

### Adding Custom Skills

Drop a `.md` file with YAML frontmatter into `internal/orchestration/skilldata/` and rebuild. No Go code changes needed.

```yaml
---
name: my-skill
triggers: [keyword1, keyword2]
role: You are an expert in X.
phase: implement
mcp_tools: []
---

Detailed instructions for the skill go here.
```

---

## Tips for Best Results

### Be Specific

The agent works best with concrete, scoped requests. Instead of "make it better," say "refactor the router to use interfaces instead of concrete types."

### Let the Agent Plan

For non-trivial tasks, the agent will plan before implementing. Do not interrupt the planning phase. The plan produces acceptance criteria that all later phases check against.

### Use model: auto

Unless you need a specific model, let the router pick. It starts with fast/cheap models and escalates automatically when needed. This optimizes cost without sacrificing quality.

### Use --project for New Work

The `--project` flag creates a dedicated directory under `~/Development/` and sets the agent's working directory there. This keeps new projects organized and isolated.

```bash
./synroute chat --project my-new-api
```

### Use --worktree for Risky Changes

When making changes to an existing codebase that you might want to discard, use `--worktree` for isolation.

### Check the Logs

Each chat run creates a timestamped log file at `.synroute/logs/run-<timestamp>.log` in the working directory. Check these for debugging when things go wrong.

### Post-Change Pipeline

After any code change, the agent automatically runs:

1. `go vet ./...`
2. `go test -race ./...`
3. `./synroute test` (E2E provider smoke test)
4. `go build -o synroute .`

This pipeline runs without prompting. If you see the agent running tests after your change, that is expected behavior.

---

## Related Pages

- [[CLI Commands]] -- Full CLI reference with all flags
- [[Architecture]] -- System architecture and provider chain details
