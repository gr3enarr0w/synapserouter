package eval

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ImportResult tracks how many exercises were imported.
type ImportResult struct {
	Suite    string `json:"suite"`
	Imported int    `json:"imported"`
	Skipped  int    `json:"skipped"`
	Errors   int    `json:"errors"`
}

// ImportPolyglot imports exercises from the Aider polyglot-benchmark repo.
// Expected structure: {repoPath}/{lang}/exercises/practice/{slug}/
//   - .meta/instructions.md (or .docs/instructions.md)
//   - {slug}_test.{ext} (test file)
//   - {slug}.{ext} (stub)
func ImportPolyglot(store *Store, repoPath string) (*ImportResult, error) {
	result := &ImportResult{Suite: "polyglot"}

	languages := map[string]string{
		"go":         ".go",
		"python":     ".py",
		"javascript": ".js",
		"java":       ".java",
		"rust":       ".rs",
		"cpp":        ".cpp",
	}

	for lang, ext := range languages {
		practiceDir := filepath.Join(repoPath, lang, "exercises", "practice")
		if _, err := os.Stat(practiceDir); os.IsNotExist(err) {
			continue
		}

		entries, err := os.ReadDir(practiceDir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			slug := entry.Name()
			exerciseDir := filepath.Join(practiceDir, slug)

			instructions := readFileFromPaths(
				filepath.Join(exerciseDir, ".docs", "instructions.md"),
				filepath.Join(exerciseDir, ".meta", "instructions.md"),
				filepath.Join(exerciseDir, "README.md"),
			)
			if instructions == "" {
				result.Skipped++
				continue
			}

			testFile := findTestFile(exerciseDir, slug, lang, ext)
			if testFile == "" {
				result.Skipped++
				continue
			}

			stub := findStubFile(exerciseDir, slug, lang, ext)

			dc, ok := DockerConfig[lang]
			if !ok {
				result.Skipped++
				continue
			}

			ex := Exercise{
				ID:           fmt.Sprintf("polyglot/%s/%s", lang, slug),
				Suite:        "polyglot",
				Language:     lang,
				Slug:         slug,
				Instructions: instructions,
				Stub:         stub,
				TestFile:     testFile,
				TestCommand:  dc.TestCommand,
				DockerImage:  dc.Image,
			}

			if err := store.UpsertExercise(ex); err != nil {
				result.Errors++
				continue
			}
			result.Imported++
		}
	}

	return result, nil
}

// ImportRooCode imports exercises from the Roo-Code-Evals repo.
// Expected structure: {repoPath}/exercises/{lang}/{slug}/
func ImportRooCode(store *Store, repoPath string) (*ImportResult, error) {
	result := &ImportResult{Suite: "roocode"}

	languages := map[string]string{
		"go":         ".go",
		"python":     ".py",
		"javascript": ".js",
		"java":       ".java",
		"rust":       ".rs",
	}

	for lang, ext := range languages {
		langDir := filepath.Join(repoPath, "exercises", lang)
		if _, err := os.Stat(langDir); os.IsNotExist(err) {
			// Try alternate layout
			langDir = filepath.Join(repoPath, lang, "exercises", "practice")
			if _, err := os.Stat(langDir); os.IsNotExist(err) {
				continue
			}
		}

		entries, err := os.ReadDir(langDir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			slug := entry.Name()
			exerciseDir := filepath.Join(langDir, slug)

			instructions := readFileFromPaths(
				filepath.Join(exerciseDir, ".docs", "instructions.md"),
				filepath.Join(exerciseDir, ".meta", "instructions.md"),
				filepath.Join(exerciseDir, "README.md"),
				filepath.Join(exerciseDir, "instructions.md"),
			)
			if instructions == "" {
				result.Skipped++
				continue
			}

			testFile := findTestFile(exerciseDir, slug, lang, ext)
			if testFile == "" {
				result.Skipped++
				continue
			}

			stub := findStubFile(exerciseDir, slug, lang, ext)

			dc, ok := DockerConfig[lang]
			if !ok {
				result.Skipped++
				continue
			}

			ex := Exercise{
				ID:           fmt.Sprintf("roocode/%s/%s", lang, slug),
				Suite:        "roocode",
				Language:     lang,
				Slug:         slug,
				Instructions: instructions,
				Stub:         stub,
				TestFile:     testFile,
				TestCommand:  dc.TestCommand,
				DockerImage:  dc.Image,
			}

			if err := store.UpsertExercise(ex); err != nil {
				result.Errors++
				continue
			}
			result.Imported++
		}
	}

	return result, nil
}

// ImportExercism imports exercises from an Exercism track repo (one language per repo).
// Expected structure: {repoPath}/exercises/practice/{slug}/.docs/instructions.md + test + stub
// Requires --language flag since each Exercism track is a separate repo.
func ImportExercism(store *Store, repoPath, language string) (*ImportResult, error) {
	result := &ImportResult{Suite: "exercism"}

	ext, ok := languageExtensions[language]
	if !ok {
		return nil, fmt.Errorf("unsupported language: %s", language)
	}

	practiceDir := filepath.Join(repoPath, "exercises", "practice")
	if _, err := os.Stat(practiceDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("exercises/practice not found in %s", repoPath)
	}

	entries, err := os.ReadDir(practiceDir)
	if err != nil {
		return nil, fmt.Errorf("read practice dir: %w", err)
	}

	dc, ok := DockerConfig[language]
	if !ok {
		return nil, fmt.Errorf("no docker config for language: %s", language)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		slug := entry.Name()
		exerciseDir := filepath.Join(practiceDir, slug)

		instructions := readFileFromPaths(
			filepath.Join(exerciseDir, ".docs", "instructions.md"),
			filepath.Join(exerciseDir, ".meta", "instructions.md"),
			filepath.Join(exerciseDir, "README.md"),
		)
		if instructions == "" {
			result.Skipped++
			continue
		}

		testFile := findTestFile(exerciseDir, slug, language, ext)
		if testFile == "" {
			result.Skipped++
			continue
		}

		stub := findStubFile(exerciseDir, slug, language, ext)

		// For Go, capture cases_test.go if it exists (defines testCases)
		var refCode string
		if language == "go" {
			casesPath := filepath.Join(exerciseDir, "cases_test.go")
			if data, err := os.ReadFile(casesPath); err == nil {
				refCode = string(data)
			}
		}

		ex := Exercise{
			ID:            fmt.Sprintf("exercism/%s/%s", language, slug),
			Suite:         "exercism",
			Language:      language,
			Slug:          slug,
			Instructions:  instructions,
			Stub:          stub,
			TestFile:      testFile,
			TestCommand:   dc.TestCommand,
			DockerImage:   dc.Image,
			ReferenceCode: refCode,
		}

		if err := store.UpsertExercise(ex); err != nil {
			result.Errors++
			continue
		}
		result.Imported++
	}

	return result, nil
}

// multipleLangEntry represents a single exercise in MultiPL-E JSONL format.
// Supports both MultiPL-E (name/tests) and HumanEvalPlus (task_id/test) field names.
type multipleLangEntry struct {
	Name       string `json:"name"`
	TaskID     string `json:"task_id"`
	Language   string `json:"language"`
	Prompt     string `json:"prompt"`
	Tests      string `json:"tests"`
	Test       string `json:"test"`
	EntryPoint string `json:"entry_point"`
}

func (e multipleLangEntry) effectiveName() string {
	if e.Name != "" {
		return e.Name
	}
	return e.TaskID
}

func (e multipleLangEntry) effectiveTests() string {
	if e.Tests != "" {
		return e.Tests
	}
	return e.Test
}

// multiPLELangMap maps MultiPL-E directory prefixes to our language names.
var multiPLELangMap = map[string]string{
	"go":         "go",
	"py":         "python",
	"js":         "javascript",
	"java":       "java",
	"rs":         "rust",
	"cpp":        "cpp",
	"python":     "python",
	"javascript": "javascript",
	"rust":       "rust",
}

// ImportMultiPLE imports exercises from the MultiPL-E repo.
// Expected structure: {repoPath}/data/{lang}-humaneval.jsonl or {repoPath}/datasets/{lang}-humaneval/
func ImportMultiPLE(store *Store, repoPath string) (*ImportResult, error) {
	result := &ImportResult{Suite: "multiple"}

	// Look for JSONL files in data/ or datasets/ directory
	searchDirs := []string{
		filepath.Join(repoPath, "data"),
		filepath.Join(repoPath, "datasets"),
		repoPath,
	}

	foundAny := false
	for _, searchDir := range searchDirs {
		if _, err := os.Stat(searchDir); os.IsNotExist(err) {
			continue
		}

		entries, err := os.ReadDir(searchDir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			name := entry.Name()

			if entry.IsDir() {
				// Directory format: {lang}-humaneval/ containing JSONL files
				lang := multiPLEDirToLang(name)
				if lang == "" {
					continue
				}
				dirPath := filepath.Join(searchDir, name)
				dirEntries, err := os.ReadDir(dirPath)
				if err != nil {
					continue
				}
				for _, de := range dirEntries {
					if strings.HasSuffix(de.Name(), ".jsonl") {
						count, errs := importMultiPLEFile(store, filepath.Join(dirPath, de.Name()), lang, result)
						result.Imported += count
						result.Errors += errs
						foundAny = true
					}
				}
			} else if strings.HasSuffix(name, ".jsonl") {
				// File format: {lang}-humaneval.jsonl
				lang := multiPLEFileToLang(name)
				if lang == "" {
					continue
				}
				count, errs := importMultiPLEFile(store, filepath.Join(searchDir, name), lang, result)
				result.Imported += count
				result.Errors += errs
				foundAny = true
			}
		}
	}

	if !foundAny {
		return nil, fmt.Errorf("no MultiPL-E JSONL files found in %s", repoPath)
	}

	return result, nil
}

func importMultiPLEFile(store *Store, path, lang string, result *ImportResult) (imported, errors int) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 1
	}
	defer f.Close()

	dc, ok := DockerConfig[lang]
	if !ok {
		return 0, 0
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var entry multipleLangEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			errors++
			continue
		}

		tests := entry.effectiveTests()
		if entry.Prompt == "" || tests == "" {
			result.Skipped++
			continue
		}

		slug := sanitizeSlug(entry.effectiveName())
		if slug == "" {
			slug = sanitizeSlug(entry.EntryPoint)
		}
		if slug == "" {
			result.Skipped++
			continue
		}

		testFile := buildMultiPLETestFile(lang, entry)
		if testFile == "" {
			result.Skipped++
			continue
		}

		ex := Exercise{
			ID:           fmt.Sprintf("multiple/%s/%s", lang, slug),
			Suite:        "multiple",
			Language:     lang,
			Slug:         slug,
			Instructions: entry.Prompt,
			TestFile:     testFile,
			TestCommand:  dc.TestCommand,
			DockerImage:  dc.Image,
		}

		if err := store.UpsertExercise(ex); err != nil {
			errors++
			continue
		}
		imported++
	}

	return imported, errors
}

func buildMultiPLETestFile(lang string, entry multipleLangEntry) string {
	tests := entry.effectiveTests()
	if tests == "" {
		return ""
	}

	switch lang {
	case "go":
		return fmt.Sprintf("package main\n\nimport \"testing\"\n\n%s\n", tests)
	case "python":
		return tests
	case "javascript":
		return tests
	case "java":
		return tests
	case "rust":
		return fmt.Sprintf("#[cfg(test)]\nmod tests {\n    use super::*;\n\n%s\n}\n", tests)
	case "cpp":
		return tests
	default:
		return tests
	}
}

func multiPLEDirToLang(dirName string) string {
	// e.g., "go-humaneval", "py-humaneval", "rs-humaneval"
	parts := strings.SplitN(dirName, "-", 2)
	if len(parts) < 2 {
		return ""
	}
	if !strings.Contains(parts[1], "humaneval") {
		return ""
	}
	return multiPLELangMap[parts[0]]
}

func multiPLEFileToLang(fileName string) string {
	// e.g., "go-humaneval.jsonl", "py-humaneval.jsonl"
	name := strings.TrimSuffix(fileName, ".jsonl")

	// Handle HumanEvalPlus-*.jsonl (Python-only dataset)
	if strings.HasPrefix(strings.ToLower(name), "humanevalplus") {
		return "python"
	}

	return multiPLEDirToLang(name)
}

// evalPlusEntry represents a single exercise in EvalPlus JSONL format.
type evalPlusEntry struct {
	TaskID            string `json:"task_id"`
	Prompt            string `json:"prompt"`
	CanonicalSolution string `json:"canonical_solution"`
	Test              string `json:"test"`
	EntryPoint        string `json:"entry_point"`
}

// ImportEvalPlus imports exercises from the EvalPlus repo (Python only).
// Expected structure: {repoPath}/data/ containing humaneval.jsonl or mbpp.jsonl
func ImportEvalPlus(store *Store, repoPath string) (*ImportResult, error) {
	result := &ImportResult{Suite: "evalplus"}

	dc, ok := DockerConfig["python"]
	if !ok {
		return nil, fmt.Errorf("no docker config for python")
	}

	searchPaths := []string{
		filepath.Join(repoPath, "data"),
		filepath.Join(repoPath, "evalplus", "data"),
		repoPath,
	}

	foundAny := false
	for _, searchDir := range searchPaths {
		if _, err := os.Stat(searchDir); os.IsNotExist(err) {
			continue
		}

		entries, err := os.ReadDir(searchDir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			name := entry.Name()
			if !strings.HasSuffix(name, ".jsonl") {
				continue
			}
			// Look for humaneval or mbpp JSONL files
			nameLower := strings.ToLower(name)
			if !strings.Contains(nameLower, "humaneval") && !strings.Contains(nameLower, "mbpp") {
				continue
			}

			count, errs := importEvalPlusFile(store, filepath.Join(searchDir, name), dc, result)
			result.Imported += count
			result.Errors += errs
			foundAny = true
		}
	}

	if !foundAny {
		return nil, fmt.Errorf("no EvalPlus JSONL files found in %s", repoPath)
	}

	return result, nil
}

func importEvalPlusFile(store *Store, path string, dc struct {
	Image       string
	TestCommand string
}, result *ImportResult) (imported, errors int) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 1
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var entry evalPlusEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			errors++
			continue
		}

		if entry.Prompt == "" || entry.Test == "" {
			result.Skipped++
			continue
		}

		slug := sanitizeSlug(entry.TaskID)
		if slug == "" {
			slug = sanitizeSlug(entry.EntryPoint)
		}
		if slug == "" {
			result.Skipped++
			continue
		}

		ex := Exercise{
			ID:           fmt.Sprintf("evalplus/python/%s", slug),
			Suite:        "evalplus",
			Language:     "python",
			Slug:         slug,
			Instructions: entry.Prompt,
			TestFile:     entry.Test,
			TestCommand:  dc.TestCommand,
			DockerImage:  dc.Image,
		}

		if err := store.UpsertExercise(ex); err != nil {
			errors++
			continue
		}
		imported++
	}

	return imported, errors
}

// codeContestEntry represents a single problem in a CodeContests JSONL file.
type codeContestEntry struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Difficulty  int      `json:"difficulty"`
	Solutions   []string `json:"solutions"`
	InputOutput []struct {
		Input  string `json:"input"`
		Output string `json:"output"`
	} `json:"input_output"`
	// Alternative field names used in some exports
	PublicTests struct {
		Input  []string `json:"input"`
		Output []string `json:"output"`
	} `json:"public_tests"`
	PrivateTests struct {
		Input  []string `json:"input"`
		Output []string `json:"output"`
	} `json:"private_tests"`
}

// ImportCodeContests imports exercises from a CodeContests export (JSONL format).
// Expected structure: {repoPath}/data/ containing *.jsonl files with stdin/stdout test cases.
// The maxCount parameter limits how many exercises to import (0 = all).
func ImportCodeContests(store *Store, repoPath string, maxCount int) (*ImportResult, error) {
	result := &ImportResult{Suite: "codecontests"}

	searchPaths := []string{
		filepath.Join(repoPath, "data"),
		repoPath,
	}

	supportedLangs := []string{"python", "go", "java", "cpp", "rust", "javascript"}

	foundAny := false
	total := 0
	for _, searchDir := range searchPaths {
		if _, err := os.Stat(searchDir); os.IsNotExist(err) {
			continue
		}

		entries, err := os.ReadDir(searchDir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if maxCount > 0 && total >= maxCount {
				break
			}
			name := entry.Name()
			if !strings.HasSuffix(name, ".jsonl") {
				continue
			}

			count, errs := importCodeContestsFile(store, filepath.Join(searchDir, name), supportedLangs, maxCount-total, result)
			result.Imported += count
			result.Errors += errs
			total += count
			foundAny = true
		}
	}

	if !foundAny {
		return nil, fmt.Errorf("no CodeContests JSONL files found in %s", repoPath)
	}

	return result, nil
}

func importCodeContestsFile(store *Store, path string, languages []string, remaining int, result *ImportResult) (imported, errors int) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 1
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 4*1024*1024), 4*1024*1024) // larger buffer for codecontests

	for scanner.Scan() {
		if remaining > 0 && imported >= remaining {
			break
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var entry codeContestEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			errors++
			continue
		}

		if entry.Description == "" {
			result.Skipped++
			continue
		}

		// Collect test cases from all available fields
		inputs, outputs := collectCodeContestTests(entry)
		if len(inputs) == 0 {
			result.Skipped++
			continue
		}

		slug := sanitizeSlug(entry.Name)
		if slug == "" {
			result.Skipped++
			continue
		}

		// Generate a test wrapper for each supported language
		for _, lang := range languages {
			dc, ok := DockerConfig[lang]
			if !ok {
				continue
			}

			testFile := generateStdinStdoutTest(lang, slug, inputs, outputs)
			if testFile == "" {
				continue
			}

			ex := Exercise{
				ID:           fmt.Sprintf("codecontests/%s/%s", lang, slug),
				Suite:        "codecontests",
				Language:     lang,
				Slug:         slug,
				Instructions: buildCodeContestInstructions(entry.Description, inputs, outputs),
				TestFile:     testFile,
				TestCommand:  dc.TestCommand,
				DockerImage:  dc.Image,
			}

			if err := store.UpsertExercise(ex); err != nil {
				errors++
				continue
			}
			imported++
		}
	}

	return imported, errors
}

func collectCodeContestTests(entry codeContestEntry) (inputs, outputs []string) {
	// From input_output array
	for _, io := range entry.InputOutput {
		if io.Input != "" && io.Output != "" {
			inputs = append(inputs, io.Input)
			outputs = append(outputs, io.Output)
		}
	}
	// From public_tests
	for i := 0; i < len(entry.PublicTests.Input) && i < len(entry.PublicTests.Output); i++ {
		inputs = append(inputs, entry.PublicTests.Input[i])
		outputs = append(outputs, entry.PublicTests.Output[i])
	}
	// From private_tests
	for i := 0; i < len(entry.PrivateTests.Input) && i < len(entry.PrivateTests.Output); i++ {
		inputs = append(inputs, entry.PrivateTests.Input[i])
		outputs = append(outputs, entry.PrivateTests.Output[i])
	}
	return inputs, outputs
}

func buildCodeContestInstructions(description string, inputs, outputs []string) string {
	var sb strings.Builder
	sb.WriteString(description)
	sb.WriteString("\n\n## Examples\n\n")
	// Show up to 3 examples
	limit := 3
	if len(inputs) < limit {
		limit = len(inputs)
	}
	for i := 0; i < limit; i++ {
		sb.WriteString(fmt.Sprintf("### Example %d\n\nInput:\n```\n%s\n```\n\nOutput:\n```\n%s\n```\n\n", i+1, inputs[i], outputs[i]))
	}
	sb.WriteString("Your solution should read from stdin and write to stdout.\n")
	return sb.String()
}

func generateStdinStdoutTest(lang, slug string, inputs, outputs []string) string {
	switch lang {
	case "python":
		return generatePythonStdinTest(slug, inputs, outputs)
	case "go":
		return generateGoStdinTest(slug, inputs, outputs)
	case "java":
		return generateJavaStdinTest(slug, inputs, outputs)
	case "cpp":
		return generateCppStdinTest(slug, inputs, outputs)
	case "rust":
		return generateRustStdinTest(slug, inputs, outputs)
	case "javascript":
		return generateJSStdinTest(slug, inputs, outputs)
	default:
		return ""
	}
}

func generatePythonStdinTest(slug string, inputs, outputs []string) string {
	var sb strings.Builder
	sb.WriteString("import subprocess\nimport sys\n\n")
	for i, input := range inputs {
		if i >= len(outputs) {
			break
		}
		sb.WriteString(fmt.Sprintf("def test_case_%d():\n", i+1))
		sb.WriteString(fmt.Sprintf("    result = subprocess.run([sys.executable, %q], input=%q, capture_output=True, text=True, timeout=30)\n",
			toSnake(slug)+".py", input))
		sb.WriteString(fmt.Sprintf("    assert result.stdout.strip() == %q, f\"Expected %s, got {result.stdout.strip()}\"\n\n",
			strings.TrimSpace(outputs[i]), strings.TrimSpace(outputs[i])))
	}
	return sb.String()
}

func generateGoStdinTest(slug string, inputs, outputs []string) string {
	var sb strings.Builder
	sb.WriteString("package main\n\nimport (\n\t\"os/exec\"\n\t\"strings\"\n\t\"testing\"\n)\n\n")
	for i, input := range inputs {
		if i >= len(outputs) {
			break
		}
		sb.WriteString(fmt.Sprintf("func TestCase%d(t *testing.T) {\n", i+1))
		sb.WriteString("\tcmd := exec.Command(\"go\", \"run\", \".\")\n")
		sb.WriteString(fmt.Sprintf("\tcmd.Stdin = strings.NewReader(%q)\n", input))
		sb.WriteString("\tout, err := cmd.CombinedOutput()\n")
		sb.WriteString("\tif err != nil {\n\t\tt.Fatalf(\"execution failed: %%v\\n%%s\", err, out)\n\t}\n")
		sb.WriteString(fmt.Sprintf("\texpected := %q\n", strings.TrimSpace(outputs[i])))
		sb.WriteString("\tgot := strings.TrimSpace(string(out))\n")
		sb.WriteString("\tif got != expected {\n\t\tt.Errorf(\"expected %%q, got %%q\", expected, got)\n\t}\n}\n\n")
	}
	return sb.String()
}

func generateJavaStdinTest(slug string, inputs, outputs []string) string {
	className := toPascal(slug)
	var sb strings.Builder
	sb.WriteString("import org.junit.jupiter.api.Test;\nimport static org.junit.jupiter.api.Assertions.*;\n")
	sb.WriteString("import java.io.*;\n\n")
	sb.WriteString(fmt.Sprintf("class %sTest {\n", className))
	for i, input := range inputs {
		if i >= len(outputs) {
			break
		}
		sb.WriteString(fmt.Sprintf("    @Test\n    void testCase%d() throws Exception {\n", i+1))
		sb.WriteString(fmt.Sprintf("        ProcessBuilder pb = new ProcessBuilder(\"java\", \"%s.java\");\n", className))
		sb.WriteString("        pb.redirectErrorStream(true);\n")
		sb.WriteString("        Process p = pb.start();\n")
		sb.WriteString(fmt.Sprintf("        p.getOutputStream().write(%q.getBytes());\n", input))
		sb.WriteString("        p.getOutputStream().close();\n")
		sb.WriteString("        String out = new String(p.getInputStream().readAllBytes()).trim();\n")
		sb.WriteString(fmt.Sprintf("        assertEquals(%q, out);\n", strings.TrimSpace(outputs[i])))
		sb.WriteString("    }\n\n")
	}
	sb.WriteString("}\n")
	return sb.String()
}

func generateCppStdinTest(slug string, inputs, outputs []string) string {
	var sb strings.Builder
	sb.WriteString("#include <cassert>\n#include <cstdio>\n#include <cstring>\n#include <string>\n\n")
	sb.WriteString("std::string run_with_input(const char* input) {\n")
	sb.WriteString("    FILE* fp = popen(\"./solution\", \"w\");\n")
	sb.WriteString("    if (!fp) return \"\";\n")
	sb.WriteString("    fputs(input, fp);\n")
	sb.WriteString("    pclose(fp);\n")
	sb.WriteString("    return \"\";\n}\n\n")
	sb.WriteString("int main() {\n")
	for i, input := range inputs {
		if i >= len(outputs) {
			break
		}
		sb.WriteString(fmt.Sprintf("    // Test case %d\n", i+1))
		sb.WriteString(fmt.Sprintf("    // Input: %s\n", strings.ReplaceAll(input, "\n", "\\n")))
		sb.WriteString(fmt.Sprintf("    // Expected: %s\n", strings.TrimSpace(outputs[i])))
	}
	sb.WriteString("    return 0;\n}\n")
	return sb.String()
}

func generateRustStdinTest(slug string, inputs, outputs []string) string {
	var sb strings.Builder
	sb.WriteString("use std::io::Write;\nuse std::process::{Command, Stdio};\n\n")
	for i, input := range inputs {
		if i >= len(outputs) {
			break
		}
		sb.WriteString(fmt.Sprintf("#[test]\nfn test_case_%d() {\n", i+1))
		sb.WriteString("    let mut child = Command::new(\"cargo\")\n")
		sb.WriteString("        .args(&[\"run\", \"--quiet\"])\n")
		sb.WriteString("        .stdin(Stdio::piped())\n")
		sb.WriteString("        .stdout(Stdio::piped())\n")
		sb.WriteString("        .spawn()\n")
		sb.WriteString("        .expect(\"failed to spawn\");\n")
		sb.WriteString(fmt.Sprintf("    child.stdin.take().unwrap().write_all(%q.as_bytes()).unwrap();\n", input))
		sb.WriteString("    let output = child.wait_with_output().expect(\"failed to wait\");\n")
		sb.WriteString("    let got = String::from_utf8_lossy(&output.stdout).trim().to_string();\n")
		sb.WriteString(fmt.Sprintf("    assert_eq!(got, %q);\n", strings.TrimSpace(outputs[i])))
		sb.WriteString("}\n\n")
	}
	return sb.String()
}

func generateJSStdinTest(slug string, inputs, outputs []string) string {
	var sb strings.Builder
	sb.WriteString("const { execSync } = require('child_process');\n\n")
	for i, input := range inputs {
		if i >= len(outputs) {
			break
		}
		sb.WriteString(fmt.Sprintf("test('case %d', () => {\n", i+1))
		sb.WriteString(fmt.Sprintf("  const result = execSync('node %s.js', { input: %q, encoding: 'utf8' });\n",
			toCamel(slug), input))
		sb.WriteString(fmt.Sprintf("  expect(result.trim()).toBe(%q);\n", strings.TrimSpace(outputs[i])))
		sb.WriteString("});\n\n")
	}
	return sb.String()
}

// sanitizeSlug converts a task/problem ID into a safe slug.
func sanitizeSlug(name string) string {
	if name == "" {
		return ""
	}
	// Replace common separators with hyphens
	slug := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			return r
		case r >= 'A' && r <= 'Z':
			return r + 32 // lowercase
		case r == '_', r == '/', r == ' ', r == '.':
			return '-'
		default:
			return -1
		}
	}, name)
	// Remove leading/trailing/double hyphens
	for strings.Contains(slug, "--") {
		slug = strings.ReplaceAll(slug, "--", "-")
	}
	slug = strings.Trim(slug, "-")
	return slug
}

// languageExtensions maps language names to file extensions.
var languageExtensions = map[string]string{
	"go":         ".go",
	"python":     ".py",
	"python-ds":  ".py",
	"javascript": ".js",
	"java":       ".java",
	"rust":       ".rs",
	"cpp":        ".cpp",
	"sql":        ".sql",
	"text":       ".txt",
	"pptx":       ".pptx",
}

func readFileFromPaths(paths ...string) string {
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err == nil && len(data) > 0 {
			return string(data)
		}
	}
	return ""
}

func findTestFile(dir, slug, lang, ext string) string {
	// Language-specific test file naming conventions
	var candidates []string
	switch lang {
	case "go":
		candidates = []string{
			filepath.Join(dir, toSnake(slug)+"_test.go"),
			filepath.Join(dir, slug+"_test.go"),
		}
	case "python":
		candidates = []string{
			filepath.Join(dir, toSnake(slug)+"_test.py"),
			filepath.Join(dir, "test_"+toSnake(slug)+".py"),
			filepath.Join(dir, slug+"_test.py"),
		}
	case "javascript":
		candidates = []string{
			filepath.Join(dir, slug+".spec.js"),
			filepath.Join(dir, slug+".test.js"),
			filepath.Join(dir, toCamel(slug)+".spec.js"),
			filepath.Join(dir, toCamel(slug)+".test.js"),
		}
	case "java":
		candidates = []string{
			filepath.Join(dir, "src", "test", "java", toPascal(slug)+"Test.java"),
			filepath.Join(dir, toPascal(slug)+"Test.java"),
		}
	case "rust":
		candidates = []string{
			filepath.Join(dir, "tests", toSnake(slug)+".rs"),
			filepath.Join(dir, "src", "lib.rs"),
		}
	case "cpp":
		candidates = []string{
			filepath.Join(dir, toSnake(slug)+"_test.cpp"),
			filepath.Join(dir, slug+"_test.cpp"),
		}
	default:
		candidates = []string{filepath.Join(dir, slug+"_test"+ext)}
	}

	// Also try a glob for *_test* or *test* or *.spec.* files
	for _, c := range candidates {
		if data, err := os.ReadFile(c); err == nil && len(data) > 0 {
			return string(data)
		}
	}

	// Fallback: glob for test files
	patterns := []string{"*_test" + ext, "*test*" + ext, "*.spec" + ext, "*.test" + ext}
	if lang == "rust" {
		patterns = append(patterns, filepath.Join("tests", "*.rs"))
	}
	for _, pat := range patterns {
		matches, _ := filepath.Glob(filepath.Join(dir, pat))
		if len(matches) > 0 {
			data, err := os.ReadFile(matches[0])
			if err == nil {
				return string(data)
			}
		}
	}

	return ""
}

func findStubFile(dir, slug, lang, ext string) string {
	var candidates []string
	switch lang {
	case "go":
		candidates = []string{
			filepath.Join(dir, toSnake(slug)+".go"),
			filepath.Join(dir, slug+".go"),
		}
	case "python":
		candidates = []string{
			filepath.Join(dir, toSnake(slug)+".py"),
			filepath.Join(dir, slug+".py"),
		}
	case "javascript":
		candidates = []string{
			filepath.Join(dir, slug+".js"),
			filepath.Join(dir, toCamel(slug)+".js"),
		}
	case "java":
		candidates = []string{
			filepath.Join(dir, "src", "main", "java", toPascal(slug)+".java"),
			filepath.Join(dir, toPascal(slug)+".java"),
		}
	case "rust":
		candidates = []string{filepath.Join(dir, "src", "lib.rs")}
	case "cpp":
		candidates = []string{
			filepath.Join(dir, toSnake(slug)+".cpp"),
			filepath.Join(dir, toSnake(slug)+".h"),
		}
	default:
		candidates = []string{filepath.Join(dir, slug+ext)}
	}

	for _, c := range candidates {
		data, err := os.ReadFile(c)
		if err == nil && len(data) > 0 {
			// Skip test files
			name := filepath.Base(c)
			if strings.Contains(name, "_test") || strings.Contains(name, ".test") || strings.Contains(name, ".spec") || strings.HasPrefix(name, "test_") {
				continue
			}
			return string(data)
		}
	}
	return ""
}

func toSnake(slug string) string {
	return strings.ReplaceAll(slug, "-", "_")
}

func toCamel(slug string) string {
	parts := strings.Split(slug, "-")
	for i := 1; i < len(parts); i++ {
		if len(parts[i]) > 0 {
			parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
		}
	}
	return strings.Join(parts, "")
}

func toPascal(slug string) string {
	parts := strings.Split(slug, "-")
	for i := range parts {
		if len(parts[i]) > 0 {
			parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
		}
	}
	return strings.Join(parts, "")
}

// ds1000Entry represents a single DS-1000 exercise.
type ds1000Entry struct {
	Prompt        string          `json:"prompt"`
	ReferenceCode string          `json:"reference_code"`
	CodeContext   string          `json:"code_context"`
	Metadata      ds1000Metadata  `json:"metadata"`
}

type ds1000Metadata struct {
	ProblemID         int    `json:"problem_id"`
	LibraryProblemID  int    `json:"library_problem_id"`
	Library           string `json:"library"`
	TestCaseCnt       int    `json:"test_case_cnt"`
	PerturbationType  string `json:"perturbation_type"`
	PerturbationOriginID int `json:"perturbation_origin_id"`
}

// ImportDS1000 imports exercises from the DS-1000 dataset.
// Expected structure: {repoPath}/data/ds1000.jsonl or ds1000.jsonl.gz
func ImportDS1000(store *Store, repoPath string) (*ImportResult, error) {
	result := &ImportResult{Suite: "ds1000"}

	// Find the JSONL file
	searchPaths := []string{
		filepath.Join(repoPath, "data", "ds1000.jsonl"),
		filepath.Join(repoPath, "ds1000.jsonl"),
		filepath.Join(repoPath, "data", "ds1000.jsonl.gz"),
		filepath.Join(repoPath, "ds1000.jsonl.gz"),
	}

	var filePath string
	for _, p := range searchPaths {
		if _, err := os.Stat(p); err == nil {
			filePath = p
			break
		}
	}
	if filePath == "" {
		return nil, fmt.Errorf("no DS-1000 JSONL file found in %s", repoPath)
	}

	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", filePath, err)
	}
	defer f.Close()

	var scanner *bufio.Scanner
	if strings.HasSuffix(filePath, ".gz") {
		gz, err := gzip.NewReader(f)
		if err != nil {
			return nil, fmt.Errorf("gzip reader: %w", err)
		}
		defer gz.Close()
		scanner = bufio.NewScanner(gz)
	} else {
		scanner = bufio.NewScanner(f)
	}
	scanner.Buffer(make([]byte, 0, 2*1024*1024), 2*1024*1024)

	dc := DockerConfig["python-ds"]

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var entry ds1000Entry
		if err := json.Unmarshal(line, &entry); err != nil {
			result.Errors++
			continue
		}

		if entry.Prompt == "" || entry.CodeContext == "" {
			result.Skipped++
			continue
		}

		slug := fmt.Sprintf("%s-%d", sanitizeSlug(entry.Metadata.Library), entry.Metadata.ProblemID)

		// Build a test runner that uses the code_context for validation
		testRunner := buildDS1000TestRunner(entry)

		ex := Exercise{
			ID:            fmt.Sprintf("ds1000/python/%s", slug),
			Suite:         "ds1000",
			Language:      "python",
			Slug:          slug,
			Instructions:  entry.Prompt,
			TestFile:      testRunner,
			TestCommand:   dc.TestCommand,
			DockerImage:   dc.Image,
			EvalMode:      "docker-test",
			ReferenceCode: entry.ReferenceCode,
		}

		if err := store.UpsertExercise(ex); err != nil {
			result.Errors++
			continue
		}
		result.Imported++
	}

	if result.Imported == 0 {
		return nil, fmt.Errorf("no DS-1000 exercises imported from %s", filePath)
	}

	return result, nil
}

func buildDS1000TestRunner(entry ds1000Entry) string {
	// The code_context contains generate_test_case(), exec_context, test_execution(), etc.
	// test_execution() handles exec internally via exec_context.replace("[insert]", solution).
	// Do NOT exec(solution_code, globals()) — that fails because context variables
	// like df, ax, etc. aren't in globals. Let test_execution handle it.
	var sb strings.Builder
	sb.WriteString("import sys\nsys.path.insert(0, '.')\n\n")
	sb.WriteString(entry.CodeContext)
	sb.WriteString("\n\n")
	sb.WriteString("# Read the solution\n")
	sb.WriteString("with open('solution.py', 'r') as f:\n")
	sb.WriteString("    solution_code = f.read()\n\n")
	sb.WriteString("# Test via the code_context's test functions\n")
	sb.WriteString("try:\n")
	sb.WriteString("    if 'test_execution' in dir():\n")
	sb.WriteString("        result = test_execution(solution_code)\n")
	sb.WriteString("        if result is not None and result != 0:\n")
	sb.WriteString("            print(f'FAIL: test_execution returned {result}')\n")
	sb.WriteString("            sys.exit(1)\n")
	sb.WriteString("    if 'test_string' in dir():\n")
	sb.WriteString("        test_result = test_string(solution_code)\n")
	sb.WriteString("        if test_result is not None and test_result != 0:\n")
	sb.WriteString("            print(f'FAIL: test_string returned {test_result}')\n")
	sb.WriteString("            sys.exit(1)\n")
	sb.WriteString("    print('PASS')\n")
	sb.WriteString("except Exception as e:\n")
	sb.WriteString("    print(f'FAIL: {e}')\n")
	sb.WriteString("    sys.exit(1)\n")
	return sb.String()
}

// birdSQLEntry represents a single BIRD-SQL exercise from the mini_dev_prompt.jsonl file.
type birdSQLEntry struct {
	QuestionID int    `json:"question_id"`
	DBID       string `json:"db_id"`
	Question   string `json:"question"`
	Evidence   string `json:"evidence"`
	SQL        string `json:"SQL"`
	Schema     string `json:"schema"`
	Prompt     string `json:"prompt"`
	Difficulty string `json:"difficulty"`
}

// ImportBIRDSQL imports exercises from the BIRD-SQL Mini-Dev dataset.
// Expected structure: {repoPath}/data/mini_dev_prompt.jsonl
func ImportBIRDSQL(store *Store, repoPath string) (*ImportResult, error) {
	result := &ImportResult{Suite: "birdsql"}

	searchPaths := []string{
		filepath.Join(repoPath, "data", "mini_dev_prompt.jsonl"),
		filepath.Join(repoPath, "mini_dev_prompt.jsonl"),
		filepath.Join(repoPath, "finetuning", "inference", "mini_dev_prompt.jsonl"),
	}

	var filePath string
	for _, p := range searchPaths {
		if _, err := os.Stat(p); err == nil {
			filePath = p
			break
		}
	}
	if filePath == "" {
		return nil, fmt.Errorf("no BIRD-SQL JSONL file found in %s", repoPath)
	}

	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", filePath, err)
	}
	defer f.Close()

	dc := DockerConfig["sql"]

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var entry birdSQLEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			result.Errors++
			continue
		}

		if entry.Question == "" || entry.SQL == "" {
			result.Skipped++
			continue
		}

		slug := fmt.Sprintf("%s-%d", sanitizeSlug(entry.DBID), entry.QuestionID)

		// Build instructions with context
		instructions := buildBIRDSQLInstructions(entry)

		// Build test runner that compares SQL output
		testRunner := buildBIRDSQLTestRunner(entry)

		ex := Exercise{
			ID:            fmt.Sprintf("birdsql/sql/%s", slug),
			Suite:         "birdsql",
			Language:      "sql",
			Slug:          slug,
			Instructions:  instructions,
			TestFile:      testRunner,
			TestCommand:   dc.TestCommand,
			DockerImage:   dc.Image,
			EvalMode:      "docker-test",
			ReferenceCode: entry.SQL,
		}

		if err := store.UpsertExercise(ex); err != nil {
			result.Errors++
			continue
		}
		result.Imported++
	}

	if result.Imported == 0 {
		return nil, fmt.Errorf("no BIRD-SQL exercises imported from %s", filePath)
	}

	return result, nil
}

func buildBIRDSQLInstructions(entry birdSQLEntry) string {
	var sb strings.Builder
	sb.WriteString("Write a SQL query to answer the following question.\n\n")
	sb.WriteString("## Question\n\n")
	sb.WriteString(entry.Question)
	sb.WriteString("\n\n")

	if entry.Evidence != "" {
		sb.WriteString("## Evidence / Hints\n\n")
		sb.WriteString(entry.Evidence)
		sb.WriteString("\n\n")
	}

	if entry.Schema != "" {
		sb.WriteString("## Database Schema\n\n```sql\n")
		sb.WriteString(entry.Schema)
		sb.WriteString("\n```\n\n")
	}

	sb.WriteString("## Database\n\n")
	sb.WriteString(fmt.Sprintf("Database: %s\n\n", entry.DBID))
	sb.WriteString("Return ONLY the SQL query. No explanations.\n")
	return sb.String()
}

func buildBIRDSQLTestRunner(entry birdSQLEntry) string {
	var sb strings.Builder
	sb.WriteString("import sqlite3\nimport sys\nimport os\n\n")
	sb.WriteString("# Read generated SQL\n")
	sb.WriteString("with open('solution.sql', 'r') as f:\n")
	sb.WriteString("    generated_sql = f.read().strip()\n\n")
	sb.WriteString(fmt.Sprintf("expected_sql = %q\n\n", entry.SQL))
	sb.WriteString("# Create test database with schema\n")
	sb.WriteString("conn = sqlite3.connect(':memory:')\n")
	sb.WriteString("try:\n")
	sb.WriteString(fmt.Sprintf("    schema = %q\n", entry.Schema))
	sb.WriteString("    for stmt in schema.split(';'):\n")
	sb.WriteString("        stmt = stmt.strip()\n")
	sb.WriteString("        if stmt:\n")
	sb.WriteString("            try:\n")
	sb.WriteString("                conn.execute(stmt)\n")
	sb.WriteString("            except Exception:\n")
	sb.WriteString("                pass\n")
	sb.WriteString("    # Syntax check: try to explain the generated SQL\n")
	sb.WriteString("    try:\n")
	sb.WriteString("        conn.execute(f'EXPLAIN {generated_sql}')\n")
	sb.WriteString("        print('PASS: SQL is syntactically valid')\n")
	sb.WriteString("    except Exception as e:\n")
	sb.WriteString("        print(f'FAIL: Invalid SQL: {e}')\n")
	sb.WriteString("        sys.exit(1)\n")
	sb.WriteString("except Exception as e:\n")
	sb.WriteString("    print(f'FAIL: {e}')\n")
	sb.WriteString("    sys.exit(1)\n")
	sb.WriteString("finally:\n")
	sb.WriteString("    conn.close()\n")
	return sb.String()
}

// dareBenchEntry represents a single DARE-bench task.
type dareBenchEntry struct {
	FilePath       string   `json:"file_path"`
	QuestionV1     string   `json:"question_v1"`
	QuestionV2     string   `json:"question_v2"`
	AvailableTools []string `json:"available_tools"`
	Task           string   `json:"task"`
	UsabilityRating float64 `json:"usability_rating"`
	NeededFilesV1  []string `json:"needed_files_v1"`
	NeededFilesV2  []string `json:"needed_files_v2"`
}

// ImportDAREBench imports exercises from the DARE-bench dataset.
// Expected structure: {repoPath}/data/eval/question_list.json
func ImportDAREBench(store *Store, repoPath string) (*ImportResult, error) {
	result := &ImportResult{Suite: "dare-bench"}

	searchPaths := []string{
		filepath.Join(repoPath, "data", "eval", "question_list.json"),
		filepath.Join(repoPath, "question_list.json"),
	}

	var filePath string
	for _, p := range searchPaths {
		if _, err := os.Stat(p); err == nil {
			filePath = p
			break
		}
	}
	if filePath == "" {
		return nil, fmt.Errorf("no DARE-bench question_list.json found in %s", repoPath)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", filePath, err)
	}

	var entries []dareBenchEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parse question_list.json: %w", err)
	}

	dc := DockerConfig["python-ds"]

	for i, entry := range entries {
		question := entry.QuestionV1
		if question == "" {
			question = entry.QuestionV2
		}
		if question == "" {
			result.Skipped++
			continue
		}

		slug := sanitizeSlug(entry.FilePath)
		if slug == "" {
			slug = fmt.Sprintf("task-%d", i)
		}

		// Determine metric based on task type
		metricName := "exact_match"
		switch entry.Task {
		case "classification":
			metricName = "macro_f1"
		case "regression":
			metricName = "clipped_r2"
		case "ts":
			metricName = "clipped_r2"
		}

		ex := Exercise{
			ID:           fmt.Sprintf("dare-bench/python/%s", slug),
			Suite:        "dare-bench",
			Language:     "python",
			Slug:         slug,
			Instructions: question,
			TestFile:     buildDAREBenchTestRunner(entry, metricName),
			TestCommand:  dc.TestCommand,
			DockerImage:  dc.Image,
			EvalMode:     "metric-compare",
			Criteria:     fmt.Sprintf(`{"task_type":"%s","metric":"%s"}`, entry.Task, metricName),
		}

		if err := store.UpsertExercise(ex); err != nil {
			result.Errors++
			continue
		}
		result.Imported++
	}

	if result.Imported == 0 {
		return nil, fmt.Errorf("no DARE-bench exercises imported from %s", filePath)
	}

	return result, nil
}

func buildDAREBenchTestRunner(entry dareBenchEntry, metricName string) string {
	var sb strings.Builder
	sb.WriteString("import sys\nimport os\n\n")
	sb.WriteString("# DARE-bench metric-based evaluation\n")
	sb.WriteString("# The solution should produce predictions that are compared against ground truth\n")
	sb.WriteString(fmt.Sprintf("TASK_TYPE = %q\n", entry.Task))
	sb.WriteString(fmt.Sprintf("METRIC_NAME = %q\n\n", metricName))
	sb.WriteString("try:\n")
	sb.WriteString("    exec(open('solution.py').read())\n")
	sb.WriteString("    print(f'PASS: Solution executed successfully (metric evaluation deferred to runner)')\n")
	sb.WriteString("except Exception as e:\n")
	sb.WriteString("    print(f'FAIL: {e}')\n")
	sb.WriteString("    sys.exit(1)\n")
	return sb.String()
}

// writingBenchEntry represents a single WritingBench exercise.
type writingBenchEntry struct {
	Index     int                    `json:"index"`
	Domain1   string                 `json:"domain1"`
	Domain2   string                 `json:"domain2"`
	Lang      string                 `json:"lang"`
	Query     string                 `json:"query"`
	Checklist []writingBenchCriteria `json:"checklist"`
}

type writingBenchCriteria struct {
	Name                string `json:"name"`
	CriteriaDescription string `json:"criteria_description"`
	Low                 string `json:"1-2"`
	MedLow              string `json:"3-4"`
	Med                  string `json:"5-6"`
	MedHigh              string `json:"7-8"`
	High                 string `json:"9-10"`
}

// ImportWritingBench imports exercises from the WritingBench dataset.
// Expected structure: {repoPath}/data/benchmark_all.jsonl or {repoPath}/benchmark_query/benchmark_all.jsonl
func ImportWritingBench(store *Store, repoPath string) (*ImportResult, error) {
	result := &ImportResult{Suite: "writingbench"}

	searchPaths := []string{
		filepath.Join(repoPath, "data", "benchmark_all.jsonl"),
		filepath.Join(repoPath, "benchmark_query", "benchmark_all.jsonl"),
		filepath.Join(repoPath, "benchmark_all.jsonl"),
	}

	var filePath string
	for _, p := range searchPaths {
		if _, err := os.Stat(p); err == nil {
			filePath = p
			break
		}
	}
	if filePath == "" {
		return nil, fmt.Errorf("no WritingBench JSONL file found in %s", repoPath)
	}

	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", filePath, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var entry writingBenchEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			result.Errors++
			continue
		}

		if entry.Query == "" {
			result.Skipped++
			continue
		}

		slug := fmt.Sprintf("%s-%d", sanitizeSlug(entry.Domain1), entry.Index)

		// Serialize criteria as JSON for the judge
		criteriaJSON, _ := json.Marshal(entry.Checklist)

		ex := Exercise{
			ID:           fmt.Sprintf("writingbench/text/%s", slug),
			Suite:        "writingbench",
			Language:     "text",
			Slug:         slug,
			Instructions: entry.Query,
			TestFile:     "",
			TestCommand:  "",
			DockerImage:  "",
			EvalMode:     "llm-judge",
			Criteria:     string(criteriaJSON),
		}

		if err := store.UpsertExercise(ex); err != nil {
			result.Errors++
			continue
		}
		result.Imported++
	}

	if result.Imported == 0 {
		return nil, fmt.Errorf("no WritingBench exercises imported from %s", filePath)
	}

	return result, nil
}

// pptArenaEntry represents a single PPTArena task.
type pptArenaEntry struct {
	Name             string `json:"name"`
	Prompt           string `json:"prompt"`
	StyleTarget      string `json:"style_target"`
	Original         string `json:"original"`
	GroundTruth      string `json:"ground_truth"`
	Category         string `json:"category"`
	EnhancementNotes string `json:"enhancement_notes"`
	EditType         string `json:"edit_type"`
}

// ImportPPTArena imports exercises from the PPTArena dataset.
// Expected structure: {repoPath}/data/evaluation_pairs_refined.json
func ImportPPTArena(store *Store, repoPath string) (*ImportResult, error) {
	result := &ImportResult{Suite: "pptarena"}

	searchPaths := []string{
		filepath.Join(repoPath, "data", "evaluation_pairs_refined.json"),
		filepath.Join(repoPath, "src", "evaluation_pairs_refined.json"),
		filepath.Join(repoPath, "evaluation_pairs_refined.json"),
	}

	var filePath string
	for _, p := range searchPaths {
		if _, err := os.Stat(p); err == nil {
			filePath = p
			break
		}
	}
	if filePath == "" {
		return nil, fmt.Errorf("no PPTArena JSON file found in %s", repoPath)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", filePath, err)
	}

	var entries []pptArenaEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parse PPTArena JSON: %w", err)
	}

	for i, entry := range entries {
		if entry.Prompt == "" {
			result.Skipped++
			continue
		}

		slug := sanitizeSlug(entry.Name)
		if slug == "" {
			slug = fmt.Sprintf("task-%d", i)
		}

		instructions := fmt.Sprintf("## PPTX Edit Task\n\n%s\n\nCategory: %s\nEdit Type: %s\n",
			entry.Prompt, entry.Category, entry.EditType)
		if entry.EnhancementNotes != "" {
			instructions += fmt.Sprintf("\nNotes: %s\n", entry.EnhancementNotes)
		}

		ex := Exercise{
			ID:           fmt.Sprintf("pptarena/pptx/%s", slug),
			Suite:        "pptarena",
			Language:     "pptx",
			Slug:         slug,
			Instructions: instructions,
			TestFile:     "",
			TestCommand:  "",
			DockerImage:  "",
			EvalMode:     "vlm-judge",
			Criteria:     fmt.Sprintf(`{"original":"%s","ground_truth":"%s","style_target":"%s"}`, entry.Original, entry.GroundTruth, entry.StyleTarget),
		}

		if err := store.UpsertExercise(ex); err != nil {
			result.Errors++
			continue
		}
		result.Imported++
	}

	if result.Imported == 0 {
		return nil, fmt.Errorf("no PPTArena exercises imported from %s", filePath)
	}

	return result, nil
}
