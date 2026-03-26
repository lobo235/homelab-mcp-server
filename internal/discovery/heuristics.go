package discovery

import (
	"fmt"
	"strconv"
	"strings"
)

// estimateResources returns resource sizing estimates based on mod count and mod types.
// Returns initMemory, maxMemory (as JVM flags like "4g"), and recommended CPU in MHz.
func estimateResources(modCount int, heavyMods []string, hasShaders bool) (initMem, maxMem string, cpuMHz int) {
	// Base tier by mod count.
	switch {
	case modCount < 50:
		maxMem = "4g"
		cpuMHz = 3000
	case modCount < 150:
		maxMem = "6g"
		cpuMHz = 3500
	case modCount < 300:
		maxMem = "8g"
		cpuMHz = 4000
	default:
		maxMem = "10g"
		cpuMHz = 4500
	}

	// Bump for heavy mods.
	if len(heavyMods) >= 5 {
		maxMem = bumpMemory(maxMem, "2g")
		cpuMHz += 500
	} else if len(heavyMods) >= 2 {
		maxMem = bumpMemory(maxMem, "1g")
	}

	// Shader bump (server-side shader packs are rare but some packs include them).
	if hasShaders {
		maxMem = bumpMemory(maxMem, "1g")
	}

	// InitMemory = half of MaxMemory.
	initMem = halfMemory(maxMem)

	return initMem, maxMem, cpuMHz
}

// inferJavaVersion returns the recommended Java version and itzg Docker tag
// based on the Minecraft version string.
func inferJavaVersion(mcVersion string) (int, string) {
	major, minor := parseMCVersion(mcVersion)

	switch {
	case major >= 1 && minor >= 21:
		// 1.21+ requires Java 21
		return 21, "java21"
	case major >= 1 && minor >= 20 && minor <= 20:
		// 1.20.x — Java 17 recommended, 21 works
		return 17, "java17"
	case major >= 1 && minor >= 18:
		// 1.18-1.19 — Java 17
		return 17, "java17"
	case major >= 1 && minor >= 17:
		// 1.17 — Java 16/17
		return 17, "java17"
	case major >= 1 && minor >= 12:
		// 1.12-1.16 — Java 8
		return 8, "java8"
	default:
		// Older versions — Java 8
		return 8, "java8"
	}
}

// parseMCVersion extracts major and minor version numbers from a Minecraft version string.
// For example, "1.20.1" returns (1, 20).
func parseMCVersion(version string) (major, minor int) {
	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		return 1, 0
	}
	major, _ = strconv.Atoi(parts[0])
	minor, _ = strconv.Atoi(parts[1])
	return major, minor
}

// parseMemoryGB parses a JVM memory string like "4g" or "8g" and returns the value in GB.
func parseMemoryGB(mem string) int {
	mem = strings.TrimSpace(strings.ToLower(mem))
	mem = strings.TrimSuffix(mem, "g")
	val, _ := strconv.Atoi(mem)
	return val
}

// bumpMemory adds additional memory to a base allocation.
func bumpMemory(base, add string) string {
	baseGB := parseMemoryGB(base)
	addGB := parseMemoryGB(add)
	return fmt.Sprintf("%dg", baseGB+addGB)
}

// halfMemory returns half of the given memory allocation.
func halfMemory(mem string) string {
	gb := parseMemoryGB(mem)
	half := gb / 2
	if half < 1 {
		half = 1
	}
	return fmt.Sprintf("%dg", half)
}
