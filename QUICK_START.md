# Quick Start - Synapse Router

```bash
cd src/services/synapse-router
export SYNROUTE_ANTHROPIC_API_KEY=...
export SYNROUTE_OPENAI_API_KEY=...
export SYNROUTE_GEMINI_API_KEY=...
export SYNROUTE_QWEN_API_KEY=...
export NANOGPT_API_KEY=...
go build -o synroute .
./synroute
```

Health check:

```bash
curl http://localhost:8090/health
curl http://localhost:8090/v1/models
```

Chat request:

```bash
curl -X POST http://localhost:8090/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "auto",
    "messages": [
      {"role": "user", "content": "Explain how request routing works here"}
    ]
  }'
```

Responses request:

```bash
curl -X POST http://localhost:8090/v1/responses \
  -H "Content-Type: application/json" \
  -d '{
    "model": "auto",
    "input": "Summarize the current orchestration runtime."
  }'
```

Example orchestration task:

```bash
curl -X POST http://localhost:8090/v1/orchestration/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "goal": "Review the routing and orchestration stack",
    "roles": ["architect", "reviewer"],
    "execute": false
  }'
```

This service is local and in-process. There is no separate `CLIProxyAPI` or `subscription-gateway` runtime to start.
