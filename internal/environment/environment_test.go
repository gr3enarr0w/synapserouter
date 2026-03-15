package environment

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectGo(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte(`module example.com/test

go 1.22

require github.com/gorilla/mux v1.8.1
`), 0644)

	env := Detect(dir)
	if env == nil {
		t.Fatal("expected Go project detection")
	}
	if env.Language != "go" {
		t.Errorf("language = %q, want go", env.Language)
	}
	if env.Version != "1.22" {
		t.Errorf("version = %q, want 1.22", env.Version)
	}
	if env.PackageFile != "go.mod" {
		t.Errorf("package file = %q, want go.mod", env.PackageFile)
	}
}

func TestDetectPython(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "requirements.txt"), []byte(`
tensorflow>=2.15
numpy==1.26.0
requests>=2.28
# comment line
`), 0644)

	env := Detect(dir)
	if env == nil {
		t.Fatal("expected Python project detection")
	}
	if env.Language != "python" {
		t.Errorf("language = %q, want python", env.Language)
	}
	if len(env.Dependencies) != 3 {
		t.Errorf("dependencies = %d, want 3", len(env.Dependencies))
	}

	// Check tensorflow dep
	found := false
	for _, dep := range env.Dependencies {
		if dep.Name == "tensorflow" {
			found = true
			if dep.Version != "2.15" {
				t.Errorf("tensorflow version = %q, want 2.15", dep.Version)
			}
		}
	}
	if !found {
		t.Error("expected tensorflow in dependencies")
	}
}

func TestDetectPythonWithVenv(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "requirements.txt"), []byte("flask\n"), 0644)
	os.Mkdir(filepath.Join(dir, ".venv"), 0755)

	env := Detect(dir)
	if env == nil {
		t.Fatal("expected Python project detection")
	}
	if env.VenvPath == "" {
		t.Error("expected venv path to be detected")
	}
}

func TestDetectJavaScript(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{
  "name": "test",
  "engines": {
    "node": ">=18"
  }
}`), 0644)

	env := Detect(dir)
	if env == nil {
		t.Fatal("expected JS project detection")
	}
	if env.Language != "javascript" {
		t.Errorf("language = %q, want javascript", env.Language)
	}
}

func TestDetectRust(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte(`[package]
name = "test"
edition = "2021"
`), 0644)

	env := Detect(dir)
	if env == nil {
		t.Fatal("expected Rust project detection")
	}
	if env.Language != "rust" {
		t.Errorf("language = %q, want rust", env.Language)
	}
	if env.Version != "2021" {
		t.Errorf("version = %q, want 2021", env.Version)
	}
}

func TestDetectJava(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "pom.xml"), []byte(`<project>
  <properties>
    <java.version>17</java.version>
  </properties>
</project>`), 0644)

	env := Detect(dir)
	if env == nil {
		t.Fatal("expected Java project detection")
	}
	if env.Language != "java" {
		t.Errorf("language = %q, want java", env.Language)
	}
	if env.Version != "17" {
		t.Errorf("version = %q, want 17", env.Version)
	}
}

func TestDetectNothing(t *testing.T) {
	dir := t.TempDir()
	env := Detect(dir)
	if env != nil {
		t.Errorf("expected nil for empty dir, got %+v", env)
	}
}

func TestDetectAll(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\ngo 1.22\n"), 0644)
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"test"}`), 0644)

	envs := DetectAll(dir)
	if len(envs) < 2 {
		t.Errorf("expected at least 2 environments, got %d", len(envs))
	}

	langs := map[string]bool{}
	for _, e := range envs {
		langs[e.Language] = true
	}
	if !langs["go"] {
		t.Error("expected Go detection")
	}
	if !langs["javascript"] {
		t.Error("expected JavaScript detection")
	}
}

func TestCheckBestPracticesGo(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\ngo 1.22\n"), 0644)
	os.WriteFile(filepath.Join(dir, "go.sum"), []byte(""), 0644)
	os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*.exe\n"), 0644)

	env := Detect(dir)
	results := CheckBestPractices(env, dir)

	allPassed := true
	for _, r := range results {
		if !r.Passed {
			allPassed = false
			t.Logf("FAIL: %s - %s", r.Practice.Name, r.Message)
		}
	}
	if !allPassed {
		t.Error("expected all Go best practices to pass")
	}
}

func TestCheckBestPracticesPython(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "requirements.txt"), []byte("flask\n"), 0644)

	env := Detect(dir)
	results := CheckBestPractices(env, dir)

	// Should flag missing venv
	venvFound := false
	for _, r := range results {
		if r.Practice.Name == "virtual_env" {
			venvFound = true
			if r.Passed {
				t.Error("should flag missing venv")
			}
			if r.AutoFix == "" {
				t.Error("should have autofix for venv")
			}
		}
	}
	if !venvFound {
		t.Error("expected virtual_env check")
	}
}

func TestWrapCommandPython(t *testing.T) {
	env := &ProjectEnv{
		Language: "python",
		VenvPath: "/project/.venv",
	}

	wrapped := WrapCommand(env, "pytest")
	if wrapped != "source /project/.venv/bin/activate && pytest" {
		t.Errorf("wrapped = %q", wrapped)
	}

	// Don't double-wrap
	wrapped2 := WrapCommand(env, "source .venv/bin/activate && pytest")
	if wrapped2 != "source .venv/bin/activate && pytest" {
		t.Errorf("double-wrapped = %q", wrapped2)
	}
}

func TestWrapCommandNoVenv(t *testing.T) {
	env := &ProjectEnv{Language: "python"}
	if WrapCommand(env, "pytest") != "pytest" {
		t.Error("should not wrap when no venv")
	}
}

func TestWrapCommandNil(t *testing.T) {
	if WrapCommand(nil, "ls") != "ls" {
		t.Error("should pass through with nil env")
	}
}

func TestSetupCommands(t *testing.T) {
	env := &ProjectEnv{
		Language:    "python",
		PackageFile: "requirements.txt",
	}
	cmds := SetupCommands(env)
	if len(cmds) == 0 {
		t.Error("expected setup commands for Python")
	}
}

func TestSummary(t *testing.T) {
	env := &ProjectEnv{
		Language:    "go",
		Version:     "1.22",
		PackageFile: "go.mod",
	}
	s := Summary(env)
	if s == "" {
		t.Error("summary should not be empty")
	}
	if Summary(nil) == "" {
		t.Error("nil summary should return message")
	}
}

func TestParseRequirementLine(t *testing.T) {
	tests := []struct {
		line     string
		wantName string
		wantVer  string
	}{
		{"flask==2.0", "flask", "2.0"},
		{"tensorflow>=2.15", "tensorflow", "2.15"},
		{"numpy~=1.26.0", "numpy", "1.26.0"},
		{"requests", "requests", ""},
		{"package[extra]>=1.0", "package", "1.0"},
		{"dep>=1.0; python_version>'3.6'", "dep", "1.0"},
	}

	for _, tc := range tests {
		dep := parseRequirementLine(tc.line)
		if dep.Name != tc.wantName {
			t.Errorf("parseRequirementLine(%q).Name = %q, want %q", tc.line, dep.Name, tc.wantName)
		}
		if dep.Version != tc.wantVer {
			t.Errorf("parseRequirementLine(%q).Version = %q, want %q", tc.line, dep.Version, tc.wantVer)
		}
	}
}

func TestVersionCompatible(t *testing.T) {
	tests := []struct {
		installed string
		required  string
		want      bool
	}{
		{"1.22.3", "1.22", true},
		{"3.11.6", "3.11", true},
		{"3.12.0", "3.11", false},
		{"", "1.22", true},
		{"1.22", "", true},
	}

	for _, tc := range tests {
		got := versionCompatible(tc.installed, tc.required)
		if got != tc.want {
			t.Errorf("versionCompatible(%q, %q) = %v, want %v", tc.installed, tc.required, got, tc.want)
		}
	}
}

func TestExtractVersion(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"go version go1.22.0 darwin/arm64", "1.22.0"},
		{"Python 3.11.6", "3.11.6"},
		{"v20.10.0", "20.10.0"},
		{"rustc 1.75.0 (82e1608df 2023-12-21)", "1.75.0"},
	}

	for _, tc := range tests {
		got := extractVersion(tc.input)
		if got != tc.want {
			t.Errorf("extractVersion(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestKnownPythonConstraint(t *testing.T) {
	if knownPythonConstraint("tensorflow") == "" {
		t.Error("should return constraint for tensorflow")
	}
	if knownPythonConstraint("unknown-package") != "" {
		t.Error("should return empty for unknown package")
	}
}

func TestTestCommand(t *testing.T) {
	env := &ProjectEnv{Language: "go"}
	if TestCommand(env) != "go test -race ./..." {
		t.Errorf("go test command = %q", TestCommand(env))
	}

	if TestCommand(nil) != "" {
		t.Error("nil env should return empty")
	}
}

func TestBuildCommand(t *testing.T) {
	env := &ProjectEnv{Language: "go"}
	if BuildCommand(env) != "go build ./..." {
		t.Errorf("go build command = %q", BuildCommand(env))
	}
}
