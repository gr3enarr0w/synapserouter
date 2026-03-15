package eval

import "time"

// Exercise represents a coding exercise imported from a benchmark suite.
type Exercise struct {
	ID            string    `json:"id"`
	Suite         string    `json:"suite"`
	Language      string    `json:"language"`
	Slug          string    `json:"slug"`
	Instructions  string    `json:"instructions"`
	Stub          string    `json:"stub,omitempty"`
	TestFile      string    `json:"test_file"`
	TestCommand   string    `json:"test_command"`
	DockerImage   string    `json:"docker_image"`
	EvalMode      string    `json:"eval_mode,omitempty"`
	ReferenceCode string    `json:"reference_code,omitempty"`
	Criteria      string    `json:"criteria,omitempty"`
	ImportedAt    time.Time `json:"imported_at"`
}

// EvalRunConfig configures an eval run.
type EvalRunConfig struct {
	Languages    []string `json:"languages,omitempty"`
	Suite        string   `json:"suite,omitempty"`
	Provider     string   `json:"provider,omitempty"`
	Model        string   `json:"model,omitempty"`
	Mode         string   `json:"mode"`
	Count        int      `json:"count,omitempty"`
	CountPerSuite int     `json:"count_per_suite,omitempty"`
	Seed         int64    `json:"seed,omitempty"`
	TwoPass      bool     `json:"two_pass,omitempty"`
	Timeout      int      `json:"timeout_seconds,omitempty"`
	Concurrency  int      `json:"concurrency,omitempty"`
}

// DefaultCountPerSuite is the default number of exercises sampled per suite
// when running pipeline validation (not exhaustive benchmarking).
const DefaultCountPerSuite = 40

// EvalRun represents a single evaluation run.
type EvalRun struct {
	ID          string     `json:"id"`
	Config      EvalRunConfig `json:"config"`
	Status      string     `json:"status"`
	StartedAt   time.Time  `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	Summary     *EvalSummary `json:"summary,omitempty"`
}

// EvalResult holds the outcome for one exercise in one run.
type EvalResult struct {
	ID               string    `json:"id"`
	RunID            string    `json:"run_id"`
	ExerciseID       string    `json:"exercise_id"`
	Provider         string    `json:"provider"`
	Model            string    `json:"model,omitempty"`
	Pass1            bool      `json:"pass_1"`
	Pass2            bool      `json:"pass_2"`
	GeneratedCode    string    `json:"generated_code,omitempty"`
	TestOutput       string    `json:"test_output,omitempty"`
	ErrorFeedback    string    `json:"error_feedback,omitempty"`
	GeneratedCode2   string    `json:"generated_code_2,omitempty"`
	TestOutput2      string    `json:"test_output_2,omitempty"`
	LatencyMs        int64     `json:"latency_ms"`
	LatencyMs2       int64     `json:"latency_ms_2,omitempty"`
	PromptTokens     int       `json:"prompt_tokens"`
	CompletionTokens int       `json:"completion_tokens"`
	TotalTokens      int       `json:"total_tokens"`
	FallbackUsed     bool      `json:"fallback_used"`
	FallbackChain    []string  `json:"fallback_chain,omitempty"`
	DockerExitCode   int       `json:"docker_exit_code"`
	MetricScore      float64   `json:"metric_score,omitempty"`
	MetricName       string    `json:"metric_name,omitempty"`
	JudgeProvider    string    `json:"judge_provider,omitempty"`
	Error            string    `json:"error,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

// EvalSummary aggregates results for a run.
type EvalSummary struct {
	TotalExercises int                        `json:"total_exercises"`
	Pass1Rate      float64                    `json:"pass_1_rate"`
	Pass2Rate      float64                    `json:"pass_2_rate"`
	AvgLatencyMs   int64                      `json:"avg_latency_ms"`
	TotalTokens    int                        `json:"total_tokens"`
	FallbackRate   float64                    `json:"fallback_rate"`
	AvgMetricScore float64                    `json:"avg_metric_score,omitempty"`
	MetricResults  int                        `json:"metric_results,omitempty"`
	ByProvider     map[string]*ProviderStats  `json:"by_provider"`
	ByLanguage     map[string]*LanguageStats  `json:"by_language"`
}

// ProviderStats holds per-provider metrics.
type ProviderStats struct {
	Total          int     `json:"total"`
	Pass1          int     `json:"pass_1"`
	Pass2          int     `json:"pass_2"`
	Pass1Rate      float64 `json:"pass_1_rate"`
	Pass2Rate      float64 `json:"pass_2_rate"`
	AvgLatency     int64   `json:"avg_latency_ms"`
	Tokens         int     `json:"total_tokens"`
	AvgMetricScore float64 `json:"avg_metric_score,omitempty"`
	MetricResults  int     `json:"metric_results,omitempty"`
}

// LanguageStats holds per-language metrics.
type LanguageStats struct {
	Total          int     `json:"total"`
	Pass1          int     `json:"pass_1"`
	Pass2          int     `json:"pass_2"`
	Pass1Rate      float64 `json:"pass_1_rate"`
	Pass2Rate      float64 `json:"pass_2_rate"`
	AvgMetricScore float64 `json:"avg_metric_score,omitempty"`
	MetricResults  int     `json:"metric_results,omitempty"`
}

// RunComparison shows differences between two runs.
type RunComparison struct {
	RunA          string  `json:"run_a"`
	RunB          string  `json:"run_b"`
	Pass1Delta    float64 `json:"pass_1_delta"`
	Pass2Delta    float64 `json:"pass_2_delta"`
	LatencyDelta  int64   `json:"latency_delta_ms"`
	TokensDelta   int     `json:"tokens_delta"`
	FallbackDelta float64 `json:"fallback_delta"`
}

// DockerConfig maps languages to test execution details.
var DockerConfig = map[string]struct {
	Image       string
	TestCommand string
}{
	"go":         {Image: "golang:1.22", TestCommand: "go test -v ./..."},
	"python":     {Image: "python:3.12", TestCommand: "python -m pytest -v"},
	"python-ds":  {Image: "python:3.12", TestCommand: "pip install -q numpy pandas scipy matplotlib seaborn scikit-learn tensorflow && python test_runner.py"},
	"sql":        {Image: "python:3.12", TestCommand: "python test_runner.py"},
	"javascript": {Image: "node:20", TestCommand: "npm test"},
	"java":       {Image: "eclipse-temurin:21", TestCommand: "gradle test"},
	"rust":       {Image: "rust:1.77", TestCommand: "cargo test"},
	"cpp":        {Image: "gcc:14", TestCommand: "cmake --build . && ctest"},
	"text":       {Image: "", TestCommand: ""},
	"pptx":       {Image: "", TestCommand: ""},
}
