package app

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
	_ "github.com/mattn/go-sqlite3"

	"github.com/gr3enarr0w/synapserouter/internal/agent"
	"github.com/gr3enarr0w/synapserouter/internal/memory"
	"github.com/gr3enarr0w/synapserouter/internal/providers"
	"github.com/gr3enarr0w/synapserouter/internal/router"
	"github.com/gr3enarr0w/synapserouter/internal/subscriptions"
	"github.com/gr3enarr0w/synapserouter/internal/usage"
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
	// SQLite connection limits — prevents bottleneck with concurrent agents
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(2)

	// Run embedded migrations if registered (ensures all tables exist for code mode)
	if hasMigrations {
		if err := RunEmbeddedMigrations(db, registeredMigrations); err != nil {
			log.Printf("Warning: embedded migration error: %v", err)
		}
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

// registeredMigrations holds the embedded migration FS, set by RegisterMigrations.
var (
	registeredMigrations    embed.FS
	hasMigrations           bool
)

// RegisterMigrations stores the embedded migration FS for use by InitLight.
// Call from main.go init() or before InitLight.
func RegisterMigrations(fs embed.FS) {
	registeredMigrations = fs
	hasMigrations = true
}

// RunEmbeddedMigrations applies all .sql files from the embedded FS to the database.
func RunEmbeddedMigrations(db *sql.DB, fs embed.FS) error {
	files, err := fs.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}
	for _, file := range files {
		if !strings.HasSuffix(file.Name(), ".sql") {
			continue
		}
		data, err := fs.ReadFile("migrations/" + file.Name())
		if err != nil {
			return fmt.Errorf("read migration %s: %w", file.Name(), err)
		}
		if _, err := db.Exec(string(data)); err != nil {
			if strings.Contains(err.Error(), "duplicate column") {
				continue
			}
			return fmt.Errorf("execute migration %s: %w", file.Name(), err)
		}
	}
	return nil
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

	switch profile {
	case "work":
		providerList = append(providerList, initializeWorkProviders()...)
	default:
		// Ollama Cloud — supports multiple API keys for concurrent subscriptions
		apiKeys := ParseOllamaAPIKeys()
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
			// Each | group is a level, models within rotate/cross-review
			chainLevels := ParseOllamaChain(os.Getenv("OLLAMA_CHAIN"))
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

		// Subscription providers (gemini, codex, claude-code) — disable with SUBSCRIPTIONS_DISABLED=true
		if os.Getenv("SUBSCRIPTIONS_DISABLED") != "true" {
			providerList = append(providerList, initializePersonalProviders()...)
		}
	}

	return providerList
}

// ParseOllamaAPIKeys returns all available Ollama API keys.
// Supports OLLAMA_API_KEYS (comma-separated, for multiple subscriptions)
// and falls back to single OLLAMA_API_KEY.
func ParseOllamaAPIKeys() []string {
	if keys := os.Getenv("OLLAMA_API_KEYS"); keys != "" {
		var parsed []string
		for _, k := range strings.Split(keys, ",") {
			k = strings.TrimSpace(k)
			if k != "" {
				parsed = append(parsed, k)
			}
		}
		if len(parsed) > 0 {
			return parsed
		}
	}
	if key := os.Getenv("OLLAMA_API_KEY"); key != "" {
		return []string{key}
	}
	return nil
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

	geminiProject := envFirst("VERTEX_GEMINI_PROJECT", "GEMINI_PROJECT")
	geminiLocation := envFirst("VERTEX_GEMINI_LOCATION", "GEMINI_LOCATION")
	if geminiLocation == "" {
		geminiLocation = "global"
	}
	geminiSAKey := envFirst("VERTEX_GEMINI_SA_KEY", "GOOGLE_SERVICE_ACCOUNT_JSON")

	// Create tiered providers: 2 per level (Claude + Gemini)
	// L0 cheap: haiku + flash
	// L1 mid: sonnet + pro
	// L2 frontier: opus + pro-preview
	// Vertex model names per tier
	// Claude: haiku-4.5 (cheap), sonnet-4.6 (mid), opus-4.6 (frontier)
	// Gemini: flash (cheap), pro (mid+frontier) — only 2 distinct tiers
	claudeModels := []struct{ name, model string }{
		{"vertex-claude-cheap", "claude-haiku-4-5"},
		{"vertex-claude-mid", "claude-sonnet-4-6"},
		{"vertex-claude-frontier", "claude-opus-4-6"},
	}
	geminiModels := []struct{ name, model string }{
		{"vertex-gemini-cheap", "gemini-3-flash-preview"},
		{"vertex-gemini-mid", "gemini-3.1-pro-preview"},
		{"vertex-gemini-frontier", "gemini-3.1-pro-preview"},
	}

	if claudeProject != "" {
		for _, m := range claudeModels {
			providerList = append(providerList, providers.NewVertexProvider(providers.VertexConfig{
				Name:      m.name,
				Project:   claudeProject,
				Location:  claudeRegion,
				Publisher: "anthropic",
				Prefix:    "claude",
				Model:     m.model,
			}))
		}
		log.Printf("✓ vertex-claude: 3 tiers (haiku/sonnet/opus) on %s/%s", claudeProject, claudeRegion)
	}

	if geminiProject != "" {
		for _, m := range geminiModels {
			providerList = append(providerList, providers.NewVertexProvider(providers.VertexConfig{
				Name:      m.name,
				Project:   geminiProject,
				Location:  geminiLocation,
				Publisher: "google",
				SAKeyFile: geminiSAKey,
				Prefix:    "gemini",
				Model:     m.model,
			}))
		}
		log.Printf("✓ vertex-gemini: 3 tiers (flash/pro/pro-preview) on %s/%s", geminiProject, geminiLocation)
	}

	// models.corp (Red Hat AI Inference) — OpenAI-compatible endpoint
	// Requires VPN. Auth via user_key. Configure:
	//   MODELS_CORP_BASE_URL=https://models.corp.redhat.com/v1
	//   MODELS_CORP_USER_KEY=your-user-key
	//   MODELS_CORP_MODEL=model-name (optional, default: auto)
	modelsCorpURL := os.Getenv("MODELS_CORP_BASE_URL")
	modelsCorpKey := os.Getenv("MODELS_CORP_USER_KEY")
	modelsCorpModel := os.Getenv("MODELS_CORP_MODEL")
	if modelsCorpURL != "" {
		if modelsCorpModel == "" {
			modelsCorpModel = "auto"
		}
		providerList = append(providerList, providers.NewOllamaCloudProvider(
			modelsCorpURL, modelsCorpKey, modelsCorpModel, "models-corp",
		))
		log.Printf("✓ models-corp provider initialized (%s)", modelsCorpURL)
	}

	return providerList
}

// buildEscalationChain creates the escalation chain from OLLAMA_CHAIN env var.
// Format: model1,model2|model3,model4|model5 — pipe separates levels, comma separates models within a level.
// Each level's models rotate (cross-review). Each level = 2 stages of work.
//
// Tier classification (three-tier model routing):
//   - If OLLAMA_CHAIN_TIERS is set (pipe-separated: "cheap|cheap|mid|frontier"),
//     each level gets the corresponding tier.
//   - Otherwise, auto-classifies: bottom third = cheap, middle = mid, top third = frontier.
//   - Subscription providers always get frontier tier.
// buildWorkEscalationChain builds the escalation chain for the work profile.
// Uses WORK_CHAIN env var (same format as OLLAMA_CHAIN: pipe-separated levels,
// comma-separated providers within a level). Provider names reference the
// tiered providers created by initializeWorkProviders.
//
// Example: WORK_CHAIN=vertex-claude-cheap|vertex-claude-mid,vertex-gemini-mid|vertex-claude-frontier,vertex-gemini-frontier
//
// If WORK_CHAIN is not set, creates a default 3-level chain from available providers.
func buildWorkEscalationChain() []agent.EscalationLevel {
	// User-configured chain
	if workChain := os.Getenv("WORK_CHAIN"); workChain != "" {
		levels := ParseOllamaChain(workChain) // same format
		var chain []agent.EscalationLevel
		for _, providers := range levels {
			chain = append(chain, agent.EscalationLevel{Providers: providers})
		}
		// Apply explicit tiers if set
		if tiers := parseChainTiers(os.Getenv("WORK_CHAIN_TIERS")); len(tiers) > 0 {
			for i := range chain {
				if i < len(tiers) {
					chain[i].Tier = tiers[i]
				}
			}
		} else {
			autoClassifyTiers(chain, len(chain))
		}
		log.Printf("[Agent] work escalation chain (configured): %d levels", len(chain))
		return chain
	}

	// Default: 3 levels from available tiered providers
	chain := []agent.EscalationLevel{
		{Providers: []string{"vertex-claude-cheap"}, Tier: agent.TierCheap},
		{Providers: []string{"vertex-claude-mid", "vertex-gemini-mid"}, Tier: agent.TierMid},
		{Providers: []string{"vertex-claude-frontier", "vertex-gemini-frontier"}, Tier: agent.TierFrontier},
	}
	log.Printf("[Agent] work escalation chain (default): 3 levels")
	return chain
}

func buildEscalationChain(profile string) []agent.EscalationLevel {
	var chain []agent.EscalationLevel

	// Work profile: Vertex AI escalation chain (Claude + Gemini per level)
	if profile == "work" {
		return buildWorkEscalationChain()
	}

	// YAML config takes priority over OLLAMA_CHAIN env var
	if tc, err := LoadTierConfig(); err == nil && tc != nil {
		yamlChain := tc.ToEscalationChain()
		if len(yamlChain) > 0 {
			warnings := ValidateTierConfig(tc)
			for _, w := range warnings {
				log.Printf("[Config] warning: %s", w)
			}
			log.Printf("[Config] using YAML tier config (%d levels)", len(yamlChain))
			return yamlChain
		}
	}

	chainLevels := ParseOllamaChain(os.Getenv("OLLAMA_CHAIN"))
	explicitTiers := parseChainTiers(os.Getenv("OLLAMA_CHAIN_TIERS"))

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

	// Apply tier classification to Ollama chain levels
	ollamaLevelCount := len(chain)
	if len(explicitTiers) > 0 {
		// Explicit tiers from OLLAMA_CHAIN_TIERS
		for i := range chain {
			if i < len(explicitTiers) {
				chain[i].Tier = explicitTiers[i]
			}
		}
	} else {
		// Auto-classify: bottom third = cheap, middle = mid, top third = frontier
		autoClassifyTiers(chain, ollamaLevelCount)
	}

	// Subscription providers — disable with SUBSCRIPTIONS_DISABLED=true
	// Subscriptions always get frontier tier (they're the strongest/most expensive).
	if profile != "work" && os.Getenv("SUBSCRIPTIONS_DISABLED") != "true" {
		sps, err := subscriptions.LoadRuntimeProviders(context.Background())
		if err == nil {
			for _, p := range sps {
				chain = append(chain, agent.EscalationLevel{
					Providers: []string{p.Name()},
					Tier:      agent.TierFrontier,
				})
			}
		}
	}

	var names []string
	for _, level := range chain {
		tier := string(level.Tier)
		if tier == "" {
			tier = "auto"
		}
		names = append(names, fmt.Sprintf("%v(%s)", level.Providers, tier))
	}
	log.Printf("[Agent] escalation chain (%d levels): %s", len(chain), strings.Join(names, " → "))

	return chain
}

// parseChainTiers parses OLLAMA_CHAIN_TIERS env var.
// Format: "cheap|cheap|mid|frontier" — pipe-separated tier names matching OLLAMA_CHAIN levels.
func parseChainTiers(tiersStr string) []agent.ModelTier {
	if tiersStr == "" {
		return nil
	}
	var tiers []agent.ModelTier
	for _, t := range strings.Split(tiersStr, "|") {
		t = strings.TrimSpace(strings.ToLower(t))
		switch t {
		case "cheap":
			tiers = append(tiers, agent.TierCheap)
		case "mid":
			tiers = append(tiers, agent.TierMid)
		case "frontier":
			tiers = append(tiers, agent.TierFrontier)
		default:
			// Unknown tier — default to mid
			tiers = append(tiers, agent.TierMid)
		}
	}
	return tiers
}

// autoClassifyTiers assigns tiers based on position in the chain.
// Bottom third = cheap, middle third = mid, top third = frontier.
// For chains with 1-2 levels, all levels get frontier (no point splitting).
func autoClassifyTiers(chain []agent.EscalationLevel, ollamaCount int) {
	if ollamaCount <= 2 {
		for i := 0; i < ollamaCount && i < len(chain); i++ {
			chain[i].Tier = agent.TierFrontier
		}
		return
	}

	cheapEnd := ollamaCount / 3
	midEnd := (2 * ollamaCount) / 3

	for i := 0; i < ollamaCount && i < len(chain); i++ {
		switch {
		case i < cheapEnd:
			chain[i].Tier = agent.TierCheap
		case i < midEnd:
			chain[i].Tier = agent.TierMid
		default:
			chain[i].Tier = agent.TierFrontier
		}
	}
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

// LoadDotEnv loads .env files from standard locations.
// Priority: binary's directory (project config) first, then ~/.mcp/synapse/.env.
// Does NOT load CWD/.env to avoid picking up unrelated secrets (e.g., ~/.env
// containing GitHub tokens when running from ~).
// godotenv does not overwrite existing env vars, so first-loaded wins.
func LoadDotEnv() error {
	home, _ := os.UserHomeDir()
	loaded := false

	// 1. Binary's directory — the project .env (highest priority when running from source)
	if execPath, err := os.Executable(); err == nil {
		if resolved, err := filepath.EvalSymlinks(execPath); err == nil {
			execEnv := filepath.Join(filepath.Dir(resolved), ".env")
			if _, err := os.Stat(execEnv); err == nil {
				if err := godotenv.Load(execEnv); err == nil {
					loaded = true
				}
			}
		}
	}

	// 2. Current working directory — for installed binaries running in project dirs.
	// When synroute is installed to ~/.local/bin/ but run from a project directory
	// that has its own .env (with API keys, chain config, etc.), load it.
	// godotenv.Load does NOT override existing vars, so binary-dir .env wins.
	// Security note: matches Docker/Node/Rails behavior. Attacker would need user
	// to clone malicious repo + run synroute + not have their own .env already.
	if cwd, err := os.Getwd(); err == nil {
		cwdEnv := filepath.Join(cwd, ".env")
		if _, err := os.Stat(cwdEnv); err == nil {
			if err := godotenv.Load(cwdEnv); err == nil {
				log.Printf("Loaded .env from working directory: %s", cwdEnv)
				loaded = true
			}
		}
	}

	// 3. User config directory
	userEnv := filepath.Join(home, ".mcp", "synapse", ".env")
	if _, err := os.Stat(userEnv); err == nil {
		godotenv.Load(userEnv) // merge, don't overwrite
		loaded = true
	}

	if !loaded {
		return os.ErrNotExist
	}
	return nil
}

func envFirst(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}
