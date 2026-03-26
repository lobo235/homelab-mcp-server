package discovery

import (
	"context"
	"fmt"
	"strconv"
	"sync"
)

// EnrichFromAPI adds metadata from platform APIs to the extracted data.
// This includes descriptions, categories, file details, and server pack file IDs.
func (p *Pipeline) EnrichFromAPI(ctx context.Context, resolved *ResolvedPack, data *ExtractedData) error {
	switch resolved.Platform {
	case "curseforge":
		return p.enrichFromCurseForge(ctx, resolved, data)
	case "modrinth":
		return p.enrichFromModrinth(ctx, resolved, data)
	case "ftb":
		// FTB data is already fully populated from the API in the analyzer stage.
		return nil
	default:
		return fmt.Errorf("unsupported platform for API enrichment: %q", resolved.Platform)
	}
}

// enrichFromCurseForge adds CurseForge-specific metadata.
func (p *Pipeline) enrichFromCurseForge(ctx context.Context, resolved *ResolvedPack, data *ExtractedData) error {
	if p.CurseForge == nil {
		return nil
	}

	projectID := strconv.Itoa(resolved.CurseForgeID)

	// Get modpack details for description.
	modpack, err := p.CurseForge.GetModpack(ctx, projectID)
	if err != nil {
		p.Log.Warn("failed to get curseforge modpack details", "id", projectID, "error", err)
	} else {
		if data.Name == "" {
			data.Name = modpack.Name
		}
	}

	// Check for server pack if not already found.
	if !data.HasServerPack && resolved.ServerPackFileID == 0 {
		files, err := p.CurseForge.GetModpackFiles(ctx, projectID)
		if err == nil {
			clientFile := findClientFileByURL(files, resolved.DownloadURL)
			if clientFile != nil {
				if sp := findServerPackFile(files, clientFile); sp != nil {
					data.HasServerPack = true
					data.ServerPackPattern = "curseforge_server_pack"
				}
			}
		}
	}

	// Enrich mod list: resolve filenames from CurseForge API.
	// This enables mod intelligence (client-only detection, heavy mods, etc.)
	p.enrichModFiles(ctx, data)

	data.ConfidenceFlags = append(data.ConfidenceFlags, FlagAPIEnriched)
	return nil
}

// enrichModFiles resolves FileName for mods that only have ProjectID/FileID.
// Uses concurrent API calls with a semaphore to limit parallelism.
func (p *Pipeline) enrichModFiles(ctx context.Context, data *ExtractedData) {
	if p.CurseForge == nil {
		return
	}

	// Count mods needing enrichment.
	var needsEnrich []int
	for i, m := range data.ModList {
		if m.FileName == "" && m.ProjectID > 0 && m.FileID > 0 {
			needsEnrich = append(needsEnrich, i)
		}
	}

	if len(needsEnrich) == 0 {
		return
	}

	// Cap enrichment to avoid excessive API calls.
	const maxEnrich = 500
	if len(needsEnrich) > maxEnrich {
		needsEnrich = needsEnrich[:maxEnrich]
	}

	p.Log.Info("enriching mod files from CurseForge API", "count", len(needsEnrich))

	// Concurrent enrichment with semaphore.
	const concurrency = 10
	sem := make(chan struct{}, concurrency)
	var mu sync.Mutex
	var enriched int
	var wg sync.WaitGroup

	for _, idx := range needsEnrich {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()

			m := data.ModList[i]
			file, err := p.CurseForge.GetModFile(ctx, strconv.Itoa(m.ProjectID), strconv.Itoa(m.FileID))
			if err != nil {
				return // Skip mods we can't resolve.
			}

			mu.Lock()
			defer mu.Unlock()

			if file.FileName != "" {
				data.ModList[i].FileName = file.FileName
				if data.ModList[i].Slug == "" {
					data.ModList[i].Slug = slugFromFilename(file.FileName)
				}
				if data.ModList[i].Name == "" {
					data.ModList[i].Name = file.DisplayName
				}
				enriched++
			}
		}(idx)
	}

	wg.Wait()
	p.Log.Info("mod file enrichment complete", "enriched", enriched, "total", len(needsEnrich))
}

// enrichFromModrinth adds Modrinth-specific metadata.
func (p *Pipeline) enrichFromModrinth(ctx context.Context, resolved *ResolvedPack, data *ExtractedData) error {
	if p.Modrinth == nil {
		return nil
	}

	project, err := p.Modrinth.GetProject(ctx, resolved.ModrinthID)
	if err != nil {
		p.Log.Warn("failed to get modrinth project details", "id", resolved.ModrinthID, "error", err)
		return nil
	}

	if data.Name == "" {
		data.Name = project.Title
	}

	data.ConfidenceFlags = append(data.ConfidenceFlags, FlagAPIEnriched)
	return nil
}
