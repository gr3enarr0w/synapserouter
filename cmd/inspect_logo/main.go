package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	data, _ := os.ReadFile("assets/logo-ascii-full.txt")
	lines := strings.Split(string(data), "\n")

	// Write brain lines (16-50, 0-indexed 15-49) and text lines (54-61, 0-indexed 53-60) to separate files
	brainOut, _ := os.Create("brain_lines.txt")
	textOut, _ := os.Create("text_lines.txt")
	defer brainOut.Close()
	defer textOut.Close()

	for i := 15; i < 50 && i < len(lines); i++ {
		trimmed := strings.TrimRight(lines[i], " \t\r")
		fmt.Fprintf(brainOut, "%s\n", trimmed)
	}
	for i := 53; i < 61 && i < len(lines); i++ {
		trimmed := strings.TrimRight(lines[i], " \t\r")
		fmt.Fprintf(textOut, "%s\n", trimmed)
	}
}
