package ftb

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSearchPacks(t *testing.T) {
	pack1 := Pack{ID: 1, Name: "FTB Revelation", Synopsis: "Kitchen sink"}
	pack2 := Pack{ID: 2, Name: "FTB Infinity", Synopsis: "Classic pack"}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/public/modpack/search/") {
			term := r.URL.Query().Get("term")
			if term != "ftb" {
				t.Errorf("unexpected term: %s", term)
			}
			json.NewEncoder(w).Encode(SearchResponse{Packs: []int{1, 2}, Total: 2})
			return
		}
		if r.URL.Path == "/public/modpack/1" {
			json.NewEncoder(w).Encode(pack1)
			return
		}
		if r.URL.Path == "/public/modpack/2" {
			json.NewEncoder(w).Encode(pack2)
			return
		}
		t.Errorf("unexpected path: %s", r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client := NewClientWithBaseURL(srv.URL)
	packs, err := client.SearchPacks(context.Background(), "ftb")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(packs) != 2 {
		t.Fatalf("len = %d, want 2", len(packs))
	}
	if packs[0].Name != "FTB Revelation" {
		t.Errorf("Name = %q, want %q", packs[0].Name, "FTB Revelation")
	}
	if packs[1].Name != "FTB Infinity" {
		t.Errorf("Name = %q, want %q", packs[1].Name, "FTB Infinity")
	}
}

func TestSearchPacks_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("server error"))
	}))
	defer srv.Close()

	client := NewClientWithBaseURL(srv.URL)
	_, err := client.SearchPacks(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGetPack(t *testing.T) {
	pack := Pack{
		ID:       42,
		Name:     "FTB Revelation",
		Synopsis: "A big modpack",
		Tags:     []Tag{{ID: 1, Name: "tech"}},
		Versions: []PackVersionRef{{ID: 100, Name: "1.0.0"}},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/public/modpack/42" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(pack)
	}))
	defer srv.Close()

	client := NewClientWithBaseURL(srv.URL)
	result, err := client.GetPack(context.Background(), 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != "FTB Revelation" {
		t.Errorf("Name = %q, want %q", result.Name, "FTB Revelation")
	}
	if len(result.Tags) != 1 {
		t.Errorf("Tags len = %d, want 1", len(result.Tags))
	}
	if len(result.Versions) != 1 {
		t.Errorf("Versions len = %d, want 1", len(result.Versions))
	}
}

func TestGetPack_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("not found"))
	}))
	defer srv.Close()

	client := NewClientWithBaseURL(srv.URL)
	_, err := client.GetPack(context.Background(), 999)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGetVersion(t *testing.T) {
	version := PackVersion{
		ID:   100,
		Name: "1.0.0",
		Targets: []Target{
			{Name: "minecraft", Version: "1.20.1", Type: "game"},
			{Name: "forge", Version: "47.2.0", Type: "modloader"},
		},
		Files: []File{
			{ID: 1, Name: "server.jar", Path: ".", URL: "https://example.com/server.jar", Size: 1024, ServerOnly: true},
			{ID: 2, Name: "client-mod.jar", Path: "mods", URL: "https://example.com/client.jar", Size: 512, ClientOnly: true},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/public/modpack/42/100" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(version)
	}))
	defer srv.Close()

	client := NewClientWithBaseURL(srv.URL)
	result, err := client.GetVersion(context.Background(), 42, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != "1.0.0" {
		t.Errorf("Name = %q, want %q", result.Name, "1.0.0")
	}
	if len(result.Targets) != 2 {
		t.Errorf("Targets len = %d, want 2", len(result.Targets))
	}
	if len(result.Files) != 2 {
		t.Errorf("Files len = %d, want 2", len(result.Files))
	}
	if !result.Files[0].ServerOnly {
		t.Error("expected Files[0].ServerOnly = true")
	}
	if !result.Files[1].ClientOnly {
		t.Error("expected Files[1].ClientOnly = true")
	}
}

func TestGetVersion_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("not found"))
	}))
	defer srv.Close()

	client := NewClientWithBaseURL(srv.URL)
	_, err := client.GetVersion(context.Background(), 42, 999)
	if err == nil {
		t.Fatal("expected error")
	}
}
