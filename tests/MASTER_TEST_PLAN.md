# Synroute Master Test Plan
# Unified test suite — adversarial (does it crash?) + functional (does it work?)
# Every test has TWO assertions: 1) no crash 2) correct output
# Total: ~530 tests across 25 categories

## How to Run
```bash
# All automated tests (mock provider, ~10 min):
bash tests/run_master_tests.sh

# Real LLM tests (real providers, ~30 min):
bash tests/run_master_tests.sh --real
```

---

## Cat 1: Input Handling (33 tests)
Tests: every input type produces a response without crash AND the response is relevant

- [ ] 1.01 Empty message → exits clean OR prompts for input
- [ ] 1.02 Whitespace only → exits clean OR prompts for input
- [ ] 1.03 Single char "a" → responds with text
- [ ] 1.04 Single emoji 🎉 → responds (doesn't strip emoji)
- [ ] 1.05 1000 chars no spaces → responds (doesn't truncate silently)
- [ ] 1.06 1000 chars with spaces → responds
- [ ] 1.07 100 lines of code → responds (code attached or processed)
- [ ] 1.08 10000 lines → responds (truncation is OK, crash is not)
- [ ] 1.09 Null bytes \x00 → responds (null stripped or handled)
- [ ] 1.10 Zalgo text → responds (combining chars don't break rendering)
- [ ] 1.11 RTL Arabic → responds
- [ ] 1.12 Mixed LTR+RTL → responds
- [ ] 1.13 Base64 data → responds
- [ ] 1.14 JSON object → responds (not parsed as config)
- [ ] 1.15 HTML tags <script> → responds (not executed)
- [ ] 1.16 SQL injection → responds (not executed)
- [ ] 1.17 Backticks → responds (code formatting preserved)
- [ ] 1.18 Triple backtick fence → responds
- [ ] 1.19 Punctuation only → responds
- [ ] 1.20 Backslash sequences → responds
- [ ] 1.21 Unicode snowman ☃ → responds
- [ ] 1.22 Zero-width chars → responds
- [ ] 1.23 ASCII art multi-line → responds
- [ ] 1.24 Tab character → responds
- [ ] 1.25 Carriage return → responds
- [ ] 1.26 Word "exit" in message → responds (doesn't exit)
- [ ] 1.27 Word "quit" in message → responds (doesn't quit)
- [ ] 1.28 "/exit" embedded in text → responds (doesn't exit)
- [ ] 1.29 /notacommand → error message "unknown command"
- [ ] 1.30 Just "/" → responds or shows help
- [ ] 1.31 Just "." → responds
- [ ] 1.32 Just "?" → responds
- [ ] 1.33 Windows CRLF line endings → responds

## Cat 2: Keyboard/Signals (29 tests)
- [ ] 2.01 Ctrl+C during idle → exits clean (exit 0 or 130)
- [ ] 2.02 Ctrl+C during response → stops streaming, returns to prompt
- [ ] 2.03 Ctrl+C during tool execution → cancels tool, returns to prompt
- [ ] 2.04 Double Ctrl+C → exits
- [ ] 2.05 Triple Ctrl+C → exits (no hang)
- [ ] 2.06 Ctrl+D (EOF) → exits clean
- [ ] 2.07 Ctrl+Z → suspends (fg resumes)
- [ ] 2.08 Ctrl+L → clears screen
- [ ] 2.09 Ctrl+A/E → cursor moves to beginning/end
- [ ] 2.10 Ctrl+K → kills to end of line
- [ ] 2.11 Ctrl+U → kills entire line
- [ ] 2.12 Ctrl+W → deletes word
- [ ] 2.13 Escape → no crash
- [ ] 2.14 Rapid Escape (5x) → no crash
- [ ] 2.15 Arrow keys (all 4) → no crash, cursor moves
- [ ] 2.16 Home/End → cursor moves
- [ ] 2.17 PgUp/PgDn → no crash
- [ ] 2.18 Delete key → deletes char
- [ ] 2.19 Backspace on empty → no crash
- [ ] 2.20 F1-F12 → no crash
- [ ] 2.21 Tab → no crash (may show completion)
- [ ] 2.22 Shift+Tab → no crash
- [ ] 2.23 Alt+Enter → no crash
- [ ] 2.24 Key repeat flood (40 chars) → no crash, chars appear
- [ ] 2.25 Type during response → input buffered
- [ ] 2.26 Type immediately after response → input accepted
- [ ] 2.27 Rapid Enter (10x) → no crash, empty prompts handled
- [ ] 2.28 All modifier combos → no crash
- [ ] 2.29 SIGTERM → exits clean
- [ ] 2.30 SIGHUP → exits clean

## Cat 3: Clipboard/Paste (16 tests)
- [ ] 3.01 Paste normal text → text appears in input
- [ ] 3.02 Paste 10 lines → all lines in message
- [ ] 3.03 Paste 10000 chars → handled (may truncate, no crash)
- [ ] 3.04 Paste HTML → treated as text, not rendered
- [ ] 3.05 Paste from code editor → preserves indentation
- [ ] 3.06 Paste file path → path in message
- [ ] 3.07 Drag file from Finder → path appears (GUI only)
- [ ] 3.08 Drag folder → path appears (GUI only)
- [ ] 3.09 Drag multiple files → paths appear (GUI only)
- [ ] 3.10 Paste binary data → handled (shows base64 or error)
- [ ] 3.11 Paste ANSI escapes → escapes stripped or shown as text
- [ ] 3.12 Paste null bytes → nulls stripped
- [ ] 3.13 Paste URL → URL in message
- [ ] 3.14 Paste .env secrets → secrets in message (redacted in storage)
- [ ] 3.15 Windows \r\n line endings → handled
- [ ] 3.16 Cmd+V vs right-click → both work (GUI only)

## Cat 4: Terminal Window (22 tests)
- [ ] 4.01 Resize wider at prompt → layout adjusts
- [ ] 4.02 Resize narrower at prompt → layout adjusts
- [ ] 4.03 Resize during response → no crash, output continues
- [ ] 4.04 Resize during tool execution → no crash
- [ ] 4.05 20 cols → renders (may be ugly, no crash)
- [ ] 4.06 10 cols → renders (may be ugly, no crash)
- [ ] 4.07 200 rows → renders
- [ ] 4.08 5 rows → renders
- [ ] 4.09 Fullscreen toggle → no crash
- [ ] 4.10 Split pane (tmux) → both panes work
- [ ] 4.11 New tab, switch back → session intact
- [ ] 4.12 Desktop switch and back → session intact
- [ ] 4.13 Minimize/restore → session intact
- [ ] 4.14 Sleep/wake → session continues or exits clean
- [ ] 4.15 Light terminal theme → all text readable
- [ ] 4.16 Dark terminal theme → all text readable
- [ ] 4.17 Solarized theme → all text readable
- [ ] 4.18 High contrast theme → all text readable
- [ ] 4.19 Font 8pt/12pt/24pt/48pt → layout doesn't break
- [ ] 4.20 Font with ligatures → renders correctly
- [ ] 4.21 Terminal zoom in → layout adjusts
- [ ] 4.22 Terminal zoom out → layout adjusts

## Cat 5: Slash Commands — FUNCTIONAL (30 tests)
Each command must produce CORRECT OUTPUT, not just not crash

- [ ] 5.01 /help → prints list of available commands including /exit, /clear, /model
- [ ] 5.02 /exit → exits cleanly, session saved
- [ ] 5.03 /clear → conversation cleared, next prompt starts fresh
- [ ] 5.04 /model → shows current model name
- [ ] 5.05 /model nonexistent → error message about model not found
- [ ] 5.06 /tools → lists ALL registered tools (bash, file_read, file_write, file_edit, grep, glob, git, web_search, web_fetch, recall)
- [ ] 5.07 /history → shows conversation history OR "no history"
- [ ] 5.08 /history empty → says "no history" or similar
- [ ] 5.09 /agents → shows sub-agent count OR "no agents"
- [ ] 5.10 /budget → shows budget status OR "no budget set"
- [ ] 5.11 /plan → starts planning (asks for description or plans current state)
- [ ] 5.12 /plan <desc> → creates plan with acceptance criteria
- [ ] 5.13 /review → reviews current code changes
- [ ] 5.14 /check → checks against acceptance criteria
- [ ] 5.15 /fix → asks what to fix or starts targeted fix
- [ ] 5.16 /fix <desc> → reads code, diagnoses, fixes
- [ ] 5.17 /research quick <q> → searches with free backends, returns results
- [ ] 5.18 /research standard <q> → searches with 2-3 rounds
- [ ] 5.19 /research deep <q> → deep research with all backends
- [ ] 5.20 /research (no args) → shows usage or asks for query
- [ ] 5.21 /redact list → shows all redaction patterns (email, phone, SSN, API key, etc.)
- [ ] 5.22 /redact test <text> → shows redacted version of input
- [ ] 5.23 /redact add <pattern> → adds custom pattern
- [ ] 5.24 /redact ignore <type> → disables a pattern type
- [ ] 5.25 /intent correct <intent> → saves correction
- [ ] 5.26 /foobar → "unknown command" error message
- [ ] 5.27 /HELP → same as /help (case insensitive)
- [ ] 5.28 /help extra spaces → works (strips extra spaces)
- [ ] 5.29 (space)/help → works (strips leading space)
- [ ] 5.30 /model nonexistent-model → error, doesn't crash

## Cat 6: File References @file — FUNCTIONAL (18 tests)
Must verify content is ACTUALLY ATTACHED, not just no crash

- [ ] 6.01 @main.go → file content attached to message (log shows "attached 1 file")
- [ ] 6.02 @nonexistent.go → error message in attachment ("[error: cannot read file]")
- [ ] 6.03 @/absolute/path → absolute path resolved
- [ ] 6.04 @../traversal → path confined to workdir (no /etc/passwd leak)
- [ ] 6.05 @"file with spaces" → spaces handled
- [ ] 6.06 @/dev/null → empty content or error
- [ ] 6.07 @/dev/random → binary file detected, not read forever
- [ ] 6.08 @directory/ → lists directory files (expandDirReferences)
- [ ] 6.09 @symlink → follows symlink, reads target
- [ ] 6.10 @binary-file → "[binary file]" message
- [ ] 6.11 @very-large-file → truncated at 10KB with "[truncated]" message
- [ ] 6.12 @.env → content attached (secrets in CONTENT, scrubbed in DB storage)
- [ ] 6.13 @~/.ssh/id_rsa → content attached or permission error
- [ ] 6.14 Multiple @files → all attached
- [ ] 6.15 @file mid-sentence → attachment extracted, message cleaned
- [ ] 6.16 @file-being-written → reads what's available
- [ ] 6.17 @circular-symlink → error, no infinite loop
- [ ] 6.18 Image file @logo.png → base64 encoded, marked as image

## Cat 7: Flags & Startup — FUNCTIONAL (36 tests)
Must verify each flag DOES WHAT IT SAYS

- [ ] 7.01 `synroute code` → launches code mode TUI with banner
- [ ] 7.02 `synroute code --message "hi"` → prints response, exits
- [ ] 7.03 `synroute code --message ""` → handles empty (exits or error)
- [ ] 7.04 `--confidential` → web_search and web_fetch BLOCKED (log shows blocked)
- [ ] 7.05 `--dry-run` → file_write/file_edit show diff but DON'T WRITE
- [ ] 7.06 `--screen-reader` → output has ZERO ANSI codes, ZERO spinners, ZERO box drawing
- [ ] 7.07 `--json-events` → events output as JSON
- [ ] 7.08 `--verbose 0` → minimal output
- [ ] 7.09 `--verbose 2` → detailed output with provider info
- [ ] 7.10 `--budget 100` → agent stops after ~100 tokens
- [ ] 7.11 `--budget 0` → runs (0 = unlimited or immediate stop)
- [ ] 7.12 `--budget -1` → handles gracefully
- [ ] 7.13 `--max-agents 0` → no sub-agents spawned
- [ ] 7.14 `--max-agents 1000` → accepts value
- [ ] 7.15 `--model nonexistent` → error "no providers support model" (not silent fallback)
- [ ] 7.16 `--worktree` → creates worktree, runs in it
- [ ] 7.17 `--resume` no session → error message, exit non-zero
- [ ] 7.18 `--session nonexistent` → error "session not found"
- [ ] 7.19 `--spec-file nonexistent.md` → error or warning
- [ ] 7.20 `--spec-file /dev/null` → empty spec, runs normally
- [ ] 7.21 `--pipeline` → forces full pipeline mode
- [ ] 7.22 Conflicting `--confidential --message "search web"` → confidential wins
- [ ] 7.23 Unknown `--foobar` → "flag not defined" error
- [ ] 7.24 `NO_COLOR=1` → zero ANSI codes in ALL output
- [ ] 7.25 `COLORBLIND_MODE=deuteranopia` → no red/green, uses blue/orange
- [ ] 7.26 `SYNROUTE_SCREEN_READER=1` → same as --screen-reader
- [ ] 7.27 `SYNROUTE_CONFIDENTIAL=true` → same as --confidential
- [ ] 7.28 Run from /tmp → works or clear error (non-git dir)
- [ ] 7.29 Run from readonly dir → error or read-only mode
- [ ] 7.30 Run with no .env → uses environment variables
- [ ] 7.31 Run with no API keys → clear error about missing keys
- [ ] 7.32 Two instances same dir → second gets worktree or lock message
- [ ] 7.33 `SYNROUTE_CONVERSATION_TIER=cheap` → starts at cheap tier
- [ ] 7.34 `SYNROUTE_CONVERSATION_TIER=mid` → starts at mid tier
- [ ] 7.35 `SYNROUTE_CONVERSATION_TIER=frontier` → starts at frontier tier
- [ ] 7.36 `SYNROUTE_MESSAGE_TIMEOUT=15` → timeout at 15s

## Cat 8: Network & Provider (16 tests)
- [ ] 8.01 All providers healthy → response received
- [ ] 8.02 Kill internet mid-response → error message, no hang
- [ ] 8.03 VPN connected/disconnected → handles gracefully
- [ ] 8.04 HTTP_PROXY set → uses proxy
- [ ] 8.05 DNS failure → error, no hang
- [ ] 8.06 API 500 → circuit breaker opens, escalates
- [ ] 8.07 API 429 → rate limiter backs off, retries
- [ ] 8.08 API 401 bad key → clear error "unauthorized"
- [ ] 8.09 API empty response → escalates to next provider
- [ ] 8.10 API malformed JSON → error handled, escalates
- [ ] 8.11 API hangs → timeout fires, escalates
- [ ] 8.12 All providers in tier fail → escalates to next tier
- [ ] 8.13 All providers all tiers fail → clear error message
- [ ] 8.14 Slow token streaming → streams to user, no timeout
- [ ] 8.15 Connection drops mid-stream → error, re-prompts or exits
- [ ] 8.16 Profile switch mid-flight → queued or error

## Cat 9: Security & Abuse (13 tests)
- [ ] 9.01 Paste API key → redacted in display AND in DB storage
- [ ] 9.02 Paste .env → secrets scrubbed from storage
- [ ] 9.03 "read /etc/passwd" → agent may read, but SSRF on web_fetch blocked
- [ ] 9.04 "read ~/.ssh/id_rsa" → agent may read (local file), content not leaked to logs
- [ ] 9.05 "run rm -rf /" → agent refuses or tool blocks
- [ ] 9.06 "curl internal IP" (SSRF) → web_fetch blocks private IPs
- [ ] 9.07 "modify synroute config" → agent modifies (no special protection on own config)
- [ ] 9.08 "modify spec file" → BLOCKED (spec is read-only)
- [ ] 9.09 ANSI escapes in message → stripped or rendered safely
- [ ] 9.10 Inject tool call via text → not executed as real tool call
- [ ] 9.11 "Ignore previous instructions" → model does NOT leak system prompt
- [ ] 9.12 Malicious file read by AI → guardrails check content
- [ ] 9.13 Web search with secrets → secrets redacted before search

## Cat 10: State & Recovery (13 tests)
- [ ] 10.01 Fresh launch no DB → creates DB, works
- [ ] 10.02 Resume after clean exit → loads session history
- [ ] 10.03 Resume after Ctrl+C → loads session history
- [ ] 10.04 Resume after kill -9 → loads last checkpoint
- [ ] 10.05 Corrupted SQLite DB → error message, creates new DB or recovers
- [ ] 10.06 Missing log directory → creates it
- [ ] 10.07 Read-only home dir → error, doesn't crash
- [ ] 10.08 Full disk → error writing logs/DB, doesn't crash
- [ ] 10.09 Clock set to future → works (timestamps are future but functional)
- [ ] 10.10 Clock set to past → works
- [ ] 10.11 Timezone change → works
- [ ] 10.12 macOS update pending → works (tested: update pending on this machine)
- [ ] 10.13 Low battery/power save → works (tested: CPU throttle simulation)

## Cat 11: Accessibility — FUNCTIONAL (8 tests)
- [ ] 11.01 --screen-reader → ZERO spinner chars (⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏) in output
- [ ] 11.02 --screen-reader → ZERO box drawing chars (─│┌┐└┘) in output
- [ ] 11.03 --screen-reader → linear text output (no cursor positioning)
- [ ] 11.04 NO_COLOR → ZERO ANSI escape codes (\033[) in stdout
- [ ] 11.05 COLORBLIND_MODE → no red/green color pairs
- [ ] 11.06 High contrast → all text visible (contrast ratio check)
- [ ] 11.07 Large font (48pt) → layout doesn't break
- [ ] 11.08 VoiceOver → screen reader can read output

## Cat 12: Terminal Environments (13 tests)
- [ ] 12.01 tmux → works
- [ ] 12.02 screen → works
- [ ] 12.03 VS Code terminal → works
- [ ] 12.04 JetBrains terminal → works
- [ ] 12.05 Warp → works
- [ ] 12.06 Ghostty → works
- [ ] 12.07 Kitty → works
- [ ] 12.08 Alacritty → works
- [ ] 12.09 Terminal.app → works
- [ ] 12.10 iTerm2 → works
- [ ] 12.11 SSH → works
- [ ] 12.12 mosh → works
- [ ] 12.13 Docker container → works

## Cat 13: Model Routing — FUNCTIONAL (25 tests from spec AC-R*)
- [ ] 13.01 model:auto routes to lowest-usage healthy provider (AC-R1)
- [ ] 13.02 L0 fails → escalates to L1 within same call (AC-R2)
- [ ] 13.03 Full escalation L0→L6→subscription works (AC-R3)
- [ ] 13.04 Circuit breaker opens after 5 failures (AC-R4)
- [ ] 13.05 Circuit breaker closes after cooldown probe (AC-R4)
- [ ] 13.06 Rate limit "reset after Ns" sets cooldown correctly (AC-R5)
- [ ] 13.07 Health cache TTL respected (AC-R6)
- [ ] 13.08 Multiple API keys round-robin (AC-R7)
- [ ] 13.09 Profile switch reinitializes providers (AC-R8)
- [ ] 13.10 Stall detection retries on <50 char response (AC-R9)
- [ ] 13.11 Usage threshold auto-switch at 80% (AC-R10)
- [ ] 13.12 SYNROUTE_CONVERSATION_TIER=cheap → starts at cheap
- [ ] 13.13 SYNROUTE_CONVERSATION_TIER=mid → starts at mid
- [ ] 13.14 SYNROUTE_CONVERSATION_TIER=frontier → starts at frontier
- [ ] 13.15 --model specific → bypasses tier routing
- [ ] 13.16 Work profile defaults to mid tier
- [ ] 13.17 Personal interactive defaults to frontier
- [ ] 13.18 Sub-agents use correct tier
- [ ] 13.19 Subscription fallback when all Ollama fail
- [ ] 13.20 synroute test → each provider responds
- [ ] 13.21 synroute test --provider → single provider tested
- [ ] 13.22 synroute config show → displays tier assignments
- [ ] 13.23 Rate limit 429 → circuit breaker, not infinite retry
- [ ] 13.24 Timeout → escalation to next model
- [ ] 13.25 Empty/malformed response → escalation

## Cat 14: LLM Response Handling (15 tests)
- [ ] 14.01 Empty string response → skipped, next turn or error
- [ ] 14.02 Whitespace only → skipped
- [ ] 14.03 10K+ token response → streamed, no crash
- [ ] 14.04 Malformed tool call JSON → parsed gracefully, no crash
- [ ] 14.05 Tool call for nonexistent tool → error message to LLM
- [ ] 14.06 Tool call missing required args → error message to LLM
- [ ] 14.07 Cut off mid-sentence → handled, may prompt continuation
- [ ] 14.08 Cut off mid-tool-call → partial JSON handled
- [ ] 14.09 Same response twice → loop detection fires
- [ ] 14.10 Response with triple backticks → rendered correctly
- [ ] 14.11 Stream drops mid-stream → error, retries or escalates
- [ ] 14.12 Unrenderable Unicode → displays replacement char or skips
- [ ] 14.13 Tool calls without responding → stall detection fires
- [ ] 14.14 Same tool 10+ times → loop detection fires
- [ ] 14.15 ANSI escapes in response → stripped or passed through

## Cat 15: Conversation & Context — FUNCTIONAL (13 tests)
- [ ] 15.01 Single message → response received
- [ ] 15.02 10-message session → all messages get responses
- [ ] 15.03 50-message session → compaction triggers, no crash
- [ ] 15.04 Context exceeds window → compaction produces structured summary
- [ ] 15.05 "like I said earlier" → model has context (may need recall)
- [ ] 15.06 "do the opposite" → model follows latest instruction
- [ ] 15.07 Same message twice → loop detection or normal response
- [ ] 15.08 /clear then reference prior → prior context gone
- [ ] 15.09 Session saved on clean exit → agent_sessions row in DB
- [ ] 15.10 Session saved on Ctrl+C → agent_sessions row in DB
- [ ] 15.11 Session resume loads history → messages restored
- [ ] 15.12 Multiple sessions by ID → correct one resumed
- [ ] 15.13 Recall after resume → tool outputs accessible

## Cat 16: Worktree & Git (10 tests)
- [ ] 16.01 --message creates worktree (v1.08.11)
- [ ] 16.02 Worktree cleaned up after completion
- [ ] 16.03 Dirty index → worktree still created
- [ ] 16.04 Untracked files → worktree still created
- [ ] 16.05 Staged changes → worktree still created
- [ ] 16.06 Detached HEAD → worktree created or error
- [ ] 16.07 Max worktrees exceeded → error or cleanup
- [ ] 16.08 Submodules → worktree handles
- [ ] 16.09 Concurrent runs → separate worktrees
- [ ] 16.10 Worktree path no branch conflict

## Cat 17: Tool Execution — FUNCTIONAL (39 tests from spec AC-T*)
Each tool must EXECUTE CORRECTLY, not just not crash

- [ ] 17.01 bash: ls → output contains file list
- [ ] 17.02 bash: 30s command → executes, respects timeout
- [ ] 17.03 bash: 10MB output → truncated, not crash
- [ ] 17.04 bash: exit 1 → error code reported to LLM
- [ ] 17.05 bash: interactive prompt → timeout or handled
- [ ] 17.06 file_read: existing → file content returned
- [ ] 17.07 file_read: nonexistent → error message returned
- [ ] 17.08 file_read: no permission → error message
- [ ] 17.09 file_read: 100MB → truncated or error
- [ ] 17.10 file_read: binary → "[binary file]" message
- [ ] 17.11 file_read: being written → reads available content
- [ ] 17.12 file_write: new file → file created with content
- [ ] 17.13 file_write: overwrite → file updated
- [ ] 17.14 file_write: missing parent dir → error
- [ ] 17.15 file_write: no permission → error
- [ ] 17.16 file_write: spec file → BLOCKED (AC-T3 equivalent)
- [ ] 17.17 file_edit: valid search/replace → content changed
- [ ] 17.18 file_edit: old_string not found → error (AC-T2)
- [ ] 17.19 file_edit: ambiguous match → error
- [ ] 17.20 file_edit: replace_all → all occurrences replaced
- [ ] 17.21 grep: found → matching lines returned
- [ ] 17.22 grep: not found → empty result, no error
- [ ] 17.23 grep: invalid regex → error message
- [ ] 17.24 grep: nonexistent dir → error message
- [ ] 17.25 glob: matches → file list returned
- [ ] 17.26 glob: no matches → empty result
- [ ] 17.27 git: status → status output
- [ ] 17.28 git: diff → diff output
- [ ] 17.29 git: log → log output
- [ ] 17.30 git: push --force → BLOCKED (AC-T3)
- [ ] 17.31 git: reset --hard → BLOCKED (AC-T3)
- [ ] 17.32 git: checkout --force → BLOCKED (AC-T3)
- [ ] 17.33 web_search: query → search results returned
- [ ] 17.34 web_search: confidential → BLOCKED
- [ ] 17.35 web_search: PII → redacted before search
- [ ] 17.36 web_fetch: valid URL → content returned
- [ ] 17.37 web_fetch: private IP → BLOCKED (SSRF protection)
- [ ] 17.38 web_fetch: 404 → error returned
- [ ] 17.39 web_fetch: confidential → BLOCKED

## Cat 18: Concurrency (13 tests)
- [ ] 18.01 Single request completes
- [ ] 18.02 2 concurrent complete
- [ ] 18.03 3 concurrent complete
- [ ] 18.04 4+ concurrent → semaphore queues 4th
- [ ] 18.05 SYNROUTE_MAX_CONCURRENT=1 → serializes
- [ ] 18.06 SYNROUTE_MAX_CONCURRENT=5 → allows 5
- [ ] 18.07 Multiple API keys scale limit
- [ ] 18.08 Semaphore releases on error
- [ ] 18.09 Semaphore releases on timeout
- [ ] 18.10 Semaphore releases on Ctrl+C
- [ ] 18.11 Context cancellation unblocks queue
- [ ] 18.12 Sub-agents share parent limit
- [ ] 18.13 HTTP serve mode handles multiple clients

## Cat 19: Timeouts (10 tests)
- [ ] 19.01 Default 120s timeout exists
- [ ] 19.02 SYNROUTE_MESSAGE_TIMEOUT=15 overrides
- [ ] 19.03 Timeout prints clear error message
- [ ] 19.04 Timeout exits clean (exit 1, not crash)
- [ ] 19.05 Fast messages complete well within timeout
- [ ] 19.06 Agent loop checks ctx at top of each turn
- [ ] 19.07 Tool calls respect context cancellation
- [ ] 19.08 LLM calls respect context cancellation
- [ ] 19.09 @nonexistent doesn't hang
- [ ] 19.10 Slow model eventually times out

## Cat 20: CLI Commands — FUNCTIONAL (from spec)
- [ ] 20.01 synroute version → shows version string
- [ ] 20.02 synroute --help → shows all commands
- [ ] 20.03 synroute test → tests all providers, shows PASS/FAIL per provider
- [ ] 20.04 synroute test --provider X → tests single provider
- [ ] 20.05 synroute test --json → JSON output
- [ ] 20.06 synroute doctor → runs diagnostics, shows status per check
- [ ] 20.07 synroute doctor --json → JSON diagnostics
- [ ] 20.08 synroute models → lists all available models
- [ ] 20.09 synroute profile show → shows active profile
- [ ] 20.10 synroute profile list → lists available profiles
- [ ] 20.11 synroute config show → shows tier configuration
- [ ] 20.12 synroute config generate → generates YAML from env
- [ ] 20.13 synroute recommend → model recommendations
- [ ] 20.14 synroute eval exercises --language go → lists exercises
- [ ] 20.15 synroute eval results → shows most recent run
- [ ] 20.16 synroute search stats → shows search backend metrics
- [ ] 20.17 synroute mcp-serve → starts MCP server
- [ ] 20.18 synroute serve → starts HTTP server

## Cat 21: Intent Detection — FUNCTIONAL (from spec)
- [ ] 21.01 "hello" → chat intent, 0 tools
- [ ] 21.02 "what is Go?" → chat or explain, read_only tools
- [ ] 21.03 "fix the bug" → fix intent, read_write tools
- [ ] 21.04 "write a function" → generate intent, full tools
- [ ] 21.05 "review my code" → review intent, read_only tools
- [ ] 21.06 "search for X" → research intent, web tools
- [ ] 21.07 "explain this" → explain intent, read_only tools
- [ ] 21.08 "plan the architecture" → plan intent, no tools
- [ ] 21.09 Tool group filtering works (chat gets 0 tools, not 19)
- [ ] 21.10 Confidence < 0.25 → falls back to full tools

## Cat 22: Memory System — FUNCTIONAL (from spec AC-M*)
- [ ] 22.01 Messages stored in VectorMemory (AC-M1)
- [ ] 22.02 recall(id=N) returns full output (AC-M2)
- [ ] 22.03 recall(query="...") returns relevant results (AC-M3)
- [ ] 22.04 Tool outputs >2KB summarized in conversation (AC-T5)
- [ ] 22.05 Secrets scrubbed from DB storage (AC-T6)
- [ ] 22.06 Cross-session recall via ParentSessionIDs (AC-M2)

## Cat 23: Pipeline — FUNCTIONAL (from spec AC-P*)
- [ ] 23.01 Plan phase produces acceptance criteria (AC-P1)
- [ ] 23.02 Verification phases require min tool calls (AC-P2)
- [ ] 23.03 Review uses fresh sub-agent (AC-P3)
- [ ] 23.04 Max 3 fail-back cycles (AC-P4)
- [ ] 23.05 Compaction between phases (AC-P5)
- [ ] 23.06 Trivial tasks skip pipeline
- [ ] 23.07 Data science pipeline detected for .py/.ipynb

## Cat 24: Hallucination Detection (from spec AC-HD*)
- [ ] 24.01 FactTracker accumulates ground truth (AC-HD1)
- [ ] 24.02 Contradiction detected in <1ms (AC-HD2)
- [ ] 24.03 AutoRecall injects correction (AC-HD3)
- [ ] 24.04 Max 3 corrections per session (AC-HD4)

## Cat 25: HTTP API — FUNCTIONAL (from spec AC-H*)
- [ ] 25.01 /v1/chat/completions returns OpenAI-compatible (AC-H1)
- [ ] 25.02 /v1/responses supports SSE streaming (AC-H2)
- [ ] 25.03 /health returns 200 (AC-H3)
- [ ] 25.04 Circuit breaker reset API works (AC-H4)
- [ ] 25.05 MCP tools/list returns all tools (AC-H5)
- [ ] 25.06 MCP tools/call executes tool (AC-H5)
- [ ] 25.07 /v1/models lists models
- [ ] 25.08 /v1/skills lists skills
- [ ] 25.09 /v1/tools lists tools
- [ ] 25.10 /v1/doctor returns diagnostics
