package subscriptions

import (
	"strings"
	"time"
)

const (
	credentialTypeAPIKey  = "api_key"
	credentialTypeBearer  = "bearer"
	credentialTypeSession = "session"
	credentialTypeUnknown = "unknown"
)

type ProviderCredential struct {
	APIKey         string
	SessionToken   string
	AccessToken    string
	RefreshToken   string
	TokenType      string
	CredentialType string
	ExpiresAt      string
	Email          string
	ProjectID      string
}

func (c ProviderCredential) credentialType() string {
	switch normalizeCredentialType(c.CredentialType) {
	case credentialTypeSession:
		return credentialTypeSession
	case credentialTypeBearer:
		return credentialTypeBearer
	case credentialTypeAPIKey, credentialTypeUnknown, "":
		if c.AccessToken != "" && c.APIKey == "" && c.SessionToken == "" {
			return credentialTypeBearer
		}
		if c.SessionToken != "" {
			return credentialTypeSession
		}
		return credentialTypeAPIKey
	default:
		return credentialTypeUnknown
	}
}

func (c ProviderCredential) authToken() string {
	if token := strings.TrimSpace(c.AccessToken); token != "" {
		return token
	}
	if token := strings.TrimSpace(c.APIKey); token != "" {
		return token
	}
	return strings.TrimSpace(c.SessionToken)
}

func (c ProviderCredential) isSessionCredential() bool {
	return c.credentialType() == credentialTypeSession
}

func (c ProviderCredential) isBearerCredential() bool {
	typ := c.credentialType()
	return typ == credentialTypeBearer || typ == credentialTypeAPIKey && c.AccessToken == "" && c.APIKey != ""
}

func (c ProviderCredential) UniqueKey() string {
	parts := []string{
		normalizeCredentialType(c.CredentialType),
		strings.TrimSpace(c.APIKey),
		strings.TrimSpace(c.SessionToken),
		strings.TrimSpace(c.AccessToken),
		strings.TrimSpace(c.RefreshToken),
	}
	return strings.Join(parts, "\x00")
}

func (c ProviderCredential) expiryTime() (time.Time, bool) {
	expiresAt := strings.TrimSpace(c.ExpiresAt)
	if expiresAt == "" {
		return time.Time{}, false
	}

	ts, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		return time.Time{}, false
	}
	return ts.UTC(), true
}

func (c ProviderCredential) NeedsRefresh(lead time.Duration) bool {
	if strings.TrimSpace(c.RefreshToken) == "" || !c.isBearerCredential() {
		return false
	}

	expiresAt, ok := c.expiryTime()
	if !ok {
		return false
	}
	if lead < 0 {
		lead = 0
	}
	return !time.Now().UTC().Before(expiresAt.Add(-lead))
}

func normalizeCredentialType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case credentialTypeBearer, "bearer_token", "oauth", "oauth2", "access":
		return credentialTypeBearer
	case credentialTypeSession, "session_token", "cookie":
		return credentialTypeSession
	case credentialTypeAPIKey:
		return credentialTypeAPIKey
	default:
		return credentialTypeUnknown
	}
}

func resolveCredentialType(apiKey, sessionToken, fallback string) string {
	apiKey = strings.TrimSpace(apiKey)
	sessionToken = strings.TrimSpace(sessionToken)
	kind := normalizeCredentialType(fallback)
	if sessionToken != "" {
		return credentialTypeSession
	}
	if apiKey != "" && kind == credentialTypeUnknown {
		return credentialTypeAPIKey
	}
	if kind == credentialTypeUnknown {
		return credentialTypeAPIKey
	}
	return kind
}
