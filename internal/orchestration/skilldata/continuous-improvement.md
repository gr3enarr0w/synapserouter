---
name: continuous-improvement
description: "Instinct-based continuous learning — observe sessions, capture patterns, evolve skills."
triggers:
  - "learn"
  - "improve"
  - "pattern"
  - "instinct"
  - "retrospective"
role: reviewer
phase: review
---
# Skill: Continuous Improvement (Global)

Instinct-based continuous learning — observe sessions, capture patterns, evolve skills and code.

Source: [continuous-learning-v2.1](https://mcpmarket.com/tools/skills/continuous-learning-v2-1), [Claudeception](https://github.com/blader/Claudeception) (2K stars).

---

## When to Use

- After completing a significant task — capture what was learned
- When you notice a recurring pattern across sessions
- Periodic review of skill quality and accuracy
- When user corrections suggest a systematic issue

---

## Instinct Architecture

An **instinct** is an atomic learned behavior:

```json
{
  "trigger": "When doing X",
  "action": "Do Y instead of Z",
  "confidence": 0.5,
  "domain": "code-quality",
  "evidence": ["session 1 observation", "session 2 observation"]
}
```

### Confidence scoring
| Score | Behavior |
|-------|----------|
| 0.3 | Suggest only, needs confirmation |
| 0.5 | Apply when context matches |
| 0.7 | Auto-apply, log for transparency |
| 0.9 | Core behavior, always apply |

---

## Storage

```
.claude/learnings/
├── patterns.md         # Confirmed patterns worth codifying
├── anti-patterns.md    # Things that consistently fail
└── corrections.md      # User corrections log

.claude/instincts/
├── observations.jsonl  # Raw observation log
└── personal/           # Domain-specific instincts
```

---

## Quality Thresholds (before evolving)

| Check | Threshold |
|-------|-----------|
| Evidence count | >= 3 observations |
| Confidence | >= 0.7 for auto-apply |
| Recency | Last seen within 14 days |
| No contradictions | No opposing instincts at >= 0.5 |

---

## Key Principles

- **Observe before acting** — gather evidence first
- **Small atomic changes** — one instinct = one improvement
- **Regression prevention** — verify no breakage after changes
- **Document everything** — update MEMORY.md with confirmed patterns
