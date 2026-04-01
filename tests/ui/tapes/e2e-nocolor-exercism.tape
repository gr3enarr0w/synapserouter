# E2E NO_COLOR — Exercism solve
Output tests/ui/recordings/e2e-nocolor-exercism.gif
Set Width 1200
Set Height 800

Env NO_COLOR "1"

Type "mkdir -p /tmp/e2e-nc-exercism && cd /tmp/e2e-nc-exercism && /Users/ceverson/Development/synapserouter/synroute code"
Enter
Sleep 5s
Screenshot tests/ui/screenshots/e2e-nc-09-exercism-launch.png

Type "create a Go hello-world with go.mod, hello.go, hello_test.go"
Enter
Sleep 60s
Screenshot tests/ui/screenshots/e2e-nc-10-exercism-progress.png
Sleep 60s
Screenshot tests/ui/screenshots/e2e-nc-11-exercism-result.png

Type "/exit"
Enter
Sleep 2s
