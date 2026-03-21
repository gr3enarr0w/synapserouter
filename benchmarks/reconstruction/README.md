# Router Reconstruction Benchmark Suite

35 projects across 7 languages used to **find and fix weaknesses in the synapserouter agent**. Each project that fails reveals a gap — wrong tool usage, bad architecture decisions, missing skill knowledge, escalation problems, etc. We fix the router, re-run, and iterate until it handles all 35.

This is not a scorecard. It's a training loop for improving the router.

## Projects

### Machine Learning (Python)
1. goodbooks-10k — Book recommendation system
2. handson-ml2/end_to_end_project — Housing price prediction pipeline
3. credit-card-fraud-detection — Fraud detection with imbalanced data
4. stock-prediction-machine-learning — Stock forecasting with ML
5. End-to-End-Diamond-Price-Prediction — Diamond price regression

### Rust
1. ripgrep — Fast recursive grep
2. bat — cat clone with syntax highlighting
3. fd — Fast file finder
4. starship — Cross-shell prompt
5. vaultwarden — Bitwarden-compatible password server

### Go
1. cli/cli — GitHub CLI
2. rclone — Cloud storage sync
3. syncthing — Distributed file sync
4. caddy — HTTP/2 web server with auto HTTPS
5. restic — Encrypted backup tool

### Python
1. yt-dlp — Video downloader
2. httpie/cli — HTTP client CLI
3. home-assistant/core — Home automation platform
4. streamlit — Data app framework
5. prefect — Workflow orchestration

### TypeScript
1. n8n — Workflow automation
2. outline — Knowledge base wiki
3. cal.com — Scheduling platform
4. umami — Web analytics
5. directus — Headless CMS

### Java
1. spring-petclinic — Spring Boot sample app
2. jhipster-sample-app — Full-stack generator output
3. java-design-patterns — Design pattern implementations
4. airbyte — ETL/EL platform
5. quarkus-quickstarts — Quarkus sample apps

### C#
1. jellyfin — Media server
2. duplicati — Encrypted backup
3. ShareX — Screen capture + sharing
4. eShop — Microservices reference app
5. PowerShell — Shell + scripting language

## Architecture

Two separate actors — synapserouter never sees the original code:

```
Claude Code (examiner)              Synapserouter (test subject)
─────────────────────               ────────────────────────────
~/eval-repos/                       ~/Development/
  ripgrep/  (git clone)               ripgrep-impl/  (built from spec)
  caddy/    (git clone)               caddy-impl/    (built from spec)
  ...                                  ...

benchmarks/reconstruction/specs/    ← shared: specs written by Claude Code,
                                      read by synapserouter
```

- **Claude Code** clones repos into `~/eval-repos/`, reads the code, writes specs
- **Specs** are markdown files in `benchmarks/reconstruction/specs/`
- **Synapserouter** reads only the spec, runs `synroute chat --project <name>` to build it
- **Claude Code** compares the implementation against the original and scores it

Synapserouter NEVER sees the original repos. Only the spec.

## Workflow
1. Claude Code downloads original repo to ~/eval-repos/
2. Claude Code analyzes functionality, writes spec to benchmarks/reconstruction/specs/
3. Synapserouter implements from spec only (`synroute chat --project <name> --message "implement this spec"`)
4. When the router fails or produces bad output → that's a router bug
5. Fix the router (skills, pipeline, escalation, tools, prompts)
6. Re-run the same project to verify the fix
7. Move to next project, repeat

Each project failure teaches us what to fix. The 35 projects cover CLI tools, web apps, ML pipelines, distributed systems, etc. — forcing the router to handle every category.

## Evaluation Metrics
- Feature Coverage
- Architecture Similarity
- Correctness
- Test Coverage
- Performance
- Code Quality
- Modularity
- Documentation
- Error Handling
- Edge Case Handling
- Scalability
- Maintainability
