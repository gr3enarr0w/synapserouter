package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/eval"
)

func evalExercisesHandler(w http.ResponseWriter, r *http.Request) {
	store := eval.NewStore(db)

	suite := r.URL.Query().Get("suite")
	language := r.URL.Query().Get("language")

	exercises, err := store.ListExercises(suite, language)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	count, _ := store.CountExercises(suite, language)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"exercises": exercises,
		"count":     count,
	})
}

func evalRunsListHandler(w http.ResponseWriter, r *http.Request) {
	store := eval.NewStore(db)

	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	runs, err := store.ListRuns(limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"runs":  runs,
		"count": len(runs),
	})
}

func evalRunStartHandler(w http.ResponseWriter, r *http.Request) {
	var config eval.EvalRunConfig
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if config.Mode == "" {
		if config.Provider != "" {
			config.Mode = "direct"
		} else {
			config.Mode = "routing"
		}
	}

	if !eval.IsDockerAvailable() {
		http.Error(w, "Docker is not available", http.StatusServiceUnavailable)
		return
	}

	store := eval.NewStore(db)
	runner := eval.NewRunner(store, proxyRouter, providerList)

	// Run async
	runID := ""
	run := eval.EvalRun{
		ID:        "eval-" + strconv.FormatInt(time.Now().UnixNano(), 10),
		Config:    config,
		Status:    "running",
		StartedAt: time.Now(),
	}
	if err := store.CreateRun(run); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	runID = run.ID

	go func() {
		config2 := config
		// Use a fresh context since the HTTP request context will be canceled
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()

		_, err := runner.Run(ctx, config2)
		if err != nil {
			store.FailRun(runID)
		}
	}()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"run_id": runID,
		"status": "running",
	})
}

func evalRunGetHandler(w http.ResponseWriter, r *http.Request) {
	store := eval.NewStore(db)
	runID := r.PathValue("run_id")

	run, err := store.GetRun(runID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if run == nil {
		http.Error(w, "run not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(run)
}

func evalRunResultsHandler(w http.ResponseWriter, r *http.Request) {
	store := eval.NewStore(db)
	runID := r.PathValue("run_id")

	results, err := store.GetResultsByRun(runID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"run_id":  runID,
		"results": results,
		"count":   len(results),
	})
}

func evalCompareHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RunA string `json:"run_a"`
		RunB string `json:"run_b"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(req.RunA) == "" || strings.TrimSpace(req.RunB) == "" {
		http.Error(w, "run_a and run_b are required", http.StatusBadRequest)
		return
	}

	store := eval.NewStore(db)

	runA, err := store.GetRun(req.RunA)
	if err != nil || runA == nil {
		http.Error(w, "run_a not found", http.StatusNotFound)
		return
	}
	runB, err := store.GetRun(req.RunB)
	if err != nil || runB == nil {
		http.Error(w, "run_b not found", http.StatusNotFound)
		return
	}

	comp := eval.CompareRuns(runA, runB)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(comp)
}

func evalImportHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Source string `json:"source"`
		Path   string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Source == "" || req.Path == "" {
		http.Error(w, "source and path are required", http.StatusBadRequest)
		return
	}

	store := eval.NewStore(db)

	var result *eval.ImportResult
	var err error
	switch req.Source {
	case "polyglot":
		result, err = eval.ImportPolyglot(store, req.Path)
	case "roocode":
		result, err = eval.ImportRooCode(store, req.Path)
	default:
		http.Error(w, "unknown source (use polyglot or roocode)", http.StatusBadRequest)
		return
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}
