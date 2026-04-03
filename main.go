package main

import (
	"bytes"
	"context"
	"crypto/subtle"
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/gr3enarr0w/synapserouter/internal/brand"
	_ "github.com/mattn/go-sqlite3"

	"github.com/gr3enarr0w/synapserouter/internal/app"
	"github.com/gr3enarr0w/synapserouter/internal/compat"
	"github.com/gr3enarr0w/synapserouter/internal/mcpserver"
	"github.com/gr3enarr0w/synapserouter/internal/memory"
	"github.com/gr3enarr0w/synapserouter/internal/orchestration"
	"github.com/gr3enarr0w/synapserouter/internal/providers"
	"github.com/gr3enarr0w/synapserouter/internal/router"
	"github.com/gr3enarr0w/synapserouter/internal/subscriptions"
	"github.com/gr3enarr0w/synapserouter/internal/tools"
	"github.com/gr3enarr0w/synapserouter/internal/usage"
)

var (
	proxyRouter    *router.Router
	orchestrator   *orchestration.Manager
	usageTracker   *usage.Tracker
	vectorMemory   *memory.VectorMemory
	sessionTracker *memory.SessionTracker
	db             *sql.DB
	providerList   []providers.Provider
	startupCheck   map[string]interface{}
	ampConfig      compat.AmpCodeConfig
)

//go:embed migrations/*.sql
var embeddedMigrations embed.FS


func makeHTTPRequestWithTimeout(urlStr string, timeout time.Duration) (*http.Response, error) {
	url, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{
		Timeout: timeout,
	}
	return client.Do(url)
}

func init() {
	// Register embedded migrations so InitLight can apply them for all modes (not just serve)
	app.RegisterMigrations(embeddedMigrations)
}

func printLogo() {
	if os.Getenv("NO_COLOR") != "" {
		fmt.Println("\n  SynRoute - LLM Router & Code Agent")
		fmt.Println()
		return
	}
	brand.PrintLogo()
}

func main() {
	if len(os.Args) >= 2 {
		switch os.Args[1] {
		case "serve":
			startServer()
		case "test":
			cmdTest(os.Args[2:])
		case "profile":
			cmdProfile(os.Args[2:])
		case "doctor":
			cmdDoctor(os.Args[2:])
		case "models":
			cmdModels(os.Args[2:])
		case "recommend":
			cmdRecommend(os.Args[2:])
		case "config":
			cmdConfig(os.Args[2:])
		case "eval":
			cmdEval(os.Args[2:])
		case "chat":
			cmdChat(os.Args[2:])
		case "code":
			cmdCode(os.Args[2:])
			return
		case "mcp":
			cmdMCP(os.Args[2:])
		case "mcp-serve":
			cmdMCPServe(os.Args[2:])
		case "worktree":
			cmdWorktree(os.Args[2:])
		case "auth":
			cmdAuth(os.Args[2:])
		case "version":
			cmdVersion()
		case "help", "--help", "-h":
			printUsage()
		default:
			fmt.Fprintf(os.Stderr, "Unknown command: %s\nRun 'synroute help' for usage.\n", os.Args[1])
			os.Exit(1)
		}
		return
	}

	// No command given — default to code mode (interactive TUI)
	cmdCode(nil)
}

func startServer() {
	// Load .env file
	if err := loadDotEnv(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	// Initialize database
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		home, _ := os.UserHomeDir()
		dbPath = filepath.Join(home, ".mcp", "proxy", "usage.db")
	}

	// Expand ~ in path
	if strings.HasPrefix(dbPath, "~/") {
		home, _ := os.UserHomeDir()
		dbPath = filepath.Join(home, dbPath[2:])
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		log.Fatal("Failed to create database directory:", err)
	}

	// Open database
	var err error
	db, err = sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Fatal("Failed to open database:", err)
	}

	// Run migrations
	if err := runMigrations(db); err != nil {
		log.Fatal("Failed to run migrations:", err)
	}
	if err := ensureRuntimeSchema(db); err != nil {
		log.Fatal("Failed to finalize runtime schema:", err)
	}
	if err := applyQuotaOverrides(db); err != nil {
		log.Fatal("Failed to apply quota overrides:", err)
	}

	// Initialize usage tracker
	usageTracker, err = usage.NewTracker(dbPath)
	if err != nil {
		log.Fatal("Failed to initialize usage tracker:", err)
	}
	defer usageTracker.Close()

	// Initialize vector memory
	vectorMemory = memory.NewVectorMemory(db)

	// Initialize session tracker for cross-session memory continuity
	sessionTracker = memory.NewSessionTracker(db)
	go func() {
		for range time.Tick(5 * time.Minute) {
			n, _ := sessionTracker.EndInactiveSessions(30 * time.Minute)
			if n > 0 {
				log.Printf("[Sessions] Auto-ended %d inactive sessions", n)
			}
		}
	}()

	// Initialize providers
	providerList = initializeProviders()
	startupCheck = runStartupCheck(providerList)

	// Initialize router
	proxyRouter = router.NewRouter(providerList, usageTracker, vectorMemory, db)
	proxyRouter.SetSessionTracker(sessionTracker)
	orchestrator = orchestration.NewManagerWithStore(proxyRouter, vectorMemory, db)
	toolRegistry := tools.DefaultRegistry()
	orchestrator.SetToolRegistry(toolRegistry)
	cwd, _ := os.Getwd()
	orchestrator.SetWorkDir(cwd)
	ampConfig, _ = compat.LoadAmpCodeConfig(db)

	// Create HTTP router (Go 1.22+ stdlib routing)
	r := http.NewServeMux()

	// Health check
	r.HandleFunc("GET /health", healthHandler)
	r.HandleFunc("GET /v1/startup-check", startupCheckHandler)
	r.HandleFunc("GET /anthropic/callback", subscriptionProviderCallbackHandler("anthropic"))
	r.HandleFunc("GET /codex/callback", subscriptionProviderCallbackHandler("openai"))
	r.HandleFunc("GET /google/callback", subscriptionProviderCallbackHandler("gemini"))

	// OpenAI-compatible endpoints
	r.HandleFunc("GET /v1/models", modelsHandler)
	r.HandleFunc("POST /v1/chat/completions", chatHandler)
	r.HandleFunc("POST /v1/responses", responsesHandler)
	r.HandleFunc("POST /v1/responses/compact", responsesCompactHandler)
	r.HandleFunc("GET /v1/responses/{response_id}", responseGetHandler)
	r.HandleFunc("DELETE /v1/responses/{response_id}", responseDeleteHandler)
	r.HandleFunc("GET /api/provider/{provider}/v1/models", providerModelsHandler)
	r.HandleFunc("POST /api/provider/{provider}/v1/chat/completions", providerChatHandler)
	r.HandleFunc("POST /api/provider/{provider}/v1/responses", providerResponsesHandler)

	// Anthropic-compatible endpoints
	r.HandleFunc("POST /v1/messages", messagesHandler)
	r.HandleFunc("POST /api/provider/{provider}/v1/messages", providerMessagesHandler)

	// Usage stats endpoint
	r.HandleFunc("GET /v1/providers", providersHandler)
	r.Handle("GET /v1/usage", withAdminAuth(http.HandlerFunc(usageHandler)))
	r.Handle("GET /v1/memory/search", withAdminAuth(http.HandlerFunc(memorySearchHandler)))
	r.Handle("GET /v1/memory/session/{session_id}", withAdminAuth(http.HandlerFunc(memorySessionHandler)))
	r.Handle("GET /v1/audit/session/{session_id}", withAdminAuth(http.HandlerFunc(auditSessionHandler)))
	r.Handle("GET /v1/audit/request/{request_id}", withAdminAuth(http.HandlerFunc(auditRequestHandler)))
	r.Handle("POST /v1/debug/trace", withAdminAuth(http.HandlerFunc(traceHandler)))
	// Skill dispatch endpoints
	r.Handle("GET /v1/skills", withAdminAuth(http.HandlerFunc(skillsListHandler)))
	r.Handle("GET /v1/skills/match", withAdminAuth(http.HandlerFunc(skillsMatchHandler)))
	r.Handle("GET /v1/tools", withAdminAuth(http.HandlerFunc(toolsListHandler(toolRegistry))))
	r.Handle("POST /v1/agent/chat", withAdminAuth(http.HandlerFunc(agentChatHandler(toolRegistry))))
	r.Handle("GET /v1/agent/pool", withAdminAuth(http.HandlerFunc(agentPoolHandler)))

	// MCP server mode (expose tools to other agents)
	if strings.EqualFold(os.Getenv("SYNROUTE_MCP_SERVER"), "true") {
		mcpSrv := mcpserver.NewServer(toolRegistry, cwd)
		mcpHandler := mcpSrv.Handler()
		r.HandleFunc("POST /mcp/initialize", mcpHandler.HandleInitialize)
		r.HandleFunc("GET /mcp/tools/list", mcpHandler.HandleToolsList)
		r.HandleFunc("POST /mcp/tools/list", mcpHandler.HandleToolsList)
		r.HandleFunc("POST /mcp/tools/call", mcpHandler.HandleToolsCall)
		log.Println("MCP server enabled on /mcp/* routes")
	}

	r.Handle("GET /v1/orchestration/roles", withAdminAuth(http.HandlerFunc(orchestrationRolesHandler)))
	r.Handle("GET /v1/orchestration/workflows", withAdminAuth(http.HandlerFunc(orchestrationWorkflowsHandler)))
	r.Handle("POST /v1/orchestration/workflows", withAdminAuth(http.HandlerFunc(orchestrationWorkflowsHandler)))
	r.Handle("GET /v1/orchestration/workflows/{template_id}", withAdminAuth(http.HandlerFunc(orchestrationWorkflowHandler)))
	r.Handle("PUT /v1/orchestration/workflows/{template_id}", withAdminAuth(http.HandlerFunc(orchestrationWorkflowHandler)))
	r.Handle("DELETE /v1/orchestration/workflows/{template_id}", withAdminAuth(http.HandlerFunc(orchestrationWorkflowHandler)))
	r.Handle("POST /v1/orchestration/workflows/{template_id}/run", withAdminAuth(http.HandlerFunc(orchestrationWorkflowRunHandler)))
	r.Handle("GET /v1/orchestration/executions/{workflow_id}/state", withAdminAuth(http.HandlerFunc(orchestrationExecutionStateHandler)))
	r.Handle("GET /v1/orchestration/executions/{workflow_id}/metrics", withAdminAuth(http.HandlerFunc(orchestrationExecutionMetricsHandler)))
	r.Handle("GET /v1/orchestration/executions/{workflow_id}/debug", withAdminAuth(http.HandlerFunc(orchestrationExecutionDebugHandler)))
	r.Handle("GET /v1/orchestration/sessions/{session_id}/tasks", withAdminAuth(http.HandlerFunc(orchestrationSessionTasksHandler)))
	r.Handle("POST /v1/orchestration/sessions/{session_id}/resume", withAdminAuth(http.HandlerFunc(orchestrationSessionResumeHandler)))
	r.Handle("POST /v1/orchestration/sessions/{session_id}/fork", withAdminAuth(http.HandlerFunc(orchestrationSessionForkHandler)))
	r.Handle("GET /v1/orchestration/tasks", withAdminAuth(http.HandlerFunc(orchestrationTasksHandler)))
	r.Handle("POST /v1/orchestration/tasks", withAdminAuth(http.HandlerFunc(orchestrationTasksHandler)))
	r.Handle("GET /v1/orchestration/tasks/{task_id}", withAdminAuth(http.HandlerFunc(orchestrationTaskHandler)))
	r.Handle("POST /v1/orchestration/tasks/{task_id}/run", withAdminAuth(http.HandlerFunc(orchestrationTaskRunHandler)))
	r.Handle("POST /v1/orchestration/tasks/{task_id}/pause", withAdminAuth(http.HandlerFunc(orchestrationTaskPauseHandler)))
	r.Handle("POST /v1/orchestration/tasks/{task_id}/resume", withAdminAuth(http.HandlerFunc(orchestrationTaskResumeHandler)))
	r.Handle("POST /v1/orchestration/tasks/{task_id}/cancel", withAdminAuth(http.HandlerFunc(orchestrationTaskCancelHandler)))
	r.Handle("POST /v1/orchestration/tasks/{task_id}/assign", withAdminAuth(http.HandlerFunc(orchestrationTaskAssignHandler)))
	r.Handle("POST /v1/orchestration/tasks/{task_id}/steal", withAdminAuth(http.HandlerFunc(orchestrationTaskStealHandler)))
	r.Handle("POST /v1/orchestration/tasks/{task_id}/contest", withAdminAuth(http.HandlerFunc(orchestrationTaskContestHandler)))
	r.Handle("POST /v1/orchestration/tasks/{task_id}/contest/resolve", withAdminAuth(http.HandlerFunc(orchestrationTaskContestResolveHandler)))
	r.Handle("POST /v1/orchestration/tasks/{task_id}/refine", withAdminAuth(http.HandlerFunc(orchestrationTaskRefineHandler)))
	r.Handle("GET /v1/orchestration/tasks/{task_id}/events", withAdminAuth(http.HandlerFunc(orchestrationTaskEventsHandler)))
	r.Handle("GET /v1/orchestration/agents", withAdminAuth(http.HandlerFunc(orchestrationAgentsHandler)))
	r.Handle("POST /v1/orchestration/agents", withAdminAuth(http.HandlerFunc(orchestrationAgentsHandler)))
	r.Handle("GET /v1/orchestration/agents/health", withAdminAuth(http.HandlerFunc(orchestrationAgentHealthHandler)))
	r.Handle("GET /v1/orchestration/agents/{agent_id}", withAdminAuth(http.HandlerFunc(orchestrationAgentHandler)))
	r.Handle("GET /v1/orchestration/agents/{agent_id}/status", withAdminAuth(http.HandlerFunc(orchestrationAgentStatusHandler)))
	r.Handle("POST /v1/orchestration/agents/{agent_id}/stop", withAdminAuth(http.HandlerFunc(orchestrationAgentStopHandler)))
	r.Handle("GET /v1/orchestration/agents/{agent_id}/metrics", withAdminAuth(http.HandlerFunc(orchestrationAgentMetricsHandler)))
	r.Handle("GET /v1/orchestration/agents/{agent_id}/logs", withAdminAuth(http.HandlerFunc(orchestrationAgentLogsHandler)))
	r.Handle("GET /v1/orchestration/swarms", withAdminAuth(http.HandlerFunc(orchestrationSwarmsHandler)))
	r.Handle("POST /v1/orchestration/swarms", withAdminAuth(http.HandlerFunc(orchestrationSwarmsHandler)))
	r.Handle("GET /v1/orchestration/swarms/{swarm_id}", withAdminAuth(http.HandlerFunc(orchestrationSwarmHandler)))
	r.Handle("GET /v1/orchestration/swarms/{swarm_id}/status", withAdminAuth(http.HandlerFunc(orchestrationSwarmStatusHandler)))
	r.Handle("POST /v1/orchestration/swarms/{swarm_id}/start", withAdminAuth(http.HandlerFunc(orchestrationSwarmStartHandler)))
	r.Handle("POST /v1/orchestration/swarms/{swarm_id}/stop", withAdminAuth(http.HandlerFunc(orchestrationSwarmStopHandler)))
	r.Handle("POST /v1/orchestration/swarms/{swarm_id}/pause", withAdminAuth(http.HandlerFunc(orchestrationSwarmPauseHandler)))
	r.Handle("POST /v1/orchestration/swarms/{swarm_id}/resume", withAdminAuth(http.HandlerFunc(orchestrationSwarmResumeHandler)))
	r.Handle("POST /v1/orchestration/swarms/{swarm_id}/scale", withAdminAuth(http.HandlerFunc(orchestrationSwarmScaleHandler)))
	r.Handle("POST /v1/orchestration/swarms/{swarm_id}/coordinate", withAdminAuth(http.HandlerFunc(orchestrationSwarmCoordinateHandler)))
	r.Handle("GET /v1/orchestration/swarms/{swarm_id}/load", withAdminAuth(http.HandlerFunc(orchestrationSwarmLoadHandler)))
	r.Handle("GET /v1/orchestration/swarms/{swarm_id}/imbalance", withAdminAuth(http.HandlerFunc(orchestrationSwarmImbalanceHandler)))
	r.Handle("POST /v1/orchestration/swarms/{swarm_id}/rebalance/preview", withAdminAuth(http.HandlerFunc(orchestrationSwarmRebalancePreviewHandler)))
	r.Handle("GET /v1/orchestration/swarms/{swarm_id}/stealable", withAdminAuth(http.HandlerFunc(orchestrationSwarmStealableTasksHandler)))

	// Session management
	r.HandleFunc("POST /v1/session/end", sessionEndHandler)

	// Diagnostic and management endpoints
	r.Handle("POST /v1/test/providers", withAdminAuth(http.HandlerFunc(smokeTestHandler)))
	r.Handle("POST /v1/circuit-breakers/reset", withAdminAuth(http.HandlerFunc(circuitBreakerResetHandler)))
	r.Handle("GET /v1/profile", withAdminAuth(http.HandlerFunc(profileHandler)))
	r.Handle("POST /v1/profile/switch", withAdminAuth(http.HandlerFunc(profileSwitchHandler)))
	r.Handle("GET /v1/doctor", withAdminAuth(http.HandlerFunc(doctorHandler)))

	// Eval framework endpoints
	r.Handle("GET /v1/eval/exercises", withAdminAuth(http.HandlerFunc(evalExercisesHandler)))
	r.Handle("GET /v1/eval/runs", withAdminAuth(http.HandlerFunc(evalRunsListHandler)))
	r.Handle("POST /v1/eval/runs", withAdminAuth(http.HandlerFunc(evalRunStartHandler)))
	r.Handle("GET /v1/eval/runs/{run_id}", withAdminAuth(http.HandlerFunc(evalRunGetHandler)))
	r.Handle("GET /v1/eval/runs/{run_id}/results", withAdminAuth(http.HandlerFunc(evalRunResultsHandler)))
	r.Handle("POST /v1/eval/compare", withAdminAuth(http.HandlerFunc(evalCompareHandler)))
	r.Handle("POST /v1/eval/import", withAdminAuth(http.HandlerFunc(evalImportHandler)))

	// Amp compatibility management endpoints
	r.Handle("GET /v0/management/anthropic-auth-url", withAdminAuth(http.HandlerFunc(subscriptionAnthropicAuthURLHandler)))
	r.Handle("GET /v0/management/codex-auth-url", withAdminAuth(http.HandlerFunc(subscriptionCodexAuthURLHandler)))
	r.Handle("GET /v0/management/gemini-cli-auth-url", withAdminAuth(http.HandlerFunc(subscriptionGeminiAuthURLHandler)))
	r.Handle("POST /v0/management/oauth-callback", withAdminAuth(http.HandlerFunc(subscriptionOAuthCallbackHandler)))
	r.Handle("GET /v0/management/get-auth-status", withAdminAuth(http.HandlerFunc(subscriptionAuthStatusHandler)))
	r.Handle("GET /v0/management/ampcode", withAdminAuth(http.HandlerFunc(ampConfigHandler)))
	r.Handle("GET /v0/management/ampcode/upstream-url", withAdminAuth(http.HandlerFunc(ampUpstreamURLHandler)))
	r.Handle("PUT /v0/management/ampcode/upstream-url", withAdminAuth(http.HandlerFunc(ampUpstreamURLHandler)))
	r.Handle("DELETE /v0/management/ampcode/upstream-url", withAdminAuth(http.HandlerFunc(ampUpstreamURLHandler)))
	r.Handle("GET /v0/management/ampcode/upstream-api-key", withAdminAuth(http.HandlerFunc(ampUpstreamAPIKeyHandler)))
	r.Handle("PUT /v0/management/ampcode/upstream-api-key", withAdminAuth(http.HandlerFunc(ampUpstreamAPIKeyHandler)))
	r.Handle("DELETE /v0/management/ampcode/upstream-api-key", withAdminAuth(http.HandlerFunc(ampUpstreamAPIKeyHandler)))
	r.Handle("GET /v0/management/ampcode/upstream-api-keys", withAdminAuth(http.HandlerFunc(ampUpstreamAPIKeysHandler)))
	r.Handle("PUT /v0/management/ampcode/upstream-api-keys", withAdminAuth(http.HandlerFunc(ampUpstreamAPIKeysHandler)))
	r.Handle("PATCH /v0/management/ampcode/upstream-api-keys", withAdminAuth(http.HandlerFunc(ampUpstreamAPIKeysHandler)))
	r.Handle("DELETE /v0/management/ampcode/upstream-api-keys", withAdminAuth(http.HandlerFunc(ampUpstreamAPIKeysHandler)))
	r.Handle("GET /v0/management/ampcode/model-mappings", withAdminAuth(http.HandlerFunc(ampModelMappingsHandler)))
	r.Handle("PUT /v0/management/ampcode/model-mappings", withAdminAuth(http.HandlerFunc(ampModelMappingsHandler)))
	r.Handle("PATCH /v0/management/ampcode/model-mappings", withAdminAuth(http.HandlerFunc(ampModelMappingsHandler)))
	r.Handle("DELETE /v0/management/ampcode/model-mappings", withAdminAuth(http.HandlerFunc(ampModelMappingsHandler)))
	r.Handle("GET /v0/management/ampcode/force-model-mappings", withAdminAuth(http.HandlerFunc(ampForceModelMappingsHandler)))
	r.Handle("PUT /v0/management/ampcode/force-model-mappings", withAdminAuth(http.HandlerFunc(ampForceModelMappingsHandler)))
	r.Handle("GET /v0/management/ampcode/restrict-management-to-localhost", withAdminAuth(http.HandlerFunc(ampRestrictManagementToLocalhostHandler)))
	r.Handle("PUT /v0/management/ampcode/restrict-management-to-localhost", withAdminAuth(http.HandlerFunc(ampRestrictManagementToLocalhostHandler)))

	// Apply middleware (replaces gorilla/mux r.Use())
	handler := applyMiddleware(r, maxBodySizeMiddleware(10<<20), securityHeadersMiddleware)

	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8090"
	}

	addr := fmt.Sprintf(":%s", port)
	log.Printf("🚀 Synapse Router starting on %s", addr)
	log.Printf("📊 Database: %s", dbPath)
	chainNames := make([]string, len(providerList))
	for i, p := range providerList {
		chainNames[i] = p.Name()
	}
	log.Printf("🔄 Provider chain: %s", strings.Join(chainNames, " → "))
	log.Printf("💾 Unified context across all providers via vector memory")
	log.Printf("⚡ Usage tracking enabled (80%% auto-switch threshold)")
	log.Printf("🔗 Cross-session memory continuity enabled (30min window)")
	log.Printf("🧠 Orchestration roles loaded: %d", len(orchestration.DefaultRoles()))
	log.Printf("🎯 Skill dispatch registry: %d skills", len(orchestration.DefaultSkills()))
	if strings.TrimSpace(os.Getenv("SYNROUTE_ADMIN_TOKEN")) == "" && strings.TrimSpace(os.Getenv("ADMIN_API_KEY")) == "" {
		log.Println("🔓 No SYNROUTE_ADMIN_TOKEN set — admin endpoints restricted to localhost")
	}
	logStartupCheck(startupCheck)
	printLogo()

	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatal(err)
	}
	db.Close()
}

func initializeProviders() []providers.Provider {
	profile := strings.ToLower(strings.TrimSpace(os.Getenv("ACTIVE_PROFILE")))
	log.Printf("🔧 Active profile: %s", map[bool]string{true: profile, false: "personal (default)"}[profile != ""])

	var providerList []providers.Provider

	switch profile {
	case "work":
		providerList = append(providerList, initializeWorkProviders()...)
	default:
		providerList = append(providerList, initializePersonalProviders()...)
	}

	// Ollama Cloud — supports multiple API keys for concurrent subscriptions
	apiKeys := app.ParseOllamaAPIKeys()
	ollamaBaseURL := os.Getenv("OLLAMA_BASE_URL")
	if len(apiKeys) == 0 && ollamaBaseURL != "" {
		apiKeys = []string{""} // local Ollama, no key needed
	}
	if len(apiKeys) > 0 {
		registered := 0
		keyIdx := 0

		// Planners (unique — planning is a separate phase)
		for _, m := range []struct{ envVar, name string }{
			{"OLLAMA_PLANNER_1", "ollama-planner-1"},
			{"OLLAMA_PLANNER_2", "ollama-planner-2"},
		} {
			model := os.Getenv(m.envVar)
			if model == "" {
				continue
			}
			apiKey := apiKeys[keyIdx%len(apiKeys)]
			keyIdx++
			providerList = append(providerList,
				providers.NewOllamaCloudProvider(ollamaBaseURL, apiKey, model, m.name))
			log.Printf("✓ %s provider initialized (model=%s, key=%d/%d)", m.name, model, (keyIdx-1)%len(apiKeys)+1, len(apiKeys))
			registered++
		}

		// Chain models — grouped by level with | separator
		chainLevels := app.ParseOllamaChain(os.Getenv("OLLAMA_CHAIN"))
		modelIdx := 0
		for _, models := range chainLevels {
			for _, model := range models {
				modelIdx++
				apiKey := apiKeys[keyIdx%len(apiKeys)]
				keyIdx++
				name := fmt.Sprintf("ollama-chain-%d", modelIdx)
				providerList = append(providerList,
					providers.NewOllamaCloudProvider(ollamaBaseURL, apiKey, model, name))
				log.Printf("✓ %s provider initialized (model=%s, key=%d/%d)", name, model, (keyIdx-1)%len(apiKeys)+1, len(apiKeys))
				registered++
			}
		}

		// Fallback: single OLLAMA_MODEL if no chain configured
		if registered == 0 {
			ollamaModel := os.Getenv("OLLAMA_MODEL")
			if ollamaModel != "" {
				providerList = append(providerList,
					providers.NewOllamaCloudProvider(ollamaBaseURL, apiKeys[0], ollamaModel, "ollama-cloud"))
				log.Printf("✓ ollama-cloud provider initialized (model=%s)", ollamaModel)
			}
		}

		if registered > 0 {
			log.Printf("✓ Ollama Cloud: %d API keys, %d models", len(apiKeys), registered)
		}
	}

	log.Printf("Initialized %d providers", len(providerList))
	if len(providerList) == 0 {
		log.Printf("No providers configured yet; start the built-in login flow with synroute-cli login <provider>")
	}
	return providerList
}

func initializePersonalProviders() []providers.Provider {
	var providerList []providers.Provider
	subscriptionProviders, err := subscriptions.LoadRuntimeProviders(context.Background())
	if err != nil {
		log.Fatalf("Failed to initialize subscription providers: %v", err)
	}
	for _, provider := range subscriptionProviders {
		providerList = append(providerList, provider)
		log.Printf("✓ %s subscription provider initialized", provider.Name())
	}
	return providerList
}

func initializeWorkProviders() []providers.Provider {
	var providerList []providers.Provider

	// Vertex Claude — uses native GCP auth, project from env
	claudeProject := envFirst("VERTEX_CLAUDE_PROJECT", "ANTHROPIC_VERTEX_PROJECT_ID", "VERTEX_PROJECT_ID")
	claudeRegion := envFirst("VERTEX_CLAUDE_REGION", "VERTEX_REGION")
	if claudeRegion == "" {
		claudeRegion = "us-east5"
	}
	if claudeProject != "" {
		vClaude := providers.NewVertexProvider(providers.VertexConfig{
			Name:      "vertex-claude",
			Project:   claudeProject,
			Location:  claudeRegion,
			Publisher: "anthropic",
			Prefix:    "claude",
		})
		providerList = append(providerList, vClaude)
		log.Printf("✓ vertex-claude initialized (project=%s, region=%s, auth=adc)", claudeProject, claudeRegion)
	}

	// Vertex Gemini — uses service account, project from env
	geminiProject := envFirst("VERTEX_GEMINI_PROJECT", "GEMINI_PROJECT")
	geminiLocation := envFirst("VERTEX_GEMINI_LOCATION", "GEMINI_LOCATION")
	if geminiLocation == "" {
		geminiLocation = "global"
	}
	geminiSAKey := envFirst("VERTEX_GEMINI_SA_KEY", "GOOGLE_SERVICE_ACCOUNT_JSON")
	if geminiProject != "" {
		vGemini := providers.NewVertexProvider(providers.VertexConfig{
			Name:      "vertex-gemini",
			Project:   geminiProject,
			Location:  geminiLocation,
			Publisher: "google",
			SAKeyFile: geminiSAKey,
			Prefix:    "gemini",
		})
		providerList = append(providerList, vGemini)
		authMethod := "adc"
		if geminiSAKey != "" {
			authMethod = "service-account"
		}
		log.Printf("✓ vertex-gemini initialized (project=%s, location=%s, auth=%s)", geminiProject, geminiLocation, authMethod)
	}

	return providerList
}

func runMigrations(db *sql.DB) error {
	// Discover all migration files from embedded FS
	files, err := embeddedMigrations.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("failed to read embedded migrations: %w", err)
	}

	for _, file := range files {
		if !strings.HasSuffix(file.Name(), ".sql") {
			continue
		}

		migrationPath := "migrations/" + file.Name()
		migrationSQL, err := embeddedMigrations.ReadFile(migrationPath)
		if err != nil {
			return fmt.Errorf("failed to read embedded migration file %s: %w", file.Name(), err)
		}

		// Execute migration — ignore "duplicate column" errors from re-running ALTER TABLE
		if _, err := db.Exec(string(migrationSQL)); err != nil {
			if strings.Contains(err.Error(), "duplicate column") {
				log.Printf("✓ Database migration skipped (already applied): %s", file.Name())
				continue
			}
			return fmt.Errorf("failed to execute migration %s: %w", file.Name(), err)
		}
		log.Printf("✓ Database migration applied (embedded): %s", file.Name())
	}

	return nil
}

func loadDotEnv() error {
	home, _ := os.UserHomeDir()
	candidates := []string{
		".env",
		filepath.Join(home, ".mcp", "synapse", ".env"),
	}

	if executablePath, err := resolvedExecutablePath(); err == nil {
		executableEnv := filepath.Join(filepath.Dir(executablePath), ".env")
		if executableEnv != ".env" {
			candidates = append(candidates, executableEnv)
		}
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return godotenv.Load(candidate)
		}
	}

	return os.ErrNotExist
}

func resolveRuntimeFilePath(relativePath string) string {
	if _, err := os.Stat(relativePath); err == nil {
		return relativePath
	}

	executablePath, err := resolvedExecutablePath()
	if err != nil {
		return relativePath
	}

	candidate := filepath.Join(filepath.Dir(executablePath), relativePath)
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}

	return relativePath
}

func resolvedExecutablePath() (string, error) {
	executablePath, err := os.Executable()
	if err != nil {
		return "", err
	}
	resolvedPath, err := filepath.EvalSymlinks(executablePath)
	if err != nil {
		return executablePath, nil
	}
	return resolvedPath, nil
}

func ensureRuntimeSchema(db *sql.DB) error {
	statements := []string{
		`ALTER TABLE orchestration_tasks ADD COLUMN assigned_to TEXT`,
		// Eval metric columns (moved from 005_eval_metrics.sql for idempotency)
		`ALTER TABLE eval_exercises ADD COLUMN eval_mode TEXT DEFAULT 'docker-test'`,
		`ALTER TABLE eval_exercises ADD COLUMN reference_code TEXT`,
		`ALTER TABLE eval_exercises ADD COLUMN criteria TEXT`,
		`ALTER TABLE eval_results ADD COLUMN metric_score REAL`,
		`ALTER TABLE eval_results ADD COLUMN metric_name TEXT`,
		`ALTER TABLE eval_results ADD COLUMN judge_provider TEXT`,
	}

	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			errText := strings.ToLower(err.Error())
			if strings.Contains(errText, "duplicate column name") {
				continue
			}
			if strings.Contains(errText, "no such table") {
				continue
			}
			return err
		}
	}
	return nil
}

func applyQuotaOverrides(db *sql.DB) error {
	overrides := []struct {
		provider       string
		dailyEnv       string
		monthlyEnv     string
		defaultDaily   int64
		defaultMonthly int64
		tier           string
	}{
		{provider: "claude-code", dailyEnv: "CLAUDE_CODE_DAILY_LIMIT", defaultDaily: 500000, defaultMonthly: 15000000, tier: "pro"},
		{provider: "codex", dailyEnv: "CODEX_DAILY_LIMIT", defaultDaily: 300000, defaultMonthly: 9000000, tier: "pro"},
		{provider: "gemini", dailyEnv: "GEMINI_DAILY_LIMIT", defaultDaily: 500000, defaultMonthly: 15000000, tier: "pro"},
		{provider: "qwen", dailyEnv: "QWEN_DAILY_LIMIT", defaultDaily: 500000, defaultMonthly: 15000000, tier: "pro"},
		{provider: "ollama-cloud", dailyEnv: "OLLAMA_CLOUD_DAILY_LIMIT", defaultDaily: 1000000, defaultMonthly: 30000000, tier: "pro"},
	}

	for _, override := range overrides {
		dailyLimit := override.defaultDaily
		monthlyLimit := override.defaultMonthly

		if override.dailyEnv != "" {
			if parsed, ok := lookupInt64Env(override.dailyEnv); ok {
				dailyLimit = parsed
			}
		}
		if override.monthlyEnv != "" {
			if parsed, ok := lookupInt64Env(override.monthlyEnv); ok {
				monthlyLimit = parsed
			}
		}

		if _, err := db.Exec(`
			INSERT INTO provider_quotas (provider, daily_limit, monthly_limit, reset_time, tier, enabled)
			VALUES (?, ?, ?, datetime('now', '+1 day'), ?, 1)
			ON CONFLICT(provider) DO UPDATE SET
				daily_limit = excluded.daily_limit,
				monthly_limit = excluded.monthly_limit,
				tier = excluded.tier
		`, override.provider, dailyLimit, monthlyLimit, override.tier); err != nil {
			return err
		}
	}

	return nil
}

func lookupInt64Env(key string) (int64, bool) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return 0, false
	}

	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		log.Printf("Ignoring invalid %s value %q", key, raw)
		return 0, false
	}

	return value, true
}

func envFirst(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

func orchestratorRoleCount() int {
	if orchestrator == nil {
		return 0
	}
	return len(orchestrator.Roles())
}

func sessionEndHandler(w http.ResponseWriter, r *http.Request) {
	sid := strings.TrimSpace(r.Header.Get("X-Session-ID"))
	if sid == "" {
		var body struct {
			SessionID string `json:"session_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			sid = strings.TrimSpace(body.SessionID)
		}
	}
	if sid == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "session_id required (header X-Session-ID or JSON body)"})
		return
	}
	if err := sessionTracker.End(sid); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	log.Printf("[Sessions] Session %s explicitly ended", sid)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ended", "session_id": sid})
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	// Check circuit breaker states
	states, _ := router.GetAllCircuitStates(db)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":           "ok",
		"timestamp":        time.Now().Unix(),
		"circuit_breakers": states,
	})
}

func modelsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"object": "list",
		"data":   availableModels(),
	})
}

func chatHandler(w http.ResponseWriter, r *http.Request) {
	var req providers.ChatRequest
	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "Invalid request body")
		return
	}
	if err := json.NewDecoder(bytes.NewReader(rawBody)).Decode(&req); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "Invalid request body")
		return
	}
	if resolved := compat.ResolveModel(ampConfig, req.Model, knownModelIDs()); resolved != "" {
		req.Model = resolved
	}
	if shouldUseAmpFallback(req.Model) {
		if forwardToAmpUpstream(w, r, rawBody) {
			return
		}
	}

	// Get complete response from provider — we convert to SSE ourselves if streaming
	wantStream := req.Stream
	req.Stream = false
	resp, err := routeChatRequest(r, req, requestSessionID(r), "")
	if err != nil {
		if strings.Contains(err.Error(), "invalid request:") || strings.Contains(err.Error(), "unknown model") || strings.Contains(err.Error(), "not compatible") {
			writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
			return
		}
		if forwardToAmpUpstream(w, r, rawBody) {
			return
		}
		log.Printf("All providers failed: %v", err)
		writeOpenAIError(w, http.StatusServiceUnavailable, "server_error", fmt.Sprintf("Service unavailable: %v", err))
		return
	}

	if wantStream {
		writeChatCompletionStream(w, resp)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func usageHandler(w http.ResponseWriter, r *http.Request) {
	quotas, err := usageTracker.GetAllQuotas()
	if err != nil {
		http.Error(w, "Failed to get usage stats", http.StatusInternalServerError)
		return
	}

	// Convert to JSON-friendly format
	result := make(map[string]interface{})
	for name, quota := range quotas {
		result[name] = map[string]interface{}{
			"current_usage": quota.CurrentUsage,
			"daily_limit":   quota.DailyLimit,
			"usage_percent": fmt.Sprintf("%.1f%%", quota.UsagePercent*100),
			"tier":          quota.Tier,
			"reset_time":    quota.ResetTime,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func orchestrationRolesHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"count": orchestratorRoleCount(),
		"data":  orchestrator.Roles(),
	})
}

func orchestrationTasksHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))
		status := strings.TrimSpace(r.URL.Query().Get("status"))
		tasks := orchestrator.ListTasks()
		if sessionID != "" || status != "" {
			filtered := make([]*orchestration.Task, 0, len(tasks))
			for _, task := range tasks {
				if sessionID != "" && task.SessionID != sessionID {
					continue
				}
				if status != "" && !strings.EqualFold(string(task.Status), status) {
					continue
				}
				filtered = append(filtered, task)
			}
			tasks = filtered
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"count": len(tasks),
			"data":  tasks,
		})
	case http.MethodPost:
		var req orchestration.TaskRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		task, err := orchestrator.CreateTask(r.Context(), req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(task)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func orchestrationSessionTasksHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSpace(r.PathValue("session_id"))
	tasks := orchestrator.ListTasksBySession(sessionID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"count":      len(tasks),
		"session_id": sessionID,
		"data":       tasks,
	})
}

func orchestrationSessionResumeHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSpace(r.PathValue("session_id"))
	var req struct {
		Goal    string `json:"goal,omitempty"`
		Execute bool   `json:"execute,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	task, err := orchestrator.ResumeSessionTask(r.Context(), sessionID, req.Goal, req.Execute)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(task)
}

func orchestrationSessionForkHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSpace(r.PathValue("session_id"))
	var req orchestration.SessionForkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	task, err := orchestrator.ForkSessionTask(r.Context(), sessionID, req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(task)
}

func orchestrationTaskHandler(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("task_id")
	task, err := orchestrator.GetTask(taskID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(task)
}

func orchestrationTaskRunHandler(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("task_id")
	if err := orchestrator.StartTask(taskID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func orchestrationTaskPauseHandler(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("task_id")
	task, err := orchestrator.PauseTask(taskID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(task)
}

func orchestrationTaskResumeHandler(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("task_id")
	task, err := orchestrator.ResumeTask(taskID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(task)
}

func orchestrationTaskCancelHandler(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("task_id")
	task, err := orchestrator.CancelTask(taskID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(task)
}

func orchestrationTaskRefineHandler(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("task_id")
	var req orchestration.RefineRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	task, err := orchestrator.RefineTask(r.Context(), taskID, req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(task)
}

func providersHandler(w http.ResponseWriter, r *http.Request) {
	quotas, _ := usageTracker.GetAllQuotas()
	states, _ := router.GetAllCircuitStates(db)

	result := make([]map[string]interface{}, 0, len(providerList))
	for _, provider := range providerList {
		entry := map[string]interface{}{
			"name":               provider.Name(),
			"healthy":            provider.IsHealthy(r.Context()),
			"max_context_tokens": provider.MaxContextTokens(),
			"circuit_state":      states[provider.Name()],
		}
		if quota, ok := quotas[provider.Name()]; ok {
			entry["current_usage"] = quota.CurrentUsage
			entry["daily_limit"] = quota.DailyLimit
			entry["usage_percent"] = fmt.Sprintf("%.1f%%", quota.UsagePercent*100)
			entry["tier"] = quota.Tier
		}
		result = append(result, entry)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"count":     len(result),
		"providers": result,
	})
}

func startupCheckHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(startupCheck)
}

func memorySearchHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))
	if sessionID == "" {
		http.Error(w, "session_id is required", http.StatusBadRequest)
		return
	}

	query := strings.TrimSpace(r.URL.Query().Get("q"))
	maxTokens, err := parsePositiveIntQuery(r, "max_tokens", 4000)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var messages []memory.Message
	if query == "" {
		messages, err = vectorMemory.RetrieveRecent(sessionID, 20, "")
	} else {
		messages, err = vectorMemory.RetrieveRelevant(query, sessionID, maxTokens)
	}
	if err != nil {
		http.Error(w, "Failed to search memory", http.StatusInternalServerError)
		return
	}

	totalTokens := memory.EstimateMessagesTokens(messages)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"session_id":   sessionID,
		"query":        query,
		"result_count": len(messages),
		"total_tokens": totalTokens,
		"max_tokens":   maxTokens,
		"messages":     messages,
	})
}

func memorySessionHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSpace(r.PathValue("session_id"))
	if sessionID == "" {
		http.Error(w, "session_id is required", http.StatusBadRequest)
		return
	}

	messages, err := vectorMemory.GetSessionHistory(sessionID)
	if err != nil {
		http.Error(w, "Failed to load session history", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"session_id":    sessionID,
		"message_count": len(messages),
		"total_tokens":  memory.EstimateMessagesTokens(messages),
		"messages":      messages,
	})
}

func parsePositiveIntQuery(r *http.Request, key string, fallback int) (int, error) {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return fallback, nil
	}

	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", key)
	}

	return value, nil
}

func auditSessionHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSpace(r.PathValue("session_id"))
	if sessionID == "" {
		http.Error(w, "session_id is required", http.StatusBadRequest)
		return
	}

	limit, err := parsePositiveIntQuery(r, "limit", 25)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	rows, err := db.Query(`
		SELECT request_id, selected_provider, final_provider, final_model,
		       memory_query, memory_candidate_count, success, error_message, created_at
		FROM request_audit
		WHERE session_id = ?
		ORDER BY created_at DESC
		LIMIT ?
	`, sessionID, limit)
	if err != nil {
		http.Error(w, "Failed to load audit session", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var entries []map[string]interface{}
	for rows.Next() {
		var requestID, selectedProvider, finalProvider, finalModel, memoryQuery string
		var memoryCandidateCount, success int
		var errorMessage string
		var createdAt time.Time

		if err := rows.Scan(&requestID, &selectedProvider, &finalProvider, &finalModel,
			&memoryQuery, &memoryCandidateCount, &success, &errorMessage, &createdAt); err != nil {
			continue
		}

		entries = append(entries, map[string]interface{}{
			"request_id":             requestID,
			"selected_provider":      selectedProvider,
			"final_provider":         finalProvider,
			"final_model":            finalModel,
			"memory_query":           memoryQuery,
			"memory_candidate_count": memoryCandidateCount,
			"success":                success == 1,
			"error_message":          errorMessage,
			"created_at":             createdAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"session_id": sessionID,
		"count":      len(entries),
		"entries":    entries,
	})
}

func auditRequestHandler(w http.ResponseWriter, r *http.Request) {
	requestID := strings.TrimSpace(r.PathValue("request_id"))
	if requestID == "" {
		http.Error(w, "request_id is required", http.StatusBadRequest)
		return
	}

	var sessionID, selectedProvider, finalProvider, finalModel, memoryQuery string
	var memoryCandidateCount, success int
	var errorMessage string
	var createdAt time.Time

	err := db.QueryRow(`
		SELECT session_id, selected_provider, final_provider, final_model,
		       memory_query, memory_candidate_count, success, error_message, created_at
		FROM request_audit
		WHERE request_id = ?
	`, requestID).Scan(&sessionID, &selectedProvider, &finalProvider, &finalModel,
		&memoryQuery, &memoryCandidateCount, &success, &errorMessage, &createdAt)
	if err == sql.ErrNoRows {
		http.Error(w, "request audit not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Failed to load request audit", http.StatusInternalServerError)
		return
	}

	rows, err := db.Query(`
		SELECT provider, attempt_index, success, error_message, created_at
		FROM provider_attempt_audit
		WHERE request_id = ?
		ORDER BY attempt_index ASC
	`, requestID)
	if err != nil {
		http.Error(w, "Failed to load provider attempts", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var attempts []map[string]interface{}
	for rows.Next() {
		var provider, attemptError string
		var attemptIndex, attemptSuccess int
		var attemptCreatedAt time.Time
		if err := rows.Scan(&provider, &attemptIndex, &attemptSuccess, &attemptError, &attemptCreatedAt); err != nil {
			continue
		}
		attempts = append(attempts, map[string]interface{}{
			"provider":      provider,
			"attempt_index": attemptIndex,
			"success":       attemptSuccess == 1,
			"error_message": attemptError,
			"created_at":    attemptCreatedAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"request_id":             requestID,
		"session_id":             sessionID,
		"selected_provider":      selectedProvider,
		"final_provider":         finalProvider,
		"final_model":            finalModel,
		"memory_query":           memoryQuery,
		"memory_candidate_count": memoryCandidateCount,
		"success":                success == 1,
		"error_message":          errorMessage,
		"created_at":             createdAt,
		"attempts":               attempts,
	})
}

func traceHandler(w http.ResponseWriter, r *http.Request) {
	var req providers.ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	sessionID := r.Header.Get("X-Session-ID")
	if sessionID == "" {
		sessionID = strings.TrimSpace(r.URL.Query().Get("session_id"))
	}
	if sessionID == "" {
		sessionID = fmt.Sprintf("trace-%d", time.Now().UnixNano())
	}

	trace, err := proxyRouter.TraceDecision(r.Context(), req, sessionID)
	if err != nil {
		http.Error(w, "Failed to build trace", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(trace)
}

// applyMiddleware wraps a handler with middleware in order (last = outermost).
func applyMiddleware(h http.Handler, middlewares ...func(http.Handler) http.Handler) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
}

func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

func maxBodySizeMiddleware(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil && (r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch) {
				r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			}
			next.ServeHTTP(w, r)
		})
	}
}

func withAdminAuth(next http.Handler) http.Handler {
	adminKey := strings.TrimSpace(os.Getenv("SYNROUTE_ADMIN_TOKEN"))
	if adminKey == "" {
		adminKey = strings.TrimSpace(os.Getenv("ADMIN_API_KEY"))
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// If no admin token configured, only allow localhost access
		if adminKey == "" {
			remoteAddr := r.RemoteAddr
			if !strings.HasPrefix(remoteAddr, "127.0.0.1:") && !strings.HasPrefix(remoteAddr, "[::1]:") && !strings.HasPrefix(remoteAddr, "localhost:") {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		token := extractBearerToken(r.Header.Get("Authorization"))
		if token == "" {
			token = strings.TrimSpace(r.Header.Get("X-Admin-API-Key"))
		}
		if subtle.ConstantTimeCompare([]byte(token), []byte(adminKey)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func extractBearerToken(header string) string {
	header = strings.TrimSpace(header)
	if header == "" {
		return ""
	}

	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}

	return strings.TrimSpace(parts[1])
}

func runStartupCheck(providerList []providers.Provider) map[string]interface{} {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	results := make([]map[string]interface{}, 0, len(providerList))
	readyCount := 0
	for _, provider := range providerList {
		healthy := provider.IsHealthy(ctx)
		if healthy {
			readyCount++
		}
		results = append(results, map[string]interface{}{
			"name":               provider.Name(),
			"healthy":            healthy,
			"max_context_tokens": provider.MaxContextTokens(),
			"configured":         providerConfigured(provider.Name()),
			"notes":              providerNotes(provider.Name(), healthy),
		})
	}

	return map[string]interface{}{
		"timestamp":      time.Now().Unix(),
		"provider_count": len(providerList),
		"healthy_count":  readyCount,
		"all_healthy":    readyCount == len(providerList) && len(providerList) > 0,
		"providers":      results,
	}
}

func logStartupCheck(report map[string]interface{}) {
	log.Printf("🧪 Startup check: %v healthy providers", report["healthy_count"])
	if providers, ok := report["providers"].([]map[string]interface{}); ok {
		for _, provider := range providers {
			log.Printf("🧪 Provider %s configured=%v healthy=%v notes=%s",
				provider["name"], provider["configured"], provider["healthy"], provider["notes"])
		}
	}
}

func providerConfigured(name string) bool {
	switch name {
	case "vertex-claude":
		return envFirst("VERTEX_CLAUDE_PROJECT", "ANTHROPIC_VERTEX_PROJECT_ID", "VERTEX_PROJECT_ID") != ""
	case "vertex-gemini":
		return envFirst("VERTEX_GEMINI_PROJECT", "GEMINI_PROJECT") != ""
	case "claude-code":
		if subscriptions.HasStoredCredentialsForProvider("anthropic") {
			return true
		}
		return envFirst("SYNROUTE_ANTHROPIC_API_KEY", "SYNROUTE_ANTHROPIC_API_KEYS", "SYNROUTE_ANTHROPIC_SESSION_TOKEN", "SYNROUTE_ANTHROPIC_SESSION_TOKENS", "SUBSCRIPTION_GATEWAY_ANTHROPIC_API_KEY", "SUBSCRIPTION_GATEWAY_ANTHROPIC_API_KEYS", "SUBSCRIPTION_GATEWAY_ANTHROPIC_SESSION_TOKEN", "SUBSCRIPTION_GATEWAY_ANTHROPIC_SESSION_TOKENS", "SYNAPSE_GATEWAY_ANTHROPIC_API_KEY", "SYNAPSE_GATEWAY_ANTHROPIC_API_KEYS", "SYNAPSE_GATEWAY_ANTHROPIC_SESSION_TOKEN", "SYNAPSE_GATEWAY_ANTHROPIC_SESSION_TOKENS", "CLIPROXY_ANTHROPIC_API_KEY", "CLIPROXY_ANTHROPIC_API_KEYS", "CLIPROXY_ANTHROPIC_SESSION_TOKEN", "CLIPROXY_ANTHROPIC_SESSION_TOKENS") != ""
	case "codex":
		if subscriptions.HasStoredCredentialsForProvider("openai") {
			return true
		}
		return envFirst("SYNROUTE_OPENAI_API_KEY", "SYNROUTE_OPENAI_API_KEYS", "SYNROUTE_OPENAI_SESSION_TOKEN", "SYNROUTE_OPENAI_SESSION_TOKENS", "SUBSCRIPTION_GATEWAY_OPENAI_API_KEY", "SUBSCRIPTION_GATEWAY_OPENAI_API_KEYS", "SUBSCRIPTION_GATEWAY_OPENAI_SESSION_TOKEN", "SUBSCRIPTION_GATEWAY_OPENAI_SESSION_TOKENS", "SYNAPSE_GATEWAY_OPENAI_API_KEY", "SYNAPSE_GATEWAY_OPENAI_API_KEYS", "SYNAPSE_GATEWAY_OPENAI_SESSION_TOKEN", "SYNAPSE_GATEWAY_OPENAI_SESSION_TOKENS", "CLIPROXY_OPENAI_API_KEY", "CLIPROXY_OPENAI_API_KEYS", "CLIPROXY_OPENAI_SESSION_TOKEN", "CLIPROXY_OPENAI_SESSION_TOKENS") != ""
	case "gemini":
		if subscriptions.HasStoredCredentialsForProvider("gemini") {
			return true
		}
		return envFirst("SYNROUTE_GEMINI_API_KEY", "SYNROUTE_GEMINI_API_KEYS", "SYNROUTE_GEMINI_SESSION_TOKEN", "SYNROUTE_GEMINI_SESSION_TOKENS", "SUBSCRIPTION_GATEWAY_GEMINI_API_KEY", "SUBSCRIPTION_GATEWAY_GEMINI_API_KEYS", "SUBSCRIPTION_GATEWAY_GEMINI_SESSION_TOKEN", "SUBSCRIPTION_GATEWAY_GEMINI_SESSION_TOKENS", "SYNAPSE_GATEWAY_GEMINI_API_KEY", "SYNAPSE_GATEWAY_GEMINI_API_KEYS", "SYNAPSE_GATEWAY_GEMINI_SESSION_TOKEN", "SYNAPSE_GATEWAY_GEMINI_SESSION_TOKENS", "CLIPROXY_GEMINI_API_KEY", "CLIPROXY_GEMINI_API_KEYS", "CLIPROXY_GEMINI_SESSION_TOKEN", "CLIPROXY_GEMINI_SESSION_TOKENS") != ""
	case "qwen":
		return envFirst("SYNROUTE_QWEN_API_KEY", "SYNROUTE_QWEN_API_KEYS", "SYNROUTE_QWEN_SESSION_TOKEN", "SYNROUTE_QWEN_SESSION_TOKENS") != ""
	case "ollama-cloud":
		return len(app.ParseOllamaAPIKeys()) > 0
	default:
		return true
	}
}

func providerNotes(name string, healthy bool) string {
	switch name {
	case "vertex-claude":
		if healthy {
			return "Vertex AI Claude (work profile)"
		}
		return "Check GCP credentials (ADC) and VERTEX_CLAUDE_PROJECT"
	case "vertex-gemini":
		if healthy {
			return "Vertex AI Gemini (work profile)"
		}
		return "Check service account and VERTEX_GEMINI_PROJECT"
	case "claude-code":
		if healthy {
			return "Anthropic-backed Claude Code subscription path available"
		}
		return "Check SYNROUTE_ANTHROPIC_* credentials or run synroute-cli login anthropic"
	case "codex":
		if healthy {
			return "OpenAI-backed Codex subscription path available"
		}
		return "Check SYNROUTE_OPENAI_* credentials or run synroute-cli login openai"
	case "gemini":
		if healthy {
			return "Gemini subscription path available"
		}
		return "Check SYNROUTE_GEMINI_* credentials or run synroute-cli login gemini"
	case "qwen":
		if healthy {
			return "Qwen OpenAI-compatible subscription path available"
		}
		return "Check SYNROUTE_QWEN_* credentials"
	case "ollama-cloud":
		if healthy {
			return "Ollama Cloud API reachable"
		}
		return "Check OLLAMA_API_KEY/OLLAMA_API_KEYS and OLLAMA_BASE_URL"
	default:
		if healthy {
			return name + " available"
		}
		return name + " not reachable"
	}
}
