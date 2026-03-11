package compat

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

const ampConfigSettingKey = "ampcode.config"

type AmpUpstreamAPIKeyEntry struct {
	Name           string   `json:"name,omitempty"`
	UpstreamAPIKey string   `json:"upstream-api-key"`
	APIKeys        []string `json:"api-keys,omitempty"`
}

type AmpModelMapping struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type AmpCodeConfig struct {
	UpstreamURL                   string                   `json:"upstream-url,omitempty"`
	UpstreamAPIKey                string                   `json:"upstream-api-key,omitempty"`
	UpstreamAPIKeys               []AmpUpstreamAPIKeyEntry `json:"upstream-api-keys,omitempty"`
	RestrictManagementToLocalhost bool                     `json:"restrict-management-to-localhost"`
	ModelMappings                 []AmpModelMapping        `json:"model-mappings,omitempty"`
	ForceModelMappings            bool                     `json:"force-model-mappings"`
}

func DefaultAmpCodeConfig() AmpCodeConfig {
	return AmpCodeConfig{
		ForceModelMappings: true,
	}
}

func ResolveAmpSecret(cfg AmpCodeConfig) string {
	if key := strings.TrimSpace(cfg.UpstreamAPIKey); key != "" {
		return key
	}
	if key := strings.TrimSpace(os.Getenv("AMP_API_KEY")); key != "" {
		return key
	}
	if key := strings.TrimSpace(os.Getenv("AMP_UPSTREAM_API_KEY")); key != "" {
		return key
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return ""
	}
	raw, err := os.ReadFile(filepath.Join(home, ".local", "share", "amp", "secrets.json"))
	if err != nil {
		return ""
	}
	var secrets map[string]string
	if err := json.Unmarshal(raw, &secrets); err != nil {
		return ""
	}
	return strings.TrimSpace(secrets["apiKey@https://ampcode.com/"])
}

func LoadAmpCodeConfig(db *sql.DB) (AmpCodeConfig, error) {
	if db == nil {
		return DefaultAmpCodeConfig(), nil
	}
	var raw string
	err := db.QueryRow(`SELECT value FROM runtime_settings WHERE key = ?`, ampConfigSettingKey).Scan(&raw)
	if err == sql.ErrNoRows {
		return DefaultAmpCodeConfig(), nil
	}
	if err != nil {
		return DefaultAmpCodeConfig(), err
	}
	if strings.TrimSpace(raw) == "" {
		return DefaultAmpCodeConfig(), nil
	}
	cfg := DefaultAmpCodeConfig()
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return DefaultAmpCodeConfig(), err
	}
	return cfg, nil
}

func SaveAmpCodeConfig(db *sql.DB, cfg AmpCodeConfig) error {
	if db == nil {
		return nil
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	_, err = db.Exec(`
		INSERT INTO runtime_settings (key, value, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET
			value = excluded.value,
			updated_at = CURRENT_TIMESTAMP
	`, ampConfigSettingKey, string(raw))
	return err
}

func ApplyModelMapping(cfg AmpCodeConfig, model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return model
	}
	for _, mapping := range cfg.ModelMappings {
		from := strings.TrimSpace(mapping.From)
		to := strings.TrimSpace(mapping.To)
		if from == "" || to == "" {
			continue
		}
		if strings.EqualFold(from, model) {
			return to
		}
		if strings.HasPrefix(from, "*") {
			suffix := strings.TrimPrefix(strings.ToLower(from), "*")
			if suffix != "" && strings.HasSuffix(strings.ToLower(model), suffix) {
				return to
			}
		}
	}
	return model
}

func ResolveModel(cfg AmpCodeConfig, model string, availableModels []string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return model
	}

	mapped := ApplyModelMapping(cfg, model)
	if mapped == model {
		return model
	}
	if cfg.ForceModelMappings {
		return mapped
	}
	for _, availableModel := range availableModels {
		if strings.EqualFold(strings.TrimSpace(availableModel), model) {
			return model
		}
	}
	return mapped
}
