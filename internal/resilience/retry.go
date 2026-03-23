// Package resilience provides retry policies and circuit breaker patterns for gateway calls.
package resilience

import (
	"context"
	"math/rand/v2"
	"net"
	"time"

	"github.com/lobo235/homelab-mcp-server/internal/clients"
)

// RetryConfig holds retry policy configuration.
type RetryConfig struct {
	MaxRetries int
	BaseDelay  time.Duration
	MaxJitter  time.Duration
}

// DefaultRetryConfig returns the standard retry configuration.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries: 3,
		BaseDelay:  100 * time.Millisecond,
		MaxJitter:  25 * time.Millisecond,
	}
}

// Do executes fn with retries according to the retry policy.
// Only retries on transient errors (503, timeout, connection refused).
// Does NOT retry on 4xx errors.
func Do(ctx context.Context, cfg RetryConfig, fn func() error) error {
	var lastErr error
	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		lastErr = fn()
		if lastErr == nil {
			return nil
		}

		if !isRetryable(lastErr) {
			return lastErr
		}

		if attempt < cfg.MaxRetries {
			delay := cfg.BaseDelay * time.Duration(1<<uint(attempt))
			jitter := time.Duration(rand.Int64N(int64(cfg.MaxJitter*2))) - cfg.MaxJitter
			delay += jitter

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}
	}
	return lastErr
}

// isRetryable determines if an error is transient and worth retrying.
func isRetryable(err error) bool {
	// Gateway errors: retry on 503 (upstream_unavailable).
	if ge, ok := err.(*clients.GatewayError); ok {
		return ge.StatusCode == 503
	}

	// Network errors: connection refused, timeout.
	if _, ok := err.(net.Error); ok {
		return true
	}

	return false
}
