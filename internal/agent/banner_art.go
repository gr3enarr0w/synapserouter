package agent

import (
	"fmt"
	"os"
	"strings"
)

// Banner returns the SynRoute logo for display at startup.
func Banner() string {
	return renderBanner(os.Getenv("NO_COLOR") != "")
}

// BannerForWidth returns a banner appropriate for the terminal width.
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

// brainPlain is the NO_COLOR version — traced from synroute-logo.png pixels.
var brainPlain = []string{
	`                      ○○        `,
	`            ○○○○○○○  ○○○        `,
	`         ○○○●●○ ○○ ○  ○○        `,
	`       ○○    ○○○○○ ○○○○○○○      `,
	`      ○○●  ● ○ ○○○○○○○○○·○·     `,
	`      ○○  ○○○○○○○○  ○○○ ○○○     `,
	`       ○ ○○ ●   ●○○○ ○○ ○       `,
	`        ○○   ●●                 `,
}

// brainColor returns ANSI-colored brain lines — per-pixel colors from PNG.
func brainColor() []string {
	return []string{
		"                      \033[38;2;97;255;255m○\033[0m\033[38;2;97;249;255m○\033[0m",
		"            \033[38;2;86;219;249m○\033[0m\033[38;2;110;182;243m○\033[0m\033[38;2;163;159;250m○\033[0m\033[38;2;200;150;255m○\033[0m\033[38;2;212;141;250m○\033[0m\033[38;2;222;142;251m○\033[0m\033[38;2;211;143;248m○\033[0m  \033[38;2;107;249;253m○\033[0m\033[38;2;109;255;255m○\033[0m\033[38;2;100;251;253m○\033[0m",
		"         \033[38;2;82;231;251m○\033[0m\033[38;2;79;215;241m○\033[0m\033[38;2;85;210;241m○\033[0m\033[38;2;85;198;238m●\033[0m\033[38;2;105;168;235m●\033[0m\033[38;2;170;141;243m○\033[0m \033[38;2;207;134;252m○\033[0m\033[38;2;214;135;251m○\033[0m \033[38;2;205;131;242m○\033[0m  \033[38;2;105;250;253m○\033[0m\033[38;2;121;236;255m○\033[0m",
		"       \033[38;2;82;223;247m○\033[0m\033[38;2;80;223;244m○\033[0m    \033[38;2;146;140;242m○\033[0m\033[38;2;176;134;245m○\033[0m\033[38;2;188;127;248m○\033[0m\033[38;2;186;124;248m○\033[0m\033[38;2;188;122;244m○\033[0m \033[38;2;156;156;244m○\033[0m\033[38;2;95;224;246m○\033[0m\033[38;2;94;240;252m○\033[0m\033[38;2;104;240;251m○\033[0m\033[38;2;242;124;242m○\033[0m\033[38;2;239;133;245m○\033[0m\033[38;2;244;134;247m○\033[0m",
		"      \033[38;2;85;228;248m○\033[0m\033[38;2;76;214;238m○\033[0m\033[38;2;72;194;230m●\033[0m  \033[38;2;85;196;239m●\033[0m \033[38;2;147;159;247m○\033[0m \033[38;2;88;210;249m○\033[0m\033[38;2;98;196;243m○\033[0m\033[38;2;100;206;248m○\033[0m\033[38;2;96;225;252m○\033[0m\033[38;2;96;220;246m○\033[0m\033[38;2;114;209;249m○\033[0m\033[38;2;130;195;243m○\033[0m\033[38;2;189;148;237m○\033[0m\033[38;2;234;142;246m○\033[0m\033[38;2;244;140;249m·\033[0m\033[38;2;244;134;248m○\033[0m\033[38;2;247;134;251m·\033[0m",
		"      \033[38;2;81;228;249m○\033[0m\033[38;2;78;212;242m○\033[0m  \033[38;2;76;220;244m○\033[0m\033[38;2;78;220;249m○\033[0m\033[38;2;82;226;249m○\033[0m\033[38;2;81;226;247m○\033[0m\033[38;2;85;220;247m○\033[0m\033[38;2;89;214;246m○\033[0m\033[38;2;102;189;249m○\033[0m\033[38;2;108;181;248m○\033[0m  \033[38;2;206;115;230m○\033[0m\033[38;2;201;118;231m○\033[0m\033[38;2;215;127;240m○\033[0m \033[38;2;240;129;245m○\033[0m\033[38;2;238;123;246m○\033[0m\033[38;2;243;127;247m○\033[0m",
		"       \033[38;2;73;215;238m○\033[0m \033[38;2;82;233;250m○\033[0m\033[38;2;74;217;241m○\033[0m \033[38;2;83;202;237m●\033[0m   \033[38;2;117;156;245m●\033[0m\033[38;2;134;151;248m○\033[0m\033[38;2;175;136;243m○\033[0m\033[38;2;193;127;240m○\033[0m \033[38;2;221;125;240m○\033[0m\033[38;2;224;125;242m○\033[0m \033[38;2;235;127;245m○\033[0m",
		"        \033[38;2;91;245;255m○\033[0m\033[38;2;91;248;252m○\033[0m   \033[38;2;94;182;232m●\033[0m\033[38;2;102;178;239m●\033[0m",
	}
}

// textRight is the branding text placed to the right of the brain.
var textRight = []string{
	"",
	"",
	"SynRoute",
	"──────────────────────",
	"neural routing engine",
	"",
	"",
	"",
}

func renderBanner(noColor bool) string {
	var b strings.Builder

	var brainLines []string
	if noColor {
		brainLines = brainPlain
	} else {
		brainLines = brainColor()
	}

	maxLines := len(brainLines)
	if len(textRight) > maxLines {
		maxLines = len(textRight)
	}

	for i := 0; i < maxLines; i++ {
		brain := ""
		if i < len(brainLines) {
			brain = brainLines[i]
		}

		txt := ""
		if i < len(textRight) {
			txt = textRight[i]
		}

		if noColor {
			// Pad brain to 32 chars for alignment
			plainLen := len([]rune(brain))
			padding := ""
			if plainLen < 32 {
				padding = strings.Repeat(" ", 32-plainLen)
			}
			fmt.Fprintf(&b, "%s%s %s\n", brain, padding, txt)
		} else {
			// Color brain has ANSI escapes — pad based on visible chars
			visible := 0
			inEsc := false
			for _, ch := range brain {
				if ch == '\033' {
					inEsc = true
				} else if inEsc && ch == 'm' {
					inEsc = false
				} else if !inEsc {
					visible++
				}
			}
			padding := ""
			if visible < 32 {
				padding = strings.Repeat(" ", 32-visible)
			}

			if txt != "" {
				// Color the text with a blue-cyan gradient
				fmt.Fprintf(&b, "%s%s \033[1;38;2;100;200;255m%s\033[0m\n", brain, padding, txt)
			} else {
				fmt.Fprintf(&b, "%s%s\n", brain, padding)
			}
		}
	}

	return b.String()
}
