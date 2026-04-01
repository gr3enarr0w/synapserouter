# E2E Personal Profile — NO_COLOR Mode
# Tests: accessibility, no ANSI escapes

Output tests/ui/recordings/e2e-personal-nocolor.gif
Set Width 1200
Set Height 600

Env NO_COLOR "1"

Type "cd /Users/ceverson/Development/synapserouter && ./synroute code"
Enter
Sleep 5s
Screenshot tests/ui/screenshots/e2e-p-30-nocolor-launch.png

Type "hello"
Enter
Sleep 10s
Screenshot tests/ui/screenshots/e2e-p-31-nocolor-hello.png

Type "/help"
Enter
Sleep 3s
Screenshot tests/ui/screenshots/e2e-p-32-nocolor-help.png

Type "/exit"
Enter
Sleep 2s
