package orchestration

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
)

// MatchSkills returns all skills whose triggers match the given goal text.
// Multi-word triggers use substring matching (e.g. "api key" matches anywhere).
// Single short words (<=4 chars) use word-boundary matching to avoid false
// positives like "go" matching "going" or "got".
func MatchSkills(goal string, registry []Skill) []Skill {
	text := strings.ToLower(goal)
	words := tokenize(text)
	var matched []Skill
	seen := make(map[string]bool)

	for _, skill := range registry {
		if seen[skill.Name] {
			continue
		}
		for _, trigger := range skill.Triggers {
			trigger = strings.ToLower(trigger)
			if matchesTrigger(text, words, trigger) {
				matched = append(matched, skill)
				seen[skill.Name] = true
				break
			}
		}
	}

	return matched
}

// ambiguousWords are short words that commonly appear as prefixes in unrelated
// words (e.g. "go" in "going"/"got", "do" in "doing"/"done").
// These require exact word-boundary matching.
var ambiguousWords = map[string]bool{
	"go": true, "do": true, "is": true, "it": true, "or": true,
	"an": true, "as": true, "at": true, "be": true, "by": true,
	"in": true, "no": true, "of": true, "on": true, "so": true,
	"to": true, "up": true, "us": true, "we": true,
}

// matchesTrigger checks if a trigger matches the goal text.
// Compound triggers with "+" (e.g. "go+handler") require ALL parts to match.
// Multi-word triggers and triggers with special chars use substring matching.
// Short ambiguous words (like "go") use exact word matching to prevent
// false positives ("go" matching "going"/"got"). All other triggers use
// substring matching.
func matchesTrigger(text string, words []string, trigger string) bool {
	// Compound triggers: "go+handler" means both "go" AND "handler" must match
	if strings.Contains(trigger, "+") {
		parts := strings.Split(trigger, "+")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if !matchesTrigger(text, words, part) {
				return false
			}
		}
		return true
	}

	// Multi-word triggers or triggers with special chars: substring match
	if strings.Contains(trigger, " ") || strings.ContainsAny(trigger, ".-_/") {
		return strings.Contains(text, trigger)
	}

	// Short ambiguous words: require exact word match
	if ambiguousWords[trigger] {
		for _, w := range words {
			if w == trigger {
				return true
			}
		}
		return false
	}

	// Everything else: substring match
	return strings.Contains(text, trigger)
}

// tokenize splits text into lowercase words, stripping punctuation.
func tokenize(text string) []string {
	return strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

// BuildSkillChain orders matched skills by phase and resolves dependencies.
// Skills are sorted by phase order (analyze → implement → verify → review),
// and dependencies are validated — a skill whose DependsOn references an
// unmatched skill has that dependency dropped (the chain still runs).
func BuildSkillChain(matched []Skill) []Skill {
	if len(matched) == 0 {
		return nil
	}

	// Deduplicate by name
	byName := make(map[string]Skill, len(matched))
	for _, s := range matched {
		byName[s.Name] = s
	}

	// Build deduplicated list
	skills := make([]Skill, 0, len(byName))
	for _, s := range byName {
		skills = append(skills, s)
	}

	// Sort by phase order, then alphabetically within phase for determinism
	sort.Slice(skills, func(i, j int) bool {
		pi := PhaseOrder[skills[i].Phase]
		pj := PhaseOrder[skills[j].Phase]
		if pi != pj {
			return pi < pj
		}
		return skills[i].Name < skills[j].Name
	})

	return skills
}

// SkillChainToSteps converts an ordered skill chain into TaskSteps for the
// orchestration engine. Each skill becomes a step with a skill-aware prompt.
func SkillChainToSteps(chain []Skill, goal string) []TaskStep {
	steps := make([]TaskStep, 0, len(chain))
	for i, skill := range chain {
		steps = append(steps, TaskStep{
			ID:     stepID(i),
			Role:   skill.Role,
			Prompt: skillPrompt(skill, goal),
			Status: StepStatusPending,
		})
	}
	return steps
}

// Dispatch is the main entry point: goal → skill chain → task steps.
// If no skills match, it returns nil (caller should fall back to role-based planning).
func Dispatch(goal string, registry []Skill) []TaskStep {
	matched := MatchSkills(goal, registry)
	if len(matched) == 0 {
		return nil
	}
	chain := BuildSkillChain(matched)
	return SkillChainToSteps(chain, goal)
}

// DispatchResult holds the full dispatch output for inspection/debugging.
type DispatchResult struct {
	Goal           string   `json:"goal"`
	MatchedSkills  []Skill  `json:"matched_skills"`
	SkillChain     []Skill  `json:"skill_chain"`
	Steps          []TaskStep `json:"steps"`
	FallbackToRole bool     `json:"fallback_to_role"`
}

// DispatchWithDetails returns the full dispatch result for debugging.
func DispatchWithDetails(goal string, registry []Skill) *DispatchResult {
	matched := MatchSkills(goal, registry)
	result := &DispatchResult{
		Goal:          goal,
		MatchedSkills: matched,
	}

	if len(matched) == 0 {
		result.FallbackToRole = true
		result.Steps = BuildPlan(goal, nil, 0)
		return result
	}

	chain := BuildSkillChain(matched)
	result.SkillChain = chain
	result.Steps = SkillChainToSteps(chain, goal)
	return result
}

// MCPToolsForChain returns the unique MCP tool names referenced by a skill chain.
func MCPToolsForChain(chain []Skill) []string {
	seen := make(map[string]bool)
	var tools []string
	for _, skill := range chain {
		for _, tool := range skill.MCPTools {
			if !seen[tool] {
				seen[tool] = true
				tools = append(tools, tool)
			}
		}
	}
	return tools
}

// VerifyCommandsForChain collects all verification commands from matched skills.
// Returns a formatted string of commands the reviewer should execute.
func VerifyCommandsForChain(chain []Skill) string {
	var b strings.Builder
	idx := 0
	for _, skill := range chain {
		for _, v := range skill.Verify {
			idx++
			if v.Manual != "" && v.Command == "" {
				// Manual-only check
				b.WriteString(fmt.Sprintf("%d. [MANUAL] [%s] %s\n", idx, skill.Name, v.Name))
				b.WriteString(fmt.Sprintf("   Check: %s\n\n", v.Manual))
				continue
			}
			b.WriteString(fmt.Sprintf("%d. [%s] %s\n", idx, skill.Name, v.Name))
			if v.Command != "" {
				b.WriteString(fmt.Sprintf("   Command: `%s`\n", v.Command))
			}
			if v.Expect != "" {
				b.WriteString(fmt.Sprintf("   Expected: output contains \"%s\"\n", v.Expect))
			}
			if v.ExpectNot != "" {
				b.WriteString(fmt.Sprintf("   Expected: output does NOT contain \"%s\"\n", v.ExpectNot))
			}
			if v.Manual != "" {
				b.WriteString(fmt.Sprintf("   [MANUAL] Also check: %s\n", v.Manual))
			}
			b.WriteString("\n")
		}
	}
	return b.String()
}

func skillPrompt(skill Skill, goal string) string {
	var b strings.Builder
	b.WriteString("[" + skill.Name + "] " + skill.Description)
	if skill.Instructions != "" {
		b.WriteString("\n\n--- Skill Instructions ---\n")
		b.WriteString(skill.Instructions)
		b.WriteString("\n--- End Instructions ---\n")
	}
	b.WriteString("\n\nApply to this goal: " + goal)
	return b.String()
}
