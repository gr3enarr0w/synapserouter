# Synapserouter Documentation

Go-based LLM proxy router and coding agent that distributes requests across multiple providers with automatic escalation, skill-based dispatch, deep research, and an interactive coding agent.

**New here?** Start with the [User Guide](./guides/User%20Guide.md) for installation and first-run instructions, or the [README](../README.md) for a quick start in 5 steps.

**Status: v1.05 — Performance + Model Optimization + Deep Search**

## What Works Today

### Core Router
- Provider routing with dynamic multi-level escalation chain (6 levels, 22 models)
- Circuit breakers with smart rate-limit cooldowns
- Two profiles: personal (Ollama Cloud) and work (Vertex AI)
- YAML tier configuration (`~/.synroute/config.yaml`) or OLLAMA_CHAIN env var

### Agent & Tools
- Code mode TUI with status bar, scroll regions, keyboard shortcuts (^P pipeline, ^T tools, ^L verbosity, ^E escalate, ^/ help)
- Interactive agent with 10 built-in tools (bash, file read/write/edit, grep, glob, git, web_search, web_fetch, notebook_edit) + 2 agent tools (delegate, handoff)
- File attachments via `@file` and `@dir/` references with path traversal protection
- Token streaming via SSE (StreamingProvider interface, Ollama implements it)
- Text-based tool call parser (5 formats for Ollama models without native function calling)
- Permission prompting (y/n/a via /dev/tty) in chat mode

### Search (19 backends, RRF fusion)
- Brave, Tavily, Serper, Exa, SearXNG, DuckDuckGo, You.com, SerpAPI, SearchAPI.io, Jina, Kagi, Linkup, Semantic Scholar, OpenAlex, GitHub Search, Sourcegraph, NewsAPI, Newsdata.io, TheNewsAPI
- Reciprocal Rank Fusion merges results from all configured backends in parallel
- Dynamic backend selection based on query type (code/academic/news/general)

### Deep Research Pipeline (v1.05)
- `/research [quick|standard|deep] <query>` command
- Multi-round search with saturation detection and budget enforcement
- Free-first backend ordering — paid backends only when needed
- Citation-aware synthesis with inline references
- Query type classification routes to matching backends automatically

### Speed Optimizations (v1.05)
- K-LLM parallel verification — K independent reviewers with finding merge
- Context compression — structured summaries, observation masking, 70% fill trigger
- Diff-based review — send git diffs to reviewers (60-80% token savings)
- Plan caching — reuse plans for similar tasks (~50% cost savings)
- PASTE speculative execution — pre-execute predicted read-only tools while LLM thinks

### Model Recommendation & Config (v1.05)
- `synroute recommend` — detects providers, suggests tier configurations with scores
- `synroute config show/generate` — YAML tier config management
- Bundled reference benchmark scores for ~30 common models

### Eval Framework
- 11 benchmark sources (exercism, ds1000, birdsql, dare-bench, evalplus, etc.)
- Latency percentiles (p50/p95/p99) and cost tracking
- Config-vs-config A/B testing
- Docker-based test execution

### Other
- Loop/stall detection in all modes, completion signal detection
- Regression tracking and review stability/divergence detection
- 54 embedded skills with trigger-based matching
- Spec compliance system with constraint extraction
- Worktree isolation for safe code changes
- MCP server mode
- Session persistence and resume

## Vault Structure

- **architecture/** — How the system works internally
- **features/** — Each major feature with spec + current status
- **guides/** — User guide, dev guide, troubleshooting
- **reference/** — API endpoints, CLI commands, configuration
- **specs/** — Requirements, roadmap, and acceptance criteria

Open this folder in Obsidian for wiki-link navigation.
