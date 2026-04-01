# UAT 15: Empty input
# Verify: Enter without typing → prompt returns (no crash, no error)
# Launch dir: project dir

Output tests/ui/recordings/15-empty-input.gif
Set Width 1200
Set Height 600

Type "cd /Users/ceverson/Development/synapserouter && ./synroute code"
Enter
Sleep 5s

Enter
Sleep 3s
Screenshot tests/ui/screenshots/15-empty-input.png

Enter
Sleep 3s
Screenshot tests/ui/screenshots/15-empty-input-2.png

Type "/exit"
Enter
Sleep 2s
