---
name: rust-patterns
description: "Idiomatic Rust — ownership, memory safety, concurrency, error handling, Cargo."
triggers:
  - "rust"
  - ".rs"
  - "cargo"
role: coder
phase: analyze
language: rust
mcp_tools:
  - "context7.query-docs"
verify:
  - name: "Cargo.lock exists"
    command: "test -f Cargo.lock && echo 'OK' || echo 'MISSING'"
    expect_not: "MISSING"
  - name: "README exists"
    command: "test -f README.md && echo 'OK' || echo 'MISSING'"
    expect_not: "MISSING"
---
# Skill: Rust Patterns

Idiomatic Rust — ownership, memory safety, concurrency, error handling, Cargo.

Source: [Rust Best Practices](https://mcpmarket.com/es/tools/skills/rust-best-practices), [Rust Development Workflow](https://mcpmarket.com/es/tools/skills/rust-development-workflow).

---

## When to Use

- Writing or reviewing Rust code
- Designing ownership/borrowing patterns
- Concurrent/parallel Rust programs
- Cargo project setup and dependency management

---

## Core Rules

1. **Ownership is the type system** — think about who owns what
2. **Prefer borrowing** — `&T` and `&mut T` over cloning
3. **Result<T, E> for errors** — `?` operator for propagation
4. **Enums for state machines** — exhaustive pattern matching
5. **Traits for abstraction** — not inheritance
6. **clippy + fmt** — run before every commit
7. **No unwrap() in production** — use `?`, `unwrap_or`, `expect` with message

---

## Patterns

### Error handling with thiserror
```rust
use thiserror::Error;

#[derive(Error, Debug)]
pub enum AppError {
    #[error("ticket {0} not found")]
    NotFound(String),
    #[error("API request failed: {0}")]
    ApiError(#[from] reqwest::Error),
    #[error("database error: {0}")]
    DbError(#[from] rusqlite::Error),
}
```

### Builder pattern
```rust
pub struct QueryBuilder {
    project: Option<String>,
    status: Option<String>,
    limit: usize,
}

impl QueryBuilder {
    pub fn new() -> Self { Self { project: None, status: None, limit: 50 } }
    pub fn project(mut self, p: &str) -> Self { self.project = Some(p.to_string()); self }
    pub fn status(mut self, s: &str) -> Self { self.status = Some(s.to_string()); self }
    pub fn build(self) -> Query { /* ... */ }
}
```

### Concurrent processing with rayon
```rust
use rayon::prelude::*;

let results: Vec<Classification> = tickets
    .par_iter()
    .map(|ticket| classify(ticket))
    .collect();
```

---

## Quality Gates

```bash
cargo fmt --check        # Formatting
cargo clippy -- -D warnings  # Linting
cargo test               # Tests
cargo test -- --ignored  # Integration tests
cargo audit              # Security vulnerabilities
```
