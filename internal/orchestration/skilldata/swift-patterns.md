---
name: swift-patterns
description: "Idiomatic Swift / iOS development — SwiftUI, async/await, protocols, SPM."
triggers:
  - "swift"
  - "swiftui"
  - "ios"
  - "xcode"
  - ".swift"
  - "spm"
  - "swift package"
  - "uikit"
  - "cocoapods"
role: coder
phase: analyze
language: swift
mcp_tools:
  - "context7.query-docs"
verify:
  - name: "swift build"
    command: "swift build 2>&1 || echo 'BUILD_FAILED'"
    expect_not: "BUILD_FAILED"
  - name: "no force unwraps"
    command: "grep -rn '![^=]' --include='*.swift' | grep -v 'IBOutlet\\|IBAction\\|test\\|Test\\|//' | head -5 || echo 'OK'"
    expect: "OK"
---
# swift-patterns

## Core Principles
1. **Value types over reference** — prefer structs over classes
2. **Protocol-oriented** — compose via protocols, not inheritance
3. **Optionals** — guard let, if let, never force unwrap (!) in production
4. **async/await** — structured concurrency with Task, TaskGroup, actors
5. **SwiftUI** — declarative UI with @State, @Binding, @Observable
6. **Error handling** — do/try/catch, typed throws

## Patterns
- Codable for JSON, Result for error propagation
- @MainActor for UI-thread safety
- SPM over CocoaPods/Carthage
- MVVM or TCA architecture for SwiftUI
- Sendable protocol for concurrency safety
- Extension-based API design

## Anti-Patterns
- Force unwrapping (!) outside IBOutlets
- Massive view controllers
- Retain cycles — use [weak self] in closures
- Stringly-typed APIs — use enums and protocols
