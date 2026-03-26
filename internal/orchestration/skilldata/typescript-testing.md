---
name: typescript-testing
description: "TypeScript testing — Vitest, Jest, Testing Library, Playwright, MSW."
triggers:
  - "vitest"
  - "jest+typescript"
  - "jest+ts"
  - "typescript+test"
  - ".test.ts"
  - ".spec.ts"
  - "playwright"
role: tester
phase: verify
language: typescript
verify:
  - name: "run tests"
    command: "npx vitest run 2>&1 || npx jest 2>&1 || echo 'TEST_FAILED'"
    expect_not: "TEST_FAILED"
---
# typescript-testing

## Vitest (preferred)
- describe/it/expect API (Jest-compatible)
- Built-in TypeScript support, no config needed
- vi.fn() for mocks, vi.spyOn() for spies
- vi.mock() for module mocking

## Jest + TypeScript
- ts-jest or @swc/jest for transform
- Type-safe mocks: jest.fn<ReturnType, Args>()
- jest.mocked() for auto-typing mocked modules

## Testing Library
- screen.getByRole, getByText, getByTestId
- userEvent over fireEvent
- waitFor for async assertions
- render() returns container for snapshot

## Playwright (E2E)
- page.goto, page.click, page.fill
- expect(page).toHaveURL, toHaveText
- Test fixtures for setup/teardown
- Parallel execution with workers

## MSW (API Mocking)
- http.get/post handlers for REST
- graphql.query/mutation for GraphQL
- server.use() for per-test overrides

## Anti-Patterns
- Testing implementation details (state, methods)
- `as any` in test code — use proper types
- No type safety in mock return values
- Snapshot overuse — prefer explicit assertions
