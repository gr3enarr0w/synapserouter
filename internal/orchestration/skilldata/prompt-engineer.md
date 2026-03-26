---
name: prompt-engineer
description: "Prompt crafting, optimization, multi-agent orchestration prompts, model-specific tuning."
triggers:
  - "prompt"
  - "system prompt"
  - "few-shot"
  - "chain of thought"
role: architect
phase: analyze
---
# Skill: Prompt Engineer

Prompt crafting, optimization, multi-agent orchestration prompts, model-specific tuning.

Source: [Prompt Engineering Expert](https://mcpmarket.com/tools/skills/prompt-engineering-expert), [TomsTools11/prompt-engineering-expert](https://github.com/TomsTools11/prompt-engineering-expert).

---

## When to Use

- Writing or optimizing LLM prompts
- Designing system prompts for agents
- Creating structured output templates
- Debugging prompt failures or hallucinations

---

## Core Principles

1. **Clarity over cleverness** — direct instructions beat clever tricks
2. **Conciseness** — remove redundant words, but keep necessary context
3. **Appropriate freedom** — constrain where needed, allow flexibility elsewhere
4. **Examples beat descriptions** — show don't tell (few-shot)
5. **Test and iterate** — measure prompt quality, don't assume

---

## Techniques

### Chain-of-Thought
```
Analyze this ticket and classify it step by step:
1. Read the summary and description
2. Identify the core problem type
3. Map to a category from this list: [...]
4. Assign confidence based on clarity
```

### Few-Shot Learning
```
Classify these tickets:

Ticket: "Can't log in to Confluence after migration"
Category: Access
Issue Type: Login Failure

Ticket: "Dashboard widgets missing in Cloud"
Category: Configuration
Issue Type: Dashboard Migration

Ticket: "<new ticket>"
Category:
```

### Structured Output (XML tags)
```xml
<task>Classify the following Jira ticket</task>
<context>Migration from DC to Cloud, 5000+ users</context>
<constraints>
  - Use only these categories: [list]
  - Confidence must be 0.0-1.0
  - Include evidence from the ticket text
</constraints>
<output_format>JSON matching the TicketClassification schema</output_format>
```

### Role-Based
```
You are a senior Atlassian support engineer with 10 years of experience
in DC-to-Cloud migrations. Analyze this ticket from a support perspective.
```

---

## Model-Specific Tips

| Model | Tip |
|-------|-----|
| Claude | Use XML tags for structure, be direct, supports long context |
| Gemini | Structured output via Pydantic schemas, good with JSON |
| GPT | JSON mode for structured output, system message for role |

---

## Anti-Patterns

- **Vague instructions** — "make it better" → "reduce response to 3 bullet points"
- **Contradictions** — "be concise" + "explain in detail"
- **Over-specification** — constraining every word kills creativity
- **No examples** — descriptions without demonstrations
- **Ignoring failure modes** — not handling edge cases or refusals
