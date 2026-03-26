---
name: git-hooks
description: "Git hooks — pre-commit, pre-push, husky, lint-staged, commit validation."
triggers:
  - "pre-commit"
  - "pre-push"
  - "husky"
  - "lint-staged"
  - "git hook"
  - "commit hook"
role: coder
phase: implement
verify:
  - name: "hooks exist"
    command: "ls .husky/ 2>/dev/null || ls .git/hooks/pre-commit 2>/dev/null || ls .pre-commit-config.yaml 2>/dev/null || echo 'NO_HOOKS'"
    expect_not: "NO_HOOKS"
---
# git-hooks

## Frameworks
- **Husky** (JS): `.husky/pre-commit`, runs via `npx husky`
- **pre-commit** (Python): `.pre-commit-config.yaml`, language-agnostic
- **lefthook** (Go): `lefthook.yml`, fast, parallel execution
- **Raw**: `.git/hooks/` scripts (not portable)

## Common Hooks
| Hook | When | Use For |
|------|------|---------|
| pre-commit | Before commit created | Lint, format, type-check |
| commit-msg | After message written | Validate conventional commits |
| pre-push | Before push to remote | Run tests, check coverage |
| prepare-commit-msg | Before editor opens | Add ticket number |

## lint-staged (JS)
Run linters only on staged files:
```json
{ "*.{ts,tsx}": ["eslint --fix", "prettier --write"] }
```

## pre-commit (Python)
```yaml
repos:
  - repo: https://github.com/pre-commit/pre-commit-hooks
    hooks:
      - id: trailing-whitespace
      - id: check-yaml
```

## Anti-Patterns
- Hooks that take >5 seconds (use pre-push for slow checks)
- No `--no-verify` escape hatch documented
- Hooks not in version control (use husky/pre-commit, not .git/hooks)
