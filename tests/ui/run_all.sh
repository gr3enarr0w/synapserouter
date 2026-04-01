#!/bin/bash
# Run all VHS UI tests sequentially — stop on first failure
# Usage: ./tests/ui/run_all.sh [--continue-on-fail]
#
# Results:
#   tests/ui/screenshots/*.png  — individual screenshots
#   tests/ui/recordings/*.gif   — full session recordings
#   tests/ui/results.txt        — pass/fail summary

set -e

PROJ_DIR="/Users/ceverson/Development/synapserouter"
cd "$PROJ_DIR"

STOP_ON_FAIL=true
if [[ "$1" == "--continue-on-fail" ]]; then
    STOP_ON_FAIL=false
fi

echo "=== SynRoute UAT v1.03 ==="
echo "Date: $(date)"
echo "Binary: $(./synroute version 2>&1 | head -1)"
echo "Mode: $(if $STOP_ON_FAIL; then echo 'stop on first failure'; else echo 'continue on fail'; fi)"
echo ""

# Clean previous results
rm -rf tests/ui/screenshots/*.png tests/ui/recordings/*.gif
mkdir -p tests/ui/screenshots tests/ui/recordings

PASS=0
FAIL=0
TOTAL=0
RESULTS=""

for tape in tests/ui/tapes/*.tape; do
    name=$(basename "$tape" .tape)
    TOTAL=$((TOTAL + 1))
    echo "[$TOTAL/$(ls tests/ui/tapes/*.tape | wc -l | tr -d ' ')] Running: $name"

    if vhs "$tape" 2>&1 | tail -3; then
        echo "  ✓ PASS: $name"
        RESULTS="${RESULTS}PASS: $name\n"
        PASS=$((PASS + 1))

        # List screenshots produced by this tape
        echo "  Screenshots:"
        ls tests/ui/screenshots/${name}*.png 2>/dev/null | while read f; do
            echo "    → $(basename $f)"
        done
    else
        echo "  ✗ FAIL: $name"
        RESULTS="${RESULTS}FAIL: $name\n"
        FAIL=$((FAIL + 1))

        if $STOP_ON_FAIL; then
            echo ""
            echo "=== STOPPED: $name failed ==="
            echo "Fix the issue, then re-run. Screenshots so far:"
            ls tests/ui/screenshots/*.png 2>/dev/null
            echo -e "\nPass: $PASS  Fail: $FAIL  Remaining: $(($(ls tests/ui/tapes/*.tape | wc -l | tr -d ' ') - TOTAL))"
            echo -e "Date: $(date)\n\n$RESULTS\nPass: $PASS  Fail: $FAIL  Stopped at: $name" > tests/ui/results.txt
            exit 1
        fi
    fi
    echo ""
done

echo "=== Results ==="
echo -e "$RESULTS"
echo "Pass: $PASS  Fail: $FAIL  Total: $TOTAL"
echo ""
echo "Screenshots:"
ls -la tests/ui/screenshots/
echo ""

# Save results
echo -e "Date: $(date)\n\n$RESULTS\nPass: $PASS  Fail: $FAIL  Total: $TOTAL" > tests/ui/results.txt
