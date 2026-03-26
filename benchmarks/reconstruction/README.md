# Router Reconstruction Test Suite

50 projects across 10 categories used to **find and fix weaknesses in the synapserouter agent**. Each project that fails reveals a gap — wrong tool usage, bad architecture decisions, missing skill knowledge, escalation problems, wrong output format, etc. We fix the router, re-run, and iterate until it handles all 50.

This is not a scorecard. It's a test-driven improvement plan for the router.

## Structure

Each project gets its own folder under `~/Development/` with a `spec.md`:

```
~/Development/
  spring-petclinic/
    spec.md           # Written by Claude Code (examiner)
    synroute.md       # Created by synapserouter (tracks state)
    src/              # Built by synapserouter (test subject)
  umami/
    spec.md
    ...
```

Review repos cloned to `~/Development/Project_Review/` for examiner reference.

- **spec.md** — Written by Claude Code after analyzing the original repo
- **synroute.md** — Agent's project state file, created/updated each run
- Synapserouter NEVER sees the original repos. Only the spec.

## Projects

### Source Code Languages (35 projects)

#### Wave 1: Calibration (one per language with skills)
| # | Project | Language | Spec | Tested |
|---|---------|----------|------|--------|
| 1 | spring-petclinic | Java | Done | - |
| 2 | umami | TypeScript | Done | - |
| 3 | fd | Rust | Done | Testing |
| 4 | goodbooks-10k | ML Python | Done | - |
| 5 | restic | Go | Done | - |

#### Wave 2: Skill Gaps
| # | Project | Language | Spec | Tested |
|---|---------|----------|------|--------|
| 6 | duplicati | C# | Done | - |
| 7 | end-to-end-diamond-price-prediction | ML Python | Done | PASS (R²=0.981) |
| 8 | httpie-cli | Python | Done | - |
| 9 | quarkus-quickstarts | Java | Done | - |
| 10 | bat | Rust | Done | - |

#### Wave 3: Architecture Complexity
| # | Project | Language | Spec |
|---|---------|----------|------|
| 11-20 | caddy, n8n, outline, cli-cli, credit-card-fraud, jhipster, java-design-patterns, eshop, directus, starship | Mixed | Done |

#### Wave 4: Scale
| # | Project | Language | Spec |
|---|---------|----------|------|
| 21-30 | ripgrep, cal-com, prefect, streamlit, syncthing, rclone, vaultwarden, stock-prediction, handson-ml2, sharex | Mixed | Done |

#### Wave 5: Boss Level
| # | Project | Language | Spec |
|---|---------|----------|------|
| 31-35 | yt-dlp, home-assistant-core, airbyte, jellyfin, powershell | Mixed | Done |

### Jupyter Notebooks — .ipynb (5 projects)

Tests the agent's ability to generate structured JSON notebook files with code cells, markdown cells, and outputs.

| # | Project | What It Tests | Difficulty |
|---|---------|--------------|------------|
| 36 | traffic-sign-recognition | CNN image classification, data augmentation, GTSRB dataset | Hard |
| 37 | sentiment-analysis-lstm | NLP, RNN/LSTM, word embeddings, movie reviews | Hard |
| 38 | cifar10-cnn | Computer vision, Keras CNN, hyperparameter tuning | Medium |
| 39 | movie-recommender-knn | KNN from scratch, collaborative filtering | Medium |
| 40 | human-activity-recognition | 1D CNN on time-series sensor data | Hard |

Reference: [chhayac/Machine-Learning-Notebooks](https://github.com/chhayac/Machine-Learning-Notebooks), [fchollet/deep-learning-with-python-notebooks](https://github.com/fchollet/deep-learning-with-python-notebooks)

### R Markdown — .Rmd (5 projects)

Tests the agent's ability to generate R code chunks, ggplot2 visualizations, and reproducible statistical reports.

| # | Project | What It Tests | Difficulty |
|---|---------|--------------|------------|
| 41 | wine-quality-eda | EDA, distributions, outliers, multivariate analysis, ggplot2 | Medium |
| 42 | co2-emissions-analysis | 12 visualizations, linear regression, OWID data | Medium |
| 43 | electricity-load-forecasting | Time-series forecasting, R-Shiny dashboard | Hard |
| 44 | icu-kidney-injury-prediction | Clinical statistics, logistic regression, survival analysis | Hard |
| 45 | reproducible-research-template | Academic paper pipeline, BibTeX, Makefile, LaTeX | Medium |

Reference: [Ashish25/EDA_Rmd](https://github.com/Ashish25/EDA_Rmd), [hyunjoonbok/R-projects](https://github.com/hyunjoonbok/R-projects)

### SQL / Database (5 projects)

Tests the agent's ability to generate DDL schemas, complex queries, stored procedures, triggers, and migrations.

| # | Project | What It Tests | Difficulty |
|---|---------|--------------|------------|
| 46 | data-warehouse-etl | Star schema, ETL pipeline (Bronze→Silver→Gold), CTEs, window functions | Hard |
| 47 | railway-reservation-system | PostgreSQL, PL/pgSQL, row-level security, triggers | Hard |
| 48 | hospital-management | MySQL, ER design, normalization, complex joins, views | Medium |
| 49 | restaurant-management-db | Schema design, 30 query exercises, indexing | Medium |
| 50 | bank-account-system | PostgreSQL, transactions, isolation levels, constraints | Hard |

Reference: [tushar2704/SQL-Portfolio](https://github.com/tushar2704/SQL-Portfolio), [ptyadana/SQL-Data-Analysis-and-Visualization-Projects](https://github.com/ptyadana/SQL-Data-Analysis-and-Visualization-Projects)

## Skills Required Per Category

| Category | Existing Skills | New Skills Needed |
|----------|----------------|-------------------|
| Go | go-patterns, go-testing | - |
| Python | python-patterns, python-testing | - |
| ML Python | ml-patterns, python-patterns | - |
| Rust | rust-patterns | - |
| TypeScript | javascript-patterns | - |
| Java | java-spring | - |
| C# | csharp-patterns | - |
| Jupyter (.ipynb) | ml-patterns | notebook-patterns (NEW) |
| R Markdown (.Rmd) | - | r-patterns (NEW) |
| SQL | sql-expert | sql-expert verify commands (NEW) |

## Workflow

1. Clone original repo to `~/Development/Project_Review/`
2. Analyze functionality, write `~/Development/<project>/spec.md`
3. Synapserouter builds from spec:
   ```bash
   cd ~/Development/<project>
   synroute chat --spec-file spec.md
   ```
4. When the router fails or produces bad output → that's a router bug
5. Fix the router (skills, pipeline, escalation, tools, prompts)
6. Re-run the same project to verify the fix
7. Move to next project, repeat

## Failure Categories

| Category | Description | Example Fix |
|----------|-------------|-------------|
| `skill_gap` | No skill for language/framework | Create new skill .md |
| `architecture` | Wrong patterns chosen | Add patterns to skill |
| `tool_usage` | Tools used incorrectly | Fix tool implementation |
| `escalation` | Stuck on weak model | Fix escalation thresholds |
| `pipeline` | Skipped phases | Fix pipeline enforcement |
| `scope` | Overwhelmed by project size | Improve task decomposition |
| `context` | Lost context mid-task | Fix context management |
| `dependency` | Can't set up build env | Improve environment detection |
| `output_format` | Wrong file format (ipynb/Rmd/SQL) | Add format-specific skill |
| `destructive` | Agent deleted/destroyed work | Add safety guards to tools |

## Test Results

| # | Project | Language | Result | Bugs Found |
|---|---------|----------|--------|------------|
| 7 | diamond-price-prediction | ML Python | PASS (R²=0.981, 6/6 phases) | 11 router bugs fixed |
| 3 | fd | Rust | IN PROGRESS | Bash timeout fix, rm -rf safety |

## Router Bugs Found & Fixed

| Bug | Category | Fix |
|-----|----------|-----|
| Models call wrong tool names | tool_usage | Blocklist in system prompt |
| Agent exits after plan phase | pipeline | Removed toolCallCount gate |
| Memory injects 163 msgs (overflow) | context | 8K cap, negative clamp |
| Level 0 models can't function-call | escalation | Stall detection → escalate |
| Pipeline infinite loop on quality gate | pipeline | Phase retry cap (5→escalate, 10→skip) |
| Sub-agent uses wrong provider | escalation | Children inherit escalation chain |
| Phase signals ignored during tool calls | pipeline | Check advancePipeline after tools |
| Agent runs full ML training | tool_usage | Execution rules in prompt |
| PhasePrompt returns raw %s | pipeline | Descriptive placeholder |
| Pipeline phases never engage | pipeline | Inject phase prompt at startup |
| Bash timeout doesn't kill children | tool_usage | Start()+Wait()+select pattern |
