---
title: Developer Guide
project: synapserouter
tags: [guide, development, contributing]
created: 2026-03-26
updated: 2026-03-26
---

# Developer Guide

This guide covers the common extension points in synapserouter: adding skills, providers, tools, pipeline phases, and migrations. For the full architecture overview, see [[Requirements]] and the project `CLAUDE.md`.

---

## Project Structure

```
synapserouter/
├── main.go                          # Server setup, CLI dispatch, HTTP handlers
├── commands.go                      # CLI command implementations
├── compat_handlers.go               # OpenAI-compatible /v1/chat/completions
├── diagnostic_handlers.go           # Testing/diagnostics API endpoints
├── eval_commands.go                 # CLI: synroute eval {import,run,results,...}
├── eval_handlers.go                 # API: /v1/eval/* endpoints
├── migrations/                      # Numbered SQL migration files (001-008)
│
├── internal/
│   ├── agent/                       # Agent loop, REPL, pipeline, sub-agents
│   │   ├── agent.go                 # Core agent loop (LLM -> tools -> pipeline -> repeat)
│   │   ├── pipeline.go              # Pipeline phases and pass/fail signal detection
│   │   ├── subagent.go              # Parent-child agent spawning
│   │   ├── handoff.go               # Swarm-style agent-to-agent transfer
│   │   ├── pool.go                  # Concurrency-limited agent pool
│   │   ├── guardrails.go            # Input/output validation chains
│   │   ├── state.go                 # SQLite-backed session persistence
│   │   ├── budget.go                # Turn/token/duration limits
│   │   ├── trace.go                 # Structured event tracing
│   │   ├── metrics.go               # Performance tracking
│   │   ├── streaming.go             # Line-by-line output streaming
│   │   └── tool_store.go            # DB-backed tool output storage
│   │
│   ├── providers/                   # LLM provider implementations
│   │   ├── provider.go              # Provider interface + base types
│   │   ├── ollama.go                # Ollama Cloud provider
│   │   ├── vertex.go                # Vertex AI (Claude + Gemini)
│   │   └── gcp_auth.go              # GCP authentication
│   │
│   ├── tools/                       # Agent tool implementations
│   │   ├── tool.go                  # Tool interface (Name, Description, Category, InputSchema, Execute)
│   │   ├── registry.go              # Tool registry (Register, Get, Execute, OpenAIToolDefinitions)
│   │   ├── permissions.go           # Permission checker (read_only, write, dangerous)
│   │   ├── bash.go                  # Bash command execution
│   │   ├── file_read.go             # File reading
│   │   ├── file_write.go            # File writing
│   │   ├── file_edit.go             # File editing (diffs)
│   │   ├── grep.go                  # Content search
│   │   ├── glob.go                  # File pattern matching
│   │   ├── git.go                   # Git operations (with safety guards)
│   │   └── recall.go                # Recall past tool outputs from DB
│   │
│   ├── orchestration/               # Skill system
│   │   ├── skills.go                # Skill registry, trigger matching
│   │   ├── dispatch.go              # Auto-dispatch: goal -> skill chain -> task steps
│   │   └── skilldata/               # 50+ embedded .md skill files (go:embed)
│   │
│   ├── router/                      # Provider routing
│   │   ├── router.go                # Provider selection, fallback chain, health caching
│   │   └── circuit.go               # Circuit breaker with rate-limit cooldowns
│   │
│   ├── environment/                 # Project detection
│   │   ├── detector.go              # Language detection from config files
│   │   ├── resolver.go              # Runtime version resolution
│   │   ├── best_practices.go        # Per-language checks
│   │   └── setup.go                 # Command wrapping (venv, etc.)
│   │
│   ├── worktree/                    # Git worktree isolation
│   ├── mcpserver/                   # MCP server (tools/list, tools/call)
│   ├── subscriptions/               # OAuth subscription provider management
│   └── app/                         # Shared CLI/API logic
│
└── benchmarks/                      # Eval benchmark data
```

---

## Adding a New Skill

Skills are the simplest extension point. No Go code changes required.

### Steps

1. Create a new `.md` file in `internal/orchestration/skilldata/`:

```markdown
---
name: my-new-skill
description: "One-line description of what this skill does."
triggers:
  - "keyword1"
  - "keyword2"
  - "keyword1+keyword2"    # co-occurrence: both must appear
role: coder                # coder | reviewer | planner | researcher
phase: implement           # analyze | implement | verify | review
language: go               # optional: go | python | javascript | rust | etc.
mcp_tools:                 # optional: MCP tools to auto-invoke
  - "context7.query-docs"
verify:                    # optional: commands the verification gate runs
  - name: "check build"
    command: "go build ./..."
    expect: ""             # empty = expect no output (success)
  - name: "run tests"
    command: "go test ./... 2>&1 | tail -5"
    expect: "ok"           # substring expected in output
---

## Instructions

Full instructions for the LLM when this skill is matched.
Write these as if you're briefing an expert on how to approach the task.

### Patterns to Follow

- Pattern 1
- Pattern 2

### Common Mistakes

- Mistake 1
- Mistake 2
```

2. Rebuild the binary:

```bash
go build -o synroute .
```

The skill files are compiled into the binary via `go:embed` in `internal/orchestration/skilldata/`. The `ParseSkillsFromFS()` function reads YAML frontmatter at startup. No registry code changes needed.

### Trigger Syntax

- Simple keyword: `"golang"` -- matches if "golang" appears in the message
- Co-occurrence: `"go+handler"` -- matches only if BOTH "go" AND "handler" appear
- Keep triggers specific to avoid false positives (see [[Requirements]] Story 3.2)

### Skill Matching Flow

1. User sends a message
2. `computeSkillContext()` scans all skills' triggers against the message
3. Matched skills are ordered by phase: `analyze -> implement -> verify -> review`
4. Skill instructions are injected into the system prompt
5. If skills have `mcp_tools`, those are auto-invoked and results added as context

### Verifying Your Skill

```bash
# Check skill appears in the registry
curl localhost:8090/v1/skills

# Test trigger matching against a query
curl "localhost:8090/v1/skills/match?q=your+test+query"
```

### Reference Files

- Existing skill example: `internal/orchestration/skilldata/go-patterns.md`
- Skill parser: `internal/orchestration/skills.go` (`ParseSkillsFromFS`)
- Dispatch engine: `internal/orchestration/dispatch.go`

---

## Adding a New Provider

Providers implement the `Provider` interface from `internal/providers/provider.go`.

### The Provider Interface

```go
type Provider interface {
    Name() string
    ChatCompletion(ctx context.Context, req ChatRequest, sessionID string) (ChatResponse, error)
    IsHealthy(ctx context.Context) bool
    MaxContextTokens() int
    SupportsModel(model string) bool
}
```

### Steps

1. Create a new file in `internal/providers/`, e.g. `myprovider.go`:

```go
package providers

import (
    "context"
    "time"
)

type MyProvider struct {
    BaseProvider              // embeds Name() and MaxContextTokens()
    client *http.Client
    model  string
}

func NewMyProvider(baseURL, apiKey, model string) *MyProvider {
    return &MyProvider{
        BaseProvider: BaseProvider{
            name:       "my-provider",
            baseURL:    baseURL,
            apiKey:     apiKey,
            maxContext: 128000,       // context window size
            timeout:    120 * time.Second,
        },
        client: NewLLMClient(120 * time.Second),
        model:  model,
    }
}

func (p *MyProvider) ChatCompletion(ctx context.Context, req ChatRequest, sessionID string) (ChatResponse, error) {
    // Transform req to provider's API format
    // Make HTTP call
    // Transform response back to ChatResponse
    // Return
}

func (p *MyProvider) IsHealthy(ctx context.Context) bool {
    // Lightweight health check (cached 2min by router)
    // Return true if provider is reachable
}

func (p *MyProvider) SupportsModel(model string) bool {
    // Return true if this provider can serve the requested model
}
```

2. Register the provider in `main.go` during provider initialization (look for where `NewOllamaCloudProvider` or `NewVertexProvider` are called).

3. Add the provider to the router's escalation chain if it should participate in auto-escalation.

### Key Patterns

- Use `NewLLMClient(timeout)` for HTTP clients -- it bounds connection setup, not total body read time (safe for streaming)
- Use `BaseProvider` embedding for `Name()` and `MaxContextTokens()` -- only implement the 3 remaining methods
- Health checks are cached by the router (2min TTL) -- keep them lightweight
- Circuit breakers are managed by the router, not the provider -- providers just return errors
- Parse `429` rate-limit responses for "reset after Ns" to feed circuit breaker cooldowns

### Reference Files

- Interface definition: `internal/providers/provider.go`
- Ollama implementation: `internal/providers/ollama.go` (simplest example)
- Vertex implementation: `internal/providers/vertex.go` (complex, handles Claude + Gemini)
- Router registration: `internal/router/router.go`
- Circuit breaker: `internal/router/circuit.go`

---

## Adding a New Tool

Tools extend the agent's capabilities. Each tool implements the `Tool` interface from `internal/tools/tool.go`.

### The Tool Interface

```go
type Tool interface {
    Name() string
    Description() string
    Category() ToolCategory           // read_only | write | dangerous
    InputSchema() map[string]interface{}  // JSON Schema for args
    Execute(ctx context.Context, args map[string]interface{}, workDir string) (*ToolResult, error)
}
```

### Tool Categories

| Category | Behavior | Examples |
|---|---|---|
| `read_only` | Always allowed | `file_read`, `grep`, `glob` |
| `write` | Needs approval in interactive mode | `file_write`, `file_edit` |
| `dangerous` | Extra scrutiny, explicit confirmation | `bash` (for destructive commands) |

### Steps

1. Create a new file in `internal/tools/`, e.g. `mytool.go`:

```go
package tools

import "context"

type MyTool struct{}

func (t *MyTool) Name() string        { return "my_tool" }
func (t *MyTool) Description() string { return "One-line description for the LLM." }
func (t *MyTool) Category() ToolCategory { return CategoryReadOnly }

func (t *MyTool) InputSchema() map[string]interface{} {
    return map[string]interface{}{
        "type": "object",
        "properties": map[string]interface{}{
            "query": map[string]interface{}{
                "type":        "string",
                "description": "The search query",
            },
        },
        "required": []string{"query"},
    }
}

func (t *MyTool) Execute(ctx context.Context, args map[string]interface{}, workDir string) (*ToolResult, error) {
    query, _ := args["query"].(string)
    // Do the work...
    return &ToolResult{Output: "result"}, nil
}
```

2. Register the tool in `DefaultRegistry()` in `internal/tools/registry.go`:

```go
func DefaultRegistry() *Registry {
    r := NewRegistry()
    r.Register(&BashTool{})
    r.Register(&FileReadTool{})
    // ... existing tools ...
    r.Register(&MyTool{})    // add here
    return r
}
```

3. Rebuild: `go build -o synroute .`

### Key Patterns

- The `InputSchema()` return value becomes the `parameters` field in OpenAI function-calling format
- `workDir` is the agent's working directory -- use it for file operations
- Return errors via `ToolResult.Error`, not Go errors (Go errors = tool infrastructure failure)
- The `ToolResult.Output` is what the LLM sees -- keep it concise, store full output in DB via `tool_store.go`
- Tools are automatically included in OpenAI tool definitions via `Registry.OpenAIToolDefinitions()`

### Verifying Your Tool

```bash
# List registered tools
curl localhost:8090/v1/tools

# Test via MCP server (if enabled)
curl -X POST localhost:8090/mcp/tools/call \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"my_tool","arguments":{"query":"test"}}}'
```

### Reference Files

- Interface: `internal/tools/tool.go`
- Registry: `internal/tools/registry.go`
- Simple example: `internal/tools/glob.go`
- Complex example: `internal/tools/bash.go`
- Tests: `internal/tools/tools_test.go`, `internal/tools/integration_test.go`

---

## Adding a Pipeline Phase

Pipelines define the ordered phases of the agent's project lifecycle. There are two built-in pipelines in `internal/agent/pipeline.go`:

- `DefaultPipeline` (software): plan -> implement -> self-check -> code-review -> acceptance-test -> deploy
- `DataSciencePipeline`: eda -> data-prep -> model -> review -> deploy -> verify

### PipelinePhase Struct

```go
type PipelinePhase struct {
    Name         string   // phase identifier
    Prompt       string   // injected prompt (use %s for acceptance criteria)
    Escalate     bool     // escalate to next provider tier
    StoreAs      string   // store LLM response ("criteria", "subtasks")
    FailAction   string   // "retry" or "back:N"
    MinToolCalls int      // minimum tool calls before phase can advance
    UseSubAgent  bool     // spawn fresh agent (no shared conversation)
    ParallelSubAgents int // number of parallel sub-agents
    CoderProviders []string // provider names for parallel agents
    MergeProvider  string  // provider to merge parallel outputs
}
```

### Steps

1. Edit `internal/agent/pipeline.go`
2. Add a new `PipelinePhase` entry in the appropriate pipeline's `Phases` slice
3. Add pass/fail signal strings to `phasePassSignals` and `phaseFailSignals`
4. Rebuild: `go build -o synroute .`

### Example: Adding a "Security Scan" Phase

```go
{
    Name:         "security-scan",
    MinToolCalls: 1,
    UseSubAgent:  true,     // independent reviewer, no shared context
    Prompt: `PHASE: SECURITY SCAN
Check for security issues:
1. Scan for hardcoded secrets, credentials, API keys
2. Check input validation and sanitization
3. Review authentication and authorization
4. Check against criteria:
---
%s
---
Say SECURITY_PASS if no critical issues, SECURITY_FAIL with details.`,
},
```

Then add signals:
```go
var phasePassSignals = []string{
    // ... existing ...
    "security_pass",
}

var phaseFailSignals = []string{
    // ... existing ...
    "security_fail",
}
```

### Key Patterns

- Phases with `UseSubAgent: true` spawn a fresh agent with NO shared conversation context -- good for independent reviews
- `MinToolCalls` prevents rubber-stamping -- the LLM must actually inspect code, not just say "looks good"
- `%s` in the prompt is replaced with acceptance criteria from the plan phase
- `StoreAs: "criteria"` on the plan phase stores the LLM's output for later injection
- Pipeline detection is dynamic from skill metadata, not hardcoded

### Reference Files

- Pipeline definitions: `internal/agent/pipeline.go`
- Pipeline advancement: `internal/agent/agent.go` (`advancePipeline`)

---

## Adding a Migration

Database schema changes use numbered SQL migration files in `migrations/`.

### Current Migrations

| File | Purpose |
|---|---|
| `001_init.sql` | Core tables |
| `002_responses.sql` | Response tracking |
| `003_sessions.sql` | Session management |
| `004_eval.sql` | Eval framework tables |
| `005_eval_metrics.sql` | Eval metrics |
| `006_agent_sessions.sql` | Agent state persistence |
| `007_tool_outputs.sql` | Tool output storage for recall |
| `008_memory_tool_calls.sql` | Memory-aware tool call tracking |

### Steps

1. Create the next numbered file: `migrations/009_your_feature.sql`

2. Write idempotent SQL (always use `IF NOT EXISTS`):

```sql
-- Description of what this migration adds.
CREATE TABLE IF NOT EXISTS my_table (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT NOT NULL,
    data TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_my_table_session ON my_table(session_id);
```

3. Rebuild: `go build -o synroute .`

### Key Patterns

- Migrations are embedded via `go:embed` and run sequentially on startup
- Always use `CREATE TABLE IF NOT EXISTS` and `CREATE INDEX IF NOT EXISTS` -- migrations may run multiple times
- Use `INSERT OR IGNORE` for seed data to avoid duplicate key errors
- SQLite is the only supported database (Postgres planned in [[Requirements]] Story 10.1)
- Foreign keys are not enforced in SQLite by default -- don't rely on them
- Keep migrations small and focused on one feature

### Reference Files

- Example migration: `migrations/007_tool_outputs.sql`
- Migration runner: embedded in `main.go` startup

---

## Testing Patterns

### Running Tests

```bash
# All tests with race detection (standard)
go test -race ./...

# Specific package
go test -race ./internal/tools/...

# Verbose output
go test -race -v ./internal/agent/...

# Run a specific test
go test -race -run TestBashTool ./internal/tools/...
```

### Table-Driven Tests

The project uses Go's standard table-driven test pattern:

```go
func TestMyFunction(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected string
        wantErr  bool
    }{
        {
            name:     "valid input",
            input:    "hello",
            expected: "HELLO",
        },
        {
            name:    "empty input",
            input:   "",
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := MyFunction(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("MyFunction() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if got != tt.expected {
                t.Errorf("MyFunction() = %q, want %q", got, tt.expected)
            }
        })
    }
}
```

### Post-Change Pipeline

After ANY code change, run this pipeline in order:

```bash
go vet ./...                    # 1. Catch issues early
go test -race ./...             # 2. Unit tests with race detection
./synroute test                 # 3. E2E provider smoke test
go build -o synroute .          # 4. Verify clean build
```

### Test File Conventions

- Test files live alongside the code they test: `bash.go` -> `bash_test.go`
- Integration tests use the `_test.go` suffix: `integration_test.go`
- Use `t.Helper()` in test helper functions
- Use `t.Parallel()` where tests are independent

### Reference Files

- Tool tests: `internal/tools/tools_test.go`, `internal/tools/bash_test.go`
- Integration tests: `internal/tools/integration_test.go`
- Permission tests: `internal/tools/permissions_test.go`
- Provider tests: `internal/providers/gcp_auth_test.go`

---

## Build and Run

```bash
# Build
go build -o synroute .

# Run server
./synroute                     # or: ./synroute serve

# Interactive agent
./synroute chat

# Smoke test providers
./synroute test

# Diagnostics
./synroute doctor

# List models
./synroute models
```

See `CLAUDE.md` for the full CLI reference and API endpoint documentation.
