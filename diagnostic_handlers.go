package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/app"
	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/router"
	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/tools"
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
