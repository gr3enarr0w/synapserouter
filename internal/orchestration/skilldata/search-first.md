---
name: search-first
description: "Research existing solutions before building custom code."
triggers:
  - "find library"
  - "find package"
  - "existing solution"
  - "alternative"
role: researcher
phase: analyze
mcp_tools:
  - "research-mcp.research_search"
  - "context7.query-docs"
---
# Skill: Search First

Research existing solutions before building custom code.

Source: [affaan-m/search-first](https://github.com/affaan-m/everything-claude-code/tree/main/skills/search-first) (70K stars).

---

## When to Use

- Starting a new feature that likely has existing solutions
- Adding a dependency or integration
- Before creating a new utility, helper, or abstraction
- When asked "add X functionality" and about to write code

---

## Workflow

```
1. NEED ANALYSIS
   Define what functionality is needed
   Identify language/framework constraints

2. PARALLEL SEARCH
   ┌──────────┐ ┌──────────┐ ┌──────────┐
   │  npm /   │ │  MCP /   │ │  GitHub / │
   │  PyPI    │ │  Skills  │ │  Web      │
   └──────────┘ └──────────┘ └──────────┘

3. EVALUATE
   Score candidates (functionality, maintenance,
   community, docs, license, dependencies)

4. DECIDE
   ┌─────────┐  ┌──────────┐  ┌─────────┐
   │  Adopt  │  │  Extend  │  │  Build   │
   │ as-is   │  │  /Wrap   │  │  Custom  │
   └─────────┘  └──────────┘  └─────────┘

5. IMPLEMENT
   Install package / Configure MCP /
   Write minimal custom code
```

---

## Decision Matrix

| Signal | Action |
|--------|--------|
| Exact match, well-maintained, MIT/Apache | **Adopt** — install and use directly |
| Partial match, good foundation | **Extend** — install + write thin wrapper |
| Multiple weak matches | **Compose** — combine 2-3 small packages |
| Nothing suitable found | **Build** — write custom, informed by research |

---

## Search Checklist

Before writing any utility:

0. **Repo search** — `rg` through existing modules/tests first
1. **Package registry** — Search npm/PyPI/crates.io
2. **MCP servers** — Check configured MCPs and search for new ones
3. **Skills** — Check `~/.claude/skills/` and project skills
4. **GitHub** — Run code search for maintained OSS

---

## Anti-Patterns

- **Jumping to code** — writing a utility without checking if one exists
- **Ignoring MCP** — not checking if an MCP server provides the capability
- **Over-customizing** — wrapping a library so heavily it loses benefits
- **Dependency bloat** — installing a massive package for one small feature
