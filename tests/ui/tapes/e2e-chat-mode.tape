# E2E — Chat mode (synroute chat, not code)
Output tests/ui/recordings/e2e-chat-mode.gif
Set Width 1200
Set Height 800

Type "cd /Users/ceverson/Development/synapserouter && ./synroute chat"
Enter
Sleep 5s
Screenshot tests/ui/screenshots/e2e-extra-01-chat-launch.png

Type "hello, what can you do?"
Enter
Sleep 10s
Screenshot tests/ui/screenshots/e2e-extra-02-chat-hello.png

Type "/exit"
Enter
Sleep 2s
Screenshot tests/ui/screenshots/e2e-extra-03-chat-exit.png
