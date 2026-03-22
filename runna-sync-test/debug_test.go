package main

import (
	"fmt"
	"testing"
)

func TestDebugParseICal(t *testing.T) {
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
	
	events := ParseICal(icalContent)
	
	fmt.Printf("Number of events: %d\n", len(events))
	
	for i, event := range events {
		fmt.Printf("Event %d:\n", i)
		fmt.Printf("  UID: %s\n", event.UID)
		fmt.Printf("  Summary: %s\n", event.Summary)
		fmt.Printf("  StartDate: %v\n", event.StartDate)
	}
}
