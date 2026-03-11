# Synapse Router - Setup and Launch Guide

## Quick Start: Connect to Claude, Gemini, and Codex Subscriptions

This guide shows you how to launch synapse-router and connect it to your subscription-backed LLM providers.

## Important Reality Check

Current `synroute` support for subscription-style providers is:

- supported: direct API-key auth for Anthropic, OpenAI, and Gemini
- supported: pre-obtained session-token or cookie credentials for Anthropic, OpenAI, and Gemini
- supported: built-in OAuth/browser login flow that acquires and refreshes those subscription credentials for you

If you already have valid session credentials from those providers, `synroute` can use them. If you prefer the runtime to perform the login flow itself, use `synroute-cli login anthropic`, `synroute-cli login openai`, or `synroute-cli login gemini`.

---

## Step 1: Configure Environment Variables

### 1.1 Create your .env file

```bash
cd /Users/ceverson/MCP_Advanced_Multi_Agent_Ecosystem/src/services/synapse-router
cp .env.example .env
```

### 1.2 Add Your Provider Credentials

Edit `.env` and set either API keys, session tokens, or a mix of both:

```bash
# ==========================================
# Subscription Provider Configuration
# ==========================================

# Provider routing order (left-to-right priority)
SYNROUTE_SUBSCRIPTION_PROVIDER_ORDER=anthropic,openai,gemini

# Claude (Anthropic) - For Claude Code / Sonnet models
SYNROUTE_ANTHROPIC_API_KEY=sk-ant-api03-XXXXXXXXXXXXXXXXXXXXX
SYNROUTE_ANTHROPIC_BASE_URL=https://api.anthropic.com
SYNROUTE_ANTHROPIC_SESSION_TOKEN=

# Codex (OpenAI) - For GPT-5 / Codex models
SYNROUTE_OPENAI_API_KEY=sk-proj-XXXXXXXXXXXXXXXXXXXXX
SYNROUTE_OPENAI_BASE_URL=https://api.openai.com/v1
SYNROUTE_OPENAI_SESSION_TOKEN=

# Gemini (Google) - For Gemini models
SYNROUTE_GEMINI_API_KEY=AIzaSyXXXXXXXXXXXXXXXXXXXXXXXX
SYNROUTE_GEMINI_BASE_URL=https://generativelanguage.googleapis.com/v1beta
SYNROUTE_GEMINI_SESSION_TOKEN=

# ==========================================
# Optional: Fallback Providers
# ==========================================

# NanoGPT (fallback provider with 2M context)
NANOGPT_API_KEY=your-nanogpt-key-here
NANOGPT_BASE_URL=https://nano-gpt.com
NANOGPT_MONTHLY_QUOTA=60000

# Ollama Cloud (optional)
# OLLAMA_API_KEY=your-ollama-key
# OLLAMA_BASE_URL=https://api.ollama.com
# OLLAMA_MODEL=llama3.1:8b

# ==========================================
# Optional: Vector Embeddings for Semantic Search
# ==========================================

# Set for real semantic search (recommended)
OPENAI_API_KEY=sk-XXXXXXXXXXXXXXXXXXXXX

# If not set, will use fast local hash-based embeddings

# ==========================================
# Server Configuration
# ==========================================

PORT=8090
DB_PATH=~/.mcp/proxy/usage.db

# Optional: Admin token for protected endpoints
# SYNROUTE_ADMIN_TOKEN=your-secret-admin-key
# ADMIN_API_KEY=your-secret-admin-key  # legacy fallback
```

### Important Notes:

- **Anthropic API Key**: Get from https://console.anthropic.com/ (Claude/Sonnet)
- **OpenAI API Key**: Get from https://platform.openai.com/ (Codex/GPT)
- **Gemini API Key**: Get from https://makersuite.google.com/app/apikey (Gemini)
- **Session tokens**: use these only if you already have valid provider cookie/session credentials
- **Provider Order**: Defines fallback chain - first provider is tried first
- **Vector Embeddings**: Optional OPENAI_API_KEY enables semantic search (recommended but not required)

### 1.3 `CLIProxyAPI` Compatibility Env Names

The runtime also accepts legacy compatibility env names from archived `CLIProxyAPI`, including:

```bash
CLIPROXY_PROVIDER_ORDER=anthropic,openai,gemini

CLIPROXY_ANTHROPIC_API_KEY=
CLIPROXY_ANTHROPIC_SESSION_TOKEN=

CLIPROXY_OPENAI_API_KEY=
CLIPROXY_OPENAI_SESSION_TOKEN=

CLIPROXY_GEMINI_API_KEY=
CLIPROXY_GEMINI_SESSION_TOKEN=
```

Use either the `SYNROUTE_*` names or the compatibility aliases, but prefer `SYNROUTE_*` for new setup.

---

## Step 2: Build the System

### 2.1 Build Server and CLI

```bash
# Build both server and CLI in one command
make build-all
```

This creates:
- `./synroute` - Main server binary (15MB)
- `./synroute-cli` - Command-line interface (11MB)

### 2.2 Alternative: Build Separately

```bash
# Build just the server
make build

# Build just the CLI
make build-cli
```

---

## Step 3: Launch Synapse Router

### 3.1 Quick Start (Recommended)

```bash
./start.sh
```

The `start.sh` script:
- Validates `.env` file exists
- Loads environment variables
- Builds if needed
- Starts the server

### 3.2 Manual Start

```bash
# Load environment
set -a
source .env
set +a

# Run the server
./synroute
```

### 3.3 Expected Output

```
🚀 Synapse Router starting on :8090
📊 Database: /Users/ceverson/.mcp/proxy/usage.db
🔄 Provider chain: Claude Code → Codex → Gemini → Qwen → Ollama Cloud → NanoGPT
💾 Unified context across all providers via vector memory
⚡ Usage tracking enabled (80% auto-switch threshold)
🧠 Orchestration roles loaded: 52
✓ anthropic subscription provider initialized
✓ openai subscription provider initialized
✓ gemini subscription provider initialized
✓ NanoGPT provider initialized
Initialized 4 providers
🧪 Startup check: 4 healthy providers
🧪 Provider claude-code configured=true healthy=true notes=Anthropic-backed Claude Code subscription path available
🧪 Provider codex configured=true healthy=true notes=OpenAI-backed Codex subscription path available
🧪 Provider gemini configured=true healthy=true notes=Gemini subscription path available
🧪 Provider nanogpt configured=true healthy=true notes=NanoGPT API reachable
```

---

## Step 4: Verify Provider Connections

### 4.1 Check Health Status

```bash
./synroute-cli health
```

Expected output:
```json
{
  "status": "ok",
  "timestamp": 1710123456,
  "circuit_breakers": {
    "claude-code": "closed",
    "codex": "closed",
    "gemini": "closed",
    "nanogpt": "closed"
  }
}
```

### 4.2 List Available Models

```bash
./synroute-cli models
```

Expected output:
```json
{
  "object": "list",
  "data": [
    {
      "id": "claude-3-5-sonnet-latest",
      "object": "model",
      "created": 1710123456,
      "owned_by": "anthropic"
    },
    {
      "id": "gpt-5-codex",
      "object": "model",
      "created": 1710123456,
      "owned_by": "openai"
    },
    {
      "id": "gemini-2.5-pro",
      "object": "model",
      "created": 1710123456,
      "owned_by": "google"
    }
  ]
}
```

### 4.3 Check Provider Status

```bash
curl -s http://localhost:8090/v1/providers | jq
```

This shows detailed status for each provider:
```json
{
  "count": 4,
  "providers": [
    {
      "name": "claude-code",
      "healthy": true,
      "max_context_tokens": 200000,
      "circuit_state": "closed",
      "current_usage": 0,
      "daily_limit": 500000,
      "usage_percent": "0.0%",
      "tier": "pro"
    },
    {
      "name": "codex",
      "healthy": true,
      "max_context_tokens": 128000,
      "circuit_state": "closed",
      "current_usage": 0,
      "daily_limit": 300000,
      "usage_percent": "0.0%",
      "tier": "pro"
    },
    {
      "name": "gemini",
      "healthy": true,
      "max_context_tokens": 1048576,
      "circuit_state": "closed",
      "current_usage": 0,
      "daily_limit": 500000,
      "usage_percent": "0.0%",
      "tier": "pro"
    }
  ]
}
```

---

## Step 5: Test with CLI

### 5.1 Create a Task

```bash
./synroute-cli task create "Research the latest AI safety developments"
```

Output:
```json
{
  "id": "orch-1234567890",
  "goal": "Research the latest AI safety developments",
  "session_id": "cli-1234567890",
  "status": "pending",
  "created_at": "2026-03-08T12:00:00Z"
}
```

### 5.2 List Tasks

```bash
./synroute-cli task list
```

### 5.3 Get Task Details

```bash
./synroute-cli task get orch-1234567890
```

### 5.4 Spawn an Agent

```bash
./synroute-cli agent spawn researcher
```

### 5.5 Create a Swarm

```bash
./synroute-cli swarm create "Distributed analysis" --topology mesh --max-agents 5
```

---

## Step 6: Test with API Calls

### 6.1 Chat Completion (OpenAI-compatible)

```bash
curl -X POST http://localhost:8090/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-3-5-sonnet-latest",
    "messages": [
      {"role": "user", "content": "Hello! Can you explain unified context?"}
    ]
  }'
```

### 6.2 Create Orchestration Task

```bash
curl -X POST http://localhost:8090/v1/orchestration/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "goal": "Analyze recent AI research papers",
    "execute": true
  }'
```

### 6.3 Search Memory

```bash
curl -X GET "http://localhost:8090/v1/memory/search?session_id=cli-1234567890&q=AI+safety"
```

---

## How It Works

### Unified Context Architecture

Synapse Router's **core innovation** is unified context across all providers via vector memory:

1. **You send a request** → System selects best available provider
2. **Provider processes** → Response stored in vector memory with embeddings
3. **Provider switches** → New provider sees full conversation history
4. **Seamless continuity** → Context preserved across Claude, Codex, Gemini

Example flow:
```
Request 1 → Claude (stored in memory)
Request 2 → Codex (sees Request 1 context)
Request 3 → Gemini (sees Requests 1-2 context)
```

### Provider Fallback Chain

Providers are tried in order defined by `SYNROUTE_SUBSCRIPTION_PROVIDER_ORDER`:

```
Claude Code (200K tokens)
    ↓ (if quota exceeded or unhealthy)
Codex (128K tokens)
    ↓ (if quota exceeded or unhealthy)
Gemini (1M tokens)
    ↓ (if quota exceeded or unhealthy)
NanoGPT (2M tokens, fallback)
```

### Circuit Breaker Protection

- Tracks provider failures
- Automatically skips unhealthy providers
- Prevents cascade failures
- Auto-recovery when provider healthy again

---

## Troubleshooting

### Problem: "No providers configured"

**Solution**: Check your .env file has API keys set
```bash
cat .env | grep -E "(ANTHROPIC|OPENAI|GEMINI)_API_KEY"
```

### Problem: Provider shows `healthy: false`

**Causes**:
1. Invalid API key
2. API endpoint unreachable
3. Quota exceeded

**Solution**:
```bash
# Check startup logs
./synroute 2>&1 | grep -E "(Provider|healthy|notes)"

# Test API key manually
curl -H "Authorization: Bearer $SYNROUTE_ANTHROPIC_API_KEY" \
  https://api.anthropic.com/v1/messages
```

### Problem: "Connection refused" on port 8090

**Solution**: Check if another process is using port 8090
```bash
lsof -i :8090
# Kill if needed, then restart
```

### Problem: Tasks not executing

**Solution**: Check orchestration status
```bash
./synroute-cli task list
./synroute-cli agent list
```

---

## Advanced Configuration

### Custom Provider Order

Change fallback priority:
```bash
# Try Gemini first (largest context), then Claude, then Codex
SYNROUTE_SUBSCRIPTION_PROVIDER_ORDER=gemini,anthropic,openai
```

### Quota Limits

Override default daily limits:
```bash
CLAUDE_CODE_DAILY_LIMIT=1000000     # 1M tokens/day
CODEX_DAILY_LIMIT=500000            # 500K tokens/day
GEMINI_DAILY_LIMIT=2000000          # 2M tokens/day
```

### Semantic Search

Enable real vector embeddings for better context retrieval:
```bash
OPENAI_API_KEY=sk-XXXXXX  # Uses OpenAI embeddings API
```

Without this, system uses fast local hash-based embeddings (good for most use cases).

---

## CLI Reference

See [CLI_GUIDE.md](./CLI_GUIDE.md) for complete command reference.

Quick commands:
```bash
# Tasks
./synroute-cli task create "goal"
./synroute-cli task list
./synroute-cli task get <id>
./synroute-cli task pause <id>
./synroute-cli task resume <id>

# Agents
./synroute-cli agent spawn <type>
./synroute-cli agent list
./synroute-cli agent stop <id>

# Swarms
./synroute-cli swarm create "objective" --topology mesh
./synroute-cli swarm status <id>
./synroute-cli swarm start <id>

# Workflows
./synroute-cli workflow list
./synroute-cli workflow run <template> "goal"

# System
./synroute-cli health
./synroute-cli models
./synroute-cli memory search "query" --session <id>
```

---

## Next Steps

1. **Explore Features**: See [FEATURES_COMPLETED.md](./FEATURES_COMPLETED.md)
2. **API Examples**: See [API_EXAMPLES.md](./API_EXAMPLES.md)
3. **Architecture**: See [ARCHITECTURE.md](./ARCHITECTURE.md)
4. **Vector Embeddings**: See [VECTOR_EMBEDDINGS.md](./VECTOR_EMBEDDINGS.md)

---

## Summary

You now have:
- ✅ Synapse Router running on port 8090
- ✅ Claude (Anthropic) provider connected by API key or supplied session token
- ✅ Codex (OpenAI) provider connected by API key or supplied session token
- ✅ Gemini (Google) provider connected by API key or supplied session token
- ✅ Unified context across all providers
- ✅ CLI for easy task/agent/swarm management
- ✅ Automatic failover and circuit breaker protection

Known gap:
- the built-in OAuth/browser login flow parity from archived `CLIProxyAPI` is now implemented inside `synroute`
