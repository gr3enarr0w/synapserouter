# SynapseRouter Agent Notes

See `CLAUDE.md` for full project context, skill triggers, and subagent delegation rules.

## Documentation guardrails

- use precise language for parity claims
- say `implemented slice` for partial ports
- say `targeted parity bucket complete` only for the exact verified subset
- do not claim full parity without an explicit audit
- do not describe `synroute` as an MCP server

## Current active docs

- `README.md`
- `STATUS.md`
- `BUGS.md`
- `FEATURE_ENHANCEMENTS.md`
- `BUG_GEMINI_001_FIX_PLAN.md`

## Current priorities

1. stabilize runtime bugs in `BUGS.md`
2. fix Gemini adapter inconsistency per `BUG_GEMINI_001_FIX_PLAN.md`
3. keep provider/browser-login work scoped to verified behavior
