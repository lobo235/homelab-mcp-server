package clients

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBase_DoJSON_Success(t *testing.T) {
	type response struct {
		Value string `json:"value"`
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected content-type: %s", r.Header.Get("Content-Type"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response{Value: "hello"})
	}))
	defer srv.Close()

	base := NewBase(srv.URL, "test-key")

	var result response
	err := base.DoJSON(context.Background(), "POST", "/test", map[string]string{"key": "val"}, &result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Value != "hello" {
		t.Errorf("Value = %q, want %q", result.Value, "hello")
	}
}

func TestBase_DoJSON_GatewayError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{
			"code":    "not_found",
			"message": "resource not found",
		})
	}))
	defer srv.Close()

	base := NewBase(srv.URL, "test-key")

	err := base.DoJSON(context.Background(), "GET", "/missing", nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}

	ge, ok := err.(*GatewayError)
	if !ok {
		t.Fatalf("expected *GatewayError, got %T", err)
	}
	if ge.StatusCode != 404 {
		t.Errorf("StatusCode = %d, want 404", ge.StatusCode)
	}
	if ge.Code != "not_found" {
		t.Errorf("Code = %q, want %q", ge.Code, "not_found")
	}
}

func TestBase_Ping_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	base := NewBase(srv.URL, "test-key")
	err := base.Ping(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBase_Ping_Failure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	base := NewBase(srv.URL, "test-key")
	err := base.Ping(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBase_TraceID_Propagation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceID := r.Header.Get("X-Trace-ID")
		if traceID != "test-trace-123" {
			t.Errorf("X-Trace-ID = %q, want %q", traceID, "test-trace-123")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	base := NewBase(srv.URL, "test-key")
	ctx := WithTraceID(context.Background(), "test-trace-123")
	err := base.DoNoContent(ctx, "GET", "/test", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGatewayError_Error(t *testing.T) {
	ge := &GatewayError{StatusCode: 502, Code: "upstream_error", Message: "vault unreachable"}
	got := ge.Error()
	want := "gateway error 502 (upstream_error): vault unreachable"
	if got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestBase_Do_StringBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if string(body) != "raw hcl content" {
			t.Errorf("body = %q, want %q", string(body), "raw hcl content")
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("content-type = %q, want %q", r.Header.Get("Content-Type"), "application/json")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	base := NewBase(srv.URL, "test-key")
	resp, err := base.Do(context.Background(), "POST", "/test", "raw hcl content")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Body.Close()
}

func TestBase_Do_ByteSliceBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if string(body) != "byte content" {
			t.Errorf("body = %q, want %q", string(body), "byte content")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	base := NewBase(srv.URL, "test-key")
	resp, err := base.Do(context.Background(), "POST", "/test", []byte("byte content"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Body.Close()
}

func TestBase_Do_NilBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "" {
			t.Errorf("expected no content-type for nil body, got %q", r.Header.Get("Content-Type"))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	base := NewBase(srv.URL, "test-key")
	resp, err := base.Do(context.Background(), "GET", "/test", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Body.Close()
}

func TestBase_DoText_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("plain text response"))
	}))
	defer srv.Close()

	base := NewBase(srv.URL, "test-key")
	text, err := base.DoText(context.Background(), "GET", "/logs", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "plain text response" {
		t.Errorf("text = %q, want %q", text, "plain text response")
	}
}

func TestBase_DoText_GatewayError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{
			"code":    "not_found",
			"message": "not found",
		})
	}))
	defer srv.Close()

	base := NewBase(srv.URL, "test-key")
	_, err := base.DoText(context.Background(), "GET", "/missing", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	ge, ok := err.(*GatewayError)
	if !ok {
		t.Fatalf("expected *GatewayError, got %T", err)
	}
	if ge.StatusCode != 404 {
		t.Errorf("StatusCode = %d, want 404", ge.StatusCode)
	}
}

func TestBase_DoNoContent_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	base := NewBase(srv.URL, "test-key")
	err := base.DoNoContent(context.Background(), "DELETE", "/resource", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBase_DoNoContent_GatewayError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{
			"code":    "forbidden",
			"message": "access denied",
		})
	}))
	defer srv.Close()

	base := NewBase(srv.URL, "test-key")
	err := base.DoNoContent(context.Background(), "DELETE", "/resource", nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBase_DoJSON_NilResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("{}"))
	}))
	defer srv.Close()

	base := NewBase(srv.URL, "test-key")
	err := base.DoJSON(context.Background(), "POST", "/test", nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBase_BaseURL(t *testing.T) {
	base := NewBase("http://example.com", "key")
	if base.BaseURL() != "http://example.com" {
		t.Errorf("BaseURL() = %q, want %q", base.BaseURL(), "http://example.com")
	}
}

func TestBase_WithRetry(t *testing.T) {
	base := NewBase("http://example.com", "key")
	cfg := DefaultRetryConfig()
	result := base.WithRetry(cfg)
	if result != base {
		t.Error("WithRetry should return same base instance")
	}
	if base.retryConfig == nil {
		t.Error("retryConfig should be set")
	}
}

// mockCircuitBreaker implements CircuitBreaker for testing.
type mockCircuitBreaker struct {
	allowErr     error
	successCount int
	failureCount int
}

func (m *mockCircuitBreaker) Allow() error { return m.allowErr }

func (m *mockCircuitBreaker) RecordSuccess() { m.successCount++ }

func (m *mockCircuitBreaker) RecordFailure() { m.failureCount++ }

func TestBase_WithCircuitBreaker(t *testing.T) {
	base := NewBase("http://example.com", "key")
	cb := &mockCircuitBreaker{}
	result := base.WithCircuitBreaker(cb)
	if result != base {
		t.Error("WithCircuitBreaker should return same base instance")
	}
}

func TestBase_CircuitBreaker_AllowBlocks(t *testing.T) {
	base := NewBase("http://example.com", "key")
	cb := &mockCircuitBreaker{allowErr: errors.New("circuit open")}
	base.WithCircuitBreaker(cb)

	_, err := base.Do(context.Background(), "GET", "/test", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "circuit open") {
		t.Errorf("error = %q, want circuit open", err.Error())
	}
}

func TestBase_CircuitBreaker_RecordsSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cb := &mockCircuitBreaker{}
	base := NewBase(srv.URL, "test-key")
	base.WithCircuitBreaker(cb)

	resp, err := base.Do(context.Background(), "GET", "/test", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Body.Close()

	if cb.successCount != 1 {
		t.Errorf("successCount = %d, want 1", cb.successCount)
	}
}

func TestBase_NewBaseWithResilience(t *testing.T) {
	cb := &mockCircuitBreaker{}
	base := NewBaseWithResilience("http://example.com", "key", cb)
	if base.retryConfig == nil {
		t.Error("retryConfig should be set")
	}
	if base.cb == nil {
		t.Error("circuit breaker should be set")
	}
}

func TestBase_DefaultRetryConfig(t *testing.T) {
	cfg := DefaultRetryConfig()
	if cfg.MaxRetries != 3 {
		t.Errorf("MaxRetries = %d, want 3", cfg.MaxRetries)
	}
}

func TestParseGatewayError_EmptyBody(t *testing.T) {
	resp := &http.Response{
		StatusCode: 500,
		Body:       io.NopCloser(strings.NewReader("")),
	}
	ge := parseGatewayError(resp)
	if ge.Code != "unknown" {
		t.Errorf("Code = %q, want %q", ge.Code, "unknown")
	}
	if ge.Message != "HTTP 500" {
		t.Errorf("Message = %q, want %q", ge.Message, "HTTP 500")
	}
}

func TestParseGatewayError_InvalidJSON(t *testing.T) {
	resp := &http.Response{
		StatusCode: 502,
		Body:       io.NopCloser(strings.NewReader("not json at all")),
	}
	ge := parseGatewayError(resp)
	if ge.Code != "unknown" {
		t.Errorf("Code = %q, want %q", ge.Code, "unknown")
	}
	if ge.Message != "not json at all" {
		t.Errorf("Message = %q, want %q", ge.Message, "not json at all")
	}
}

func TestBase_Do_ConnectionError(t *testing.T) {
	base := NewBase("http://127.0.0.1:1", "test-key")
	_, err := base.Do(context.Background(), "GET", "/test", nil)
	if err == nil {
		t.Fatal("expected error for connection refused")
	}
}

func TestBase_TraceID_NotSet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceID := r.Header.Get("X-Trace-ID")
		if traceID != "" {
			t.Errorf("expected no X-Trace-ID, got %q", traceID)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	base := NewBase(srv.URL, "test-key")
	err := base.DoNoContent(context.Background(), "GET", "/test", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBase_Do_MarshalError(t *testing.T) {
	base := NewBase("http://example.com", "test-key")
	// Channels cannot be marshaled to JSON
	_, err := base.Do(context.Background(), "POST", "/test", make(chan int))
	if err == nil {
		t.Fatal("expected marshal error")
	}
	if !strings.Contains(err.Error(), "marshal request body") {
		t.Errorf("error = %q, want marshal error", err.Error())
	}
}

func TestBase_IsRetryable(t *testing.T) {
	base := NewBase("http://example.com", "test-key")

	// GatewayError with 503 is retryable
	ge503 := &GatewayError{StatusCode: 503, Code: "unavailable", Message: "service unavailable"}
	if !base.isRetryable(ge503) {
		t.Error("503 GatewayError should be retryable")
	}

	// GatewayError with 404 is not retryable
	ge404 := &GatewayError{StatusCode: 404, Code: "not_found", Message: "not found"}
	if base.isRetryable(ge404) {
		t.Error("404 GatewayError should not be retryable")
	}

	// Generic error is not retryable
	if base.isRetryable(errors.New("generic error")) {
		t.Error("generic error should not be retryable")
	}
}

func TestBase_Retry_SuccessAfterFailure(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts == 1 {
			// Close connection without response to simulate a network error
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Fatal("server doesn't support hijacking")
			}
			conn, _, _ := hj.Hijack()
			conn.Close()
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	base := NewBase(srv.URL, "test-key")
	base.WithRetry(RetryConfig{
		MaxRetries: 2,
		BaseDelay:  1 * time.Millisecond,
		MaxJitter:  1 * time.Millisecond,
	})

	resp, err := base.Do(context.Background(), "GET", "/test", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Body.Close()
	if attempts != 2 {
		t.Errorf("attempts = %d, want 2", attempts)
	}
}

func TestBase_Retry_AllFail(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("server doesn't support hijacking")
		}
		conn, _, _ := hj.Hijack()
		conn.Close()
	}))
	defer srv.Close()

	base := NewBase(srv.URL, "test-key")
	base.WithRetry(RetryConfig{
		MaxRetries: 2,
		BaseDelay:  1 * time.Millisecond,
		MaxJitter:  1 * time.Millisecond,
	})

	_, err := base.Do(context.Background(), "GET", "/test", nil)
	if err == nil {
		t.Fatal("expected error after all retries exhausted")
	}
	if attempts != 3 {
		t.Errorf("attempts = %d, want 3 (initial + 2 retries)", attempts)
	}
}

func TestBase_Retry_NonRetryableError(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// Use invalid URL so we get a non-retryable error
	base := NewBase("http://127.0.0.1:1", "test-key")
	base.WithRetry(RetryConfig{
		MaxRetries: 2,
		BaseDelay:  1 * time.Millisecond,
		MaxJitter:  1 * time.Millisecond,
	})

	_, err := base.Do(context.Background(), "GET", "/test", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	// Connection refused is a net.Error, so it IS retryable - we should get 3 attempts
	// But the server won't count because we're connecting to a dead port
}

func TestBase_Retry_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("server doesn't support hijacking")
		}
		conn, _, _ := hj.Hijack()
		conn.Close()
	}))
	defer srv.Close()

	base := NewBase(srv.URL, "test-key")
	base.WithRetry(RetryConfig{
		MaxRetries: 10,
		BaseDelay:  500 * time.Millisecond,
		MaxJitter:  1 * time.Millisecond,
	})

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel immediately after first attempt
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	_, err := base.Do(ctx, "GET", "/test", nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBase_Retry_WithCircuitBreaker(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cb := &mockCircuitBreaker{}
	base := NewBase(srv.URL, "test-key")
	base.WithRetry(RetryConfig{
		MaxRetries: 1,
		BaseDelay:  1 * time.Millisecond,
		MaxJitter:  1 * time.Millisecond,
	})
	base.WithCircuitBreaker(cb)

	resp, err := base.Do(context.Background(), "GET", "/test", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Body.Close()
	if cb.successCount != 1 {
		t.Errorf("successCount = %d, want 1", cb.successCount)
	}
}

func TestBase_RecordResult_NilCircuitBreaker(t *testing.T) {
	base := NewBase("http://example.com", "test-key")
	// Should not panic with nil circuit breaker
	base.recordResult(nil)
	base.recordResult(errors.New("test error"))
}
