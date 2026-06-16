package middleware

import (
	"net/http"
	"strings"
)

// Auth returns middleware that validates a Bearer token.
// If token is empty, authentication is disabled.
func Auth(validToken string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if validToken == "" {
				// Auth disabled
				next.ServeHTTP(w, r)
				return
			}

			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error":{"code":"UNAUTHORIZED","message":"missing Authorization header"}}`))
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error":{"code":"UNAUTHORIZED","message":"invalid Authorization format, expected: Bearer <token>"}}`))
				return
			}

			if parts[1] != validToken {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error":{"code":"UNAUTHORIZED","message":"invalid token"}}`))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
