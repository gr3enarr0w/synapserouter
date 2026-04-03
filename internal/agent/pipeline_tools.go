package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/gr3enarr0w/synapserouter/internal/tools"
)

// PipelineToolAgent is the interface pipeline tools need from the agent.
type PipelineToolAgent interface {
	AgentSpawner
	RunVerificationGate(phaseName string) (bool, []VerifyResult)
	GetAcceptanceCriteria() string
	SetAcceptanceCriteria(criteria string)
	GetOriginalRequest() string
	GetConfig() Config
	Emit(eventType EventType, content string, metadata map[string]any)
}

// --- pipeline_plan ---

type PipelinePlanTool struct {
	agent PipelineToolAgent
}

func NewPipelinePlanTool(agent PipelineToolAgent) *PipelinePlanTool {
	return &PipelinePlanTool{agent: agent}
}

func (t *PipelinePlanTool) Name() string        { return "pipeline_plan" }
func (t *PipelinePlanTool) Category() tools.ToolCategory { return tools.CategoryWrite }
func (t *PipelinePlanTool) Description() string {
	return "Create a structured plan with acceptance criteria for a complex task. Use for multi-file changes, specs, or coordinated work. Do NOT use for simple questions or single-file edits."
}

func (t *PipelinePlanTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"task": map[string]interface{}{
				"type":        "string",
				"description": "The task to plan, including any spec content or requirements",
			},
			"scope": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"small", "medium", "large"},
				"description": "small: 1-3 files, medium: 4-10 files, large: 10+ files",
			},
		},
		"required": []string{"task"},
	}
}

func (t *PipelinePlanTool) Execute(ctx context.Context, args map[string]interface{}, workDir string) (*tools.ToolResult, error) {
	task, _ := args["task"].(string)
	if task == "" {
		return &tools.ToolResult{Error: "task is required"}, nil
	}

	t.agent.Emit(EventPhaseStart, "", map[string]any{"phase_name": "plan", "triggered_by": "pipeline_plan_tool"})
	defer t.agent.Emit(EventPhaseComplete, "", map[string]any{"phase_name": "plan"})

	t.agent.SetAcceptanceCriteria(task)
	log.Printf("[PipelineTool] plan: stored acceptance criteria (%d bytes)", len(task))

	return &tools.ToolResult{
		Output: "Plan registered. Acceptance criteria stored. Use pipeline_verify after implementation to check build/test. Use pipeline_review for independent code review.",
	}, nil
}

// --- pipeline_implement ---

type PipelineImplementTool struct {
	agent PipelineToolAgent
}

func NewPipelineImplementTool(agent PipelineToolAgent) *PipelineImplementTool {
	return &PipelineImplementTool{agent: agent}
}

func (t *PipelineImplementTool) Name() string        { return "pipeline_implement" }
func (t *PipelineImplementTool) Category() tools.ToolCategory { return tools.CategoryWrite }
func (t *PipelineImplementTool) Description() string {
	return "Delegate implementation to a sub-agent. Use after pipeline_plan for complex tasks that benefit from dedicated focus."
}

func (t *PipelineImplementTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"plan": map[string]interface{}{
				"type":        "string",
				"description": "The implementation plan to execute",
			},
		},
		"required": []string{"plan"},
	}
}

func (t *PipelineImplementTool) Execute(ctx context.Context, args map[string]interface{}, workDir string) (*tools.ToolResult, error) {
	plan, _ := args["plan"].(string)
	if plan == "" {
		return &tools.ToolResult{Error: "plan is required"}, nil
	}

	t.agent.Emit(EventPhaseStart, "", map[string]any{"phase_name": "implement", "triggered_by": "pipeline_implement_tool"})

	task := fmt.Sprintf("Implement the following plan:\n\n%s\n\nWhen done, verify the code compiles and tests pass.", plan)
	result, err := t.agent.RunChild(ctx, SpawnConfig{
		Role:   "implementer",
		Budget: &AgentBudget{MaxTurns: 25},
	}, task)

	t.agent.Emit(EventPhaseComplete, "", map[string]any{"phase_name": "implement"})

	if err != nil {
		return &tools.ToolResult{Error: fmt.Sprintf("implementation sub-agent failed: %v", err)}, nil
	}

	return &tools.ToolResult{Output: result}, nil
}

// --- pipeline_verify ---

type PipelineVerifyTool struct {
	agent PipelineToolAgent
}

func NewPipelineVerifyTool(agent PipelineToolAgent) *PipelineVerifyTool {
	return &PipelineVerifyTool{agent: agent}
}

func (t *PipelineVerifyTool) Name() string        { return "pipeline_verify" }
func (t *PipelineVerifyTool) Category() tools.ToolCategory { return tools.CategoryReadOnly }
func (t *PipelineVerifyTool) Description() string {
	return "Run programmatic verification: build, test, lint, and spec compliance checks. Returns pass/fail for each check."
}

func (t *PipelineVerifyTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"checks": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "Which checks to run: build, test, lint, spec. Default: all applicable.",
			},
		},
	}
}

func (t *PipelineVerifyTool) Execute(ctx context.Context, args map[string]interface{}, workDir string) (*tools.ToolResult, error) {
	t.agent.Emit(EventPhaseStart, "", map[string]any{"phase_name": "verify", "triggered_by": "pipeline_verify_tool"})

	passed, results := t.agent.RunVerificationGate("self-check")

	t.agent.Emit(EventPhaseComplete, "", map[string]any{"phase_name": "verify", "passed": passed})

	var b strings.Builder
	if passed {
		b.WriteString("ALL CHECKS PASSED\n\n")
	} else {
		b.WriteString("SOME CHECKS FAILED\n\n")
	}

	for _, r := range results {
		status := "PASS"
		if !r.Passed {
			status = "FAIL"
		}
		b.WriteString(fmt.Sprintf("- %s: %s (exit %d)\n", r.Name, status, r.ExitCode))
		if !r.Passed && r.Output != "" {
			output := r.Output
			if len(output) > 500 {
				output = output[:500] + "\n...(truncated)"
			}
			b.WriteString(fmt.Sprintf("  Output: %s\n", output))
		}
	}

	return &tools.ToolResult{Output: b.String()}, nil
}

// --- pipeline_review ---

type PipelineReviewTool struct {
	agent PipelineToolAgent
}

func NewPipelineReviewTool(agent PipelineToolAgent) *PipelineReviewTool {
	return &PipelineReviewTool{agent: agent}
}

func (t *PipelineReviewTool) Name() string        { return "pipeline_review" }
func (t *PipelineReviewTool) Category() tools.ToolCategory { return tools.CategoryWrite }
func (t *PipelineReviewTool) Description() string {
	return "Request independent code review from a fresh sub-agent using a larger model. Use after implementation is complete and pipeline_verify passes."
}

func (t *PipelineReviewTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"criteria": map[string]interface{}{
				"type":        "string",
				"description": "Acceptance criteria to review against",
			},
			"scope": map[string]interface{}{
				"type":        "string",
				"description": "What to review: file paths, component names, or 'all'",
			},
		},
		"required": []string{"criteria"},
	}
}

func (t *PipelineReviewTool) Execute(ctx context.Context, args map[string]interface{}, workDir string) (*tools.ToolResult, error) {
	criteria, _ := args["criteria"].(string)
	if criteria == "" {
		criteria = t.agent.GetAcceptanceCriteria()
	}

	t.agent.Emit(EventPhaseStart, "", map[string]any{"phase_name": "code-review", "triggered_by": "pipeline_review_tool"})

	task := fmt.Sprintf(`CODE REVIEW (independent reviewer)
You are reviewing work done by a previous model. Check against these criteria:
---
%s
---

Original request:
---
%s
---

1. Read ALL relevant files — do NOT trust claims from previous context
2. For each criterion: PASS or FAIL with evidence
3. Check for: null/empty values, missing edge cases, spec violations
4. Say CODE_REVIEW_PASS if all criteria met, or CODE_REVIEW_FAIL with specifics.`, criteria, t.agent.GetOriginalRequest())

	result, err := t.agent.RunChild(ctx, SpawnConfig{
		Role:   "code-reviewer",
		Tier:   TierFrontier,
		Budget: &AgentBudget{MaxTurns: 15},
	}, task)

	passed := err == nil && strings.Contains(strings.ToLower(result), "code_review_pass")
	t.agent.Emit(EventPhaseComplete, "", map[string]any{"phase_name": "code-review", "passed": passed})

	if err != nil {
		return &tools.ToolResult{Error: fmt.Sprintf("review sub-agent failed: %v", err)}, nil
	}

	return &tools.ToolResult{Output: result}, nil
}

// --- pipeline_test ---

type PipelineTestTool struct {
	agent PipelineToolAgent
}

func NewPipelineTestTool(agent PipelineToolAgent) *PipelineTestTool {
	return &PipelineTestTool{agent: agent}
}

func (t *PipelineTestTool) Name() string        { return "pipeline_test" }
func (t *PipelineTestTool) Category() tools.ToolCategory { return tools.CategoryWrite }
func (t *PipelineTestTool) Description() string {
	return "Run end-to-end acceptance testing from the user's perspective. Spawns a fresh sub-agent that tests the output as a user would."
}

func (t *PipelineTestTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"criteria": map[string]interface{}{
				"type":        "string",
				"description": "Acceptance criteria to test against",
			},
			"spec": map[string]interface{}{
				"type":        "string",
				"description": "Original spec/request for scope verification",
			},
		},
		"required": []string{"criteria"},
	}
}

func (t *PipelineTestTool) Execute(ctx context.Context, args map[string]interface{}, workDir string) (*tools.ToolResult, error) {
	criteria, _ := args["criteria"].(string)
	if criteria == "" {
		criteria = t.agent.GetAcceptanceCriteria()
	}
	spec, _ := args["spec"].(string)
	if spec == "" {
		spec = t.agent.GetOriginalRequest()
	}

	t.agent.Emit(EventPhaseStart, "", map[string]any{"phase_name": "acceptance-test", "triggered_by": "pipeline_test_tool"})

	task := fmt.Sprintf(`ACCEPTANCE TEST
Run the deliverable end-to-end from the USER's perspective:

Original request:
---
%s
---

Acceptance criteria:
---
%s
---

1. Execute/call/open the output as a user would
2. Check every aspect of the end-user experience
3. Is anything null, broken, missing, or incomplete?
4. Say ACCEPTANCE_PASS if the user would be satisfied, or ACCEPTANCE_FAIL with what's wrong.`, spec, criteria)

	result, err := t.agent.RunChild(ctx, SpawnConfig{
		Role:   "acceptance-tester",
		Tier:   TierFrontier,
		Budget: &AgentBudget{MaxTurns: 15},
	}, task)

	passed := err == nil && strings.Contains(strings.ToLower(result), "acceptance_pass")
	t.agent.Emit(EventPhaseComplete, "", map[string]any{"phase_name": "acceptance-test", "passed": passed})

	if err != nil {
		return &tools.ToolResult{Error: fmt.Sprintf("acceptance test sub-agent failed: %v", err)}, nil
	}

	return &tools.ToolResult{Output: result}, nil
}

// --- pipeline_status ---

type PipelineStatusTool struct {
	agent PipelineToolAgent
}

func NewPipelineStatusTool(agent PipelineToolAgent) *PipelineStatusTool {
	return &PipelineStatusTool{agent: agent}
}

func (t *PipelineStatusTool) Name() string        { return "pipeline_status" }
func (t *PipelineStatusTool) Category() tools.ToolCategory { return tools.CategoryReadOnly }
func (t *PipelineStatusTool) Description() string {
	return "Check current pipeline state: what has been planned, verified, and reviewed in this session."
}

func (t *PipelineStatusTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	}
}

func (t *PipelineStatusTool) Execute(ctx context.Context, args map[string]interface{}, workDir string) (*tools.ToolResult, error) {
	criteria := t.agent.GetAcceptanceCriteria()

	status := map[string]interface{}{
		"has_plan":         criteria != "",
		"criteria_length":  len(criteria),
		"original_request": truncateStr(t.agent.GetOriginalRequest(), 200),
	}
	if criteria != "" {
		status["acceptance_criteria"] = truncateStr(criteria, 500)
	}

	data, _ := json.MarshalIndent(status, "", "  ")
	return &tools.ToolResult{Output: string(data)}, nil
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// RegisterPipelineTools registers all 6 pipeline tools on the given registry.
func RegisterPipelineTools(registry *tools.Registry, agent PipelineToolAgent) {
	registry.Register(NewPipelinePlanTool(agent))
	registry.Register(NewPipelineImplementTool(agent))
	registry.Register(NewPipelineVerifyTool(agent))
	registry.Register(NewPipelineReviewTool(agent))
	registry.Register(NewPipelineTestTool(agent))
	registry.Register(NewPipelineStatusTool(agent))
}
