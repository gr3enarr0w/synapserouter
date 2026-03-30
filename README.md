# Synapse Router

SynapseRouter (`synroute`) is a Go-based LLM proxy router and coding agent. It routes requests across multiple LLM providers with automatic fallback and escalation, and includes an interactive coding agent with tool execution (bash, file I/O, grep, glob, git, web search).

## What It Does

- **Code mode TUI** -- the default interface. Type `synroute code` (or just `synroute`) and start prompting. The agent reads, writes, and edits files, runs commands, and manages git on your behalf.
- **Provider routing** -- distributes requests across Ollama Cloud, subscription providers (Gemini, Codex, Claude Code), and Vertex AI with automatic escalation when a provider fails or rate-limits.
- **54 embedded skills** that fire automatically based on what you ask (Go patterns, testing, security review, etc.).
- **OpenAI-compatible API** at `/v1/chat/completions` for use with other tools.

## Prerequisites

- **Go 1.22 or later** -- [install instructions](https://go.dev/doc/install). On macOS with Homebrew: `brew install go`.
- **Git** -- required for worktree isolation and the git tool.
- **An LLM provider account** -- at minimum, one of:
  - [Ollama Cloud](https://ollama.com) subscription (personal profile)
  - Google Cloud project with Vertex AI enabled (work profile)
  - Gemini, OpenAI, or Anthropic API key (subscription providers)

## Quick Start

### 1. Clone and build

```bash
git clone https://github.com/gr3enarr0w/mcp-ecosystem.git
cd mcp-ecosystem/synapse-router
go build -o synroute .
```

This produces a `synroute` binary in the current directory.

### 2. Make it available system-wide (optional)

Pick one of these approaches so you can run `synroute` from any directory:

```bash
# Option A: symlink into a directory already on your PATH
ln -s "$(pwd)/synroute" /usr/local/bin/synroute

# Option B: symlink into ~/bin (create it if needed, then add to PATH)
mkdir -p ~/bin
ln -s "$(pwd)/synroute" ~/bin/synroute
# Add to your shell profile (~/.zshrc or ~/.bashrc) if not already there:
#   export PATH="$HOME/bin:$PATH"

# Option C: use `make install` (copies to /usr/local/bin, may need sudo)
make install
```

### 3. Configure your `.env` file

Copy the example and fill in your credentials:

```bash
cp .env.example .env
```

Open `.env` in your editor. The two most important settings are the **profile** and at least one **provider key**.

**Personal profile** (Ollama Cloud as the primary provider):

```env
ACTIVE_PROFILE=personal

# Required: your Ollama Cloud API key
OLLAMA_API_KEY=your-ollama-api-key

# Optional: model escalation chain (pipe-separated levels, comma-separated models within a level)
# OLLAMA_CHAIN=ministral-3:14b,gpt-oss:20b|qwen3-8b|gemma3-27b

# Optional: disable subscription fallback providers
# SUBSCRIPTIONS_DISABLED=true
```

**Work profile** (Vertex AI with Google Cloud):

```env
ACTIVE_PROFILE=work

# Required: your GCP project and region
GCP_PROJECT_ID=your-gcp-project
GCP_REGION=us-central1
# Authentication: run `gcloud auth application-default login` first
```

See `.env.example` for all available settings including subscription provider keys (Gemini, OpenAI, Anthropic).

### 4. Verify your setup

```bash
./synroute doctor    # Check configuration and provider health
./synroute test      # Smoke test all configured providers
```

### 5. Start using it

```bash
./synroute           # Opens the code mode TUI (default command)
```

Type a prompt and press Enter. The agent will read files, run commands, and write code. Type `/help` for keyboard shortcuts, or `/exit` to quit.

### Alternative: start the HTTP server

```bash
./synroute serve     # Starts the API server on port 8090
```

See [docs/guides/User Guide.md](./docs/guides/User%20Guide.md) for the full user guide and [docs/reference/CLI Commands.md](./docs/reference/CLI%20Commands.md) for every command and flag.

## Primary Endpoints

Routing:
- `POST /v1/chat/completions`
- `POST /v1/responses`
- `POST /v1/responses/compact`
- `GET /v1/responses/{response_id}`
- `DELETE /v1/responses/{response_id}`
- `GET /v1/models`
- `GET /v1/providers`
- `GET /health`

Orchestration:
- `GET /v1/orchestration/roles`
- `GET|POST /v1/orchestration/tasks`
- `GET /v1/orchestration/tasks/{task_id}`
- `POST /v1/orchestration/tasks/{task_id}/run`
- `POST /v1/orchestration/tasks/{task_id}/pause`
- `POST /v1/orchestration/tasks/{task_id}/resume`
- `POST /v1/orchestration/tasks/{task_id}/cancel`
- `POST /v1/orchestration/tasks/{task_id}/assign`
- `POST /v1/orchestration/tasks/{task_id}/steal`
- `POST /v1/orchestration/tasks/{task_id}/contest`
- `POST /v1/orchestration/tasks/{task_id}/contest/resolve`
- `POST /v1/orchestration/tasks/{task_id}/refine`
- `GET /v1/orchestration/tasks/{task_id}/events`
- `GET|POST /v1/orchestration/agents`
- `GET /v1/orchestration/agents/{agent_id}`
- `GET /v1/orchestration/agents/{agent_id}/status`
- `POST /v1/orchestration/agents/{agent_id}/stop`
- `GET /v1/orchestration/agents/{agent_id}/metrics`
- `GET /v1/orchestration/agents/{agent_id}/logs`
- `GET|POST /v1/orchestration/swarms`
- `GET /v1/orchestration/swarms/{swarm_id}`
- `GET /v1/orchestration/swarms/{swarm_id}/status`
- `POST /v1/orchestration/swarms/{swarm_id}/start`
- `POST /v1/orchestration/swarms/{swarm_id}/stop`
- `POST /v1/orchestration/swarms/{swarm_id}/pause`
- `POST /v1/orchestration/swarms/{swarm_id}/resume`
- `POST /v1/orchestration/swarms/{swarm_id}/scale`
- `POST /v1/orchestration/swarms/{swarm_id}/coordinate`
- `GET /v1/orchestration/swarms/{swarm_id}/load`
- `GET /v1/orchestration/swarms/{swarm_id}/imbalance`
- `POST /v1/orchestration/swarms/{swarm_id}/rebalance/preview`
- `GET /v1/orchestration/swarms/{swarm_id}/stealable`
- `GET|POST /v1/orchestration/workflows`
- `GET|PUT|DELETE /v1/orchestration/workflows/{template_id}`
- `POST /v1/orchestration/workflows/{template_id}/run`
- `GET /v1/orchestration/executions/{workflow_id}/state`
- `GET /v1/orchestration/executions/{workflow_id}/metrics`
- `GET /v1/orchestration/executions/{workflow_id}/debug`

## Profiles

Synroute has two profiles that control which LLM providers are available.

| Profile | Primary Provider | Fallback | When to use |
| --- | --- | --- | --- |
| `personal` | Ollama Cloud (multi-level escalation) | Gemini, OpenAI, Anthropic (optional) | Personal development, home use |
| `work` | Vertex AI (Claude + Gemini via GCP) | models.corp (optional) | Corporate environment with GCP |

Switch profiles by editing `ACTIVE_PROFILE` in `.env`, or override per-run:

```bash
ACTIVE_PROFILE=work ./synroute code
```

## Testing and Verification

```bash
go test ./...                          # Unit tests
go test -race ./...                    # Tests with race detection
go vet ./...                           # Lint
go build -o synroute .                 # Build verification
./synroute test                        # E2E provider smoke test
```

Or with make:

```bash
make build      # Build binary
make test       # Run tests with race detection
make start      # Build and start server
make install    # Install to /usr/local/bin
make help       # Show all available commands
```

## Documentation

- **[User Guide](./docs/guides/User%20Guide.md)** -- Getting started, daily usage, tips
- **[CLI Commands](./docs/reference/CLI%20Commands.md)** -- Every command, flag, and example
- **[API_EXAMPLES.md](./API_EXAMPLES.md)** -- Curl, Python, and JavaScript API examples
- **[.env.example](./.env.example)** -- All environment variables with descriptions
