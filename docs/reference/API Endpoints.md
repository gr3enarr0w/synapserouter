---
title: API Endpoints
type: reference
created: 2026-03-26
tags: [api, endpoints, http, rest]
related:
  - "[[Provider Chain]]"
  - "[[main.go]]"
  - "[[compat_handlers.go]]"
  - "[[diagnostic_handlers.go]]"
---

# API Endpoints

SynapseRouter exposes HTTP endpoints on port `8090` (configurable via `PORT` env var). Admin endpoints require either `SYNROUTE_ADMIN_TOKEN` / `ADMIN_API_KEY` header auth, or localhost access.

Default: `http://localhost:8090`

## Authentication

Admin-protected endpoints accept auth via:
- `Authorization: Bearer <token>` header
- `X-Admin-API-Key: <token>` header
- Localhost access (if no token is configured)

---

## Health & Status

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/health` | No | Health check with circuit breaker states |
| `GET` | `/v1/startup-check` | No | Startup probe results for all providers |

### `GET /health`

```json
{
  "status": "ok",
  "timestamp": 1711411200,
  "circuit_breakers": {
    "ollama-chain-1": "closed",
    "gemini": "closed"
  }
}
```

---

## Chat & Completions (OpenAI-compatible)

| Method | Path | Auth | Description |
|---|---|---|---|
| `POST` | `/v1/chat/completions` | No | OpenAI-compatible chat completion (streaming supported) |
| `GET` | `/v1/models` | No | List available models |
| `POST` | `/api/provider/{provider}/v1/chat/completions` | No | Chat completion pinned to a specific provider |
| `GET` | `/api/provider/{provider}/v1/models` | No | List models for a specific provider |

### `POST /v1/chat/completions`

Standard OpenAI chat completions format. Set `model` to `"auto"` for router-selected provider.

**Request:**

```json
{
  "model": "auto",
  "messages": [
    {"role": "user", "content": "Hello, world!"}
  ],
  "stream": false
}
```

**Headers (optional):**
- `X-Session-ID` -- session identifier for memory continuity
- `X-Debug-Memory: true` -- include memory retrieval debug info in response
- `X-Skip-Skills: true` -- skip skill-aware preprocessing

**Response:**

```json
{
  "id": "chatcmpl-abc123",
  "object": "chat.completion",
  "model": "qwen3.5",
  "choices": [
    {
      "index": 0,
      "message": {"role": "assistant", "content": "Hello!"},
      "finish_reason": "stop"
    }
  ],
  "usage": {"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15}
}
```

When `stream: true`, returns SSE (`text/event-stream`) in OpenAI chunk format.

---

## Responses API (OpenAI Responses format)

| Method | Path | Auth | Description |
|---|---|---|---|
| `POST` | `/v1/responses` | No | Create a response (SSE streaming supported) |
| `POST` | `/v1/responses/compact` | No | Create a compact response (no streaming) |
| `GET` | `/v1/responses/{response_id}` | No | Retrieve a stored response |
| `DELETE` | `/v1/responses/{response_id}` | No | Delete a stored response |
| `POST` | `/api/provider/{provider}/v1/responses` | No | Create response pinned to provider |

### `POST /v1/responses`

**Request:**

```json
{
  "model": "auto",
  "input": "Explain circuit breakers",
  "instructions": "Be concise",
  "stream": true,
  "previous_response_id": "resp-xyz",
  "tools": [],
  "max_output_tokens": 4096
}
```

The `input` field accepts a plain string, an array of chat messages, or structured OpenAI Responses API items (including `function_call` and `function_call_output` types).

**Response (non-streaming):**

```json
{
  "id": "resp-abc123",
  "object": "response",
  "model": "qwen3.5",
  "output": [{"type": "message", "role": "assistant", "content": [{"type": "output_text", "text": "..."}]}],
  "output_text": "Circuit breakers prevent cascading failures...",
  "usage": {"prompt_tokens": 15, "completion_tokens": 42, "total_tokens": 57}
}
```

---

## Anthropic Messages API

| Method | Path | Auth | Description |
|---|---|---|---|
| `POST` | `/v1/messages` | No | Anthropic-compatible messages endpoint |
| `POST` | `/api/provider/{provider}/v1/messages` | No | Messages endpoint pinned to provider |

---

## Providers & Usage

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/v1/providers` | No | List all providers with health, usage, and circuit state |
| `GET` | `/v1/usage` | Admin | Usage stats and quota info per provider |

### `GET /v1/providers`

```json
{
  "count": 21,
  "providers": [
    {
      "name": "ollama-chain-1",
      "healthy": true,
      "max_context_tokens": 128000,
      "circuit_state": "closed",
      "current_usage": 12500,
      "daily_limit": 1000000,
      "usage_percent": "1.3%",
      "tier": "pro"
    }
  ]
}
```

---

## Skills & Tools

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/v1/skills` | Admin | List all registered skills |
| `GET` | `/v1/skills/match?q=...` | Admin | Preview skill chain for a goal |
| `GET` | `/v1/tools` | Admin | List agent tools |

### `GET /v1/skills/match?q=fix+the+Go+auth`

Returns the matched skill chain ordered by phase (`analyze` -> `implement` -> `verify` -> `review`).

---

## Agent SDK

| Method | Path | Auth | Description |
|---|---|---|---|
| `POST` | `/v1/agent/chat` | Admin | Run an agent task (one-shot) |
| `GET` | `/v1/agent/pool` | Admin | Agent pool metrics |

### `POST /v1/agent/chat`

**Request:**

```json
{
  "message": "fix the failing tests",
  "model": "auto",
  "system": "You are a Go expert",
  "max_turns": 20,
  "max_tokens": 50000,
  "session_id": "session-123",
  "work_dir": "/path/to/project",
  "project": "my-app"
}
```

If `project` is provided, the agent works in `~/Development/<project>/`. If neither `work_dir` nor `project` is set, a temp directory is created.

**Response:**

```json
{
  "session_id": "agent-abc123",
  "model": "auto",
  "response": "Fixed 3 failing tests in internal/router/...",
  "trace": {"spans": 12, "duration": "45.2s"},
  "pool": {"active": 1, "max": 5, "total_runs": 42}
}
```

---

## Memory & Audit

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/v1/memory/search?session_id=...&q=...` | Admin | Search vector memory (semantic or recent) |
| `GET` | `/v1/memory/session/{session_id}` | Admin | Get full session history |
| `GET` | `/v1/audit/session/{session_id}` | Admin | Audit trail for a session |
| `GET` | `/v1/audit/request/{request_id}` | Admin | Audit detail for a single request (includes provider attempts) |
| `POST` | `/v1/debug/trace` | Admin | Trace provider selection decision without executing |

### `GET /v1/memory/search`

**Query parameters:**
- `session_id` (required) -- session to search
- `q` -- semantic search query (omit for recent messages)
- `max_tokens` -- max token budget for results (default: 4000)

### `GET /v1/audit/request/{request_id}`

Returns the full audit trail for a request including each provider attempt:

```json
{
  "request_id": "req-123",
  "session_id": "session-abc",
  "selected_provider": "ollama-chain-1",
  "final_provider": "ollama-chain-5",
  "final_model": "nemotron-3-super",
  "success": true,
  "attempts": [
    {"provider": "ollama-chain-1", "attempt_index": 1, "success": false, "error_message": "rate limit"},
    {"provider": "ollama-chain-5", "attempt_index": 2, "success": true, "error_message": ""}
  ]
}
```

---

## Session Management

| Method | Path | Auth | Description |
|---|---|---|---|
| `POST` | `/v1/session/end` | No | Explicitly end a session |

**Request:** `X-Session-ID` header or JSON body `{"session_id": "..."}`.

---

## Orchestration

### Workflows

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/v1/orchestration/roles` | Admin | List orchestration roles |
| `GET` | `/v1/orchestration/workflows` | Admin | List workflow templates |
| `POST` | `/v1/orchestration/workflows` | Admin | Create a workflow template |
| `GET` | `/v1/orchestration/workflows/{template_id}` | Admin | Get workflow template |
| `PUT` | `/v1/orchestration/workflows/{template_id}` | Admin | Update workflow template |
| `DELETE` | `/v1/orchestration/workflows/{template_id}` | Admin | Delete workflow template |
| `POST` | `/v1/orchestration/workflows/{template_id}/run` | Admin | Run a workflow |

### Execution Monitoring

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/v1/orchestration/executions/{workflow_id}/state` | Admin | Execution state |
| `GET` | `/v1/orchestration/executions/{workflow_id}/metrics` | Admin | Execution metrics |
| `GET` | `/v1/orchestration/executions/{workflow_id}/debug` | Admin | Execution debug info |

### Tasks

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/v1/orchestration/tasks` | Admin | List tasks (filter by `session_id`, `status`) |
| `POST` | `/v1/orchestration/tasks` | Admin | Create a task |
| `GET` | `/v1/orchestration/tasks/{task_id}` | Admin | Get task details |
| `POST` | `/v1/orchestration/tasks/{task_id}/run` | Admin | Start a task |
| `POST` | `/v1/orchestration/tasks/{task_id}/pause` | Admin | Pause a task |
| `POST` | `/v1/orchestration/tasks/{task_id}/resume` | Admin | Resume a paused task |
| `POST` | `/v1/orchestration/tasks/{task_id}/cancel` | Admin | Cancel a task |
| `POST` | `/v1/orchestration/tasks/{task_id}/assign` | Admin | Assign task to agent |
| `POST` | `/v1/orchestration/tasks/{task_id}/steal` | Admin | Steal task from another agent |
| `POST` | `/v1/orchestration/tasks/{task_id}/contest` | Admin | Contest a task assignment |
| `POST` | `/v1/orchestration/tasks/{task_id}/contest/resolve` | Admin | Resolve a task contest |
| `POST` | `/v1/orchestration/tasks/{task_id}/refine` | Admin | Refine a task with new input |
| `GET` | `/v1/orchestration/tasks/{task_id}/events` | Admin | Task event stream |

### Sessions

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/v1/orchestration/sessions/{session_id}/tasks` | Admin | List tasks for a session |
| `POST` | `/v1/orchestration/sessions/{session_id}/resume` | Admin | Resume a session's task |
| `POST` | `/v1/orchestration/sessions/{session_id}/fork` | Admin | Fork a session's task |

### Agents

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/v1/orchestration/agents` | Admin | List orchestration agents |
| `POST` | `/v1/orchestration/agents` | Admin | Register an agent |
| `GET` | `/v1/orchestration/agents/health` | Admin | Agent health overview |
| `GET` | `/v1/orchestration/agents/{agent_id}` | Admin | Get agent details |
| `GET` | `/v1/orchestration/agents/{agent_id}/status` | Admin | Agent status |
| `POST` | `/v1/orchestration/agents/{agent_id}/stop` | Admin | Stop an agent |
| `GET` | `/v1/orchestration/agents/{agent_id}/metrics` | Admin | Agent metrics |
| `GET` | `/v1/orchestration/agents/{agent_id}/logs` | Admin | Agent logs |

### Swarms

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/v1/orchestration/swarms` | Admin | List swarms |
| `POST` | `/v1/orchestration/swarms` | Admin | Create a swarm |
| `GET` | `/v1/orchestration/swarms/{swarm_id}` | Admin | Get swarm details |
| `GET` | `/v1/orchestration/swarms/{swarm_id}/status` | Admin | Swarm status |
| `POST` | `/v1/orchestration/swarms/{swarm_id}/start` | Admin | Start a swarm |
| `POST` | `/v1/orchestration/swarms/{swarm_id}/stop` | Admin | Stop a swarm |
| `POST` | `/v1/orchestration/swarms/{swarm_id}/pause` | Admin | Pause a swarm |
| `POST` | `/v1/orchestration/swarms/{swarm_id}/resume` | Admin | Resume a swarm |
| `POST` | `/v1/orchestration/swarms/{swarm_id}/scale` | Admin | Scale swarm agents |
| `POST` | `/v1/orchestration/swarms/{swarm_id}/coordinate` | Admin | Coordinate swarm tasks |
| `GET` | `/v1/orchestration/swarms/{swarm_id}/load` | Admin | Swarm load metrics |
| `GET` | `/v1/orchestration/swarms/{swarm_id}/imbalance` | Admin | Swarm load imbalance |
| `POST` | `/v1/orchestration/swarms/{swarm_id}/rebalance/preview` | Admin | Preview rebalance plan |
| `GET` | `/v1/orchestration/swarms/{swarm_id}/stealable` | Admin | List stealable tasks |

---

## Eval Framework

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/v1/eval/exercises` | Admin | List imported eval exercises (filter by `language`) |
| `GET` | `/v1/eval/runs` | Admin | List recent eval runs |
| `POST` | `/v1/eval/runs` | Admin | Start an eval run |
| `GET` | `/v1/eval/runs/{run_id}` | Admin | Get run status and summary |
| `GET` | `/v1/eval/runs/{run_id}/results` | Admin | Get individual run results |
| `POST` | `/v1/eval/compare` | Admin | Compare two eval runs |
| `POST` | `/v1/eval/import` | Admin | Import eval exercises |

### `POST /v1/eval/runs`

```json
{
  "languages": ["go", "python"],
  "count": 20,
  "provider": "ollama-chain-1",
  "two_pass": true
}
```

---

## Diagnostics & Management

| Method | Path | Auth | Description |
|---|---|---|---|
| `POST` | `/v1/test/providers` | Admin | Smoke test providers |
| `POST` | `/v1/circuit-breakers/reset` | Admin | Reset circuit breakers (all or specific) |
| `GET` | `/v1/profile` | Admin | Show active profile info |
| `POST` | `/v1/profile/switch` | Admin | Switch profile (requires restart) |
| `GET` | `/v1/doctor` | Admin | Run diagnostic checks |

### `POST /v1/test/providers`

```json
{
  "provider": "ollama-chain-1",
  "timeout": "30s"
}
```

Omit `provider` to test all. Returns per-provider success/failure results.

### `POST /v1/profile/switch`

```json
{"profile": "work"}
```

Updates `.env` file. Server restart required to take effect.

---

## Subscription OAuth

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/anthropic/callback` | No | OAuth callback for Anthropic |
| `GET` | `/codex/callback` | No | OAuth callback for OpenAI/Codex |
| `GET` | `/google/callback` | No | OAuth callback for Gemini |

---

## Amp Code Management (`/v0/management`)

These endpoints manage Amp Code compatibility configuration (upstream proxy, API keys, model mappings).

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/v0/management/anthropic-auth-url` | Admin | Get Anthropic OAuth URL |
| `GET` | `/v0/management/codex-auth-url` | Admin | Get Codex OAuth URL |
| `GET` | `/v0/management/gemini-cli-auth-url` | Admin | Get Gemini OAuth URL |
| `POST` | `/v0/management/oauth-callback` | Admin | Handle OAuth callback |
| `GET` | `/v0/management/get-auth-status` | Admin | Check auth status for all providers |
| `GET` | `/v0/management/ampcode` | Admin | Get full Amp config |
| `GET/PUT/DELETE` | `/v0/management/ampcode/upstream-url` | Admin | Manage upstream URL |
| `GET/PUT/DELETE` | `/v0/management/ampcode/upstream-api-key` | Admin | Manage single upstream API key |
| `GET/PUT/PATCH/DELETE` | `/v0/management/ampcode/upstream-api-keys` | Admin | Manage upstream API key entries |
| `GET/PUT/PATCH/DELETE` | `/v0/management/ampcode/model-mappings` | Admin | Manage model name mappings |
| `GET/PUT` | `/v0/management/ampcode/force-model-mappings` | Admin | Toggle forced model mapping |
| `GET/PUT` | `/v0/management/ampcode/restrict-management-to-localhost` | Admin | Toggle localhost restriction |

---

## MCP Server

Enabled with `SYNROUTE_MCP_SERVER=true` or via `synroute mcp-serve`.

| Method | Path | Auth | Description |
|---|---|---|---|
| `POST` | `/mcp/initialize` | No | MCP protocol initialization |
| `GET/POST` | `/mcp/tools/list` | No | List available MCP tools |
| `POST` | `/mcp/tools/call` | No | Execute an MCP tool |

### `POST /mcp/tools/call`

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/call",
  "params": {
    "name": "bash",
    "arguments": {"command": "ls -la"}
  }
}
```
