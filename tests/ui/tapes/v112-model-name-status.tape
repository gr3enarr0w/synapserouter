# UAT: v1.12 status bar shows model name not provider ID
# Verify: Status bar displays friendly model name (e.g., "claude-sonnet-4") not provider ID
# Run for: personal profile, NO_COLOR

Output tests/ui/recordings/v112-model-name-status.gif
Set Width 1200
Set Height 600

# Run synroute code
Type "cd /tmp && NO_COLOR=1 /Users/ceverson/Development/synapserouter/synroute code"
Enter
Sleep 4s

# Send a simple prompt to trigger status bar update
Type "Hello"
Enter
Sleep 2s
Screenshot tests/ui/screenshots/v112-model-name-status.png

# Exit
Type "/exit"
Enter
Sleep 2s
