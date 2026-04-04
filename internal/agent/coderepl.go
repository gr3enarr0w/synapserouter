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

	"golang.org/x/term"

	"github.com/gr3enarr0w/synapserouter/internal/security"
)

// CodeREPL implements the code mode interactive loop with readline-based input
// for history, tab completion, and proper terminal handling.
type CodeREPL struct {
	agent    *Agent
	renderer *CodeRenderer
	out      io.Writer
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

// Run starts the code mode REPL. Blocks until exit.
func (cr *CodeREPL) Run(ctx context.Context) error {
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

	// Set up history file (for future use)
	home, _ := os.UserHomeDir()
	historyDir := filepath.Join(home, ".synroute")
	os.MkdirAll(historyDir, 0755)

	// Put terminal in raw mode for character-at-a-time input (keyboard shortcuts).
	// Raw mode lets us intercept Ctrl-L, Ctrl-P, etc. before the terminal processes them.
	stdinFd := int(os.Stdin.Fd()) //nolint:G115 // os.Stdin.Fd() always fits in int on supported platforms
	isTerminal := term.IsTerminal(stdinFd)

	var restoreTerminal func()
	if isTerminal {
		oldState, err := term.MakeRaw(stdinFd)
		if err != nil {
			log.Printf("[REPL] failed to enter raw mode: %v — falling back to cooked mode", err)
			isTerminal = false
		} else {
			restoreTerminal = func() { _ = term.Restore(stdinFd, oldState) }
			defer restoreTerminal()
		}
	}

	// Start signal handler goroutine for Ctrl-C
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
				if now.Sub(lastCtrlC) <= 2*time.Second {
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
					close(exitCh)
					return
				}
				cr.renderer.mu.Lock()
				cr.renderer.writeContent(cr.renderer.color("\033[2m", "  (press Ctrl-C again to exit, or type /exit)"))
				cr.renderer.mu.Unlock()
			}
		}
	}()

	noColor := os.Getenv("NO_COLOR") != ""

	for {
		// Check if double Ctrl-C exit was triggered
		select {
		case <-exitCh:
			return nil
		default:
		}

		// Print prompt (in raw mode we must use \r\n for newlines)
		if noColor {
			fmt.Fprint(cr.out, "synroute> ")
		} else {
			fmt.Fprint(cr.out, "\033[32msynroute>\033[0m ")
		}

		// Read a line of input (character-at-a-time in raw mode)
		input, eof := cr.readLine(stdinFd, isTerminal, exitCh, &reqMu, &agentRunning, cancelFn, &ctrlCCount, &lastCtrlC)
		if eof {
			cr.rawWrite("\r\nbye\r\n")
			return nil
		}

		log.Printf("[REPL] got input: %q", input)

		if input == "" {
			cr.rawWrite("\r\n")
			continue
		}

		cr.rawWrite("\r\n")

		// Handle REPL slash commands
		if strings.HasPrefix(input, "/") {
			// Temporarily restore terminal for agent output
			if restoreTerminal != nil {
				restoreTerminal()
			}
			if done := cr.handleCommand(ctx, input); done {
				return nil
			}
			// Re-enter raw mode
			if isTerminal {
				if oldState, err := term.MakeRaw(stdinFd); err == nil {
					restoreTerminal = func() { _ = term.Restore(stdinFd, oldState) }
				}
			}
			continue
		}

		// Temporarily restore terminal for agent execution (tools use cooked mode)
		if restoreTerminal != nil {
			restoreTerminal()
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
			if reqCtx.Err() != nil {
				// Re-enter raw mode before next prompt
				if isTerminal {
					if oldState, err := term.MakeRaw(stdinFd); err == nil {
						restoreTerminal = func() { _ = term.Restore(stdinFd, oldState) }
					}
				}
				continue
			}
			cr.renderer.Error(err.Error())
		}

		log.Printf("[REPL] response: len=%d, streaming=%v, eventBus=%v",
			len(response), cr.agent.config.Streaming, cr.agent.config.EventBus != nil)

		if response != "" {
			cr.renderer.mu.Lock()
			// Always display the response. If tokens were already streamed via
			// EventTokenStream, the content is already on screen — just add spacing.
			// If streaming didn't fire (non-streaming provider, cached response, or
			// fast return), we need to print the full response here.
			if cr.agent.config.Streaming && cr.agent.config.EventBus != nil && cr.agent.lastResponseStreamed {
				cr.renderer.writeContent("")
			} else {
				cr.renderer.writeContent("")
				cr.renderer.writeContent(response)
				cr.renderer.writeContent("")
			}
			cr.renderer.mu.Unlock()
		} else if err == nil {
			log.Printf("[REPL] warning: agent returned empty response with no error")
		}

		// Re-enter raw mode for next prompt
		if isTerminal {
			if oldState, err := term.MakeRaw(stdinFd); err == nil {
				restoreTerminal = func() { _ = term.Restore(stdinFd, oldState) }
			}
		}
	}
}

// rawWrite writes directly to the output, used during raw mode where \r\n is needed.
func (cr *CodeREPL) rawWrite(s string) {
	fmt.Fprint(cr.out, s)
}

// readLine reads a line of input character-by-character in raw terminal mode.
// Handles keyboard shortcuts (Ctrl-L/P/T/E) inline, returns the completed line on Enter.
// Returns (line, eof). If eof is true, the user pressed Ctrl-D or the terminal closed.
func (cr *CodeREPL) readLine(fd int, isRaw bool, exitCh chan struct{}, reqMu *sync.Mutex, agentRunning *bool, cancelFn context.CancelFunc, ctrlCCount *int, lastCtrlC *time.Time) (string, bool) {
	if !isRaw {
		// Fallback: cooked mode (piped input) — read a line from stdin
		buf := make([]byte, 64*1024)
		n, err := os.Stdin.Read(buf)
		if err != nil || n == 0 {
			return "", true
		}
		line := strings.TrimRight(string(buf[:n]), "\r\n")
		return strings.TrimSpace(line), false
	}

	// Raw mode: read byte by byte
	var lineBuf []byte
	buf := make([]byte, 1)

	for {
		// Check exit channel
		select {
		case <-exitCh:
			return "", true
		default:
		}

		n, err := os.Stdin.Read(buf)
		if err != nil || n == 0 {
			return "", true
		}

		b := buf[0]

		switch {
		case b == 0x0D || b == 0x0A: // Enter (CR or LF)
			return strings.TrimSpace(string(lineBuf)), false

		case b == 0x04: // Ctrl-D (EOF)
			if len(lineBuf) == 0 {
				return "", true
			}
			// If there's text, ignore Ctrl-D (like bash)

		case b == 0x03: // Ctrl-C
			if len(lineBuf) > 0 {
				// Text on line: clear it
				cr.rawWrite("\r\033[K")
				lineBuf = lineBuf[:0]
				cr.rawWrite(cr.renderer.color("\033[36m", "synroute> "))
				continue
			}
			// Empty line: track double-tap for exit
			now := time.Now()
			if now.Sub(*lastCtrlC) <= 2*time.Second {
				*ctrlCCount++
			} else {
				*ctrlCCount = 1
			}
			*lastCtrlC = now

			if *ctrlCCount >= 2 {
				cr.rawWrite("\r\n")
				cr.rawWrite("bye\r\n")
				return "", true // exit signal
			}
			// First Ctrl-C: show hint
			cr.rawWrite("\r\n")
			cr.rawWrite(cr.renderer.color("\033[2m", "  (press Ctrl-C again to exit)"))
			cr.rawWrite("\r\n")
			cr.rawWrite(cr.renderer.color("\033[36m", "synroute> "))
			continue

		case b == 0x7F || b == 0x08: // Backspace (DEL or BS)
			if len(lineBuf) > 0 {
				lineBuf = lineBuf[:len(lineBuf)-1]
				cr.rawWrite("\b \b") // erase last char visually
			}

		case b == 0x0C: // Ctrl-L — cycle verbosity
			v := (cr.renderer.verbosity + 1) % 3
			cr.renderer.SetVerbosity(v)
			labels := []string{"compact", "normal", "verbose"}
			cr.rawWrite("\r\n")
			cr.renderer.mu.Lock()
			cr.renderer.writeContent(cr.renderer.color("\033[33m", "  verbosity: "+labels[v]))
			cr.renderer.mu.Unlock()
			// Reprint prompt with current input
			cr.rawWrite("\r\n")
			noColor := os.Getenv("NO_COLOR") != ""
			if noColor {
				cr.rawWrite("synroute> ")
			} else {
				cr.rawWrite("\033[32msynroute>\033[0m ")
			}
			cr.rawWrite(string(lineBuf))

		case b == 0x10: // Ctrl-P — pipeline status
			cr.rawWrite("\r\n")
			cr.renderer.ShowPipeline()
			cr.rawWrite("\r\n")
			noColor := os.Getenv("NO_COLOR") != ""
			if noColor {
				cr.rawWrite("synroute> ")
			} else {
				cr.rawWrite("\033[32msynroute>\033[0m ")
			}
			cr.rawWrite(string(lineBuf))

		case b == 0x14: // Ctrl-T — recent tools
			cr.rawWrite("\r\n")
			cr.renderer.ShowRecentTools()
			cr.rawWrite("\r\n")
			noColor := os.Getenv("NO_COLOR") != ""
			if noColor {
				cr.rawWrite("synroute> ")
			} else {
				cr.rawWrite("\033[32msynroute>\033[0m ")
			}
			cr.rawWrite(string(lineBuf))

		case b == 0x05: // Ctrl-E — force escalation
			cr.rawWrite("\r\n")
			if cr.agent.ForceEscalate() {
				cr.renderer.mu.Lock()
				cr.renderer.writeContent(cr.renderer.color("\033[33m", "  escalated to next tier"))
				cr.renderer.mu.Unlock()
			} else {
				cr.renderer.mu.Lock()
				cr.renderer.writeContent(cr.renderer.color("\033[2m", "  already at highest tier"))
				cr.renderer.mu.Unlock()
			}
			cr.rawWrite("\r\n")
			noColor := os.Getenv("NO_COLOR") != ""
			if noColor {
				cr.rawWrite("synroute> ")
			} else {
				cr.rawWrite("\033[32msynroute>\033[0m ")
			}
			cr.rawWrite(string(lineBuf))

		case b == 0x1F: // Ctrl-/ — help
			cr.rawWrite("\r\n")
			cr.renderer.ShowHelp()
			cr.rawWrite("\r\n")
			noColor := os.Getenv("NO_COLOR") != ""
			if noColor {
				cr.rawWrite("synroute> ")
			} else {
				cr.rawWrite("\033[32msynroute>\033[0m ")
			}
			cr.rawWrite(string(lineBuf))

		case b == 0x15: // Ctrl-U — clear line
			// Erase the visible line
			cr.rawWrite("\r\033[K")
			noColor := os.Getenv("NO_COLOR") != ""
			if noColor {
				cr.rawWrite("synroute> ")
			} else {
				cr.rawWrite("\033[32msynroute>\033[0m ")
			}
			lineBuf = lineBuf[:0]

		case b == 0x1B: // ESC — start of escape sequence, consume and ignore
			// Read the rest of the escape sequence (e.g. arrow keys: ESC [ A)
			seq := make([]byte, 2)
			_, _ = os.Stdin.Read(seq) // ignore arrow keys for now

		case b >= 0x20 && b < 0x7F: // Printable ASCII
			lineBuf = append(lineBuf, b)
			cr.rawWrite(string([]byte{b})) // echo the char

		default:
			// Ignore other control characters
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

func (cr *CodeREPL) handleCommand(ctx context.Context, input string) bool {
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
		response, err := cr.agent.RunPhase(ctx, "plan", msg)
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
		response, err := cr.agent.RunPhase(ctx, "code-review", msg)
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
		response, err := cr.agent.RunPhase(ctx, "self-check", msg)
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
			response, err := cr.agent.RunPhase(ctx, "implement", msg)
			if err != nil {
				cr.renderer.Error(err.Error())
			} else if response != "" {
				cr.renderer.Text(response)
			}
		}

	case "/research":
		msg := strings.TrimSpace(strings.TrimPrefix(input, "/research"))
		if msg == "" {
			cr.renderer.mu.Lock()
			cr.renderer.writeContent("  usage: /research [quick|standard|deep] <query>")
			cr.renderer.mu.Unlock()
		} else {
			// Parse optional depth prefix
			depth := "standard"
			parts := strings.SplitN(msg, " ", 2)
			switch parts[0] {
			case "quick", "standard", "deep":
				depth = parts[0]
				if len(parts) > 1 {
					msg = parts[1]
				} else {
					cr.renderer.mu.Lock()
					cr.renderer.writeContent("  usage: /research [quick|standard|deep] <query>")
					cr.renderer.mu.Unlock()
					break
				}
			}

			config := DefaultResearchConfig(depth)
			cr.renderer.mu.Lock()
			cr.renderer.writeContent(cr.renderer.color("\033[35m",
				fmt.Sprintf("  researching (%s): %d rounds, %d max queries, %d budget...",
					depth, config.MaxRounds, config.MaxQueries, config.MaxAPICalls)))
			cr.renderer.mu.Unlock()

			report, err := cr.agent.RunResearch(ctx, msg, depth)
			if err != nil {
				cr.renderer.Error(err.Error())
			} else if report != nil {
				cr.renderer.Text(report.Findings)
			}
		}

	case "/intent":
		if len(parts) < 3 || parts[1] != "correct" {
			cr.renderer.mu.Lock()
			cr.renderer.writeContent("Usage: /intent correct <intent>")
			cr.renderer.writeContent("Example: /intent correct code")
			cr.renderer.mu.Unlock()
			return false
		}
		intent := parts[2]
		// Save the last user message + corrected intent
		if err := saveIntentCorrection(cr.agent.conversation.LastUserMessage(), intent); err != nil {
			cr.renderer.Error(err.Error())
		} else {
			cr.renderer.mu.Lock()
			cr.renderer.writeContent(fmt.Sprintf("Saved correction: message -> intent '%s'", intent))
			cr.renderer.mu.Unlock()
		}
		return false

	case "/redact":
		if len(parts) < 2 {
			cr.renderer.mu.Lock()
			cr.renderer.writeContent("Usage: /redact <add|ignore|list|test> [args]")
			cr.renderer.writeContent("  /redact add <name> <regex>  - Add custom redaction pattern")
			cr.renderer.writeContent("  /redact ignore <name>       - Ignore a built-in pattern")
			cr.renderer.writeContent("  /redact list                - List all active patterns")
			cr.renderer.writeContent("  /redact test <text>         - Test redaction on text")
			cr.renderer.mu.Unlock()
			return false
		}
		subcmd := parts[1]
		redactor := security.NewRedactor()
		rulesPath := os.ExpandEnv("$HOME/.synroute/redaction_rules.json")

		switch subcmd {
		case "add":
			if len(parts) < 4 {
				cr.renderer.mu.Lock()
				cr.renderer.writeContent("Usage: /redact add <name> <regex>")
				cr.renderer.mu.Unlock()
				return false
			}
			name := parts[2]
			regex := strings.Join(parts[3:], " ")
			if err := security.SaveCustomRule(rulesPath, name, regex, "redact"); err != nil {
				cr.renderer.Error(err.Error())
			} else {
				cr.renderer.mu.Lock()
				cr.renderer.writeContent(fmt.Sprintf("Added redaction rule '%s': %s", name, regex))
				cr.renderer.mu.Unlock()
			}
		case "ignore":
			if len(parts) < 3 {
				cr.renderer.mu.Lock()
				cr.renderer.writeContent("Usage: /redact ignore <name>")
				cr.renderer.mu.Unlock()
				return false
			}
			name := parts[2]
			if err := security.SaveCustomRule(rulesPath, name, "", "ignore"); err != nil {
				cr.renderer.Error(err.Error())
			} else {
				cr.renderer.mu.Lock()
				cr.renderer.writeContent(fmt.Sprintf("Ignored pattern '%s'", name))
				cr.renderer.mu.Unlock()
			}
		case "list":
			if err := redactor.LoadCustomRules(rulesPath); err != nil {
				cr.renderer.Error(fmt.Sprintf("Error loading rules: %v", err))
			}
			active := redactor.GetActivePatterns()
			ignored := redactor.GetIgnoredPatterns()
			cr.renderer.mu.Lock()
			cr.renderer.writeContent("Active redaction patterns:")
			for _, p := range active {
				cr.renderer.writeContent(fmt.Sprintf("  - %s", p))
			}
			if len(ignored) > 0 {
				cr.renderer.writeContent("Ignored patterns:")
				for _, p := range ignored {
					cr.renderer.writeContent(fmt.Sprintf("  - %s", p))
				}
			}
			cr.renderer.mu.Unlock()
		case "test":
			if len(parts) < 3 {
				cr.renderer.mu.Lock()
				cr.renderer.writeContent("Usage: /redact test <text>")
				cr.renderer.mu.Unlock()
				return false
			}
			text := strings.Join(parts[2:], " ")
			if err := redactor.LoadCustomRules(rulesPath); err != nil {
				log.Printf("[Redact] error loading custom rules: %v", err)
			}
			result := redactor.TestRedaction(text)
			cr.renderer.mu.Lock()
			cr.renderer.writeContent(fmt.Sprintf("Original: %s", text))
			cr.renderer.writeContent(fmt.Sprintf("Redacted: %s", result.Text))
			cr.renderer.writeContent(fmt.Sprintf("Redacted %d items", result.RedactedCount))
			cr.renderer.mu.Unlock()
		default:
			cr.renderer.mu.Lock()
			cr.renderer.writeContent(fmt.Sprintf("Unknown /redact subcommand: %s", subcmd))
			cr.renderer.mu.Unlock()
		}
		return false

	case "/help":
		cr.renderer.ShowHelp()

	default:
		cr.renderer.mu.Lock()
		cr.renderer.writeContent(fmt.Sprintf("  unknown command: %s (type /help)", cmd))
		cr.renderer.mu.Unlock()
	}
	return false
}
