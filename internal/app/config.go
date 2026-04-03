package app

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/gr3enarr0w/synapserouter/internal/agent"
	"github.com/gr3enarr0w/synapserouter/internal/providers"
	"gopkg.in/yaml.v3"
)

// TierConfig holds user-configurable tier definitions from YAML.
type TierConfig struct {
	Tiers                 map[string][]string                    `yaml:"tiers"`                    // "cheap" → ["model1", "model2"]
	OpenAICompatProviders []providers.OpenAICompatProviderConfig `yaml:"openai_compat_providers"`  // Custom OpenAI-compatible endpoints
}

// tierOrder defines the escalation order (ascending capability).
var tierOrder = []string{"cheap", "mid", "frontier"}

// LoadTierConfig searches for a YAML tier config file.
// Priority: .synroute.yaml in CWD → ~/.synroute/config.yaml
// Returns nil if no config file exists (fallback to env vars).
func LoadTierConfig() (*TierConfig, error) {
	// 1. Project-level: .synroute.yaml in CWD
	if data, err := os.ReadFile(".synroute.yaml"); err == nil {
		tc := &TierConfig{}
		if err := yaml.Unmarshal(data, tc); err != nil {
			return nil, fmt.Errorf("parse .synroute.yaml: %w", err)
		}
		log.Printf("[Config] loaded tier config from .synroute.yaml")
		return tc, nil
	}

	// 2. User-level: ~/.synroute/config.yaml
	home, err := os.UserHomeDir()
	if err == nil {
		configPath := filepath.Join(home, ".synroute", "config.yaml")
		if data, err := os.ReadFile(configPath); err == nil {
			tc := &TierConfig{}
			if err := yaml.Unmarshal(data, tc); err != nil {
				return nil, fmt.Errorf("parse %s: %w", configPath, err)
			}
			log.Printf("[Config] loaded tier config from %s", configPath)
			return tc, nil
		}
	}

	return nil, nil // no config file — use env var fallback
}

// ToEscalationChain converts YAML tiers to the agent's escalation chain.
// Tiers are ordered: cheap → mid → frontier (ascending capability).
func (tc *TierConfig) ToEscalationChain() []agent.EscalationLevel {
	if tc == nil || len(tc.Tiers) == 0 {
		return nil
	}

	var chain []agent.EscalationLevel
	for _, tierName := range tierOrder {
		models, ok := tc.Tiers[tierName]
		if !ok || len(models) == 0 {
			continue
		}

		tier := agent.TierCheap
		switch tierName {
		case "mid":
			tier = agent.TierMid
		case "frontier":
			tier = agent.TierFrontier
		}

		chain = append(chain, agent.EscalationLevel{
			Providers: models,
			Tier:      tier,
		})
	}

	return chain
}

// ToOllamaChain generates an OLLAMA_CHAIN string from the tier config.
func (tc *TierConfig) ToOllamaChain() string {
	if tc == nil {
		return ""
	}

	var levels []string
	for _, tierName := range tierOrder {
		models, ok := tc.Tiers[tierName]
		if !ok || len(models) == 0 {
			continue
		}
		levels = append(levels, strings.Join(models, ","))
	}
	return strings.Join(levels, "|")
}

// FormatTierConfig produces a human-readable display of the config.
func FormatTierConfig(tc *TierConfig, source string) string {
	if tc == nil {
		return "No YAML config found. Using OLLAMA_CHAIN env var.\n"
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "ACTIVE CONFIG SOURCE: %s\n\n", source)

	for _, tierName := range tierOrder {
		models, ok := tc.Tiers[tierName]
		if !ok || len(models) == 0 {
			continue
		}
		fmt.Fprintf(&sb, "%s TIER (%d models):\n", strings.ToUpper(tierName), len(models))
		for _, m := range models {
			fmt.Fprintf(&sb, "  - %s\n", m)
		}
		sb.WriteString("\n")
	}

	chain := tc.ToOllamaChain()
	if chain != "" {
		fmt.Fprintf(&sb, "EQUIVALENT OLLAMA_CHAIN:\n  %s\n", chain)
	}

	return sb.String()
}

// ValidateTierConfig checks a config for common issues.
// Returns a list of warning messages (empty = valid).
func ValidateTierConfig(tc *TierConfig) []string {
	if tc == nil {
		return nil
	}

	var warnings []string

	// Check for unknown tier names
	for name := range tc.Tiers {
		valid := false
		for _, t := range tierOrder {
			if name == t {
				valid = true
				break
			}
		}
		if !valid {
			warnings = append(warnings, fmt.Sprintf("unknown tier name %q (expected: cheap, mid, frontier)", name))
		}
	}

	// Check for empty tiers
	for _, tierName := range tierOrder {
		models, ok := tc.Tiers[tierName]
		if ok && len(models) == 0 {
			warnings = append(warnings, fmt.Sprintf("tier %q is defined but has no models", tierName))
		}
	}

	// Check for duplicate models across tiers
	seen := make(map[string]string) // model → tier
	for tierName, models := range tc.Tiers {
		for _, m := range models {
			if prevTier, exists := seen[m]; exists {
				warnings = append(warnings, fmt.Sprintf("model %q appears in both %q and %q tiers", m, prevTier, tierName))
			}
			seen[m] = tierName
		}
	}

	// Warn if tiers are very unbalanced
	for _, tierName := range tierOrder {
		if models, ok := tc.Tiers[tierName]; ok && len(models) > 5 {
			warnings = append(warnings, fmt.Sprintf("tier %q has %d models — consider splitting (recommended: 2-5 per tier)", tierName, len(models)))
		}
	}

	return warnings
}

// WriteTierConfig saves a tier config to ~/.synroute/config.yaml.
func WriteTierConfig(tc *TierConfig) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}

	configDir := filepath.Join(home, ".synroute")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := yaml.Marshal(tc)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	configPath := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	log.Printf("[Config] saved tier config to %s", configPath)
	return nil
}
