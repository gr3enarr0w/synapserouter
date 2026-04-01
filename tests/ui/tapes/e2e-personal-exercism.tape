# E2E Personal Profile — Exercism Workflow
# Tests: create project, implement, test, verify

Output tests/ui/recordings/e2e-personal-exercism.gif
Set Width 1200
Set Height 800

# 1. Create temp dir and launch
Type "mkdir -p /tmp/e2e-exercism && cd /tmp/e2e-exercism && /Users/ceverson/Development/synapserouter/synroute code"
Enter
Sleep 5s
Screenshot tests/ui/screenshots/e2e-p-20-exercism-launch.png

# 2. Solve exercism hello-world
Type "solve the Go exercism hello-world exercise: create go.mod, hello.go, and hello_test.go"
Enter
Sleep 60s
Screenshot tests/ui/screenshots/e2e-p-21-exercism-progress.png
Sleep 60s
Screenshot tests/ui/screenshots/e2e-p-22-exercism-result.png

# 3. Exit
Type "/exit"
Enter
Sleep 2s
