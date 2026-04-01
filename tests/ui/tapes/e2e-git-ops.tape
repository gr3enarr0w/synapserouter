# E2E — Git operations via agent
Output tests/ui/recordings/e2e-git-ops.gif
Set Width 1200
Set Height 800

Type "cd /Users/ceverson/Development/synapserouter && ./synroute code"
Enter
Sleep 5s

Type "show me the git status and last 3 commits"
Enter
Sleep 15s
Screenshot tests/ui/screenshots/e2e-extra-07-git-status.png
Sleep 10s
Screenshot tests/ui/screenshots/e2e-extra-08-git-log.png

Type "/exit"
Enter
Sleep 2s
