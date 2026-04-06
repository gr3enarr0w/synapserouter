# UAT: v1.12 file format awareness for notebooks
# Verify: System prompt includes notebook guidance when .ipynb files detected
# Run for: personal profile, NO_COLOR

Output tests/ui/recordings/v112-file-format-awareness.gif
Set Width 1200
Set Height 600

# Create a test directory with notebook files
Type "mkdir -p /tmp/notebook-aware-test"
Enter
Sleep 0.5s

Type "cd /tmp/notebook-aware-test"
Enter
Sleep 0.5s

# Create a sample .ipynb file
Type "cat > notebook.ipynb << 'EOF'"
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

# Ask about available tools to trigger system prompt
Type "What can you help me with?"
Enter
Sleep 3s
Screenshot tests/ui/screenshots/v112-file-format-awareness.png

# Exit
Ctrl+C
Sleep 1s

# Cleanup
Type "cd /Users/ceverson/Development/synapserouter"
Enter
Sleep 0.5s
Type "rm -rf /tmp/notebook-aware-test"
Enter
Sleep 0.5s
