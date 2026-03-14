package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/app"
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
	fmt.Println(`synroute — LLM proxy router

Usage:
  synroute [command]

Commands:
  serve       Start the HTTP server (default if no command given)
  test        Smoke test providers
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
