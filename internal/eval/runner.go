package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/providers"
	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/router"
)

// Runner executes eval runs against providers.
type Runner struct {
	store        *Store
	proxyRouter  *router.Router
	providerList []providers.Provider
}

// NewRunner creates a Runner.
func NewRunner(store *Store, proxyRouter *router.Router, providerList []providers.Provider) *Runner {
	return &Runner{
		store:        store,
		proxyRouter:  proxyRouter,
		providerList: providerList,
	}
}

// Run executes an eval run based on the given config.
func (r *Runner) Run(ctx context.Context, config EvalRunConfig) (*EvalRun, error) {
	// Select exercises
	exercises, err := r.selectExercises(config)
	if err != nil {
		return nil, fmt.Errorf("select exercises: %w", err)
	}
	if len(exercises) == 0 {
		return nil, fmt.Errorf("no exercises match the given filters")
	}

	// Log breakdown
	bySuite := make(map[string]int)
	byMode := make(map[string]int)
	for _, ex := range exercises {
		bySuite[ex.Suite]++
		mode := ex.EvalMode
		if mode == "" {
			mode = "docker-test"
		}
		byMode[mode]++
	}
	log.Printf("[Eval] Selected %d exercises across %d suites", len(exercises), len(bySuite))
	for suite, count := range bySuite {
		log.Printf("[Eval]   %s: %d", suite, count)
	}
	for mode, count := range byMode {
		log.Printf("[Eval]   mode=%s: %d", mode, count)
	}

	// Create run
	run := EvalRun{
		ID:        fmt.Sprintf("eval-%d", time.Now().UnixNano()),
		Config:    config,
		Status:    "running",
		StartedAt: time.Now(),
	}
	if err := r.store.CreateRun(run); err != nil {
		return nil, fmt.Errorf("create run: %w", err)
	}

	timeout := 120 * time.Second
	if config.Timeout > 0 {
		timeout = time.Duration(config.Timeout) * time.Second
	}

	// Run exercises (parallel if concurrency > 1)
	concurrency := config.Concurrency
	if concurrency <= 0 {
		concurrency = 1
	}
	if concurrency > 10 {
		concurrency = 10 // cap to avoid overwhelming providers
	}

	if concurrency == 1 {
		// Sequential execution (original behavior)
		for _, ex := range exercises {
			select {
			case <-ctx.Done():
				r.store.FailRun(run.ID)
				return &run, ctx.Err()
			default:
			}

			result := r.runExercise(ctx, ex, config, timeout)
			result.RunID = run.ID
			if err := r.store.InsertResult(result); err != nil {
				log.Printf("[Eval] Warning: failed to store result for %s: %v", ex.ID, err)
			}
		}
	} else {
		// Parallel execution
		log.Printf("[Eval] Running %d exercises with concurrency=%d", len(exercises), concurrency)
		sem := make(chan struct{}, concurrency)
		var wg sync.WaitGroup

		for _, ex := range exercises {
			select {
			case <-ctx.Done():
				r.store.FailRun(run.ID)
				wg.Wait()
				return &run, ctx.Err()
			default:
			}

			wg.Add(1)
			go func(exercise Exercise) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				result := r.runExercise(ctx, exercise, config, timeout)
				result.RunID = run.ID
				if err := r.store.InsertResult(result); err != nil {
					log.Printf("[Eval] Warning: failed to store result for %s: %v", exercise.ID, err)
				}
			}(ex)
		}

		wg.Wait()
	}

	// Compute summary
	results, _ := r.store.GetResultsByRun(run.ID)
	summary := ComputeSummary(results)
	if err := r.store.CompleteRun(run.ID, summary); err != nil {
		log.Printf("[Eval] Warning: failed to complete run: %v", err)
	}

	run.Status = "completed"
	now := time.Now()
	run.CompletedAt = &now
	run.Summary = summary
	return &run, nil
}

func (r *Runner) selectExercises(config EvalRunConfig) ([]Exercise, error) {
	// Build language filter
	language := ""
	if len(config.Languages) == 1 {
		language = config.Languages[0]
	}

	exercises, err := r.store.ListExercises(config.Suite, language)
	if err != nil {
		return nil, err
	}

	// Multi-language filter
	if len(config.Languages) > 1 {
		langSet := make(map[string]bool)
		for _, l := range config.Languages {
			langSet[l] = true
		}
		filtered := exercises[:0]
		for _, ex := range exercises {
			if langSet[ex.Language] {
				filtered = append(filtered, ex)
			}
		}
		exercises = filtered
	}

	seed := config.Seed
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	rng := rand.New(rand.NewSource(seed))

	// Balanced per-suite sampling: take N exercises from each suite
	perSuite := config.CountPerSuite
	if perSuite > 0 {
		exercises = samplePerSuite(exercises, perSuite, rng)
	}

	// Global count cap (applied after per-suite sampling)
	if config.Count > 0 && config.Count < len(exercises) {
		rng.Shuffle(len(exercises), func(i, j int) {
			exercises[i], exercises[j] = exercises[j], exercises[i]
		})
		exercises = exercises[:config.Count]
	}

	return exercises, nil
}

// samplePerSuite takes up to n exercises from each suite, randomly sampled.
func samplePerSuite(exercises []Exercise, n int, rng *rand.Rand) []Exercise {
	bySuite := make(map[string][]Exercise)
	for _, ex := range exercises {
		bySuite[ex.Suite] = append(bySuite[ex.Suite], ex)
	}

	var result []Exercise
	for _, suiteExercises := range bySuite {
		if len(suiteExercises) <= n {
			result = append(result, suiteExercises...)
		} else {
			rng.Shuffle(len(suiteExercises), func(i, j int) {
				suiteExercises[i], suiteExercises[j] = suiteExercises[j], suiteExercises[i]
			})
			result = append(result, suiteExercises[:n]...)
		}
	}
	return result
}

func (r *Runner) runExercise(ctx context.Context, ex Exercise, config EvalRunConfig, timeout time.Duration) EvalResult {
	// Agent mode: iterative test → fix loop with test file context
	if config.AgentMode {
		return r.runExerciseAgent(ctx, ex, config, timeout)
	}

	result := EvalResult{
		ID:         fmt.Sprintf("res-%d", time.Now().UnixNano()),
		ExerciseID: ex.ID,
	}

	evalMode := ex.EvalMode
	if evalMode == "" {
		evalMode = "docker-test"
	}

	// LLM-judge and VLM-judge modes have different flow
	switch evalMode {
	case "llm-judge":
		return r.runLLMJudgeExercise(ctx, ex, config, &result)
	case "vlm-judge":
		result.Error = "vlm-judge eval mode not yet implemented"
		result.Provider = config.Provider
		log.Printf("[Eval] %s | SKIPPED (vlm-judge not implemented)", ex.ID)
		return result
	}

	// Build prompt
	prompt := buildPrompt(ex)

	// Send to provider
	start := time.Now()
	resp, provider, fallbackChain, err := r.sendToProvider(ctx, prompt, config)
	result.LatencyMs = time.Since(start).Milliseconds()

	if err != nil {
		result.Error = err.Error()
		result.Provider = config.Provider
		return result
	}

	result.Provider = provider
	result.Model = resp.Model
	result.PromptTokens = resp.Usage.PromptTokens
	result.CompletionTokens = resp.Usage.CompletionTokens
	result.TotalTokens = resp.Usage.TotalTokens
	if len(fallbackChain) > 1 {
		result.FallbackUsed = true
		result.FallbackChain = fallbackChain
	}

	// Extract code from response
	code := extractCode(resp)
	result.GeneratedCode = code

	// Run tests (pass 1) — Docker if available, native fallback
	var testResult DockerTestResult
	if IsDockerAvailable() {
		testResult = RunTestInDocker(ctx, ex, code, timeout)
	} else if NativeTestSupported(ex.Language) {
		testResult = RunTestNative(ctx, ex, code, timeout)
	} else {
		testResult = DockerTestResult{Error: fmt.Sprintf("no test runtime for %s (Docker unavailable)", ex.Language)}
	}
	result.Pass1 = testResult.Passed
	result.TestOutput = testResult.Output
	result.DockerExitCode = testResult.ExitCode

	// For metric-compare mode, extract metric score from test output
	if evalMode == "metric-compare" {
		result.MetricName = extractMetricName(ex.Criteria)
		result.MetricScore = extractMetricScore(testResult.Output)
		if result.MetricScore > 0 {
			result.Pass1 = true
		}
	}

	// Pass 2 (if enabled and pass 1 failed)
	if config.TwoPass && !result.Pass1 && testResult.Error == "" {
		result.ErrorFeedback = testResult.Output

		pass2Prompt := buildPass2Prompt(ex, code, testResult.Output)
		start2 := time.Now()
		resp2, _, _, err2 := r.sendToProvider(ctx, pass2Prompt, config)
		result.LatencyMs2 = time.Since(start2).Milliseconds()

		if err2 == nil {
			code2 := extractCode(resp2)
			result.GeneratedCode2 = code2
			result.TotalTokens += resp2.Usage.TotalTokens

			var testResult2 DockerTestResult
			if IsDockerAvailable() {
				testResult2 = RunTestInDocker(ctx, ex, code2, timeout)
			} else {
				testResult2 = RunTestNative(ctx, ex, code2, timeout)
			}
			result.Pass2 = testResult2.Passed
			result.TestOutput2 = testResult2.Output
			if testResult2.ExitCode != 0 {
				result.DockerExitCode = testResult2.ExitCode
			}

			if evalMode == "metric-compare" {
				score2 := extractMetricScore(testResult2.Output)
				if score2 > result.MetricScore {
					result.MetricScore = score2
				}
				if score2 > 0 {
					result.Pass2 = true
				}
			}
		}
	}

	log.Printf("[Eval] %s | provider=%s | pass1=%v pass2=%v | %dms",
		ex.ID, provider, result.Pass1, result.Pass2, result.LatencyMs)

	return result
}

func (r *Runner) runLLMJudgeExercise(ctx context.Context, ex Exercise, config EvalRunConfig, result *EvalResult) EvalResult {
	// Step 1: Generate the writing response
	prompt := buildWritingPrompt(ex)

	start := time.Now()
	resp, provider, fallbackChain, err := r.sendToProvider(ctx, prompt, config)
	result.LatencyMs = time.Since(start).Milliseconds()

	if err != nil {
		result.Error = err.Error()
		result.Provider = config.Provider
		return *result
	}

	result.Provider = provider
	result.Model = resp.Model
	result.PromptTokens = resp.Usage.PromptTokens
	result.CompletionTokens = resp.Usage.CompletionTokens
	result.TotalTokens = resp.Usage.TotalTokens
	if len(fallbackChain) > 1 {
		result.FallbackUsed = true
		result.FallbackChain = fallbackChain
	}

	response := extractFullResponse(resp)
	result.GeneratedCode = response

	// Step 2: Send to LLM judge for scoring
	judgePrompt := buildJudgePrompt(ex, response)
	judgeConfig := config
	// Use routing mode for judge to let the router pick
	judgeConfig.Mode = "routing"
	judgeConfig.Provider = ""

	judgeResp, judgeProvider, _, judgeErr := r.sendToProvider(ctx, judgePrompt, judgeConfig)
	if judgeErr != nil {
		result.Error = fmt.Sprintf("judge error: %v", judgeErr)
		return *result
	}

	result.JudgeProvider = judgeProvider
	result.TotalTokens += judgeResp.Usage.TotalTokens

	// Parse judge score
	judgeOutput := extractFullResponse(judgeResp)
	result.TestOutput = judgeOutput
	result.MetricName = "llm_judge_score"
	result.MetricScore = parseJudgeScore(judgeOutput)
	result.Pass1 = result.MetricScore >= 5.0

	log.Printf("[Eval] %s | provider=%s | judge=%s | score=%.1f | %dms",
		ex.ID, provider, judgeProvider, result.MetricScore, result.LatencyMs)

	return *result
}

func buildWritingPrompt(ex Exercise) string {
	return ex.Instructions
}

func buildJudgePrompt(ex Exercise, response string) string {
	var sb strings.Builder
	sb.WriteString("You are an expert writing evaluator. Score the following response on a scale of 1-10.\n\n")
	sb.WriteString("## Original Task\n\n")
	sb.WriteString(ex.Instructions)
	sb.WriteString("\n\n## Response to Evaluate\n\n")
	sb.WriteString(response)
	sb.WriteString("\n\n## Evaluation Criteria\n\n")
	if ex.Criteria != "" {
		sb.WriteString(ex.Criteria)
	} else {
		sb.WriteString("Evaluate on: relevance, quality, completeness, clarity, and style.")
	}
	sb.WriteString("\n\n## Instructions\n\n")
	sb.WriteString("Provide a single overall score from 1-10. Output ONLY the numeric score on the last line, like:\nSCORE: 7\n")
	return sb.String()
}

func parseJudgeScore(output string) float64 {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	// Search from bottom up for "SCORE: N" or just a number
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(strings.ToUpper(line), "SCORE:") {
			scoreStr := strings.TrimSpace(strings.TrimPrefix(strings.ToUpper(line), "SCORE:"))
			var score float64
			if _, err := fmt.Sscanf(scoreStr, "%f", &score); err == nil && score >= 1 && score <= 10 {
				return score
			}
		}
		// Try parsing just a number
		var score float64
		if _, err := fmt.Sscanf(line, "%f", &score); err == nil && score >= 1 && score <= 10 {
			return score
		}
	}
	return 0
}

func extractFullResponse(resp providers.ChatResponse) string {
	if len(resp.Choices) == 0 {
		return ""
	}
	return resp.Choices[0].Message.Content
}

func extractMetricName(criteria string) string {
	// Parse metric from criteria JSON like {"task_type":"classification","metric":"macro_f1"}
	var parsed struct {
		Metric string `json:"metric"`
	}
	if err := json.Unmarshal([]byte(criteria), &parsed); err == nil && parsed.Metric != "" {
		return parsed.Metric
	}
	return "metric_score"
}

func extractMetricScore(output string) float64 {
	// Look for patterns like "METRIC_SCORE: 0.85" or "Score: 0.92" in output
	lines := strings.Split(output, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		upper := strings.ToUpper(line)
		for _, prefix := range []string{"METRIC_SCORE:", "SCORE:", "METRIC:"} {
			if strings.HasPrefix(upper, prefix) {
				scoreStr := strings.TrimSpace(line[len(prefix):])
				var score float64
				if _, err := fmt.Sscanf(scoreStr, "%f", &score); err == nil {
					return score
				}
			}
		}
	}
	return 0
}

func (r *Runner) sendToProvider(ctx context.Context, prompt string, config EvalRunConfig) (providers.ChatResponse, string, []string, error) {
	req := providers.ChatRequest{
		Model: config.Model,
		Messages: []providers.Message{
			{Role: "system", Content: "You are a coding assistant. Return ONLY the implementation code. Do not include tests, explanations, or markdown fences. Return raw code only."},
			{Role: "user", Content: prompt},
		},
		MaxTokens:  4096,
		SkipMemory: true,
	}
	if req.Model == "" {
		req.Model = "auto"
	}

	sessionID := fmt.Sprintf("eval-%d", time.Now().UnixNano())

	if config.Mode == "routing" || config.Provider == "" {
		// Routing mode: let the router decide
		resp, err := r.proxyRouter.ChatCompletion(ctx, req, sessionID)
		if err != nil {
			return providers.ChatResponse{}, "", nil, err
		}
		provider := ""
		if resp.XProxyMetadata != nil {
			provider = resp.XProxyMetadata.Provider
		}
		return resp, provider, nil, nil
	}

	// Direct mode: send to specific provider
	resp, err := r.proxyRouter.ChatCompletionForProvider(ctx, req, sessionID, config.Provider, false)
	if err != nil {
		return providers.ChatResponse{}, "", nil, err
	}
	provider := config.Provider
	if resp.XProxyMetadata != nil {
		provider = resp.XProxyMetadata.Provider
	}
	return resp, provider, nil, nil
}

func buildPrompt(ex Exercise) string {
	var sb strings.Builder
	sb.WriteString("Implement the following exercise.\n\n")
	sb.WriteString("## Instructions\n\n")
	sb.WriteString(ex.Instructions)
	sb.WriteString("\n\n")

	if ex.Stub != "" {
		sb.WriteString("## Starter Code\n\n")
		sb.WriteString("```\n")
		sb.WriteString(ex.Stub)
		sb.WriteString("\n```\n\n")
	}

	sb.WriteString("Return ONLY the implementation code. No tests, no explanations.\n")
	return sb.String()
}

func buildPass2Prompt(ex Exercise, firstAttempt, testOutput string) string {
	var sb strings.Builder
	sb.WriteString("Your previous implementation failed the tests. Fix the code.\n\n")
	sb.WriteString("## Instructions\n\n")
	sb.WriteString(ex.Instructions)
	sb.WriteString("\n\n")
	sb.WriteString("## Your Previous Code\n\n```\n")
	sb.WriteString(firstAttempt)
	sb.WriteString("\n```\n\n")
	sb.WriteString("## Test Errors\n\n```\n")
	sb.WriteString(testOutput)
	sb.WriteString("\n```\n\n")
	sb.WriteString("Return ONLY the corrected implementation code. No tests, no explanations.\n")
	return sb.String()
}

var codeBlockRegex = regexp.MustCompile("(?s)```(?:\\w+)?\\s*\n(.*?)\\s*```")

func extractCode(resp providers.ChatResponse) string {
	if len(resp.Choices) == 0 {
		return ""
	}

	content := resp.Choices[0].Message.Content

	// Try to extract from markdown code blocks
	matches := codeBlockRegex.FindStringSubmatch(content)
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}

	// No code blocks — assume the entire response is code
	return strings.TrimSpace(content)
}
