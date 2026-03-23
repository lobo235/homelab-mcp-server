// Package minecraft provides an HTTP client wrapper for the minecraft-gateway API.
package minecraft

import (
	"context"
	"fmt"
	"io"

	"github.com/lobo235/homelab-mcp-server/internal/clients"
)

// Server represents a Minecraft server directory entry.
type Server struct {
	Name string `json:"name"`
}

// FileEntry represents a file in a server directory.
type FileEntry struct {
	Name  string `json:"name"`
	Size  int64  `json:"size"`
	IsDir bool   `json:"is_dir"`
}

// Backup represents a backup entry.
type Backup struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	CreatedAt string `json:"created_at"`
}

// RCONRequest represents an RCON command request.
type RCONRequest struct {
	Command string `json:"command"`
}

// RCONResponse represents the response from an RCON command.
type RCONResponse struct {
	Response string `json:"response"`
}

// Client wraps the minecraft-gateway HTTP API.
type Client struct {
	base *clients.Base
}

// NewClient creates a new minecraft-gateway client.
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

// ListServers returns all server directories.
func (c *Client) ListServers(ctx context.Context) ([]Server, error) {
	var servers []Server
	if err := c.base.DoJSON(ctx, "GET", "/servers", nil, &servers); err != nil {
		return nil, fmt.Errorf("list servers: %w", err)
	}
	return servers, nil
}

// InitServer creates a new server directory.
func (c *Client) InitServer(ctx context.Context, name string) error {
	if err := c.base.DoNoContent(ctx, "POST", "/servers/"+name, nil); err != nil {
		return fmt.Errorf("init server %q: %w", name, err)
	}
	return nil
}

// DeleteServer deletes a server directory.
func (c *Client) DeleteServer(ctx context.Context, name string) error {
	if err := c.base.DoNoContent(ctx, "DELETE", "/servers/"+name, nil); err != nil {
		return fmt.Errorf("delete server %q: %w", name, err)
	}
	return nil
}

// ListFiles returns files in a server directory.
func (c *Client) ListFiles(ctx context.Context, name string) ([]FileEntry, error) {
	var files []FileEntry
	if err := c.base.DoJSON(ctx, "GET", "/servers/"+name+"/files", nil, &files); err != nil {
		return nil, fmt.Errorf("list files for %q: %w", name, err)
	}
	return files, nil
}

// ReadFile reads a file from a server directory.
func (c *Client) ReadFile(ctx context.Context, name string) ([]byte, error) {
	resp, err := c.base.Do(ctx, "GET", "/servers/"+name+"/files/read", nil)
	if err != nil {
		return nil, fmt.Errorf("read file for %q: %w", name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("read file for %q: HTTP %d", name, resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read file body for %q: %w", name, err)
	}
	return data, nil
}

// ListBackups returns backups for a server.
func (c *Client) ListBackups(ctx context.Context, name string) ([]Backup, error) {
	var backups []Backup
	if err := c.base.DoJSON(ctx, "GET", "/servers/"+name+"/backups", nil, &backups); err != nil {
		return nil, fmt.Errorf("list backups for %q: %w", name, err)
	}
	return backups, nil
}

// CreateBackup triggers a backup for a server.
func (c *Client) CreateBackup(ctx context.Context, name string) (*Backup, error) {
	var backup Backup
	if err := c.base.DoJSON(ctx, "POST", "/servers/"+name+"/backups", nil, &backup); err != nil {
		return nil, fmt.Errorf("create backup for %q: %w", name, err)
	}
	return &backup, nil
}

// GetBackup returns a specific backup.
func (c *Client) GetBackup(ctx context.Context, name, backupID string) (*Backup, error) {
	var backup Backup
	path := fmt.Sprintf("/servers/%s/backups/%s", name, backupID)
	if err := c.base.DoJSON(ctx, "GET", path, nil, &backup); err != nil {
		return nil, fmt.Errorf("get backup %q for %q: %w", backupID, name, err)
	}
	return &backup, nil
}

// Restore triggers a restore for a server.
func (c *Client) Restore(ctx context.Context, name string, body any) error {
	if err := c.base.DoNoContent(ctx, "POST", "/servers/"+name+"/restore", body); err != nil {
		return fmt.Errorf("restore %q: %w", name, err)
	}
	return nil
}

// ExecuteRCON sends an RCON command to a server.
func (c *Client) ExecuteRCON(ctx context.Context, name, command string) (string, error) {
	req := RCONRequest{Command: command}
	var resp RCONResponse
	if err := c.base.DoJSON(ctx, "POST", "/servers/"+name+"/rcon", req, &resp); err != nil {
		return "", fmt.Errorf("rcon %q: %w", name, err)
	}
	return resp.Response, nil
}
