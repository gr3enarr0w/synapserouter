package main

import (
	"net/http"
	"strings"
	"testing"

	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/providers"
)

// TestModelValidation tests the model validation fixes for BUG-MODEL-VALIDATION-001 and BUG-MODEL-VALIDATION-002
func TestModelValidation(t *testing.T) {
	// Ensure clean AMP config so unknown models are properly rejected
	originalAmpConfig := ampConfig
	ampConfig.UpstreamURL = ""
	t.Cleanup(func() { ampConfig = originalAmpConfig })
	tests := []struct {
		name           string
		model          string
		provider       string // empty for generic route, set for pinned route
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "BUG-MODEL-VALIDATION-002: Invalid model should return 400",
			model:          "not-a-real-model",
			provider:       "",
			expectedStatus: http.StatusBadRequest,
			expectedError:  "unknown model",
		},
		{
			name:           "BUG-MODEL-VALIDATION-001: Codex provider with Claude model should return 400",
			model:          "claude-sonnet-4-5-20250929",
			provider:       "codex",
			expectedStatus: http.StatusBadRequest,
			expectedError:  "not compatible",
		},
		{
			name:           "BUG-MODEL-VALIDATION-001: Claude provider with Codex model should return 400",
			model:          "gpt-5.3-codex",
			provider:       "claude-code",
			expectedStatus: http.StatusBadRequest,
			expectedError:  "not compatible",
		},
		{
			name:           "Valid Claude model on generic route should work",
			model:          "claude-sonnet-4-5-20250929",
			provider:       "",
			expectedStatus: http.StatusOK,
			expectedError:  "",
		},
		{
			name:           "Valid Codex model on Codex route should work",
			model:          "gpt-5.3-codex",
			provider:       "codex",
			expectedStatus: http.StatusOK,
			expectedError:  "",
		},
		{
			name:           "Valid Claude model on Claude route should work",
			model:          "claude-sonnet-4-5-20250929",
			provider:       "claude-code",
			expectedStatus: http.StatusOK,
			expectedError:  "",
		},
		{
			name:           "Auto model should work on any provider",
			model:          "auto",
			provider:       "codex",
			expectedStatus: http.StatusOK,
			expectedError:  "",
		},
		{
			name:           "Gemini model on wrong provider should return 400",
			model:          "gemini-3.1-pro-preview",
			provider:       "codex",
			expectedStatus: http.StatusBadRequest,
			expectedError:  "not compatible",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test validation function directly
			chatReq := providers.ChatRequest{
				Model: tt.model,
				Messages: []providers.Message{
					{Role: "user", Content: "test"},
				},
			}

			err := validateModelForProvider(chatReq.Model, tt.provider)

			if tt.expectedStatus == http.StatusBadRequest {
				if err == nil {
					t.Errorf("Expected validation error for model=%s provider=%s, got nil", tt.model, tt.provider)
				} else if !strings.Contains(err.Error(), tt.expectedError) {
					t.Errorf("Expected error containing '%s', got: %s", tt.expectedError, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no validation error for model=%s provider=%s, got: %v", tt.model, tt.provider, err)
				}
			}
		})
	}
}

// TestPreferredProviderForModel tests the provider detection logic
func TestPreferredProviderForModel(t *testing.T) {
	tests := []struct {
		model            string
		expectedProvider string
	}{
		{"claude-sonnet-4-5-20250929", "claude-code"},
		{"gpt-5.3-codex", "codex"},
		{"gemini-3.1-pro-preview", "gemini"},
		{"qwen-max", "qwen"},
		{"claude-3-5-sonnet-latest", "claude-code"},
		{"gpt-4", "codex"},
		{"o1-preview", "codex"},
		{"not-a-real-model", ""},
		{"", ""},
		{"auto", ""},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			provider := preferredProviderForModel(tt.model)
			if provider != tt.expectedProvider {
				t.Errorf("preferredProviderForModel(%s) = %s, want %s", tt.model, provider, tt.expectedProvider)
			}
		})
	}
}

// TestModelValidationWithAmpFallback tests that unknown models can fall back to AMP
func TestModelValidationWithAmpFallback(t *testing.T) {
	// Save and restore original config
	originalAmpConfig := ampConfig
	defer func() { ampConfig = originalAmpConfig }()

	// Set AMP upstream
	ampConfig.UpstreamURL = "https://amp.example.com"

	// Unknown model should be allowed when AMP upstream is configured
	err := validateModelForProvider("unknown-model", "")
	if err != nil {
		t.Errorf("Expected unknown model to be allowed with AMP upstream, got error: %v", err)
	}

	// Clear AMP upstream
	ampConfig.UpstreamURL = ""

	// Unknown model should fail without AMP upstream
	err = validateModelForProvider("unknown-model", "")
	if err == nil {
		t.Error("Expected unknown model to fail without AMP upstream")
	}
}
