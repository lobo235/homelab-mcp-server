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
// If a server pack URL is available it is preferred over the client pack.
func (p *Pipeline) Download(ctx context.Context, resolved *ResolvedPack, tempDir string) (string, error) {
	// FTB packs don't have a single download URL — they use the API directly.
	if resolved.Format == "ftb_pack" {
		p.Log.Info("ftb pack uses API-based distribution, skipping download", "slug", resolved.Slug)
		return "", nil
	}

	dlURL := resolved.DownloadURL
	if resolved.ServerPackURL != "" {
		dlURL = resolved.ServerPackURL
		p.Log.Info("using server pack URL", "slug", resolved.Slug)
	}

	if dlURL == "" {
		return "", fmt.Errorf("no download URL available for %q", resolved.Slug)
	}

	// Security: only allow HTTPS downloads.
	if !strings.HasPrefix(dlURL, "https://") {
		return "", fmt.Errorf("only HTTPS download URLs are allowed, got %q", dlURL)
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

	p.Log.Info("downloading modpack", "slug", resolved.Slug, "dest", destPath)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, dlURL, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return "", fmt.Errorf("create file %q: %w", destPath, err)
	}

	// Limit download size to prevent resource exhaustion.
	written, err := io.Copy(out, io.LimitReader(resp.Body, maxDownloadSize))
	if closeErr := out.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		return "", fmt.Errorf("write file %q: %w", destPath, err)
	}
	if written >= maxDownloadSize {
		_ = os.Remove(destPath)
		return "", fmt.Errorf("download exceeded maximum size of %d bytes", maxDownloadSize)
	}

	p.Log.Info("download complete", "slug", resolved.Slug, "bytes", written)

	return destPath, nil
}
