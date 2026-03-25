package modpackkb

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ModpackKnowledge represents deployment knowledge for a modpack.
type ModpackKnowledge struct {
	// Identity
	Slug         string `json:"slug"`
	Name         string `json:"name"`
	CurseForgeID int    `json:"curseforge_id,omitempty"`

	// Modpack-wide defaults (apply unless a version overrides)
	ModloaderType     string `json:"modloader_type"`
	ModloaderStrategy string `json:"modloader_strategy"`
	HasServerPack     bool   `json:"has_server_pack"`
	ServerPackPattern string `json:"server_pack_pattern,omitempty"`
	MinecraftVersion  string `json:"minecraft_version,omitempty"`

	// Modpack-wide resource sizing defaults
	RecommendedCPU    int    `json:"recommended_cpu_mhz,omitempty"`
	RecommendedMemory int    `json:"recommended_memory_mb,omitempty"`
	InitMemory        string `json:"init_memory,omitempty"`
	MaxMemory         string `json:"max_memory,omitempty"`

	// Deployment notes (free text, modpack-wide)
	DeploymentNotes string `json:"deployment_notes,omitempty"`

	// Per-version knowledge (newest first)
	Versions []VersionKnowledge `json:"versions,omitempty"`

	// Metadata
	Source    string `json:"source"`
	UpdatedBy string `json:"updated_by"`
	UpdatedAt string `json:"updated_at"`
	CreatedAt string `json:"created_at"`
}

// VersionKnowledge captures deployment details specific to a modpack version.
type VersionKnowledge struct {
	PackVersion      string `json:"pack_version"`
	MinecraftVersion string `json:"minecraft_version"`
	ModloaderType    string `json:"modloader_type,omitempty"`
	ModloaderVersion string `json:"modloader_version"`
	JavaVersion      int    `json:"java_version"`
	ItzgDockerTag    string `json:"itzg_docker_tag"`
	ServerPackFileID int    `json:"server_pack_file_id,omitempty"`
	ServerPackNotes  string `json:"server_pack_notes,omitempty"`
	DeploymentNotes  string `json:"deployment_notes,omitempty"`
	Source           string `json:"source"`
	UpdatedAt        string `json:"updated_at"`
}

// KB is the modpack knowledge base backed by JSON files on disk.
type KB struct {
	dir string
	log *slog.Logger
	mu  sync.RWMutex
}

// New creates a KB that stores modpack knowledge in dir.
func New(dir string, log *slog.Logger) (*KB, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create modpack-kb dir: %w", err)
	}
	return &KB{dir: dir, log: log}, nil
}

// Get reads a modpack knowledge file by slug.
func (kb *KB) Get(slug string) (*ModpackKnowledge, error) {
	kb.mu.RLock()
	defer kb.mu.RUnlock()

	data, err := os.ReadFile(kb.path(slug))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("modpack %q not found", slug)
		}
		return nil, fmt.Errorf("read modpack %q: %w", slug, err)
	}

	var mk ModpackKnowledge
	if err := json.Unmarshal(data, &mk); err != nil {
		return nil, fmt.Errorf("parse modpack %q: %w", slug, err)
	}
	return &mk, nil
}

// GetByName looks up a modpack using fuzzy name matching.
// It handles abbreviations like "ATM9" matching "all-the-mods-9" and
// case-insensitive substring matches. Prefers exact matches over substrings.
func (kb *KB) GetByName(name string) (*ModpackKnowledge, error) {
	// First try exact slug match.
	slug := Slugify(name)
	if mk, err := kb.Get(slug); err == nil {
		return mk, nil
	}

	// Scan all entries for fuzzy matches with scoring.
	entries, err := kb.listEntries()
	if err != nil {
		return nil, err
	}

	lowerName := strings.ToLower(name)
	expandedName := expandAbbreviation(lowerName)

	// Try expanded abbreviation as exact slug first.
	if expandedName != lowerName {
		if mk, getErr := kb.Get(expandedName); getErr == nil {
			return mk, nil
		}
	}

	var bestMatch *ModpackKnowledge
	bestScore := 0

	for _, mk := range entries {
		lowerMKName := strings.ToLower(mk.Name)
		lowerSlug := mk.Slug
		score := 0

		// Exact name match (case-insensitive).
		if lowerMKName == lowerName {
			score = 4
		} else if lowerSlug == lowerName {
			score = 4
		} else if expandedName != lowerName && lowerSlug == expandedName {
			// Exact expanded abbreviation match.
			score = 3
		} else if strings.Contains(lowerMKName, lowerName) || strings.Contains(lowerSlug, lowerName) {
			// Substring match on original query.
			score = 2
		} else if expandedName != lowerName && strings.Contains(lowerSlug, expandedName) {
			// Substring match on expanded abbreviation.
			score = 1
		}

		if score > bestScore {
			bestScore = score
			bestMatch = mk
		}
	}

	if bestMatch != nil {
		return bestMatch, nil
	}

	return nil, fmt.Errorf("modpack %q not found", name)
}

// GetByCurseForgeID scans all entries for a matching CurseForge project ID.
func (kb *KB) GetByCurseForgeID(id int) (*ModpackKnowledge, error) {
	entries, err := kb.listEntries()
	if err != nil {
		return nil, err
	}

	for _, mk := range entries {
		if mk.CurseForgeID == id {
			return mk, nil
		}
	}

	return nil, fmt.Errorf("modpack with CurseForge ID %d not found", id)
}

// Save writes or updates a modpack knowledge entry. When updating, non-zero
// fields in the provided entry overwrite existing values (merge semantics).
func (kb *KB) Save(mk *ModpackKnowledge) error {
	kb.mu.Lock()
	defer kb.mu.Unlock()

	if mk.Slug == "" {
		if mk.Name == "" {
			return fmt.Errorf("modpack slug or name is required")
		}
		mk.Slug = Slugify(mk.Name)
	}

	now := time.Now().UTC().Format(time.RFC3339)

	// Try to load existing entry for merge.
	existing, err := kb.readUnlocked(mk.Slug)
	if err == nil {
		// Merge: overwrite only non-zero fields.
		mergeModpack(existing, mk)
		existing.UpdatedAt = now
		mk = existing
	} else {
		// New entry.
		if mk.CreatedAt == "" {
			mk.CreatedAt = now
		}
		mk.UpdatedAt = now
	}

	data, err := json.MarshalIndent(mk, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal modpack %q: %w", mk.Slug, err)
	}

	if err := os.WriteFile(kb.path(mk.Slug), data, 0o644); err != nil {
		return fmt.Errorf("write modpack %q: %w", mk.Slug, err)
	}

	kb.log.Info("modpack-kb saved", "slug", mk.Slug, "name", mk.Name)
	return nil
}

// Delete removes a modpack knowledge file by slug.
func (kb *KB) Delete(slug string) error {
	kb.mu.Lock()
	defer kb.mu.Unlock()

	if err := os.Remove(kb.path(slug)); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("modpack %q not found", slug)
		}
		return fmt.Errorf("delete modpack %q: %w", slug, err)
	}

	kb.log.Info("modpack-kb deleted", "slug", slug)
	return nil
}

// List returns all modpack knowledge entries.
func (kb *KB) List() ([]*ModpackKnowledge, error) {
	return kb.listEntries()
}

// Search returns modpack entries whose name or slug contains the query (case-insensitive).
func (kb *KB) Search(query string) ([]*ModpackKnowledge, error) {
	entries, err := kb.listEntries()
	if err != nil {
		return nil, err
	}

	lowerQuery := strings.ToLower(query)
	expandedQuery := expandAbbreviation(lowerQuery)

	var results []*ModpackKnowledge
	for _, mk := range entries {
		lowerName := strings.ToLower(mk.Name)
		lowerSlug := mk.Slug

		if strings.Contains(lowerName, lowerQuery) ||
			strings.Contains(lowerSlug, lowerQuery) ||
			(expandedQuery != lowerQuery && strings.Contains(lowerSlug, expandedQuery)) {
			results = append(results, mk)
		}
	}

	return results, nil
}

// listEntries reads all JSON files from the KB directory.
func (kb *KB) listEntries() ([]*ModpackKnowledge, error) {
	kb.mu.RLock()
	defer kb.mu.RUnlock()

	entries, err := os.ReadDir(kb.dir)
	if err != nil {
		return nil, fmt.Errorf("read modpack-kb dir: %w", err)
	}

	var results []*ModpackKnowledge
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		slug := strings.TrimSuffix(e.Name(), ".json")
		mk, readErr := kb.readUnlocked(slug)
		if readErr != nil {
			kb.log.Warn("modpack-kb: skipping corrupt entry", "slug", slug, "error", readErr)
			continue
		}
		results = append(results, mk)
	}

	return results, nil
}

// readUnlocked reads a single entry without acquiring the lock (caller must hold it).
func (kb *KB) readUnlocked(slug string) (*ModpackKnowledge, error) {
	data, err := os.ReadFile(kb.path(slug))
	if err != nil {
		return nil, err
	}
	var mk ModpackKnowledge
	if err := json.Unmarshal(data, &mk); err != nil {
		return nil, err
	}
	return &mk, nil
}

func (kb *KB) path(slug string) string {
	return filepath.Join(kb.dir, slug+".json")
}

// mergeModpack overwrites fields in dst with non-zero values from src.
func mergeModpack(dst, src *ModpackKnowledge) {
	if src.Name != "" {
		dst.Name = src.Name
	}
	if src.CurseForgeID != 0 {
		dst.CurseForgeID = src.CurseForgeID
	}
	if src.ModloaderType != "" {
		dst.ModloaderType = src.ModloaderType
	}
	if src.ModloaderStrategy != "" {
		dst.ModloaderStrategy = src.ModloaderStrategy
	}
	if src.HasServerPack {
		dst.HasServerPack = src.HasServerPack
	}
	if src.ServerPackPattern != "" {
		dst.ServerPackPattern = src.ServerPackPattern
	}
	if src.MinecraftVersion != "" {
		dst.MinecraftVersion = src.MinecraftVersion
	}
	if src.RecommendedCPU != 0 {
		dst.RecommendedCPU = src.RecommendedCPU
	}
	if src.RecommendedMemory != 0 {
		dst.RecommendedMemory = src.RecommendedMemory
	}
	if src.InitMemory != "" {
		dst.InitMemory = src.InitMemory
	}
	if src.MaxMemory != "" {
		dst.MaxMemory = src.MaxMemory
	}
	if src.DeploymentNotes != "" {
		dst.DeploymentNotes = src.DeploymentNotes
	}
	if len(src.Versions) > 0 {
		dst.Versions = mergeVersions(dst.Versions, src.Versions)
	}
	if src.Source != "" {
		dst.Source = src.Source
	}
	if src.UpdatedBy != "" {
		dst.UpdatedBy = src.UpdatedBy
	}
}

// mergeVersions merges version entries by pack_version. Incoming versions
// overwrite existing entries with the same pack_version; new versions are appended.
func mergeVersions(existing, incoming []VersionKnowledge) []VersionKnowledge {
	byVersion := make(map[string]int, len(existing))
	result := make([]VersionKnowledge, len(existing))
	copy(result, existing)

	for i, v := range result {
		byVersion[v.PackVersion] = i
	}

	for _, v := range incoming {
		if idx, ok := byVersion[v.PackVersion]; ok {
			result[idx] = v
		} else {
			result = append(result, v)
			byVersion[v.PackVersion] = len(result) - 1
		}
	}

	return result
}

// expandAbbreviation expands common modpack abbreviations.
// Example: "atm9" -> "all-the-mods-9", "atm10" -> "all-the-mods-10"
func expandAbbreviation(name string) string {
	lower := strings.ToLower(name)

	// ATM pattern: atm9, atm10, atm 9, etc.
	if strings.HasPrefix(lower, "atm") {
		rest := strings.TrimPrefix(lower, "atm")
		rest = strings.TrimSpace(rest)
		rest = strings.ReplaceAll(rest, " ", "-")
		if rest != "" {
			return "all-the-mods-" + rest
		}
		return "all-the-mods"
	}

	return name
}
