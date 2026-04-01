package agent

import (
	"strings"
)

// PipelinePhase represents a stage in the project lifecycle.
type PipelinePhase struct {
	Name         string    // phase identifier
	Prompt       string    // injected prompt for this phase (use %s for acceptance criteria)
	Escalate     bool      // whether to escalate to next provider for this phase
	StoreAs      string    // if non-empty, store the LLM's response in this field ("criteria", "subtasks")
	FailAction   string    // "retry" = stay in current phase, "back:N" = go back N phases
	MinToolCalls int       // minimum tool calls required before phase can advance (0 = no requirement)
	UseSubAgent  bool      // spawn a fresh sub-agent for this phase (no shared conversation context)
	Tier         ModelTier // preferred model tier for this phase (cheap, mid, frontier)

	// Parallel execution: spawn N sub-agents working concurrently on split tasks.
	// CoderProviders lists which providers to target (one per sub-agent).
	// For implement phase: populated at runtime from EscalationChain[0].
	// For plan phase: hardcoded to planner providers (planning is unique).
	ParallelSubAgents int      // number of parallel sub-agents (0 = sequential)
	CoderProviders    []string // provider names for each parallel sub-agent

	// MergeProvider: after parallel sub-agents complete, a third model merges
	// their outputs. Used for plan phase only (planners are unique).
	MergeProvider string
}

// Pipeline defines the ordered phases for a project type.
type Pipeline struct {
	Name   string
	Phases []PipelinePhase
}

// DefaultPipeline is the standard software development lifecycle.
var DefaultPipeline = Pipeline{
	Name: "software",
	Phases: []PipelinePhase{
		{
			Name:              "plan",
			Tier:              TierFrontier,
			StoreAs:           "criteria",
			MinToolCalls:      1,
			ParallelSubAgents: 2,
			CoderProviders:    []string{"ollama-planner-1", "ollama-planner-2"},
			MergeProvider:     "auto", // resolved at runtime from escalation chain top
			Prompt: `PHASE: PLAN
Your plan MUST begin with a SPEC CONSTRAINTS section that explicitly lists:
- Required package/directory structure
- OUT OF SCOPE items (things you must NOT build)
- Prohibited patterns (e.g., "no service layer")
If you skip this section, the plan will be rejected.

0. SPEC PERCEPTION (do this FIRST):
   Before planning, restate the spec's key architectural decisions:
   - Required package/directory structure?
   - IN SCOPE and OUT OF SCOPE?
   - Mandated/prohibited design patterns?
   - Technology constraints?
   If the spec has an "Acceptance Criteria" section, EXTRACT those criteria verbatim — do not generate your own.
   If no spec is provided, state "No spec provided" in the SPEC CONSTRAINTS section and proceed.

Before implementing, create:
1. TASK DECOMPOSITION: ordered subtasks with dependencies
2. ACCEPTANCE CRITERIA for each subtask AND the overall deliverable:
   - What fields/values must be non-null/non-zero?
   - What does the end-user experience look like when correct?
   - What does INCORRECT look like? (negative criteria — nulls, zeros, missing structure)
   - Edge cases that could fail silently
   - PRODUCTION QUALITY: no stubs, no flat structures, no missing edge cases, no empty fields
3. UNKNOWNS: list anything in the requirements you're uncertain about.
   For each: what research is needed? What will you do if research doesn't resolve it?
4. ASSUMPTIONS: list any decisions where multiple valid approaches exist.
   Document which you'll choose and why.
5. DEFINITION OF DONE: what "complete" means for this task — production quality, not just "it compiles"

Output your plan, criteria, unknowns, and assumptions, then say PLAN_COMPLETE.`,
		},
		{
			Name:         "implement",
			Tier:         TierCheap,
			MinToolCalls: 1,
			// ParallelSubAgents and CoderProviders populated at runtime
			// from EscalationChain[0] via initializeImplementPhase()
			Prompt: `PHASE: IMPLEMENT
Execute the plan. Build all subtasks in order.
Track progress — note which subtasks are done/pending/blocked.
If blocked on an API or tool: RESEARCH first (read docs, test with curl), don't guess.
Show math for all calculated values.
When all subtasks are complete and code compiles/runs, say IMPLEMENT_COMPLETE.`,
		},
		{
			Name:         "self-check",
			Tier:         TierMid,
			MinToolCalls: 2,
			Prompt: `PHASE: SELF-CHECK
Check your work against YOUR OWN acceptance criteria:
---
%CRITERIA%
---

ORIGINAL SPEC/REQUEST:
---
%SPEC%
---

For each criterion: PASS or FAIL with evidence.
- Fetch real data (API responses, file contents, test output)
- Check for nulls, zeros, empty fields, placeholder text
- Show math for numeric values
- Would a SENIOR ENGINEER ship each output as-is? Or would they add more structure,
  handling, or detail before delivering?
- Are there any assumptions you made silently? Document them.
- Are there any values you defaulted to minimum viable? Upgrade them.
- Did you encounter any unknowns you guessed at instead of researching? Fix those now.
- CROSS-COMPARISON: Compare ALL outputs you produced of the same type.
  Do they have consistent structure? If one has significantly MORE structure
  than another (more sections, more steps, more fields), the simpler one is
  likely incomplete. Flag it and improve it to match the more detailed one.
- SPEC COMPLIANCE: verify your implementation matches the original spec's scope,
  directory structure, and constraints. Flag any deviations.
Fix anything that fails. When all criteria pass, say SELF_CHECK_PASS.
If anything fails and you can't fix it, say SELF_CHECK_FAIL.`,
		},
		{
			Name:         "code-review",
			Tier:         TierFrontier,
			Escalate:     true, // force escalation — reviewer must be a DIFFERENT (bigger) model than implementer
			MinToolCalls: 2,
			UseSubAgent:  true,
			Prompt: `PHASE: CODE REVIEW (independent reviewer)
You are a DIFFERENT model reviewing work done by a previous model.
Check against these acceptance criteria:
---
%CRITERIA%
---

ORIGINAL SPEC/REQUEST:
---
%SPEC%
---

1. Fetch ALL real results — do NOT trust the previous model's claims
2. For each criterion: PASS or FAIL with evidence
3. Check for things the implementer missed:
   - Null/empty/zero values where real data is expected
   - End-user experience — would a human say this is right?
   - Edge cases, missing structure, completeness gaps
4. SPEC COMPLIANCE: verify implementation matches the original spec's scope,
   directory structure, and constraints. Flag any out-of-scope additions.
5. Say CODE_REVIEW_PASS if all criteria met, or CODE_REVIEW_FAIL with specifics.`,
		},
		{
			Name:         "acceptance-test",
			Tier:         TierFrontier,
			Escalate:     true, // force escalation — acceptance tester must be bigger model
			MinToolCalls: 1,
			UseSubAgent:  true,
			Prompt: `PHASE: ACCEPTANCE TEST
Run the actual deliverable end-to-end from the USER'S perspective:

ORIGINAL SPEC/REQUEST:
---
%SPEC%
---

1. Execute/call/open the output as a user would
2. Check every aspect of the end-user experience
3. Verify against acceptance criteria:
---
%CRITERIA%
---
4. Verify the implementation matches the spec's architecture, package structure, and scope
5. Check for OUT OF SCOPE violations (features added that spec excludes)
6. Is anything null, broken, missing, or incomplete that a user would notice?
7. Say ACCEPTANCE_PASS if the user would be satisfied, or ACCEPTANCE_FAIL with what's wrong.`,
		},
		{
			Name: "deploy",
			Tier: TierCheap,
			Prompt: `PHASE: DEPLOY/DELIVER
Final cleanup and delivery:
1. Remove any temp files, test artifacts, stale resources
2. Confirm final state matches the original request
3. Summarize what was delivered
Say DEPLOY_COMPLETE.`,
		},
	},
}

// DataSciencePipeline is the lifecycle for data/ML projects.
var DataSciencePipeline = Pipeline{
	Name: "data-science",
	Phases: []PipelinePhase{
		{
			Name:         "eda",
			Tier:         TierFrontier,
			StoreAs:      "criteria",
			MinToolCalls: 2,
			Prompt: `PHASE: EXPLORATORY DATA ANALYSIS
1. Load and inspect the data — shape, types, nulls, distributions
2. Identify what's available and what's missing
3. Can a human interpret what needs to happen from this data?
4. Define ACCEPTANCE CRITERIA: what does a good model output look like?
5. Note any data quality issues that need fixing
Say EDA_COMPLETE with your findings and criteria.`,
		},
		{
			Name:         "data-prep",
			Tier:         TierCheap,
			MinToolCalls: 1,
			Prompt: `PHASE: DATA PREPARATION
1. Clean data issues found in EDA
2. Feature engineering
3. Train/test split
4. Verify data is ready for modeling
Say DATA_PREP_COMPLETE.`,
		},
		{
			Name:         "model",
			Tier:         TierCheap,
			MinToolCalls: 1,
			Prompt: `PHASE: MODELING
1. Build models per the plan
2. This is CODE — goes through the standard code pipeline
3. Train, evaluate, compare
4. Select best model with justification
Say MODEL_COMPLETE with results.`,
		},
		{
			Name:         "review",
			Tier:         TierFrontier,
			Escalate:     true,
			MinToolCalls: 2,
			UseSubAgent:  true,
			Prompt: `PHASE: MODEL REVIEW (independent reviewer)
Check against acceptance criteria:
---
%s
---
1. Can a human interpret the results?
2. Are the metrics reasonable? Any signs of overfitting/leakage?
3. Does the output match what was asked for?
Say MODEL_REVIEW_PASS or MODEL_REVIEW_FAIL.`,
		},
		{
			Name: "deploy",
			Tier: TierCheap,
			Prompt: `PHASE: DEPLOY
1. Deploy model/results
2. Verify deployment works end-to-end
3. Clean up artifacts
Say DEPLOY_COMPLETE.`,
		},
		{
			Name: "verify",
			Tier: TierMid,
			Prompt: `PHASE: VERIFY RESULTS
1. Run deployed model/output
2. Can a human still interpret the results?
3. Do results match acceptance criteria?
Say VERIFY_PASS or VERIFY_FAIL.`,
		},
	},
}

// phasePassSignals are the keywords that indicate a phase completed successfully.
var phasePassSignals = []string{
	"plan_complete", "implement_complete",
	"self_check_pass", "code_review_pass", "acceptance_pass", "deploy_complete",
	"eda_complete", "data_prep_complete", "model_complete",
	"model_review_pass", "verify_pass",
	"verified_correct", // sub-agent review pass signal
}

// phaseFailSignals indicate a phase found issues.
var phaseFailSignals = []string{
	"self_check_fail", "code_review_fail", "acceptance_fail",
	"model_review_fail", "verify_fail", "needs_fix",
}

// SinglePhase creates a pipeline with just one phase. Used for targeted operations
// like "review my code" where running the full pipeline would be wrong.
func SinglePhase(phase PipelinePhase) *Pipeline {
	return &Pipeline{
		Name:   "single-" + phase.Name,
		Phases: []PipelinePhase{phase},
	}
}

// PhasePrompt returns the prompt for a phase, with acceptance criteria and
// the original spec/request injected into review phases.
func (p *Pipeline) PhasePrompt(phaseIdx int, criteria, spec string) string {
	if phaseIdx < 0 || phaseIdx >= len(p.Phases) {
		return ""
	}
	phase := p.Phases[phaseIdx]

	prompt := phase.Prompt
	if criteria != "" {
		prompt = strings.ReplaceAll(prompt, "%CRITERIA%", criteria)
	} else {
		prompt = strings.ReplaceAll(prompt, "%CRITERIA%", "(criteria will be defined during earlier phases)")
	}

	if spec != "" {
		prompt = strings.ReplaceAll(prompt, "%SPEC%", spec)
	} else {
		prompt = strings.ReplaceAll(prompt, "%SPEC%", "(no spec provided)")
	}

	// Legacy %s placeholder support (plan phase uses %s for criteria)
	if strings.Contains(prompt, "%s") {
		if criteria != "" {
			prompt = strings.ReplaceAll(prompt, "%s", criteria)
		} else {
			prompt = strings.ReplaceAll(prompt, "%s", "(criteria will be defined during earlier phases)")
		}
	}

	return prompt
}

// IsPassSignal checks if the LLM response indicates phase completion.
func IsPassSignal(content string) bool {
	lower := strings.ToLower(content)
	for _, signal := range phasePassSignals {
		if strings.Contains(lower, signal) {
			return true
		}
	}
	return false
}

// IsFailSignal checks if the LLM response indicates phase failure.
func IsFailSignal(content string) bool {
	lower := strings.ToLower(content)
	for _, signal := range phaseFailSignals {
		if strings.Contains(lower, signal) {
			return true
		}
	}
	return false
}
