---
title: Known Gaps
updated: 2026-03-26
status: tracking
---

# Known Gaps

Issues identified during UAT and development that need fixing. This is a living document -- gaps get closed as stories are completed.

## Roadmap Priority

Gaps are ordered by when they will be addressed. Phase 1 is active work; later phases are planned but not started.

### Phase 1 -- Foundation + Pipeline (Active)

| Gap | Severity | Story | Wave |
|---|---|---|---|
| Secret scrubbing for tool output storage | Critical | 0.4 | 1 |
| Recall tool session scoping for sub-agents | High | 0.5 | 2 |
| Compaction summary missing key context | High | 0.6 | 2 |
| Skill injection has no token budget | Low | 0.7 | 2 |
| ~~No tool call verification on text-only responses~~ | ~~High~~ | ~~1.1~~ | ~~3~~ | **CLOSED** -- text-based tool call parser (5 formats) + completion signal detection |
| ~~Review cycle spinning (no improvement detection)~~ | ~~High~~ | ~~1.2~~ | ~~3~~ | **CLOSED** -- review stability/divergence detection implemented |
| Language detection false positives | Medium | 3.2 | 1 |
| Context propagation / thread safety | High | 2.2 | 2 |
| diagnostics.go file race with concurrent agents | Low | 2.2 | 2 |
| Migration runner re-runs all migrations | Low | 4.1 | 3 |

### Phase 2 -- DevOps + Self-Improvement

| Gap | Severity | Story |
|---|---|---|
| No CI pipeline or Dockerfile | High | 4.1 |
| Go 1.21 EOL, gorilla/mux archived | Medium | 4.1 |
| Documentation contradictions (NanoGPT references) | Medium | 4.2 |
| No self-improvement feedback loop | Medium | 5.1 |

### Phase 3 -- Infrastructure + Chat API

| Gap | Severity | Story |
|---|---|---|
| TF-IDF embeddings miss semantic paraphrases | Medium | 6.1 |
| MCP tool invocation is a no-op | High | 6.2 |
| Agent can't install missing build tools | Critical | 6.5 |
| No stateful chat sessions via API | High | 7.1 |
| Gemini thought_signature errors on tool calls | High | 7.1 |

### Future

| Gap | Severity | Story |
|---|---|---|
| No rich content creation (docs, presentations) | Medium | 8.1 |
| ~~No terminal UI beyond basic REPL~~ | ~~Low~~ | ~~9.1~~ | **CLOSED** -- code mode TUI with status bar, scroll regions, keyboard shortcuts (^P, ^T, ^L, ^E, ^/) |
| SQLite single-writer bottleneck at scale | Low | 10.1 |

---

## Detailed Descriptions

### Critical

#### Agent can't install missing build tools
- **Found:** Java UAT -- agent couldn't run Maven because `mvn` not on PATH
- **Impact:** Projects requiring specific build tools (Maven, Gradle, npm, cargo) fail silently
- **Fix needed:** Agent should detect missing tools and install them (brew, sdkman, nvm) or report the gap clearly
- **Files:** `internal/agent/agent.go` (system prompt), `internal/environment/detector.go`
- **Roadmap:** Phase 3 (Story 6.5 -- bash tool sandboxing includes tool management)

#### Agent API uses temp dir by default
- **Found:** E2E testing -- files were deleted after request (FIXED in `74c62a4`)
- **Status:** Fixed -- now respects `project` and `work_dir` params
- **Remaining:** CLI `synroute chat --message` still starts own server, can conflict with running instance

### High

#### ~~No tool call verification on text-only responses~~ (CLOSED)
- **Found:** First E2E test -- agent described creating files without using tools
- **Status:** Fixed -- text-based tool call parser (`internal/agent/text_tool_parser.go`) parses 5 tool call formats from Ollama models that don't support native function calling. Completion signal detection prevents infinite loops when the agent has finished its task.

#### Sub-agent permissions in worktrees
- **Found:** All worktree agents -- Write and Bash denied
- **Status:** Fixed via settings.local.json permissions
- **Root cause:** Claude Code permission model doesn't propagate to worktree sub-processes

#### Gemini thought_signature errors on tool calls
- **Found:** E2E testing -- Gemini rejects tool calls missing thought_signature
- **Impact:** Gemini can't be used for agent tool-calling sessions
- **Files:** Provider-specific tool call formatting needed
- **Roadmap:** Phase 3 (Story 7.1 -- chat backend needs provider-specific formatting)

### Medium

#### TF-IDF embeddings miss semantic paraphrases
- **Found:** Deep dive analysis -- "HTTP handler" vs "web server" similarity ~0.15
- **Impact:** Recall quality limited for synonym/paraphrase queries
- **Fix:** Story 6.1 -- ONNX bundled real embedding model (all-MiniLM-L6-v2)
- **Roadmap:** Phase 3

#### Language detection false positives
- **Found:** Code review -- "func " matches Go, but also "functional requirements"
- **Impact:** Wrong language skills injected into system prompt
- **Files:** `internal/router/preprocess.go`
- **Roadmap:** Phase 1 (Story 3.2 -- trigger quality)

#### Model-aware compaction untested at scale
- **Found:** UAT 3 ran 93 LLM calls but conversation didn't reach 500 messages
- **Impact:** MaxMessages=500 for Gemini is theoretical, not validated
- **Roadmap:** Phase 1 (Story 2.1 -- test coverage)

### Low

#### diagnostics.go file race with concurrent agents
- **Found:** Code review -- read-modify-write on synroute.md not atomic across processes
- **Status:** Mitigated with temp-file-rename, but rename itself races on read
- **Roadmap:** Phase 1 (Story 2.2 -- thread safety)

#### Story 0.7 (skill injection budget) not implemented
- **Status:** Deferred -- 52 skills, but no token budget enforcement
- **Impact:** System prompt could grow large with many matched skills
- **Roadmap:** Phase 1 (Story 0.7 -- Wave 2)

#### Migration runner re-runs all migrations
- **Status:** Handled for ALTER TABLE (ignore duplicate column errors)
- **Better fix:** Track applied migrations in a `schema_migrations` table
- **Roadmap:** Phase 2 (Story 4.1 -- DevOps)
