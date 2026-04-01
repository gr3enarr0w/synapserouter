# <Project Name> — Reconstruction Spec

## Overview
One-paragraph description: what this project does, who uses it, core value proposition.

## Scope
Explicit boundary of what the agent should build.

**IN SCOPE:**
- Feature A
- Feature B
- Feature C

**OUT OF SCOPE:**
- Feature X (too large / external dependency)
- Feature Y (UI-only, not testable)

**TARGET:** ~XXXX LOC, ~XX files

## Architecture

- **Language/Runtime:** Go 1.22 / Python 3.12 / Rust 1.78 / etc.
- **Framework:** (if applicable)
- **Key dependencies:** (with versions)

### Directory Structure
```
project/
  cmd/
    main.go
  internal/
    foo/
      foo.go
    bar/
      bar.go
  go.mod
  README.md
```

### Design Patterns
- Pattern 1 (e.g., middleware chain, plugin system, repository pattern)
- Pattern 2

## Data Flow
How data moves through the system. For CLIs: input -> parse -> process -> output.
For web apps: request -> middleware -> handler -> service -> store -> response.

```
[Input] -> [Parser] -> [Processor] -> [Output]
                          |
                     [Config Store]
```

## Core Components

### Component A
- **Purpose:** What it does
- **Public API:**
  ```
  func NewFoo(cfg Config) *Foo
  func (f *Foo) Process(input []byte) (Result, error)
  ```
- **Key algorithm:** Brief pseudocode for non-trivial logic
- **Error handling:** What errors can occur, how handled
- **Dependencies:** Which other components it uses

### Component B
(same structure)

## Configuration
- Config format (TOML, YAML, env vars, CLI flags)
- Required settings with descriptions
- Optional settings with defaults
- Example config file

## Test Cases

### Functional Tests
1. **Basic operation:** [input] -> [expected output]
2. **Multiple inputs:** [input] -> [expected output]
3. **Config override:** [input + config] -> [expected output]
4. **Error case:** [bad input] -> [expected error]
5. **Edge case:** [boundary input] -> [expected output]

### Integration Tests
1. **End-to-end:** Full pipeline from input to output
2. **Error recovery:** System recovers from [failure scenario]

## Build & Run

### Build
```bash
# exact build commands
```

### Run
```bash
# example invocations with expected output
```

### Test
```bash
# test commands
```

## Acceptance Criteria

These are what the scorer checks. Every criterion must be verifiable.

1. Project compiles/builds without errors
2. All test cases from "Test Cases" section pass
3. [Specific feature] works correctly for [specific input]
4. Error handling returns appropriate messages for [error case]
5. Configuration is loaded from [config format]
6. Output format matches [expected format]
7. Performance: [specific operation] completes in < [time]
