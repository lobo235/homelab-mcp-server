package discovery

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	// maxExtractFileSize is the maximum allowed size for a single extracted file (1 GB).
	maxExtractFileSize = 1 * 1024 * 1024 * 1024
	// maxExtractTotalSize is the maximum total extracted size (5 GB).
	maxExtractTotalSize = 5 * 1024 * 1024 * 1024
)

// Extract decompresses a .zip or .mrpack archive into destDir.
// It normalizes directory structure to ensure files are directly in destDir
// rather than in a nested subdirectory.
func Extract(archivePath, destDir string) error {
	if archivePath == "" {
		return nil // FTB packs don't have a local archive.
	}

	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("open archive %q: %w", archivePath, err)
	}
	defer r.Close() //nolint:errcheck // best-effort close in read-only context

	if err := os.MkdirAll(destDir, 0o700); err != nil {
		return fmt.Errorf("create dest dir: %w", err)
	}

	// Detect common prefix (single top-level directory).
	commonPrefix := detectCommonPrefix(r.File)

	// Pre-check total uncompressed size to detect zip bombs.
	var totalUncompressed uint64
	for _, f := range r.File {
		totalUncompressed += f.UncompressedSize64
		if f.UncompressedSize64 > maxExtractFileSize {
			return fmt.Errorf("file %q too large: %d bytes (max %d)", f.Name, f.UncompressedSize64, maxExtractFileSize)
		}
	}
	if totalUncompressed > maxExtractTotalSize {
		return fmt.Errorf("archive total uncompressed size too large: %d bytes (max %d)", totalUncompressed, maxExtractTotalSize)
	}

	var totalWritten int64
	for _, f := range r.File {
		name := f.Name

		// Security: reject paths with directory traversal.
		if strings.Contains(name, "..") {
			return fmt.Errorf("illegal path with traversal in archive: %q", name)
		}

		// Strip common prefix to normalize structure.
		if commonPrefix != "" {
			name = strings.TrimPrefix(name, commonPrefix)
			if name == "" {
				continue // Skip the prefix directory itself.
			}
		}

		// Security: prevent zip slip.
		target := filepath.Join(destDir, filepath.FromSlash(name))
		if !strings.HasPrefix(target, filepath.Clean(destDir)+string(os.PathSeparator)) && target != filepath.Clean(destDir) {
			return fmt.Errorf("illegal file path in archive: %q", f.Name)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o700); err != nil {
				return fmt.Errorf("create dir %q: %w", target, err)
			}
			continue
		}

		// Ensure parent directory exists.
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return fmt.Errorf("create parent dir for %q: %w", target, err)
		}

		written, err := extractFile(f, target)
		if err != nil {
			return err
		}
		totalWritten += written
		if totalWritten > maxExtractTotalSize {
			return fmt.Errorf("extraction exceeded maximum total size of %d bytes", maxExtractTotalSize)
		}
	}

	return nil
}

// extractFile writes a single zip entry to the target path and returns bytes written.
func extractFile(f *zip.File, target string) (int64, error) {
	rc, err := f.Open()
	if err != nil {
		return 0, fmt.Errorf("open %q in archive: %w", f.Name, err)
	}
	defer rc.Close() //nolint:errcheck // best-effort close in read-only context

	out, err := os.Create(target)
	if err != nil {
		return 0, fmt.Errorf("create %q: %w", target, err)
	}
	defer out.Close() //nolint:errcheck // write error checked via io.Copy

	// Limit per-file extraction size.
	written, err := io.Copy(out, io.LimitReader(rc, maxExtractFileSize))
	if err != nil {
		return 0, fmt.Errorf("extract %q: %w", f.Name, err)
	}

	return written, nil
}

// detectCommonPrefix finds a common directory prefix shared by all files in the archive.
// Returns the prefix with a trailing slash, or empty string if there's no common prefix.
func detectCommonPrefix(files []*zip.File) string {
	if len(files) == 0 {
		return ""
	}

	// Find the first path component of each file.
	prefixes := make(map[string]bool)
	for _, f := range files {
		parts := strings.SplitN(f.Name, "/", 2)
		if len(parts) > 0 {
			prefixes[parts[0]] = true
		}
	}

	// If all files share the same top-level directory, that's the common prefix.
	if len(prefixes) == 1 {
		for prefix := range prefixes {
			// Verify it's actually a directory (has files underneath).
			for _, f := range files {
				if strings.HasPrefix(f.Name, prefix+"/") && f.Name != prefix+"/" {
					return prefix + "/"
				}
			}
		}
	}

	return ""
}
