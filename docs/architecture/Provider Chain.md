---
title: Provider Chain
type: architecture
created: 2026-03-26
tags: [routing, providers, ollama, circuit-breaker, escalation]
related:
  - "[[API Endpoints]]"
  - "[[internal/router/router.go]]"
  - "[[internal/router/circuit.go]]"
---

# Provider Chain

SynapseRouter distributes LLM requests across a multi-level provider chain. The router selects the best available provider based on usage quotas, circuit breaker state, and health checks, then falls back through the chain on failure.

## Profiles

SynapseRouter supports two profiles, controlled by the `ACTIVE_PROFILE` environment variable.

### Personal Profile (default)

The personal profile uses **Ollama Cloud** as the primary provider with a dynamic multi-level escalation chain (configured via `OLLAMA_CHAIN` env var), plus optional subscription providers as fallback.

Provider initialization order:

1. Subscription providers (gemini, codex, claude-code) -- loaded first via `subscriptions.LoadRuntimeProviders`
2. Ollama Cloud planners (optional, separate phase)
3. Ollama Cloud chain models (L0-L6)

Subscription providers can be disabled entirely with `SUBSCRIPTIONS_DISABLED=true`.

### Work Profile

The work profile uses **Vertex AI** exclusively:

| Provider | Backend | Auth | Config Env Vars |
|---|---|---|---|
| `vertex-claude` | Claude via Vertex rawPredict | ADC (Application Default Credentials) | `VERTEX_CLAUDE_PROJECT`, `VERTEX_CLAUDE_REGION` (default: `us-east5`) |
| `vertex-gemini` | Gemini via Vertex generateContent | ADC or service account | `VERTEX_GEMINI_PROJECT`, `VERTEX_GEMINI_LOCATION` (default: `global`), `VERTEX_GEMINI_SA_KEY` |

No Ollama Cloud or subscription providers are loaded in work profile.

## Ollama Cloud Escalation Chain

The chain is configured via the `OLLAMA_CHAIN` environment variable, with levels separated by `|` and models within a level separated by commas.

### 7-Level Model Hierarchy

| Level | Models | Class |
|---|---|---|
| L0 | `ministral-3:14b`, `rnj-1:8b`, `nemotron-3-nano:30b` | Fast/cheap |
| L1 | `gpt-oss:20b`, `devstral-small-2:24b`, `qwen3.5` | Small coders |
| L2 | `nemotron-3-super`, `gpt-oss:120b`, `minimax-m2.7` | Medium |
| L3 | `devstral-2:123b`, `qwen3-coder:480b` | Large coders |
| L4 | `qwen3.5:397b`, `kimi-k2.5`, `minimax-m2.5` | XL general |
| L5 | `deepseek-v3.1:671b`, `cogito-2.1:671b` | XXL reasoning |
| L6 | `glm-5`, `kimi-k2-thinking`, `glm-4.7` | Frontier |

Each model in the chain is registered as a separate provider named `ollama-chain-N` (e.g., `ollama-chain-1`, `ollama-chain-2`, etc.). When all providers at a given level fail (circuit breakers open), the router automatically moves to the next level.

### Planners

Two optional planner slots exist for planning-phase tasks, configured via `OLLAMA_PLANNER_1` and `OLLAMA_PLANNER_2` environment variables. Planners are registered as `ollama-planner-1` and `ollama-planner-2`.

### Subscription Fallback

After all Ollama Cloud levels are exhausted, requests fall through to subscription providers:

```
L0-L6 (Ollama Cloud) --> gemini --> codex --> claude-code
```

Default subscription provider order is `gemini,openai,anthropic` (configurable via env). Each uses OAuth-based credentials managed by `internal/subscriptions/`.

## Multiple API Keys

Ollama Cloud supports multiple concurrent API keys for higher throughput:

```env
OLLAMA_API_KEYS=key1,key2,key3
```

Keys are distributed across providers in **round-robin** fashion. Each chain model and planner gets the next key in rotation. This allows multiple paid subscriptions ($20/mo each) to run concurrently without hitting per-key rate limits.

If `OLLAMA_API_KEYS` is not set, the router falls back to a single `OLLAMA_API_KEY`. For local Ollama (no cloud), only `OLLAMA_BASE_URL` is needed.

## Provider Selection Algorithm

The router in `internal/router/router.go` selects providers using this logic:

1. **Filter candidates** -- only providers that support the requested model (or all if model is `auto`/empty)
2. **Usage-based ranking** -- select the provider with the lowest usage percentage via `usageTracker.GetBestProvider`
3. **Circuit breaker check** -- skip providers with open circuit breakers
4. **Health check** -- verify the provider is reachable (cached, see below)
5. **Fallback chain** -- if the primary fails, try remaining healthy providers ordered by usage

When a specific provider is requested (e.g., via `/api/provider/{provider}/v1/chat/completions`), the router uses `selectPreferredProvider` which validates the provider is healthy and its circuit is closed before routing directly to it.

## Circuit Breakers

Each provider gets a [[circuit breaker|Circuit Breaker]] instance (`internal/router/circuit.go`) backed by SQLite for persistence across restarts.

### States

| State | Meaning |
|---|---|
| `closed` | Normal operation, requests flow through |
| `open` | Provider is blocked, no requests sent until cooldown expires |
| `half_open` | Cooldown expired, next request is a probe -- success closes, failure re-opens |

### Thresholds and Cooldowns

- **Failure threshold**: 5 consecutive failures triggers open state
- **Default cooldown**: 5 minutes
- **Rate limit cooldowns** (dynamic, parsed from error messages):
  - If error contains `"reset after Ns"` -- uses N+5 seconds
  - `gemini` -- 30 seconds
  - `claude-code` -- 60 seconds
  - All others -- 2 minutes

### Rate Limit Detection

The router detects rate limits by checking error messages for:
- `"rate limit"` or `"429"`
- `"quota exceeded"`
- `"too many requests"`

When detected, the circuit breaker opens with a provider-specific cooldown rather than waiting for 5 failures.

### Management

Circuit breakers can be reset via:

```bash
# Reset all
curl -X POST localhost:8090/v1/circuit-breakers/reset

# Reset specific provider
curl -X POST localhost:8090/v1/circuit-breakers/reset \
  -d '{"provider":"ollama-chain-3"}'
```

## Health Caching

Health checks are cached for **2 minutes** (`healthCacheTTL`) to avoid burning provider API quota on repeated `IsHealthy` calls.

### Cache Behavior

- **Cache hit** (within TTL): return cached result immediately, no API call
- **Cache miss/stale**: perform real `IsHealthy` check, update cache
- **Circuit breaker open**: return unhealthy immediately without checking cache
- **Successful request**: resets cache to healthy with fresh timestamp

The cache is protected by a read-write mutex (`healthMu`) for concurrent access.

```
Request arrives
  --> Circuit breaker open? --> Return unhealthy (no API call)
  --> Cache valid (< 2min)?  --> Return cached result
  --> Cache stale/miss?     --> Call provider.IsHealthy(), update cache
```

## Stall Detection

If a provider takes longer than `STALL_TIMEOUT_SECONDS` (default: 180) and returns fewer than 50 characters, the router automatically retries with a continuation prompt. This handles providers that hang or return empty responses.

## Usage Tracking

Every successful request records token usage (reported or estimated at ~4 chars/token) in the SQLite database. The usage tracker enforces an **80% auto-switch threshold** -- when a provider exceeds 80% of its daily quota, the router prefers other providers.

### Default Quotas

| Provider | Daily Limit | Monthly Limit |
|---|---|---|
| `ollama-cloud` | 1,000,000 | 30,000,000 |
| `claude-code` | 500,000 | 15,000,000 |
| `gemini` | 500,000 | 15,000,000 |
| `codex` | 300,000 | 9,000,000 |

Quotas are configurable via environment variables (e.g., `OLLAMA_CLOUD_DAILY_LIMIT`).

## Request Flow

```
Client Request
  |
  v
routeChatRequest()
  |-- Validate model/provider compatibility
  |-- Resolve preferred provider from model family
  |
  v
Router.ChatCompletionWithDebug()
  |-- Refine vague prompts using conversation context
  |-- Touch session tracker
  |-- Retrieve memory (vector search, cross-session fallback)
  |-- Inject retrieved memory into request
  |-- Preprocess: inject matched skill context
  |-- Store new messages in vector memory
  |-- Select provider (usage + circuit + health)
  |
  v
tryProvidersWithFallback()
  |-- Try primary provider
  |   |-- Pre-check context limit
  |   |-- Call provider.ChatCompletion()
  |   |-- On failure: record in circuit breaker, check rate limit
  |   |-- On success: reset circuit, update health cache, track usage
  |
  |-- If primary fails: try fallback providers in usage order
  |
  v
handleStall() -- retry if response looks stalled
  |
  v
Store assistant response in memory
  |
  v
Persist audit trail (request_audit + provider_attempt_audit)
```

## Audit Trail

Every request generates audit records in SQLite:

- **`request_audit`** -- session, selected/final provider, model, memory stats, success/error
- **`provider_attempt_audit`** -- each provider attempt with index, success, and error details

Query audits via the [[API Endpoints#Audit|audit endpoints]].
