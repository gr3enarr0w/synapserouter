---
name: doc-coauthoring
description: "3-stage structured doc writing — context gathering, refinement, reader testing."
triggers:
  - "write doc"
  - "draft"
  - "proposal"
  - "blog post"
  - "technical spec"
  - "decision doc"
role: writer
phase: implement
---
# Skill: Doc Coauthoring

3-stage structured doc writing — context gathering, refinement, reader testing.

Source: [anthropics/doc-coauthoring](https://github.com/anthropics/skills/tree/main/skills/doc-coauthoring) (89K stars), [affaan-m/article-writing](https://github.com/affaan-m/everything-claude-code/tree/main/skills/article-writing) (70K stars).

---

## When to Use

- Writing documentation, proposals, technical specs, decision docs
- Drafting blog posts or long-form content
- Creating structured content that needs iteration
- Any document that benefits from a review workflow

---

## Stage 1: Context Gathering

### Ask meta-context questions
- What type of document? (spec, proposal, guide, blog post)
- Who is the audience? (technical, non-technical, mixed)
- What's the goal? (inform, persuade, instruct)
- What format constraints exist? (length, template, style guide)

### Gather information
- Let the user dump all relevant context
- Pull from existing code, docs, and data as needed
- Ask clarifying questions to fill gaps

---

## Stage 2: Refinement & Structure

For each section:
1. **Clarifying questions** — what specifically should this section cover?
2. **Brainstorm options** — generate 5-20 possible approaches/angles
3. **Curate** — select the best 2-3 with the user
4. **Gap check** — what's missing?
5. **Draft** — write the section
6. **Iterate** — refine based on feedback

### Quality checkpoint
After 3 iterations without substantial changes → move to next section.

### Near-completion review
At 80%+ done, do a full-document review:
- Flow and coherence between sections
- Consistent voice and terminology
- No contradictions or gaps

---

## Stage 3: Reader Testing

### Automated testing (via subagent)
1. Predict 5-10 questions a reader would ask
2. Test with a fresh Claude instance (no prior context)
3. Run ambiguity checks — are any statements unclear?

### Manual testing
- Read the doc from the reader's perspective
- Mark any confusing passages
- Verify all claims are supported

---

## Voice Consistency (from article-writing)

### Anti-template rules
- Never start with "In this article, we will..."
- Avoid passive voice — use direct "You" addressing
- No filler paragraphs — every sentence must add value
- Use specific examples, not generic ones
- Vary sentence length for rhythm

### Tone matching
- Technical docs: precise, direct, code examples
- Blog posts: conversational, story-driven, takeaways
- Proposals: evidence-based, clear recommendations, next steps
- Guides: step-by-step, screenshot placeholders, troubleshooting

---

## Final Checklist

- [ ] Purpose stated clearly in opening
- [ ] Audience-appropriate language
- [ ] Logical section flow
- [ ] Consistent terminology
- [ ] Actionable conclusions
- [ ] Proofread for typos and grammar
