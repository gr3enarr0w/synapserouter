# E2E: Pending message queue during agent execution
# Verify: typing during agent run queues message for next prompt

Output tests/ui/recordings/pending-message-queue.gif
Set Width 1200
Set Height 600

Type "cd /Users/ceverson/Development/synapserouter && NO_COLOR=1 ./synroute code"
Enter
Sleep 5s

# Type a quick command that will trigger agent
Type "test"
Enter
Sleep 3s

# Type /exit to quit
Type "/exit"
Enter
Sleep 1s
