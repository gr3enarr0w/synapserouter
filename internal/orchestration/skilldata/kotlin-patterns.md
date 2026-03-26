---
name: kotlin-patterns
description: "Idiomatic Kotlin — coroutines, null safety, data classes, Ktor, Jetpack Compose."
triggers:
  - "kotlin"
  - ".kt"
  - ".kts"
  - "ktor"
  - "jetpack"
  - "compose"
  - "kotlin+android"
  - "gradle+kotlin"
role: coder
phase: analyze
language: kotlin
mcp_tools:
  - "context7.query-docs"
verify:
  - name: "gradle build"
    command: "gradle build 2>&1 || ./gradlew build 2>&1 || echo 'BUILD_FAILED'"
    expect_not: "BUILD_FAILED"
  - name: "no force non-null"
    command: "grep -rn '!!' --include='*.kt' | grep -v 'test\\|Test\\|//' | head -5 || echo 'OK'"
    expect: "OK"
---
# kotlin-patterns

## Core Principles
1. **Null safety** — ?, ?., ?:, let/also/apply/run scope functions
2. **Data classes** — automatic equals/hashCode/copy/toString
3. **Sealed classes** — exhaustive when expressions
4. **Coroutines** — suspend functions, Flow, StateFlow, launch/async
5. **Extension functions** — extend without inheritance
6. **DSL builders** — type-safe builders with receivers

## Patterns
- Companion objects for factory methods
- Ktor for server (routing, serialization, auth)
- Jetpack Compose (@Composable, State, remember, LaunchedEffect)
- Kotlin serialization over Gson/Jackson
- Flow for reactive streams, StateFlow for UI state
- Scope functions: let (transform), also (side effect), apply (configure), run (compute)

## Anti-Patterns
- !! operator (force non-null) — use safe calls or require()
- Java-style getters/setters — use properties
- Blocking coroutine scope (runBlocking in production)
- Mutable global state — use StateFlow or sealed state
