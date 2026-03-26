package discovery

import (
	"context"
	"fmt"
	"strconv"
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
			for _, f := range files {
				if f.ServerPackFileID != 0 {
					data.HasServerPack = true
					data.ServerPackPattern = "curseforge_server_pack"
					break
				}
			}
		}
	}

	data.ConfidenceFlags = append(data.ConfidenceFlags, FlagAPIEnriched)
	return nil
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
