package discovery

import "testing"

func TestIsClientOnly(t *testing.T) {
	tests := []struct {
		slug string
		want bool
	}{
		{"optifine", true},
		{"sodium", true},
		{"journeymap", true},
		{"jei", true},
		{"just-enough-items", true},
		{"create", false},
		{"mekanism", false},
		{"unknown-mod", false},
		// Case insensitive.
		{"Optifine", true},
		{"SODIUM", true},
		// Underscore normalization.
		{"just_enough_items", true},
	}

	for _, tt := range tests {
		t.Run(tt.slug, func(t *testing.T) {
			got := IsClientOnly(tt.slug)
			if got != tt.want {
				t.Errorf("IsClientOnly(%q) = %v, want %v", tt.slug, got, tt.want)
			}
		})
	}
}

func TestIsClientOnlyFile(t *testing.T) {
	tests := []struct {
		filename string
		want     bool
	}{
		{"optifine-1.20.1.jar", true},
		{"sodium-fabric-0.5.3+mc1.20.1.jar", true},
		{"JourneyMap-1.20.1-5.9.7-fabric.jar", true},
		{"create-1.20.1-0.5.1.e.jar", false},
		{"mekanism-1.20.1-10.4.0.jar", false},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			got := IsClientOnlyFile(tt.filename)
			if got != tt.want {
				t.Errorf("IsClientOnlyFile(%q) = %v, want %v", tt.filename, got, tt.want)
			}
		})
	}
}

func TestGetExtraPorts(t *testing.T) {
	tests := []struct {
		slug     string
		wantNil  bool
		wantPort int
	}{
		{"simple-voice-chat", false, 24454},
		{"bluemap", false, 8100},
		{"dynmap", false, 8123},
		{"unknown-mod", true, 0},
		// Case insensitive.
		{"Simple-Voice-Chat", false, 24454},
		// Underscore normalization.
		{"simple_voice_chat", false, 24454},
	}

	for _, tt := range tests {
		t.Run(tt.slug, func(t *testing.T) {
			got := GetExtraPorts(tt.slug)
			if tt.wantNil {
				if got != nil {
					t.Errorf("GetExtraPorts(%q) = %+v, want nil", tt.slug, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("GetExtraPorts(%q) = nil, want port %d", tt.slug, tt.wantPort)
			}
			if got.Port != tt.wantPort {
				t.Errorf("GetExtraPorts(%q).Port = %d, want %d", tt.slug, got.Port, tt.wantPort)
			}
		})
	}
}

func TestGetExtraPortsByFilename(t *testing.T) {
	tests := []struct {
		filename string
		wantNil  bool
		wantPort int
	}{
		{"simple-voice-chat-fabric-1.20.1-2.4.24.jar", false, 24454},
		{"simple_voice_chat-1.0.jar", false, 24454},
		{"bluemap-3.14-fabric.jar", false, 8100},
		{"create-1.20.1-0.5.1.e.jar", true, 0},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			got := GetExtraPortsByFilename(tt.filename)
			if tt.wantNil {
				if got != nil {
					t.Errorf("GetExtraPortsByFilename(%q) = %+v, want nil", tt.filename, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("GetExtraPortsByFilename(%q) = nil, want port %d", tt.filename, tt.wantPort)
			}
			if got.Port != tt.wantPort {
				t.Errorf("GetExtraPortsByFilename(%q).Port = %d, want %d", tt.filename, got.Port, tt.wantPort)
			}
		})
	}
}

func TestIsHeavyMod(t *testing.T) {
	tests := []struct {
		slug string
		want bool
	}{
		{"create", true},
		{"mekanism", true},
		{"applied-energistics-2", true},
		{"optifine", false},
		{"unknown-mod", false},
	}

	for _, tt := range tests {
		t.Run(tt.slug, func(t *testing.T) {
			got := IsHeavyMod(tt.slug)
			if got != tt.want {
				t.Errorf("IsHeavyMod(%q) = %v, want %v", tt.slug, got, tt.want)
			}
		})
	}
}

func TestGetWorldGenInfo(t *testing.T) {
	tests := []struct {
		slug string
		want string
	}{
		{"biomes-o-plenty", "biomesoplenty"},
		{"terralith", "terralith"},
		{"create", ""},
		{"unknown-mod", ""},
	}

	for _, tt := range tests {
		t.Run(tt.slug, func(t *testing.T) {
			got := GetWorldGenInfo(tt.slug)
			if got != tt.want {
				t.Errorf("GetWorldGenInfo(%q) = %q, want %q", tt.slug, got, tt.want)
			}
		})
	}
}
