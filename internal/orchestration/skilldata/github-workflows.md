---
name: github-workflows
description: "GitHub CLI, PR management, issues, Actions, code review, publishing."
triggers:
  - "github"
  - "pull request"
  - "issue"
  - "actions"
  - "workflow"
  - "gh cli"
role: coder
phase: implement
mcp_tools:
  - "context7.query-docs"
---
# Skill: GitHub Workflows

GitHub CLI, PR management, issues, Actions, code review, publishing.

Source: [GitHub MCP](https://github.com/github/github-mcp-server) (28K stars), [GitHub CLI Automation](https://mcpmarket.com/tools/skills/github-cli-automation).

---

## When to Use

- Managing pull requests and issues via `gh` CLI
- Setting up GitHub Actions workflows
- Code review and PR automation
- Repository management and releases

---

## Core Rules

1. **`gh` CLI first** — faster and more reliable than web UI for automation
2. **Branch protection** — require reviews, status checks, signed commits
3. **Conventional commits** — `feat:`, `fix:`, `docs:`, `refactor:`, `test:`
4. **PR templates** — `.github/pull_request_template.md`
5. **Issue labels** — `bug`, `feature`, `documentation`, `priority:high`

---

## Common Commands

```bash
# Pull Requests
gh pr create --title "Add feature" --body "Description"
gh pr list --state open
gh pr view 123
gh pr diff 123
gh pr merge 123 --squash
gh pr review 123 --approve

# Issues
gh issue create --title "Bug: ..." --label bug
gh issue list --label "priority:high"
gh issue close 123

# Releases
gh release create v1.0.0 --generate-notes
gh release list

# Repo
gh repo clone owner/repo
gh repo view --web

# Actions
gh run list
gh run view 12345
gh run watch 12345
```

---

## GitHub Actions Patterns

### Basic CI
```yaml
name: CI
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-python@v5
        with: { python-version: "3.12" }
      - run: pip install -r requirements.txt
      - run: pytest
```

### Release on tag
```yaml
on:
  push:
    tags: ["v*"]
jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: gh release create ${{ github.ref_name }} --generate-notes
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

---

## GitHub MCP

The `github` MCP is configured globally and provides direct API access to GitHub. Use it for operations beyond what `gh` CLI offers, or when you need structured API responses.

The MCP wraps the GitHub REST/GraphQL API at `https://api.githubcopilot.com/mcp`.

**Prefer `gh` CLI** for common operations (PRs, issues, releases) — it's faster and uses less tokens. Use the MCP for bulk operations, complex queries, or when `gh` output isn't structured enough.

---

## Tools

- `Bash` — `gh` CLI commands
- `github` MCP — direct GitHub API access (fallback)
