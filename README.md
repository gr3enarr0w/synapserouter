# Synapse Router

`synroute` is the unified Go runtime for local LLM routing and orchestration inside this repo.

It combines:
- multi-provider LLM routing and fallback
- embedded subscription-backed providers
- OpenAI-compatible chat and responses APIs
- in-process orchestration rewritten from `ruflo`

## Subscription Status

`synroute` can route to Claude, Codex/OpenAI, and Gemini using direct API keys, pre-obtained session-token/cookie credentials, or the built-in OAuth/browser login flow.

Current boundary:
- supported: embedded provider routing with API keys
- supported: embedded provider routing with supplied session-token credentials
- supported: built-in OAuth/browser login acquisition and refresh flow from archived `CLIProxyAPI`

## What It Does

- Routes requests across Ollama Cloud (6-level escalation, 19+ models), subscription providers (Gemini, Codex, Claude Code), and Vertex AI
- Exposes `/v1/chat/completions`, `/v1/responses`, `/v1/models`, and provider-specific compatibility routes
- Stores usage, tool outputs, and orchestration state in SQLite
- 54 embedded skills with trigger-based matching and language-field routing
- Spec compliance system with constraint extraction and tool-layer protection
- 6-phase pipeline: Plan → Implement → Self-Check → Code Review → Acceptance Test → Deploy
- Provides orchestration APIs for tasks, swarms, agents, workflows, load balancing, work stealing, and workflow execution state

## Quick Start

```bash
cd src/services/synapse-router
cp .env.example .env
# Edit .env with your provider credentials
./start.sh
```

Or using make:
```bash
make start
```

See [API_EXAMPLES.md](./API_EXAMPLES.md) for detailed usage examples.

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

## Current Rewrite Status

`CLIProxyAPI`:
- embedded and operational inside `synroute`
- targeted compatibility bucket is implemented
- direct API-key, supplied session-token, and built-in OAuth/browser login routing are present
- full repo-to-repo parity is not yet proven by a formal audit

`ruflo`:
- substantial runtime behavior is now embedded
- task/swarm/agent/workflow APIs, event streaming, load balancing, stealing, contests, and workflow state are present
- deeper coordinator semantics, nested/dependency workflow semantics, plugin hooks, and final parity audit still remain

## Testing and Verification

Run all tests:
```bash
make test
```

Run integration tests:
```bash
./run_integration_tests.sh
```

Or manually:
```bash
go test ./...                          # Unit tests
go test -tags=integration ./...        # Integration tests
go build ./...                         # Build verification
```

## Documentation

- **[API_EXAMPLES.md](./API_EXAMPLES.md)** - Practical API usage examples with curl, Python, and JavaScript
- **[COMPLETION_REPORT.md](./COMPLETION_REPORT.md)** - Complete feature status and gap analysis
- **[STATUS.md](./STATUS.md)** - Current implementation status
- **[.env.example](./.env.example)** - Environment configuration template

## Development

Common commands:
```bash
make build      # Build binary
make test       # Run tests
make start      # Start server
make clean      # Clean build artifacts
make fmt        # Format code
make check      # Run linters
make help       # Show all commands
```
