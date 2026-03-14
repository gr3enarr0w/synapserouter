package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetActiveProfile_Default(t *testing.T) {
	t.Setenv("ACTIVE_PROFILE", "")
	profile := GetActiveProfile()
	if profile != "personal" {
		t.Errorf("expected personal, got %s", profile)
	}
}

func TestGetActiveProfile_Work(t *testing.T) {
	t.Setenv("ACTIVE_PROFILE", "work")
	profile := GetActiveProfile()
	if profile != "work" {
		t.Errorf("expected work, got %s", profile)
	}
}

func TestAvailableProfiles(t *testing.T) {
	profiles := AvailableProfiles()
	if len(profiles) != 2 {
		t.Fatalf("expected 2 profiles, got %d", len(profiles))
	}
	names := make(map[string]bool)
	for _, p := range profiles {
		names[p.Name] = true
	}
	if !names["personal"] || !names["work"] {
		t.Errorf("expected personal and work profiles, got %v", names)
	}
}

func TestShowProfile(t *testing.T) {
	t.Setenv("ACTIVE_PROFILE", "work")
	info := ShowProfile([]string{"vertex-claude", "vertex-gemini"})
	if info["active"] != "work" {
		t.Errorf("expected active=work, got %v", info["active"])
	}
	providers := info["providers"].([]string)
	if len(providers) != 2 {
		t.Errorf("expected 2 providers, got %d", len(providers))
	}
}

func TestSwitchProfile_Valid(t *testing.T) {
	// Create a temp .env
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	os.WriteFile(envPath, []byte("ACTIVE_PROFILE=personal\nPORT=8090\n"), 0644)

	// Override findEnvFile for testing by working directly with rewriteEnvVar
	err := rewriteEnvVar(envPath, "ACTIVE_PROFILE", "work")
	if err != nil {
		t.Fatal(err)
	}

	content, _ := os.ReadFile(envPath)
	if !strings.Contains(string(content), "ACTIVE_PROFILE=work") {
		t.Errorf("expected ACTIVE_PROFILE=work, got:\n%s", content)
	}
	// Ensure PORT wasn't lost
	if !strings.Contains(string(content), "PORT=8090") {
		t.Errorf("expected PORT=8090 preserved, got:\n%s", content)
	}
}

func TestSwitchProfile_InvalidProfile(t *testing.T) {
	err := SwitchProfile("invalid")
	if err == nil {
		t.Error("expected error for invalid profile")
	}
}

func TestRewriteEnvVar_AddNew(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	os.WriteFile(envPath, []byte("PORT=8090\n"), 0644)

	err := rewriteEnvVar(envPath, "ACTIVE_PROFILE", "work")
	if err != nil {
		t.Fatal(err)
	}

	content, _ := os.ReadFile(envPath)
	if !strings.Contains(string(content), "ACTIVE_PROFILE=work") {
		t.Errorf("expected ACTIVE_PROFILE=work added, got:\n%s", content)
	}
}
