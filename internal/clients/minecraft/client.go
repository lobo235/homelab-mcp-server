// Package minecraft provides an HTTP client wrapper for the minecraft-gateway API.
package minecraft

import (
	"context"
	"fmt"
	"net/url"

	"github.com/lobo235/homelab-mcp-server/internal/clients"
)

// FileEntry represents a file in a server directory.
type FileEntry struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	IsDir   bool   `json:"is_dir"`
	ModTime string `json:"mod_time"`
}

// BackupInfo represents a backup file entry from the list endpoint.
type BackupInfo struct {
	ID      string `json:"id"`
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	Created string `json:"created"`
}

// BackupStatus represents backup status from create/get endpoints.
type BackupStatus struct {
	Server      string `json:"server"`
	ID          string `json:"id,omitempty"`
	BackupID    string `json:"backup_id,omitempty"`
	Status      string `json:"status"`
	StartedAt   string `json:"started_at,omitempty"`
	CompletedAt string `json:"completed_at,omitempty"`
	BackupPath  string `json:"backup_path,omitempty"`
	Error       string `json:"error,omitempty"`
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

// ListServers returns all server directory names.
func (c *Client) ListServers(ctx context.Context) ([]string, error) {
	var envelope struct {
		Servers []string `json:"servers"`
	}
	if err := c.base.DoJSON(ctx, "GET", "/servers", nil, &envelope); err != nil {
		return nil, fmt.Errorf("list servers: %w", err)
	}
	return envelope.Servers, nil
}

// initServerRequest is the JSON body for POST /servers.
type initServerRequest struct {
	Name string `json:"name"`
	UID  int    `json:"uid"`
	GID  int    `json:"gid"`
}

// InitServer creates a new server directory.
func (c *Client) InitServer(ctx context.Context, name string, uid, gid int) error {
	body := initServerRequest{Name: name, UID: uid, GID: gid}
	if err := c.base.DoNoContent(ctx, "POST", "/servers", body); err != nil {
		return fmt.Errorf("init server %q: %w", name, err)
	}
	return nil
}

// DeleteServer deletes a server directory.
func (c *Client) DeleteServer(ctx context.Context, name string) error {
	if err := c.base.DoNoContent(ctx, "DELETE", "/servers/"+name+"?confirm=true", nil); err != nil {
		return fmt.Errorf("delete server %q: %w", name, err)
	}
	return nil
}

// ListFiles returns files in a server directory.
func (c *Client) ListFiles(ctx context.Context, name, subPath string) ([]FileEntry, error) {
	var envelope struct {
		Files []FileEntry `json:"files"`
	}
	path := "/servers/" + name + "/files"
	if subPath != "" {
		path += "?path=" + url.QueryEscape(subPath)
	}
	if err := c.base.DoJSON(ctx, "GET", path, nil, &envelope); err != nil {
		return nil, fmt.Errorf("list files for %q: %w", name, err)
	}
	return envelope.Files, nil
}

// ReadFile reads a file from a server directory and returns the content string.
func (c *Client) ReadFile(ctx context.Context, name, filePath string) (string, error) {
	var envelope struct {
		Content string `json:"content"`
	}
	path := "/servers/" + name + "/files/read?path=" + url.QueryEscape(filePath)
	if err := c.base.DoJSON(ctx, "GET", path, nil, &envelope); err != nil {
		return "", fmt.Errorf("read file for %q: %w", name, err)
	}
	return envelope.Content, nil
}

// ListBackups returns backups for a server.
func (c *Client) ListBackups(ctx context.Context, name string) ([]BackupInfo, error) {
	var envelope struct {
		Backups []BackupInfo `json:"backups"`
	}
	if err := c.base.DoJSON(ctx, "GET", "/servers/"+name+"/backups", nil, &envelope); err != nil {
		return nil, fmt.Errorf("list backups for %q: %w", name, err)
	}
	return envelope.Backups, nil
}

// CreateBackup triggers a backup for a server.
func (c *Client) CreateBackup(ctx context.Context, name string) (*BackupStatus, error) {
	var status BackupStatus
	if err := c.base.DoJSON(ctx, "POST", "/servers/"+name+"/backups", nil, &status); err != nil {
		return nil, fmt.Errorf("create backup for %q: %w", name, err)
	}
	return &status, nil
}

// GetBackupStatus returns the status of a specific backup.
func (c *Client) GetBackupStatus(ctx context.Context, name, backupID string) (*BackupStatus, error) {
	var status BackupStatus
	path := fmt.Sprintf("/servers/%s/backups/%s", name, backupID)
	if err := c.base.DoJSON(ctx, "GET", path, nil, &status); err != nil {
		return nil, fmt.Errorf("get backup status %q for %q: %w", backupID, name, err)
	}
	return &status, nil
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
