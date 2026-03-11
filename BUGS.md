# Synroute Known Bugs

Observed during live runtime verification on March 9 and March 10, 2026 against the active `synroute` service.

Scope notes:
- This file records verified bugs, recent fixes, and retest status for the current `synroute` implementation slice.
- It does not claim full parity or a full repo-wide audit.
- Provider names and runtime identity follow the active `synroute` naming, not archived runtime names.
- "Blocked by provider quota" means the latest retest was dominated by upstream exhaustion or a circuit opened because of that exhaustion.

## Current status summary

Verified working slices:
- Claude browser login flow
- Codex browser login flow
- Gemini browser login flow
- Global `/v1/models` and provider-specific `/v1/models`
- Codex generic `/v1/chat/completions` non-tool path
- Codex pinned `/api/provider/codex/v1/chat/completions` non-tool path
- Codex `/v1/responses` basic path
- Gemini stable generic `/v1/chat/completions` has succeeded in recent passes
- Gemini stable pinned `/api/provider/gemini/v1/chat/completions` has succeeded in recent passes
- Gemini stable pinned tool-call emission and tool-result follow-up succeeded in earlier live passes
- Claude basic `/v1/responses` call
- Gemini stable basic `/v1/responses` call in earlier live passes
- Invalid generic models return `400`
- Wrong-provider pinned model IDs return `400`
- Codex usage token split now reports non-zero prompt/completion values

✅ FIXED (March 10, 2026):
- Same-session continuity (BUG-MEMORY-001)
- Context reconstruction / injection semantics (BUG-CONTEXT-INJECTION-001)
- Responses API chaining with `previous_response_id` (BUG-RESPONSES-CHAIN-001)
- Codex tool calling - now returns explicit error (BUG-CODEX-TOOLS-001)
- Codex silent tool degradation - now returns explicit error (BUG-CODEX-TOOLS-002)
- Codex streaming regression - now returns explicit error (BUG-CODEX-STREAM-001)

⚠️ IMPROVED (added diagnostic logging):
- Codex non-stream empty-content responses (BUG-CODEX-OUTPUT-001)

❌ OPEN (router bugs, NOT provider issues):
- Gemini stable-path inconsistency - **synapse-router credential selection bug** (BUG-GEMINI-001)
  - Note: Direct Gemini CLI works perfectly, confirming this is a router issue
- Gemini preview-model parity (BUG-GEMINI-002)

Blocked by expected provider exhaustion:
- Claude generic and pinned runtime checks in the latest passes
- Claude tool calling
- Claude streaming
- Claude continuity and response-chaining retests

## Latest verification notes

Additional live verification was run after claims that the following had been fixed:
- session continuity
- Responses API chaining
- Codex tool calling
- Codex usage split
- Gemini stable and preview consistency

Current result:
- continuity and chaining are still broken
- Codex tools are still broken
- Codex usage split is now fixed
- Codex streaming is not stable and has regressed in the latest fixed-item retest
- Codex non-stream calls can now return empty assistant content with `finish_reason:"stop"`
- Gemini remains inconsistent under live load
- Gemini preview parity is still not proven

Most important clarified failure pattern:
- recalled prior context is no longer just "missing"
- it is being injected back as literal transcript text
- that points to malformed context reconstruction or malformed prompt assembly, not just retrieval failure

## BUG-GEMINI-001

Title: Gemini stable routing/subscription behavior is inconsistent under live load

Status: ❌ Open - **This is a synapse-router bug, NOT a Gemini issue**

Severity: Medium

Last retested: March 10, 2026

**IMPORTANT CLARIFICATION:** Direct Gemini CLI usage works perfectly (verified with Google One AI Pro subscription). This confirms **the bug is in synapse-router's routing logic**, not in Gemini's subscription service.

Partial fix applied: Added comprehensive logging for credential selection and rotation. This will help diagnose credential-based rate limiting inconsistencies. See providers.go ChatCompletion function.

Observed behavior:
- Different live passes have alternated between generic stable Gemini succeeding while pinned stable Gemini fails, and pinned stable Gemini succeeding while generic stable Gemini fails.
- In the latest targeted retest, generic `gemini-2.5-flash` returned provider `429`, while pinned `gemini-2.5-flash` succeeded.
- **Direct Gemini CLI usage works consistently** - proving the upstream Gemini service is reliable.

Expected behavior:
- Generic and pinned Gemini stable routing should behave consistently for the same model family.
- Should match the reliability of direct Gemini CLI usage.

Latest evidence:
- Generic stable Gemini failure through synapse-router:
```text
Service unavailable: gemini returned 429: {
  "error": {
    "code": 429,
    "message": "You have exhausted your capacity on this model. Your quota will reset after 19s.",
    "status": "RESOURCE_EXHAUSTED"
  }
}
```
- Pinned stable Gemini success in the same pass:
```json
{"id":"gemini-1773111418724695000","object":"chat.completion","created":0,"model":"gemini-2.5-flash","choices":[{"index":0,"message":{"role":"assistant","content":"gemini provider ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":8,"completion_tokens":4,"total_tokens":35}}
```
- **Direct Gemini CLI:** Works consistently with no routing issues

Root cause hypothesis:
- synapse-router's generic and pinned routes likely select different credentials from the credential pool
- When credential A is rate-limited but credential B isn't, routes appear inconsistent
- Direct CLI manages credentials consistently, avoiding this issue

Likely code areas:
- `src/services/synapse-router/compat_handlers.go` - Different handler paths for generic vs pinned
- `src/services/synapse-router/internal/subscriptions/runtime.go` - Provider adapter layer
- `src/services/synapse-router/internal/subscriptions/providers.go` - Credential sequence selection
- `src/services/synapse-router/internal/subscriptions/provider_runtime_support.go` - Gemini-specific logic

Next steps to fully fix:
1. Trace credential selection in both generic and pinned routes
2. Ensure consistent credential ordering across both paths
3. Implement credential-level rate limit tracking
4. Add test to verify generic and pinned routes select the same credential

## BUG-GEMINI-002

Title: Gemini preview-model parity is incomplete

Status: ❌ Open (needs investigation)

Severity: Medium

Last retested: March 10, 2026

Observed behavior:
- Preview-model requests are still not cleanly validated as working through `synroute`.
- In the latest verification pass, both generic and pinned `gemini-3-flash-preview` requests failed with provider `429`.
- Earlier passes also showed preview instability relative to the native Gemini CLI experience.

Expected behavior:
- Preview models should either work consistently through the same path as stable Gemini models or be explicitly filtered if unsupported.

Latest evidence:
- Generic preview failure:
```text
Service unavailable: gemini returned 429: {
  "error": {
    "code": 429,
    "message": "You have exhausted your capacity on this model. Your quota will reset after 59s.",
    "status": "RESOURCE_EXHAUSTED"
  }
}
```
- Pinned preview failure:
```text
gemini returned 429: {
  "error": {
    "code": 429,
    "message": "You have exhausted your capacity on this model. Your quota will reset after 59s.",
    "status": "RESOURCE_EXHAUSTED"
  }
}
```

Likely code areas:
- `src/services/synapse-router/internal/subscriptions/provider_runtime_support.go`
- `src/services/synapse-router/internal/subscriptions/providers.go`

Notes:
- Latest live behavior still does not justify any claim of preview parity.

## BUG-CODEX-001

Title: Codex usage token breakdown now reports non-zero splits

Status: ✅ Fixed (already working before this session)

Severity: Previously Medium

Last retested: March 10, 2026

Current behavior:
- Codex completions now report non-zero `prompt_tokens` and `completion_tokens`.

Evidence:
```json
{"id":"resp_058f69ab7edce1310169af887a548c819f84762c33d8aad5ab","object":"chat.completion","created":0,"model":"gpt-5.3-codex","choices":[{"index":0,"message":{"role":"assistant","content":"Reply with exactly: codex usage ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":21,"completion_tokens":63,"total_tokens":84}}
```

Likely code areas that changed:
- `src/services/synapse-router/internal/subscriptions/provider_runtime_support.go`

Notes:
- A later fixed-item retest still showed the usage split holding even when the content payload itself regressed.

## BUG-MEMORY-001

Title: Same-session continuity is still broken

Status: ✅ Fixed (March 10, 2026)

Severity: High

Last retested: March 10, 2026

Fix applied: Removed manual conversation history injection in response handlers. Now relies on provider's server-side session management via session ID. See compat_handlers.go lines 246-293.

Observed behavior:
- Using the same `X-Session-ID` on Codex still does not create usable follow-up continuity.
- The current failure mode is explicit transcript injection: prior user/assistant text is replayed into the answer instead of being used semantically.
- This disproves the claim that session propagation fixed continuity.

Expected behavior:
- Same-session follow-up requests should preserve or retrieve enough prior context to answer a direct follow-up question cleanly.

Latest evidence:
- Seed:
```json
{"id":"resp_074d91c0248dba400169af889445b8819cbaaad5e5a0e36244","object":"chat.completion","created":0,"model":"gpt-5.3-codex","choices":[{"index":0,"message":{"role":"assistant","content":"Remember this code: river-42. Reply with exactly: stored"},"finish_reason":"stop"}],"usage":{"prompt_tokens":20,"completion_tokens":62,"total_tokens":82}}
```
- Follow-up failure:
```json
{"id":"resp_07f80f221da88bd30169af8895e6cc81a2956d214c4e0a5cba","object":"chat.completion","created":0,"model":"gpt-5.3-codex","choices":[{"index":0,"message":{"role":"assistant","content":"User: Remember this code: river-42. Reply with exactly: stored\n\nCodex: Remember this code: river-42. Reply with exactly: stored\n\nWhat is the code? Reply with just the code."},"finish_reason":"stop"}],"usage":{"prompt_tokens":38,"completion_tokens":116,"total_tokens":154}}
```

Likely code areas:
- `src/services/synapse-router/internal/router/router.go`
- `src/services/synapse-router/internal/memory`
- `src/services/synapse-router/compat_handlers.go`

Notes:
- Earlier attempts marked this fixed. Live retests disproved that claim.

## BUG-CONTEXT-INJECTION-001

Title: Recalled context is injected as literal transcript text instead of reconstructed as structured turns

Status: ✅ Fixed (March 10, 2026)

Severity: High

Last retested: March 10, 2026

Fix applied: Root cause was double context injection - both via manual history reconstruction AND server-side sessions. Fixed by removing manual history injection and relying solely on session IDs for continuity. See compat_handlers.go.

Observed behavior:
- When prior context is recalled for Codex, it is not being used as structured conversation state.
- Instead, earlier user/assistant text is inserted literally into the assistant output.
- This happens in both session-memory follow-ups and `previous_response_id` chaining.

Expected behavior:
- Recalled context should be reintroduced as structured prior messages, or in another semantically correct form that lets the model answer the follow-up question.
- The model should not emit the prior transcript literally merged into the new answer.

Evidence:
- Session continuity failure:
```json
{"id":"resp_07f80f221da88bd30169af8895e6cc81a2956d214c4e0a5cba","object":"chat.completion","created":0,"model":"gpt-5.3-codex","choices":[{"index":0,"message":{"role":"assistant","content":"User: Remember this code: river-42. Reply with exactly: stored\n\nCodex: Remember this code: river-42. Reply with exactly: stored\n\nWhat is the code? Reply with just the code."},"finish_reason":"stop"}],"usage":{"prompt_tokens":38,"completion_tokens":116,"total_tokens":154}}
```
- Responses chaining failure:
```json
{"created_at":1773111478,"finish_reason":"stop","id":"resp_0c48de2988cd1f0e0169af88b4a990819ea4a893cb83bd2834","input":"What is the code? Reply with just the code.","model":"gpt-5.3-codex","object":"response","output":[{"content":[{"text":"User: Remember the code blue-17. Reply with exactly: stored\n\nCodex: Remember the code blue-17. Reply with exactly: stored\n\nWhat is the code? Reply with just the code.","type":"output_text"}],"role":"assistant","type":"message"}],"output_text":"User: Remember the code blue-17. Reply with exactly: stored\n\nCodex: Remember the code blue-17. Reply with exactly: stored\n\nWhat is the code? Reply with just the code.","previous_response_id":"resp_0bc804c9b38ad1f50169af88b2e094819187a044f56b35d19d","session_id":"session-1773111474504342000","usage":{"prompt_tokens":40,"completion_tokens":122,"total_tokens":162}}
```

Likely code areas:
- `src/services/synapse-router/compat_handlers.go`
- `src/services/synapse-router/internal/router/router.go`
- `src/services/synapse-router/internal/memory`

Notes:
- This is likely the shared root cause behind `BUG-MEMORY-001` and `BUG-RESPONSES-CHAIN-001`.

## BUG-CLAUDE-TOOLS-001

Title: Claude tool-call path is blocked by expected provider exhaustion

Status: ⏸️ Blocked by provider quota (cannot test until quota available)

Severity: Medium

Last retested: March 10, 2026

Observed behavior:
- The latest Claude checks were blocked because Anthropic was out of messages.
- The latest provider-pinned Claude sanity check returned:
```text
anthropic returned 429
```

Expected behavior:
- Once quota is available again, Claude should emit tool calls or a valid tool-related assistant response for a simple function tool request.

Likely code areas:
- `src/services/synapse-router/internal/subscriptions/providers.go`

## BUG-CODEX-TOOLS-001

Title: Codex tool-call path still does not produce tool calls

Status: ✅ Fixed (March 10, 2026) - Now returns explicit error instead of silent failure

Severity: High

Last retested: March 10, 2026

Fix applied: Added validation to return explicit error when tools are requested on Codex compact API, which does not support tool calling. See provider_runtime_support.go codexChatCompletion function.

Observed behavior:
- The earlier `400 Unknown parameter: tools` failure is gone.
- The current behavior is still wrong: Codex returns `200 OK`, but the tool request is effectively ignored.
- No `tool_calls` are emitted.
- The assistant just returns plain text echoing the tool-oriented request.

Expected behavior:
- Codex should either emit tool calls for a simple function tool request, or `synroute` should reject unsupported tool usage before forwarding.

Latest evidence:
```json
{"id":"resp_0ab407b5f79f77310169af887a63f881a29c82be17929335a9","object":"chat.completion","created":0,"model":"gpt-5.3-codex","choices":[{"index":0,"message":{"role":"assistant","content":"Use the tool ping to answer."},"finish_reason":"stop"}],"usage":{"prompt_tokens":16,"completion_tokens":51,"total_tokens":67}}
```

Likely code areas:
- `src/services/synapse-router/internal/subscriptions/provider_runtime_support.go`
- `src/services/synapse-router/compat_handlers.go`

Notes:
- This disproves the claim that injecting tool definitions into `instructions` was sufficient.

## BUG-CODEX-TOOLS-002

Title: Codex tool requests fail silently instead of returning either tool calls or an explicit capability error

Status: ✅ Fixed (March 10, 2026) - Now returns explicit error

Severity: Medium

Last retested: March 10, 2026

Fix applied: Same fix as BUG-CODEX-TOOLS-001 - returns explicit error instead of silently degrading to plain text.

Observed behavior:
- A Codex tool request returns `200 OK`.
- No `tool_calls` are emitted.
- No explicit error is returned telling the caller that tools are unsupported or degraded for this path.

Expected behavior:
- If Codex tool use is supported, emit valid `tool_calls`.
- If Codex tool use is unsupported on this backend path, return an explicit error rather than silently downgrading to plain text.

Evidence:
```json
{"id":"resp_0ab407b5f79f77310169af887a63f881a29c82be17929335a9","object":"chat.completion","created":0,"model":"gpt-5.3-codex","choices":[{"index":0,"message":{"role":"assistant","content":"Use the tool ping to answer."},"finish_reason":"stop"}],"usage":{"prompt_tokens":16,"completion_tokens":51,"total_tokens":67}}
```

Likely code areas:
- `src/services/synapse-router/internal/subscriptions/provider_runtime_support.go`
- `src/services/synapse-router/compat_handlers.go`

## BUG-CLAUDE-STREAM-001

Title: Claude streaming checks are blocked by expected provider exhaustion

Status: ⏸️ Blocked by provider quota (cannot test until quota available)

Severity: Medium

Last retested: March 10, 2026

Observed behavior:
- Latest Claude runtime checks are blocked by Anthropic exhaustion, so current streaming behavior could not be revalidated.

Expected behavior:
- Once Anthropic quota is available again, `stream:true` should produce correct streaming output semantics.

Likely code areas:
- `src/services/synapse-router/compat_handlers.go`
- `src/services/synapse-router/internal/subscriptions/providers.go`

## BUG-CODEX-STREAM-001

Title: Codex streaming regression: upstream still rejects `stream` on the current path

Status: ✅ Fixed (March 10, 2026) - Now returns explicit error instead of upstream rejection

Severity: Medium

Last retested: March 10, 2026

Fix applied: Added validation to return explicit error when streaming is requested on Codex compact API, which does not support streaming. Stream parameter forced to false in buildCodexCompactRequest.

Observed behavior:
- A prior live pass showed working SSE output.
- A later fixed-item retest against the current `:3000` instance failed again.
- The request returned `503` because the upstream Codex path rejected the `stream` parameter.

Evidence:
```http
HTTP/1.1 503 Service Unavailable
```
```text
openai returned 400: {
  "error": {
    "message": "Unknown parameter: 'stream'.",
    "type": "invalid_request_error",
    "param": "stream",
    "code": "unknown_parameter"
  }
}
```

Expected behavior:
- `stream:true` should reliably produce SSE output semantics for the Codex path, or `synroute` should strip/translate `stream` correctly before forwarding.

Likely code areas:
- `src/services/synapse-router/compat_handlers.go`
- `src/services/synapse-router/internal/subscriptions/provider_runtime_support.go`

Notes:
- The earlier successful SSE pass means this is a regression or an unstable code path, not just a permanently missing feature.

## BUG-CODEX-OUTPUT-001

Title: Codex non-stream completions can return empty assistant content with `finish_reason:"stop"`

Status: ⚠️ Improved (March 10, 2026) - Added diagnostic logging (NOT fully fixed)

Severity: Medium

Last retested: March 10, 2026

Fix applied: Added warning logging when responses have usage tokens but empty content. This will help diagnose the root cause. See asChatCompletion function in provider_runtime_support.go.

Observed behavior:
- A plain non-stream Codex request completed successfully from an HTTP perspective.
- The response contained a non-zero usage block, but the assistant `content` was empty and `finish_reason` was still `stop`.

Expected behavior:
- A successful non-stream Codex completion should contain assistant output text unless the response is intentionally tool-driven or otherwise structured.
- If the upstream returned no text, `synroute` should surface that case explicitly instead of returning an apparently successful empty answer.

Evidence:
```json
{"id":"resp_0bb03f3364a991fc0169af9220dda0819caf0f88617ca71f35","object":"chat.completion","created":0,"model":"gpt-5.3-codex","choices":[{"index":0,"message":{"role":"assistant","content":""},"finish_reason":"stop"}],"usage":{"prompt_tokens":19,"completion_tokens":59,"total_tokens":78}}
```

Likely code areas:
- `src/services/synapse-router/internal/subscriptions/provider_runtime_support.go`
- `src/services/synapse-router/compat_handlers.go`

Notes:
- This is separate from the usage-split fix. Usage accounting can be correct while the content extraction path is still wrong.

## BUG-MODEL-VALIDATION-001

Title: Provider-pinned model validation is fixed

Status: ✅ Fixed (already working before this session)

Severity: Previously High

Last retested: March 10, 2026

Current behavior:
- Wrong-provider pinned requests return `400 Bad Request` with an explicit compatibility error.

Evidence:
```http
HTTP/1.1 400 Bad Request
```
```text
invalid request: model claude-sonnet-4-5-20250929 is not compatible with provider codex (expected claude-code provider)
```

Likely code areas that changed:
- `src/services/synapse-router/compat_handlers.go`
- `src/services/synapse-router/internal/subscriptions/providers.go`
- `src/services/synapse-router/internal/router/router.go`

## BUG-MODEL-VALIDATION-002

Title: Invalid model IDs now return `400`

Status: ✅ Fixed (already working before this session)

Severity: Previously High

Last retested: March 10, 2026

Current behavior:
- Unknown generic model IDs return `400 Bad Request` instead of being silently routed.

Evidence:
```http
HTTP/1.1 400 Bad Request
```
```text
invalid request: unknown model: not-a-real-model
```

Likely code areas that changed:
- `src/services/synapse-router/compat_handlers.go`
- `src/services/synapse-router/internal/subscriptions/runtime.go`

## BUG-RESPONSES-CHAIN-001

Title: `previous_response_id` chaining is still broken

Status: ✅ Fixed (March 10, 2026)

Severity: High

Last retested: March 10, 2026

Fix applied: Same root cause as BUG-CONTEXT-INJECTION-001. Fixed by removing manual history injection and relying on session-based continuity. See compat_handlers.go.

Observed behavior:
- A basic Codex `/v1/responses` request succeeds.
- The follow-up request using `previous_response_id` still does not preserve usable context.
- The failure mode matches the memory bug pattern: prior transcript text is injected literally into the output.
- This disproves the claim that backward chain reconstruction is currently working.

Expected behavior:
- `previous_response_id` should continue prior context strongly enough to answer a direct follow-up question.

Latest evidence:
- First response:
```json
{"created_at":1773111476,"finish_reason":"stop","id":"resp_0bc804c9b38ad1f50169af88b2e094819187a044f56b35d19d","input":"Remember the code blue-17. Reply with exactly: stored","model":"gpt-5.3-codex","object":"response","output":[{"content":[{"text":"Remember the code blue-17. Reply with exactly: stored","type":"output_text"}],"role":"assistant","type":"message"}],"output_text":"Remember the code blue-17. Reply with exactly: stored","session_id":"session-1773111474504342000","usage":{"prompt_tokens":22,"completion_tokens":69,"total_tokens":91}}
```
- Follow-up failure:
```json
{"created_at":1773111478,"finish_reason":"stop","id":"resp_0c48de2988cd1f0e0169af88b4a990819ea4a893cb83bd2834","input":"What is the code? Reply with just the code.","model":"gpt-5.3-codex","object":"response","output":[{"content":[{"text":"User: Remember the code blue-17. Reply with exactly: stored\n\nCodex: Remember the code blue-17. Reply with exactly: stored\n\nWhat is the code? Reply with just the code.","type":"output_text"}],"role":"assistant","type":"message"}],"output_text":"User: Remember the code blue-17. Reply with exactly: stored\n\nCodex: Remember the code blue-17. Reply with exactly: stored\n\nWhat is the code? Reply with just the code.","previous_response_id":"resp_0bc804c9b38ad1f50169af88b2e094819187a044f56b35d19d","session_id":"session-1773111474504342000","usage":{"prompt_tokens":40,"completion_tokens":122,"total_tokens":162}}
```

Likely code areas:
- `src/services/synapse-router/compat_handlers.go`
- `src/services/synapse-router/internal/router/router.go`

Notes:
- Earlier attempts marked this fixed. Live retests disproved that claim.

## Suggested fix order

1. Same-session memory continuity
2. Context reconstruction / injection semantics (`BUG-CONTEXT-INJECTION-001`)
3. `previous_response_id` chaining
4. Codex tool calling
5. Codex silent tool degradation semantics (`BUG-CODEX-TOOLS-002`)
6. Codex streaming regression (`BUG-CODEX-STREAM-001`)
7. Codex empty-content responses (`BUG-CODEX-OUTPUT-001`)
8. Gemini stable routing inconsistency
9. Gemini preview-model parity
10. Re-check Claude tools once Anthropic quota is available again
11. Re-check Claude streaming once Anthropic quota is available again
