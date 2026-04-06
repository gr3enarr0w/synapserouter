# UAT: v1.12 non-git directory handling
# Verify: No worktree warning when running in non-git directory
# Run for: personal profile, NO_COLOR

Output tests/ui/recordings/v112-non-git-dir.gif
Set Width 1200
Set Height 600

# Create a non-git directory
Type "mkdir -p /tmp/test-nogit-v112"
Enter
Sleep 0.5s

Type "cd /tmp/test-nogit-v112"
Enter
Sleep 0.5s

# Run synroute - should NOT show worktree warning
Type "NO_COLOR=1 /Users/ceverson/Development/synapserouter/synroute code"
Enter
Sleep 4s
Screenshot tests/ui/screenshots/v112-non-git-dir.png

# Exit
Type "/exit"
Enter
Sleep 2s

# Cleanup
Type "cd /Users/ceverson/Development/synapserouter"
Enter
Sleep 0.5s
Type "rm -rf /tmp/test-nogit-v112"
Enter
Sleep 0.5s
