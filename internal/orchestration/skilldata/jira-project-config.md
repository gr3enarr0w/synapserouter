---
name: jira-project-config
description: "Jira project configuration — components, versions, boards, project settings."
triggers:
  - "jira component"
  - "jira version"
  - "jira board"
  - "project config"
role: coder
phase: implement
---
# Jira Project Configuration

Manage project-level configuration for Jira DC at issues.redhat.com.

## Components

Components group issues by functional area. The Atlassian MCP does NOT have a create-component tool — use the REST API directly.

### Naming Convention

Use lowercase `word` or `word-word` format: `faq-service`, `jsm-modeling`, `user-sync`

### Create Component (REST API)

```bash
export JIRA_PERSONAL_TOKEN="<token-from-env>"
curl -s -X POST "https://issues.redhat.com/rest/api/2/component" \
  -H "Authorization: Bearer $JIRA_PERSONAL_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "component-name",
    "project": "PROJ",
    "description": "Component description",
    "lead": {"name": "your-username"}
  }'
```

Token is stored in `~/.env.atlassian` as `JIRA_PERSONAL_TOKEN`.

### List Components (MCP)

```
mcp__atlassian__jira_get_project_components(project_key: "PROJ")
```

### Assign Component to Issues (MCP)

```
mcp__atlassian__jira_update_issue(issue_key: "PROJ-123", fields: "{}", components: "component-name")
```

## Versions

Versions track releases. The naming convention ties versions to components.

### Naming Convention

`component-name-X.Y` — e.g., `faq-service-0.1`, `jsm-modeling-0.2`

### Create Versions (MCP — batch)

```
mcp__atlassian__jira_batch_create_versions(
  project_key: "PROJ",
  versions: '[{"name": "comp-0.1", "description": "Initial release"}, {"name": "comp-0.2"}]'
)
```

If batch fails (DC compatibility issues), use REST API:

```bash
curl -s -X POST "https://issues.redhat.com/rest/api/2/version" \
  -H "Authorization: Bearer $JIRA_PERSONAL_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name": "comp-0.1", "project": "PROJ", "description": "Initial release"}'
```

### List Versions (MCP)

```
mcp__atlassian__jira_get_project_versions(project_key: "PROJ")
```

### Set Version on Issues

**Affects Version** (which release the work belongs to):
```
fields: {"versions": [{"name": "comp-0.1"}]}
```

**Fix Version** (which release will ship the fix):
```
fields: {"fixVersions": [{"name": "comp-0.2"}]}
```

## Standard Setup for a New Component

1. Check existing components: `mcp__atlassian__jira_get_project_components`
2. Create component via REST API (curl)
3. Create 3 versions: `comp-0.1`, `comp-0.2`, `comp-0.3`
4. Create epic with component and affects version 0.1
5. Create stories linked to epic, assign component and appropriate version
6. Update issues in parallel (6 at a time)

## Auth Reference

- Token file: `~/.env.atlassian`
- Key: `JIRA_PERSONAL_TOKEN`
- DC URL: `https://issues.redhat.com`
- Default user: `your-username`
