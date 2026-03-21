---
name: deep-research
description: "Deep web research using Gemini AI with Google Search grounding."
triggers:
  - "deep research"
  - "market research"
  - "competitor analysis"
  - "industry benchmark"
role: researcher
phase: analyze
mcp_tools:
  - "research-mcp.research_deep"
---
# Skill: Deep Research

Perform web research using Gemini AI with Google Search grounding. No external APIs — uses existing GCP service account via the research-mcp server.

## User-Invocable

- name: deep-research
- description: Research a topic using Gemini + Google Search grounding. Provide a research question after the command.
- arguments: query (the research topic or question)

## When to Use

- Researching market data, pricing, competitor analysis
- Looking up current technical documentation or best practices
- Gathering evidence for business cases or proposals
- Fact-checking claims with web sources
- Industry benchmarks and statistics

## Process

1. Parse the user's research question from the arguments
2. Determine appropriate depth:
   - Simple factual questions → use `mcp__research-mcp__research_search` for a quick lookup
   - Multi-faceted topics → use `mcp__research-mcp__research_deep` with `moderate` depth
   - Comprehensive research requests → use `mcp__research-mcp__research_deep` with `thorough` depth
3. Choose output format based on context:
   - `summary` for conversational answers
   - `report` for business cases or detailed analysis
   - `bullets` for quick reference or comparison lists
4. Present findings with source citations
5. If the user asks follow-up questions, use `research_search` for targeted lookups

## Examples

```
/deep-research IT ticket deflection rates from self-service portals
/deep-research cost of JSM Cloud agent seats at enterprise scale
/deep-research AI-assisted help desk ROI benchmarks
/deep-research predictive analytics in IT service management
```
