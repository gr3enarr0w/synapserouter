# E2E Personal Profile — Coding Workflow
# Tests: project launch, file ops, code gen, tool calls, review

Output tests/ui/recordings/e2e-personal-coding.gif
Set Width 1200
Set Height 800

# 1. Launch from project dir
Type "cd /Users/ceverson/Development/synapserouter && ./synroute code"
Enter
Sleep 5s
Screenshot tests/ui/screenshots/e2e-p-10-launch-project.png

# 2. Read a file — tool call test
Type "read the first 20 lines of go.mod"
Enter
Sleep 12s
Screenshot tests/ui/screenshots/e2e-p-11-read-file.png

# 3. Code review — tests /review command (was crashing before fix)
Type "/review check go.mod for best practices"
Enter
Sleep 45s
Screenshot tests/ui/screenshots/e2e-p-12-review-progress.png
Sleep 30s
Screenshot tests/ui/screenshots/e2e-p-13-review-result.png

# 4. Exit
Type "/exit"
Enter
Sleep 2s
Screenshot tests/ui/screenshots/e2e-p-14-exit.png
