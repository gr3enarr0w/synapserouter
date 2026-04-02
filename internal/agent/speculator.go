package agent

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/tools"
)

// SpeculativeCache holds pre-executed tool results for the PASTE pattern.
// Predictions are launched in background goroutines while the LLM thinks;
// when the LLM responds with matching tool calls, cached results are used
// instead of re-executing.
type SpeculativeCache struct {
	mu      sync.Mutex
	pending map[string]*speculativeResult
}

type speculativeResult struct {
	result *tools.ToolResult
	err    error
	done   chan struct{} // closed when execution completes
}

// ToolPrediction is a predicted tool call.
type ToolPrediction struct {
	Name string
	Args map[string]interface{}
}

// toolCallRecord tracks a recent tool call for pattern matching.
type toolCallRecord struct {
	Name   string
	Args   map[string]interface{}
	Output string // first 500 chars for pattern extraction
}

// NewSpeculativeCache creates a new speculative execution cache.
func NewSpeculativeCache() *SpeculativeCache {
	return &SpeculativeCache{
		pending: make(map[string]*speculativeResult),
	}
}

// Speculate launches background goroutines for predicted read-only tool calls.
// Only executes tools with CategoryReadOnly — write/dangerous tools are skipped.
func (sc *SpeculativeCache) Speculate(ctx context.Context, registry *tools.Registry, workDir string, predictions []ToolPrediction) {
	if sc == nil || len(predictions) == 0 {
		return
	}

	sc.mu.Lock()
	defer sc.mu.Unlock()

	// Clear previous speculations
	sc.pending = make(map[string]*speculativeResult)

	for _, pred := range predictions {
		// Safety check: only speculate read-only tools
		tool, ok := registry.Get(pred.Name)
		if !ok || tool.Category() != tools.CategoryReadOnly {
			continue
		}

		key := speculationKey(pred.Name, pred.Args)
		sr := &speculativeResult{done: make(chan struct{})}
		sc.pending[key] = sr

		go func(name string, args map[string]interface{}, sr *speculativeResult) {
			defer close(sr.done)
			specCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			result, err := registry.Execute(specCtx, name, args, workDir)
			sr.result = result
			sr.err = err
		}(pred.Name, pred.Args, sr)
	}

	if len(sc.pending) > 0 {
		log.Printf("[Speculator] launched %d speculative tool calls", len(sc.pending))
	}
}

// Get returns a cached speculative result if the tool call was predicted.
// If the prediction is still running, waits for completion (no wasted work).
// Returns (result, true) on hit, (nil, false) on miss.
func (sc *SpeculativeCache) Get(name string, args map[string]interface{}) (*tools.ToolResult, bool) {
	if sc == nil {
		return nil, false
	}

	key := speculationKey(name, args)

	sc.mu.Lock()
	sr, ok := sc.pending[key]
	sc.mu.Unlock()

	if !ok {
		return nil, false
	}

	// Wait for completion if still running
	<-sr.done

	if sr.err != nil {
		return nil, false // speculation failed, let normal path retry
	}

	return sr.result, true
}

// Clear discards all pending speculations.
func (sc *SpeculativeCache) Clear() {
	if sc == nil {
		return
	}
	sc.mu.Lock()
	sc.pending = make(map[string]*speculativeResult)
	sc.mu.Unlock()
}

// speculationKey creates a deterministic key from tool name + args.
func speculationKey(name string, args map[string]interface{}) string {
	data, _ := json.Marshal(args)
	hash := sha256.Sum256(data)
	return fmt.Sprintf("%s:%x", name, hash[:8])
}

// URL extraction pattern for web_search results.
var urlExtractRe = regexp.MustCompile(`https?://[^\s\])"']+`)

// File path pattern for grep/glob results.
var filePathExtractRe = regexp.MustCompile(`(?m)^(\S+\.\w{1,5}):\d+:`)

// PredictNextTools predicts likely next tool calls based on recent history.
// Uses simple pattern matching — <1ms, deterministic, zero cost.
func PredictNextTools(history []toolCallRecord) []ToolPrediction {
	if len(history) == 0 {
		return nil
	}

	var predictions []ToolPrediction
	last := history[len(history)-1]

	switch last.Name {
	case "grep":
		// After grep → predict file_read on top matched files
		files := filePathExtractRe.FindAllStringSubmatch(last.Output, 5)
		seen := make(map[string]bool)
		for _, match := range files {
			path := match[1]
			if seen[path] || path == "" {
				continue
			}
			seen[path] = true
			predictions = append(predictions, ToolPrediction{
				Name: "file_read",
				Args: map[string]interface{}{"file_path": path},
			})
			if len(predictions) >= 3 {
				break
			}
		}

	case "glob":
		// After glob → predict file_read on first 3 results
		lines := strings.Split(strings.TrimSpace(last.Output), "\n")
		for i, line := range lines {
			if i >= 3 {
				break
			}
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "Found") {
				predictions = append(predictions, ToolPrediction{
					Name: "file_read",
					Args: map[string]interface{}{"file_path": line},
				})
			}
		}

	case "web_search":
		// After web_search → predict web_fetch on top 2 result URLs
		urls := urlExtractRe.FindAllString(last.Output, 5)
		seen := make(map[string]bool)
		for _, u := range urls {
			if seen[u] {
				continue
			}
			seen[u] = true
			predictions = append(predictions, ToolPrediction{
				Name: "web_fetch",
				Args: map[string]interface{}{"url": u},
			})
			if len(predictions) >= 2 {
				break
			}
		}
	}

	return predictions
}

// isSpeculationEnabled checks whether speculative execution is active.
func isSpeculationEnabled() bool {
	switch strings.ToLower(os.Getenv("SYNROUTE_SPECULATE")) {
	case "false", "0", "no":
		return false
	default:
		return true // enabled by default
	}
}
