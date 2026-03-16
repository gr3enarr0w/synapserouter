package eval

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/providers"
	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/router"
)

// runExerciseAgent uses an iterative agent loop to solve an exercise.
// The agent generates code, runs tests, reads errors, fixes code, and repeats.
func (r *Runner) runExerciseAgent(ctx context.Context, ex Exercise, config EvalRunConfig, timeout time.Duration) EvalResult {
	result := EvalResult{
		ID:         fmt.Sprintf("res-%d", time.Now().UnixNano()),
		ExerciseID: ex.ID,
	}

	maxTurns := config.MaxTurns
	if maxTurns <= 0 {
		maxTurns = 5 // default: up to 5 attempts
	}

	start := time.Now()

	// Step 1: Initial code generation with test file context
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

	// Step 2: Iterative test → fix loop
	for turn := 0; turn < maxTurns; turn++ {
		testResult := runTest(ctx, ex, code, timeout)
		result.TestOutput = testResult.Output
		result.DockerExitCode = testResult.ExitCode

		if testResult.Passed {
			result.Pass1 = true
			if turn > 0 {
				result.Pass2 = true // fixed on a subsequent turn
			}
			log.Printf("[Eval/Agent] %s | PASS on turn %d/%d | %dms",
				ex.ID, turn+1, maxTurns, time.Since(start).Milliseconds())
			break
		}

		if turn == maxTurns-1 {
			// Last turn, no more fixes
			log.Printf("[Eval/Agent] %s | FAIL after %d turns | %dms",
				ex.ID, maxTurns, time.Since(start).Milliseconds())
			break
		}

		// Step 3: Feed error back to LLM for a fix
		fixPrompt := buildAgentFixPrompt(ex, code, testResult.Output, turn+1, maxTurns)
		fixResp, _, _, fixErr := r.sendToProvider(ctx, fixPrompt, config)
		if fixErr != nil {
			result.Error = fmt.Sprintf("fix turn %d: %v", turn+1, fixErr)
			break
		}

		result.TotalTokens += fixResp.Usage.TotalTokens
		newCode := extractCode(fixResp)
		if newCode == "" || newCode == code {
			// LLM returned same code or empty — stop iterating
			log.Printf("[Eval/Agent] %s | no new code on turn %d, stopping", ex.ID, turn+2)
			break
		}
		code = newCode
		result.GeneratedCode2 = code
	}

	result.LatencyMs = time.Since(start).Milliseconds()

	log.Printf("[Eval/Agent] %s | provider=%s | pass=%v | turns=%d | %dms | tokens=%d",
		ex.ID, provider, result.Pass1, maxTurns, result.LatencyMs, result.TotalTokens)

	return result
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

	sb.WriteString("Return ONLY the implementation code. Match the exact function signatures and types the tests expect. No tests, no main function, no explanations.\n")
	return sb.String()
}

func buildAgentFixPrompt(ex Exercise, code, testOutput string, turn, maxTurns int) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Your implementation failed the tests (attempt %d/%d). Fix the code based on the error output.\n\n", turn, maxTurns))

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

	// Include test file for reference
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

func runTest(ctx context.Context, ex Exercise, code string, timeout time.Duration) DockerTestResult {
	if IsDockerAvailable() {
		return RunTestInDocker(ctx, ex, code, timeout)
	}
	if NativeTestSupported(ex.Language) {
		return RunTestNative(ctx, ex, code, timeout)
	}
	return DockerTestResult{Error: fmt.Sprintf("no test runtime for %s", ex.Language)}
}

// scaffoldAndRunNative creates a workspace, writes code, and runs tests natively.
// Used internally by RunTestNative but exposed for the agent runner.
func scaffoldAndRunNative(ctx context.Context, ex Exercise, code string, timeout time.Duration) (string, bool, error) {
	tmpDir, err := os.MkdirTemp("", "eval-agent-*")
	if err != nil {
		return "", false, err
	}
	defer os.RemoveAll(tmpDir)

	if err := scaffoldWorkspace(tmpDir, ex, code); err != nil {
		return "", false, fmt.Errorf("scaffold: %w", err)
	}

	result := runTest(ctx, ex, code, timeout)
	return result.Output, result.Passed, nil
}

// Ensure router import is used
var _ *router.Router
var _ providers.ChatResponse
var _ = filepath.Join
