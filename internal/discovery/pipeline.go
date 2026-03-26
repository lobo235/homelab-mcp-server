package discovery

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/lobo235/homelab-mcp-server/internal/clients/curseforge"
	"github.com/lobo235/homelab-mcp-server/internal/clients/ftb"
	"github.com/lobo235/homelab-mcp-server/internal/clients/modrinth"
	"github.com/lobo235/homelab-mcp-server/internal/modpackkb"
)

const (
	defaultTempDir  = "/tmp/modpack-discovery"
	pipelineTimeout = 30 * time.Minute
)

// PipelineConfig holds the dependencies for the discovery pipeline.
type PipelineConfig struct {
	KB           *modpackkb.KB
	CurseForge   *curseforge.Client
	Modrinth     *modrinth.Client
	FTB          *ftb.Client
	AnthropicKey string // empty = skip web search
	TempDir      string // defaults to /tmp/modpack-discovery
	Log          *slog.Logger
}

// Pipeline orchestrates the modpack discovery process.
type Pipeline struct {
	KB           *modpackkb.KB
	CurseForge   *curseforge.Client
	Modrinth     *modrinth.Client
	FTB          *ftb.Client
	AnthropicKey string
	TempDir      string
	Log          *slog.Logger
}

// NewPipeline creates a new discovery pipeline with the given configuration.
func NewPipeline(cfg PipelineConfig) *Pipeline {
	tempDir := cfg.TempDir
	if tempDir == "" {
		tempDir = defaultTempDir
	}
	log := cfg.Log
	if log == nil {
		log = slog.Default()
	}

	return &Pipeline{
		KB:           cfg.KB,
		CurseForge:   cfg.CurseForge,
		Modrinth:     cfg.Modrinth,
		FTB:          cfg.FTB,
		AnthropicKey: cfg.AnthropicKey,
		TempDir:      tempDir,
		Log:          log,
	}
}

// Start initiates an asynchronous discovery process for the given modpack.
// It creates the discovery state file and launches a background goroutine.
// Returns error only if state creation fails or pipeline is already in progress.
func (p *Pipeline) Start(ctx context.Context, slug, packName, requestedVersion, requestedBy string) error {
	if p.KB.IsDiscoveryInProgress(slug) {
		return fmt.Errorf("discovery already in progress for %q", slug)
	}

	_, err := p.KB.StartDiscovery(slug, requestedBy, requestedVersion)
	if err != nil {
		return fmt.Errorf("create discovery state: %w", err)
	}

	// Launch the pipeline in a background goroutine with its own context.
	go func() {
		runCtx, cancel := context.WithTimeout(context.Background(), pipelineTimeout)
		defer cancel()
		p.run(runCtx, slug, packName, requestedVersion)
	}()

	return nil
}

// GetState returns the current discovery state for a slug.
func (p *Pipeline) GetState(slug string) (*modpackkb.DiscoveryState, error) {
	return p.KB.GetDiscoveryState(slug)
}

// IsInProgress returns true if a discovery is currently in progress for the slug.
func (p *Pipeline) IsInProgress(slug string) bool {
	return p.KB.IsDiscoveryInProgress(slug)
}

// run sequences the discovery stages, updating state at each step.
func (p *Pipeline) run(ctx context.Context, slug, packName, requestedVersion string) {
	tempDir := filepath.Join(p.TempDir, slug)
	defer func() {
		// Clean up temp directory on completion.
		if err := os.RemoveAll(tempDir); err != nil {
			p.Log.Warn("failed to clean up temp dir", "dir", tempDir, "error", err)
		}
	}()

	p.Log.Info("discovery pipeline started", "slug", slug, "packName", packName, "requestedVersion", requestedVersion)

	// Stage 1: RESOLVE
	if err := p.KB.UpdateDiscoveryState(slug, "resolving", "", ""); err != nil {
		p.Log.Error("failed to update discovery state", "error", err)
		return
	}

	resolved, err := p.Resolve(ctx, slug, packName, requestedVersion)
	if err != nil {
		p.failDiscovery(slug, "resolve", err)
		return
	}
	if err := p.KB.UpdateDiscoveryState(slug, "downloading", "resolve", ""); err != nil {
		p.Log.Error("failed to update discovery state", "error", err)
	}
	p.Log.Info("resolve complete", "slug", slug, "platform", resolved.Platform, "version", resolved.PackVersion)

	// Stage 2: DOWNLOAD
	archivePath, err := p.Download(ctx, resolved, p.TempDir)
	if err != nil {
		p.failDiscovery(slug, "download", err)
		return
	}
	if err := p.KB.UpdateDiscoveryState(slug, "extracting", "download", ""); err != nil {
		p.Log.Error("failed to update discovery state", "error", err)
	}
	p.Log.Info("download complete", "slug", slug, "archive", archivePath)

	// Stage 3: EXTRACT
	extractDir := filepath.Join(p.TempDir, slug, "extracted")
	if archivePath != "" {
		if err := Extract(archivePath, extractDir); err != nil {
			p.failDiscovery(slug, "extract", err)
			return
		}
	}
	if err := p.KB.UpdateDiscoveryState(slug, "analyzing", "extract", ""); err != nil {
		p.Log.Error("failed to update discovery state", "error", err)
	}
	p.Log.Info("extract complete", "slug", slug)

	// Stage 4: MECHANICAL ANALYSIS
	data, err := p.Analyze(ctx, extractDir, resolved)
	if err != nil {
		p.failDiscovery(slug, "analyze", err)
		return
	}
	if err := p.KB.UpdateDiscoveryState(slug, "api_enriching", "analyze", ""); err != nil {
		p.Log.Error("failed to update discovery state", "error", err)
	}
	p.Log.Info("analysis complete", "slug", slug, "mods", data.TotalModCount, "serverMods", data.ServerModCount)

	// Stage 5: API ENRICHMENT
	if err := p.EnrichFromAPI(ctx, resolved, data); err != nil {
		p.Log.Warn("api enrichment failed, continuing", "slug", slug, "error", err)
	}
	if err := p.KB.UpdateDiscoveryState(slug, "web_searching", "api_enrich", ""); err != nil {
		p.Log.Error("failed to update discovery state", "error", err)
	}

	// Stage 6: WEB SEARCH
	webResults, err := p.EnrichFromWeb(ctx, data, p.AnthropicKey)
	if err != nil {
		p.Log.Warn("web search enrichment failed, continuing", "slug", slug, "error", err)
	}
	if err := p.KB.UpdateDiscoveryState(slug, "finalizing", "web_search", ""); err != nil {
		p.Log.Error("failed to update discovery state", "error", err)
	}

	// Stage 7: FINALIZE
	mk := Finalize(resolved, data, webResults)
	if err := p.KB.Save(mk); err != nil {
		p.failDiscovery(slug, "finalize", err)
		return
	}
	if err := p.KB.UpdateDiscoveryState(slug, "ready", "finalize", ""); err != nil {
		p.Log.Error("failed to update discovery state", "error", err)
	}

	p.Log.Info("discovery pipeline complete", "slug", slug, "needsReview", mk.NeedsReview, "flags", mk.ConfidenceFlags)
}

// failDiscovery marks the discovery as failed and logs the error.
func (p *Pipeline) failDiscovery(slug, step string, err error) {
	p.Log.Error("discovery pipeline failed", "slug", slug, "step", step, "error", err)
	if updateErr := p.KB.UpdateDiscoveryState(slug, "failed", "", fmt.Sprintf("%s: %v", step, err)); updateErr != nil {
		p.Log.Error("failed to update discovery state after failure", "error", updateErr)
	}
}
