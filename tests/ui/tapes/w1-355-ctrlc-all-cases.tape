# Wave 1 #355: ALL Ctrl-C use cases
# Tests every scenario a user would encounter

Output tests/ui/recordings/w1-355-ctrlc-all.gif
Set Width 1200
Set Height 600

Type "cd /Users/ceverson/Development/synapserouter && ./synroute code"
Enter
Sleep 5s

# CASE 1: Text on line → Ctrl-C clears text (not exit)
Type "something I changed my mind about"
Sleep 1s
Ctrl+C
Sleep 2s
Screenshot tests/ui/screenshots/w1-355-case1-clear-text.png

# CASE 2: After clearing, type new text (verify REPL still works)
Type "hello"
Enter
Sleep 10s
Screenshot tests/ui/screenshots/w1-355-case2-still-works.png

# CASE 3: During LLM response → Ctrl-C interrupts
Type "explain quantum computing in extreme detail"
Enter
Sleep 4s
Ctrl+C
Sleep 3s
Screenshot tests/ui/screenshots/w1-355-case3-interrupt.png

# CASE 4: After interrupt, type text then Ctrl-C (should clear, NOT exit)
Type "more text"
Sleep 1s
Ctrl+C
Sleep 2s
Screenshot tests/ui/screenshots/w1-355-case4-clear-after-interrupt.png

# CASE 5: Empty prompt, first Ctrl-C → hint
Ctrl+C
Sleep 2s
Screenshot tests/ui/screenshots/w1-355-case5-hint.png

# CASE 6: Wait 3 seconds (timer expires), Ctrl-C again → new hint (not exit)
Sleep 3s
Ctrl+C
Sleep 2s
Screenshot tests/ui/screenshots/w1-355-case6-timer-reset.png

# CASE 7: Quick double Ctrl-C → exit
Ctrl+C
Sleep 3s
Screenshot tests/ui/screenshots/w1-355-case7-exit.png
