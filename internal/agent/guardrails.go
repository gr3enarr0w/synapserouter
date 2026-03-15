package agent

import (
	"strings"
)

// Guardrail validates agent input or output.
type Guardrail interface {
	Name() string
	Validate(content string) *GuardrailResult
}

// GuardrailResult describes whether content passed validation.
type GuardrailResult struct {
	Passed bool   `json:"passed"`
	Reason string `json:"reason,omitempty"`
	Action string `json:"action"` // "block", "warn"
}

// GuardrailChain runs multiple guardrails in sequence.
type GuardrailChain struct {
	guardrails []Guardrail
}

// NewGuardrailChain creates a chain from the given guardrails.
func NewGuardrailChain(guardrails ...Guardrail) *GuardrailChain {
	return &GuardrailChain{guardrails: guardrails}
}

// Validate runs all guardrails and returns the first failure, or nil if all pass.
func (gc *GuardrailChain) Validate(content string) *GuardrailResult {
	for _, g := range gc.guardrails {
		result := g.Validate(content)
		if !result.Passed {
			return result
		}
	}
	return &GuardrailResult{Passed: true}
}

// Add appends a guardrail to the chain.
func (gc *GuardrailChain) Add(g Guardrail) {
	gc.guardrails = append(gc.guardrails, g)
}

// --- Built-in guardrails ---

// MaxLengthGuardrail rejects content exceeding a character limit.
type MaxLengthGuardrail struct {
	MaxChars int
}

func (g *MaxLengthGuardrail) Name() string { return "max_length" }
func (g *MaxLengthGuardrail) Validate(content string) *GuardrailResult {
	if len(content) > g.MaxChars {
		return &GuardrailResult{
			Passed: false,
			Reason: "content exceeds maximum length",
			Action: "block",
		}
	}
	return &GuardrailResult{Passed: true}
}

// SecretPatternGuardrail blocks content that appears to contain secrets.
type SecretPatternGuardrail struct{}

func (g *SecretPatternGuardrail) Name() string { return "secret_pattern" }
func (g *SecretPatternGuardrail) Validate(content string) *GuardrailResult {
	lower := strings.ToLower(content)
	patterns := []string{
		"api_key=",
		"api-key:",
		"secret_key=",
		"password=",
		"aws_access_key_id",
		"aws_secret_access_key",
		"private_key",
		"-----begin rsa private key",
		"-----begin openssh private key",
		"ghp_", // GitHub personal access token prefix
		"sk-",  // OpenAI key prefix
	}
	for _, p := range patterns {
		if strings.Contains(lower, p) {
			return &GuardrailResult{
				Passed: false,
				Reason: "content may contain secrets or credentials",
				Action: "block",
			}
		}
	}
	return &GuardrailResult{Passed: true}
}

// BlocklistGuardrail rejects content containing any blocked term.
type BlocklistGuardrail struct {
	Terms []string
}

func (g *BlocklistGuardrail) Name() string { return "blocklist" }
func (g *BlocklistGuardrail) Validate(content string) *GuardrailResult {
	lower := strings.ToLower(content)
	for _, term := range g.Terms {
		if strings.Contains(lower, strings.ToLower(term)) {
			return &GuardrailResult{
				Passed: false,
				Reason: "content contains blocked term",
				Action: "block",
			}
		}
	}
	return &GuardrailResult{Passed: true}
}
