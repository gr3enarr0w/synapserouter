# Wave 3 edge case testing — multiple scenarios
# Tests trigger quality, language detection, and general UX

Output tests/ui/recordings/w3-edge-cases.gif
Set Width 1200
Set Height 800

Type "cd /Users/ceverson/Development/synapserouter && ./synroute code"
Enter
Sleep 5s

# Edge 1: "trust" should NOT trigger rust skill
Type "I trust this approach will work"
Enter
Sleep 10s
Screenshot tests/ui/screenshots/w3-edge-trust.png

# Edge 2: "mysql" should NOT trigger sql skill
Type "how do I connect to mysql"
Enter
Sleep 10s
Screenshot tests/ui/screenshots/w3-edge-mysql.png

# Edge 3: "build" as standalone word in coding context SHOULD work
Type "build the project and run tests"
Enter
Sleep 15s
Screenshot tests/ui/screenshots/w3-edge-build.png

Type "/exit"
Enter
Sleep 2s
