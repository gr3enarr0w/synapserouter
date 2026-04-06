# UAT: v1.12 compact verbosity toggle
# Verify: Ctrl-L toggles verbosity and shows 'verbosity: compact'
# Run for: personal profile, NO_COLOR

Output tests/ui/recordings/v112-compact-verbosity.gif
Set Width 1200
Set Height 600

# Run synroute code
Type "cd /tmp && NO_COLOR=1 /Users/ceverson/Development/synapserouter/synroute code"
Enter
Sleep 4s

# Press Ctrl-L to toggle verbosity
Ctrl+L
Sleep 1s
Screenshot tests/ui/screenshots/v112-compact-verbosity.png

# Exit
Type "/exit"
Enter
Sleep 2s
