package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBashNormalExecution(t *testing.T) {
	tool := &BashTool{}
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"command": "echo hello",
	}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit 0, got %d", result.ExitCode)
	}
	if !strings.Contains(result.Output, "hello") {
		t.Errorf("expected output to contain 'hello', got %q", result.Output)
	}
}

func TestBashNonZeroExit(t *testing.T) {
	tool := &BashTool{}
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"command": "exit 42",
	}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 42 {
		t.Errorf("expected exit 42, got %d", result.ExitCode)
	}
}

func TestBashTimeout(t *testing.T) {
	tool := &BashTool{}
	start := time.Now()
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"command":    "sleep 999",
		"timeout_ms": float64(2000), // 2 second timeout
	}, t.TempDir())
	elapsed := time.Since(start)

	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != -1 {
		t.Errorf("expected exit -1 on timeout, got %d", result.ExitCode)
	}
	if !strings.Contains(result.Error, "timed out") {
		t.Errorf("expected timeout error, got %q", result.Error)
	}
	// Should complete within ~3 seconds (2s timeout + 1s grace), not 999s
	if elapsed > 10*time.Second {
		t.Errorf("timeout took too long: %v (expected ~2s)", elapsed)
	}
}

func TestBashTimeoutKillsChildren(t *testing.T) {
	tool := &BashTool{}
	// Use a unique marker so we don't match other sleep processes.
	// Run a foreground process that spawns a child — the parent blocks on the child.
	marker := fmt.Sprintf("bash_test_%d", time.Now().UnixNano())
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		// sh -c runs a subshell that sleeps. The parent sh waits for it.
		// Both should be killed by the process group SIGKILL.
		"command":    fmt.Sprintf("sh -c 'exec sleep 300' # %s", marker),
		"timeout_ms": float64(2000),
	}, t.TempDir())

	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != -1 {
		t.Errorf("expected exit -1 on timeout, got %d", result.ExitCode)
	}

	// Wait briefly for OS to clean up processes
	time.Sleep(500 * time.Millisecond)

	// Verify no orphaned processes with our marker remain
	out, _ := exec.Command("pgrep", "-f", marker).Output()
	if len(strings.TrimSpace(string(out))) > 0 {
		t.Errorf("orphaned child process still running: %s", string(out))
	}
}

func TestBashStderrCaptured(t *testing.T) {
	tool := &BashTool{}
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"command": "echo stdout-msg; echo stderr-msg >&2",
	}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Output, "stdout-msg") {
		t.Errorf("expected stdout captured, got %q", result.Output)
	}
	if !strings.Contains(result.Output, "stderr-msg") {
		t.Errorf("expected stderr captured, got %q", result.Output)
	}
}

func TestBashContextCancellation(t *testing.T) {
	tool := &BashTool{}
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after 1 second
	go func() {
		time.Sleep(1 * time.Second)
		cancel()
	}()

	start := time.Now()
	result, err := tool.Execute(ctx, map[string]interface{}{
		"command": "sleep 999",
	}, t.TempDir())
	elapsed := time.Since(start)

	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != -1 {
		t.Errorf("expected exit -1 on cancel, got %d", result.ExitCode)
	}
	if elapsed > 10*time.Second {
		t.Errorf("cancellation took too long: %v", elapsed)
	}
}

func TestBashRejectsInlineCode(t *testing.T) {
	tool := &BashTool{}
	tests := []struct {
		name    string
		command string
	}{
		{"python -c", `python -c "print('hello')"`},
		{"python3 -c", `python3 -c "import sys; print(sys.version)"`},
		{"node -e", `node -e "console.log('hi')"`},
		{"ruby -e", `ruby -e "puts 'hi'"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tool.Execute(context.Background(), map[string]interface{}{
				"command": tt.command,
			}, t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			if result.ExitCode != -1 {
				t.Errorf("expected exit -1 for inline code, got %d", result.ExitCode)
			}
			if !strings.Contains(result.Error, "rejected") {
				t.Errorf("expected rejection error, got %q", result.Error)
			}
		})
	}
}

func TestBashAllowsPythonFileExecution(t *testing.T) {
	// Running a .py file should NOT be blocked — only python -c
	tool := &BashTool{}
	dir := t.TempDir()
	// Create a test Python file
	os.WriteFile(filepath.Join(dir, "test.py"), []byte("print('hello')"), 0644)
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"command": "python3 test.py",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	// Should succeed (if python3 is installed) or fail normally, but NOT be rejected
	if strings.Contains(result.Error, "rejected") {
		t.Error("python3 test.py should not be rejected")
	}
}

func TestBashWorkDir(t *testing.T) {
	tool := &BashTool{}
	dir := t.TempDir()
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"command": "pwd",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Output, dir) {
		t.Errorf("expected working dir %s in output, got %q", dir, result.Output)
	}
}
