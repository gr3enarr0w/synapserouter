# UAT: v1.12 /plan command
# Verify: /plan command shows execution plan
# Run for: personal profile, NO_COLOR

Output tests/ui/recordings/v112-pipeline-plan.gif
Set Width 1200
Set Height 600

# Run synroute code
Type "cd /tmp && NO_COLOR=1 /Users/ceverson/Development/synapserouter/synroute code"
Enter
Sleep 4s

# Use /plan command
Type "/plan Create a simple Go program that prints hello world"
Enter
Sleep 4s
Screenshot tests/ui/screenshots/v112-pipeline-plan.png

# Exit
Type "/exit"
Enter
Sleep 2s
