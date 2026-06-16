package synthesis

import (
	"context"
	"log"
	"strings"
	"time"
)

// RetryPolicy handles exponential backoff for transient failures
type RetryPolicy struct {
	MaxRetries int
	BaseDelay  time.Duration
	MaxDelay   time.Duration

	// IsRetryable determines if an error warrants a retry (e.g., 429 rate limits)
	IsRetryable func(error) bool
}

// DefaultRetryPolicy returns a policy tuned for TTS API rate limits
func DefaultRetryPolicy() *RetryPolicy {
	return &RetryPolicy{
		MaxRetries: 3,
		BaseDelay:  1 * time.Second,
		MaxDelay:   10 * time.Second,
		IsRetryable: func(err error) bool {
			msg := err.Error()
			return strings.Contains(msg, "429") || strings.Contains(msg, "Too many")
		},
	}
}

// Do executes fn with retries. Returns the first non-retryable error or
// the last retryable error if all attempts are exhausted.
func (p *RetryPolicy) Do(ctx context.Context, fn func() error) error {
	var lastErr error
	for attempt := 0; attempt <= p.MaxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if attempt > 0 {
			delay := p.backoff(attempt)
			log.Printf("🔄 Retry attempt %d/%d (waiting %v)", attempt, p.MaxRetries, delay)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}

		err := fn()
		if err == nil {
			return nil
		}

		lastErr = err
		if !p.IsRetryable(err) {
			return err
		}
	}
	return lastErr
}

func (p *RetryPolicy) backoff(attempt int) time.Duration {
	delay := p.BaseDelay * time.Duration(1<<(attempt-1)) // 1s, 2s, 4s
	if delay > p.MaxDelay {
		delay = p.MaxDelay
	}
	return delay
}
