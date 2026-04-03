package subscriptions

import (
	"context"
	"log"
	"strings"

	"github.com/gr3enarr0w/synapserouter/internal/providers"
)

type runtimeProviderAdapter struct {
	name       string
	model      string
	maxContext int
	upstream   provider
}

func (p *runtimeProviderAdapter) Name() string {
	return p.name
}

func (p *runtimeProviderAdapter) ChatCompletion(ctx context.Context, req providers.ChatRequest, sessionID string) (providers.ChatResponse, error) {
	model := strings.TrimSpace(req.Model)
	// Allow empty or "auto" model to use default
	if model == "" || strings.EqualFold(model, "auto") {
		model = p.model
	} else if !p.upstream.SupportsModel(model) {
		// Instead of silently coercing, use the default model
		// Model validation should have already happened at the handler level
		model = p.model
	}
	return p.upstream.ChatCompletion(ctx, req, model, sessionID)
}

func (p *runtimeProviderAdapter) IsHealthy(ctx context.Context) bool {
	return p.upstream.IsHealthy(ctx)
}

func (p *runtimeProviderAdapter) MaxContextTokens() int {
	return p.maxContext
}

func (p *runtimeProviderAdapter) SupportsModel(model string) bool {
	return p.upstream.SupportsModel(model)
}

func LoadRuntimeProviders(ctx context.Context) ([]providers.Provider, error) {
	cfg, err := LoadConfig(ctx)
	if err != nil {
		if !hasAnyCredential() {
			return nil, nil
		}
		return nil, err
	}

	upstreams, err := buildProviders(cfg)
	if err != nil {
		return nil, err
	}

	byName := make(map[string]provider, len(upstreams))
	for _, upstream := range upstreams {
		byName[upstream.Name()] = upstream
	}

	ordered := make([]providers.Provider, 0, len(upstreams))
	for _, providerName := range cfg.RouteModelOrder {
		switch providerName {
		case "anthropic":
			// Removed: Claude Code subscription provider violates Anthropic TOS.
			// Using Claude Code's API as a proxy backend is not permitted.
			log.Printf("[Subscriptions] skipping anthropic provider — Claude Code subscription use not permitted by TOS")
		case "openai":
			if upstream, ok := byName[providerName]; ok {
				ordered = append(ordered, &runtimeProviderAdapter{
					name:       "codex",
					model:      preferredProviderModel("openai", upstream.ListModels(), "gpt-5.3-codex"),
					maxContext: 128000,
					upstream:   upstream,
				})
			}
		case "gemini":
			if upstream, ok := byName[providerName]; ok {
				ordered = append(ordered, &runtimeProviderAdapter{
					name:       "gemini",
					model:      preferredProviderModel("gemini", upstream.ListModels(), "gemini-3-flash-preview"),
					maxContext: 1048576,
					upstream:   upstream,
				})
			}
		case "qwen":
			if upstream, ok := byName[providerName]; ok {
				ordered = append(ordered, &runtimeProviderAdapter{
					name:       "qwen",
					model:      defaultModel(upstream.ListModels(), "qwen-max"),
					maxContext: 131072,
					upstream:   upstream,
				})
			}
		}
	}

	return ordered, nil
}

func hasAnyCredential() bool {
	keys := []string{
		"SYNROUTE_ANTHROPIC_API_KEY",
		"SYNROUTE_ANTHROPIC_API_KEYS",
		"SYNROUTE_ANTHROPIC_SESSION_TOKEN",
		"SYNROUTE_ANTHROPIC_SESSION_TOKENS",
		"SYNROUTE_OPENAI_API_KEY",
		"SYNROUTE_OPENAI_API_KEYS",
		"SYNROUTE_OPENAI_SESSION_TOKEN",
		"SYNROUTE_OPENAI_SESSION_TOKENS",
		"SYNROUTE_GEMINI_API_KEY",
		"SYNROUTE_GEMINI_API_KEYS",
		"SYNROUTE_GEMINI_SESSION_TOKEN",
		"SYNROUTE_GEMINI_SESSION_TOKENS",
		"SYNROUTE_QWEN_API_KEY",
		"SYNROUTE_QWEN_API_KEYS",
		"SYNROUTE_QWEN_SESSION_TOKEN",
		"SYNROUTE_QWEN_SESSION_TOKENS",
		"SUBSCRIPTION_GATEWAY_ANTHROPIC_API_KEY",
		"SUBSCRIPTION_GATEWAY_ANTHROPIC_API_KEYS",
		"SUBSCRIPTION_GATEWAY_ANTHROPIC_SESSION_TOKEN",
		"SUBSCRIPTION_GATEWAY_ANTHROPIC_SESSION_TOKENS",
		"SUBSCRIPTION_GATEWAY_OPENAI_API_KEY",
		"SUBSCRIPTION_GATEWAY_OPENAI_API_KEYS",
		"SUBSCRIPTION_GATEWAY_OPENAI_SESSION_TOKEN",
		"SUBSCRIPTION_GATEWAY_OPENAI_SESSION_TOKENS",
		"SUBSCRIPTION_GATEWAY_GEMINI_API_KEY",
		"SUBSCRIPTION_GATEWAY_GEMINI_API_KEYS",
		"SUBSCRIPTION_GATEWAY_GEMINI_SESSION_TOKEN",
		"SUBSCRIPTION_GATEWAY_GEMINI_SESSION_TOKENS",
		"SYNAPSE_GATEWAY_ANTHROPIC_API_KEY",
		"SYNAPSE_GATEWAY_ANTHROPIC_API_KEYS",
		"SYNAPSE_GATEWAY_ANTHROPIC_SESSION_TOKEN",
		"SYNAPSE_GATEWAY_ANTHROPIC_SESSION_TOKENS",
		"SYNAPSE_GATEWAY_OPENAI_API_KEY",
		"SYNAPSE_GATEWAY_OPENAI_API_KEYS",
		"SYNAPSE_GATEWAY_OPENAI_SESSION_TOKEN",
		"SYNAPSE_GATEWAY_OPENAI_SESSION_TOKENS",
		"SYNAPSE_GATEWAY_GEMINI_API_KEY",
		"SYNAPSE_GATEWAY_GEMINI_API_KEYS",
		"SYNAPSE_GATEWAY_GEMINI_SESSION_TOKEN",
		"SYNAPSE_GATEWAY_GEMINI_SESSION_TOKENS",
		"CLIPROXY_ANTHROPIC_API_KEY",
		"CLIPROXY_ANTHROPIC_API_KEYS",
		"CLIPROXY_ANTHROPIC_SESSION_TOKEN",
		"CLIPROXY_ANTHROPIC_SESSION_TOKENS",
		"CLIPROXY_OPENAI_API_KEY",
		"CLIPROXY_OPENAI_API_KEYS",
		"CLIPROXY_OPENAI_SESSION_TOKEN",
		"CLIPROXY_OPENAI_SESSION_TOKENS",
		"CLIPROXY_GEMINI_API_KEY",
		"CLIPROXY_GEMINI_API_KEYS",
		"CLIPROXY_GEMINI_SESSION_TOKEN",
		"CLIPROXY_GEMINI_SESSION_TOKENS",
	}

	for _, key := range keys {
		if strings.TrimSpace(getenvAny([]string{key}, "")) != "" {
			return true
		}
	}
	return HasAnyStoredCredentials()
}

func defaultModel(models []ModelInfo, fallback string) string {
	if len(models) == 0 {
		return fallback
	}
	return models[0].ID
}

func preferredModel(models []ModelInfo, preferred string) string {
	for _, model := range models {
		if model.ID == preferred {
			return preferred
		}
	}
	return defaultModel(models, preferred)
}

func preferredProviderModel(providerName string, models []ModelInfo, fallback string) string {
	// Provider-specific preferences come first, fallback last
	var preferences []string
	switch providerName {
	case "anthropic":
		preferences = []string{"claude-sonnet-4-5-20250929", "claude-3-5-sonnet-latest"}
	case "openai":
		preferences = []string{"gpt-5.3-codex", "gpt-5.4", "gpt-5.2-codex", "gpt-5.1-codex-max", "gpt-5-codex"}
	case "gemini":
		preferences = []string{"gemini-3-flash-preview", "gemini-3.1-pro-preview", "gemini-2.5-pro", "gemini-2.5-flash"}
	}
	preferences = append(preferences, fallback)
	for _, preferred := range preferences {
		if selected := preferredModel(models, preferred); selected != "" {
			return selected
		}
	}
	return defaultModel(models, fallback)
}
