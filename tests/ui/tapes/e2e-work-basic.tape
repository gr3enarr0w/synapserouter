# E2E Work Profile — Basic + Coding Flow
# Tests: launch, greeting, file ops, review, shortcuts, exit

Output tests/ui/recordings/e2e-work-basic.gif
Set Width 1200
Set Height 800

Env ACTIVE_PROFILE "work"

# 1. Launch from home dir
Type "cd ~ && /Users/ceverson/Development/synapserouter/synroute code"
Enter
Sleep 5s
Screenshot tests/ui/screenshots/e2e-w-01-launch-home.png

# 2. Hello
Type "hello"
Enter
Sleep 10s
Screenshot tests/ui/screenshots/e2e-w-02-hello.png

# 3. Knowledge question
Type "explain kubernetes pods briefly"
Enter
Sleep 12s
Screenshot tests/ui/screenshots/e2e-w-03-knowledge.png

# 4. Exit and relaunch from project dir
Type "/exit"
Enter
Sleep 2s

Type "cd /Users/ceverson/Development/synapserouter && ./synroute code"
Enter
Sleep 5s
Screenshot tests/ui/screenshots/e2e-w-04-launch-project.png

# 5. Read a file
Type "show me the first 5 lines of main.go"
Enter
Sleep 12s
Screenshot tests/ui/screenshots/e2e-w-05-read-file.png

# 6. Help
Type "/help"
Enter
Sleep 3s
Screenshot tests/ui/screenshots/e2e-w-06-help.png

# 7. Pipeline status
Ctrl+P
Sleep 2s
Screenshot tests/ui/screenshots/e2e-w-07-pipeline.png

# 8. Exit
Type "/exit"
Enter
Sleep 2s
Screenshot tests/ui/screenshots/e2e-w-08-exit.png
