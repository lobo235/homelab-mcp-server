package curseforge

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lobo235/homelab-mcp-server/internal/clients"
)

func TestClient_GetModpack(t *testing.T) {
	modpack := Modpack{ID: 12345, Name: "All the Mods 10", Summary: "Kitchen sink modpack"}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/modpacks/12345" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
		}
		json.NewEncoder(w).Encode(modpack)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	result, err := client.GetModpack(context.Background(), "12345")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ID != 12345 {
		t.Errorf("ID = %d, want %d", result.ID, 12345)
	}
	if result.Name != "All the Mods 10" {
		t.Errorf("Name = %q, want %q", result.Name, "All the Mods 10")
	}
}

func TestClient_GetModpack_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{
			"code":    "not_found",
			"message": "modpack not found",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	_, err := client.GetModpack(context.Background(), "99999")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestClient_GetModpackFiles(t *testing.T) {
	files := []ModpackFile{
		{ID: 100, DisplayName: "ATM10-1.0.0", FileName: "atm10-1.0.0.zip", GameVersions: []string{"1.20.1"}, IsServerPack: false},
		{ID: 101, DisplayName: "ATM10-1.0.0-server", FileName: "atm10-1.0.0-server.zip", GameVersions: []string{"1.20.1"}, IsServerPack: true},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/modpacks/12345/files" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(files)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	result, err := client.GetModpackFiles(context.Background(), "12345")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("len = %d, want 2", len(result))
	}
	if result[1].IsServerPack != true {
		t.Error("expected result[1].IsServerPack to be true")
	}
}

func TestClient_GetModpackFiles_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{
			"code":    "not_found",
			"message": "modpack not found",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	_, err := client.GetModpackFiles(context.Background(), "99999")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestClient_GetMod(t *testing.T) {
	mod := Mod{ID: 54321, Name: "JEI", Summary: "Just Enough Items"}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/mods/54321" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(mod)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	result, err := client.GetMod(context.Background(), "54321")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != "JEI" {
		t.Errorf("Name = %q, want %q", result.Name, "JEI")
	}
}

func TestClient_GetMod_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{
			"code":    "not_found",
			"message": "mod not found",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	_, err := client.GetMod(context.Background(), "99999")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestClient_GetModFiles(t *testing.T) {
	files := []ModpackFile{
		{ID: 200, DisplayName: "JEI-1.0.0", FileName: "jei-1.0.0.jar"},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/mods/54321/files" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(files)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	result, err := client.GetModFiles(context.Background(), "54321")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("len = %d, want 1", len(result))
	}
	if result[0].DisplayName != "JEI-1.0.0" {
		t.Errorf("DisplayName = %q, want %q", result[0].DisplayName, "JEI-1.0.0")
	}
}

func TestClient_GetModFiles_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{
			"code":    "not_found",
			"message": "mod not found",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	_, err := client.GetModFiles(context.Background(), "99999")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestClient_GetModpackFile(t *testing.T) {
	file := ModpackFile{ID: 100, DisplayName: "ATM10-1.0.0", FileName: "atm10-1.0.0.zip"}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/modpacks/12345/files/100" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(file)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	result, err := client.GetModpackFile(context.Background(), "12345", "100")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ID != 100 {
		t.Errorf("ID = %d, want %d", result.ID, 100)
	}
}

func TestClient_GetModpackFile_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{
			"code":    "not_found",
			"message": "file not found",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	_, err := client.GetModpackFile(context.Background(), "12345", "999")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestClient_GetModFile(t *testing.T) {
	file := ModpackFile{ID: 200, DisplayName: "JEI-1.0.0", FileName: "jei-1.0.0.jar"}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/mods/54321/files/200" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(file)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	result, err := client.GetModFile(context.Background(), "54321", "200")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.FileName != "jei-1.0.0.jar" {
		t.Errorf("FileName = %q, want %q", result.FileName, "jei-1.0.0.jar")
	}
}

func TestClient_GetModFile_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{
			"code":    "not_found",
			"message": "file not found",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	_, err := client.GetModFile(context.Background(), "54321", "999")
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
