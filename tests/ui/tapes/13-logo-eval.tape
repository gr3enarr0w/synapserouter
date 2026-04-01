# UAT 13: Logo evaluation
# Verify: Both profiles score 8+ on logo quality
# Launch dir: project dir
# Run separately for personal and work profiles

Output tests/ui/recordings/13-logo-eval.gif
Set Width 1200
Set Height 600

Type "cd /Users/ceverson/Development/synapserouter && ./synroute code"
Enter
Sleep 5s
Screenshot tests/ui/screenshots/13-logo-full.png

Type "/exit"
Enter
Sleep 2s
