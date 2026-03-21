package subscriptions

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultAnthropicBaseURL = "https://api.anthropic.com"
	defaultOpenAIBaseURL    = "https://api.openai.com/v1"
	defaultGeminiBaseURL    = "https://generativelanguage.googleapis.com/v1beta"
	defaultModelTimeout     = 120
)

type ProviderSpec struct {
	Name         string
	DisplayName  string
	BaseURL      string
	APIKey       string
	SessionToken string
	Credentials  []ProviderCredential
	Models       []string
}

type Config struct {
	ServerTimeout    time.Duration
	Providers        []ProviderSpec
	RouteModelOrder  []string
	EnableHealthLogs bool
}

func LoadConfig(ctx context.Context) (Config, error) {
	storedCredentials, err := LoadAllStoredCredentials()
	if err != nil {
		return Config{}, fmt.Errorf("failed to load subscription credential store: %w", err)
	}

	cfg := Config{
		ServerTimeout:    getenvDurationAny([]string{"SYNROUTE_SUBSCRIPTIONS_TIMEOUT_SECONDS", "SUBSCRIPTION_GATEWAY_TIMEOUT_SECONDS", "SYNAPSE_GATEWAY_TIMEOUT_SECONDS", "CLIPROXY_API_TIMEOUT_SECONDS"}, defaultModelTimeout) * time.Second,
		RouteModelOrder:  parseModelOrder(strings.TrimSpace(getenvAny([]string{"SYNROUTE_SUBSCRIPTION_PROVIDER_ORDER", "SUBSCRIPTION_GATEWAY_PROVIDER_ORDER", "SYNAPSE_GATEWAY_PROVIDER_ORDER", "CLIPROXY_PROVIDER_ORDER"}, "gemini,openai,anthropic"))),
		EnableHealthLogs: strings.EqualFold(strings.TrimSpace(getenvAny([]string{"SYNROUTE_SUBSCRIPTIONS_ENABLE_HEALTH_LOGS", "SUBSCRIPTION_GATEWAY_ENABLE_HEALTH_LOGS", "SYNAPSE_GATEWAY_ENABLE_HEALTH_LOGS", "CLIPROXY_ENABLE_HEALTH_LOGS"}, "")), "true"),
	}

	anthropic := ProviderSpec{
		Name:        "anthropic",
		DisplayName: "Claude",
		BaseURL:     getenvAny([]string{"SYNROUTE_ANTHROPIC_BASE_URL", "SUBSCRIPTION_GATEWAY_ANTHROPIC_BASE_URL", "SYNAPSE_GATEWAY_ANTHROPIC_BASE_URL", "CLIPROXY_ANTHROPIC_BASE_URL"}, defaultAnthropicBaseURL),
		APIKey:      strings.TrimSpace(getenvAny([]string{"SYNROUTE_ANTHROPIC_API_KEY", "SUBSCRIPTION_GATEWAY_ANTHROPIC_API_KEY", "SYNAPSE_GATEWAY_ANTHROPIC_API_KEY", "CLIPROXY_ANTHROPIC_API_KEY"}, "")),
		Models: []string{
			"claude-sonnet-4-5-20250929",
			"claude-3-7-sonnet-20250219",
			"claude-3-5-sonnet-latest",
		},
	}
	if anthToken := strings.TrimSpace(getenvAny([]string{"SYNROUTE_ANTHROPIC_SESSION_TOKEN", "SUBSCRIPTION_GATEWAY_ANTHROPIC_SESSION_TOKEN", "SYNAPSE_GATEWAY_ANTHROPIC_SESSION_TOKEN", "CLIPROXY_ANTHROPIC_SESSION_TOKEN"}, "")); anthToken != "" {
		anthropic.SessionToken = anthToken
	}
	anthropic.Credentials = collectCredentials(
		anthropic.APIKey,
		anthropic.SessionToken,
		splitCredentials(getenvAny([]string{"SYNROUTE_ANTHROPIC_API_KEYS", "SUBSCRIPTION_GATEWAY_ANTHROPIC_API_KEYS", "SYNAPSE_GATEWAY_ANTHROPIC_API_KEYS", "CLIPROXY_ANTHROPIC_API_KEYS"}, "")),
		splitCredentials(getenvAny([]string{"SYNROUTE_ANTHROPIC_SESSION_TOKENS", "SUBSCRIPTION_GATEWAY_ANTHROPIC_SESSION_TOKENS", "SYNAPSE_GATEWAY_ANTHROPIC_SESSION_TOKENS", "CLIPROXY_ANTHROPIC_SESSION_TOKENS"}, "")),
		storedCredentials["anthropic"],
	)

	openai := ProviderSpec{
		Name:        "openai",
		DisplayName: "OpenAI / Codex",
		BaseURL:     getenvAny([]string{"SYNROUTE_OPENAI_BASE_URL", "SUBSCRIPTION_GATEWAY_OPENAI_BASE_URL", "SYNAPSE_GATEWAY_OPENAI_BASE_URL", "CLIPROXY_OPENAI_BASE_URL"}, defaultOpenAIBaseURL),
		APIKey:      strings.TrimSpace(getenvAny([]string{"SYNROUTE_OPENAI_API_KEY", "SUBSCRIPTION_GATEWAY_OPENAI_API_KEY", "SYNAPSE_GATEWAY_OPENAI_API_KEY", "CLIPROXY_OPENAI_API_KEY"}, "")),
		Models: []string{
			"gpt-5.3-codex",
			"gpt-5.4",
			"gpt-5.3-codex-spark",
			"gpt-5.2-codex",
			"gpt-5.1-codex-max",
			"gpt-5.2",
			"gpt-5.1-codex-mini",
			"gpt-5-codex",
		},
	}
	if openToken := strings.TrimSpace(getenvAny([]string{"SYNROUTE_OPENAI_SESSION_TOKEN", "SUBSCRIPTION_GATEWAY_OPENAI_SESSION_TOKEN", "SYNAPSE_GATEWAY_OPENAI_SESSION_TOKEN", "CLIPROXY_OPENAI_SESSION_TOKEN"}, "")); openToken != "" {
		openai.SessionToken = openToken
	}
	openai.Credentials = collectCredentials(
		openai.APIKey,
		openai.SessionToken,
		splitCredentials(getenvAny([]string{"SYNROUTE_OPENAI_API_KEYS", "SUBSCRIPTION_GATEWAY_OPENAI_API_KEYS", "SYNAPSE_GATEWAY_OPENAI_API_KEYS", "CLIPROXY_OPENAI_API_KEYS"}, "")),
		splitCredentials(getenvAny([]string{"SYNROUTE_OPENAI_SESSION_TOKENS", "SUBSCRIPTION_GATEWAY_OPENAI_SESSION_TOKENS", "SYNAPSE_GATEWAY_OPENAI_SESSION_TOKENS", "CLIPROXY_OPENAI_SESSION_TOKENS"}, "")),
		storedCredentials["openai"],
	)

	gemini := ProviderSpec{
		Name:        "gemini",
		DisplayName: "Gemini",
		BaseURL:     getenvAny([]string{"SYNROUTE_GEMINI_BASE_URL", "SUBSCRIPTION_GATEWAY_GEMINI_BASE_URL", "SYNAPSE_GATEWAY_GEMINI_BASE_URL", "CLIPROXY_GEMINI_BASE_URL"}, defaultGeminiBaseURL),
		APIKey:      strings.TrimSpace(getenvAny([]string{"SYNROUTE_GEMINI_API_KEY", "SUBSCRIPTION_GATEWAY_GEMINI_API_KEY", "SYNAPSE_GATEWAY_GEMINI_API_KEY", "CLIPROXY_GEMINI_API_KEY"}, "")),
		Models: []string{
			"gemini-3.1-pro-preview",
			"gemini-3-flash-preview",
			"gemini-2.5-pro",
			"gemini-2.5-flash",
			"gemini-2.5-flash-lite",
			"gemini-1.5-pro",
			"gemini-1.5-flash",
		},
	}
	if gemToken := strings.TrimSpace(getenvAny([]string{"SYNROUTE_GEMINI_SESSION_TOKEN", "SUBSCRIPTION_GATEWAY_GEMINI_SESSION_TOKEN", "SYNAPSE_GATEWAY_GEMINI_SESSION_TOKEN", "CLIPROXY_GEMINI_SESSION_TOKEN"}, "")); gemToken != "" {
		gemini.SessionToken = gemToken
	}
	gemini.Credentials = collectCredentials(
		gemini.APIKey,
		gemini.SessionToken,
		splitCredentials(getenvAny([]string{"SYNROUTE_GEMINI_API_KEYS", "SUBSCRIPTION_GATEWAY_GEMINI_API_KEYS", "SYNAPSE_GATEWAY_GEMINI_API_KEYS", "CLIPROXY_GEMINI_API_KEYS"}, "")),
		splitCredentials(getenvAny([]string{"SYNROUTE_GEMINI_SESSION_TOKENS", "SUBSCRIPTION_GATEWAY_GEMINI_SESSION_TOKENS", "SYNAPSE_GATEWAY_GEMINI_SESSION_TOKENS", "CLIPROXY_GEMINI_SESSION_TOKENS"}, "")),
		storedCredentials["gemini"],
	)

	qwen := ProviderSpec{
		Name:        "qwen",
		DisplayName: "Qwen",
		BaseURL:     getenvAny([]string{"SYNROUTE_QWEN_BASE_URL"}, defaultOpenAIBaseURL),
		APIKey:      strings.TrimSpace(getenvAny([]string{"SYNROUTE_QWEN_API_KEY"}, "")),
		Models: []string{
			"qwen-max",
			"qwen-plus",
			"qwen-turbo",
		},
	}
	qwen.Credentials = collectCredentials(
		qwen.APIKey,
		qwen.SessionToken,
		splitCredentials(getenvAny([]string{"SYNROUTE_QWEN_API_KEYS"}, "")),
		splitCredentials(getenvAny([]string{"SYNROUTE_QWEN_SESSION_TOKENS"}, "")),
		nil,
	)

	if anthropic.isConfigured() {
		cfg.Providers = append(cfg.Providers, anthropic)
	}
	if openai.isConfigured() {
		cfg.Providers = append(cfg.Providers, openai)
	}
	if gemini.isConfigured() {
		cfg.Providers = append(cfg.Providers, gemini)
	}
	if qwen.isConfigured() {
		cfg.Providers = append(cfg.Providers, qwen)
	}

	if len(cfg.Providers) == 0 {
		return Config{}, fmt.Errorf("no upstream providers configured. Set at least one SYNROUTE_* provider credential")
	}

	// Ensure route order includes all providers so `auto` can resolve.
	cfg.RouteModelOrder = normalizeProviderOrder(cfg.RouteModelOrder, cfg.Providers)

	select {
	case <-ctx.Done():
		return Config{}, ctx.Err()
	default:
	}

	return cfg, nil
}

func (p ProviderSpec) isConfigured() bool {
	return len(p.Credentials) > 0 || p.APIKey != "" || p.SessionToken != ""
}

func getenvAny(keys []string, fallback string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return fallback
}

func getenvDuration(key string, fallback int64) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return time.Duration(fallback)
	}
	seconds, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || seconds <= 0 {
		return time.Duration(fallback)
	}
	return time.Duration(seconds)
}

func getenvDurationAny(keys []string, fallback int64) time.Duration {
	for _, key := range keys {
		raw := strings.TrimSpace(os.Getenv(key))
		if raw == "" {
			continue
		}
		seconds, err := strconv.ParseInt(raw, 10, 64)
		if err == nil && seconds > 0 {
			return time.Duration(seconds)
		}
	}
	return time.Duration(fallback)
}

func parseModelOrder(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item == "" {
			continue
		}
		out = append(out, strings.ToLower(item))
	}
	return out
}

func normalizeProviderOrder(order []string, providers []ProviderSpec) []string {
	added := make(map[string]struct{}, len(providers))
	for _, provider := range providers {
		added[provider.Name] = struct{}{}
	}

	for _, configured := range order {
		if _, ok := added[configured]; ok {
			delete(added, configured)
		}
	}

	ordered := make([]string, 0, len(providers))
	ordered = append(ordered, order...)
	for _, provider := range providers {
		if _, ok := added[provider.Name]; ok {
			ordered = append(ordered, provider.Name)
		}
	}
	return ordered
}

func collectCredentials(primaryKey, primarySession string, extraKeys, extraSessions []string, storedCredentials []ProviderCredential) []ProviderCredential {
	seen := make(map[string]struct{})
	out := make([]ProviderCredential, 0, 1+len(extraKeys)+len(extraSessions))

	appendCredential := func(apiKey, sessionToken string) {
		apiKey = strings.TrimSpace(apiKey)
		sessionToken = strings.TrimSpace(sessionToken)
		if apiKey == "" && sessionToken == "" {
			return
		}
		credential := ProviderCredential{
			APIKey:         apiKey,
			SessionToken:   sessionToken,
			CredentialType: resolveCredentialType(apiKey, sessionToken, credentialTypeUnknown),
		}
		key := credential.UniqueKey()
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, credential)
	}

	appendCredential(primaryKey, primarySession)
	for _, apiKey := range extraKeys {
		appendCredential(apiKey, "")
	}
	for _, sessionToken := range extraSessions {
		appendCredential("", sessionToken)
	}
	for _, credential := range storedCredentials {
		credential.APIKey = strings.TrimSpace(credential.APIKey)
		credential.SessionToken = strings.TrimSpace(credential.SessionToken)
		credential.AccessToken = strings.TrimSpace(credential.AccessToken)
		if credential.APIKey == "" && credential.SessionToken == "" && credential.AccessToken == "" {
			continue
		}
		key := credential.UniqueKey()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		credential.CredentialType = normalizeCredentialType(credential.CredentialType)
		out = append(out, credential)
	}
	return out
}

func splitCredentials(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n'
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item == "" {
			continue
		}
		out = append(out, item)
	}
	return out
}
