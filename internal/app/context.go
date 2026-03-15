package app

import (
	"context"
	"database/sql"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
	_ "github.com/mattn/go-sqlite3"

	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/memory"
	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/providers"
	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/router"
	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/subscriptions"
	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/usage"
)

// AppContext holds shared state for CLI commands and API handlers.
type AppContext struct {
	DB           *sql.DB
	UsageTracker *usage.Tracker
	VectorMemory *memory.VectorMemory
	Providers    []providers.Provider
	ProxyRouter  *router.Router
	Profile      string
	Port         string
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

	return &AppContext{
		DB:           db,
		UsageTracker: tracker,
		VectorMemory: vm,
		Providers:    providerList,
		Profile:      profile,
		Port:         port,
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

	// NanoGPT subscription (first — free, zero-cost models)
	nanogptAPIKey := os.Getenv("NANOGPT_API_KEY")
	if nanogptAPIKey != "" {
		providerList = append(providerList, providers.NewNanoGPTProvider(nanogptAPIKey, "subscription"))
	}

	switch profile {
	case "work":
		providerList = append(providerList, initializeWorkProviders()...)
	default:
		providerList = append(providerList, initializePersonalProviders()...)
	}

	// Ollama Cloud (available in all profiles)
	ollamaAPIKey := os.Getenv("OLLAMA_API_KEY")
	ollamaBaseURL := os.Getenv("OLLAMA_BASE_URL")
	ollamaModel := os.Getenv("OLLAMA_MODEL")
	if ollamaAPIKey != "" {
		providerList = append(providerList, providers.NewOllamaCloudProvider(ollamaBaseURL, ollamaAPIKey, ollamaModel))
	}

	// NanoGPT paid (last — costs money, only used as fallback)
	if nanogptAPIKey != "" {
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
