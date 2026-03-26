---
title: Skill System
created: 2026-03-26
tags:
  - architecture
  - skills
  - orchestration
  - agent
aliases:
  - Skills
  - Skill Registry
  - Skill Matching
---

# Skill System

The skill system is synapserouter's mechanism for injecting domain-specific knowledge into the [[Agent Execution Layer|agent loop]] and [[Skill Auto-Dispatch|orchestration engine]]. Skills are self-contained Markdown files with YAML frontmatter, compiled into the binary at build time via `go:embed`. There are no hardcoded Go registries -- everything is driven by frontmatter metadata.

## How Skills Are Embedded

Skills live in `internal/orchestration/skilldata/`. A single Go file (`embed.go`) uses the `go:embed` directive to bundle every `.md` file in that directory into the binary:

```go
//go:embed *.md
var Skills embed.FS
```

At runtime, `DefaultSkills()` calls `ParseSkillsFromFS(skilldata.Skills)` exactly once (guarded by `sync.Once`) to parse all embedded Markdown files. The result is cached for the lifetime of the process.

### Parsing Flow

1. `ParseSkillsFromFS` iterates every `.md` file in the embedded filesystem
2. `parseSkillMarkdown` splits each file at `---` delimiters to extract YAML frontmatter
3. Frontmatter is unmarshalled into a `Skill` struct via `gopkg.in/yaml.v3`
4. The Markdown body after the closing `---` becomes the `Instructions` field
5. Files missing a `name` or `phase` in frontmatter are skipped with a warning

### Skill Struct

```go
type Skill struct {
    Name         string          // required
    Description  string
    Triggers     []string        // keywords for matching
    Role         string          // coder, reviewer, tester, researcher, architect, analyst, writer
    MCPTools     []string        // bound MCP tools (auto-invoked)
    DependsOn    []string        // other skill names (dependency ordering)
    Phase        string          // required: analyze, implement, verify, review
    Language     string          // optional: go, python, rust, etc.
    Pipeline     string          // optional: software, data-science
    Instructions string          // Markdown body (injected into prompt)
    Verify       []VerifyCommand // executable verification checks
}
```

### Frontmatter Example

```yaml
---
name: go-patterns
description: "Idiomatic Go development — concurrency, error handling, interfaces, modules."
triggers:
  - "golang"
  - ".go"
  - "go+handler"
  - "go+struct"
role: coder
phase: analyze
language: go
verify:
  - name: "go vet passes"
    command: "go vet ./... 2>&1 || echo 'VET_FAILED'"
    expect_not: "VET_FAILED"
---
# Skill: Go Patterns
(Markdown body becomes Instructions)
```

## Trigger Matching

Trigger matching is handled by `MatchSkills()` in `internal/orchestration/dispatch.go`. The algorithm converts the goal text to lowercase, tokenizes it into words, then checks each skill's triggers against the text.

### Three Matching Strategies

#### 1. Compound Triggers (`+` syntax)

A trigger containing `+` is split into parts, and **all parts must independently match** for the trigger to fire. This prevents false positives for ambiguous words like "go".

| Trigger | Matches | Does Not Match |
|---|---|---|
| `go+handler` | "write a go handler" | "going to handle this" |
| `go+struct` | "define a go struct" | "let's go restructure" |
| `go+api` | "build a go api server" | "going to call the api" |

Each part of a compound trigger is recursively matched using the same rules below.

#### 2. Substring Matching

Used for **multi-word triggers** and triggers containing special characters (`.`, `-`, `_`, `/`):

| Trigger | Strategy | Example Match |
|---|---|---|
| `"api key"` | substring | "rotate the api key" |
| `".go"` | substring | "edit main.go" |
| `"go mod"` | substring | "run go mod tidy" |
| `"go.mod"` | substring | "check go.mod for deps" |

#### 3. Word-Boundary Matching

Used for **short ambiguous words** (2 characters) that commonly appear as prefixes in unrelated words. The `ambiguousWords` map includes: `go`, `do`, `is`, `it`, `or`, `an`, `as`, `at`, `be`, `by`, `in`, `no`, `of`, `on`, `so`, `to`, `up`, `us`, `we`.

These require an **exact word match** in the tokenized goal text:

| Trigger | Matches | Does Not Match |
|---|---|---|
| `go` (standalone) | "write go code" | "going to fix this" |
| `do` (standalone) | "do the task" | "docker build" |

All other single-word triggers (5+ characters, or not in the ambiguous set) use simple substring matching.

### Matching Priority

`MatchSkills` returns **all** skills with at least one matching trigger. There is no priority or ranking at this stage -- deduplication happens by skill name, so each skill appears at most once.

## Skill Phases

Skills are ordered into four execution phases. When multiple skills match a goal, `BuildSkillChain()` sorts them by phase order, then alphabetically within each phase for determinism.

| Phase | Order | Purpose | Typical Roles |
|---|---|---|---|
| `analyze` | 0 | Understand the problem, detect patterns, research | coder, researcher, architect, analyst |
| `implement` | 1 | Write code, build features, create artifacts | coder, writer |
| `verify` | 2 | Run tests, validate output, check compliance | tester |
| `review` | 3 | Code review, quality audit, continuous improvement | reviewer |

The phase ordering ensures that analysis happens before implementation, implementation before testing, and testing before review. This maps directly to the [[Agent Pipeline|software pipeline]]: Plan, Implement, Self-Check, Code Review.

### Skill Chain to Task Steps

`SkillChainToSteps()` converts the ordered chain into `TaskStep` objects for the orchestration engine. Each step gets:

- The skill's `Role` as the step role
- A prompt combining: `[skill-name] description`, full `Instructions` body, and the original goal
- Status set to `StepStatusPending`

If no skills match the goal, the dispatcher falls back to `inferRoles()` for role-based planning.

## Language-Field Matching vs Trigger Matching

There are **two independent paths** that match skills to requests, used in different contexts.

### Trigger Matching (Orchestration Dispatch)

Used by `MatchSkills()` when the [[Skill Auto-Dispatch|dispatch engine]] processes a goal/task. Scans all skill triggers against the goal text. This is the primary matching path for `synroute chat` and the agent loop.

### Language-Field Matching (Router Preprocessor)

Used by `buildInjections()` in `internal/router/preprocess.go` when the [[Router|router]] preprocesses a chat request before forwarding to a provider. This path:

1. Scans conversation messages for programming language indicators (code blocks, file extensions, keywords)
2. Builds a set of detected languages (e.g., `{"go", "python"}`)
3. Matches skills by their `language` field first (more precise than trigger matching)
4. Falls back to trigger matching for languages without a dedicated `language`-field skill
5. Only injects `analyze`-phase skills (patterns, not testing or review)
6. Respects a **500-token budget** for total injections

The preprocessor also injects non-skill context for:
- **Error loops** -- repeated errors trigger recovery guidance
- **Validation errors** -- API parameter errors trigger documentation lookup hints
- **Post-change verification** -- code changes trigger `go vet` / `go test` / `go build` reminders
- **Security surface** -- auth/credential topics trigger security review hints
- **Research needs** -- question patterns trigger Context7 documentation hints

### Injection into System Prompt

`injectSkillContext()` prepends injection text to the system message. If a system message already exists, the injections are merged at the top. If no system message exists, a new one is created. Injections are budget-capped at ~500 tokens (estimated at 4 characters per token).

## Verification Commands

Skills can define executable verification checks via the `verify` field:

```yaml
verify:
  - name: "go vet passes"
    command: "go vet ./... 2>&1 || echo 'VET_FAILED'"
    expect: "OK"           # output should contain this
    expect_not: "FAILED"   # output should NOT contain this
  - name: "no hardcoded secrets"
    manual: "Review code for embedded credentials"  # human check
```

`VerifyCommandsForChain()` collects all verification commands from matched skills into a formatted checklist for the reviewer phase.

## Complete Skill Inventory (52 skills)

### Language Patterns (14 skills) -- Phase: `analyze`

| Skill | Language | Description |
|---|---|---|
| `go-patterns` | go | Idiomatic Go -- concurrency, error handling, interfaces, modules |
| `python-patterns` | python | Idiomatic Python -- PEP 8, type hints, async patterns, data modeling |
| `rust-patterns` | rust | Idiomatic Rust -- ownership, memory safety, concurrency, Cargo |
| `typescript-patterns` | typescript | TypeScript -- strict types, generics, utility types, Zod, type guards |
| `javascript-patterns` | javascript | Modern JS/TS -- React, Node.js, TypeScript-first, Next.js patterns |
| `swift-patterns` | swift | Swift / iOS -- SwiftUI, async/await, protocols, SPM |
| `kotlin-patterns` | kotlin | Kotlin -- coroutines, null safety, data classes, Ktor, Compose |
| `java-patterns` | java | Modern Java (17+) -- records, sealed classes, streams, virtual threads |
| `csharp-patterns` | csharp | C# / .NET -- async/await, DI, LINQ, Entity Framework, NuGet |
| `sql-expert` | sql | Cross-dialect SQL -- PostgreSQL, MySQL, SQLite, CTEs, optimization |
| `fastapi-patterns` | python | FastAPI -- async, dependency injection, Pydantic, SQLAlchemy 2.0 |
| `java-spring` | -- | Spring Boot 3.x -- JPA, constructor injection, layered architecture |
| `ml-patterns` | python | ML development -- train/test splits, feature engineering, sklearn |
| `node-toolchain` | javascript | Node.js toolchain -- npm/yarn/pnpm, workspaces, monorepos |

### Testing (10 skills) -- Phase: `verify`

| Skill | Language | Description |
|---|---|---|
| `go-testing` | go | Go testing -- table-driven, benchmarks, race detection, fuzzing |
| `python-testing` | python | pytest -- fixtures, mocking, parametrize, TDD workflow |
| `rust-testing` | rust | cargo test -- proptest, criterion benchmarks, integration tests |
| `typescript-testing` | typescript | Vitest, Jest, Testing Library, Playwright, MSW |
| `swift-testing` | swift | XCTest, Swift Testing framework, async tests, UI testing |
| `kotlin-testing` | kotlin | JUnit5, MockK, Kotest, coroutine testing, Turbine |
| `java-testing` | -- | JUnit5, Mockito, AssertJ, Testcontainers, Spring Boot slices |
| `csharp-testing` | csharp | xUnit, NUnit, Moq, FluentAssertions, WebApplicationFactory |

### Implementation (13 skills) -- Phase: `implement`

| Skill | Description |
|---|---|
| `code-implement` | Produce implementation-ready code changes |
| `docker-expert` | Docker -- multi-stage builds, security, compose, optimization |
| `git-expert` | Advanced git -- rebase, bisect, cherry-pick, worktrees, conflicts |
| `github-workflows` | GitHub CLI, PR management, Actions, code review, publishing |
| `devops-engineer` | CI/CD pipelines, IaC, Terraform, Kubernetes, monitoring |
| `python-venv` | Virtual environment management -- venv, uv, pip, isolation |
| `data-scrubber` | PII detection, removal, identity anonymization |
| `doc-coauthoring` | 3-stage structured doc writing -- context, refinement, testing |
| `task-orchestrator` | Parallel task decomposition, worker agent spawning |
| `predictive-modeler` | ML forecasting, time-series analysis, risk assessment |
| `feature-engineer` | Raw data to ML-ready features -- encoding, scaling, text |
| `slack-integration` | Slack channel operations, message posting, signal extraction |
| `intervals-icu` | Intervals.icu API -- structured workouts, calendar, Garmin sync |

### Project Management (3 skills) -- Phase: `implement`

| Skill | Description |
|---|---|
| `jira-manage` | Create, update, search Jira issues -- DC custom fields |
| `jira-project-config` | Jira project config -- components, versions, boards |
| `document-mcp` | Consulting-quality PPTX/DOCX with charts, stats, AI images |

### Research and Analysis (6 skills) -- Phase: `analyze`

| Skill | Description |
|---|---|
| `research` | Investigate context, alternatives, constraints via web search |
| `deep-research` | Deep web research using Gemini AI with Google Search grounding |
| `context7` | Fetch up-to-date library docs and code examples |
| `search-first` | Research existing solutions before building custom code |
| `eda-explorer` | Exploratory data analysis -- distributions, outliers, correlations |
| `prompt-engineer` | Prompt crafting, optimization, multi-agent orchestration prompts |

### Architecture and Design (3 skills) -- Phase: `analyze`

| Skill | Description |
|---|---|
| `api-design` | REST/OpenAPI endpoint design -- pagination, auth, versioning |
| `spec-workflow` | 5-phase spec creation, validation, implementation orchestration |
| `dbt-modeler` | dbt -- medallion architecture, staging/marts, incremental models |

### Database (1 skill) -- Phase: `analyze`

| Skill | Description |
|---|---|
| `snowflake-query` | Snowflake SQL, schema exploration, warehouse management |

### Review and Quality (3 skills) -- Phase: `review`

| Skill | Description |
|---|---|
| `code-review` | Structured code review -- SOLID, performance, multi-language |
| `security-review` | OWASP vulnerability detection, audit workflows, SAST |
| `continuous-improvement` | Session observation, pattern capture, skill evolution |

### Meta (1 skill) -- Phase: `review`

| Skill | Description |
|---|---|
| `skill-stocktake` | Automated skill quality audit -- Keep, Improve, Update, Retire, Merge |

### Project-Level Skills (not embedded)

These live in `.claude/skills/` and are **not** compiled into the binary. They are read at runtime and executed by following the instructions in the file directly.

| Skill | File | Purpose |
|---|---|---|
| `synroute-test` | `.claude/skills/synroute-test.md` | E2E provider smoke testing pipeline |
| `synroute-research` | `.claude/skills/synroute-research.md` | Research workflow for synapserouter |
| `synroute-profile` | `.claude/skills/synroute-profile.md` | Profile switching and verification |

## How to Add a New Skill

1. Create a new `.md` file in `internal/orchestration/skilldata/`
2. Add YAML frontmatter with at minimum `name` and `phase`
3. Define `triggers` for automatic matching
4. Optionally set `language` for [[#Language-Field Matching (Router Preprocessor)|language-field matching]]
5. Optionally bind `mcp_tools` for auto-invocation
6. Optionally add `verify` commands for executable checks
7. Write the Markdown body with detailed instructions
8. Rebuild: `go build -o synroute .`

No Go code changes are required. The `go:embed` directive automatically picks up new `.md` files on rebuild.

### Minimal Example

```markdown
---
name: my-new-skill
description: "One-line description of what this skill does."
triggers:
  - "keyword1"
  - "keyword2"
  - "ambiguous+qualifier"
role: coder
phase: implement
---
# Skill: My New Skill

Detailed instructions for the LLM when this skill is activated.
```

### Adding a Language Skill

For language-specific skills, set the `language` field to enable [[#Language-Field Matching (Router Preprocessor)|automatic injection]] when that language is detected in conversation:

```yaml
language: python
```

The router preprocessor recognizes: `go`, `python`, `rust`, `typescript`, `javascript`, `swift`, `kotlin`, `java`, `csharp`, `sql`.

## Dispatch Flow

```
User goal
  │
  ├─► MatchSkills(goal, registry)
  │     └─► For each skill, check each trigger via matchesTrigger()
  │           ├─► Compound "+" → all parts must match (recursive)
  │           ├─► Multi-word / special chars → substring match
  │           ├─► Ambiguous short word → exact word-boundary match
  │           └─► Everything else → substring match
  │
  ├─► BuildSkillChain(matched)
  │     └─► Deduplicate by name → sort by phase order → alphabetical within phase
  │
  ├─► SkillChainToSteps(chain, goal)
  │     └─► Each skill → TaskStep with role + skill-aware prompt + instructions
  │
  └─► If no skills match → fallback to inferRoles() (role-based planning)
```

## Related

- [[Agent Execution Layer]] -- the agent loop that executes skill-generated task steps
- [[Agent Pipeline]] -- phase-based execution (Plan, Implement, Self-Check, Code Review)
- [[Router]] -- the preprocessor that injects skill context into chat requests
- [[Skill Auto-Dispatch]] -- the orchestration engine entry point
- [[MCP Server]] -- skills can bind MCP tools for auto-invocation
