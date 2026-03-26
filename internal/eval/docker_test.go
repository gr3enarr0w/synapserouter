package eval

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScaffoldGo(t *testing.T) {
	dir := t.TempDir()

	ex := Exercise{
		Language: "go",
		Slug:     "hello-world",
		TestFile: "package greeting\n\nimport \"testing\"\n\nfunc TestHello(t *testing.T) {\n\tif Hello() != \"Hello, World!\" {\n\t\tt.Fatal(\"wrong\")\n\t}\n}",
	}

	code := "package greeting\n\nfunc Hello() string {\n\treturn \"Hello, World!\"\n}"
	if err := scaffoldGo(dir, ex, code); err != nil {
		t.Fatal(err)
	}

	// Check go.mod exists with detected package
	modData, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if !contains(string(modData), "module greeting") {
		t.Fatalf("unexpected go.mod: %s", modData)
	}

	// Check implementation file
	codeData, err := os.ReadFile(filepath.Join(dir, "hello_world.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !contains(string(codeData), "func Hello") {
		t.Fatalf("unexpected code file: %s", codeData)
	}

	// Check test file
	testData, err := os.ReadFile(filepath.Join(dir, "hello_world_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !contains(string(testData), "func TestHello") {
		t.Fatalf("unexpected test file: %s", testData)
	}
}

func TestScaffoldPython(t *testing.T) {
	dir := t.TempDir()

	ex := Exercise{
		Language: "python",
		Slug:     "two-fer",
		TestFile: "import pytest\ndef test_no_name():\n    pass",
	}

	code := "def two_fer(name='you'):\n    return f'One for {name}, one for me.'"
	if err := scaffoldPython(dir, ex, code); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, "two_fer.py")); err != nil {
		t.Fatal("expected two_fer.py")
	}
	if _, err := os.Stat(filepath.Join(dir, "two_fer_test.py")); err != nil {
		t.Fatal("expected two_fer_test.py")
	}
}

func TestScaffoldRust(t *testing.T) {
	dir := t.TempDir()

	ex := Exercise{
		Language: "rust",
		Slug:     "hello-world",
		TestFile: "#[test]\nfn test_hello() {}",
	}

	if err := scaffoldRust(dir, ex, "pub fn hello() -> &'static str { \"Hello, World!\" }"); err != nil {
		t.Fatal(err)
	}

	cargoData, err := os.ReadFile(filepath.Join(dir, "Cargo.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !contains(string(cargoData), "hello_world") {
		t.Fatalf("unexpected Cargo.toml: %s", cargoData)
	}

	if _, err := os.Stat(filepath.Join(dir, "src", "lib.rs")); err != nil {
		t.Fatal("expected src/lib.rs")
	}
}

func TestDetectGoPackage(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"package greeting\n\nimport \"testing\"", "greeting"},
		{"// comment\npackage main", "main"},
		{"no package here", ""},
	}
	for _, tt := range tests {
		got := detectGoPackage(tt.input)
		if got != tt.want {
			t.Errorf("detectGoPackage(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestExtractCodeFromResponse(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "with markdown fences",
			content: "Here's the code:\n```go\npackage main\nfunc Hello() {}\n```\n",
			want:    "package main\nfunc Hello() {}",
		},
		{
			name:    "without fences",
			content: "package main\nfunc Hello() {}",
			want:    "package main\nfunc Hello() {}",
		},
		{
			name:    "with language tag",
			content: "```python\ndef hello():\n    pass\n```",
			want:    "def hello():\n    pass",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// We test extractCode indirectly via the regex
			matches := codeBlockRegex.FindStringSubmatch(tt.content)
			if tt.name == "without fences" {
				if len(matches) > 1 {
					t.Fatal("should not match without fences")
				}
				return
			}
			if len(matches) < 2 {
				t.Fatal("expected match")
			}
			got := matches[1]
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
