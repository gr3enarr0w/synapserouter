# E2E — Fix a bug scenario
Output tests/ui/recordings/e2e-fix-bug.gif
Set Width 1200
Set Height 800

# Create a file with a deliberate bug, then ask agent to fix it
Type "mkdir -p /tmp/e2e-bugfix && cd /tmp/e2e-bugfix && /Users/ceverson/Development/synapserouter/synroute code"
Enter
Sleep 5s

Type "create a Go file called calc.go with a function Add(a, b int) int that returns a - b (this is a bug, it should add). Then create calc_test.go that tests Add(2,3)==5. Run the tests to show the failure."
Enter
Sleep 45s
Screenshot tests/ui/screenshots/e2e-extra-09-bugfix-progress.png
Sleep 45s
Screenshot tests/ui/screenshots/e2e-extra-10-bugfix-result.png

Type "/exit"
Enter
Sleep 2s
