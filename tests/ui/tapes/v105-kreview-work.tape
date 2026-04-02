# v1.05 K-LLM Parallel Review — Work Profile
# Verifies code mode TUI launches correctly on work profile after K-LLM changes.
Output tests/ui/recordings/v105-kreview-work.gif

Set Width 1200
Set Height 600
Set Shell "bash"
Set FontSize 14
Set TypingSpeed 50ms

Env ACTIVE_PROFILE "work"

Type "cd /Users/ceverson/Development/synapserouter && ./synroute code"
Enter
Sleep 5s
Screenshot tests/ui/screenshots/v105-kreview-work-01-launch.png

Type "hello"
Enter
Sleep 8s
Screenshot tests/ui/screenshots/v105-kreview-work-02-response.png

Type "/exit"
Enter
Sleep 2s
Screenshot tests/ui/screenshots/v105-kreview-work-03-exit.png
