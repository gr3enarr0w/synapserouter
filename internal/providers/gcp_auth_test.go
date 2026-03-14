package providers

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func generateTestKeyPEM(t *testing.T) ([]byte, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal PKCS8: %v", err)
	}
	pemBlock := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	return pemBlock, key
}

func TestNativeServiceAccountToken(t *testing.T) {
	keyPEM, _ := generateTestKeyPEM(t)

	// Mock token endpoint
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		if r.FormValue("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" {
			t.Errorf("unexpected grant_type: %s", r.FormValue("grant_type"))
		}
		if r.FormValue("assertion") == "" {
			t.Error("missing assertion")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"access_token": "test-sa-token-123"})
	}))
	defer ts.Close()

	// Write temp SA key file pointing to mock endpoint
	saKey := map[string]string{
		"type":         "service_account",
		"client_email": "test@test-project.iam.gserviceaccount.com",
		"private_key":  string(keyPEM),
		"token_uri":    ts.URL,
	}
	saBytes, _ := json.Marshal(saKey)
	tmpFile := filepath.Join(t.TempDir(), "sa-key.json")
	if err := os.WriteFile(tmpFile, saBytes, 0600); err != nil {
		t.Fatalf("write temp SA key: %v", err)
	}

	token, err := nativeServiceAccountTokenWithClient(tmpFile, ts.Client())
	if err != nil {
		t.Fatalf("nativeServiceAccountToken: %v", err)
	}
	if token != "test-sa-token-123" {
		t.Errorf("expected 'test-sa-token-123', got %q", token)
	}
}

func TestNativeADCToken_AuthorizedUser(t *testing.T) {
	// Mock OAuth2 token endpoint
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		if r.FormValue("grant_type") != "refresh_token" {
			t.Errorf("unexpected grant_type: %s", r.FormValue("grant_type"))
		}
		if r.FormValue("client_id") != "test-client-id" {
			t.Errorf("unexpected client_id: %s", r.FormValue("client_id"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"access_token": "test-user-token-456"})
	}))
	defer ts.Close()

	// Write temp ADC file
	adc := map[string]string{
		"type":          "authorized_user",
		"client_id":     "test-client-id",
		"client_secret": "test-client-secret",
		"refresh_token": "test-refresh-token",
	}
	adcBytes, _ := json.Marshal(adc)
	tmpFile := filepath.Join(t.TempDir(), "adc.json")
	if err := os.WriteFile(tmpFile, adcBytes, 0600); err != nil {
		t.Fatalf("write temp ADC: %v", err)
	}

	// refreshAuthorizedUserToken talks to the real Google endpoint by default,
	// so we test the plumbing via loadCredentialFile + a patched client
	token, err := refreshAuthorizedUserToken(ts.Client(), ts.URL, "test-client-id", "test-client-secret", "test-refresh-token")
	if err != nil {
		t.Fatalf("refreshAuthorizedUserToken: %v", err)
	}
	if token != "test-user-token-456" {
		t.Errorf("expected 'test-user-token-456', got %q", token)
	}
}

func TestNativeADCToken_ServiceAccount(t *testing.T) {
	keyPEM, _ := generateTestKeyPEM(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"access_token": "test-sa-via-adc-789"})
	}))
	defer ts.Close()

	saKey := map[string]string{
		"type":         "service_account",
		"client_email": "test@test-project.iam.gserviceaccount.com",
		"private_key":  string(keyPEM),
		"token_uri":    ts.URL,
	}
	saBytes, _ := json.Marshal(saKey)
	tmpFile := filepath.Join(t.TempDir(), "sa-key.json")
	if err := os.WriteFile(tmpFile, saBytes, 0600); err != nil {
		t.Fatalf("write temp SA key: %v", err)
	}

	// Set env var and test via loadCredentialFile
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", tmpFile)

	token, err := loadCredentialFile(tmpFile, ts.Client())
	if err != nil {
		t.Fatalf("loadCredentialFile (SA): %v", err)
	}
	if token != "test-sa-via-adc-789" {
		t.Errorf("expected 'test-sa-via-adc-789', got %q", token)
	}
}

func TestNativeADCToken_NoCredentials(t *testing.T) {
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")
	// Override HOME to a temp dir with no .config/gcloud
	t.Setenv("HOME", t.TempDir())

	_, err := nativeADCTokenWithClient(http.DefaultClient)
	if err == nil {
		t.Fatal("expected error when no credentials available")
	}
	if got := err.Error(); !contains(got, "no GCP credentials found") {
		t.Errorf("expected 'no GCP credentials found' in error, got: %s", got)
	}
}

func TestParseRSAKey_PKCS8(t *testing.T) {
	keyPEM, original := generateTestKeyPEM(t)
	parsed, err := parseRSAPrivateKey(keyPEM)
	if err != nil {
		t.Fatalf("parseRSAPrivateKey (PKCS8): %v", err)
	}
	if !original.Equal(parsed) {
		t.Error("parsed key does not match original")
	}
}

func TestParseRSAKey_PKCS1(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	der := x509.MarshalPKCS1PrivateKey(key)
	pemBlock := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: der})

	parsed, err := parseRSAPrivateKey(pemBlock)
	if err != nil {
		t.Fatalf("parseRSAPrivateKey (PKCS1): %v", err)
	}
	if !key.Equal(parsed) {
		t.Error("parsed key does not match original")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsImpl(s, substr))
}

func containsImpl(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
