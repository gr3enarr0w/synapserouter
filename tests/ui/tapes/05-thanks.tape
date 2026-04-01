# UAT 05: Thanks — conversational close
# Verify: "thanks" → text only, no tools
# Launch dir: project dir

Output tests/ui/recordings/05-thanks.gif
Set Width 1200
Set Height 600

Type "cd /Users/ceverson/Development/synapserouter && ./synroute code"
Enter
Sleep 5s

Type "thanks"
Enter
Sleep 8s
Screenshot tests/ui/screenshots/05-thanks-response.png

Type "/exit"
Enter
Sleep 2s
