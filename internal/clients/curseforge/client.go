// Package curseforge provides an HTTP client wrapper for the curseforge-gateway API.
package curseforge

import (
	"context"
	"fmt"

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

// ModpackFile represents a file/version of a modpack.
type ModpackFile struct {
	ID           int      `json:"id"`
	DisplayName  string   `json:"displayName"`
	FileName     string   `json:"fileName"`
	GameVersions []string `json:"gameVersions"`
	IsServerPack bool     `json:"isServerPack"`
	DownloadURL  string   `json:"downloadUrl"`
	FileLength   int64    `json:"fileLength"`
	ReleaseType  int      `json:"releaseType"`
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
