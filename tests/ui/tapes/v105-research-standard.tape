# Test: /research standard — multi-round with paid backends
# Verifies: 2-3 rounds fire, paid backends included, dedup works
Output tests/ui/recordings/v105-research-standard.gif
Set Width 1200
Set Height 800

Type "cd /Users/ceverson/Development/synapserouter && ./synroute code"
Enter
Sleep 5s

Type "/research standard Go error handling middleware patterns"
Enter
Sleep 45s
Screenshot tests/ui/screenshots/v105-research-standard-01-results.png
Sleep 15s
Screenshot tests/ui/screenshots/v105-research-standard-02-done.png

Type "/exit"
Enter
Sleep 2s
