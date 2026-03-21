---
name: code-review
description: "Structured code review — quality, SOLID principles, performance, multi-language."
triggers:
  - "review"
  - "clean"
  - "quality"
  - "check"
role: reviewer
phase: review
verify:
  - name: "build succeeds"
    command: "go build ./... 2>&1 || npm run build 2>&1 || python -m py_compile *.py 2>&1 || echo 'BUILD_CHECK'"
    manual: "Verify the project builds without errors using the appropriate language tool."
  - name: "no TODO/FIXME in new code"
    command: "grep -rn 'TODO\\|FIXME\\|HACK\\|XXX' --include='*.go' --include='*.py' --include='*.ts' --include='*.js' | grep -v 'vendor\\|node_modules\\|_test' | head -5 || echo 'OK'"
    manual: "Review any TODO/FIXME markers. They may indicate incomplete implementation that should be resolved before shipping."
  - name: "function length"
    command: "awk '/^func /{name=$0; lines=0} /^func /,/^}$/{lines++} lines>80{print \"LONG: \" name \" (\" lines \" lines)\"}' $(find . -name '*.go' -not -path '*/vendor/*' -not -name '*_test.go') 2>/dev/null | head -5 || echo 'OK'"
    manual: "Functions over 80 lines may violate Single Responsibility. Consider breaking into smaller functions."
  - name: "error return values checked"
    command: "grep -rn '_ = .*err\\|, _ :=\\|, _ =' --include='*.go' | grep -v '_test.go\\|vendor' | head -5 || echo 'OK'"
    manual: "Discarded error return values may hide failures. Each should be explicitly handled or documented why it's safe to ignore."
---
# Skill: Code Review

Structured PR review — code quality, SOLID principles, performance, multi-language.

Source: [Structured PR Reviewer](https://mcpmarket.com/tools/skills/structured-pr-reviewer), [Advanced Code Review & Refactor](https://mcpmarket.com/es/tools/skills/advanced-code-review-refactor).

---

## When to Use

- Reviewing pull requests
- Code quality audits
- Refactoring assessment
- Performance review

## When NOT to Use

- For security-specific audits → use `security-review`

---

## Review Process

### 1. Understand context
- Read the PR description and linked issues
- Understand what changed and why

### 2. Check functionality
- Does the code do what the PR claims?
- Are edge cases handled?
- Are error states considered?

### 3. Check quality
| Dimension | What to Look For |
|-----------|-----------------|
| Readability | Clear names, small functions, minimal nesting |
| SOLID | Single responsibility, open/closed, interface segregation |
| DRY | No duplicated logic (but avoid premature abstraction) |
| Performance | N+1 queries, unnecessary allocations, blocking I/O |
| Testing | Adequate coverage, meaningful assertions, edge cases |
| Documentation | Public API documented, complex logic explained |

### 4. Provide actionable feedback
- **Must fix**: Bugs, security issues, data loss risks
- **Should fix**: Performance issues, poor naming, missing tests
- **Nit**: Style preferences, optional improvements

---

## Quick Review Commands

```bash
# View PR diff
gh pr diff <number>
# View PR files changed
gh pr view <number> --json files
# Check PR status
gh pr checks <number>
```

---

## Anti-Patterns in Reviews

- Nitpicking style when a linter should handle it
- Blocking on subjective preferences
- Reviewing without running the code
- Ignoring test coverage gaps
- Rubber-stamping without reading
