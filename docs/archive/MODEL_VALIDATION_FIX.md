# Model Validation Fix

## Fixes Applied

This document describes the fixes for **BUG-MODEL-VALIDATION-001** and **BUG-MODEL-VALIDATION-002** in the Synapse Router.

## Problems Fixed

### BUG-MODEL-VALIDATION-001: Provider-pinned routes silently coerced incompatible models
**Status**: ✅ FIXED

**Previous Behavior**:
- A pinned Codex route accepted a Claude model ID and returned a Codex completion
- A pinned Claude route accepted a Codex model ID and returned a Claude completion
- Both returned HTTP 200 OK

**New Behavior**:
- Provider-pinned routes now validate model compatibility
- Incompatible model IDs are rejected with HTTP 400 and a clear error message
- Example error: `model claude-sonnet-4-5-20250929 is not compatible with provider codex (expected claude-code provider)`

### BUG-MODEL-VALIDATION-002: Invalid model IDs returned 200 OK and got silently routed
**Status**: ✅ FIXED

**Previous Behavior**:
- Generic request with `model:"not-a-real-model"` returned HTTP 200 OK and a Claude completion
- Unknown models silently routed to default provider/model

**New Behavior**:
- Unknown model IDs now return HTTP 400 with clear error message
- Example error: `invalid request: unknown model: not-a-real-model`
- Exception: If AMP upstream is configured, unknown models are allowed (fallback behavior)

## Implementation Details

### Changes Made

1. **Added `validateModelForProvider()` function** (`compat_handlers.go:800-833`)
   - Checks if model exists in registry
   - Validates model-provider compatibility for pinned routes
   - Respects AMP fallback configuration

2. **Updated `routeChatRequest()` function** (`compat_handlers.go:363-379`)
   - Calls validation before routing
   - Returns validation errors to caller

3. **Updated error handling** (`compat_handlers.go:329-365`, `main.go:454-462`)
   - Distinguishes between validation errors (400) and service errors (503)
   - Returns proper HTTP status codes based on error type

4. **Added fmt import** (`compat_handlers.go:2-20`)
   - Required for error formatting

5. **Updated runtime adapter** (`internal/subscriptions/runtime.go:21-31`)
   - Made model coercion behavior more explicit
   - Added comments explaining fallback logic

### Test Coverage

Created comprehensive test suite (`model_validation_test.go`):
- ✅ Invalid model returns 400
- ✅ Wrong provider/model combination returns 400
- ✅ Valid models work correctly
- ✅ Auto model works on any provider
- ✅ AMP fallback allows unknown models when configured
- ✅ Provider detection logic is accurate

All tests pass (verified 2026-03-09).

## Usage Examples

### Before Fix

```bash
# Invalid model - returned 200 OK (WRONG)
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"not-a-real-model","messages":[{"role":"user","content":"test"}]}'
# Response: HTTP 200, Claude completion

# Wrong provider - returned 200 OK (WRONG)
curl -X POST http://localhost:8080/api/provider/codex/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"claude-sonnet-4-5-20250929","messages":[{"role":"user","content":"test"}]}'
# Response: HTTP 200, Codex completion
```

### After Fix

```bash
# Invalid model - returns 400 (CORRECT)
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"not-a-real-model","messages":[{"role":"user","content":"test"}]}'
# Response: HTTP 400, "invalid request: unknown model: not-a-real-model"

# Wrong provider - returns 400 (CORRECT)
curl -X POST http://localhost:8080/api/provider/codex/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"claude-sonnet-4-5-20250929","messages":[{"role":"user","content":"test"}]}'
# Response: HTTP 400, "invalid request: model claude-sonnet-4-5-20250929 is not compatible with provider codex (expected claude-code provider)"

# Valid requests still work
curl -X POST http://localhost:8080/api/provider/codex/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-5.3-codex","messages":[{"role":"user","content":"test"}]}'
# Response: HTTP 200, Codex completion
```

## Edge Cases Handled

1. **Empty/Auto Models**: Continue to work as before (use provider default)
2. **AMP Fallback**: Unknown models allowed when AMP upstream is configured
3. **Case Insensitivity**: Model/provider matching is case-insensitive
4. **Provider Name Aliases**: "anthropic" maps to "claude-code", "openai" maps to "codex"

## Files Modified

- `src/services/synapse-router/compat_handlers.go` - Validation logic and error handling
- `src/services/synapse-router/main.go` - Error handling in chatHandler
- `src/services/synapse-router/internal/subscriptions/runtime.go` - Explicit model coercion
- `src/services/synapse-router/model_validation_test.go` - New test file

## Verification

To verify the fixes are working:

```bash
# Run the test suite
cd src/services/synapse-router
go test -v -run TestModelValidation

# Or test manually with curl (requires running server)
./test-model-validation.sh
```

## Related Bugs

These fixes also improve error reporting for:
- BUG-GEMINI-001 (routing inconsistency now fails fast with clear errors)
- BUG-CODEX-TOOLS-001 (tools payload validation will be easier to add)
- BUG-CLAUDE-TOOLS-001 (tools payload validation will be easier to add)

## Next Steps

1. ✅ Model validation (COMPLETE)
2. 🔄 Fix memory continuity (BUG-MEMORY-001)
3. 🔄 Fix Responses API chaining (BUG-RESPONSES-CHAIN-001)
4. 🔄 Fix tool calling
5. 🔄 Fix streaming support
