package discovery

import (
	"fmt"
	"time"

	"github.com/lobo235/homelab-mcp-server/internal/modpackkb"
)

// Finalize assembles a complete ModpackKnowledge entry from all discovery sources.
// Merge priority (highest to lowest):
//  1. Pack's own startup scripts (JVM args, memory from parsed scripts)
//  2. Mechanical extraction (version, modloader, mod list from manifests)
//  3. Platform API data (identity, descriptions, file IDs)
//  4. Web search results (fills gaps only, never overwrites higher-confidence data)
//  5. Heuristic estimates (only used if nothing else available)
func Finalize(resolved *ResolvedPack, data *ExtractedData, webResults *WebSearchResults) *modpackkb.ModpackKnowledge { //nolint:gocyclo // large mapping function is inherently complex
	now := time.Now().UTC().Format(time.RFC3339)

	mk := &modpackkb.ModpackKnowledge{
		// Identity — from resolver.
		Slug:                   resolved.Slug,
		Name:                   resolved.Name,
		CurseForgeID:           resolved.CurseForgeID,
		ModrinthID:             resolved.ModrinthID,
		FTBID:                  resolved.FTBID,
		SourcePlatform:         resolved.Platform,
		PackDistributionFormat: resolved.Format,

		// Modloader — from extraction (priority 2).
		ModloaderType: data.ModloaderType,
		HasServerPack: data.HasServerPack,

		// Minecraft version.
		MinecraftVersion: data.MinecraftVersion,

		// Always needs human review.
		NeedsReview: true,

		// Metadata.
		Source:    "auto_discovery",
		UpdatedBy: "discovery_pipeline",
		UpdatedAt: now,
		CreatedAt: now,
	}

	// Server pack pattern.
	if data.ServerPackPattern != "" {
		mk.ServerPackPattern = data.ServerPackPattern
	}

	// JVM/Memory — Priority 1: startup scripts, then web search, then heuristics.
	applyMemoryAndJVM(mk, data, webResults)

	// GC strategy.
	if data.GCStrategy != "" {
		mk.GCStrategy = data.GCStrategy
	}

	// Startup method.
	if data.StartupMethod != "" {
		mk.StartupMethod = data.StartupMethod
	}
	if data.StartupScriptName != "" {
		mk.StartupScriptName = data.StartupScriptName
	}

	// Level type.
	if data.LevelType != "" {
		mk.LevelType = data.LevelType
	}

	// Additional ports from mod analysis.
	if len(data.ExtraPortMods) > 0 {
		mk.AdditionalPorts = data.ExtraPortMods
	}

	// Client-only mods.
	if len(data.ClientOnlyMods) > 0 {
		mk.KnownClientOnlyMods = data.ClientOnlyMods
	}

	// Config overrides from web search.
	if webResults != nil && len(webResults.ConfigOverrides) > 0 {
		mk.RequiredConfigOverrides = webResults.ConfigOverrides
	}

	// Mods to remove (from web search) become problematic mods.
	if webResults != nil && len(webResults.ModsToRemove) > 0 {
		for _, slug := range webResults.ModsToRemove {
			mk.KnownProblematicMods = append(mk.KnownProblematicMods, modpackkb.ProblematicMod{
				ModSlug: slug,
				Issue:   "identified for removal by web search",
				Action:  "remove",
			})
		}
	}

	// Deployment notes — prefer web search if mechanical didn't find any.
	if webResults != nil && webResults.DeploymentNotes != "" {
		mk.DeploymentNotes = webResults.DeploymentNotes
	}

	// Build the version entry.
	vk := modpackkb.VersionKnowledge{
		PackVersion:      resolved.PackVersion,
		MinecraftVersion: data.MinecraftVersion,
		ModloaderType:    data.ModloaderType,
		ModloaderVersion: data.ModloaderVersion,
		JavaVersion:      data.JavaVersion,
		ItzgDockerTag:    data.ItzgDockerTag,
		TotalModCount:    data.TotalModCount,
		ServerModCount:   data.ServerModCount,
		Source:           "auto_discovery",
		UpdatedAt:        now,
	}

	if resolved.ServerPackFileID != 0 {
		vk.ServerPackFileID = resolved.ServerPackFileID
	}
	if resolved.ModrinthVersionID != "" {
		vk.ModrinthVersionID = resolved.ModrinthVersionID
	}

	// Discovery method.
	switch resolved.Format {
	case "curseforge_zip":
		vk.DiscoveryMethod = "manifest_parse"
	case "modrinth_mrpack":
		vk.DiscoveryMethod = "mrpack_parse"
	case "ftb_pack":
		vk.DiscoveryMethod = "ftb_api"
	}

	// Known issues from web search.
	if webResults != nil && len(webResults.KnownIssues) > 0 {
		vk.KnownIssues = webResults.KnownIssues
	}

	// Source URLs from web search.
	if webResults != nil && len(webResults.Sources) > 0 {
		vk.SourceURLs = webResults.Sources
	}

	mk.Versions = []modpackkb.VersionKnowledge{vk}

	// Confidence flags.
	mk.ConfidenceFlags = assignFlags(data, webResults)

	return mk
}

// applyMemoryAndJVM applies memory and JVM settings using the merge priority.
func applyMemoryAndJVM(mk *modpackkb.ModpackKnowledge, data *ExtractedData, webResults *WebSearchResults) {
	// Priority 1: Startup scripts (already in data if found).
	if data.InitMemory != "" && data.MaxMemory != "" {
		mk.InitMemory = data.InitMemory
		mk.MaxMemory = data.MaxMemory
	}

	if data.JVMArgs != "" {
		mk.JVMArgs = data.JVMArgs
	}

	// Priority 4: Web search fills gaps.
	if webResults != nil {
		if mk.JVMArgs == "" && webResults.JVMArgs != "" {
			mk.JVMArgs = webResults.JVMArgs
		}
		if mk.MaxMemory == "" && webResults.RecommendedMemoryMB > 0 {
			mk.MaxMemory = fmt.Sprintf("%dg", webResults.RecommendedMemoryMB/1024)
			mk.InitMemory = halfMemory(mk.MaxMemory)
			mk.RecommendedMemory = webResults.RecommendedMemoryMB
		}
	}

	// Priority 5: Heuristic estimates fill remaining gaps.
	if mk.MaxMemory == "" {
		initMem, maxMem, cpuMHz := estimateResources(data.TotalModCount, data.HeavyMods, false)
		mk.InitMemory = initMem
		mk.MaxMemory = maxMem
		mk.RecommendedCPU = cpuMHz
	}

	// Set recommended memory in MB from the max memory string if not already set.
	if mk.RecommendedMemory == 0 && mk.MaxMemory != "" {
		mk.RecommendedMemory = parseMemoryGB(mk.MaxMemory) * 1024
	}

	// Set CPU if not already set.
	if mk.RecommendedCPU == 0 {
		_, _, cpuMHz := estimateResources(data.TotalModCount, data.HeavyMods, false)
		mk.RecommendedCPU = cpuMHz
	}
}
