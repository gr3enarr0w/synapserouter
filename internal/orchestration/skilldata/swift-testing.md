---
name: swift-testing
description: "Swift testing — XCTest, Swift Testing framework, async tests, UI testing."
triggers:
  - "xctest"
  - "swift test"
  - "swift+test"
  - "@testable"
  - "swift+spec"
role: tester
phase: verify
language: swift
verify:
  - name: "swift test"
    command: "swift test 2>&1 | tail -10"
    expect: "passed"
---
# swift-testing

## XCTest Patterns
- setUp/tearDown for lifecycle
- XCTAssertEqual, XCTAssertTrue, XCTAssertNil, XCTAssertThrowsError
- @testable import for internal access
- Async tests: func testAsync() async throws {}

## Swift Testing (5.9+)
- @Test macro, #expect for assertions
- @Suite for grouping, .tags for filtering
- Parameterized with @Test(arguments:)

## UI Testing
- XCUIApplication for launch
- XCUIElement queries for interaction
- Accessibility identifiers over UI hierarchy

## Anti-Patterns
- Testing private implementation details
- Flaky async tests (use expectations with timeout)
- No assertions in test methods
- Force unwrapping in tests without XCTUnwrap
