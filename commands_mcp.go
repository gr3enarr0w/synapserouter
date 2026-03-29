package main

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/mcp"
)

func cmdMCP(args []string) {
	if len(args) == 0 {
		printMCPUsage()
		return
	}

	switch args[0] {
	case "add":
		cmdMCPAdd(args[1:])
	case "list":
		cmdMCPList()
	case "remove":
		cmdMCPRemove(args[1:])
	case "status":
		cmdMCPStatus()
	default:
		fmt.Fprintf(os.Stderr, "Unknown mcp subcommand: %s\n", args[0])
		printMCPUsage()
		os.Exit(1)
	}
}

func printMCPUsage() {
	fmt.Println(`Usage: synroute mcp <subcommand>

Subcommands:
  add      Register an MCP server
  list     List registered MCP servers
  remove   Remove an MCP server
  status   Check connectivity to all servers`)
}

func cmdMCPAdd(args []string) {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: synroute mcp add <name> <url> [api-key]")
		os.Exit(1)
	}

	name := args[0]
	url := args[1]
	apiKey := ""
	if len(args) >= 3 {
		apiKey = args[2]
	}

	cfgPath := mcp.DefaultConfigPath()
	cfg, err := mcp.LoadConfig(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	cfg.AddServerConfig(name, url, apiKey)

	if err := mcp.SaveConfig(cfgPath, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Registered MCP server: %s at %s\n", name, url)
}

func cmdMCPList() {
	cfgPath := mcp.DefaultConfigPath()
	cfg, err := mcp.LoadConfig(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	if len(cfg.Servers) == 0 {
		fmt.Println("No MCP servers registered.")
		fmt.Println("Use 'synroute mcp add <name> <url>' to register one.")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tURL\tAUTH")
	for _, s := range cfg.Servers {
		auth := "none"
		if s.APIKey != "" {
			auth = "api-key"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", s.Name, s.URL, auth)
	}
	w.Flush()
}

func cmdMCPRemove(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: synroute mcp remove <name>")
		os.Exit(1)
	}

	name := args[0]
	cfgPath := mcp.DefaultConfigPath()
	cfg, err := mcp.LoadConfig(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	if !cfg.RemoveServerConfig(name) {
		fmt.Fprintf(os.Stderr, "Server %s not found\n", name)
		os.Exit(1)
	}

	if err := mcp.SaveConfig(cfgPath, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Removed MCP server: %s\n", name)
}

func cmdMCPStatus() {
	cfgPath := mcp.DefaultConfigPath()
	cfg, err := mcp.LoadConfig(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	if len(cfg.Servers) == 0 {
		fmt.Println("No MCP servers registered.")
		return
	}

	client := mcp.NewClientFromConfig(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client.ConnectAll(ctx, 0)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tURL\tSTATUS\tTOOLS")
	for name, status := range client.GetServerStatus() {
		state := "disconnected"
		if status.Connected {
			state = "connected"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\n", name, status.URL, state, len(status.Tools))
	}
	w.Flush()
}
