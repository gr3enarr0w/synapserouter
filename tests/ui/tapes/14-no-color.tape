# UAT 14: NO_COLOR mode
# Verify: Zero ANSI escape codes in output
# Launch dir: project dir

Output tests/ui/recordings/14-no-color.gif
Set Width 1200
Set Height 600

Env NO_COLOR "1"

Type "cd /Users/ceverson/Development/synapserouter && ./synroute code"
Enter
Sleep 5s
Screenshot tests/ui/screenshots/14-no-color-launch.png

Type "hello"
Enter
Sleep 8s
Screenshot tests/ui/screenshots/14-no-color-response.png

Type "/exit"
Enter
Sleep 2s
