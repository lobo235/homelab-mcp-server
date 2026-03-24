package cloudflare

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lobo235/homelab-mcp-server/internal/clients"
)

func TestClient_ListRecordsByZoneName(t *testing.T) {
	records := []DNSRecord{
		{ID: "rec-1", Type: "A", Name: "mc.example.com", Content: "192.168.1.10", TTL: 300},
		{ID: "rec-2", Type: "CNAME", Name: "web.example.com", Content: "target.example.com", TTL: 3600},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/zones-by-name/example.com/records" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
		}
		json.NewEncoder(w).Encode(records)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	result, err := client.ListRecordsByZoneName(context.Background(), "example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("len = %d, want 2", len(result))
	}
	if result[0].Name != "mc.example.com" {
		t.Errorf("result[0].Name = %q, want %q", result[0].Name, "mc.example.com")
	}
}

func TestClient_ListRecordsByZoneName_Error(t *testing.T) {
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
	_, err := client.ListRecordsByZoneName(context.Background(), "example.com")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestClient_CreateRecordByZoneName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/zones-by-name/example.com/records" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		var rec DNSRecord
		if err := json.Unmarshal(body, &rec); err != nil {
			t.Fatalf("unmarshal body: %v", err)
		}
		if rec.Type != "A" {
			t.Errorf("type = %q, want %q", rec.Type, "A")
		}
		if rec.Name != "mc.example.com" {
			t.Errorf("name = %q, want %q", rec.Name, "mc.example.com")
		}
		rec.ID = "rec-new"
		json.NewEncoder(w).Encode(rec)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	rec := DNSRecord{Type: "A", Name: "mc.example.com", Content: "192.168.1.10", TTL: 300}
	created, err := client.CreateRecordByZoneName(context.Background(), "example.com", rec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if created.ID != "rec-new" {
		t.Errorf("ID = %q, want %q", created.ID, "rec-new")
	}
}

func TestClient_CreateRecordByZoneName_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"code":    "bad_request",
			"message": "invalid record",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	rec := DNSRecord{Type: "A", Name: "mc.example.com", Content: "192.168.1.10", TTL: 300}
	_, err := client.CreateRecordByZoneName(context.Background(), "example.com", rec)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestClient_DeleteRecordByZoneName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/zones-by-name/example.com/records/mc.example.com" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	err := client.DeleteRecordByZoneName(context.Background(), "example.com", "mc.example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClient_DeleteRecordByZoneName_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{
			"code":    "not_found",
			"message": "record not found",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	err := client.DeleteRecordByZoneName(context.Background(), "example.com", "nonexistent")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestClient_CreateRecord(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/zones/zone-123/records" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		var rec DNSRecord
		if err := json.Unmarshal(body, &rec); err != nil {
			t.Fatalf("unmarshal body: %v", err)
		}
		rec.ID = "rec-created"
		json.NewEncoder(w).Encode(rec)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	rec := DNSRecord{Type: "CNAME", Name: "web.example.com", Content: "target.example.com", TTL: 3600}
	created, err := client.CreateRecord(context.Background(), "zone-123", rec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if created.ID != "rec-created" {
		t.Errorf("ID = %q, want %q", created.ID, "rec-created")
	}
}

func TestClient_CreateRecord_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"code":    "bad_request",
			"message": "invalid record",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	rec := DNSRecord{Type: "A", Name: "test.example.com", Content: "1.2.3.4", TTL: 300}
	_, err := client.CreateRecord(context.Background(), "zone-123", rec)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestClient_DeleteRecord(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/zones/zone-123/records/rec-456" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	err := client.DeleteRecord(context.Background(), "zone-123", "rec-456")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClient_DeleteRecord_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{
			"code":    "not_found",
			"message": "record not found",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	err := client.DeleteRecord(context.Background(), "zone-123", "nonexistent")
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
