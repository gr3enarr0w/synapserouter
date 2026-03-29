---
name: terminal-interaction
description: "Terminal command execution and interaction via MCP — run commands, capture output, interact with terminal applications."
triggers:
  - "terminal"
  - "run command"
  - "execute"
  - "shell"
  - "bash command"
  - "cli"
  - "command line"
  - "tui"
  - "terminal ui"
role: "tool-operator"
phase: "implement"
mcp_tools:
  - "terminal.execute_command"
  - "terminal.read_output"
language: ""
---

# Terminal Interaction Skill

Execute terminal commands and interact with terminal applications via the terminal MCP server.

## When to Use
- Running build commands (go build, cargo build, npm run)
- Executing test suites (go test, pytest, cargo test)
- Interacting with CLI tools
- Capturing terminal output for analysis
- Testing TUI applications

## MCP Tools
- `terminal.execute_command` — Run a shell command and return output
- `terminal.read_output` — Read output from a running command

## Patterns

### Run and verify
1. Execute the build command
2. Check exit code
3. Parse output for errors
4. Report results

### Interactive testing
1. Start the application
2. Send input
3. Capture output
4. Verify expected behavior

## Spec Compliance
This skill is triggered by the skill system and calls MCP tools via the MCP client.
Skills are the trigger layer for MCPs (Design Principle #11).
The agent never calls MCP tools directly — this skill mediates access.
