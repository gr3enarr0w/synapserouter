# Synapserouter Documentation

Go-based LLM proxy router and coding agent that distributes requests across multiple providers with automatic escalation, skill-based dispatch, and an interactive agent REPL.

**Status: Early development (Phase 1)**

The core router and agent are functional. Most of the roadmap -- chat backend, terminal UI, rich content, scale -- is still ahead. See [[Synapserouter]] for the full project overview.

## What Works Today

- Provider routing with dynamic multi-level escalation chain
- Code mode TUI with status bar, scroll regions, keyboard shortcuts (^P pipeline, ^T tools, ^L verbosity, ^E escalate, ^/ help)
- Interactive agent with 10 built-in tools (bash, file read/write/edit, grep, glob, git, web_search, web_fetch, notebook_edit) + 2 agent tools (delegate, handoff)
- Web search (DuckDuckGo, Tavily, SearXNG backends) and web fetch (SSRF-safe)
- File attachments via `@file` and `@dir/` references with path traversal protection
- Token streaming via SSE (StreamingProvider interface, Ollama implements it)
- Text-based tool call parser (5 formats for Ollama models without native function calling)
- Loop/stall detection in all modes, completion signal detection, response truncation at 4000 chars
- Regression tracking (RegressionTracker) and review stability/divergence detection
- Notebook support: file_read renders .ipynb cells, notebook_edit edits by cell index
- 54 embedded skills with trigger-based matching and language-field routing
- Spec compliance system with constraint extraction and tool-layer protection
- Worktree isolation for safe code changes
- MCP server mode
- Eval framework with 11 benchmark sources
- Two profiles: personal (Ollama Cloud, frontier tier) and work (Vertex AI, 3-tier chain: haiku→sonnet+gemini→opus+gemini)
- Configurable conversation tier via `SYNROUTE_CONVERSATION_TIER` env var
- Work profile: optional models.corp OpenAI-compatible provider
- Multi-mode Ctrl-C handling (cancel LLM call during generation, double-press to exit at idle)
- Permission prompting (y/n/a via /dev/tty) in chat mode

## What's Being Built

- Bug fixes for memory, recall, and data loss (Epic 0)
- Pipeline architecture improvements (Epic 1)
- Test coverage and thread safety (Epic 2)
- Skill trigger quality (Epic 3)

## Vault Structure

- **architecture/** -- How the system works internally
- **features/** -- Each major feature with spec + current status
- **guides/** -- User guide, dev guide, troubleshooting
- **reference/** -- API endpoints, CLI commands, configuration
- **specs/** -- Requirements, roadmap, and acceptance criteria

Open this folder in Obsidian for wiki-link navigation.

## Quick Links

- [[Synapserouter]] -- Project overview
- [[Architecture Overview]]
- [[Memory System]]
- [[Provider Chain]]
- [[Skill System]]
- [[Agent Pipeline]]
- [[Known Gaps]]
- [[Requirements]] -- Full roadmap with status
