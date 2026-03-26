package discovery

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const (
	// maxDownloadSize is the maximum allowed download size (2 GB).
	maxDownloadSize = 2 * 1024 * 1024 * 1024
)

// Download fetches the modpack archive to disk and returns the path to the downloaded file.
// Prefers the server pack (contains actual mod jars, configs, startup scripts) but falls
// back to the client pack if the server pack download fails or doesn't exist.
func (p *Pipeline) Download(ctx context.Context, resolved *ResolvedPack, tempDir string) (string, error) {
	// FTB packs don't have a single download URL — they use the API directly.
	if resolved.Format == "ftb_pack" {
		p.Log.Info("ftb pack uses API-based distribution, skipping download", "slug", resolved.Slug)
		return "", nil
	}

	// Create slug-specific temp directory with restrictive permissions.
	destDir := filepath.Join(tempDir, resolved.Slug)
	if err := os.MkdirAll(destDir, 0o700); err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}

	// Determine filename from URL or format.
	filename := "modpack.zip"
	if resolved.Format == "modrinth_mrpack" {
		filename = "modpack.mrpack"
	}
	destPath := filepath.Join(destDir, filename)

	// Try server pack first — it has actual mod jars, startup scripts, server.properties.
	if resolved.ServerPackURL != "" {
		p.Log.Info("trying server pack URL", "slug", resolved.Slug)
		if err := p.downloadFile(ctx, resolved.ServerPackURL, destPath); err != nil {
			p.Log.Warn("server pack download failed, falling back to client pack", "slug", resolved.Slug, "error", err)
		} else {
			return destPath, nil
		}
	}

	// Fall back to client pack.
	if resolved.DownloadURL == "" {
		return "", fmt.Errorf("no download URL available for %q", resolved.Slug)
	}

	p.Log.Info("downloading client pack", "slug", resolved.Slug)
	if err := p.downloadFile(ctx, resolved.DownloadURL, destPath); err != nil {
		return "", err
	}
	return destPath, nil
}

// downloadFile fetches a single URL to destPath with size limits and HTTPS enforcement.
func (p *Pipeline) downloadFile(ctx context.Context, dlURL, destPath string) error {
	// Security: only allow HTTPS downloads.
	if !strings.HasPrefix(dlURL, "https://") {
		return fmt.Errorf("only HTTPS download URLs are allowed, got %q", dlURL)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, dlURL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create file %q: %w", destPath, err)
	}

	// Limit download size to prevent resource exhaustion.
	written, err := io.Copy(out, io.LimitReader(resp.Body, maxDownloadSize))
	if closeErr := out.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("write file %q: %w", destPath, err)
	}
	if written >= maxDownloadSize {
		_ = os.Remove(destPath)
		return fmt.Errorf("download exceeded maximum size of %d bytes", maxDownloadSize)
	}

	p.Log.Info("download complete", "dest", destPath, "bytes", written)
	return nil
}
