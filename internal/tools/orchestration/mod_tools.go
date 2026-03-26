package orchestration

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	cfclient "github.com/lobo235/homelab-mcp-server/internal/clients/curseforge"
	"github.com/lobo235/homelab-mcp-server/internal/clients/minecraft"
	"github.com/lobo235/homelab-mcp-server/internal/tools/authz"
	"github.com/lobo235/homelab-mcp-server/internal/validation"
)

// knownModloaders lists the modloader names that may appear in a file's gameVersions array.
var knownModloaders = []string{"Forge", "NeoForge", "Fabric", "Quilt"}

// detectModloader scans a file's gameVersions for a known modloader name.
func detectModloader(gameVersions []string) string {
	for _, gv := range gameVersions {
		for _, ml := range knownModloaders {
			if strings.EqualFold(gv, ml) {
				return ml
			}
		}
	}
	return ""
}

// matchesGameVersion checks if a file's gameVersions contains the target MC version.
func matchesGameVersion(gameVersions []string, target string) bool {
	for _, gv := range gameVersions {
		if gv == target {
			return true
		}
	}
	return false
}

// depDownloadResult holds the result of starting a dependency download.
type depDownloadResult struct {
	ModID      int    `json:"modId"`
	ModName    string `json:"modName"`
	FileName   string `json:"fileName"`
	DownloadID string `json:"downloadId"`
}

// addModParams holds the validated parameters for the add_mod_to_server tool.
type addModParams struct {
	serverName   string
	modProjectID string
	fileID       string
	gameVersion  string
	dirName      string
}

func addModToServer(d *Deps) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("add_mod_to_server",
			mcp.WithDescription("Add a mod to a Minecraft server. Validates compatibility, resolves required dependencies, downloads the mod and its dependencies to the server's mods/ directory. Warns if the server needs a modloader change."),
			mcp.WithString("server_name", mcp.Required(), mcp.Description("Minecraft server name (e.g., mc-atm10)")),
			mcp.WithString("mod_project_id", mcp.Required(), mcp.Description("CurseForge mod project ID")),
			mcp.WithString("file_id", mcp.Description("Specific file/version ID. If omitted, uses the latest compatible file.")),
			mcp.WithString("game_version", mcp.Description("Minecraft version to match (e.g., 1.20.1). If omitted, auto-detects from server.")),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			params, err := validateAddModParams(req)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			result, err := d.executeAddMod(ctx, params)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultJSON(result)
		},
	}
}

// validateAddModParams extracts, validates, and returns all add_mod_to_server parameters.
func validateAddModParams(req mcp.CallToolRequest) (*addModParams, error) {
	_, role, ownedServers := authz.ExtractUserContext(req)
	serverName, err := req.RequireString("server_name")
	if err != nil {
		return nil, err
	}
	if err := validation.ValidateServerName(serverName); err != nil {
		return nil, err
	}
	if err := authz.RequireServerAccess(role, serverName, ownedServers); err != nil {
		return nil, err
	}
	modProjectID, err := req.RequireString("mod_project_id")
	if err != nil {
		return nil, err
	}
	return &addModParams{
		serverName:   serverName,
		modProjectID: modProjectID,
		fileID:       req.GetString("file_id", ""),
		gameVersion:  req.GetString("game_version", ""),
		dirName:      validation.MCServerDir(serverName),
	}, nil
}

// executeAddMod runs the full add-mod workflow: validate, resolve file, resolve deps, download.
func (d *Deps) executeAddMod(ctx context.Context, p *addModParams) (map[string]any, error) {
	// Validate the mod exists.
	d.Log.Info("add_mod: validate mod", "server", p.serverName, "mod_project_id", p.modProjectID)
	mod, err := d.Curseforge.GetMod(ctx, p.modProjectID)
	if err != nil {
		return nil, fmt.Errorf("mod validation failed for project %s: %w", p.modProjectID, err)
	}

	// Resolve the mod file.
	modFile, err := d.resolveModFile(ctx, p)
	if err != nil {
		return nil, err
	}

	// Detect modloader and update HCL if needed.
	var warnings []string
	modloader := detectModloader(modFile.GameVersions)
	modloaderUpdated := false
	if modloader != "" {
		updated, err := d.ensureModloader(ctx, p.serverName, modloader)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("Could not auto-update modloader to %s: %s. You may need to manually set TYPE=%s in the Nomad job spec.", modloader, err.Error(), strings.ToUpper(modloader)))
		} else if updated {
			modloaderUpdated = true
		}
	}
	if p.gameVersion != "" && !matchesGameVersion(modFile.GameVersions, p.gameVersion) {
		warnings = append(warnings, fmt.Sprintf("Selected file's game versions %v may not match requested version %s.", modFile.GameVersions, p.gameVersion))
	}

	// Resolve and download required dependencies.
	depDownloads, depWarnings := d.resolveAndDownloadDeps(ctx, modFile, p)
	warnings = append(warnings, depWarnings...)

	// Download the main mod.
	if modFile.DownloadURL == "" {
		return nil, fmt.Errorf("mod %q (project %s) file has no download URL — this mod may require manual download from CurseForge", mod.Name, p.modProjectID)
	}
	d.Log.Info("add_mod: downloading mod", "mod", mod.Name, "file", modFile.FileName)
	mainDL, dlErr := d.Minecraft.DownloadToServer(ctx, p.dirName, minecraft.DownloadRequest{
		URL: modFile.DownloadURL, DestPath: "mods", Mode: "overwrite", UID: 1001, GID: 1001,
	})
	if dlErr != nil {
		return nil, fmt.Errorf("failed to start download for mod %q: %w", mod.Name, dlErr)
	}

	// Build result.
	result := map[string]any{
		"mod_name":       mod.Name,
		"mod_project_id": p.modProjectID,
		"file_name":      modFile.FileName,
		"download_id":    mainDL.ID,
		"server":         p.serverName,
		"status":         "downloads_started",
		"note":           "Downloads are async. Use get_download_status to check progress.",
		"dep_count":      len(depDownloads),
	}
	if modloader != "" {
		result["modloader_required"] = modloader
		result["modloader_updated"] = modloaderUpdated
		if modloaderUpdated {
			result["restart_required"] = true
			result["note"] = "Downloads are async. The server's modloader was updated — it will restart automatically to apply the change. Use get_download_status to check download progress."
		}
	}
	if len(depDownloads) > 0 {
		result["dependencies"] = depDownloads
	}
	if len(warnings) > 0 {
		result["warnings"] = warnings
	}
	return result, nil
}

// resolveModFile finds the appropriate mod file — specific by ID or latest compatible.
func (d *Deps) resolveModFile(ctx context.Context, p *addModParams) (*cfclient.ModpackFile, error) {
	if p.fileID != "" {
		d.Log.Info("add_mod: get specific file", "mod", p.modProjectID, "file", p.fileID)
		f, err := d.Curseforge.GetModFile(ctx, p.modProjectID, p.fileID)
		if err != nil {
			return nil, fmt.Errorf("failed to get file %s for mod %s: %w", p.fileID, p.modProjectID, err)
		}
		return f, nil
	}

	d.Log.Info("add_mod: find latest compatible file", "mod", p.modProjectID, "game_version", p.gameVersion)
	files, err := d.Curseforge.GetModFiles(ctx, p.modProjectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get files for mod %s: %w", p.modProjectID, err)
	}
	best := findLatestCompatibleFile(files, p.gameVersion)
	if best == nil {
		msg := fmt.Sprintf("no compatible file found for mod project %s", p.modProjectID)
		if p.gameVersion != "" {
			msg += fmt.Sprintf(" matching game version %s", p.gameVersion)
		}
		return nil, fmt.Errorf("%s", msg)
	}
	return best, nil
}

// resolveAndDownloadDeps processes required dependencies (relationType == 3),
// starting async downloads for each. Returns download results and any warnings.
func (d *Deps) resolveAndDownloadDeps(ctx context.Context, modFile *cfclient.ModpackFile, p *addModParams) ([]depDownloadResult, []string) {
	var downloads []depDownloadResult
	var warnings []string

	for _, dep := range modFile.Dependencies {
		if dep.RelationType != 3 {
			continue
		}
		dl, warn := d.resolveOneDep(ctx, dep.ModID, p)
		if warn != "" {
			warnings = append(warnings, warn)
			continue
		}
		if dl != nil {
			downloads = append(downloads, *dl)
		}
	}
	return downloads, warnings
}

// resolveOneDep resolves and downloads a single dependency mod.
// Returns (result, "") on success or (nil, warning) on failure.
func (d *Deps) resolveOneDep(ctx context.Context, modID int, p *addModParams) (*depDownloadResult, string) {
	depProjectID := strconv.Itoa(modID)
	d.Log.Info("add_mod: resolving dependency", "dep_mod_id", depProjectID)

	depMod, err := d.Curseforge.GetMod(ctx, depProjectID)
	if err != nil {
		return nil, fmt.Sprintf("Could not validate dependency mod %s: %s", depProjectID, err.Error())
	}

	depFiles, err := d.Curseforge.GetModFiles(ctx, depProjectID)
	if err != nil {
		return nil, fmt.Sprintf("Could not get files for dependency %q (mod %s): %s", depMod.Name, depProjectID, err.Error())
	}

	depFile := findLatestCompatibleFile(depFiles, p.gameVersion)
	if depFile == nil {
		return nil, fmt.Sprintf("No compatible file found for dependency %q (mod %s)", depMod.Name, depProjectID)
	}
	if depFile.DownloadURL == "" {
		return nil, fmt.Sprintf("Dependency %q (mod %s) has no download URL — manual download may be required", depMod.Name, depProjectID)
	}

	d.Log.Info("add_mod: downloading dependency", "dep", depMod.Name, "file", depFile.FileName)
	dlResult, dlErr := d.Minecraft.DownloadToServer(ctx, p.dirName, minecraft.DownloadRequest{
		URL: depFile.DownloadURL, DestPath: "mods", Mode: "overwrite", UID: 1001, GID: 1001,
	})
	if dlErr != nil {
		return nil, fmt.Sprintf("Failed to start download for dependency %q: %s", depMod.Name, dlErr.Error())
	}

	return &depDownloadResult{
		ModID:      modID,
		ModName:    depMod.Name,
		FileName:   depFile.FileName,
		DownloadID: dlResult.ID,
	}, ""
}

// modloaderToItzgType maps modloader names to itzg/docker-minecraft-server TYPE values.
var modloaderToItzgType = map[string]string{
	"Forge":    "FORGE",
	"NeoForge": "NEOFORGE",
	"Fabric":   "FABRIC",
	"Quilt":    "QUILT",
}

// ensureModloader checks if the server's HCL has the right TYPE env var for the
// required modloader. If not, updates the HCL and resubmits. Returns true if updated.
func (d *Deps) ensureModloader(ctx context.Context, serverName, modloader string) (bool, error) {
	itzgType, ok := modloaderToItzgType[modloader]
	if !ok {
		return false, fmt.Errorf("unknown modloader %q", modloader)
	}

	// Fetch current HCL.
	hcl, err := d.Nomad.GetJobSpec(ctx, serverName)
	if err != nil {
		return false, fmt.Errorf("fetch job spec: %w", err)
	}

	// Check if TYPE is already set correctly.
	typeTarget := "TYPE"
	hasCorrectType := strings.Contains(hcl, fmt.Sprintf(`TYPE = "%s"`, itzgType)) ||
		strings.Contains(hcl, fmt.Sprintf(`TYPE = "%s"`, strings.ToLower(itzgType)))

	if hasCorrectType {
		d.Log.Info("add_mod: server already has correct modloader", "server", serverName, "type", itzgType)
		return false, nil
	}

	// Update HCL: replace TYPE = "VANILLA" or TYPE = "<other>" with the correct type.
	// Also handle case where TYPE doesn't exist — add it after EULA.
	updated := hcl
	if strings.Contains(hcl, `TYPE = "VANILLA"`) {
		updated = strings.Replace(hcl, `TYPE = "VANILLA"`, fmt.Sprintf(`%s = "%s"`, typeTarget, itzgType), 1)
	} else if strings.Contains(hcl, `TYPE = "`) {
		// Replace existing TYPE value.
		start := strings.Index(hcl, `TYPE = "`)
		if start >= 0 {
			end := strings.Index(hcl[start+8:], `"`)
			if end >= 0 {
				updated = hcl[:start] + fmt.Sprintf(`TYPE = "%s"`, itzgType) + hcl[start+8+end+1:]
			}
		}
	} else if strings.Contains(hcl, `EULA`) {
		// No TYPE env var at all — add after EULA line.
		updated = strings.Replace(hcl, `EULA        = "TRUE"`, fmt.Sprintf("EULA        = \"TRUE\"\n        %s        = \"%s\"", typeTarget, itzgType), 1)
	}

	if updated == hcl {
		return false, fmt.Errorf("could not find TYPE env var or EULA line to insert modloader")
	}

	// Resubmit updated HCL.
	d.Log.Info("add_mod: updating server modloader", "server", serverName, "type", itzgType)
	if _, err := d.Nomad.SubmitJob(ctx, updated); err != nil {
		return false, fmt.Errorf("submit updated job: %w", err)
	}

	return true, nil
}

// findLatestCompatibleFile picks the best file matching the game version.
// Preference order: release (1) > beta (2) > alpha (3).
// Files are returned from CurseForge roughly in newest-first order,
// so the first match at each release type is preferred.
func findLatestCompatibleFile(files []cfclient.ModpackFile, gameVersion string) *cfclient.ModpackFile {
	var best *cfclient.ModpackFile
	bestRelease := 4 // Lower is better: 1=release, 2=beta, 3=alpha.

	for i := range files {
		f := &files[i]
		if gameVersion != "" && !matchesGameVersion(f.GameVersions, gameVersion) {
			continue
		}
		if best == nil || f.ReleaseType < bestRelease {
			best = f
			bestRelease = f.ReleaseType
			if bestRelease == 1 {
				break // Can't do better than a release.
			}
		}
	}
	return best
}
