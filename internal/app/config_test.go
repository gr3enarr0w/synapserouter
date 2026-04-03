package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gr3enarr0w/synapserouter/internal/agent"
)

func TestLoadTierConfig_YAML(t *testing.T) {
	// Create a temp YAML file in CWD
	yaml := `tiers:
  cheap:
    - model-a
    - model-b
  mid:
    - model-c
  frontier:
    - model-d
    - model-e
`
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	os.WriteFile(".synroute.yaml", []byte(yaml), 0644)

	tc, err := LoadTierConfig()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if tc == nil {
		t.Fatal("expected non-nil config")
	}
	if len(tc.Tiers["cheap"]) != 2 {
		t.Errorf("cheap tier: expected 2 models, got %d", len(tc.Tiers["cheap"]))
	}
	if len(tc.Tiers["frontier"]) != 2 {
		t.Errorf("frontier tier: expected 2 models, got %d", len(tc.Tiers["frontier"]))
	}
}

func TestLoadTierConfig_Missing(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)
	t.Setenv("HOME", tmpDir)

	tc, err := LoadTierConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tc != nil {
		t.Error("expected nil when no config file exists")
	}
}

func TestTierConfig_ToEscalationChain(t *testing.T) {
	tc := &TierConfig{
		Tiers: map[string][]string{
			"cheap":    {"model-a", "model-b"},
			"mid":      {"model-c"},
			"frontier": {"model-d"},
		},
	}

	chain := tc.ToEscalationChain()
	if len(chain) != 3 {
		t.Fatalf("expected 3 levels, got %d", len(chain))
	}

	// Order should be cheap → mid → frontier
	if chain[0].Tier != agent.TierCheap {
		t.Errorf("level 0 should be cheap, got %s", chain[0].Tier)
	}
	if chain[1].Tier != agent.TierMid {
		t.Errorf("level 1 should be mid, got %s", chain[1].Tier)
	}
	if chain[2].Tier != agent.TierFrontier {
		t.Errorf("level 2 should be frontier, got %s", chain[2].Tier)
	}

	if len(chain[0].Providers) != 2 {
		t.Errorf("cheap tier should have 2 providers, got %d", len(chain[0].Providers))
	}
}

func TestTierConfig_ToEscalationChain_MissingTier(t *testing.T) {
	tc := &TierConfig{
		Tiers: map[string][]string{
			"cheap":    {"model-a"},
			"frontier": {"model-b"},
		},
	}

	chain := tc.ToEscalationChain()
	if len(chain) != 2 {
		t.Fatalf("expected 2 levels (mid skipped), got %d", len(chain))
	}
}

func TestTierConfig_ToOllamaChain(t *testing.T) {
	tc := &TierConfig{
		Tiers: map[string][]string{
			"cheap":    {"model-a", "model-b"},
			"mid":      {"model-c"},
			"frontier": {"model-d"},
		},
	}

	chain := tc.ToOllamaChain()
	if chain != "model-a,model-b|model-c|model-d" {
		t.Errorf("chain = %q", chain)
	}
}

func TestValidateTierConfig_Valid(t *testing.T) {
	tc := &TierConfig{
		Tiers: map[string][]string{
			"cheap": {"a", "b"},
			"mid":   {"c"},
		},
	}
	warnings := ValidateTierConfig(tc)
	if len(warnings) != 0 {
		t.Errorf("expected no warnings, got: %v", warnings)
	}
}

func TestValidateTierConfig_UnknownTier(t *testing.T) {
	tc := &TierConfig{
		Tiers: map[string][]string{
			"cheap":  {"a"},
			"turbo":  {"b"}, // unknown
		},
	}
	warnings := ValidateTierConfig(tc)
	found := false
	for _, w := range warnings {
		if strings.Contains(w, "unknown tier") {
			found = true
		}
	}
	if !found {
		t.Error("expected warning about unknown tier name")
	}
}

func TestValidateTierConfig_DuplicateModels(t *testing.T) {
	tc := &TierConfig{
		Tiers: map[string][]string{
			"cheap": {"model-a"},
			"mid":   {"model-a"}, // duplicate
		},
	}
	warnings := ValidateTierConfig(tc)
	found := false
	for _, w := range warnings {
		if strings.Contains(w, "appears in both") {
			found = true
		}
	}
	if !found {
		t.Error("expected warning about duplicate model")
	}
}

func TestWriteAndLoadTierConfig(t *testing.T) {
	// Use temp home dir
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	tc := &TierConfig{
		Tiers: map[string][]string{
			"cheap":    {"model-a"},
			"frontier": {"model-b"},
		},
	}

	if err := WriteTierConfig(tc); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Verify file exists
	configPath := filepath.Join(tmpHome, ".synroute", "config.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("config file should exist after write")
	}
}

func TestFormatTierConfig(t *testing.T) {
	tc := &TierConfig{
		Tiers: map[string][]string{
			"cheap":    {"model-a"},
			"frontier": {"model-b"},
		},
	}

	output := FormatTierConfig(tc, "~/.synroute/config.yaml")
	if !strings.Contains(output, "CHEAP TIER") {
		t.Error("should contain CHEAP TIER")
	}
	if !strings.Contains(output, "FRONTIER TIER") {
		t.Error("should contain FRONTIER TIER")
	}
	if !strings.Contains(output, "EQUIVALENT OLLAMA_CHAIN") {
		t.Error("should contain equivalent chain")
	}
}

func TestFormatTierConfig_Nil(t *testing.T) {
	output := FormatTierConfig(nil, "")
	if !strings.Contains(output, "No YAML config") {
		t.Error("nil config should say no YAML config found")
	}
}
