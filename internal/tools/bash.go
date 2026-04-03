package tools

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	defaultBashTimeout = 120 * time.Second
	maxBashTimeout     = 600 * time.Second
	maxOutputBytes     = 1 << 20 // 1MB
	maxConcurrentBash  = 10
)

// bashSemaphore limits concurrent shell executions.
var bashSemaphore = make(chan struct{}, maxConcurrentBash)

// forbiddenInlinePatterns are inline code execution patterns that the agent
// should not use. Instead, it should write code to files with file_write and
// then run the files. Same enforcement pattern as git tool blocking --force.
var forbiddenInlinePatterns = []string{
	"python -c ",
	"python3 -c ",
	"node -e ",
	"ruby -e ",
	"php -r ",
	"perl -e ",
}

// BashTool executes shell commands via sh -c.
type BashTool struct{}

func (t *BashTool) Name() string        { return "bash" }
func (t *BashTool) Description() string { return "Execute a shell command and return its output" }
func (t *BashTool) Category() ToolCategory { return CategoryWrite }

func (t *BashTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "The shell command to execute",
			},
			"timeout_ms": map[string]interface{}{
				"type":        "integer",
				"description": "Timeout in milliseconds (default 120000, max 600000)",
			},
		},
		"required": []string{"command"},
	}
}

func (t *BashTool) Execute(ctx context.Context, args map[string]interface{}, workDir string) (*ToolResult, error) {
	command, _ := args["command"].(string)
	if command == "" {
		return &ToolResult{Error: "command is required"}, nil
	}

	// Reject inline code patterns — agent should write files, not run inline code.
	// Same pattern as git tool blocking dangerous flags.
	for _, pattern := range forbiddenInlinePatterns {
		if strings.Contains(command, pattern) {
			return &ToolResult{
				Error: fmt.Sprintf("inline code (%s) rejected — write code to a file with file_write first, then run the file with bash",
					pattern),
				ExitCode: -1,
			}, nil
		}
	}

	timeout := defaultBashTimeout
	if ms, ok := args["timeout_ms"].(float64); ok && ms > 0 {
		timeout = time.Duration(ms) * time.Millisecond
		if timeout > maxBashTimeout {
			timeout = maxBashTimeout
		}
	}

	// Acquire semaphore to limit concurrent executions
	select {
	case bashSemaphore <- struct{}{}:
		defer func() { <-bashSemaphore }()
	case <-ctx.Done():
		return &ToolResult{Error: "too many concurrent commands", ExitCode: -1}, nil
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Use exec.Command (NOT exec.CommandContext) — we handle timeout ourselves
	// because CommandContext only kills the parent process, not the process group.
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = workDir
	cmd.Env = filteredEnv()

	// Apply OS-native sandboxing (Seatbelt on macOS, Bubblewrap on Linux)
	cfg := DefaultSandboxConfig(workDir)
	cmd = WrapCommand(cmd, cfg)
	// Set process group so we can kill the entire tree on timeout
	setupProcessGroup(cmd)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &limitWriter{w: &stdout, max: maxOutputBytes}
	cmd.Stderr = &limitWriter{w: &stderr, max: maxOutputBytes}

	// Start the process (non-blocking). cmd.Run() would block forever if a
	// child process hangs, making the timeout unreachable.
	if err := cmd.Start(); err != nil {
		return &ToolResult{Output: "", Error: err.Error(), ExitCode: -1}, nil
	}

	// Wait for process completion in a goroutine
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	// Select: process finishes naturally OR context times out
	select {
	case err := <-done:
		// Process finished — check result
		output := combineOutput(stdout.String(), stderr.String())
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				code := exitErr.ExitCode()
				return &ToolResult{
					Output:   output,
					Error:    fmt.Sprintf("exit code %d", code),
					ExitCode: code,
				}, nil
			}
			return &ToolResult{Output: output, Error: err.Error(), ExitCode: -1}, nil
		}
		return &ToolResult{Output: output, ExitCode: 0}, nil

	case <-ctx.Done():
		// Timeout or cancellation — kill the entire process group.
		// Kill the process via platform-specific method:
		// - Unix: syscall.Kill(-pgid, SIGKILL) kills the entire process group
		// - Windows: cmd.Process.Kill() closes the process's stdin
		killProcessGroup(cmd)
		// Wait briefly for cmd.Wait() to return after kill.
		// If it doesn't return (pipe goroutines stuck), move on anyway.
		select {
		case <-done:
		case <-time.After(3 * time.Second):
		}
		msg := "command cancelled"
		if ctx.Err() == context.DeadlineExceeded {
			msg = fmt.Sprintf("command timed out after %s", timeout)
		}
		return &ToolResult{
			Output:   combineOutput(stdout.String(), stderr.String()),
			Error:    msg,
			ExitCode: -1,
		}, nil
	}
}

// combineOutput merges stdout and stderr into a single string.
func combineOutput(stdout, stderr string) string {
	if stderr == "" {
		return stdout
	}
	if stdout == "" {
		return stderr
	}
	return stdout + "\n" + stderr
}

// limitWriter wraps a writer and stops writing after max bytes.
type limitWriter struct {
	w         *bytes.Buffer
	max       int
	written   int
	truncated bool
}

func (lw *limitWriter) Write(p []byte) (int, error) {
	remaining := lw.max - lw.written
	if remaining <= 0 {
		// Drain the pipe by accepting bytes but discarding them.
		// Must consume ALL input to prevent pipe deadlock — if we return
		// less than len(p), the child process blocks in write().
		lw.written += len(p)
		return len(p), nil
	}
	if len(p) > remaining {
		// Write only what fits, but report full len(p) consumed to prevent
		// the child from blocking. The excess is silently discarded.
		n, err := lw.w.Write(p[:remaining])
		lw.written += n + (len(p) - remaining)
		if !lw.truncated && err == nil {
			lw.truncated = true
			lw.w.WriteString("...\n[output truncated at 1MB]")
		}
		return len(p), err
	}
	n, err := lw.w.Write(p)
	lw.written += n
	if lw.written >= lw.max && !lw.truncated && err == nil {
		lw.truncated = true
		lw.w.WriteString("...\n[output truncated at 1MB]")
	}
	return len(p), err
}

// safeEnvPrefixes lists environment variable prefixes that are safe to pass to child processes.
var safeEnvPrefixes = []string{
	"PATH=", "HOME=", "USER=", "LOGNAME=", "SHELL=",
	"LANG=", "LC_", "TERM=", "TMPDIR=", "TMP=", "TEMP=",
	"GOPATH=", "GOROOT=", "GOBIN=", "GOPROXY=", "GOFLAGS=",
	"XDG_", "DISPLAY=", "SSH_AUTH_SOCK=", "EDITOR=", "VISUAL=",
	"COLORTERM=", "CLICOLOR=", "NO_COLOR=",
	"GIT_AUTHOR_", "GIT_COMMITTER_", "GIT_EDITOR=",
	"PWD=", "OLDPWD=", "SHLVL=", "HOSTNAME=",
}

// filteredEnv returns a copy of the environment with secret-bearing variables removed.
func filteredEnv() []string {
	var filtered []string
	for _, env := range os.Environ() {
		safe := false
		for _, prefix := range safeEnvPrefixes {
			if strings.HasPrefix(env, prefix) {
				safe = true
				break
			}
		}
		if safe {
			filtered = append(filtered, env)
		}
	}
	return filtered
}
