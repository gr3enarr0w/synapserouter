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

// RunTestNative executes an exercise's test suite natively on the host.
// Falls back from Docker when Docker is unavailable.
func RunTestNative(ctx context.Context, exercise Exercise, code string, timeout time.Duration) DockerTestResult {
	if timeout == 0 {
		timeout = 60 * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	switch exercise.Language {
	case "go":
		return runGoTestNative(ctx, exercise, code)
	case "python":
		return runPythonTestNative(ctx, exercise, code)
	case "javascript":
		return runJSTestNative(ctx, exercise, code)
	case "rust":
		return runRustTestNative(ctx, exercise, code)
	case "cpp":
		return runCppTestNative(ctx, exercise, code)
	case "java":
		return runJavaTestNative(ctx, exercise, code)
	case "sql":
		return runSQLTestNative(ctx, exercise, code)
	default:
		return DockerTestResult{Error: fmt.Sprintf("native test not supported for %s", exercise.Language)}
	}
}

func runGoTestNative(ctx context.Context, ex Exercise, code string) DockerTestResult {
	tmpDir, err := os.MkdirTemp("", "eval-go-*")
	if err != nil {
		return DockerTestResult{Error: fmt.Sprintf("create tmpdir: %v", err)}
	}
	defer os.RemoveAll(tmpDir)

	if err := scaffoldGo(tmpDir, ex, code); err != nil {
		return DockerTestResult{Error: fmt.Sprintf("scaffold: %v", err)}
	}

	cmd := exec.CommandContext(ctx, "go", "test", "-v", "-count=1", "./...")
	cmd.Dir = tmpDir
	// Isolate module from parent go.mod
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GOWORK=off")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	output := stdout.String()
	if stderr.Len() > 0 {
		output += "\n" + stderr.String()
	}

	return buildResult(err, output)
}

func runPythonTestNative(ctx context.Context, ex Exercise, code string) DockerTestResult {
	tmpDir, err := os.MkdirTemp("", "eval-py-*")
	if err != nil {
		return DockerTestResult{Error: fmt.Sprintf("create tmpdir: %v", err)}
	}
	defer os.RemoveAll(tmpDir)

	if err := scaffoldPython(tmpDir, ex, code); err != nil {
		return DockerTestResult{Error: fmt.Sprintf("scaffold: %v", err)}
	}

	// DS1000 pattern: run test_runner.py directly
	if isDS1000TestPattern(ex.TestFile) {
		return runPythonTestWithAutoInstall(ctx, tmpDir, "python3", "test_runner.py")
	}

	testFile := toSnake(ex.Slug) + "_test.py"

	// For check(candidate) pattern, run directly with python3
	if strings.Contains(ex.TestFile, "check(candidate)") {
		return runPythonTestWithAutoInstall(ctx, tmpDir, "python3", testFile)
	}

	if hasPytest() {
		return runPythonTestWithAutoInstall(ctx, tmpDir, "python3", "-m", "pytest", testFile, "-v")
	}
	return runPythonTestWithAutoInstall(ctx, tmpDir, "python3", "-m", "unittest", testFile, "-v")
}

// runPythonTestWithAutoInstall runs a Python test command, and if it fails with
// ModuleNotFoundError or ImportError, auto-installs the missing module and retries once.
func runPythonTestWithAutoInstall(ctx context.Context, dir string, args ...string) DockerTestResult {
	result := runPythonCmd(ctx, dir, args...)

	// Check for missing module errors and auto-install
	if !result.Passed && (strings.Contains(result.Output, "ModuleNotFoundError") || strings.Contains(result.Output, "ImportError")) {
		modules := extractMissingModules(result.Output)
		if len(modules) > 0 {
			installArgs := append([]string{"-m", "pip", "install", "--quiet"}, modules...)
			installCmd := exec.CommandContext(ctx, "python3", installArgs...)
			installCmd.Dir = dir
			installCmd.Run() // best-effort, errors non-fatal
			// Retry the test
			result = runPythonCmd(ctx, dir, args...)
		}
	}
	return result
}

func runPythonCmd(ctx context.Context, dir string, args ...string) DockerTestResult {
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	output := stdout.String()
	if stderr.Len() > 0 {
		output += "\n" + stderr.String()
	}
	return buildResult(err, output)
}

// extractMissingModules parses ModuleNotFoundError/ImportError output to find module names.
func extractMissingModules(output string) []string {
	seen := make(map[string]bool)
	var modules []string

	// Match: ModuleNotFoundError: No module named 'pandas'
	// Match: ImportError: No module named 'sklearn'
	for _, line := range strings.Split(output, "\n") {
		if idx := strings.Index(line, "No module named '"); idx >= 0 {
			rest := line[idx+len("No module named '"):]
			if end := strings.Index(rest, "'"); end > 0 {
				mod := strings.Split(rest[:end], ".")[0] // take top-level package
				if !seen[mod] {
					seen[mod] = true
					modules = append(modules, mod)
				}
			}
		}
	}
	return modules
}

func runJSTestNative(ctx context.Context, ex Exercise, code string) DockerTestResult {
	tmpDir, err := os.MkdirTemp("", "eval-js-*")
	if err != nil {
		return DockerTestResult{Error: fmt.Sprintf("create tmpdir: %v", err)}
	}
	defer os.RemoveAll(tmpDir)

	if err := scaffoldJavaScript(tmpDir, ex, code); err != nil {
		return DockerTestResult{Error: fmt.Sprintf("scaffold: %v", err)}
	}

	// Install jest and run tests
	npmInstall := exec.CommandContext(ctx, "npm", "install", "--silent")
	npmInstall.Dir = tmpDir
	if out, err := npmInstall.CombinedOutput(); err != nil {
		return DockerTestResult{Error: fmt.Sprintf("npm install: %s", string(out))}
	}

	cmd := exec.CommandContext(ctx, "npx", "jest", "--no-cache", "--verbose")
	cmd.Dir = tmpDir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	output := stdout.String()
	if stderr.Len() > 0 {
		output += "\n" + stderr.String()
	}

	return buildResult(err, output)
}

func runRustTestNative(ctx context.Context, ex Exercise, code string) DockerTestResult {
	tmpDir, err := os.MkdirTemp("", "eval-rs-*")
	if err != nil {
		return DockerTestResult{Error: fmt.Sprintf("create tmpdir: %v", err)}
	}
	defer os.RemoveAll(tmpDir)

	if err := scaffoldRust(tmpDir, ex, code); err != nil {
		return DockerTestResult{Error: fmt.Sprintf("scaffold: %v", err)}
	}

	cmd := exec.CommandContext(ctx, "cargo", "test")
	cmd.Dir = tmpDir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	output := stdout.String()
	if stderr.Len() > 0 {
		output += "\n" + stderr.String()
	}

	return buildResult(err, output)
}

func runCppTestNative(ctx context.Context, ex Exercise, code string) DockerTestResult {
	tmpDir, err := os.MkdirTemp("", "eval-cpp-*")
	if err != nil {
		return DockerTestResult{Error: fmt.Sprintf("create tmpdir: %v", err)}
	}
	defer os.RemoveAll(tmpDir)

	if err := scaffoldCpp(tmpDir, ex, code); err != nil {
		return DockerTestResult{Error: fmt.Sprintf("scaffold: %v", err)}
	}

	// Download Catch2 single-header if tests need it
	if strings.Contains(ex.TestFile, "catch.hpp") || strings.Contains(ex.TestFile, "catch2") {
		if err := ensureCatch2Header(tmpDir); err != nil {
			return DockerTestResult{Error: fmt.Sprintf("catch2 setup: %v", err)}
		}
	}

	// Build with cmake (CMakeLists already has -DEXERCISM_RUN_ALL_TESTS)
	buildDir := filepath.Join(tmpDir, "build")
	if err := os.MkdirAll(buildDir, 0755); err != nil {
		return DockerTestResult{Error: fmt.Sprintf("create build dir: %v", err)}
	}

	cmake := exec.CommandContext(ctx, "cmake", "..")
	cmake.Dir = buildDir
	if out, err := cmake.CombinedOutput(); err != nil {
		return DockerTestResult{Output: string(out), ExitCode: 1}
	}

	makeCmd := exec.CommandContext(ctx, "make")
	makeCmd.Dir = buildDir
	if out, err := makeCmd.CombinedOutput(); err != nil {
		return DockerTestResult{Output: string(out), ExitCode: 1}
	}

	// Run test binary
	slug := toSnake(ex.Slug)
	cmd := exec.CommandContext(ctx, filepath.Join(buildDir, slug+"_test"))
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	output := stdout.String()
	if stderr.Len() > 0 {
		output += "\n" + stderr.String()
	}

	return buildResult(err, output)
}

// ensureCatch2Header downloads the Catch2 single-include header if not cached,
// then copies it into the workspace's test/ directory.
func ensureCatch2Header(workDir string) error {
	// Use home directory for persistent cache (macOS cleans /tmp aggressively)
	home, _ := os.UserHomeDir()
	cacheDir := filepath.Join(home, ".cache", "synroute", "catch2")
	cachedHeader := filepath.Join(cacheDir, "catch.hpp")

	// Download to cache if not present
	if _, err := os.Stat(cachedHeader); err != nil {
		if err := os.MkdirAll(cacheDir, 0755); err != nil {
			return fmt.Errorf("create cache dir: %w", err)
		}
		cmd := exec.Command("curl", "-sL", "-o", cachedHeader,
			"https://raw.githubusercontent.com/catchorg/Catch2/v2.13.10/single_include/catch2/catch.hpp")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("download catch2: %s", string(out))
		}
	}

	// Copy cached header into workspace
	testDir := filepath.Join(workDir, "test")
	if err := os.MkdirAll(testDir, 0755); err != nil {
		return fmt.Errorf("create test dir: %w", err)
	}
	data, err := os.ReadFile(cachedHeader)
	if err != nil {
		return fmt.Errorf("read cached header: %w", err)
	}
	return os.WriteFile(filepath.Join(testDir, "catch.hpp"), data, 0644)
}

func runJavaTestNative(ctx context.Context, ex Exercise, code string) DockerTestResult {
	tmpDir, err := os.MkdirTemp("", "eval-java-*")
	if err != nil {
		return DockerTestResult{Error: fmt.Sprintf("create tmpdir: %v", err)}
	}
	defer os.RemoveAll(tmpDir)

	if err := scaffoldJava(tmpDir, ex, code); err != nil {
		return DockerTestResult{Error: fmt.Sprintf("scaffold: %v", err)}
	}

	className := toPascal(ex.Slug)
	srcDir := filepath.Join(tmpDir, "src", "main", "java")
	testDir := filepath.Join(tmpDir, "src", "test", "java")
	outDir := filepath.Join(tmpDir, "out")
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return DockerTestResult{Error: fmt.Sprintf("create output dir: %v", err)}
	}

	// Download JUnit + AssertJ if needed
	junitJar := ensureJUnit()
	if junitJar == "" {
		return DockerTestResult{Error: "JUnit not available"}
	}
	assertjJar := ensureAssertJ()

	// Build classpath with JUnit + AssertJ
	cp := srcDir + ":" + testDir + ":" + junitJar
	if assertjJar != "" {
		cp += ":" + assertjJar
	}

	// Compile
	compileCmd := exec.CommandContext(ctx, "javac",
		"-cp", cp, "-d", outDir,
		filepath.Join(srcDir, className+".java"),
		filepath.Join(testDir, className+"Test.java"))
	if out, err := compileCmd.CombinedOutput(); err != nil {
		return DockerTestResult{Output: string(out), ExitCode: 1}
	}

	// Run via junit-platform-console-standalone (JUnit 5 API)
	scanCP := outDir
	if assertjJar != "" {
		scanCP += ":" + assertjJar
	}
	cmd := exec.CommandContext(ctx, "java",
		"-jar", junitJar,
		"--class-path", scanCP,
		"--scan-classpath")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	output := stdout.String()
	if stderr.Len() > 0 {
		output += "\n" + stderr.String()
	}

	return buildResult(err, output)
}

func ensureJUnit() string {
	return ensureJar("junit-platform-console-standalone-1.10.2.jar",
		"https://repo1.maven.org/maven2/org/junit/platform/junit-platform-console-standalone/1.10.2/junit-platform-console-standalone-1.10.2.jar")
}

func ensureAssertJ() string {
	return ensureJar("assertj-core-3.25.1.jar",
		"https://repo1.maven.org/maven2/org/assertj/assertj-core/3.25.1/assertj-core-3.25.1.jar")
}

// ensureJar downloads a JAR file to a persistent cache directory if not already present.
func ensureJar(filename, url string) string {
	home, _ := os.UserHomeDir()
	cacheDir := filepath.Join(home, ".cache", "synroute", "junit")
	jarPath := filepath.Join(cacheDir, filename)

	if _, err := os.Stat(jarPath); err == nil {
		return jarPath
	}

	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return ""
	}
	cmd := exec.Command("curl", "-sL", "-o", jarPath, url)
	if _, err := cmd.CombinedOutput(); err != nil {
		return ""
	}
	return jarPath
}

func runSQLTestNative(ctx context.Context, ex Exercise, code string) DockerTestResult {
	tmpDir, err := os.MkdirTemp("", "eval-sql-*")
	if err != nil {
		return DockerTestResult{Error: fmt.Sprintf("create tmpdir: %v", err)}
	}
	defer os.RemoveAll(tmpDir)

	if err := scaffoldSQL(tmpDir, ex, code); err != nil {
		return DockerTestResult{Error: fmt.Sprintf("scaffold: %v", err)}
	}

	// SQL tests are Python scripts that use sqlite3
	cmd := exec.CommandContext(ctx, "python3", "test_runner.py")
	cmd.Dir = tmpDir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	output := stdout.String()
	if stderr.Len() > 0 {
		output += "\n" + stderr.String()
	}

	return buildResult(err, output)
}

func buildResult(err error, output string) DockerTestResult {
	if err == nil {
		return DockerTestResult{Passed: true, Output: output, ExitCode: 0}
	}
	exitCode := -1
	if exitErr, ok := err.(*exec.ExitError); ok {
		exitCode = exitErr.ExitCode()
	}
	return DockerTestResult{Output: output, ExitCode: exitCode}
}

func hasPytest() bool {
	cmd := exec.Command("python3", "-m", "pytest", "--version")
	return cmd.Run() == nil
}

// NativeTestSupported returns whether native test execution is available for a language.
func NativeTestSupported(language string) bool {
	switch language {
	case "go":
		_, err := exec.LookPath("go")
		return err == nil
	case "python", "python-ds":
		_, err := exec.LookPath("python3")
		return err == nil
	case "javascript":
		_, err := exec.LookPath("node")
		return err == nil
	case "rust":
		_, err := exec.LookPath("cargo")
		return err == nil
	case "cpp":
		_, err := exec.LookPath("cmake")
		return err == nil
	case "java":
		_, err := exec.LookPath("javac")
		return err == nil
	case "sql":
		// SQL tests run via python3 sqlite3 module
		_, err := exec.LookPath("python3")
		return err == nil
	case "text":
		return true // llm-judge, no runtime needed
	default:
		return false
	}
}

// toSnake, toPascal, toCamel are defined in importer.go — shared across package.
