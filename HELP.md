# Synapserouter Help

## Available Commands

### Primary Commands:
- `synroute` or `synroute code` - Start interactive code mode TUI (default)
- `synroute serve` - Start HTTP server on port 8080
- `synroute test` - Smoke test all providers
- `synroute test --provider <name>` - Test specific provider
- `synroute test --json` - JSON output format

### Evaluation Commands:
- `synroute eval import --source <source> --path <path>` - Import benchmark data
- `synroute eval import --source polyglot --path ~/polyglot-benchmark`
- `synroute eval import --source roocode --path ~/Roo-Code-Evals`
- `synroute eval import --source exercism --path ~/exercism-go --language go`
- `synroute eval import --source multiple --path ~/MultiPL-E`
- `synroute eval import --source evalplus --path ~/evalplus`
- `synroute eval import --source codecontests --path ~/code_contests --count 500`
- `synroute eval import --source ds1000 --path benchmarks/ds1000`

### Other Commands:
- `synroute chat` - Interactive chat REPL
- `synroute profile` - Show active profile configuration
- `synroute doctor` - Run diagnostics
- `synroute models` - List available models
- `synroute version` - Show version
- `synroute mcp-serve` - Start MCP server

## Getting Started

1. **Build the application:**
   ```bash
   go build -o synroute .
   ```

2. **Run tests:**
   ```bash
   go test ./...
   ```

3. **Start the server:**
   ```bash
   ./synroute serve
   ```

4. **Run in code mode:**
   ```bash
   ./synroute code
   ```

## Profiles

The system supports two profiles configured via `ACTIVE_PROFILE` env var:

- **personal**: Ollama Cloud (primary) + optional subscription providers (gemini, codex, claude-code)
- **work**: Vertex AI with 3-tier chain (haiku→sonnet+gemini→opus+gemini)

## Environment Variables

Key environment variables to configure:
- `ACTIVE_PROFILE`: Set to 'personal' or 'work'
- `OLLAMA_CHAIN`: Provider escalation chain
- `OLLAMA_API_KEYS`: Multiple API keys for concurrency
- `SUBSCRIPTIONS_DISABLED`: Disable subscription providers
- `SYNROUTE_CONVERSATION_TIER`: Set conversation tier

## Documentation

See `docs/specs/Synapserouter-Spec.md` for detailed product specification.

## Need Help?

For more detailed documentation, check the source code comments and the spec document. The system includes comprehensive inline documentation explaining all features and configuration options.