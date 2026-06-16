package middleware

import (
	"net/http"
	"sync"
	"time"
)

// RateLimiter implements a simple token bucket rate limiter.
type RateLimiter struct {
	rate     float64 // requests per second
	burst    int
	mu       sync.Mutex
	tokens   float64
	lastTime time.Time
}

// NewRateLimiter creates a new rate limiter.
// rate: requests per second, burst: max burst size
func NewRateLimiter(rate float64, burst int) *RateLimiter {
	return &RateLimiter{
		rate:     rate,
		burst:    burst,
		tokens:   float64(burst),
		lastTime: time.Now(),
	}
}

// Allow checks if a request is allowed. Returns true if allowed.
func (rl *RateLimiter) Allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(rl.lastTime).Seconds()
	rl.tokens += elapsed * rl.rate
	if rl.tokens > float64(rl.burst) {
		rl.tokens = float64(rl.burst)
	}
	rl.lastTime = now

	if rl.tokens >= 1 {
		rl.tokens--
		return true
	}
	return false
}

// Handler returns an HTTP middleware that rate limits requests.
func (rl *RateLimiter) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !rl.Allow() {
			w.Header().Set("Retry-After", "1")
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error":{"code":"TOO_MANY_REQUESTS","message":"rate limit exceeded, please retry after 1 second"}}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// PerIPRateLimiter tracks rate limits per client IP
type PerIPRateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*RateLimiter
	rate     float64
	burst    int
}

// NewPerIPRateLimiter creates a per-IP rate limiter
func NewPerIPRateLimiter(rate float64, burst int) *PerIPRateLimiter {
	return &PerIPRateLimiter{
		limiters: make(map[string]*RateLimiter),
		rate:     rate,
		burst:    burst,
	}
}

// Handler returns middleware that rate limits per client IP
func (pl *PerIPRateLimiter) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr
		// Extract IP without port if present
		if i := len(ip) - 1; i >= 0 {
			for j := i; j >= 0; j-- {
				if ip[j] == ':' {
					ip = ip[:j]
					break
				}
			}
		}

		pl.mu.Lock()
		limiter, ok := pl.limiters[ip]
		if !ok {
			limiter = NewRateLimiter(pl.rate, pl.burst)
			pl.limiters[ip] = limiter
		}
		pl.mu.Unlock()

		if !limiter.Allow() {
			w.Header().Set("Retry-After", "1")
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error":{"code":"TOO_MANY_REQUESTS","message":"rate limit exceeded, please retry after 1 second"}}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}
