package agent

import (
	"embed"
	"log"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed intent_routes/*.yaml
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
