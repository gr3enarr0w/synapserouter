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

	"github.com/gr3enarr0w/synapserouter/internal/security"
	"github.com/gr3enarr0w/synapserouter/internal/tools"
)

// REPL implements an interactive read-eval-print loop for the agent.
type REPL struct {
	agent    *Agent
	renderer TerminalRenderer
	in       io.Reader
	out      io.Writer
}

// NewREPL creates a new REPL for the given agent.
func NewREPL(agent *Agent, renderer TerminalRenderer) *REPL {
	return &REPL{
		agent:    agent,
		renderer: renderer,
		in:       os.Stdin,
		out:      os.Stdout,
	}
}

// Run starts the interactive REPL loop. Blocks until exit.
func (r *REPL) Run(ctx context.Context) error {
	// Brand name — logo image planned for v1.01
	noColor := os.Getenv("NO_COLOR") != ""
	if noColor {
		fmt.Fprintln(r.out, "\n  SynRoute — chat mode")
	} else {
		fmt.Fprintln(r.out, "\n\033[1;36m  Syn\033[1;35mRoute\033[0m\033[2m — chat mode\033[0m")
	}
	fmt.Fprintln(r.out, "  /help for commands, /exit to quit")
	fmt.Fprintln(r.out)

	scanner := bufio.NewScanner(r.in)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	// Handle Ctrl-C gracefully — cancel current request, don't exit
	var mu sync.Mutex
	reqCtx, reqCancel := context.WithCancel(ctx)
	_ = reqCtx // used after reassignment in the loop below

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
			if done := r.handleCommand(ctx, input); done {
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

func (r *REPL) handleCommand(ctx context.Context, input string) bool {
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

	case "/plan":
		msg := strings.TrimSpace(strings.TrimPrefix(input, "/plan"))
		if msg == "" {
			msg = "Generate a plan with acceptance criteria for the current project"
		}
		fmt.Fprintf(r.out, "Running plan phase...\n")
		response, err := r.agent.RunPhase(ctx, "plan", msg)
		if err != nil {
			fmt.Fprintf(r.out, "error: %s\n", err)
		} else if response != "" {
			fmt.Fprintln(r.out, response)
		}

	case "/review":
		msg := strings.TrimSpace(strings.TrimPrefix(input, "/review"))
		if msg == "" {
			msg = "Review the code in the current working directory. Read files, run tests, identify issues."
		}
		fmt.Fprintf(r.out, "Running code review...\n")
		response, err := r.agent.RunPhase(ctx, "code-review", msg)
		if err != nil {
			fmt.Fprintf(r.out, "error: %s\n", err)
		} else if response != "" {
			fmt.Fprintln(r.out, response)
		}

	case "/check":
		msg := strings.TrimSpace(strings.TrimPrefix(input, "/check"))
		if msg == "" {
			msg = "Run verification: build the code, run tests, check against acceptance criteria."
		}
		fmt.Fprintf(r.out, "Running self-check...\n")
		response, err := r.agent.RunPhase(ctx, "self-check", msg)
		if err != nil {
			fmt.Fprintf(r.out, "error: %s\n", err)
		} else if response != "" {
			fmt.Fprintln(r.out, response)
		}

	case "/fix":
		msg := strings.TrimSpace(strings.TrimPrefix(input, "/fix"))
		if msg == "" {
			fmt.Fprintln(r.out, "usage: /fix <description of what to fix>")
		} else {
			fmt.Fprintf(r.out, "Running targeted fix...\n")
			response, err := r.agent.RunPhase(ctx, "implement", msg)
			if err != nil {
				fmt.Fprintf(r.out, "error: %s\n", err)
			} else if response != "" {
				fmt.Fprintln(r.out, response)
			}
		}

	case "/research":
		msg := strings.TrimSpace(strings.TrimPrefix(input, "/research"))
		if msg == "" {
			fmt.Fprintln(r.out, "  usage: /research [quick|standard|deep] <query>")
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
					fmt.Fprintln(r.out, "  usage: /research [quick|standard|deep] <query>")
					break
				}
			}

			config := DefaultResearchConfig(depth)
			fmt.Fprintf(r.out, "  researching (%s): %d rounds, %d max queries, %d budget...\n",
				depth, config.MaxRounds, config.MaxQueries, config.MaxAPICalls)

			report, err := r.agent.RunResearch(ctx, msg, depth)
			if err != nil {
				fmt.Fprintf(r.out, "error: %s\n", err)
			} else if report != nil {
				fmt.Fprintln(r.out, report.Findings)
			}
		}

	case "/intent":
		if len(parts) < 3 || parts[1] != "correct" {
			fmt.Fprintln(r.out, "Usage: /intent correct <intent>")
			fmt.Fprintln(r.out, "Example: /intent correct code")
			return false
		}
		intent := parts[2]
		// Save the last user message + corrected intent
		if err := saveIntentCorrection(r.agent.conversation.LastUserMessage(), intent); err != nil {
			fmt.Fprintf(r.out, "Error saving correction: %v\n", err)
		} else {
			fmt.Fprintf(r.out, "Saved correction: message -> intent '%s'\n", intent)
		}
		return false

	case "/redact":
		if len(parts) < 2 {
			fmt.Fprintln(r.out, "Usage: /redact <add|ignore|list|test> [args]")
			fmt.Fprintln(r.out, "  /redact add <name> <regex>  - Add custom redaction pattern")
			fmt.Fprintln(r.out, "  /redact ignore <name>       - Ignore a built-in pattern")
			fmt.Fprintln(r.out, "  /redact list                - List all active patterns")
			fmt.Fprintln(r.out, "  /redact test <text>         - Test redaction on text")
			return false
		}
		subcmd := parts[1]
		redactor := security.NewRedactor()
		rulesPath := os.ExpandEnv("$HOME/.synroute/redaction_rules.json")
		
		switch subcmd {
		case "add":
			if len(parts) < 4 {
				fmt.Fprintln(r.out, "Usage: /redact add <name> <regex>")
				return false
			}
			name := parts[2]
			regex := strings.Join(parts[3:], " ")
			if err := security.SaveCustomRule(rulesPath, name, regex, "redact"); err != nil {
				fmt.Fprintf(r.out, "Error saving rule: %v\n", err)
			} else {
				fmt.Fprintf(r.out, "Added redaction rule '%s': %s\n", name, regex)
			}
		case "ignore":
			if len(parts) < 3 {
				fmt.Fprintln(r.out, "Usage: /redact ignore <name>")
				return false
			}
			name := parts[2]
			if err := security.SaveCustomRule(rulesPath, name, "", "ignore"); err != nil {
				fmt.Fprintf(r.out, "Error saving ignore rule: %v\n", err)
			} else {
				fmt.Fprintf(r.out, "Pattern '%s' will be ignored\n", name)
			}
		case "list":
			if err := redactor.LoadCustomRules(rulesPath); err != nil {
				fmt.Fprintf(r.out, "Error loading rules: %v\n", err)
			}
			active := redactor.GetActivePatterns()
			ignored := redactor.GetIgnoredPatterns()
			fmt.Fprintf(r.out, "Active patterns (%d):\n", len(active))
			for _, p := range active {
				fmt.Fprintf(r.out, "  - %s\n", p)
			}
			if len(ignored) > 0 {
				fmt.Fprintf(r.out, "Ignored patterns (%d):\n", len(ignored))
				for _, p := range ignored {
					fmt.Fprintf(r.out, "  - %s\n", p)
				}
			}
		case "test":
			if len(parts) < 3 {
				fmt.Fprintln(r.out, "Usage: /redact test <text>")
				return false
			}
			if err := redactor.LoadCustomRules(rulesPath); err != nil {
				fmt.Fprintf(r.out, "Error loading rules: %v\n", err)
			}
			text := strings.Join(parts[2:], " ")
			result := redactor.TestRedaction(text)
			fmt.Fprintf(r.out, "Original: %s\n", text)
			fmt.Fprintf(r.out, "Redacted: %s\n", result.Text)
			fmt.Fprintf(r.out, "Redacted %d items\n", result.RedactedCount)
		default:
			fmt.Fprintf(r.out, "Unknown /redact subcommand: %s\n", subcmd)
		}
		return false

	case "/help":
		fmt.Fprintln(r.out, `Commands:
  /exit      Exit the REPL
  /clear     Clear conversation history
  /model     Show or set the model (e.g., /model claude-sonnet-4-6)
  /tools     List available tools
  /history   Show conversation history
  /agents    Show spawned sub-agents
  /budget    Show resource budget usage
  /plan      Run plan phase (generates plan + acceptance criteria)
  /review    Run code review phase (independent assessment)
  /check     Run self-check phase (build, test, verify)
  /fix <msg> Run targeted implement phase (fix specific issue)
  /research  Run web research (e.g., /research deep <query>)
  /intent    Intent commands
    correct <intent> - Correct last message classification
  /help      Show this help`)

	default:
		fmt.Fprintf(r.out, "unknown command: %s (type /help for commands)\n", cmd)
	}
	return false
}

// RunOneShot executes a single message and returns the response.
// Sets up the same infrastructure as the REPL: pool, delegation tools, guardrails.
// If config.EventBus is set, attaches a LogRenderer for real-time structured output.
func RunOneShot(ctx context.Context, executor ChatExecutor, registry *tools.Registry, config Config, message string) (string, error) {
	renderer := NewRenderer(os.Stderr) // tool output to stderr, final response to stdout
	ag := New(executor, registry, renderer, config)

	// Apply conversation tier — one-shot must match interactive mode behavior.
	// Without this, one-shot always starts at L0 (cheapest) regardless of config.
	convTier := TierFrontier
	if tierEnv := strings.TrimSpace(strings.ToLower(os.Getenv("SYNROUTE_CONVERSATION_TIER"))); tierEnv != "" {
		switch tierEnv {
		case "cheap":
			convTier = TierCheap
		case "mid":
			convTier = TierMid
		case "frontier":
			convTier = TierFrontier
		}
	} else if strings.EqualFold(os.Getenv("ACTIVE_PROFILE"), "work") {
		convTier = TierMid
	}
	ag.SetMinProviderLevel(ag.ProviderLevelForTier(convTier))

	// Start LogRenderer if event bus is configured
	if config.EventBus != nil {
		events := config.EventBus.Subscribe()
		lr := NewLogRenderer(os.Stderr, config.Verbosity, false)
		go lr.Run(events)
		defer config.EventBus.Close()
	}

	// Match REPL setup: pool, delegation tools, guardrails
	pool := NewPool(config.MaxAgents)
	ag.SetPool(pool)
	registry.Register(NewDelegateTool(ag))
	registry.Register(NewHandoffTool(ag))
	ag.SetInputGuardrails(NewGuardrailChain(&SecretPatternGuardrail{}))

	response, err := ag.Run(ctx, message)

	// Save session so one-shot runs are resumable via --resume
	if config.DB != nil {
		_ = ag.SaveState(config.DB)
	}

	return response, err
}
