// Package curseforge provides an HTTP client wrapper for the curseforge-gateway API.
package curseforge

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/lobo235/homelab-mcp-server/internal/clients"
)

// Modpack represents a CurseForge modpack.
type Modpack struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Summary string `json:"summary"`
}

// Mod represents a CurseForge mod.
type Mod struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Summary string `json:"summary"`
}

// ModDependency represents a dependency relationship for a CurseForge file.
type ModDependency struct {
	ModID        int `json:"modId"`
	RelationType int `json:"relationType"`
}

// ModpackFile represents a file/version of a modpack.
type ModpackFile struct {
	ID               int             `json:"id"`
	DisplayName      string          `json:"displayName"`
	FileName         string          `json:"fileName"`
	GameVersions     []string        `json:"gameVersions"`
	IsServerPack     bool            `json:"isServerPack"`
	DownloadURL      string          `json:"downloadUrl"`
	FileLength       int64           `json:"fileLength"`
	ReleaseType      int             `json:"releaseType"`
	ServerPackFileID int             `json:"serverPackFileId"`
	Changelog        string          `json:"changelog,omitempty"`
	Dependencies     []ModDependency `json:"dependencies,omitempty"`
}

// Client wraps the curseforge-gateway HTTP API.
type Client struct {
	base *clients.Base
}

// NewClient creates a new curseforge-gateway client.
func NewClient(baseURL, apiKey string) *Client {
	return &Client{base: clients.NewBase(baseURL, apiKey)}
}

// NewClientWithBase creates a client using a pre-configured Base (with retry/circuit breaker).
func NewClientWithBase(base *clients.Base) *Client {
	return &Client{base: base}
}

// Ping checks gateway reachability.
func (c *Client) Ping(ctx context.Context) error {
	return c.base.Ping(ctx)
}

// GetModpack returns modpack details by project ID.
func (c *Client) GetModpack(ctx context.Context, projectID string) (*Modpack, error) {
	var modpack Modpack
	if err := c.base.DoJSON(ctx, "GET", "/modpacks/"+projectID, nil, &modpack); err != nil {
		return nil, fmt.Errorf("get modpack %s: %w", projectID, err)
	}
	return &modpack, nil
}

// GetModpackFiles returns available files for a modpack.
func (c *Client) GetModpackFiles(ctx context.Context, projectID string) ([]ModpackFile, error) {
	var files []ModpackFile
	if err := c.base.DoJSON(ctx, "GET", "/modpacks/"+projectID+"/files", nil, &files); err != nil {
		return nil, fmt.Errorf("get modpack files %s: %w", projectID, err)
	}
	return files, nil
}

// GetMod returns mod details by project ID.
func (c *Client) GetMod(ctx context.Context, projectID string) (*Mod, error) {
	var mod Mod
	if err := c.base.DoJSON(ctx, "GET", "/mods/"+projectID, nil, &mod); err != nil {
		return nil, fmt.Errorf("get mod %s: %w", projectID, err)
	}
	return &mod, nil
}

// GetModFiles returns available files for a mod.
func (c *Client) GetModFiles(ctx context.Context, projectID string) ([]ModpackFile, error) {
	var files []ModpackFile
	if err := c.base.DoJSON(ctx, "GET", "/mods/"+projectID+"/files", nil, &files); err != nil {
		return nil, fmt.Errorf("get mod files %s: %w", projectID, err)
	}
	return files, nil
}

// GetModpackFile returns a single file for a modpack by file ID.
func (c *Client) GetModpackFile(ctx context.Context, projectID, fileID string) (*ModpackFile, error) {
	var file ModpackFile
	path := fmt.Sprintf("/modpacks/%s/files/%s", projectID, fileID)
	if err := c.base.DoJSON(ctx, "GET", path, nil, &file); err != nil {
		return nil, fmt.Errorf("get modpack file %s/%s: %w", projectID, fileID, err)
	}
	return &file, nil
}

// GetModFile returns a single file for a mod by file ID.
func (c *Client) GetModFile(ctx context.Context, projectID, fileID string) (*ModpackFile, error) {
	var file ModpackFile
	path := fmt.Sprintf("/mods/%s/files/%s", projectID, fileID)
	if err := c.base.DoJSON(ctx, "GET", path, nil, &file); err != nil {
		return nil, fmt.Errorf("get mod file %s/%s: %w", projectID, fileID, err)
	}
	return &file, nil
}

// SearchResult represents a CurseForge search result.
type SearchResult struct {
	ID           int      `json:"id"`
	Name         string   `json:"name"`
	Slug         string   `json:"slug,omitempty"`
	Summary      string   `json:"summary"`
	ClassID      int      `json:"classId"`
	GameVersions []string `json:"gameVersions,omitempty"`
}

// SearchModpacks searches for modpacks by name.
func (c *Client) SearchModpacks(ctx context.Context, query string) ([]SearchResult, error) {
	var results []SearchResult
	path := "/search?type=modpack&query=" + url.QueryEscape(query)
	if err := c.base.DoJSON(ctx, "GET", path, nil, &results); err != nil {
		return nil, fmt.Errorf("search modpacks %q: %w", query, err)
	}
	return results, nil
}

// SearchModpackBySlug searches for a modpack by slug/name and returns the best match.
// Returns nil if no match is found.
func (c *Client) SearchModpackBySlug(ctx context.Context, slug string) (*SearchResult, error) {
	results, err := c.SearchModpacks(ctx, slug)
	if err != nil {
		return nil, err
	}

	normalizedSlug := strings.ToLower(strings.ReplaceAll(slug, "-", " "))
	for i := range results {
		resultSlug := strings.ToLower(strings.ReplaceAll(results[i].Slug, "-", " "))
		resultName := strings.ToLower(results[i].Name)
		if resultSlug == normalizedSlug || resultName == normalizedSlug {
			return &results[i], nil
		}
	}

	// No exact slug match; return first result if available.
	if len(results) > 0 {
		return &results[0], nil
	}
	return nil, nil
}

// SearchMods searches for mods by name.
func (c *Client) SearchMods(ctx context.Context, query string) ([]SearchResult, error) {
	var results []SearchResult
	path := "/search?type=mod&query=" + url.QueryEscape(query)
	if err := c.base.DoJSON(ctx, "GET", path, nil, &results); err != nil {
		return nil, fmt.Errorf("search mods %q: %w", query, err)
	}
	return results, nil
}
