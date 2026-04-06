# v1.12 Acceptance Test: MLII Homework Scenario
# Reproduces the exact user test that started v1.12 development.
# Clean folder with 4 files: 2 notebooks (empty cells) + 2 CSVs.
# User asks "can you review the files in this folder?"
#
# Expected v1.12 behavior:
# 1. Detects notebook project (formatContext)
# 2. Reads files (no permission needed for reads)
# 3. Shows permission prompt before first write
# 4. Uses notebook_edit (not file_write/.py files)
# 5. Shows compact tool output
# 6. Shows completion summary

Output tests/ui/recordings/v112-mlii-acceptance.gif
Set Width 1200
Set Height 800
Set TypingSpeed 30ms

# Launch synroute in the clean MLII test directory
Type "cd /tmp/mlii-v112-test && NO_COLOR=1 /Users/ceverson/Development/synapserouter/synroute code"
Enter
Sleep 5s

# Same prompt the user originally used
Type "can you review the files in this folder?"
Enter
Sleep 15s

# If permission prompt appears, approve with "a" (always for this category)
Type "a"
Sleep 600s

# Exit
Type "/exit"
Enter
Sleep 2s

# Check what files exist — should NOT have .py files
Type "ls /tmp/mlii-v112-test/"
Enter
Sleep 1s
