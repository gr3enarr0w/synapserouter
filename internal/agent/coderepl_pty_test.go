//go:build integration

package agent

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty/v2"
)

// synrouteBin returns the path to the built synroute binary.
// Tests must build first: go build -o synroute .
func synrouteBin() string {
	if p := os.Getenv("SYNROUTE_BIN"); p != "" {
		return p
	}
	return "../../synroute" // relative from internal/agent/
}

// startREPL launches synroute code in a PTY and returns the PTY master fd.
func startREPL(t *testing.T, env ...string) (*os.File, *exec.Cmd) {
	t.Helper()
	cmd := exec.Command(synrouteBin(), "code")
	cmd.Dir = t.TempDir()
	cmd.Env = append(os.Environ(), env...)
	ptmx, err := pty.Start(cmd)
	if err != nil {
		t.Fatalf("failed to start synroute in PTY: %v", err)
	}
	return ptmx, cmd
}

// readUntil reads from PTY until the target string appears or timeout.
func readUntil(ptmx *os.File, target string, timeout time.Duration) (string, bool) {
	var buf bytes.Buffer
	deadline := time.Now().Add(timeout)
	tmp := make([]byte, 4096)

	for time.Now().Before(deadline) {
		ptmx.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		n, _ := ptmx.Read(tmp)
		if n > 0 {
			buf.Write(tmp[:n])
			if strings.Contains(buf.String(), target) {
				return buf.String(), true
			}
		}
	}
	return buf.String(), false
}

func TestPTY_GreetingNoTools(t *testing.T) {
	ptmx, cmd := startREPL(t, "NO_COLOR=1")
	defer ptmx.Close()
	defer cmd.Process.Kill()

	// Wait for prompt
	output, ok := readUntil(ptmx, "synroute>", 15*time.Second)
	if !ok {
		t.Fatalf("prompt never appeared: %s", output)
	}

	// Send greeting
	ptmx.Write([]byte("hello\n"))

	// Read response — should NOT contain tool calls
	response, _ := readUntil(ptmx, "synroute>", 30*time.Second)
	if strings.Contains(response, "[bash]") || strings.Contains(response, "[file_read]") {
		t.Errorf("greeting triggered tool calls: %s", response)
	}

	// Exit
	ptmx.Write([]byte("/exit\n"))
	cmd.Wait()
}

func TestPTY_MultiTurn(t *testing.T) {
	ptmx, cmd := startREPL(t, "NO_COLOR=1")
	defer ptmx.Close()
	defer cmd.Process.Kill()

	readUntil(ptmx, "synroute>", 15*time.Second)

	// Turn 1
	ptmx.Write([]byte("hello\n"))
	_, ok := readUntil(ptmx, "synroute>", 30*time.Second)
	if !ok {
		t.Fatal("REPL didn't return to prompt after turn 1")
	}

	// Turn 2
	ptmx.Write([]byte("what is go\n"))
	_, ok = readUntil(ptmx, "synroute>", 30*time.Second)
	if !ok {
		t.Fatal("REPL didn't return to prompt after turn 2")
	}

	ptmx.Write([]byte("/exit\n"))
	cmd.Wait()
}

func TestPTY_SlashHelp(t *testing.T) {
	ptmx, cmd := startREPL(t, "NO_COLOR=1")
	defer ptmx.Close()
	defer cmd.Process.Kill()

	readUntil(ptmx, "synroute>", 15*time.Second)

	ptmx.Write([]byte("/help\n"))
	output, ok := readUntil(ptmx, "Ctrl-", 10*time.Second)
	if !ok {
		t.Errorf("/help didn't show keyboard shortcuts: %s", output)
	}

	ptmx.Write([]byte("/exit\n"))
	cmd.Wait()
}

func TestPTY_SlashExit(t *testing.T) {
	ptmx, cmd := startREPL(t, "NO_COLOR=1")
	defer ptmx.Close()
	defer cmd.Process.Kill()

	readUntil(ptmx, "synroute>", 15*time.Second)

	ptmx.Write([]byte("/exit\n"))
	output, _ := readUntil(ptmx, "bye", 5*time.Second)
	if !strings.Contains(output, "bye") {
		t.Errorf("/exit didn't produce 'bye': %s", output)
	}

	err := cmd.Wait()
	if err != nil {
		t.Errorf("synroute didn't exit cleanly: %v", err)
	}
}

func TestPTY_NoPermissionPrompt(t *testing.T) {
	ptmx, cmd := startREPL(t, "NO_COLOR=1")
	defer ptmx.Close()
	defer cmd.Process.Kill()

	readUntil(ptmx, "synroute>", 15*time.Second)

	// Ask for a file operation — should NOT prompt for permission in v1
	ptmx.Write([]byte("create a file called test.txt with hello\n"))
	output, _ := readUntil(ptmx, "synroute>", 60*time.Second)
	if strings.Contains(output, "Allow? [y/n/a]") {
		t.Errorf("permission prompt appeared but should be disabled: %s", output)
	}

	ptmx.Write([]byte("/exit\n"))
	cmd.Process.Kill()
}

func TestPTY_QueuedInputShowsQueuedMessage(t *testing.T) {
	ptmx, cmd := startREPL(t, "NO_COLOR=1")
	defer ptmx.Close()
	defer cmd.Process.Kill()

	readUntil(ptmx, "synroute>", 15*time.Second)

	ptmx.Write([]byte("hello\nwhat is go\n"))
	output, _ := readUntil(ptmx, "queued next message:", 30*time.Second)
	if !strings.Contains(output, "queued next message: what is go") {
		t.Fatalf("expected queued message indicator, got: %s", output)
	}

	ptmx.Write([]byte("/exit\n"))
	cmd.Process.Kill()
}

func TestPTY_ColorsEmitted(t *testing.T) {
	// Without NO_COLOR — should have ANSI codes
	ptmx, cmd := startREPL(t)
	defer ptmx.Close()
	defer cmd.Process.Kill()

	output, _ := readUntil(ptmx, "synroute>", 15*time.Second)
	if !strings.Contains(output, "\x1b[") {
		t.Errorf("no ANSI escape codes in output without NO_COLOR: %s", output)
	}

	ptmx.Write([]byte("/exit\n"))
	cmd.Process.Kill()
}

func TestPTY_NoColorMode(t *testing.T) {
	ptmx, cmd := startREPL(t, "NO_COLOR=1")
	defer ptmx.Close()
	defer cmd.Process.Kill()

	output, _ := readUntil(ptmx, "synroute>", 15*time.Second)
	if strings.Contains(output, "\x1b[") {
		t.Errorf("ANSI codes present with NO_COLOR=1: %s", output)
	}

	ptmx.Write([]byte("/exit\n"))
	cmd.Process.Kill()
}

func TestPTY_BannerContent(t *testing.T) {
	ptmx, cmd := startREPL(t, "NO_COLOR=1")
	defer ptmx.Close()
	defer cmd.Process.Kill()

	output, _ := readUntil(ptmx, "synroute>", 15*time.Second)

	checks := []string{"SynRoute", "tiers engaged", "/plan"}
	for _, check := range checks {
		if !strings.Contains(output, check) {
			t.Errorf("banner missing %q: %s", check, output)
		}
	}

	ptmx.Write([]byte("/exit\n"))
	cmd.Process.Kill()
}
