# Verify #94 (recall scoping) + #353 (sub-agent recall hint)
# Uses /review command which spawns a sub-agent for code review

Output tests/ui/recordings/verify-subagent.gif
Set Width 1200
Set Height 800

Type "cd /Users/ceverson/Development/synapserouter && ./synroute code 2>tests/ui/agent-subagent-logs.txt"
Enter
Sleep 5s

# /review spawns a sub-agent for code review
Type "/review check the banner_art.go file"
Enter
Sleep 30s
Screenshot tests/ui/screenshots/verify-subagent-review.png
Sleep 30s
Screenshot tests/ui/screenshots/verify-subagent-result.png

Type "/exit"
Enter
Sleep 2s
