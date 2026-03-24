package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
	"github.com/mark3labs/mcp-go/server"

	"github.com/lobo235/homelab-mcp-server/internal/clients"
	"github.com/lobo235/homelab-mcp-server/internal/clients/adguard"
	"github.com/lobo235/homelab-mcp-server/internal/clients/cloudflare"
	"github.com/lobo235/homelab-mcp-server/internal/clients/curseforge"
	"github.com/lobo235/homelab-mcp-server/internal/clients/minecraft"
	"github.com/lobo235/homelab-mcp-server/internal/clients/nomad"
	"github.com/lobo235/homelab-mcp-server/internal/clients/vault"
	"github.com/lobo235/homelab-mcp-server/internal/config"
	"github.com/lobo235/homelab-mcp-server/internal/itzgcache"
	"github.com/lobo235/homelab-mcp-server/internal/prompts"
	"github.com/lobo235/homelab-mcp-server/internal/resilience"
	"github.com/lobo235/homelab-mcp-server/internal/speccache"
	"github.com/lobo235/homelab-mcp-server/internal/tools/atomic"
	"github.com/lobo235/homelab-mcp-server/internal/tools/highlevel"
	"github.com/lobo235/homelab-mcp-server/internal/tools/orchestration"
)

// version is set at build time via -ldflags "-X main.version=<value>".
var version = "dev"

func main() {
	// Load .env if present — ignore error if file doesn't exist.
	_ = godotenv.Load()

	// Bootstrap logger at INFO so we can log config errors before cfg is loaded.
	log := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	log.Info("starting homelab-mcp-server", "version", version)

	cfg, err := config.Load()
	if err != nil {
		log.Error("config error", "error", err)
		os.Exit(1)
	}

	// Re-create logger at the configured level.
	log = slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: cfg.SlogLevel()}))

	// Create gateway clients with retry + circuit breaker.
	nomadClient := nomad.NewClientWithBase(
		clients.NewBaseWithResilience(cfg.NomadGatewayURL, cfg.NomadGatewayKey, resilience.DefaultCircuitBreaker()))
	adguardClient := adguard.NewClientWithBase(
		clients.NewBaseWithResilience(cfg.AdguardGatewayURL, cfg.AdguardGatewayKey, resilience.DefaultCircuitBreaker()))
	cfClient := cloudflare.NewClientWithBase(
		clients.NewBaseWithResilience(cfg.CFGatewayURL, cfg.CFGatewayKey, resilience.DefaultCircuitBreaker()))
	mcClient := minecraft.NewClientWithBase(
		clients.NewBaseWithResilience(cfg.MinecraftGatewayURL, cfg.MinecraftGatewayKey, resilience.DefaultCircuitBreaker()))
	curseforgeClient := curseforge.NewClientWithBase(
		clients.NewBaseWithResilience(cfg.CurseforgeGatewayURL, cfg.CurseforgeGatewayKey, resilience.DefaultCircuitBreaker()))
	vaultClient := vault.NewClientWithBase(
		clients.NewBaseWithResilience(cfg.VaultGatewayURL, cfg.VaultGatewayKey, resilience.DefaultCircuitBreaker()))

	// Startup health checks — log warnings for unreachable gateways.
	checkGatewayHealth(log, "nomad-gateway", nomadClient)
	checkGatewayHealth(log, "adguard-home-gateway", adguardClient)
	checkGatewayHealth(log, "cloudflare-gateway", cfClient)
	checkGatewayHealth(log, "minecraft-gateway", mcClient)
	checkGatewayHealth(log, "curseforge-gateway", curseforgeClient)
	checkGatewayHealth(log, "vault-gateway", vaultClient)

	// Seed spec cache (fetches all job specs if cache dir is empty).
	specCacheDir := filepath.Join(cfg.DataDir, "spec-cache")
	specCache, err := speccache.New(specCacheDir, log)
	if err != nil {
		log.Error("spec-cache init failed", "error", err)
		os.Exit(1)
	}
	seedFn := func(ctx context.Context) (map[string]string, error) {
		jobs, listErr := nomadClient.ListJobs(ctx)
		if listErr != nil {
			return nil, listErr
		}
		specs := make(map[string]string, len(jobs))
		for _, job := range jobs {
			spec, specErr := nomadClient.GetJobSpec(ctx, job.ID)
			if specErr != nil {
				log.Warn("spec-cache: skipping job", "job", job.ID, "error", specErr)
				continue
			}
			specs[job.ID] = spec
		}
		return specs, nil
	}
	if seedErr := specCache.Seed(context.Background(), seedFn); seedErr != nil {
		log.Warn("spec-cache seed failed", "error", seedErr)
	}

	// Start itzg docs cache with background refresh.
	itzgCacheDir := filepath.Join(cfg.DataDir, "itzg-docs")
	itzgCache, err := itzgcache.New(itzgCacheDir, log)
	if err != nil {
		log.Error("itzg-cache init failed", "error", err)
		os.Exit(1)
	}
	itzgCtx, itzgCancel := context.WithCancel(context.Background())
	defer itzgCancel()
	itzgCache.StartBackgroundRefresh(itzgCtx, cfg.ItzgDocsRefreshInterval)

	// Create MCP server.
	s := server.NewMCPServer(
		"homelab-mcp-server",
		version,
		server.WithToolCapabilities(true),
		server.WithPromptCapabilities(true),
		server.WithLogging(),
	)

	// Register Layer 1 — Atomic tools.
	atomic.Register(s, &atomic.Deps{
		Nomad:             nomadClient,
		Adguard:           adguardClient,
		Cloudflare:        cfClient,
		Minecraft:         mcClient,
		Curseforge:        curseforgeClient,
		Vault:             vaultClient,
		ItzgDocs:          itzgCache,
		SpecCache:         specCache,
		Log:               log,
		CFZoneName:        cfg.CFZoneName,
		ArtifactAllowlist: cfg.ArtifactAllowlist,
		NFSBasePath:       cfg.NFSBasePath,
		NomadDatacenter:   cfg.NomadDefaultDatacenter,
		NomadNodePool:     cfg.NomadDefaultNodePool,
	})

	// Register Layer 2 — Orchestration tools.
	orchestration.Register(s, &orchestration.Deps{
		Nomad:             nomadClient,
		Adguard:           adguardClient,
		Cloudflare:        cfClient,
		Minecraft:         mcClient,
		Vault:             vaultClient,
		Log:               log,
		CFZoneName:        cfg.CFZoneName,
		MCPublicDomain:    cfg.MCPublicDomain,
		ArtifactAllowlist: cfg.ArtifactAllowlist,
		NFSBasePath:       cfg.NFSBasePath,
		NomadDatacenter:   cfg.NomadDefaultDatacenter,
		NomadNodePool:     cfg.NomadDefaultNodePool,
	})

	// Register Layer 3 — High-level task tools.
	highlevel.Register(s, &highlevel.Deps{
		Nomad:     nomadClient,
		Minecraft: mcClient,
		Log:       log,
	})

	// Register MCP prompts.
	prompts.Register(s, &prompts.Config{
		NomadDefaultDatacenter: cfg.NomadDefaultDatacenter,
		NomadDefaultNodePool:   cfg.NomadDefaultNodePool,
		NFSBasePath:            cfg.NFSBasePath,
		MCPublicDomain:         cfg.MCPublicDomain,
		MCPublicIP:             cfg.MCPublicIP,
		CFZoneName:             cfg.CFZoneName,
	})

	log.Info("MCP server ready", "tools_registered", true, "prompts_registered", true)

	// Start stdio transport — blocks until process is terminated.
	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}

// healthChecker is implemented by all gateway clients.
type healthChecker interface {
	Ping(ctx context.Context) error
}

func checkGatewayHealth(log *slog.Logger, name string, client healthChecker) {
	ctx := context.Background()
	if err := client.Ping(ctx); err != nil {
		log.Warn("gateway unreachable on startup", "gateway", name, "error", err)
	} else {
		log.Info("gateway healthy", "gateway", name)
	}
}
