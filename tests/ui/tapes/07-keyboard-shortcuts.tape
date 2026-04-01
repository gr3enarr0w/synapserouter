# UAT 07: Keyboard shortcuts
# Verify: Ctrl-L/P/T/E/U all work
# Launch dir: project dir

Output tests/ui/recordings/07-keyboard-shortcuts.gif
Set Width 1200
Set Height 600

Type "cd /Users/ceverson/Development/synapserouter && ./synroute code"
Enter
Sleep 5s

# Ctrl-L: clear screen
Ctrl+L
Sleep 2s
Screenshot tests/ui/screenshots/07-ctrl-l.png

# Ctrl-P: toggle profile
Ctrl+P
Sleep 3s
Screenshot tests/ui/screenshots/07-ctrl-p.png

# Ctrl-T: toggle tier
Ctrl+T
Sleep 3s
Screenshot tests/ui/screenshots/07-ctrl-t.png

# Ctrl-E: toggle escalation
Ctrl+E
Sleep 3s
Screenshot tests/ui/screenshots/07-ctrl-e.png

# Ctrl-U: clear line
Type "some text to clear"
Ctrl+U
Sleep 2s
Screenshot tests/ui/screenshots/07-ctrl-u.png

Type "/exit"
Enter
Sleep 2s
