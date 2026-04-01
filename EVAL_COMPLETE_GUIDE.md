# Synroute Eval Framework - Complete Guide

## Overview
Multi-language evaluation framework for testing LLM coding capabilities across benchmarks.

## Commands

### 1. Import Benchmarks
```bash
synroute eval import --source <name> --path <path> [options]

Required:
  --source   Benchmark source (see list below)
  --path     Path to cloned benchmark repository

Optional:
  --language    Language filter (required for exercism)
  --count       Max exercises (codecontests only)
  --json        JSON output

Sources:
  - polyglot       Polyglot benchmark
  - roocode        Roo-Code Evals
  - exercism       Exercism exercises
  - multiple       MultiPL-E benchmark
  - evalplus       EvalPlus benchmark
  - codecontests   Code contests
  - ds1000         DS1000 data science
  - birdsql        BIRD SQL benchmark
  - dare-bench     DARE benchmark
  - writingbench   Writing benchmark
  - pptarena       PPT Arena
```

### 2. Import All from Directory
```bash
synroute eval import-all --dir <directory>
```

### 3. List Exercises
```bash
synroute eval exercises --language <lang> [--json]
```

### 4. Run Evaluations
```bash
synroute eval run [options]

Options:
  --language <langs>     Languages (comma-separated)
  --count <n>           Total exercise cap
  --mode <mode>         direct or routing (default: direct)
  --provider <name>     Specific provider (direct mode)
  --model <name>        Specific model
  --two-pass            Enable error feedback pass
  --agent               Iterative test→fix loop
  --max-turns <n>       Agent mode iterations (default: 5)
  --concurrency <n>     Parallel exercises (default: 4, max: 10)
  --per-suite <n>       Exercises per suite (default: 40)
  --suite <name>        Filter by suite
  --timeout <seconds>   Docker timeout (default: 120)
  --seed <n>            Random seed
  --json                JSON output
```

### 5. View Results
```bash
synroute eval results [--run-id <id>] [--json]
```

### 6. Compare Runs
```bash
synroute eval compare --run-a <id1> --run-b <id2> [--json]
```

## Examples

### Import
```bash
# Import exercism Go exercises
synroute eval import --source exercism --path ~/exercism-go --language go

# Import polyglot benchmark
synroute eval import --source polyglot --path ~/polyglot-benchmark

# Import all from directory
synroute eval import-all --dir ~/eval-benchmarks
```

### Run
```bash
# Basic run (10 exercises, two-pass)
synroute eval run --language go --count 10 --two-pass

# Routing mode (router selects provider)
synroute eval run --mode routing --count 20 --two-pass

# Specific provider
synroute eval run --provider ollama-chain-1 --language go --count 10

# Agent mode (iterative fixing)
synroute eval run --language python --agent --max-turns 5

# Multiple languages
synroute eval run --language go,python,javascript --count 30

# Concurrency control
synroute eval run --language go --concurrency 8 --count 50
```

### Results
```bash
# Latest run results
synroute eval results

# Specific run
synroute eval results --run-id eval-1774719114038066000 --json

# Compare runs
synroute eval compare --run-a eval-001 --run-b eval-002
```

## Modes Explained

### Direct Mode (--mode direct)
- Uses specified provider only
- Controlled testing
- Good for provider comparison

### Routing Mode (--mode routing)
- Router selects best provider
- Automatic fallback
- Production-like testing

### Two-Pass (--two-pass)
- Run 1: Generate solution
- Run 2: Refine with error feedback
- Improves pass rate significantly

### Agent Mode (--agent)
- Iterative test→fix loop
- Full test file context
- Up to --max-turns iterations
- Best for complex problems

## Current System Status

### Imported Exercises: 4,102
| Suite | Count |
|-------|-------|
| ds1000 | 1,000 |
| writingbench | 1,000 |
| exercism | 713 |
| evalplus | 564 |
| birdsql | 500 |
| multiple | 325 |

### Recent Results
| Language | Pass 1 | Pass 2 | Notes |
|----------|--------|--------|-------|
| Rust | 80% | 0% | Best single-pass |
| JavaScript | 60% | 40% | Two-pass helps |
| Java | 60% | 0% | - |
| C++ | 0% | 20% | Two-pass essential |

## Database

Location: `~/.mcp/proxy/usage.db`

Tables:
- `eval_exercises` - Imported problems (4,102 rows)
- `eval_runs` - Run metadata
- `eval_results` - Individual results (1,746 rows)

## Tips

### Testing New Models
```bash
# Quick test (5 exercises, fast feedback)
synroute eval run --provider ollama-chain-1 --count 5 --two-pass

# Compare providers (use same seed)
synroute eval run --provider ollama-chain-1 --count 20 --seed 42
synroute eval run --provider ollama-chain-2 --count 20 --seed 42
synroute eval compare --run-a <id1> --run-b <id2>
```

### Full Benchmark
```bash
# Complete benchmark (default 40 per suite)
synroute eval run --language go --two-pass

# No per-suite limit
synroute eval run --language go --per-suite 0 --count 100
```

### Debugging
```bash
# Single exercise with verbose output
synroute eval run --count 1 --language go --json

# Check specific run details
synroute eval results --run-id <id> --json | jq
```