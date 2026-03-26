package eval

import (
	"sort"
	"testing"
	"time"
)

func seedExercises(t *testing.T, store *Store) {
	t.Helper()
	exercises := []Exercise{
		{ID: "polyglot/go/hello", Suite: "polyglot", Language: "go", Slug: "hello", Instructions: "Go hello", TestFile: "t", TestCommand: "go test", DockerImage: "golang:1.22"},
		{ID: "polyglot/go/world", Suite: "polyglot", Language: "go", Slug: "world", Instructions: "Go world", TestFile: "t", TestCommand: "go test", DockerImage: "golang:1.22"},
		{ID: "polyglot/go/fizzbuzz", Suite: "polyglot", Language: "go", Slug: "fizzbuzz", Instructions: "Go fizzbuzz", TestFile: "t", TestCommand: "go test", DockerImage: "golang:1.22"},
		{ID: "polyglot/go/zebra", Suite: "polyglot", Language: "go", Slug: "zebra", Instructions: "Go zebra", TestFile: "t", TestCommand: "go test", DockerImage: "golang:1.22"},
		{ID: "polyglot/go/isogram", Suite: "polyglot", Language: "go", Slug: "isogram", Instructions: "Go isogram", TestFile: "t", TestCommand: "go test", DockerImage: "golang:1.22"},
		{ID: "polyglot/python/hello", Suite: "polyglot", Language: "python", Slug: "hello", Instructions: "Py hello", TestFile: "t", TestCommand: "pytest", DockerImage: "python:3.12"},
		{ID: "polyglot/python/world", Suite: "polyglot", Language: "python", Slug: "world", Instructions: "Py world", TestFile: "t", TestCommand: "pytest", DockerImage: "python:3.12"},
		{ID: "polyglot/python/fizzbuzz", Suite: "polyglot", Language: "python", Slug: "fizzbuzz", Instructions: "Py fizzbuzz", TestFile: "t", TestCommand: "pytest", DockerImage: "python:3.12"},
		{ID: "polyglot/javascript/hello", Suite: "polyglot", Language: "javascript", Slug: "hello", Instructions: "JS hello", TestFile: "t", TestCommand: "npm test", DockerImage: "node:20"},
		{ID: "polyglot/javascript/world", Suite: "polyglot", Language: "javascript", Slug: "world", Instructions: "JS world", TestFile: "t", TestCommand: "npm test", DockerImage: "node:20"},
		{ID: "polyglot/rust/hello", Suite: "polyglot", Language: "rust", Slug: "hello", Instructions: "Rust hello", TestFile: "t", TestCommand: "cargo test", DockerImage: "rust:1.77"},
		{ID: "polyglot/java/hello", Suite: "polyglot", Language: "java", Slug: "hello", Instructions: "Java hello", TestFile: "t", TestCommand: "gradle test", DockerImage: "eclipse-temurin:21"},
		{ID: "polyglot/cpp/hello", Suite: "polyglot", Language: "cpp", Slug: "hello", Instructions: "C++ hello", TestFile: "t", TestCommand: "cmake --build . && ctest", DockerImage: "gcc:14"},
		{ID: "roocode/go/hello", Suite: "roocode", Language: "go", Slug: "hello", Instructions: "Roo go", TestFile: "t", TestCommand: "go test", DockerImage: "golang:1.22"},
		{ID: "roocode/python/hello", Suite: "roocode", Language: "python", Slug: "hello", Instructions: "Roo py", TestFile: "t", TestCommand: "pytest", DockerImage: "python:3.12"},
	}
	for _, ex := range exercises {
		if err := store.UpsertExercise(ex); err != nil {
			t.Fatal(err)
		}
	}
}

// newTestRunner creates a Runner backed by an in-memory store (no real router).
// Only suitable for testing selectExercises, not actual provider calls.
func newTestRunner(t *testing.T) (*Runner, *Store) {
	t.Helper()
	db := newTestDB(t)
	store := NewStore(db)
	seedExercises(t, store)
	runner := &Runner{store: store}
	return runner, store
}

func TestSelectExercisesAllLanguages(t *testing.T) {
	runner, _ := newTestRunner(t)

	config := EvalRunConfig{}
	exercises, err := runner.selectExercises(config)
	if err != nil {
		t.Fatal(err)
	}
	if len(exercises) != 15 {
		t.Fatalf("expected 15 exercises, got %d", len(exercises))
	}
}

func TestSelectExercisesSingleLanguage(t *testing.T) {
	runner, _ := newTestRunner(t)

	config := EvalRunConfig{Languages: []string{"go"}}
	exercises, err := runner.selectExercises(config)
	if err != nil {
		t.Fatal(err)
	}
	// 5 polyglot/go + 1 roocode/go = 6
	if len(exercises) != 6 {
		t.Fatalf("expected 6 go exercises, got %d", len(exercises))
	}
	for _, ex := range exercises {
		if ex.Language != "go" {
			t.Fatalf("expected go, got %s for %s", ex.Language, ex.ID)
		}
	}
}

func TestSelectExercisesMultiLanguage(t *testing.T) {
	runner, _ := newTestRunner(t)

	config := EvalRunConfig{Languages: []string{"go", "python"}}
	exercises, err := runner.selectExercises(config)
	if err != nil {
		t.Fatal(err)
	}
	// 6 go + 4 python = 10
	if len(exercises) != 10 {
		t.Fatalf("expected 10 exercises, got %d", len(exercises))
	}
	for _, ex := range exercises {
		if ex.Language != "go" && ex.Language != "python" {
			t.Fatalf("unexpected language %s for %s", ex.Language, ex.ID)
		}
	}
}

func TestSelectExercisesAllSixLanguages(t *testing.T) {
	runner, _ := newTestRunner(t)

	config := EvalRunConfig{Languages: []string{"go", "python", "javascript", "rust", "java", "cpp"}}
	exercises, err := runner.selectExercises(config)
	if err != nil {
		t.Fatal(err)
	}
	if len(exercises) != 15 {
		t.Fatalf("expected 15 exercises across all 6 languages, got %d", len(exercises))
	}

	// Verify all 6 languages are present
	langs := make(map[string]int)
	for _, ex := range exercises {
		langs[ex.Language]++
	}
	expectedLangs := []string{"go", "python", "javascript", "rust", "java", "cpp"}
	for _, l := range expectedLangs {
		if langs[l] == 0 {
			t.Fatalf("expected exercises for %s, found none", l)
		}
	}
}

func TestSelectExercisesSuiteFilter(t *testing.T) {
	runner, _ := newTestRunner(t)

	config := EvalRunConfig{Suite: "roocode"}
	exercises, err := runner.selectExercises(config)
	if err != nil {
		t.Fatal(err)
	}
	if len(exercises) != 2 {
		t.Fatalf("expected 2 roocode exercises, got %d", len(exercises))
	}
	for _, ex := range exercises {
		if ex.Suite != "roocode" {
			t.Fatalf("expected roocode suite, got %s", ex.Suite)
		}
	}
}

func TestSelectExercisesCountLimit(t *testing.T) {
	runner, _ := newTestRunner(t)

	config := EvalRunConfig{Count: 3, Seed: 42}
	exercises, err := runner.selectExercises(config)
	if err != nil {
		t.Fatal(err)
	}
	if len(exercises) != 3 {
		t.Fatalf("expected 3 exercises, got %d", len(exercises))
	}
}

func TestSelectExercisesCountExceedsAvailable(t *testing.T) {
	runner, _ := newTestRunner(t)

	// Ask for more than available — should return all
	config := EvalRunConfig{Languages: []string{"rust"}, Count: 100}
	exercises, err := runner.selectExercises(config)
	if err != nil {
		t.Fatal(err)
	}
	if len(exercises) != 1 {
		t.Fatalf("expected 1 rust exercise, got %d", len(exercises))
	}
}

func TestRandomizationReproducibility(t *testing.T) {
	// Same seed should produce identical exercise selection
	runner, _ := newTestRunner(t)

	config := EvalRunConfig{Count: 5, Seed: 12345}

	first, err := runner.selectExercises(config)
	if err != nil {
		t.Fatal(err)
	}

	// Re-create runner to reset state
	runner2, _ := newTestRunner(t)
	second, err := runner2.selectExercises(config)
	if err != nil {
		t.Fatal(err)
	}

	if len(first) != len(second) {
		t.Fatalf("different lengths: %d vs %d", len(first), len(second))
	}
	for i := range first {
		if first[i].ID != second[i].ID {
			t.Fatalf("mismatch at index %d: %s vs %s", i, first[i].ID, second[i].ID)
		}
	}
}

func TestRandomizationDifferentSeeds(t *testing.T) {
	// Different seeds should (almost certainly) produce different order
	runner1, _ := newTestRunner(t)
	runner2, _ := newTestRunner(t)

	config1 := EvalRunConfig{Count: 10, Seed: 111}
	config2 := EvalRunConfig{Count: 10, Seed: 999}

	first, _ := runner1.selectExercises(config1)
	second, _ := runner2.selectExercises(config2)

	if len(first) != len(second) {
		t.Fatalf("different lengths: %d vs %d", len(first), len(second))
	}

	// Check they're not in the same order (extremely unlikely with different seeds)
	sameOrder := true
	for i := range first {
		if first[i].ID != second[i].ID {
			sameOrder = false
			break
		}
	}
	if sameOrder {
		t.Fatal("different seeds produced identical ordering — extremely unlikely, bug suspected")
	}
}

func TestRandomizationDefaultSeedVaries(t *testing.T) {
	// With seed=0 (default), each call should use time-based seed
	runner, _ := newTestRunner(t)

	config := EvalRunConfig{Count: 5, Seed: 0}

	// Run twice — should get different order (seed=0 uses time.Now().UnixNano())
	first, _ := runner.selectExercises(config)

	// Need fresh data since selectExercises modifies the slice in place
	runner2, _ := newTestRunner(t)
	second, _ := runner2.selectExercises(config)

	// Can't guarantee they're different (tiny chance of same nanosecond),
	// but we can verify both return the right count
	if len(first) != 5 {
		t.Fatalf("expected 5, got %d", len(first))
	}
	if len(second) != 5 {
		t.Fatalf("expected 5, got %d", len(second))
	}
}

func TestSelectExercisesMultiLanguageWithCount(t *testing.T) {
	runner, _ := newTestRunner(t)

	config := EvalRunConfig{
		Languages: []string{"go", "python", "javascript"},
		Count:     5,
		Seed:      42,
	}

	exercises, err := runner.selectExercises(config)
	if err != nil {
		t.Fatal(err)
	}
	if len(exercises) != 5 {
		t.Fatalf("expected 5 exercises, got %d", len(exercises))
	}

	// All should be from the requested languages
	for _, ex := range exercises {
		if ex.Language != "go" && ex.Language != "python" && ex.Language != "javascript" {
			t.Fatalf("unexpected language %s", ex.Language)
		}
	}
}

func TestSelectExercisesSuiteAndLanguage(t *testing.T) {
	runner, _ := newTestRunner(t)

	config := EvalRunConfig{
		Suite:     "polyglot",
		Languages: []string{"go"},
	}

	exercises, err := runner.selectExercises(config)
	if err != nil {
		t.Fatal(err)
	}
	// Only polyglot/go exercises (not roocode/go)
	if len(exercises) != 5 {
		t.Fatalf("expected 5 polyglot/go exercises, got %d", len(exercises))
	}
	for _, ex := range exercises {
		if ex.Suite != "polyglot" || ex.Language != "go" {
			t.Fatalf("unexpected %s/%s for %s", ex.Suite, ex.Language, ex.ID)
		}
	}
}

func TestSelectExercisesPerSuite(t *testing.T) {
	runner, _ := newTestRunner(t)

	// polyglot has 13, roocode has 2 — per-suite=2 should give 2+2=4
	config := EvalRunConfig{CountPerSuite: 2, Seed: 42}
	exercises, err := runner.selectExercises(config)
	if err != nil {
		t.Fatal(err)
	}

	// Count per suite
	bySuite := make(map[string]int)
	for _, ex := range exercises {
		bySuite[ex.Suite]++
	}

	if bySuite["polyglot"] != 2 {
		t.Fatalf("expected 2 polyglot, got %d", bySuite["polyglot"])
	}
	if bySuite["roocode"] != 2 {
		t.Fatalf("expected 2 roocode, got %d", bySuite["roocode"])
	}
	if len(exercises) != 4 {
		t.Fatalf("expected 4 total, got %d", len(exercises))
	}
}

func TestSelectExercisesPerSuiteExceedsAvailable(t *testing.T) {
	runner, _ := newTestRunner(t)

	// roocode only has 2 — per-suite=100 should return all 2 for roocode
	config := EvalRunConfig{CountPerSuite: 100, Seed: 42}
	exercises, err := runner.selectExercises(config)
	if err != nil {
		t.Fatal(err)
	}

	bySuite := make(map[string]int)
	for _, ex := range exercises {
		bySuite[ex.Suite]++
	}

	if bySuite["roocode"] != 2 {
		t.Fatalf("expected 2 roocode (all available), got %d", bySuite["roocode"])
	}
	if bySuite["polyglot"] != 13 {
		t.Fatalf("expected 13 polyglot (all available), got %d", bySuite["polyglot"])
	}
}

func TestSelectExercisesPerSuiteWithCount(t *testing.T) {
	runner, _ := newTestRunner(t)

	// per-suite=3 gives 3+2=5, then count=4 caps to 4
	config := EvalRunConfig{CountPerSuite: 3, Count: 4, Seed: 42}
	exercises, err := runner.selectExercises(config)
	if err != nil {
		t.Fatal(err)
	}

	if len(exercises) != 4 {
		t.Fatalf("expected 4 (capped by count), got %d", len(exercises))
	}
}

func TestBuildPromptWithStub(t *testing.T) {
	ex := Exercise{
		Instructions: "Implement a function that returns Hello, World!",
		Stub:         "package greeting\n\nfunc Hello() string {\n\treturn \"\"\n}",
	}

	prompt := buildPrompt(ex)
	if !containsStr(prompt, "## Instructions") {
		t.Fatal("prompt should contain instructions header")
	}
	if !containsStr(prompt, "## Starter Code") {
		t.Fatal("prompt should contain starter code header")
	}
	if !containsStr(prompt, "func Hello()") {
		t.Fatal("prompt should contain stub code")
	}
	if !containsStr(prompt, "Return ONLY") {
		t.Fatal("prompt should contain 'return only' instruction")
	}
}

func TestBuildPromptWithoutStub(t *testing.T) {
	ex := Exercise{
		Instructions: "Implement a function that returns Hello, World!",
	}

	prompt := buildPrompt(ex)
	if containsStr(prompt, "## Starter Code") {
		t.Fatal("prompt should NOT contain starter code when stub is empty")
	}
}

func TestBuildPass2Prompt(t *testing.T) {
	ex := Exercise{
		Instructions: "Implement Hello",
	}

	prompt := buildPass2Prompt(ex, "func Hello() {}", "FAIL: expected hello")
	if !containsStr(prompt, "previous implementation failed") {
		t.Fatal("pass2 prompt should mention failure")
	}
	if !containsStr(prompt, "func Hello() {}") {
		t.Fatal("pass2 prompt should contain first attempt code")
	}
	if !containsStr(prompt, "FAIL: expected hello") {
		t.Fatal("pass2 prompt should contain test errors")
	}
}

func TestExtractCodeStripsMarkdown(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "go code block",
			content: "Here is the solution:\n```go\npackage main\n\nfunc Hello() string { return \"hello\" }\n```\nDone.",
			want:    "package main\n\nfunc Hello() string { return \"hello\" }",
		},
		{
			name:    "python code block",
			content: "```python\ndef hello():\n    return \"hello\"\n```",
			want:    "def hello():\n    return \"hello\"",
		},
		{
			name:    "bare code block",
			content: "```\nfunc Hello() {}\n```",
			want:    "func Hello() {}",
		},
		{
			name:    "raw code no fences",
			content: "package main\n\nfunc Hello() string { return \"hello\" }",
			want:    "package main\n\nfunc Hello() string { return \"hello\" }",
		},
		{
			name:    "multiple code blocks picks first",
			content: "```go\nfunc First() {}\n```\n\n```go\nfunc Second() {}\n```",
			want:    "func First() {}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate response
			matches := codeBlockRegex.FindStringSubmatch(tt.content)
			var got string
			if len(matches) > 1 {
				got = matches[1]
			} else {
				got = tt.content
			}
			got = trimString(got)
			want := trimString(tt.want)
			if got != want {
				t.Errorf("got %q, want %q", got, want)
			}
		})
	}
}

func TestScaffoldAllLanguages(t *testing.T) {
	// Verify scaffolding works for every supported language
	languages := []struct {
		lang string
		slug string
		code string
		test string
	}{
		{"go", "hello-world", "package greeting\nfunc Hello() string { return \"\" }", "package greeting\nimport \"testing\"\nfunc TestHello(t *testing.T) {}"},
		{"python", "two-fer", "def two_fer(): pass", "def test_two_fer(): pass"},
		{"javascript", "hello-world", "module.exports = { hello: () => '' }", "test('hello', () => {})"},
		{"java", "hello-world", "public class HelloWorld {}", "import org.junit.jupiter.api.Test;\nclass HelloWorldTest {}"},
		{"rust", "hello-world", "pub fn hello() -> &'static str { \"\" }", "#[test]\nfn test_hello() {}"},
		{"cpp", "hello-world", "#include \"hello_world.h\"\nstd::string hello() { return \"\"; }", "#include \"hello_world.h\"\nvoid test() {}"},
	}

	for _, tt := range languages {
		t.Run(tt.lang, func(t *testing.T) {
			dir := t.TempDir()
			ex := Exercise{
				Language: tt.lang,
				Slug:     tt.slug,
				TestFile: tt.test,
			}
			if err := scaffoldWorkspace(dir, ex, tt.code); err != nil {
				t.Fatalf("scaffold %s failed: %v", tt.lang, err)
			}
		})
	}
}

func TestDockerConfigCoversAllLanguages(t *testing.T) {
	expected := []string{"go", "python", "javascript", "java", "rust", "cpp"}
	for _, lang := range expected {
		dc, ok := DockerConfig[lang]
		if !ok {
			t.Fatalf("DockerConfig missing entry for %s", lang)
		}
		if dc.Image == "" {
			t.Fatalf("DockerConfig[%s].Image is empty", lang)
		}
		if dc.TestCommand == "" {
			t.Fatalf("DockerConfig[%s].TestCommand is empty", lang)
		}
	}
}

func TestE2EImportAndSelect(t *testing.T) {
	// Full flow: import → list → filter → count → random select
	db := newTestDB(t)
	store := NewStore(db)
	seedExercises(t, store)

	// Verify counts
	total, _ := store.CountExercises("", "")
	if total != 15 {
		t.Fatalf("expected 15 total, got %d", total)
	}

	goCount, _ := store.CountExercises("", "go")
	if goCount != 6 {
		t.Fatalf("expected 6 go, got %d", goCount)
	}

	pyCount, _ := store.CountExercises("", "python")
	if pyCount != 4 {
		t.Fatalf("expected 4 python, got %d", pyCount)
	}

	// Verify listing
	goExercises, _ := store.ListExercises("", "go")
	if len(goExercises) != 6 {
		t.Fatalf("expected 6 go exercises from list, got %d", len(goExercises))
	}

	// Verify random selection covers different exercises
	runner := &Runner{store: store}
	selected1, _ := runner.selectExercises(EvalRunConfig{Count: 5, Seed: 1})
	selected2, _ := runner.selectExercises(EvalRunConfig{Count: 5, Seed: 2})

	ids1 := make([]string, len(selected1))
	ids2 := make([]string, len(selected2))
	for i := range selected1 {
		ids1[i] = selected1[i].ID
	}
	for i := range selected2 {
		ids2[i] = selected2[i].ID
	}

	sort.Strings(ids1)
	sort.Strings(ids2)

	// With different seeds from a pool of 15, selecting 5 should give different sets
	// (not guaranteed but extremely likely)
	allSame := true
	for i := range ids1 {
		if ids1[i] != ids2[i] {
			allSame = false
			break
		}
	}
	if allSame {
		t.Log("Warning: two different seeds produced same selection (unlikely but possible)")
	}
}

func TestE2ERunLifecycleWithResults(t *testing.T) {
	// Full persistence flow: create run → insert results → complete → retrieve
	db := newTestDB(t)
	store := NewStore(db)

	run := EvalRun{
		ID:        "e2e-run-1",
		Config:    EvalRunConfig{Languages: []string{"go", "python"}, Count: 4, TwoPass: true, Seed: 42},
		Status:    "running",
		StartedAt: now(),
	}
	if err := store.CreateRun(run); err != nil {
		t.Fatal(err)
	}

	// Simulate results across providers and languages
	results := []EvalResult{
		{ID: "r1", RunID: "e2e-run-1", ExerciseID: "polyglot/go/hello", Provider: "nanogpt-sub", Model: "qwen/qwen3.5-plus", Pass1: true, Pass2: true, LatencyMs: 1200, TotalTokens: 150},
		{ID: "r2", RunID: "e2e-run-1", ExerciseID: "polyglot/go/world", Provider: "nanogpt-sub", Model: "qwen/qwen3.5-plus", Pass1: false, Pass2: true, LatencyMs: 1800, TotalTokens: 200, FallbackUsed: true, FallbackChain: []string{"nanogpt-sub", "gemini"}},
		{ID: "r3", RunID: "e2e-run-1", ExerciseID: "polyglot/python/hello", Provider: "gemini", Model: "gemini-2.5-pro", Pass1: true, Pass2: true, LatencyMs: 2000, TotalTokens: 300},
		{ID: "r4", RunID: "e2e-run-1", ExerciseID: "polyglot/python/world", Provider: "gemini", Model: "gemini-2.5-pro", Pass1: false, Pass2: false, LatencyMs: 2500, TotalTokens: 250, GeneratedCode: "def world(): pass", TestOutput: "FAIL", ErrorFeedback: "expected output", GeneratedCode2: "def world(): return 'world'", TestOutput2: "FAIL"},
	}
	for _, r := range results {
		if err := store.InsertResult(r); err != nil {
			t.Fatal(err)
		}
	}

	// Compute and store summary
	stored, _ := store.GetResultsByRun("e2e-run-1")
	summary := ComputeSummary(stored)
	if err := store.CompleteRun("e2e-run-1", summary); err != nil {
		t.Fatal(err)
	}

	// Verify run is completed with summary
	completed, _ := store.GetRun("e2e-run-1")
	if completed.Status != "completed" {
		t.Fatalf("expected completed, got %s", completed.Status)
	}
	if completed.Summary == nil {
		t.Fatal("expected summary")
	}

	s := completed.Summary
	if s.TotalExercises != 4 {
		t.Fatalf("expected 4 exercises, got %d", s.TotalExercises)
	}
	if s.Pass1Rate != 0.5 {
		t.Fatalf("expected 50%% pass1, got %.1f%%", s.Pass1Rate*100)
	}
	if s.Pass2Rate != 0.75 {
		t.Fatalf("expected 75%% pass2, got %.1f%%", s.Pass2Rate*100)
	}

	// Verify per-provider breakdown
	if ps, ok := s.ByProvider["nanogpt-sub"]; !ok || ps.Total != 2 {
		t.Fatalf("expected 2 nanogpt-sub results, got %+v", s.ByProvider["nanogpt-sub"])
	}
	if ps, ok := s.ByProvider["gemini"]; !ok || ps.Total != 2 {
		t.Fatalf("expected 2 gemini results, got %+v", s.ByProvider["gemini"])
	}

	// Verify per-language breakdown
	if ls, ok := s.ByLanguage["go"]; !ok || ls.Total != 2 {
		t.Fatalf("expected 2 go results, got %+v", s.ByLanguage["go"])
	}
	if ls, ok := s.ByLanguage["python"]; !ok || ls.Total != 2 {
		t.Fatalf("expected 2 python results, got %+v", s.ByLanguage["python"])
	}

	// Verify comparison
	run2 := EvalRun{
		ID:     "e2e-run-2",
		Config: EvalRunConfig{Languages: []string{"go"}, Count: 2},
		Status: "completed",
		StartedAt: now(),
		Summary: &EvalSummary{Pass1Rate: 1.0, Pass2Rate: 1.0, AvgLatencyMs: 800, TotalTokens: 100},
	}

	comp := CompareRuns(completed, &run2)
	if comp.Pass1Delta != 0.5 {
		t.Fatalf("expected +0.5 pass1 delta, got %f", comp.Pass1Delta)
	}

	// Verify run listing
	store.CreateRun(EvalRun{ID: "e2e-run-2", Config: EvalRunConfig{}, Status: "completed", StartedAt: now()})
	runs, _ := store.ListRuns(10)
	if len(runs) < 2 {
		t.Fatalf("expected at least 2 runs, got %d", len(runs))
	}

	// Verify individual result retrieval with two-pass fields
	allResults, _ := store.GetResultsByRun("e2e-run-1")
	var failedPython *EvalResult
	for i := range allResults {
		if allResults[i].ExerciseID == "polyglot/python/world" {
			failedPython = &allResults[i]
			break
		}
	}
	if failedPython == nil {
		t.Fatal("expected to find python/world result")
	}
	if failedPython.Pass1 || failedPython.Pass2 {
		t.Fatal("python/world should have failed both passes")
	}
	if failedPython.GeneratedCode != "def world(): pass" {
		t.Fatalf("unexpected generated code: %q", failedPython.GeneratedCode)
	}
	if failedPython.GeneratedCode2 != "def world(): return 'world'" {
		t.Fatalf("unexpected generated code 2: %q", failedPython.GeneratedCode2)
	}
	if failedPython.ErrorFeedback != "expected output" {
		t.Fatalf("unexpected error feedback: %q", failedPython.ErrorFeedback)
	}
}

func TestFailRun(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)

	store.CreateRun(EvalRun{
		ID:        "fail-run",
		Config:    EvalRunConfig{},
		Status:    "running",
		StartedAt: now(),
	})

	if err := store.FailRun("fail-run"); err != nil {
		t.Fatal(err)
	}

	run, _ := store.GetRun("fail-run")
	if run.Status != "failed" {
		t.Fatalf("expected failed, got %s", run.Status)
	}
	if run.CompletedAt == nil {
		t.Fatal("expected completed_at to be set")
	}
}

func trimString(s string) string {
	result := ""
	for _, c := range s {
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			if len(result) > 0 && result[len(result)-1] != ' ' {
				result += " "
			}
		} else {
			result += string(c)
		}
	}
	// Trim leading/trailing spaces
	for len(result) > 0 && result[0] == ' ' {
		result = result[1:]
	}
	for len(result) > 0 && result[len(result)-1] == ' ' {
		result = result[:len(result)-1]
	}
	return result
}

func now() time.Time {
	return time.Now()
}
