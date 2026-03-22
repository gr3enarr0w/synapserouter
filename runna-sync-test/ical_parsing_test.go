package main

import (
	"strings"
	"testing"
	"time"
)

// TestParseICal_LineUnfolding verifies that line unfolding happens before regex parsing
func TestParseICal_LineUnfolding(t *testing.T) {
	// Create iCal content with folded lines (lines ending with space for continuation)
	icalContent := `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Example Corp//Example Calendar//EN
BEGIN:VEVENT
UID:test-123
SUMMARY:Test Workout
DESCRIPTION:This is a test workout
 that spans multiple lines
 with line folding.
DTSTART:20260322T080000
DTEND:20260322T090000
END:VEVENT
END:VCALENDAR`
	
	events := ParseICal(icalContent)
	
	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}
	
	event := events[0]
	if event.UID != "test-123" {
		t.Errorf("Expected UID 'test-123', got '%s'", event.UID)
	}
	
	// The description should have the unfolded line
	if !strings.Contains(event.Description, "spans multiple lines") {
		t.Errorf("Expected unfolded description, got: %q", event.Description)
	}
}

// TestParseICal_DTSTART_WithTZID verifies DTSTART parsing with timezone
func TestParseICal_DTSTART_WithTZID(t *testing.T) {
	icalContent := `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Example Corp//Example Calendar//EN
BEGIN:VEVENT
UID:test-tzid
SUMMARY:Workout with TZID
DTSTART;TZID=America/New_York:20260322T080000
DTEND;TZID=America/New_York:20260322T090000
END:VEVENT
END:VCALENDAR`
	
	events := ParseICal(icalContent)
	
	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}
	
	event := events[0]
	
	// Check that the date was parsed correctly
	expectedDate := time.Date(2026, 3, 22, 8, 0, 0, 0, time.Now().Location())
	if !event.StartDate.Equal(expectedDate) {
		t.Errorf("Expected StartDate %v, got %v", expectedDate, event.StartDate)
	}
}

// TestParseICal_DTSTART_BareDate verifies DTSTART parsing without timezone
func TestParseICal_DTSTART_BareDate(t *testing.T) {
	icalContent := `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Example Corp//Example Calendar//EN
BEGIN:VEVENT
UID:test-bare
SUMMARY:Workout without TZID
DTSTART:20260322T080000
DTEND:20260322T090000
END:VEVENT
END:VCALENDAR`
	
	events := ParseICal(icalContent)
	
	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}
	
	event := events[0]
	
	// Check that the date was parsed correctly
	expectedDate := time.Date(2026, 3, 22, 8, 0, 0, 0, time.Now().Location())
	if !event.StartDate.Equal(expectedDate) {
		t.Errorf("Expected StartDate %v, got %v", expectedDate, event.StartDate)
	}
}

// TestParseICal_DescriptionEscaping verifies that escaped characters in description are handled
func TestParseICal_DescriptionEscaping(t *testing.T) {
	icalContent := `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Example Corp//Example Calendar//EN
BEGIN:VEVENT
UID:test-escape
SUMMARY:Workout with escaped chars
DESCRIPTION:Run at 10:30/mi\, 90s walking rest\; next line
DTSTART:20260322T080000
DTEND:20260322T090000
END:VEVENT
END:VCALENDAR`
	
	events := ParseICal(icalContent)
	
	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}
	
	event := events[0]
	
	// Check that escaped characters were unescaped
	if !strings.Contains(event.Description, ",") {
		t.Errorf("Expected comma to be unescaped in description: %q", event.Description)
	}
	if !strings.Contains(event.Description, ";") {
		t.Errorf("Expected semicolon to be unescaped in description: %q", event.Description)
	}
}

// TestParseICal_MultipleEvents verifies parsing multiple events
func TestParseICal_MultipleEvents(t *testing.T) {
	icalContent := `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Example Corp//Example Calendar//EN
BEGIN:VEVENT
UID:event-1
SUMMARY:First Workout
DTSTART:20260322T080000
DTEND:20260322T090000
END:VEVENT
BEGIN:VEVENT
UID:event-2
SUMMARY:Second Workout
DTSTART:20260323T080000
DTEND:20260323T090000
END:VEVENT
END:VCALENDAR`
	
	events := ParseICal(icalContent)
	
	if len(events) != 2 {
		t.Fatalf("Expected 2 events, got %d", len(events))
	}
	
	if events[0].UID != "event-1" {
		t.Errorf("Expected first event UID 'event-1', got '%s'", events[0].UID)
	}
	if events[1].UID != "event-2" {
		t.Errorf("Expected second event UID 'event-2', got '%s'", events[1].UID)
	}
}

// TestParseICal_EmptyUIDSkipped verifies events without UID are skipped
func TestParseICal_EmptyUIDSkipped(t *testing.T) {
	icalContent := `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Example Corp//Example Calendar//EN
BEGIN:VEVENT
SUMMARY:Workout without UID
DTSTART:20260322T080000
DTEND:20260322T090000
END:VEVENT
END:VCALENDAR`
	
	events := ParseICal(icalContent)
	
	if len(events) != 0 {
		t.Fatalf("Expected 0 events (no UID), got %d", len(events))
	}
}
