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

	testFile := toSnake(ex.Slug) + "_test.py"

	// For check(candidate) pattern, run directly with python3
	var cmd *exec.Cmd
	if strings.Contains(ex.TestFile, "check(candidate)") {
		cmd = exec.CommandContext(ctx, "python3", testFile)
	} else if hasPytest() {
		cmd = exec.CommandContext(ctx, "python3", "-m", "pytest", testFile, "-v")
	} else {
		cmd = exec.CommandContext(ctx, "python3", "-m", "unittest", testFile, "-v")
	}
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

	// Build with cmake
	buildDir := filepath.Join(tmpDir, "build")
	os.MkdirAll(buildDir, 0755)

	cmake := exec.CommandContext(ctx, "cmake", "..")
	cmake.Dir = buildDir
	if out, err := cmake.CombinedOutput(); err != nil {
		return DockerTestResult{Error: fmt.Sprintf("cmake: %s", string(out))}
	}

	make := exec.CommandContext(ctx, "make")
	make.Dir = buildDir
	if out, err := make.CombinedOutput(); err != nil {
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
	case "python":
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
	default:
		return false
	}
}

// toSnake, toPascal, toCamel are defined in importer.go — shared across package.
