package providers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// ProviderError wraps an error from an LLM provider with HTTP metadata.
// Allows the router to inspect status codes and Retry-After headers
// without string-matching error messages.
type ProviderError struct {
	Provider       string
	StatusCode     int
	RetryAfterSecs int
	Body           string
	Err            error
}

func (e *ProviderError) Error() string {
	return fmt.Sprintf("%s returned %d: %s", e.Provider, e.StatusCode, e.Body)
}

func (e *ProviderError) Unwrap() error {
	return e.Err
}

func (e *ProviderError) IsRateLimit() bool {
	if e.StatusCode == http.StatusTooManyRequests {
		return true
	}
	lower := strings.ToLower(e.Body)
	return strings.Contains(lower, "resource_exhausted") ||
		strings.Contains(lower, "rate limit") ||
		strings.Contains(lower, "quota exceeded")
}

// NewProviderError creates a ProviderError from an HTTP response.
func NewProviderError(providerName string, resp *http.Response, body string) *ProviderError {
	retryAfter := 0
	if ra := resp.Header.Get("Retry-After"); ra != "" {
		if secs, err := strconv.Atoi(ra); err == nil {
			retryAfter = secs
		}
	}
	return &ProviderError{
		Provider:       providerName,
		StatusCode:     resp.StatusCode,
		RetryAfterSecs: retryAfter,
		Body:           body,
	}
}
