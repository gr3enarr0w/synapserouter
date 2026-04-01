package agent

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// IntentEntry represents the detected pipeline entry point and mode.
type IntentEntry struct {
	Phase       int    // starting phase index in the pipeline
	Mode        string // "full", "implement", "review", "single" — affects system prompt
	Reason      string // human-readable explanation of why this entry was chosen
	SinglePhase string // if Mode == "single", which phase to run (e.g., "code-review")
}

// DetectPipelineEntry inspects project state to determine where the pipeline
// should start. This is NOT keyword matching on the user message — the LLM
// handles intent from the message. This function examines the working directory
// to determine what phase of work is appropriate given the project's current state.
//
// The heuristics are:
//   - Spec file present (*.spec.md, SPEC.md, spec/) → full pipeline from plan (phase 0)
//   - No existing code files → full pipeline from plan (phase 0)
//   - Existing code + passing tests → start at review (skip plan+implement)
//   - Existing code + no tests or failing → start at implement (skip plan)
//   - hasExistingCode override: when true, skips plan regardless of dir contents
//
// Returns the phase index to start at and a mode string for system prompt tuning.
func DetectPipelineEntry(userMessage string, workDir string, pipeline *Pipeline, hasExistingCode bool) IntentEntry {
	if pipeline == nil || len(pipeline.Phases) == 0 {
		return IntentEntry{Phase: 0, Mode: "full", Reason: "no pipeline defined"}
	}

	// Check for review-only signals in project state.
	// If the working directory has code AND passing tests, the user likely wants
	// review, not implementation. The LLM will interpret the message; we just
	// set the entry point based on what EXISTS.
	if workDir == "" {
		return IntentEntry{Phase: 0, Mode: "full", Reason: "no working directory"}
	}

	hasSpec := detectSpecFile(workDir)
	hasCode := hasExistingCode || detectCodeFiles(workDir)
	hasTests := detectTestFiles(workDir)

	// Spec file present → full pipeline (user brought a spec to implement)
	if hasSpec && !hasCode {
		return IntentEntry{
			Phase:  0,
			Mode:   "full",
			Reason: "spec file detected, no existing code — full pipeline",
		}
	}

	// No code at all → full pipeline from plan
	if !hasCode {
		return IntentEntry{
			Phase:  0,
			Mode:   "full",
			Reason: "no existing code files — full pipeline from plan",
		}
	}

	// Has code + has tests → start at review (skip plan and implement)
	if hasTests {
		reviewIdx := findPhaseByName(pipeline, "self-check")
		if reviewIdx < 0 {
			reviewIdx = findPhaseByName(pipeline, "review")
		}
		if reviewIdx < 0 {
			reviewIdx = findPhaseByName(pipeline, "code-review")
		}
		if reviewIdx >= 0 {
			return IntentEntry{
				Phase:  reviewIdx,
				Mode:   "review",
				Reason: "existing code with tests detected — starting at review",
			}
		}
	}

	// Has code but no tests → start at implement (skip plan)
	implementIdx := findPhaseByName(pipeline, "implement")
	if implementIdx < 0 {
		implementIdx = findPhaseByName(pipeline, "data-prep")
	}
	if implementIdx >= 0 {
		return IntentEntry{
			Phase:  implementIdx,
			Mode:   "implement",
			Reason: "existing code detected, no tests — starting at implement",
		}
	}

	return IntentEntry{Phase: 0, Mode: "full", Reason: "fallback — full pipeline"}
}

// IntentSystemPromptAdjustment returns additional system prompt text based on
// the detected intent mode. This steers the LLM's behavior without changing
// the core system prompt.
func IntentSystemPromptAdjustment(entry IntentEntry) string {
	switch entry.Mode {
	case "review":
		return `MODE: REVIEW
You are reviewing existing code. Do NOT rewrite or reimplement from scratch.
Focus on: reading the code, running tests, identifying issues, suggesting fixes.
Use file_read and grep to understand the codebase before making changes.
Only use file_edit for targeted fixes — never file_write to replace existing files.`

	case "implement":
		return `MODE: IMPLEMENT
Existing code is present. Extend or modify it — do NOT start from scratch.
Read existing files first with file_read to understand the current structure.
Use file_edit for modifications. Only use file_write for genuinely new files.`

	case "single":
		return "MODE: SINGLE PHASE\nComplete the requested operation and report results. " +
			"Do not run additional pipeline phases."

	default:
		return "" // "full" mode gets no adjustment — default behavior
	}
}

// findPhaseByName returns the index of a phase by name, or -1 if not found.
func findPhaseByName(pipeline *Pipeline, name string) int {
	if pipeline == nil {
		return -1
	}
	for i, p := range pipeline.Phases {
		if p.Name == name {
			return i
		}
	}
	return -1
}

// detectSpecFile checks if the working directory contains a spec file.
func detectSpecFile(workDir string) bool {
	specPatterns := []string{
		"*.spec.md", "SPEC.md", "spec.md",
		"*.spec.yaml", "*.spec.yml",
	}
	for _, pattern := range specPatterns {
		matches, err := filepath.Glob(filepath.Join(workDir, pattern))
		if err == nil && len(matches) > 0 {
			return true
		}
	}
	// Check for spec/ directory
	info, err := os.Stat(filepath.Join(workDir, "spec"))
	if err == nil && info.IsDir() {
		return true
	}
	return false
}

// detectCodeFiles checks if the working directory has source code files.
// Looks for common source file extensions at the top level and one level deep.
func detectCodeFiles(workDir string) bool {
	codeExtensions := []string{
		"*.go", "*.py", "*.js", "*.ts", "*.java", "*.rs", "*.rb",
		"*.cpp", "*.c", "*.cs", "*.swift", "*.kt",
	}
	for _, ext := range codeExtensions {
		// Check top level
		matches, err := filepath.Glob(filepath.Join(workDir, ext))
		if err == nil && len(matches) > 0 {
			return true
		}
		// Check one level deep (src/*, cmd/*, lib/*, etc.)
		matches, err = filepath.Glob(filepath.Join(workDir, "*", ext))
		if err == nil && len(matches) > 0 {
			return true
		}
	}
	return false
}

// detectTestFiles checks if the working directory has test files.
func detectTestFiles(workDir string) bool {
	testPatterns := []string{
		"*_test.go", "*_test.py", "*.test.js", "*.test.ts",
		"*.spec.js", "*.spec.ts", "*Test.java", "*_test.rs",
		"test_*.py",
	}
	for _, pattern := range testPatterns {
		matches, err := filepath.Glob(filepath.Join(workDir, pattern))
		if err == nil && len(matches) > 0 {
			return true
		}
		matches, err = filepath.Glob(filepath.Join(workDir, "*", pattern))
		if err == nil && len(matches) > 0 {
			return true
		}
	}
	return false
}

// ApplyIntentEntry configures the agent's pipeline state based on the detected
// intent. Called from Run() after pipeline initialization.
func (a *Agent) ApplyIntentEntry(entry IntentEntry) {
	if entry.Phase > 0 && entry.Phase < len(a.pipeline.Phases) {
		a.pipelinePhase = entry.Phase
		log.Printf("[Agent] intent-based entry: skipping to phase %d (%s) — %s",
			entry.Phase, a.pipeline.Phases[entry.Phase].Name, entry.Reason)
	}

	// For single-phase mode, replace the pipeline with just the target phase
	if entry.Mode == "single" && entry.SinglePhase != "" {
		idx := findPhaseByName(a.pipeline, entry.SinglePhase)
		if idx >= 0 {
			a.pipeline = SinglePhase(a.pipeline.Phases[idx])
			a.pipelinePhase = 0
			log.Printf("[Agent] single-phase mode: running only %s", entry.SinglePhase)
		}
	}

	// Adjust the system prompt to reflect the mode
	adjustment := IntentSystemPromptAdjustment(entry)
	if adjustment != "" {
		a.cachedSystemPrompt = "" // force rebuild on next buildMessages()
		a.cachedPromptLevel = -1
		// Store the adjustment so buildMessages can append it
		a.intentPromptAdjustment = adjustment
	}
}

// GitContext holds detected git state for the working directory.
type GitContext struct {
	IsRepo         bool     // working directory is a git repo
	Branch         string   // current branch name
	HasUncommitted bool     // uncommitted changes exist
	RecentCommits  []string // last 3 commit messages
	DiffSummary    string   // summary of changed files
}

// DetectGitContext inspects the working directory's git state.
// Returns nil if not a git repo. Does not fail — returns best-effort data.
func DetectGitContext(workDir string) *GitContext {
	// Check if git repo
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	cmd.Dir = workDir
	if out, err := cmd.Output(); err != nil || strings.TrimSpace(string(out)) != "true" {
		return nil
	}

	ctx := &GitContext{IsRepo: true}

	// Get branch
	cmd = exec.Command("git", "branch", "--show-current")
	cmd.Dir = workDir
	if out, err := cmd.Output(); err == nil {
		ctx.Branch = strings.TrimSpace(string(out))
	}

	// Check for uncommitted changes
	cmd = exec.Command("git", "status", "--porcelain")
	cmd.Dir = workDir
	if out, err := cmd.Output(); err == nil {
		ctx.HasUncommitted = len(strings.TrimSpace(string(out))) > 0
	}

	// Get recent commits
	cmd = exec.Command("git", "log", "--oneline", "-3", "--no-decorate")
	cmd.Dir = workDir
	if out, err := cmd.Output(); err == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if line != "" {
				ctx.RecentCommits = append(ctx.RecentCommits, line)
			}
		}
	}

	// Diff summary (staged + unstaged)
	cmd = exec.Command("git", "diff", "--stat", "HEAD")
	cmd.Dir = workDir
	if out, err := cmd.Output(); err == nil {
		summary := strings.TrimSpace(string(out))
		if len(summary) > 500 {
			summary = summary[:500] + "..."
		}
		ctx.DiffSummary = summary
	}

	return ctx
}

// FormatGitContext returns a formatted string for system prompt injection.
func FormatGitContext(gc *GitContext) string {
	if gc == nil || !gc.IsRepo {
		return ""
	}

	var b strings.Builder
	b.WriteString("\n## Git Context\n")
	if gc.Branch != "" {
		b.WriteString("Branch: " + gc.Branch + "\n")
	}
	if gc.HasUncommitted {
		b.WriteString("Status: uncommitted changes present\n")
	} else {
		b.WriteString("Status: clean working tree\n")
	}
	if len(gc.RecentCommits) > 0 {
		b.WriteString("Recent commits:\n")
		for _, c := range gc.RecentCommits {
			b.WriteString("  " + c + "\n")
		}
	}
	if gc.DiffSummary != "" {
		b.WriteString("Changes:\n" + gc.DiffSummary + "\n")
	}
	return b.String()
}

// phasePromptForEntry returns the appropriate phase prompt when entering
// mid-pipeline. Wraps Pipeline.PhasePrompt with context about the skip.
func phasePromptForEntry(pipeline *Pipeline, entry IntentEntry, criteria string) string {
	if entry.Phase >= len(pipeline.Phases) {
		return ""
	}
	phase := pipeline.Phases[entry.Phase]
	prompt := pipeline.PhasePrompt(entry.Phase, criteria, "")

	if entry.Phase > 0 {
		var skippedNames []string
		for i := 0; i < entry.Phase; i++ {
			skippedNames = append(skippedNames, pipeline.Phases[i].Name)
		}
		prompt = "Skipped phases: " + strings.Join(skippedNames, ", ") +
			" (project state indicates they are not needed).\n\n" + prompt
	}

	_ = phase // used above
	return prompt
}
