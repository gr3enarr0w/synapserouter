package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gr3enarr0w/synapserouter/internal/agent"
	"github.com/gr3enarr0w/synapserouter/internal/app"
	"github.com/gr3enarr0w/synapserouter/internal/router"
	"github.com/gr3enarr0w/synapserouter/internal/tools"
)

var (
	agentPool     *agent.Pool
	agentPoolOnce sync.Once
)

func smokeTestHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Provider string `json:"provider"`
		Timeout  string `json:"timeout"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	timeout := 45 * time.Second
	if req.Timeout != "" {
		if parsed, err := time.ParseDuration(req.Timeout); err == nil {
			timeout = parsed
		}
	}

	opts := app.SmokeTestOpts{
		Provider: req.Provider,
		Timeout:  timeout,
	}

	results := app.RunSmokeTests(r.Context(), providerList, opts)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

func circuitBreakerResetHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Provider string `json:"provider"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	var resetProviders []string
	var err error

	if req.Provider != "" {
		if proxyRouter != nil {
			err = proxyRouter.ResetCircuitBreaker(req.Provider)
			if err == nil {
				resetProviders = []string{req.Provider}
			}
		} else {
			// Fallback: reset directly in DB
			cb := router.NewCircuitBreaker(db, req.Provider)
			err = cb.Reset()
			if err == nil {
				resetProviders = []string{req.Provider}
			}
		}
	} else {
		if proxyRouter != nil {
			resetProviders, err = proxyRouter.ResetAllCircuitBreakers()
		} else {
			resetProviders, err = router.ResetAllCircuitStates(db)
		}
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	states, _ := router.GetAllCircuitStates(db)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"reset":  resetProviders,
		"states": states,
	})
}

func profileHandler(w http.ResponseWriter, r *http.Request) {
	names := make([]string, len(providerList))
	for i, p := range providerList {
		names[i] = p.Name()
	}

	info := app.ShowProfile(names)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

func profileSwitchHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Profile string `json:"profile"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(req.Profile) == "" {
		http.Error(w, "profile field is required", http.StatusBadRequest)
		return
	}

	if err := app.SwitchProfile(req.Profile); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "updated",
		"profile": req.Profile,
		"message": "Restart server to apply",
	})
}

func skillsListHandler(w http.ResponseWriter, r *http.Request) {
	skills := orchestrator.Skills()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"skills": skills,
		"count":  len(skills),
	})
}

func toolsListHandler(registry *tools.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		info := registry.ToolInfo()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"tools": info,
			"count": len(info),
		})
	}
}

func skillsMatchHandler(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, "q parameter is required", http.StatusBadRequest)
		return
	}

	result := orchestrator.MatchSkillsForGoal(query)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func doctorHandler(w http.ResponseWriter, r *http.Request) {
	ac := &app.AppContext{
		DB:           db,
		UsageTracker: usageTracker,
		VectorMemory: vectorMemory,
		Providers:    providerList,
		Profile:      strings.ToLower(strings.TrimSpace(os.Getenv("ACTIVE_PROFILE"))),
	}

	checks := app.RunDiagnostics(context.Background(), ac)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(checks)
}

func getAgentPool() *agent.Pool {
	agentPoolOnce.Do(func() {
		agentPool = agent.NewPool(5)
	})
	return agentPool
}

func agentChatHandler(registry *tools.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Message     string `json:"message"`
			Model       string `json:"model"`
			System      string `json:"system"`
			MaxTurns    int    `json:"max_turns"`
			MaxTokens   int64  `json:"max_tokens"`
			SessionID   string `json:"session_id"`
			WorkDir     string `json:"work_dir"`
			Project     string `json:"project"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if req.Message == "" {
			http.Error(w, "message is required", http.StatusBadRequest)
			return
		}

		config := agent.DefaultConfig()
		if req.Model != "" {
			config.Model = req.Model
		}
		if req.System != "" {
			config.SystemPrompt = req.System
		}
		if req.MaxTurns > 0 {
			config.MaxTurns = req.MaxTurns
		}

		// Determine working directory:
		// 1. Explicit work_dir from request (absolute path)
		// 2. project name → ~/Development/<project>/
		// 3. Temp directory (cleaned up after request)
		switch {
		case req.WorkDir != "":
			config.WorkDir = req.WorkDir
			os.MkdirAll(config.WorkDir, 0755)
		case req.Project != "":
			home, _ := os.UserHomeDir()
			config.WorkDir = filepath.Join(home, "Development", req.Project)
			os.MkdirAll(config.WorkDir, 0755)
		default:
			tmpDir, tmpErr := os.MkdirTemp("", "synroute-agent-*")
			if tmpErr == nil {
				config.WorkDir = tmpDir
				// Don't delete — files must persist after agent exits
			} else {
				cwd, _ := os.Getwd()
				config.WorkDir = cwd
			}
		}

		// Wire memory systems for unlimited context + recall + hallucination detection
		config.VectorMemory = vectorMemory
		config.ToolStore = agent.NewToolOutputStore(db)
		config.PlanCache = agent.NewPlanCache(db)

		// Create agent with tracing enabled
		ag := agent.New(proxyRouter, registry, nil, config)
		ag.EnableTracing()

		// Set budget if specified
		if req.MaxTokens > 0 {
			ag.SetInputGuardrails(agent.NewGuardrailChain(&agent.SecretPatternGuardrail{}))
		}

		// Register delegation tools
		agentRegistry := tools.DefaultRegistry()
		agentRegistry.Register(agent.NewDelegateTool(ag))

		pool := getAgentPool()
		result, err := pool.RunInPool(r.Context(), ag, "api", req.Message)

		response := map[string]interface{}{
			"session_id": ag.SessionID(),
			"model":      config.Model,
		}

		if err != nil {
			response["error"] = err.Error()
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			response["response"] = result
		}

		// Include trace if available
		if trace := ag.Trace(); trace != nil {
			response["trace"] = map[string]interface{}{
				"spans":    trace.SpanCount(),
				"duration": trace.TotalDuration().String(),
			}
		}

		// Include pool metrics
		response["pool"] = pool.Metrics()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}
}

func agentPoolHandler(w http.ResponseWriter, r *http.Request) {
	pool := getAgentPool()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(pool.Metrics())
}
