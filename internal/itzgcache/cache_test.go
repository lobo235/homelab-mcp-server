package itzgcache

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestNew_CreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "itzg-docs")
	c, err := New(dir, testLogger())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if c == nil {
		t.Fatal("New() returned nil")
	}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Fatal("directory was not created")
	}
}

// mockGitHubServer creates a test server that mocks the GitHub API tree endpoint
// and raw content endpoints for documentation files.
func mockGitHubServer() *httptest.Server {
	mux := http.NewServeMux()

	// Mock the GitHub API tree endpoint.
	mux.HandleFunc("/tree", func(w http.ResponseWriter, _ *http.Request) {
		tree := treeResponse{
			Tree: []treeEntry{
				{Path: "README.md", Type: "blob"},
				{Path: "docs", Type: "tree"},
				{Path: "docs/configuration", Type: "tree"},
				{Path: "docs/configuration/server-properties.md", Type: "blob"},
				{Path: "docs/configuration/interpolating.md", Type: "blob"},
				{Path: "docs/types-and-platforms", Type: "tree"},
				{Path: "docs/types-and-platforms/server-types/curseforge.md", Type: "blob"},
				{Path: "src/main.go", Type: "blob"},
				{Path: "docs/misc/image.png", Type: "blob"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tree)
	})

	// Mock raw content endpoints.
	mux.HandleFunc("/raw/README.md", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("# itzg/docker-minecraft-server\nThis is the main README.\nCURSEFORGE support is available."))
	})
	mux.HandleFunc("/raw/docs/configuration/server-properties.md", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("# Server Properties\nUse SERVER_PROPERTIES env var.\nMEMORY settings here."))
	})
	mux.HandleFunc("/raw/docs/configuration/interpolating.md", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("# Interpolating\nVariable interpolation docs."))
	})
	mux.HandleFunc("/raw/docs/types-and-platforms/server-types/curseforge.md", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("# CurseForge\nSet TYPE=CURSEFORGE and CF_PAGE_URL.\nCURSEFORGE modpack download."))
	})

	return httptest.NewServer(mux)
}

// setupTestCache creates a Cache wired to a mock server for testing.
func setupTestCache(t *testing.T, srv *httptest.Server) *Cache {
	t.Helper()
	c, err := New(t.TempDir(), testLogger())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	c.treeURL = srv.URL + "/tree"
	c.rawBase = srv.URL + "/raw/"
	return c
}

func TestRefresh_FetchesAllDocs(t *testing.T) {
	srv := mockGitHubServer()
	defer srv.Close()

	c := setupTestCache(t, srv)

	err := c.Refresh(context.Background())
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}

	// Should have discovered 4 docs (README + 3 under docs/).
	docs := c.ListDocs()
	if len(docs) != 4 {
		t.Fatalf("ListDocs() got %d docs, want 4: %v", len(docs), docs)
	}

	// Verify README content.
	readme, err := c.Get("README.md")
	if err != nil {
		t.Fatalf("Get(README.md) error = %v", err)
	}
	if !strings.Contains(readme, "itzg/docker-minecraft-server") {
		t.Errorf("README content unexpected: %q", readme)
	}

	// Verify nested doc content.
	sp, err := c.Get("docs/configuration/server-properties.md")
	if err != nil {
		t.Fatalf("Get(docs/configuration/server-properties.md) error = %v", err)
	}
	if !strings.Contains(sp, "Server Properties") {
		t.Errorf("server-properties content unexpected: %q", sp)
	}

	// Verify deeply nested doc.
	cf, err := c.Get("docs/types-and-platforms/server-types/curseforge.md")
	if err != nil {
		t.Fatalf("Get(curseforge.md) error = %v", err)
	}
	if !strings.Contains(cf, "CurseForge") {
		t.Errorf("curseforge content unexpected: %q", cf)
	}
}

func TestRefresh_HandlesTreeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := setupTestCache(t, srv)
	err := c.Refresh(context.Background())
	if err == nil {
		t.Fatal("Refresh() expected error on tree HTTP 500")
	}
}

func TestRefresh_HandlesFetchError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/tree", func(w http.ResponseWriter, _ *http.Request) {
		tree := treeResponse{
			Tree: []treeEntry{
				{Path: "README.md", Type: "blob"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tree)
	})
	mux.HandleFunc("/raw/README.md", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := setupTestCache(t, srv)
	err := c.Refresh(context.Background())
	if err == nil {
		t.Fatal("Refresh() expected error on file fetch HTTP 500")
	}
}

func TestGet_NotFound(t *testing.T) {
	c, _ := New(t.TempDir(), testLogger())
	_, err := c.Get("nonexistent.md")
	if err == nil {
		t.Fatal("Get() expected error for missing file")
	}
}

func TestGet_PathTraversal(t *testing.T) {
	c, _ := New(t.TempDir(), testLogger())
	_, err := c.Get("../../etc/passwd")
	if err == nil {
		t.Fatal("Get() expected error for path traversal")
	}
}

func TestListDocs_Empty(t *testing.T) {
	c, _ := New(t.TempDir(), testLogger())
	docs := c.ListDocs()
	if len(docs) != 0 {
		t.Fatalf("ListDocs() got %d, want 0", len(docs))
	}
}

func TestSearch_FindsMatches(t *testing.T) {
	srv := mockGitHubServer()
	defer srv.Close()

	c := setupTestCache(t, srv)
	if err := c.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}

	results := c.Search("CURSEFORGE")
	if len(results) == 0 {
		t.Fatal("Search(CURSEFORGE) returned no results")
	}

	// Should find matches in both README and curseforge doc.
	files := make(map[string]bool)
	for _, r := range results {
		files[r.Filename] = true
	}
	if !files["README.md"] {
		t.Error("expected match in README.md")
	}
	if !files["docs/types-and-platforms/server-types/curseforge.md"] {
		t.Error("expected match in curseforge.md")
	}
}

func TestSearch_CaseInsensitive(t *testing.T) {
	srv := mockGitHubServer()
	defer srv.Close()

	c := setupTestCache(t, srv)
	if err := c.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}

	results := c.Search("memory")
	if len(results) == 0 {
		t.Fatal("Search(memory) returned no results (case-insensitive)")
	}
	if results[0].Filename != "docs/configuration/server-properties.md" {
		t.Errorf("expected match in server-properties.md, got %s", results[0].Filename)
	}
}

func TestSearch_NoMatch(t *testing.T) {
	srv := mockGitHubServer()
	defer srv.Close()

	c := setupTestCache(t, srv)
	if err := c.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}

	results := c.Search("ZZZNONEXISTENT")
	if len(results) != 0 {
		t.Fatalf("Search(ZZZNONEXISTENT) got %d results, want 0", len(results))
	}
}

func TestSearch_LimitsResults(t *testing.T) {
	srv := mockGitHubServer()
	defer srv.Close()

	c := setupTestCache(t, srv)
	if err := c.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}

	// Search for something that appears in many lines (lowercase chars).
	results := c.Search("e")
	if len(results) > maxSearchResults {
		t.Fatalf("Search returned %d results, should be capped at %d", len(results), maxSearchResults)
	}
}

func TestStartBackgroundRefresh_Cancellation(t *testing.T) {
	srv := mockGitHubServer()
	defer srv.Close()

	c := setupTestCache(t, srv)

	ctx, cancel := context.WithCancel(context.Background())
	c.StartBackgroundRefresh(ctx, 100*time.Millisecond)

	// Wait for initial refresh.
	time.Sleep(50 * time.Millisecond)
	cancel()

	// Verify docs were fetched.
	docs := c.ListDocs()
	if len(docs) == 0 {
		t.Fatal("ListDocs() empty after background refresh")
	}
	_, err := c.Get("README.md")
	if err != nil {
		t.Fatalf("Get(README.md) after refresh error = %v", err)
	}
}
