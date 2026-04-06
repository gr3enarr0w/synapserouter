# UAT: v1.12 completion summary block
# Verify: Done summary block appears after task completion
# Run for: personal profile, NO_COLOR

Output tests/ui/recordings/v112-completion-summary.gif
Set Width 1200
Set Height 600

# Run synroute code with a simple task
Type "cd /tmp && NO_COLOR=1 /Users/ceverson/Development/synapserouter/synroute code --message 'Create a file called hello.txt with the text hello'"
Enter
Sleep 10s
Screenshot tests/ui/screenshots/v112-completion-summary.png

# Exit
Type "/exit"
Enter
Sleep 2s
