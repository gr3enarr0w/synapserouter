---
name: vhs-verify
description: "Terminal UI verification via VHS recordings — visual regression testing for TUI changes."
triggers:
  - "banner"
  - "logo"
  - "terminal"
  - "tui"
  - "renderer"
  - "coderenderer"
  - "coderepl"
  - "prompt"
  - "ansi"
  - "color"
  - "NO_COLOR"
  - "keyboard"
  - "shortcut"
  - "Ctrl-"
  - "verbosity"
  - "status bar"
  - "cli"
  - "command line"
  - "interactive"
  - "repl"
  - "console"
  - "stdin"
  - "stdout"
  - "user interface"
  - "display"
  - "output format"
role: verifier
phase: verify
verify:
  - name: "vhs installed"
    command: "which vhs || echo 'MISSING'"
    expect_not: "MISSING"
  - name: "greeting tape"
    command: "vhs tests/ui/tapes/01-greeting.tape 2>&1 && test -f tests/ui/screenshots/01-launch.png && echo 'OK' || echo 'FAIL'"
    expect: "OK"
  - name: "screenshots captured"
    command: "ls tests/ui/screenshots/*.png 2>/dev/null | wc -l | tr -d ' '"
    expect_not: "0"
---
# vhs-verify

## Purpose
Verify terminal UI and CLI tool output visually using VHS (Charmbracelet) terminal recordings.
VHS is the ONLY sign-off gate for any change that affects what users see in the terminal.
This applies to synroute's own TUI AND any CLI tool the agent builds for the user.

## When to Use
- After ANY change to synroute's terminal code (coderenderer, coderepl, terminal.go)
- After building ANY CLI tool, REPL, or interactive program
- After modifying output formatting, colors, or layout
- When the user asks to build a terminal application
- When verifying that a built tool works as expected from the user's perspective

## How to Verify

### 1. Write a VHS tape
Create a `.tape` file in `tests/ui/tapes/` that exercises the changed behavior:

```tape
Output tests/ui/recordings/<name>.gif
Set Shell "bash"
Set FontSize 14
Set Width 1300
Set Height 600
Set TypingSpeed 50ms

Type "cd /path/to/project && ./synroute code"
Enter
Sleep 8s
Screenshot tests/ui/screenshots/<name>-launch.png

Type "<test input>"
Enter
Sleep 15s
Screenshot tests/ui/screenshots/<name>-result.png

Type "/exit"
Enter
Sleep 3s
```

### 2. Run the tape
```bash
vhs tests/ui/tapes/<name>.tape
```

### 3. Verify screenshots
Read each screenshot with `file_read` tool and verify:
- Banner displays correctly (logo, profile name, tier count)
- Response text is visible (not empty, not garbage)
- Colors render properly (or absent with NO_COLOR)
- Keyboard shortcuts produce expected output
- Prompt returns after each response
- /exit shows "bye"

### 4. Test both profiles
Run tapes with both `ACTIVE_PROFILE=personal` and `ACTIVE_PROFILE=work`.

### 5. Test NO_COLOR mode
Run with `NO_COLOR=1` and verify zero ANSI escape codes.

## Standard Tapes
These tapes should pass after any UI change:
- `01-greeting.tape` — launch, hello, /help, /exit
- `02-multi-turn.tape` — 3 turns, REPL stays alive
- `21-keyboard-shortcuts-v2.tape` — Ctrl-L/P/T/E/U
- `39-logo-verify.tape` — logo on personal, work, NO_COLOR

## Key Rules
- Go tests passing != done. VHS looking correct = done.
- Never declare a UI change complete without a VHS screenshot that proves it.
- Always test both profiles (personal + work).
- Always test NO_COLOR mode.
