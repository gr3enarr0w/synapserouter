# Phase 1.1.1 — Git status via agent
Output tests/ui/recordings/e2e-1.1-git-status.gif
Set Width 1200
Set Height 800

Type "cd /Users/ceverson/Development/synapserouter && ./synroute code"
Enter
Sleep 5s
Screenshot tests/ui/screenshots/p1-1.1-01-launch.png

Type "show me the git status"
Enter
Sleep 15s
Screenshot tests/ui/screenshots/p1-1.1-02-git-status.png
Sleep 5s
Screenshot tests/ui/screenshots/p1-1.1-03-git-status-done.png

Type "/exit"
Enter
Sleep 3s
