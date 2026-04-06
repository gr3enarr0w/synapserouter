# UAT: v1.12 message queue during execution
# Verify: Typing during execution queues the message
# Run for: personal profile, NO_COLOR

Output tests/ui/recordings/v112-message-queue.gif
Set Width 1200
Set Height 600

# Run synroute code
Type "cd /tmp && NO_COLOR=1 /Users/ceverson/Development/synapserouter/synroute code"
Enter
Sleep 4s

# Send a message that will take time
Type "Create a file called test1.txt with content hello"
Enter
Sleep 1s

# While executing, type another message (should queue)
Type "Now create test2.txt with world"
Enter
Sleep 5s
Screenshot tests/ui/screenshots/v112-message-queue.png

# Exit
Type "/exit"
Enter
Sleep 2s
