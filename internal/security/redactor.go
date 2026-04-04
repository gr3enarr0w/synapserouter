package security

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Redactor scans text and redacts sensitive information (PII, secrets, credentials)
type Redactor struct {
	patterns      []redactionPattern
	customRules   []CustomRule
	ignoredNames  map[string]bool
}

type redactionPattern struct {
	name        string
	regex       *regexp.Regexp
	replacement string
}

// CustomRule represents a user-defined redaction rule loaded from JSON
type CustomRule struct {
	Name   string `json:"name"`
	Regex  string `json:"regex"`
	Action string `json:"action"` // "redact" or "ignore"
}

// NewRedactor creates a redactor with all known PII/secret patterns
func NewRedactor() *Redactor {
	r := &Redactor{
		patterns: []redactionPattern{
			// OpenAI API keys (sk-...)
			{
				name:        "OPENAI_KEY",
				regex:       regexp.MustCompile(`\bsk-[a-zA-Z0-9]{20,}\b`),
				replacement: "[REDACTED_OPENAI_KEY]",
			},
			// Generic API keys (api_key=..., apikey=..., API_KEY=...)
			{
				name:        "API_KEY",
				regex:       regexp.MustCompile(`(?i)(?:api[_-]?key|apikey)\s*[=:]\s*["']?[a-zA-Z0-9_-]{16,}["']?`),
				replacement: "[REDACTED_API_KEY]",
			},
			// Passwords (password=..., passwd=..., pwd=...)
			{
				name:        "PASSWORD",
				regex:       regexp.MustCompile(`(?i)(?:password|passwd|pwd)\s*[=:]\s*["']?[^\s"']{4,}["']?`),
				replacement: "[REDACTED_PASSWORD]",
			},
			// Email addresses
			{
				name:        "EMAIL",
				regex:       regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`),
				replacement: "[REDACTED_EMAIL]",
			},
			// AWS Access Key ID
			{
				name:        "AWS_ACCESS_KEY",
				regex:       regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
				replacement: "[REDACTED_AWS_KEY]",
			},
			// AWS Secret Access Key (40 char base64)
			{
				name:        "AWS_SECRET_KEY",
				regex:       regexp.MustCompile(`(?i)(aws_secret_access_key|aws_secret_key)\s*[=:]\s*['"]?([A-Za-z0-9/+=]{40})['"]?`),
				replacement: "$1=[REDACTED_AWS_SECRET]",
			},
			// GitHub Personal Access Token (classic)
			{
				name:        "GITHUB_TOKEN",
				regex:       regexp.MustCompile(`\bghp_[a-zA-Z0-9]{36}\b`),
				replacement: "[REDACTED_GITHUB_TOKEN]",
			},
			// GitHub OAuth Access Token
			{
				name:        "GITHUB_OAUTH",
				regex:       regexp.MustCompile(`\bgho_[a-zA-Z0-9]{36}\b`),
				replacement: "[REDACTED_GITHUB_OAUTH]",
			},
			// GitHub App Token
			{
				name:        "GITHUB_APP_TOKEN",
				regex:       regexp.MustCompile(`\b(ghu|ghs)_[a-zA-Z0-9]{36}\b`),
				replacement: "[REDACTED_GITHUB_APP_TOKEN]",
			},
			// Anthropic API Key
			{
				name:        "ANTHROPIC_KEY",
				regex:       regexp.MustCompile(`\bsk-ant-[a-zA-Z0-9_-]{90,}\b`),
				replacement: "[REDACTED_ANTHROPIC_KEY]",
			},
			// OpenAI API Key
			{
				name:        "OPENAI_KEY",
				regex:       regexp.MustCompile(`\bsk-[a-zA-Z0-9]{48}\b`),
				replacement: "[REDACTED_OPENAI_KEY]",
			},
			// Generic API Key patterns
			{
				name:        "API_KEY",
				regex:       regexp.MustCompile(`(?i)(api[_-]?key|apikey)\s*[=:]\s*['"]?[a-zA-Z0-9_-]{20,}['"]?`),
				replacement: "$1=[REDACTED_API_KEY]",
			},
			// Generic secret/token patterns
			{
				name:        "SECRET",
				regex:       regexp.MustCompile(`(?i)(secret|token)\s*[=:]\s*['"]?[a-zA-Z0-9_-]{16,}['"]?`),
				replacement: "$1=[REDACTED_SECRET]",
			},
			// Bearer token
			{
				name:        "BEARER_TOKEN",
				regex:       regexp.MustCompile(`(?i)bearer\s+[a-zA-Z0-9_-]+\.[a-zA-Z0-9_-]+\.[a-zA-Z0-9_-]+`),
				replacement: "Bearer [REDACTED_JWT]",
			},
			// JWT token (standalone)
			{
				name:        "JWT",
				regex:       regexp.MustCompile(`\beyJ[a-zA-Z0-9_-]*\.eyJ[a-zA-Z0-9_-]*\.[a-zA-Z0-9_-]*\b`),
				replacement: "[REDACTED_JWT]",
			},
			// Social Security Number
			{
				name:        "SSN",
				regex:       regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`),
				replacement: "[REDACTED_SSN]",
			},
			// Phone numbers (various formats)
			{
				name:        "PHONE",
				regex:       regexp.MustCompile(`\+?1?[-.\s]?\(?\d{3}\)?[-.\s]?\d{3}[-.\s]?\d{4}\b`),
				replacement: "[REDACTED_PHONE]",
			},
			// IPv4 addresses
			{
				name:        "IPV4",
				regex:       regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`),
				replacement: "[REDACTED_IP]",
			},
			// IPv6 addresses (simplified)
			{
				name:        "IPV6",
				regex:       regexp.MustCompile(`\b(?:[0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4}\b`),
				replacement: "[REDACTED_IP]",
			},
			// Credit card numbers: 4-4-4-4 (Visa/MC), 4-6-5 (Amex), or 13-19 consecutive digits
			{
				name:        "CREDIT_CARD",
				regex:       regexp.MustCompile(`\b(?:\d{4}[-\s]){3}\d{4}\b|\b\d{4}[-\s]\d{6}[-\s]\d{5}\b|\b\d{13,19}\b`),
				replacement: "[REDACTED_CC]",
			},
			// Private keys (PEM format)
			{
				name:        "PRIVATE_KEY",
				regex:       regexp.MustCompile(`-----BEGIN (?:RSA |EC |DSA )?PRIVATE KEY-----[a-zA-Z0-9+/=\s]+-----END (?:RSA |EC |DSA )?PRIVATE KEY-----`),
				replacement: "[REDACTED_PRIVATE_KEY]",
			},
		},
		ignoredNames: make(map[string]bool),
	}
	return r
}

// LoadCustomRules loads user-defined redaction rules from a JSON file
func (r *Redactor) LoadCustomRules(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // File doesn't exist yet, that's OK
		}
		return err
	}

	var rules []CustomRule
	if err := json.Unmarshal(data, &rules); err != nil {
		return err
	}

	r.customRules = rules
	for _, rule := range rules {
		if rule.Action == "ignore" {
			r.ignoredNames[rule.Name] = true
		} else if rule.Action == "redact" {
			compiled, err := regexp.Compile(rule.Regex)
			if err != nil {
				continue // Skip invalid regex
			}
			r.patterns = append(r.patterns, redactionPattern{
				name:        "CUSTOM_" + rule.Name,
				regex:       compiled,
				replacement: "[REDACTED_" + strings.ToUpper(rule.Name) + "]",
			})
		}
	}

	return nil
}

// SaveCustomRule saves a new custom rule to the JSON file
func SaveCustomRule(path, name, regex, action string) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Load existing rules
	var rules []CustomRule
	data, err := os.ReadFile(path)
	if err == nil {
		json.Unmarshal(data, &rules)
	}

	// Check if rule already exists, update if so
	found := false
	for i, rule := range rules {
		if rule.Name == name {
			rules[i].Regex = regex
			rules[i].Action = action
			found = true
			break
		}
	}

	if !found {
		rules = append(rules, CustomRule{
			Name:   name,
			Regex:  regex,
			Action: action,
		})
	}

	// Write back
	data, err = json.MarshalIndent(rules, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// RemoveCustomRule removes a custom rule from the JSON file
func RemoveCustomRule(path, name string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var rules []CustomRule
	if err := json.Unmarshal(data, &rules); err != nil {
		return err
	}

	// Filter out the rule with matching name
	newRules := make([]CustomRule, 0, len(rules))
	for _, rule := range rules {
		if rule.Name != name {
			newRules = append(newRules, rule)
		}
	}

	data, err = json.MarshalIndent(newRules, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// RedactResult contains the redacted text and metadata about what was redacted
type RedactResult struct {
	Text          string
	RedactedCount int
	Types         map[string]int
}

// Redact scans the input text and replaces all sensitive patterns with redaction markers
func (r *Redactor) Redact(text string) *RedactResult {
	result := &RedactResult{
		Text:   text,
		Types:  make(map[string]int),
	}

	for _, pattern := range r.patterns {
		// Skip if this pattern name is ignored
		if r.ignoredNames[pattern.name] {
			continue
		}

		matches := pattern.regex.FindAllString(result.Text, -1)
		if len(matches) > 0 {
			result.RedactedCount += len(matches)
			result.Types[pattern.name] += len(matches)
			result.Text = pattern.regex.ReplaceAllString(result.Text, pattern.replacement)
		}
	}

	return result
}

// TestRedaction returns what would be redacted without modifying the original
func (r *Redactor) TestRedaction(text string) *RedactResult {
	return r.Redact(text)
}

// GetActivePatterns returns all active pattern names (built-in + custom, excluding ignored)
func (r *Redactor) GetActivePatterns() []string {
	var names []string
	for _, p := range r.patterns {
		if !r.ignoredNames[p.name] {
			names = append(names, p.name)
		}
	}
	return names
}

// GetIgnoredPatterns returns all ignored pattern names
func (r *Redactor) GetIgnoredPatterns() []string {
	var names []string
	for name := range r.ignoredNames {
		names = append(names, name)
	}
	return names
}

// RedactQuery is a convenience function for redacting search queries
func RedactQuery(query string) (string, int) {
	redactor := NewRedactor()
	result := redactor.Redact(query)
	return result.Text, result.RedactedCount
}

// GetRedactionTypes returns a list of all pattern types the redactor can detect
func GetRedactionTypes() []string {
	redactor := NewRedactor()
	types := make([]string, 0, len(redactor.patterns))
	for _, p := range redactor.patterns {
		types = append(types, p.name)
	}
	return types
}

// IsRedacted checks if a string contains any redaction markers
func IsRedacted(text string) bool {
	return strings.Contains(text, "[REDACTED_")
}
