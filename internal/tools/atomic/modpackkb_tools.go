package atomic

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/lobo235/homelab-mcp-server/internal/modpackkb"
)

func getModpackKnowledge(d *Deps) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("get_modpack_knowledge",
			mcp.WithDescription("Look up modpack deployment knowledge by name, slug, or CurseForge ID. Returns deployment details (modloader, Java version, memory, server pack info) learned from previous deployments."),
			mcp.WithString("query", mcp.Description("Modpack name or slug (e.g., 'ATM9', 'all-the-mods-10')")),
			mcp.WithNumber("curseforge_id", mcp.Description("CurseForge project ID (alternative to query)")),
		),
		Handler: func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if d.ModpackKB == nil {
				return mcp.NewToolResultError("modpack knowledge base not initialized"), nil
			}

			args := req.GetArguments()

			// Try CurseForge ID first.
			if cfIDRaw, ok := args["curseforge_id"]; ok {
				cfID, err := toInt(cfIDRaw)
				if err != nil {
					return mcp.NewToolResultError("invalid curseforge_id: " + err.Error()), nil
				}
				mk, err := d.ModpackKB.GetByCurseForgeID(cfID)
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				return mcp.NewToolResultJSON(mk)
			}

			// Try name/slug query.
			query, err := req.RequireString("query")
			if err != nil {
				return mcp.NewToolResultError("either query or curseforge_id is required"), nil
			}
			mk, err := d.ModpackKB.GetByName(query)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultJSON(mk)
		},
	}
}

func saveModpackKnowledge(d *Deps) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("save_modpack_knowledge",
			mcp.WithDescription("Save or update modpack deployment knowledge. Admin-only. Non-empty fields merge with existing data. Use this to record deployment details learned during server creation."),
			mcp.WithString("slug", mcp.Description("URL-safe slug (auto-generated from name if omitted)")),
			mcp.WithString("name", mcp.Description("Human-readable modpack name (e.g., 'All the Mods 9')")),
			mcp.WithNumber("curseforge_id", mcp.Description("CurseForge project ID")),
			mcp.WithString("modloader_type", mcp.Description("Modloader: forge, neoforge, fabric, quilt")),
			mcp.WithString("modloader_strategy", mcp.Description("How modloader is installed: bundled_in_server_pack, separate_install")),
			mcp.WithBoolean("has_server_pack", mcp.Description("Whether the modpack has a server pack on CurseForge")),
			mcp.WithString("server_pack_pattern", mcp.Description("Filename pattern for server pack archives")),
			mcp.WithString("minecraft_version", mcp.Description("Latest known Minecraft version")),
			mcp.WithNumber("recommended_cpu_mhz", mcp.Description("Recommended CPU in MHz")),
			mcp.WithNumber("recommended_memory_mb", mcp.Description("Recommended memory in MB")),
			mcp.WithString("init_memory", mcp.Description("JVM initial memory (e.g., '4G')")),
			mcp.WithString("max_memory", mcp.Description("JVM max memory (e.g., '8G')")),
			mcp.WithString("deployment_notes", mcp.Description("Free-text deployment notes")),
			mcp.WithString("source", mcp.Description("Knowledge source: manual, conversation, curseforge_api")),
			mcp.WithString("versions", mcp.Description("JSON array of version knowledge objects")),
		),
		Handler: func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if d.ModpackKB == nil {
				return mcp.NewToolResultError("modpack knowledge base not initialized"), nil
			}

			// Admin-only check.
			_, role, _ := extractUserContext(req)
			if role != "admin" && role != "" {
				return mcp.NewToolResultError("save_modpack_knowledge requires admin role"), nil
			}

			mk, err := parseModpackArgs(req)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			// Set updated_by from user context.
			if role == "admin" {
				mk.UpdatedBy = "admin"
			} else {
				mk.UpdatedBy = "claude"
			}

			if err := d.ModpackKB.Save(mk); err != nil {
				return mcp.NewToolResultError("save failed: " + err.Error()), nil
			}

			return mcp.NewToolResultText(fmt.Sprintf("Modpack knowledge saved for %q (slug: %s)", mk.Name, mk.Slug)), nil
		},
	}
}

func listModpackKnowledge(d *Deps) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("list_modpack_knowledge",
			mcp.WithDescription("List all known modpacks in the deployment knowledge base with key details."),
		),
		Handler: func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if d.ModpackKB == nil {
				return mcp.NewToolResultError("modpack knowledge base not initialized"), nil
			}

			entries, err := d.ModpackKB.List()
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			// Return compact summaries.
			type summary struct {
				Slug             string `json:"slug"`
				Name             string `json:"name"`
				CurseForgeID     int    `json:"curseforge_id,omitempty"`
				ModloaderType    string `json:"modloader_type"`
				MinecraftVersion string `json:"minecraft_version,omitempty"`
				HasServerPack    bool   `json:"has_server_pack"`
				VersionCount     int    `json:"version_count"`
			}

			summaries := make([]summary, 0, len(entries))
			for _, mk := range entries {
				summaries = append(summaries, summary{
					Slug:             mk.Slug,
					Name:             mk.Name,
					CurseForgeID:     mk.CurseForgeID,
					ModloaderType:    mk.ModloaderType,
					MinecraftVersion: mk.MinecraftVersion,
					HasServerPack:    mk.HasServerPack,
					VersionCount:     len(mk.Versions),
				})
			}

			return mcp.NewToolResultJSON(summaries)
		},
	}
}

func deleteModpackKnowledge(d *Deps) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("delete_modpack_knowledge",
			mcp.WithDescription("Delete a modpack from the deployment knowledge base. Admin-only."),
			mcp.WithString("slug", mcp.Required(), mcp.Description("Modpack slug to delete")),
		),
		Handler: func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if d.ModpackKB == nil {
				return mcp.NewToolResultError("modpack knowledge base not initialized"), nil
			}

			// Admin-only check.
			_, role, _ := extractUserContext(req)
			if role != "admin" && role != "" {
				return mcp.NewToolResultError("delete_modpack_knowledge requires admin role"), nil
			}

			slug, err := req.RequireString("slug")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			if err := d.ModpackKB.Delete(slug); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			return mcp.NewToolResultText(fmt.Sprintf("Modpack knowledge deleted for %q", slug)), nil
		},
	}
}

// parseModpackArgs extracts ModpackKnowledge fields from a tool request.
func parseModpackArgs(req mcp.CallToolRequest) (*modpackkb.ModpackKnowledge, error) {
	mk := &modpackkb.ModpackKnowledge{}
	args := req.GetArguments()

	// Extract string fields.
	mk.Slug, _ = getStr(args, "slug")
	mk.Name, _ = getStr(args, "name")
	mk.ModloaderType, _ = getStr(args, "modloader_type")
	mk.ModloaderStrategy, _ = getStr(args, "modloader_strategy")
	mk.ServerPackPattern, _ = getStr(args, "server_pack_pattern")
	mk.MinecraftVersion, _ = getStr(args, "minecraft_version")
	mk.InitMemory, _ = getStr(args, "init_memory")
	mk.MaxMemory, _ = getStr(args, "max_memory")
	mk.DeploymentNotes, _ = getStr(args, "deployment_notes")
	mk.Source, _ = getStr(args, "source")

	// Extract numeric fields.
	if v, ok := args["curseforge_id"]; ok {
		n, err := toInt(v)
		if err != nil {
			return nil, fmt.Errorf("invalid curseforge_id: %w", err)
		}
		mk.CurseForgeID = n
	}
	if v, ok := args["recommended_cpu_mhz"]; ok {
		n, err := toInt(v)
		if err != nil {
			return nil, fmt.Errorf("invalid recommended_cpu_mhz: %w", err)
		}
		mk.RecommendedCPU = n
	}
	if v, ok := args["recommended_memory_mb"]; ok {
		n, err := toInt(v)
		if err != nil {
			return nil, fmt.Errorf("invalid recommended_memory_mb: %w", err)
		}
		mk.RecommendedMemory = n
	}

	// Extract boolean fields.
	if v, ok := args["has_server_pack"]; ok {
		if b, isBool := v.(bool); isBool {
			mk.HasServerPack = b
		}
	}

	// Extract versions JSON string.
	if versionsStr, ok := getStr(args, "versions"); ok && versionsStr != "" {
		var versions []modpackkb.VersionKnowledge
		if err := json.Unmarshal([]byte(versionsStr), &versions); err != nil {
			return nil, fmt.Errorf("invalid versions JSON: %w", err)
		}
		mk.Versions = versions
	}

	return mk, nil
}

// getStr extracts a string value from a map if present.
func getStr(args map[string]any, key string) (string, bool) {
	v, ok := args[key]
	if !ok {
		return "", false
	}
	s, isStr := v.(string)
	if !isStr {
		return "", false
	}
	return s, true
}

// toInt converts a JSON number (float64) or string to int.
func toInt(v any) (int, error) {
	switch n := v.(type) {
	case float64:
		return int(n), nil
	case int:
		return n, nil
	case string:
		return strconv.Atoi(n)
	default:
		return 0, fmt.Errorf("expected number, got %T", v)
	}
}

// toMap converts a struct to map[string]any via JSON round-trip.
func toMap(v any) map[string]any {
	data, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil
	}
	return m
}

// toMaps converts a slice of structs to []map[string]any via JSON round-trip.
func toMaps(v any) []map[string]any {
	data, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var m []map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil
	}
	return m
}

// enrichWithKB attaches deployment_knowledge to a CurseForge result map if KB has data.
func enrichWithKB(kb *modpackkb.KB, result map[string]any) map[string]any {
	if kb == nil {
		return result
	}

	// Try by CurseForge ID first.
	if idRaw, ok := result["id"]; ok {
		if id, err := toInt(idRaw); err == nil && id > 0 {
			if mk, err := kb.GetByCurseForgeID(id); err == nil {
				result["deployment_knowledge"] = mk
				return result
			}
		}
	}

	// Try by name.
	if name, ok := result["name"].(string); ok && name != "" {
		if mk, err := kb.GetByName(name); err == nil {
			result["deployment_knowledge"] = mk
			return result
		}
	}

	return result
}

// enrichResultsWithKB enriches a slice of CurseForge results with KB knowledge.
func enrichResultsWithKB(kb *modpackkb.KB, results []map[string]any) []map[string]any {
	if kb == nil {
		return results
	}
	for i := range results {
		results[i] = enrichWithKB(kb, results[i])
	}
	return results
}
