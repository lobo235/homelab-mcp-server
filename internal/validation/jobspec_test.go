package validation

import (
	"testing"
)

const testVolumePrefix = "/mnt/data/"

const validHCL = `
job "mc-test" {
  datacenters = ["dc1"]
  type = "service"

  group "server" {
    task "minecraft" {
      driver = "docker"

      config {
        image = "itzg/minecraft-server:latest"
        volumes = ["/mnt/data/minecraft/mc-test/data:/data"]
      }

      resources {
        cpu    = 4000
        memory = 4096
      }
    }
  }
}
`

func TestValidateJobSpec_Valid(t *testing.T) {
	err := ValidateJobSpec(validHCL, nil, testVolumePrefix)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateJobSpec_MissingJob(t *testing.T) {
	hcl := `
  group "server" {
    task "minecraft" {
      driver = "docker"
      resources { cpu = 1000; memory = 1024 }
    }
  }
`
	err := ValidateJobSpec(hcl, nil, testVolumePrefix)
	if err == nil {
		t.Fatal("expected error for missing job block")
	}
	ve, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *Error, got %T", err)
	}
	if len(ve.Errors) == 0 {
		t.Fatal("expected at least one error")
	}
}

func TestValidateJobSpec_NetworkModeHost(t *testing.T) {
	hcl := `
job "test-job" {
  group "server" {
    task "app" {
      driver = "docker"
      config {
        network_mode = "host"
      }
      resources { cpu = 1000; memory = 1024 }
    }
  }
}
`
	err := ValidateJobSpec(hcl, nil, testVolumePrefix)
	if err == nil {
		t.Fatal("expected error for network_mode = host")
	}
}

func TestValidateJobSpec_Privileged(t *testing.T) {
	hcl := `
job "test-job" {
  group "server" {
    task "app" {
      driver = "docker"
      config {
        privileged = true
      }
      resources { cpu = 1000; memory = 1024 }
    }
  }
}
`
	err := ValidateJobSpec(hcl, nil, testVolumePrefix)
	if err == nil {
		t.Fatal("expected error for privileged = true")
	}
}

func TestValidateJobSpec_BadVolumePath(t *testing.T) {
	hcl := `
job "test-job" {
  group "server" {
    task "app" {
      driver = "docker"
      config {
        volumes = ["/etc/secrets:/data"]
      }
      resources { cpu = 1000; memory = 1024 }
    }
  }
}
`
	err := ValidateJobSpec(hcl, nil, testVolumePrefix)
	if err == nil {
		t.Fatal("expected error for volume outside /mnt/data/")
	}
}

func TestValidateJobSpec_BadArtifactSource(t *testing.T) {
	hcl := `
job "test-job" {
  group "server" {
    task "app" {
      driver = "docker"
      artifact {
        source = "https://evil.com/malware.sh"
      }
      resources { cpu = 1000; memory = 1024 }
    }
  }
}
`
	err := ValidateJobSpec(hcl, nil, testVolumePrefix)
	if err == nil {
		t.Fatal("expected error for disallowed artifact source")
	}
}

func TestValidateJobSpec_AllowedArtifactSource(t *testing.T) {
	hcl := `
job "test-job" {
  group "server" {
    task "app" {
      driver = "docker"
      artifact {
        source = "https://raw.githubusercontent.com/lobo235/nomad-jobs/main/script.sh"
      }
      config {
        volumes = ["/mnt/data/minecraft/test/data:/data"]
      }
      resources { cpu = 1000; memory = 1024 }
    }
  }
}
`
	err := ValidateJobSpec(hcl, nil, testVolumePrefix)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateJobSpec_ExtraAllowlist(t *testing.T) {
	hcl := `
job "test-job" {
  group "server" {
    task "app" {
      driver = "docker"
      artifact {
        source = "https://custom.example.com/file.tar.gz"
      }
      config {
        volumes = ["/mnt/data/data/test:/data"]
      }
      resources { cpu = 1000; memory = 1024 }
    }
  }
}
`
	err := ValidateJobSpec(hcl, []string{"custom.example.com/"}, testVolumePrefix)
	if err != nil {
		t.Fatalf("unexpected error with extra allowlist: %v", err)
	}
}

func TestValidateJobSpec_BadJobName(t *testing.T) {
	hcl := `
job "BAD_NAME!" {
  group "server" {
    task "app" {
      driver = "docker"
      config {
        volumes = ["/mnt/data/data/test:/data"]
      }
      resources { cpu = 1000; memory = 1024 }
    }
  }
}
`
	err := ValidateJobSpec(hcl, nil, testVolumePrefix)
	if err == nil {
		t.Fatal("expected error for bad job name")
	}
}

func TestValidateJobSpec_CpuTooLow(t *testing.T) {
	hcl := `
job "test-job" {
  group "server" {
    task "app" {
      driver = "docker"
      config {
        volumes = ["/mnt/data/data/test:/data"]
      }
      resources {
        cpu    = 100
        memory = 1024
      }
    }
  }
}
`
	err := ValidateJobSpec(hcl, nil, testVolumePrefix)
	if err == nil {
		t.Fatal("expected error for cpu < 500")
	}
}

func TestValidateJobSpec_CpuZeroAllowed(t *testing.T) {
	hcl := `
job "test-job" {
  group "server" {
    task "app" {
      driver = "docker"
      config {
        volumes = ["/mnt/data/data/test:/data"]
      }
      resources {
        cpu    = 0
        memory = 1024
      }
    }
  }
}
`
	err := ValidateJobSpec(hcl, nil, testVolumePrefix)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateJobSpec_MemoryTooLow(t *testing.T) {
	hcl := `
job "test-job" {
  group "server" {
    task "app" {
      driver = "docker"
      config {
        volumes = ["/mnt/data/data/test:/data"]
      }
      resources {
        cpu    = 1000
        memory = 256
      }
    }
  }
}
`
	err := ValidateJobSpec(hcl, nil, testVolumePrefix)
	if err == nil {
		t.Fatal("expected error for memory < 512")
	}
}

func TestValidateJobSpec_MemoryTooHigh(t *testing.T) {
	hcl := `
job "test-job" {
  group "server" {
    task "app" {
      driver = "docker"
      config {
        volumes = ["/mnt/data/data/test:/data"]
      }
      resources {
        cpu    = 1000
        memory = 65536
      }
    }
  }
}
`
	err := ValidateJobSpec(hcl, nil, testVolumePrefix)
	if err == nil {
		t.Fatal("expected error for memory > 32768")
	}
}

func TestValidateServerName(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{"mc-atm10", false},
		{"a", false},              // single char ok for server names
		{"mc-test-server", false}, // valid
		{"ABC", true},             // uppercase not allowed
		{"-start", true},          // can't start with hyphen
		{"a/../etc", true},        // path traversal rejected
		{"mc_test", true},         // underscore not allowed
		{"", true},                // empty
		{"valid-name-123", false}, // valid
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateServerName(tt.name)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateServerName(%q) error = %v, wantErr %v", tt.name, err, tt.wantErr)
			}
		})
	}
}

func TestValidateAllocID(t *testing.T) {
	tests := []struct {
		id      string
		wantErr bool
	}{
		{"a1b2c3d4-e5f6-7890-abcd-ef1234567890", false}, // valid UUID
		{"not-a-uuid", true},
		{"", true},
		{"../../../etc/passwd", true},
		{"A1B2C3D4-E5F6-7890-ABCD-EF1234567890", true}, // uppercase not allowed
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			err := ValidateAllocID(tt.id)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateAllocID(%q) error = %v, wantErr %v", tt.id, err, tt.wantErr)
			}
		})
	}
}

func TestValidateJobSpec_PrivilegedVarInterpolation(t *testing.T) {
	hcl := `
job "test-job" {
  group "server" {
    task "app" {
      driver = "docker"
      config {
        privileged = var.priv
        volumes = ["/mnt/data/data/test:/data"]
      }
      resources { cpu = 1000; memory = 1024 }
    }
  }
}
`
	err := ValidateJobSpec(hcl, nil, testVolumePrefix)
	if err == nil {
		t.Fatal("expected error for privileged = var.priv (variable interpolation)")
	}
}

func TestValidateJobSpec_NetworkModeVarInterpolation(t *testing.T) {
	hcl := `
job "test-job" {
  group "server" {
    task "app" {
      driver = "docker"
      config {
        network_mode = var.net_mode
        volumes = ["/mnt/data/data/test:/data"]
      }
      resources { cpu = 1000; memory = 1024 }
    }
  }
}
`
	err := ValidateJobSpec(hcl, nil, testVolumePrefix)
	if err == nil {
		t.Fatal("expected error for network_mode = var.net_mode (variable interpolation)")
	}
}

func TestValidateJobSpec_NetworkModeBridgeAllowed(t *testing.T) {
	hcl := `
job "test-job" {
  group "server" {
    task "app" {
      driver = "docker"
      config {
        network_mode = "bridge"
        volumes = ["/mnt/data/data/test:/data"]
      }
      resources { cpu = 1000; memory = 1024 }
    }
  }
}
`
	err := ValidateJobSpec(hcl, nil, testVolumePrefix)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMCServerDir(t *testing.T) {
	tests := []struct {
		jobID string
		want  string
	}{
		{"mc-atm9", "atm9"},
		{"mc-vanilla1", "vanilla1"},
		{"mc-atm10-kids", "atm10-kids"},
		{"nomad-job", "nomad-job"}, // no mc- prefix, unchanged
		{"minecraft", "minecraft"}, // no prefix
		{"mc-a", "a"},              // minimal
		{"mc-", ""},                // edge case: just "mc-"
	}
	for _, tt := range tests {
		t.Run(tt.jobID, func(t *testing.T) {
			got := MCServerDir(tt.jobID)
			if got != tt.want {
				t.Errorf("MCServerDir(%q) = %q, want %q", tt.jobID, got, tt.want)
			}
		})
	}
}

func TestValidateJobName(t *testing.T) {
	tests := []struct {
		name  string
		valid bool
	}{
		{"mc-atm10", true},
		{"mc-vanilla1", true},
		{"ab", false},      // too short (< 3 chars)
		{"a", false},       // too short
		{"ABC", false},     // uppercase not allowed
		{"mc_test", false}, // underscore not allowed
		{"-start", false},  // can't start with hyphen
		{"end-", false},    // can't end with hyphen
		{"mc-a", true},     // minimum valid (4 chars)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateJobName(tt.name)
			if got != tt.valid {
				t.Errorf("ValidateJobName(%q) = %v, want %v", tt.name, got, tt.valid)
			}
		})
	}
}
