# Synapse Router Status

**Date**: March 8, 2026
**Runtime**: one-process `synroute`

## Current State

- `synroute` is the active Go runtime for routing and orchestration
- subscription-backed provider access is embedded in-process
- SQLite-backed usage, memory, tasks, swarms, agents, and workflows are active
- `go test ./...` passes
- `go build ./...` passes

## Implemented

Routing and compatibility:
- OpenAI-compatible chat and models APIs
- OpenAI `responses` compatibility including stored response lifecycle
- provider-specific compatibility routes
- Amp config and upstream fallback behavior
- multi-account credential rotation and retry behavior

Embedded providers:
- Anthropic / Claude Code path
- OpenAI / Codex path
- Gemini path
- Qwen path
- Ollama Cloud fallback
- NanoGPT fallback

Subscription auth state:
- direct API-key auth is implemented
- supplied session-token or cookie auth is implemented
- built-in OAuth/browser login acquisition and refresh flow parity is implemented for Anthropic, OpenAI/Codex, and Gemini

Orchestration:
- tasks, refinement, pause, resume, cancel, assign
- swarms, scaling, coordination, rebalance
- agents, metrics, health, logs
- workflow templates and workflow runs
- SSE autopilot-style task events
- swarm load overview and imbalance detection
- rebalance preview
- work stealing
- steal contests and contest resolution
- workflow state, metrics, and debug views

## Still Missing For Full Rewrite Claims

`CLIProxyAPI`:
- built-in OAuth/browser login flow parity is implemented inside `synroute`
- formal full-repo parity audit against the archived reference
- any remaining upstream quirks not yet discovered by that audit

`ruflo`:
- deeper coordinator/voting/reconfiguration behavior
- nested/dependency/rollback workflow semantics closer to upstream `WorkflowEngine`
- broader MCP tool-family parity from the archived repo
- plugin and extension-point behavior
- formal full-repo parity audit against the archived reference

## What This File Means

This file tracks the real runtime state of `synroute`.

It does **not** mean:
- context persistence is fixed
- the whole repository is production ready
- full one-to-one parity with all archived upstream repos has been proven
