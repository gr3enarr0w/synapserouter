# Phase 1.1.4-1.1.6 — Git write operations (branch, commit guardrail, push guardrail)
Output tests/ui/recordings/e2e-1.1-git-write.gif
Set Width 1200
Set Height 800

Type "cd /Users/ceverson/Development/synapserouter && ./synroute code"
Enter
Sleep 5s

# 1.1.4 — Create a branch
Type "create a new branch called test-git-ops"
Enter
Sleep 15s
Screenshot tests/ui/screenshots/p1-1.1-08-git-branch.png
Sleep 5s
Screenshot tests/ui/screenshots/p1-1.1-09-git-branch-done.png

# 1.1.5 — Commit guardrail
Type "commit all changes with message test commit"
Enter
Sleep 15s
Screenshot tests/ui/screenshots/p1-1.1-10-git-commit.png
Sleep 5s
Screenshot tests/ui/screenshots/p1-1.1-11-git-commit-done.png

# 1.1.6 — Push guardrail
Type "push to remote"
Enter
Sleep 15s
Screenshot tests/ui/screenshots/p1-1.1-12-git-push.png
Sleep 5s
Screenshot tests/ui/screenshots/p1-1.1-13-git-push-done.png

Type "/exit"
Enter
Sleep 3s
