package app

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
	_ "github.com/mattn/go-sqlite3"

	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/agent"
	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/memory"
	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/providers"
	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/router"
	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/subscriptions"
	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/usage"
)

// AppContext holds shared state for CLI commands and API handlers.
type AppContext struct {
	DB              *sql.DB
	UsageTracker    *usage.Tracker
	VectorMemory    *memory.VectorMemory
	Providers       []providers.Provider
	ProxyRouter     *router.Router
	EscalationChain []agent.EscalationLevel
	Profile         string
	Port            string
}

// InitLight loads .env, opens DB, runs migrations, and initializes providers.
// Suitable for CLI commands that don't need HTTP routing.
func InitLight(ctx context.Context) (*AppContext, error) {
	if err := LoadDotEnv(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	profile := strings.ToLower(strings.TrimSpace(os.Getenv("ACTIVE_PROFILE")))
	if profile == "" {
		profile = "personal"
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8090"
	}

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		home, _ := os.UserHomeDir()
		dbPath = filepath.Join(home, ".mcp", "proxy", "usage.db")
	}
	if strings.HasPrefix(dbPath, "~/") {
		home, _ := os.UserHomeDir()
		dbPath = filepath.Join(home, dbPath[2:])
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}

	tracker, err := usage.NewTracker(dbPath)
	if err != nil {
		db.Close()
		return nil, err
	}

	vm := memory.NewVectorMemory(db)
	providerList := initializeProviders(profile)
	escalationChain := buildEscalationChain(profile)

	return &AppContext{
		DB:              db,
		UsageTracker:    tracker,
		VectorMemory:    vm,
		Providers:       providerList,
		EscalationChain: escalationChain,
		Profile:         profile,
		Port:            port,
	}, nil
}

// InitFull extends InitLight by creating the Router. Suitable for server mode.
func (ac *AppContext) InitFull() {
	ac.ProxyRouter = router.NewRouter(ac.Providers, ac.UsageTracker, ac.VectorMemory, ac.DB)
}

// Close releases resources.
func (ac *AppContext) Close() {
	if ac.UsageTracker != nil {
		ac.UsageTracker.Close()
	}
	if ac.DB != nil {
		ac.DB.Close()
	}
}

func initializeProviders(profile string) []providers.Provider {
	var providerList []providers.Provider

	nanogptAPIKey := os.Getenv("NANOGPT_API_KEY")

	switch profile {
	case "work":
		providerList = append(providerList, initializeWorkProviders()...)
	default:
		// Ollama Cloud (3 concurrent models + sequential fallbacks)
		ollamaAPIKey := os.Getenv("OLLAMA_API_KEY")
		ollamaBaseURL := os.Getenv("OLLAMA_BASE_URL")
		if ollamaAPIKey != "" || ollamaBaseURL != "" {
			registered := 0

			// Planners (unique — planning is a separate phase)
			for _, m := range []struct{ envVar, name string }{
				{"OLLAMA_PLANNER_1", "ollama-planner-1"},
				{"OLLAMA_PLANNER_2", "ollama-planner-2"},
			} {
				model := os.Getenv(m.envVar)
				if model == "" {
					continue
				}
				providerList = append(providerList,
					providers.NewOllamaCloudProvider(ollamaBaseURL, ollamaAPIKey, model, m.name))
				log.Printf("✓ %s provider initialized (model=%s)", m.name, model)
				registered++
			}

			// Chain models — grouped by level with | separator
			// Each | group is a level, models within rotate/cross-review
			chainLevels := ParseOllamaChain(os.Getenv("OLLAMA_CHAIN"))
			modelIdx := 0
			for _, models := range chainLevels {
				for _, model := range models {
					modelIdx++
					name := fmt.Sprintf("ollama-chain-%d", modelIdx)
					providerList = append(providerList,
						providers.NewOllamaCloudProvider(ollamaBaseURL, ollamaAPIKey, model, name))
					log.Printf("✓ %s provider initialized (model=%s)", name, model)
					registered++
				}
			}

			// Fallback: single OLLAMA_MODEL if no chain configured
			if registered == 0 {
				ollamaModel := os.Getenv("OLLAMA_MODEL")
				if ollamaModel != "" {
					providerList = append(providerList,
						providers.NewOllamaCloudProvider(ollamaBaseURL, ollamaAPIKey, ollamaModel, "ollama-cloud"))
					log.Printf("✓ ollama-cloud provider initialized (model=%s)", ollamaModel)
				}
			}
		}

		// NanoGPT subscription (55K tokens/week cap)
		// Disable via NANOGPT_DISABLED=true (kills both sub+paid) or NANOGPT_SUB_DISABLED=true (sub only)
		if nanogptAPIKey != "" && os.Getenv("NANOGPT_DISABLED") != "true" && os.Getenv("NANOGPT_SUB_DISABLED") != "true" {
			providerList = append(providerList, providers.NewNanoGPTProvider(nanogptAPIKey, "subscription"))
		}

		// Subscription providers (gemini, codex, claude-code)
		providerList = append(providerList, initializePersonalProviders()...)
	}

	// NanoGPT paid (last — costs money, only used as fallback, personal only)
	// Disable via NANOGPT_DISABLED=true to prevent all NanoGPT API calls
	if nanogptAPIKey != "" && profile != "work" && os.Getenv("NANOGPT_DISABLED") != "true" {
		providerList = append(providerList, providers.NewNanoGPTProvider(nanogptAPIKey, "paid"))
	}

	return providerList
}

func initializePersonalProviders() []providers.Provider {
	var providerList []providers.Provider
	sps, err := subscriptions.LoadRuntimeProviders(context.Background())
	if err != nil {
		log.Printf("Warning: failed to init subscription providers: %v", err)
		return providerList
	}
	for _, p := range sps {
		providerList = append(providerList, p)
	}
	return providerList
}

func initializeWorkProviders() []providers.Provider {
	var providerList []providers.Provider

	claudeProject := envFirst("VERTEX_CLAUDE_PROJECT", "ANTHROPIC_VERTEX_PROJECT_ID", "VERTEX_PROJECT_ID")
	claudeRegion := envFirst("VERTEX_CLAUDE_REGION", "VERTEX_REGION")
	if claudeRegion == "" {
		claudeRegion = "us-east5"
	}
	if claudeProject != "" {
		providerList = append(providerList, providers.NewVertexProvider(providers.VertexConfig{
			Name:      "vertex-claude",
			Project:   claudeProject,
			Location:  claudeRegion,
			Publisher: "anthropic",
			Prefix:    "claude",
		}))
	}

	geminiProject := envFirst("VERTEX_GEMINI_PROJECT", "GEMINI_PROJECT")
	geminiLocation := envFirst("VERTEX_GEMINI_LOCATION", "GEMINI_LOCATION")
	if geminiLocation == "" {
		geminiLocation = "global"
	}
	geminiSAKey := envFirst("VERTEX_GEMINI_SA_KEY", "GOOGLE_SERVICE_ACCOUNT_JSON")
	if geminiProject != "" {
		providerList = append(providerList, providers.NewVertexProvider(providers.VertexConfig{
			Name:      "vertex-gemini",
			Project:   geminiProject,
			Location:  geminiLocation,
			Publisher: "google",
			SAKeyFile: geminiSAKey,
			Prefix:    "gemini",
		}))
	}

	return providerList
}

// buildEscalationChain creates the escalation chain from OLLAMA_CHAIN env var.
// Format: model1,model2|model3,model4|model5 — pipe separates levels, comma separates models within a level.
// Each level's models rotate (cross-review). Each level = 2 stages of work.
func buildEscalationChain(profile string) []agent.EscalationLevel {
	var chain []agent.EscalationLevel

	chainLevels := ParseOllamaChain(os.Getenv("OLLAMA_CHAIN"))
	modelIdx := 0
	for _, models := range chainLevels {
		var levelProviders []string
		for range models {
			modelIdx++
			levelProviders = append(levelProviders, fmt.Sprintf("ollama-chain-%d", modelIdx))
		}
		if len(levelProviders) > 0 {
			chain = append(chain, agent.EscalationLevel{Providers: levelProviders})
		}
	}

	// Subscription providers
	if profile != "work" {
		sps, err := subscriptions.LoadRuntimeProviders(context.Background())
		if err == nil {
			for _, p := range sps {
				chain = append(chain, agent.EscalationLevel{Providers: []string{p.Name()}})
			}
		}
	}

	// NanoGPT paid — last resort
	if os.Getenv("NANOGPT_API_KEY") != "" && profile != "work" {
		chain = append(chain, agent.EscalationLevel{Providers: []string{"nanogpt-paid"}})
	}

	var names []string
	for _, level := range chain {
		names = append(names, fmt.Sprintf("%v", level.Providers))
	}
	log.Printf("[Agent] escalation chain (%d levels): %s", len(chain), strings.Join(names, " → "))

	return chain
}

// ParseOllamaChain parses the OLLAMA_CHAIN env var format.
// Pipe separates levels, comma separates models within a level.
// Returns grouped model names: [[model1,model2],[model3,model4],...]
func ParseOllamaChain(chainStr string) [][]string {
	if chainStr == "" {
		return nil
	}
	var levels [][]string
	for _, levelStr := range strings.Split(chainStr, "|") {
		var models []string
		for _, model := range strings.Split(levelStr, ",") {
			model = strings.TrimSpace(model)
			if model != "" {
				models = append(models, model)
			}
		}
		if len(models) > 0 {
			levels = append(levels, models)
		}
	}
	return levels
}

// LoadDotEnv searches for .env files in standard locations.
func LoadDotEnv() error {
	home, _ := os.UserHomeDir()
	candidates := []string{
		".env",
		filepath.Join(home, ".mcp", "synapse", ".env"),
	}

	if execPath, err := os.Executable(); err == nil {
		if resolved, err := filepath.EvalSymlinks(execPath); err == nil {
			execEnv := filepath.Join(filepath.Dir(resolved), ".env")
			if execEnv != ".env" {
				candidates = append(candidates, execEnv)
			}
		}
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return godotenv.Load(candidate)
		}
	}

	return os.ErrNotExist
}

func envFirst(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}
