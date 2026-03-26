# Synapserouter Documentation

Go-based LLM proxy router and coding agent that distributes requests across multiple providers with automatic escalation, skill-based dispatch, and an interactive agent REPL.

**Status: Early development (Phase 1 of 5)**

The core router and agent are functional. Most of the roadmap -- chat backend, terminal UI, rich content, scale -- is still ahead. See [[Synapserouter]] for the full project overview.

## What Works Today

- Provider routing with 7-level escalation chain (19+ models)
- Interactive agent with tool execution (bash, files, grep, glob, git)
- 38+ embedded skills with trigger-based matching
- Worktree isolation for safe code changes
- MCP server mode
- Eval framework with 11 benchmark sources
- Two profiles: personal (Ollama Cloud) and work (Vertex AI)

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
