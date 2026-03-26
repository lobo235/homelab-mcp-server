package discovery

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// modrinthIndex represents the modrinth.index.json found in .mrpack files.
type modrinthIndex struct {
	Name         string            `json:"name"`
	VersionID    string            `json:"versionId"`
	Dependencies map[string]string `json:"dependencies"`
	Files        []modrinthFile    `json:"files"`
}

// modrinthFile represents a file entry in a Modrinth index.
type modrinthFile struct {
	Path      string            `json:"path"`
	Hashes    map[string]string `json:"hashes"`
	Env       *modrinthEnv      `json:"env,omitempty"`
	Downloads []string          `json:"downloads"`
	FileSize  int64             `json:"fileSize"`
}

// modrinthEnv specifies client/server requirements for a file.
type modrinthEnv struct {
	Client string `json:"client"` // required|optional|unsupported
	Server string `json:"server"` // required|optional|unsupported
}

// parseModrinthIndex reads and parses a Modrinth modrinth.index.json.
func parseModrinthIndex(extractDir string) (*ExtractedData, error) {
	indexPath := filepath.Join(extractDir, "modrinth.index.json")

	// Fast path: check root.
	if _, err := os.Stat(indexPath); err != nil {
		// Search recursively (max depth 3).
		found := findFile(extractDir, "modrinth.index.json", 3)
		if found == "" {
			return nil, &manifestNotFoundError{format: "modrinth", dir: extractDir}
		}
		indexPath = found
	}

	raw, err := os.ReadFile(indexPath)
	if err != nil {
		return nil, fmt.Errorf("read modrinth.index.json: %w", err)
	}

	var index modrinthIndex
	if err := json.Unmarshal(raw, &index); err != nil {
		return nil, fmt.Errorf("parse modrinth.index.json: %w", err)
	}

	data := &ExtractedData{
		Name: index.Name,
	}

	// Extract Minecraft version from dependencies.
	if mc, ok := index.Dependencies["minecraft"]; ok {
		data.MinecraftVersion = mc
	}

	// Extract modloader from dependencies.
	for key, version := range index.Dependencies {
		switch key {
		case "forge":
			data.ModloaderType = "forge"
			data.ModloaderVersion = version
		case "neoforge":
			data.ModloaderType = "neoforge"
			data.ModloaderVersion = version
		case "fabric-loader":
			data.ModloaderType = "fabric"
			data.ModloaderVersion = version
		case "quilt-loader":
			data.ModloaderType = "quilt"
			data.ModloaderVersion = version
		}
	}

	// Build mod list from files.
	for _, f := range index.Files {
		// Extract mod slug from path: "mods/sodium-0.5.3.jar" -> "sodium"
		filename := filepath.Base(f.Path)
		slug := slugFromFilename(filename)

		mod := ExtractedMod{
			FileName: filename,
			Slug:     slug,
			Name:     slug,
		}
		data.ModList = append(data.ModList, mod)
	}

	return data, nil
}

// slugFromFilename extracts a mod slug from a jar filename.
// Example: "sodium-fabric-0.5.3+mc1.20.1.jar" -> "sodium-fabric"
// Example: "create-1.20.1-0.5.1.e.jar" -> "create"
func slugFromFilename(filename string) string {
	// Remove extension.
	name := strings.TrimSuffix(filename, filepath.Ext(filename))

	// Try to find a version number boundary.
	// Common patterns: name-1.2.3, name-mc1.20, name+mc1.20
	for i := 0; i < len(name); i++ {
		if name[i] == '-' || name[i] == '+' {
			rest := name[i+1:]
			if len(rest) > 0 && (rest[0] >= '0' && rest[0] <= '9') {
				return strings.ToLower(name[:i])
			}
			// Check for "mc" prefix followed by version.
			if strings.HasPrefix(strings.ToLower(rest), "mc") {
				return strings.ToLower(name[:i])
			}
		}
	}

	return strings.ToLower(name)
}
