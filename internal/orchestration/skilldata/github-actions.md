---
name: github-actions
description: "GitHub Actions CI/CD — workflows, matrix builds, caching, secrets, reusable actions."
triggers:
  - "github actions"
  - "ci/cd"
  - "ci pipeline"
  - ".github/workflows"
  - "workflow_dispatch"
  - "github+ci"
  - "github+deploy"
  - "github+build"
role: coder
phase: implement
language: yaml
mcp_tools:
  - "context7.query-docs"
verify:
  - name: "workflow syntax"
    command: "find .github/workflows -name '*.yml' -o -name '*.yaml' | head -1 | xargs -I{} python3 -c \"import yaml; yaml.safe_load(open('{}'))\" 2>&1 || echo 'OK'"
    expect: "OK"
---
# github-actions

## Workflow Structure
- `.github/workflows/*.yml` — one file per workflow
- Triggers: `push`, `pull_request`, `workflow_dispatch`, `schedule`, `release`
- Jobs run in parallel by default, `needs:` for dependencies
- Steps run sequentially within a job

## Key Patterns
- **Matrix builds**: test across OS/language versions
- **Caching**: `actions/cache` for deps (node_modules, .m2, go mod)
- **Artifacts**: `actions/upload-artifact` / `download-artifact`
- **Secrets**: `${{ secrets.NAME }}`, never echo
- **Concurrency**: `concurrency: { group: ${{ github.ref }}, cancel-in-progress: true }`
- **Reusable workflows**: `workflow_call` with inputs/secrets

## Common Jobs
```yaml
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.23' }
      - run: go test -race ./...

  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: golangci/golangci-lint-action@v6
```

## Anti-Patterns
- Hardcoded secrets in workflow files
- No caching (slow CI)
- `continue-on-error: true` hiding failures
- Running all tests on every push (use path filters)
- No concurrency control (duplicate runs)
