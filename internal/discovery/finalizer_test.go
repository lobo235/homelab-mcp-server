package discovery

import (
	"testing"

	"github.com/lobo235/homelab-mcp-server/internal/modpackkb"
)

func TestFinalize_BasicAssembly(t *testing.T) {
	resolved := &ResolvedPack{
		Slug:             "all-the-mods-9",
		Name:             "All the Mods 9",
		Platform:         "curseforge",
		CurseForgeID:     715572,
		PackVersion:      "0.3.1",
		MinecraftVersion: "1.20.1",
		Format:           "curseforge_zip",
		ServerPackFileID: 12345,
	}

	data := &ExtractedData{
		Name:             "All the Mods 9",
		MinecraftVersion: "1.20.1",
		ModloaderType:    "neoforge",
		ModloaderVersion: "21.1.77",
		JavaVersion:      17,
		ItzgDockerTag:    "java17",
		TotalModCount:    250,
		ServerModCount:   200,
	}

	mk := Finalize(resolved, data, nil)

	if mk.Slug != "all-the-mods-9" {
		t.Errorf("Slug = %q, want %q", mk.Slug, "all-the-mods-9")
	}
	if mk.Name != "All the Mods 9" {
		t.Errorf("Name = %q, want %q", mk.Name, "All the Mods 9")
	}
	if mk.CurseForgeID != 715572 {
		t.Errorf("CurseForgeID = %d, want 715572", mk.CurseForgeID)
	}
	if mk.SourcePlatform != "curseforge" {
		t.Errorf("SourcePlatform = %q, want %q", mk.SourcePlatform, "curseforge")
	}
	if mk.ModloaderType != "neoforge" {
		t.Errorf("ModloaderType = %q, want %q", mk.ModloaderType, "neoforge")
	}
	if !mk.NeedsReview {
		t.Error("NeedsReview should be true")
	}
	if mk.Source != "auto_discovery" {
		t.Errorf("Source = %q, want %q", mk.Source, "auto_discovery")
	}
	if mk.UpdatedBy != "discovery_pipeline" {
		t.Errorf("UpdatedBy = %q, want %q", mk.UpdatedBy, "discovery_pipeline")
	}
	if len(mk.Versions) != 1 {
		t.Fatalf("Versions len = %d, want 1", len(mk.Versions))
	}

	vk := mk.Versions[0]
	if vk.PackVersion != "0.3.1" {
		t.Errorf("Version.PackVersion = %q, want %q", vk.PackVersion, "0.3.1")
	}
	if vk.JavaVersion != 17 {
		t.Errorf("Version.JavaVersion = %d, want 17", vk.JavaVersion)
	}
	if vk.ServerPackFileID != 12345 {
		t.Errorf("Version.ServerPackFileID = %d, want 12345", vk.ServerPackFileID)
	}
	if vk.DiscoveryMethod != "manifest_parse" {
		t.Errorf("Version.DiscoveryMethod = %q, want %q", vk.DiscoveryMethod, "manifest_parse")
	}
}

func TestFinalize_ScriptMemoryWins(t *testing.T) {
	resolved := &ResolvedPack{
		Slug:     "test-pack",
		Name:     "Test Pack",
		Platform: "curseforge",
		Format:   "curseforge_zip",
	}

	data := &ExtractedData{
		// From startup script (priority 1).
		InitMemory: "6g",
		MaxMemory:  "12g",
		JVMArgs:    "-XX:+UseG1GC -XX:MaxGCPauseMillis=200",
	}

	webResults := &WebSearchResults{
		RecommendedMemoryMB: 8192,          // Should be ignored — script wins.
		JVMArgs:             "-XX:+UseZGC", // Should be ignored — script wins.
	}

	mk := Finalize(resolved, data, webResults)

	if mk.InitMemory != "6g" {
		t.Errorf("InitMemory = %q, want %q (script should win)", mk.InitMemory, "6g")
	}
	if mk.MaxMemory != "12g" {
		t.Errorf("MaxMemory = %q, want %q (script should win)", mk.MaxMemory, "12g")
	}
	if mk.JVMArgs != "-XX:+UseG1GC -XX:MaxGCPauseMillis=200" {
		t.Errorf("JVMArgs = %q, want script value", mk.JVMArgs)
	}
}

func TestFinalize_WebSearchFillsGaps(t *testing.T) {
	resolved := &ResolvedPack{
		Slug:     "test-pack",
		Name:     "Test Pack",
		Platform: "modrinth",
		Format:   "modrinth_mrpack",
	}

	data := &ExtractedData{
		// No memory or JVM args from extraction.
		TotalModCount: 100,
	}

	webResults := &WebSearchResults{
		RecommendedMemoryMB: 8192,
		JVMArgs:             "-XX:+UseG1GC",
		KnownIssues:         []string{"crashes on startup without forge fix"},
		DeploymentNotes:     "Needs manual config edit",
		Sources:             []string{"https://example.com/guide"},
	}

	mk := Finalize(resolved, data, webResults)

	if mk.MaxMemory != "8g" {
		t.Errorf("MaxMemory = %q, want %q (from web search)", mk.MaxMemory, "8g")
	}
	if mk.JVMArgs != "-XX:+UseG1GC" {
		t.Errorf("JVMArgs = %q, want value from web search", mk.JVMArgs)
	}
	if mk.DeploymentNotes != "Needs manual config edit" {
		t.Errorf("DeploymentNotes not set from web search")
	}
	if len(mk.Versions) != 1 || len(mk.Versions[0].KnownIssues) != 1 {
		t.Error("KnownIssues not populated from web search")
	}
}

func TestFinalize_HeuristicFallback(t *testing.T) {
	resolved := &ResolvedPack{
		Slug:     "test-pack",
		Name:     "Test Pack",
		Platform: "curseforge",
		Format:   "curseforge_zip",
	}

	data := &ExtractedData{
		// No memory from scripts or web.
		TotalModCount: 200,
		HeavyMods:     []string{"create", "mekanism"},
	}

	mk := Finalize(resolved, data, nil)

	// Should use heuristic: 200 mods with 2 heavy = 8g + 1g bump = 9g.
	if mk.MaxMemory != "9g" {
		t.Errorf("MaxMemory = %q, want %q (heuristic for 200 mods + 2 heavy)", mk.MaxMemory, "9g")
	}
	if mk.InitMemory != "4g" {
		t.Errorf("InitMemory = %q, want %q (half of 9g rounded)", mk.InitMemory, "4g")
	}
}

func TestFinalize_ModIntelligence(t *testing.T) {
	resolved := &ResolvedPack{
		Slug:     "test-pack",
		Name:     "Test Pack",
		Platform: "curseforge",
		Format:   "curseforge_zip",
	}

	data := &ExtractedData{
		ClientOnlyMods: []string{"optifine", "sodium"},
		ExtraPortMods: []modpackkb.PortMapping{
			{Port: 24454, Protocol: "udp", Purpose: "Simple Voice Chat", ModSlug: "simple-voice-chat"},
		},
	}

	mk := Finalize(resolved, data, nil)

	if len(mk.KnownClientOnlyMods) != 2 {
		t.Errorf("KnownClientOnlyMods len = %d, want 2", len(mk.KnownClientOnlyMods))
	}
	if len(mk.AdditionalPorts) != 1 {
		t.Fatalf("AdditionalPorts len = %d, want 1", len(mk.AdditionalPorts))
	}
	if mk.AdditionalPorts[0].Port != 24454 {
		t.Errorf("AdditionalPorts[0].Port = %d, want 24454", mk.AdditionalPorts[0].Port)
	}
}

func TestFinalize_WebSearchModsToRemove(t *testing.T) {
	resolved := &ResolvedPack{
		Slug:     "test-pack",
		Name:     "Test Pack",
		Platform: "curseforge",
		Format:   "curseforge_zip",
	}

	data := &ExtractedData{}
	webResults := &WebSearchResults{
		ModsToRemove: []string{"fancy-particles", "better-clouds"},
	}

	mk := Finalize(resolved, data, webResults)

	if len(mk.KnownProblematicMods) != 2 {
		t.Fatalf("KnownProblematicMods len = %d, want 2", len(mk.KnownProblematicMods))
	}
	if mk.KnownProblematicMods[0].Action != "remove" {
		t.Errorf("Action = %q, want %q", mk.KnownProblematicMods[0].Action, "remove")
	}
}
