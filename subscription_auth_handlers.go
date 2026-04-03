package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gr3enarr0w/synapserouter/internal/subscriptions"
)

func subscriptionAnthropicAuthURLHandler(w http.ResponseWriter, r *http.Request) {
	subscriptionAuthURLHandler(w, r, "anthropic")
}

func subscriptionCodexAuthURLHandler(w http.ResponseWriter, r *http.Request) {
	subscriptionAuthURLHandler(w, r, "openai")
}

func subscriptionGeminiAuthURLHandler(w http.ResponseWriter, r *http.Request) {
	subscriptionAuthURLHandler(w, r, "gemini")
}

func subscriptionAuthURLHandler(w http.ResponseWriter, r *http.Request, provider string) {
	opts := subscriptions.LoginOptions{NoBrowser: true}
	if raw := strings.TrimSpace(r.URL.Query().Get("callback_port")); raw != "" {
		port, err := strconv.Atoi(raw)
		if err != nil || port <= 0 {
			http.Error(w, "invalid callback_port", http.StatusBadRequest)
			return
		}
		opts.CallbackPort = port
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("timeout_seconds")); raw != "" {
		seconds, err := strconv.Atoi(raw)
		if err != nil || seconds <= 0 {
			http.Error(w, "invalid timeout_seconds", http.StatusBadRequest)
			return
		}
		opts.Timeout = time.Duration(seconds) * time.Second
	}
	if provider == "gemini" {
		if projectID := strings.TrimSpace(r.URL.Query().Get("project_id")); projectID != "" {
			opts.Metadata = map[string]string{"project_id": projectID}
		}
	}

	result, err := subscriptions.BeginManagedLogin(r.Context(), provider, opts)
	if err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(strings.ToLower(err.Error()), "unsupported provider") {
			status = http.StatusBadRequest
		}
		http.Error(w, err.Error(), status)
		return
	}

	writeJSON(w, map[string]interface{}{
		"status":     "ok",
		"provider":   result.Provider,
		"state":      result.State,
		"url":        result.URL,
		"store_path": subscriptions.StorePath(),
	})
}

func subscriptionAuthStatusHandler(w http.ResponseWriter, r *http.Request) {
	state := strings.TrimSpace(r.URL.Query().Get("state"))
	if state == "" {
		writeJSON(w, map[string]interface{}{
			"status":     "ok",
			"store_path": subscriptions.StorePath(),
		})
		return
	}

	provider, status, ok := subscriptions.ManagedLoginStatus(state)
	if !ok {
		writeJSON(w, map[string]interface{}{
			"status":     "ok",
			"store_path": subscriptions.StorePath(),
		})
		return
	}
	if status != "" {
		writeJSON(w, map[string]interface{}{
			"status":     "error",
			"provider":   provider,
			"error":      status,
			"store_path": subscriptions.StorePath(),
		})
		return
	}

	writeJSON(w, map[string]interface{}{
		"status":     "wait",
		"provider":   provider,
		"store_path": subscriptions.StorePath(),
	})
}

func subscriptionOAuthCallbackHandler(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Provider    string `json:"provider"`
		RedirectURL string `json:"redirect_url"`
		Code        string `json:"code"`
		State       string `json:"state"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	state := strings.TrimSpace(payload.State)
	code := strings.TrimSpace(payload.Code)
	errText := strings.TrimSpace(payload.Error)

	if rawRedirect := strings.TrimSpace(payload.RedirectURL); rawRedirect != "" {
		redirectState, redirectCode, redirectErr, err := parseOAuthRedirect(rawRedirect)
		if err != nil {
			http.Error(w, "invalid redirect_url", http.StatusBadRequest)
			return
		}
		if state == "" {
			state = redirectState
		}
		if code == "" {
			code = redirectCode
		}
		if errText == "" {
			errText = redirectErr
		}
	}

	if state == "" || (code == "" && errText == "") {
		http.Error(w, "state and code or error are required", http.StatusBadRequest)
		return
	}

	if err := subscriptions.SubmitManagedLoginCallback(payload.Provider, state, code, errText); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, subscriptions.ErrManagedLoginNotFound()) {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}

	writeJSON(w, map[string]string{"status": "ok"})
}

func subscriptionProviderCallbackHandler(provider string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		state := strings.TrimSpace(r.URL.Query().Get("state"))
		code := strings.TrimSpace(r.URL.Query().Get("code"))
		errText := strings.TrimSpace(r.URL.Query().Get("error"))
		if errText == "" {
			errText = strings.TrimSpace(r.URL.Query().Get("error_description"))
		}
		if state != "" {
			_ = subscriptions.SubmitManagedLoginCallback(provider, state, code, errText)
		}
		if errText != "" {
			subscriptions.WriteOAuthCallbackHTML(w, http.StatusBadRequest, false, "Authentication failed", errText)
			return
		}
		subscriptions.WriteOAuthCallbackHTML(w, http.StatusOK, true, "", "")
	}
}

func parseOAuthRedirect(raw string) (state string, code string, errText string, err error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", "", "", err
	}

	query := u.Query()
	state = strings.TrimSpace(query.Get("state"))
	code = strings.TrimSpace(query.Get("code"))
	errText = strings.TrimSpace(query.Get("error"))
	if errText == "" {
		errText = strings.TrimSpace(query.Get("error_description"))
	}
	if code == "" && u.Fragment != "" {
		fragment, err := url.ParseQuery(u.Fragment)
		if err == nil {
			code = strings.TrimSpace(fragment.Get("code"))
			if state == "" {
				state = strings.TrimSpace(fragment.Get("state"))
			}
			if errText == "" {
				errText = strings.TrimSpace(fragment.Get("error"))
			}
		}
	}
	return state, code, errText, nil
}
