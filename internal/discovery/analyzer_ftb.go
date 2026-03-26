package discovery

import (
	"context"
	"fmt"
	"strings"

	"github.com/lobo235/homelab-mcp-server/internal/clients/ftb"
)

// parseFTBPack uses the FTB API to build extracted data from pack targets and files.
// Unlike CurseForge and Modrinth, FTB packs don't have a local archive to parse;
// instead we use the API response data directly.
func (p *Pipeline) parseFTBPack(ctx context.Context, resolved *ResolvedPack) (*ExtractedData, error) {
	if p.FTB == nil {
		return nil, fmt.Errorf("ftb client not available")
	}

	pack, err := p.FTB.GetPack(ctx, resolved.FTBID)
	if err != nil {
		return nil, fmt.Errorf("get ftb pack %d: %w", resolved.FTBID, err)
	}

	// Find the version that matches.
	var versionID int
	for _, v := range pack.Versions {
		if v.Name == resolved.PackVersion {
			versionID = v.ID
			break
		}
	}
	if versionID == 0 && len(pack.Versions) > 0 {
		versionID = pack.Versions[0].ID
	}

	version, err := p.FTB.GetVersion(ctx, resolved.FTBID, versionID)
	if err != nil {
		return nil, fmt.Errorf("get ftb version %d/%d: %w", resolved.FTBID, versionID, err)
	}

	data := &ExtractedData{
		Name: pack.Name,
	}

	// Parse targets for MC version and modloader.
	for _, t := range version.Targets {
		switch strings.ToLower(t.Type) {
		case "game":
			data.MinecraftVersion = t.Version
		case "modloader":
			data.ModloaderType = strings.ToLower(t.Name)
			data.ModloaderVersion = t.Version
		}
	}

	// Build mod list from files.
	for _, f := range version.Files {
		if !isModFile(f) {
			continue
		}
		mod := ExtractedMod{
			Name:     f.Name,
			FileName: f.Name,
			Slug:     slugFromFilename(f.Name),
		}
		data.ModList = append(data.ModList, mod)
	}

	// Track server-only vs client-only from FTB file metadata.
	for _, f := range version.Files {
		if f.ClientOnly {
			slug := slugFromFilename(f.Name)
			data.ClientOnlyMods = append(data.ClientOnlyMods, slug)
		}
	}

	data.ConfidenceFlags = append(data.ConfidenceFlags, FlagFTBFormatParsed)

	return data, nil
}

// isModFile checks whether an FTB file entry represents a mod (as opposed to a config or other file).
func isModFile(f ftb.File) bool {
	lower := strings.ToLower(f.Name)
	if strings.HasSuffix(lower, ".jar") {
		// Check if it's in a mods path.
		path := strings.ToLower(f.Path)
		return strings.Contains(path, "mods") || path == "" || path == "."
	}
	return false
}
