# E2E Work Profile — /review command
Output tests/ui/recordings/e2e-work-review.gif
Set Width 1200
Set Height 800

Env ACTIVE_PROFILE "work"

Type "cd /Users/ceverson/Development/synapserouter && ./synroute code"
Enter
Sleep 5s

Type "/review check go.mod"
Enter
Sleep 45s
Screenshot tests/ui/screenshots/e2e-wf-06-review-progress.png
Sleep 30s
Screenshot tests/ui/screenshots/e2e-wf-07-review-result.png

Type "/exit"
Enter
Sleep 2s
