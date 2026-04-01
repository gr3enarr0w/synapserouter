package agent

import (
	"fmt"
	"os"
	"strings"
)

// Banner returns the SynRoute logo for display at startup.
// Brain on left, text on right, 78 cols max.
// Gradient: cyan → magenta matching the PNG brand colors.
func Banner() string {
	return renderBanner(os.Getenv("NO_COLOR") != "")
}

// BannerForWidth returns a banner appropriate for the terminal width.
// Falls back to text-only if width < 60.
func BannerForWidth(width int, noColor bool) string {
	if width < 60 {
		return renderCompactBanner(noColor)
	}
	return renderBanner(noColor)
}

func renderCompactBanner(noColor bool) string {
	if noColor {
		return "  synroute\n  neural routing engine\n"
	}
	return "  \033[1;36msynroute\033[0m\n  \033[2mneural routing engine\033[0m\n"
}

func renderBanner(noColor bool) string {
	// Brain (left, 24 chars) + gap (3 chars) + text (right)
	// Matches PNG: smooth dome, left=brain folds, right=circuit nodes,
	// blue S-curve through center, pin marker upper-right
	brain := []string{
		`      ╭──╮╭──╮╭──╮       `,
		`    ╭─╯  ╰╯  ╰╯  ╰─╮◉   `,
		`  ╭─╯╭───╮    ●──●──╰─╮  `,
		`  │  ╰───╯╮  ╭╯  │    │  `,
		`  │  ╭━━━━╰━━╯   ●    │  `,
		`  │  ╰───╮╭╮ ╭───╯    │  `,
		`  ╰─╮╭───╯╰╯─╯ ●──● ╭╯  `,
		`    ╰─╯             ╭─╯   `,
		`      ╰─────────────╯    `,
	}

	textClean := []string{
		``,
		``,
		`SynRoute`,
		`──────────────────────`,
		`neural routing engine`,
		``,
		``,
		``,
		``,
	}

	if noColor {
		var b strings.Builder
		for i, line := range brain {
			txt := ""
			if i < len(textClean) {
				txt = textClean[i]
			}
			fmt.Fprintf(&b, "%s   %s\n", line, txt)
		}
		return b.String()
	}

	// Gradient colors per line: cyan → magenta (matching PNG)
	type rgb struct{ r, g, b int }
	gradientBrain := []rgb{
		{0, 207, 255},   // Cyan
		{21, 196, 255},  // Cyan
		{42, 185, 255},  // Cyan-blue
		{85, 170, 255},  // Blue
		{127, 155, 255}, // Blue-purple
		{170, 140, 255}, // Purple
		{200, 125, 255}, // Purple-magenta
		{230, 110, 255}, // Magenta
		{255, 100, 255}, // Bright magenta
	}

	gradientText := []rgb{
		{0, 207, 255},
		{0, 207, 255},
		{42, 185, 255},
		{85, 170, 255},
		{127, 155, 255},
		{170, 140, 255},
		{200, 125, 255},
		{230, 110, 255},
		{255, 100, 255},
	}

	var b strings.Builder
	for i, line := range brain {
		bc := gradientBrain[i]
		tc := gradientText[i]
		txt := ""
		if i < len(textClean) {
			txt = textClean[i]
		}
		// Brain part with gradient
		fmt.Fprintf(&b, "\033[1;38;2;%d;%d;%dm%s\033[0m", bc.r, bc.g, bc.b, line)
		// Text part with gradient
		if txt != "" {
			fmt.Fprintf(&b, "   \033[1;38;2;%d;%d;%dm%s\033[0m", tc.r, tc.g, tc.b, txt)
		}
		b.WriteString("\n")
	}
	return b.String()
}
