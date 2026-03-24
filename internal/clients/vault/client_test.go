package vault

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lobo235/homelab-mcp-server/internal/clients"
)

func TestClient_CreateSecret(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/secrets/minecraft/mc-test" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	err := client.CreateSecret(context.Background(), "mc-test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClient_GetSecret(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/secrets/minecraft/mc-test" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(Secret{RCONPassword: "test-pass"})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	secret, err := client.GetSecret(context.Background(), "mc-test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if secret.RCONPassword != "test-pass" {
		t.Errorf("RCONPassword = %q, want %q", secret.RCONPassword, "test-pass")
	}
}

func TestClient_RotateSecret(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PUT" || r.URL.Path != "/secrets/minecraft/mc-test" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	err := client.RotateSecret(context.Background(), "mc-test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClient_DeleteSecret(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" || r.URL.Path != "/secrets/minecraft/mc-test" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	err := client.DeleteSecret(context.Background(), "mc-test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClient_Ping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	err := client.Ping(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClient_GatewayError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{
			"code":    "not_found",
			"message": "secret not found",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	_, err := client.GetSecret(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNewClientWithBase(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	base := clients.NewBase(srv.URL, "test-key")
	client := NewClientWithBase(base)
	if err := client.Ping(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClient_CreateSecret_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"code":    "internal",
			"message": "vault unavailable",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	err := client.CreateSecret(context.Background(), "mc-fail")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), `create secret for "mc-fail"`) {
		t.Errorf("error should wrap server name, got: %v", err)
	}
}

func TestClient_GetSecret_ErrorWrapping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{
			"code":    "forbidden",
			"message": "access denied",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	secret, err := client.GetSecret(context.Background(), "mc-denied")
	if err == nil {
		t.Fatal("expected error")
	}
	if secret != nil {
		t.Error("expected nil secret on error")
	}
	if !strings.Contains(err.Error(), `get secret for "mc-denied"`) {
		t.Errorf("error should wrap server name, got: %v", err)
	}
}

func TestClient_RotateSecret_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{
			"code":    "not_found",
			"message": "secret not found",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	err := client.RotateSecret(context.Background(), "mc-missing")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), `rotate secret for "mc-missing"`) {
		t.Errorf("error should wrap server name, got: %v", err)
	}
}

func TestClient_DeleteSecret_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"code":    "internal",
			"message": "delete failed",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	err := client.DeleteSecret(context.Background(), "mc-broken")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), `delete secret for "mc-broken"`) {
		t.Errorf("error should wrap server name, got: %v", err)
	}
}

func TestClient_Ping_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	err := client.Ping(context.Background())
	if err == nil {
		t.Fatal("expected error from unhealthy gateway")
	}
}

func TestClient_Ping_ConnectionRefused(t *testing.T) {
	client := NewClient("http://127.0.0.1:1", "test-key")
	err := client.Ping(context.Background())
	if err == nil {
		t.Fatal("expected error from unreachable server")
	}
}

func TestClient_GetSecret_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`not valid json`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	secret, err := client.GetSecret(context.Background(), "mc-badjson")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if secret != nil {
		t.Error("expected nil secret on decode error")
	}
	if !strings.Contains(err.Error(), `get secret for "mc-badjson"`) {
		t.Errorf("error should wrap server name, got: %v", err)
	}
}
