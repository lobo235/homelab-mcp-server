package adguard

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lobo235/homelab-mcp-server/internal/clients"
)

func TestClient_ListRewrites(t *testing.T) {
	rewrites := []Rewrite{
		{Domain: "mc.example.com", Answer: "192.168.1.10"},
		{Domain: "web.example.com", Answer: "192.168.1.20"},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/rewrites" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
		}
		json.NewEncoder(w).Encode(rewrites)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	result, err := client.ListRewrites(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("len = %d, want 2", len(result))
	}
	if result[0].Domain != "mc.example.com" {
		t.Errorf("result[0].Domain = %q, want %q", result[0].Domain, "mc.example.com")
	}
	if result[1].Answer != "192.168.1.20" {
		t.Errorf("result[1].Answer = %q, want %q", result[1].Answer, "192.168.1.20")
	}
}

func TestClient_ListRewrites_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"code":    "internal_error",
			"message": "upstream failure",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	_, err := client.ListRewrites(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestClient_CreateRewrite(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/rewrites" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		var rw Rewrite
		if err := json.Unmarshal(body, &rw); err != nil {
			t.Fatalf("unmarshal body: %v", err)
		}
		if rw.Domain != "mc.example.com" {
			t.Errorf("domain = %q, want %q", rw.Domain, "mc.example.com")
		}
		if rw.Answer != "192.168.1.10" {
			t.Errorf("answer = %q, want %q", rw.Answer, "192.168.1.10")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	err := client.CreateRewrite(context.Background(), "mc.example.com", "192.168.1.10")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClient_CreateRewrite_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]string{
			"code":    "conflict",
			"message": "rewrite already exists",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	err := client.CreateRewrite(context.Background(), "mc.example.com", "192.168.1.10")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestClient_DeleteRewrite(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/rewrites/mc.example.com" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	err := client.DeleteRewrite(context.Background(), "mc.example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClient_DeleteRewrite_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{
			"code":    "not_found",
			"message": "rewrite not found",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	err := client.DeleteRewrite(context.Background(), "nonexistent.example.com")
	if err == nil {
		t.Fatal("expected error")
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

func TestNewClientWithBase(t *testing.T) {
	base := clients.NewBase("http://localhost:8080", "key")
	client := NewClientWithBase(base)
	if client == nil {
		t.Fatal("expected non-nil client")
	}
}
