package minecraft

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lobo235/homelab-mcp-server/internal/clients"
)

func TestClient_ExecuteRCON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/servers/mc-atm10/rcon" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		var req RCONRequest
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("unmarshal body: %v", err)
		}
		if req.Command != "list" {
			t.Errorf("command = %q, want %q", req.Command, "list")
		}
		json.NewEncoder(w).Encode(RCONResponse{
			Response: "There are 3 of a max of 20 players online: Steve, Alex, Notch",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	resp, err := client.ExecuteRCON(context.Background(), "mc-atm10", "list")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "There are 3 of a max of 20 players online: Steve, Alex, Notch" {
		t.Errorf("response = %q, unexpected", resp)
	}
}

func TestClient_ExecuteRCON_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]string{
			"code":    "upstream_error",
			"message": "RCON connection refused",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	_, err := client.ExecuteRCON(context.Background(), "mc-atm10", "list")
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
