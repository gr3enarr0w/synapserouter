package agent

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
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
