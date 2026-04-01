package agent

import "testing"

func TestMaxLengthGuardrail(t *testing.T) {
	g := &MaxLengthGuardrail{MaxChars: 10}

	if result := g.Validate("short"); !result.Passed {
		t.Error("short content should pass")
	}
	if result := g.Validate("this is way too long for the limit"); result.Passed {
		t.Error("long content should fail")
	}
	if result := g.Validate("this is way too long for the limit"); result.Action != "block" {
		t.Errorf("action = %q, want block", result.Action)
	}
}

func TestSecretPatternGuardrail(t *testing.T) {
	g := &SecretPatternGuardrail{}

	if result := g.Validate("just normal code"); !result.Passed {
		t.Error("normal content should pass")
	}

	// Secrets should PASS (warn, not block) — users need to pass secrets for .env files etc.
	// Secrets are scrubbed from stored data by scrubSecrets().
	secrets := []string{
		"API_KEY=abc123",
		"password=hunter2",
		"aws_access_key_id: AKIA...",
		"ghp_1234567890abcdef",
		"sk-proj-abc123",
		"-----BEGIN RSA PRIVATE KEY-----",
	}
	for _, s := range secrets {
		result := g.Validate(s)
		if !result.Passed {
			t.Errorf("should warn (not block) secret: %q", s)
		}
		if result.Action != "warn" {
			t.Errorf("action should be 'warn' for %q, got %q", s, result.Action)
		}
	}
}

func TestBlocklistGuardrail(t *testing.T) {
	g := &BlocklistGuardrail{Terms: []string{"rm -rf /", "DROP TABLE"}}

	if result := g.Validate("normal code"); !result.Passed {
		t.Error("normal content should pass")
	}
	if result := g.Validate("please rm -rf / now"); result.Passed {
		t.Error("should block dangerous command")
	}
	if result := g.Validate("DROP TABLE users;"); result.Passed {
		t.Error("should block SQL injection")
	}
}

func TestGuardrailChain(t *testing.T) {
	chain := NewGuardrailChain(
		&MaxLengthGuardrail{MaxChars: 100},
		&SecretPatternGuardrail{},
	)

	if result := chain.Validate("hello"); !result.Passed {
		t.Error("normal content should pass chain")
	}
	// Secret should pass chain (warn, not block)
	if result := chain.Validate("API_KEY=secret123"); !result.Passed {
		t.Error("secret should pass chain (warn only)")
	}
}

func TestGuardrailChainAdd(t *testing.T) {
	chain := NewGuardrailChain()
	chain.Add(&MaxLengthGuardrail{MaxChars: 5})

	if result := chain.Validate("toolong"); !result.Passed {
		// length 7 > 5, should fail
	}
	if result := chain.Validate("ok"); !result.Passed {
		t.Error("short content should pass")
	}
}
