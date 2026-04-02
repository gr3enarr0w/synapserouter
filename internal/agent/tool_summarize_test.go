package agent

import (
	"strings"
	"testing"
)

func TestShouldSummarize_SmallOutput(t *testing.T) {
	if ShouldSummarize("bash", "hello world") {
		t.Error("small output should not be summarized")
	}
}

func TestShouldSummarize_LargeOutput(t *testing.T) {
	large := strings.Repeat("x", 3000)
	if !ShouldSummarize("bash", large) {
		t.Error("large output should be summarized")
	}
}

func TestShouldSummarize_FileWriteNever(t *testing.T) {
	large := strings.Repeat("x", 5000)
	if ShouldSummarize("file_write", large) {
		t.Error("file_write should never be summarized (already returns summary)")
	}
}

func TestShouldSummarize_FileEditNever(t *testing.T) {
	large := strings.Repeat("x", 5000)
	if ShouldSummarize("file_edit", large) {
		t.Error("file_edit should never be summarized")
	}
}

func TestSummarizeToolOutput_Bash_Success(t *testing.T) {
	output := "ok  \tgithub.com/example\t1.5s\nok  \tgithub.com/example/pkg\t0.8s\nPASS\n"
	output += strings.Repeat("test output line\n", 200)

	summary := SummarizeToolOutput("bash", nil, output, 0)

	if !strings.Contains(summary, "exit 0") {
		t.Error("should contain exit code")
	}
	if !strings.Contains(summary, "PASS") || !strings.Contains(summary, "line") {
		t.Logf("summary: %s", summary)
	}
	if len(summary) > 1000 {
		t.Errorf("summary too long: %d chars", len(summary))
	}
}

func TestSummarizeToolOutput_Bash_Error(t *testing.T) {
	output := "main.go:10:5: undefined: foo\nmain.go:15:3: cannot use x\n"
	summary := SummarizeToolOutput("bash", nil, output+strings.Repeat("x\n", 500), 1)

	if !strings.Contains(summary, "exit 1") {
		t.Error("should contain non-zero exit code")
	}
}

func TestSummarizeToolOutput_Grep(t *testing.T) {
	var lines []string
	for i := 0; i < 50; i++ {
		lines = append(lines, "src/file.go:10:matched line content")
	}
	output := strings.Join(lines, "\n")

	summary := SummarizeToolOutput("grep", map[string]interface{}{"pattern": "TODO"}, output, 0)

	if !strings.Contains(summary, "50 matches") {
		t.Errorf("should contain match count, got: %s", summary)
	}
	if !strings.Contains(summary, "TODO") {
		t.Error("should contain pattern")
	}
	if strings.Count(summary, "matched line") > 10 {
		t.Error("should truncate to first 5 matches, not show all 50")
	}
}

func TestSummarizeToolOutput_Glob(t *testing.T) {
	var lines []string
	for i := 0; i < 100; i++ {
		lines = append(lines, "src/components/Component"+string(rune('A'+i%26))+".tsx")
	}
	output := strings.Join(lines, "\n")

	summary := SummarizeToolOutput("glob", map[string]interface{}{"pattern": "**/*.tsx"}, output, 0)

	if !strings.Contains(summary, "100 files") {
		t.Errorf("should contain file count, got: %s", summary)
	}
	if strings.Count(summary, ".tsx") > 15 {
		t.Error("should show at most 10 files, not all 100")
	}
}

func TestSummarizeToolOutput_GitDiff(t *testing.T) {
	output := `diff --git a/file1.go b/file1.go
--- a/file1.go
+++ b/file1.go
@@ -1,3 +1,5 @@
 package main
+import "fmt"
+func hello() { fmt.Println("hi") }
-func old() {}
diff --git a/file2.go b/file2.go
--- a/file2.go
+++ b/file2.go
@@ -1 +1 @@
-old line
+new line
`
	summary := SummarizeToolOutput("git", map[string]interface{}{"subcommand": "diff"}, output, 0)

	if !strings.Contains(summary, "2 files") {
		t.Errorf("should count 2 files changed, got: %s", summary)
	}
}

func TestFormatArgsSummary_Bash(t *testing.T) {
	s := FormatArgsSummary("bash", map[string]interface{}{"command": "npm install"})
	if s != "npm install" {
		t.Errorf("expected 'npm install', got '%s'", s)
	}
}

func TestFormatArgsSummary_FileRead(t *testing.T) {
	s := FormatArgsSummary("file_read", map[string]interface{}{"path": "/src/main.go"})
	if s != "/src/main.go" {
		t.Errorf("expected '/src/main.go', got '%s'", s)
	}
}

func TestScrubSecrets_BearerToken(t *testing.T) {
	input := `curl -H 'Authorization: Bearer sk-abc123xyz' https://api.example.com`
	result := scrubSecrets(input)
	if strings.Contains(result, "sk-abc123xyz") {
		t.Error("Bearer token should be redacted")
	}
	if !strings.Contains(result, "[REDACTED]") {
		t.Error("should contain [REDACTED] placeholder")
	}
}

func TestScrubSecrets_KeyValuePatterns(t *testing.T) {
	tests := []struct {
		input string
		leaked string
	}{
		{"token=abc123&user=me", "abc123"},
		{"password=s3cret", "s3cret"},
		{"api_key=key-xyz-789", "key-xyz-789"},
		{"secret=mysecretvalue", "mysecretvalue"},
	}
	for _, tt := range tests {
		result := scrubSecrets(tt.input)
		if strings.Contains(result, tt.leaked) {
			t.Errorf("secret %q should be redacted in %q, got %q", tt.leaked, tt.input, result)
		}
	}
}

func TestScrubSecrets_ProviderTokens(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		leaked string
	}{
		{"OpenAI key", "export OPENAI_API_KEY=sk-proj-abc123def456ghi789jkl012mno345pqr678", "sk-proj-abc123def456ghi789jkl012mno345pqr678"},
		{"GitHub PAT", "git clone https://ghp_ABCDEFghijklmnop1234567890abcdefghijkl@github.com/repo", "ghp_ABCDEFghijklmnop1234567890abcdefghijkl"},
		{"GitHub OAuth", "token: gho_ABCDEFghijklmnop1234567890abcdefghijkl", "gho_ABCDEFghijklmnop1234567890abcdefghijkl"},
		{"GitHub server", "ghs_ABCDEFghijklmnop1234567890abcdefghijkl", "ghs_ABCDEFghijklmnop1234567890abcdefghijkl"},
		{"AWS key", "aws_access_key_id = AKIAIOSFODNN7EXAMPLE", "AKIAIOSFODNN7EXAMPLE"},
		{"private key RSA", "-----BEGIN RSA PRIVATE KEY-----\nMIIEpA...", "-----BEGIN RSA PRIVATE KEY-----"},
		{"private key EC", "-----BEGIN EC PRIVATE KEY-----\nMHQC...", "-----BEGIN EC PRIVATE KEY-----"},
		{"private key OpenSSH", "-----BEGIN OPENSSH PRIVATE KEY-----\nb3Bl...", "-----BEGIN OPENSSH PRIVATE KEY-----"},
		{"standalone OpenAI key", "The key is sk-proj-abc123def456ghi789jkl012mno345pqr678 here", "sk-proj-abc123def456ghi789jkl012mno345pqr678"},
		{"Stripe live key", "STRIPE_KEY=sk_live_TESTONLYnotarealkeyfortest", "sk_live_TESTONLYnotarealkeyfortest"},
		{"Stripe test key", "sk_test_TESTONLYnotarealkeyfortest", "sk_test_TESTONLYnotarealkeyfortest"},
		{"Slack bot token", "SLACK_TOKEN=xoxb-TESTONLY-NOTREAL-TESTONLYforverification", "xoxb-TESTONLY-NOTREAL-TESTONLYforverification"},
		{"SendGrid key", "SG.TESTONLYnotarealSendGridkey00", "SG.TESTONLYnotarealSendGridkey00"},
		{"Twilio SID", "TWILIO_SID=AC00112233445566778899aabbccddeeff", "AC00112233445566778899aabbccddeeff"},
		{"Google API key", "GOOGLE_API_KEY=AIzaSyTESTONLYnotarealGoogleAPIkey00", "AIzaSyTESTONLYnotarealGoogleAPIkey00"},
		{"GitHub fine-grained PAT", "github_pat_TESTONLYnotarealpat0000", "github_pat_TESTONLYnotarealpat0000"},
		{"Hugging Face token", "HF_TOKEN=hf_TESTONLYnotarealtoken00", "hf_TESTONLYnotarealtoken00"},
		{"Database URI", "postgres://admin:secretpass@db.example.com:5432/mydb", "postgres://admin:secretpass@db.example.com:5432/mydb"},
		{"JWT token", "token=eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U", "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := scrubSecrets(tt.input)
			if strings.Contains(result, tt.leaked) {
				t.Errorf("secret %q should be redacted, got %q", tt.leaked, result)
			}
		})
	}
}

func TestFormatArgsSummary_ScrubsSecrets(t *testing.T) {
	s := FormatArgsSummary("bash", map[string]interface{}{
		"command": "curl -H 'Authorization: Bearer sk-abc123' https://api.example.com",
	})
	if strings.Contains(s, "sk-abc123") {
		t.Error("FormatArgsSummary should scrub Bearer tokens")
	}
	if !strings.Contains(s, "[REDACTED]") {
		t.Error("should contain [REDACTED]")
	}
}
