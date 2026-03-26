---
name: node-toolchain
description: "Node.js toolchain — npm/yarn/pnpm, package.json, workspaces, monorepos."
triggers:
  - "npm"
  - "yarn"
  - "pnpm"
  - "package.json"
  - "node_modules"
  - "monorepo"
  - "workspace"
  - "turbo"
  - "nx"
role: coder
phase: analyze
language: javascript
mcp_tools:
  - "context7.query-docs"
verify:
  - name: "install"
    command: "npm install 2>&1 || yarn install 2>&1 || pnpm install 2>&1 || echo 'INSTALL_FAILED'"
    expect_not: "INSTALL_FAILED"
  - name: "audit"
    command: "npm audit --audit-level=high 2>&1 || echo 'AUDIT_WARN'"
    expect_not: "AUDIT_WARN"
---
# node-toolchain

## Package Management
- **npm**: default, good enough for most projects
- **yarn**: better for monorepos (workspaces), deterministic installs
- **pnpm**: fastest, strictest (no phantom deps), best disk usage

## package.json Best Practices
- engines field: pin Node version
- scripts: build, test, lint, start, dev
- Exact versions for apps, ranges for libraries
- type: "module" for ESM

## Monorepos
- npm/yarn/pnpm workspaces for multi-package repos
- Turborepo for task orchestration and caching
- Nx for advanced dependency graph and affected detection

## Version Management
- .nvmrc or .node-version for Node version
- corepack for package manager version
- lockfile MUST be committed

## Anti-Patterns
- No lockfile committed
- `npm install -g` for project dependencies
- Missing engines field
- Unpinned versions in applications
- Ignoring npm audit warnings
- Installing devDependencies in production
