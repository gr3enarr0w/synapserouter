# UAT 11: /exit command
# Verify: Clean exit with "bye"
# Launch dir: project dir

Output tests/ui/recordings/11-exit.gif
Set Width 1200
Set Height 600

Type "cd /Users/ceverson/Development/synapserouter && ./synroute code"
Enter
Sleep 5s

Type "/exit"
Enter
Sleep 3s
Screenshot tests/ui/screenshots/11-exit.png
