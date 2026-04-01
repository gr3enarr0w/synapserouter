# UAT 04: Knowledge question
# Verify: "what is a goroutine" → text only, no tools
# Launch dir: project dir

Output tests/ui/recordings/04-knowledge-question.gif
Set Width 1200
Set Height 600

Type "cd /Users/ceverson/Development/synapserouter && ./synroute code"
Enter
Sleep 5s

Type "what is a goroutine"
Enter
Sleep 15s
Screenshot tests/ui/screenshots/04-knowledge-response.png

Type "/exit"
Enter
Sleep 2s
