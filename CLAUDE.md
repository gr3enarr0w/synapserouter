# SynapseRouter (synroute)

Go-based LLM proxy router that distributes requests across subscription providers (Claude Code, Codex, Gemini) and direct providers (NanoGPT, Ollama, Vertex AI). Two profiles: `personal` (OAuth subscriptions) and `work` (Vertex AI). Auto-discovers 159+ models across all active providers.

## Key Files

- `main.go` — Server setup, CLI dispatch, provider initialization, HTTP handlers
- `commands.go` — CLI command implementations (test, profile, doctor, models, version)
- `diagnostic_handlers.go` — API endpoints for testing, diagnostics, circuit breaker reset, skill dispatch
- `internal/orchestration/skills.go` — Skill registry with trigger-based matching
- `internal/orchestration/dispatch.go` — Auto-dispatch engine: goal → skill chain → task steps
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

### Provider Chain (Personal Profile)
Cost-optimized escalation: free/included models first, paid API last.

`NanoGPT-Sub → Gemini → Codex → Claude Code → Ollama → NanoGPT-Paid`

- `nanogpt-sub`: NanoGPT subscription models (qwen, deepseek, etc. — zero cost)
- `gemini`, `codex`, `claude-code`: CLI subscription providers (free)
- `nanogpt-paid`: NanoGPT paid API models (claude/gpt/gemini via NanoGPT — costs money)
- List order = priority (`GetBestProvider()` returns first under threshold)
- Circuit breakers open on failures, parse "reset after Ns" from 429 errors
- Health checks cached (2min TTL) to avoid burning API quota

### NanoGPT Split
Two provider instances from one `NANOGPT_API_KEY`:
- `nanogpt-sub`: baseURL `https://nano-gpt.com/api/subscription/v1`, handles auto/subscription models
- `nanogpt-paid`: baseURL `https://nano-gpt.com/api/paid/v1`, handles paid API models only
- `SupportsModel` routes by tier; `nanogpt-paid` returns false for auto (only reachable via fallback)

### Profiles
- `personal`: NanoGPT-Sub + OAuth subscription providers + NanoGPT-Paid
- `work`: Vertex AI (Claude + Gemini via native GCP auth)
- Controlled by `ACTIVE_PROFILE` in `.env`

### Skill Auto-Dispatch
When a task/goal is submitted to orchestration, the dispatch engine automatically:
1. Matches goal text against skill triggers (keyword matching)
2. Orders matched skills by phase: `analyze → implement → verify → review`
3. Converts skill chain to TaskSteps with skill-aware prompts
4. Auto-invokes bound MCP tools and injects results as context
5. Falls back to role-based inference (`inferRoles`) if no skills match

Built-in skills: `go-patterns`, `python-patterns`, `security-review`, `code-implement`, `go-testing`, `python-testing`, `code-review`, `api-design`, `docker-expert`, `research`

### Key Patterns
- Gemini 2.5 models: thinking tokens from output budget, min 1024 maxOutputTokens enforced
- Codex: SSE streaming via `/responses` endpoint (NOT `/responses/compact`)
- NanoGPT-Sub: defaults to "qwen/qwen3.5-plus" for auto
- NanoGPT-Paid: defaults to "chatgpt-4o-latest" for auto (only via fallback)
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
curl localhost:8090/v1/skills                           # List registered skills
curl "localhost:8090/v1/skills/match?q=fix+the+Go+auth"  # Preview skill chain for a goal
```

## Auto-Dispatch Rules (MANDATORY)

Skills are a chain, not a menu. For ANY task, scan ALL available skills (user-level + project-level) and auto-invoke every one whose trigger matches. A single user request like "clean the code" should cascade through 3-5+ skills automatically:

1. **Detect language/domain** → invoke the matching pattern skill (go-patterns, python-patterns, etc.)
2. **Detect changes were made** → invoke code-review
3. **Detect tests exist** → invoke the testing skill (go-testing, python-testing, etc.)
4. **Detect security surface** → invoke security-review if auth/credentials/API keys are touched
5. **Detect verification needed** → invoke the project test workflow (synroute-test)

Never ask "should I run the tests?" or "want me to review?" — just do it. The skills exist to be used automatically.

### Skill Triggers — Auto-invoke when condition matches

| Condition | Skill | How to invoke |
|---|---|---|
| Writing or modifying Go code | `go-patterns` | Skill tool |
| Writing or modifying Go tests | `go-testing` | Skill tool |
| Testing, verifying, or checking | `synroute-test` | Read `.claude/skills/synroute-test.md` and execute its instructions |
| Reviewing code or changes | `code-review` | Skill tool |
| Security, credentials, or auth | `security-review` | Skill tool |
| Researching an API, library, or concept | `synroute-research` | Read `.claude/skills/synroute-research.md` and execute its instructions |
| Switching or checking profile | `synroute-profile` | Read `.claude/skills/synroute-profile.md` and execute its instructions |
| Docker or containers | `docker-expert` | Skill tool |
| Designing or modifying API endpoints | `api-design` | Skill tool |
| GitHub PRs, issues, or CI | `github-workflows` | Skill tool |

**Project skills** (`.claude/skills/*.md`) are not invocable via the Skill tool — read the file and follow its instructions directly.
**User-level skills** (`~/.claude/skills/`) are invocable via the Skill tool.

### Subagent Delegation — Auto-delegate when condition matches

| Condition | Agent | File |
|---|---|---|
| E2E provider testing | `@provider-tester` | `.claude/agents/provider-tester.md` |
| Code review of changes | `@code-reviewer` | `.claude/agents/code-reviewer.md` |
| Profile switching | `@profile-manager` | `.claude/agents/profile-manager.md` |
| Technical research | `@research-assistant` | `.claude/agents/research-assistant.md` |

### Standard Post-Change Pipeline

After ANY code change, automatically run this pipeline in order:
1. `go vet ./...` — catch issues early
2. `go test -race ./...` — unit tests with race detection
3. `./synroute test` — E2E provider smoke test (or delegate to `@provider-tester`)
4. Verify build: `go build -o synroute .`

Do NOT skip steps or ask whether to run them. The pipeline runs automatically after changes are complete.

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
