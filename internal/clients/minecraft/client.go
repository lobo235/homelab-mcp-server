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
// All backup files/dirs are chowned to uid:gid.
func (c *Client) CreateBackup(ctx context.Context, name string, uid, gid int) (*BackupStatus, error) {
	body := map[string]int{"uid": uid, "gid": gid}
	var status BackupStatus
	if err := c.base.DoJSON(ctx, "POST", "/servers/"+name+"/backups", body, &status); err != nil {
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

// DownloadRequest represents a file download request.
type DownloadRequest struct {
	URL      string `json:"url"`
	DestPath string `json:"dest_path"`
	Extract  bool   `json:"extract"`
	Mode     string `json:"mode"`
	UID      int    `json:"uid"`
	GID      int    `json:"gid"`
}

// DownloadStartResult represents the response from starting an async download.
type DownloadStartResult struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// DownloadStatus represents the status of an async download.
type DownloadStatus struct {
	ID          string          `json:"id"`
	Status      string          `json:"status"`
	URL         string          `json:"url"`
	DestPath    string          `json:"dest_path"`
	Extract     bool            `json:"extract"`
	StartedAt   string          `json:"started_at"`
	CompletedAt string          `json:"completed_at,omitempty"`
	Result      *DownloadResult `json:"result,omitempty"`
	Error       string          `json:"error,omitempty"`
}

// DownloadResult represents the outcome of a completed download.
type DownloadResult struct {
	FilesCount int   `json:"files_count"`
	TotalBytes int64 `json:"total_bytes"`
}

// DownloadToServer starts an async file download to a server directory.
// Returns immediately with a download ID for status polling.
func (c *Client) DownloadToServer(ctx context.Context, name string, req DownloadRequest) (*DownloadStartResult, error) {
	var result DownloadStartResult
	if err := c.base.DoJSON(ctx, "POST", "/servers/"+name+"/download", req, &result); err != nil {
		return nil, fmt.Errorf("download to %q: %w", name, err)
	}
	return &result, nil
}

// GetDownloadStatus returns the status of an async download.
func (c *Client) GetDownloadStatus(ctx context.Context, name, downloadID string) (*DownloadStatus, error) {
	var status DownloadStatus
	path := fmt.Sprintf("/servers/%s/downloads/%s", name, downloadID)
	if err := c.base.DoJSON(ctx, "GET", path, nil, &status); err != nil {
		return nil, fmt.Errorf("get download status %q for %q: %w", downloadID, name, err)
	}
	return &status, nil
}

// ArchiveEntry represents a file inside an archive on the server filesystem.
type ArchiveEntry struct {
	Name  string `json:"name"`
	Size  int64  `json:"size"`
	IsDir bool   `json:"is_dir"`
}

// ListArchiveContents lists files inside an archive on the server filesystem.
func (c *Client) ListArchiveContents(ctx context.Context, name, path string) ([]ArchiveEntry, error) {
	var envelope struct {
		Entries []ArchiveEntry `json:"entries"`
	}
	endpoint := "/servers/" + name + "/archive-contents?path=" + url.QueryEscape(path)
	if err := c.base.DoJSON(ctx, "GET", endpoint, nil, &envelope); err != nil {
		return nil, fmt.Errorf("list archive contents for %q: %w", name, err)
	}
	return envelope.Entries, nil
}

// writeFileRequest is the JSON body for PUT /servers/{name}/files/write.
type writeFileRequest struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	UID     int    `json:"uid"`
	GID     int    `json:"gid"`
}

// WriteFile writes content to a file on the server filesystem.
func (c *Client) WriteFile(ctx context.Context, name, path, content string, uid, gid int) error {
	body := writeFileRequest{Path: path, Content: content, UID: uid, GID: gid}
	if err := c.base.DoNoContent(ctx, "PUT", "/servers/"+name+"/files/write", body); err != nil {
		return fmt.Errorf("write file for %q: %w", name, err)
	}
	return nil
}

// renameServerRequest is the JSON body for POST /servers/{name}/rename.
type renameServerRequest struct {
	NewName string `json:"new_name"`
}

// RenameServer renames a server directory on the NFS filesystem.
func (c *Client) RenameServer(ctx context.Context, oldName, newName string) error {
	body := renameServerRequest{NewName: newName}
	if err := c.base.DoNoContent(ctx, "POST", "/servers/"+oldName+"/rename", body); err != nil {
		return fmt.Errorf("rename server %q to %q: %w", oldName, newName, err)
	}
	return nil
}

// MoveFile moves/renames a file or directory within a server's filesystem.
// Parent directories are chowned to uid:gid.
func (c *Client) MoveFile(ctx context.Context, name, srcPath, dstPath string, uid, gid int) error {
	body := map[string]any{"src_path": srcPath, "dst_path": dstPath, "uid": uid, "gid": gid}
	if err := c.base.DoNoContent(ctx, "POST", "/servers/"+name+"/files/move", body); err != nil {
		return fmt.Errorf("move file for %q: %w", name, err)
	}
	return nil
}

// DeleteFile removes a file or directory from a server's filesystem.
func (c *Client) DeleteFile(ctx context.Context, name, path string) error {
	if err := c.base.DoNoContent(ctx, "DELETE", "/servers/"+name+"/files/delete?path="+url.QueryEscape(path), nil); err != nil {
		return fmt.Errorf("delete file for %q: %w", name, err)
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
