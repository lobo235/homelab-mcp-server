package itzgcache

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func testServer() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/readme", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("# itzg README"))
	})
	mux.HandleFunc("/envvars", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("# Environment Variables"))
	})
	return httptest.NewServer(mux)
}

func testDocs(srvURL string) []docSpec {
	return []docSpec{
		{url: srvURL + "/readme", filename: "itzg-readme.md"},
		{url: srvURL + "/envvars", filename: "itzg-env-vars.md"},
	}
}

func TestRefresh_FetchesAndCaches(t *testing.T) {
	srv := testServer()
	defer srv.Close()

	c, _ := New(t.TempDir(), testLogger())
	c.docs = testDocs(srv.URL)

	err := c.Refresh(context.Background())
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}

	readme, err := c.GetReadme()
	if err != nil {
		t.Fatalf("GetReadme() error = %v", err)
	}
	if readme != "# itzg README" {
		t.Errorf("GetReadme() = %q, want %q", readme, "# itzg README")
	}

	envVars, err := c.GetEnvVars()
	if err != nil {
		t.Fatalf("GetEnvVars() error = %v", err)
	}
	if envVars != "# Environment Variables" {
		t.Errorf("GetEnvVars() = %q, want %q", envVars, "# Environment Variables")
	}
}

func TestRefresh_HandlesHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c, _ := New(t.TempDir(), testLogger())
	c.docs = []docSpec{
		{url: srv.URL + "/readme", filename: "itzg-readme.md"},
		{url: srv.URL + "/envvars", filename: "itzg-env-vars.md"},
	}

	err := c.Refresh(context.Background())
	if err == nil {
		t.Fatal("Refresh() expected error on HTTP 500")
	}
}

func TestGet_NotFound(t *testing.T) {
	c, _ := New(t.TempDir(), testLogger())
	_, err := c.Get("nonexistent.md")
	if err == nil {
		t.Fatal("Get() expected error for missing file")
	}
}

func TestStartBackgroundRefresh_Cancellation(t *testing.T) {
	srv := testServer()
	defer srv.Close()

	c, _ := New(t.TempDir(), testLogger())
	c.docs = testDocs(srv.URL)

	ctx, cancel := context.WithCancel(context.Background())
	c.StartBackgroundRefresh(ctx, 100*time.Millisecond)

	// Wait for initial refresh.
	time.Sleep(50 * time.Millisecond)
	cancel()

	// Verify docs were fetched.
	_, err := c.GetReadme()
	if err != nil {
		t.Fatalf("GetReadme() after refresh error = %v", err)
	}
}
