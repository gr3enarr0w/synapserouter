#!/bin/bash
# UI test runner for synroute
# Launches synroute in a real Terminal, types commands via osascript,
# captures screenshots at each step.
#
# Usage: ./run_test.sh <test_name> <command1> [command2] [command3] ...
# Example: ./run_test.sh greeting "hello" "/exit"
#
# Screenshots saved to: /tmp/synroute-ui/<test_name>/01-launch.png, 02-cmd1.png, etc.
# Exit: always sends /exit at the end if not already sent

set -e

TEST_NAME="${1:?Usage: $0 <test_name> <cmd1> [cmd2] ...}"
shift
COMMANDS=("$@")

PROJ_DIR="/Users/ceverson/Development/synapserouter"
CAPTURE_DIR="/tmp/synroute-ui/$TEST_NAME"
rm -rf "$CAPTURE_DIR"
mkdir -p "$CAPTURE_DIR"

echo "=== UI Test: $TEST_NAME ==="
echo "Commands: ${COMMANDS[*]}"
echo "Captures: $CAPTURE_DIR"

# Launch synroute in a new Terminal window
osascript <<EOF
tell application "Terminal"
    do script "cd $PROJ_DIR && clear && ./synroute code"
    activate
end tell
EOF

echo "Waiting for synroute to start..."
sleep 8

# Capture launch state
screencapture -x "$CAPTURE_DIR/01-launch.png"
echo "  [1] Captured launch"

# Run each command
STEP=2
SENT_EXIT=false
for CMD in "${COMMANDS[@]}"; do
    echo "  Typing: $CMD"

    # Type the command character by character via System Events
    osascript <<EOF
tell application "Terminal" to activate
delay 0.5
tell application "System Events"
    tell process "Terminal"
        keystroke "$CMD"
        delay 0.3
        keystroke return
    end tell
end tell
EOF

    if [ "$CMD" = "/exit" ]; then
        SENT_EXIT=true
        sleep 3
    else
        # Wait for response
        sleep 15
    fi

    # Capture
    PADDED=$(printf "%02d" $STEP)
    screencapture -x "$CAPTURE_DIR/${PADDED}-${CMD// /-}.png"
    echo "  [$STEP] Captured after: $CMD"
    STEP=$((STEP + 1))
done

# Send /exit if we didn't already
if [ "$SENT_EXIT" = "false" ]; then
    osascript <<EOF
tell application "Terminal" to activate
delay 0.5
tell application "System Events"
    tell process "Terminal"
        keystroke "/exit"
        delay 0.3
        keystroke return
    end tell
end tell
EOF
    sleep 3
    PADDED=$(printf "%02d" $STEP)
    screencapture -x "$CAPTURE_DIR/${PADDED}-exit.png"
    echo "  [$STEP] Captured exit"
fi

echo "=== Done: $TEST_NAME ==="
echo "Screenshots:"
ls -la "$CAPTURE_DIR/"
