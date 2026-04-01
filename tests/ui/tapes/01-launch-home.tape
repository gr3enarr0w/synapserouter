# UAT 01: Launch from home directory
# Verify: Logo correct, profile name, tier count, no garbage
# Launch dir: ~/
# Run for: personal, work, NO_COLOR

Output tests/ui/recordings/01-launch-home.gif
Set Width 1200
Set Height 600

Type "cd ~ && /Users/ceverson/Development/synapserouter/synroute code"
Enter
Sleep 5s
Screenshot tests/ui/screenshots/01-launch-home.png

Type "/exit"
Enter
Sleep 2s
Screenshot tests/ui/screenshots/01-launch-home-exit.png
