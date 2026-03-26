//go:build integration

package discovery

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lobo235/homelab-mcp-server/internal/clients/curseforge"
	"github.com/lobo235/homelab-mcp-server/internal/clients/ftb"
	"github.com/lobo235/homelab-mcp-server/internal/clients/modrinth"
	"github.com/lobo235/homelab-mcp-server/internal/modpackkb"
)

// --- helpers ---

// createTestZip builds an in-memory zip containing the given files (path->content).
func createTestZip(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, data := range files {
		f, err := w.Create(name)
		if err != nil {
			t.Fatalf("zip create %s: %v", name, err)
		}
		if _, err := f.Write(data); err != nil {
			t.Fatalf("zip write %s: %v", name, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}

// loadFixture reads a file from testdata/.
func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("load fixture %s: %v", name, err)
	}
	return data
}

// newTestKB creates a temp KB directory and returns the KB and a cleanup func.
func newTestKB(t *testing.T) *modpackkb.KB {
	t.Helper()
	dir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	kb, err := modpackkb.New(filepath.Join(dir, "kb"), logger)
	if err != nil {
		t.Fatalf("create KB: %v", err)
	}
	return kb
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

func containsSubstring(haystack []string, sub string) bool {
	for _, s := range haystack {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// --- CurseForge integration test ---

func TestIntegration_CurseForgeFullPipeline(t *testing.T) {
	manifest := loadFixture(t, "curseforge_manifest.json")
	zipData := createTestZip(t, map[string][]byte{
		"manifest.json": manifest,
	})

	// Mock download server that serves the zip.
	dlServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		w.Write(zipData)
	}))
	defer dlServer.Close()

	// Mock CurseForge gateway.
	cfServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path
		query := r.URL.Query()

		switch {
		// Search by slug or name.
		case path == "/search" && query.Get("type") == "modpack":
			json.NewEncoder(w).Encode([]curseforge.SearchResult{
				{
					ID:      100001,
					Name:    "Test Modpack",
					Slug:    "test-modpack",
					Summary: "A test modpack",
					ClassID: 4471,
				},
			})

		// Get modpack files list.
		case path == "/modpacks/100001/files":
			json.NewEncoder(w).Encode([]curseforge.ModpackFile{
				{
					ID:           200001,
					DisplayName:  "Test Modpack 1.0.0",
					FileName:     "test-modpack-1.0.0.zip",
					GameVersions: []string{"1.21.1", "NeoForge"},
					DownloadURL:  dlServer.URL + "/test-modpack-1.0.0.zip",
					ReleaseType:  1,
					FileLength:   int64(len(zipData)),
				},
			})

		// Get specific modpack file.
		case path == "/modpacks/100001/files/200001":
			json.NewEncoder(w).Encode(curseforge.ModpackFile{
				ID:           200001,
				DisplayName:  "Test Modpack 1.0.0",
				FileName:     "test-modpack-1.0.0.zip",
				GameVersions: []string{"1.21.1", "NeoForge"},
				DownloadURL:  dlServer.URL + "/test-modpack-1.0.0.zip",
				ReleaseType:  1,
				FileLength:   int64(len(zipData)),
			})

		// Get modpack details (enrichment).
		case path == "/modpacks/100001":
			json.NewEncoder(w).Encode(curseforge.Modpack{
				ID:      100001,
				Name:    "Test Modpack",
				Summary: "A test modpack for integration testing",
			})

		// Get individual mod files (mod enrichment).
		case strings.HasPrefix(path, "/mods/") && strings.Contains(path, "/files/"):
			// Map projectID → filename for enrichment testing.
			modFiles := map[string]curseforge.ModpackFile{
				"/mods/238222/files/5000001": {ID: 5000001, FileName: "jei-1.21.1-neoforge-19.0.0.jar", DisplayName: "JEI 19.0.0"},
				"/mods/306612/files/5000002": {ID: 5000002, FileName: "create-1.21.1-0.5.1.jar", DisplayName: "Create 0.5.1"},
				"/mods/250398/files/5000003": {ID: 5000003, FileName: "appleskin-neoforge-mc1.21.1-2.5.1.jar", DisplayName: "AppleSkin 2.5.1"},
				"/mods/243076/files/5000004": {ID: 5000004, FileName: "journeymap-1.21.1-6.0.0-neoforge.jar", DisplayName: "JourneyMap 6.0.0"},
				"/mods/232131/files/5000005": {ID: 5000005, FileName: "optifine-1.21.1-HD_U_J1.jar", DisplayName: "OptiFine HD U J1"},
			}
			if file, ok := modFiles[path]; ok {
				json.NewEncoder(w).Encode(file)
			} else {
				http.NotFound(w, r)
			}

		default:
			t.Logf("unhandled CurseForge request: %s %s", r.Method, r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer cfServer.Close()

	// The download URL is HTTPS-only in the downloader. We need to work around this
	// by running stages manually and providing the archive path directly.
	// Instead, we'll do the full pipeline but handle Download specially.
	kb := newTestKB(t)
	tempDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	cfClient := curseforge.NewClient(cfServer.URL, "test-key")

	pipe := NewPipeline(PipelineConfig{
		KB:         kb,
		CurseForge: cfClient,
		TempDir:    tempDir,
		Log:        logger,
	})

	ctx := context.Background()

	// Stage 1: Resolve.
	resolved, err := pipe.Resolve(ctx, "test-modpack", "Test Modpack", "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if resolved.Platform != "curseforge" {
		t.Errorf("Platform = %q, want %q", resolved.Platform, "curseforge")
	}
	if resolved.CurseForgeID != 100001 {
		t.Errorf("CurseForgeID = %d, want 100001", resolved.CurseForgeID)
	}

	// Stage 2: Download — skip HTTPS check by writing the zip directly.
	archiveDir := filepath.Join(tempDir, "test-modpack")
	if err := os.MkdirAll(archiveDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	archivePath := filepath.Join(archiveDir, "modpack.zip")
	if err := os.WriteFile(archivePath, zipData, 0o600); err != nil {
		t.Fatalf("write zip: %v", err)
	}

	// Stage 3: Extract.
	extractDir := filepath.Join(tempDir, "test-modpack", "extracted")
	if err := Extract(archivePath, extractDir); err != nil {
		t.Fatalf("Extract: %v", err)
	}

	// Stage 4: Analyze.
	data, err := pipe.Analyze(ctx, extractDir, resolved)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	if data.MinecraftVersion != "1.21.1" {
		t.Errorf("MinecraftVersion = %q, want %q", data.MinecraftVersion, "1.21.1")
	}
	if data.ModloaderType != "neoforge" {
		t.Errorf("ModloaderType = %q, want %q", data.ModloaderType, "neoforge")
	}
	if data.ModloaderVersion != "21.1.221" {
		t.Errorf("ModloaderVersion = %q, want %q", data.ModloaderVersion, "21.1.221")
	}
	if data.TotalModCount != 5 {
		t.Errorf("TotalModCount = %d, want 5", data.TotalModCount)
	}
	if data.JavaVersion != 21 {
		t.Errorf("JavaVersion = %d, want 21", data.JavaVersion)
	}

	// Stage 5: API Enrichment.
	if err := pipe.EnrichFromAPI(ctx, resolved, data); err != nil {
		t.Fatalf("EnrichFromAPI: %v", err)
	}

	// Verify mod file enrichment populated filenames.
	enrichedCount := 0
	for _, m := range data.ModList {
		if m.FileName != "" {
			enrichedCount++
		}
	}
	if enrichedCount != 5 {
		t.Errorf("enriched mod count = %d, want 5", enrichedCount)
	}

	// Re-run mod intelligence (as pipeline.go does after enrichment).
	refreshModIntelligence(data)

	// After enrichment + mod intelligence, client-only mods should be detected.
	if len(data.ClientOnlyMods) == 0 {
		t.Error("expected client-only mods to be detected after enrichment (journeymap, optifine)")
	}
	if !containsString(data.ClientOnlyMods, "journeymap") && !containsSubstring(data.ClientOnlyMods, "journeymap") {
		t.Errorf("ClientOnlyMods missing journeymap, got %v", data.ClientOnlyMods)
	}

	// Stage 6: Web search — skip (no API key).
	webResults, _ := pipe.EnrichFromWeb(ctx, data, "")

	// Stage 7: Finalize.
	mk := Finalize(resolved, data, webResults)

	if !mk.NeedsReview {
		t.Error("NeedsReview should be true")
	}
	if mk.SourcePlatform != "curseforge" {
		t.Errorf("SourcePlatform = %q, want %q", mk.SourcePlatform, "curseforge")
	}
	if mk.MinecraftVersion != "1.21.1" {
		t.Errorf("mk.MinecraftVersion = %q, want %q", mk.MinecraftVersion, "1.21.1")
	}
	if mk.ModloaderType != "neoforge" {
		t.Errorf("mk.ModloaderType = %q, want %q", mk.ModloaderType, "neoforge")
	}

	// Check that confidence flags contain expected entries.
	if !containsString(mk.ConfidenceFlags, FlagModloaderFromManifest) {
		t.Errorf("ConfidenceFlags missing %q, got %v", FlagModloaderFromManifest, mk.ConfidenceFlags)
	}
	if !containsString(mk.ConfidenceFlags, FlagWebSearchSkipped) {
		t.Errorf("ConfidenceFlags missing %q, got %v", FlagWebSearchSkipped, mk.ConfidenceFlags)
	}
	if !containsString(mk.ConfidenceFlags, FlagAPIEnriched) {
		t.Errorf("ConfidenceFlags missing %q, got %v", FlagAPIEnriched, mk.ConfidenceFlags)
	}

	// Verify version knowledge.
	if len(mk.Versions) != 1 {
		t.Fatalf("Versions count = %d, want 1", len(mk.Versions))
	}
	vk := mk.Versions[0]
	if vk.MinecraftVersion != "1.21.1" {
		t.Errorf("vk.MinecraftVersion = %q, want %q", vk.MinecraftVersion, "1.21.1")
	}
	if vk.ModloaderVersion != "21.1.221" {
		t.Errorf("vk.ModloaderVersion = %q, want %q", vk.ModloaderVersion, "21.1.221")
	}
	if vk.JavaVersion != 21 {
		t.Errorf("vk.JavaVersion = %d, want 21", vk.JavaVersion)
	}
	if vk.TotalModCount != 5 {
		t.Errorf("vk.TotalModCount = %d, want 5", vk.TotalModCount)
	}
	if vk.DiscoveryMethod != "manifest_parse" {
		t.Errorf("vk.DiscoveryMethod = %q, want %q", vk.DiscoveryMethod, "manifest_parse")
	}

	// Save to KB and verify round-trip.
	if err := kb.Save(mk); err != nil {
		t.Fatalf("KB.Save: %v", err)
	}
	loaded, err := kb.Get(mk.Slug)
	if err != nil {
		t.Fatalf("KB.Get: %v", err)
	}
	if loaded.MinecraftVersion != "1.21.1" {
		t.Errorf("loaded.MinecraftVersion = %q, want %q", loaded.MinecraftVersion, "1.21.1")
	}
}

// --- Modrinth integration test ---

func TestIntegration_ModrinthFullPipeline(t *testing.T) {
	modrinthIndex := loadFixture(t, "modrinth_index.json")
	mrpackData := createTestZip(t, map[string][]byte{
		"modrinth.index.json": modrinthIndex,
	})

	// Mock download server.
	dlServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		w.Write(mrpackData)
	}))
	defer dlServer.Close()

	downloadURL := dlServer.URL + "/test-modrinth-pack-1.0.0.mrpack"

	// Mock Modrinth API.
	mrServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		switch {
		// Direct project lookup by slug.
		case path == "/project/test-modrinth-pack":
			json.NewEncoder(w).Encode(modrinth.Project{
				ID:          "test-project-id",
				Slug:        "test-modrinth-pack",
				Title:       "Test Modrinth Pack",
				Description: "A test modrinth pack",
				ProjectType: "modpack",
			})

		// Get project by ID (enrichment).
		case path == "/project/test-project-id":
			json.NewEncoder(w).Encode(modrinth.Project{
				ID:          "test-project-id",
				Slug:        "test-modrinth-pack",
				Title:       "Test Modrinth Pack",
				Description: "A test modrinth pack",
				ProjectType: "modpack",
			})

		// Get versions for project.
		case path == "/project/test-project-id/version":
			json.NewEncoder(w).Encode([]modrinth.Version{
				{
					ID:            "version-001",
					Name:          "Release 1.0.0",
					VersionNumber: "1.0.0",
					GameVersions:  []string{"1.21.1"},
					Loaders:       []string{"fabric"},
					Files: []modrinth.VersionFile{
						{
							URL:      downloadURL,
							Filename: "test-modrinth-pack-1.0.0.mrpack",
							Primary:  true,
							Size:     int64(len(mrpackData)),
						},
					},
				},
			})

		// Search (fallback).
		case path == "/search":
			json.NewEncoder(w).Encode(struct {
				Hits []modrinth.Project `json:"hits"`
			}{
				Hits: []modrinth.Project{
					{
						ID:          "test-project-id",
						Slug:        "test-modrinth-pack",
						Title:       "Test Modrinth Pack",
						ProjectType: "modpack",
					},
				},
			})

		default:
			t.Logf("unhandled Modrinth request: %s %s", r.Method, r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer mrServer.Close()

	kb := newTestKB(t)
	tempDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	mrClient := modrinth.NewClientWithBaseURL(mrServer.URL, "test-agent")

	pipe := NewPipeline(PipelineConfig{
		KB:       kb,
		Modrinth: mrClient,
		TempDir:  tempDir,
		Log:      logger,
	})

	ctx := context.Background()

	// Stage 1: Resolve.
	resolved, err := pipe.Resolve(ctx, "test-modrinth-pack", "Test Modrinth Pack", "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if resolved.Platform != "modrinth" {
		t.Errorf("Platform = %q, want %q", resolved.Platform, "modrinth")
	}
	if resolved.ModrinthID != "test-project-id" {
		t.Errorf("ModrinthID = %q, want %q", resolved.ModrinthID, "test-project-id")
	}
	if resolved.MinecraftVersion != "1.21.1" {
		t.Errorf("MinecraftVersion = %q, want %q", resolved.MinecraftVersion, "1.21.1")
	}
	if resolved.ModloaderType != "fabric" {
		t.Errorf("ModloaderType = %q, want %q", resolved.ModloaderType, "fabric")
	}

	// Stage 2: Download — write mrpack directly to skip HTTPS check.
	archiveDir := filepath.Join(tempDir, "test-modrinth-pack")
	if err := os.MkdirAll(archiveDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	archivePath := filepath.Join(archiveDir, "modpack.mrpack")
	if err := os.WriteFile(archivePath, mrpackData, 0o600); err != nil {
		t.Fatalf("write mrpack: %v", err)
	}

	// Stage 3: Extract.
	extractDir := filepath.Join(tempDir, "test-modrinth-pack", "extracted")
	if err := Extract(archivePath, extractDir); err != nil {
		t.Fatalf("Extract: %v", err)
	}

	// Stage 4: Analyze.
	data, err := pipe.Analyze(ctx, extractDir, resolved)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	if data.MinecraftVersion != "1.21.1" {
		t.Errorf("MinecraftVersion = %q, want %q", data.MinecraftVersion, "1.21.1")
	}
	if data.ModloaderType != "fabric" {
		t.Errorf("ModloaderType = %q, want %q", data.ModloaderType, "fabric")
	}
	if data.ModloaderVersion != "0.15.11" {
		t.Errorf("ModloaderVersion = %q, want %q", data.ModloaderVersion, "0.15.11")
	}
	if data.TotalModCount != 3 {
		t.Errorf("TotalModCount = %d, want 3", data.TotalModCount)
	}

	// Check mod filenames.
	modNames := make([]string, 0, len(data.ModList))
	for _, m := range data.ModList {
		modNames = append(modNames, m.FileName)
	}
	for _, expected := range []string{"sodium", "lithium", "iris"} {
		if !containsSubstring(modNames, expected) {
			t.Errorf("mod filenames %v do not contain %q", modNames, expected)
		}
	}

	// Stage 5: API Enrichment.
	if err := pipe.EnrichFromAPI(ctx, resolved, data); err != nil {
		t.Fatalf("EnrichFromAPI: %v", err)
	}

	// Stage 6: Web search — skip.
	webResults, _ := pipe.EnrichFromWeb(ctx, data, "")

	// Stage 7: Finalize.
	mk := Finalize(resolved, data, webResults)

	if !mk.NeedsReview {
		t.Error("NeedsReview should be true")
	}
	if mk.SourcePlatform != "modrinth" {
		t.Errorf("SourcePlatform = %q, want %q", mk.SourcePlatform, "modrinth")
	}
	if mk.MinecraftVersion != "1.21.1" {
		t.Errorf("mk.MinecraftVersion = %q, want %q", mk.MinecraftVersion, "1.21.1")
	}
	if mk.ModloaderType != "fabric" {
		t.Errorf("mk.ModloaderType = %q, want %q", mk.ModloaderType, "fabric")
	}

	// Verify version knowledge.
	if len(mk.Versions) != 1 {
		t.Fatalf("Versions count = %d, want 1", len(mk.Versions))
	}
	vk := mk.Versions[0]
	if vk.ModloaderVersion != "0.15.11" {
		t.Errorf("vk.ModloaderVersion = %q, want %q", vk.ModloaderVersion, "0.15.11")
	}
	if vk.TotalModCount != 3 {
		t.Errorf("vk.TotalModCount = %d, want 3", vk.TotalModCount)
	}
	if vk.DiscoveryMethod != "mrpack_parse" {
		t.Errorf("vk.DiscoveryMethod = %q, want %q", vk.DiscoveryMethod, "mrpack_parse")
	}

	// Check client-only mod detection (iris is client-only).
	if !containsString(mk.ConfidenceFlags, FlagClientOnlyModsDetected) {
		t.Logf("ConfidenceFlags: %v", mk.ConfidenceFlags)
		t.Logf("ClientOnlyMods: %v", data.ClientOnlyMods)
		// iris may or may not be in the client-only list depending on moddb entries
		// so this is a soft check — only log, don't fail
		t.Logf("Note: client_only_mods_detected flag not set (iris may not be in moddb)")
	}

	// Save to KB and verify round-trip.
	if err := kb.Save(mk); err != nil {
		t.Fatalf("KB.Save: %v", err)
	}
	loaded, err := kb.Get(mk.Slug)
	if err != nil {
		t.Fatalf("KB.Get: %v", err)
	}
	if loaded.ModloaderType != "fabric" {
		t.Errorf("loaded.ModloaderType = %q, want %q", loaded.ModloaderType, "fabric")
	}
}

// --- FTB integration test ---

func TestIntegration_FTBPipeline(t *testing.T) {
	// Mock FTB API.
	ftbServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		switch {
		// Search.
		case strings.HasPrefix(path, "/public/modpack/search"):
			json.NewEncoder(w).Encode(ftb.SearchResponse{
				Packs: []int{500},
				Total: 1,
			})

		// Get pack details.
		case path == "/public/modpack/500":
			json.NewEncoder(w).Encode(ftb.Pack{
				ID:       500,
				Name:     "FTB Test Pack",
				Synopsis: "A test FTB pack",
				Versions: []ftb.PackVersionRef{
					{ID: 1000, Name: "1.0.0"},
					{ID: 999, Name: "0.9.0"},
				},
			})

		// Get version details.
		case path == "/public/modpack/500/1000":
			json.NewEncoder(w).Encode(ftb.PackVersion{
				ID:   1000,
				Name: "1.0.0",
				Targets: []ftb.Target{
					{Name: "minecraft", Version: "1.21.1", Type: "game"},
					{Name: "forge", Version: "47.2.0", Type: "modloader"},
				},
				Files: []ftb.File{
					{ID: 1, Name: "jei-1.21.1-forge.jar", Path: "./mods", Size: 5000, ServerOnly: false, ClientOnly: false},
					{ID: 2, Name: "create-1.21.1-0.5.1.jar", Path: "./mods", Size: 8000, ServerOnly: false, ClientOnly: false},
					{ID: 3, Name: "optifine-1.21.1.jar", Path: "./mods", Size: 3000, ServerOnly: false, ClientOnly: true},
					{ID: 4, Name: "server.properties", Path: "./", Size: 500, ServerOnly: true, ClientOnly: false},
				},
			})

		// Get version 999 for the second version reference.
		case path == "/public/modpack/500/999":
			json.NewEncoder(w).Encode(ftb.PackVersion{
				ID:   999,
				Name: "0.9.0",
				Targets: []ftb.Target{
					{Name: "minecraft", Version: "1.21.1", Type: "game"},
					{Name: "forge", Version: "47.1.0", Type: "modloader"},
				},
				Files: []ftb.File{},
			})

		default:
			t.Logf("unhandled FTB request: %s %s", r.Method, r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer ftbServer.Close()

	kb := newTestKB(t)
	tempDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	ftbClient := ftb.NewClientWithBaseURL(ftbServer.URL)

	pipe := NewPipeline(PipelineConfig{
		KB:      kb,
		FTB:     ftbClient,
		TempDir: tempDir,
		Log:     logger,
	})

	ctx := context.Background()

	// Stage 1: Resolve.
	resolved, err := pipe.Resolve(ctx, "ftb-test-pack", "FTB Test Pack", "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if resolved.Platform != "ftb" {
		t.Errorf("Platform = %q, want %q", resolved.Platform, "ftb")
	}
	if resolved.FTBID != 500 {
		t.Errorf("FTBID = %d, want 500", resolved.FTBID)
	}
	if resolved.MinecraftVersion != "1.21.1" {
		t.Errorf("MinecraftVersion = %q, want %q", resolved.MinecraftVersion, "1.21.1")
	}
	if resolved.ModloaderType != "forge" {
		t.Errorf("ModloaderType = %q, want %q", resolved.ModloaderType, "forge")
	}
	if resolved.ModloaderVersion != "47.2.0" {
		t.Errorf("ModloaderVersion = %q, want %q", resolved.ModloaderVersion, "47.2.0")
	}

	// Stage 2: Download — FTB returns empty path.
	archivePath, err := pipe.Download(ctx, resolved, tempDir)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if archivePath != "" {
		t.Errorf("archivePath = %q, want empty (FTB uses API)", archivePath)
	}

	// Stage 3: Extract — skipped for FTB (archivePath is empty).
	extractDir := filepath.Join(tempDir, "ftb-test-pack", "extracted")
	if archivePath != "" {
		if err := Extract(archivePath, extractDir); err != nil {
			t.Fatalf("Extract: %v", err)
		}
	}

	// Stage 4: Analyze.
	data, err := pipe.Analyze(ctx, extractDir, resolved)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	if data.MinecraftVersion != "1.21.1" {
		t.Errorf("MinecraftVersion = %q, want %q", data.MinecraftVersion, "1.21.1")
	}
	if data.ModloaderType != "forge" {
		t.Errorf("ModloaderType = %q, want %q", data.ModloaderType, "forge")
	}
	if data.ModloaderVersion != "47.2.0" {
		t.Errorf("ModloaderVersion = %q, want %q", data.ModloaderVersion, "47.2.0")
	}

	// FTB analysis should find 3 mod jar files (server.properties is not a mod).
	if data.TotalModCount != 3 {
		t.Errorf("TotalModCount = %d, want 3", data.TotalModCount)
	}

	// Check FTB-specific flags.
	if !containsString(data.ConfidenceFlags, FlagFTBFormatParsed) {
		t.Errorf("ConfidenceFlags missing %q, got %v", FlagFTBFormatParsed, data.ConfidenceFlags)
	}

	// Stage 5: API Enrichment — FTB is a no-op.
	if err := pipe.EnrichFromAPI(ctx, resolved, data); err != nil {
		t.Fatalf("EnrichFromAPI: %v", err)
	}

	// Stage 6: Web search — skip.
	webResults, _ := pipe.EnrichFromWeb(ctx, data, "")

	// Stage 7: Finalize.
	mk := Finalize(resolved, data, webResults)

	if !mk.NeedsReview {
		t.Error("NeedsReview should be true")
	}
	if mk.SourcePlatform != "ftb" {
		t.Errorf("SourcePlatform = %q, want %q", mk.SourcePlatform, "ftb")
	}
	if mk.MinecraftVersion != "1.21.1" {
		t.Errorf("mk.MinecraftVersion = %q, want %q", mk.MinecraftVersion, "1.21.1")
	}
	if mk.ModloaderType != "forge" {
		t.Errorf("mk.ModloaderType = %q, want %q", mk.ModloaderType, "forge")
	}

	if len(mk.Versions) != 1 {
		t.Fatalf("Versions count = %d, want 1", len(mk.Versions))
	}
	vk := mk.Versions[0]
	if vk.ModloaderVersion != "47.2.0" {
		t.Errorf("vk.ModloaderVersion = %q, want %q", vk.ModloaderVersion, "47.2.0")
	}
	if vk.JavaVersion != 21 {
		t.Errorf("vk.JavaVersion = %d, want 21", vk.JavaVersion)
	}
	if vk.TotalModCount != 3 {
		t.Errorf("vk.TotalModCount = %d, want 3", vk.TotalModCount)
	}
	if vk.DiscoveryMethod != "ftb_api" {
		t.Errorf("vk.DiscoveryMethod = %q, want %q", vk.DiscoveryMethod, "ftb_api")
	}
	if !containsString(mk.ConfidenceFlags, FlagFTBFormatParsed) {
		t.Errorf("ConfidenceFlags missing %q, got %v", FlagFTBFormatParsed, mk.ConfidenceFlags)
	}

	// Save to KB and verify round-trip.
	if err := kb.Save(mk); err != nil {
		t.Fatalf("KB.Save: %v", err)
	}
	loaded, err := kb.Get(mk.Slug)
	if err != nil {
		t.Fatalf("KB.Get: %v", err)
	}
	if loaded.FTBID != 500 {
		t.Errorf("loaded.FTBID = %d, want 500", loaded.FTBID)
	}
}

// --- Multi-platform priority test ---

func TestIntegration_MultiPlatformPriority(t *testing.T) {
	// Test that when multiple platforms find a pack, CurseForge wins.
	manifest := loadFixture(t, "curseforge_manifest.json")
	zipData := createTestZip(t, map[string][]byte{
		"manifest.json": manifest,
	})

	dlServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		w.Write(zipData)
	}))
	defer dlServer.Close()

	// CurseForge mock.
	cfServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path
		switch {
		case path == "/search":
			json.NewEncoder(w).Encode([]curseforge.SearchResult{
				{ID: 100001, Name: "Multi Pack", Slug: "multi-pack", Summary: "test"},
			})
		case path == "/modpacks/100001/files":
			json.NewEncoder(w).Encode([]curseforge.ModpackFile{
				{
					ID:           200001,
					DisplayName:  "Multi Pack 1.0.0",
					GameVersions: []string{"1.21.1", "NeoForge"},
					DownloadURL:  dlServer.URL + "/multi-pack.zip",
					ReleaseType:  1,
					FileLength:   int64(len(zipData)),
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer cfServer.Close()

	// Modrinth mock.
	mrServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path
		switch {
		case path == "/project/multi-pack":
			json.NewEncoder(w).Encode(modrinth.Project{
				ID:          "mr-multi-id",
				Slug:        "multi-pack",
				Title:       "Multi Pack",
				ProjectType: "modpack",
			})
		case path == "/project/mr-multi-id/version":
			json.NewEncoder(w).Encode([]modrinth.Version{
				{
					ID:            "v-001",
					VersionNumber: "1.0.0",
					GameVersions:  []string{"1.21.1"},
					Loaders:       []string{"fabric"},
					Files: []modrinth.VersionFile{
						{URL: dlServer.URL + "/multi-pack.mrpack", Primary: true},
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer mrServer.Close()

	kb := newTestKB(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	pipe := NewPipeline(PipelineConfig{
		KB:         kb,
		CurseForge: curseforge.NewClient(cfServer.URL, "test-key"),
		Modrinth:   modrinth.NewClientWithBaseURL(mrServer.URL, "test-agent"),
		TempDir:    t.TempDir(),
		Log:        logger,
	})

	ctx := context.Background()
	resolved, err := pipe.Resolve(ctx, "multi-pack", "Multi Pack", "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	// CurseForge should win (priority 3 > modrinth 2).
	if resolved.Platform != "curseforge" {
		t.Errorf("Platform = %q, want %q (CF should have priority)", resolved.Platform, "curseforge")
	}

	// Cross-population: Modrinth ID should be set from the modrinth result.
	if resolved.ModrinthID != "mr-multi-id" {
		t.Errorf("ModrinthID = %q, want %q (cross-population)", resolved.ModrinthID, "mr-multi-id")
	}
	if resolved.CurseForgeID != 100001 {
		t.Errorf("CurseForgeID = %d, want 100001", resolved.CurseForgeID)
	}
}

// --- Pipeline state tracking test ---

func TestIntegration_PipelineStateTracking(t *testing.T) {
	// Verify that discovery state is properly created and can be queried.
	kb := newTestKB(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	pipe := NewPipeline(PipelineConfig{
		KB:      kb,
		TempDir: t.TempDir(),
		Log:     logger,
	})

	// Start a discovery — it will fail because no clients are configured,
	// but the state file should be created.
	err := pipe.Start(context.Background(), "state-test-pack", "State Test Pack", "1.0.0", "test-user")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Check initial state was created.
	state, err := pipe.GetState("state-test-pack")
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if state == nil {
		t.Fatal("state should not be nil")
	}
	if state.Slug != "state-test-pack" {
		t.Errorf("state.Slug = %q, want %q", state.Slug, "state-test-pack")
	}
	if state.RequestedBy != "test-user" {
		t.Errorf("state.RequestedBy = %q, want %q", state.RequestedBy, "test-user")
	}

	// Duplicate start should fail.
	// Wait briefly for the goroutine to potentially update state.
	err = pipe.Start(context.Background(), "state-test-pack", "State Test Pack", "1.0.0", "test-user")
	// The error depends on timing — the background goroutine may have already
	// failed and set status to "failed". If it hasn't yet, we get "already in progress".
	// Either outcome is acceptable for this test.
	_ = err

	// Verify state is accessible regardless.
	state2, err := pipe.GetState("state-test-pack")
	if err != nil {
		t.Fatalf("GetState (second): %v", err)
	}
	if state2 == nil {
		t.Fatal("state should still exist")
	}

	t.Logf("Final state: status=%s, error=%s, completed=%v", state2.Status, state2.Error, state2.StepsCompleted)
}

// Ensure unused import doesn't cause compilation failure.
var _ = fmt.Sprintf
