---
name: git-expert
description: "Advanced git workflows — rebase, bisect, cherry-pick, worktrees, conflict resolution."
triggers:
  - "rebase"
  - "bisect"
  - "cherry-pick"
  - "merge conflict"
  - "worktree"
  - "git history"
role: coder
phase: implement
---
# Skill: Git Expert

Advanced git workflows — rebase, bisect, cherry-pick, worktrees, conflict resolution.

---

## When to Use

- Complex git operations (rebase, cherry-pick, bisect)
- Resolving merge conflicts
- Git history archaeology
- Branch management strategies

---

## Core Rules

1. **Commit early, commit often** — small, focused commits
2. **Meaningful commit messages** — imperative mood, explain why not what
3. **Rebase for feature branches** — clean linear history
4. **Merge for long-lived branches** — preserve branch context
5. **Never force-push shared branches** — only your own feature branches
6. **Bisect for bug hunting** — binary search through history

---

## Patterns

### Interactive rebase (clean up before PR)
```bash
git rebase -i HEAD~5  # Squash/reorder last 5 commits
```

### Cherry-pick specific commits
```bash
git cherry-pick abc123         # Single commit
git cherry-pick abc123..def456  # Range of commits
```

### Bisect to find a bug
```bash
git bisect start
git bisect bad                 # Current commit is broken
git bisect good v1.0           # v1.0 was working
# Git checks out middle commit — test and mark:
git bisect good  # or git bisect bad
# Repeat until git finds the culprit
git bisect reset               # Return to original branch
```

### Worktrees (parallel work)
```bash
git worktree add ../feature-branch feature-branch
# Work in ../feature-branch without switching branches
git worktree remove ../feature-branch
```

### Conflict resolution
```bash
git merge feature-branch       # Conflict!
# Edit conflicted files, then:
git add <resolved-files>
git merge --continue
```

### Stash with message
```bash
git stash push -m "WIP: refactoring auth"
git stash list
git stash pop stash@{0}
```

### Find who changed a line
```bash
git blame -L 50,60 file.py
git log -p -S "search_string" -- file.py  # Pickaxe search
```

### Clean up merged branches
```bash
git branch --merged main | grep -v main | xargs git branch -d
```

---

## Commit Message Convention

```
type(scope): short description

Longer explanation if needed.

Fixes #123
```

Types: `feat`, `fix`, `docs`, `refactor`, `test`, `chore`, `ci`
