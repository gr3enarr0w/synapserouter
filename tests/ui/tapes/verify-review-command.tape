# Verify /review command works without crash and produces useful output
Output tests/ui/recordings/verify-review.gif
Set Width 1200
Set Height 800

Type "cd /Users/ceverson/Development/synapserouter && ./synroute code"
Enter
Sleep 5s

Type "/review check banner_art.go for code quality"
Enter
Sleep 45s
Screenshot tests/ui/screenshots/verify-review-progress.png
Sleep 45s
Screenshot tests/ui/screenshots/verify-review-result.png

Type "/exit"
Enter
Sleep 2s
