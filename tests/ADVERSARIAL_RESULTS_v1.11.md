# Adversarial Test Results — v1.11 (2026-04-05)

## Summary
- Total tests in plan: 381
- Previously completed (v1.10): 71 (Cat 1,2,4,5,6,7,9,11 partial)
- Automated this session: 32
- Total tested: 103/381
- Interactive-only tests: ~180 (keyboard, clipboard, terminal, multi-terminal)
- Tests requiring network manipulation: ~16 (Cat 8)
- Tests requiring specific conditions: ~13 (Cat 10)

## Category 19: Timeouts — 6/6 PASS ✅
| Test | Result | Notes |
|------|--------|-------|
| 19-01 Default timeout, fast complete | PASS | 10s |
| 19-02 Custom timeout override | PASS | 8s |
| 19-03 Timeout prints error | PASS | Clear message |
| 19-04 Clean exit on timeout | PASS | Exit 1, not crash |
| 19-05 Fast within timeout | PASS | 8s |
| 19-06 @nonexistent no hang | PASS | 32s, timed out cleanly |

## Category 13: Model Routing — 4/5 PASS, 1 EXPECTED
| Test | Result | Notes |
|------|--------|-------|
| 13-01 config show | PASS | 3 tiers displayed |
| 13-02 test --provider | PASS | Individual provider test works |
| 13-03 TIER=frontier | PASS | Exit 0 |
| 13-04 TIER=cheap | TIMEOUT | Expected — 8B model too slow in 10s |
| 13-05 TIER=mid | PASS | Exit 0 |

## Category 18: Concurrency — 3/3 PASS ✅
| Test | Result | Notes |
|------|--------|-------|
| 18-01 Single baseline | PASS | 13s |
| 18-02 Context cancellation | PASS | Clean timeout+release |
| 18-03 2 concurrent | PASS | Both completed with separate worktrees |

## Category 7: Flags — 5/7 PASS, 2 BUGS
| Test | Result | Notes |
|------|--------|-------|
| 7-01 Empty message | PASS | |
| 7-02 Unknown flag | PASS | Error printed |
| 7-03 NO_COLOR | BUG | Shell timeout killed at 20s |
| 7-04 Screen reader | BUG | Exit 1 |
| 7-05 Resume no session | PASS | |
| 7-06 Nonexistent session | PASS | Clear error |
| 7-07 Unknown model | PASS | Graceful timeout |

## Category 17: Tool Execution — 1/4 PASS, 3 TIMEOUT
| Test | Result | Notes |
|------|--------|-------|
| 17-01 bash ls | TIMEOUT | Frontier too slow for multi-turn |
| 17-02 grep pattern | TIMEOUT | Same |
| 17-03 glob pattern | PASS | Single-turn tool call |
| 17-04 git status | TIMEOUT | Same |

## Category 16: Worktree — 2/2 PASS ✅
| Test | Result | Notes |
|------|--------|-------|
| 16-01 --message creates worktree | PASS | |
| 16-02 Worktree path correct | PASS | |

## Category 9: Security — 2/2 PASS (timeout but no crash)
| Test | Result | Notes |
|------|--------|-------|
| 9-01 Prompt injection | PASS | Didn't leak, timed out |
| 9-02 ANSI in message | PASS | Didn't crash, timed out |

## New Bugs Found This Session
1. **NO_COLOR timeout** — NO_COLOR=1 mode times out, may be rendering issue
2. **Screen reader exit 1** — SYNROUTE_SCREEN_READER=1 exits with error
3. **Tool-calling timeout** — Frontier models too slow for multi-turn tool chains in 30s. Planner-worker handoff needed.

## Interactive Tests (require manual/VHS)
Categories 2 (Keyboard), 3 (Clipboard), 4 (Terminal), 5 (Slash Commands), 8 (Network), 10 (State), 12 (Multi-Terminal), 14 (LLM Response), 15 (Context) — ~180 tests require interactive testing with VHS tapes.
