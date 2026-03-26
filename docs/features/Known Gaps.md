---
title: Known Gaps
updated: 2026-03-26
status: tracking
---

# Known Gaps

Issues identified during UAT and development that need fixing.

## Critical

### Agent can't install missing build tools
- **Found:** Java UAT — agent couldn't run Maven because `mvn` not on PATH
- **Impact:** Projects requiring specific build tools (Maven, Gradle, npm, cargo) fail silently
- **Fix needed:** Agent should detect missing tools and install them (brew, sdkman, nvm) or report the gap clearly
- **Files:** `internal/agent/agent.go` (system prompt), `internal/environment/detector.go`

### Agent API uses temp dir by default
- **Found:** E2E testing — files were deleted after request (FIXED in `74c62a4`)
- **Status:** Fixed — now respects `project` and `work_dir` params
- **Remaining:** CLI `synroute chat --message` still starts own server, can conflict with running instance

## High

### No tool call verification on text-only responses
- **Found:** First E2E test — agent described creating files without using tools
- **Impact:** LLM can "complete" a task by describing what it would do
- **Partial fix:** System prompt now says "Every response must include tool calls"
- **Better fix:** Detect text-only responses that claim completion, force tool verification

### Sub-agent permissions in worktrees
- **Found:** All worktree agents — Write and Bash denied
- **Status:** Fixed via settings.local.json permissions
- **Root cause:** Claude Code permission model doesn't propagate to worktree sub-processes

### Gemini thought_signature errors on tool calls
- **Found:** E2E testing — Gemini rejects tool calls missing thought_signature
- **Impact:** Gemini can't be used for agent tool-calling sessions
- **Files:** Provider-specific tool call formatting needed

## Medium

### TF-IDF embeddings miss semantic paraphrases
- **Found:** Deep dive analysis — "HTTP handler" vs "web server" similarity ~0.15
- **Impact:** Recall quality limited for synonym/paraphrase queries
- **Fix:** Story 6.1 — ONNX bundled real embedding model (all-MiniLM-L6-v2)

### Language detection false positives
- **Found:** Code review — "func " matches Go, but also "functional requirements"
- **Impact:** Wrong language skills injected into system prompt
- **Files:** `internal/router/preprocess.go`

### Model-aware compaction untested at scale
- **Found:** UAT 3 ran 93 LLM calls but conversation didn't reach 500 messages
- **Impact:** MaxMessages=500 for Gemini is theoretical, not validated

## Low

### diagnostics.go file race with concurrent agents
- **Found:** Code review — read-modify-write on synroute.md not atomic across processes
- **Status:** Mitigated with temp-file-rename, but rename itself races on read

### Story 0.7 (skill injection budget) not implemented
- **Status:** Deferred — 52 skills, but no token budget enforcement
- **Impact:** System prompt could grow large with many matched skills

### Migration runner re-runs all migrations
- **Status:** Handled for ALTER TABLE (ignore duplicate column errors)
- **Better fix:** Track applied migrations in a `schema_migrations` table
