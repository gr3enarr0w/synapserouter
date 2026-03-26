# Synroute Feature Enhancements

Requested feature enhancements that are separate from the current bug register.

Scope notes:
- This file tracks requested enhancement work for the active `synroute` runtime.
- These are not bug fixes unless explicitly linked to a bug in [BUGS.md](/Users/ceverson/MCP_Advanced_Multi_Agent_Ecosystem/src/services/synapse-router/BUGS.md).
- These entries do not imply implementation or parity unless they are later marked complete.

## FEAT-OLLAMA-001

Title: Add Ollama Cloud access through local signed-in Ollama auth instead of requiring a direct API key

Status: Requested

Priority: Medium

Summary:
- Add an Ollama integration mode where `synroute` talks to the local Ollama daemon and relies on `ollama signin` authentication, instead of requiring `OLLAMA_API_KEY` for direct cloud access.

Current state:
- `synroute` only initializes Ollama Cloud when `OLLAMA_API_KEY` is set.
- Current implementation sends a bearer token directly to the configured Ollama base URL.
- Relevant code:
  - [main.go](/Users/ceverson/MCP_Advanced_Multi_Agent_Ecosystem/src/services/synapse-router/main.go)
  - [ollama.go](/Users/ceverson/MCP_Advanced_Multi_Agent_Ecosystem/src/services/synapse-router/internal/providers/ollama.go)

Requested behavior:
- Support a second auth mode where:
  - `synroute` sends requests to the local Ollama API, typically `http://localhost:11434/api`
  - `synroute` does not send an API key
  - the local Ollama daemon uses its own signed-in cloud session and SSH-key-backed identity to reach Ollama Cloud models

Recommended design:
- Add an auth mode switch, for example:
  - `OLLAMA_AUTH_MODE=api_key|local_signin`
- Behavior:
  - `api_key`
    - current behavior
    - base URL defaults to the direct cloud endpoint
    - bearer auth uses `OLLAMA_API_KEY`
  - `local_signin`
    - base URL defaults to the local Ollama API
    - no bearer token added by `synroute`
    - health check validates the local daemon, not cloud API-key presence

Why this is the right boundary:
- `synroute` should not implement or reverse-engineer Ollama’s internal SSH-key signing flow directly.
- The local Ollama daemon is the correct component to own signed-in cloud auth, key material, and session handling.
- This keeps `synroute` simpler and aligned with the local-user workflow.

User value:
- lets `synroute` use Ollama Cloud models on a signed-in workstation without copying API keys into router config
- better fit for personal desktop use
- still preserves direct API-key mode for server and headless deployments

Implementation outline:
1. add Ollama auth-mode config parsing in [main.go](/Users/ceverson/MCP_Advanced_Multi_Agent_Ecosystem/src/services/synapse-router/main.go)
2. update [ollama.go](/Users/ceverson/MCP_Advanced_Multi_Agent_Ecosystem/src/services/synapse-router/internal/providers/ollama.go) to support no-auth local mode
3. adjust health/readiness logic so local-signin mode does not require `OLLAMA_API_KEY`
4. expose the mode clearly in setup docs
5. optionally add model defaults better suited for local signed-in cloud usage

Acceptance criteria:
- `synroute` can initialize an Ollama provider with no `OLLAMA_API_KEY` when `OLLAMA_AUTH_MODE=local_signin`
- a local signed-in Ollama daemon can serve cloud-backed models through `synroute`
- provider health succeeds when the local daemon is signed in and reachable
- docs clearly distinguish direct-cloud API-key mode from local-signed-in mode

Open questions:
- whether `synroute` should auto-detect local signed-in availability when `OLLAMA_BASE_URL` points to localhost
- whether local-signin mode should be named `local_signin`, `daemon`, or `local_cloud`

## FEAT-JETBRAINS-ACP-001

Title: Add JetBrains ACP agents network integration

Status: Requested

Priority: Medium

Summary:
- Add a `synroute` enhancement for JetBrains ACP agents network support, so `synroute` can participate in or route through that agent/network surface as requested.

Current state:
- no explicit JetBrains ACP agents-network integration is tracked in the active `synroute` runtime
- no active implementation slice is documented for this capability

Requested behavior:
- support JetBrains ACP agents network as a first-class enhancement target, separate from the current provider and orchestration bug work

Intent of the enhancement:
- make `synroute` able to interoperate with JetBrains ACP agents-network workflows instead of treating them as out-of-band tooling
- keep this work separate from current provider-runtime stabilization

Proposed implementation track:
1. define the JetBrains ACP integration surface precisely
   - transport
   - auth
   - message/task format
   - agent discovery model
2. decide whether this belongs in:
   - `synroute` core runtime
   - an adapter layer inside `src/services/synapse-router`
   - a separate component that `synroute` talks to
3. add a narrow first implementation slice
   - connectivity
   - registration/discovery
   - one request/response execution path
4. document the capability separately from provider routing claims

Acceptance criteria:
- a concrete JetBrains ACP integration spec is written for `synroute`
- one implemented slice exists and is documented precisely
- the docs do not overclaim parity or completion

Open questions:
- exact ACP expansion and canonical protocol boundary to target
- whether the desired integration is:
  - agent discovery only
  - remote task execution
  - shared networked agent orchestration
  - IDE-mediated routing through `synroute`
- expected auth and trust model between JetBrains ACP and `synroute`

Notes:
- This should stay in the enhancement tracker until the target protocol and scope are pinned down more concretely.
