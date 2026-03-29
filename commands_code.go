package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/agent"
	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/app"
	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/environment"
	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/mcp"
	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/tools"
	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/worktree"
)

func cmdCode(args []string) {
	fs := flag.NewFlagSet("code", flag.ExitOnError)
	model := fs.String("model", "auto", "Model to use")
	message := fs.String("message", "", "One-shot message (non-interactive)")
	specFile := fs.String("spec-file", "", "Read spec from file")
	system := fs.String("system", "", "Custom system prompt")
	project := fs.String("project", "", "Project name — creates ~/Development/<name>/ and works there")
	useWorktree := fs.Bool("worktree", false, "Run in isolated git worktree")
	maxAgents := fs.Int("max-agents", 5, "Max concurrent sub-agents")
	budgetTokens := fs.Int64("budget", 0, "Max total tokens budget (0 = unlimited)")
	resume := fs.Bool("resume", false, "Resume most recent session")
	sessionID := fs.String("session", "", "Resume specific session ID")
	verbose := fs.Int("verbose", 1, "Verbosity level: 0=compact, 1=normal, 2=verbose")
	fs.Parse(args)

	registry := tools.DefaultRegistry()
	cwd, _ := os.Getwd()

	// Derive project name for status bar
	projectName := filepath.Base(cwd)

	// Create project directory if --project specified
	if *project != "" {
		home, _ := os.UserHomeDir()
		projectDir := filepath.Join(home, "Development", *project)
		if err := os.MkdirAll(projectDir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating project directory: %v\n", err)
			os.Exit(1)
		}
		cwd = projectDir
		projectName = *project
	}

	// Set up timestamped log file BEFORE provider init so all logs go to file
	logDir := filepath.Join(cwd, ".synroute", "logs")
	os.MkdirAll(logDir, 0755)
	logTimestamp := time.Now().Format("2006-01-02T15-04-05")
	logPath := filepath.Join(logDir, fmt.Sprintf("run-%s.log", logTimestamp))
	if logFile, err := os.Create(logPath); err == nil {
		// In code mode, logs go to file ONLY (not stderr — that's the TUI)
		log.SetOutput(logFile)
		defer logFile.Close()
	}

	ac, err := app.InitLight(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing: %v\n", err)
		os.Exit(1)
	}
	defer ac.Close()

	if len(ac.Providers) == 0 {
		fmt.Fprintln(os.Stderr, "No providers configured for this profile")
		os.Exit(1)
	}

	ac.InitFull()

	config := agent.DefaultConfig()
	config.Model = *model
	config.WorkDir = cwd
	config.MaxAgents = *maxAgents
	config.DB = ac.DB
	config.ToolStore = agent.NewToolOutputStore(ac.DB)
	config.VectorMemory = ac.VectorMemory
	if *system != "" {
		config.SystemPrompt = *system
	}
	if *budgetTokens > 0 {
		config.Budget = &agent.AgentBudget{MaxTokens: *budgetTokens}
	}
	config.Resume = *resume
	config.SessionID = *sessionID
	config.EscalationChain = ac.EscalationChain
	providerNames := make([]string, len(ac.Providers))
	for i, p := range ac.Providers {
		providerNames[i] = p.Name()
	}
	config.Providers = providerNames
	config.AutoOrchestrate = true
	config.Verbosity = *verbose

	// MCP client — load config and connect to registered servers
	mcpCfg, err := mcp.LoadConfig(mcp.DefaultConfigPath())
	if err != nil {
		log.Printf("Warning: failed to load MCP config: %v", err)
	} else if len(mcpCfg.Servers) > 0 {
		mcpClient := mcp.NewClientFromConfig(mcpCfg)
		mcpCtx, mcpCancel := context.WithTimeout(context.Background(), 10*time.Second)
		mcpClient.ConnectAll(mcpCtx, 2)
		mcpCancel()
		config.MCPClient = mcpClient
	}

	// Event bus
	bus := agent.NewEventBus()
	config.EventBus = bus

	ctx := context.Background()

	// Worktree isolation
	var wt *worktree.Worktree
	if *useWorktree {
		wtMgr, err := worktree.NewManager(worktree.DefaultConfig())
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating worktree manager: %v\n", err)
			os.Exit(1)
		}

		wt, err = wtMgr.Create(cwd, "code")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating worktree: %v\n", err)
			os.Exit(1)
		}

		config.WorkDir = wt.Path
		projectName += " (worktree)"

		stopCleaner := worktree.StartCleaner(ctx, wtMgr, worktree.DefaultConfig().CleanupInterval)
		defer stopCleaner()
	}

	// Spec file handling
	if *specFile != "" {
		if absPath, err := filepath.Abs(*specFile); err == nil {
			config.SpecFilePath = absPath
		}
	}

	if *specFile != "" && *message == "" {
		specContent, err := os.ReadFile(*specFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading spec file: %v\n", err)
			os.Exit(1)
		}

		if lang := detectLanguageFromSpec(string(specContent)); lang != "" {
			config.ProjectLanguage = lang
		}

		var composed string
		if hasExistingProject(cwd) {
			composed = "Review and improve the existing implementation against this specification. " +
				"Inspect the current code with file_read, run tests with bash, fix any issues, " +
				"and fill in anything missing. Do NOT rewrite files that already work correctly.\n\n" +
				string(specContent)
		} else {
			composed = "Implement the following specification:\n\n" + string(specContent)
		}
		message = &composed
	}

	// Detect language from project
	if config.ProjectLanguage == "" {
		if env := environment.Detect(cwd); env != nil && env.Language != "" {
			config.ProjectLanguage = env.Language
		}
	}

	// Get terminal size
	term := agent.NewTerminal(int(os.Stdin.Fd()))
	width, height, err := term.GetSize()
	if err != nil {
		width, height = 80, 24
	}

	// Detect language
	detectedLang := config.ProjectLanguage

	// Create code renderer
	codeRenderer := agent.NewCodeRenderer(os.Stdout, width, height, projectName, *model, detectedLang)
	codeRenderer.SetVersion(version)
	codeRenderer.SetVerbosity(*verbose)

	// Subscribe renderer to event bus
	events := bus.Subscribe()
	go codeRenderer.Run(events)

	// Set up agent (shared between one-shot and interactive modes)
	pool := agent.NewPool(config.MaxAgents)

	var ag *agent.Agent
	if config.SessionID != "" && config.DB != nil {
		state, err := agent.LoadState(config.DB, config.SessionID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading session: %v\n", err)
			os.Exit(1)
		}
		ag = agent.RestoreAgent(ac.ProxyRouter, registry, codeRenderer, state)
	} else if config.Resume && config.DB != nil {
		state, err := agent.LoadLatestState(config.DB)
		if err != nil {
			ag = agent.New(ac.ProxyRouter, registry, codeRenderer, config)
		} else {
			ag = agent.RestoreAgent(ac.ProxyRouter, registry, codeRenderer, state)
		}
	} else {
		ag = agent.New(ac.ProxyRouter, registry, codeRenderer, config)
	}

	ag.SetPool(pool)
	registry.Register(agent.NewDelegateTool(ag))
	registry.Register(agent.NewHandoffTool(ag))
	ag.SetInputGuardrails(agent.NewGuardrailChain(&agent.SecretPatternGuardrail{}))

	// One-shot mode — use the same agent (no RunOneShot to avoid duplicate LogRenderer)
	if *message != "" {
		if *specFile != "" {
			specContent, err := os.ReadFile(*specFile)
			if err == nil {
				composed := "Implement the following specification:\n\n" + string(specContent) + "\n\nUser instruction: " + *message
				message = &composed
			}
		}
		response, err := ag.Run(ctx, *message)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		codeRenderer.Cleanup()
		bus.Close()
		fmt.Println(response)
		if wt != nil {
			fmt.Fprintf(os.Stderr, "Worktree still active at: %s\n", wt.Path)
		}
		return
	}

	// Run the code mode REPL (uses cooked mode input with TUI chrome)
	codeRepl := agent.NewCodeREPL(ag, codeRenderer, term)
	if err := codeRepl.Run(ctx); err != nil {
		log.Printf("REPL error: %v", err)
	}

	bus.Close()

	// Auto-save session
	if config.DB != nil {
		if err := ag.SaveState(config.DB); err != nil {
			// Can't write to stderr in code mode (TUI owns it)
			log.Printf("Warning: failed to save session: %v", err)
		} else {
			log.Printf("Session saved: %s", ag.SessionID())
		}
	}
}
