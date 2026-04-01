# Wave 1 #355: Ctrl-C after conversation
# Verify: Ctrl-C works correctly AFTER having a conversation
# Flow: launch → hello → response → type text → Ctrl-C clears → Ctrl-C hint → Ctrl-C exit

Output tests/ui/recordings/w1-355-ctrlc-mid-convo.gif
Set Width 1200
Set Height 600

Type "cd /Users/ceverson/Development/synapserouter && ./synroute code"
Enter
Sleep 5s

# Have a conversation first
Type "hello"
Enter
Sleep 10s
Screenshot tests/ui/screenshots/w1-355-after-hello.png

# Now type some text and Ctrl-C to clear it
Type "some text I want to clear"
Sleep 1s
Ctrl+C
Sleep 2s
Screenshot tests/ui/screenshots/w1-355-ctrlc-clear-text.png

# Now at empty prompt, first Ctrl-C should show hint
Ctrl+C
Sleep 2s
Screenshot tests/ui/screenshots/w1-355-ctrlc-hint.png

# Second Ctrl-C should exit
Ctrl+C
Sleep 3s
Screenshot tests/ui/screenshots/w1-355-ctrlc-exit.png
