---
name: task-orchestrator
description: "Parallel task decomposition, worker agent spawning, structured task graphs."
triggers:
  - "parallel"
  - "decompose"
  - "orchestrate"
  - "fan-out"
  - "subtask"
role: architect
phase: implement
---

> **Spec Override:** These patterns are defaults. If a project spec defines different
> architecture, package structure, or scope, follow the spec instead.
# Skill: Task Orchestrator

Parallel task decomposition, worker agent spawning, structured task graphs.

Source: [Claude Code Orchestrator](https://mcpmarket.com/tools/skills/claude-code-orchestrator), [ORC Multi-Agent](https://mcpmarket.com/ko/tools/skills/orc-multi-agent-orchestrator).

---

## When to Use

- Complex tasks requiring multiple parallel workstreams
- Large refactors touching many files
- Tasks with clear subtask boundaries
- When sequential work is too slow

---

## Process

### 1. Analyze & Decompose
Break the request into independent subtasks:
- Identify dependencies between subtasks
- Group independent tasks for parallel execution
- Estimate complexity of each subtask

### 2. Plan Task Graph
```
Task A (research) ─┐
Task B (research) ─┼─→ Task D (implement) ─→ Task F (verify)
Task C (research) ─┘                    │
                                         └─→ Task E (test)
```

### 3. Execute
- **Independent tasks** → launch as parallel agents
- **Dependent tasks** → wait for prerequisites, then launch
- **Review points** → pause between phases for user checkpoint

### 4. Integrate & Review
- Collect results from all agents
- Resolve any conflicts between parallel changes
- Run integration tests
- Present summary to user

---

## Agent Spawning Patterns

### Research phase (parallel)
Launch 2-3 explore agents simultaneously for different aspects:
```
Agent 1: "Explore the authentication system..."
Agent 2: "Find all database schema definitions..."
Agent 3: "Check test coverage patterns..."
```

### Implementation phase (parallel where possible)
```
Agent 1: "Modify auth module to add JWT support..."
Agent 2: "Update API endpoints to require auth..."
```

### Verification phase
```
Agent: "Run all tests, check for regressions..."
```

---

## Rules

1. **Never duplicate work** — if an agent is researching X, don't also research X
2. **Clear boundaries** — each agent gets a specific, well-defined scope
3. **Dependency ordering** — don't start dependent tasks before prerequisites complete
4. **Checkpoint with user** — after research phase, before implementation
5. **Conflict resolution** — when parallel changes conflict, resolve before committing
