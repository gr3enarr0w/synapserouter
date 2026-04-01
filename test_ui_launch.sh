#!/bin/bash
# Launch synroute in a real Terminal window and capture screenshots
# Usage: ./test_ui_launch.sh

PROJ_DIR="/Users/ceverson/Development/synapserouter"
CAPTURE_DIR="/tmp/synroute-ui-captures"
mkdir -p "$CAPTURE_DIR"

# Launch in a new Terminal window with correct working directory
osascript -e "tell application \"Terminal\" to do script \"cd $PROJ_DIR && clear && ./synroute code\""
osascript -e 'tell application "Terminal" to activate'

echo "Waiting for synroute to start..."
sleep 8

# Capture the screen
screencapture -x "$CAPTURE_DIR/01-launch.png"
echo "Captured: $CAPTURE_DIR/01-launch.png"

# Type hello
osascript -e 'tell application "System Events" to tell process "Terminal" to keystroke "hello"'
osascript -e 'tell application "System Events" to tell process "Terminal" to keystroke return'

echo "Waiting for response..."
sleep 15

screencapture -x "$CAPTURE_DIR/02-hello-response.png"
echo "Captured: $CAPTURE_DIR/02-hello-response.png"

# Type /exit
osascript -e 'tell application "System Events" to tell process "Terminal" to keystroke "/exit"'
osascript -e 'tell application "System Events" to tell process "Terminal" to keystroke return'

sleep 3
screencapture -x "$CAPTURE_DIR/03-exit.png"
echo "Captured: $CAPTURE_DIR/03-exit.png"

echo "Done. Screenshots in: $CAPTURE_DIR/"
ls -la "$CAPTURE_DIR/"
