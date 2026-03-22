package main

import (
    "strings"
    "testing"
    "time"
)

// Test the fixes
func TestFixValidation(t *testing.T) {
    // Test case 1: Line unfolding
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
    
    // Check that the description was properly unfolded
    expectedText := "spans multiple lines"
    if !strings.Contains(event.Description, expectedText) {
        t.Errorf("Expected '%s' in description, got: %q", expectedText, event.Description)
    }
    
    // Test case 2: DTSTART with TZID format
    icalContent2 := `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Example Corp//Example Calendar//EN
BEGIN:VEVENT
UID:test-tzid
SUMMARY:Workout with TZID
DTSTART;TZID=America/New_York:20260322T080000
DTEND;TZID=America/New_York:20260322T090000
END:VEVENT
END:VCALENDAR`
    
    events2 := ParseICal(icalContent2)
    
    if len(events2) != 1 {
        t.Fatalf("Expected 1 event, got %d", len(events2))
    }
    
    event2 := events2[0]
    expectedDate := time.Date(2026, 3, 22, 8, 0, 0, 0, time.Now().Location())
    if !event2.StartDate.Equal(expectedDate) {
        t.Errorf("Expected StartDate %v, got %v", expectedDate, event2.StartDate)
    }
}

func TestExtractDateTimeValue(t *testing.T) {
    // Test the extractor function
    testCases := []struct {
        input    string
        expected string
    }{
        {"20260322T080000", "20260322T080000"},
        {"  20260322T080000  ", "20260322T080000"},
        {"20260322T080000X", "20260322T080000"},
        {"someText20260322T080000more", "20260322T080000"},
    }
    
    for _, tc := range testCases {
        result := extractDateTimeValue(tc.input)
        if result != tc.expected {
            t.Errorf("extractDateTimeValue(%q) = %q, want %q", tc.input, result, tc.expected)
        }
    }
}
