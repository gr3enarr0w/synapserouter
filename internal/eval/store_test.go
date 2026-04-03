package eval

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	schema := `
	CREATE TABLE eval_exercises (
		id TEXT PRIMARY KEY,
		suite TEXT NOT NULL,
		language TEXT NOT NULL,
		slug TEXT NOT NULL,
		instructions TEXT NOT NULL,
		stub TEXT,
		test_file TEXT NOT NULL,
		test_command TEXT NOT NULL,
		docker_image TEXT NOT NULL,
		eval_mode TEXT DEFAULT 'docker-test',
		reference_code TEXT,
		criteria TEXT,
		imported_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE eval_runs (
		id TEXT PRIMARY KEY,
		config TEXT NOT NULL,
		status TEXT NOT NULL,
		started_at DATETIME NOT NULL,
		completed_at DATETIME,
		summary TEXT
	);
	CREATE TABLE eval_results (
		id TEXT PRIMARY KEY,
		run_id TEXT NOT NULL,
		exercise_id TEXT NOT NULL,
		provider TEXT NOT NULL,
		model TEXT,
		pass_1 INTEGER DEFAULT 0,
		pass_2 INTEGER DEFAULT 0,
		generated_code TEXT,
		test_output TEXT,
		error_feedback TEXT,
		generated_code_2 TEXT,
		test_output_2 TEXT,
		latency_ms INTEGER,
		latency_ms_2 INTEGER,
		prompt_tokens INTEGER DEFAULT 0,
		completion_tokens INTEGER DEFAULT 0,
		total_tokens INTEGER DEFAULT 0,
		fallback_used INTEGER DEFAULT 0,
		fallback_chain TEXT,
		docker_exit_code INTEGER,
		metric_score REAL,
		metric_name TEXT,
		judge_provider TEXT,
		error TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`
	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestUpsertAndGetExercise(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)

	ex := Exercise{
		ID:          "polyglot/go/hello-world",
		Suite:       "polyglot",
		Language:    "go",
		Slug:        "hello-world",
		Instructions: "Return 'Hello, World!'",
		Stub:        "package greeting\n\nfunc HelloWorld() string {\n\treturn \"\"\n}",
		TestFile:    "package greeting\n\nimport \"testing\"\n\nfunc TestHelloWorld(t *testing.T) {}",
		TestCommand: "go test -v ./...",
		DockerImage: "golang:1.22",
	}

	if err := store.UpsertExercise(ex); err != nil {
		t.Fatal(err)
	}

	got, err := store.GetExercise("polyglot/go/hello-world")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected exercise, got nil")
	}
	if got.Suite != "polyglot" || got.Language != "go" || got.Slug != "hello-world" {
		t.Fatalf("unexpected exercise: %+v", got)
	}

	// Upsert should replace
	ex.Instructions = "Updated instructions"
	if err := store.UpsertExercise(ex); err != nil {
		t.Fatal(err)
	}
	got, _ = store.GetExercise("polyglot/go/hello-world")
	if got.Instructions != "Updated instructions" {
		t.Fatalf("expected updated instructions, got %q", got.Instructions)
	}
}

func TestListExercises(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)

	exercises := []Exercise{
		{ID: "polyglot/go/hello", Suite: "polyglot", Language: "go", Slug: "hello", Instructions: "hi", TestFile: "t", TestCommand: "go test", DockerImage: "golang:1.22"},
		{ID: "polyglot/go/world", Suite: "polyglot", Language: "go", Slug: "world", Instructions: "hi", TestFile: "t", TestCommand: "go test", DockerImage: "golang:1.22"},
		{ID: "polyglot/python/hello", Suite: "polyglot", Language: "python", Slug: "hello", Instructions: "hi", TestFile: "t", TestCommand: "pytest", DockerImage: "python:3.12"},
		{ID: "roocode/go/hello", Suite: "roocode", Language: "go", Slug: "hello", Instructions: "hi", TestFile: "t", TestCommand: "go test", DockerImage: "golang:1.22"},
	}
	for _, ex := range exercises {
		if err := store.UpsertExercise(ex); err != nil {
			t.Fatal(err)
		}
	}

	// All exercises
	all, err := store.ListExercises("", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 4 {
		t.Fatalf("expected 4 exercises, got %d", len(all))
	}

	// Filter by suite
	polyglot, err := store.ListExercises("polyglot", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(polyglot) != 3 {
		t.Fatalf("expected 3 polyglot exercises, got %d", len(polyglot))
	}

	// Filter by language
	goExercises, err := store.ListExercises("", "go")
	if err != nil {
		t.Fatal(err)
	}
	if len(goExercises) != 3 {
		t.Fatalf("expected 3 go exercises, got %d", len(goExercises))
	}

	// Filter by both
	polyglotGo, err := store.ListExercises("polyglot", "go")
	if err != nil {
		t.Fatal(err)
	}
	if len(polyglotGo) != 2 {
		t.Fatalf("expected 2 polyglot/go exercises, got %d", len(polyglotGo))
	}
}

func TestCountExercises(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)

	if err := store.UpsertExercise(Exercise{ID: "a/go/1", Suite: "a", Language: "go", Slug: "1", Instructions: "x", TestFile: "t", TestCommand: "go test", DockerImage: "golang:1.22"}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertExercise(Exercise{ID: "a/py/1", Suite: "a", Language: "python", Slug: "1", Instructions: "x", TestFile: "t", TestCommand: "pytest", DockerImage: "python:3.12"}); err != nil {
		t.Fatal(err)
	}

	count, err := store.CountExercises("", "")
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("expected 2, got %d", count)
	}

	count, _ = store.CountExercises("a", "go")
	if count != 1 {
		t.Fatalf("expected 1, got %d", count)
	}
}

func TestRunLifecycle(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)

	run := EvalRun{
		ID:     "run-1",
		Config: EvalRunConfig{Languages: []string{"go"}, Count: 5, TwoPass: true},
		Status: "running",
		StartedAt: time.Now(),
	}

	if err := store.CreateRun(run); err != nil {
		t.Fatal(err)
	}

	got, err := store.GetRun("run-1")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected run")
	}
	if got.Status != "running" {
		t.Fatalf("expected running, got %s", got.Status)
	}
	if len(got.Config.Languages) != 1 || got.Config.Languages[0] != "go" {
		t.Fatalf("unexpected config: %+v", got.Config)
	}

	summary := &EvalSummary{
		TotalExercises: 5,
		Pass1Rate:      0.6,
		Pass2Rate:      0.8,
	}
	if err := store.CompleteRun("run-1", summary); err != nil {
		t.Fatal(err)
	}

	got, _ = store.GetRun("run-1")
	if got.Status != "completed" {
		t.Fatalf("expected completed, got %s", got.Status)
	}
	if got.Summary == nil || got.Summary.Pass1Rate != 0.6 {
		t.Fatalf("unexpected summary: %+v", got.Summary)
	}
}

func TestListRuns(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)

	for i := 0; i < 3; i++ {
		if err := store.CreateRun(EvalRun{
			ID:        fmt.Sprintf("run-%d", i),
			Config:    EvalRunConfig{Count: i},
			Status:    "completed",
			StartedAt: time.Now().Add(time.Duration(i) * time.Second),
		}); err != nil {
			t.Fatal(err)
		}
	}

	runs, err := store.ListRuns(2)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 {
		t.Fatalf("expected 2 runs, got %d", len(runs))
	}
	// Most recent first
	if runs[0].ID != "run-2" {
		t.Fatalf("expected run-2 first, got %s", runs[0].ID)
	}
}

func TestInsertAndGetResults(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)

	result := EvalResult{
		ID:            "res-1",
		RunID:         "run-1",
		ExerciseID:    "polyglot/go/hello",
		Provider:      "nanogpt-sub",
		Model:         "qwen/qwen3.5-plus",
		Pass1:         true,
		Pass2:         false,
		GeneratedCode: "package main\nfunc Hello() string { return \"hello\" }",
		TestOutput:    "PASS",
		LatencyMs:     1500,
		PromptTokens:  100,
		CompletionTokens: 200,
		TotalTokens:   300,
		FallbackUsed:  true,
		FallbackChain: []string{"nanogpt-sub", "gemini"},
	}

	if err := store.InsertResult(result); err != nil {
		t.Fatal(err)
	}

	results, err := store.GetResultsByRun("run-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	r := results[0]
	if !r.Pass1 {
		t.Fatal("expected pass_1 = true")
	}
	if r.Pass2 {
		t.Fatal("expected pass_2 = false")
	}
	if r.Provider != "nanogpt-sub" {
		t.Fatalf("expected nanogpt-sub, got %s", r.Provider)
	}
	if !r.FallbackUsed {
		t.Fatal("expected fallback_used = true")
	}
	if len(r.FallbackChain) != 2 || r.FallbackChain[0] != "nanogpt-sub" {
		t.Fatalf("unexpected fallback chain: %v", r.FallbackChain)
	}
}

func TestGetNonExistent(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)

	ex, err := store.GetExercise("nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if ex != nil {
		t.Fatal("expected nil")
	}

	run, err := store.GetRun("nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if run != nil {
		t.Fatal("expected nil")
	}
}
