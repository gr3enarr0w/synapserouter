package agent

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ergochat/readline"
)

// CodeREPL implements the code mode interactive loop with readline-based input
// for history, tab completion, and proper terminal handling.
type CodeREPL struct {
	agent    *Agent
	renderer *CodeRenderer
	out      io.Writer
	ctx      context.Context // parent context for phase commands
}

// NewCodeREPL creates a code mode REPL. The Terminal parameter is accepted
// for API compatibility but no longer used — readline handles terminal mode.
func NewCodeREPL(agent *Agent, renderer *CodeRenderer, _ *Terminal) *CodeREPL {
	return &CodeREPL{
		agent:    agent,
		renderer: renderer,
		out:      os.Stdout,
	}
}

// slashCommands returns the list of available slash commands for tab completion.
func slashCommands() []string {
	return []string{
		"/plan", "/review", "/check", "/fix", "/help",
		"/exit", "/clear", "/model", "/tools", "/history",
		"/agents", "/budget",
	}
}

// Run starts the code mode REPL. Blocks until exit.
func (cr *CodeREPL) Run(ctx context.Context) error {
	cr.ctx = ctx

	// Initialize the screen layout (prints launch banner)
	cr.renderer.Init()

	// Detect spec/synroute.md in working directory
	cr.detectProjectFiles()

	// Set up resize handler
	stopResize := OnResize(func(w, h int) {
		cr.renderer.Resize(w, h)
	})
	defer stopResize()

	// Multi-mode Ctrl-C handling:
	// 1. During LLM call: cancel current request, return to prompt
	// 2. At empty prompt: first press shows message, rapid second press exits
	// 3. At prompt with text: clears the line (handled by readline)
	var reqMu sync.Mutex
	var cancelFn context.CancelFunc
	var agentRunning bool // true while agent.Run is executing
	var ctrlCCount int    // consecutive Ctrl-C presses at idle prompt
	var lastCtrlC time.Time
	var eofRetries int    // spurious EOF retries (readline cursor position leak)

	newReqCtx := func() context.Context {
		reqMu.Lock()
		defer reqMu.Unlock()
		if cancelFn != nil {
			cancelFn()
		}
		var reqCtx context.Context
		reqCtx, cancelFn = context.WithCancel(ctx)
		agentRunning = true
		ctrlCCount = 0
		return reqCtx
	}
	markDone := func() {
		reqMu.Lock()
		agentRunning = false
		reqMu.Unlock()
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

	exitCh := make(chan struct{})

	// Set up history file
	home, _ := os.UserHomeDir()
	historyDir := filepath.Join(home, ".synroute")
	os.MkdirAll(historyDir, 0755)
	historyFile := filepath.Join(historyDir, "history")

	// Build tab completer for slash commands
	cmds := slashCommands()
	completionItems := make([]*readline.PrefixCompleter, len(cmds))
	for i, cmd := range cmds {
		completionItems[i] = readline.PcItem(cmd)
	}

	rl, err := readline.NewEx(&readline.Config{
		Prompt:          "\033[32msynroute>\033[0m ",
		HistoryFile:     historyFile,
		AutoComplete:    readline.NewPrefixCompleter(completionItems...),
		InterruptPrompt: "",  // empty = return ErrInterrupt (don't just print ^C and loop)
		EOFPrompt:       "bye",
	})
	if err != nil {
		return fmt.Errorf("readline init: %w", err)
	}
	defer rl.Close()

	// Start signal handler goroutine AFTER readline is created (needs rl reference)
	go func() {
		for range sigCh {
			reqMu.Lock()
			running := agentRunning
			reqMu.Unlock()

			if running {
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
			} else {
				now := time.Now()
				if now.Sub(lastCtrlC) < 2*time.Second {
					ctrlCCount++
				} else {
					ctrlCCount = 1
				}
				lastCtrlC = now

				if ctrlCCount >= 2 {
					cr.renderer.mu.Lock()
					cr.renderer.writeContent("")
					cr.renderer.writeContent("bye")
					cr.renderer.mu.Unlock()
					rl.Close()
					close(exitCh)
					return
				}
				cr.renderer.mu.Lock()
				cr.renderer.writeContent(cr.renderer.color("\033[2m", "  (press Ctrl-C again to exit, or type /exit)"))
				cr.renderer.mu.Unlock()
			}
		}
	}()

	for {
		// Check if double Ctrl-C exit was triggered
		select {
		case <-exitCh:
			return nil
		default:
		}

		line, err := rl.Readline()
		if err != nil {
			// readline.ErrInterrupt = Ctrl-C at prompt
			if err == readline.ErrInterrupt {
				now := time.Now()
				if now.Sub(lastCtrlC) < 2*time.Second {
					ctrlCCount++
				} else {
					ctrlCCount = 1
				}
				lastCtrlC = now

				if ctrlCCount >= 2 {
					fmt.Fprintln(cr.out, "\nbye")
					return nil
				}
				cr.renderer.mu.Lock()
				cr.renderer.writeContent(cr.renderer.color("\033[2m", "  (press Ctrl-C again to exit, or type /exit)"))
				cr.renderer.mu.Unlock()
				continue
			}
			// EOF — could be real (Ctrl-D) or spurious (cursor position response
			// from readline's terminal queries leaking as \033[row;colR).
			// Only exit on real EOF: if the agent has been used, retry once.
			if eofRetries < 2 && cr.agent != nil {
				eofRetries++
				log.Printf("[REPL] spurious EOF (retry %d/2) — readline cursor response leak", eofRetries)
				// Recreate readline to reset terminal state
				rl.Close()
				rl, err = readline.NewEx(&readline.Config{
					Prompt:       "\033[32msynroute>\033[0m ",
					HistoryFile:  historyFile,
					AutoComplete: readline.NewPrefixCompleter(completionItems...),
					InterruptPrompt: "",
					EOFPrompt:    "bye",
				})
				if err != nil {
					return fmt.Errorf("readline reinit: %w", err)
				}
				continue
			}
			fmt.Fprintln(cr.out, "bye")
			return nil
		}
		ctrlCCount = 0 // reset on any normal input
		eofRetries = 0 // reset on successful read

		input := strings.TrimSpace(line)
		if input == "" {
			continue
		}

		// Handle keyboard shortcuts (Ctrl-key combos sent as single chars)
		if len(input) == 1 {
			switch input[0] {
			case 0x10: // ^P — pipeline status
				cr.renderer.ShowPipeline()
				continue
			case 0x14: // ^T — recent tools
				cr.renderer.ShowRecentTools()
				continue
			case 0x0C: // ^L — cycle verbosity
				v := (cr.renderer.verbosity + 1) % 3
				cr.renderer.SetVerbosity(v)
				continue
			case 0x05: // ^E — force escalation
				if cr.agent.ForceEscalate() {
					cr.renderer.mu.Lock()
					cr.renderer.writeContent(cr.renderer.color("\033[33m", "  escalated to next tier"))
					cr.renderer.mu.Unlock()
				} else {
					cr.renderer.mu.Lock()
					cr.renderer.writeContent(cr.renderer.color("\033[2m", "  already at highest tier"))
					cr.renderer.mu.Unlock()
				}
				continue
			}
		}
		// ^/ — help (0x1F)
		if len(input) == 1 && input[0] == 0x1F {
			cr.renderer.ShowHelp()
			continue
		}

		// Handle REPL slash commands
		if strings.HasPrefix(input, "/") {
			if done := cr.handleCommand(input); done {
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
		markDone()
		if err != nil {
			// Don't show context cancelled as an error — just return to prompt
			if reqCtx.Err() != nil {
				continue
			}
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

// detectProjectFiles checks the working directory for spec and synroute.md files
// on startup and displays what was found.
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

	if len(specFiles) > 0 {
		cr.renderer.writeContent(cr.renderer.color("\033[32m", fmt.Sprintf("  spec found: %s", strings.Join(specFiles, ", "))))
	}
	if synrouteMD != "" {
		cr.renderer.writeContent(cr.renderer.color("\033[32m", fmt.Sprintf("  project state: %s", synrouteMD)))
	}
	if len(specFiles) > 0 || synrouteMD != "" {
		cr.renderer.writeContent("")
	}
}

func (cr *CodeREPL) handleCommand(input string) bool {
	parts := strings.Fields(input)
	cmd := parts[0]

	switch cmd {
	case "/exit", "/quit":
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
