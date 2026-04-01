package environment

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// langConfigEntry pairs a language with its config file patterns.
// Ordered by specificity — more specific languages first.
type langConfigEntry struct {
	Language string
	Files    []string
}

// configFileOrder defines language detection priority. Order matters:
// more specific/less common config files first, generic ones last.
var configFileOrder = []langConfigEntry{
	{"rust", []string{"Cargo.toml"}},
	{"go", []string{"go.mod"}},
	{"java", []string{"pom.xml", "build.gradle", "build.gradle.kts"}},
	{"ruby", []string{"Gemfile"}},
	{"cpp", []string{"CMakeLists.txt"}},
	{"javascript", []string{"package.json"}},
	{"python", []string{"pyproject.toml", "requirements.txt", "setup.py", "Pipfile"}},
}

// configFiles kept for backward compatibility with DetectAll().
var configFiles = map[string][]string{
	"go":         {"go.mod"},
	"python":     {"pyproject.toml", "requirements.txt", "setup.py", "Pipfile"},
	"javascript": {"package.json"},
	"rust":       {"Cargo.toml"},
	"java":       {"pom.xml", "build.gradle", "build.gradle.kts"},
	"ruby":       {"Gemfile"},
	"cpp":        {"CMakeLists.txt"},
}

// Detect scans the working directory for project config files and returns
// a ProjectEnv describing the detected language and version.
// Uses deterministic priority order (not random map iteration).
// Also detects SQL projects by scanning for .sql files.
// Returns nil if no recognized project files are found.
func Detect(workDir string) *ProjectEnv {
	// Check config files in priority order (deterministic, not random)
	for _, entry := range configFileOrder {
		for _, file := range entry.Files {
			path := filepath.Join(workDir, file)
			if _, err := os.Stat(path); err == nil {
				env := &ProjectEnv{
					Language:    entry.Language,
					PackageFile: file,
					EnvVars:     make(map[string]string),
				}
				parseProjectFile(env, path)
				return env
			}
		}
	}

	// Fallback: detect SQL projects by scanning for .sql files
	if hasSQLFiles(workDir) {
		return &ProjectEnv{
			Language: "sql",
			EnvVars:  make(map[string]string),
		}
	}

	return nil
}

// hasSQLFiles checks if the directory is a SQL project.
// Requires both .sql files AND a project indicator to avoid false positives
// in directories like ~/ that happen to contain stray .sql files.
func hasSQLFiles(workDir string) bool {
	patterns := []string{
		filepath.Join(workDir, "*.sql"),
		filepath.Join(workDir, "sql", "*.sql"),
	}
	var found bool
	for _, pattern := range patterns {
		matches, _ := filepath.Glob(pattern)
		if len(matches) > 0 {
			found = true
			break
		}
	}
	if !found {
		return false
	}

	indicators := []string{
		".git",
		"migrations",
		"db",
		"Makefile",
		"README.md",
		"docker-compose.yml",
		"docker-compose.yaml",
	}
	for _, ind := range indicators {
		if _, err := os.Stat(filepath.Join(workDir, ind)); err == nil {
			return true
		}
	}
	return false
}

// DetectAll returns all detected project environments in the directory.
// A polyglot project may have both go.mod and package.json.
func DetectAll(workDir string) []*ProjectEnv {
	var envs []*ProjectEnv
	for lang, files := range configFiles {
		for _, file := range files {
			path := filepath.Join(workDir, file)
			if _, err := os.Stat(path); err == nil {
				env := &ProjectEnv{
					Language:    lang,
					PackageFile: file,
					EnvVars:     make(map[string]string),
				}
				parseProjectFile(env, path)
				envs = append(envs, env)
				break // one per language
			}
		}
	}
	return envs
}

func parseProjectFile(env *ProjectEnv, path string) {
	switch env.Language {
	case "go":
		parseGoMod(env, path)
	case "python":
		parsePythonProject(env, path)
	case "javascript":
		parsePackageJSON(env, path)
	case "rust":
		parseCargoToml(env, path)
	case "java":
		parseJavaProject(env, path)
	}
}

var goModVersionRe = regexp.MustCompile(`^go\s+([\d.]+)`)

// Compile once at package level for Java version parsing
var javaVersionRegex = regexp.MustCompile(`>(\d+)</`)
var sourceCompatRegex = regexp.MustCompile(`['"](\d+)['"]`)

func parseGoMod(env *ProjectEnv, path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if m := goModVersionRe.FindStringSubmatch(line); m != nil {
			env.Version = m[1]
		}
		if strings.HasPrefix(line, "require ") && !strings.Contains(line, "(") {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				env.Dependencies = append(env.Dependencies, Dependency{
					Name:    parts[1],
					Version: parts[2],
				})
			}
		}
	}
}

func parsePythonProject(env *ProjectEnv, path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	base := filepath.Base(path)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		switch base {
		case "requirements.txt":
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			dep := parseRequirementLine(line)
			if dep.Name != "" {
				env.Dependencies = append(env.Dependencies, dep)
			}
		case "pyproject.toml":
			if strings.HasPrefix(line, "requires-python") {
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 {
					env.Version = strings.Trim(strings.TrimSpace(parts[1]), `"'>=<`)
				}
			}
		}
	}

	// Check for venv
	for _, venvDir := range []string{".venv", "venv", "env"} {
		venvPath := filepath.Join(filepath.Dir(path), venvDir)
		if info, err := os.Stat(venvPath); err == nil && info.IsDir() {
			env.VenvPath = venvPath
		}
	}
}

func parseRequirementLine(line string) Dependency {
	// Handle: package==1.0, package>=1.0, package~=1.0, package[extra]>=1.0
	line = strings.Split(line, ";")[0] // strip markers
	line = strings.TrimSpace(line)

	// Split on version specifier
	for _, sep := range []string{"==", ">=", "<=", "~=", "!="} {
		if idx := strings.Index(line, sep); idx > 0 {
			name := strings.TrimSpace(line[:idx])
			version := strings.TrimSpace(line[idx+len(sep):])
			name = strings.Split(name, "[")[0] // strip extras
			return Dependency{Name: name, Version: version}
		}
	}

	name := strings.Split(line, "[")[0]
	return Dependency{Name: strings.TrimSpace(name)}
}

func parsePackageJSON(env *ProjectEnv, path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	content := string(data)

	// Extract engines.node version
	if idx := strings.Index(content, `"node"`); idx > 0 {
		// Simple extraction — look for the version string after "node"
		sub := content[idx:]
		if start := strings.Index(sub, `"`); start > 0 {
			sub = sub[start+1:]
			if end := strings.Index(sub, `"`); end > 0 {
				sub = sub[end+1:]
				if start2 := strings.Index(sub, `"`); start2 >= 0 {
					sub = sub[start2+1:]
					if end2 := strings.Index(sub, `"`); end2 > 0 {
						env.Version = sub[:end2]
					}
				}
			}
		}
	}

	// Check for lockfile
	dir := filepath.Dir(path)
	for _, lock := range []string{"package-lock.json", "yarn.lock", "pnpm-lock.yaml"} {
		if _, err := os.Stat(filepath.Join(dir, lock)); err == nil {
			return
		}
	}
}

func parseCargoToml(env *ProjectEnv, path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "edition") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				env.Version = strings.Trim(strings.TrimSpace(parts[1]), `"'`)
			}
		}
	}
}

func parseJavaProject(env *ProjectEnv, path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Maven: <java.version>17</java.version> or <maven.compiler.source>17</maven.compiler.source>
		if strings.Contains(line, "java.version") || strings.Contains(line, "maven.compiler.source") {
			if m := javaVersionRegex.FindStringSubmatch(line); m != nil {
				env.Version = m[1]
			}
		}
		// Gradle: sourceCompatibility = '17'
		if strings.Contains(line, "sourceCompatibility") {
			if m := sourceCompatRegex.FindStringSubmatch(line); m != nil {
				env.Version = m[1]
			}
		}
	}
}
