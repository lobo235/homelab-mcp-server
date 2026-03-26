// Package discovery implements the modpack discovery pipeline that automates
// downloading, extracting, analyzing, and enriching modpack data to create
// knowledge base entries.
package discovery

import (
	"strings"

	"github.com/lobo235/homelab-mcp-server/internal/modpackkb"
)

// clientOnlyMods is a set of mod slugs known to be client-only and safe to
// exclude from server deployments.
var clientOnlyMods = map[string]bool{
	"optifine":                     true,
	"sodium":                       true,
	"iris":                         true,
	"oculus":                       true,
	"rubidium":                     true,
	"embeddium":                    true,
	"just-enough-items":            true,
	"jei":                          true,
	"roughly-enough-items":         true,
	"rei":                          true,
	"emi":                          true,
	"controlling":                  true,
	"mouse-tweaks":                 true,
	"inventory-sorter":             true,
	"better-fps":                   true,
	"fps-reducer":                  true,
	"entityculling":                true,
	"entity-culling":               true,
	"sound-filters":                true,
	"sound-physics-remastered":     true,
	"dynamic-fps":                  true,
	"smooth-boot":                  true,
	"not-enough-animations":        true,
	"skin-layers-3d":               true,
	"ears":                         true,
	"first-person-model":           true,
	"better-third-person":          true,
	"torohealth-damage-indicators": true,
	"appleskin":                    true,
	"jade":                         true,
	"wthit":                        true,
	"hwyla":                        true,
	"waila":                        true,
	"the-one-probe":                true,
	"journeymap":                   true,
	"xaeros-minimap":               true,
	"xaeros-world-map":             true,
	"map-atlases":                  true,
	"shaders-mod":                  true,
	"continuity":                   true,
	"better-foliage":               true,
	"effective":                    true,
	"presence-footsteps":           true,
	"ambient-sounds":               true,
	"drip-sounds":                  true,
	"extreme-sound-muffler":        true,
	"crafting-tweaks":              true,
	"toast-control":                true,
	"tipthescales":                 true,
	"configured":                   true,
	"catalogue":                    true,
	"mod-menu":                     true,
	"loading-profiler":             true,
	"neat":                         true,
	"betterf3":                     true,
	"chunk-animator":               true,
	"fancymenu":                    true,
	"custom-main-menu":             true,
	"drippyloadingscreen":          true,
	"panorama-creator":             true,
	"screenshot-to-clipboard":      true,
	"screenshot-viewer":            true,
	"blur":                         true,
	"light-overlay":                true,
}

// clientOnlyPatterns are filename substrings that indicate client-only mods.
var clientOnlyPatterns = []string{
	"optifine", "sodium", "iris-", "oculus-", "rubidium", "embeddium",
	"jei-", "rei-", "emi-", "mouse-tweaks", "sound-filters",
	"dynamic-fps", "journeymap", "xaerosminimap", "xaerosworldmap",
	"shaders", "continuity", "presence-footsteps", "mod-menu",
}

// extraPortMods maps mod slugs to their required port mappings.
var extraPortMods = map[string]modpackkb.PortMapping{
	"simple-voice-chat": {Port: 24454, Protocol: "udp", Purpose: "Simple Voice Chat"},
	"plasmo-voice":      {Port: 25565, Protocol: "udp", Purpose: "Plasmo Voice (shares game port, UDP)"},
	"bluemap":           {Port: 8100, Protocol: "tcp", Purpose: "BlueMap web interface"},
	"dynmap":            {Port: 8123, Protocol: "tcp", Purpose: "Dynmap web interface"},
	"squaremap":         {Port: 8080, Protocol: "tcp", Purpose: "Squaremap web interface"},
	"opencomputers":     {Port: 8080, Protocol: "tcp", Purpose: "OpenComputers HTTP API"},
}

// heavyMods is a set of mod slugs known to be resource-intensive.
var heavyMods = map[string]bool{
	"create":                       true,
	"mekanism":                     true,
	"applied-energistics-2":        true,
	"ae2":                          true,
	"refined-storage":              true,
	"immersive-engineering":        true,
	"thermal-expansion":            true,
	"botania":                      true,
	"ender-io":                     true,
	"draconic-evolution":           true,
	"rftools":                      true,
	"industrialcraft":              true,
	"buildcraft":                   true,
	"gregtech":                     true,
	"nuclearcraft":                 true,
	"bigger-reactors":              true,
	"extreme-reactors":             true,
	"pneumaticcraft-repressurized": true,
	"computercraft":                true,
	"cc-tweaked":                   true,
	"mystical-agriculture":         true,
	"ftb-chunks":                   true,
	"ftb-quests":                   true,
	"the-twilight-forest":          true,
	"aether":                       true,
	"alexs-mobs":                   true,
	"ice-and-fire":                 true,
}

// worldGenMods maps mod slugs to the level-type they typically require.
var worldGenMods = map[string]string{
	"biomes-o-plenty":        "biomesoplenty",
	"oh-the-biomes-youll-go": "terralith",
	"terralith":              "terralith",
	"amplified-nether":       "default",
	"better-end":             "default",
	"the-twilight-forest":    "default",
	"aether":                 "default",
	"nullscape":              "default",
}

// IsClientOnly returns true if the mod slug is known to be client-only.
func IsClientOnly(modSlug string) bool {
	normalized := normalizeModSlug(modSlug)
	return clientOnlyMods[normalized]
}

// IsClientOnlyFile returns true if the filename matches a known client-only pattern.
func IsClientOnlyFile(filename string) bool {
	lower := strings.ToLower(filename)
	for _, pattern := range clientOnlyPatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}

// GetExtraPorts returns the port mapping for a mod if it requires additional ports.
func GetExtraPorts(modSlug string) *modpackkb.PortMapping {
	normalized := normalizeModSlug(modSlug)
	if pm, ok := extraPortMods[normalized]; ok {
		pm.ModSlug = normalized
		return &pm
	}
	return nil
}

// GetExtraPortsByFilename checks a filename against known extra-port mod patterns.
func GetExtraPortsByFilename(filename string) *modpackkb.PortMapping {
	lower := strings.ToLower(filename)
	for slug, pm := range extraPortMods {
		// Convert slug to filename-like pattern (replace hyphens with various separators)
		patterns := []string{slug, strings.ReplaceAll(slug, "-", "_"), strings.ReplaceAll(slug, "-", "")}
		for _, p := range patterns {
			if strings.Contains(lower, p) {
				pm.ModSlug = slug
				return &pm
			}
		}
	}
	return nil
}

// IsHeavyMod returns true if the mod slug is known to be resource-intensive.
func IsHeavyMod(modSlug string) bool {
	normalized := normalizeModSlug(modSlug)
	return heavyMods[normalized]
}

// GetWorldGenInfo returns the level_type string for a world-gen mod, or empty string.
func GetWorldGenInfo(modSlug string) string {
	normalized := normalizeModSlug(modSlug)
	return worldGenMods[normalized]
}

// normalizeModSlug converts a mod slug to a normalized form for lookups.
// Handles case-insensitive matching and common separator variations.
func normalizeModSlug(slug string) string {
	s := strings.ToLower(strings.TrimSpace(slug))
	s = strings.ReplaceAll(s, "_", "-")
	return s
}
