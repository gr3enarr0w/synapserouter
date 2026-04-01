# UAT 12: Self-fix task
# Verify: plan-first fires, skills inject, verification runs
# Launch dir: project dir

Output tests/ui/recordings/12-self-fix.gif
Set Width 1200
Set Height 600

Type "cd /Users/ceverson/Development/synapserouter && ./synroute code"
Enter
Sleep 5s

Type "add a comment to main.go explaining what the file does"
Enter
Sleep 30s
Screenshot tests/ui/screenshots/12-self-fix-progress.png
Sleep 30s
Screenshot tests/ui/screenshots/12-self-fix-result.png

Type "/exit"
Enter
Sleep 2s
