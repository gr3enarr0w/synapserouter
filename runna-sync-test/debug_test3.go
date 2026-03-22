package main

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
)

func TestDebugParseICalDetailed(t *testing.T) {
	icalContent := `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Example Corp//Example Calendar//EN
BEGIN:VEVENT
UID:test-123
SUMMARY:Test Workout
DESCRIPTION:This is a test workout
DTSTART:20260322T080000
DTEND:20260322T090000
END:VEVENT
END:VCALENDAR`
	
	fmt.Printf("Input content:\n%s\n\n", icalContent)
	
	// Simulate the parsing logic step by step
	lines := strings.Split(icalContent, "\n")
	fmt.Printf("Lines after split (count=%d):\n", len(lines))
	for i, line := range lines {
		fmt.Printf("  %d: %q\n", i, line)
	}
	
	var unfoldedLines []string
	
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		unfolded := line
		
		isFolded := false
		if i > 0 && (strings.HasSuffix(line, "\r") || strings.HasSuffix(line, "\n")) {
			if strings.HasSuffix(line, " ") || (len(line) > 0 && line[len(line)-1] == ' ') {
				isFolded = true
			}
		}
		
		if isFolded {
			unfolded = line[:len(line)-1]
		}
		
		unfoldedLines = append(unfoldedLines, unfolded)
	}
	
	unfoldedContent := strings.Join(unfoldedLines, "")
	fmt.Printf("\nUnfolded content:\n%s\n\n", unfoldedContent)
	
	// Check the split logic
	blocks := strings.Split(unfoldedContent, "BEGIN:VEVENT")
	fmt.Printf("Number of blocks after split: %d\n", len(blocks))
	for i, block := range blocks {
		fmt.Printf("Block %d (len=%d, empty=%v, contains END=%v): %q\n", 
			i, len(block), strings.TrimSpace(block)=="", strings.Contains(block, "END:VEVENT"), 
			block[:min(50, len(block))])
	}
	
	// Now test the actual regex
	for i, block := range blocks {
		if strings.TrimSpace(block) == "" {
			continue
		}
		if !strings.Contains(block, "END:VEVENT") {
			continue
		}
		
		fmt.Printf("\nProcessing block %d:\n", i)
		
		uidMatch := regexp.MustCompile(`UID:(.+?)\n`).FindStringSubmatch(block)
		fmt.Printf("  UID match: %v\n", uidMatch)
		
		summaryMatch := regexp.MustCompile(`SUMMARY:(.+?)\n`).FindStringSubmatch(block)
		fmt.Printf("  Summary match: %v\n", summaryMatch)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
