# E2E NO_COLOR — File read, review, exercism
Output tests/ui/recordings/e2e-nocolor-coding.gif
Set Width 1200
Set Height 800

Env NO_COLOR "1"

Type "cd /Users/ceverson/Development/synapserouter && ./synroute code"
Enter
Sleep 5s

# File read
Type "read the first 5 lines of go.mod"
Enter
Sleep 12s
Screenshot tests/ui/screenshots/e2e-nc-06-fileread.png

# /review
Type "/review check go.mod briefly"
Enter
Sleep 45s
Screenshot tests/ui/screenshots/e2e-nc-07-review.png
Sleep 30s
Screenshot tests/ui/screenshots/e2e-nc-08-review-result.png

Type "/exit"
Enter
Sleep 2s
