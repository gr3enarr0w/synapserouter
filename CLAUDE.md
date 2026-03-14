# SynapseRouter (synroute)

Go-based LLM proxy router that distributes requests across subscription providers (Claude Code, Codex, Gemini) and direct providers (NanoGPT, Ollama, Vertex AI). Two profiles: `personal` (OAuth subscriptions) and `work` (Vertex AI). Auto-discovers 159+ models across all active providers.

## Key Files

- `main.go` — Server setup, CLI dispatch, provider initialization, HTTP handlers
- `commands.go` — CLI command implementations (test, profile, doctor, models, version)
- `diagnostic_handlers.go` — API endpoints for testing, diagnostics, circuit breaker reset
- `compat_handlers.go` — OpenAI-compatible `/v1/chat/completions` and `/v1/responses` endpoints
- `internal/app/` — Shared logic for CLI and API (smoketest, diagnostics, profile, models)
- `internal/router/router.go` — Provider selection, fallback chain, circuit breakers, health caching
- `internal/providers/provider.go` — Provider interface (`Name`, `ChatCompletion`, `IsHealthy`, `SupportsModel`)
- `internal/providers/vertex.go` — Vertex AI provider (Claude via rawPredict, Gemini via generateContent)
- `internal/providers/nanogpt.go` — NanoGPT provider with tiered model routing
- `internal/subscriptions/providers.go` — OAuth subscription provider management
- `internal/subscriptions/credential_store.go` — Credential storage and OAuth refresh
- `internal/router/circuit.go` — Circuit breaker with smart rate-limit cooldowns and reset

## Architecture

### Provider Chain
Request → Router.selectProvider() → tryProvider() → fallback chain → response
- Selects provider with lowest usage among healthy candidates
- Circuit breakers open on failures, parse "reset after Ns" from 429 errors
- Health checks cached (2min TTL) to avoid burning API quota

### Profiles
- `personal`: OAuth subscription providers (Claude Code, Codex, Gemini CLI) via CLIProxy
- `work`: Vertex AI (Claude + Gemini via native GCP auth)
- Controlled by `ACTIVE_PROFILE` in `.env`

### Key Patterns
- Gemini 2.5 models: thinking tokens from output budget, min 1024 maxOutputTokens enforced
- Codex: SSE streaming via `/responses` endpoint (NOT `/responses/compact`)
- NanoGPT: defaults to "chatgpt-4o-latest" when model is "auto"
- Vertex Claude: use model names without dates (e.g. `claude-sonnet-4-6`)

## Dev Commands

```bash
go build -o synroute .                    # Build
go test ./...                              # Run all tests
go vet ./...                               # Lint
```

## CLI Commands

```bash
./synroute                                 # Start server (default)
./synroute serve                           # Start server (explicit)
./synroute test                            # Smoke test all providers
./synroute test --provider nanogpt         # Test single provider
./synroute test --json                     # JSON output
./synroute profile show                    # Show active profile
./synroute profile list                    # List available profiles
./synroute profile switch work             # Switch to work profile
./synroute doctor                          # Run diagnostics
./synroute doctor --json                   # JSON diagnostics
./synroute models                          # List available models
./synroute version                         # Show version info
```

## API Endpoints (Diagnostic)

```bash
curl -X POST localhost:8090/v1/test/providers          # Smoke test providers
curl -X POST localhost:8090/v1/circuit-breakers/reset   # Reset all circuit breakers
curl localhost:8090/v1/profile                          # Show profile info
curl -X POST localhost:8090/v1/profile/switch -d '{"profile":"work"}'
curl localhost:8090/v1/doctor                           # Run diagnostics
curl localhost:8090/health                              # Health check
curl localhost:8090/v1/models                           # List models
```

## Skill Triggers

Use these rules to activate the right skill for each request:

| When the user... | Use skill |
|---|---|
| Writes or modifies Go code | `go-patterns` |
| Asks to test, verify, or check | `synroute-test` |
| Asks to review code or changes | `code-review` |
| Asks about security, credentials, auth | `security-review` |
| Asks to research an API, library, or concept | `synroute-research` |
| Asks to switch profile or check profile | `synroute-profile` |
| Works with Docker or containers | `docker-expert` |
| Designs or modifies API endpoints | `api-design` |
| Works with GitHub PRs, issues, CI | `github-workflows` |
| Writes or modifies Go tests | `go-testing` |

## Subagent Delegation

| When the user... | Delegate to |
|---|---|
| Asks to test providers end-to-end | `@provider-tester` |
| Asks for a code review | `@code-reviewer` |
| Asks to switch profiles | `@profile-manager` |
| Asks to research a technical topic | `@research-assistant` |

## Documentation Guardrails

- Use precise language for parity claims
- Say "implemented slice" for partial ports
- Say "targeted parity bucket complete" only for the exact verified subset
- Do not claim full parity without an explicit audit
- Do not describe synroute as an MCP server — it is a standalone LLM router

## Do Not

- Put code style rules here (use linters and `go vet`)
- Add comments, docstrings, or type annotations to code you didn't change
- Over-engineer solutions beyond what was asked
- Commit `.env` or credential files
