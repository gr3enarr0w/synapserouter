package agent

import (
	"fmt"
	"os"
	"strings"
)

// Banner returns the SynRoute wordmark for display at startup.
// Logo art deferred to v1.0.4 — see GitHub issue #356.
func Banner() string {
	return renderBanner(os.Getenv("NO_COLOR") != "")
}

// BannerForWidth returns a banner appropriate for the terminal width.
// Currently returns the standard banner; width parameter reserved for future use.
func BannerForWidth(width int, noColor bool) string {
	return renderBanner(noColor)
}

func renderBanner(noColor bool) string {
	wordmark := "synroute"
	subtitle := "neural routing engine"

	var b strings.Builder
	b.WriteString("\n")

	if noColor {
		b.WriteString("  " + wordmark + "\n")
		b.WriteString("  " + subtitle + "\n")
	} else {
		// Gradient: cyan (#00CFFF) → blue (#4B8EF1)
		type rgb struct{ r, g, b int }
		from := rgb{0, 207, 255}
		to := rgb{75, 142, 241}

		runes := []rune(wordmark)
		n := len(runes)
		b.WriteString("  ")
		for j, ch := range runes {
			t := float64(j) / float64(n-1)
			r := int(float64(from.r)*(1-t) + float64(to.r)*t)
			g := int(float64(from.g)*(1-t) + float64(to.g)*t)
			bv := int(float64(from.b)*(1-t) + float64(to.b)*t)
			fmt.Fprintf(&b, "\033[1;38;2;%d;%d;%dm%c", r, g, bv, ch)
		}
		b.WriteString("\033[0m\n")
		b.WriteString("\033[2m  " + subtitle + "\033[0m\n")
	}

	return b.String()
}
