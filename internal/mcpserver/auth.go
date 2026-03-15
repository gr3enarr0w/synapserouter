package mcpserver

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strings"
)

// AuthMiddleware wraps an http.Handler with bearer token authentication.
func AuthMiddleware(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			writeError(w, http.StatusUnauthorized, -32000, "authorization required")
			return
		}
		provided := strings.TrimPrefix(auth, "Bearer ")
		if subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
			writeError(w, http.StatusForbidden, -32000, "invalid token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// GenerateToken creates a cryptographically random hex token.
func GenerateToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "synroute-mcp-default-token"
	}
	return hex.EncodeToString(b)
}
