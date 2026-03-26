package discovery

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/lobo235/homelab-mcp-server/internal/clients/curseforge"
	"github.com/lobo235/homelab-mcp-server/internal/clients/ftb"
	"github.com/lobo235/homelab-mcp-server/internal/clients/modrinth"
	"github.com/lobo235/homelab-mcp-server/internal/modpackkb"
)

// Resolve searches all three platforms in parallel and returns the best match.
// Prefer CurseForge > Modrinth > FTB when multiple matches are found.
func (p *Pipeline) Resolve(ctx context.Context, slug, packName, requestedVersion string) (*ResolvedPack, error) {
	searchName := packName
	if searchName == "" {
		// Convert slug back to a search-friendly name.
		searchName = strings.ReplaceAll(slug, "-", " ")
	}

	var (
		mu      sync.Mutex
		results []*ResolvedPack
		wg      sync.WaitGroup
	)

	// Search CurseForge.
	if p.CurseForge != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pack, err := p.resolveCurseForge(ctx, slug, searchName, requestedVersion)
			if err != nil {
				p.Log.Debug("curseforge resolve failed", "slug", slug, "error", err)
				return
			}
			mu.Lock()
			results = append(results, pack)
			mu.Unlock()
		}()
	}

	// Search Modrinth.
	if p.Modrinth != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pack, err := p.resolveModrinth(ctx, slug, searchName, requestedVersion)
			if err != nil {
				p.Log.Debug("modrinth resolve failed", "slug", slug, "error", err)
				return
			}
			mu.Lock()
			results = append(results, pack)
			mu.Unlock()
		}()
	}

	// Search FTB.
	if p.FTB != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pack, err := p.resolveFTB(ctx, slug, searchName, requestedVersion)
			if err != nil {
				p.Log.Debug("ftb resolve failed", "slug", slug, "error", err)
				return
			}
			mu.Lock()
			results = append(results, pack)
			mu.Unlock()
		}()
	}

	wg.Wait()

	if len(results) == 0 {
		return nil, fmt.Errorf("modpack %q not found on any platform", slug)
	}

	// Select best result: CurseForge > Modrinth > FTB.
	best := selectBest(results)

	// Cross-populate IDs from other results.
	for _, r := range results {
		if r.Platform != best.Platform {
			switch r.Platform {
			case "curseforge":
				best.CurseForgeID = r.CurseForgeID
			case "modrinth":
				best.ModrinthID = r.ModrinthID
			case "ftb":
				best.FTBID = r.FTBID
			}
		}
	}

	return best, nil
}

// selectBest picks the highest-priority platform result.
func selectBest(results []*ResolvedPack) *ResolvedPack {
	priority := map[string]int{"curseforge": 3, "modrinth": 2, "ftb": 1}
	var best *ResolvedPack
	bestPrio := 0
	for _, r := range results {
		p := priority[r.Platform]
		if p > bestPrio {
			bestPrio = p
			best = r
		}
	}
	return best
}

// resolveCurseForge searches CurseForge for the modpack and resolves version info.
func (p *Pipeline) resolveCurseForge(ctx context.Context, slug, searchName, requestedVersion string) (*ResolvedPack, error) {
	// Try slug-based search first.
	sr, err := p.CurseForge.SearchModpackBySlug(ctx, slug)
	if err != nil || sr == nil {
		// Fall back to name search.
		results, searchErr := p.CurseForge.SearchModpacks(ctx, searchName)
		if searchErr != nil {
			return nil, fmt.Errorf("search curseforge: %w", searchErr)
		}
		if len(results) == 0 {
			return nil, fmt.Errorf("no curseforge results for %q", searchName)
		}
		sr = &results[0]
	}

	projectID := strconv.Itoa(sr.ID)

	// Get files to find the right version.
	files, err := p.CurseForge.GetModpackFiles(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("get curseforge files: %w", err)
	}

	file := selectCurseForgeFile(files, requestedVersion)
	if file == nil {
		return nil, fmt.Errorf("no suitable file found for %q on curseforge", slug)
	}

	// Determine Minecraft version and modloader from game versions.
	mcVersion, modloaderType := parseCurseForgeGameVersions(file.GameVersions)

	rp := &ResolvedPack{
		Slug:             modpackkb.Slugify(sr.Name),
		Name:             sr.Name,
		Platform:         "curseforge",
		CurseForgeID:     sr.ID,
		PackVersion:      file.DisplayName,
		MinecraftVersion: mcVersion,
		ModloaderType:    modloaderType,
		DownloadURL:      file.DownloadURL,
		Format:           "curseforge_zip",
	}

	// Check for server pack: tagged via ServerPackFileID, IsServerPack flag, or filename heuristic.
	if sp := findServerPackFile(files, file); sp != nil {
		rp.ServerPackFileID = sp.ID
		if sp.DownloadURL != "" {
			rp.ServerPackURL = sp.DownloadURL
		} else {
			// File list may omit download URLs; fetch the full file details.
			fetched, fetchErr := p.CurseForge.GetModpackFile(ctx, projectID, strconv.Itoa(sp.ID))
			if fetchErr == nil && fetched != nil && fetched.DownloadURL != "" {
				rp.ServerPackURL = fetched.DownloadURL
			}
		}
	}

	return rp, nil
}

// selectCurseForgeFile picks the best file from a list. Prefers the requested version,
// then latest release (ReleaseType 1), then latest beta (2), then any.
func selectCurseForgeFile(files []curseforge.ModpackFile, requestedVersion string) *curseforge.ModpackFile {
	if len(files) == 0 {
		return nil
	}

	// If a specific version was requested, try to find it.
	if requestedVersion != "" {
		for i := range files {
			if files[i].DisplayName == requestedVersion || files[i].FileName == requestedVersion {
				return &files[i]
			}
		}
		// Try substring match.
		for i := range files {
			if strings.Contains(files[i].DisplayName, requestedVersion) {
				return &files[i]
			}
		}
	}

	// Pick latest release, then beta, then any (files are typically newest-first).
	var latestRelease, latestBeta, latest *curseforge.ModpackFile
	for i := range files {
		if files[i].IsServerPack {
			continue // Skip server packs, we want the main file.
		}
		if latest == nil {
			latest = &files[i]
		}
		switch files[i].ReleaseType {
		case 1: // Release
			if latestRelease == nil {
				latestRelease = &files[i]
			}
		case 2: // Beta
			if latestBeta == nil {
				latestBeta = &files[i]
			}
		}
	}

	if latestRelease != nil {
		return latestRelease
	}
	if latestBeta != nil {
		return latestBeta
	}
	return latest
}

// findServerPackFile finds the server pack file corresponding to the given client file.
// It checks three sources in priority order:
//  1. ServerPackFileID on the client file (authoritative when set)
//  2. IsServerPack flag on files sharing at least one game version
//  3. Filename containing "server" (case-insensitive) on version-matched files
//
// Returns nil if no server pack is found.
func findServerPackFile(files []curseforge.ModpackFile, clientFile *curseforge.ModpackFile) *curseforge.ModpackFile {
	// 1. Direct link via ServerPackFileID.
	if clientFile.ServerPackFileID != 0 {
		for i := range files {
			if files[i].ID == clientFile.ServerPackFileID {
				return &files[i]
			}
		}
	}

	// 2. IsServerPack flag with overlapping game versions.
	for i := range files {
		if files[i].ID == clientFile.ID {
			continue
		}
		if files[i].IsServerPack && sharesGameVersion(&files[i], clientFile) {
			return &files[i]
		}
	}

	// 3. Filename heuristic: "server" in filename with overlapping game versions.
	for i := range files {
		if files[i].ID == clientFile.ID {
			continue
		}
		if files[i].IsServerPack {
			continue // Already checked above.
		}
		if sharesGameVersion(&files[i], clientFile) && strings.Contains(strings.ToLower(files[i].FileName), "server") {
			return &files[i]
		}
	}

	return nil
}

// sharesGameVersion returns true if two files share at least one game version string.
func sharesGameVersion(a, b *curseforge.ModpackFile) bool {
	for _, va := range a.GameVersions {
		for _, vb := range b.GameVersions {
			if va == vb {
				return true
			}
		}
	}
	return false
}

// findClientFileByURL finds the file in a list matching the given download URL.
func findClientFileByURL(files []curseforge.ModpackFile, downloadURL string) *curseforge.ModpackFile {
	for i := range files {
		if files[i].DownloadURL == downloadURL {
			return &files[i]
		}
	}
	return nil
}

// parseCurseForgeGameVersions extracts Minecraft version and modloader type
// from CurseForge game version strings.
func parseCurseForgeGameVersions(versions []string) (mcVersion, modloaderType string) {
	for _, v := range versions {
		lower := strings.ToLower(v)
		switch {
		case lower == "forge":
			modloaderType = "forge"
		case lower == "neoforge":
			modloaderType = "neoforge"
		case lower == "fabric":
			modloaderType = "fabric"
		case lower == "quilt":
			modloaderType = "quilt"
		case strings.HasPrefix(v, "1.") && strings.Contains(v, "."):
			// Looks like a Minecraft version.
			if mcVersion == "" || len(v) > len(mcVersion) {
				mcVersion = v
			}
		}
	}
	return mcVersion, modloaderType
}

// resolveModrinth searches Modrinth for the modpack and resolves version info.
func (p *Pipeline) resolveModrinth(ctx context.Context, slug, searchName, requestedVersion string) (*ResolvedPack, error) {
	// Try direct slug/ID lookup first.
	project, err := p.Modrinth.GetProject(ctx, slug)
	if err != nil {
		// Fall back to search.
		results, searchErr := p.Modrinth.SearchProjects(ctx, searchName, "modpack")
		if searchErr != nil {
			return nil, fmt.Errorf("search modrinth: %w", searchErr)
		}
		if len(results) == 0 {
			return nil, fmt.Errorf("no modrinth results for %q", searchName)
		}
		project, err = p.Modrinth.GetProject(ctx, results[0].ID)
		if err != nil {
			return nil, fmt.Errorf("get modrinth project: %w", err)
		}
	}

	// Get versions.
	versions, err := p.Modrinth.GetVersions(ctx, project.ID)
	if err != nil {
		return nil, fmt.Errorf("get modrinth versions: %w", err)
	}

	version := selectModrinthVersion(versions, requestedVersion)
	if version == nil {
		return nil, fmt.Errorf("no suitable version found for %q on modrinth", slug)
	}

	// Extract modloader and MC version from the version data.
	mcVersion := ""
	modloaderType := ""
	if len(version.GameVersions) > 0 {
		mcVersion = version.GameVersions[0]
	}
	if len(version.Loaders) > 0 {
		modloaderType = version.Loaders[0]
	}

	// Find primary download file.
	downloadURL := ""
	for _, f := range version.Files {
		if f.Primary || downloadURL == "" {
			downloadURL = f.URL
		}
	}

	return &ResolvedPack{
		Slug:              modpackkb.Slugify(project.Title),
		Name:              project.Title,
		Platform:          "modrinth",
		ModrinthID:        project.ID,
		PackVersion:       version.VersionNumber,
		MinecraftVersion:  mcVersion,
		ModloaderType:     modloaderType,
		ModloaderVersion:  "", // Modrinth doesn't always separate modloader version.
		DownloadURL:       downloadURL,
		ModrinthVersionID: version.ID,
		Format:            "modrinth_mrpack",
	}, nil
}

// selectModrinthVersion picks the best version from a list.
func selectModrinthVersion(versions []modrinth.Version, requestedVersion string) *modrinth.Version {
	if len(versions) == 0 {
		return nil
	}

	if requestedVersion != "" {
		for i := range versions {
			if versions[i].VersionNumber == requestedVersion || versions[i].Name == requestedVersion {
				return &versions[i]
			}
		}
	}

	// Return first (latest) version.
	return &versions[0]
}

// resolveFTB searches FTB for the modpack and resolves version info.
func (p *Pipeline) resolveFTB(ctx context.Context, slug, searchName, requestedVersion string) (*ResolvedPack, error) {
	packs, err := p.FTB.SearchPacks(ctx, searchName)
	if err != nil {
		return nil, fmt.Errorf("search ftb: %w", err)
	}
	if len(packs) == 0 {
		return nil, fmt.Errorf("no ftb results for %q", searchName)
	}

	pack := &packs[0]

	// Get full pack details.
	fullPack, err := p.FTB.GetPack(ctx, pack.ID)
	if err != nil {
		return nil, fmt.Errorf("get ftb pack: %w", err)
	}

	// Select version.
	versionRef := selectFTBVersion(fullPack.Versions, requestedVersion)
	if versionRef == nil {
		return nil, fmt.Errorf("no suitable ftb version for %q", slug)
	}

	// Get full version details.
	version, err := p.FTB.GetVersion(ctx, pack.ID, versionRef.ID)
	if err != nil {
		return nil, fmt.Errorf("get ftb version: %w", err)
	}

	mcVersion, modloaderType, modloaderVersion := parseFTBTargets(version.Targets)

	return &ResolvedPack{
		Slug:             modpackkb.Slugify(fullPack.Name),
		Name:             fullPack.Name,
		Platform:         "ftb",
		FTBID:            fullPack.ID,
		PackVersion:      versionRef.Name,
		MinecraftVersion: mcVersion,
		ModloaderType:    modloaderType,
		ModloaderVersion: modloaderVersion,
		Format:           "ftb_pack",
	}, nil
}

// selectFTBVersion picks the best version from FTB pack version refs.
func selectFTBVersion(versions []ftb.PackVersionRef, requestedVersion string) *ftb.PackVersionRef {
	if len(versions) == 0 {
		return nil
	}

	if requestedVersion != "" {
		for i := range versions {
			if versions[i].Name == requestedVersion {
				return &versions[i]
			}
		}
	}

	// Return first (latest) version.
	return &versions[0]
}

// parseFTBTargets extracts Minecraft version and modloader info from FTB targets.
func parseFTBTargets(targets []ftb.Target) (mcVersion, modloaderType, modloaderVersion string) {
	for _, t := range targets {
		switch strings.ToLower(t.Type) {
		case "game":
			mcVersion = t.Version
		case "modloader":
			modloaderType = strings.ToLower(t.Name)
			modloaderVersion = t.Version
		}
	}
	return mcVersion, modloaderType, modloaderVersion
}
