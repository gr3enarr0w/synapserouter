# UAT: File format context awareness - notebook detection
# Verify: Detects .ipynb files and adds notebook guidance to system prompt
# Run for: personal profile, NO_COLOR

Output tests/ui/recordings/notebook-detection.gif
Set Width 1200
Set Height 600

# Create a test directory with notebook files
Type "mkdir -p /tmp/notebook-test"
Enter
Sleep 0.5s

Type "cd /tmp/notebook-test"
Enter
Sleep 0.5s

# Create a sample .ipynb file using cat with heredoc
Type "cat > analysis.ipynb << 'EOF'"
Enter
Sleep 0.3s
Type "{"cells": [], "metadata": {}, "nbformat": 4, "nbformat_minor": 5}"
Enter
Sleep 0.3s
Type "EOF"
Enter
Sleep 0.5s

# Verify only .ipynb exists
Type "ls -la"
Enter
Sleep 1s
Screenshot tests/ui/screenshots/notebook-test-dir.png

# Run synroute chat - should detect notebook project
Type "NO_COLOR=1 /Users/ceverson/Development/synapserouter/synroute chat"
Enter
Sleep 3s

# Type a simple question to trigger system prompt generation
Type "What tools do I have?"
Enter
Sleep 2s
Screenshot tests/ui/screenshots/notebook-detection-prompt.png

# Exit with Ctrl+C
Ctrl+C
Sleep 1s

# Cleanup
Type "cd /Users/ceverson/Development/synapserouter"
Enter
Sleep 0.5s
Type "rm -rf /tmp/notebook-test"
Enter
Sleep 0.5s
