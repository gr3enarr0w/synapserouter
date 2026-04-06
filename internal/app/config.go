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

// ProviderConfig defines a single provider backend (any type).
// All fields are optional except Name. Type defaults to "ollama" for backward compat.
type ProviderConfig struct {
	Name          string  `yaml:"name"`                     // unique provider name (user-defined)
	Type          string  `yaml:"type,omitempty"`           // ollama, openai_compat, gemini, vertex, anthropic
	BaseURL       string  `yaml:"base_url,omitempty"`       // API endpoint
	APIKeyEnv     string  `yaml:"api_key_env,omitempty"`    // env var name for API key
	Models        []string `yaml:"models,omitempty"`         // available models on this provider
	RateLimitRPM  float64 `yaml:"rate_limit_rpm,omitempty"` // requests per minute (0 = use default)
	MaxConcurrent int     `yaml:"max_concurrent,omitempty"` // max concurrent requests (0 = unlimited)
	Failover      string  `yaml:"failover,omitempty"`       // next provider name on failure, or "skip"
}

// PlannerConfig defines the planning phase provider setup.
type PlannerConfig struct {
	Providers []string `yaml:"providers"`          // provider names for parallel planning
	Merge     string   `yaml:"merge,omitempty"`    // provider name that merges plans (default: last planner)
}

// WorkerTierConfig defines a single worker tier.
type WorkerTierConfig struct {
	Name      string   `yaml:"name"`      // tier name (user-defined, e.g. "low", "mid", "high")
	Providers []string `yaml:"providers"` // provider names for this tier
}

// TierConfig holds user-configurable tier definitions from YAML.
// Supports both legacy format (tiers: map) and new provider-agnostic format.
type TierConfig struct {
	// Legacy format (backward compatible)
	Tiers                 map[string][]string                    `yaml:"tiers,omitempty"`                    // "cheap" → ["model1", "model2"]
	OpenAICompatProviders []providers.OpenAICompatProviderConfig `yaml:"openai_compat_providers,omitempty"`  // Custom OpenAI-compatible endpoints

	// New provider-agnostic format (v1.11+)
	Providers   []ProviderConfig   `yaml:"providers,omitempty"`    // all provider backends
	Planners    *PlannerConfig     `yaml:"planners,omitempty"`     // planning phase config
	WorkerTiers []WorkerTierConfig `yaml:"worker_tiers,omitempty"` // execution tier config
}

// tierOrder defines the escalation order (ascending capability).
// Used for legacy config format. New format uses WorkerTiers order directly.
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
// Supports both legacy format (tiers: map) and new format (worker_tiers: list).
func (tc *TierConfig) ToEscalationChain() []agent.EscalationLevel {
	if tc == nil {
		return nil
	}

	// New format: use WorkerTiers directly
	if tc.IsNewFormat() && len(tc.WorkerTiers) > 0 {
		return tc.WorkerTierEscalationChain()
	}

	// Legacy format: tiers map
	if len(tc.Tiers) == 0 {
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

// IsNewFormat returns true if the config uses the v1.11+ provider-agnostic format.
func (tc *TierConfig) IsNewFormat() bool {
	return tc != nil && len(tc.Providers) > 0
}

// AllModelNames returns all unique model names from the YAML config (both legacy and new format).
func (tc *TierConfig) AllModelNames() []string {
	if tc == nil {
		return nil
	}
	seen := make(map[string]bool)
	var models []string
	// Legacy format
	for _, tierModels := range tc.Tiers {
		for _, m := range tierModels {
			if !seen[m] {
				seen[m] = true
				models = append(models, m)
			}
		}
	}
	// New format
	for _, wt := range tc.WorkerTiers {
		for _, m := range wt.Providers {
			if !seen[m] {
				seen[m] = true
				models = append(models, m)
			}
		}
	}
	return models
}

// GetProvider returns a provider config by name, or nil if not found.
func (tc *TierConfig) GetProvider(name string) *ProviderConfig {
	if tc == nil {
		return nil
	}
	for i := range tc.Providers {
		if tc.Providers[i].Name == name {
			return &tc.Providers[i]
		}
	}
	return nil
}

// GetFailoverChain returns the ordered failover chain starting from a provider.
// Follows failover links until "skip" or a provider not found (cycle protection).
func (tc *TierConfig) GetFailoverChain(startProvider string) []string {
	if tc == nil {
		return nil
	}
	var chain []string
	seen := make(map[string]bool)
	current := startProvider
	for current != "" && current != "skip" && !seen[current] {
		seen[current] = true
		chain = append(chain, current)
		p := tc.GetProvider(current)
		if p == nil {
			break
		}
		current = p.Failover
	}
	return chain
}

// PlannerProviderNames returns the configured planner provider names.
// Falls back to the top tier of WorkerTiers if no explicit planners configured.
func (tc *TierConfig) PlannerProviderNames() []string {
	if tc != nil && tc.Planners != nil && len(tc.Planners.Providers) > 0 {
		return tc.Planners.Providers
	}
	// Fallback: use the highest worker tier providers
	if tc != nil && len(tc.WorkerTiers) > 0 {
		top := tc.WorkerTiers[len(tc.WorkerTiers)-1]
		return top.Providers
	}
	return nil
}

// MergeProviderName returns the configured merge provider for planning.
func (tc *TierConfig) MergeProviderName() string {
	if tc != nil && tc.Planners != nil && tc.Planners.Merge != "" {
		return tc.Planners.Merge
	}
	// Default: last planner
	planners := tc.PlannerProviderNames()
	if len(planners) > 0 {
		return planners[len(planners)-1]
	}
	return ""
}

// WorkerTierEscalationChain converts WorkerTiers to the agent's escalation chain.
func (tc *TierConfig) WorkerTierEscalationChain() []agent.EscalationLevel {
	if tc == nil || len(tc.WorkerTiers) == 0 {
		return nil
	}
	var chain []agent.EscalationLevel
	for i, wt := range tc.WorkerTiers {
		tier := agent.TierCheap
		if len(tc.WorkerTiers) >= 3 {
			switch {
			case i >= len(tc.WorkerTiers)*2/3:
				tier = agent.TierFrontier
			case i >= len(tc.WorkerTiers)/3:
				tier = agent.TierMid
			}
		} else if len(tc.WorkerTiers) == 2 {
			if i == 1 {
				tier = agent.TierFrontier
			}
		} else {
			tier = agent.TierFrontier // single tier = frontier
		}
		chain = append(chain, agent.EscalationLevel{
			Providers: wt.Providers,
			Tier:      tier,
		})
	}
	return chain
}

// RateLimitRPM returns the configured RPM for a provider, or 0 if not set.
func (tc *TierConfig) RateLimitRPM(providerName string) float64 {
	p := tc.GetProvider(providerName)
	if p != nil {
		return p.RateLimitRPM
	}
	return 0
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
