package security

import (
	"strings"
	"testing"
)

func TestRedactor_Email(t *testing.T) {
	redactor := NewRedactor()
	
	tests := []struct {
		input    string
		expected string
	}{
		{"Contact me at john@example.com", "Contact me at [REDACTED_EMAIL]"},
		{"Email: alice.smith@company.co.uk", "Email: [REDACTED_EMAIL]"},
		{"test.user+tag@domain.org", "[REDACTED_EMAIL]"},
	}

	for _, tt := range tests {
		result := redactor.Redact(tt.input)
		if result.Text != tt.expected {
			t.Errorf("Redact(%q) = %q, want %q", tt.input, result.Text, tt.expected)
		}
		if result.Types["EMAIL"] == 0 {
			t.Errorf("Expected EMAIL type to be detected in %q", tt.input)
		}
	}
}

func TestRedactor_AWSKeys(t *testing.T) {
	redactor := NewRedactor()
	
	tests := []struct {
		input    string
		expected string
	}{
		{"AKIAIOSFODNN7EXAMPLE", "[REDACTED_AWS_KEY]"},
		{"aws_secret_access_key = wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", "aws_secret_access_key=[REDACTED_AWS_SECRET]"},
		{"AWS_SECRET_KEY: abcdefghij1234567890abcdefghij1234567890", "AWS_SECRET_KEY=[REDACTED_AWS_SECRET]"},
	}

	for _, tt := range tests {
		result := redactor.Redact(tt.input)
		if !strings.Contains(result.Text, "[REDACTED_AWS") {
			t.Errorf("Redact(%q) should contain AWS redaction marker, got %q", tt.input, result.Text)
		}
	}
}

func TestRedactor_GitHubTokens(t *testing.T) {
	redactor := NewRedactor()
	
	tests := []struct {
		input    string
		expected string
	}{
		{"ghp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx", "[REDACTED_GITHUB_TOKEN]"},
		{"gho_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx", "[REDACTED_GITHUB_OAUTH]"},
		{"ghu_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx", "[REDACTED_GITHUB_APP_TOKEN]"},
		{"ghs_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx", "[REDACTED_GITHUB_APP_TOKEN]"},
	}

	for _, tt := range tests {
		result := redactor.Redact(tt.input)
		if !strings.Contains(result.Text, "[REDACTED_GITHUB") {
			t.Errorf("Redact(%q) should contain GitHub redaction marker, got %q", tt.input, result.Text)
		}
	}
}

func TestRedactor_APIKeys(t *testing.T) {
	redactor := NewRedactor()
	
	tests := []struct {
		input    string
		expected string
	}{
		{"sk-ant-api03-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx", "[REDACTED_ANTHROPIC_KEY]"},
		{"sk-abcdefghijklmnopqrstuvwxyz0123456789ABCDEFGH", "[REDACTED_OPENAI_KEY]"},
		{"api_key = abcdefghij1234567890abcd", "api_key=[REDACTED_API_KEY]"},
		{"APIKEY: xyz123abc456def789ghi012jkl", "APIKEY=[REDACTED_API_KEY]"},
	}

	for _, tt := range tests {
		result := redactor.Redact(tt.input)
		if !strings.Contains(result.Text, "[REDACTED_") {
			t.Errorf("Redact(%q) should contain redaction marker, got %q", tt.input, result.Text)
		}
	}
}

func TestRedactor_Passwords(t *testing.T) {
	redactor := NewRedactor()
	
	tests := []struct {
		input    string
		expected string
	}{
		{"password = mysecret123", "password=[REDACTED_PASSWORD]"},
		{"PASSWORD: hunter2", "PASSWORD=[REDACTED_PASSWORD]"},
		{"pwd=abc123", "pwd=[REDACTED_PASSWORD]"},
	}

	for _, tt := range tests {
		result := redactor.Redact(tt.input)
		if !strings.Contains(result.Text, "[REDACTED_PASSWORD]") {
			t.Errorf("Redact(%q) should contain password redaction, got %q", tt.input, result.Text)
		}
	}
}

func TestRedactor_Tokens(t *testing.T) {
	redactor := NewRedactor()
	
	tests := []struct {
		input    string
		expected string
	}{
		{"Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c", "Bearer [REDACTED_JWT]"},
		{"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U", "[REDACTED_JWT]"},
	}

	for _, tt := range tests {
		result := redactor.Redact(tt.input)
		if !strings.Contains(result.Text, "[REDACTED_JWT]") {
			t.Errorf("Redact(%q) should contain JWT redaction, got %q", tt.input, result.Text)
		}
	}
}

func TestRedactor_SSN(t *testing.T) {
	redactor := NewRedactor()
	
	input := "SSN: 123-45-6789"
	result := redactor.Redact(input)
	
	if result.Text != "SSN: [REDACTED_SSN]" {
		t.Errorf("Redact(%q) = %q, want %q", input, result.Text, "SSN: [REDACTED_SSN]")
	}
}

func TestRedactor_Phone(t *testing.T) {
	redactor := NewRedactor()
	
	tests := []struct {
		input    string
		expected string
	}{
		{"Call 555-123-4567", "Call [REDACTED_PHONE]"},
		{"+1 (555) 123-4567", "[REDACTED_PHONE]"},
		{"555.123.4567", "[REDACTED_PHONE]"},
	}

	for _, tt := range tests {
		result := redactor.Redact(tt.input)
		if !strings.Contains(result.Text, "[REDACTED_PHONE]") {
			t.Errorf("Redact(%q) should contain phone redaction, got %q", tt.input, result.Text)
		}
	}
}

func TestRedactor_IPAddresses(t *testing.T) {
	redactor := NewRedactor()
	
	tests := []struct {
		input    string
		expected string
	}{
		{"Server at 192.168.1.1", "Server at [REDACTED_IP]"},
		{"10.0.0.1:8080", "[REDACTED_IP]:8080"},
		{"2001:0db8:85a3:0000:0000:8a2e:0370:7334", "[REDACTED_IP]"},
	}

	for _, tt := range tests {
		result := redactor.Redact(tt.input)
		if !strings.Contains(result.Text, "[REDACTED_IP]") {
			t.Errorf("Redact(%q) should contain IP redaction, got %q", tt.input, result.Text)
		}
	}
}

func TestRedactor_CreditCard(t *testing.T) {
	redactor := NewRedactor()
	
	tests := []struct {
		input    string
		expected string
	}{
		{"Card: 4111-1111-1111-1111", "Card: [REDACTED_CC]"},
		{"Card: 5500 0000 0000 0004", "Card: [REDACTED_CC]"},
		{"Amex: 3782-822463-10005", "Amex: [REDACTED_CC]"},
	}

	for _, tt := range tests {
		result := redactor.Redact(tt.input)
		if !strings.Contains(result.Text, "[REDACTED_CC]") {
			t.Errorf("Redact(%q) should contain CC redaction, got %q", tt.input, result.Text)
		}
	}
}

func TestRedactor_PrivateKey(t *testing.T) {
	redactor := NewRedactor()
	
	input := `-----BEGIN RSA PRIVATE KEY-----
MIIEpAIBAAKCAQEA0Z3VS5JJcds3xfn/ygWyF8PbnGy0AHB7MmE8D9fI5K3qH
-----END RSA PRIVATE KEY-----`
	
	result := redactor.Redact(input)
	
	if !strings.Contains(result.Text, "[REDACTED_PRIVATE_KEY]") {
		t.Errorf("Redact should contain private key redaction, got %q", result.Text)
	}
}

func TestRedactor_MultiplePatterns(t *testing.T) {
	redactor := NewRedactor()
	
	input := "Contact john@example.com or call 555-123-4567. API key is sk-abcdefghijklmnopqrstuvwxyz0123456789ABCDEFGH"
	result := redactor.Redact(input)
	
	if result.RedactedCount < 3 {
		t.Errorf("Expected at least 3 redactions, got %d", result.RedactedCount)
	}
	
	if !strings.Contains(result.Text, "[REDACTED_EMAIL]") {
		t.Error("Expected email redaction")
	}
	if !strings.Contains(result.Text, "[REDACTED_PHONE]") {
		t.Error("Expected phone redaction")
	}
	if !strings.Contains(result.Text, "[REDACTED_OPENAI_KEY]") {
		t.Error("Expected OpenAI key redaction")
	}
}

func TestRedactor_RedactQuery(t *testing.T) {
	input := "Find user with email test@example.com and phone 555-123-4567"
	redacted, count := RedactQuery(input)
	
	if count < 2 {
		t.Errorf("Expected at least 2 redactions, got %d", count)
	}
	
	if !strings.Contains(redacted, "[REDACTED_EMAIL]") {
		t.Error("Expected email redaction")
	}
	if !strings.Contains(redacted, "[REDACTED_PHONE]") {
		t.Error("Expected phone redaction")
	}
}

func TestRedactor_IsRedacted(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"Normal text", false},
		{"[REDACTED_EMAIL]", true},
		{"Contains [REDACTED_PHONE] marker", true},
		{"", false},
	}

	for _, tt := range tests {
		result := IsRedacted(tt.input)
		if result != tt.expected {
			t.Errorf("IsRedacted(%q) = %v, want %v", tt.input, result, tt.expected)
		}
	}
}

func TestRedactor_GetRedactionTypes(t *testing.T) {
	types := GetRedactionTypes()
	
	if len(types) == 0 {
		t.Error("Expected at least one redaction type")
	}
	
	// Check for expected types
	expectedTypes := []string{"EMAIL", "AWS_ACCESS_KEY", "PASSWORD", "SSN", "PHONE", "IPV4", "CREDIT_CARD"}
	for _, expected := range expectedTypes {
		found := false
		for _, t := range types {
			if t == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected redaction type %q not found", expected)
		}
	}
}

func TestRedactor_NoFalsePositives(t *testing.T) {
	redactor := NewRedactor()
	
	// These should NOT be redacted
	safeInputs := []string{
		"The quick brown fox jumps over the lazy dog",
		"Version 1.2.3 of the software",
		"Meeting at 3pm tomorrow",
		"Room number 42",
	}

	for _, input := range safeInputs {
		result := redactor.Redact(input)
		if result.RedactedCount > 0 {
			t.Errorf("Unexpected redaction in safe input %q: got %q", input, result.Text)
		}
	}
}
