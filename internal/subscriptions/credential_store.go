package subscriptions

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type storedCredentialRecord struct {
	APIKey         string `json:"api_key,omitempty"`
	SessionToken   string `json:"session_token,omitempty"`
	AccessToken    string `json:"access_token,omitempty"`
	RefreshToken   string `json:"refresh_token,omitempty"`
	TokenType      string `json:"token_type,omitempty"`
	CredentialType string `json:"credential_type,omitempty"`
	ExpiresAt      string `json:"expires_at,omitempty"`
	Email          string `json:"email,omitempty"`
	ProjectID      string `json:"project_id,omitempty"`
	CreatedAt      string `json:"created_at,omitempty"`
	UpdatedAt      string `json:"updated_at,omitempty"`
}

type credentialStoreFile struct {
	Version   int                                 `json:"version"`
	UpdatedAt string                              `json:"updated_at"`
	Providers map[string][]storedCredentialRecord `json:"providers"`
}

func LoadAllStoredCredentials() (map[string][]ProviderCredential, error) {
	records, err := loadCredentialStore()
	if err != nil {
		return nil, err
	}

	providers := make(map[string][]ProviderCredential)
	for rawProvider, entries := range records.Providers {
		provider := normalizeStoredProviderName(rawProvider)
		if provider == "" {
			continue
		}
		out := make([]ProviderCredential, 0, len(entries))
		for _, entry := range entries {
			credential := ProviderCredential{
				APIKey:         strings.TrimSpace(entry.APIKey),
				SessionToken:   strings.TrimSpace(entry.SessionToken),
				AccessToken:    strings.TrimSpace(entry.AccessToken),
				RefreshToken:   strings.TrimSpace(entry.RefreshToken),
				TokenType:      strings.TrimSpace(entry.TokenType),
				CredentialType: normalizeCredentialType(entry.CredentialType),
				ExpiresAt:      strings.TrimSpace(entry.ExpiresAt),
				Email:          strings.TrimSpace(entry.Email),
				ProjectID:      strings.TrimSpace(entry.ProjectID),
			}
			if credential.APIKey == "" && credential.SessionToken == "" && credential.AccessToken == "" {
				continue
			}
			out = append(out, credential)
		}
		if len(out) > 0 {
			providers[provider] = out
		}
	}
	return providers, nil
}

func LoadStoredCredentials(provider string) ([]ProviderCredential, error) {
	all, err := LoadAllStoredCredentials()
	if err != nil {
		return nil, err
	}
	return all[normalizeStoredProviderName(provider)], nil
}

func HasAnyStoredCredentials() bool {
	all, err := LoadAllStoredCredentials()
	if err != nil {
		return false
	}
	for _, credentials := range all {
		if len(credentials) > 0 {
			return true
		}
	}
	return false
}

func HasStoredCredentialsForProvider(provider string) bool {
	stored, err := LoadStoredCredentials(provider)
	if err != nil {
		return false
	}
	return len(stored) > 0
}

func StoreCredential(provider string, credential ProviderCredential) error {
	provider = normalizeStoredProviderName(provider)
	if provider == "" {
		return fmt.Errorf("invalid provider")
	}
	normalizedType := normalizeCredentialType(credential.CredentialType)
	if normalizedType == credentialTypeUnknown {
		normalizedType = resolveCredentialType(credential.APIKey, credential.SessionToken, "")
	}
	credential.CredentialType = normalizedType
	credential.APIKey = strings.TrimSpace(credential.APIKey)
	credential.SessionToken = strings.TrimSpace(credential.SessionToken)
	credential.AccessToken = strings.TrimSpace(credential.AccessToken)
	credential.RefreshToken = strings.TrimSpace(credential.RefreshToken)

	if credential.APIKey == "" && credential.SessionToken == "" && credential.AccessToken == "" {
		return fmt.Errorf("credential is empty")
	}

	store, err := loadCredentialStore()
	if err != nil {
		store = credentialStoreFile{
			Version:   1,
			UpdatedAt: time.Now().UTC().Format(time.RFC3339),
			Providers: map[string][]storedCredentialRecord{},
		}
	}

	existing := make([]storedCredentialRecord, 0, len(store.Providers[provider])+1)
	existing = append(existing, store.Providers[provider]...)

	now := time.Now().UTC().Format(time.RFC3339)
	record := storedCredentialRecord{
		APIKey:         credential.APIKey,
		SessionToken:   credential.SessionToken,
		AccessToken:    credential.AccessToken,
		RefreshToken:   credential.RefreshToken,
		TokenType:      strings.TrimSpace(credential.TokenType),
		CredentialType: normalizeCredentialType(credential.CredentialType),
		ExpiresAt:      strings.TrimSpace(credential.ExpiresAt),
		Email:          strings.TrimSpace(credential.Email),
		ProjectID:      strings.TrimSpace(credential.ProjectID),
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	replaced := false
	for i, item := range existing {
		if !storedCredentialRecordMatches(item, record) {
			continue
		}
		record.CreatedAt = item.CreatedAt
		record.UpdatedAt = now
		existing[i] = record
		replaced = true
		break
	}
	if !replaced {
		existing = append(existing, record)
	}

	if store.Providers == nil {
		store.Providers = map[string][]storedCredentialRecord{}
	}
	store.Version = 1
	store.UpdatedAt = now
	store.Providers[provider] = existing
	return saveCredentialStore(store)
}

func storedCredentialRecordMatches(existing, record storedCredentialRecord) bool {
	if existing.APIKey == record.APIKey &&
		existing.SessionToken == record.SessionToken &&
		existing.AccessToken == record.AccessToken &&
		existing.CredentialType == record.CredentialType {
		return true
	}
	if record.RefreshToken != "" && existing.RefreshToken == record.RefreshToken {
		return true
	}
	if record.Email != "" &&
		existing.Email == record.Email &&
		existing.ProjectID == record.ProjectID &&
		existing.CredentialType == record.CredentialType {
		return true
	}
	if record.APIKey != "" &&
		existing.APIKey == record.APIKey &&
		existing.CredentialType == record.CredentialType {
		return true
	}
	if record.SessionToken != "" &&
		existing.SessionToken == record.SessionToken &&
		existing.CredentialType == record.CredentialType {
		return true
	}
	return false
}

func StorePath() string {
	return resolveCredentialStorePath()
}

func loadCredentialStore() (credentialStoreFile, error) {
	path := resolveCredentialStorePath()
	data, err := os.ReadFile(path)
	if err != nil {
		return credentialStoreFile{}, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return credentialStoreFile{
			Version:   1,
			UpdatedAt: time.Now().UTC().Format(time.RFC3339),
			Providers: map[string][]storedCredentialRecord{},
		}, nil
	}

	var store credentialStoreFile
	if err := json.Unmarshal(data, &store); err != nil {
		return credentialStoreFile{}, fmt.Errorf("invalid credential store json: %w", err)
	}

	if store.Version == 0 {
		store.Version = 1
	}
	if store.Providers == nil {
		store.Providers = map[string][]storedCredentialRecord{}
	}
	return store, nil
}

func saveCredentialStore(store credentialStoreFile) error {
	path := resolveCredentialStorePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create credential store directory: %w", err)
	}

	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return fmt.Errorf("serialize credential store: %w", err)
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write credential store: %w", err)
	}

	return nil
}

func resolveCredentialStorePath() string {
	custom := strings.TrimSpace(os.Getenv("SYNROUTE_SUBSCRIPTION_CREDENTIAL_STORE"))
	if custom != "" {
		return expandCredentialPath(custom)
	}
	return defaultCredentialStorePath()
}

func defaultCredentialStorePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".mcp", "synapse", "subscription-credentials.json")
	}
	return filepath.Join(home, ".mcp", "synapse", "subscription-credentials.json")
}

func expandCredentialPath(path string) string {
	path = strings.TrimSpace(path)
	if strings.HasPrefix(path, "~"+string(filepath.Separator)) {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	if path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return home
	}
	return path
}

func normalizeStoredProviderName(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "claude", "anthropic", "claude-code", "claudecode":
		return "anthropic"
	case "codex", "openai", "gpt", "openai-compatible":
		return "openai"
	case "gemini", "google", "gcp":
		return "gemini"
	case "qwen", "qwen-ai":
		return "qwen"
	default:
		return strings.TrimSpace(strings.ToLower(provider))
	}
}
