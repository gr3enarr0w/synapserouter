# v1.05 K-LLM Parallel Review — NO_COLOR Mode
# Verifies code mode TUI works with NO_COLOR after K-LLM changes.
Output tests/ui/recordings/v105-kreview-nocolor.gif

Set Width 1200
Set Height 600
Set Shell "bash"
Set FontSize 14
Set TypingSpeed 50ms

Env NO_COLOR "1"

Type "cd /Users/ceverson/Development/synapserouter && ./synroute code"
Enter
Sleep 5s
Screenshot tests/ui/screenshots/v105-kreview-nocolor-01-launch.png

Type "hello"
Enter
Sleep 8s
Screenshot tests/ui/screenshots/v105-kreview-nocolor-02-response.png

Type "/exit"
Enter
Sleep 2s
Screenshot tests/ui/screenshots/v105-kreview-nocolor-03-exit.png
