package eval

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestImportPolyglot(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)

	// Create a minimal polyglot-benchmark repo structure
	repoDir := t.TempDir()

	// Go exercise
	goExDir := filepath.Join(repoDir, "go", "exercises", "practice", "hello-world")
	if err := os.MkdirAll(filepath.Join(goExDir, ".docs"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(goExDir, ".docs", "instructions.md"), []byte("# Hello World\nReturn Hello, World!"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(goExDir, "hello_world_test.go"), []byte("package greeting\n\nimport \"testing\"\n\nfunc TestHello(t *testing.T) {}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(goExDir, "hello_world.go"), []byte("package greeting\n\nfunc Hello() string { return \"\" }"), 0644); err != nil {
		t.Fatal(err)
	}

	// Python exercise
	pyExDir := filepath.Join(repoDir, "python", "exercises", "practice", "two-fer")
	if err := os.MkdirAll(filepath.Join(pyExDir, ".docs"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pyExDir, ".docs", "instructions.md"), []byte("# Two Fer\nReturn One for X"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pyExDir, "two_fer_test.py"), []byte("def test_no_name(): pass"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pyExDir, "two_fer.py"), []byte("def two_fer(): pass"), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := ImportPolyglot(store, repoDir)
	if err != nil {
		t.Fatal(err)
	}

	if result.Suite != "polyglot" {
		t.Fatalf("expected polyglot, got %s", result.Suite)
	}
	if result.Imported != 2 {
		t.Fatalf("expected 2 imported, got %d", result.Imported)
	}

	// Verify exercises exist in store
	ex, err := store.GetExercise("polyglot/go/hello-world")
	if err != nil {
		t.Fatal(err)
	}
	if ex == nil {
		t.Fatal("expected go exercise")
	}
	if ex.DockerImage != "golang:1.22" {
		t.Fatalf("expected golang:1.22, got %s", ex.DockerImage)
	}

	ex, _ = store.GetExercise("polyglot/python/two-fer")
	if ex == nil {
		t.Fatal("expected python exercise")
	}
}

func TestImportRooCode(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)

	repoDir := t.TempDir()

	// Go exercise under exercises/go/slug
	goExDir := filepath.Join(repoDir, "exercises", "go", "hello-world")
	if err := os.MkdirAll(goExDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(goExDir, "README.md"), []byte("# Hello World"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(goExDir, "hello_world_test.go"), []byte("package greeting\n\nfunc TestHello() {}"), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := ImportRooCode(store, repoDir)
	if err != nil {
		t.Fatal(err)
	}

	if result.Imported != 1 {
		t.Fatalf("expected 1 imported, got %d (skipped=%d, errors=%d)", result.Imported, result.Skipped, result.Errors)
	}

	ex, _ := store.GetExercise("roocode/go/hello-world")
	if ex == nil {
		t.Fatal("expected exercise")
	}
}

func TestImportSkipsMissingInstructions(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)

	repoDir := t.TempDir()
	exerciseDir := filepath.Join(repoDir, "go", "exercises", "practice", "no-docs")
	if err := os.MkdirAll(exerciseDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(exerciseDir, "no_docs_test.go"), []byte("package x\nfunc TestX() {}"), 0644); err != nil {
		t.Fatal(err)
	}

	result, _ := ImportPolyglot(store, repoDir)
	if result.Imported != 0 {
		t.Fatalf("expected 0 imported, got %d", result.Imported)
	}
	if result.Skipped != 1 {
		t.Fatalf("expected 1 skipped, got %d", result.Skipped)
	}
}

func TestNameConversions(t *testing.T) {
	tests := []struct {
		slug   string
		snake  string
		camel  string
		pascal string
	}{
		{"hello-world", "hello_world", "helloWorld", "HelloWorld"},
		{"two-fer", "two_fer", "twoFer", "TwoFer"},
		{"simple", "simple", "simple", "Simple"},
	}

	for _, tt := range tests {
		if got := toSnake(tt.slug); got != tt.snake {
			t.Errorf("toSnake(%q) = %q, want %q", tt.slug, got, tt.snake)
		}
		if got := toCamel(tt.slug); got != tt.camel {
			t.Errorf("toCamel(%q) = %q, want %q", tt.slug, got, tt.camel)
		}
		if got := toPascal(tt.slug); got != tt.pascal {
			t.Errorf("toPascal(%q) = %q, want %q", tt.slug, got, tt.pascal)
		}
	}
}

func TestImportExercism(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)

	repoDir := t.TempDir()

	// Exercism Go track structure: exercises/practice/{slug}/.docs/instructions.md
	slugs := []string{"hello-world", "two-fer", "leap"}
	for _, slug := range slugs {
		exDir := filepath.Join(repoDir, "exercises", "practice", slug)
		if err := os.MkdirAll(filepath.Join(exDir, ".docs"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(exDir, ".docs", "instructions.md"),
			[]byte("# "+slug+"\nSolve this exercise."), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(exDir, toSnake(slug)+"_test.go"),
			[]byte("package "+toSnake(slug)+"\n\nimport \"testing\"\n\nfunc TestSolution(t *testing.T) {}"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(exDir, toSnake(slug)+".go"),
			[]byte("package "+toSnake(slug)+"\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	result, err := ImportExercism(store, repoDir, "go")
	if err != nil {
		t.Fatal(err)
	}

	if result.Suite != "exercism" {
		t.Fatalf("expected exercism, got %s", result.Suite)
	}
	if result.Imported != 3 {
		t.Fatalf("expected 3 imported, got %d (skipped=%d, errors=%d)", result.Imported, result.Skipped, result.Errors)
	}

	ex, _ := store.GetExercise("exercism/go/hello-world")
	if ex == nil {
		t.Fatal("expected exercism/go/hello-world")
	}
	if ex.Language != "go" {
		t.Fatalf("expected go, got %s", ex.Language)
	}
	if ex.DockerImage != "golang:1.22" {
		t.Fatalf("expected golang:1.22, got %s", ex.DockerImage)
	}
}

func TestImportExercismRequiresLanguage(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)

	_, err := ImportExercism(store, t.TempDir(), "")
	if err == nil {
		t.Fatal("expected error for empty language")
	}
}

func TestImportExercismUnsupportedLanguage(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)

	_, err := ImportExercism(store, t.TempDir(), "haskell")
	if err == nil {
		t.Fatal("expected error for unsupported language")
	}
}

func TestImportMultiPLE(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)

	repoDir := t.TempDir()

	// Create JSONL file with MultiPL-E format
	dataDir := filepath.Join(repoDir, "data")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatal(err)
	}

	entries := []multipleLangEntry{
		{Name: "HumanEval_0", Prompt: "def has_close_elements(numbers, threshold):\n    \"\"\"Check if any two numbers are closer than threshold.\"\"\"", Tests: "def test_has_close():\n    assert has_close_elements([1.0, 2.0], 0.5) == False", EntryPoint: "has_close_elements"},
		{Name: "HumanEval_1", Prompt: "def separate_paren_groups(paren_string):\n    \"\"\"Separate parentheses groups.\"\"\"", Tests: "def test_separate():\n    assert separate_paren_groups('(())') == ['(())']", EntryPoint: "separate_paren_groups"},
	}

	f, err := os.Create(filepath.Join(dataDir, "py-humaneval.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		data, _ := json.Marshal(e)
		if _, err := f.Write(data); err != nil {
			t.Fatal(err)
		}
		if _, err := f.WriteString("\n"); err != nil {
			t.Fatal(err)
		}
	}
	f.Close()

	// Go JSONL
	goEntries := []multipleLangEntry{
		{Name: "HumanEval_0", Prompt: "// HasCloseElements checks if any two numbers are closer than threshold.", Tests: "func TestHasClose(t *testing.T) {\n\tif HasCloseElements([]float64{1.0, 2.0}, 0.5) {\n\t\tt.Error(\"should be false\")\n\t}\n}", EntryPoint: "HasCloseElements"},
	}

	f2, err := os.Create(filepath.Join(dataDir, "go-humaneval.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range goEntries {
		data, _ := json.Marshal(e)
		if _, err := f2.Write(data); err != nil {
			t.Fatal(err)
		}
		if _, err := f2.WriteString("\n"); err != nil {
			t.Fatal(err)
		}
	}
	f2.Close()

	result, err := ImportMultiPLE(store, repoDir)
	if err != nil {
		t.Fatal(err)
	}

	if result.Suite != "multiple" {
		t.Fatalf("expected multiple, got %s", result.Suite)
	}
	if result.Imported != 3 {
		t.Fatalf("expected 3 imported, got %d (skipped=%d, errors=%d)", result.Imported, result.Skipped, result.Errors)
	}

	// Verify Python exercises
	ex, _ := store.GetExercise("multiple/python/humaneval-0")
	if ex == nil {
		t.Fatal("expected multiple/python/humaneval-0")
	}
	if ex.Language != "python" {
		t.Fatalf("expected python, got %s", ex.Language)
	}

	// Verify Go exercise
	ex, _ = store.GetExercise("multiple/go/humaneval-0")
	if ex == nil {
		t.Fatal("expected multiple/go/humaneval-0")
	}
}

func TestImportMultiPLEDirectory(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)

	repoDir := t.TempDir()

	// Directory format: datasets/py-humaneval/*.jsonl
	dsDir := filepath.Join(repoDir, "datasets", "py-humaneval")
	if err := os.MkdirAll(dsDir, 0755); err != nil {
		t.Fatal(err)
	}

	entries := []multipleLangEntry{
		{Name: "HumanEval_5", Prompt: "def intersperse(numbers, delimiter):", Tests: "def test_intersperse():\n    assert intersperse([], 4) == []", EntryPoint: "intersperse"},
	}

	f, err := os.Create(filepath.Join(dsDir, "humaneval.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		data, _ := json.Marshal(e)
		if _, err := f.Write(data); err != nil {
			t.Fatal(err)
		}
		if _, err := f.WriteString("\n"); err != nil {
			t.Fatal(err)
		}
	}
	f.Close()

	result, err := ImportMultiPLE(store, repoDir)
	if err != nil {
		t.Fatal(err)
	}

	if result.Imported != 1 {
		t.Fatalf("expected 1 imported, got %d", result.Imported)
	}
}

func TestImportMultiPLENoFiles(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)

	_, err := ImportMultiPLE(store, t.TempDir())
	if err == nil {
		t.Fatal("expected error for missing JSONL files")
	}
}

func TestImportEvalPlus(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)

	repoDir := t.TempDir()
	dataDir := filepath.Join(repoDir, "data")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatal(err)
	}

	entries := []evalPlusEntry{
		{TaskID: "HumanEval/0", Prompt: "def has_close_elements(numbers, threshold):", Test: "def test_has_close():\n    assert has_close_elements([1.0, 2.0], 0.5) == False", EntryPoint: "has_close_elements"},
		{TaskID: "HumanEval/1", Prompt: "def separate_paren_groups(paren_string):", Test: "def test_separate():\n    assert separate_paren_groups('(())') == ['(())']", EntryPoint: "separate_paren_groups"},
		{TaskID: "MBPP/1", Prompt: "def add(a, b):", Test: "def test_add():\n    assert add(1, 2) == 3", EntryPoint: "add"},
	}

	f, err := os.Create(filepath.Join(dataDir, "humaneval_plus.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries[:2] {
		data, _ := json.Marshal(e)
		if _, err := f.Write(data); err != nil {
			t.Fatal(err)
		}
		if _, err := f.WriteString("\n"); err != nil {
			t.Fatal(err)
		}
	}
	f.Close()

	f2, err := os.Create(filepath.Join(dataDir, "mbpp_plus.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(entries[2])
	if _, err := f2.Write(data); err != nil {
		t.Fatal(err)
	}
	if _, err := f2.WriteString("\n"); err != nil {
		t.Fatal(err)
	}
	f2.Close()

	result, err := ImportEvalPlus(store, repoDir)
	if err != nil {
		t.Fatal(err)
	}

	if result.Suite != "evalplus" {
		t.Fatalf("expected evalplus, got %s", result.Suite)
	}
	if result.Imported != 3 {
		t.Fatalf("expected 3 imported, got %d (skipped=%d, errors=%d)", result.Imported, result.Skipped, result.Errors)
	}

	ex, _ := store.GetExercise("evalplus/python/humaneval-0")
	if ex == nil {
		t.Fatal("expected evalplus/python/humaneval-0")
	}
	if ex.Language != "python" {
		t.Fatalf("expected python, got %s", ex.Language)
	}

	ex, _ = store.GetExercise("evalplus/python/mbpp-1")
	if ex == nil {
		t.Fatal("expected evalplus/python/mbpp-1")
	}
}

func TestImportEvalPlusNoFiles(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)

	_, err := ImportEvalPlus(store, t.TempDir())
	if err == nil {
		t.Fatal("expected error for missing JSONL files")
	}
}

func TestImportCodeContests(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)

	repoDir := t.TempDir()
	dataDir := filepath.Join(repoDir, "data")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatal(err)
	}

	entries := []codeContestEntry{
		{
			Name:        "problem_alpha",
			Description: "Given two integers, find their sum.",
			PublicTests: struct {
				Input  []string `json:"input"`
				Output []string `json:"output"`
			}{
				Input:  []string{"1 2\n", "3 4\n"},
				Output: []string{"3\n", "7\n"},
			},
		},
	}

	f, err := os.Create(filepath.Join(dataDir, "train.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		data, _ := json.Marshal(e)
		if _, err := f.Write(data); err != nil {
			t.Fatal(err)
		}
		if _, err := f.WriteString("\n"); err != nil {
			t.Fatal(err)
		}
	}
	f.Close()

	result, err := ImportCodeContests(store, repoDir, 0)
	if err != nil {
		t.Fatal(err)
	}

	if result.Suite != "codecontests" {
		t.Fatalf("expected codecontests, got %s", result.Suite)
	}
	// Should import for multiple languages (python, go, java, cpp, rust, javascript)
	if result.Imported < 1 {
		t.Fatalf("expected at least 1 imported, got %d (skipped=%d, errors=%d)", result.Imported, result.Skipped, result.Errors)
	}

	// Check a specific language
	ex, _ := store.GetExercise("codecontests/python/problem-alpha")
	if ex == nil {
		t.Fatal("expected codecontests/python/problem-alpha")
	}
	if ex.Language != "python" {
		t.Fatalf("expected python, got %s", ex.Language)
	}
	// Instructions should contain the examples
	if ex.Instructions == "" {
		t.Fatal("expected non-empty instructions")
	}

	ex, _ = store.GetExercise("codecontests/go/problem-alpha")
	if ex == nil {
		t.Fatal("expected codecontests/go/problem-alpha")
	}
}

func TestImportCodeContestsMaxCount(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)

	repoDir := t.TempDir()
	dataDir := filepath.Join(repoDir, "data")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create multiple problems
	f, err := os.Create(filepath.Join(dataDir, "test.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		entry := codeContestEntry{
			Name:        "problem_" + string(rune('a'+i)),
			Description: "Problem " + string(rune('a'+i)),
			PublicTests: struct {
				Input  []string `json:"input"`
				Output []string `json:"output"`
			}{
				Input:  []string{"1\n"},
				Output: []string{"1\n"},
			},
		}
		data, _ := json.Marshal(entry)
		if _, err := f.Write(data); err != nil {
			t.Fatal(err)
		}
		if _, err := f.WriteString("\n"); err != nil {
			t.Fatal(err)
		}
	}
	f.Close()

	// Import with maxCount=12 (2 problems x 6 languages = 12)
	result, err := ImportCodeContests(store, repoDir, 12)
	if err != nil {
		t.Fatal(err)
	}

	if result.Imported > 12 {
		t.Fatalf("expected at most 12 imported (maxCount), got %d", result.Imported)
	}
}

func TestImportCodeContestsNoFiles(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)

	_, err := ImportCodeContests(store, t.TempDir(), 0)
	if err == nil {
		t.Fatal("expected error for missing JSONL files")
	}
}

func TestSanitizeSlug(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"HumanEval/0", "humaneval-0"},
		{"HumanEval_1", "humaneval-1"},
		{"MBPP/123", "mbpp-123"},
		{"problem alpha", "problem-alpha"},
		{"Hello.World", "hello-world"},
		{"", ""},
		{"a--b", "a-b"},
	}

	for _, tt := range tests {
		got := sanitizeSlug(tt.input)
		if got != tt.want {
			t.Errorf("sanitizeSlug(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestImportDS1000(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)

	repoDir := t.TempDir()
	dataDir := filepath.Join(repoDir, "data")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatal(err)
	}

	entries := []ds1000Entry{
		{
			Prompt:        "Given a DataFrame df and a list of indices, select the rows.",
			ReferenceCode: "result = df.iloc[indices]",
			CodeContext:   "import pandas as pd\ndef test_execution(code):\n    return 0\n",
			Metadata:      ds1000Metadata{ProblemID: 0, Library: "Pandas", TestCaseCnt: 1},
		},
		{
			Prompt:        "Compute the mean of a numpy array.",
			ReferenceCode: "result = np.mean(arr)",
			CodeContext:   "import numpy as np\ndef test_execution(code):\n    return 0\n",
			Metadata:      ds1000Metadata{ProblemID: 1, Library: "NumPy", TestCaseCnt: 1},
		},
	}

	f, err := os.Create(filepath.Join(dataDir, "ds1000.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		data, _ := json.Marshal(e)
		if _, err := f.Write(data); err != nil {
			t.Fatal(err)
		}
		if _, err := f.WriteString("\n"); err != nil {
			t.Fatal(err)
		}
	}
	f.Close()

	result, err := ImportDS1000(store, repoDir)
	if err != nil {
		t.Fatal(err)
	}

	if result.Suite != "ds1000" {
		t.Fatalf("expected ds1000, got %s", result.Suite)
	}
	if result.Imported != 2 {
		t.Fatalf("expected 2 imported, got %d (skipped=%d, errors=%d)", result.Imported, result.Skipped, result.Errors)
	}

	ex, _ := store.GetExercise("ds1000/python/pandas-0")
	if ex == nil {
		t.Fatal("expected ds1000/python/pandas-0")
	}
	if ex.Language != "python" {
		t.Fatalf("expected python, got %s", ex.Language)
	}
	if ex.EvalMode != "docker-test" {
		t.Fatalf("expected docker-test, got %s", ex.EvalMode)
	}
	if ex.ReferenceCode == "" {
		t.Fatal("expected reference code")
	}

	ex, _ = store.GetExercise("ds1000/python/numpy-1")
	if ex == nil {
		t.Fatal("expected ds1000/python/numpy-1")
	}
}

func TestImportDS1000NoFiles(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)

	_, err := ImportDS1000(store, t.TempDir())
	if err == nil {
		t.Fatal("expected error for missing JSONL files")
	}
}

func TestImportBIRDSQL(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)

	repoDir := t.TempDir()
	dataDir := filepath.Join(repoDir, "data")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatal(err)
	}

	entries := []birdSQLEntry{
		{
			QuestionID: 1,
			DBID:       "california_schools",
			Question:   "What is the highest eligible free rate for K-12?",
			Evidence:   "highest = MAX()",
			SQL:        "SELECT MAX(free_rate) FROM schools WHERE grade = 'K-12'",
			Schema:     "CREATE TABLE schools (id INTEGER, free_rate REAL, grade TEXT);",
			Difficulty: "simple",
		},
		{
			QuestionID: 2,
			DBID:       "financial",
			Question:   "How many accounts were opened in 1995?",
			SQL:        "SELECT COUNT(*) FROM accounts WHERE year = 1995",
			Schema:     "CREATE TABLE accounts (id INTEGER, year INTEGER);",
			Difficulty: "moderate",
		},
	}

	f, err := os.Create(filepath.Join(dataDir, "mini_dev_prompt.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		data, _ := json.Marshal(e)
		if _, err := f.Write(data); err != nil {
			t.Fatal(err)
		}
		if _, err := f.WriteString("\n"); err != nil {
			t.Fatal(err)
		}
	}
	f.Close()

	result, err := ImportBIRDSQL(store, repoDir)
	if err != nil {
		t.Fatal(err)
	}

	if result.Suite != "birdsql" {
		t.Fatalf("expected birdsql, got %s", result.Suite)
	}
	if result.Imported != 2 {
		t.Fatalf("expected 2 imported, got %d (skipped=%d, errors=%d)", result.Imported, result.Skipped, result.Errors)
	}

	ex, _ := store.GetExercise("birdsql/sql/california-schools-1")
	if ex == nil {
		t.Fatal("expected birdsql/sql/california-schools-1")
	}
	if ex.Language != "sql" {
		t.Fatalf("expected sql, got %s", ex.Language)
	}
	if ex.ReferenceCode == "" {
		t.Fatal("expected reference SQL")
	}
}

func TestImportDAREBench(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)

	repoDir := t.TempDir()
	evalDir := filepath.Join(repoDir, "data", "eval")
	if err := os.MkdirAll(evalDir, 0755); err != nil {
		t.Fatal(err)
	}

	entries := []dareBenchEntry{
		{
			FilePath:   "test_dataset_class",
			QuestionV1: "Train a classification model on the provided dataset.",
			Task:       "classification",
		},
		{
			FilePath:   "test_dataset_reg",
			QuestionV1: "Train a regression model to predict prices.",
			Task:       "regression",
		},
	}

	data, _ := json.Marshal(entries)
	if err := os.WriteFile(filepath.Join(evalDir, "question_list.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	result, err := ImportDAREBench(store, repoDir)
	if err != nil {
		t.Fatal(err)
	}

	if result.Suite != "dare-bench" {
		t.Fatalf("expected dare-bench, got %s", result.Suite)
	}
	if result.Imported != 2 {
		t.Fatalf("expected 2 imported, got %d (skipped=%d, errors=%d)", result.Imported, result.Skipped, result.Errors)
	}

	ex, _ := store.GetExercise("dare-bench/python/test-dataset-class")
	if ex == nil {
		t.Fatal("expected dare-bench/python/test-dataset-class")
	}
	if ex.EvalMode != "metric-compare" {
		t.Fatalf("expected metric-compare, got %s", ex.EvalMode)
	}
	if ex.Criteria == "" {
		t.Fatal("expected criteria with task_type and metric")
	}
}

func TestImportWritingBench(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)

	repoDir := t.TempDir()
	dataDir := filepath.Join(repoDir, "data")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatal(err)
	}

	entries := []writingBenchEntry{
		{
			Index:   1,
			Domain1: "Academic",
			Domain2: "Paper Outline",
			Lang:    "en",
			Query:   "Write a research paper outline on machine learning.",
			Checklist: []writingBenchCriteria{
				{Name: "Structure", CriteriaDescription: "Evaluate structure completeness."},
			},
		},
		{
			Index:   2,
			Domain1: "Marketing",
			Domain2: "Ad Copy",
			Lang:    "en",
			Query:   "Write an ad for a new product.",
		},
	}

	f, err := os.Create(filepath.Join(dataDir, "benchmark_all.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		data, _ := json.Marshal(e)
		if _, err := f.Write(data); err != nil {
			t.Fatal(err)
		}
		if _, err := f.WriteString("\n"); err != nil {
			t.Fatal(err)
		}
	}
	f.Close()

	result, err := ImportWritingBench(store, repoDir)
	if err != nil {
		t.Fatal(err)
	}

	if result.Suite != "writingbench" {
		t.Fatalf("expected writingbench, got %s", result.Suite)
	}
	if result.Imported != 2 {
		t.Fatalf("expected 2 imported, got %d (skipped=%d, errors=%d)", result.Imported, result.Skipped, result.Errors)
	}

	ex, _ := store.GetExercise("writingbench/text/academic-1")
	if ex == nil {
		t.Fatal("expected writingbench/text/academic-1")
	}
	if ex.Language != "text" {
		t.Fatalf("expected text, got %s", ex.Language)
	}
	if ex.EvalMode != "llm-judge" {
		t.Fatalf("expected llm-judge, got %s", ex.EvalMode)
	}
	if ex.Criteria == "" {
		t.Fatal("expected criteria")
	}
}

func TestImportPPTArena(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)

	repoDir := t.TempDir()
	dataDir := filepath.Join(repoDir, "data")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatal(err)
	}

	entries := []pptArenaEntry{
		{
			Name:        "AddSource_TestA",
			Prompt:      "Add a source citation to slide 3",
			Category:    "content",
			EditType:    "add",
			Original:    "AddSource_TestA.pptx",
			GroundTruth: "AddSource_TestA_GT.pptx",
		},
		{
			Name:        "FixTable_TestB",
			Prompt:      "Fix the table formatting on slide 2",
			Category:    "formatting",
			EditType:    "fix",
			Original:    "FixTable_TestB.pptx",
			GroundTruth: "FixTable_TestB_GT.pptx",
		},
	}

	data, _ := json.Marshal(entries)
	if err := os.WriteFile(filepath.Join(dataDir, "evaluation_pairs_refined.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	result, err := ImportPPTArena(store, repoDir)
	if err != nil {
		t.Fatal(err)
	}

	if result.Suite != "pptarena" {
		t.Fatalf("expected pptarena, got %s", result.Suite)
	}
	if result.Imported != 2 {
		t.Fatalf("expected 2 imported, got %d (skipped=%d, errors=%d)", result.Imported, result.Skipped, result.Errors)
	}

	ex, _ := store.GetExercise("pptarena/pptx/addsource-testa")
	if ex == nil {
		t.Fatal("expected pptarena/pptx/addsource-testa")
	}
	if ex.Language != "pptx" {
		t.Fatalf("expected pptx, got %s", ex.Language)
	}
	if ex.EvalMode != "vlm-judge" {
		t.Fatalf("expected vlm-judge, got %s", ex.EvalMode)
	}
}

func TestImportPPTArenaNoFiles(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)

	_, err := ImportPPTArena(store, t.TempDir())
	if err == nil {
		t.Fatal("expected error for missing JSON file")
	}
}

func TestMetricScoreInResults(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)

	result := EvalResult{
		ID:           "res-metric-1",
		RunID:        "run-1",
		ExerciseID:   "dare-bench/python/test",
		Provider:     "nanogpt-sub",
		Pass1:        true,
		MetricScore:  0.85,
		MetricName:   "macro_f1",
		LatencyMs:    500,
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
	if r.MetricScore != 0.85 {
		t.Fatalf("expected metric_score 0.85, got %f", r.MetricScore)
	}
	if r.MetricName != "macro_f1" {
		t.Fatalf("expected macro_f1, got %s", r.MetricName)
	}
}

func TestJudgeScoreInResults(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)

	result := EvalResult{
		ID:            "res-judge-1",
		RunID:         "run-2",
		ExerciseID:    "writingbench/text/test",
		Provider:      "nanogpt-sub",
		Pass1:         true,
		MetricScore:   7.5,
		MetricName:    "llm_judge_score",
		JudgeProvider: "gemini",
		LatencyMs:     800,
	}

	if err := store.InsertResult(result); err != nil {
		t.Fatal(err)
	}

	results, err := store.GetResultsByRun("run-2")
	if err != nil {
		t.Fatal(err)
	}

	r := results[0]
	if r.JudgeProvider != "gemini" {
		t.Fatalf("expected gemini judge, got %s", r.JudgeProvider)
	}
	if r.MetricScore != 7.5 {
		t.Fatalf("expected 7.5 score, got %f", r.MetricScore)
	}
}

func TestParseJudgeScore(t *testing.T) {
	tests := []struct {
		input string
		want  float64
	}{
		{"Great work!\n\nSCORE: 8", 8},
		{"SCORE: 7.5", 7.5},
		{"The response is good.\n\n7", 7},
		{"Nothing useful", 0},
		{"SCORE: 11", 0}, // out of range
	}

	for _, tt := range tests {
		got := parseJudgeScore(tt.input)
		if got != tt.want {
			t.Errorf("parseJudgeScore(%q) = %f, want %f", tt.input, got, tt.want)
		}
	}
}

func TestExtractMetricScore(t *testing.T) {
	tests := []struct {
		input string
		want  float64
	}{
		{"Training complete\nMETRIC_SCORE: 0.92", 0.92},
		{"SCORE: 0.75", 0.75},
		{"Some output\nMETRIC: 0.88\n", 0.88},
		{"No score here", 0},
	}

	for _, tt := range tests {
		got := extractMetricScore(tt.input)
		if got != tt.want {
			t.Errorf("extractMetricScore(%q) = %f, want %f", tt.input, got, tt.want)
		}
	}
}

func TestDeduplicationAcrossSuites(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)

	repoDir := t.TempDir()

	// Create a polyglot exercise
	goExDir := filepath.Join(repoDir, "go", "exercises", "practice", "hello-world")
	if err := os.MkdirAll(filepath.Join(goExDir, ".docs"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(goExDir, ".docs", "instructions.md"), []byte("# Hello World"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(goExDir, "hello_world_test.go"), []byte("package greeting\n\nfunc TestHello() {}"), 0644); err != nil {
		t.Fatal(err)
	}

	r1, _ := ImportPolyglot(store, repoDir)
	if r1.Imported != 1 {
		t.Fatalf("expected 1 polyglot imported, got %d", r1.Imported)
	}

	// Create an exercism exercise with the same slug
	exDir := t.TempDir()
	exSlugDir := filepath.Join(exDir, "exercises", "practice", "hello-world")
	if err := os.MkdirAll(filepath.Join(exSlugDir, ".docs"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(exSlugDir, ".docs", "instructions.md"), []byte("# Hello World (exercism)"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(exSlugDir, "hello_world_test.go"), []byte("package greeting\n\nfunc TestHello() {}"), 0644); err != nil {
		t.Fatal(err)
	}

	r2, _ := ImportExercism(store, exDir, "go")
	if r2.Imported != 1 {
		t.Fatalf("expected 1 exercism imported, got %d", r2.Imported)
	}

	// Both should exist with unique IDs
	p, _ := store.GetExercise("polyglot/go/hello-world")
	e, _ := store.GetExercise("exercism/go/hello-world")
	if p == nil || e == nil {
		t.Fatal("both exercises should exist with unique IDs")
	}
	if p.Suite != "polyglot" || e.Suite != "exercism" {
		t.Fatal("suites should differ")
	}

	// Total count should be 2
	count, _ := store.CountExercises("", "go")
	if count != 2 {
		t.Fatalf("expected 2 total go exercises, got %d", count)
	}
}
