package eval

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// DockerTestResult holds the outcome of running tests in Docker.
type DockerTestResult struct {
	Passed   bool
	Output   string
	ExitCode int
	Error    string
}

// RunTestInDocker executes an exercise's test suite inside a Docker container.
func RunTestInDocker(ctx context.Context, exercise Exercise, code string, timeout time.Duration) DockerTestResult {
	if timeout == 0 {
		timeout = 60 * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	tmpDir, err := os.MkdirTemp("", "eval-*")
	if err != nil {
		return DockerTestResult{Error: fmt.Sprintf("create tmpdir: %v", err)}
	}
	defer os.RemoveAll(tmpDir)

	if err := scaffoldWorkspace(tmpDir, exercise, code); err != nil {
		return DockerTestResult{Error: fmt.Sprintf("scaffold: %v", err)}
	}

	args := []string{
		"run", "--rm",
		"--network", "none",
		"--memory", "512m",
		"--cpus", "1",
		"-v", tmpDir + ":/workspace",
		"-w", "/workspace",
		exercise.DockerImage,
		"sh", "-c", exercise.TestCommand,
	}

	cmd := exec.CommandContext(ctx, "docker", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	output := stdout.String()
	if stderr.Len() > 0 {
		output += "\n" + stderr.String()
	}

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return DockerTestResult{
				Output:   output,
				ExitCode: -1,
				Error:    err.Error(),
			}
		}
	}

	return DockerTestResult{
		Passed:   exitCode == 0,
		Output:   output,
		ExitCode: exitCode,
	}
}

// scaffoldWorkspace sets up the workspace directory for the exercise's language.
func scaffoldWorkspace(dir string, exercise Exercise, code string) error {
	switch exercise.Language {
	case "go":
		return scaffoldGo(dir, exercise, code)
	case "python":
		return scaffoldPython(dir, exercise, code)
	case "python-ds":
		return scaffoldPythonDS(dir, exercise, code)
	case "sql":
		return scaffoldSQL(dir, exercise, code)
	case "javascript":
		return scaffoldJavaScript(dir, exercise, code)
	case "java":
		return scaffoldJava(dir, exercise, code)
	case "rust":
		return scaffoldRust(dir, exercise, code)
	case "cpp":
		return scaffoldCpp(dir, exercise, code)
	case "text", "pptx":
		// Non-docker eval modes (llm-judge, vlm-judge) — no scaffolding needed
		return nil
	default:
		return fmt.Errorf("unsupported language: %s", exercise.Language)
	}
}

func scaffoldGo(dir string, ex Exercise, code string) error {
	// Detect package name from code or test file
	pkg := detectGoPackage(ex.TestFile)
	if pkg == "" {
		pkg = toSnake(ex.Slug)
	}

	// go.mod
	modContent := fmt.Sprintf("module %s\n\ngo 1.22\n", pkg)
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(modContent), 0644); err != nil {
		return err
	}

	// Implementation file
	codeFile := toSnake(ex.Slug) + ".go"
	// Ensure package declaration matches test package
	if !strings.HasPrefix(strings.TrimSpace(code), "package ") {
		code = "package " + pkg + "\n\n" + code
	}
	if err := os.WriteFile(filepath.Join(dir, codeFile), []byte(code), 0644); err != nil {
		return err
	}

	// Test file
	testFile := toSnake(ex.Slug) + "_test.go"
	if err := os.WriteFile(filepath.Join(dir, testFile), []byte(ex.TestFile), 0644); err != nil {
		return err
	}

	// If test references testCases (exercism pattern), generate cases_test.go
	// by including the test cases definition in the prompt via ReferenceCode field,
	// or scaffold a stub that the LLM should have generated.
	if strings.Contains(ex.TestFile, "testCases") && ex.ReferenceCode != "" {
		casesFile := "cases_test.go"
		if err := os.WriteFile(filepath.Join(dir, casesFile), []byte(ex.ReferenceCode), 0644); err != nil {
			return err
		}
	}

	return nil
}

func scaffoldPython(dir string, ex Exercise, code string) error {
	// Implementation
	implFile := toSnake(ex.Slug) + ".py"
	if err := os.WriteFile(filepath.Join(dir, implFile), []byte(code), 0644); err != nil {
		return err
	}

	// Test file
	testContent := ex.TestFile
	testFile := toSnake(ex.Slug) + "_test.py"

	// HumanEval/MBPP check(candidate) pattern: wrap with import + invocation
	if strings.Contains(testContent, "check(candidate)") {
		funcName := extractPythonFuncName(code)
		if funcName == "" {
			funcName = toSnake(ex.Slug)
		}

		// Fix inline def: "def check(candidate):assert ..." → proper multiline
		fixedTest := fixInlineCheckDef(testContent)

		wrapper := fmt.Sprintf(`import sys
sys.path.insert(0, '.')
from %s import %s

%s

check(%s)
print("PASS")
`, toSnake(ex.Slug), funcName, fixedTest, funcName)
		testContent = wrapper
	}

	return os.WriteFile(filepath.Join(dir, testFile), []byte(testContent), 0644)
}

// fixInlineCheckDef handles MBPP tests where the check function is on one line:
// "def check(candidate):assert candidate(...)==True" → proper multiline with indentation.
func fixInlineCheckDef(testContent string) string {
	lines := strings.Split(testContent, "\n")
	var fixed []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Fix "def check(candidate):assert ..." → split into def + indented assert
		if strings.HasPrefix(trimmed, "def check(candidate):") && strings.Contains(trimmed, "assert") {
			body := strings.TrimPrefix(trimmed, "def check(candidate):")
			fixed = append(fixed, "def check(candidate):")
			fixed = append(fixed, "    "+strings.TrimSpace(body))
			continue
		}
		// Fix tab-indented assertions to use 4 spaces
		if strings.HasPrefix(line, "\t") {
			fixed = append(fixed, "    "+strings.TrimLeft(line, "\t"))
			continue
		}
		fixed = append(fixed, line)
	}
	return strings.Join(fixed, "\n")
}

func extractPythonFuncName(code string) string {
	for _, line := range strings.Split(code, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "def ") {
			// "def foo_bar(x):" -> "foo_bar"
			name := strings.TrimPrefix(line, "def ")
			if idx := strings.Index(name, "("); idx > 0 {
				return strings.TrimSpace(name[:idx])
			}
		}
	}
	return ""
}

func scaffoldJavaScript(dir string, ex Exercise, code string) error {
	// Detect if tests use ESM imports
	usesESM := strings.Contains(ex.TestFile, "import ") && strings.Contains(ex.TestFile, " from ")

	// package.json with jest + ESM support if needed
	pkgJSON := `{"scripts":{"test":"jest"},"devDependencies":{"jest":"^29","@jest/globals":"^29"}}`
	if usesESM {
		// Enable ESM transform for jest
		pkgJSON = `{"scripts":{"test":"jest"},"devDependencies":{"jest":"^29","@jest/globals":"^29","@babel/core":"^7","@babel/preset-env":"^7","babel-jest":"^29"}}`
	}
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(pkgJSON), 0644); err != nil {
		return err
	}

	if usesESM {
		// Babel config for ESM → CJS transform
		babelRC := `{"presets":[["@babel/preset-env",{"targets":{"node":"current"}}]]}`
		if err := os.WriteFile(filepath.Join(dir, "babel.config.json"), []byte(babelRC), 0644); err != nil {
			return err
		}
	}

	// Detect import path from test file to match implementation filename
	// Tests import from './slug-name' (kebab-case), not './slugName' (camelCase)
	implName := detectJSImportPath(ex.TestFile, ex.Slug)
	implFile := implName + ".js"

	implCode := code
	// If tests use ESM imports but code uses module.exports, convert
	if usesESM && strings.Contains(code, "module.exports") && !strings.Contains(code, "export ") {
		implCode = strings.ReplaceAll(code, "module.exports", "export default")
	}
	if err := os.WriteFile(filepath.Join(dir, implFile), []byte(implCode), 0644); err != nil {
		return err
	}

	// Test file — enable all tests (replace xtest/xit with test/it)
	testContent := ex.TestFile
	testContent = strings.ReplaceAll(testContent, "xtest(", "test(")
	testContent = strings.ReplaceAll(testContent, "xit(", "it(")
	// Name spec file to match the import path used in the test
	testFile := implName + ".spec.js"
	return os.WriteFile(filepath.Join(dir, testFile), []byte(testContent), 0644)
}

// detectJSImportPath extracts the module path that the test file imports from.
// e.g. "import { convert } from './all-your-base'" → "all-your-base"
func detectJSImportPath(testFile, slug string) string {
	for _, line := range strings.Split(testFile, "\n") {
		line = strings.TrimSpace(line)
		// Match: import ... from './module-name'
		if strings.Contains(line, "from './") {
			parts := strings.Split(line, "from './")
			if len(parts) >= 2 {
				name := strings.TrimRight(parts[1], "';")
				if name != "" {
					return name
				}
			}
		}
		// Match: import ... from "./module-name"
		if strings.Contains(line, `from "./`) {
			parts := strings.Split(line, `from "./`)
			if len(parts) >= 2 {
				name := strings.TrimRight(parts[1], `";`)
				if name != "" {
					return name
				}
			}
		}
	}
	// Fallback to slug (kebab-case, which exercism uses)
	return slug
}

func scaffoldJava(dir string, ex Exercise, code string) error {
	srcDir := filepath.Join(dir, "src", "main", "java")
	testDir := filepath.Join(dir, "src", "test", "java")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(testDir, 0755); err != nil {
		return err
	}

	className := toPascal(ex.Slug)
	if err := os.WriteFile(filepath.Join(srcDir, className+".java"), []byte(code), 0644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(testDir, className+"Test.java"), []byte(ex.TestFile), 0644); err != nil {
		return err
	}

	// build.gradle
	gradle := `plugins { id 'java' }
repositories { mavenCentral() }
dependencies { testImplementation 'org.junit.jupiter:junit-jupiter:5.10.0' }
test { useJUnitPlatform() }
`
	return os.WriteFile(filepath.Join(dir, "build.gradle"), []byte(gradle), 0644)
}

func scaffoldRust(dir string, ex Exercise, code string) error {
	srcDir := filepath.Join(dir, "src")
	testsDir := filepath.Join(dir, "tests")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(testsDir, 0755); err != nil {
		return err
	}

	// Cargo.toml
	cargo := fmt.Sprintf("[package]\nname = %q\nversion = \"0.1.0\"\nedition = \"2021\"\n", toSnake(ex.Slug))
	if err := os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte(cargo), 0644); err != nil {
		return err
	}

	if err := os.WriteFile(filepath.Join(srcDir, "lib.rs"), []byte(code), 0644); err != nil {
		return err
	}

	testFileName := toSnake(ex.Slug) + ".rs"
	return os.WriteFile(filepath.Join(testsDir, testFileName), []byte(ex.TestFile), 0644)
}

func scaffoldCpp(dir string, ex Exercise, code string) error {
	slug := toSnake(ex.Slug)

	// Implementation
	if err := os.WriteFile(filepath.Join(dir, slug+".cpp"), []byte(code), 0644); err != nil {
		return err
	}

	// Header if code references it
	if strings.Contains(ex.TestFile, slug+".h") {
		header := fmt.Sprintf("#pragma once\n// Generated header for %s\n", slug)
		if err := os.WriteFile(filepath.Join(dir, slug+".h"), []byte(header), 0644); err != nil {
			return err
		}
	}

	// Test
	if err := os.WriteFile(filepath.Join(dir, slug+"_test.cpp"), []byte(ex.TestFile), 0644); err != nil {
		return err
	}

	// CMakeLists.txt
	cmake := fmt.Sprintf(`cmake_minimum_required(VERSION 3.14)
project(%s)
set(CMAKE_CXX_STANDARD 17)
add_executable(%s_test %s.cpp %s_test.cpp)
enable_testing()
add_test(NAME %s_test COMMAND %s_test)
`, slug, slug, slug, slug, slug, slug)

	return os.WriteFile(filepath.Join(dir, "CMakeLists.txt"), []byte(cmake), 0644)
}

func scaffoldPythonDS(dir string, ex Exercise, code string) error {
	// Solution file
	if err := os.WriteFile(filepath.Join(dir, "solution.py"), []byte(code), 0644); err != nil {
		return err
	}
	// Test runner from the exercise's test_file
	return os.WriteFile(filepath.Join(dir, "test_runner.py"), []byte(ex.TestFile), 0644)
}

func scaffoldSQL(dir string, ex Exercise, code string) error {
	// Solution SQL file
	if err := os.WriteFile(filepath.Join(dir, "solution.sql"), []byte(code), 0644); err != nil {
		return err
	}
	// Test runner
	return os.WriteFile(filepath.Join(dir, "test_runner.py"), []byte(ex.TestFile), 0644)
}

func detectGoPackage(testFile string) string {
	for _, line := range strings.Split(testFile, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "package ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				return parts[1]
			}
		}
	}
	return ""
}

// IsDockerAvailable checks if Docker is accessible.
func IsDockerAvailable() bool {
	cmd := exec.Command("docker", "info")
	return cmd.Run() == nil
}
