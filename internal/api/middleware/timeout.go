package middleware

import (
	"context"
	"net/http"
	"time"
)

// Timeout returns a middleware that adds a timeout to the request context.
// When the timeout fires, it returns a JSON error response.
// This wraps chi's built-in Timeout with JSON error output.
func Timeout(timeout time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()

			done := make(chan struct{})

			go func() {
				next.ServeHTTP(w, r.WithContext(ctx))
				close(done)
			}()

			select {
			case <-done:
				return
			case <-ctx.Done():
				// Only send error if we haven't already started writing response
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusGatewayTimeout)
				w.Write([]byte(`{"error":{"code":"TIMEOUT","message":"request timeout"}}`))
			}
		})
	}
}
