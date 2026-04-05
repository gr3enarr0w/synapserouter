package agent

import (
	"encoding/json"
	"embed"
	"log"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed intent_routes/*.yaml intent_data/*.json
var embeddedIntentRoutes embed.FS

// IntentRouteConfig represents a YAML intent route file
type IntentRouteConfig struct {
	Intent              string   `yaml:"intent"`
	ToolGroup           string   `yaml:"tool_group"`
	Keywords            []string `yaml:"keywords"`
	QuestionPrefixes    []string `yaml:"question_prefixes"`
	ShortMessageDefault bool     `yaml:"short_message_default"`
	Description         string   `yaml:"description"`
}

// loadIntentRoutes loads intent routes from embedded YAML + user directory
func loadIntentRoutes() []IntentRouteConfig {
	var routes []IntentRouteConfig

	// Load embedded routes
	entries, err := embeddedIntentRoutes.ReadDir("intent_routes")
	if err != nil {
		log.Printf("[IntentRouter] warning: no embedded intent routes: %v", err)
	} else {
		for _, entry := range entries {
			if !strings.HasSuffix(entry.Name(), ".yaml") {
				continue
			}
			data, err := embeddedIntentRoutes.ReadFile("intent_routes/" + entry.Name())
			if err != nil {
				log.Printf("[IntentRouter] warning: can't read %s: %v", entry.Name(), err)
				continue
			}
			var cfg IntentRouteConfig
			if err := yaml.Unmarshal(data, &cfg); err != nil {
				log.Printf("[IntentRouter] warning: can't parse %s: %v", entry.Name(), err)
				continue
			}
			routes = append(routes, cfg)
		}
	}

	// Load user routes from ~/.synroute/intents/
	home, err := os.UserHomeDir()
	if err == nil {
		userDir := filepath.Join(home, ".synroute", "intents")
		userEntries, err := os.ReadDir(userDir)
		if err == nil {
			for _, entry := range userEntries {
				if !strings.HasSuffix(entry.Name(), ".yaml") {
					continue
				}
				data, err := os.ReadFile(filepath.Join(userDir, entry.Name()))
				if err != nil {
					continue
				}
				var cfg IntentRouteConfig
				if err := yaml.Unmarshal(data, &cfg); err != nil {
					continue
				}
				routes = append(routes, cfg)
				log.Printf("[IntentRouter] loaded user intent route: %s (%d keywords)", cfg.Intent, len(cfg.Keywords))
			}
		}
	}

	return routes
}

// applyRoutesToRouter adds YAML-loaded keywords to the router's maps
func applyRoutesToRouter(r *IntentRouter, routes []IntentRouteConfig) {
	for _, route := range routes {
		intent := Intent(route.Intent)

		// Add keywords to exact phrase map
		for _, kw := range route.Keywords {
			kw = strings.ToLower(strings.TrimSpace(kw))
			if kw == "" {
				continue
			}
			r.exactPhraseToIntent[kw] = intent
		}

		// Add question prefixes (chat only)
		if route.Intent == "chat" && len(route.QuestionPrefixes) > 0 {
			r.questionPrefixes = append(r.questionPrefixes, route.QuestionPrefixes...)
		}

		// Add greeting keywords (single words from chat)
		if route.Intent == "chat" {
			for _, kw := range route.Keywords {
				kw = strings.ToLower(strings.TrimSpace(kw))
				// Single words or very short → also add as greetings
				if !strings.Contains(kw, " ") && len(kw) <= 10 {
					r.greetingKeywords[kw] = true
				}
			}
		}
	}
}

// IntentCorrection represents a user-corrected intent classification
type IntentCorrection struct {
	Message string `json:"message"`
	Intent  string `json:"intent"`
}

// loadCorrections reads user corrections from ~/.synroute/intent_corrections.json
func loadCorrections() []IntentCorrection {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	correctionsPath := filepath.Join(home, ".synroute", "intent_corrections.json")
	data, err := os.ReadFile(correctionsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		log.Printf("[IntentRouter] warning: can't read corrections: %v", err)
		return nil
	}
	var corrections []IntentCorrection
	if err := json.Unmarshal(data, &corrections); err != nil {
		log.Printf("[IntentRouter] warning: can't parse corrections: %v", err)
		return nil
	}
	return corrections
}

// saveIntentCorrection appends a correction to ~/.synroute/intent_corrections.json
func saveIntentCorrection(message, intent string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(home, ".synroute")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	correctionsPath := filepath.Join(dir, "intent_corrections.json")

	// Load existing corrections
	var corrections []IntentCorrection
	data, err := os.ReadFile(correctionsPath)
	if err == nil {
		if err := json.Unmarshal(data, &corrections); err != nil {
			corrections = nil
		}
	}

	// Deduplicate: if same message already has this intent, skip.
	// If same message has a DIFFERENT intent, update it (latest wins).
	msgLower := strings.ToLower(strings.TrimSpace(message))
	found := false
	for i := range corrections {
		if strings.ToLower(strings.TrimSpace(corrections[i].Message)) == msgLower {
			corrections[i].Intent = intent // update to latest
			found = true
			break
		}
	}
	if !found {
		corrections = append(corrections, IntentCorrection{Message: message, Intent: intent})
	}

	// Cap at 200 corrections to prevent unbounded growth.
	// Keep the most recent ones (end of slice).
	if len(corrections) > 200 {
		corrections = corrections[len(corrections)-200:]
	}

	// Write back
	outData, err := json.MarshalIndent(corrections, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(correctionsPath, outData, 0644)
}

// applyCorrectionsToRouter adds user corrections as high-priority exact phrase matches
func applyCorrectionsToRouter(r *IntentRouter, corrections []IntentCorrection) {
	for _, c := range corrections {
		msg := strings.ToLower(strings.TrimSpace(c.Message))
		if msg == "" {
			continue
		}
		intent := Intent(c.Intent)
		r.exactPhraseToIntent[msg] = intent
		log.Printf("[IntentRouter] loaded correction: '%s' -> %s", msg, intent)
	}
}
