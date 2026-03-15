package environment

import (
	"os/exec"
	"strings"
)

// RuntimeSpecs contains known runtime specifications per language.
var RuntimeSpecs = map[string]RuntimeSpec{
	"go": {
		Language:       "go",
		StableVersions: []string{"1.26", "1.25", "1.24", "1.23", "1.22"},
		SetupCommand:   "",
		InstallCommand: "go mod download",
		TestCommand:    "go test -race ./...",
		BuildCommand:   "go build ./...",
	},
	"python": {
		Language:       "python",
		StableVersions: []string{"3.13", "3.12", "3.11", "3.10", "3.9"},
		SetupCommand:   "python3 -m venv .venv",
		InstallCommand: "pip install -r requirements.txt",
		TestCommand:    "python -m pytest",
		BuildCommand:   "",
	},
	"javascript": {
		Language:       "javascript",
		StableVersions: []string{"22", "20", "18"},
		SetupCommand:   "",
		InstallCommand: "npm install",
		TestCommand:    "npm test",
		BuildCommand:   "npm run build",
	},
	"rust": {
		Language:       "rust",
		StableVersions: []string{"1.83", "1.82", "1.81", "1.80"},
		SetupCommand:   "",
		InstallCommand: "cargo build",
		TestCommand:    "cargo test",
		BuildCommand:   "cargo build --release",
	},
	"java": {
		Language:       "java",
		StableVersions: []string{"21", "17", "11"},
		SetupCommand:   "",
		InstallCommand: "./gradlew build",
		TestCommand:    "./gradlew test",
		BuildCommand:   "./gradlew build",
	},
	"ruby": {
		Language:       "ruby",
		StableVersions: []string{"3.3", "3.2", "3.1"},
		SetupCommand:   "",
		InstallCommand: "bundle install",
		TestCommand:    "bundle exec rake test",
		BuildCommand:   "",
	},
}

// InstalledVersion detects the installed version of a language runtime.
func InstalledVersion(language string) string {
	switch language {
	case "go":
		return runVersion("go", "version")
	case "python":
		return runVersion("python3", "--version")
	case "javascript":
		return runVersion("node", "--version")
	case "rust":
		return runVersion("rustc", "--version")
	case "java":
		return runVersion("java", "--version")
	case "ruby":
		return runVersion("ruby", "--version")
	default:
		return ""
	}
}

func runVersion(cmd string, args ...string) string {
	out, err := exec.Command(cmd, args...).Output()
	if err != nil {
		return ""
	}
	return extractVersion(strings.TrimSpace(string(out)))
}

func extractVersion(output string) string {
	// Handle various version output formats:
	// "go version go1.22.0 darwin/arm64" → "1.22.0"
	// "Python 3.11.6" → "3.11.6"
	// "v20.10.0" → "20.10.0"
	// "rustc 1.75.0 (82e1608df 2023-12-21)" → "1.75.0"
	parts := strings.Fields(output)
	for _, part := range parts {
		part = strings.TrimPrefix(part, "v")
		part = strings.TrimPrefix(part, "go")
		// Check if it looks like a version number
		if len(part) > 0 && part[0] >= '0' && part[0] <= '9' && strings.Contains(part, ".") {
			return part
		}
	}
	return output
}

// ResolveVersion determines the best runtime version for a project
// based on its dependencies and available versions.
func ResolveVersion(env *ProjectEnv) VersionRecommendation {
	installed := InstalledVersion(env.Language)
	spec, ok := RuntimeSpecs[env.Language]
	if !ok {
		return VersionRecommendation{
			Current:     installed,
			Recommended: installed,
			Match:       true,
		}
	}

	// If project specifies a version, check compatibility
	if env.Version != "" {
		compatible := versionCompatible(installed, env.Version)
		return VersionRecommendation{
			Current:     installed,
			Required:    env.Version,
			Recommended: env.Version,
			Match:       compatible,
		}
	}

	// Default to latest stable
	recommended := installed
	if len(spec.StableVersions) > 0 {
		recommended = spec.StableVersions[0]
	}

	return VersionRecommendation{
		Current:     installed,
		Recommended: recommended,
		Match:       true,
	}
}

// VersionRecommendation describes the version resolution result.
type VersionRecommendation struct {
	Current     string `json:"current"`
	Required    string `json:"required,omitempty"`
	Recommended string `json:"recommended"`
	Match       bool   `json:"match"`
	Warning     string `json:"warning,omitempty"`
}

// versionCompatible does a simple prefix check.
func versionCompatible(installed, required string) bool {
	if installed == "" || required == "" {
		return true
	}
	// Strip patch for comparison: "1.22.3" compatible with "1.22"
	return strings.HasPrefix(installed, required) || strings.HasPrefix(required, majorMinor(installed))
}

func majorMinor(version string) string {
	parts := strings.SplitN(version, ".", 3)
	if len(parts) >= 2 {
		return parts[0] + "." + parts[1]
	}
	return version
}
