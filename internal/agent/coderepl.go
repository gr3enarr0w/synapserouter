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
	"github.com/gr3enarr0w/synapserouter/internal/tools"
)

// CodeREPL implements the code mode interactive loop with readline-based input
// for history, tab completion, and proper terminal handling.
type CodeREPL struct {
	agent           *Agent
	renderer        *CodeRenderer
	out             io.Writer
	pendingMessages []string
	permissionReqCh chan permissionRequest
}

type permissionRequest struct {
	toolName string
	category tools.ToolCategory
	args     map[string]interface{}
	response chan bool
}

type runResult struct {
	response string
	err      error
}

// NewCodeREPL creates a code mode REPL. The Terminal parameter is accepted
// for API compatibility but no longer used — readline handles terminal mode.
func NewCodeREPL(agent *Agent, renderer *CodeRenderer, _ *Terminal) *CodeREPL {
	return &CodeREPL{
		agent:           agent,
		renderer:        renderer,
		out:             os.Stdout,
		permissionReqCh: make(chan permissionRequest),
	}
}

func (cr *CodeREPL) PermissionPrompt() tools.PermissionPromptFunc {
	return func(toolName string, category tools.ToolCategory, args map[string]interface{}) bool {
		req := permissionRequest{
			toolName: toolName,
			category: category,
			args:     args,
			response: make(chan bool, 1),
		}
		cr.permissionReqCh <- req
		return <-req.response
	}
}

func (cr *CodeREPL) enqueuePendingMessage(input string) {
	input = strings.TrimSpace(input)
	if input == "" {
		return
	}
	cr.pendingMessages = append(cr.pendingMessages, input)
}

func (cr *CodeREPL) dequeuePendingMessage() string {
	if len(cr.pendingMessages) == 0 {
		return ""
	}
	input := cr.pendingMessages[0]
	cr.pendingMessages = cr.pendingMessages[1:]
	return input
}

// Run starts the code mode REPL. Blocks until exit.
func (cr *CodeREPL) Run(ctx context.Context) error {
	// Initialize the screen layout (prints launch banner)
	cr.renderer.Init()
	cr.renderer.RenderStatusBar()

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

	var inputCh <-chan byte
	var inputClosedCh <-chan struct{}
	if isTerminal {
		inputCh, inputClosedCh = cr.startInputReader()
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

	for {
		// Check if double Ctrl-C exit was triggered
		select {
		case <-exitCh:
			return nil
		default:
		}

		cr.renderer.SetInputActive(true)
		cr.renderer.SetInputLine("")

		// Check for pending message typed during agent execution
		var input string
		var eof bool
		if queued := cr.dequeuePendingMessage(); queued != "" {
			input = queued
		} else {
			input, eof = cr.readLine(ctx, stdinFd, isTerminal, exitCh, inputCh, inputClosedCh, &reqMu, &agentRunning, cancelFn, &ctrlCCount, &lastCtrlC)
			if eof {
				cr.rawWrite("bye\r\n")
				return nil
			}
		}

		log.Printf("[REPL] got input: %q", input)

		if input == "" {
			continue
		}
		cr.renderer.SetInputLine("")

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

		cr.renderer.SetInputActive(false)

		// Create fresh context for the request
		reqCtx := newReqCtx()

		// Message queue disabled — conflicts with permission prompts which also
		// read from /dev/tty. Both race for input, causing "y" to go to queue
		// instead of permission handler. Will be re-enabled in v1.13 with proper
		// input multiplexing. See #493.

		// Run the agent
		cr.renderer.mu.Lock()
		cr.renderer.writeContent("")
		cr.renderer.mu.Unlock()

		resultCh := make(chan runResult, 1)
		go func() {
			response, err := cr.agent.Run(reqCtx, input)
			markDone()
			resultCh <- runResult{response: response, err: err}
		}()

		response, err := cr.waitForAgent(reqCtx, exitCh, inputCh, inputClosedCh, &reqMu, cancelFn, &ctrlCCount, &lastCtrlC, resultCh)
		if err != nil {
			if reqCtx.Err() != nil {
				cr.renderer.SetInputActive(true)
				cr.renderer.SetInputLine("")
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

		// Render status bar after agent completes
		cr.renderer.SetInputActive(true)
		cr.renderer.SetInputLine("")
		cr.renderer.RenderStatusBar()

	}
}

func (cr *CodeREPL) startInputReader() (<-chan byte, <-chan struct{}) {
	inputCh := make(chan byte, 256)
	closedCh := make(chan struct{})
	go func() {
		defer close(closedCh)
		buf := make([]byte, 1)
		for {
			n, err := os.Stdin.Read(buf)
			if err != nil || n == 0 {
				return
			}
			inputCh <- buf[0]
		}
	}()
	return inputCh, closedCh
}

func (cr *CodeREPL) waitForAgent(ctx context.Context, exitCh chan struct{}, inputCh <-chan byte, inputClosedCh <-chan struct{}, reqMu *sync.Mutex, cancelFn context.CancelFunc, ctrlCCount *int, lastCtrlC *time.Time, resultCh <-chan runResult) (string, error) {
	var queueBuf []byte
	cr.renderer.SetInputActive(true)
	cr.renderer.SetFooterNote(cr.renderer.color("\033[2m", "  agent running - type and press Enter to queue next message"))
	cr.renderer.SetInputLine("")

	for {
		select {
		case <-exitCh:
			return "", io.EOF
		case <-inputClosedCh:
			return "", io.EOF
		case result := <-resultCh:
			cr.renderer.SetFooterNote("")
			cr.renderer.SetInputLine("")
			return result.response, result.err
		case req := <-cr.permissionReqCh:
			req.response <- cr.handlePermissionRequest(inputCh, inputClosedCh, exitCh, req)
		case b := <-inputCh:
			switch {
			case b == 0x03:
				reqMu.Lock()
				if cancelFn != nil {
					cancelFn()
				}
				reqMu.Unlock()
				queueBuf = queueBuf[:0]
				cr.renderer.SetInputLine("")
			case b == 0x7F || b == 0x08:
				if len(queueBuf) > 0 {
					queueBuf = queueBuf[:len(queueBuf)-1]
					cr.renderer.SetInputLine(string(queueBuf))
				}
			case b == 0x15:
				queueBuf = queueBuf[:0]
				cr.renderer.SetInputLine("")
			case b == 0x0D || b == 0x0A:
				queued := strings.TrimSpace(string(queueBuf))
				if queued != "" {
					cr.enqueuePendingMessage(queued)
					cr.renderer.mu.Lock()
					cr.renderer.writeContent(cr.renderer.color("\033[36m", "  queued next message: "+queued))
					cr.renderer.mu.Unlock()
				}
				queueBuf = queueBuf[:0]
				cr.renderer.SetInputLine("")
			case b >= 0x20 && b < 0x7F:
				queueBuf = append(queueBuf, b)
				cr.renderer.SetInputLine(string(queueBuf))
			}
		case <-ctx.Done():
			// Wait for result channel to surface the final error/response.
		}
	}
}

func (cr *CodeREPL) handlePermissionRequest(inputCh <-chan byte, inputClosedCh <-chan struct{}, exitCh chan struct{}, req permissionRequest) bool {
	var lineBuf []byte
	label := "write"
	if req.category == tools.CategoryDangerous {
		label = "dangerous"
	}
	summary := formatPermissionSummary(req.toolName, req.args)

	cr.renderer.mu.Lock()
	cr.renderer.writeContent("")
	cr.renderer.writeContent(cr.renderer.color("\033[1;33m", fmt.Sprintf("  [permission] %s tool: %s", label, req.toolName)))
	if summary != "" {
		for _, line := range strings.Split(summary, "\n") {
			cr.renderer.writeContent(cr.renderer.color("\033[2m", "  "+line))
		}
	}
	cr.renderer.mu.Unlock()
	cr.renderer.SetFooterNote(cr.renderer.color("\033[33m", "  Allow? [y/n/a]"))
	cr.renderer.SetInputLine("")

	for {
		select {
		case <-exitCh:
			cr.renderer.SetFooterNote("")
			return false
		case <-inputClosedCh:
			cr.renderer.SetFooterNote("")
			return false
		case b := <-inputCh:
			switch {
			case b == 0x03:
				cr.renderer.SetFooterNote("")
				cr.renderer.SetInputLine("")
				return false
			case b == 0x7F || b == 0x08:
				if len(lineBuf) > 0 {
					lineBuf = lineBuf[:len(lineBuf)-1]
					cr.renderer.SetInputLine(string(lineBuf))
				}
			case b == 0x0D || b == 0x0A:
				cr.renderer.SetFooterNote("")
				cr.renderer.SetInputLine("")
				input := strings.TrimSpace(string(lineBuf))
				if input == "" {
					return parsePermissionByte('\n', new(bool))
				}
				approveAll := false
				if approved, ok := parsePermissionString(input, &approveAll); ok {
					return approved
				}
				lineBuf = lineBuf[:0]
				cr.renderer.SetFooterNote(cr.renderer.color("\033[33m", "  Allow? [y/n/a]"))
			case b >= 0x20 && b < 0x7F:
				lineBuf = append(lineBuf, b)
				cr.renderer.SetInputLine(string(lineBuf))
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
func (cr *CodeREPL) readLine(ctx context.Context, fd int, isRaw bool, exitCh chan struct{}, inputCh <-chan byte, inputClosedCh <-chan struct{}, reqMu *sync.Mutex, agentRunning *bool, cancelFn context.CancelFunc, ctrlCCount *int, lastCtrlC *time.Time) (string, bool) {
	if !isRaw {
		// Fallback: cooked mode (piped input) — read a line from stdin
		// Use goroutine to allow context cancellation
		type result struct {
			line string
			eof  bool
		}
		resultCh := make(chan result, 1)
		go func() {
			buf := make([]byte, 64*1024)
			n, err := os.Stdin.Read(buf)
			if err != nil || n == 0 {
				resultCh <- result{eof: true}
				return
			}
			resultCh <- result{line: strings.TrimSpace(string(buf[:n]))}
		}()
		select {
		case <-ctx.Done():
			return "", false
		case r := <-resultCh:
			if r.eof {
				return "", true
			}
			return r.line, false
		}
	}

	// Raw mode: read byte by byte
	var lineBuf []byte
	cr.renderer.SetInputLine("")

	for {
		// Check exit channel and context cancellation
		select {
		case <-exitCh:
			return "", true
		case <-ctx.Done():
			return "", false
		default:
		}

		var b byte
		select {
		case <-inputClosedCh:
			return "", true
		case b = <-inputCh:
		}

		switch {
		case b == 0x0D || b == 0x0A: // Enter (CR or LF)
			cr.renderer.SetInputLine("")
			return strings.TrimSpace(string(lineBuf)), false

		case b == 0x04: // Ctrl-D (EOF)
			if len(lineBuf) == 0 {
				return "", true
			}
			// If there's text, ignore Ctrl-D (like bash)

		case b == 0x03: // Ctrl-C
			if len(lineBuf) > 0 {
				lineBuf = lineBuf[:0]
				cr.renderer.SetInputLine("")
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
				cr.rawWrite("bye\r\n")
				return "", true // exit signal
			}
			// First Ctrl-C: show hint
			cr.renderer.mu.Lock()
			cr.renderer.writeContent(cr.renderer.color("\033[2m", "  (press Ctrl-C again to exit)"))
			cr.renderer.mu.Unlock()
			cr.renderer.SetInputLine("")
			continue

		case b == 0x7F || b == 0x08: // Backspace (DEL or BS)
			if len(lineBuf) > 0 {
				lineBuf = lineBuf[:len(lineBuf)-1]
				cr.renderer.SetInputLine(string(lineBuf))
			}

		case b == 0x0C: // Ctrl-L — cycle verbosity
			v := (cr.renderer.verbosity + 1) % 3
			cr.renderer.SetVerbosity(v)
			labels := []string{"compact", "normal", "verbose"}
			cr.renderer.mu.Lock()
			cr.renderer.writeContent(cr.renderer.color("\033[33m", "  verbosity: "+labels[v]))
			cr.renderer.mu.Unlock()
			cr.renderer.SetInputLine(string(lineBuf))

		case b == 0x10: // Ctrl-P — pipeline status
			cr.renderer.ShowPipeline()
			cr.renderer.SetInputLine(string(lineBuf))

		case b == 0x14: // Ctrl-T — recent tools
			cr.renderer.ShowRecentTools()
			cr.renderer.SetInputLine(string(lineBuf))

		case b == 0x05: // Ctrl-E — force escalation
			if cr.agent.ForceEscalate() {
				cr.renderer.mu.Lock()
				cr.renderer.writeContent(cr.renderer.color("\033[33m", "  escalated to next tier"))
				cr.renderer.mu.Unlock()
			} else {
				cr.renderer.mu.Lock()
				cr.renderer.writeContent(cr.renderer.color("\033[2m", "  already at highest tier"))
				cr.renderer.mu.Unlock()
			}
			cr.renderer.SetInputLine(string(lineBuf))

		case b == 0x1F: // Ctrl-/ — help
			cr.renderer.ShowHelp()
			cr.renderer.SetInputLine(string(lineBuf))

		case b == 0x15: // Ctrl-U — clear line
			lineBuf = lineBuf[:0]
			cr.renderer.SetInputLine("")

		case b == 0x1B: // ESC — start of escape sequence, consume and ignore
			// Read the rest of the escape sequence (e.g. arrow keys: ESC [ A)
			seq := make([]byte, 2)
			_, _ = os.Stdin.Read(seq) // ignore arrow keys for now

		case b >= 0x20 && b < 0x7F: // Printable ASCII
			lineBuf = append(lineBuf, b)
			cr.renderer.SetInputLine(string(lineBuf))

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
