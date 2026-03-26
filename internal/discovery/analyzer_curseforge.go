package discovery

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// curseForgeManifest represents the manifest.json found in CurseForge modpack zips.
type curseForgeManifest struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	Author    string `json:"author"`
	Minecraft struct {
		Version    string `json:"version"`
		ModLoaders []struct {
			ID      string `json:"id"`
			Primary bool   `json:"primary"`
		} `json:"modLoaders"`
	} `json:"minecraft"`
	Files []struct {
		ProjectID int  `json:"projectID"`
		FileID    int  `json:"fileID"`
		Required  bool `json:"required"`
	} `json:"files"`
}

// parseCurseForgeManifest reads and parses a CurseForge manifest.json.
func parseCurseForgeManifest(extractDir string) (*ExtractedData, error) {
	manifestPath := filepath.Join(extractDir, "manifest.json")

	// Fast path: check root.
	if _, err := os.Stat(manifestPath); err != nil {
		// Search recursively (max depth 3).
		found := findFile(extractDir, "manifest.json", 3)
		if found == "" {
			return nil, &manifestNotFoundError{format: "curseforge", dir: extractDir}
		}
		manifestPath = found
	}

	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("read manifest.json: %w", err)
	}

	var manifest curseForgeManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, fmt.Errorf("parse manifest.json: %w", err)
	}

	data := &ExtractedData{
		Name:             manifest.Name,
		MinecraftVersion: manifest.Minecraft.Version,
	}

	// Parse modloader from manifest.
	if len(manifest.Minecraft.ModLoaders) > 0 {
		loader := manifest.Minecraft.ModLoaders[0]
		data.ModloaderType, data.ModloaderVersion = parseCFModloader(loader.ID)
	}

	// Build mod list from files.
	for _, f := range manifest.Files {
		mod := ExtractedMod{
			ProjectID: f.ProjectID,
			FileID:    f.FileID,
			Slug:      "", // Will be enriched later via API.
		}
		data.ModList = append(data.ModList, mod)
	}

	return data, nil
}

// parseCFModloader parses a CurseForge modloader ID like "forge-47.2.0" or "neoforge-21.1.77"
// into separate type and version strings.
func parseCFModloader(id string) (loaderType, version string) {
	id = strings.ToLower(id)
	parts := strings.SplitN(id, "-", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return id, ""
}
