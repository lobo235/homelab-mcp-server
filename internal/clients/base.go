// Package clients provides a base HTTP client used by all gateway client wrappers.
package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"time"
)

// GatewayError represents a structured error response from a gateway.
type GatewayError struct {
	StatusCode int
	Code       string `json:"code"`
	Message    string `json:"message"`
}

func (e *GatewayError) Error() string {
	return fmt.Sprintf("gateway error %d (%s): %s", e.StatusCode, e.Code, e.Message)
}

// Base is the common HTTP client used by all gateway wrappers.
type Base struct {
	httpClient *http.Client
	baseURL    string
	apiKey     string

	retryConfig *RetryConfig
	cb          CircuitBreaker
}

// RetryConfig holds retry policy configuration for gateway calls.
type RetryConfig struct {
	MaxRetries int
	BaseDelay  time.Duration
	MaxJitter  time.Duration
}

// CircuitBreaker allows or blocks requests based on failure patterns.
type CircuitBreaker interface {
	Allow() error
	RecordSuccess()
	RecordFailure()
}

// NewBase creates a new base gateway client.
func NewBase(baseURL, apiKey string) *Base {
	return &Base{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		baseURL:    baseURL,
		apiKey:     apiKey,
	}
}

// WithRetry configures retry policy on the base client.
func (b *Base) WithRetry(cfg RetryConfig) *Base {
	b.retryConfig = &cfg
	return b
}

// WithCircuitBreaker configures a circuit breaker on the base client.
func (b *Base) WithCircuitBreaker(cb CircuitBreaker) *Base {
	b.cb = cb
	return b
}

// Do executes an HTTP request against the gateway, adding auth and trace headers.
// If a circuit breaker is configured, it checks/records state.
// If retry is configured, transient errors (503, network) are retried.
func (b *Base) Do(ctx context.Context, method, path string, body any) (*http.Response, error) {
	// Check circuit breaker before making the call.
	if b.cb != nil {
		if err := b.cb.Allow(); err != nil {
			return nil, err
		}
	}

	// Marshal body once (for retries).
	var bodyData []byte
	if body != nil {
		var err error
		bodyData, err = json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request body: %w", err)
		}
	}

	doOnce := func() (*http.Response, error) {
		var bodyReader io.Reader
		if bodyData != nil {
			bodyReader = bytes.NewReader(bodyData)
		}

		url := b.baseURL + path
		req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}

		req.Header.Set("Authorization", "Bearer "+b.apiKey)
		if bodyData != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		// Propagate trace ID from context if present.
		if traceID, ok := ctx.Value(traceIDKey).(string); ok && traceID != "" {
			req.Header.Set("X-Trace-ID", traceID)
		}

		return b.httpClient.Do(req)
	}

	// Without retry, just call once.
	if b.retryConfig == nil {
		resp, err := doOnce()
		b.recordResult(err)
		return resp, err
	}

	// With retry: wrap in retry loop.
	var lastResp *http.Response
	var lastErr error
	for attempt := 0; attempt <= b.retryConfig.MaxRetries; attempt++ {
		lastResp, lastErr = doOnce()
		if lastErr == nil {
			b.recordResult(nil)
			return lastResp, nil
		}

		if !b.isRetryable(lastErr) || attempt == b.retryConfig.MaxRetries {
			break
		}

		delay := b.retryConfig.BaseDelay * time.Duration(1<<uint(attempt))
		jitter := time.Duration(rand.Int64N(int64(b.retryConfig.MaxJitter*2))) - b.retryConfig.MaxJitter
		delay += jitter

		select {
		case <-ctx.Done():
			b.recordResult(ctx.Err())
			return nil, ctx.Err()
		case <-time.After(delay):
		}
	}

	b.recordResult(lastErr)
	return lastResp, lastErr
}

// recordResult updates the circuit breaker with success/failure.
func (b *Base) recordResult(err error) {
	if b.cb == nil {
		return
	}
	if err == nil {
		b.cb.RecordSuccess()
	} else {
		b.cb.RecordFailure()
	}
}

// isRetryable checks if an error is transient.
func (b *Base) isRetryable(err error) bool {
	if ge, ok := err.(*GatewayError); ok {
		return ge.StatusCode == 503
	}
	if _, ok := err.(net.Error); ok {
		return true
	}
	return false
}

// DoJSON executes an HTTP request and decodes the JSON response into result.
// Returns a *GatewayError for non-2xx status codes.
func (b *Base) DoJSON(ctx context.Context, method, path string, body, result any) error {
	resp, err := b.Do(ctx, method, path, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return parseGatewayError(resp)
	}

	if result != nil {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}

	return nil
}

// DoText executes an HTTP request and returns the response body as a string.
// Returns a *GatewayError for non-2xx status codes.
func (b *Base) DoText(ctx context.Context, method, path string, body any) (string, error) {
	resp, err := b.Do(ctx, method, path, body)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", parseGatewayError(resp)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response body: %w", err)
	}

	return string(data), nil
}

// DoNoContent executes an HTTP request expecting a 2xx response with no body.
func (b *Base) DoNoContent(ctx context.Context, method, path string, body any) error {
	resp, err := b.Do(ctx, method, path, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return parseGatewayError(resp)
	}

	return nil
}

// Ping calls GET /health on the gateway.
func (b *Base) Ping(ctx context.Context) error {
	resp, err := b.Do(ctx, http.MethodGet, "/health", nil)
	if err != nil {
		return fmt.Errorf("health check: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returned status %d", resp.StatusCode)
	}
	return nil
}

// BaseURL returns the configured base URL.
func (b *Base) BaseURL() string {
	return b.baseURL
}

// DefaultRetryConfig returns the standard retry configuration:
// 3 retries with exponential backoff from 100ms and ±25ms jitter.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries: 3,
		BaseDelay:  100 * time.Millisecond,
		MaxJitter:  25 * time.Millisecond,
	}
}

// NewBaseWithResilience creates a Base client pre-configured with retry and circuit breaker.
func NewBaseWithResilience(baseURL, apiKey string, cb CircuitBreaker) *Base {
	b := NewBase(baseURL, apiKey)
	cfg := DefaultRetryConfig()
	b.retryConfig = &cfg
	b.cb = cb
	return b
}

func parseGatewayError(resp *http.Response) *GatewayError {
	ge := &GatewayError{StatusCode: resp.StatusCode}
	data, err := io.ReadAll(resp.Body)
	if err != nil || len(data) == 0 {
		ge.Code = "unknown"
		ge.Message = fmt.Sprintf("HTTP %d", resp.StatusCode)
		return ge
	}
	if err := json.Unmarshal(data, ge); err != nil {
		ge.Code = "unknown"
		ge.Message = string(data)
	}
	return ge
}

// traceIDKeyType is the context key type for trace IDs.
type traceIDKeyType string

const traceIDKey traceIDKeyType = "trace_id"

// WithTraceID returns a context with the given trace ID attached.
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDKey, traceID)
}
