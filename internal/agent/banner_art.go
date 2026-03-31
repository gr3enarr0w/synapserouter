package agent

// Professional enterprise-grade banner with clean circuit-brain icon, SynapseRouter text
// centered with taglines, smooth gradient across entire banner.

func Banner() string {
	return gradientBanner()
}

func gradientBanner() string {
	// Circuit-brain icon centered with SynapseRouter text and taglines
	// Icon is 19 chars wide, text block is centered, total ~60 chars for 100-char width
	bannerLines := []string{
		"         ╭───────────────╮                   ███████╗██╗   ██╗██████╗ ███████╗██████╗ ██╗██████╗ ███████╗██╗   ██╗ ██████╗ █████╗ ███████╗███████╗",
		"        ╭╯   ╭─────╮   ╰╮                  ██╔════╝██║   ██║██╔══██╗██╔════╝██╔══██╗██║██╔══██╗██╔════╝╚██╗ ██╔╝██╔════╝██╔══██╗██╔════╝██╔════╝",
		"       ╭╯   ╭╯     ╰╮  ╰╮                 ███████╗██║   ██║██████╔╝█████╗  ██████╔╝██║██║  ██║█████╗   ╚████╔╝ ██║     ███████║███████╗█████╗",
		"      ╭╯   ╭╯  ●  ● ╰╮ ╰╮                 ╚════██║██║   ██║██╔══██╗██╔══╝  ██╔══██╗██║██║  ██║██╔══╝    ╚██╔╝  ██║     ██╔══██║╚════██║██╔══╝",
		"      │   ╭╯         ╰╮│                   ███████║╚██████╔╝██║  ██╗███████╗██║  ██║██║██████╔╝███████╗  ██║   ╚██████╗ ██║  ██║███████║███████╗",
		"      │   │    ╭──╮   ││                   ╚══════╝ ╚═════╝ ╚═╝  ╚═╝╚══════╝╚═╝  ╚═╝╚═╝╚═════╝ ╚══════╝  ╚═╝    ╚═════╝ ╚═╝  ╚═╝╚══════╝╚══════╝",
		"      │   ╰╮   ╰──╯  ╭╯│                                                                                                             ",
		"      ╰╮   ╰╮       ╭╯ ╰╮                                      ┌─────────────────────────┐                                      ",
		"       ╰╮   ╰───────╯  ╰╮                                      │  Neural Routing Engine  │                                      ",
		"        ╰╮            ╭╯                                       │  Enterprise LLM Gateway │                                      ",
		"         ╰────────────╯                                         └─────────────────────────┘                                      ",
	}

	gradientColors := []string{
		"38;2;0;255;255",    // Cyan
		"38;2;42;236;255",   // Cyan-blue
		"38;2;85;217;255",   // Light cyan
		"38;2;127;198;255",  // Blue-cyan
		"38;2;170;179;255",  // Blue-purple
		"38;2;212;160;255",  // Purple
		"38;2;255;141;255",  // Magenta-purple
		"38;2;255;120;255",  // Magenta
		"38;2;255;100;255",  // Bright magenta
		"38;2;255;80;255",   // Pink-magenta
		"38;2;255;60;255",   // Pink
	}

	b := []byte{}
	for i, line := range bannerLines {
		colorIndex := i % len(gradientColors)
		b = append(b, []byte("\033["+gradientColors[colorIndex]+"m"+line+"\033[0m\n")...)
	}
	return string(b)
}