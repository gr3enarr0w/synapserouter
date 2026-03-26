---
name: rust-testing
description: "Rust testing — cargo test, proptest, criterion benchmarks, integration tests."
triggers:
  - "rust+test"
  - "cargo test"
  - "proptest"
  - "criterion"
  - "rust+spec"
role: tester
phase: verify
language: rust
verify:
  - name: "cargo test"
    command: "cargo test 2>&1 | tail -10"
    expect: "test result: ok"
  - name: "cargo clippy"
    command: "cargo clippy -- -D warnings 2>&1 || echo 'CLIPPY_WARN'"
    expect_not: "CLIPPY_WARN"
---
# rust-testing

## Unit Tests
- #[test] functions in #[cfg(test)] modules
- assert!, assert_eq!, assert_ne! macros
- #[should_panic] for expected panics
- #[ignore] for slow tests (run with --ignored)

## Integration Tests
- tests/ directory (separate crate, tests public API)
- Each file is a separate test binary
- Common setup in tests/common/mod.rs

## Property Testing (proptest)
- proptest! macro for generative testing
- prop_assert! for property assertions
- Strategy composition for complex inputs

## Benchmarks (criterion)
- criterion_group! and criterion_main! macros
- Statistical analysis with confidence intervals
- Comparison benchmarks with bench_function

## Async Testing
- #[tokio::test] for async tests
- #[actix_rt::test] for Actix-web

## Anti-Patterns
- unwrap() in tests without context (use expect("msg"))
- No integration tests (only unit tests)
- Ignoring clippy warnings
- Testing private internals instead of public API
