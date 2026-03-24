// Package itzgcache provides a cached copy of the itzg/docker-minecraft-server documentation.
// It fetches the docs on startup and refreshes them periodically in a background goroutine.
// Documentation pages are discovered dynamically via the GitHub API tree endpoint.
package itzgcache

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	// defaultTreeURL is the GitHub API endpoint for the repo's recursive tree.
	defaultTreeURL = "https://api.github.com/repos/itzg/docker-minecraft-server/git/trees/master?recursive=1"
	// defaultRawBase is the base URL for fetching raw file content.
	defaultRawBase = "https://raw.githubusercontent.com/itzg/docker-minecraft-server/master/"
	// maxSearchResults is the maximum number of search results returned.
	maxSearchResults = 20
)

// SearchResult represents a single search match within a cached doc.
type SearchResult struct {
	Filename string `json:"filename"`
	Line     string `json:"line"`
	LineNum  int    `json:"line_num"`
}

// treeResponse is the GitHub API response for a recursive tree listing.
type treeResponse struct {
	Tree []treeEntry `json:"tree"`
}

// treeEntry is a single entry in the GitHub tree response.
type treeEntry struct {
	Path string `json:"path"`
	Type string `json:"type"`
}

// Cache manages on-disk copies of itzg documentation.
type Cache struct {
	dir     string
	log     *slog.Logger
	client  *http.Client
	treeURL string
	rawBase string

	mu       sync.RWMutex
	loaded   bool
	lastErr  error
	docPaths []string // cached list of doc file paths (relative, e.g. "docs/foo.md")
}

// New creates a Cache that stores docs in dir.
func New(dir string, log *slog.Logger) (*Cache, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create itzg-cache dir: %w", err)
	}
	return &Cache{
		dir:     dir,
		log:     log,
		client:  &http.Client{Timeout: 30 * time.Second},
		treeURL: defaultTreeURL,
		rawBase: defaultRawBase,
	}, nil
}

// Refresh discovers all documentation files via the GitHub API tree endpoint,
// then fetches each file and writes it to disk preserving directory structure.
func (c *Cache) Refresh(ctx context.Context) error {
	paths, err := c.discoverDocs(ctx)
	if err != nil {
		c.mu.Lock()
		c.lastErr = err
		c.mu.Unlock()
		return fmt.Errorf("itzg-cache refresh: %w", err)
	}

	var lastErr error
	var fetched int
	cacheBust := fmt.Sprintf("?t=%d", time.Now().Unix())
	for _, p := range paths {
		url := c.rawBase + p + cacheBust
		if fetchErr := c.fetchFile(ctx, url, p); fetchErr != nil {
			c.log.Warn("itzg-cache: fetch failed", "doc", p, "error", fetchErr)
			lastErr = fetchErr
		} else {
			fetched++
		}
	}

	c.mu.Lock()
	c.loaded = fetched > 0 || c.loaded
	c.lastErr = lastErr
	c.docPaths = paths
	c.mu.Unlock()

	if lastErr != nil {
		return fmt.Errorf("itzg-cache refresh: %w", lastErr)
	}
	c.log.Info("itzg-cache refreshed", "docs", fetched)
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

// Get returns the contents of a cached document by filename (relative path).
func (c *Cache) Get(filename string) (string, error) {
	// Sanitize: prevent path traversal.
	clean := filepath.Clean(filename)
	if strings.HasPrefix(clean, "..") {
		return "", fmt.Errorf("invalid filename %q", filename)
	}
	data, err := os.ReadFile(filepath.Join(c.dir, clean))
	if err != nil {
		return "", fmt.Errorf("read itzg doc %q: %w", filename, err)
	}
	return string(data), nil
}

// ListDocs returns all cached doc filenames (relative paths).
func (c *Cache) ListDocs() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]string, len(c.docPaths))
	copy(out, c.docPaths)
	return out
}

// Search performs a case-insensitive substring search across all cached docs.
// Returns up to maxSearchResults matching lines with filenames and line numbers.
func (c *Cache) Search(query string) []SearchResult {
	c.mu.RLock()
	paths := c.docPaths
	c.mu.RUnlock()

	lowerQuery := strings.ToLower(query)
	var results []SearchResult

	for _, p := range paths {
		if len(results) >= maxSearchResults {
			break
		}
		fullPath := filepath.Join(c.dir, p)
		f, err := os.Open(fullPath)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(f)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := scanner.Text()
			if strings.Contains(strings.ToLower(line), lowerQuery) {
				results = append(results, SearchResult{
					Filename: p,
					Line:     line,
					LineNum:  lineNum,
				})
				if len(results) >= maxSearchResults {
					break
				}
			}
		}
		_ = f.Close()
	}

	return results
}

// discoverDocs calls the GitHub API tree endpoint to find all doc files.
// It returns files matching docs/**/*.md and README.md.
func (c *Cache) discoverDocs(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.treeURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create tree request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch tree: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch tree: status %d", resp.StatusCode)
	}

	var tree treeResponse
	if err := json.NewDecoder(resp.Body).Decode(&tree); err != nil {
		return nil, fmt.Errorf("decode tree: %w", err)
	}

	var paths []string
	for _, entry := range tree.Tree {
		if entry.Type != "blob" {
			continue
		}
		if entry.Path == "README.md" {
			paths = append(paths, entry.Path)
			continue
		}
		if strings.HasPrefix(entry.Path, "docs/") && strings.HasSuffix(entry.Path, ".md") {
			paths = append(paths, entry.Path)
		}
	}

	sort.Strings(paths)
	return paths, nil
}

// fetchFile downloads a single file from url and writes it to the cache dir at relPath.
func (c *Cache) fetchFile(ctx context.Context, url, relPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("fetch %s: %w", relPath, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch %s: status %d", relPath, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read body %s: %w", relPath, err)
	}

	dest := filepath.Join(c.dir, relPath)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("create dir for %s: %w", relPath, err)
	}
	if err := os.WriteFile(dest, body, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", relPath, err)
	}
	return nil
}
