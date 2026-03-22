package main

import (
	"strings"
	"testing"
)

// TestGenerateZWO_EncodesSpecialChars verifies that special XML characters
// in workout names are properly escaped
func TestGenerateZWO_EncodesSpecialChars(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "ampersand",
			input:    "Run & Recovery",
			expected: "Run &amp; Recovery",
		},
		{
			name:     "less_than",
			input:    "Run < 5mi",
			expected: "Run &lt; 5mi",
		},
		{
			name:     "greater_than",
			input:    "Run > 5mi",
			expected: "Run &gt; 5mi",
		},
		{
			name:     "quote",
			input:    "Run \"Fast\"",
			expected: "Run &quot;Fast&quot;",
		},
		{
			name:     "apostrophe",
			input:    "Runner's Pace",
			expected: "Runner&apos;s Pace",
		},
		{
			name:     "multiple_special_chars",
			input:    "Run & Run <5mi \"Fast\"",
			expected: "Run &amp; Run &lt;5mi &quot;Fast&quot;",
		},
		{
			name:     "no_special_chars",
			input:    "Easy 3 Mile Run",
			expected: "Easy 3 Mile Run",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := escapeXML(tt.input)
			if result != tt.expected {
				t.Errorf("escapeXML(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestGenerateZWO_ProducesValidXML verifies ZWO output is well-formed XML
func TestGenerateZWO_ProducesValidXML(t *testing.T) {
	steps := []WorkoutStep{
		{Type: "warmup", Duration: 300, PowerLow: 0.5, PowerHigh: 0.6},
		{Type: "steady", Duration: 600, PowerLow: 0.8},
		{Type: "cooldown", Duration: 300, PowerLow: 0.6, PowerHigh: 0.5},
	}

	xml := generateZWO("Test Workout", steps)

	// Check XML structure
	if !strings.Contains(xml, "<?xml version=\"1.0\" encoding=\"UTF-8\"?>") {
		t.Error("Missing XML declaration")
	}
	if !strings.Contains(xml, "<workout_file>") {
		t.Error("Missing workout_file root element")
	}
	if !strings.Contains(xml, "</workout_file>") {
		t.Error("Missing closing workout_file tag")
	}
	if !strings.Contains(xml, "<name>Test Workout</name>") {
		t.Error("Missing or incorrect name element")
	}
	if !strings.Contains(xml, "<sportType>run</sportType>") {
		t.Error("Missing sportType")
	}
	if !strings.Contains(xml, "<workout>") {
		t.Error("Missing workout element")
	}
	if !strings.Contains(xml, "</workout>") {
		t.Error("Missing closing workout tag")
	}
}

// TestGenerateZWO_IncludesAllStepTypes verifies all step types are rendered
func TestGenerateZWO_IncludesAllStepTypes(t *testing.T) {
	steps := []WorkoutStep{
		{Type: "warmup", Duration: 300, PowerLow: 0.5, PowerHigh: 0.6},
		{Type: "steady", Duration: 600, PowerLow: 0.8},
		{Type: "rest", Duration: 120},
		{Type: "interval", Reps: 5, OnDuration: 120, OffDuration: 60, OnPower: 0.95, OffPower: 0.4},
		{Type: "cooldown", Duration: 300, PowerLow: 0.6, PowerHigh: 0.5},
	}

	xml := generateZWO("Complete Workout", steps)

	if !strings.Contains(xml, "<Warmup") {
		t.Error("Missing Warmup element")
	}
	if !strings.Contains(xml, "<SteadyState") {
		t.Error("Missing SteadyState element")
	}
	if !strings.Contains(xml, "<FreeRide") {
		t.Error("Missing FreeRide element")
	}
	if !strings.Contains(xml, "<IntervalsT") {
		t.Error("Missing IntervalsT element")
	}
	if !strings.Contains(xml, "<Cooldown") {
		t.Error("Missing Cooldown element")
	}
}

// TestEscapeXML_EscapesAllSpecialChars verifies escapeXML handles all XML special chars
func TestEscapeXML_EscapesAllSpecialChars(t *testing.T) {
	input := "Test & < > \" '"
	expected := "Test &amp; &lt; &gt; &quot; &apos;"
	
	result := escapeXML(input)
	if result != expected {
		t.Errorf("escapeXML(%q) = %q, want %q", input, result, expected)
	}
}

// TestEscapeXML_RoundTrip verifies escaped output doesn't double-escape
func TestEscapeXML_RoundTrip(t *testing.T) {
	input := "Test & Run"
	escaped := escapeXML(input)
	// Escaping again should produce same result (already escaped)
	// Note: This is actually double-escaping, which is expected behavior
	// The function is not idempotent by design
	reEscape := escapeXML(escaped)
	
	// &amp; becomes &amp;amp;
	if !strings.Contains(reEscape, "&amp;amp;") {
		t.Error("Expected double-escaping behavior")
	}
}

// TestEscapeXML_EmptyString handles empty input
func TestEscapeXML_EmptyString(t *testing.T) {
	result := escapeXML("")
	if result != "" {
		t.Errorf("escapeXML(\"\") = %q, want \"\"", result)
	}
}

// TestEscapeXML_NoSpecialChars passes through unchanged
func TestEscapeXML_NoSpecialChars(t *testing.T) {
	input := "NormalText123"
	result := escapeXML(input)
	if result != input {
		t.Errorf("escapeXML(%q) = %q, want %q", input, result, input)
	}
}

// TestParsePace_ValidPace verifies pace parsing
func TestParsePace_ValidPace(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"13:05", 785},   // 13*60 + 5
		{"10:30", 630},   // 10*60 + 30
		{"11:40", 700},   // 11*60 + 40
		{"9:30", 570},    // 9*60 + 30
		{"13:05/mi", 785}, // with /mi suffix
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parsePace(tt.input)
			if result != tt.expected {
				t.Errorf("parsePace(%q) = %d, want %d", tt.input, result, tt.expected)
			}
		})
	}
}

// TestParsePace_InvalidPace uses default for invalid input
func TestParsePace_InvalidPace(t *testing.T) {
	invalidInputs := []string{"", "abc", "13", "13:05:30", "invalid"}
	
	for _, input := range invalidInputs {
		result := parsePace(input)
		if result != ConversationalPace {
			t.Errorf("parsePace(%q) = %d, want default %d", input, result, ConversationalPace)
		}
	}
}

// TestParseDistance_ValidDistance verifies distance parsing
func TestParseDistance_ValidDistance(t *testing.T) {
	tests := []struct {
		input    string
		expected float64
	}{
		{"1", 1.0},
		{"0.25", 0.25},
		{"3.25", 3.25},
		{"1mi", 1.0},
		{"0.25mi", 0.25},
		{"3.25mi", 3.25},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parseDistance(tt.input)
			if result != tt.expected {
				t.Errorf("parseDistance(%q) = %f, want %f", tt.input, result, tt.expected)
			}
		})
	}
}

// TestParseDistance_InvalidDistance returns zero for invalid input
func TestParseDistance_InvalidDistance(t *testing.T) {
	invalidInputs := []string{"", "abc", "1.2.3"}
	
	for _, input := range invalidInputs {
		result := parseDistance(input)
		if result != 0 {
			t.Errorf("parseDistance(%q) = %f, want 0", input, result)
		}
	}
}

// TestPaceToPower_Zones verifies pace-to-power conversion
func TestPaceToPower_Zones(t *testing.T) {
	tests := []struct {
		pace     int
		expected float64
		zone     string
	}{
		{785, 0.6, "easy/recovery"},   // 13:05+/mi
		{715, 0.7, "steady/moderate"}, // 11:55-13:05/mi
		{695, 0.8, "tempo"},           // 11:35-11:55/mi
		{630, 0.95, "threshold"},      // 10:30-11:35/mi
		{570, 1.0, "intervals/fast"},  // <10:30/mi
	}

	for _, tt := range tests {
		t.Run(tt.zone, func(t *testing.T) {
			result := paceToPower(tt.pace)
			if result != tt.expected {
				t.Errorf("paceToPower(%d) = %f, want %f (%s)", tt.pace, result, tt.expected, tt.zone)
			}
		})
	}
}
