package subscriptions

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const (
	claudeClientID           = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"
	claudeAuthURL            = "https://claude.ai/oauth/authorize"
	claudeTokenURL           = "https://platform.claude.com/v1/oauth/token"
	claudeRedirectFormat     = "http://localhost:%d/callback"
	claudeManagedRedirectURI = "https://platform.claude.com/oauth/code/callback"
	claudeDefaultPort        = 54545

	codexClientID       = "app_EMoamEEZ73f0CkXaXp7hrann"
	codexAuthURL        = "https://auth.openai.com/oauth/authorize"
	codexTokenURL       = "https://auth.openai.com/oauth/token"
	codexRedirectFormat = "http://localhost:%d/auth/callback"
	codexDefaultPort    = 1455

	geminiClientID       = "681255809395-oo8ft2oprdrnp9e3aqf6av3hmdib135j.apps.googleusercontent.com"
	geminiClientSecretDefault = "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"
	geminiAuthURL        = "https://accounts.google.com/o/oauth2/v2/auth"
	geminiTokenURL       = "https://oauth2.googleapis.com/token"
	geminiUserInfoURL    = "https://www.googleapis.com/oauth2/v1/userinfo?alt=json"
	geminiCallbackFormat = "http://127.0.0.1:%d/oauth2callback"
	geminiDefaultPort    = 8085

	loginTimeout = 5 * time.Minute
)

func geminiClientSecret() string {
	if s := strings.TrimSpace(os.Getenv("GEMINI_CLIENT_SECRET")); s != "" {
		return s
	}
	return geminiClientSecretDefault
}

type LoginOptions struct {
	CallbackPort int
	NoBrowser    bool
	Timeout      time.Duration
	Metadata     map[string]string
}

type oauthCallbackResult struct {
	Code        string
	State       string
	Error       string
	ErrorDetail string
}

type oauthServer struct {
	server    *http.Server
	callbacks chan oauthCallbackResult
	errors    chan error
}

func Login(ctx context.Context, provider string, opts LoginOptions) (ProviderCredential, error) {
	switch normalizeStoredProviderName(provider) {
	case "anthropic":
		return loginWithClaude(ctx, opts)
	case "openai":
		return loginWithCodex(ctx, opts)
	case "gemini":
		return loginWithGemini(ctx, opts)
	default:
		return ProviderCredential{}, fmt.Errorf("unsupported provider %q", provider)
	}
}

func loginWithClaude(ctx context.Context, opts LoginOptions) (ProviderCredential, error) {
	pkce, err := generatePKCE()
	if err != nil {
		return ProviderCredential{}, err
	}

	state, err := generateRandomState()
	if err != nil {
		return ProviderCredential{}, err
	}

	port := claudeDefaultPort
	if opts.CallbackPort > 0 {
		port = opts.CallbackPort
	}
	redirectURI := fmt.Sprintf(claudeRedirectFormat, port)

	server, err := startOAuthServer(port, "/callback")
	if err != nil {
		return ProviderCredential{}, err
	}
	defer func() {
		ctxShutdown, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.stop(ctxShutdown)
	}()

	authURL := buildClaudeAuthURL(state, pkce.challenge, redirectURI)
	oauthDebugf("anthropic interactive auth start redirect_uri=%q state=%s auth_url=%s", redirectURI, redactOAuthValue(state), redactOAuthURL(authURL))
	if err := startBrowserIfAvailable(authURL, opts.NoBrowser); err != nil {
		return ProviderCredential{}, err
	}

	result, err := server.waitForCallback(effectiveLoginTimeout(opts.Timeout))
	if err != nil {
		return ProviderCredential{}, err
	}
	if result.Error != "" {
		if result.ErrorDetail != "" {
			return ProviderCredential{}, fmt.Errorf("%s: %s", result.Error, result.ErrorDetail)
		}
		return ProviderCredential{}, fmt.Errorf("%s", result.Error)
	}
	if result.State != state {
		return ProviderCredential{}, fmt.Errorf("oauth state mismatch: expected %q, got %q", state, result.State)
	}

	access, err := exchangeClaudeToken(ctx, result.Code, state, pkce.verifier, redirectURI)
	if err != nil {
		return ProviderCredential{}, err
	}
	return access, nil
}

func loginWithCodex(ctx context.Context, opts LoginOptions) (ProviderCredential, error) {
	pkce, err := generatePKCE()
	if err != nil {
		return ProviderCredential{}, err
	}

	state, err := generateRandomState()
	if err != nil {
		return ProviderCredential{}, err
	}

	port := codexDefaultPort
	if opts.CallbackPort > 0 {
		port = opts.CallbackPort
	}
	redirectURI := fmt.Sprintf(codexRedirectFormat, port)

	server, err := startOAuthServer(port, "/auth/callback")
	if err != nil {
		return ProviderCredential{}, err
	}
	defer func() {
		ctxShutdown, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.stop(ctxShutdown)
	}()

	authURL := buildCodexAuthURL(state, pkce.challenge, redirectURI)
	oauthDebugf("openai interactive auth start redirect_uri=%q state=%s auth_url=%s", redirectURI, redactOAuthValue(state), redactOAuthURL(authURL))
	if err := startBrowserIfAvailable(authURL, opts.NoBrowser); err != nil {
		return ProviderCredential{}, err
	}

	result, err := server.waitForCallback(effectiveLoginTimeout(opts.Timeout))
	if err != nil {
		return ProviderCredential{}, err
	}
	if result.Error != "" {
		if result.ErrorDetail != "" {
			return ProviderCredential{}, fmt.Errorf("%s: %s", result.Error, result.ErrorDetail)
		}
		return ProviderCredential{}, fmt.Errorf("%s", result.Error)
	}
	if result.State != state {
		return ProviderCredential{}, fmt.Errorf("oauth state mismatch: expected %q, got %q", state, result.State)
	}

	access, err := exchangeCodexToken(ctx, result.Code, pkce.verifier, redirectURI)
	if err != nil {
		return ProviderCredential{}, err
	}
	return access, nil
}

func loginWithGemini(ctx context.Context, opts LoginOptions) (ProviderCredential, error) {
	state, err := generateRandomState()
	if err != nil {
		return ProviderCredential{}, err
	}

	port := geminiDefaultPort
	if opts.CallbackPort > 0 {
		port = opts.CallbackPort
	}
	redirectURI := fmt.Sprintf(geminiCallbackFormat, port)

	server, err := startOAuthServer(port, "/oauth2callback")
	if err != nil {
		return ProviderCredential{}, err
	}
	defer func() {
		ctxShutdown, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.stop(ctxShutdown)
	}()

	authURL := buildGeminiAuthURL(state, redirectURI)
	oauthDebugf("gemini interactive auth start redirect_uri=%q state=%s auth_url=%s", redirectURI, redactOAuthValue(state), redactOAuthURL(authURL))
	if err := startBrowserIfAvailable(authURL, opts.NoBrowser); err != nil {
		return ProviderCredential{}, err
	}

	result, err := server.waitForCallback(effectiveLoginTimeout(opts.Timeout))
	if err != nil {
		return ProviderCredential{}, err
	}
	if result.Error != "" {
		if result.ErrorDetail != "" {
			return ProviderCredential{}, fmt.Errorf("%s: %s", result.Error, result.ErrorDetail)
		}
		return ProviderCredential{}, fmt.Errorf("%s", result.Error)
	}
	if result.State != state {
		return ProviderCredential{}, fmt.Errorf("oauth state mismatch: expected %q, got %q", state, result.State)
	}

	access, err := exchangeGeminiToken(ctx, result.Code, redirectURI)
	if err != nil {
		return ProviderCredential{}, err
	}
	if opts.Metadata != nil {
		access.ProjectID = strings.TrimSpace(opts.Metadata["project_id"])
	}
	return access, nil
}

func buildClaudeAuthURL(state, codeChallenge, redirectURI string) string {
	params := url.Values{
		"code":                  []string{"true"},
		"client_id":             []string{claudeClientID},
		"response_type":         []string{"code"},
		"redirect_uri":          []string{redirectURI},
		"scope":                 []string{"org:create_api_key user:profile user:inference user:sessions:claude_code user:mcp_servers"},
		"code_challenge":        []string{codeChallenge},
		"code_challenge_method": []string{"S256"},
		"state":                 []string{state},
	}
	return fmt.Sprintf("%s?%s", claudeAuthURL, params.Encode())
}

func buildCodexAuthURL(state, codeChallenge, redirectURI string) string {
	params := url.Values{
		"client_id":                  []string{codexClientID},
		"response_type":              []string{"code"},
		"redirect_uri":               []string{redirectURI},
		"scope":                      []string{"openid email profile offline_access"},
		"state":                      []string{state},
		"code_challenge":             []string{codeChallenge},
		"code_challenge_method":      []string{"S256"},
		"prompt":                     []string{"login"},
		"id_token_add_organizations": []string{"true"},
		"codex_cli_simplified_flow":  []string{"true"},
	}
	return fmt.Sprintf("%s?%s", codexAuthURL, params.Encode())
}

func buildGeminiAuthURL(state, redirectURI string) string {
	params := url.Values{
		"client_id":     []string{geminiClientID},
		"redirect_uri":  []string{redirectURI},
		"response_type": []string{"code"},
		"scope":         []string{"https://www.googleapis.com/auth/cloud-platform https://www.googleapis.com/auth/userinfo.email https://www.googleapis.com/auth/userinfo.profile"},
		"access_type":   []string{"offline"},
		"state":         []string{state},
	}
	return fmt.Sprintf("%s?%s", geminiAuthURL, params.Encode())
}

func exchangeClaudeToken(ctx context.Context, code, state, codeVerifier, redirectURI string) (ProviderCredential, error) {
	code, state = normalizeClaudeCodeAndState(code, state)
	payload := map[string]string{
		"code":          strings.TrimSpace(code),
		"state":         strings.TrimSpace(state),
		"grant_type":    "authorization_code",
		"client_id":     claudeClientID,
		"redirect_uri":  redirectURI,
		"code_verifier": codeVerifier,
	}
	body, _ := json.Marshal(payload)
	oauthDebugf("anthropic token exchange endpoint=%q redirect_uri=%q state=%s code_len=%d verifier_len=%d", claudeTokenURL, redirectURI, redactOAuthValue(state), len(strings.TrimSpace(code)), len(strings.TrimSpace(codeVerifier)))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, claudeTokenURL, strings.NewReader(string(body)))
	if err != nil {
		return ProviderCredential{}, fmt.Errorf("create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return ProviderCredential{}, fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return ProviderCredential{}, fmt.Errorf("read token response: %w", err)
	}
	oauthDebugf("anthropic token exchange status=%d body=%s", resp.StatusCode, redactOAuthJSON(bodyBytes))
	if resp.StatusCode != http.StatusOK {
		return ProviderCredential{}, fmt.Errorf("token exchange failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}

	var token struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.Unmarshal(bodyBytes, &token); err != nil {
		return ProviderCredential{}, fmt.Errorf("parse token response: %w", err)
	}
	if strings.TrimSpace(token.AccessToken) == "" {
		return ProviderCredential{}, fmt.Errorf("empty access token in response")
	}

	credential := ProviderCredential{
		AccessToken:    strings.TrimSpace(token.AccessToken),
		RefreshToken:   strings.TrimSpace(token.RefreshToken),
		TokenType:      strings.TrimSpace(token.TokenType),
		CredentialType: credentialTypeBearer,
		ExpiresAt:      time.Now().Add(time.Duration(token.ExpiresIn) * time.Second).UTC().Format(time.RFC3339),
	}
	if strings.EqualFold(credential.TokenType, "token") {
		credential.TokenType = "Bearer"
	}
	return credential, nil
}

func exchangeCodexToken(ctx context.Context, code, codeVerifier, redirectURI string) (ProviderCredential, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {codexClientID},
		"code":          {strings.TrimSpace(code)},
		"redirect_uri":  {redirectURI},
		"code_verifier": {codeVerifier},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, codexTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return ProviderCredential{}, fmt.Errorf("create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return ProviderCredential{}, fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return ProviderCredential{}, fmt.Errorf("read token response: %w", err)
	}
	oauthDebugf("openai token exchange status=%d redirect_uri=%q code_len=%d verifier_len=%d body=%s", resp.StatusCode, redirectURI, len(strings.TrimSpace(code)), len(strings.TrimSpace(codeVerifier)), redactOAuthJSON(bodyBytes))
	if resp.StatusCode != http.StatusOK {
		return ProviderCredential{}, fmt.Errorf("token exchange failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}

	var token struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.Unmarshal(bodyBytes, &token); err != nil {
		return ProviderCredential{}, fmt.Errorf("parse token response: %w", err)
	}
	if strings.TrimSpace(token.AccessToken) == "" {
		return ProviderCredential{}, fmt.Errorf("empty access token in response")
	}

	return ProviderCredential{
		AccessToken:    strings.TrimSpace(token.AccessToken),
		RefreshToken:   strings.TrimSpace(token.RefreshToken),
		TokenType:      strings.TrimSpace(token.TokenType),
		CredentialType: credentialTypeBearer,
		ExpiresAt:      time.Now().Add(time.Duration(token.ExpiresIn) * time.Second).UTC().Format(time.RFC3339),
	}, nil
}

func exchangeGeminiToken(ctx context.Context, code, redirectURI string) (ProviderCredential, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {geminiClientID},
		"client_secret": {geminiClientSecret()},
		"code":          {strings.TrimSpace(code)},
		"redirect_uri":  {redirectURI},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, geminiTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return ProviderCredential{}, fmt.Errorf("create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return ProviderCredential{}, fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return ProviderCredential{}, fmt.Errorf("read token response: %w", err)
	}
	oauthDebugf("gemini token exchange status=%d redirect_uri=%q code_len=%d body=%s", resp.StatusCode, redirectURI, len(strings.TrimSpace(code)), redactOAuthJSON(bodyBytes))
	if resp.StatusCode != http.StatusOK {
		return ProviderCredential{}, fmt.Errorf("token exchange failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}

	var token struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
		IDToken      string `json:"id_token"`
	}
	if err := json.Unmarshal(bodyBytes, &token); err != nil {
		return ProviderCredential{}, fmt.Errorf("parse token response: %w", err)
	}
	if strings.TrimSpace(token.AccessToken) == "" {
		return ProviderCredential{}, fmt.Errorf("empty access token in response")
	}

	userEmail, err := fetchGeminiEmail(ctx, token.AccessToken)
	if err != nil {
		return ProviderCredential{}, err
	}

	return ProviderCredential{
		AccessToken:    strings.TrimSpace(token.AccessToken),
		RefreshToken:   strings.TrimSpace(token.RefreshToken),
		TokenType:      strings.TrimSpace(token.TokenType),
		Email:          userEmail,
		CredentialType: credentialTypeBearer,
		ExpiresAt:      time.Now().Add(time.Duration(token.ExpiresIn) * time.Second).UTC().Format(time.RFC3339),
	}, nil
}

func fetchGeminiEmail(ctx context.Context, token string) (string, error) {
	if strings.TrimSpace(token) == "" {
		return "", nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, geminiUserInfoURL, nil)
	if err != nil {
		return "", fmt.Errorf("create userinfo request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return "", nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil
	}
	if resp.StatusCode != http.StatusOK {
		return "", nil
	}
	var payload struct {
		Email string `json:"email"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", nil
	}
	return payload.Email, nil
}

func startOAuthServer(port int, callbackPath string) (*oauthServer, error) {
	server := &oauthServer{
		callbacks: make(chan oauthCallbackResult, 1),
		errors:    make(chan error, 1),
	}
	mux := http.NewServeMux()
	mux.HandleFunc(callbackPath, server.handleCallback)
	server.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	go func() {
		_ = server.server.ListenAndServe()
	}()

	return server, nil
}

func (o *oauthServer) stop(ctx context.Context) error {
	if o.server == nil {
		return nil
	}
	return o.server.Shutdown(ctx)
}

func (o *oauthServer) waitForCallback(timeout time.Duration) (oauthCallbackResult, error) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case callback := <-o.callbacks:
		return callback, nil
	case err := <-o.errors:
		return oauthCallbackResult{}, err
	case <-timer.C:
		return oauthCallbackResult{}, fmt.Errorf("oauth flow timed out after %s", timeout)
	}
}

func (o *oauthServer) handleCallback(w http.ResponseWriter, r *http.Request) {
	callback := parseOAuthCallback(r.URL)
	oauthDebugf("oauth callback path=%q state=%s code_len=%d error=%q error_detail=%q", r.URL.Path, redactOAuthValue(callback.State), len(callback.Code), callback.Error, callback.ErrorDetail)
	if callback.Error != "" {
		o.sendError(fmt.Errorf("%s: %s", callback.Error, callback.ErrorDetail))
		WriteOAuthCallbackHTML(w, http.StatusBadRequest, false, callback.Error, callback.ErrorDetail)
		return
	}

	select {
	case o.callbacks <- callback:
	default:
	}

	WriteOAuthCallbackHTML(w, http.StatusOK, true, "", "")
}

func (o *oauthServer) sendError(err error) {
	select {
	case o.errors <- err:
	default:
	}
}

func parseOAuthCallback(u *url.URL) oauthCallbackResult {
	if u == nil {
		return oauthCallbackResult{Error: "invalid callback"}
	}
	query := u.Query()
	result := oauthCallbackResult{
		Code:        strings.TrimSpace(query.Get("code")),
		State:       strings.TrimSpace(query.Get("state")),
		Error:       strings.TrimSpace(query.Get("error")),
		ErrorDetail: strings.TrimSpace(query.Get("error_description")),
	}
	if result.Error == "" && result.Code == "" && u.Fragment != "" {
		if fragment, err := url.ParseQuery(u.Fragment); err == nil {
			if result.Code == "" {
				result.Code = strings.TrimSpace(fragment.Get("code"))
			}
			if result.State == "" {
				result.State = strings.TrimSpace(fragment.Get("state"))
			}
			if result.Error == "" {
				result.Error = strings.TrimSpace(fragment.Get("error"))
			}
		}
	}
	if result.Code != "" && result.State == "" && strings.Contains(result.Code, "#") {
		parts := strings.SplitN(result.Code, "#", 2)
		result.Code = strings.TrimSpace(parts[0])
		result.State = strings.TrimSpace(parts[1])
	}
	return result
}

func generateRandomState() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", fmt.Errorf("generate random state: %w", err)
	}
	return hex.EncodeToString(bytes[:]), nil
}

func normalizeClaudeCodeAndState(rawCode, fallbackState string) (string, string) {
	rawCode = strings.TrimSpace(rawCode)
	fallbackState = strings.TrimSpace(fallbackState)
	if rawCode == "" {
		return "", fallbackState
	}
	if !strings.Contains(rawCode, "#") {
		return rawCode, fallbackState
	}
	parts := strings.SplitN(rawCode, "#", 2)
	code := strings.TrimSpace(parts[0])
	state := fallbackState
	if len(parts) > 1 && strings.TrimSpace(parts[1]) != "" {
		state = strings.TrimSpace(parts[1])
	}
	return code, state
}

func WriteOAuthCallbackHTML(w http.ResponseWriter, status int, ok bool, errCode, errDetail string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if ok {
		_, _ = w.Write([]byte("<!doctype html><html><body><h1>Authentication complete</h1><p>You can close this window.</p></body></html>"))
		return
	}

	message := strings.TrimSpace(errCode)
	if message == "" {
		message = "Authentication failed"
	}
	detail := strings.TrimSpace(errDetail)
	if detail == "" || detail == message {
		_, _ = w.Write([]byte(fmt.Sprintf("<!doctype html><html><body><h1>Authentication failed</h1><p>%s</p></body></html>", message)))
		return
	}
	_, _ = w.Write([]byte(fmt.Sprintf("<!doctype html><html><body><h1>Authentication failed</h1><p>%s</p><p>%s</p></body></html>", message, detail)))
}

func oauthDebugf(format string, args ...interface{}) {
	if strings.TrimSpace(os.Getenv("SYNROUTE_OAUTH_DEBUG")) == "" {
		return
	}
	log.Printf("[OAuthDebug] "+format, args...)
}

func redactOAuthURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return raw
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	query := parsed.Query()
	for _, key := range []string{"state", "code", "code_challenge", "client_secret"} {
		if query.Has(key) {
			query.Set(key, redactOAuthValue(query.Get(key)))
		}
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func redactOAuthJSON(body []byte) string {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return ""
	}

	var payload interface{}
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return trimmed
	}
	redactOAuthJSONValue(payload)
	sanitized, err := json.Marshal(payload)
	if err != nil {
		return trimmed
	}
	return string(sanitized)
}

func redactOAuthJSONValue(value interface{}) {
	switch typed := value.(type) {
	case map[string]interface{}:
		for key, nested := range typed {
			lower := strings.ToLower(strings.TrimSpace(key))
			switch lower {
			case "access_token", "refresh_token", "id_token", "code", "client_secret":
				if text, ok := nested.(string); ok {
					typed[key] = redactOAuthValue(text)
				}
			default:
				redactOAuthJSONValue(nested)
			}
		}
	case []interface{}:
		for _, nested := range typed {
			redactOAuthJSONValue(nested)
		}
	}
}

func redactOAuthValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) <= 8 {
		return "[redacted]"
	}
	return value[:4] + "..." + value[len(value)-4:]
}

type pkceBundle struct {
	verifier  string
	challenge string
}

func generatePKCE() (pkceBundle, error) {
	verifierBytes := make([]byte, 96)
	if _, err := rand.Read(verifierBytes); err != nil {
		return pkceBundle{}, fmt.Errorf("generate pkce verifier: %w", err)
	}
	verifier := base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(verifierBytes)
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(sum[:])
	return pkceBundle{verifier: verifier, challenge: challenge}, nil
}

func startBrowserIfAvailable(rawURL string, noBrowser bool) error {
	if noBrowser {
		return nil
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", rawURL)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL)
	default:
		cmd = exec.Command("xdg-open", rawURL)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to open browser: %w", err)
	}
	return nil
}

func effectiveLoginTimeout(raw time.Duration) time.Duration {
	if raw <= 0 {
		return loginTimeout
	}
	return raw
}
