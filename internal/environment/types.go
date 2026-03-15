package environment

// ProjectEnv describes the detected project environment.
type ProjectEnv struct {
	Language     string            `json:"language"`      // "go", "python", "javascript", "rust", "java", "ruby", "cpp"
	Version      string            `json:"version"`       // detected or resolved version
	PackageFile  string            `json:"package_file"`  // go.mod, requirements.txt, etc.
	Dependencies []Dependency      `json:"dependencies,omitempty"`
	VenvPath     string            `json:"venv_path,omitempty"`
	RuntimePath  string            `json:"runtime_path,omitempty"`
	EnvVars      map[string]string `json:"env_vars,omitempty"`
}

// Dependency represents a project dependency with version constraints.
type Dependency struct {
	Name       string `json:"name"`
	Version    string `json:"version,omitempty"`
	MinRuntime string `json:"min_runtime,omitempty"`
	MaxRuntime string `json:"max_runtime,omitempty"`
}

// RuntimeSpec describes a language runtime's capabilities.
type RuntimeSpec struct {
	Language       string   `json:"language"`
	StableVersions []string `json:"stable_versions"`
	SetupCommand   string   `json:"setup_command"`
	InstallCommand string   `json:"install_command"`
	TestCommand    string   `json:"test_command"`
	BuildCommand   string   `json:"build_command"`
}

// BestPractice is a checkable best practice rule.
type BestPractice struct {
	Language    string `json:"language"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// BestPracticeResult is the outcome of checking a best practice.
type BestPracticeResult struct {
	Practice BestPractice `json:"practice"`
	Passed   bool         `json:"passed"`
	Message  string       `json:"message"`
	AutoFix  string       `json:"auto_fix,omitempty"` // command to fix, if available
}
