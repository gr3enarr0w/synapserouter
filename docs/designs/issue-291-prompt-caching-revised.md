# Issue #291: Prompt Caching — Revised Design

**Status**: Design
**Date**: 2026-03-28
**Author**: auto-generated from codebase analysis

## Problem

Every LLM call in synapserouter sends the full system prompt + conversation history. Most providers support prefix-based prompt caching (where an identical message prefix across calls is served from cache, cutting input token costs 50-90%). Synapserouter currently does not structure its messages to take advantage of this. Worse, several subsystems inject dynamic content *before* stable content, which would actively defeat prefix caching.

## Codebase Analysis: What Gets Injected and Where

### 1. System Prompt (`buildMessages` — agent.go:870)

The system prompt is built once per provider level and cached in `cachedSystemPrompt`. It is reconstructed when the agent escalates to a new provider level (`cachedPromptLevel != providerIdx`).

**Construction order** (agent.go:870-914):

```
1. defaultSystemPrompt(workDir, providerLevel, projectLanguage)   ← STATIC per session
2. + Project Instructions (CLAUDE.md / AGENTS.md)                 ← STATIC per session
3. + matchedSkillContext() (computed once via sync.Once)           ← STATIC per session
4. + "NOTE: Skill patterns are reference examples..."             ← STATIC literal
5. + specConstraints.FormatConstraints()                          ← STATIC per session
6. + toolchainSetup                                               ← STATIC per session
7. + resolvedBuildCmds                                            ← STATIC per session
8. + intentPromptAdjustment                                       ← STATIC per session
```

**Verdict**: The system prompt is **static within a provider level**. It only changes when `providerIdx` changes (escalation). This is excellent for caching — the system message is identical across all calls at the same level.

### 2. Auto-Context / Recall Injection (`buildMessages` — agent.go:925-945)

After compaction, `buildMessages` injects retrieved VectorMemory context:

```
msgs[0] = system prompt                              ← STATIC
msgs[1] = "[Retrieved context from earlier:] ..."    ← DYNAMIC (changes per call)
msgs[2] = "Understood, I have the retrieved context"  ← DYNAMIC (synthetic ack)
msgs[3..N] = conversation.Messages()                  ← DYNAMIC (grows each turn)
```

**Verdict**: **DYNAMIC per call**. The retrieved context depends on the last user message and what was compacted. This content appears at position [1] — immediately after the system prompt — which **breaks prefix caching** because the prefix changes every call after compaction.

### 3. Router-Level Memory Injection (`router.go:179-238`)

The router's `ChatCompletionWithDebug` retrieves VectorMemory messages and **prepends** them before the request messages:

```
priorMessages = [recalled msg 1, recalled msg 2, ...]  ← DYNAMIC
priorMessages = append(priorMessages, req.Messages...)
req.Messages = priorMessages
```

**Verdict**: **DYNAMIC**, inserted at the front of the message array. This also breaks prefix caching. However, this path is for the `/v1/chat/completions` API endpoint — the agent loop uses `buildMessages()` directly, not this router path. The agent calls the executor which goes through the router, but the agent has already constructed the message array via `buildMessages()`. The router then adds memory injection on top.

**Critical finding**: There are TWO memory injection points — one in `buildMessages()` (auto-context after compaction) and one in `router.go` (cross-session recall). Both inject dynamic content early in the message sequence.

### 4. Router-Level Skill Preprocessing (`preprocess.go:43`)

`PreprocessRequest` analyzes the conversation and injects skill context into the **system message**:

```go
// injectSkillContext (preprocess.go:213-242)
req.Messages[0].Content = injectionText + "\n\n" + req.Messages[0].Content
```

**Verdict**: **DYNAMIC per call** — the injected text changes based on detected languages, error loops, code changes, and security surface. This **prepends** to the system message, which means the system prompt prefix changes on every call where the conversation signals differ. This directly defeats prefix caching.

### 5. Conversation Compaction (`compactConversation` — agent.go:2166)

Between pipeline phases, messages are compacted:
- Messages > 30: drop oldest, keep last 20
- Dropped messages stored to DB
- A summary placeholder replaces them
- Sets `hasCompacted = true`, which triggers auto-context injection in future `buildMessages()` calls

**Verdict**: Compaction **resets the conversation prefix**. After compaction, the message sequence is:
```
[system] [summary placeholder] [last 20 messages]
```
This is a prefix break but is expected — compaction is infrequent (between phases). The real issue is that post-compaction, auto-context injection makes every subsequent call's prefix different.

### 6. Spec Constraint Injection (`spec_constraints.go`)

Constraints are extracted once at session startup via `ExtractSpecConstraints()` (regex, no LLM call). The formatted block is appended to the system prompt in `buildMessages()`.

**Verdict**: **STATIC per session**. Already in the right place (inside the cached system prompt).

### 7. Pipeline Phase Prompts (`pipeline.go:281 — PhasePrompt`)

Phase prompts are injected as user messages by the pipeline controller. They contain `%CRITERIA%` and `%SPEC%` placeholders that get replaced with session-specific content.

**Verdict**: **STATIC within a phase** — the phase prompt doesn't change between LLM calls within the same phase. It changes when the phase advances. These appear as user messages in the conversation, so they're part of the growing conversation prefix and naturally cacheable as long as earlier messages don't change.

## Provider Caching Mechanisms

### Anthropic (Claude via Vertex rawPredict)

Anthropic's prompt caching works on **exact prefix matching**:
- The system prompt, tools, and message prefix are hashed
- If the prefix matches a previous call (within the cache TTL, typically 5 minutes), cached tokens are charged at 10% of input cost
- Cache breakpoints can be set via `cache_control: {"type": "ephemeral"}` on system/message content blocks
- Cache is per-model, per-organization
- Minimum cacheable prefix: 1024 tokens (Sonnet), 2048 tokens (Haiku)

**Current gap in vertex.go**: The `claudeRawPredict` function does not emit `cache_control` on any content blocks. System content is joined as a plain string, not as content blocks with cache markers.

### Ollama Cloud (OpenAI-compatible)

Ollama Cloud uses the OpenAI-compatible `/v1/chat/completions` endpoint. OpenAI's automatic prompt caching:
- Caches the **longest matching prefix** of the message array
- Minimum 128 tokens, prefix must match exactly
- No explicit cache markers needed — it's automatic
- Cached tokens charged at 50% discount
- Cache TTL: 5-10 minutes of inactivity

**Implication**: For Ollama Cloud, message ordering is everything. Any dynamic content injected before stable content destroys the cacheable prefix.

### Gemini (Vertex generateContent)

Gemini supports context caching via `CachedContent` API:
- Must explicitly create a cache object with a TTL
- Cache is referenced by ID in subsequent requests
- Minimum 32,768 tokens to be cacheable
- Cached tokens charged at 25% of input cost
- TTL must be managed (creation, refresh, deletion)

**Implication**: Gemini caching requires a fundamentally different approach — explicit cache lifecycle management rather than prefix ordering. This is out of scope for phase 1.

## Current Message Sequence (Before Caching)

```
Call N (normal, no compaction):
┌─────────────────────────────────────────┐
│ [0] system: defaultPrompt + project     │ ← STATIC per level
│            + skills + spec + toolchain  │
│ [1] user: "fix the auth bug"           │ ← conversation
│ [2] assistant: "Let me look at..."      │   history grows
│ [3] user: tool result                   │   each turn
│ [4] assistant: "I found the issue..."   │
│ [5] user: "now add tests"              │
│ ...                                     │
└─────────────────────────────────────────┘
  ↑ prefix caching works naturally here

Call N (after compaction + auto-context):
┌─────────────────────────────────────────┐
│ [0] system: defaultPrompt + skills...   │ ← STATIC
│ [1] user: "[Retrieved context from...]" │ ← DYNAMIC ← breaks cache
│ [2] assistant: "Understood..."          │ ← DYNAMIC
│ [3] user: "[Phase X completed. ...]"    │ ← summary
│ [4..23] last 20 conversation messages   │
└─────────────────────────────────────────┘
  ↑ prefix breaks at [1] because retrieved context changes per call

Call N (router-level memory injection):
┌─────────────────────────────────────────┐
│ [0] system: SKILL_INJECT + prompt       │ ← DYNAMIC prefix ← breaks cache
│ [1] user: recalled memory msg           │ ← DYNAMIC
│ [2] assistant: recalled memory msg      │ ← DYNAMIC
│ [3..N] original request messages        │
└─────────────────────────────────────────┘
  ↑ prefix breaks at [0] because skill preprocess prepends dynamic text
```

## Revised Design: Maximum Cache Hit Rate

### Principle: Static Prefix, Dynamic Suffix

Structure every LLM call so that content is ordered from most-stable to least-stable:

```
CACHE-FRIENDLY MESSAGE ORDERING:

┌─────────────────────────────────────────┐
│ ZONE 1: STATIC (cacheable prefix)       │
│ ┌─────────────────────────────────────┐ │
│ │ [0] system: base prompt + project   │ │  Never changes within a
│ │     instructions + skills + spec    │ │  provider level. Identical
│ │     constraints + toolchain setup   │ │  across all calls.
│ │     + intent adjustment             │ │
│ │     ──── cache breakpoint ────      │ │
│ └─────────────────────────────────────┘ │
│ ┌─────────────────────────────────────┐ │
│ │ [1] tools definition (if any)       │ │  Tool list is static per
│ │     (part of API request, not msg)  │ │  session. Cached by
│ │                                     │ │  Anthropic as part of
│ │                                     │ │  prefix.
│ └─────────────────────────────────────┘ │
├─────────────────────────────────────────┤
│ ZONE 2: SEMI-STATIC (conversation)      │
│ ┌─────────────────────────────────────┐ │
│ │ [1..N-1] conversation history       │ │  Only grows — earlier
│ │   user → assistant → tool → ...     │ │  messages never change.
│ │                                     │ │  Each new call extends
│ │                                     │ │  the prefix by 1-2 msgs.
│ │     ──── cache breakpoint ────      │ │
│ └─────────────────────────────────────┘ │
├─────────────────────────────────────────┤
│ ZONE 3: DYNAMIC (never cached)          │
│ ┌─────────────────────────────────────┐ │
│ │ [N] user: current turn's message    │ │  The new user message or
│ │     OR: retrieved context + message │ │  pipeline phase prompt.
│ └─────────────────────────────────────┘ │
└─────────────────────────────────────────┘
```

### Change 1: Move Auto-Context to End of Messages

**Current** (agent.go:925-945): Retrieved context is injected at position [1], right after system.

**Proposed**: Move retrieved context to a user message appended at the **end** of the message array, just before or as part of the current turn's message.

```
BEFORE (breaks cache):
  [system] [retrieved context] [ack] [conversation...]

AFTER (preserves cache):
  [system] [conversation...] [retrieved context + current message]
```

The retrieved context should be formatted as part of the current user turn:

```
[Earlier context that may be relevant:]
<retrieved messages>

[Current task:]
<actual user message or phase prompt>
```

This keeps the system prompt + conversation history prefix identical across calls, maximizing cache hits.

### Change 2: Move Skill Preprocessing to End (Router Path)

**Current** (preprocess.go:213-242): `injectSkillContext` prepends dynamic text to `req.Messages[0].Content` (the system message).

**Proposed**: For the router path (`/v1/chat/completions`), inject skill preprocessing as a **user message appended at the end** rather than modifying the system prompt. This preserves the system prompt prefix.

```
BEFORE (breaks cache):
  [system: DYNAMIC_SKILL_TEXT + original_prompt] [messages...]

AFTER (preserves cache):
  [system: original_prompt] [messages...] [user: skill_context_hints]
```

Note: The agent loop path already handles skills correctly — `matchedSkillContext()` is computed once and baked into the cached system prompt. The router-level preprocessor is the problem.

### Change 3: Move Router Memory Injection to End

**Current** (router.go:206-215): Retrieved memory messages are prepended before request messages.

**Proposed**: Append recalled memory as context within the last user message rather than prepending as separate messages.

```
BEFORE (breaks cache):
  [system] [recalled_msg_1] [recalled_msg_2] [original_messages...]

AFTER (preserves cache):
  [system] [original_messages...] [user: "Context from memory:\n..." + current_message]
```

### Change 4: Add Cache Breakpoints for Anthropic

**Current** (vertex.go:188-189): System content is a plain string.

**Proposed**: When building the Anthropic rawPredict payload, structure the system content as content blocks with `cache_control`:

```json
{
  "system": [
    {
      "type": "text",
      "text": "<full system prompt>",
      "cache_control": {"type": "ephemeral"}
    }
  ],
  "messages": [
    ...
    {
      "role": "user",
      "content": [
        {
          "type": "text",
          "text": "<conversation turn N-1>",
          "cache_control": {"type": "ephemeral"}
        }
      ]
    },
    {
      "role": "user",
      "content": [{"type": "text", "text": "<current turn>"}]
    }
  ]
}
```

Place cache breakpoints at:
1. End of system prompt (caches the static instructions)
2. Second-to-last user message (caches conversation history up to the previous turn)

### Change 5: Compaction-Aware Prefix Preservation

**Current**: After compaction, `buildMessages()` prepends retrieved context at position [1].

**Proposed**: After compaction, the summary placeholder becomes part of the conversation history (this is already the case). The auto-context retrieval moves to the end (per Change 1). The message sequence after compaction becomes:

```
[system]                                    ← STATIC, cached
[user: "Phase X completed. N msgs compacted"] ← becomes part of prefix
[kept_msg_1] ... [kept_msg_20]              ← becomes part of prefix
[user: retrieved_context + current_message]  ← DYNAMIC, at end
```

This means the first call after compaction breaks the cache (the prefix changes because old messages were removed), but all subsequent calls within the new phase benefit from caching again.

### Change 6: System Prompt Stability Across Escalation

**Current**: When the agent escalates (`providerIdx` changes), `cachedSystemPrompt` is invalidated and rebuilt. The Level 0 prompt is structurally different from Level 1+ (shorter, different wording).

**Impact on caching**: Escalation inherently breaks the cache. This is acceptable because escalation is infrequent (typically 0-2 times per session). No change needed — just document that escalation is a known cache break.

## Message Ordering Diagram

```
                    ┌──────────────────────────────────┐
                    │        LLM API Call N             │
                    └──────────────────────────────────┘

 CACHEABLE PREFIX   ┌──────────────────────────────────┐
 (identical to      │ system: base prompt              │
  call N-1)         │   + project instructions         │  Zone 1: Static
                    │   + matched skills               │  per provider level
                    │   + spec constraints             │
                    │   + toolchain + build cmds       │
                    │   + intent adjustment            │
                    │   ─── CACHE BREAKPOINT ───       │
                    ├──────────────────────────────────┤
                    │ user: "fix the auth bug"         │
                    │ assistant: "Let me check..."     │  Zone 2: Conversation
                    │ user: [tool result]              │  history (append-only,
                    │ assistant: "Found it, fixing..." │  each call extends by
                    │ user: [tool result]              │  1-2 messages)
                    │   ─── CACHE BREAKPOINT ───       │
 ──── cache ─────── ├──────────────────────────────────┤
      boundary      │ user: [retrieved context]        │  Zone 3: Dynamic
 NEW THIS CALL      │       + current message          │  (new each call)
                    └──────────────────────────────────┘

 After compaction:
                    ┌──────────────────────────────────┐
 NEW PREFIX         │ system: (same as above)          │  Zone 1: Cached if
 (cache miss on     │   ─── CACHE BREAKPOINT ───       │  same provider level
  first call,       ├──────────────────────────────────┤
  then cached)      │ user: "Phase plan completed..."  │  Zone 2: Compacted
                    │ [last 20 messages from before]   │  history
                    │   ─── CACHE BREAKPOINT ───       │
                    ├──────────────────────────────────┤
                    │ user: [auto-context + new msg]   │  Zone 3: Dynamic
                    └──────────────────────────────────┘
```

## Specific Changes to `buildMessages()`

### Current Code (agent.go:870-948)

```go
func (a *Agent) buildMessages() []providers.Message {
    // 1. Build/cache system prompt (lines 872-914)
    // 2. Start with system message (line 917-919)
    // 3. If hasCompacted: inject retrieved context at [1] (lines 925-945)
    // 4. Append conversation messages (line 947)
}
```

### Proposed Code Structure

```go
func (a *Agent) buildMessages() []providers.Message {
    // 1. Build/cache system prompt — UNCHANGED
    //    (already static per provider level, good for caching)

    // 2. Start with system message — UNCHANGED
    msgs := []providers.Message{{
        Role:    "system",
        Content: a.cachedSystemPrompt,
    }}

    // 3. Append conversation messages — UNCHANGED, moved before auto-context
    msgs = append(msgs, a.conversation.Messages()...)

    // 4. MOVED: Auto-context injection goes AFTER conversation, not before
    //    Append as part of the last user message or as a new user message at the end
    if a.hasCompacted && a.config.VectorMemory != nil {
        lastUser := a.conversation.LastUserMessage()
        if lastUser != "" {
            retrieved, err := a.config.VectorMemory.RetrieveRelevant(lastUser, a.sessionID, 2048)
            if err == nil && len(retrieved) > 0 {
                // Format as context block appended at the end
                var contextBuf strings.Builder
                contextBuf.WriteString("\n\n[Retrieved context from earlier in this session:]\n")
                for _, m := range retrieved {
                    contextBuf.WriteString(fmt.Sprintf("[%s] %s\n", m.Role, m.Content))
                }
                // Append to the last message if it's a user message,
                // otherwise add a new user message
                if len(msgs) > 0 && msgs[len(msgs)-1].Role == "user" {
                    msgs[len(msgs)-1].Content += contextBuf.String()
                } else {
                    msgs = append(msgs, providers.Message{
                        Role:    "user",
                        Content: contextBuf.String(),
                    })
                    msgs = append(msgs, providers.Message{
                        Role:    "assistant",
                        Content: "Understood, I have the retrieved context.",
                    })
                }
            }
        }
    }

    return msgs
}
```

### Changes to Router Memory Injection (router.go)

```go
// CURRENT: prepend recalled messages before request messages
// PROPOSED: append recalled context to the last user message

if len(retrievedMessages) > 0 {
    var contextBuf strings.Builder
    contextBuf.WriteString("[Recalled context from previous conversation:]\n")
    for _, msg := range retrievedMessages {
        if msg.Role == "tool" || msg.Content == "" {
            continue
        }
        contextBuf.WriteString(fmt.Sprintf("[%s] %s\n", msg.Role, msg.Content))
    }
    // Find the last user message and append context to it
    for i := len(req.Messages) - 1; i >= 0; i-- {
        if req.Messages[i].Role == "user" {
            req.Messages[i].Content = contextBuf.String() + "\n\n" + req.Messages[i].Content
            break
        }
    }
}
```

### Changes to Skill Preprocessor (preprocess.go)

```go
// CURRENT: prepend injection text to system message content
// PROPOSED: append as a user message at the end

func injectSkillContext(req *providers.ChatRequest, injections []SkillInjection) {
    // ... build injectionText as before ...

    // Append as a separate user message at the end (preserves system prompt prefix)
    req.Messages = append(req.Messages, providers.Message{
        Role:    "user",
        Content: injectionText,
    })
}
```

### Changes to Vertex Claude Provider (vertex.go)

```go
// In claudeRawPredict, structure system as content blocks with cache_control:

if len(systemParts) > 0 {
    systemText := strings.Join(systemParts, "\n")
    payload["system"] = []interface{}{
        map[string]interface{}{
            "type": "text",
            "text": systemText,
            "cache_control": map[string]interface{}{"type": "ephemeral"},
        },
    }
}

// For the second-to-last user message, add cache_control to its content block
// (implementation detail: track message index and add cache_control to the
// penultimate user message's content block)
```

## Cache Hit Rate Estimates

| Scenario | Current | After Changes |
|---|---|---|
| Normal agent loop (no compaction) | ~80% (system prompt cached, conversation grows) | ~95% (same, but router/preprocess no longer break prefix) |
| After compaction | 0% (auto-context at [1] changes every call) | ~80% (first call misses, subsequent calls hit) |
| Router API path | 0% (memory prepend + skill prepend break prefix) | ~85% (both moved to suffix) |
| Sub-agent (fresh) | 0% (new conversation) | ~60% (system prompt cached if same model as parent) |
| After escalation | 0% (prompt rebuilt) | 0% (expected, escalation is infrequent) |

## Acceptance Criteria

### Core Caching

- [ ] **AC-1**: System prompt is the first message in every LLM call and does not change within a provider level
- [ ] **AC-2**: No dynamic content is prepended before the system prompt or between the system prompt and conversation history
- [ ] **AC-3**: Auto-context (retrieved VectorMemory after compaction) appears at the END of the message array, not at position [1]
- [ ] **AC-4**: Router-level memory injection appears at the END of the message array (in the last user message), not prepended as separate messages
- [ ] **AC-5**: Router-level skill preprocessing does NOT modify the system message content; skill hints are appended as a trailing user message

### Anthropic-Specific

- [ ] **AC-6**: Vertex Claude rawPredict sends `system` as a content block array with `cache_control: {"type": "ephemeral"}` on the system text block
- [ ] **AC-7**: The second-to-last user message in Anthropic calls includes `cache_control: {"type": "ephemeral"}` on its content block
- [ ] **AC-8**: Cache hit rate is observable via Anthropic's `usage.cache_creation_input_tokens` and `usage.cache_read_input_tokens` response fields

### Interaction Verification

- [ ] **AC-9**: After `compactConversation()`, the next call has a cache miss on the conversation portion but hits on the system prompt
- [ ] **AC-10**: After `compactConversation()`, the second and subsequent calls in the same phase get cache hits on the full prefix (system + compacted conversation)
- [ ] **AC-11**: Skill context computed via `matchedSkillContext()` remains inside the cached system prompt (not duplicated or moved)
- [ ] **AC-12**: Spec constraints remain inside the cached system prompt
- [ ] **AC-13**: Pipeline phase prompts (injected as user messages) become part of the cacheable prefix on subsequent turns within the same phase
- [ ] **AC-14**: Sub-agent calls that use the same model as the parent get system prompt cache hits (Anthropic cross-call caching)

### Regression

- [ ] **AC-15**: Retrieved context is still visible to the LLM (functional equivalence — moving it to the end doesn't lose information)
- [ ] **AC-16**: Router memory injection still provides cross-session continuity (functional equivalence)
- [ ] **AC-17**: Skill preprocessing hints are still delivered to the LLM (functional equivalence)
- [ ] **AC-18**: Tool results and assistant messages with tool_calls maintain correct ordering (no orphaned tool messages)
- [ ] **AC-19**: `go test ./...` passes with no regressions in agent, router, or provider tests

### Observability

- [ ] **AC-20**: Log the estimated cache savings per call (cached tokens vs total input tokens) when provider reports cache usage
- [ ] **AC-21**: Add `cache_read_tokens` and `cache_creation_tokens` to the `Usage` struct in `provider.go` for providers that report it

## Out of Scope (Phase 1)

1. **Gemini CachedContent API** — requires explicit cache lifecycle management (create, refresh, delete). Architecturally different from prefix caching. Defer to phase 2.
2. **Cross-session cache sharing** — cache keys are per-call prefix, not shareable across sessions without explicit cache IDs. Not applicable to prefix caching.
3. **Cache warming** — proactively sending a request to populate cache before the user's first message. Marginal benefit for agent workloads.
4. **Tool definition caching** — Anthropic caches tools as part of the prefix automatically. No code change needed; the tool list is already static per session.

## Risk Analysis

| Risk | Mitigation |
|---|---|
| Moving retrieved context to end reduces LLM attention to it | Retrieved context is already a secondary signal; placing it adjacent to the current message may actually improve relevance. Monitor quality. |
| Skill preprocessing as trailing user message has different semantic weight than system prompt injection | The agent loop path is unaffected (skills are in system prompt). Only the router API path changes. Monitor router-path quality. |
| Anthropic cache_control adds payload size | Negligible — two small JSON objects. No measurable impact. |
| Ollama Cloud may not honor prefix caching | Ollama Cloud uses OpenAI-compatible API which supports automatic prefix caching. Even if not supported, the message reordering has no downside. |
| Compaction frequency affects cache hit rate | Compaction only happens between pipeline phases (infrequent). Within a phase, the prefix grows monotonically — ideal for caching. |

## Implementation Order

1. **`buildMessages()` reorder** — move auto-context injection to end (highest impact, agent loop)
2. **Router memory injection** — move to end of message array (high impact, API path)
3. **Skill preprocessor** — append instead of prepend (medium impact, API path)
4. **Vertex Claude cache_control** — add breakpoints to rawPredict payload (Anthropic-specific savings)
5. **Observability** — add cache usage fields to Usage struct, log savings
6. **Tests** — verify functional equivalence and prefix stability

Estimated effort: 2-3 days implementation + 1 day testing.
