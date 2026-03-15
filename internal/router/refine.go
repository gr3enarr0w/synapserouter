package router

import (
	"context"
	"log"
	"strings"
	"unicode"

	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/orchestration"
	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/providers"
)

const refinementSystemPrompt = `Rewrite the user's vague message as a single imperative sentence (a command/request) using context from the conversation. Start with an action verb (check, fix, review, test, implement, diagnose, update, etc). Reply with ONLY the rewritten sentence. No markdown, no explanation, no code, no bullet points. One sentence only.

Example input: "whats going on with it" (context: implementing OAuth token refresh in Go)
Example output: Check the status of the Go OAuth token refresh implementation and verify it handles expired tokens correctly.

Example input: "hows it look" (context: wrote a circuit breaker)
Example output: Review the Go circuit breaker implementation for correctness and edge case handling.

Example input: "fix it" (context: auth handler returning wrong errors)
Example output: Fix the authentication handler to return proper error codes for expired tokens.`

// RefineIntent checks if a user prompt would benefit from refinement and, if so,
// uses the LLM with conversation context to rewrite it into a clear, actionable prompt.
// It modifies req.Messages in place (replaces last user message content).
// Returns refined=true if the prompt was rewritten, false if skipped.
//
// Three categories of prompts:
//   - Well-structured: code references, file paths, clear action+target → skip entirely
//   - Has triggers but sloppy: skill keywords present but messy phrasing → refine
//   - Vague: no technical content at all → refine (needs context most)
func (r *Router) RefineIntent(ctx context.Context, req *providers.ChatRequest, sessionID string) (refined bool, err error) {
	userMsg := lastUserMessage(req.Messages)
	if userMsg == "" {
		return false, nil
	}

	triggers := allSkillTriggers()
	need := needsRefinement(userMsg, triggers)

	if need == refinementNone {
		return false, nil
	}

	// No session or no memory = no context to refine with
	if sessionID == "" || r.vectorMemory == nil {
		return false, nil
	}

	// Retrieve conversation context — more for vague prompts
	contextBudget := 1000
	if need == refinementFull {
		contextBudget = 2000
	}
	relevant, err := r.vectorMemory.RetrieveRelevant(userMsg, sessionID, contextBudget)
	if err != nil {
		log.Printf("[Router] Refinement: failed to retrieve context: %v", err)
		return false, nil
	}
	if len(relevant) == 0 {
		return false, nil
	}

	// Build refinement request with context messages + the user prompt
	messages := make([]providers.Message, 0, len(relevant)+2)
	messages = append(messages, providers.Message{
		Role:    "system",
		Content: refinementSystemPrompt,
	})
	for _, msg := range relevant {
		if msg.Role == "tool" {
			continue // orphaned tool results lack ToolCallID, break Gemini/Claude APIs
		}
		messages = append(messages, providers.Message{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}
	messages = append(messages, providers.Message{
		Role:    "user",
		Content: userMsg,
	})

	refinementReq := providers.ChatRequest{
		Model:       "auto",
		Messages:    messages,
		MaxTokens:   80,
		Temperature: 0,
		SkipMemory:  true,
	}

	// Use the router's own provider chain for the refinement call
	resp, err := r.ChatCompletion(ctx, refinementReq, "")
	if err != nil {
		log.Printf("[Router] Refinement: LLM call failed: %v", err)
		return false, nil
	}

	if len(resp.Choices) == 0 || strings.TrimSpace(resp.Choices[0].Message.Content) == "" {
		return false, nil
	}

	refinedText := strings.TrimSpace(resp.Choices[0].Message.Content)

	// Post-process: if the LLM over-generated (returned multiple lines,
	// markdown, code blocks, etc.), extract just the first meaningful line.
	refinedText = extractFirstSentence(refinedText)

	// Don't replace if the LLM returned the same thing
	if strings.EqualFold(refinedText, strings.TrimSpace(userMsg)) {
		return false, nil
	}

	// Replace last user message in the request
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == "user" && strings.TrimSpace(req.Messages[i].Content) != "" {
			req.Messages[i].Content = refinedText
			break
		}
	}

	log.Printf("[Router] Refined (%s): %q → %q", need, userMsg, refinedText)
	return true, nil
}

// refinementLevel indicates how much refinement a prompt needs.
type refinementLevel string

const (
	refinementNone  refinementLevel = "none"  // well-structured, skip
	refinementLight refinementLevel = "light" // has triggers but sloppy
	refinementFull  refinementLevel = "full"  // vague, no technical content
)

// needsRefinement evaluates whether a prompt would benefit from LLM-assisted
// rewriting. Returns the level of refinement needed.
//
// Skip (refinementNone) only when the prompt is clearly well-structured:
//   - Contains code references (backticks, file paths, function syntax)
//   - Long AND well-structured (>100 chars with action verb + technical terms)
//
// Light refinement for prompts that have skill triggers but are sloppy:
//   - Has triggers but is short/fragmented
//   - Has triggers but reads like word vomit
//
// Full refinement for prompts with no technical content at all:
//   - Short, no triggers, no code references
func needsRefinement(text string, skillTriggers []string) refinementLevel {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return refinementNone
	}

	words := strings.Fields(trimmed)
	charCount := len(trimmed)
	lowerText := strings.ToLower(trimmed)
	lowerWords := tokenizeWords(lowerText)

	// Code references = the human is being precise, skip
	if containsCodeIndicators(trimmed) {
		return refinementNone
	}

	hasTriggers := matchesAnyTrigger(lowerText, lowerWords, skillTriggers)
	hasActionVerb := startsWithActionVerb(lowerWords)
	hasSpecificTarget := hasNounTarget(lowerWords)

	// Well-structured: action verb + trigger + enough detail
	if hasActionVerb && hasTriggers && charCount > 40 && hasSpecificTarget {
		return refinementNone
	}

	// Long, detailed, and has triggers — probably fine
	if charCount > 100 && hasTriggers && len(words) >= 10 {
		return refinementNone
	}

	// Has triggers but sloppy — light refinement to clean up
	if hasTriggers {
		// Short sloppy: "fix the thing in auth"
		// Fragmented: "auth stuff broken whatever"
		// Word vomit: "can you like fix the auth thing or whatever"
		return refinementLight
	}

	// Action verb but no triggers — might still be clear enough
	// "delete the old backups" — no skill triggers but actionable
	if hasActionVerb && hasSpecificTarget && charCount > 30 {
		return refinementNone
	}

	// No triggers, no code, no clear structure → full refinement
	return refinementFull
}

// hasNounTarget checks if there's a non-stopword noun after the first word.
// Catches "fix the bug" (has "bug") vs "fix it" (only stopword).
func hasNounTarget(words []string) bool {
	if len(words) < 2 {
		return false
	}
	for _, w := range words[1:] {
		if !stopWords[w] && len(w) > 2 {
			return true
		}
	}
	return false
}

var stopWords = map[string]bool{
	"the": true, "a": true, "an": true, "it": true, "its": true,
	"this": true, "that": true, "these": true, "those": true,
	"is": true, "are": true, "was": true, "were": true, "be": true,
	"been": true, "being": true, "have": true, "has": true, "had": true,
	"do": true, "does": true, "did": true, "will": true, "would": true,
	"could": true, "should": true, "may": true, "might": true, "can": true,
	"and": true, "but": true, "or": true, "not": true, "no": true,
	"so": true, "if": true, "then": true, "than": true, "too": true,
	"very": true, "just": true, "also": true, "like": true, "with": true,
	"for": true, "from": true, "into": true, "about": true, "some": true,
	"any": true, "all": true, "you": true, "your": true, "my": true,
	"me": true, "i": true, "we": true, "our": true, "to": true,
	"of": true, "in": true, "on": true, "at": true, "by": true,
	"up": true, "out": true, "off": true, "over": true, "how": true,
	"what": true, "when": true, "where": true, "why": true, "who": true,
}

// matchesAnyTrigger checks if the text matches any skill trigger.
func matchesAnyTrigger(lowerText string, lowerWords []string, skillTriggers []string) bool {
	for _, trigger := range skillTriggers {
		trigger = strings.ToLower(trigger)
		if strings.Contains(trigger, " ") || strings.ContainsAny(trigger, ".-_/") {
			if strings.Contains(lowerText, trigger) {
				return true
			}
		} else if ambiguousWordsRefine[trigger] {
			for _, w := range lowerWords {
				if w == trigger {
					return true
				}
			}
		} else {
			if strings.Contains(lowerText, trigger) {
				return true
			}
		}
	}
	return false
}

// ambiguousWordsRefine mirrors the dispatch matching logic for short words
var ambiguousWordsRefine = map[string]bool{
	"go": true, "do": true, "is": true, "it": true, "or": true,
	"an": true, "as": true, "at": true, "be": true, "by": true,
	"in": true, "no": true, "of": true, "on": true, "so": true,
	"to": true, "up": true, "us": true, "we": true,
}

var codeIndicators = []string{
	".go", ".py", ".js", ".ts", ".rs", ".java", ".rb", ".sh",
	"()", "func ", "def ", "class ", "import ", "package ",
	"::", "->", "=>",
}

func containsCodeIndicators(text string) bool {
	lower := strings.ToLower(text)
	for _, indicator := range codeIndicators {
		if strings.Contains(lower, indicator) {
			return true
		}
	}
	if strings.Contains(text, "`") {
		return true
	}
	return false
}

var actionVerbs = map[string]bool{
	"implement": true, "fix": true, "add": true, "create": true,
	"build": true, "write": true, "refactor": true, "update": true,
	"delete": true, "remove": true, "migrate": true, "deploy": true,
	"configure": true, "install": true, "test": true, "debug": true,
	"optimize": true, "review": true, "research": true, "investigate": true,
	"diagnose": true, "explain": true, "check": true, "design": true,
}

func startsWithActionVerb(words []string) bool {
	if len(words) == 0 {
		return false
	}
	return actionVerbs[words[0]]
}

func tokenizeWords(text string) []string {
	return strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

// extractFirstSentence pulls the first meaningful line from LLM output.
// If the LLM over-generated (returned paragraphs, markdown, code blocks),
// this extracts just the actionable request.
func extractFirstSentence(text string) string {
	// Strip markdown formatting
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimPrefix(text, "**")
	text = strings.TrimSuffix(text, "**")

	// If it starts with a code block or markdown header, it's not a prompt rewrite
	if strings.HasPrefix(text, "#") || strings.HasPrefix(text, "```") {
		// Try to find the first line before the code/markdown
		lines := strings.SplitN(text, "\n", 2)
		first := strings.TrimSpace(lines[0])
		first = strings.TrimPrefix(first, "#")
		first = strings.TrimSpace(first)
		if len(first) > 20 {
			return first
		}
	}

	// Split by newlines — take the first non-empty line
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Skip markdown artifacts
		if strings.HasPrefix(line, "```") || strings.HasPrefix(line, "---") ||
			strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") ||
			strings.HasPrefix(line, "|") {
			continue
		}
		// Strip leading markdown
		line = strings.TrimPrefix(line, "# ")
		line = strings.TrimPrefix(line, "## ")
		line = strings.TrimPrefix(line, "### ")
		line = strings.TrimLeft(line, "*_")
		line = strings.TrimRight(line, "*_")
		line = strings.TrimSpace(line)

		if len(line) > 10 {
			// Cap at ~300 chars to prevent runaway
			if len(line) > 300 {
				// Find a sentence boundary
				for _, sep := range []string{". ", "? ", "! "} {
					if idx := strings.Index(line, sep); idx > 20 && idx < 300 {
						return line[:idx+1]
					}
				}
				return line[:300]
			}
			return line
		}
	}

	return text
}

// allSkillTriggers returns all trigger strings from the default skill registry.
func allSkillTriggers() []string {
	skills := orchestration.DefaultSkills()
	var triggers []string
	for _, s := range skills {
		triggers = append(triggers, s.Triggers...)
	}
	return triggers
}
