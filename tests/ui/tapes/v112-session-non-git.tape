# UAT: v1.12 session save in non-git directory
# Verify: Session saves correctly even without git worktree
# Run for: personal profile, NO_COLOR

Output tests/ui/recordings/v112-session-non-git.gif
Set Width 1200
Set Height 600

# Create a non-git directory
Type "mkdir -p /tmp/test-session-nogit"
Enter
Sleep 0.5s

Type "cd /tmp/test-session-nogit"
Enter
Sleep 0.5s

# Run synroute and do something
Type "NO_COLOR=1 /Users/ceverson/Development/synapserouter/synroute code"
Enter
Sleep 4s

# Send a message
Type "Remember this test session"
Enter
Sleep 3s

# Check session
Type "/history"
Enter
Sleep 2s
Screenshot tests/ui/screenshots/v112-session-non-git.png

# Exit
Type "/exit"
Enter
Sleep 2s

# Cleanup
Type "cd /Users/ceverson/Development/synapserouter"
Enter
Sleep 0.5s
Type "rm -rf /tmp/test-session-nogit"
Enter
Sleep 0.5s
