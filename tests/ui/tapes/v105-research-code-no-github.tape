# Test: Code query with NO GitHub/Sourcegraph tokens — should fallback to web
# Verifies: code query classification falls back to web backends
Output tests/ui/recordings/v105-research-code-no-github.gif
Set Width 1200
Set Height 800

Env GITHUB_TOKEN ""
Env SOURCEGRAPH_TOKEN ""

Type "cd /Users/ceverson/Development/synapserouter && ./synroute code"
Enter
Sleep 5s

Type "/research quick Go struct embedding interface patterns"
Enter
Sleep 20s
Screenshot tests/ui/screenshots/v105-research-code-no-github-01.png

Type "/exit"
Enter
Sleep 2s
