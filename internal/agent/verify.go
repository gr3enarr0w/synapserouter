package agent

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/environment"
	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/orchestration"
)

// VerifyResult holds the result of a single programmatic verification check.
type VerifyResult struct {
	Name     string
	Passed   bool
	Output   string
	ExitCode int
}

// RunVerificationGate runs programmatic checks for the current phase.
// The LLM's claim of "PASS" is necessary but not sufficient — the gate
// requires actual exit codes from build/test/verify commands.
// Returns true if all checks pass, false with details if any fail.
func (a *Agent) RunVerificationGate(phaseName string) (bool, []VerifyResult) {
	var results []VerifyResult

	env := environment.Detect(a.config.WorkDir)

	// If a project language is declared (from spec), use it instead of auto-detection.
	// This prevents wrong-language detection when the agent creates config files
	// for the wrong language (e.g., go.mod in a SQL project).
	if a.config.ProjectLanguage != "" && env != nil && env.Language != a.config.ProjectLanguage {
		log.Printf("[Verify] detected %s but project declares %s — using declared language",
			env.Language, a.config.ProjectLanguage)
		env.Language = a.config.ProjectLanguage
	}
	if a.config.ProjectLanguage != "" && env == nil {
		env = &environment.ProjectEnv{
			Language: a.config.ProjectLanguage,
			EnvVars:  make(map[string]string),
		}
	}

	// Layer 1: Build check
	if shouldRunBuild(phaseName) && env != nil {
		if buildCmd := environment.BuildCommand(env); buildCmd != "" {
			r := a.runVerifyCommand("build/"+env.Language, buildCmd)
			results = append(results, r)
			log.Printf("[Verify] build: %s → exit %d", buildCmd, r.ExitCode)
		}
	}

	// Layer 2: Test check
	if shouldRunTests(phaseName) && env != nil {
		if testCmd := environment.TestCommand(env); testCmd != "" {
			r := a.runVerifyCommand("test/"+env.Language, testCmd)
			results = append(results, r)
			log.Printf("[Verify] test: %s → exit %d", testCmd, r.ExitCode)
		}
	}

	// Layer 3: Skill verify commands
	if shouldRunSkillVerify(phaseName) {
		skillResults := a.runSkillVerifyCommands()
		results = append(results, skillResults...)
	}

	if len(results) == 0 {
		return true, nil // No checks applicable for this phase/language
	}

	allPassed := true
	passed, failed := 0, 0
	for _, r := range results {
		if r.Passed {
			passed++
		} else {
			failed++
			allPassed = false
		}
	}

	log.Printf("[Verify] gate for phase %s: %d passed, %d failed (total %d)",
		phaseName, passed, failed, len(results))

	return allPassed, results
}

// runVerifyCommand executes a single shell command and returns the result.
func (a *Agent) runVerifyCommand(name, command string) VerifyResult {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := a.registry.Execute(ctx, "bash",
		map[string]interface{}{
			"command":    command,
			"timeout_ms": float64(30000),
		},
		a.config.WorkDir)

	if err != nil {
		return VerifyResult{Name: name, Passed: false, Output: err.Error(), ExitCode: -1}
	}

	exitCode := 0
	output := ""
	if result != nil {
		exitCode = result.ExitCode
		output = result.Output
		if result.Error != "" && output == "" {
			output = result.Error
		}
	}

	return VerifyResult{
		Name:     name,
		Passed:   exitCode == 0,
		Output:   output,
		ExitCode: exitCode,
	}
}

// runSkillVerifyCommands collects and executes verify commands from all matched skills.
func (a *Agent) runSkillVerifyCommands() []VerifyResult {
	matched := orchestration.MatchSkillsForLanguage(a.originalRequest, a.config.Skills, a.config.ProjectLanguage)
	var results []VerifyResult

	for _, skill := range matched {
		// Skip language-specific skills that don't match project language.
		// Only filters when both skill.Language and ProjectLanguage are known.
		if skill.Language != "" && a.config.ProjectLanguage != "" &&
			skill.Language != a.config.ProjectLanguage {
			continue
		}
		for _, vc := range skill.Verify {
			if vc.Command == "" || vc.Manual != "" {
				continue // Skip manual-only checks
			}

			r := a.runVerifyCommand(skill.Name+"/"+vc.Name, vc.Command)

			// Check expect/expect_not against output
			if vc.Expect != "" && !strings.Contains(r.Output, vc.Expect) {
				r.Passed = false
			}
			if vc.ExpectNot != "" && strings.Contains(r.Output, vc.ExpectNot) {
				r.Passed = false
			}

			results = append(results, r)
		}
	}

	if len(results) > 0 {
		passed := 0
		for _, r := range results {
			if r.Passed {
				passed++
			}
		}
		log.Printf("[Verify] skill checks: %d/%d passed", passed, len(results))
	}

	return results
}

// FormatVerifyFailures formats failed verification results as a message for the agent.
func FormatVerifyFailures(results []VerifyResult) string {
	var b strings.Builder
	b.WriteString("VERIFICATION GATE FAILED — the following programmatic checks did not pass:\n\n")

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

	b.WriteString("\nFix these issues and try again. The phase cannot advance until all verification checks pass.")
	return b.String()
}

// countVerifyPassed counts the number of passed verification results.
func countVerifyPassed(results []VerifyResult) int {
	n := 0
	for _, r := range results {
		if r.Passed {
			n++
		}
	}
	return n
}

// countVerifyFailed counts the number of failed verification results.
func countVerifyFailed(results []VerifyResult) int {
	n := 0
	for _, r := range results {
		if !r.Passed {
			n++
		}
	}
	return n
}

// shouldRunBuild returns true if this phase should run the build check.
func shouldRunBuild(phase string) bool {
	switch phase {
	case "implement", "self-check", "code-review", "acceptance-test", "deploy", "model", "review":
		return true
	}
	return false
}

// shouldRunTests returns true if this phase should run the test check.
func shouldRunTests(phase string) bool {
	switch phase {
	case "self-check", "code-review", "acceptance-test", "model", "review":
		return true
	}
	return false
}

// shouldRunSkillVerify returns true if this phase should run skill verify commands.
func shouldRunSkillVerify(phase string) bool {
	switch phase {
	case "code-review", "acceptance-test", "review":
		return true
	}
	return false
}
