# Skill: SynapseRouter Research

## Triggers
Use this skill when the user says: "research", "look up", "find out about", "what does X API do", "how does X work", "investigate", "dig into", "explore"

## Process

1. Delegate to `@research-assistant` subagent
2. The subagent has access to research-mcp (Gemini + Google Search grounding)
3. Research results are kept in the subagent's context to avoid bloating the main conversation
4. The subagent returns a concise summary with key findings and sources
