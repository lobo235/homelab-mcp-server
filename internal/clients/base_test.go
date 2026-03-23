package clients

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
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
