# UAT: v1.12 rejection escalation for python3 -c
# Verify: Repeated python3 -c attempts are rejected/escalated
# Run for: personal profile, NO_COLOR

Output tests/ui/recordings/v112-rejection-escalation.gif
Set Width 1200
Set Height 600

# Run synroute code
Type "cd /tmp && NO_COLOR=1 /Users/ceverson/Development/synapserouter/synroute code"
Enter
Sleep 4s

# Try to run python3 -c (should be rejected)
Type "Run python3 -c 'print(123)'"
Enter
Sleep 3s
Screenshot tests/ui/screenshots/v112-rejection-escalation-1.png

# Try again (should escalate/reject again)
Type "Just run python3 -c 'print(456)'"
Enter
Sleep 3s
Screenshot tests/ui/screenshots/v112-rejection-escalation-2.png

# Exit
Type "/exit"
Enter
Sleep 2s
