---
name: spec-workflow
description: "5-phase spec creation, validation against project rules, implementation orchestration."
triggers:
  - "spec"
  - "specification"
  - "rfc"
  - "design doc"
  - "architecture decision"
role: architect
phase: analyze
mcp_tools:
  - "spec-workflow.spec-workflow-guide"
---

> **Spec Override:** These patterns are defaults. If a project spec defines different
> architecture, package structure, or scope, follow the spec instead.
# Skill: Spec Workflow

5-phase spec creation, validation against project rules, implementation orchestration.

Source: [Spec Creation Workflow](https://mcpmarket.com/tools/skills/spec-creation-workflow), [Implement-Spec Orchestrator](https://mcpmarket.com/ko/tools/skills/implement-spec-orchestrator-1), [Spec Reviewer](https://mcpmarket.com/tools/skills/spec-reviewer).

---

## When to Use

- Creating technical specifications for new features
- Validating specs before implementation begins
- Orchestrating implementation from spec to PR
- Reviewing proposed architecture changes

---

## 5-Phase Spec Creation

### Phase 1: Requirements Gathering
- Capture user stories and acceptance criteria
- Identify constraints (tech, timeline, dependencies)
- Define success metrics

### Phase 2: Architecture Design
- Component diagram (what interacts with what)
- Data flow (inputs → processing → outputs)
- API surface (endpoints, schemas)
- Database changes (migrations, new tables)

### Phase 3: Implementation Plan
- Break into ordered tasks
- Estimate scope per task
- Identify risks and mitigations
- Define testing strategy

### Phase 4: Validation
Cross-reference against:
- `CLAUDE.md` project rules
- Existing architecture patterns
- Known anti-patterns
- Edge cases and error scenarios

### Phase 5: Sign-Off
- Present spec to stakeholders
- Address feedback
- Lock spec version before implementation

---

## Spec Template

```markdown
# Feature: [Name]

## Problem Statement
What problem does this solve? Who is affected?

## Proposed Solution
High-level approach.

## Detailed Design

### API Changes
- `POST /api/v1/resource` — Create resource
  - Request: `{ "name": "...", "type": "..." }`
  - Response: `201 { "id": "...", "created_at": "..." }`

### Database Changes
- New table: `resources (id, name, type, created_at)`
- Migration: `001_create_resources.sql`

### Component Changes
- `services/resource_service.py` — New service
- `routers/resources.py` — New router

## Testing Plan
- Unit tests for service logic
- Integration tests for API endpoints
- Edge cases: [list]

## Rollout Plan
- Feature flag: `ENABLE_RESOURCES`
- Migration steps: [list]

## Open Questions
- [List any unresolved decisions]
```

---

## Spec Review Checklist

- [ ] Problem clearly stated
- [ ] Solution addresses the problem
- [ ] No over-engineering (YAGNI)
- [ ] Consistent with existing patterns
- [ ] Edge cases considered
- [ ] Testing strategy defined
- [ ] Rollback plan exists

---

## Spec Workflow MCP

The `spec-workflow` MCP (`@pimzino/spec-workflow-mcp`) is configured globally and provides:
- Implementation logs with searchable history
- Task tracking with code statistics
- Structured spec-to-implementation pipeline

Use the MCP tools for persistent tracking across sessions. Use this skill's templates for spec authoring.

---

## Tools

- `Bash` — file creation, git operations
- `spec-workflow` MCP — implementation tracking and task management
