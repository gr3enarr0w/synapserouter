package agent

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// CodeREPL implements the code mode interactive loop with raw terminal input
// for keyboard shortcuts and pipeline-aware display.
type CodeREPL struct {
	agent    *Agent
	renderer *CodeRenderer
	terminal *Terminal
	in       io.Reader
	out      io.Writer

	// Raw mode state
	restore func()
	rawMode bool
	mu      sync.Mutex
	ctx     context.Context // parent context for phase commands
}

// NewCodeREPL creates a code mode REPL.
func NewCodeREPL(agent *Agent, renderer *CodeRenderer, terminal *Terminal) *CodeREPL {
	return &CodeREPL{
		agent:    agent,
		renderer: renderer,
		terminal: terminal,
		in:       os.Stdin,
		out:      os.Stdout,
	}
}

// Run starts the code mode REPL. Blocks until exit.
func (cr *CodeREPL) Run(ctx context.Context) error {
	cr.ctx = ctx

	// Initialize the screen layout
	cr.renderer.Init()

	// Detect spec/synroute.md in working directory
	cr.detectProjectFiles()

	// Set up resize handler
	stopResize := OnResize(func(w, h int) {
		cr.renderer.Resize(w, h)
	})
	defer stopResize()

	// Handle Ctrl-C — cancel current request, don't exit
	var reqMu sync.Mutex
	var cancelFn context.CancelFunc
	newReqCtx := func() context.Context {
		reqMu.Lock()
		defer reqMu.Unlock()
		if cancelFn != nil {
			cancelFn()
		}
		var reqCtx context.Context
		reqCtx, cancelFn = context.WithCancel(ctx)
		return reqCtx
	}
	defer func() {
		reqMu.Lock()
		if cancelFn != nil {
			cancelFn()
		}
		reqMu.Unlock()
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT)
	defer signal.Stop(sigCh)

	go func() {
		for range sigCh {
			reqMu.Lock()
			if cancelFn != nil {
				cancelFn()
			}
			reqMu.Unlock()
			cr.renderer.mu.Lock()
			cr.renderer.writeContent("")
			cr.renderer.writeContent(cr.renderer.color("\033[33m", "  (interrupted)"))
			cr.renderer.writeContent("")
			cr.renderer.mu.Unlock()
		}
	}()

	// Use cooked mode (normal terminal input) with TUI chrome.
	// Raw mode keyboard shortcuts are available via slash commands instead:
	// /plan, /review, /check, /fix, /help
	scanner := bufio.NewScanner(cr.in)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for {
		// Show prompt
		cr.renderer.Prompt()

		// Read input in normal cooked mode
		if !scanner.Scan() {
			// EOF (Ctrl-D)
			cr.renderer.Cleanup()
			fmt.Fprintln(cr.out, "bye")
			return nil
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		// Handle REPL commands
		if strings.HasPrefix(input, "/") {
			if done := cr.handleCommand(input); done {
				cr.renderer.Cleanup()
				return nil
			}
			continue
		}

		// Create fresh context for the request
		reqCtx := newReqCtx()

		// Run the agent
		cr.renderer.mu.Lock()
		cr.renderer.writeContent("")
		cr.renderer.mu.Unlock()

		response, err := cr.agent.Run(reqCtx, input)
		if err != nil {
			cr.renderer.Error(err.Error())
			continue
		}

		if response != "" {
			cr.renderer.mu.Lock()
			cr.renderer.writeContent("")
			cr.renderer.writeContent(response)
			cr.renderer.writeContent("")
			cr.renderer.mu.Unlock()
		}
	}
}

// readInput handles raw mode key reading. Returns the user's input line,
// whether to exit, and any error.
func (cr *CodeREPL) readInput() (string, bool, error) {
	for {
		if !cr.rawMode {
			// Fallback to line reading
			return cr.readLine()
		}

		key, err := cr.terminal.ReadKey()
		if err != nil {
			return "", false, err
		}

		if key.Ctrl {
			switch key.Rune {
			case 'd': // Ctrl-D — exit
				return "", true, nil
			case 'c': // Ctrl-C — handled by signal handler
				continue
			case 'p': // Ctrl-P — pipeline status
				fmt.Fprintln(cr.out) // newline after prompt
				cr.renderer.ShowPipeline()
				cr.renderer.Prompt()
				continue
			case 't': // Ctrl-T — recent tools
				fmt.Fprintln(cr.out)
				cr.renderer.ShowRecentTools()
				cr.renderer.Prompt()
				continue
			case 'l': // Ctrl-L — cycle verbosity
				fmt.Fprintln(cr.out)
				cr.renderer.mu.Lock()
				newLevel := (cr.renderer.verbosity + 1) % 3
				cr.renderer.mu.Unlock()
				cr.renderer.SetVerbosity(newLevel)
				cr.renderer.Prompt()
				continue
			case 'e': // Ctrl-E — force escalation
				fmt.Fprintln(cr.out)
				cr.renderer.mu.Lock()
				cr.renderer.writeContent(cr.renderer.color("\033[33m", "  forcing provider escalation..."))
				cr.renderer.mu.Unlock()
				cr.agent.ForceEscalate()
				cr.renderer.Prompt()
				continue
			case '/': // Ctrl-/ — help
				fmt.Fprintln(cr.out)
				cr.renderer.ShowHelp()
				cr.renderer.Prompt()
				continue
			}
		}

		// User started typing — switch to cooked mode for full line input
		cr.exitRawMode()

		// Echo the first character, then read the rest of the line
		fmt.Fprintf(cr.out, "%c", key.Rune)
		line, eof := cr.readRestOfLine()
		if eof {
			return "", true, nil
		}

		fullInput := string(key.Rune) + line
		return strings.TrimSpace(fullInput), false, nil
	}
}

// readLine reads a full line in cooked mode.
func (cr *CodeREPL) readLine() (string, bool, error) {
	scanner := bufio.NewScanner(cr.in)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	if !scanner.Scan() {
		return "", true, nil
	}
	return strings.TrimSpace(scanner.Text()), false, nil
}

// readRestOfLine reads the remainder of a line after the first character.
func (cr *CodeREPL) readRestOfLine() (string, bool) {
	scanner := bufio.NewScanner(cr.in)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	if !scanner.Scan() {
		return "", true
	}
	return scanner.Text(), false
}

func (cr *CodeREPL) enterRawMode() {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	if cr.rawMode {
		return
	}
	restore, err := cr.terminal.MakeRaw()
	if err != nil {
		// Fall back to cooked mode
		return
	}
	cr.restore = restore
	cr.rawMode = true
}

func (cr *CodeREPL) exitRawMode() {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	if !cr.rawMode {
		return
	}
	if cr.restore != nil {
		cr.restore()
	}
	cr.rawMode = false
}

// detectProjectFiles checks the working directory for spec and synroute.md files
// on startup and displays what was found. If neither exists, prompts the user to
// create one.
func (cr *CodeREPL) detectProjectFiles() {
	workDir := cr.agent.config.WorkDir

	var specFiles []string
	var synrouteMD string

	// Check for spec files: spec.md, SPEC.md, *.spec.md
	candidates := []string{"spec.md", "SPEC.md"}
	for _, name := range candidates {
		path := filepath.Join(workDir, name)
		if _, err := os.Stat(path); err == nil {
			specFiles = append(specFiles, name)
		}
	}

	// Glob for *.spec.md
	if matches, err := filepath.Glob(filepath.Join(workDir, "*.spec.md")); err == nil {
		for _, m := range matches {
			base := filepath.Base(m)
			// Avoid duplicates with the explicit candidates
			if base != "spec.md" && base != "SPEC.md" {
				specFiles = append(specFiles, base)
			}
		}
	}

	// Check for synroute.md
	synroutePath := filepath.Join(workDir, "synroute.md")
	if _, err := os.Stat(synroutePath); err == nil {
		synrouteMD = "synroute.md"
	}

	cr.renderer.mu.Lock()
	defer cr.renderer.mu.Unlock()

	if len(specFiles) > 0 || synrouteMD != "" {
		// Display what was found
		if len(specFiles) > 0 {
			cr.renderer.writeContent(cr.renderer.color("\033[32m", fmt.Sprintf("  spec found: %s", strings.Join(specFiles, ", "))))
		}
		if synrouteMD != "" {
			cr.renderer.writeContent(cr.renderer.color("\033[32m", fmt.Sprintf("  project state: %s", synrouteMD)))
		}
		cr.renderer.writeContent("")
		return
	}

	// Neither found — prompt the user
	cr.renderer.writeContent(cr.renderer.color("\033[33m", "  No spec or synroute.md found in this directory."))
	cr.renderer.writeContent(cr.renderer.color("\033[33m", "  Create one? [spec/synroute/no]: "))

	// Read user choice (cooked mode, scanner on stdin)
	cr.renderer.mu.Unlock() // unlock before blocking on input
	scanner := bufio.NewScanner(cr.in)
	scanner.Buffer(make([]byte, 0, 1024), 4096)
	var choice string
	if scanner.Scan() {
		choice = strings.TrimSpace(strings.ToLower(scanner.Text()))
	}
	cr.renderer.mu.Lock() // re-lock for deferred unlock

	switch choice {
	case "spec":
		cr.createSpecFile(workDir)
	case "synroute":
		cr.createSynrouteFile(workDir)
	default:
		// "no", empty, or anything else — skip
	}
}

// createSpecFile generates a minimal spec.md in the given directory.
func (cr *CodeREPL) createSpecFile(workDir string) {
	projectName := filepath.Base(workDir)
	content := fmt.Sprintf(`# %s — Specification

## Overview
<!-- Describe what this project does -->

## Requirements
<!-- List the requirements -->

## Acceptance Criteria
<!-- Define how to verify the implementation is correct -->
`, projectName)

	path := filepath.Join(workDir, "spec.md")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		cr.renderer.writeContent(cr.renderer.color("\033[31m", fmt.Sprintf("  error creating spec.md: %v", err)))
		return
	}
	cr.renderer.writeContent(cr.renderer.color("\033[32m", fmt.Sprintf("  created %s — edit it to define your project spec", path)))
	cr.renderer.writeContent("")
}

// createSynrouteFile generates a minimal synroute.md project state file.
func (cr *CodeREPL) createSynrouteFile(workDir string) {
	projectName := filepath.Base(workDir)
	now := time.Now().Format("2006-01-02 15:04")
	content := fmt.Sprintf(`# synroute.md — Project State

## Project
- Name: %s
- Created: %s

## Status
- Last run: %s
- Tool calls: 0
- Provider level: 0
- Escalation cycles: 0

## Original Request

`, projectName, now, now)

	path := filepath.Join(workDir, "synroute.md")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		cr.renderer.writeContent(cr.renderer.color("\033[31m", fmt.Sprintf("  error creating synroute.md: %v", err)))
		return
	}
	cr.renderer.writeContent(cr.renderer.color("\033[32m", fmt.Sprintf("  created %s — project state will be tracked here", path)))
	cr.renderer.writeContent("")
}

func (cr *CodeREPL) handleCommand(input string) bool {
	parts := strings.Fields(input)
	cmd := parts[0]

	switch cmd {
	case "/exit", "/quit":
		cr.renderer.Cleanup()
		fmt.Fprintln(cr.out, "bye")
		return true

	case "/clear":
		cr.agent.Clear()
		cr.renderer.mu.Lock()
		cr.renderer.writeContent(cr.renderer.color("\033[2m", "  conversation cleared"))
		cr.renderer.mu.Unlock()

	case "/model":
		if len(parts) < 2 {
			cr.renderer.mu.Lock()
			cr.renderer.writeContent(fmt.Sprintf("  current model: %s", cr.agent.config.Model))
			cr.renderer.mu.Unlock()
		} else {
			cr.agent.config.Model = parts[1]
			cr.renderer.mu.Lock()
			cr.renderer.writeContent(fmt.Sprintf("  model set to: %s", parts[1]))
			cr.renderer.mu.Unlock()
		}

	case "/tools":
		names := cr.agent.registry.List()
		cr.renderer.mu.Lock()
		cr.renderer.writeContent(fmt.Sprintf("  Available tools (%d):", len(names)))
		for _, name := range names {
			tool, _ := cr.agent.registry.Get(name)
			cr.renderer.writeContent(fmt.Sprintf("    %-15s [%s] %s", name, tool.Category(), tool.Description()))
		}
		cr.renderer.mu.Unlock()

	case "/history":
		msgs := cr.agent.conversation.Messages()
		cr.renderer.mu.Lock()
		cr.renderer.writeContent(fmt.Sprintf("  Conversation history (%d messages):", len(msgs)))
		for i, msg := range msgs {
			content := msg.Content
			if len(content) > 80 {
				content = content[:80] + "..."
			}
			cr.renderer.writeContent(fmt.Sprintf("    %3d. [%s] %s", i+1, msg.Role, content))
		}
		cr.renderer.mu.Unlock()

	case "/agents":
		children := cr.agent.Children()
		cr.renderer.mu.Lock()
		if len(children) == 0 {
			cr.renderer.writeContent("  No sub-agents spawned")
		} else {
			cr.renderer.writeContent(fmt.Sprintf("  Sub-agents (%d):", len(children)))
			for _, c := range children {
				cr.renderer.writeContent(fmt.Sprintf("    %-20s [%s] %s", c.ID, c.Role, c.Status))
			}
		}
		cr.renderer.mu.Unlock()

	case "/budget":
		cr.renderer.mu.Lock()
		if cr.agent.budget == nil {
			cr.renderer.writeContent("  No budget configured")
		} else {
			snap := cr.agent.budget.Snapshot()
			cr.renderer.writeContent("  Budget usage:")
			turnLine := fmt.Sprintf("    Turns:   %d", snap.Turns)
			if snap.Budget.MaxTurns > 0 {
				turnLine += fmt.Sprintf(" / %d", snap.Budget.MaxTurns)
			}
			cr.renderer.writeContent(turnLine)
			tokenLine := fmt.Sprintf("    Tokens:  %d", snap.Tokens)
			if snap.Budget.MaxTokens > 0 {
				tokenLine += fmt.Sprintf(" / %d", snap.Budget.MaxTokens)
			}
			cr.renderer.writeContent(tokenLine)
			elapsedLine := fmt.Sprintf("    Elapsed: %s", snap.Elapsed.Round(time.Second))
			if snap.Budget.MaxDuration > 0 {
				elapsedLine += fmt.Sprintf(" / %s", snap.Budget.MaxDuration)
			}
			cr.renderer.writeContent(elapsedLine)
		}
		cr.renderer.mu.Unlock()

	case "/plan":
		msg := strings.TrimSpace(strings.TrimPrefix(input, "/plan"))
		if msg == "" {
			msg = "Generate a plan with acceptance criteria for the current project"
		}
		cr.renderer.mu.Lock()
		cr.renderer.writeContent(cr.renderer.color("\033[35m", "  running plan phase..."))
		cr.renderer.mu.Unlock()
		response, err := cr.agent.RunPhase(cr.ctx, "plan", msg)
		if err != nil {
			cr.renderer.Error(err.Error())
		} else if response != "" {
			cr.renderer.Text(response)
		}

	case "/review":
		msg := strings.TrimSpace(strings.TrimPrefix(input, "/review"))
		if msg == "" {
			msg = "Review the code in the current working directory. Read files, run tests, identify issues."
		}
		cr.renderer.mu.Lock()
		cr.renderer.writeContent(cr.renderer.color("\033[35m", "  running code review..."))
		cr.renderer.mu.Unlock()
		response, err := cr.agent.RunPhase(cr.ctx, "code-review", msg)
		if err != nil {
			cr.renderer.Error(err.Error())
		} else if response != "" {
			cr.renderer.Text(response)
		}

	case "/check":
		msg := strings.TrimSpace(strings.TrimPrefix(input, "/check"))
		if msg == "" {
			msg = "Run verification: build the code, run tests, check against acceptance criteria."
		}
		cr.renderer.mu.Lock()
		cr.renderer.writeContent(cr.renderer.color("\033[35m", "  running self-check..."))
		cr.renderer.mu.Unlock()
		response, err := cr.agent.RunPhase(cr.ctx, "self-check", msg)
		if err != nil {
			cr.renderer.Error(err.Error())
		} else if response != "" {
			cr.renderer.Text(response)
		}

	case "/fix":
		msg := strings.TrimSpace(strings.TrimPrefix(input, "/fix"))
		if msg == "" {
			cr.renderer.mu.Lock()
			cr.renderer.writeContent("  usage: /fix <description of what to fix>")
			cr.renderer.mu.Unlock()
		} else {
			cr.renderer.mu.Lock()
			cr.renderer.writeContent(cr.renderer.color("\033[35m", "  running targeted fix..."))
			cr.renderer.mu.Unlock()
			response, err := cr.agent.RunPhase(cr.ctx, "implement", msg)
			if err != nil {
				cr.renderer.Error(err.Error())
			} else if response != "" {
				cr.renderer.Text(response)
			}
		}

	case "/help":
		cr.renderer.ShowHelp()

	default:
		cr.renderer.mu.Lock()
		cr.renderer.writeContent(fmt.Sprintf("  unknown command: %s (type /help)", cmd))
		cr.renderer.mu.Unlock()
	}
	return false
}
