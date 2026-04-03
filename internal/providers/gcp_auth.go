package providers

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// nativeServiceAccountToken generates an access token from a GCP service account JSON key file.
// Pure Go implementation — no external dependencies.
func nativeServiceAccountToken(keyFile string) (string, error) {
	return nativeServiceAccountTokenWithClient(keyFile, http.DefaultClient)
}

func nativeServiceAccountTokenWithClient(keyFile string, client *http.Client) (string, error) {
	data, err := os.ReadFile(keyFile) //nolint:G703 // keyFile is operator-configured path (env var or gcloud default)
	if err != nil {
		return "", fmt.Errorf("read service account key: %w", err)
	}

	var sa struct {
		ClientEmail string `json:"client_email"`
		PrivateKey  string `json:"private_key"`
		TokenURI    string `json:"token_uri"`
	}
	if err := json.Unmarshal(data, &sa); err != nil {
		return "", fmt.Errorf("parse service account key: %w", err)
	}
	if sa.TokenURI == "" {
		sa.TokenURI = "https://oauth2.googleapis.com/token"
	}

	key, err := parseRSAPrivateKey([]byte(sa.PrivateKey))
	if err != nil {
		return "", fmt.Errorf("parse private key: %w", err)
	}

	now := time.Now().Unix()
	header := base64Encode([]byte(`{"alg":"RS256","typ":"JWT"}`))
	claims, _ := json.Marshal(map[string]interface{}{
		"iss":   sa.ClientEmail,
		"scope": "https://www.googleapis.com/auth/cloud-platform",
		"aud":   sa.TokenURI,
		"iat":   now,
		"exp":   now + 3600,
	})
	claimsEnc := base64Encode(claims)

	sigInput := header + "." + claimsEnc
	hash := sha256.Sum256([]byte(sigInput))
	sig, err := rsa.SignPKCS1v15(nil, key, crypto.SHA256, hash[:])
	if err != nil {
		return "", fmt.Errorf("sign JWT: %w", err)
	}
	jwt := sigInput + "." + base64Encode(sig)

	return exchangeJWTForToken(client, sa.TokenURI, jwt)
}

// nativeADCToken gets an access token using Application Default Credentials.
// Checks GOOGLE_APPLICATION_CREDENTIALS, then ~/.config/gcloud/application_default_credentials.json.
func nativeADCToken() (string, error) {
	return nativeADCTokenWithClient(http.DefaultClient)
}

func nativeADCTokenWithClient(client *http.Client) (string, error) {
	// 1. Check GOOGLE_APPLICATION_CREDENTIALS
	if path := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"); path != "" {
		return loadCredentialFile(path, client)
	}

	// 2. Check default gcloud ADC location
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("no GCP credentials found: cannot determine home dir: %w", err)
	}
	adcPath := filepath.Join(home, ".config", "gcloud", "application_default_credentials.json")
	if _, err := os.Stat(adcPath); err != nil {
		return "", fmt.Errorf("no GCP credentials found: set GOOGLE_APPLICATION_CREDENTIALS or run 'gcloud auth application-default login'")
	}
	return loadCredentialFile(adcPath, client)
}

func loadCredentialFile(path string, client *http.Client) (string, error) {
	data, err := os.ReadFile(path) //nolint:G703 // path is operator-configured credential file location
	if err != nil {
		return "", fmt.Errorf("read credentials file: %w", err)
	}

	var cred struct {
		Type         string `json:"type"`
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.Unmarshal(data, &cred); err != nil {
		return "", fmt.Errorf("parse credentials file: %w", err)
	}

	switch cred.Type {
	case "service_account":
		return nativeServiceAccountTokenWithClient(path, client)
	case "authorized_user":
		return refreshAuthorizedUserToken(client, "https://oauth2.googleapis.com/token", cred.ClientID, cred.ClientSecret, cred.RefreshToken)
	default:
		return "", fmt.Errorf("unsupported credential type: %q", cred.Type)
	}
}

func refreshAuthorizedUserToken(client *http.Client, tokenURL, clientID, clientSecret, refreshToken string) (string, error) {
	resp, err := client.PostForm(tokenURL, url.Values{
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"refresh_token": {refreshToken},
		"grant_type":    {"refresh_token"},
	})
	if err != nil {
		return "", fmt.Errorf("refresh token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token refresh returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("parse token response: %w", err)
	}
	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("token refresh returned empty access_token")
	}
	return tokenResp.AccessToken, nil
}

func exchangeJWTForToken(client *http.Client, tokenURI, jwt string) (string, error) {
	resp, err := client.PostForm(tokenURI, url.Values{
		"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"},
		"assertion":  {jwt},
	})
	if err != nil {
		return "", fmt.Errorf("token exchange request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token exchange returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("parse token response: %w", err)
	}
	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("token exchange returned empty access_token")
	}
	return tokenResp.AccessToken, nil
}

func parseRSAPrivateKey(pemBytes []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found in private key")
	}

	// Try PKCS8 first (most common for GCP SA keys), then PKCS1
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		if rsaKey, ok := key.(*rsa.PrivateKey); ok {
			return rsaKey, nil
		}
		return nil, fmt.Errorf("PKCS8 key is not RSA")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	return nil, fmt.Errorf("failed to parse private key as PKCS8 or PKCS1")
}

func base64Encode(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}
