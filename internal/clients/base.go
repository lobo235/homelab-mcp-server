// Package clients provides a base HTTP client used by all gateway client wrappers.
package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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
}

// NewBase creates a new base gateway client.
func NewBase(baseURL, apiKey string) *Base {
	return &Base{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		baseURL:    baseURL,
		apiKey:     apiKey,
	}
}

// Do executes an HTTP request against the gateway, adding auth and trace headers.
func (b *Base) Do(ctx context.Context, method, path string, body any) (*http.Response, error) {
	url := b.baseURL + path

	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+b.apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// Propagate trace ID from context if present.
	if traceID, ok := ctx.Value(traceIDKey).(string); ok && traceID != "" {
		req.Header.Set("X-Trace-ID", traceID)
	}

	return b.httpClient.Do(req)
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
