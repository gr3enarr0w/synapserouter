# UAT 02: Launch from project directory
# Verify: Logo correct, language detection (go), project state
# Launch dir: /Users/ceverson/Development/synapserouter
# Run for: personal, work, NO_COLOR

Output tests/ui/recordings/02-launch-project.gif
Set Width 1200
Set Height 600

Type "cd /Users/ceverson/Development/synapserouter && ./synroute code"
Enter
Sleep 5s
Screenshot tests/ui/screenshots/02-launch-project.png

Type "/exit"
Enter
Sleep 2s
Screenshot tests/ui/screenshots/02-launch-project-exit.png
