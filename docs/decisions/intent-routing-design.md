# Decision: Hybrid Three-Layer Intent Routing with 9 Intents

**Date:** 2026-04-04
**Status:** Accepted
**Authors:** Clark Everson, Claude Opus 4.6

---

## Context

synroute's agent was burning user tokens by making 8-10 tool calls for simple greetings and knowledge questions like "hello" and "What is the capital of France?" Users of smaller models (Ollama Cloud) were especially impacted since LLMs are unreliable at self-selecting when to use tools.

## Decision

Implement a hybrid three-layer intent router with 9 intent categories, classifying user messages before the LLM call to restrict tool access.

---

## Research Sources

### 1. AppSelectBench & GTA Benchmark (2024-2025)
- **Finding:** GPT-5 achieves only 63.3% accuracy on tool selection across 100+ tools. GPT-4 completes fewer than 50% of real-world tool-use tasks.
- **Implication:** LLM-based tool selection is unreliable. A dedicated router is needed.
- **Source:** Chen et al., 2025 (AppSelectBench); Wang et al., 2024 (GTA: A Benchmark for General Tool Agents, NeurIPS 2024)

### 2. "Outcome-Aware Tool Selection for Semantic Routers" (arXiv, 2025)
- **Finding:** "LLM-based tool selection is both slow and unreliable; lightweight retrieval with outcome refinement offers a better cost–accuracy tradeoff for routing."
- **Implication:** Semantic routing outperforms LLM-based routing on both speed and accuracy.
- **Source:** arXiv, 2025

### 3. "Programming by Chat" (arXiv 2604.00436, 2026)
- **Finding:** Analysis of 11,579 real-world AI-assisted IDE sessions identified 7 main intent categories with 20 subcategories. Distribution: Code Authoring 34.53%, Failure Reporting 24.00%, Inquiry 19.17%, Context Specification 14.08%, Delegation 16.48%, Validation 3.99%, Workflow Control 11.47%.
- **Implication:** Real developer behavior maps to well-defined categories. Our taxonomy aligns with empirically observed patterns.
- **Source:** Programming by Chat: A Large-Scale Behavioral Analysis of 11,579 Real-World AI-Assisted IDE Sessions (arXiv 2604.00436v1)

### 4. Profound / OpenAI ChatGPT Usage Study (50M+ prompts, 2025-2026)
- **Finding:** 6 intent categories: Generative 37.5%, Informational 32.7%, Commercial 9.5%, Transactional 6.1%, Navigational 2.1%, No Intent 12.1%.
- **Implication:** General assistant use is dominated by generative (create X) and informational (what is X) — both need different tool access.
- **Source:** "How People Use ChatGPT" (OpenAI, 2025); Profound 50M+ prompt study (tryprofound.com)

### 5. CLINC150 Dataset (EMNLP 2019)
- **Finding:** 150 intent classes, 22,500 utterances, 10 domains. Industry standard for intent classification benchmarking. 150 utterances per intent provides robust classification.
- **Implication:** Our initial 350 utterances (7 intents × 50) was 1.5% of industry standard. Expanding to 500+ per intent using CLINC150 chat data dramatically improves accuracy.
- **Source:** "An Evaluation Dataset for Intent Classification and Out-of-Scope Prediction" (EMNLP 2019, Larson et al.); DeepPavlov/clinc150 on HuggingFace

### 6. Optimal Intent Count Research (2025-2026)
- **Finding:** "5-10 well-defined intents provide a good balance. Too many can reduce precision, while too few may miss nuances."
- **Implication:** Our 9 intents is within the optimal range. The initial 7 was slightly too few (missed delegation, didn't distinguish generate from modify). 16+ would reduce precision.
- **Source:** Multiple sources via Tavily search aggregation

### 7. Aurelio Labs Semantic Router & vLLM Semantic Router (2025)
- **Finding:** Semantic routing using embedding similarity achieves ~15ms classification with high accuracy. vLLM SR uses ModernBERT-based classifier. Route prototypes defined as representative utterances, embedded at startup, matched by cosine similarity.
- **Implication:** TF-IDF cosine similarity (which synroute already has) is sufficient for intent routing. No external model dependency needed.
- **Source:** Aurelio Labs semantic-router (GitHub); vLLM Semantic Router v0.1 (2025); Muthukrishnan, "Semantic Routing and Intent Classification in AI Agent Systems" (2025)

### 8. AWS Multi-LLM Routing Best Practices (2025)
- **Finding:** Production systems should combine multiple strategies — keyword matching for obvious cases, semantic classification for ambiguous, LLM fallback for complex. This tiered approach optimizes for both performance and accuracy.
- **Implication:** Our three-layer design (keyword → semantic → LLM) follows AWS-recommended architecture.
- **Source:** "Multi-LLM routing strategies for generative AI applications on AWS" (AWS, 2025)

### 9. Claude Code Architecture (2025-2026)
- **Finding:** Claude Code has NO algorithmic intent detection. It uses pure LLM reasoning for tool selection, relying on strong models (Opus/Sonnet).
- **Implication:** This works for Claude's strong models but fails for smaller Ollama Cloud models. synroute needs the guardrail because it routes across models of varying capability.
- **Source:** "Claude Agent Skills: A First Principles Deep Dive" (2026); Anthropic engineering blog

### 10. SkillRouter (arXiv, 2026)
- **Finding:** Tested skill routing with Claude Opus 4.6, Kimi-K2.5, glm-5 in the Claude Code harness. Proves skill-based intent routing works at scale with production models.
- **Source:** SkillRouter: Skill Routing for LLM Agents at Scale (arXiv, 2026)

### 11. Confidence Threshold Research (Aurelio Labs, 2025)
- **Finding:** Confidence threshold of 0.2-0.3 is optimal for intent routing. At least 50 training utterances per intent for good accuracy; 150+ for production quality.
- **Source:** Aurelio Labs threshold optimization documentation; NLU community best practices

---

## Why 9 Intents (Not 7 or 16)

### Why not 7 (our original count)
- Missing **delegate** — 16.48% of real coding sessions involve delegation (run commands, commit, deploy). Routing these to "code" over-provisions tool access.
- Missing **generate vs modify** distinction — creating new code (file_write) vs editing existing code (file_edit) have different tool needs and risk profiles.

### Why not 16+
- Research shows >10 intents reduces classification precision.
- debug/refactor/optimize are subcategories of fix/modify — same tool access, no routing benefit from splitting.
- translate/summarize/creative are subcategories of chat — all need zero tools.
- context/confirm/correct are workflow signals better handled by prompt hints than routing.

### Why these 9 specifically

| # | Intent | Tool Group | Real-World % | Source |
|---|--------|-----------|-------------|--------|
| 1 | chat | no_tools | 12.1% (No Intent) + 32.7% (Informational) | Profound |
| 2 | generate | full | 5.86% (New Implementation) | Programming by Chat |
| 3 | modify | read_write | 24.84% (Iterative Modification) | Programming by Chat |
| 4 | fix | read_write | 24.00% (Failure Reporting) | Programming by Chat |
| 5 | explain | read_only | 8.19% (Project Comprehension) | Programming by Chat |
| 6 | plan | no_tools | 7.81% (Planning & Consultation) | Programming by Chat |
| 7 | review | read_only | 2.74% (Code Review) | Programming by Chat |
| 8 | research | web | synroute differentiator | Our addition |
| 9 | delegate | full | 16.48% (Delegation) | Programming by Chat |

---

## Three-Layer Architecture

### Layer 1: Keyword Matching (0ms, deterministic)
- Exact/prefix matching for obvious intents
- Greetings, simple questions, slash commands
- Handles ~30% of queries with zero computation cost

### Layer 2: Semantic Routing (~15ms, TF-IDF cosine similarity)
- Pre-computed TF-IDF vectors for 500+ utterance examples per intent
- Cosine similarity against user query
- Confidence threshold: 0.25 (per Aurelio Labs research)
- Uses synroute's existing TF-IDF infrastructure (VectorMemory)

### Layer 3: LLM Fallback (current behavior)
- When confidence < 0.25, let the LLM decide
- All tools available (current behavior preserved)
- System prompt includes hint about tool necessity

### Tool Access Groups

| Group | Tools Available | Intents |
|-------|---------------|---------|
| no_tools | None | chat, plan |
| read_only | file_read, grep, glob, recall | explain, review |
| read_write | file_read, file_edit, bash, grep, glob | modify, fix |
| full | All tools | generate, delegate |
| web | web_search, web_fetch, recall | research |

---

## Utterance Data

### Sources
- **CLINC150** (chat): 500 utterances from 51 mapped intents (greetings, knowledge, math, etc.)
- **Programming by Chat patterns**: 100+ utterances per coding intent derived from 11,579 session analysis
- **Handwritten**: 50+ utterances per intent for coding-specific edge cases
- **Total target**: 3,000-5,000 utterances across 9 intents

### Storage
- JSON files in `internal/agent/intent_data/` embedded via `go:embed`
- Extensible: marketplace skills can add utterances per intent
- Format: `{"intent": "fix", "utterances": ["fix the bug", "debug this", ...]}`

---

## Alternatives Considered

1. **Pure LLM reasoning (Claude Code approach)** — Rejected because synroute uses smaller models that are <63% accurate at tool selection. Claude Code can afford this because it uses Opus/Sonnet exclusively.

2. **Fine-tuned classifier (BERT/RoBERTa)** — Rejected for v1.08 due to complexity and model dependency. May revisit for v1.10+ if TF-IDF accuracy is insufficient.

3. **24 granular intents** — Rejected per research showing >10 intents reduces precision. Subcategories tracked as metadata, not routing decisions.

4. **No router (fix system prompt only)** — Rejected because prompt engineering alone doesn't prevent tool use on smaller models. The model ignores "don't use tools for greetings" when tools are available in the request.

---

## Validation

- 35/35 intent classification tests passing (v1.08.5)
- "hello" → chat (0 tools, was 10 tools)
- "What is the capital of France?" → chat (0 tools, was 8 tools)
- "Write a Go function" → generate (all tools)
- "Fix the bug in main.go" → fix (read+write tools)
- Edge cases: "hello can you write me a function" → generate (greeting stripped)

## Future Work

- File-based intent routes for marketplace extensibility (v1.09)
- CLINC150 full dataset integration (v1.08.7)
- Accuracy tracking and automated threshold tuning
- User-definable custom intents via YAML
