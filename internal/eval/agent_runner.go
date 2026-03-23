package eval

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"
)

// runExerciseAgent uses an iterative agent loop to solve an exercise.
// The full pipeline: generate → test → fix → escalate → review → cleanup → re-test.
func (r *Runner) runExerciseAgent(ctx context.Context, ex Exercise, config EvalRunConfig, timeout time.Duration) EvalResult {
	// LLM-judge exercises (text/writingbench) don't use iterative test loop
	evalMode := ex.EvalMode
	if evalMode == "" {
		evalMode = "docker-test"
	}
	if evalMode == "llm-judge" {
		result := EvalResult{ID: fmt.Sprintf("res-%d", time.Now().UnixNano()), ExerciseID: ex.ID}
		return r.runLLMJudgeExercise(ctx, ex, config, &result)
	}
	if evalMode == "vlm-judge" {
		return EvalResult{
			ID: fmt.Sprintf("res-%d", time.Now().UnixNano()), ExerciseID: ex.ID,
			Error: "vlm-judge not implemented",
		}
	}

	result := EvalResult{
		ID:         fmt.Sprintf("res-%d", time.Now().UnixNano()),
		ExerciseID: ex.ID,
	}

	maxTurns := config.MaxTurns
	if maxTurns <= 0 {
		maxTurns = 5
	}

	// Escalation threshold: after this many failed turns with the initial provider,
	// switch to routing mode (lets the router pick a better provider).
	escalateTurn := maxTurns / 2
	if escalateTurn < 2 {
		escalateTurn = 2
	}

	start := time.Now()

	// Step 1: Initial code generation
	prompt := buildAgentPrompt(ex)
	resp, provider, _, err := r.sendToProvider(ctx, prompt, config)
	if err != nil {
		result.Error = err.Error()
		result.Provider = config.Provider
		result.LatencyMs = time.Since(start).Milliseconds()
		return result
	}

	result.Provider = provider
	result.Model = resp.Model
	result.TotalTokens = resp.Usage.TotalTokens

	code := extractCode(resp)
	result.GeneratedCode = code

	// Step 2: Iterative test → fix → escalate loop
	escalated := false
	for turn := 0; turn < maxTurns; turn++ {
		testResult := runTest(ctx, ex, code, timeout)
		result.TestOutput = testResult.Output
		result.DockerExitCode = testResult.ExitCode

		// Auto-install missing Python modules before counting as a real failure
		if !testResult.Passed && isEnvironmentError(testResult.Output) {
			if fixedResult := autoFixEnvironment(ctx, ex, code, testResult.Output, timeout); fixedResult != nil {
				testResult = *fixedResult
				result.TestOutput = testResult.Output
				result.DockerExitCode = testResult.ExitCode
			}
		}

		if testResult.Passed {
			result.Pass1 = true
			if turn > 0 {
				result.Pass2 = true
			}
			log.Printf("[Eval/Agent] %s | PASS on turn %d/%d | %dms",
				ex.ID, turn+1, maxTurns, time.Since(start).Milliseconds())

			// Step 4: Code review pass — send passing code to a reviewer for cleanup
			if reviewedCode := r.reviewAndCleanup(ctx, ex, code, config); reviewedCode != "" {
				// Re-test the reviewed code to make sure it still passes
				reviewResult := runTest(ctx, ex, reviewedCode, timeout)
				if reviewResult.Passed {
					code = reviewedCode
					result.GeneratedCode2 = code
					log.Printf("[Eval/Agent] %s | review cleanup applied, still passes", ex.ID)
				} else {
					log.Printf("[Eval/Agent] %s | review cleanup broke tests, keeping original", ex.ID)
				}
			}
			break
		}

		if turn == maxTurns-1 {
			log.Printf("[Eval/Agent] %s | FAIL after %d turns | %dms",
				ex.ID, maxTurns, time.Since(start).Milliseconds())
			break
		}

		// Step 3: Build error-aware fix prompt and optionally escalate provider
		errType := classifyError(testResult.Output, ex.Language)
		fixPrompt := buildErrorAwareFixPrompt(ex, code, testResult.Output, errType, turn+1, maxTurns)

		// Escalate to routing mode: either after threshold OR when cheap model gives up
		fixConfig := config
		if !escalated && config.Provider != "" && turn >= escalateTurn {
			fixConfig.Provider = ""
			fixConfig.Mode = "routing"
			escalated = true
			log.Printf("[Eval/Agent] %s | escalating to routing mode on turn %d", ex.ID, turn+2)
		}

		fixResp, fixProvider, _, fixErr := r.sendToProvider(ctx, fixPrompt, fixConfig)
		if fixErr != nil {
			result.Error = fmt.Sprintf("fix turn %d: %v", turn+1, fixErr)
			break
		}
		if escalated && fixProvider != "" {
			result.FallbackUsed = true
		}

		result.TotalTokens += fixResp.Usage.TotalTokens
		newCode := extractCode(fixResp)
		if newCode == "" || newCode == code {
			// Cheap model gave up (returned same/empty code) — escalate to a better provider
			if !escalated {
				escalated = true
				log.Printf("[Eval/Agent] %s | cheap model stalled, escalating on turn %d", ex.ID, turn+2)

				// Try each escalation provider in order until one produces new code
				for _, escProvider := range escalationProviders(result.Provider) {
					escConfig := config
					escConfig.Provider = escProvider
					escConfig.Mode = ""
					escalateResp, actualProvider, _, escErr := r.sendToProvider(ctx, fixPrompt, escConfig)
					if escErr != nil {
						continue
					}
					result.TotalTokens += escalateResp.Usage.TotalTokens
					escalatedCode := extractCode(escalateResp)
					if escalatedCode != "" && escalatedCode != code {
						code = escalatedCode
						result.GeneratedCode2 = code
						result.FallbackUsed = true
						log.Printf("[Eval/Agent] %s | got new code from %s", ex.ID, actualProvider)
						break
					}
				}
				if code != newCode && result.FallbackUsed {
					continue // retry test with escalated code
				}
			}
			log.Printf("[Eval/Agent] %s | no new code on turn %d, stopping", ex.ID, turn+2)
			break
		}
		code = newCode
		result.GeneratedCode2 = code
	}

	result.LatencyMs = time.Since(start).Milliseconds()
	log.Printf("[Eval/Agent] %s | provider=%s | pass=%v | escalated=%v | %dms | tokens=%d",
		ex.ID, provider, result.Pass1, escalated, result.LatencyMs, result.TotalTokens)

	return result
}

// errorType classifies what kind of test failure occurred.
type errorType int

const (
	errUnknown    errorType = iota
	errCompile              // code doesn't compile/parse
	errRuntime              // runtime crash (nil pointer, index out of range, etc.)
	errAssertion            // test assertion failed (logic error)
	errTimeout              // execution timed out
	errImport               // missing import/module
)

// classifyError determines the category of test failure from output.
func classifyError(output, language string) errorType {
	upper := strings.ToUpper(output)

	// Compile errors
	switch language {
	case "go":
		if strings.Contains(output, "cannot use") || strings.Contains(output, "undefined:") ||
			strings.Contains(output, "syntax error") || strings.Contains(output, "does not compile") {
			return errCompile
		}
	case "java":
		if strings.Contains(output, "error:") && strings.Contains(output, ".java:") {
			return errCompile
		}
	case "cpp":
		if strings.Contains(output, "error:") && (strings.Contains(output, ".cpp:") || strings.Contains(output, ".h:")) {
			return errCompile
		}
	case "rust":
		if strings.Contains(output, "error[E") {
			return errCompile
		}
	case "python":
		if strings.Contains(output, "SyntaxError") || strings.Contains(output, "IndentationError") {
			return errCompile
		}
	case "javascript":
		if strings.Contains(output, "SyntaxError") {
			return errCompile
		}
	}

	// Import/module errors
	if strings.Contains(output, "ModuleNotFoundError") || strings.Contains(output, "ImportError") ||
		strings.Contains(output, "cannot find module") {
		return errImport
	}

	// Timeout
	if strings.Contains(upper, "TIMEOUT") || strings.Contains(upper, "TIMED OUT") ||
		strings.Contains(output, "context deadline exceeded") {
		return errTimeout
	}

	// Runtime errors
	if strings.Contains(output, "panic:") || strings.Contains(output, "NullPointerException") ||
		strings.Contains(output, "IndexError") || strings.Contains(output, "KeyError") ||
		strings.Contains(output, "TypeError") || strings.Contains(output, "RuntimeError") ||
		strings.Contains(output, "segmentation fault") || strings.Contains(output, "SIGSEGV") {
		return errRuntime
	}

	// Assertion failures (most common for passing code)
	if strings.Contains(output, "FAIL") || strings.Contains(output, "AssertionError") ||
		strings.Contains(output, "assertion failed") || strings.Contains(upper, "EXPECTED") {
		return errAssertion
	}

	return errUnknown
}

// buildErrorAwareFixPrompt creates a fix prompt tailored to the error type.
func buildErrorAwareFixPrompt(ex Exercise, code, testOutput string, et errorType, turn, maxTurns int) string {
	var sb strings.Builder

	// Error-specific guidance
	switch et {
	case errCompile:
		sb.WriteString(fmt.Sprintf("Your code has COMPILATION ERRORS (attempt %d/%d). Fix the syntax and type errors.\n\n", turn, maxTurns))
	case errRuntime:
		sb.WriteString(fmt.Sprintf("Your code CRASHES at runtime (attempt %d/%d). Fix the runtime error — check for nil values, out-of-bounds access, type mismatches.\n\n", turn, maxTurns))
	case errAssertion:
		sb.WriteString(fmt.Sprintf("Your code compiles and runs but produces WRONG RESULTS (attempt %d/%d). The logic is incorrect. Read the test expectations carefully.\n\n", turn, maxTurns))
	case errTimeout:
		sb.WriteString(fmt.Sprintf("Your code TIMED OUT (attempt %d/%d). It's too slow or has an infinite loop. Optimize the algorithm.\n\n", turn, maxTurns))
	case errImport:
		sb.WriteString(fmt.Sprintf("Your code has MISSING IMPORTS (attempt %d/%d). Add the required import statements.\n\n", turn, maxTurns))
	default:
		sb.WriteString(fmt.Sprintf("Your implementation failed the tests (attempt %d/%d). Fix the code based on the error output.\n\n", turn, maxTurns))
	}

	sb.WriteString("## Instructions\n\n")
	sb.WriteString(ex.Instructions)
	sb.WriteString("\n\n")

	sb.WriteString("## Your Code\n\n```\n")
	sb.WriteString(code)
	sb.WriteString("\n```\n\n")

	sb.WriteString("## Test Errors\n\n```\n")
	errOutput := testOutput
	if len(errOutput) > 2000 {
		errOutput = errOutput[:2000] + "\n... (truncated)"
	}
	sb.WriteString(errOutput)
	sb.WriteString("\n```\n\n")

	sb.WriteString("## Test File (for reference)\n\n```\n")
	testContent := ex.TestFile
	if len(testContent) > 2000 {
		testContent = testContent[:2000] + "\n... (truncated)"
	}
	sb.WriteString(testContent)
	sb.WriteString("\n```\n\n")

	sb.WriteString("Return ONLY the corrected implementation code. Fix the specific errors shown above. No tests, no explanations.\n")
	return sb.String()
}

// reviewAndCleanup sends passing code to a reviewer (via routing) for cleanup.
// Returns improved code or empty string if review is skipped/fails.
func (r *Runner) reviewAndCleanup(ctx context.Context, ex Exercise, code string, config EvalRunConfig) string {
	// Only review if code is substantial enough to benefit
	if len(code) < 50 {
		return ""
	}

	reviewPrompt := buildReviewPrompt(ex, code)

	// Use routing mode for review — let the router pick the best available provider
	reviewConfig := config
	reviewConfig.Provider = ""
	reviewConfig.Mode = "routing"

	resp, reviewProvider, _, err := r.sendToProvider(ctx, reviewPrompt, reviewConfig)
	if err != nil {
		log.Printf("[Eval/Agent] %s | review failed: %v", ex.ID, err)
		return ""
	}

	log.Printf("[Eval/Agent] %s | reviewed by %s", ex.ID, reviewProvider)
	reviewed := extractCode(resp)
	if reviewed == "" || reviewed == code {
		return "" // no changes
	}
	return reviewed
}

// buildReviewPrompt asks a reviewer to clean up passing code without changing behavior.
func buildReviewPrompt(ex Exercise, code string) string {
	var sb strings.Builder

	sb.WriteString("You are a code reviewer. The following code PASSES all tests. Your job is to clean it up WITHOUT changing its behavior.\n\n")

	sb.WriteString("## Language\n\n")
	sb.WriteString(ex.Language)
	sb.WriteString("\n\n")

	sb.WriteString("## Code to Review\n\n```\n")
	sb.WriteString(code)
	sb.WriteString("\n```\n\n")

	sb.WriteString("## Review Checklist\n\n")
	sb.WriteString("- Fix naming: use idiomatic conventions for the language\n")
	sb.WriteString("- Remove dead code, unused variables, redundant logic\n")
	sb.WriteString("- Improve readability: clearer variable names, simpler control flow\n")
	sb.WriteString("- Add missing error handling where appropriate\n")

	// Language-specific review guidance
	switch ex.Language {
	case "go":
		sb.WriteString("- Follow Go conventions: exported names, error returns, receiver naming\n")
		sb.WriteString("- Use `fmt.Errorf` with `%w` for error wrapping\n")
	case "python":
		sb.WriteString("- Follow PEP 8: snake_case, docstrings on public functions\n")
		sb.WriteString("- Use list comprehensions where clearer than loops\n")
	case "java":
		sb.WriteString("- Follow Java conventions: camelCase, proper access modifiers\n")
	case "cpp":
		sb.WriteString("- Use const references for read-only parameters\n")
		sb.WriteString("- Prefer standard library algorithms over raw loops\n")
	case "rust":
		sb.WriteString("- Use idiomatic Result/Option handling, avoid unwrap()\n")
		sb.WriteString("- Prefer iterators and closures over index loops\n")
	case "javascript":
		sb.WriteString("- Use const/let, arrow functions, destructuring where appropriate\n")
	}

	sb.WriteString("\nIMPORTANT: Do NOT change the logic or function signatures. The code must still pass the same tests.\n")
	sb.WriteString("Return ONLY the cleaned-up code. No explanations.\n")
	return sb.String()
}

func buildAgentPrompt(ex Exercise) string {
	var sb strings.Builder

	sb.WriteString("You are solving a coding exercise. Read the test file carefully and implement code that passes ALL tests.\n\n")
	sb.WriteString("## Instructions\n\n")
	sb.WriteString(ex.Instructions)
	sb.WriteString("\n\n")

	if ex.Stub != "" {
		sb.WriteString("## Starter Code\n\n```\n")
		sb.WriteString(ex.Stub)
		sb.WriteString("\n```\n\n")
	}

	// Include test file so the model knows exact function signatures and test cases
	sb.WriteString("## Test File (your code MUST pass these tests)\n\n```\n")
	testContent := ex.TestFile
	if len(testContent) > 3000 {
		testContent = testContent[:3000] + "\n... (truncated)"
	}
	sb.WriteString(testContent)
	sb.WriteString("\n```\n\n")

	// Include cases_test.go for Go if available
	if ex.ReferenceCode != "" && ex.Language == "go" {
		sb.WriteString("## Test Cases Data\n\n```go\n")
		refCode := ex.ReferenceCode
		if len(refCode) > 2000 {
			refCode = refCode[:2000] + "\n... (truncated)"
		}
		sb.WriteString(refCode)
		sb.WriteString("\n```\n\n")
	}

	// Language-specific hints
	switch ex.Language {
	case "cpp":
		sb.WriteString("IMPORTANT: Write a header-only implementation. Start with #pragma once and put all code in the .h file. Do NOT write a .cpp file.\n\n")
	}

	// DS1000-specific: explain the [insert] pattern
	if ex.Suite == "ds1000" {
		sb.WriteString("IMPORTANT: Your code will be inserted into a template using exec(). Write ONLY the code fragment — no function definitions wrapping everything, no if __name__ blocks. The code should directly produce the result variable.\n\n")
	}

	sb.WriteString("Return ONLY the implementation code. Match the exact function signatures and types the tests expect. No tests, no main function, no explanations.\n")
	return sb.String()
}

// escalationProviders returns higher-tier providers to try when the initial provider stalls.
// Skips the current provider and returns the rest of the chain in escalation order.
func escalationProviders(current string) []string {
	// Escalation order: free subscriptions first, then paid
	chain := []string{"gemini", "claude-code", "codex"}
	var result []string
	for _, p := range chain {
		if p != current {
			result = append(result, p)
		}
	}
	return result
}

// isEnvironmentError checks if test output indicates a missing dependency rather than a code bug.
func isEnvironmentError(output string) bool {
	return strings.Contains(output, "ModuleNotFoundError") ||
		strings.Contains(output, "ImportError: No module named")
}

// autoFixEnvironment attempts to fix environment issues (e.g., install missing Python modules)
// and re-runs the test. Returns nil if it can't help.
func autoFixEnvironment(ctx context.Context, ex Exercise, code, testOutput string, timeout time.Duration) *DockerTestResult {
	if ex.Language != "python" {
		return nil
	}

	modules := extractMissingModules(testOutput)
	if len(modules) == 0 {
		return nil
	}

	log.Printf("[Eval/Agent] %s | auto-installing missing modules: %v", ex.ID, modules)
	installArgs := append([]string{"-m", "pip", "install", "--quiet"}, modules...)
	installCmd := exec.CommandContext(ctx, "python3", installArgs...)
	if err := installCmd.Run(); err != nil {
		log.Printf("[Eval/Agent] %s | pip install failed: %v", ex.ID, err)
	}

	// Retry the test
	result := runTest(ctx, ex, code, timeout)
	return &result
}

func runTest(ctx context.Context, ex Exercise, code string, timeout time.Duration) DockerTestResult {
	if IsDockerAvailable() {
		return RunTestInDocker(ctx, ex, code, timeout)
	}
	if NativeTestSupported(ex.Language) {
		return RunTestNative(ctx, ex, code, timeout)
	}
	return DockerTestResult{Error: fmt.Sprintf("no test runtime for %s", ex.Language)}
}
