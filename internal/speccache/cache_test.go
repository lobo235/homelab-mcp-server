package speccache

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestNew_CreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "spec-cache")
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

func TestWriteAndGet(t *testing.T) {
	c, _ := New(t.TempDir(), testLogger())
	hcl := `job "test" { type = "service" }`

	if err := c.Write("test-job", hcl); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	got, err := c.Get("test-job")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != hcl {
		t.Errorf("Get() = %q, want %q", got, hcl)
	}
}

func TestGet_NotFound(t *testing.T) {
	c, _ := New(t.TempDir(), testLogger())
	_, err := c.Get("nonexistent")
	if err == nil {
		t.Fatal("Get() expected error for missing spec")
	}
}

func TestList(t *testing.T) {
	c, _ := New(t.TempDir(), testLogger())
	_ = c.Write("job-a", "a")
	_ = c.Write("job-b", "b")

	ids, err := c.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("List() returned %d items, want 2", len(ids))
	}
}

func TestSeed_PopulatesEmptyCache(t *testing.T) {
	c, _ := New(t.TempDir(), testLogger())

	fetch := func(_ context.Context) (map[string]string, error) {
		return map[string]string{
			"mc-server1": `job "mc-server1" {}`,
			"mc-server2": `job "mc-server2" {}`,
		}, nil
	}

	if err := c.Seed(context.Background(), fetch); err != nil {
		t.Fatalf("Seed() error = %v", err)
	}

	ids, _ := c.List()
	if len(ids) != 2 {
		t.Fatalf("after seed: List() = %d, want 2", len(ids))
	}
}

func TestSeed_SkipsPopulatedCache(t *testing.T) {
	c, _ := New(t.TempDir(), testLogger())
	_ = c.Write("existing", "data")

	called := false
	fetch := func(_ context.Context) (map[string]string, error) {
		called = true
		return map[string]string{"new": "spec"}, nil
	}

	if err := c.Seed(context.Background(), fetch); err != nil {
		t.Fatalf("Seed() error = %v", err)
	}
	if called {
		t.Error("Seed() called fetch function on non-empty cache")
	}
}

func TestSeed_FetchError(t *testing.T) {
	c, _ := New(t.TempDir(), testLogger())

	fetch := func(_ context.Context) (map[string]string, error) {
		return nil, fmt.Errorf("connection refused")
	}

	err := c.Seed(context.Background(), fetch)
	if err == nil {
		t.Fatal("Seed() expected error on fetch failure")
	}
}
