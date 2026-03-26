package discovery

import (
	"github.com/lobo235/homelab-mcp-server/internal/modpackkb"
)

// ResolvedPack contains all information gathered during the RESOLVE stage,
// identifying the modpack across platforms and locating its download URL.
type ResolvedPack struct {
	Slug              string
	Name              string
	Platform          string // "curseforge", "modrinth", "ftb"
	CurseForgeID      int
	ModrinthID        string
	FTBID             int
	PackVersion       string
	MinecraftVersion  string
	ModloaderType     string
	ModloaderVersion  string
	DownloadURL       string
	ServerPackURL     string // empty if no server pack
	ServerPackFileID  int
	ModrinthVersionID string
	Format            string // "curseforge_zip", "modrinth_mrpack", "ftb_pack"
}

// ExtractedData holds intermediate results from mechanical analysis.
type ExtractedData struct {
	// From format-specific parser.
	Name             string
	MinecraftVersion string
	ModloaderType    string
	ModloaderVersion string
	ModList          []ExtractedMod

	// From common analysis.
	HasServerPack     bool
	ServerPackPattern string
	JVMArgs           string
	InitMemory        string
	MaxMemory         string
	JavaVersion       int
	ItzgDockerTag     string
	StartupMethod     string
	StartupScriptName string
	ServerProperties  map[string]string
	LevelType         string
	GCStrategy        string

	// Mod intelligence.
	ClientOnlyMods []string
	ExtraPortMods  []modpackkb.PortMapping
	HeavyMods      []string
	WorldGenMods   []string
	TotalModCount  int
	ServerModCount int

	// Confidence tracking.
	ConfidenceFlags []string
}

// ExtractedMod represents a single mod found during extraction.
type ExtractedMod struct {
	Slug      string
	Name      string
	FileName  string
	ProjectID int
	FileID    int
}

// WebSearchResults holds structured data returned from LLM web search enrichment.
type WebSearchResults struct {
	RecommendedMemoryMB int                          `json:"recommended_memory_mb"`
	JVMArgs             string                       `json:"jvm_args"`
	KnownIssues         []string                     `json:"known_issues"`
	ConfigOverrides     map[string]map[string]string `json:"config_overrides"`
	ModsToRemove        []string                     `json:"mods_to_remove"`
	DeploymentNotes     string                       `json:"deployment_notes"`
	Sources             []string                     `json:"sources"`
}
