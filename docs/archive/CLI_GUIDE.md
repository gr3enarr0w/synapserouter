# Synapse Router CLI Guide

## Installation

Build the CLI:
```bash
make build-cli
# or
cd cmd/synroute-cli && go build -o ../../synroute-cli .
```

The `synroute-cli` binary will be created in the root directory.

## Configuration

The CLI connects to a running Synapse Router instance.

### Environment Variables
```bash
export SYNROUTE_URL=http://localhost:8090
export SYNROUTE_ADMIN_TOKEN=your-admin-token  # Optional
```

### Command-Line Flags
```bash
synroute --url http://localhost:8090 --token your-token <command>
```

## Quick Start

1. Start the Synapse Router server:
```bash
./start.sh
```

2. Use the CLI:
```bash
# Check health
./synroute-cli health

# List available models
./synroute-cli models

# Create a task
./synroute-cli task create "Research latest AI trends"

# List tasks
./synroute-cli task list
```

## Commands

### Task Management

#### Create a task
```bash
synroute-cli task create <goal>
```

Example:
```bash
synroute-cli task create "Analyze sales data and generate report"
```

#### List all tasks
```bash
synroute-cli task list
```

Output:
```
Found 2 tasks:

  ID: task-1
  Goal: Analyze sales data and generate report
  Status: completed
  Session: cli-1709936400

  ID: task-2
  Goal: Research latest AI trends
  Status: running
  Session: cli-1709936500
```

#### Get task details
```bash
synroute-cli task get <task-id>
```

Example:
```bash
synroute-cli task get task-1
```

#### Pause a task
```bash
synroute-cli task pause <task-id>
```

#### Resume a task
```bash
synroute-cli task resume <task-id>
```

#### Cancel a task
```bash
synroute-cli task cancel <task-id>
```

### Agent Management

#### Spawn an agent
```bash
synroute-cli agent spawn <type>
```

Example:
```bash
synroute-cli agent spawn researcher
synroute-cli agent spawn analyst
synroute-cli agent spawn queen-coordinator
```

Available agent types: researcher, analyst, coder, reviewer, tester, deployer, debugger, architect, planner, optimizer, security-auditor, queen-coordinator, and 40+ more.

#### List all agents
```bash
synroute-cli agent list
```

#### Get agent details
```bash
synroute-cli agent get <agent-id>
```

#### Stop an agent
```bash
synroute-cli agent stop <agent-id>
```

### Swarm Management

#### Create a swarm
```bash
synroute-cli swarm create <objective> [--topology <type>] [--max-agents <N>]
```

Examples:
```bash
# Default hierarchical swarm with 5 agents
synroute-cli swarm create "Parallel data processing"

# Mesh topology with 10 agents
synroute-cli swarm create "Distributed analysis" --topology mesh --max-agents 10

# Adaptive topology
synroute-cli swarm create "Complex research project" --topology adaptive
```

Topology options:
- `hierarchical` - Queen-led coordination (default)
- `mesh` - Peer-to-peer
- `ring` - Circular chain
- `star` - Hub-and-spoke
- `adaptive` - Dynamic adjustment
- `hybrid` - Hierarchical + mesh
- `security` - Security-focused
- `delivery` - Release/deployment

#### List all swarms
```bash
synroute-cli swarm list
```

#### Get swarm status
```bash
synroute-cli swarm status <swarm-id>
```

#### Start a swarm
```bash
synroute-cli swarm start <swarm-id>
```

#### Stop a swarm
```bash
synroute-cli swarm stop <swarm-id>
```

### Workflow Management

#### List workflow templates
```bash
synroute-cli workflow list
```

Output:
```
Available workflow templates:

  development - Development Workflow
    End-to-end feature development with specialized roles

  research - Research Workflow
    Distributed research and analysis

  security - Security Audit Workflow
    Comprehensive security review

  debugging - Debugging Workflow
    Systematic bug investigation and fix

  sparc - SPARC Methodology
    Multi-phase development workflow
```

#### Run a workflow
```bash
synroute-cli workflow run <template-id> <goal>
```

Examples:
```bash
synroute-cli workflow run development "Build user authentication feature"
synroute-cli workflow run research "Study impact of AI on healthcare"
synroute-cli workflow run security "Audit payment processing code"
```

### Memory & Search

#### Search vector memory
```bash
synroute-cli memory search <query> [--session <session-id>]
```

Examples:
```bash
# Search all sessions
synroute-cli memory search "machine learning best practices"

# Search specific session
synroute-cli memory search "data analysis results" --session cli-1709936400
```

### System Information

#### Check system health
```bash
synroute-cli health
```

#### List available models
```bash
synroute-cli models
```

Output:
```
Available models:

  claude-3-5-sonnet-20241022
  claude-3-opus-20240229
  gpt-4-turbo
  gpt-4
  gemini-1.5-pro
  ...
```

## Output Formats

### Human-Readable (Default)
```bash
synroute-cli task list
```

### JSON Output
```bash
synroute-cli task list --json
```

JSON output is useful for scripting:
```bash
# Get task count
synroute-cli task list --json | jq '.tasks | length'

# Get running tasks
synroute-cli task list --json | jq '.tasks[] | select(.status=="running")'
```

## Advanced Usage

### Scripting

Create a script to automate workflows:

```bash
#!/bin/bash
# research-workflow.sh

# Create research task
TASK_ID=$(synroute-cli task create "Research quantum computing trends" --json | jq -r '.id')

echo "Created task: $TASK_ID"

# Wait for completion
while true; do
  STATUS=$(synroute-cli task get $TASK_ID --json | jq -r '.status')
  echo "Status: $STATUS"

  if [ "$STATUS" = "completed" ]; then
    synroute-cli task get $TASK_ID --json | jq -r '.final_output'
    break
  fi

  sleep 5
done
```

### Monitoring

Monitor task progress:
```bash
watch -n 2 'synroute-cli task list'
```

### Batch Operations

Process multiple tasks:
```bash
# Create multiple research tasks
for topic in "AI" "Blockchain" "Quantum"; do
  synroute-cli task create "Research $topic trends"
done

# List and filter
synroute-cli task list --json | jq '.tasks[] | select(.status=="running") | .id'
```

## Configuration File

Create `~/.synroute-cli.conf`:
```bash
SYNROUTE_URL=http://localhost:8090
SYNROUTE_ADMIN_TOKEN=your-token-here
```

Then source it:
```bash
source ~/.synroute-cli.conf
synroute-cli health
```

## Troubleshooting

### Connection Refused
```
Error: API error: connection refused
```

**Solution**: Make sure Synapse Router is running:
```bash
./start.sh
```

### Authentication Error
```
Error: API error 401: Unauthorized
```

**Solution**: Provide admin token:
```bash
synroute-cli --token your-admin-token task list
```

### Task Not Found
```
Error: API error 404: task not found
```

**Solution**: Verify task ID:
```bash
synroute-cli task list  # Get valid task IDs
```

## Tips & Tricks

1. **Alias for convenience**:
```bash
alias sr='synroute-cli'
sr task list
```

2. **Use JSON for scripting**:
```bash
TASK_ID=$(sr task create "My task" --json | jq -r '.id')
```

3. **Check completion**:
```bash
sr task get $TASK_ID --json | jq '.status'
```

4. **Monitor swarm health**:
```bash
sr swarm status swarm-1 --json | jq '.agents[] | {id, status}'
```

5. **List available agent types**:
```bash
sr agent spawn --help  # See description
```

## Examples

### Complete Research Workflow
```bash
# 1. Create and run research workflow
TASK_ID=$(sr workflow run research "AI safety best practices" --json | jq -r '.id')

# 2. Monitor progress
sr task get $TASK_ID

# 3. View results
sr task get $TASK_ID --json | jq -r '.final_output'
```

### Parallel Processing with Swarm
```bash
# 1. Create a mesh swarm for parallel work
SWARM_ID=$(sr swarm create "Data analysis" --topology mesh --max-agents 10 --json | jq -r '.id')

# 2. Start the swarm
sr swarm start $SWARM_ID

# 3. Create tasks (they'll be distributed)
sr task create "Analyze dataset A"
sr task create "Analyze dataset B"
sr task create "Analyze dataset C"

# 4. Monitor swarm load
sr swarm status $SWARM_ID
```

### Agent Coordination
```bash
# Spawn specialized agents
sr agent spawn researcher
sr agent spawn analyst
sr agent spawn coder

# List active agents
sr agent list

# Check what they're working on
sr task list --json | jq '.tasks[] | select(.status=="running")'
```

## See Also

- [API_EXAMPLES.md](./API_EXAMPLES.md) - Full API reference
- [ARCHITECTURE.md](./ARCHITECTURE.md) - System architecture
- [README.md](./README.md) - Getting started guide
