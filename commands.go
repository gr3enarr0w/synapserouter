package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/agent"
	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/app"
	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/environment"
	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/mcpserver"
	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/tools"
	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/worktree"
)

// Build-time variables set via ldflags
var (
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"
)

func cmdVersion() {
	profile := app.GetActiveProfile()
	fmt.Printf("synroute %s (%s) built %s | profile: %s | %s\n",
		version, commit, buildDate, profile, runtime.Version())
}

func printUsage() {
	fmt.Println(`synroute — LLM proxy router and coding agent

Usage:
  synroute [command]

Commands:
  serve       Start the HTTP server (default if no command given)
  chat        Interactive agent REPL or one-shot message
  mcp-serve   Start standalone MCP tool server
  test        Smoke test providers
  eval        Multi-language eval framework
  profile     Show or switch active profile
  doctor      Run comprehensive diagnostics
  models      List available models
  version     Show version information
  help        Show this help

Run 'synroute <command> --help' for details on a command.`)
}

func cmdTest(args []string) {
	fs := flag.NewFlagSet("test", flag.ExitOnError)
	provider := fs.String("provider", "", "Test only this provider")
	timeout := fs.Duration("timeout", 30*time.Second, "Per-provider timeout")
	verbose := fs.Bool("verbose", false, "Show detailed output")
	jsonOut := fs.Bool("json", false, "Output JSON")
	fs.Parse(args)

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

	opts := app.SmokeTestOpts{
		Provider: *provider,
		Timeout:  *timeout,
		Verbose:  *verbose,
	}

	results := app.RunSmokeTests(context.Background(), ac.Providers, opts)

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(results)
		return
	}

	// ASCII table output
	passed, failed := 0, 0
	fmt.Printf("\n%-20s %-8s %-30s %-8s %-10s\n", "PROVIDER", "STATUS", "MODEL", "TOKENS", "LATENCY")
	fmt.Println(strings.Repeat("-", 80))
	for _, r := range results {
		model := r.Model
		if model == "" {
			model = "-"
		}
		tokens := "-"
		if r.Tokens > 0 {
			tokens = fmt.Sprintf("%d", r.Tokens)
		}
		latency := fmt.Sprintf("%dms", r.Latency)

		if r.Status == "PASS" {
			passed++
		} else {
			failed++
		}

		fmt.Printf("%-20s %-8s %-30s %-8s %-10s", r.Provider, r.Status, model, tokens, latency)
		if r.Error != "" && (*verbose || r.Status == "FAIL") {
			fmt.Printf("  %s", r.Error)
		}
		fmt.Println()
	}
	fmt.Println(strings.Repeat("-", 80))
	fmt.Printf("Results: %d passed, %d failed\n\n", passed, failed)

	if failed > 0 {
		os.Exit(1)
	}
}

func cmdProfile(args []string) {
	if len(args) == 0 {
		args = []string{"show"}
	}

	switch args[0] {
	case "show":
		cmdProfileShow(args[1:])
	case "list":
		cmdProfileList(args[1:])
	case "switch":
		cmdProfileSwitch(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "Unknown profile subcommand: %s\nUsage: synroute profile [show|list|switch <name>]\n", args[0])
		os.Exit(1)
	}
}

func cmdProfileShow(args []string) {
	fs := flag.NewFlagSet("profile show", flag.ExitOnError)
	jsonOut := fs.Bool("json", false, "Output JSON")
	fs.Parse(args)

	ac, err := app.InitLight(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing: %v\n", err)
		os.Exit(1)
	}
	defer ac.Close()

	names := make([]string, len(ac.Providers))
	for i, p := range ac.Providers {
		names[i] = p.Name()
	}

	info := app.ShowProfile(names)

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(info)
		return
	}

	fmt.Printf("Active profile: %s\n", info["active"])
	fmt.Printf("Providers: %s\n", strings.Join(names, ", "))
	fmt.Println()
	fmt.Println("Available profiles:")
	for _, p := range app.AvailableProfiles() {
		marker := "  "
		if p.Name == info["active"] {
			marker = "* "
		}
		fmt.Printf("  %s%-10s %s\n", marker, p.Name, p.Description)
	}
}

func cmdProfileList(args []string) {
	fs := flag.NewFlagSet("profile list", flag.ExitOnError)
	jsonOut := fs.Bool("json", false, "Output JSON")
	fs.Parse(args)

	profiles := app.AvailableProfiles()
	active := app.GetActiveProfile()
	for i := range profiles {
		profiles[i].Active = profiles[i].Name == active
	}

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(profiles)
		return
	}

	for _, p := range profiles {
		marker := "  "
		if p.Active {
			marker = "* "
		}
		fmt.Printf("%s%-10s %s\n", marker, p.Name, p.Description)
	}
}

func cmdProfileSwitch(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: synroute profile switch <personal|work>")
		os.Exit(1)
	}

	newProfile := args[0]
	if err := app.SwitchProfile(newProfile); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Profile switched to: %s\n", newProfile)
	fmt.Println("Restart the server to apply changes.")
}

func cmdDoctor(args []string) {
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	jsonOut := fs.Bool("json", false, "Output JSON")
	fs.Parse(args)

	ac, err := app.InitLight(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing: %v\n", err)
		os.Exit(1)
	}
	defer ac.Close()

	checks := app.RunDiagnostics(context.Background(), ac)

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(checks)
		return
	}

	// Grouped output with status indicators
	currentCategory := ""
	okCount, warnCount, failCount := 0, 0, 0
	for _, c := range checks {
		if c.Category != currentCategory {
			if currentCategory != "" {
				fmt.Println()
			}
			fmt.Printf("[%s]\n", c.Category)
			currentCategory = c.Category
		}

		icon := "OK"
		switch c.Status {
		case "ok":
			okCount++
		case "warn":
			icon = "WARN"
			warnCount++
		case "fail":
			icon = "FAIL"
			failCount++
		}

		fmt.Printf("  %-5s %-25s %s\n", icon, c.Name, c.Message)
	}

	fmt.Printf("\nSummary: %d ok, %d warnings, %d failures\n", okCount, warnCount, failCount)

	if failCount > 0 {
		os.Exit(1)
	}
}

func cmdModels(args []string) {
	fs := flag.NewFlagSet("models", flag.ExitOnError)
	provider := fs.String("provider", "", "Filter by provider")
	jsonOut := fs.Bool("json", false, "Output JSON")
	fs.Parse(args)

	ac, err := app.InitLight(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing: %v\n", err)
		os.Exit(1)
	}
	defer ac.Close()

	models := app.ListModels(ac.Providers, ac.Profile, *provider)

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(models)
		return
	}

	fmt.Printf("Models (%d):\n\n", len(models))
	fmt.Printf("%-40s %-15s %-10s\n", "MODEL", "OWNED_BY", "CONTEXT")
	fmt.Println(strings.Repeat("-", 70))
	for _, m := range models {
		id := stringVal(m, "id")
		owner := stringVal(m, "owned_by")
		ctx := stringVal(m, "context")
		if ctx == "" {
			ctx = "-"
		}
		if *provider != "" {
			providerName := stringVal(m, "provider")
			if providerName != "" && !strings.EqualFold(providerName, *provider) {
				continue
			}
		}
		fmt.Printf("%-40s %-15s %-10s\n", id, owner, ctx)
	}
}

func stringVal(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		return fmt.Sprintf("%v", v)
	}
	return ""
}

func cmdChat(args []string) {
	fs := flag.NewFlagSet("chat", flag.ExitOnError)
	model := fs.String("model", "auto", "Model to use")
	message := fs.String("message", "", "One-shot message (non-interactive)")
	specFile := fs.String("spec-file", "", "Read spec from file and use as message (prepends 'Implement the following specification:')")
	system := fs.String("system", "", "Custom system prompt")
	project := fs.String("project", "", "Project name — creates ~/Development/<name>/ and works there")
	useWorktree := fs.Bool("worktree", false, "Run in isolated git worktree")
	maxAgents := fs.Int("max-agents", 5, "Max concurrent sub-agents")
	budgetTokens := fs.Int64("budget", 0, "Max total tokens budget (0 = unlimited)")
	resume := fs.Bool("resume", false, "Resume most recent session")
	sessionID := fs.String("session", "", "Resume specific session ID")
	verbose := fs.Int("verbose", 0, "Verbosity level: 0=compact, 1=normal, 2=verbose (also -v/-vv)")
	jsonEvents := fs.Bool("json-events", false, "Emit events as JSON lines to stderr")
	fs.Parse(args)

	// Support -v / -vv shorthand via remaining args
	for _, a := range fs.Args() {
		if a == "-v" {
			*verbose = 1
		} else if a == "-vv" {
			*verbose = 2
		}
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

	registry := tools.DefaultRegistry()
	cwd, _ := os.Getwd()

	// Create project directory if --project specified
	if *project != "" {
		home, _ := os.UserHomeDir()
		projectDir := filepath.Join(home, "Development", *project)
		if err := os.MkdirAll(projectDir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating project directory: %v\n", err)
			os.Exit(1)
		}
		cwd = projectDir
		fmt.Fprintf(os.Stderr, "Working in project: %s\n", projectDir)
	}

	// Set up timestamped log file — each run gets its own file, no overwriting
	logDir := filepath.Join(cwd, ".synroute", "logs")
	os.MkdirAll(logDir, 0755)
	logTimestamp := time.Now().Format("2006-01-02T15-04-05")
	logPath := filepath.Join(logDir, fmt.Sprintf("run-%s.log", logTimestamp))
	if logFile, err := os.Create(logPath); err == nil {
		log.SetOutput(io.MultiWriter(os.Stderr, logFile))
		defer logFile.Close()
		fmt.Fprintf(os.Stderr, "Log: %s\n", logPath)
	}

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
	config.AutoOrchestrate = true

	// Event bus for real-time observability
	bus := agent.NewEventBus()
	config.EventBus = bus
	config.Verbosity = *verbose
	_ = jsonEvents // used in RunOneShot path below

	ctx := context.Background()

	// Set up worktree isolation if requested
	var wtMgr *worktree.Manager
	var wt *worktree.Worktree
	if *useWorktree {
		wtMgr, err = worktree.NewManager(worktree.DefaultConfig())
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating worktree manager: %v\n", err)
			os.Exit(1)
		}

		wt, err = wtMgr.Create(cwd, "chat")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating worktree: %v\n", err)
			os.Exit(1)
		}

		config.WorkDir = wt.Path
		fmt.Fprintf(os.Stderr, "Working in isolated worktree: %s\n", wt.Path)

		// Start cleanup goroutine
		stopCleaner := worktree.StartCleaner(ctx, wtMgr, worktree.DefaultConfig().CleanupInterval)
		defer stopCleaner()
	}

	// If --spec-file provided, read file and compose message.
	// Also detect project language from spec content for pipeline routing.
	if *specFile != "" && *message == "" {
		specContent, err := os.ReadFile(*specFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading spec file: %v\n", err)
			os.Exit(1)
		}

		// Extract language from spec content (e.g., "Language: TypeScript" or "next.js")
		if lang := detectLanguageFromSpec(string(specContent)); lang != "" {
			config.ProjectLanguage = lang
			fmt.Fprintf(os.Stderr, "Detected language from spec: %s\n", lang)
		}

		var composed string
		if hasExistingProject(cwd) {
			composed = "Review and improve the existing implementation against this specification. " +
				"Inspect the current code with file_read, run tests with bash, fix any issues, " +
				"and fill in anything missing. Do NOT rewrite files that already work correctly.\n\n" +
				string(specContent)
			fmt.Fprintf(os.Stderr, "Existing project detected — running in review/fix mode\n")
		} else {
			composed = "Implement the following specification:\n\n" + string(specContent)
		}
		message = &composed
	}

	// Fallback: detect language from existing project files (re-runs without spec)
	if config.ProjectLanguage == "" {
		if env := environment.Detect(cwd); env != nil && env.Language != "" {
			config.ProjectLanguage = env.Language
		}
	}

	if *message != "" {
		// One-shot mode: work in the current directory so created files persist.
		response, err := agent.RunOneShot(ctx, ac.ProxyRouter, registry, config, *message)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(response)
		if wt != nil {
			fmt.Fprintf(os.Stderr, "Worktree still active at: %s\n", wt.Path)
		}
		return
	}

	renderer := agent.NewRenderer(os.Stdout)

	// Start event log renderer for interactive mode (writes to stderr)
	if bus != nil {
		events := bus.Subscribe()
		lr := agent.NewLogRenderer(os.Stderr, *verbose, *jsonEvents)
		go lr.Run(events)
	}

	// Create agent pool for sub-agent concurrency
	pool := agent.NewPool(config.MaxAgents)

	var ag *agent.Agent

	// Session resume: restore from database if requested
	if config.SessionID != "" && config.DB != nil {
		state, err := agent.LoadState(config.DB, config.SessionID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading session: %v\n", err)
			os.Exit(1)
		}
		ag = agent.RestoreAgent(ac.ProxyRouter, registry, renderer, state)
		fmt.Fprintf(os.Stderr, "Resumed session: %s (%d messages)\n", state.SessionID, len(state.Messages))
	} else if config.Resume && config.DB != nil {
		state, err := agent.LoadLatestState(config.DB)
		if err != nil {
			fmt.Fprintf(os.Stderr, "No session to resume: %v\n", err)
			ag = agent.New(ac.ProxyRouter, registry, renderer, config)
		} else {
			ag = agent.RestoreAgent(ac.ProxyRouter, registry, renderer, state)
			fmt.Fprintf(os.Stderr, "Resumed session: %s (%d messages)\n", state.SessionID, len(state.Messages))
		}
	} else {
		ag = agent.New(ac.ProxyRouter, registry, renderer, config)
	}

	ag.SetPool(pool)

	// Register delegation tools
	registry.Register(agent.NewDelegateTool(ag))
	registry.Register(agent.NewHandoffTool(ag))

	// Set budget tracker if configured
	if config.Budget != nil {
		ag.SetInputGuardrails(agent.NewGuardrailChain(&agent.SecretPatternGuardrail{}))
	}

	repl := agent.NewREPL(ag, renderer)
	if err := repl.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Auto-save session on exit
	if config.DB != nil {
		if err := ag.SaveState(config.DB); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to save session: %v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "Session saved: %s\n", ag.SessionID())
		}
	}

	// On exit, offer to clean up worktree
	if wt != nil && wtMgr != nil {
		fmt.Fprintf(os.Stderr, "\nWorktree at: %s\n", wt.Path)
		fmt.Fprintf(os.Stderr, "Branch: %s\n", wt.Branch)
		fmt.Fprintf(os.Stderr, "To keep changes, merge the branch. To discard, run: synroute worktree delete %s\n", wt.ID)
	}
}

func cmdMCPServe(args []string) {
	fs := flag.NewFlagSet("mcp-serve", flag.ExitOnError)
	addr := fs.String("addr", "127.0.0.1:8091", "Listen address (default localhost only)")
	token := fs.String("token", "", "Bearer token for auth (auto-generated if empty)")
	noAuth := fs.Bool("no-auth", false, "Disable authentication (not recommended)")
	fs.Parse(args)

	registry := tools.DefaultRegistry()
	cwd, _ := os.Getwd()

	srv := mcpserver.NewServer(registry, cwd)

	if !*noAuth {
		authToken := *token
		if authToken == "" {
			authToken = os.Getenv("SYNROUTE_MCP_TOKEN")
		}
		if authToken == "" {
			authToken = mcpserver.GenerateToken()
		}
		srv.SetToken(authToken)
		fmt.Fprintf(os.Stderr, "Auth token: %s\n", authToken)
	}

	fmt.Fprintf(os.Stderr, "MCP server listening on %s\n", *addr)
	fmt.Fprintf(os.Stderr, "Tools: %d registered\n", len(registry.List()))
	fmt.Fprintf(os.Stderr, "Endpoints:\n")
	fmt.Fprintf(os.Stderr, "  POST %s/mcp/initialize\n", *addr)
	fmt.Fprintf(os.Stderr, "  POST %s/mcp/tools/list\n", *addr)
	fmt.Fprintf(os.Stderr, "  POST %s/mcp/tools/call\n", *addr)

	if err := srv.Serve(context.Background(), *addr); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// hasExistingProject checks if a directory already contains source code from
// a previous build. Used by --spec-file to switch between "build from scratch"
// and "review/fix existing" modes.
func hasExistingProject(dir string) bool {
	// Check for language config files (go.mod, package.json, Cargo.toml, etc.)
	if env := environment.Detect(dir); env != nil && env.Language != "" {
		return true
	}
	// Check for source files in root and one level deep
	extensions := []string{"*.go", "*.py", "*.rs", "*.ts", "*.js", "*.java", "*.cs", "*.rb"}
	for _, ext := range extensions {
		matches, _ := filepath.Glob(filepath.Join(dir, ext))
		if len(matches) > 0 {
			return true
		}
		matches, _ = filepath.Glob(filepath.Join(dir, "*", ext))
		if len(matches) > 0 {
			return true
		}
	}
	return false
}

// detectLanguageFromSpec scans spec text for language/framework indicators.
// Uses two tiers: explicit declarations ("Language: TypeScript") first,
// then framework keywords ("next.js", "django") as fallback.
func detectLanguageFromSpec(content string) string {
	lower := strings.ToLower(content)

	// Tier 1: Explicit language declarations (most reliable)
	// Matches patterns like "Language: TypeScript", "Language/Runtime: Python"
	langPatterns := []struct {
		pattern  string
		language string
	}{
		{`language[/\w]*[:\s]+typescript`, "javascript"},
		{`language[/\w]*[:\s]+javascript`, "javascript"},
		{`language[/\w]*[:\s]+node`, "javascript"},
		{`language[/\w]*[:\s]+python`, "python"},
		{`language[/\w]*[:\s]+golang`, "go"},
		{`language[/\w]*[:\s]+rust`, "rust"},
		{`language[/\w]*[:\s]+java\b`, "java"},
		{`language[/\w]*[:\s]+c#`, "csharp"},
		{`language[/\w]*[:\s]+csharp`, "csharp"},
		{`language[/\w]*[:\s]+sql`, "sql"},
		{`language[/\w]*[:\s]+ruby`, "ruby"},
		{`language[/\w]*[:\s]+r\b`, "r"},
		{`language[/\w]*[:\s]+cpp`, "cpp"},
		{`language[/\w]*[:\s]+c\+\+`, "cpp"},
	}
	for _, p := range langPatterns {
		if matched, _ := regexp.MatchString(p.pattern, lower); matched {
			return p.language
		}
	}

	// Special case: "Language: Go" needs word boundary (avoid "going", "google")
	if matched, _ := regexp.MatchString(`language[/\w]*[:\s]+go\b`, lower); matched {
		return "go"
	}

	// Tier 2: Framework/ecosystem keywords (less reliable but catches most specs)
	// Ordered: check more specific frameworks first
	frameworkIndicators := []struct {
		keywords []string
		language string
	}{
		{[]string{"next.js", "nextjs", "react", "express", "node.js", "npm install", "package.json", "typescript", ".tsx", ".jsx"}, "javascript"},
		{[]string{"django", "flask", "fastapi", "pytorch", "tensorflow", "pandas", "sklearn", "pip install", "requirements.txt", "pyproject.toml"}, "python"},
		{[]string{"cargo.toml", "cargo build", "cargo test"}, "rust"},
		{[]string{"spring boot", "maven", "gradle", "pom.xml"}, "java"},
		{[]string{"dotnet", "nuget", "entity framework", ".csproj"}, "csharp"},
		{[]string{"prisma", "drizzle"}, "javascript"}, // ORM = Node.js ecosystem
		{[]string{"postgresql", "mysql", "sqlite", "create table", "insert into"}, "sql"},
		{[]string{"go.mod", "go build", "go test"}, "go"},
	}
	for _, fi := range frameworkIndicators {
		for _, kw := range fi.keywords {
			if strings.Contains(lower, kw) {
				return fi.language
			}
		}
	}

	return ""
}
