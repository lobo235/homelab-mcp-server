package discovery

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseModrinthIndex(t *testing.T) {
	dir := t.TempDir()
	index := `{
		"formatVersion": 1,
		"game": "minecraft",
		"versionId": "1.5.2",
		"name": "Fabulously Optimized",
		"dependencies": {
			"minecraft": "1.20.4",
			"fabric-loader": "0.15.3"
		},
		"files": [
			{
				"path": "mods/sodium-fabric-0.5.3+mc1.20.4.jar",
				"hashes": {"sha512": "abc123"},
				"env": {"client": "required", "server": "unsupported"},
				"downloads": ["https://cdn.modrinth.com/data/AANobbMI/versions/mc1.20.4/sodium-fabric-0.5.3.jar"],
				"fileSize": 1234567
			},
			{
				"path": "mods/lithium-fabric-0.12.0+mc1.20.4.jar",
				"hashes": {"sha512": "def456"},
				"downloads": ["https://cdn.modrinth.com/data/gvQqBUqZ/versions/mc1.20.4/lithium-fabric-0.12.0.jar"],
				"fileSize": 234567
			},
			{
				"path": "mods/simple-voice-chat-fabric-1.20.4-2.4.30.jar",
				"hashes": {"sha512": "ghi789"},
				"downloads": ["https://cdn.modrinth.com/data/9eGKb6K1/versions/mc1.20.4/simple-voice-chat-fabric-2.4.30.jar"],
				"fileSize": 345678
			}
		]
	}`

	if err := os.WriteFile(filepath.Join(dir, "modrinth.index.json"), []byte(index), 0o644); err != nil {
		t.Fatal(err)
	}

	data, err := parseModrinthIndex(dir)
	if err != nil {
		t.Fatal(err)
	}

	if data.Name != "Fabulously Optimized" {
		t.Errorf("Name = %q, want %q", data.Name, "Fabulously Optimized")
	}
	if data.MinecraftVersion != "1.20.4" {
		t.Errorf("MinecraftVersion = %q, want %q", data.MinecraftVersion, "1.20.4")
	}
	if data.ModloaderType != "fabric" {
		t.Errorf("ModloaderType = %q, want %q", data.ModloaderType, "fabric")
	}
	if data.ModloaderVersion != "0.15.3" {
		t.Errorf("ModloaderVersion = %q, want %q", data.ModloaderVersion, "0.15.3")
	}
	if len(data.ModList) != 3 {
		t.Errorf("ModList len = %d, want 3", len(data.ModList))
	}
}

func TestParseModrinthIndexMissingFile(t *testing.T) {
	dir := t.TempDir()
	_, err := parseModrinthIndex(dir)
	if err == nil {
		t.Error("expected error for missing modrinth.index.json")
	}
}

func TestSlugFromFilename(t *testing.T) {
	tests := []struct {
		filename string
		want     string
	}{
		{"sodium-fabric-0.5.3+mc1.20.1.jar", "sodium-fabric"},
		{"create-1.20.1-0.5.1.e.jar", "create"},
		{"simple-voice-chat-fabric-1.20.4-2.4.30.jar", "simple-voice-chat-fabric"},
		{"lithium-fabric-0.12.0+mc1.20.4.jar", "lithium-fabric"},
		{"somemod.jar", "somemod"},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			got := slugFromFilename(tt.filename)
			if got != tt.want {
				t.Errorf("slugFromFilename(%q) = %q, want %q", tt.filename, got, tt.want)
			}
		})
	}
}
