# Router Reconstruction Benchmark Suite

35 projects across 7 languages used to **find and fix weaknesses in the synapserouter agent**. Each project that fails reveals a gap — wrong tool usage, bad architecture decisions, missing skill knowledge, escalation problems, etc. We fix the router, re-run, and iterate until it handles all 35.

This is not a scorecard. It's a training loop for improving the router.

## Structure

Each project gets its own folder with a `spec.md` and an `impl/` directory:

```
benchmarks/reconstruction/
  spring-petclinic/
    spec.md           # Written by Claude Code (examiner)
    impl/             # Built by synapserouter (test subject)
  umami/
    spec.md
    impl/
  fd/
    spec.md
    impl/
  ...
```

- **spec.md** — Written by Claude Code after analyzing the original repo
- **impl/** — Where synapserouter builds the project from the spec alone
- Synapserouter NEVER sees the original repos. Only the spec.

## Projects

### Wave 1: Calibration (one per language with skills)
| # | Project | Language | Spec |
|---|---------|----------|------|
| 1 | spring-petclinic | Java | Done |
| 2 | umami | TypeScript | Done |
| 3 | fd | Rust | Done |
| 4 | goodbooks-10k | ML Python | Done |
| 5 | restic | Go | Done |

### Wave 2: Skill Gaps
| # | Project | Language | Spec |
|---|---------|----------|------|
| 6 | duplicati | C# | - |
| 7 | end-to-end-diamond-price-prediction | ML Python | - |
| 8 | httpie-cli | Python | - |
| 9 | quarkus-quickstarts | Java | - |
| 10 | bat | Rust | - |

### Wave 3: Architecture Complexity
| # | Project | Language | Spec |
|---|---------|----------|------|
| 11 | caddy | Go | - |
| 12 | n8n | TypeScript | - |
| 13 | outline | TypeScript | - |
| 14 | cli-cli | Go | - |
| 15 | credit-card-fraud-detection | ML Python | - |
| 16 | jhipster-sample-app | Java | - |
| 17 | java-design-patterns | Java | - |
| 18 | eshop | C# | - |
| 19 | directus | TypeScript | - |
| 20 | starship | Rust | - |

### Wave 4: Scale
| # | Project | Language | Spec |
|---|---------|----------|------|
| 21 | ripgrep | Rust | - |
| 22 | cal-com | TypeScript | - |
| 23 | prefect | Python | - |
| 24 | streamlit | Python | - |
| 25 | syncthing | Go | - |
| 26 | rclone | Go | - |
| 27 | vaultwarden | Rust | - |
| 28 | stock-prediction-machine-learning | ML Python | - |
| 29 | handson-ml2 | ML Python | - |
| 30 | sharex | C# | - |

### Wave 5: Boss Level
| # | Project | Language | Spec |
|---|---------|----------|------|
| 31 | yt-dlp | Python | - |
| 32 | home-assistant-core | Python | - |
| 33 | airbyte | Java | - |
| 34 | jellyfin | C# | - |
| 35 | powershell | C# | - |

## Architecture

Two separate actors — synapserouter never sees the original code:

```
Claude Code (examiner)              Synapserouter (test subject)
─────────────────────               ────────────────────────────
~/eval-repos/                       benchmarks/reconstruction/<project>/impl/
  ripgrep/  (git clone)               ripgrep/impl/  (built from spec)
  caddy/    (git clone)               caddy/impl/    (built from spec)
  ...                                  ...
```

- **Claude Code** clones repos into `~/eval-repos/`, reads the code, writes `spec.md`
- **Synapserouter** reads only the spec, builds in the project's `impl/` directory
- **Claude Code** compares the implementation against the original and scores it

## Workflow

1. Clone original repo to `~/eval-repos/<project>/`
2. Analyze functionality, write `benchmarks/reconstruction/<project>/spec.md`
3. Synapserouter builds from spec:
   ```bash
   synroute chat --spec-file benchmarks/reconstruction/<project>/spec.md \
     --message "Build this project in benchmarks/reconstruction/<project>/impl/"
   ```
4. When the router fails or produces bad output -> that's a router bug
5. Fix the router (skills, pipeline, escalation, tools, prompts)
6. Re-run the same project to verify the fix
7. Move to next project, repeat

## Failure Categories

Each failure is categorized to drive specific router fixes:

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

## Evaluation Metrics
- Feature Coverage
- Architecture Similarity
- Correctness
- Test Coverage
- Code Quality
- Error Handling
