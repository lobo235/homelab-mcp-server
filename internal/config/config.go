package config

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
)

// Config holds all runtime configuration loaded from environment variables.
type Config struct {
	// Gateway connections (URL + API key for each)
	NomadGatewayURL      string
	NomadGatewayKey      string
	AdguardGatewayURL    string
	AdguardGatewayKey    string
	CFGatewayURL         string
	CFGatewayKey         string
	MinecraftGatewayURL  string
	MinecraftGatewayKey  string
	FilesystemGatewayURL string
	FilesystemGatewayKey string
	CurseforgeGatewayURL string
	CurseforgeGatewayKey string
	VaultGatewayURL      string
	VaultGatewayKey      string

	// Logging
	LogLevel string

	// Nomad defaults
	NomadDefaultDatacenter string
	NomadDefaultNodePool   string

	// Infrastructure
	NFSBasePath    string
	MCPublicDomain string
	MCPublicIP     string
	CFZoneName     string

	// Security
	ArtifactAllowlist []string
	VolumeAllowlist   []string

	// Data
	DataDir                 string
	ItzgDocsRefreshInterval time.Duration

	// Discovery pipeline
	AnthropicAPIKey  string
	DiscoveryTempDir string
}

// SlogLevel converts the LogLevel string to a slog.Level.
func (c *Config) SlogLevel() slog.Level {
	switch strings.ToLower(c.LogLevel) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Load reads configuration from environment variables.
// Returns an error if any required variable is missing or any value is invalid.
func Load() (*Config, error) {
	cfg := &Config{
		NomadGatewayURL:      os.Getenv("NOMAD_GATEWAY_URL"),
		NomadGatewayKey:      os.Getenv("NOMAD_GATEWAY_KEY"),
		AdguardGatewayURL:    os.Getenv("ADGUARD_GATEWAY_URL"),
		AdguardGatewayKey:    os.Getenv("ADGUARD_GATEWAY_KEY"),
		CFGatewayURL:         os.Getenv("CF_GATEWAY_URL"),
		CFGatewayKey:         os.Getenv("CF_GATEWAY_KEY"),
		MinecraftGatewayURL:  os.Getenv("MINECRAFT_GATEWAY_URL"),
		MinecraftGatewayKey:  os.Getenv("MINECRAFT_GATEWAY_KEY"),
		FilesystemGatewayURL: os.Getenv("FILESYSTEM_GATEWAY_URL"),
		FilesystemGatewayKey: os.Getenv("FILESYSTEM_GATEWAY_KEY"),
		CurseforgeGatewayURL: os.Getenv("CURSEFORGE_GATEWAY_URL"),
		CurseforgeGatewayKey: os.Getenv("CURSEFORGE_GATEWAY_KEY"),
		VaultGatewayURL:      os.Getenv("VAULT_GATEWAY_URL"),
		VaultGatewayKey:      os.Getenv("VAULT_GATEWAY_KEY"),

		LogLevel: os.Getenv("LOG_LEVEL"),

		NomadDefaultDatacenter: os.Getenv("NOMAD_DEFAULT_DATACENTER"),
		NomadDefaultNodePool:   os.Getenv("NOMAD_DEFAULT_NODE_POOL"),

		NFSBasePath:    os.Getenv("NFS_BASE_PATH"),
		MCPublicDomain: os.Getenv("MC_PUBLIC_DOMAIN"),
		MCPublicIP:     os.Getenv("MC_PUBLIC_IP"),
		CFZoneName:     os.Getenv("CF_ZONE_NAME"),

		DataDir: os.Getenv("DATA_DIR"),
	}

	// Parse artifact allowlist.
	if v := os.Getenv("ARTIFACT_ALLOWLIST"); v != "" {
		for _, s := range strings.Split(v, ",") {
			s = strings.TrimSpace(s)
			if s != "" {
				cfg.ArtifactAllowlist = append(cfg.ArtifactAllowlist, s)
			}
		}
	}

	// Parse volume allowlist — always includes NFS_BASE_PATH.
	cfg.VolumeAllowlist = []string{cfg.NFSBasePath}
	if v := os.Getenv("VOLUME_ALLOWLIST"); v != "" {
		for _, s := range strings.Split(v, ",") {
			s = strings.TrimSpace(s)
			if s != "" {
				cfg.VolumeAllowlist = append(cfg.VolumeAllowlist, s)
			}
		}
	}

	// Parse itzg docs refresh interval.
	if v := os.Getenv("ITZG_DOCS_REFRESH_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("ITZG_DOCS_REFRESH_INTERVAL: invalid duration %q: %w", v, err)
		}
		cfg.ItzgDocsRefreshInterval = d
	} else {
		cfg.ItzgDocsRefreshInterval = 24 * time.Hour
	}

	// Validate required fields.
	required := map[string]string{
		"NOMAD_GATEWAY_URL":        cfg.NomadGatewayURL,
		"NOMAD_GATEWAY_KEY":        cfg.NomadGatewayKey,
		"ADGUARD_GATEWAY_URL":      cfg.AdguardGatewayURL,
		"ADGUARD_GATEWAY_KEY":      cfg.AdguardGatewayKey,
		"CF_GATEWAY_URL":           cfg.CFGatewayURL,
		"CF_GATEWAY_KEY":           cfg.CFGatewayKey,
		"MINECRAFT_GATEWAY_URL":    cfg.MinecraftGatewayURL,
		"MINECRAFT_GATEWAY_KEY":    cfg.MinecraftGatewayKey,
		"FILESYSTEM_GATEWAY_URL":   cfg.FilesystemGatewayURL,
		"FILESYSTEM_GATEWAY_KEY":   cfg.FilesystemGatewayKey,
		"CURSEFORGE_GATEWAY_URL":   cfg.CurseforgeGatewayURL,
		"CURSEFORGE_GATEWAY_KEY":   cfg.CurseforgeGatewayKey,
		"VAULT_GATEWAY_URL":        cfg.VaultGatewayURL,
		"VAULT_GATEWAY_KEY":        cfg.VaultGatewayKey,
		"NOMAD_DEFAULT_DATACENTER": cfg.NomadDefaultDatacenter,
		"NFS_BASE_PATH":            cfg.NFSBasePath,
		"MC_PUBLIC_DOMAIN":         cfg.MCPublicDomain,
		"CF_ZONE_NAME":             cfg.CFZoneName,
	}
	for name, val := range required {
		if val == "" {
			return nil, fmt.Errorf("%s is required", name)
		}
	}

	// Defaults.
	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}
	switch strings.ToLower(cfg.LogLevel) {
	case "debug", "info", "warn", "warning", "error":
		// valid
	default:
		return nil, fmt.Errorf("LOG_LEVEL must be debug, info, warn, or error, got %q", cfg.LogLevel)
	}

	if cfg.NomadDefaultNodePool == "" {
		cfg.NomadDefaultNodePool = "default"
	}

	if cfg.DataDir == "" {
		cfg.DataDir = "/data"
	}

	// Discovery pipeline (optional).
	cfg.AnthropicAPIKey = os.Getenv("ANTHROPIC_API_KEY")
	cfg.DiscoveryTempDir = os.Getenv("DISCOVERY_TEMP_DIR")
	if cfg.DiscoveryTempDir == "" {
		cfg.DiscoveryTempDir = "/tmp/modpack-discovery"
	}

	return cfg, nil
}
