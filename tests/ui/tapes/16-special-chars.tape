# UAT 16: Special characters
# Verify: "what does && do" → handled correctly, no shell interpretation
# Launch dir: project dir

Output tests/ui/recordings/16-special-chars.gif
Set Width 1200
Set Height 600

Type "cd /Users/ceverson/Development/synapserouter && ./synroute code"
Enter
Sleep 5s

Type "what does && do in bash"
Enter
Sleep 10s
Screenshot tests/ui/screenshots/16-special-chars.png

Type "/exit"
Enter
Sleep 2s
