package app

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ProfileInfo describes a synapserouter profile.
type ProfileInfo struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Active      bool     `json:"active"`
	Providers   []string `json:"providers,omitempty"`
}

// AvailableProfiles returns the list of known profiles.
func AvailableProfiles() []ProfileInfo {
	return []ProfileInfo{
		{Name: "personal", Description: "OAuth subscription providers (Claude Code, Codex, Gemini CLI)"},
		{Name: "work", Description: "Vertex AI (Claude via gcloud, Gemini via service account)"},
	}
}

// GetActiveProfile reads ACTIVE_PROFILE from .env (or environment).
func GetActiveProfile() string {
	profile := strings.ToLower(strings.TrimSpace(os.Getenv("ACTIVE_PROFILE")))
	if profile == "" {
		profile = "personal"
	}
	return profile
}

// ShowProfile returns the current profile info with active providers.
func ShowProfile(providerNames []string) map[string]interface{} {
	active := GetActiveProfile()
	profiles := AvailableProfiles()
	for i := range profiles {
		profiles[i].Active = profiles[i].Name == active
	}

	return map[string]interface{}{
		"active":    active,
		"providers": providerNames,
		"available": profiles,
	}
}

// SwitchProfile atomically rewrites ACTIVE_PROFILE in .env.
func SwitchProfile(newProfile string) error {
	newProfile = strings.ToLower(strings.TrimSpace(newProfile))
	valid := false
	for _, p := range AvailableProfiles() {
		if p.Name == newProfile {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("unknown profile %q (valid: personal, work)", newProfile)
	}

	envPath := findEnvFile()
	if envPath == "" {
		return fmt.Errorf(".env file not found")
	}

	return rewriteEnvVar(envPath, "ACTIVE_PROFILE", newProfile)
}

// findEnvFile locates the .env file.
func findEnvFile() string {
	candidates := []string{".env"}
	home, _ := os.UserHomeDir()
	candidates = append(candidates, filepath.Join(home, ".mcp", "synapse", ".env"))

	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

// rewriteEnvVar atomically updates a key in a .env file.
func rewriteEnvVar(path, key, value string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}

	var lines []string
	found := false
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, key+"=") {
			lines = append(lines, key+"="+value)
			found = true
		} else {
			lines = append(lines, line)
		}
	}
	f.Close()

	if err := scanner.Err(); err != nil {
		return err
	}
	if !found {
		lines = append(lines, key+"="+value)
	}

	// Write to temp, then rename
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(strings.Join(lines, "\n")+"\n"), 0644); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
