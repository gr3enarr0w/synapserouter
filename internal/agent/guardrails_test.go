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

	secrets := []string{
		"API_KEY=abc123",
		"password=hunter2",
		"aws_access_key_id: AKIA...",
		"ghp_1234567890abcdef",
		"sk-proj-abc123",
		"-----BEGIN RSA PRIVATE KEY-----",
	}
	for _, s := range secrets {
		if result := g.Validate(s); result.Passed {
			t.Errorf("should block secret: %q", s)
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
	if result := chain.Validate("API_KEY=secret123"); result.Passed {
		t.Error("secret should fail chain")
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
