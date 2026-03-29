package agent

import (
	"os"
	"strings"
	"testing"
)

func TestDefaultColorsEnabled(t *testing.T) {
	os.Unsetenv("NO_COLOR")
	os.Unsetenv("TERM")
	colors := DefaultColors()
	if !colors.IsEnabled() {
		t.Fatal("colors should be enabled without NO_COLOR")
	}
	if colors.Reset != "\033[0m" {
		t.Fatal("expected reset code")
	}
}

func TestDefaultColorsDisabledByNOCOLOR(t *testing.T) {
	os.Setenv("NO_COLOR", "1")
	defer os.Unsetenv("NO_COLOR")

	colors := DefaultColors()
	if colors.IsEnabled() {
		t.Fatal("colors should be disabled with NO_COLOR")
	}
	if colors.Reset != "" {
		t.Fatal("reset should be empty")
	}
}

func TestDefaultColorsDisabledByDumbTerm(t *testing.T) {
	os.Setenv("TERM", "dumb")
	defer os.Unsetenv("TERM")

	colors := DefaultColors()
	if colors.IsEnabled() {
		t.Fatal("colors should be disabled with TERM=dumb")
	}
}

func TestNoColors(t *testing.T) {
	colors := NoColors()
	if colors.IsEnabled() {
		t.Fatal("NoColors should not be enabled")
	}
}

func TestColorWrap(t *testing.T) {
	colors := SemanticColor{
		ToolName: "\033[36m",
		Reset:    "\033[0m",
	}

	result := colors.Wrap(colors.ToolName, "bash")
	if !strings.Contains(result, "\033[36m") {
		t.Fatal("expected color code")
	}
	if !strings.HasSuffix(result, "\033[0m") {
		t.Fatal("expected reset suffix")
	}
}

func TestColorWrapEmpty(t *testing.T) {
	colors := NoColors()
	result := colors.Wrap(colors.ToolName, "bash")
	if result != "bash" {
		t.Fatalf("expected plain text, got %q", result)
	}
}
