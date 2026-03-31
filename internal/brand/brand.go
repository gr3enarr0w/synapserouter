// Package brand provides the SynRoute visual identity for terminal output.
// The logo is pre-rendered from the brand PNG (assets/synroute-logo.png) using
// chafa and embedded as ANSI art. Works on any terminal with truecolor support,
// degrades gracefully on 256-color terminals.
package brand

import (
	_ "embed"
	"fmt"
	"os"
)

//go:embed logo.ansi
var Logo string

// PrintLogo prints the SynRoute brain-circuit logo to stdout.
// Falls back to plain text when NO_COLOR is set.
func PrintLogo() {
	if os.Getenv("NO_COLOR") != "" {
		fmt.Println("\n  SynRoute - LLM Router & Code Agent")
		fmt.Println()
		return
	}
	fmt.Print(Logo)
}

// FprintLogo prints the logo to the given writer.
func FprintLogo(w *os.File) {
	if os.Getenv("NO_COLOR") != "" {
		fmt.Fprintln(w, "\n  SynRoute")
		return
	}
	fmt.Fprint(w, Logo)
	fmt.Fprintln(w, "\033[1;36m  Syn\033[1;35mRoute\033[0m")
}
