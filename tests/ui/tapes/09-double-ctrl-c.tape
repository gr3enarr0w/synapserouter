# UAT 09: Double Ctrl-C exit
# Verify: Exits with "bye"
# Launch dir: project dir

Output tests/ui/recordings/09-double-ctrl-c.gif
Set Width 1200
Set Height 600

Type "cd /Users/ceverson/Development/synapserouter && ./synroute code"
Enter
Sleep 5s
Screenshot tests/ui/screenshots/09-before-exit.png

Ctrl+C
Sleep 1s
Ctrl+C
Sleep 3s
Screenshot tests/ui/screenshots/09-double-ctrl-c-exit.png
