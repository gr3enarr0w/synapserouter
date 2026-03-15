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

	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/tools"
)

// REPL implements an interactive read-eval-print loop for the agent.
type REPL struct {
	agent    *Agent
	renderer *Renderer
	in       io.Reader
	out      io.Writer
}

// NewREPL creates a new REPL for the given agent.
func NewREPL(agent *Agent, renderer *Renderer) *REPL {
	return &REPL{
		agent:    agent,
		renderer: renderer,
		in:       os.Stdin,
		out:      os.Stdout,
	}
}

// Run starts the interactive REPL loop. Blocks until exit.
func (r *REPL) Run(ctx context.Context) error {
	fmt.Fprintln(r.out, "synroute agent — type /help for commands, /exit to quit")
	fmt.Fprintln(r.out)

	scanner := bufio.NewScanner(r.in)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	// Handle Ctrl-C gracefully — cancel current request, don't exit
	var mu sync.Mutex
	reqCtx, reqCancel := context.WithCancel(ctx)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT)
	defer signal.Stop(sigCh)

	go func() {
		for range sigCh {
			mu.Lock()
			reqCancel()
			mu.Unlock()
			fmt.Fprintln(r.out, "\n(interrupted)")
			r.renderer.Prompt()
		}
	}()

	for {
		r.renderer.Prompt()
		if !scanner.Scan() {
			// EOF (Ctrl-D)
			mu.Lock()
			reqCancel()
			mu.Unlock()
			fmt.Fprintln(r.out, "\nbye")
			return nil
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		// Handle REPL commands
		if strings.HasPrefix(input, "/") {
			if done := r.handleCommand(input); done {
				mu.Lock()
				reqCancel()
				mu.Unlock()
				return nil
			}
			continue
		}

		// Create fresh context for each request
		mu.Lock()
		reqCancel() // cancel previous if still active
		reqCtx, reqCancel = context.WithCancel(ctx)
		mu.Unlock()

		response, err := r.agent.Run(reqCtx, input)
		if err != nil {
			r.renderer.Error(err.Error())
			continue
		}

		if response != "" {
			fmt.Fprintln(r.out)
			r.renderer.Text(response)
		}
		fmt.Fprintln(r.out)
	}
}

func (r *REPL) handleCommand(input string) bool {
	parts := strings.Fields(input)
	cmd := parts[0]

	switch cmd {
	case "/exit", "/quit":
		fmt.Fprintln(r.out, "bye")
		return true

	case "/clear":
		r.agent.Clear()
		fmt.Fprintln(r.out, "conversation cleared")

	case "/model":
		if len(parts) < 2 {
			fmt.Fprintf(r.out, "current model: %s\n", r.agent.config.Model)
		} else {
			r.agent.config.Model = parts[1]
			fmt.Fprintf(r.out, "model set to: %s\n", parts[1])
		}

	case "/tools":
		names := r.agent.registry.List()
		fmt.Fprintf(r.out, "Available tools (%d):\n", len(names))
		for _, name := range names {
			tool, _ := r.agent.registry.Get(name)
			fmt.Fprintf(r.out, "  %-15s [%s] %s\n", name, tool.Category(), tool.Description())
		}

	case "/history":
		msgs := r.agent.conversation.Messages()
		fmt.Fprintf(r.out, "Conversation history (%d messages):\n", len(msgs))
		for i, msg := range msgs {
			content := msg.Content
			if len(content) > 80 {
				content = content[:80] + "..."
			}
			fmt.Fprintf(r.out, "  %3d. [%s] %s\n", i+1, msg.Role, content)
		}

	case "/agents":
		children := r.agent.Children()
		if len(children) == 0 {
			fmt.Fprintln(r.out, "No sub-agents spawned")
		} else {
			fmt.Fprintf(r.out, "Sub-agents (%d):\n", len(children))
			for _, c := range children {
				fmt.Fprintf(r.out, "  %-20s [%s] %s\n", c.ID, c.Role, c.Status)
			}
		}

	case "/budget":
		if r.agent.budget == nil {
			fmt.Fprintln(r.out, "No budget configured")
		} else {
			snap := r.agent.budget.Snapshot()
			fmt.Fprintf(r.out, "Budget usage:\n")
			fmt.Fprintf(r.out, "  Turns:   %d", snap.Turns)
			if snap.Budget.MaxTurns > 0 {
				fmt.Fprintf(r.out, " / %d", snap.Budget.MaxTurns)
			}
			fmt.Fprintln(r.out)
			fmt.Fprintf(r.out, "  Tokens:  %d", snap.Tokens)
			if snap.Budget.MaxTokens > 0 {
				fmt.Fprintf(r.out, " / %d", snap.Budget.MaxTokens)
			}
			fmt.Fprintln(r.out)
			fmt.Fprintf(r.out, "  Elapsed: %s", snap.Elapsed.Round(time.Second))
			if snap.Budget.MaxDuration > 0 {
				fmt.Fprintf(r.out, " / %s", snap.Budget.MaxDuration)
			}
			fmt.Fprintln(r.out)
		}

	case "/help":
		fmt.Fprintln(r.out, `Commands:
  /exit      Exit the REPL
  /clear     Clear conversation history
  /model     Show or set the model (e.g., /model claude-sonnet-4-6)
  /tools     List available tools
  /history   Show conversation history
  /agents    Show spawned sub-agents
  /budget    Show resource budget usage
  /help      Show this help`)

	default:
		fmt.Fprintf(r.out, "unknown command: %s (type /help for commands)\n", cmd)
	}
	return false
}

// RunOneShot executes a single message and returns the response.
func RunOneShot(ctx context.Context, executor ChatExecutor, registry *tools.Registry, config Config, message string) (string, error) {
	renderer := NewRenderer(os.Stderr) // tool output to stderr, final response to stdout
	agent := New(executor, registry, renderer, config)
	return agent.Run(ctx, message)
}
