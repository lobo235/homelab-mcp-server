package config

import (
	"os"
	"testing"
	"time"
)

// setRequiredEnvVars sets all required environment variables to valid defaults.
func setRequiredEnvVars(t *testing.T) {
	t.Helper()
	vars := map[string]string{
		"NOMAD_GATEWAY_URL":        "http://localhost:8081",
		"NOMAD_GATEWAY_KEY":        "test-key",
		"ADGUARD_GATEWAY_URL":      "http://localhost:8082",
		"ADGUARD_GATEWAY_KEY":      "test-key",
		"CF_GATEWAY_URL":           "http://localhost:8083",
		"CF_GATEWAY_KEY":           "test-key",
		"MINECRAFT_GATEWAY_URL":    "http://localhost:8084",
		"MINECRAFT_GATEWAY_KEY":    "test-key",
		"CURSEFORGE_GATEWAY_URL":   "http://localhost:8085",
		"CURSEFORGE_GATEWAY_KEY":   "test-key",
		"VAULT_GATEWAY_URL":        "http://localhost:8086",
		"VAULT_GATEWAY_KEY":        "test-key",
		"NOMAD_DEFAULT_DATACENTER": "dc1",
		"NFS_BASE_PATH":            "/mnt/data/minecraft",
		"MC_PUBLIC_DOMAIN":         "mc.example.com",
		"CF_ZONE_NAME":             "example.com",
	}
	for k, v := range vars {
		t.Setenv(k, v)
	}
}

func TestLoad_AllRequired(t *testing.T) {
	setRequiredEnvVars(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.NomadGatewayURL != "http://localhost:8081" {
		t.Errorf("NomadGatewayURL = %q, want %q", cfg.NomadGatewayURL, "http://localhost:8081")
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "info")
	}
	if cfg.NomadDefaultNodePool != "default" {
		t.Errorf("NomadDefaultNodePool = %q, want %q", cfg.NomadDefaultNodePool, "default")
	}
	if cfg.DataDir != "/data" {
		t.Errorf("DataDir = %q, want %q", cfg.DataDir, "/data")
	}
	if cfg.ItzgDocsRefreshInterval != 24*time.Hour {
		t.Errorf("ItzgDocsRefreshInterval = %v, want %v", cfg.ItzgDocsRefreshInterval, 24*time.Hour)
	}
}

func TestLoad_MissingRequired(t *testing.T) {
	requiredVars := []string{
		"NOMAD_GATEWAY_URL",
		"NOMAD_GATEWAY_KEY",
		"ADGUARD_GATEWAY_URL",
		"ADGUARD_GATEWAY_KEY",
		"CF_GATEWAY_URL",
		"CF_GATEWAY_KEY",
		"MINECRAFT_GATEWAY_URL",
		"MINECRAFT_GATEWAY_KEY",
		"CURSEFORGE_GATEWAY_URL",
		"CURSEFORGE_GATEWAY_KEY",
		"VAULT_GATEWAY_URL",
		"VAULT_GATEWAY_KEY",
		"NOMAD_DEFAULT_DATACENTER",
		"NFS_BASE_PATH",
		"MC_PUBLIC_DOMAIN",
		"CF_ZONE_NAME",
	}

	for _, varName := range requiredVars {
		t.Run(varName, func(t *testing.T) {
			setRequiredEnvVars(t)
			os.Unsetenv(varName)

			_, err := Load()
			if err == nil {
				t.Fatalf("expected error for missing %s", varName)
			}
		})
	}
}

func TestLoad_InvalidLogLevel(t *testing.T) {
	setRequiredEnvVars(t)
	t.Setenv("LOG_LEVEL", "verbose")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid LOG_LEVEL")
	}
}

func TestLoad_ValidLogLevels(t *testing.T) {
	levels := []string{"debug", "info", "warn", "warning", "error"}
	for _, level := range levels {
		t.Run(level, func(t *testing.T) {
			setRequiredEnvVars(t)
			t.Setenv("LOG_LEVEL", level)

			cfg, err := Load()
			if err != nil {
				t.Fatalf("unexpected error for LOG_LEVEL=%q: %v", level, err)
			}
			if cfg.LogLevel != level {
				t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, level)
			}
		})
	}
}

func TestLoad_CustomDefaults(t *testing.T) {
	setRequiredEnvVars(t)
	t.Setenv("NOMAD_DEFAULT_NODE_POOL", "gpu-pool")
	t.Setenv("DATA_DIR", "/mcp-data")
	t.Setenv("ITZG_DOCS_REFRESH_INTERVAL", "12h")
	t.Setenv("LOG_LEVEL", "debug")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.NomadDefaultNodePool != "gpu-pool" {
		t.Errorf("NomadDefaultNodePool = %q, want %q", cfg.NomadDefaultNodePool, "gpu-pool")
	}
	if cfg.DataDir != "/mcp-data" {
		t.Errorf("DataDir = %q, want %q", cfg.DataDir, "/mcp-data")
	}
	if cfg.ItzgDocsRefreshInterval != 12*time.Hour {
		t.Errorf("ItzgDocsRefreshInterval = %v, want %v", cfg.ItzgDocsRefreshInterval, 12*time.Hour)
	}
}

func TestLoad_ArtifactAllowlist(t *testing.T) {
	setRequiredEnvVars(t)
	t.Setenv("ARTIFACT_ALLOWLIST", "example.com, other.org , third.net")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cfg.ArtifactAllowlist) != 3 {
		t.Fatalf("ArtifactAllowlist length = %d, want 3", len(cfg.ArtifactAllowlist))
	}
	expected := []string{"example.com", "other.org", "third.net"}
	for i, want := range expected {
		if cfg.ArtifactAllowlist[i] != want {
			t.Errorf("ArtifactAllowlist[%d] = %q, want %q", i, cfg.ArtifactAllowlist[i], want)
		}
	}
}

func TestLoad_VolumeAllowlist(t *testing.T) {
	setRequiredEnvVars(t)

	// Without VOLUME_ALLOWLIST, should only contain NFS_BASE_PATH.
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.VolumeAllowlist) != 1 || cfg.VolumeAllowlist[0] != "/mnt/data/minecraft" {
		t.Errorf("VolumeAllowlist = %v, want [/mnt/data/minecraft]", cfg.VolumeAllowlist)
	}

	// With VOLUME_ALLOWLIST, should include NFS_BASE_PATH + extras.
	t.Setenv("VOLUME_ALLOWLIST", "/mnt/data/gitea, /mnt/data/postgres")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.VolumeAllowlist) != 3 {
		t.Fatalf("VolumeAllowlist length = %d, want 3", len(cfg.VolumeAllowlist))
	}
	expected := []string{"/mnt/data/minecraft", "/mnt/data/gitea", "/mnt/data/postgres"}
	for i, want := range expected {
		if cfg.VolumeAllowlist[i] != want {
			t.Errorf("VolumeAllowlist[%d] = %q, want %q", i, cfg.VolumeAllowlist[i], want)
		}
	}
}

func TestLoad_InvalidItzgDuration(t *testing.T) {
	setRequiredEnvVars(t)
	t.Setenv("ITZG_DOCS_REFRESH_INTERVAL", "not-a-duration")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid ITZG_DOCS_REFRESH_INTERVAL")
	}
}

func TestSlogLevel(t *testing.T) {
	tests := []struct {
		level string
		want  string
	}{
		{"debug", "DEBUG"},
		{"info", "INFO"},
		{"warn", "WARN"},
		{"warning", "WARN"},
		{"error", "ERROR"},
		{"unknown", "INFO"},
	}

	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			cfg := &Config{LogLevel: tt.level}
			got := cfg.SlogLevel().String()
			if got != tt.want {
				t.Errorf("SlogLevel() for %q = %q, want %q", tt.level, got, tt.want)
			}
		})
	}
}
