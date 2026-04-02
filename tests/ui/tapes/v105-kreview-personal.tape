# v1.05 K-LLM Parallel Review — Personal Profile
# Verifies code mode TUI launches and responds correctly after K-LLM changes.
# K-LLM fires during pipeline review phases, so this smoke tests for regression.
Output tests/ui/recordings/v105-kreview-personal.gif

Set Width 1200
Set Height 600
Set Shell "bash"
Set FontSize 14
Set TypingSpeed 50ms

Type "cd /Users/ceverson/Development/synapserouter && ./synroute code"
Enter
Sleep 5s
Screenshot tests/ui/screenshots/v105-kreview-personal-01-launch.png

Type "hello"
Enter
Sleep 8s
Screenshot tests/ui/screenshots/v105-kreview-personal-02-response.png

Type "/exit"
Enter
Sleep 2s
Screenshot tests/ui/screenshots/v105-kreview-personal-03-exit.png
