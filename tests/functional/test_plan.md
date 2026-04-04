# Synroute Functional Test Plan

Every feature tested as a user would use it. Pass = works as expected. Fail = bug to fix.

## 1. CLI Modes

| # | Test | Command | Expected | Status |
|---|------|---------|----------|--------|
| 1.1 | Default launches code mode | `./synroute` | Code mode TUI with banner | |
| 1.2 | Explicit code mode | `./synroute code` | Same as 1.1 | |
| 1.3 | Chat mode | `./synroute chat` | REPL with synroute> prompt | |
| 1.4 | One-shot message | `./synroute chat --message "hi"` | Prints response, exits | |
| 1.5 | One-shot clean output | `./synroute chat --message "say hello"` | ONLY the response, zero logs | |
| 1.6 | Serve mode | `./synroute serve` | HTTP server starts | |
| 1.7 | MCP serve | `./synroute mcp-serve` | MCP server starts | |
| 1.8 | Version | `./synroute version` | Version + logo displayed | |
| 1.9 | Help | `./synroute --help` | Usage info | |

## 2. Slash Commands (in REPL)

| # | Test | Command | Expected | Status |
|---|------|---------|----------|--------|
| 2.1 | Exit | `/exit` | Exits cleanly | |
| 2.2 | Clear | `/clear` | Clears conversation | |
| 2.3 | Model | `/model` | Shows current model | |
| 2.4 | Tools | `/tools` | Lists all tools | |
| 2.5 | History | `/history` | Shows conversation | |
| 2.6 | Agents | `/agents` | Shows sub-agents | |
| 2.7 | Budget | `/budget` | Shows budget status | |
| 2.8 | Help | `/help` | Shows all commands | |
| 2.9 | Plan | `/plan fix the auth` | Plans without coding | |
| 2.10 | Review | `/review` | Reviews current code | |
| 2.11 | Check | `/check` | Checks against criteria | |
| 2.12 | Fix | `/fix the bug` | Targeted fix | |
| 2.13 | Research quick | `/research quick Go error handling` | Quick research, free backends | |
| 2.14 | Research standard | `/research standard Python async` | Standard research | |
| 2.15 | Research deep | `/research deep AI agent UX` | Deep research | |

## 3. Slash Commands in --message Mode

| # | Test | Command | Expected | Status |
|---|------|---------|----------|--------|
| 3.1 | Research in message | `--message "/research quick Go"` | Runs research pipeline | |
| 3.2 | Plan in message | `--message "/plan fix tests"` | Plans without coding | |
| 3.3 | Review in message | `--message "/review"` | Reviews code | |

## 4. Tool Execution

| # | Test | Prompt | Expected | Status |
|---|------|--------|----------|--------|
| 4.1 | Bash basic | "Run ls in current dir" | Lists files | |
| 4.2 | Bash with env | "Source .env and echo vars" | Sources inline, no loop | |
| 4.3 | File read | "Read main.go" | Shows file contents | |
| 4.4 | File write | "Create /tmp/test.txt with hello" | Creates file | |
| 4.5 | File edit | "Change X to Y in file" | Edits in place | |
| 4.6 | Grep | "Search for 'func main' in this project" | Finds matches | |
| 4.7 | Glob | "Find all .go files" | Lists files | |
| 4.8 | Git | "Show git status" | Shows status | |
| 4.9 | Web search | "Search for Go error handling" | Returns results, max 3 backends | |
| 4.10 | Web fetch | "Fetch https://httpbin.org/get" | Returns content | |
| 4.11 | Recall | "What did I ask earlier" | Retrieves from memory | |

## 5. Intent Detection

| # | Test | Prompt | Expected | Status |
|---|------|--------|----------|--------|
| 5.1 | Chat intent | "What is Go?" | Direct answer, no tools | |
| 5.2 | Code intent | "Write a function" | Uses file_write | |
| 5.3 | Fix intent | "Fix the bug in main.go" | Reads then edits | |
| 5.4 | Explain intent | "Explain this codebase" | Reads files, explains in text | |
| 5.5 | Research intent | "Research best practices for X" | Uses web_search | |
| 5.6 | Review intent | "Review my code" | Reads files, gives feedback | |

## 6. Provider/Routing

| # | Test | Command | Expected | Status |
|---|------|---------|----------|--------|
| 6.1 | Provider test | `./synroute test` | All providers pass | |
| 6.2 | Single provider | `./synroute test --provider ollama-chain-1` | One provider tested | |
| 6.3 | Doctor | `./synroute doctor` | Diagnostics pass | |
| 6.4 | Models list | `./synroute models` | Lists all models | |
| 6.5 | Config show | `./synroute config show` | Shows tier config | |
| 6.6 | Profile show | `./synroute profile show` | Shows active profile | |
| 6.7 | Profile switch | `./synroute profile switch work` | Switches profile | |
| 6.8 | Recommend | `./synroute recommend` | Shows recommendations | |

## 7. Session Management

| # | Test | Command | Expected | Status |
|---|------|---------|----------|--------|
| 7.1 | Resume | `./synroute chat --resume` | Resumes last session | |
| 7.2 | Session ID | `./synroute chat --session <id>` | Resumes specific | |
| 7.3 | Worktree | `./synroute chat --worktree` | Creates worktree | |
| 7.4 | Background | `./synroute chat --background --message "fix"` | Returns task ID | |
| 7.5 | Tasks list | `./synroute tasks` | Shows background tasks | |

## 8. Search Fusion

| # | Test | Expected | Status |
|---|------|----------|--------|
| 8.1 | Regular search caps at 3 backends | Log shows "using 3/N backends" | |
| 8.2 | Free backends prioritized | DuckDuckGo/OpenAlex selected first | |
| 8.3 | Circuit breaker skips failed backends | Failed backend not reselected | |
| 8.4 | Search stats command | `./synroute search stats` shows metrics | |

## 9. Code Quality Safeguards

| # | Test | Expected | Status |
|---|------|----------|--------|
| 9.1 | Go module path in prompt | System prompt includes go.mod path | |
| 9.2 | Build verification after .go write | go build runs automatically | |
| 9.3 | goimports runs after .go write | Missing imports auto-fixed | |
| 9.4 | Per-language verify (.py) | py_compile runs after .py write | |
| 9.5 | Per-language verify (.js) | node --check runs after .js write | |
| 9.6 | Cleanup after session | RunCleanup called | |
| 9.7 | No junk files created | No .sql, .bak, temp dirs | |

## 10. Eval Framework

| # | Test | Command | Expected | Status |
|---|------|---------|----------|--------|
| 10.1 | List exercises | `./synroute eval exercises --language go` | Lists exercises | |
| 10.2 | Run eval | `./synroute eval run --language go --count 1` | Runs 1 exercise | |
| 10.3 | Results | `./synroute eval results` | Shows results | |

## 11. Edge Cases

| # | Test | Expected | Status |
|---|------|----------|--------|
| 11.1 | Empty message | `--message ""` | Error or prompt | |
| 11.2 | Very long message | 10KB input | Handles without crash | |
| 11.3 | Special chars | `--message "hello 'world' \"test\""` | Handles quotes | |
| 11.4 | NO_COLOR | `NO_COLOR=1 ./synroute version` | No ANSI codes | |
| 11.5 | Narrow terminal | 60 cols | Degrades gracefully | |
| 11.6 | Parallel runs | Two synroute processes | Auto-worktree | |
| 11.7 | Ctrl+C interrupt | Mid-response | Clean exit | |

## 12. Work Profile

| # | Test | Expected | Status |
|---|------|----------|--------|
| 12.1 | Simple query | "Say hello" | Works on Vertex | |
| 12.2 | Tool use | "Read main.go" | Tools work on Vertex | |
| 12.3 | Bash with env | "Source .env and curl" | No env var loop | |
| 12.4 | Complex task | Multi-step API task | Completes without loop | |
