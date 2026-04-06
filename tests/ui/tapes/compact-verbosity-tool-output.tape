# UAT: Compact verbosity (Ctrl-L level 0) collapses tool output to single-line summaries
# Verify: file_read shows "[file_read] path (N lines) ✓", bash shows "[bash] cmd (duration) ✓", etc.
# Run for: personal, work, NO_COLOR

Output tests/ui/recordings/compact-verbosity-tool-output.gif
Set Width 1200
Set Height 600

# Launch in code mode
Type "cd ~ && /Users/ceverson/Development/synapserouter/synroute code"
Enter
Sleep 5s
Screenshot tests/ui/screenshots/compact-01-launch.png

# Cycle to compact mode (Ctrl-L three times: Normal->Verbose->Compact)
Type "Ctrl-L"
Sleep 1s
Type "Ctrl-L"
Sleep 1s
Type "Ctrl-L"
Sleep 1s
Screenshot tests/ui/screenshots/compact-02-mode-set.png

# Trigger a file_read tool
Type "read the package.json file"
Enter
Sleep 3s
Screenshot tests/ui/screenshots/compact-03-file-read.png

# Trigger a bash tool
Type "run ls -la"
Enter
Sleep 3s
Screenshot tests/ui/screenshots/compact-04-bash.png

# Exit
Type "/exit"
Enter
Sleep 2s
