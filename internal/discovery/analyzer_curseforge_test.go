package discovery

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseCurseForgeManifest(t *testing.T) {
	dir := t.TempDir()
	manifest := `{
		"minecraft": {
			"version": "1.20.1",
			"modLoaders": [
				{
					"id": "neoforge-21.1.77",
					"primary": true
				}
			]
		},
		"name": "All the Mods 9",
		"version": "0.3.1",
		"files": [
			{"projectID": 238222, "fileID": 4567890, "required": true},
			{"projectID": 248787, "fileID": 4567891, "required": true},
			{"projectID": 352232, "fileID": 4567892, "required": false}
		]
	}`

	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	data, err := parseCurseForgeManifest(dir)
	if err != nil {
		t.Fatal(err)
	}

	if data.Name != "All the Mods 9" {
		t.Errorf("Name = %q, want %q", data.Name, "All the Mods 9")
	}
	if data.MinecraftVersion != "1.20.1" {
		t.Errorf("MinecraftVersion = %q, want %q", data.MinecraftVersion, "1.20.1")
	}
	if data.ModloaderType != "neoforge" {
		t.Errorf("ModloaderType = %q, want %q", data.ModloaderType, "neoforge")
	}
	if data.ModloaderVersion != "21.1.77" {
		t.Errorf("ModloaderVersion = %q, want %q", data.ModloaderVersion, "21.1.77")
	}
	if len(data.ModList) != 3 {
		t.Errorf("ModList len = %d, want 3", len(data.ModList))
	}
	if data.ModList[0].ProjectID != 238222 {
		t.Errorf("ModList[0].ProjectID = %d, want 238222", data.ModList[0].ProjectID)
	}
}

func TestParseCurseForgeManifestMissingFile(t *testing.T) {
	dir := t.TempDir()
	_, err := parseCurseForgeManifest(dir)
	if err == nil {
		t.Error("expected error for missing manifest.json")
	}
}

func TestParseCFModloader(t *testing.T) {
	tests := []struct {
		id          string
		wantType    string
		wantVersion string
	}{
		{"forge-47.2.0", "forge", "47.2.0"},
		{"neoforge-21.1.77", "neoforge", "21.1.77"},
		{"fabric-0.15.3", "fabric", "0.15.3"},
		{"forge", "forge", ""},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			gotType, gotVersion := parseCFModloader(tt.id)
			if gotType != tt.wantType {
				t.Errorf("type = %q, want %q", gotType, tt.wantType)
			}
			if gotVersion != tt.wantVersion {
				t.Errorf("version = %q, want %q", gotVersion, tt.wantVersion)
			}
		})
	}
}
