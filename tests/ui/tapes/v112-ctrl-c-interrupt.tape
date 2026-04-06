# UAT: v1.12 Ctrl-C interrupt during execution
# Verify: Ctrl-C returns to prompt without crashing
# Run for: personal profile, NO_COLOR

Output tests/ui/recordings/v112-ctrl-c-interrupt.gif
Set Width 1200
Set Height 600

# Run synroute code
Type "cd /tmp && NO_COLOR=1 /Users/ceverson/Development/synapserouter/synroute code"
Enter
Sleep 4s

# Send a prompt
Type "Tell me a joke"
Enter
Sleep 2s

# Interrupt with Ctrl-C
Ctrl+C
Sleep 1s
Screenshot tests/ui/screenshots/v112-ctrl-c-interrupt.png

# Verify prompt returns - type something
Type "/exit"
Enter
Sleep 2s
