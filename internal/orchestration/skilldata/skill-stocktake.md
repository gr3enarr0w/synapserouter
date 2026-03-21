---
name: skill-stocktake
description: "Automated skill quality audit with verdicts — Keep, Improve, Update, Retire, or Merge."
triggers:
  - "stocktake"
  - "skill audit"
  - "skill review"
  - "skill quality"
role: reviewer
phase: review
---
# Skill: /skill-stocktake

Automated skill quality audit with verdicts — Keep, Improve, Update, Retire, or Merge.

Source: [affaan-m/skill-stocktake](https://github.com/affaan-m/everything-claude-code/tree/main/skills/skill-stocktake) (70K stars).

---

## When to Use

- Periodic quality review of all installed skills
- After adding multiple new skills
- When skills feel stale or overlapping
- To identify skills that should be merged or retired

---

## Scope

Scans both locations:
- `~/.claude/skills/` — Global skills
- `{cwd}/.claude/skills/` — Project-level skills

---

## Two Modes

### Quick Scan (5-10 min)
Re-evaluate only skills modified since last stocktake.

### Full Stocktake (20-30 min)
Complete review of all skills with detailed evaluation.

---

## Full Stocktake Process

### Phase 1: Inventory
```bash
echo "=== Global Skills ==="
ls -la ~/.claude/skills/*.md 2>/dev/null | wc -l
ls ~/.claude/skills/*.md 2>/dev/null

echo "=== Project Skills ==="
ls -la .claude/skills/*.md 2>/dev/null | wc -l
ls .claude/skills/*.md 2>/dev/null
```

### Phase 2: Quality Evaluation

For each skill, evaluate on 4 dimensions:

| Dimension | Question | Score |
|-----------|----------|-------|
| Actionability | Does it contain executable commands/patterns? | 1-5 |
| Scope fit | Is it correctly scoped (global vs project)? | 1-5 |
| Uniqueness | Does it overlap with other skills? | 1-5 |
| Currency | Are references/commands still valid? | 1-5 |

### Phase 3: Verdict

| Verdict | Meaning |
|---------|---------|
| **Keep** | Skill is high quality, no changes needed |
| **Improve** | Good foundation but needs specific enhancements |
| **Update** | References are stale, needs refresh |
| **Retire** | No longer useful, should be deleted |
| **Merge into [X]** | Overlaps with another skill, combine them |

### Phase 4: Summary Table

Generate a table of all skills with verdicts:
```
| Skill | Scope | Verdict | Reason |
|-------|-------|---------|--------|
| python-patterns | Global | Keep | Comprehensive, current references |
| how-to-writer | Project | Merge into kb-article-drafter | Overlapping functionality |
```

---

## Reason Quality Requirements

Good reasons are:
- Self-contained (no need to read the skill to understand)
- Decision-enabling (reader can act on the verdict)
- Specific (cite evidence: "references deprecated v2 API")

Bad reasons:
- "Looks good" (not actionable)
- "Needs improvement" (not specific)

---

## Results Storage

Save results to `.claude/stocktake-results.json`:
```json
{
  "evaluated_at": "2026-03-10T15:00:00Z",
  "mode": "full",
  "skills": [
    {
      "name": "python-patterns",
      "scope": "global",
      "verdict": "Keep",
      "reason": "Comprehensive Python patterns with current 3.12+ features",
      "mtime": "2026-03-10T14:00:00Z"
    }
  ]
}
```
