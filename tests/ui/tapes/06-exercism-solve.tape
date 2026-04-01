# UAT 06: Exercism solve
# Verify: plan→implement→test→verify, all tests pass
# Launch dir: /tmp/test-exercism
# Note: Needs an exercism exercise set up first

Output tests/ui/recordings/06-exercism-solve.gif
Set Width 1200
Set Height 600

Type "mkdir -p /tmp/test-exercism && cd /tmp/test-exercism && /Users/ceverson/Development/synapserouter/synroute code"
Enter
Sleep 5s
Screenshot tests/ui/screenshots/06-exercism-launch.png

Type "solve the go exercism hello-world exercise"
Enter
Sleep 60s
Screenshot tests/ui/screenshots/06-exercism-progress.png
Sleep 60s
Screenshot tests/ui/screenshots/06-exercism-result.png

Type "/exit"
Enter
Sleep 2s
