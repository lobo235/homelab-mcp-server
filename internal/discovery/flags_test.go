package discovery

import (
	"testing"

	"github.com/lobo235/homelab-mcp-server/internal/modpackkb"
)

func TestAssignFlags(t *testing.T) {
	t.Run("all heuristic fallbacks", func(t *testing.T) {
		data := &ExtractedData{
			TotalModCount: 100,
		}
		flags := assignFlags(data, nil)

		assertContains(t, flags, FlagJVMArgsFromHeuristic)
		assertContains(t, flags, FlagMemoryFromHeuristic)
		assertContains(t, flags, FlagResourceSizingEstimated)
		assertContains(t, flags, FlagNoServerPackFound)
		assertContains(t, flags, FlagNoStartupScript)
		assertContains(t, flags, FlagJavaVersionInferred)
	})

	t.Run("with extracted data", func(t *testing.T) {
		data := &ExtractedData{
			JVMArgs:           "-XX:+UseG1GC",
			InitMemory:        "4g",
			MaxMemory:         "8g",
			HasServerPack:     true,
			StartupMethod:     "custom_script",
			StartupScriptName: "start.sh",
			JavaVersion:       17,
			ModloaderType:     "forge",
			ClientOnlyMods:    []string{"optifine"},
			ExtraPortMods:     []modpackkb.PortMapping{{Port: 24454}},
			HeavyMods:         []string{"create"},
			WorldGenMods:      []string{"biomes-o-plenty"},
		}
		flags := assignFlags(data, nil)

		assertNotContains(t, flags, FlagJVMArgsFromHeuristic)
		assertNotContains(t, flags, FlagMemoryFromHeuristic)
		assertNotContains(t, flags, FlagNoServerPackFound)
		assertNotContains(t, flags, FlagNoStartupScript)
		assertNotContains(t, flags, FlagJavaVersionInferred)
		assertContains(t, flags, FlagModloaderFromManifest)
		assertContains(t, flags, FlagClientOnlyModsDetected)
		assertContains(t, flags, FlagExtraPortsDetected)
		assertContains(t, flags, FlagHeavyModsDetected)
		assertContains(t, flags, FlagWorldGenModDetected)
	})

	t.Run("with web search results", func(t *testing.T) {
		data := &ExtractedData{}
		webResults := &WebSearchResults{
			RecommendedMemoryMB: 8192,
			Sources:             []string{"https://example.com"},
		}
		flags := assignFlags(data, webResults)

		assertContains(t, flags, FlagWebSearchUsed)
	})

	t.Run("deduplication", func(t *testing.T) {
		data := &ExtractedData{
			ConfidenceFlags: []string{FlagJVMArgsFromHeuristic}, // Already present.
		}
		flags := assignFlags(data, nil)

		count := 0
		for _, f := range flags {
			if f == FlagJVMArgsFromHeuristic {
				count++
			}
		}
		if count != 1 {
			t.Errorf("FlagJVMArgsFromHeuristic appears %d times, want 1", count)
		}
	})
}

func assertContains(t *testing.T, flags []string, flag string) {
	t.Helper()
	for _, f := range flags {
		if f == flag {
			return
		}
	}
	t.Errorf("flags %v does not contain %q", flags, flag)
}

func assertNotContains(t *testing.T, flags []string, flag string) {
	t.Helper()
	for _, f := range flags {
		if f == flag {
			t.Errorf("flags %v should not contain %q", flags, flag)
			return
		}
	}
}
