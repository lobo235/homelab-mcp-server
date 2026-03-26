package discovery

// Confidence flag constants used throughout the discovery pipeline.
const (
	FlagResourceSizingEstimated = "resource_sizing_estimated"
	FlagNoServerPackFound       = "no_server_pack_found"
	FlagNoStartupScript         = "no_startup_script"
	FlagJVMArgsFromHeuristic    = "jvm_args_from_heuristic"
	FlagMemoryFromHeuristic     = "memory_from_heuristic"
	FlagJavaVersionInferred     = "java_version_inferred"
	FlagWebSearchUsed           = "web_search_used"
	FlagWebSearchFailed         = "web_search_failed"
	FlagWebSearchSkipped        = "web_search_skipped"
	FlagModloaderFromManifest   = "modloader_from_manifest"
	FlagClientOnlyModsDetected  = "client_only_mods_detected"
	FlagExtraPortsDetected      = "extra_ports_detected"
	FlagHeavyModsDetected       = "heavy_mods_detected"
	FlagWorldGenModDetected     = "world_gen_mod_detected"
	FlagMultiplePlatformsFound  = "multiple_platforms_found"
	FlagFTBFormatParsed         = "ftb_format_parsed"
	FlagAPIEnriched             = "api_enriched"
)

// assignFlags generates confidence flags based on what data was and wasn't found
// during discovery.
func assignFlags(data *ExtractedData, webResults *WebSearchResults) []string {
	var flags []string

	// Resource sizing flags.
	if data.JVMArgs == "" {
		flags = append(flags, FlagJVMArgsFromHeuristic)
	}
	if data.InitMemory == "" && data.MaxMemory == "" {
		flags = append(flags, FlagMemoryFromHeuristic)
		flags = append(flags, FlagResourceSizingEstimated)
	}

	// Server pack flags.
	if !data.HasServerPack {
		flags = append(flags, FlagNoServerPackFound)
	}

	// Startup script flags.
	if data.StartupMethod == "" && data.StartupScriptName == "" {
		flags = append(flags, FlagNoStartupScript)
	}

	// Java version flags.
	if data.JavaVersion == 0 {
		flags = append(flags, FlagJavaVersionInferred)
	}

	// Modloader flag.
	if data.ModloaderType != "" {
		flags = append(flags, FlagModloaderFromManifest)
	}

	// Mod intelligence flags.
	if len(data.ClientOnlyMods) > 0 {
		flags = append(flags, FlagClientOnlyModsDetected)
	}
	if len(data.ExtraPortMods) > 0 {
		flags = append(flags, FlagExtraPortsDetected)
	}
	if len(data.HeavyMods) > 0 {
		flags = append(flags, FlagHeavyModsDetected)
	}
	if len(data.WorldGenMods) > 0 {
		flags = append(flags, FlagWorldGenModDetected)
	}

	// Web search flags.
	if webResults != nil {
		flags = append(flags, FlagWebSearchUsed)
	}

	// Include any flags accumulated during extraction.
	flags = append(flags, data.ConfidenceFlags...)

	return dedup(flags)
}

// dedup removes duplicate strings from a slice while preserving order.
func dedup(items []string) []string {
	seen := make(map[string]bool, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}
	return result
}
