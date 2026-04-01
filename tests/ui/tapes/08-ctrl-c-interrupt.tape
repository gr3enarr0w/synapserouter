# UAT 08: Ctrl-C interrupt
# Verify: Interrupts streaming response, returns to prompt
# Launch dir: project dir

Output tests/ui/recordings/08-ctrl-c-interrupt.gif
Set Width 1200
Set Height 600

Type "cd /Users/ceverson/Development/synapserouter && ./synroute code"
Enter
Sleep 5s

Type "explain the entire history of computing in detail"
Enter
Sleep 5s
Ctrl+C
Sleep 3s
Screenshot tests/ui/screenshots/08-ctrl-c-interrupt.png

Type "/exit"
Enter
Sleep 2s
