# Test: /research deep — full depth, budget ceiling test
# Verifies: 3-5 rounds, all backends, budget enforcement, saturation detection
Output tests/ui/recordings/v105-research-deep.gif
Set Width 1200
Set Height 800

Type "cd /Users/ceverson/Development/synapserouter && ./synroute code"
Enter
Sleep 5s

Type "/research deep compare container orchestration Kubernetes vs Nomad vs Docker Swarm"
Enter
Sleep 120s
Screenshot tests/ui/screenshots/v105-research-deep-01-results.png
Sleep 30s
Screenshot tests/ui/screenshots/v105-research-deep-02-done.png

Type "/exit"
Enter
Sleep 2s
