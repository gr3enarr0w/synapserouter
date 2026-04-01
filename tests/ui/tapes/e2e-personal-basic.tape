# E2E Personal Profile — Basic Flow
# Tests: launch, greeting, knowledge, shortcuts, Ctrl-C, exit

Output tests/ui/recordings/e2e-personal-basic.gif
Set Width 1200
Set Height 800

# 1. Launch from home dir
Type "cd ~ && /Users/ceverson/Development/synapserouter/synroute code"
Enter
Sleep 5s
Screenshot tests/ui/screenshots/e2e-p-01-launch-home.png

# 2. Hello — should be conversational, no tools
Type "hello"
Enter
Sleep 10s
Screenshot tests/ui/screenshots/e2e-p-02-hello.png

# 3. Knowledge question — text only
Type "what is a goroutine in Go"
Enter
Sleep 12s
Screenshot tests/ui/screenshots/e2e-p-03-knowledge.png

# 4. Ctrl-L — cycle verbosity
Ctrl+L
Sleep 2s
Screenshot tests/ui/screenshots/e2e-p-04-verbosity.png

# 5. Ctrl-P — pipeline status
Ctrl+P
Sleep 2s
Screenshot tests/ui/screenshots/e2e-p-05-pipeline.png

# 6. Help
Type "/help"
Enter
Sleep 3s
Screenshot tests/ui/screenshots/e2e-p-06-help.png

# 7. Empty input
Enter
Sleep 2s
Screenshot tests/ui/screenshots/e2e-p-07-empty.png

# 8. Ctrl-C clear text
Type "something to clear"
Ctrl+C
Sleep 2s
Screenshot tests/ui/screenshots/e2e-p-08-ctrlc-clear.png

# 9. Double Ctrl-C exit
Ctrl+C
Sleep 1s
Ctrl+C
Sleep 3s
Screenshot tests/ui/screenshots/e2e-p-09-exit.png
