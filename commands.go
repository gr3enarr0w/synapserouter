package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/gr3enarr0w/synapserouter/internal/agent"
	"github.com/gr3enarr0w/synapserouter/internal/app"
	"github.com/gr3enarr0w/synapserouter/internal/marketplace"
	"github.com/gr3enarr0w/synapserouter/internal/subscriptions"
	"github.com/gr3enarr0w/synapserouter/internal/environment"
	"github.com/gr3enarr0w/synapserouter/internal/mcp"
	"github.com/gr3enarr0w/synapserouter/internal/mcpserver"
	"github.com/gr3enarr0w/synapserouter/internal/tasks"
	"github.com/gr3enarr0w/synapserouter/internal/tools"
	"github.com/gr3enarr0w/synapserouter/internal/worktree"
)

// Build-time variables set via ldflags.
// When ldflags are not provided (plain "go build"), resolveVersionInfo()
// populates these from runtime/debug.BuildInfo (VCS data embedded by Go 1.18+).
var (
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"
)

func init() {
	resolveVersionInfo()
}

// resolveVersionInfo fills version/commit/buildDate from Go's embedded VCS
// build info when ldflags were not injected at build time.
func resolveVersionInfo() {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}

	// Check if we should override the current version with git tag info.
	// We override if: version is "dev", OR version looks like a module pseudo-version
	// (e.g., v1.0.3-0.20260404210859-96ecb4de7580+dirty instead of a clean tag like v1.09).
	shouldOverride := version == "dev" || isModulePseudoVersion(version)

	if shouldOverride {
		// Prefer git describe (gives clean tag-based version like v1.0.1).
		if desc, err := gitDescribe(); err == nil && desc != "" {
			version = desc
		} else if v := info.Main.Version; v != "" && v != "(devel)" {
			// Fall back to module version (set by go install module@version).
			version = v
		} else {
			version = versionFromVCS(info)
		}
	}

	if commit == "unknown" {
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" {
				commit = s.Value
				if len(commit) > 12 {
					commit = commit[:12]
				}
				break
			}
		}
	}

	if buildDate == "unknown" {
		for _, s := range info.Settings {
			if s.Key == "vcs.time" {
				buildDate = s.Value
				break
			}
		}
	}
}

// versionFromVCS tries `git describe --tags` first (gives semver-like output),
// then falls back to the short VCS revision from build info.
func versionFromVCS(info *debug.BuildInfo) string {
	// Try git describe for a nice tag-based version (e.g. v1.0.1 or v1.0.1-3-gabcdef).
	if desc, err := gitDescribe(); err == nil && desc != "" {
		return desc
	}
	// Fall back to short commit hash.
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" {
			rev := s.Value
			if len(rev) > 12 {
				rev = rev[:12]
			}
			dirty := false
			for _, s2 := range info.Settings {
				if s2.Key == "vcs.modified" && s2.Value == "true" {
					dirty = true
					break
				}
			}
			if dirty {
				return "dev-" + rev + "-dirty"
			}
			return "dev-" + rev
		}
	}
	return "dev"
}

// gitDescribe shells out to git describe to get the most recent tag.
func gitDescribe() (string, error) {
	out, err := execGitDescribe()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// isModulePseudoVersion checks if a version string looks like a Go module
// pseudo-version (e.g., v1.0.3-0.20260404210859-96ecb4de7580+dirty)
// instead of a clean git tag (e.g., v1.09, v1.0.1).
// Pseudo-versions contain a timestamp and commit hash after the base version.
func isModulePseudoVersion(v string) bool {
	if v == "" || v == "dev" {
		return false
	}
	// Pseudo-versions have format: vX.Y.Z-YYYYMMDDHHMMSS-<commit>[-dirty]
	// They contain a 14-digit timestamp after the first dash.
	parts := strings.Split(v, "-")
	if len(parts) >= 3 {
		// Check if second part looks like a timestamp (14 digits)
		if len(parts[1]) == 14 {
			for _, c := range parts[1] {
				if c < '0' || c > '9' {
					return false
				}
			}
			return true
		}
	}
	return false
}

func cmdVersion() {
	printLogo()
	profile := app.GetActiveProfile()
	fmt.Printf("synroute %s (%s) built %s | profile: %s | %s\n",
		version, commit, buildDate, profile, runtime.Version())
}

// execGitDescribe runs "git describe --tags --always --dirty" and returns the output.
func execGitDescribe() (string, error) {
	cmd := exec.Command("git", "describe", "--tags", "--always", "--dirty")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func printUsage() {
	fmt.Println(`synroute — LLM proxy router and coding agent

Usage:
  synroute [command]

Commands:
  serve       Start the HTTP server
  chat        Interactive agent REPL or one-shot message
  code        Pipeline-aware code mode with TUI (default if no command given)
  mcp         Manage MCP server registrations (add, list, remove, status)
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

func cmdRecommend(args []string) {
	fs := flag.NewFlagSet("recommend", flag.ExitOnError)
	jsonOutput := fs.Bool("json", false, "Output as JSON")
	fs.Parse(args)

	ctx := context.Background()
	ac, err := app.InitLight(ctx)
	if err != nil {
		log.Fatalf("Failed to initialize: %v", err)
	}

	// Get available models
	models := app.ListModels(ac.Providers, ac.Profile, "")

	// Convert to ModelInfo for the recommendation engine
	var available []subscriptions.ModelInfo
	for _, m := range models {
		available = append(available, subscriptions.ModelInfo{
			ID:      stringVal(m, "id"),
			OwnedBy: stringVal(m, "owned_by"),
		})
	}

	// Load catalog and generate recommendation
	catalog := agent.LoadModelCatalog()
	report := agent.GenerateRecommendation(available, catalog, ac.Profile)

	if *jsonOutput {
		data, _ := json.MarshalIndent(report, "", "  ")
		fmt.Println(string(data))
	} else {
		fmt.Print(agent.FormatRecommendation(report))
	}
}

func cmdConfig(args []string) {
	if len(args) > 0 && args[0] == "show" {
		// Show current effective config
		tc, err := app.LoadTierConfig()
		if err != nil {
			log.Printf("Warning: %v", err)
		}

		source := "OLLAMA_CHAIN env var (no YAML config found)"
		if tc != nil {
			home, _ := os.UserHomeDir()
			if _, err := os.Stat(".synroute.yaml"); err == nil {
				source = ".synroute.yaml (project-level)"
			} else {
				source = home + "/.synroute/config.yaml (user-level)"
			}
		}

		fmt.Print(app.FormatTierConfig(tc, source))

		warnings := app.ValidateTierConfig(tc)
		for _, w := range warnings {
			fmt.Printf("WARNING: %s\n", w)
		}
		return
	}

	if len(args) > 0 && args[0] == "generate" {
		// Generate config from current OLLAMA_CHAIN
		chainStr := os.Getenv("OLLAMA_CHAIN")
		if chainStr == "" {
			fmt.Println("No OLLAMA_CHAIN configured. Set it in .env first, then run synroute config generate.")
			return
		}

		levels := app.ParseOllamaChain(chainStr)
		tc := &app.TierConfig{Tiers: make(map[string][]string)}

		// Auto-classify into tiers (same logic as autoClassifyTiers)
		total := len(levels)
		for i, models := range levels {
			tier := "mid"
			switch {
			case total <= 2:
				tier = "frontier"
			case i < total/3:
				tier = "cheap"
			case i >= total-total/3:
				tier = "frontier"
			}
			tc.Tiers[tier] = append(tc.Tiers[tier], models...)
		}

		if err := app.WriteTierConfig(tc); err != nil {
			log.Fatalf("Failed to write config: %v", err)
		}
		fmt.Print(app.FormatTierConfig(tc, "~/.synroute/config.yaml (generated)"))
		fmt.Println("\nConfig saved. Edit ~/.synroute/config.yaml to customize.")
		return
	}

	// Default: show help
	fmt.Println("Usage: synroute config <subcommand>")
	fmt.Println()
	fmt.Println("Subcommands:")
	fmt.Println("  show       Show current effective tier configuration")
	fmt.Println("  generate   Generate YAML config from current OLLAMA_CHAIN")
	fmt.Println()
	fmt.Println("Config file locations (priority order):")
	fmt.Println("  1. .synroute.yaml (project-level, in CWD)")
	fmt.Println("  2. ~/.synroute/config.yaml (user-level)")
	fmt.Println("  3. OLLAMA_CHAIN env var (fallback)")
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
	background := fs.Bool("background", false, "Run agent in background and return task ID")
	message := fs.String("message", "", "One-shot message (non-interactive)")
	specFile := fs.String("spec-file", "", "Read spec from file and use as message (prepends 'Implement the following specification:')")
	system := fs.String("system", "", "Custom system prompt")
	project := fs.String("project", "", "Project name — creates ~/Development/<name>/ and works there")
	useWorktree := fs.Bool("worktree", false, "Run in isolated git worktree")
	confidential := fs.Bool("confidential", false, "Confidential mode: blocks external API calls (web_search, web_fetch)")
	maxAgents := fs.Int("max-agents", 5, "Max concurrent sub-agents")
	budgetTokens := fs.Int64("budget", 0, "Max total tokens budget (0 = unlimited)")
	resume := fs.Bool("resume", false, "Resume most recent session")
	sessionID := fs.String("session", "", "Resume specific session ID")
	verbose := fs.Int("verbose", 0, "Verbosity level: 0=compact, 1=normal, 2=verbose (also -v/-vv)")
	jsonEvents := fs.Bool("json-events", false, "Emit events as JSON lines to stderr")
	usePipeline := fs.Bool("pipeline", false, "Force legacy 6-phase pipeline (default: frontier model with pipeline tools)")
	fs.Parse(args)

	// BUG 2 (11.1): If --message was explicitly passed but empty, error and exit
	messageFlagSet := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "message" {
			messageFlagSet = true
		}
	})
	if messageFlagSet && *message == "" {
		fmt.Fprintln(os.Stderr, "Error: --message flag requires a non-empty value")
		os.Exit(1)
	}

	// Support -v / -vv shorthand via remaining args
	for _, a := range fs.Args() {
		if a == "-v" {
			*verbose = 1
		} else if a == "-vv" {
			*verbose = 2
		}
	}

	// Set up timestamped log file BEFORE InitLight — each run gets its own file, no overwriting
	cwd, _ := os.Getwd()
	if *project != "" {
		home, _ := os.UserHomeDir()
		cwd = filepath.Join(home, "Development", *project)
	}
	logDir := filepath.Join(cwd, ".synroute", "logs")
	os.MkdirAll(logDir, 0755)
	logTimestamp := time.Now().Format("2006-01-02T15-04-05")
	logPath := filepath.Join(logDir, fmt.Sprintf("run-%s.log", logTimestamp))
	logFile, err := os.Create(logPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating log file: %v\n", err)
		os.Exit(1)
	}
	defer logFile.Close()

	// Redirect logs based on mode BEFORE InitLight (so provider init logs are also suppressed)
	if *message != "" && *verbose == 0 {
		// --message mode (non-verbose): logs to file only, user sees only response
		log.SetOutput(logFile)
	} else {
		// Interactive mode: logs to stderr and file
		log.SetOutput(io.MultiWriter(os.Stderr, logFile))
		fmt.Fprintf(os.Stderr, "Log: %s\n", logPath)
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

	// First-time hint
	home, _ := os.UserHomeDir()
	configPath := filepath.Join(home, ".synroute", "config.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		fmt.Fprintln(os.Stderr, "First time? Run 'synroute setup' for guided configuration.")
	}

	ac.InitFull()

	registry := tools.DefaultRegistry()

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

	config := agent.DefaultConfig()
	config.Model = *model
	config.WorkDir = cwd
	config.MaxAgents = *maxAgents
	config.DB = ac.DB
	if ts, err := agent.NewToolOutputStore(ac.DB); err != nil {
		log.Printf("[Agent] failed to initialize tool store: %v", err)
	} else {
		config.ToolStore = ts
	}
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
	config.PlannerProviders = ac.PlannerProviders
	config.MergeProvider = ac.MergeProvider
	// Pass full provider list so hasProviders() can find standalone providers
	// (e.g., planner models) that aren't in the escalation chain.
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

	// MCP client — load config and connect to registered servers
	mcpCfg, mcpErr := mcp.LoadConfig(mcp.DefaultConfigPath())
	if mcpErr != nil {
		log.Printf("Warning: failed to load MCP config: %v", mcpErr)
	} else if len(mcpCfg.Servers) > 0 {
		mcpClient := mcp.NewClientFromConfig(mcpCfg)
		mcpCtx, mcpCancel := context.WithTimeout(context.Background(), 10*time.Second)
		mcpClient.ConnectAll(mcpCtx, 2)
		mcpCancel()
		config.MCPClient = mcpClient
	}

	// Event bus for real-time observability
	bus := agent.NewEventBus()
	config.EventBus = bus
	config.Verbosity = *verbose
	_ = jsonEvents // used in RunOneShot path below

	ctx := context.Background()

	// Worktree isolation - mandatory for --message mode to prevent writes to main repo
	var wtMgr *worktree.Manager
	var wt *worktree.Worktree
	useWorktreeForMessage := *message != "" || *useWorktree
	if useWorktreeForMessage {
		wtMgr, err = worktree.NewManager(worktree.DefaultConfig())
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating worktree manager: %v\n", err)
			os.Exit(1)
		}

		wt, err = wtMgr.Create(cwd, "chat")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating worktree: %v\n", err)
			if *message != "" {
				fmt.Fprintf(os.Stderr, "ABORTING: Cannot run --message mode without worktree isolation\n")
			}
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
	// Track spec file path for tool-layer protection — regardless of --message flag
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
		config.NonInteractive = true
		// If --spec-file provided with --message, prepend spec content to message
		if *specFile != "" {
			specContent, err := os.ReadFile(*specFile)
			if err == nil {
				composed := "Implement the following specification:\n\n" + string(specContent) + "\n\nUser instruction: " + *message
				message = &composed
			}
		}

		// Background mode: create task, run in goroutine, return task ID
		if *background {
			taskID := fmt.Sprintf("task-%d", time.Now().UnixNano())
			worktreePath := cwd
			if wt != nil {
				worktreePath = wt.Path
			}

			// Create task record
			_, err := ac.TaskManager.CreateTask(taskID, worktreePath, *message)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error creating task: %v\n", err)
				os.Exit(1)
			}

			// Update status to running
			if err := ac.TaskManager.UpdateTask(taskID, tasks.StatusRunning, "", ""); err != nil {
				log.Printf("Warning: failed to update task status: %v", err)
			}

			// Run agent in background goroutine
			go func() {
				msg := *message
				response, err := agent.RunOneShot(ctx, ac.ProxyRouter, registry, config, msg)
				if err != nil {
					ac.TaskManager.UpdateTask(taskID, tasks.StatusFailed, "", err.Error())
					log.Printf("Background task %s failed: %v", taskID, err)
					return
				}

				// Create PR if worktree exists
				prURL := ""
				if wt != nil {
					cmd := exec.Command("gh", "pr", "create",
						"--title", fmt.Sprintf("Agent task: %s", taskID),
						"--body", response,
						"--base", "main")
					cmd.Dir = wt.Path
					output, err := cmd.CombinedOutput()
					if err != nil {
						log.Printf("Warning: gh pr create failed: %v - %s", err, output)
					} else {
						prURL = strings.TrimSpace(string(output))
					}
				}

				ac.TaskManager.UpdateTask(taskID, tasks.StatusCompleted, prURL, "")
				log.Printf("Background task %s completed", taskID)
			}()

			fmt.Printf("Task started: %s\n", taskID)
			fmt.Printf("Worktree: %s\n", worktreePath)
			fmt.Println("Run 'synroute tasks' to check status")
			return
		}

		// One-shot mode: work in the current directory so created files persist.
		// Parse slash commands before passing to RunOneShot
		msg := *message
		if strings.HasPrefix(msg, "/") {
			parts := strings.SplitN(strings.TrimPrefix(msg, "/"), " ", 2)
			cmd := parts[0]
			arg := ""
			if len(parts) > 1 {
				arg = parts[1]
			}
			switch cmd {
			case "research":
				config.PipelineMode = "research"
				msg = arg
			case "plan":
				config.PipelineMode = "plan"
				msg = arg
			case "review":
				config.PipelineMode = "review"
				msg = arg
			case "check":
				config.PipelineMode = "check"
				msg = arg
			case "fix":
				config.PipelineMode = "fix"
				msg = arg
			}
		}
		response, err := agent.RunOneShot(ctx, ac.ProxyRouter, registry, config, msg)
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
		ag = agent.RestoreAgent(ac.ProxyRouter, registry, renderer, state)
		fmt.Fprintf(os.Stderr, "Resumed session: %s (%d messages)\n", state.SessionID, len(state.Messages))
	} else if config.Resume && config.DB != nil {
		state, err := agent.LoadLatestState(config.DB, userID)
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

	// Conversation tier — configurable via SYNROUTE_CONVERSATION_TIER
	// --message mode defaults to cheap (escalate on failure) instead of frontier
	convTier := agent.TierFrontier
	if *message != "" {
		convTier = agent.TierCheap
	}
	if tierEnv := os.Getenv("SYNROUTE_CONVERSATION_TIER"); tierEnv != "" {
		switch strings.ToLower(tierEnv) {
		case "cheap":
			convTier = agent.TierCheap
		case "mid":
			convTier = agent.TierMid
		case "frontier":
			convTier = agent.TierFrontier
		}
	}
	ag.SetMinProviderLevel(ag.ProviderLevelForTier(convTier))

	// Register delegation tools
	registry.Register(agent.NewDelegateTool(ag))
	registry.Register(agent.NewHandoffTool(ag))
	if !config.AutoOrchestrate {
		agent.RegisterPipelineTools(registry, ag)
	}

	// Set budget tracker if configured
	if config.Budget != nil {
		ag.SetInputGuardrails(agent.NewGuardrailChain(&agent.SecretPatternGuardrail{}))
	}

	// Interactive permission prompting for chat mode
	ag.SetPermissions(tools.NewPermissionChecker(tools.ModeInteractive))
	ag.SetPermissionPrompt(agent.DefaultPermissionPrompt(os.Stdout, os.Stdin))

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

// cmdAuth handles subscription provider authentication.
// cmdWorktree manages git worktrees created by synroute chat --worktree.
// Usage: synroute worktree list
//
//	synroute worktree delete <id>
func cmdWorktree(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: synroute worktree <list|delete> [id]")
		return
	}

	mgr, err := worktree.NewManager(worktree.Config{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	switch args[0] {
	case "list":
		trees := mgr.List()
		if len(trees) == 0 {
			fmt.Println("No active worktrees.")
			return
		}
		for _, wt := range trees {
			fmt.Printf("  %s  %s  (branch: %s, session: %s, expires: %s)\n",
				wt.ID, wt.Path, wt.Branch, wt.SessionID,
				wt.ExpiresAt.Format("2006-01-02 15:04"))
		}
	case "delete":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: synroute worktree delete <id>")
			os.Exit(1)
		}
		if err := mgr.Delete(args[1]); err != nil {
			fmt.Fprintf(os.Stderr, "Error deleting worktree: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Worktree %s deleted.\n", args[1])
	default:
		fmt.Fprintf(os.Stderr, "Unknown worktree subcommand: %s\nUsage: synroute worktree <list|delete> [id]\n", args[0])
		os.Exit(1)
	}
}

// cmdTasks shows status of background tasks
// Usage: synroute tasks [options]
//
//	Options:
//	  --status <pending|running|completed|failed>  Filter by status
//	  --id <task-id>                               Show single task details
//	  --json                                       Output as JSON
func cmdTasks(args []string) {
	fs := flag.NewFlagSet("tasks", flag.ExitOnError)
	status := fs.String("status", "", "Filter by status (pending, running, completed, failed)")
	taskID := fs.String("id", "", "Show single task by ID")
	jsonOut := fs.Bool("json", false, "Output as JSON")
	fs.Parse(args)

	ac, err := app.InitLight(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing: %v\n", err)
		os.Exit(1)
	}
	defer ac.Close()

	var taskStatus tasks.TaskStatus
	if *status != "" {
		taskStatus = tasks.TaskStatus(*status)
	}

	var tasksList []*tasks.BackgroundTask
	if *taskID != "" {
		task, err := ac.TaskManager.GetTask(*taskID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: task not found: %s\n", *taskID)
			os.Exit(1)
		}
		tasksList = []*tasks.BackgroundTask{task}
	} else {
		tasksList, err = ac.TaskManager.GetTasks(taskStatus)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error fetching tasks: %v\n", err)
			os.Exit(1)
		}
	}

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(tasksList)
		return
	}

	if len(tasksList) == 0 {
		fmt.Println("No background tasks found.")
		return
	}

	fmt.Printf("%-20s %-10s %-40s %-25s %-25s %s\n",
		"ID", "STATUS", "WORKTREE", "STARTED", "ENDED", "PR URL")
	fmt.Println(strings.Repeat("-", 130))
	for _, task := range tasksList {
		endTime := "running"
		if task.EndTime.Valid {
			endTime = task.EndTime.Time.Format("2006-01-02 15:04:05")
		}
		prURL := "-"
		if task.PRURL.Valid {
			prURL = task.PRURL.String
		}
		fmt.Printf("%-20s %-10s %-40s %-25s %-25s %s\n",
			task.ID, task.Status, task.WorktreePath,
			task.StartTime.Format("2006-01-02 15:04:05"), endTime, prURL)
	}
}

// Usage: synroute auth login codex|gemini
//
//	synroute auth status
func cmdAuth(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: synroute auth <command>")
		fmt.Println()
		fmt.Println("Commands:")
		fmt.Println("  login <provider>   Authenticate with a subscription provider")
		fmt.Println("                     Providers: codex (OpenAI), gemini (Google)")
		fmt.Println("  status             Show credential status for all providers")
		fmt.Println()
		fmt.Println("Examples:")
		fmt.Println("  synroute auth login codex    # Opens browser for OpenAI OAuth")
		fmt.Println("  synroute auth login gemini   # Opens browser for Google OAuth")
		fmt.Println("  synroute auth status         # Check token expiry")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	switch args[0] {
	case "login":
		if len(args) < 2 {
			fmt.Println("Usage: synroute auth login <provider>")
			fmt.Println("Providers: codex, gemini")
			os.Exit(1)
		}
		provider := args[1]
		switch strings.ToLower(provider) {
		case "codex", "openai":
			provider = "openai"
		case "gemini", "google":
			provider = "gemini"
		default:
			fmt.Fprintf(os.Stderr, "Unknown provider: %s\nSupported: codex, gemini\n", provider)
			os.Exit(1)
		}

		fmt.Printf("Authenticating with %s...\n", provider)
		fmt.Println("A browser window will open for you to sign in.")
		fmt.Println()

		credential, err := subscriptions.Login(ctx, provider, subscriptions.LoginOptions{})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Authentication failed: %v\n", err)
			os.Exit(1)
		}

		if err := subscriptions.StoreCredential(provider, credential); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to store credential: %v\n", err)
			os.Exit(1)
		}

		email := credential.Email
		if email == "" {
			email = "(unknown)"
		}
		fmt.Printf("\n✓ Authenticated as %s\n", email)
		fmt.Println("Credential stored. Run 'synroute test' to verify.")

	case "status":
		creds, err := subscriptions.LoadAllStoredCredentials()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to load credentials: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("Subscription Provider Credentials:")
		fmt.Println("──────────────────────────────────")
		if len(creds) == 0 {
			fmt.Println("  No credentials stored.")
			fmt.Println("  Run 'synroute auth login codex' or 'synroute auth login gemini'")
			return
		}
		for provider, providerCreds := range creds {
			for _, c := range providerCreds {
				status := "✓ valid"
				if c.NeedsRefresh(0) {
					status = "⚠ expired"
				}
				email := c.Email
				if email == "" {
					email = "(no email)"
				}
				fmt.Printf("  %-10s %s  %s\n", provider, status, email)
			}
		}

	default:
		fmt.Fprintf(os.Stderr, "Unknown auth command: %s\nRun 'synroute auth' for help.\n", args[0])
		os.Exit(1)
	}
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

func cmdSearch(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: synroute search <subcommand>")
		fmt.Println("Subcommands:")
		fmt.Println("  stats    Show search backend circuit breaker statistics")
		os.Exit(1)
	}

	switch args[0] {
	case "stats":
		runSearchStats()
	default:
		fmt.Fprintf(os.Stderr, "Unknown search subcommand: %s\n", args[0])
		os.Exit(1)
	}
}

func runSearchStats() {
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		home, _ := os.UserHomeDir()
		dbPath = filepath.Join(home, ".mcp", "proxy", "usage.db")
	}
	if strings.HasPrefix(dbPath, "~/") {
		home, _ := os.UserHomeDir()
		dbPath = filepath.Join(home, dbPath[2:])
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	rows, err := db.Query(`
		SELECT backend_name, state, failure_count, last_failure, cooldown_until
		FROM search_circuit_breakers
		ORDER BY backend_name
	`)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error querying circuit breakers: %v\n", err)
		os.Exit(1)
	}
	defer rows.Close()

	var found bool
	fmt.Println("Search Backend Circuit Breaker Status")
	fmt.Println("=====================================")
	fmt.Printf("%-20s %-10s %-8s %-20s %-20s\n", "Backend", "State", "Failures", "Last Failure", "Cooldown Until")
	fmt.Println(strings.Repeat("-", 90))

	for rows.Next() {
		found = true
		var backendName, state string
		var failureCount int
		var lastFailure, cooldownUntil sql.NullString

		if err := rows.Scan(&backendName, &state, &failureCount, &lastFailure, &cooldownUntil); err != nil {
			fmt.Fprintf(os.Stderr, "Error scanning row: %v\n", err)
			continue
		}

		lastFailureStr := "N/A"
		if lastFailure.Valid {
			lastFailureStr = lastFailure.String
		}

		cooldownStr := "N/A"
		if cooldownUntil.Valid {
			cooldownStr = cooldownUntil.String
		}

		fmt.Printf("%-20s %-10s %-8d %-20s %-20s\n", backendName, state, failureCount, lastFailureStr, cooldownStr)
	}

	if !found {
		fmt.Println("\nNo search stats yet — run some searches first.")
	}
}

// cmdSkills handles the 'skills' command for skill discovery
func cmdSkills(args []string) {
	if len(args) < 2 {
		fmt.Println("Usage: synroute skills <list|install> [name]")
		fmt.Println()
		fmt.Println("Commands:")
		fmt.Println("  list            List available and installed skills")
		fmt.Println("  install <name>  Install a skill from the catalog")
		fmt.Println()
		fmt.Println("Examples:")
		fmt.Println("  synroute skills list")
		fmt.Println("  synroute skills install rust-patterns")
		return
	}

	subcmd := args[1]

	switch subcmd {
	case "list":
		skills, err := marketplace.ListSkills()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing skills: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("Available Skills")
		fmt.Println("================")
		fmt.Println()

		// Group by category
		categories := make(map[string][]marketplace.SkillInfo)
		for _, skill := range skills {
			categories[skill.Category] = append(categories[skill.Category], skill)
		}

		for _, cat := range []string{"backend", "frontend", "data", "general", "testing"} {
			catSkills, ok := categories[cat]
			if !ok || len(catSkills) == 0 {
				continue
			}

			fmt.Printf("[%s]\n", strings.ToUpper(cat))
			for _, skill := range catSkills {
				status := "○"
				if skill.Installed {
					status = "●"
				}
				fmt.Printf("  %s %-25s %s\n", status, skill.Name, skill.Description)
			}
			fmt.Println()
		}

	case "install":
		if len(args) < 3 {
			fmt.Println("Usage: synroute skills install <name>")
			fmt.Println()
			fmt.Println("Run 'synroute skills list' to see available skills.")
			return
		}

		name := args[2]
		err := marketplace.InstallSkill(name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✓ Skill '%s' installed successfully\n", name)
		fmt.Println()
		fmt.Println("The skill is now available for intent routing.")

	default:
		fmt.Fprintf(os.Stderr, "Unknown subcommand: %s\n", subcmd)
		fmt.Println("Run 'synroute skills' for usage.")
		os.Exit(1)
	}
}

// cmdIntegrations handles the 'integrations' command for MCP server bundles
func cmdIntegrations(args []string) {
	if len(args) < 2 {
		fmt.Println("Usage: synroute integrations <list|add> [name]")
		fmt.Println()
		fmt.Println("Commands:")
		fmt.Println("  list         List available integrations and their status")
		fmt.Println("  add <name>   Show setup instructions for an integration")
		fmt.Println()
		fmt.Println("Examples:")
		fmt.Println("  synroute integrations list")
		fmt.Println("  synroute integrations add github")
		return
	}

	subcmd := args[1]

	switch subcmd {
	case "list":
		integrations := marketplace.ListIntegrations()

		fmt.Println("MCP Server Integrations")
		fmt.Println("=======================")
		fmt.Println()

		for _, integ := range integrations {
			status := "○"
			if integ.Installed {
				status = "●"
			}
			fmt.Printf("%s %-12s %s\n", status, integ.Name, integ.Description)
			if len(integ.RequiredEnvVars) > 0 {
				fmt.Printf("   Required: %s\n", strings.Join(integ.RequiredEnvVars, ", "))
			}
		}
		fmt.Println()
		fmt.Println("Legend: ● installed (env vars set)  ○ not installed")

	case "add":
		if len(args) < 3 {
			fmt.Println("Usage: synroute integrations add <name>")
			fmt.Println()
			fmt.Println("Run 'synroute integrations list' to see available integrations.")
			return
		}

		name := args[2]
		instructions, err := marketplace.AddIntegration(name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(instructions)

	default:
		fmt.Fprintf(os.Stderr, "Unknown subcommand: %s\n", subcmd)
		fmt.Println("Run 'synroute integrations' for usage.")
		os.Exit(1)
	}
}
