# Synapse Router API Examples

This document provides practical examples for using the Synapse Router API.

## Table of Contents

- [Basic Chat Completion](#basic-chat-completion)
- [Provider-Specific Routing](#provider-specific-routing)
- [Orchestration Tasks](#orchestration-tasks)
- [Swarm Management](#swarm-management)
- [Workflow Execution](#workflow-execution)
- [Memory and Context](#memory-and-context)

## Prerequisites

Make sure the router is running:

```bash
./start.sh
# or
make start
```

Default endpoint: `http://localhost:8090`

## Basic Chat Completion

### Simple chat request

```bash
curl -X POST http://localhost:8090/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-3-5-sonnet-20241022",
    "messages": [
      {
        "role": "user",
        "content": "Explain quantum computing in simple terms"
      }
    ],
    "max_tokens": 1000
  }'
```

### Chat with streaming

```bash
curl -X POST http://localhost:8090/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-3-5-sonnet-20241022",
    "messages": [
      {"role": "user", "content": "Write a haiku about coding"}
    ],
    "stream": true
  }'
```

### List available models

```bash
curl http://localhost:8090/v1/models
```

### Check provider status

```bash
curl http://localhost:8090/v1/providers
```

## Provider-Specific Routing

### Force use of specific provider

```bash
# Use Anthropic/Claude specifically
curl -X POST http://localhost:8090/api/provider/claude-code/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-3-5-sonnet-20241022",
    "messages": [
      {"role": "user", "content": "Hello from Claude!"}
    ]
  }'

# Use OpenAI/Codex specifically
curl -X POST http://localhost:8090/api/provider/codex/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4",
    "messages": [
      {"role": "user", "content": "Hello from OpenAI!"}
    ]
  }'
```

## Orchestration Tasks

### Create and execute a task

```bash
curl -X POST http://localhost:8090/v1/orchestration/tasks \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-admin-token" \
  -d '{
    "goal": "Research and summarize the latest developments in LLMs",
    "session_id": "research-session-001",
    "execute": true
  }'
```

Response:
```json
{
  "id": "task-1",
  "goal": "Research and summarize the latest developments in LLMs",
  "status": "running",
  "session_id": "research-session-001",
  "created_at": "2026-03-08T22:00:00Z"
}
```

### Get task status

```bash
curl http://localhost:8090/v1/orchestration/tasks/task-1 \
  -H "Authorization: Bearer your-admin-token"
```

### Subscribe to task events (SSE)

```bash
curl -N http://localhost:8090/v1/orchestration/tasks/task-1/events \
  -H "Authorization: Bearer your-admin-token"
```

### List all tasks

```bash
curl http://localhost:8090/v1/orchestration/tasks \
  -H "Authorization: Bearer your-admin-token"
```

### Pause a task

```bash
curl -X POST http://localhost:8090/v1/orchestration/tasks/task-1/pause \
  -H "Authorization: Bearer your-admin-token"
```

### Resume a task

```bash
curl -X POST http://localhost:8090/v1/orchestration/tasks/task-1/resume \
  -H "Authorization: Bearer your-admin-token"
```

### Cancel a task

```bash
curl -X POST http://localhost:8090/v1/orchestration/tasks/task-1/cancel \
  -H "Authorization: Bearer your-admin-token"
```

### Refine a task with feedback

```bash
curl -X POST http://localhost:8090/v1/orchestration/tasks/task-1/refine \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-admin-token" \
  -d '{
    "feedback": "Please focus more on practical applications"
  }'
```

## Swarm Management

### Create a swarm

```bash
curl -X POST http://localhost:8090/v1/orchestration/swarms \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-admin-token" \
  -d '{
    "objective": "Parallel data processing and analysis",
    "topology": "hierarchical",
    "strategy": "divide-conquer",
    "max_agents": 5
  }'
```

### Get swarm status

```bash
curl http://localhost:8090/v1/orchestration/swarms/swarm-1/status \
  -H "Authorization: Bearer your-admin-token"
```

### Start a swarm

```bash
curl -X POST http://localhost:8090/v1/orchestration/swarms/swarm-1/start \
  -H "Authorization: Bearer your-admin-token"
```

### Scale a swarm

```bash
curl -X POST http://localhost:8090/v1/orchestration/swarms/swarm-1/scale \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-admin-token" \
  -d '{
    "target_agents": 10
  }'
```

### Check swarm load balance

```bash
curl http://localhost:8090/v1/orchestration/swarms/swarm-1/load \
  -H "Authorization: Bearer your-admin-token"
```

### Detect load imbalance

```bash
curl http://localhost:8090/v1/orchestration/swarms/swarm-1/imbalance \
  -H "Authorization: Bearer your-admin-token"
```

### Preview rebalance operations

```bash
curl -X POST http://localhost:8090/v1/orchestration/swarms/swarm-1/rebalance/preview \
  -H "Authorization: Bearer your-admin-token"
```

### Get stealable tasks

```bash
curl http://localhost:8090/v1/orchestration/swarms/swarm-1/stealable \
  -H "Authorization: Bearer your-admin-token"
```

## Workflow Execution

### List available workflow templates

```bash
curl http://localhost:8090/v1/orchestration/workflows \
  -H "Authorization: Bearer your-admin-token"
```

### Create a custom workflow template

```bash
curl -X POST http://localhost:8090/v1/orchestration/workflows \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-admin-token" \
  -d '{
    "template_id": "custom-analysis",
    "name": "Custom Data Analysis Workflow",
    "roles": ["researcher", "analyst", "reviewer"],
    "description": "Multi-stage data analysis workflow"
  }'
```

### Run a workflow

```bash
curl -X POST http://localhost:8090/v1/orchestration/workflows/sparc/run \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-admin-token" \
  -d '{
    "goal": "Analyze customer feedback sentiment",
    "session_id": "analysis-001"
  }'
```

### Get workflow execution state

```bash
curl http://localhost:8090/v1/orchestration/executions/workflow-1/state \
  -H "Authorization: Bearer your-admin-token"
```

### Get workflow metrics

```bash
curl http://localhost:8090/v1/orchestration/executions/workflow-1/metrics \
  -H "Authorization: Bearer your-admin-token"
```

### Debug workflow execution

```bash
curl http://localhost:8090/v1/orchestration/executions/workflow-1/debug \
  -H "Authorization: Bearer your-admin-token"
```

## Memory and Context

### Search vector memory

```bash
curl "http://localhost:8090/v1/memory/search?query=quantum+computing&session_id=research-001" \
  -H "Authorization: Bearer your-admin-token"
```

### Get session memory

```bash
curl http://localhost:8090/v1/memory/session/research-001 \
  -H "Authorization: Bearer your-admin-token"
```

## Agent Management

### List all agents

```bash
curl http://localhost:8090/v1/orchestration/agents \
  -H "Authorization: Bearer your-admin-token"
```

### Create an agent

```bash
curl -X POST http://localhost:8090/v1/orchestration/agents \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-admin-token" \
  -d '{
    "type": "researcher",
    "name": "Research Agent 1"
  }'
```

### Get agent status

```bash
curl http://localhost:8090/v1/orchestration/agents/agent-1/status \
  -H "Authorization: Bearer your-admin-token"
```

### Get agent metrics

```bash
curl http://localhost:8090/v1/orchestration/agents/agent-1/metrics \
  -H "Authorization: Bearer your-admin-token"
```

### Get agent logs

```bash
curl http://localhost:8090/v1/orchestration/agents/agent-1/logs \
  -H "Authorization: Bearer your-admin-token"
```

### Stop an agent

```bash
curl -X POST http://localhost:8090/v1/orchestration/agents/agent-1/stop \
  -H "Authorization: Bearer your-admin-token"
```

## Session Management

### List tasks in a session

```bash
curl http://localhost:8090/v1/orchestration/sessions/research-001/tasks \
  -H "Authorization: Bearer your-admin-token"
```

### Resume a session

```bash
curl -X POST http://localhost:8090/v1/orchestration/sessions/research-001/resume \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-admin-token" \
  -d '{
    "continue_from_last": true
  }'
```

### Fork a session

```bash
curl -X POST http://localhost:8090/v1/orchestration/sessions/research-001/fork \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-admin-token" \
  -d '{
    "new_session_id": "research-001-branch-1"
  }'
```

## Advanced Features

### Work Stealing

```bash
# Steal a task from another agent
curl -X POST http://localhost:8090/v1/orchestration/tasks/task-1/steal \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-admin-token" \
  -d '{
    "agent_id": "agent-2"
  }'
```

### Contest a steal

```bash
curl -X POST http://localhost:8090/v1/orchestration/tasks/task-1/contest \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-admin-token" \
  -d '{
    "agent_id": "agent-1",
    "reason": "I was already 80% complete"
  }'
```

### Resolve a contest

```bash
curl -X POST http://localhost:8090/v1/orchestration/tasks/task-1/contest/resolve \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-admin-token" \
  -d '{
    "winner": "agent-1"
  }'
```

## Health and Monitoring

### Health check

```bash
curl http://localhost:8090/health
```

### Startup check

```bash
curl http://localhost:8090/v1/startup-check
```

### Usage statistics

```bash
curl http://localhost:8090/v1/usage \
  -H "Authorization: Bearer your-admin-token"
```

### Audit request

```bash
curl http://localhost:8090/v1/audit/request/req-123 \
  -H "Authorization: Bearer your-admin-token"
```

### Audit session

```bash
curl http://localhost:8090/v1/audit/session/session-123 \
  -H "Authorization: Bearer your-admin-token"
```

## Client Libraries

### Python Example

```python
import requests

class SynapseRouter:
    def __init__(self, base_url="http://localhost:8090", admin_token=None):
        self.base_url = base_url
        self.admin_token = admin_token

    def chat(self, messages, model="claude-3-5-sonnet-20241022"):
        response = requests.post(
            f"{self.base_url}/v1/chat/completions",
            json={
                "model": model,
                "messages": messages
            }
        )
        return response.json()

    def create_task(self, goal, session_id, execute=True):
        response = requests.post(
            f"{self.base_url}/v1/orchestration/tasks",
            headers={"Authorization": f"Bearer {self.admin_token}"},
            json={
                "goal": goal,
                "session_id": session_id,
                "execute": execute
            }
        )
        return response.json()

    def get_task(self, task_id):
        response = requests.get(
            f"{self.base_url}/v1/orchestration/tasks/{task_id}",
            headers={"Authorization": f"Bearer {self.admin_token}"}
        )
        return response.json()

# Usage
client = SynapseRouter(admin_token="your-admin-token")

# Simple chat
result = client.chat([
    {"role": "user", "content": "Hello!"}
])
print(result)

# Create orchestration task
task = client.create_task(
    goal="Research machine learning trends",
    session_id="ml-research-001"
)
print(f"Task created: {task['id']}")
```

### JavaScript Example

```javascript
class SynapseRouter {
  constructor(baseUrl = 'http://localhost:8090', adminToken = null) {
    this.baseUrl = baseUrl;
    this.adminToken = adminToken;
  }

  async chat(messages, model = 'claude-3-5-sonnet-20241022') {
    const response = await fetch(`${this.baseUrl}/v1/chat/completions`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({ model, messages })
    });
    return response.json();
  }

  async createTask(goal, sessionId, execute = true) {
    const response = await fetch(`${this.baseUrl}/v1/orchestration/tasks`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${this.adminToken}`
      },
      body: JSON.stringify({ goal, session_id: sessionId, execute })
    });
    return response.json();
  }

  async getTask(taskId) {
    const response = await fetch(
      `${this.baseUrl}/v1/orchestration/tasks/${taskId}`,
      {
        headers: {
          'Authorization': `Bearer ${this.adminToken}`
        }
      }
    );
    return response.json();
  }
}

// Usage
const client = new SynapseRouter('http://localhost:8090', 'your-admin-token');

// Simple chat
const result = await client.chat([
  { role: 'user', content: 'Hello!' }
]);
console.log(result);

// Create orchestration task
const task = await client.createTask(
  'Research machine learning trends',
  'ml-research-001'
);
console.log(`Task created: ${task.id}`);
```

## Notes

- **Authentication**: Most orchestration endpoints require the `Authorization: Bearer <token>` header. Set `ADMIN_TOKEN` in your `.env` file.
- **Rate Limiting**: The router automatically handles provider quota limits and switches to fallback providers.
- **Streaming**: When using `stream: true`, responses are returned as Server-Sent Events (SSE).
- **Session Management**: Sessions provide context isolation and enable multi-turn conversations with memory.
