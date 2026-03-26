package discovery

import "testing"

func TestEstimateResources(t *testing.T) {
	tests := []struct {
		name       string
		modCount   int
		heavyMods  []string
		hasShaders bool
		wantInit   string
		wantMax    string
		wantCPU    int
	}{
		{
			name:     "small pack under 50 mods",
			modCount: 30,
			wantInit: "2g",
			wantMax:  "4g",
			wantCPU:  3000,
		},
		{
			name:     "medium pack 50-150 mods",
			modCount: 100,
			wantInit: "3g",
			wantMax:  "6g",
			wantCPU:  3500,
		},
		{
			name:     "large pack 150-300 mods",
			modCount: 200,
			wantInit: "4g",
			wantMax:  "8g",
			wantCPU:  4000,
		},
		{
			name:     "huge pack 300+ mods",
			modCount: 400,
			wantInit: "5g",
			wantMax:  "10g",
			wantCPU:  4500,
		},
		{
			name:      "medium pack with 5+ heavy mods",
			modCount:  100,
			heavyMods: []string{"create", "mekanism", "ae2", "refined-storage", "botania"},
			wantInit:  "4g",
			wantMax:   "8g",
			wantCPU:   4000,
		},
		{
			name:      "medium pack with 2 heavy mods",
			modCount:  100,
			heavyMods: []string{"create", "mekanism"},
			wantInit:  "3g",
			wantMax:   "7g",
			wantCPU:   3500,
		},
		{
			name:       "small pack with shaders",
			modCount:   30,
			hasShaders: true,
			wantInit:   "2g",
			wantMax:    "5g",
			wantCPU:    3000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			initMem, maxMem, cpuMHz := estimateResources(tt.modCount, tt.heavyMods, tt.hasShaders)
			if initMem != tt.wantInit {
				t.Errorf("initMem = %q, want %q", initMem, tt.wantInit)
			}
			if maxMem != tt.wantMax {
				t.Errorf("maxMem = %q, want %q", maxMem, tt.wantMax)
			}
			if cpuMHz != tt.wantCPU {
				t.Errorf("cpuMHz = %d, want %d", cpuMHz, tt.wantCPU)
			}
		})
	}
}

func TestInferJavaVersion(t *testing.T) {
	tests := []struct {
		mcVersion  string
		wantJava   int
		wantDocker string
	}{
		{"1.21.1", 21, "java21"},
		{"1.21", 21, "java21"},
		{"1.20.4", 17, "java17"},
		{"1.20.1", 17, "java17"},
		{"1.19.2", 17, "java17"},
		{"1.18.2", 17, "java17"},
		{"1.17.1", 17, "java17"},
		{"1.16.5", 8, "java8"},
		{"1.12.2", 8, "java8"},
		{"1.7.10", 8, "java8"},
	}

	for _, tt := range tests {
		t.Run(tt.mcVersion, func(t *testing.T) {
			javaVersion, dockerTag := inferJavaVersion(tt.mcVersion)
			if javaVersion != tt.wantJava {
				t.Errorf("javaVersion = %d, want %d", javaVersion, tt.wantJava)
			}
			if dockerTag != tt.wantDocker {
				t.Errorf("dockerTag = %q, want %q", dockerTag, tt.wantDocker)
			}
		})
	}
}

func TestParseMCVersion(t *testing.T) {
	tests := []struct {
		version   string
		wantMajor int
		wantMinor int
	}{
		{"1.20.1", 1, 20},
		{"1.21", 1, 21},
		{"1.7.10", 1, 7},
		{"", 1, 0},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			major, minor := parseMCVersion(tt.version)
			if major != tt.wantMajor || minor != tt.wantMinor {
				t.Errorf("parseMCVersion(%q) = (%d, %d), want (%d, %d)", tt.version, major, minor, tt.wantMajor, tt.wantMinor)
			}
		})
	}
}

func TestParseMemoryGB(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"4g", 4},
		{"8G", 8},
		{"10g", 10},
		{" 6g ", 6},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseMemoryGB(tt.input)
			if got != tt.want {
				t.Errorf("parseMemoryGB(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestHalfMemory(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"4g", "2g"},
		{"8g", "4g"},
		{"10g", "5g"},
		{"1g", "1g"}, // Minimum 1g.
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := halfMemory(tt.input)
			if got != tt.want {
				t.Errorf("halfMemory(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
