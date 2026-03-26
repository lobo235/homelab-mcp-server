//go:build ignore

// Research tool — downloads real modpacks from CurseForge, Modrinth, and FTB,
// runs the full discovery pipeline on each, and produces SAMPLE_RESEARCH.md.
//
// Usage:
//
//	cd homelab-mcp-server && go run ./cmd/research
//
// Requires .env with CURSEFORGE_GATEWAY_URL, CURSEFORGE_GATEWAY_KEY set.
// Optional: ANTHROPIC_API_KEY for web search enrichment.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/joho/godotenv"

	"github.com/lobo235/homelab-mcp-server/internal/clients"
	"github.com/lobo235/homelab-mcp-server/internal/clients/curseforge"
	"github.com/lobo235/homelab-mcp-server/internal/clients/ftb"
	"github.com/lobo235/homelab-mcp-server/internal/clients/modrinth"
	"github.com/lobo235/homelab-mcp-server/internal/discovery"
	"github.com/lobo235/homelab-mcp-server/internal/modpackkb"
)

// testPack defines a modpack to test against.
type testPack struct {
	Slug     string
	Name     string
	Platform string // "curseforge", "modrinth", "ftb" — informational only, resolver searches all
}

var testPacks = []testPack{
	// CurseForge packs.
	{Slug: "all-the-mods-10", Name: "All The Mods 10", Platform: "curseforge"},
	{Slug: "better-mc-fabric", Name: "Better MC [FABRIC]", Platform: "curseforge"},
	{Slug: "rlcraft", Name: "RLCraft", Platform: "curseforge"},
	// Modrinth packs.
	{Slug: "fabulously-optimized", Name: "Fabulously Optimized", Platform: "modrinth"},
	{Slug: "cobblemon-modpack", Name: "Cobblemon", Platform: "modrinth"},
	// FTB packs.
	{Slug: "ftb-oceanblock", Name: "FTB OceanBlock", Platform: "ftb"},
	{Slug: "ftb-presents-direwolf20", Name: "FTB Presents Direwolf20", Platform: "ftb"},
}

// packResult captures the output of running the pipeline on one pack.
type packResult struct {
	Pack       testPack
	Resolved   *discovery.ResolvedPack
	Extracted  *discovery.ExtractedData
	WebSearch  *discovery.WebSearchResults
	KB         *modpackkb.ModpackKnowledge
	Error      string
	StageFail  string
	Duration   time.Duration
}

func main() {
	// Load .env — try local first, then chatbot's .env (which contains gateway URLs).
	_ = godotenv.Load()
	_ = godotenv.Load("../.env")
	_ = godotenv.Load("../homelab-chatbot/.env")

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// Validate required env vars.
	cfURL := os.Getenv("CURSEFORGE_GATEWAY_URL")
	cfKey := os.Getenv("CURSEFORGE_GATEWAY_KEY")
	if cfURL == "" || cfKey == "" {
		log.Error("CURSEFORGE_GATEWAY_URL and CURSEFORGE_GATEWAY_KEY are required")
		os.Exit(1)
	}

	anthropicKey := os.Getenv("ANTHROPIC_API_KEY")
	if anthropicKey == "" {
		log.Warn("ANTHROPIC_API_KEY not set — web search enrichment will be skipped")
	}

	// Create clients.
	cfClient := curseforge.NewClientWithBase(clients.NewBase(cfURL, cfKey))
	modrinthClient := modrinth.NewClient("homelab-mcp-server/research")
	ftbClient := ftb.NewClient()

	// Create temp directories.
	tmpBase, err := os.MkdirTemp("", "modpack-research-*")
	if err != nil {
		log.Error("failed to create temp dir", "error", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmpBase)

	kbDir := filepath.Join(tmpBase, "kb")
	if err := os.MkdirAll(kbDir, 0o700); err != nil {
		log.Error("failed to create kb dir", "error", err)
		os.Exit(1)
	}

	// Run pipeline for each pack.
	var results []packResult
	for i, tp := range testPacks {
		fmt.Printf("\n=== [%d/%d] %s (%s) ===\n", i+1, len(testPacks), tp.Name, tp.Platform)

		result := runPack(log, tp, cfClient, modrinthClient, ftbClient, anthropicKey, tmpBase, kbDir)
		results = append(results, result)

		if result.Error != "" {
			fmt.Printf("  FAILED at %s: %s (%.1fs)\n", result.StageFail, result.Error, result.Duration.Seconds())
		} else {
			fmt.Printf("  OK: %d mods, %d server-side, mc=%s, loader=%s (%.1fs)\n",
				result.Extracted.TotalModCount,
				result.Extracted.ServerModCount,
				result.Extracted.MinecraftVersion,
				result.Extracted.ModloaderType,
				result.Duration.Seconds())
		}
	}

	// Write report.
	report := buildReport(results)
	reportPath := "SAMPLE_RESEARCH.md"
	if err := os.WriteFile(reportPath, []byte(report), 0o644); err != nil {
		log.Error("failed to write report", "error", err)
		os.Exit(1)
	}
	fmt.Printf("\nReport written to %s\n", reportPath)
}

func runPack(
	log *slog.Logger,
	tp testPack,
	cfClient *curseforge.Client,
	modrinthClient *modrinth.Client,
	ftbClient *ftb.Client,
	anthropicKey string,
	tmpBase string,
	kbDir string,
) packResult {
	start := time.Now()
	result := packResult{Pack: tp}

	// Create a fresh KB for this pack.
	kb, err := modpackkb.New(kbDir, log)
	if err != nil {
		result.Error = fmt.Sprintf("init kb: %v", err)
		result.StageFail = "init"
		result.Duration = time.Since(start)
		return result
	}

	pipe := discovery.NewPipeline(discovery.PipelineConfig{
		KB:           kb,
		CurseForge:   cfClient,
		Modrinth:     modrinthClient,
		FTB:          ftbClient,
		AnthropicKey: anthropicKey,
		TempDir:      filepath.Join(tmpBase, "dl"),
		Log:          log,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Stage 1: RESOLVE
	fmt.Printf("  Resolving...\n")
	resolved, err := pipe.Resolve(ctx, tp.Slug, tp.Name, "")
	if err != nil {
		result.Error = err.Error()
		result.StageFail = "resolve"
		result.Duration = time.Since(start)
		return result
	}
	result.Resolved = resolved
	fmt.Printf("  Resolved: platform=%s, version=%s, format=%s\n", resolved.Platform, resolved.PackVersion, resolved.Format)

	// Stage 2: DOWNLOAD
	fmt.Printf("  Downloading...\n")
	archivePath, err := pipe.Download(ctx, resolved, filepath.Join(tmpBase, "dl"))
	if err != nil {
		result.Error = err.Error()
		result.StageFail = "download"
		result.Duration = time.Since(start)
		return result
	}
	if archivePath != "" {
		info, _ := os.Stat(archivePath)
		if info != nil {
			fmt.Printf("  Downloaded: %s (%.1f MB)\n", filepath.Base(archivePath), float64(info.Size())/(1024*1024))
		}
	}

	// Stage 3: EXTRACT
	extractDir := filepath.Join(tmpBase, "dl", tp.Slug, "extracted")
	if archivePath != "" {
		fmt.Printf("  Extracting...\n")
		if err := discovery.Extract(archivePath, extractDir); err != nil {
			result.Error = err.Error()
			result.StageFail = "extract"
			result.Duration = time.Since(start)
			return result
		}
	}

	// Stage 4: ANALYZE
	fmt.Printf("  Analyzing...\n")
	data, err := pipe.Analyze(ctx, extractDir, resolved)
	if err != nil {
		result.Error = err.Error()
		result.StageFail = "analyze"
		result.Duration = time.Since(start)
		return result
	}
	result.Extracted = data
	fmt.Printf("  Analyzed: %d mods total, %d server-side\n", data.TotalModCount, data.ServerModCount)

	// Stage 5: API ENRICHMENT
	fmt.Printf("  Enriching from API...\n")
	if err := pipe.EnrichFromAPI(ctx, resolved, data); err != nil {
		fmt.Printf("  API enrichment warning: %v\n", err)
	}

	// Stage 6: WEB SEARCH
	if anthropicKey != "" {
		fmt.Printf("  Web search enrichment...\n")
		webResults, err := pipe.EnrichFromWeb(ctx, data, anthropicKey)
		if err != nil {
			fmt.Printf("  Web search warning: %v\n", err)
		} else {
			result.WebSearch = webResults
		}
	} else {
		fmt.Printf("  Skipping web search (no API key)\n")
	}

	// Stage 7: FINALIZE
	fmt.Printf("  Finalizing...\n")
	mk := discovery.Finalize(resolved, data, result.WebSearch)
	result.KB = mk

	result.Duration = time.Since(start)

	// Clean up downloaded files for this pack.
	packDlDir := filepath.Join(tmpBase, "dl", tp.Slug)
	_ = os.RemoveAll(packDlDir)

	return result
}

func buildReport(results []packResult) string {
	var sb strings.Builder

	sb.WriteString("# Modpack Discovery Research Results\n\n")
	sb.WriteString(fmt.Sprintf("Generated: %s\n\n", time.Now().UTC().Format(time.RFC3339)))

	// Summary table.
	sb.WriteString("## Summary\n\n")
	sb.WriteString("| Pack | Platform | MC Version | Modloader | Mods | Server Mods | Status | Duration |\n")
	sb.WriteString("|------|----------|------------|-----------|------|-------------|--------|----------|\n")

	for _, r := range results {
		if r.Error != "" {
			sb.WriteString(fmt.Sprintf("| %s | %s | — | — | — | — | FAILED (%s) | %.1fs |\n",
				r.Pack.Name, r.Pack.Platform, r.StageFail, r.Duration.Seconds()))
		} else {
			sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %d | %d | OK | %.1fs |\n",
				r.Pack.Name, r.Resolved.Platform,
				r.Extracted.MinecraftVersion, r.Extracted.ModloaderType,
				r.Extracted.TotalModCount, r.Extracted.ServerModCount,
				r.Duration.Seconds()))
		}
	}
	sb.WriteString("\n")

	// Per-pack details.
	sb.WriteString("## Per-Pack Details\n\n")
	for _, r := range results {
		sb.WriteString(fmt.Sprintf("### %s\n\n", r.Pack.Name))

		if r.Error != "" {
			sb.WriteString(fmt.Sprintf("**Failed** at stage `%s`: %s\n\n", r.StageFail, r.Error))
			continue
		}

		// Resolution.
		sb.WriteString("**Resolution:**\n")
		sb.WriteString(fmt.Sprintf("- Platform: %s\n", r.Resolved.Platform))
		sb.WriteString(fmt.Sprintf("- Format: %s\n", r.Resolved.Format))
		sb.WriteString(fmt.Sprintf("- Pack version: %s\n", r.Resolved.PackVersion))
		sb.WriteString(fmt.Sprintf("- CurseForge ID: %d\n", r.Resolved.CurseForgeID))
		sb.WriteString(fmt.Sprintf("- Modrinth ID: %s\n", r.Resolved.ModrinthID))
		sb.WriteString(fmt.Sprintf("- FTB ID: %d\n", r.Resolved.FTBID))
		sb.WriteString(fmt.Sprintf("- Has server pack: %v\n\n", r.Resolved.ServerPackURL != ""))

		// Analysis.
		sb.WriteString("**Analysis:**\n")
		sb.WriteString(fmt.Sprintf("- Minecraft version: %s\n", r.Extracted.MinecraftVersion))
		sb.WriteString(fmt.Sprintf("- Modloader: %s %s\n", r.Extracted.ModloaderType, r.Extracted.ModloaderVersion))
		sb.WriteString(fmt.Sprintf("- Total mods: %d\n", r.Extracted.TotalModCount))
		sb.WriteString(fmt.Sprintf("- Server-side mods: %d\n", r.Extracted.ServerModCount))
		sb.WriteString(fmt.Sprintf("- Java version: %d\n", r.Extracted.JavaVersion))
		sb.WriteString(fmt.Sprintf("- Startup method: %s\n", r.Extracted.StartupMethod))

		if len(r.Extracted.ClientOnlyMods) > 0 {
			sb.WriteString(fmt.Sprintf("- Client-only mods (%d): %s\n", len(r.Extracted.ClientOnlyMods), strings.Join(r.Extracted.ClientOnlyMods, ", ")))
		}
		if len(r.Extracted.HeavyMods) > 0 {
			sb.WriteString(fmt.Sprintf("- Heavy mods: %s\n", strings.Join(r.Extracted.HeavyMods, ", ")))
		}
		if len(r.Extracted.ExtraPortMods) > 0 {
			ports := make([]string, len(r.Extracted.ExtraPortMods))
			for i, pm := range r.Extracted.ExtraPortMods {
				ports[i] = fmt.Sprintf("%s (%d/%s)", pm.ModSlug, pm.Port, pm.Protocol)
			}
			sb.WriteString(fmt.Sprintf("- Extra port mods: %s\n", strings.Join(ports, ", ")))
		}
		if len(r.Extracted.WorldGenMods) > 0 {
			sb.WriteString(fmt.Sprintf("- World gen mods: %s\n", strings.Join(r.Extracted.WorldGenMods, ", ")))
		}
		if r.Extracted.JVMArgs != "" {
			sb.WriteString(fmt.Sprintf("- JVM args: %s\n", r.Extracted.JVMArgs))
		}
		if r.Extracted.MaxMemory != "" {
			sb.WriteString(fmt.Sprintf("- Max memory: %s\n", r.Extracted.MaxMemory))
		}
		if r.Extracted.LevelType != "" {
			sb.WriteString(fmt.Sprintf("- Level type: %s\n", r.Extracted.LevelType))
		}
		sb.WriteString("\n")

		// Web search results.
		if r.WebSearch != nil {
			sb.WriteString("**Web Search Results:**\n")
			if r.WebSearch.RecommendedMemoryMB > 0 {
				sb.WriteString(fmt.Sprintf("- Recommended memory: %d MB\n", r.WebSearch.RecommendedMemoryMB))
			}
			if r.WebSearch.JVMArgs != "" {
				sb.WriteString(fmt.Sprintf("- JVM args: %s\n", r.WebSearch.JVMArgs))
			}
			if len(r.WebSearch.KnownIssues) > 0 {
				sb.WriteString("- Known issues:\n")
				for _, issue := range r.WebSearch.KnownIssues {
					sb.WriteString(fmt.Sprintf("  - %s\n", issue))
				}
			}
			if r.WebSearch.DeploymentNotes != "" {
				sb.WriteString(fmt.Sprintf("- Deployment notes: %s\n", r.WebSearch.DeploymentNotes))
			}
			if len(r.WebSearch.Sources) > 0 {
				sb.WriteString(fmt.Sprintf("- Sources: %s\n", strings.Join(r.WebSearch.Sources, ", ")))
			}
			sb.WriteString("\n")
		}

		// KB entry confidence flags.
		if r.KB != nil {
			sb.WriteString("**KB Entry:**\n")
			sb.WriteString(fmt.Sprintf("- Needs review: %v\n", r.KB.NeedsReview))
			if len(r.KB.ConfidenceFlags) > 0 {
				sb.WriteString(fmt.Sprintf("- Confidence flags: %s\n", strings.Join(r.KB.ConfidenceFlags, ", ")))
			}

			// Output KB JSON for reference.
			kbJSON, err := json.MarshalIndent(r.KB, "", "  ")
			if err == nil {
				sb.WriteString("\n<details><summary>Full KB JSON</summary>\n\n```json\n")
				sb.WriteString(string(kbJSON))
				sb.WriteString("\n```\n\n</details>\n")
			}
		}
		sb.WriteString("\n---\n\n")
	}

	return sb.String()
}
