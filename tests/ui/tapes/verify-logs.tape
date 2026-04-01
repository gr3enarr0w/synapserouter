# Verify: stall detection logs, tool call format, plan-first
# Runs a simple task and captures logs for verification

Output tests/ui/recordings/verify-logs.gif
Set Width 1200
Set Height 800

Type "cd /Users/ceverson/Development/synapserouter && ./synroute code 2>tests/ui/screenshots/agent-logs.txt"
Enter
Sleep 5s

# Simple hello — should NOT trigger stall (#352)
Type "hello"
Enter
Sleep 10s
Screenshot tests/ui/screenshots/verify-hello-logs.png

# Ask to read a file — verifies tool call logging (#354)
Type "read the first 10 lines of go.mod"
Enter
Sleep 10s
Screenshot tests/ui/screenshots/verify-toolcall-logs.png

Type "/exit"
Enter
Sleep 2s
