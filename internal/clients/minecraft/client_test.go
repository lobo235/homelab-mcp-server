package minecraft

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lobo235/homelab-mcp-server/internal/clients"
)

func TestClient_ListServers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/servers" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
		}
		json.NewEncoder(w).Encode(map[string][]string{
			"servers": {"atm10", "vanilla1"},
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	result, err := client.ListServers(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("len = %d, want 2", len(result))
	}
	if result[0] != "atm10" {
		t.Errorf("result[0] = %q, want %q", result[0], "atm10")
	}
}

func TestClient_ListServers_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"code":    "internal_error",
			"message": "upstream failure",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	_, err := client.ListServers(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestClient_InitServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/servers" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		var req initServerRequest
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("unmarshal body: %v", err)
		}
		if req.Name != "atm10" {
			t.Errorf("name = %q, want %q", req.Name, "atm10")
		}
		if req.UID != 1000 {
			t.Errorf("uid = %d, want %d", req.UID, 1000)
		}
		if req.GID != 1000 {
			t.Errorf("gid = %d, want %d", req.GID, 1000)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	err := client.InitServer(context.Background(), "atm10", 1000, 1000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClient_InitServer_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]string{
			"code":    "conflict",
			"message": "server already exists",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	err := client.InitServer(context.Background(), "atm10", 1000, 1000)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestClient_DeleteServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/servers/atm10" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("confirm") != "true" {
			t.Error("expected confirm=true query param")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	err := client.DeleteServer(context.Background(), "atm10")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClient_DeleteServer_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{
			"code":    "not_found",
			"message": "server not found",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	err := client.DeleteServer(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestClient_ListFiles(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/servers/atm10/files" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string][]FileEntry{
			"files": {
				{Name: "server.properties", Size: 1024, IsDir: false, ModTime: "2024-01-01T00:00:00Z"},
				{Name: "mods", Size: 0, IsDir: true, ModTime: "2024-01-01T00:00:00Z"},
			},
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	result, err := client.ListFiles(context.Background(), "atm10", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("len = %d, want 2", len(result))
	}
	if result[0].Name != "server.properties" {
		t.Errorf("result[0].Name = %q, want %q", result[0].Name, "server.properties")
	}
}

func TestClient_ListFiles_WithSubPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/servers/atm10/files" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		pathParam := r.URL.Query().Get("path")
		if pathParam != "config/mods" {
			t.Errorf("path param = %q, want %q", pathParam, "config/mods")
		}
		json.NewEncoder(w).Encode(map[string][]FileEntry{
			"files": {{Name: "mod.toml", Size: 256, IsDir: false}},
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	result, err := client.ListFiles(context.Background(), "atm10", "config/mods")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("len = %d, want 1", len(result))
	}
}

func TestClient_ListFiles_URLEncoding(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pathParam := r.URL.Query().Get("path")
		if pathParam != "config/some dir/file name.txt" {
			t.Errorf("path param = %q, want %q", pathParam, "config/some dir/file name.txt")
		}
		json.NewEncoder(w).Encode(map[string][]FileEntry{
			"files": {},
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	_, err := client.ListFiles(context.Background(), "atm10", "config/some dir/file name.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClient_ListFiles_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{
			"code":    "not_found",
			"message": "server not found",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	_, err := client.ListFiles(context.Background(), "nonexistent", "")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestClient_ReadFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/servers/atm10/files/read" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		pathParam := r.URL.Query().Get("path")
		if pathParam != "server.properties" {
			t.Errorf("path param = %q, want %q", pathParam, "server.properties")
		}
		json.NewEncoder(w).Encode(map[string]string{
			"content": "motd=Hello World\nmax-players=20\n",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	content, err := client.ReadFile(context.Background(), "atm10", "server.properties")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if content != "motd=Hello World\nmax-players=20\n" {
		t.Errorf("content = %q, unexpected", content)
	}
}

func TestClient_ReadFile_URLEncoding(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pathParam := r.URL.Query().Get("path")
		if pathParam != "config/some dir/config file.toml" {
			t.Errorf("path param = %q, want %q", pathParam, "config/some dir/config file.toml")
		}
		json.NewEncoder(w).Encode(map[string]string{
			"content": "[settings]\nkey=value",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	content, err := client.ReadFile(context.Background(), "atm10", "config/some dir/config file.toml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if content != "[settings]\nkey=value" {
		t.Errorf("content = %q, unexpected", content)
	}
}

func TestClient_ReadFile_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{
			"code":    "not_found",
			"message": "file not found",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	_, err := client.ReadFile(context.Background(), "atm10", "nonexistent.txt")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestClient_ListBackups(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/servers/atm10/backups" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string][]BackupInfo{
			"backups": {
				{ID: "backup-1", Path: "/backups/atm10/backup-1.tar.gz", Size: 1024000, Created: "2024-01-01T00:00:00Z"},
			},
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	result, err := client.ListBackups(context.Background(), "atm10")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("len = %d, want 1", len(result))
	}
	if result[0].ID != "backup-1" {
		t.Errorf("ID = %q, want %q", result[0].ID, "backup-1")
	}
}

func TestClient_ListBackups_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{
			"code":    "not_found",
			"message": "server not found",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	_, err := client.ListBackups(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestClient_CreateBackup(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/servers/atm10/backups" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(BackupStatus{
			Server: "atm10",
			ID:     "backup-new",
			Status: "in_progress",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	status, err := client.CreateBackup(context.Background(), "atm10")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.ID != "backup-new" {
		t.Errorf("ID = %q, want %q", status.ID, "backup-new")
	}
	if status.Status != "in_progress" {
		t.Errorf("Status = %q, want %q", status.Status, "in_progress")
	}
}

func TestClient_CreateBackup_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"code":    "internal_error",
			"message": "backup failed",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	_, err := client.CreateBackup(context.Background(), "atm10")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestClient_GetBackupStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/servers/atm10/backups/backup-1" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(BackupStatus{
			Server:     "atm10",
			BackupID:   "backup-1",
			Status:     "completed",
			BackupPath: "/backups/atm10/backup-1.tar.gz",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	status, err := client.GetBackupStatus(context.Background(), "atm10", "backup-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.Status != "completed" {
		t.Errorf("Status = %q, want %q", status.Status, "completed")
	}
}

func TestClient_GetBackupStatus_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{
			"code":    "not_found",
			"message": "backup not found",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	_, err := client.GetBackupStatus(context.Background(), "atm10", "nonexistent")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestClient_Restore(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/servers/atm10/restore" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		var req map[string]string
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("unmarshal body: %v", err)
		}
		if req["backup_id"] != "backup-1" {
			t.Errorf("backup_id = %q, want %q", req["backup_id"], "backup-1")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	err := client.Restore(context.Background(), "atm10", map[string]string{"backup_id": "backup-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClient_Restore_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{
			"code":    "not_found",
			"message": "server not found",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	err := client.Restore(context.Background(), "nonexistent", map[string]string{"backup_id": "backup-1"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestClient_DownloadToServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/servers/atm10/download" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		var req DownloadRequest
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("unmarshal body: %v", err)
		}
		if req.URL != "https://example.com/serverpack.zip" {
			t.Errorf("url = %q, want %q", req.URL, "https://example.com/serverpack.zip")
		}
		if req.DestPath != "." {
			t.Errorf("dest_path = %q, want %q", req.DestPath, ".")
		}
		if !req.Extract {
			t.Error("expected extract=true")
		}
		if req.Mode != "overwrite" {
			t.Errorf("mode = %q, want %q", req.Mode, "overwrite")
		}
		if req.UID != 1001 {
			t.Errorf("uid = %d, want %d", req.UID, 1001)
		}
		if req.GID != 1001 {
			t.Errorf("gid = %d, want %d", req.GID, 1001)
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(DownloadStartResult{
			ID:     "2026-03-24T12-00-00",
			Status: "running",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	result, err := client.DownloadToServer(context.Background(), "atm10", DownloadRequest{
		URL:      "https://example.com/serverpack.zip",
		DestPath: ".",
		Extract:  true,
		Mode:     "overwrite",
		UID:      1001,
		GID:      1001,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != "running" {
		t.Errorf("status = %q, want %q", result.Status, "running")
	}
	if result.ID != "2026-03-24T12-00-00" {
		t.Errorf("id = %q, want %q", result.ID, "2026-03-24T12-00-00")
	}
}

func TestClient_DownloadToServer_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"code":    "bad_request",
			"message": "invalid URL",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	_, err := client.DownloadToServer(context.Background(), "atm10", DownloadRequest{
		URL: "invalid",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestClient_ListArchiveContents(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/servers/atm10/archive-contents" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		pathParam := r.URL.Query().Get("path")
		if pathParam != "serverpack.zip" {
			t.Errorf("path param = %q, want %q", pathParam, "serverpack.zip")
		}
		json.NewEncoder(w).Encode(map[string][]ArchiveEntry{
			"entries": {
				{Name: "server.properties", Size: 1024, IsDir: false},
				{Name: "mods/", Size: 0, IsDir: true},
				{Name: "mods/jei.jar", Size: 524288, IsDir: false},
			},
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	entries, err := client.ListArchiveContents(context.Background(), "atm10", "serverpack.zip")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("len = %d, want 3", len(entries))
	}
	if entries[0].Name != "server.properties" {
		t.Errorf("entries[0].Name = %q, want %q", entries[0].Name, "server.properties")
	}
	if entries[1].IsDir != true {
		t.Error("expected entries[1].IsDir = true")
	}
}

func TestClient_ListArchiveContents_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{
			"code":    "not_found",
			"message": "archive not found",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	_, err := client.ListArchiveContents(context.Background(), "atm10", "nonexistent.zip")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestClient_WriteFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PUT" {
			t.Errorf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/servers/atm10/files/write" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		var req writeFileRequest
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("unmarshal body: %v", err)
		}
		if req.Path != "server.properties" {
			t.Errorf("path = %q, want %q", req.Path, "server.properties")
		}
		if req.Content != "motd=Test Server\nmax-players=10\n" {
			t.Errorf("content = %q, unexpected", req.Content)
		}
		if req.UID != 1001 {
			t.Errorf("uid = %d, want %d", req.UID, 1001)
		}
		if req.GID != 1001 {
			t.Errorf("gid = %d, want %d", req.GID, 1001)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	err := client.WriteFile(context.Background(), "atm10", "server.properties", "motd=Test Server\nmax-players=10\n", 1001, 1001)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClient_WriteFile_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"code":    "internal_error",
			"message": "write failed",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	err := client.WriteFile(context.Background(), "atm10", "server.properties", "content", 1001, 1001)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestClient_ExecuteRCON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/servers/mc-atm10/rcon" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		var req RCONRequest
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("unmarshal body: %v", err)
		}
		if req.Command != "list" {
			t.Errorf("command = %q, want %q", req.Command, "list")
		}
		json.NewEncoder(w).Encode(RCONResponse{
			Response: "There are 3 of a max of 20 players online: Steve, Alex, Notch",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	resp, err := client.ExecuteRCON(context.Background(), "mc-atm10", "list")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "There are 3 of a max of 20 players online: Steve, Alex, Notch" {
		t.Errorf("response = %q, unexpected", resp)
	}
}

func TestClient_ExecuteRCON_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]string{
			"code":    "upstream_error",
			"message": "RCON connection refused",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	_, err := client.ExecuteRCON(context.Background(), "mc-atm10", "list")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestClient_Ping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	err := client.Ping(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewClientWithBase(t *testing.T) {
	base := clients.NewBase("http://localhost:8080", "key")
	client := NewClientWithBase(base)
	if client == nil {
		t.Fatal("expected non-nil client")
	}
}
