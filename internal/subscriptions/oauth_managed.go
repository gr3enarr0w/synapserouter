package subscriptions

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

const managedLoginSessionTTL = 10 * time.Minute

var (
	errManagedLoginNotFound         = errors.New("oauth session not found")
	errManagedLoginProviderMismatch = errors.New("oauth session provider mismatch")
)

type ManagedLoginResult struct {
	Provider string `json:"provider"`
	State    string `json:"state"`
	URL      string `json:"url"`
}

type managedLoginSession struct {
	provider    string
	state       string
	authURL     string
	redirectURI string
	projectID   string
	pkce        pkceBundle
	server      *oauthServer
	manual      chan oauthCallbackResult
	status      string
	expiresAt   time.Time
}

type managedLoginSessionStore struct {
	mu       sync.Mutex
	ttl      time.Duration
	sessions map[string]*managedLoginSession
}

func newManagedLoginSessionStore(ttl time.Duration) *managedLoginSessionStore {
	if ttl <= 0 {
		ttl = managedLoginSessionTTL
	}
	return &managedLoginSessionStore{
		ttl:      ttl,
		sessions: make(map[string]*managedLoginSession),
	}
}

func (s *managedLoginSessionStore) purgeExpiredLocked(now time.Time) {
	for state, session := range s.sessions {
		if session == nil {
			delete(s.sessions, state)
			continue
		}
		if !session.expiresAt.IsZero() && now.After(session.expiresAt) {
			delete(s.sessions, state)
		}
	}
}

func (s *managedLoginSessionStore) register(session *managedLoginSession) {
	if session == nil || strings.TrimSpace(session.state) == "" {
		return
	}
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.purgeExpiredLocked(now)
	session.expiresAt = now.Add(s.ttl)
	s.sessions[session.state] = session
}

func (s *managedLoginSessionStore) complete(state string) {
	state = strings.TrimSpace(state)
	if state == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, state)
}

func (s *managedLoginSessionStore) setError(state, status string) {
	state = strings.TrimSpace(state)
	status = strings.TrimSpace(status)
	if state == "" || status == "" {
		return
	}
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.purgeExpiredLocked(now)
	session, ok := s.sessions[state]
	if !ok || session == nil {
		return
	}
	session.status = status
	session.expiresAt = now.Add(s.ttl)
}

func (s *managedLoginSessionStore) status(state string) (string, string, bool) {
	state = strings.TrimSpace(state)
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.purgeExpiredLocked(now)
	session, ok := s.sessions[state]
	if !ok || session == nil {
		return "", "", false
	}
	return session.provider, session.status, true
}

func (s *managedLoginSessionStore) get(state string) (*managedLoginSession, bool) {
	state = strings.TrimSpace(state)
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.purgeExpiredLocked(now)
	session, ok := s.sessions[state]
	if !ok || session == nil {
		return nil, false
	}
	return session, true
}

var managedLogins = newManagedLoginSessionStore(managedLoginSessionTTL)

func ErrManagedLoginNotFound() error {
	return errManagedLoginNotFound
}

func BeginManagedLogin(ctx context.Context, provider string, opts LoginOptions) (ManagedLoginResult, error) {
	_ = ctx
	session, err := newManagedLoginSession(provider, opts)
	if err != nil {
		return ManagedLoginResult{}, err
	}
	oauthDebugf("managed login session created provider=%q state=%s redirect_uri=%q auth_url=%s", session.provider, redactOAuthValue(session.state), session.redirectURI, redactOAuthURL(session.authURL))

	managedLogins.register(session)

	go runManagedLoginFlow(session, effectiveLoginTimeout(opts.Timeout)) //nolint:G118 // fire-and-forget OAuth flow by design

	return ManagedLoginResult{
		Provider: session.provider,
		State:    session.state,
		URL:      session.authURL,
	}, nil
}

func ManagedLoginStatus(state string) (provider string, status string, ok bool) {
	return managedLogins.status(state)
}

func SubmitManagedLoginCallback(provider, state, code, errorDetail string) error {
	provider = normalizeStoredProviderName(provider)
	if provider == "" {
		return fmt.Errorf("invalid provider")
	}
	session, ok := managedLogins.get(state)
	if !ok {
		return errManagedLoginNotFound
	}
	if session.provider != provider {
		return errManagedLoginProviderMismatch
	}

	callback := oauthCallbackResult{
		Code:        strings.TrimSpace(code),
		State:       strings.TrimSpace(state),
		Error:       strings.TrimSpace(errorDetail),
		ErrorDetail: strings.TrimSpace(errorDetail),
	}
	oauthDebugf("managed login callback submitted provider=%q state=%s code_len=%d error=%q", provider, redactOAuthValue(state), len(callback.Code), callback.Error)

	select {
	case session.manual <- callback:
	default:
	}
	return nil
}

func newManagedLoginSession(provider string, opts LoginOptions) (*managedLoginSession, error) {
	rawProvider := provider
	provider = normalizeStoredProviderName(provider)
	if provider == "" {
		return nil, fmt.Errorf("unsupported provider %q", rawProvider)
	}

	state, err := generateRandomState()
	if err != nil {
		return nil, err
	}

	session := &managedLoginSession{
		provider:  provider,
		state:     state,
		projectID: strings.TrimSpace(loginOptionValue(opts.Metadata, "project_id")),
		manual:    make(chan oauthCallbackResult, 1),
	}

	switch provider {
	case "anthropic":
		pkce, err := generatePKCE()
		if err != nil {
			return nil, err
		}
		session.pkce = pkce
		session.redirectURI = claudeManagedRedirectURI
		session.authURL = buildClaudeAuthURL(state, pkce.challenge, session.redirectURI)
	case "openai":
		pkce, err := generatePKCE()
		if err != nil {
			return nil, err
		}
		port := codexDefaultPort
		if opts.CallbackPort > 0 {
			port = opts.CallbackPort
		}
		redirectURI := fmt.Sprintf(codexRedirectFormat, port)
		server, err := startOAuthServer(port, "/auth/callback")
		if err != nil {
			return nil, err
		}
		session.pkce = pkce
		session.redirectURI = redirectURI
		session.server = server
		session.authURL = buildCodexAuthURL(state, pkce.challenge, redirectURI)
	case "gemini":
		port := geminiDefaultPort
		if opts.CallbackPort > 0 {
			port = opts.CallbackPort
		}
		redirectURI := fmt.Sprintf(geminiCallbackFormat, port)
		server, err := startOAuthServer(port, "/oauth2callback")
		if err != nil {
			return nil, err
		}
		session.redirectURI = redirectURI
		session.server = server
		session.authURL = buildGeminiAuthURL(state, redirectURI)
	default:
		return nil, fmt.Errorf("unsupported provider %q", provider)
	}

	return session, nil
}

func runManagedLoginFlow(session *managedLoginSession, timeout time.Duration) {
	if session == nil {
		return
	}
	defer func() {
		if session.server != nil {
			ctxShutdown, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = session.server.stop(ctxShutdown)
		}
	}()

	callback, err := waitForManagedLoginCallback(session, timeout)
	if err != nil {
		oauthDebugf("managed login callback wait failed provider=%q state=%s err=%v", session.provider, redactOAuthValue(session.state), err)
		managedLogins.setError(session.state, err.Error())
		return
	}
	if callback.Error != "" {
		oauthDebugf("managed login callback contained provider error provider=%q state=%s error=%q detail=%q", session.provider, redactOAuthValue(session.state), callback.Error, callback.ErrorDetail)
		message := callback.Error
		if callback.ErrorDetail != "" {
			message = callback.ErrorDetail
		}
		managedLogins.setError(session.state, message)
		return
	}
	if callback.State != session.state {
		oauthDebugf("managed login state mismatch provider=%q expected=%s got=%s", session.provider, redactOAuthValue(session.state), redactOAuthValue(callback.State))
		managedLogins.setError(session.state, fmt.Sprintf("oauth state mismatch: expected %q, got %q", session.state, callback.State))
		return
	}

	credential, err := exchangeManagedLoginCredential(context.Background(), session, callback.Code)
	if err != nil {
		oauthDebugf("managed login credential exchange failed provider=%q state=%s err=%v", session.provider, redactOAuthValue(session.state), err)
		managedLogins.setError(session.state, err.Error())
		return
	}
	if session.provider == "gemini" && session.projectID != "" {
		credential.ProjectID = session.projectID
	}
	if err := StoreCredential(session.provider, credential); err != nil {
		oauthDebugf("managed login credential store failed provider=%q state=%s err=%v", session.provider, redactOAuthValue(session.state), err)
		managedLogins.setError(session.state, err.Error())
		return
	}

	oauthDebugf("managed login completed provider=%q state=%s email=%q expires_at=%q", session.provider, redactOAuthValue(session.state), credential.Email, credential.ExpiresAt)
	managedLogins.complete(session.state)
}

func waitForManagedLoginCallback(session *managedLoginSession, timeout time.Duration) (oauthCallbackResult, error) {
	if session == nil {
		return oauthCallbackResult{}, fmt.Errorf("oauth session unavailable")
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	var callbackCh <-chan oauthCallbackResult
	var errorCh <-chan error
	if session.server != nil {
		callbackCh = session.server.callbacks
		errorCh = session.server.errors
	}

	select {
	case callback := <-callbackCh:
		return callback, nil
	case callback := <-session.manual:
		return callback, nil
	case err := <-errorCh:
		return oauthCallbackResult{}, err
	case <-timer.C:
		return oauthCallbackResult{}, fmt.Errorf("oauth flow timed out after %s", timeout)
	}
}

func exchangeManagedLoginCredential(ctx context.Context, session *managedLoginSession, code string) (ProviderCredential, error) {
	switch session.provider {
	case "anthropic":
		return exchangeClaudeToken(ctx, code, session.state, session.pkce.verifier, session.redirectURI)
	case "openai":
		return exchangeCodexToken(ctx, code, session.pkce.verifier, session.redirectURI)
	case "gemini":
		return exchangeGeminiToken(ctx, code, session.redirectURI)
	default:
		return ProviderCredential{}, fmt.Errorf("unsupported provider %q", session.provider)
	}
}

func loginOptionValue(metadata map[string]string, key string) string {
	if metadata == nil {
		return ""
	}
	return strings.TrimSpace(metadata[key])
}
