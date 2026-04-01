# E2E Work Profile — Missing tests: shortcuts, Ctrl-C, empty, review, nocolor
Output tests/ui/recordings/e2e-work-full.gif
Set Width 1200
Set Height 800

Env ACTIVE_PROFILE "work"

Type "cd /Users/ceverson/Development/synapserouter && ./synroute code"
Enter
Sleep 5s

# Keyboard shortcuts
Ctrl+L
Sleep 2s
Screenshot tests/ui/screenshots/e2e-wf-01-shortcuts-verbosity.png

Ctrl+T
Sleep 2s
Screenshot tests/ui/screenshots/e2e-wf-02-shortcuts-tools.png

# Empty input
Enter
Sleep 2s
Screenshot tests/ui/screenshots/e2e-wf-03-empty.png

# Ctrl-C clear + exit
Type "text to clear"
Ctrl+C
Sleep 2s
Screenshot tests/ui/screenshots/e2e-wf-04-ctrlc-clear.png

Ctrl+C
Sleep 1s
Ctrl+C
Sleep 3s
Screenshot tests/ui/screenshots/e2e-wf-05-ctrlc-exit.png
