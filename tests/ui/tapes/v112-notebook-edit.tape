# UAT: v1.12 notebook_edit tool availability
# Verify: notebook_edit tool appears in tool list when .ipynb files present
# Run for: personal profile, NO_COLOR

Output tests/ui/recordings/v112-notebook-edit.gif
Set Width 1200
Set Height 600

# Create a test directory with notebook files
Type "mkdir -p /tmp/notebook-edit-test"
Enter
Sleep 0.5s

Type "cd /tmp/notebook-edit-test"
Enter
Sleep 0.5s

# Create a sample .ipynb file
Type "cat > test.ipynb << 'EOF'"
Enter
Sleep 0.3s
Type "{"cells": [], "metadata": {}, "nbformat": 4, "nbformat_minor": 5}"
Enter
Sleep 0.3s
Type "EOF"
Enter
Sleep 0.5s

# Run synroute chat
Type "NO_COLOR=1 /Users/ceverson/Development/synapserouter/synroute chat"
Enter
Sleep 3s

# Check tools list
Type "/tools"
Enter
Sleep 2s
Screenshot tests/ui/screenshots/v112-notebook-edit-tools.png

# Exit
Ctrl+C
Sleep 1s

# Cleanup
Type "cd /Users/ceverson/Development/synapserouter"
Enter
Sleep 0.5s
Type "rm -rf /tmp/notebook-edit-test"
Enter
Sleep 0.5s
