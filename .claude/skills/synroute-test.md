# Skill: SynapseRouter Testing

## Triggers
Use this skill when the user says: "test", "run tests", "verify", "check providers", "does it pass", "is it working", "test the providers", "run the tests"

## Process

### Unit Tests
1. Run `go vet ./...` to catch issues early
2. Run `go test ./...` to execute all unit tests
3. Report pass/fail summary

### E2E Provider Testing
If the user specifically asks to test providers or verify they're working:
1. Run `./synroute test --verbose` for CLI-based smoke testing
2. Or delegate to `@provider-tester` subagent for full E2E testing with server API

### After Code Changes
If tests are being run after code changes:
1. Run `go vet ./...` first
2. Run `go test -race ./...` to check for race conditions
3. If all pass, optionally suggest E2E testing with `./synroute test`
