# E2E Work Profile — Exercism Solve
Output tests/ui/recordings/e2e-work-exercism.gif
Set Width 1200
Set Height 800

Env ACTIVE_PROFILE "work"

Type "mkdir -p /tmp/e2e-work-exercism && cd /tmp/e2e-work-exercism && /Users/ceverson/Development/synapserouter/synroute code"
Enter
Sleep 5s
Screenshot tests/ui/screenshots/e2e-w-10-exercism-launch.png

Type "create a simple Go hello-world exercise with go.mod, hello.go returning 'Hello, World!', and hello_test.go"
Enter
Sleep 60s
Screenshot tests/ui/screenshots/e2e-w-11-exercism-progress.png
Sleep 60s
Screenshot tests/ui/screenshots/e2e-w-12-exercism-result.png

Type "/exit"
Enter
Sleep 2s
