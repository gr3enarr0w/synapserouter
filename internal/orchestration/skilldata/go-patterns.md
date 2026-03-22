---
name: go-patterns
description: "Idiomatic Go development — concurrency, error handling, interfaces, modules."
triggers:
  - "go"
  - "golang"
  - ".go"
role: coder
phase: analyze
mcp_tools:
  - "context7.query-docs"
verify:
  - name: "race detection"
    command: "go test -race ./... 2>&1 | tail -5"
    expect: "ok"
  - name: "go vet"
    command: "go vet ./... 2>&1"
    expect: ""
  - name: "defer in loop"
    command: "grep -rn 'defer' --include='*.go' | while read line; do file=$(echo $line | cut -d: -f1); lineno=$(echo $line | cut -d: -f2); sed -n \"$((lineno-5)),$((lineno))p\" $file | grep -q 'for ' && echo \"WARN: defer inside loop at $line\"; done || echo 'OK'"
    expect_not: "WARN"
  - name: "naked goroutines"
    command: "grep -rn 'go func' --include='*.go' | grep -v 'WaitGroup\\|errgroup\\|defer.*Done\\|recover' || echo 'OK'"
    expect: "OK"
  - name: "error handling"
    command: "grep -rn 'err != nil' --include='*.go' | grep -v 'return\\|log\\|fmt' | head -5 || echo 'OK'"
    expect: "OK"
  - name: "regex compiled per-call"
    command: "grep -rn 'regexp.MustCompile\\|regexp.Compile' --include='*.go' | grep -v 'var \\|_test.go' || echo 'OK'"
    expect: "OK"
    manual: "All regexp.MustCompile calls should be package-level var declarations, not inside functions"
  - name: "goroutine lifecycle"
    command: "grep -rn 'go func\\|signal.Notify' --include='*.go' | grep -v '_test.go' || echo 'OK'"
    manual: "Verify every goroutine has a clean exit path. Signal handlers must not block forever on normal runs. Check for wg.Wait after goroutines that only exit on signal."
  - name: "double append pattern"
    command: "grep -rn 'append.*steps\\|append.*result\\|append.*items' --include='*.go' | grep -v '_test.go' | head -20"
    manual: "Check for state machine parsers that both set currentItem AND append immediately. A value should be appended exactly once — either via deferred append or immediate append, not both."
  - name: "xml attribute casing"
    command: "grep -rn 'xml:\".*,attr' --include='*.go' | head -20"
    manual: "Verify XML attribute names match the target schema casing. ZWO uses PascalCase (Duration, PowerLow, PowerHigh). Check xml struct tags match exactly."
  - name: "README exists"
    command: "test -f README.md && echo 'OK' || echo 'MISSING'"
    expect_not: "MISSING"
---
# Skill: Go Patterns

Idiomatic Go development — concurrency, error handling, interfaces, modules.

Source: [affaan-m/golang-patterns](https://github.com/affaan-m/everything-claude-code/tree/main/skills/golang-patterns) (70K stars), [Go Concurrency Master](https://mcpmarket.com/tools/skills/go-concurrency-master).

---

## When to Use

- Writing or reviewing Go code
- Designing concurrent systems
- Structuring Go projects
- Error handling patterns

---

## Core Rules

1. **Accept interfaces, return structs** — dependency inversion
2. **Errors are values** — check and handle, never ignore
3. **Small interfaces** — `io.Reader` (1 method) not `AllDoer` (20 methods)
4. **Goroutines + channels** — prefer channels over shared memory
5. **`context.Context` everywhere** — first parameter for cancellation/timeout
6. **No init()** — explicit initialization in main
7. **Table-driven tests** — standard Go testing pattern

---

## Project Structure

```
project/
├── cmd/
│   └── myapp/
│       └── main.go         # Entry point
├── internal/               # Private packages
│   ├── handler/
│   ├── service/
│   └── repository/
├── pkg/                    # Public packages
├── go.mod
└── go.sum
```

---

## Patterns

### Error handling with context
```go
if err := doSomething(); err != nil {
    return fmt.Errorf("doSomething failed: %w", err)
}
```

### Concurrency with errgroup
```go
g, ctx := errgroup.WithContext(ctx)
for _, url := range urls {
    url := url
    g.Go(func() error {
        return fetch(ctx, url)
    })
}
if err := g.Wait(); err != nil {
    return err
}
```

### Functional options
```go
type Option func(*Server)

func WithPort(port int) Option {
    return func(s *Server) { s.port = port }
}

func NewServer(opts ...Option) *Server {
    s := &Server{port: 8080}
    for _, opt := range opts {
        opt(s)
    }
    return s
}
```

### Interface-based testing
```go
type TicketStore interface {
    Get(ctx context.Context, key string) (*Ticket, error)
}

// In tests:
type mockStore struct{ tickets map[string]*Ticket }
func (m *mockStore) Get(_ context.Context, key string) (*Ticket, error) {
    t, ok := m.tickets[key]
    if !ok { return nil, ErrNotFound }
    return t, nil
}
```

---

## Anti-Patterns

- Naked goroutines (always track with WaitGroup or errgroup)
- `panic()` for recoverable errors (return errors instead)
- Package-level variables for state (pass dependencies explicitly)
- Overly broad interfaces (keep them small and focused)
