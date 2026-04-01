package subscriptions

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const credentialRefreshLead = 2 * time.Minute

func refreshCredentialIfNeeded(ctx context.Context, provider string, credential ProviderCredential) (ProviderCredential, bool) {
	if !credential.NeedsRefresh(credentialRefreshLead) {
		return credential, false
	}

	refreshed, err := RefreshCredential(ctx, provider, credential)
	if err != nil {
		return credential, false
	}
	_ = StoreCredential(provider, refreshed)
	return refreshed, true
}

func RefreshCredential(ctx context.Context, provider string, credential ProviderCredential) (ProviderCredential, error) {
	provider = normalizeStoredProviderName(provider)
	if provider == "" {
		return ProviderCredential{}, fmt.Errorf("unsupported provider %q", provider)
	}
	if strings.TrimSpace(credential.RefreshToken) == "" {
		return ProviderCredential{}, fmt.Errorf("provider %q credential has no refresh token", provider)
	}

	switch provider {
	case "anthropic":
		return ProviderCredential{}, fmt.Errorf("anthropic subscription removed — violates TOS")
	case "openai":
		return refreshCodexToken(ctx, credential)
	case "gemini":
		return refreshGeminiToken(ctx, credential)
	default:
		return ProviderCredential{}, fmt.Errorf("unsupported provider %q", provider)
	}
}

func refreshClaudeToken(ctx context.Context, credential ProviderCredential) (ProviderCredential, error) {
	payload := map[string]string{
		"client_id":     claudeClientID,
		"grant_type":    "refresh_token",
		"refresh_token": strings.TrimSpace(credential.RefreshToken),
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, claudeTokenURL, strings.NewReader(string(body)))
	if err != nil {
		return ProviderCredential{}, fmt.Errorf("create refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return ProviderCredential{}, fmt.Errorf("refresh request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return ProviderCredential{}, fmt.Errorf("read refresh response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return ProviderCredential{}, fmt.Errorf("refresh failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}

	var token struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.Unmarshal(bodyBytes, &token); err != nil {
		return ProviderCredential{}, fmt.Errorf("parse refresh response: %w", err)
	}
	if strings.TrimSpace(token.AccessToken) == "" {
		return ProviderCredential{}, fmt.Errorf("empty access token in refresh response")
	}

	refreshed := credential
	refreshed.AccessToken = strings.TrimSpace(token.AccessToken)
	if next := strings.TrimSpace(token.RefreshToken); next != "" {
		refreshed.RefreshToken = next
	}
	refreshed.TokenType = strings.TrimSpace(token.TokenType)
	refreshed.CredentialType = credentialTypeBearer
	refreshed.ExpiresAt = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second).UTC().Format(time.RFC3339)
	if strings.EqualFold(refreshed.TokenType, "token") {
		refreshed.TokenType = "Bearer"
	}
	return refreshed, nil
}

func refreshCodexToken(ctx context.Context, credential ProviderCredential) (ProviderCredential, error) {
	form := url.Values{
		"client_id":     {codexClientID},
		"grant_type":    {"refresh_token"},
		"refresh_token": {strings.TrimSpace(credential.RefreshToken)},
		"scope":         {"openid profile email"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, codexTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return ProviderCredential{}, fmt.Errorf("create refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return ProviderCredential{}, fmt.Errorf("refresh request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return ProviderCredential{}, fmt.Errorf("read refresh response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return ProviderCredential{}, fmt.Errorf("refresh failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}

	var token struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.Unmarshal(bodyBytes, &token); err != nil {
		return ProviderCredential{}, fmt.Errorf("parse refresh response: %w", err)
	}
	if strings.TrimSpace(token.AccessToken) == "" {
		return ProviderCredential{}, fmt.Errorf("empty access token in refresh response")
	}

	refreshed := credential
	refreshed.AccessToken = strings.TrimSpace(token.AccessToken)
	if next := strings.TrimSpace(token.RefreshToken); next != "" {
		refreshed.RefreshToken = next
	}
	refreshed.TokenType = strings.TrimSpace(token.TokenType)
	refreshed.CredentialType = credentialTypeBearer
	refreshed.ExpiresAt = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second).UTC().Format(time.RFC3339)
	return refreshed, nil
}

func refreshGeminiToken(ctx context.Context, credential ProviderCredential) (ProviderCredential, error) {
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {strings.TrimSpace(credential.RefreshToken)},
		"client_id":     {geminiClientID},
		"client_secret": {geminiClientSecret()},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, geminiTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return ProviderCredential{}, fmt.Errorf("create refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return ProviderCredential{}, fmt.Errorf("refresh request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return ProviderCredential{}, fmt.Errorf("read refresh response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return ProviderCredential{}, fmt.Errorf("refresh failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}

	var token struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.Unmarshal(bodyBytes, &token); err != nil {
		return ProviderCredential{}, fmt.Errorf("parse refresh response: %w", err)
	}
	if strings.TrimSpace(token.AccessToken) == "" {
		return ProviderCredential{}, fmt.Errorf("empty access token in refresh response")
	}

	refreshed := credential
	refreshed.AccessToken = strings.TrimSpace(token.AccessToken)
	if next := strings.TrimSpace(token.RefreshToken); next != "" {
		refreshed.RefreshToken = next
	}
	refreshed.TokenType = strings.TrimSpace(token.TokenType)
	refreshed.CredentialType = credentialTypeBearer
	refreshed.ExpiresAt = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second).UTC().Format(time.RFC3339)
	if strings.TrimSpace(refreshed.Email) == "" {
		if email, err := fetchGeminiEmail(ctx, refreshed.AccessToken); err == nil {
			refreshed.Email = strings.TrimSpace(email)
		}
	}
	return refreshed, nil
}
