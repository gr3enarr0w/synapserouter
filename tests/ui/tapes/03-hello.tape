# UAT 03: Hello — conversational response
# Verify: Text response, no tools, no stall, no planning triggered
# Launch dir: project dir

Output tests/ui/recordings/03-hello.gif
Set Width 1200
Set Height 600

Type "cd /Users/ceverson/Development/synapserouter && ./synroute code"
Enter
Sleep 5s
Screenshot tests/ui/screenshots/03-hello-banner.png

Type "hello"
Enter
Sleep 10s
Screenshot tests/ui/screenshots/03-hello-response.png

Type "/exit"
Enter
Sleep 2s
