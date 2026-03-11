package orchestration

import (
	"strconv"
	"strings"
)

func DefaultRoles() []Role {
	return []Role{
		{Name: "researcher", Description: "Investigates context, alternatives, and constraints", Capabilities: []string{"research", "summarize", "compare"}},
		{Name: "architect", Description: "Designs the approach and defines the implementation plan", Capabilities: []string{"design", "planning", "tradeoffs"}},
		{Name: "coder", Description: "Produces implementation-ready changes", Capabilities: []string{"implementation", "refactor", "integration"}},
		{Name: "tester", Description: "Validates behavior and identifies edge cases", Capabilities: []string{"testing", "verification", "quality"}},
		{Name: "reviewer", Description: "Reviews the result for correctness and risk", Capabilities: []string{"review", "risk", "quality"}},
		{Name: "debugger", Description: "Diagnoses failures and narrows root causes", Capabilities: []string{"debugging", "triage", "root-cause"}},
		{Name: "documenter", Description: "Writes concise technical summaries and operator guidance", Capabilities: []string{"documentation", "handoff", "explanation"}},
	}
}

func BuildPlan(goal string, explicitRoles []string, maxSteps int) []TaskStep {
	roles := explicitRoles
	if len(roles) == 0 {
		roles = inferRoles(goal)
	}
	if maxSteps > 0 && len(roles) > maxSteps {
		roles = roles[:maxSteps]
	}

	steps := make([]TaskStep, 0, len(roles))
	for i, role := range roles {
		steps = append(steps, TaskStep{
			ID:     stepID(i),
			Role:   role,
			Prompt: stepPrompt(role, goal),
			Status: StepStatusPending,
		})
	}
	return steps
}

func BuildRefinementPlan(goal, feedback string, previous Task, maxSteps int) []TaskStep {
	roles := []string{"reviewer", "debugger", "coder", "tester", "reviewer"}
	if strings.TrimSpace(previous.Error) != "" {
		roles = []string{"debugger", "coder", "tester", "reviewer"}
	}
	if maxSteps > 0 && len(roles) > maxSteps {
		roles = roles[:maxSteps]
	}

	steps := make([]TaskStep, 0, len(roles))
	for i, role := range roles {
		steps = append(steps, TaskStep{
			ID:     stepID(i),
			Role:   role,
			Prompt: refinementPrompt(role, goal, feedback, previous.FinalOutput),
			Status: StepStatusPending,
		})
	}
	return steps
}

func inferRoles(goal string) []string {
	text := strings.ToLower(goal)
	roles := []string{"researcher", "architect"}

	if containsAny(text, "debug", "bug", "failing", "error", "broken") {
		roles = append(roles, "debugger")
	}
	if containsAny(text, "implement", "build", "write", "refactor", "code", "create") {
		roles = append(roles, "coder")
	}
	if containsAny(text, "test", "verify", "validate", "coverage") {
		roles = append(roles, "tester")
	}

	if !containsRole(roles, "coder") && !containsRole(roles, "debugger") {
		roles = append(roles, "coder")
	}
	if !containsRole(roles, "tester") {
		roles = append(roles, "tester")
	}
	roles = append(roles, "reviewer")
	return dedupe(roles)
}

func stepPrompt(role, goal string) string {
	switch role {
	case "researcher":
		return "Research the request, identify constraints, dependencies, and the most relevant facts for this goal: " + goal
	case "architect":
		return "Design the implementation approach for this goal. Focus on system structure, interfaces, and tradeoffs: " + goal
	case "coder":
		return "Produce the implementation-oriented response for this goal. Focus on concrete changes and execution details: " + goal
	case "tester":
		return "Validate the proposed implementation for this goal. Focus on tests, failure modes, and regressions: " + goal
	case "reviewer":
		return "Review the combined work for this goal. Focus on correctness, risk, and what remains unresolved: " + goal
	case "debugger":
		return "Diagnose the failure modes and likely root causes for this goal: " + goal
	case "documenter":
		return "Write a concise technical handoff and operator-facing summary for this goal: " + goal
	default:
		return "Contribute as " + role + " for this goal: " + goal
	}
}

func refinementPrompt(role, goal, feedback, previousOutput string) string {
	base := "Refine the previous workflow for this goal.\nGoal: " + goal + "\nFeedback: " + feedback
	if strings.TrimSpace(previousOutput) != "" {
		base += "\nPrevious output:\n" + previousOutput
	}
	switch role {
	case "reviewer":
		return "Review the previous workflow result, isolate what must change, and define clear acceptance criteria.\n\n" + base
	case "debugger":
		return "Diagnose the weaknesses or failure modes in the previous workflow result.\n\n" + base
	case "coder":
		return "Produce the refined implementation-oriented result for the workflow.\n\n" + base
	case "tester":
		return "Validate the refined workflow result for regressions and missing cases.\n\n" + base
	default:
		return "Refine the workflow result as " + role + ".\n\n" + base
	}
}

func stepID(index int) string {
	return "step-" + strconv.Itoa(index+1)
}

func containsAny(text string, terms ...string) bool {
	for _, term := range terms {
		if strings.Contains(text, term) {
			return true
		}
	}
	return false
}

func containsRole(roles []string, target string) bool {
	for _, role := range roles {
		if role == target {
			return true
		}
	}
	return false
}

func dedupe(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
