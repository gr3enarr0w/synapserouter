# BUG-GEMINI-001 Fix Plan

Scope:
- This plan is for `BUG-GEMINI-001` only.
- Treat this as a `synroute` Gemini-adapter bug.
- Do not mark anything fixed without a live retest after each patch.

## Problem statement

Observed live behavior:
- generic `gemini-2.5-flash` sometimes succeeds while pinned `gemini-2.5-flash` fails with `429`
- in other passes, the pinned route succeeds while the generic route fails
- direct Gemini CLI usage works normally for the same user account

Conclusion:
- the bug is in `synroute`'s Gemini integration layer, not in Gemini itself

## Goal

Make generic and pinned Gemini requests behave consistently for the same model and account state.

## Non-goals

- do not redesign Gemini auth from scratch
- do not claim preview-model parity as part of this bug
- do not merge logging-only changes as fixes

## Working hypotheses

1. generic and pinned routes are not selecting credentials identically
2. generic and pinned routes are not building exactly the same upstream request
3. per-credential rate limiting is not tracked, so route behavior diverges after one credential gets throttled
4. session or project metadata differs between the two paths

## Precondition check

Before patching logic:
1. determine how many Gemini credentials actually exist in the active store
2. if there is only one credential, discard the multi-credential rotation theory
3. if there are multiple credentials, add stable fingerprint logging per credential

## Implementation plan

### Phase 1. Add deterministic tracing

Add redacted Gemini tracing that logs:
- route type: generic vs pinned
- requested model
- resolved model
- session ID
- project ID
- credential fingerprint
- upstream status code
- whether the credential was skipped due to local cooldown

Rules:
- never log raw access tokens or refresh tokens
- fingerprint should be stable and non-secret

Likely files:
- `internal/subscriptions/providers.go`
- `internal/subscriptions/provider_runtime_support.go`
- `compat_handlers.go`

### Phase 2. Prove route equivalence

Ensure both generic and pinned Gemini paths call:
- the same credential-selection function
- the same request builder
- the same upstream execution path

If there are two parallel Gemini execution paths, collapse them or make one call the other.

### Phase 3. Add per-credential rate-limit quarantine

Only if multiple credentials are present:
- when a credential gets `429`, mark it unavailable for a TTL
- skip quarantined credentials in subsequent selections
- prefer the next healthy credential
- if all are quarantined, return the last meaningful Gemini error

Suggested state per credential:
- fingerprint
- rate_limited_until
- last_status
- last_success_at

### Phase 4. Verify project/session consistency

Confirm the same values are used for generic and pinned routes:
- `sessionID`
- Gemini project ID
- OAuth credential metadata
- model normalization

### Phase 5. Add regression coverage

Add tests that assert:
1. generic and pinned Gemini route selection use the same credential-selection logic
2. a rate-limited credential is skipped after a `429`
3. credential fingerprints are stable but redacted
4. pinned and generic requests produce the same normalized upstream request for the same model

## Live verification gates

After each phase, run live checks:

1. generic stable Gemini
```bash
curl -s 'http://localhost:3000/v1/chat/completions' \
  -H 'Content-Type: application/json' \
  -d '{"model":"gemini-2.5-flash","messages":[{"role":"user","content":"Reply with exactly: gemini routed ok"}]}'
```

2. pinned stable Gemini
```bash
curl -s 'http://localhost:3000/api/provider/gemini/v1/chat/completions' \
  -H 'Content-Type: application/json' \
  -d '{"model":"gemini-2.5-flash","messages":[{"role":"user","content":"Reply with exactly: gemini provider ok"}]}'
```

Do not mark fixed unless both paths behave consistently across repeated attempts.

## Acceptance criteria

`BUG-GEMINI-001` can only be marked fixed when:
- generic and pinned stable Gemini paths use the same Gemini selection logic
- repeated live tests no longer alternate between generic success and pinned failure
- any per-credential rate limiting is handled deterministically
- the fix is verified by live retest, not inferred from code inspection

## Related bugs

- `BUG-GEMINI-002` remains separate
- preview-model parity is not part of this fix unless explicitly retested and proven
