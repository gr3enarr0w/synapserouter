# UAT: v1.12 permission prompt for write operations
# Verify: Permission prompt appears when agent tries to create a file
# Run for: personal profile, NO_COLOR

Output tests/ui/recordings/v112-permission-prompt.gif
Set Width 1200
Set Height 600

# Run synroute code with a request to create a file
Type "cd /tmp && NO_COLOR=1 /Users/ceverson/Development/synapserouter/synroute code --message 'Create a file called test.txt with hello world'"
Enter
Sleep 6s
Screenshot tests/ui/screenshots/v112-permission-prompt.png

# Exit
Type "/exit"
Enter
Sleep 2s
