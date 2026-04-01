# Comprehensive keyboard shortcut test — ALL shortcuts
# Tests each shortcut in realistic user context

Output tests/ui/recordings/keyboard-all.gif
Set Width 1200
Set Height 800

Type "cd /Users/ceverson/Development/synapserouter && ./synroute code"
Enter
Sleep 5s
Screenshot tests/ui/screenshots/kb-00-launch.png

# === Ctrl-D: Exit on empty line ===
# (Don't actually exit — test with text first to verify it's ignored)
Type "some text"
Ctrl+D
Sleep 2s
Screenshot tests/ui/screenshots/kb-01-ctrld-with-text.png

# Clear line first
Ctrl+C
Sleep 1s

# === Ctrl-L: Cycle verbosity (normal→verbose→compact→normal) ===
Ctrl+L
Sleep 2s
Screenshot tests/ui/screenshots/kb-02-ctrl-l-verbose.png

Ctrl+L
Sleep 2s
Screenshot tests/ui/screenshots/kb-03-ctrl-l-compact.png

Ctrl+L
Sleep 2s
Screenshot tests/ui/screenshots/kb-04-ctrl-l-normal.png

# === Ctrl-U: Clear line ===
Type "text I want to erase with ctrl-u"
Sleep 1s
Screenshot tests/ui/screenshots/kb-05-before-ctrl-u.png

Ctrl+U
Sleep 2s
Screenshot tests/ui/screenshots/kb-06-after-ctrl-u.png

# === Ctrl-P: Pipeline status ===
Ctrl+P
Sleep 3s
Screenshot tests/ui/screenshots/kb-07-ctrl-p-pipeline.png

# === Ctrl-T: Recent tools ===
Ctrl+T
Sleep 3s
Screenshot tests/ui/screenshots/kb-08-ctrl-t-tools.png

# === Ctrl-E: Force escalation ===
Ctrl+E
Sleep 3s
Screenshot tests/ui/screenshots/kb-09-ctrl-e-escalate.png

# === Now have a conversation, then re-test shortcuts ===
Type "hello"
Enter
Sleep 10s
Screenshot tests/ui/screenshots/kb-10-after-hello.png

# Tools should now show something after a conversation
Ctrl+T
Sleep 3s
Screenshot tests/ui/screenshots/kb-11-ctrl-t-after-convo.png

# Pipeline status after conversation
Ctrl+P
Sleep 3s
Screenshot tests/ui/screenshots/kb-12-ctrl-p-after-convo.png

# === Ctrl-/: Help ===
# Note: Ctrl-/ sends 0x1F
Type "q"
Backspace
Sleep 1s

# Exit cleanly
Type "/exit"
Enter
Sleep 3s
Screenshot tests/ui/screenshots/kb-13-exit.png
