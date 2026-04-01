# E2E NO_COLOR — All missing tests
Output tests/ui/recordings/e2e-nocolor-full.gif
Set Width 1200
Set Height 800

Env NO_COLOR "1"

# Launch from project dir
Type "cd /Users/ceverson/Development/synapserouter && ./synroute code"
Enter
Sleep 5s
Screenshot tests/ui/screenshots/e2e-nc-01-launch-project.png

# Knowledge question
Type "what is a docker container"
Enter
Sleep 12s
Screenshot tests/ui/screenshots/e2e-nc-02-knowledge.png

# Keyboard shortcuts
Ctrl+L
Sleep 2s
Screenshot tests/ui/screenshots/e2e-nc-03-shortcuts.png

Ctrl+P
Sleep 2s
Screenshot tests/ui/screenshots/e2e-nc-04-pipeline.png

# Ctrl-C exit
Type "text to clear"
Ctrl+C
Sleep 2s
Ctrl+C
Sleep 1s
Ctrl+C
Sleep 3s
Screenshot tests/ui/screenshots/e2e-nc-05-ctrlc-exit.png
