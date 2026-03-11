package subscriptions

import (
	"context"
	"strings"
)

type contextKey string

const preferredUpstreamAPIKeyContextKey contextKey = "preferred-upstream-api-key"

func WithPreferredUpstreamAPIKey(ctx context.Context, apiKey string) context.Context {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return ctx
	}
	return context.WithValue(ctx, preferredUpstreamAPIKeyContextKey, apiKey)
}

func preferredUpstreamAPIKey(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	value, _ := ctx.Value(preferredUpstreamAPIKeyContextKey).(string)
	return strings.TrimSpace(value)
}
