# Eval Benchmarks

Exercise data for the synroute eval framework. These are curated subsets from open-source benchmark repositories, stripped of build tooling and test frameworks to minimize size.

## Sources

### Code Generation (docker-test)

| Directory | Source | License | Exercises | Language |
|-----------|--------|---------|-----------|----------|
| `exercism-go/` | [exercism/go](https://github.com/exercism/go) | MIT | 144 | Go |
| `exercism-python/` | [exercism/python](https://github.com/exercism/python) | MIT | 140 | Python |
| `exercism-javascript/` | [exercism/javascript](https://github.com/exercism/javascript) | MIT | 138 | JavaScript |
| `exercism-java/` | [exercism/java](https://github.com/exercism/java) | MIT | 97 | Java |
| `exercism-rust/` | [exercism/rust](https://github.com/exercism/rust) | MIT | 106 | Rust |
| `exercism-cpp/` | [exercism/cpp](https://github.com/exercism/cpp) | MIT | 86 | C++ |
| `multiple/` | [nuprl/MultiPL-E](https://github.com/nuprl/MultiPL-E) | Apache 2.0 | 325 | Python |
| `evalplus/` | [evalplus/evalplus](https://github.com/evalplus/evalplus) | Apache 2.0 | 564 | Python |

### Data Science (docker-test)

| Directory | Source | License | Exercises | Language |
|-----------|--------|---------|-----------|----------|
| `ds1000/` | [xlang-ai/DS-1000](https://github.com/xlang-ai/DS-1000) | CC-BY-SA-4.0 | 1,000 | Python |

### Text-to-SQL (docker-test)

| Directory | Source | License | Exercises | Language |
|-----------|--------|---------|-----------|----------|
| `birdsql/` | [bird-bench/mini_dev](https://github.com/bird-bench/mini_dev) | CC-BY-SA-4.0 | 500 | SQL |

### ML Modeling (metric-compare)

| Directory | Source | License | Exercises | Language |
|-----------|--------|---------|-----------|----------|
| `dare-bench/` | [Snowflake-Labs/dare-bench](https://github.com/Snowflake-Labs/dare-bench) | Apache 2.0 | 162 | Python |

### Business Writing (llm-judge)

| Directory | Source | License | Exercises | Language |
|-----------|--------|---------|-----------|----------|
| `writingbench/` | [X-PLUG/WritingBench](https://github.com/X-PLUG/WritingBench) | Apache 2.0 | 1,000 | Text |

### Slide Editing (vlm-judge, deferred)

| Directory | Source | License | Exercises | Language |
|-----------|--------|---------|-----------|----------|
| `pptarena/` | [michaelofengend/PPTArena](https://github.com/michaelofengend/PPTArena) | MIT | 100 | PPTX |

**Total: ~3,800+ exercises across 5 eval modes**

## Eval Modes

| Mode | Description | Scoring |
|------|-------------|---------|
| `docker-test` | Run tests in Docker container | Binary pass/fail |
| `metric-compare` | Compare predictions vs ground truth | F1, R², Exact Match |
| `llm-judge` | Route response + criteria to LLM for scoring | 1-10 rubric scale |
| `vlm-judge` | Visual comparison of PPTX output (deferred) | Structural + visual |

## Structure

### Exercism (`exercism-{lang}/exercises/practice/{slug}/`)
- `.docs/instructions.md` — Problem description
- `{slug}_test.{ext}` — Test file
- `{slug}.{ext}` — Stub file (starting point)
- `.meta/` — Example solutions and metadata

### MultiPL-E (`multiple/datasets/`)
- `HumanEvalPlus-v0.1.8.jsonl` — 164 HumanEval+ problems (Python)
- `py-humaneval-originals.jsonl` — 161 HumanEval originals (Python)
- `py-mbpp-originals.jsonl` — 400 MBPP problems (Python)

### EvalPlus (`evalplus/data/`)
- `humaneval_plus.jsonl` — 164 HumanEval+ with 80x more tests (Python)
- `mbpp_plus.jsonl` — 400 MBPP+ problems (Python)

### DS-1000 (`ds1000/data/`)
- `ds1000.jsonl` — 1,000 data science problems (Pandas, NumPy, SciPy, Matplotlib, Seaborn, Scikit-learn, TensorFlow)
- Each entry: `prompt`, `reference_code`, `code_context` (test functions), `metadata`

### BIRD-SQL (`birdsql/data/`)
- `mini_dev_prompt.jsonl` — 500 text-to-SQL pairs across 11 databases
- Each entry: `question`, `SQL`, `schema`, `db_id`, `evidence`, `difficulty`

### DARE-bench (`dare-bench/data/eval/`)
- `question_list.json` — 162 ML tasks (classification, regression, time series)
- `databases/{task_id}/` — Per-task CSV datasets and metadata

### WritingBench (`writingbench/data/`)
- `benchmark_all.jsonl` — 1,000 writing queries with 5 criteria each
- Domains: Finance, Academic, Marketing, Law, Education, Literature

### PPTArena (`pptarena/`)
- `data/evaluation_pairs_refined.json` — 100 edit task specifications
- `Original/` — Source PPTX files
- `GroundTruth/` — Expected output PPTX files

## Import

```bash
# Code generation
./synroute eval import --source exercism --path benchmarks/exercism-go --language go
./synroute eval import --source exercism --path benchmarks/exercism-python --language python
./synroute eval import --source multiple --path benchmarks/multiple
./synroute eval import --source evalplus --path benchmarks/evalplus

# Data science & SQL
./synroute eval import --source ds1000 --path benchmarks/ds1000
./synroute eval import --source birdsql --path benchmarks/birdsql
./synroute eval import --source dare-bench --path benchmarks/dare-bench

# Writing & slides
./synroute eval import --source writingbench --path benchmarks/writingbench
./synroute eval import --source pptarena --path benchmarks/pptarena
```

## What was stripped

To keep the repo manageable, the following were removed:
- Catch2 test framework headers (`catch.hpp`, `catch_amalgamated.*`) — duplicated per C++ exercise
- Gradle wrappers (`gradlew`, `*.jar`) — duplicated per Java exercise
- Git metadata (`.git/`)
- Build artifacts and node_modules
- Answer/result files from benchmark repos
