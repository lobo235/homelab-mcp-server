// Package speccache provides on-disk caching of Nomad job specs fetched from nomad-gateway.
// On startup, if the cache directory is empty, it seeds itself by fetching all running job specs.
package speccache

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// SeedFunc fetches all job IDs and their specs. Returns a map of jobID → HCL.
type SeedFunc func(ctx context.Context) (map[string]string, error)

// Cache manages on-disk spec files in a directory.
type Cache struct {
	dir string
	log *slog.Logger
}

// New creates a Cache rooted at dir. The directory is created if it does not exist.
func New(dir string, log *slog.Logger) (*Cache, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create spec-cache dir: %w", err)
	}
	return &Cache{dir: dir, log: log}, nil
}

// Seed populates the cache using the provided fetch function,
// but only if the cache directory contains no .hcl files.
func (c *Cache) Seed(ctx context.Context, fetch SeedFunc) error {
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return fmt.Errorf("read spec-cache dir: %w", err)
	}

	// Count existing .hcl files.
	count := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".hcl") {
			count++
		}
	}
	if count > 0 {
		c.log.Info("spec-cache already populated, skipping seed", "cached_specs", count)
		return nil
	}

	c.log.Info("spec-cache empty, seeding from nomad-gateway")

	specs, err := fetch(ctx)
	if err != nil {
		return fmt.Errorf("seed fetch: %w", err)
	}

	seeded := 0
	for jobID, hcl := range specs {
		if writeErr := c.Write(jobID, hcl); writeErr != nil {
			c.log.Warn("seed: failed to write spec", "job", jobID, "error", writeErr)
			continue
		}
		seeded++
	}

	c.log.Info("spec-cache seed complete", "seeded", seeded, "total_jobs", len(specs))
	return nil
}

// Get reads a cached spec from disk.
func (c *Cache) Get(jobID string) (string, error) {
	data, err := os.ReadFile(c.path(jobID))
	if err != nil {
		return "", fmt.Errorf("read cached spec %q: %w", jobID, err)
	}
	return string(data), nil
}

// Write saves an HCL spec to disk.
func (c *Cache) Write(jobID, hcl string) error {
	if err := os.WriteFile(c.path(jobID), []byte(hcl), 0o644); err != nil {
		return fmt.Errorf("write cached spec %q: %w", jobID, err)
	}
	return nil
}

// List returns all cached job IDs.
func (c *Cache) List() ([]string, error) {
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return nil, fmt.Errorf("read spec-cache dir: %w", err)
	}

	var ids []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".hcl") {
			ids = append(ids, strings.TrimSuffix(e.Name(), ".hcl"))
		}
	}
	return ids, nil
}

func (c *Cache) path(jobID string) string {
	return filepath.Join(c.dir, jobID+".hcl")
}
