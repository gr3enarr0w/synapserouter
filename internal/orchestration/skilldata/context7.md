---
name: context7
description: "Fetch up-to-date library documentation and code examples."
triggers:
  - "context7"
  - "current docs"
  - "latest docs"
  - "library docs"
role: researcher
phase: analyze
mcp_tools:
  - "context7.resolve-library-id"
  - "context7.query-docs"
---
# Skill: Context7

Fetch up-to-date library documentation and code examples — wraps the Context7 MCP for token efficiency.

Source: [upstash/context7](https://github.com/upstash/context7).

---

## When to Use

- When writing code with a library that may have changed since training cutoff
- Before generating code that depends on a specific API version
- When encountering deprecated methods or unknown APIs
- When the user says "use context7" or asks for current docs

---

## How It Works

Context7 MCP provides two tools:
1. **`resolve-library-id`** — resolves a library name to a Context7 ID
2. **`query-docs`** — fetches current documentation for that library

---

## Process

### Step 1: Resolve the library
Use the Context7 MCP `resolve-library-id` tool with the library name (e.g., "react", "fastapi", "sqlalchemy").

### Step 2: Query documentation
Use the Context7 MCP `query-docs` tool with the resolved library ID to get current docs, examples, and API references.

### Step 3: Apply to code generation
Use the fetched documentation to write code with current APIs, avoiding deprecated patterns.

---

## Common Libraries

| Library | Typical Query |
|---------|--------------|
| React | "react hooks", "react server components" |
| FastAPI | "fastapi dependency injection", "fastapi middleware" |
| SQLAlchemy | "sqlalchemy 2.0 async session" |
| Pydantic | "pydantic v2 model config" |
| Next.js | "next.js app router", "next.js server actions" |

---

## Key Details

- **MCP**: `context7` (configured globally)
- **Free tier**: Works without API key (rate-limited)
- **API key**: Get at context7.com/dashboard for higher limits
- **Token-efficient**: Only fetches relevant doc sections, not entire docs
- **Auto-invoke rule**: Add "use context7" to prompts for automatic doc fetching

---

## Tools

- Context7 MCP — `resolve-library-id`, `query-docs`
