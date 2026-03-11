package subscriptions

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestStoreCredentialReplacesByRefreshToken(t *testing.T) {
	t.Setenv("SYNROUTE_SUBSCRIPTION_CREDENTIAL_STORE", t.TempDir()+"/credentials.json")

	first := ProviderCredential{
		AccessToken:    "access-1",
		RefreshToken:   "refresh-1",
		CredentialType: credentialTypeBearer,
		Email:          "user@example.com",
	}
	second := ProviderCredential{
		AccessToken:    "access-2",
		RefreshToken:   "refresh-1",
		CredentialType: credentialTypeBearer,
		Email:          "user@example.com",
	}

	if err := StoreCredential("openai", first); err != nil {
		t.Fatalf("store first credential: %v", err)
	}
	if err := StoreCredential("openai", second); err != nil {
		t.Fatalf("store second credential: %v", err)
	}

	stored, err := LoadStoredCredentials("openai")
	if err != nil {
		t.Fatalf("load stored credentials: %v", err)
	}
	if len(stored) != 1 {
		t.Fatalf("expected 1 stored credential, got %d", len(stored))
	}
	if stored[0].AccessToken != "access-2" {
		t.Fatalf("expected refreshed access token to replace existing record, got %q", stored[0].AccessToken)
	}
}

func TestOAuthCredentialsUseProviderSpecificHeaders(t *testing.T) {
	t.Run("anthropic access token uses bearer auth", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "https://api.anthropic.com/v1/messages", nil)
		p := &anthropicProvider{}
		p.applyCredential(req, ProviderCredential{
			AccessToken:    "anth-access",
			CredentialType: credentialTypeBearer,
		})

		if got := req.Header.Get("Authorization"); got != "Bearer anth-access" {
			t.Fatalf("expected bearer auth, got %q", got)
		}
		if got := req.Header.Get("x-api-key"); got != "" {
			t.Fatalf("expected no api key header, got %q", got)
		}
	})

	t.Run("gemini access token uses bearer auth", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "https://generativelanguage.googleapis.com/v1beta/models/test:generateContent", nil)
		p := &geminiProvider{}
		p.applyCredential(req, ProviderCredential{
			AccessToken:    "gem-access",
			CredentialType: credentialTypeBearer,
		})

		if got := req.Header.Get("Authorization"); got != "Bearer gem-access" {
			t.Fatalf("expected bearer auth, got %q", got)
		}
		if key := req.URL.Query().Get("key"); key != "" {
			t.Fatalf("expected no api key query parameter, got %q", key)
		}
	})
}

func TestGenerateRandomStateMatchesArchivedLength(t *testing.T) {
	state, err := generateRandomState()
	if err != nil {
		t.Fatalf("generate random state: %v", err)
	}
	if len(state) != 32 {
		t.Fatalf("expected 32-character oauth state, got %d (%q)", len(state), state)
	}
}

func TestNormalizeClaudeCodeAndState(t *testing.T) {
	code, state := normalizeClaudeCodeAndState("auth-code#callback-state", "fallback-state")
	if code != "auth-code" {
		t.Fatalf("expected auth-code, got %q", code)
	}
	if state != "callback-state" {
		t.Fatalf("expected callback-state, got %q", state)
	}

	code, state = normalizeClaudeCodeAndState("auth-code", "fallback-state")
	if code != "auth-code" {
		t.Fatalf("expected auth-code, got %q", code)
	}
	if state != "fallback-state" {
		t.Fatalf("expected fallback-state, got %q", state)
	}
}

func TestWaitForManagedLoginCallbackAllowsManualOnlySessions(t *testing.T) {
	session := &managedLoginSession{
		manual: make(chan oauthCallbackResult, 1),
	}
	session.manual <- oauthCallbackResult{Code: "manual-code", State: "manual-state"}

	callback, err := waitForManagedLoginCallback(session, time.Second)
	if err != nil {
		t.Fatalf("wait for managed login callback: %v", err)
	}
	if callback.Code != "manual-code" {
		t.Fatalf("expected manual code, got %q", callback.Code)
	}
	if callback.State != "manual-state" {
		t.Fatalf("expected manual state, got %q", callback.State)
	}
}
