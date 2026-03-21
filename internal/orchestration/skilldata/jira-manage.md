---
name: jira-manage
description: "Create, update, search, and manage Jira issues — handles DC custom fields automatically."
triggers:
  - "jira"
  - "ticket"
  - "epic"
  - "story"
  - "sprint"
  - "backlog"
role: coder
phase: implement
---
# Jira Issue Management

Efficiently manage Jira issues using the Atlassian MCP tools. This skill encodes the field mappings, workflows, and best practices for Red Hat's Jira DC instance at issues.redhat.com.

## DC Custom Field Reference

| Field | ID | Usage |
|-------|----|-------|
| Epic Link | `customfield_12311140` | Link a story/task to an epic: `{"customfield_12311140": "PROJ-123"}` |
| Epic Name | `customfield_12311141` | Short label for the epic (required on create): `{"customfield_12311141": "My Epic"}` |
| Epic Colour | `customfield_12311143` | Epic color in board view |
| Epic Status | `customfield_12311142` | Epic status field |

## MCP Tools to Use

| Action | Tool | Key Parameters |
|--------|------|----------------|
| Search issues | `mcp__atlassian__jira_search` | `jql`, `fields`, `limit` |
| Create issue | `mcp__atlassian__jira_create_issue` | `project_key`, `summary`, `issue_type`, `assignee`, `description`, `additional_fields` |
| Update issue | `mcp__atlassian__jira_update_issue` | `issue_key`, `fields`, `components`, `additional_fields` |
| Get single issue | `mcp__atlassian__jira_get_issue` | `issue_key` |
| Add comment | `mcp__atlassian__jira_add_comment` | `issue_key`, `body` |
| Transition status | `mcp__atlassian__jira_transition_issue` | `issue_key`, `transition` |

## Workflows

### Before Creating Issues — Check for Duplicates

Always search first to avoid creating duplicate tickets:

```
jql: project = PROJ AND summary ~ "keyword" ORDER BY created DESC
```

If a match exists, update it instead of creating a new one.

### Creating an Epic

```
project_key: "PROJ"
summary: "Epic title"
issue_type: "Epic"
assignee: "rhn-support-ceverson"
additional_fields: {"customfield_12311141": "Short Epic Name", "reporter": {"name": "rhn-support-ceverson"}}
```

### Creating a Story Linked to an Epic

```
project_key: "PROJ"
summary: "Story title"
issue_type: "Story"
assignee: "rhn-support-ceverson"
additional_fields: {"customfield_12311140": "PROJ-123", "reporter": {"name": "rhn-support-ceverson"}}
```

### Creating a Subtask

```
project_key: "PROJ"
summary: "Subtask title"
issue_type: "Subtask"
additional_fields: {"parent": "PROJ-456"}
```

### Bulk Updates — Components and Versions

When updating multiple issues with the same component or version, batch the calls in parallel:

```
fields: {"versions": [{"name": "component-name-0.1"}]}
components: "component-name"
```

### Setting Affects Version vs Fix Version

- **Affects Version** (`versions`): Which release the issue was found in or belongs to
- **Fix Version** (`fixVersions`): Which release will contain the fix

```json
{"versions": [{"name": "faq-service-0.1"}]}
{"fixVersions": [{"name": "faq-service-0.2"}]}
```

## Reporter and Assignee

Default user for this instance: `rhn-support-ceverson` (Clark Everson, ceverson@redhat.com)

To set reporter on create, include in additional_fields:
```json
{"reporter": {"name": "rhn-support-ceverson"}}
```

## Status Transitions

Check available transitions before transitioning:
```
mcp__atlassian__jira_get_transitions(issue_key: "PROJ-123")
```

Common statuses: New → In Progress → Waiting for support → Resolved → Closed

## Auto-Create Project-Level Skills

When working in a project that uses Jira tracking but has no `project-tracker` skill:

1. Check for `.claude/skills/project-tracker/SKILL.md` in the project root
2. If missing, create one with:
   - Project key, component names, version names
   - Epic keys and story map (key → summary → status)
   - Affects version assignments
   - Workflows for syncing code changes to Jira
3. Populate the story map by searching Jira: `project = PROJ AND component = comp-name ORDER BY key ASC`
4. This ensures future sessions have the mapping without re-querying Jira

Template structure:
```
---
name: project-tracker
description: Track [project] implementation work in [PROJ] Jira project. Knows epic keys, components, versions, and story status.
---
# [Project] — Project Tracker
## Epics
## Versions
## Story Map
## Workflows
```

## Efficiency Tips

- Use `fields` parameter on search to limit returned data — avoid fetching all fields
- When creating many issues under one epic, fire creates in parallel (6 at a time max)
- When updating many issues, batch updates in parallel
- Use `limit` on searches — default is 10, max is 50
- For components/versions that don't exist yet, create them first via REST API (no MCP tool for component creation)
- When a project-tracker skill exists, read it first instead of querying Jira for epic/story mappings
