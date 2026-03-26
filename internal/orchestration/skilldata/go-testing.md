---
name: go-testing
description: "Go testing — table-driven tests, benchmarks, race detection, fuzzing."
triggers:
  - "test"
  - "verify"
  - "validate"
  - "coverage"
role: tester
phase: verify
language: go
mcp_tools:
  - "context7.query-docs"
depends_on:
  - "code-implement"
verify:
  - name: "tests pass"
    command: "go test ./... 2>&1 | tail -10"
    expect: "ok"
  - name: "tests exist"
    command: "find . -name '*_test.go' -not -path '*/vendor/*' | head -5 || echo 'MISSING'"
    expect_not: "MISSING"
  - name: "table-driven tests"
    command: "grep -rn 'tests := \\[\\]struct\\|tt := range\\|tc := range' --include='*_test.go' | head -3 || echo 'MISSING'"
    manual: "Verify test files use table-driven test pattern ([]struct with t.Run subtests). Not required for every test, but complex logic should be table-driven."
  - name: "no t.Fatal in goroutines"
    command: "grep -rn 't.Fatal\\|t.Fatalf' --include='*_test.go' | while read line; do file=$(echo $line | cut -d: -f1); lineno=$(echo $line | cut -d: -f2); sed -n \"$((lineno-10)),$((lineno))p\" $file | grep -q 'go func' && echo \"WARN: t.Fatal inside goroutine at $line\"; done || echo 'OK'"
    expect_not: "WARN"
  - name: "race detection"
    command: "go test -race ./... 2>&1 | tail -5"
    expect: "ok"
---
# Skill: Go Testing

Go testing best practices — table-driven tests, benchmarks, race detection, fuzzing.

Source: [affaan-m/golang-testing](https://github.com/affaan-m/everything-claude-code/tree/main/skills/golang-testing) (70K stars).

---

## When to Use

- Writing or reviewing Go tests
- Setting up benchmarks
- Debugging race conditions
- Fuzzing for edge cases

---

## Core Rules

1. **Table-driven tests** — standard Go idiom for test variants
2. **`t.Helper()`** — mark helper functions for better stack traces
3. **`-race` flag** — always run with race detection in CI
4. **Subtests** — `t.Run("case", func(t *testing.T) {...})`
5. **`testdata/`** — fixtures in testdata/ directory (ignored by build)
6. **No testify** unless needed — stdlib `testing` is usually sufficient

---

## Patterns

### Table-driven tests
```go
func TestScrubPII(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected string
    }{
        {"email", "user@example.com", "[REDACTED]"},
        {"mention", "[~username]", "[REDACTED]"},
        {"clean", "no pii here", "no pii here"},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := ScrubPII(tt.input)
            if got != tt.expected {
                t.Errorf("ScrubPII(%q) = %q, want %q", tt.input, got, tt.expected)
            }
        })
    }
}
```

### Benchmarks
```go
func BenchmarkScrubPII(b *testing.B) {
    input := "Contact user@example.com for details"
    for i := 0; i < b.N; i++ {
        ScrubPII(input)
    }
}
// Run: go test -bench=. -benchmem
```

### Fuzzing
```go
func FuzzScrubPII(f *testing.F) {
    f.Add("user@example.com")
    f.Add("[~admin]")
    f.Fuzz(func(t *testing.T, input string) {
        result := ScrubPII(input)
        if strings.Contains(result, "@") && strings.Contains(input, "@") {
            t.Errorf("PII not scrubbed: %q → %q", input, result)
        }
    })
}
// Run: go test -fuzz=FuzzScrubPII
```

---

## Running Tests

```bash
go test ./...                    # All tests
go test -v ./...                 # Verbose
go test -race ./...              # Race detection
go test -bench=. -benchmem       # Benchmarks
go test -fuzz=FuzzName           # Fuzzing
go test -cover -coverprofile=c.out  # Coverage
go tool cover -html=c.out        # Coverage HTML report
```
