package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gr3enarr0w/synapserouter/internal/agent"
	"github.com/gr3enarr0w/synapserouter/internal/app"
	"github.com/gr3enarr0w/synapserouter/internal/environment"
	"github.com/gr3enarr0w/synapserouter/internal/mcp"
	"github.com/gr3enarr0w/synapserouter/internal/tools"
	"github.com/gr3enarr0w/synapserouter/internal/worktree"
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
	usePipeline := fs.Bool("pipeline", false, "Force legacy 6-phase pipeline (default: frontier model with pipeline tools)")
	confidential := fs.Bool("confidential", false, "Confidential mode: blocks external API calls (web_search, web_fetch)")
	screenReader := fs.Bool("screen-reader", os.Getenv("SYNROUTE_SCREEN_READER") != "", "Screen-reader-friendly output")
	jsonEvents := fs.Bool("json-events", false, "Emit events as JSON lines to stderr")
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
	config.Streaming = true // Enable token streaming for code mode TUI
	config.MaxAgents = *maxAgents
	config.DB = ac.DB
	config.ToolStore = agent.NewToolOutputStore(ac.DB)
	config.PlanCache = agent.NewPlanCache(ac.DB)
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
	config.AutoOrchestrate = *usePipeline
	config.Confidential = *confidential || os.Getenv("SYNROUTE_CONFIDENTIAL") == "true"
	if config.Confidential {
		log.Println("[Security] Confidential mode enabled — external API calls blocked")
		tools.SetConfidentialMode(true)
	}
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

	// Worktree isolation - mandatory for --message mode to prevent writes to main repo
	var wt *worktree.Worktree
	useWorktreeForMessage := *message != "" || *useWorktree
	if useWorktreeForMessage {
		wtMgr, err := worktree.NewManager(worktree.DefaultConfig())
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating worktree manager: %v\n", err)
			os.Exit(1)
		}

		wt, err = wtMgr.Create(cwd, "code")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating worktree: %v\n", err)
			fmt.Fprintf(os.Stderr, "ABORTING: Cannot run --message mode without worktree isolation\n")
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
	codeRenderer.SetProviderLabel(ac.Profile)
	codeRenderer.SetScreenReaderMode(*screenReader || os.Getenv("SYNROUTE_SCREEN_READER") != "")

	// Subscribe renderer to event bus
	events := bus.Subscribe()
	go codeRenderer.Run(events)

	// Start event log renderer for JSON events (writes to stderr)
	if *jsonEvents {
		logEvents := bus.Subscribe()
		lr := agent.NewLogRenderer(os.Stderr, *verbose, *jsonEvents)
		go lr.Run(logEvents)
	}

	// Set up agent (shared between one-shot and interactive modes)
	pool := agent.NewPool(config.MaxAgents)

	var ag *agent.Agent
	userID := config.UserID
	if userID == "" {
		userID = "local"
	}
	if config.SessionID != "" && config.DB != nil {
		state, err := agent.LoadState(config.DB, config.SessionID, userID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading session: %v\n", err)
			os.Exit(1)
		}
		ag = agent.RestoreAgent(ac.ProxyRouter, registry, codeRenderer, state)
	} else if config.Resume && config.DB != nil {
		state, err := agent.LoadLatestState(config.DB, userID)
		if err != nil {
			ag = agent.New(ac.ProxyRouter, registry, codeRenderer, config)
		} else {
			ag = agent.RestoreAgent(ac.ProxyRouter, registry, codeRenderer, state)
		}
	} else {
		ag = agent.New(ac.ProxyRouter, registry, codeRenderer, config)
	}

	ag.SetPool(pool)

	// Set conversation tier — configurable via SYNROUTE_CONVERSATION_TIER env var.
	// Default: frontier (personal), mid (work). Sub-agents use their own tiers.
	convTier := agent.TierFrontier
	if tierEnv := os.Getenv("SYNROUTE_CONVERSATION_TIER"); tierEnv != "" {
		switch strings.ToLower(tierEnv) {
		case "cheap":
			convTier = agent.TierCheap
		case "mid":
			convTier = agent.TierMid
		case "frontier":
			convTier = agent.TierFrontier
		}
	} else if ac.Profile == "work" {
		convTier = agent.TierMid // work default: sonnet-level for conversation
	}
	ag.SetMinProviderLevel(ag.ProviderLevelForTier(convTier))

	registry.Register(agent.NewDelegateTool(ag))
	registry.Register(agent.NewHandoffTool(ag))
	if !config.AutoOrchestrate {
		agent.RegisterPipelineTools(registry, ag)
	}
	ag.SetInputGuardrails(agent.NewGuardrailChain(&agent.SecretPatternGuardrail{}))

	// Permission prompting disabled for v1 — the /dev/tty read in
	// DefaultPermissionPrompt conflicts with the terminal input layer,
	// causing the REPL to hang. Will be restored in v1.01 with Bubble Tea.
	// TODO(v1.01): integrate permission prompt with terminal input system

	// One-shot mode — use the same agent (no RunOneShot to avoid duplicate LogRenderer)
	if *message != "" {
		// One-shot: switch to auto-approve since there's no user to prompt
		ag.SetPermissions(tools.NewPermissionChecker(tools.ModeAutoApprove))
		ag.SetNonInteractive(true)
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

	// Auto-save session and project continuity
	if config.DB != nil {
		if err := ag.SaveState(config.DB); err != nil {
			log.Printf("Warning: failed to save session: %v", err)
		} else {
			log.Printf("Session saved: %s", ag.SessionID())
		}
		continuity := agent.BuildContinuityFromAgent(ag)
		if err := agent.SaveContinuity(config.DB, continuity); err != nil {
			log.Printf("Warning: failed to save continuity: %v", err)
		}
	}
}
