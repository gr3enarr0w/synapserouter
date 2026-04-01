# UAT 10: /help command
# Verify: All commands and shortcuts listed
# Launch dir: project dir

Output tests/ui/recordings/10-help.gif
Set Width 1200
Set Height 600

Type "cd /Users/ceverson/Development/synapserouter && ./synroute code"
Enter
Sleep 5s

Type "/help"
Enter
Sleep 5s
Screenshot tests/ui/screenshots/10-help.png

Type "/exit"
Enter
Sleep 2s
