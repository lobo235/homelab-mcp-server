package discovery

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lobo235/homelab-mcp-server/internal/modpackkb"
)

// Analyze performs mechanical analysis on an extracted modpack.
// It delegates to a format-specific parser for manifest data, then runs
// common post-extraction analysis on the results.
func (p *Pipeline) Analyze(ctx context.Context, extractDir string, resolved *ResolvedPack) (*ExtractedData, error) {
	var data *ExtractedData
	var err error

	switch resolved.Format {
	case "curseforge_zip":
		data, err = parseCurseForgeManifest(extractDir)
	case "modrinth_mrpack":
		data, err = parseModrinthIndex(extractDir)
	case "ftb_pack":
		data, err = p.parseFTBPack(ctx, resolved)
	default:
		return nil, fmt.Errorf("unsupported format: %q", resolved.Format)
	}

	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", resolved.Format, err)
	}

	// Override with resolver data if the parser didn't find it.
	if data.MinecraftVersion == "" {
		data.MinecraftVersion = resolved.MinecraftVersion
	}
	if data.ModloaderType == "" {
		data.ModloaderType = resolved.ModloaderType
	}
	if data.ModloaderVersion == "" {
		data.ModloaderVersion = resolved.ModloaderVersion
	}
	if data.Name == "" {
		data.Name = resolved.Name
	}

	// Server pack detection.
	data.HasServerPack = resolved.ServerPackURL != "" || resolved.ServerPackFileID != 0

	// Common post-extraction analysis.
	if extractDir != "" {
		analyzeStartupScripts(extractDir, data)
		analyzeServerProperties(extractDir, data)
	}
	analyzeModIntelligence(data)

	// Java version inference.
	if data.JavaVersion == 0 && data.MinecraftVersion != "" {
		data.JavaVersion, data.ItzgDockerTag = inferJavaVersion(data.MinecraftVersion)
	}

	// Mod counts.
	data.TotalModCount = len(data.ModList)
	serverMods := 0
	for _, m := range data.ModList {
		slug := m.Slug
		if slug == "" {
			slug = modpackkb.Slugify(m.Name)
		}
		if !IsClientOnly(slug) && !IsClientOnlyFile(m.FileName) {
			serverMods++
		}
	}
	data.ServerModCount = serverMods

	return data, nil
}

// analyzeStartupScripts looks for startup scripts in the extracted pack.
func analyzeStartupScripts(extractDir string, data *ExtractedData) {
	scriptNames := []string{
		"start.sh", "start-server.sh", "run.sh", "ServerStart.sh",
		"startserver.sh", "launch.sh", "server-start.sh",
		"start.bat", "ServerStart.bat", "startserver.bat",
	}

	for _, name := range scriptNames {
		scriptPath := filepath.Join(extractDir, name)
		if _, err := os.Stat(scriptPath); err == nil {
			data.StartupMethod = "custom_script"
			data.StartupScriptName = name
			parseStartupScript(scriptPath, data)
			return
		}
	}

	// Check for forge/neoforge installer.
	entries, err := os.ReadDir(extractDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		lower := strings.ToLower(e.Name())
		if strings.Contains(lower, "forge") && strings.HasSuffix(lower, ".jar") && strings.Contains(lower, "installer") {
			data.StartupMethod = "bootstrap_installer"
			return
		}
	}
}

// sensitivePatterns lists keywords that indicate a line may contain secrets.
// Lines matching these patterns are skipped during startup script parsing.
var sensitivePatterns = []string{"PASSWORD", "TOKEN", "KEY", "SECRET", "RCON_PASS", "CREDENTIAL"}

// containsSensitive returns true if the line contains any sensitive keyword.
func containsSensitive(line string) bool {
	upper := strings.ToUpper(line)
	for _, pattern := range sensitivePatterns {
		if strings.Contains(upper, pattern) {
			return true
		}
	}
	return false
}

// parseStartupScript reads a startup script and extracts JVM arguments and memory settings.
// Lines containing sensitive keywords (passwords, tokens, keys) are skipped.
func parseStartupScript(path string, data *ExtractedData) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close() //nolint:errcheck // best-effort close in read-only context

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}

		// Security: skip lines that may contain secrets.
		if containsSensitive(line) {
			continue
		}

		// Look for JVM args.
		if strings.Contains(line, "-Xms") || strings.Contains(line, "-Xmx") {
			parseJVMLine(line, data)
		}

		// Look for GC strategy.
		if strings.Contains(line, "UseG1GC") {
			data.GCStrategy = "g1gc"
		} else if strings.Contains(line, "UseZGC") {
			data.GCStrategy = "zgc"
		} else if strings.Contains(line, "UseShenandoahGC") {
			data.GCStrategy = "shenandoah"
		}
	}
}

// parseJVMLine extracts memory settings from a JVM command line.
func parseJVMLine(line string, data *ExtractedData) {
	parts := strings.Fields(line)
	var jvmArgs []string

	for _, part := range parts {
		if strings.HasPrefix(part, "-Xms") {
			data.InitMemory = strings.TrimPrefix(part, "-Xms")
		} else if strings.HasPrefix(part, "-Xmx") {
			data.MaxMemory = strings.TrimPrefix(part, "-Xmx")
		} else if strings.HasPrefix(part, "-XX:") || strings.HasPrefix(part, "-D") {
			jvmArgs = append(jvmArgs, part)
		}
	}

	if len(jvmArgs) > 0 && data.JVMArgs == "" {
		data.JVMArgs = strings.Join(jvmArgs, " ")
	}
}

// analyzeServerProperties reads server.properties if present.
func analyzeServerProperties(extractDir string, data *ExtractedData) {
	propsPath := filepath.Join(extractDir, "server.properties")
	f, err := os.Open(propsPath)
	if err != nil {
		return
	}
	defer f.Close() //nolint:errcheck // best-effort close in read-only context

	data.ServerProperties = make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			data.ServerProperties[key] = value

			if key == "level-type" && value != "" {
				data.LevelType = value
			}
		}
	}
}

// analyzeModIntelligence scans the mod list and classifies mods.
func analyzeModIntelligence(data *ExtractedData) {
	for _, m := range data.ModList {
		slug := m.Slug
		if slug == "" {
			slug = modpackkb.Slugify(m.Name)
		}

		// Client-only detection.
		if IsClientOnly(slug) || IsClientOnlyFile(m.FileName) {
			data.ClientOnlyMods = append(data.ClientOnlyMods, slug)
		}

		// Extra ports.
		if pm := GetExtraPorts(slug); pm != nil {
			data.ExtraPortMods = append(data.ExtraPortMods, *pm)
		} else if pm := GetExtraPortsByFilename(m.FileName); pm != nil {
			data.ExtraPortMods = append(data.ExtraPortMods, *pm)
		}

		// Heavy mods.
		if IsHeavyMod(slug) {
			data.HeavyMods = append(data.HeavyMods, slug)
		}

		// World gen mods.
		if levelType := GetWorldGenInfo(slug); levelType != "" {
			data.WorldGenMods = append(data.WorldGenMods, slug)
			if data.LevelType == "" {
				data.LevelType = levelType
			}
		}
	}
}
