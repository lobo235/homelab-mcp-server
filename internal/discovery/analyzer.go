package discovery

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/lobo235/homelab-mcp-server/internal/modpackkb"
)

// manifestNotFoundError indicates the format-specific manifest wasn't found.
type manifestNotFoundError struct {
	format string
	dir    string
}

func (e *manifestNotFoundError) Error() string {
	return fmt.Sprintf("%s manifest not found in %s", e.format, e.dir)
}

// findFile searches for a file by name within root up to maxDepth levels deep.
func findFile(root, name string, maxDepth int) string {
	var found string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || found != "" {
			return filepath.SkipDir
		}
		// Check depth.
		rel, _ := filepath.Rel(root, path)
		depth := strings.Count(rel, string(filepath.Separator))
		if depth > maxDepth {
			return filepath.SkipDir
		}
		if !d.IsDir() && d.Name() == name {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

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

	// If manifest not found, try server-pack-style directory analysis.
	var mnfErr *manifestNotFoundError
	if errors.As(err, &mnfErr) {
		p.Log.Info("manifest not found, attempting directory-based analysis", "format", resolved.Format, "dir", extractDir)
		data, err = analyzeServerPackDir(extractDir)
		if err != nil {
			return nil, fmt.Errorf("fallback analysis failed: %w", err)
		}
	} else if err != nil {
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
	// Clear previous results (allows safe re-run after enrichment).
	data.ClientOnlyMods = nil
	data.ExtraPortMods = nil
	data.HeavyMods = nil
	data.WorldGenMods = nil

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

// analyzeServerPackDir performs best-effort analysis when no manifest is found.
// This typically happens with server packs that contain raw server files.
func analyzeServerPackDir(extractDir string) (*ExtractedData, error) {
	data := &ExtractedData{}

	// Scan mods/ directory for jar files.
	modsDir := filepath.Join(extractDir, "mods")
	entries, err := os.ReadDir(modsDir)
	if err == nil {
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".jar") {
				slug := slugFromFilename(e.Name())
				data.ModList = append(data.ModList, ExtractedMod{
					FileName: e.Name(),
					Slug:     slug,
					Name:     slug,
				})
			}
		}
	}

	// Detect modloader from jar files at root.
	rootEntries, err := os.ReadDir(extractDir)
	if err == nil {
		for _, e := range rootEntries {
			name := strings.ToLower(e.Name())
			if strings.HasSuffix(name, ".jar") {
				switch {
				case strings.Contains(name, "forge") && !strings.Contains(name, "neoforge"):
					data.ModloaderType = "forge"
				case strings.Contains(name, "neoforge"):
					data.ModloaderType = "neoforge"
				case strings.Contains(name, "fabric"):
					data.ModloaderType = "fabric"
				}
			}
		}
	}

	data.ConfidenceFlags = append(data.ConfidenceFlags, "directory_scan_fallback")
	return data, nil
}
