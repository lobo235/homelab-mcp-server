// Package itzhcache provides a cached copy of the itzg/docker-minecraft-server documentation.
// It fetches the docs on startup and refreshes them periodically in a background goroutine.
package itzhcache

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	// readmeURL is the raw README for itzg/docker-minecraft-server.
	readmeURL = "https://raw.githubusercontent.com/itzg/docker-minecraft-server/master/README.md"
	// envVarsURL is the raw ENVIRONMENT_VARIABLES doc.
	envVarsURL = "https://raw.githubusercontent.com/itzg/docker-minecraft-server/master/docs/environment-variables.md"
)

// docSpec defines a document to fetch and cache.
type docSpec struct {
	url      string
	filename string
}

// defaultDocs are the documents fetched by default.
var defaultDocs = []docSpec{
	{url: readmeURL, filename: "itzg-readme.md"},
	{url: envVarsURL, filename: "itzg-env-vars.md"},
}

// Cache manages on-disk copies of itzg documentation.
type Cache struct {
	dir    string
	log    *slog.Logger
	client *http.Client
	docs   []docSpec

	mu      sync.RWMutex
	loaded  bool
	lastErr error
}

// New creates a Cache that stores docs in dir.
func New(dir string, log *slog.Logger) (*Cache, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create itzg-cache dir: %w", err)
	}
	return &Cache{
		dir:    dir,
		log:    log,
		client: &http.Client{Timeout: 30 * time.Second},
		docs:   defaultDocs,
	}, nil
}

// Refresh fetches all docs from GitHub and writes them to disk.
func (c *Cache) Refresh(ctx context.Context) error {
	var lastErr error
	for _, d := range c.docs {
		if err := c.fetchDoc(ctx, d); err != nil {
			c.log.Warn("itzg-cache: fetch failed", "doc", d.filename, "error", err)
			lastErr = err
		}
	}

	c.mu.Lock()
	c.loaded = lastErr == nil || c.loaded // stay loaded if at least one prior success
	c.lastErr = lastErr
	c.mu.Unlock()

	if lastErr != nil {
		return fmt.Errorf("itzg-cache refresh: %w", lastErr)
	}
	c.log.Info("itzg-cache refreshed", "docs", len(c.docs))
	return nil
}

// StartBackgroundRefresh runs Refresh immediately, then every interval in a background goroutine.
// It stops when ctx is cancelled.
func (c *Cache) StartBackgroundRefresh(ctx context.Context, interval time.Duration) {
	// Initial fetch.
	if err := c.Refresh(ctx); err != nil {
		c.log.Warn("itzg-cache: initial refresh failed, will retry", "error", err)
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := c.Refresh(ctx); err != nil {
					c.log.Warn("itzg-cache: background refresh failed", "error", err)
				}
			}
		}
	}()
}

// Get returns the contents of a cached document by filename.
func (c *Cache) Get(filename string) (string, error) {
	data, err := os.ReadFile(filepath.Join(c.dir, filename))
	if err != nil {
		return "", fmt.Errorf("read itzg doc %q: %w", filename, err)
	}
	return string(data), nil
}

// GetReadme returns the cached itzg README.
func (c *Cache) GetReadme() (string, error) {
	return c.Get("itzg-readme.md")
}

// GetEnvVars returns the cached itzg environment variables doc.
func (c *Cache) GetEnvVars() (string, error) {
	return c.Get("itzg-env-vars.md")
}

func (c *Cache) fetchDoc(ctx context.Context, d docSpec) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("fetch %s: %w", d.filename, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch %s: status %d", d.filename, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read body %s: %w", d.filename, err)
	}

	path := filepath.Join(c.dir, d.filename)
	if err := os.WriteFile(path, body, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", d.filename, err)
	}
	return nil
}
