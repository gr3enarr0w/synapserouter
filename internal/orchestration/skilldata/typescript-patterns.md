---
name: typescript-patterns
description: "TypeScript development — strict types, generics, utility types, Zod, type guards."
triggers:
  - "typescript"
  - ".ts"
  - ".tsx"
  - "tsc"
  - "tsconfig"
  - "zod"
  - "trpc"
  - "type+safe"
role: coder
phase: analyze
language: typescript
mcp_tools:
  - "context7.query-docs"
verify:
  - name: "type check"
    command: "npx tsc --noEmit 2>&1 || echo 'TYPE_ERROR'"
    expect_not: "TYPE_ERROR"
  - name: "no any"
    command: "grep -rn ': any' --include='*.ts' --include='*.tsx' | grep -v 'test\\|spec\\|node_modules\\|.d.ts' | head -5 || echo 'OK'"
    expect: "OK"
---
# typescript-patterns

## Core Principles
1. **Strict mode always** — strict: true in tsconfig.json
2. **No `any`** — use `unknown` for truly unknown types
3. **Utility types** — Partial, Pick, Omit, Record, Required, Readonly
4. **Generics** — type-safe abstractions without duplication
5. **Discriminated unions** — type narrowing via literal type fields
6. **Zod** — runtime validation that generates TypeScript types

## Patterns
- Type guards: `is`, `in`, typeof, instanceof
- Template literal types for string patterns
- const assertions for literal inference
- satisfies operator for type checking without widening
- Branded types for domain primitives (UserId, Email)
- Module resolution: ESM with explicit .js extensions
- Mapped types and conditional types for meta-programming

## Anti-Patterns
- `any` type (use `unknown` + type guards)
- Type assertions (`as`) without validation
- `@ts-ignore` / `@ts-expect-error` without comment
- Non-strict tsconfig
- `enum` (use const objects with `as const`)
- Barrel files (index.ts re-exports) in large projects
