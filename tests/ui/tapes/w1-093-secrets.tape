# Wave 1 #93: Secret scrubbing — verify secrets work in commands
# but don't leak into stored output
# User scenario: ask agent to create a .env file with API key

Output tests/ui/recordings/w1-093-secrets.gif
Set Width 1200
Set Height 800

Type "cd /Users/ceverson/Development/synapserouter && ./synroute code"
Enter
Sleep 5s

Type "write a file /tmp/test_env.txt with content OPENAI_API_KEY=sk-test123456789abcdefghij"
Enter
Sleep 15s
Screenshot tests/ui/screenshots/w1-093-secret-write.png

Type "read /tmp/test_env.txt"
Enter
Sleep 10s
Screenshot tests/ui/screenshots/w1-093-secret-read.png

Type "/exit"
Enter
Sleep 2s
