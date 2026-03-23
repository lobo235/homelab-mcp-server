package validation

import (
	"testing"
)

const validHCL = `
job "mc-test" {
  datacenters = ["dc1"]
  type = "service"

  group "server" {
    task "minecraft" {
      driver = "docker"

      config {
        image = "itzg/minecraft-server:latest"
        volumes = ["/mnt/fast/minecraft/mc-test/data:/data"]
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
	err := ValidateJobSpec(validHCL, nil)
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
	err := ValidateJobSpec(hcl, nil)
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
	err := ValidateJobSpec(hcl, nil)
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
	err := ValidateJobSpec(hcl, nil)
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
	err := ValidateJobSpec(hcl, nil)
	if err == nil {
		t.Fatal("expected error for volume outside /mnt/fast/")
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
	err := ValidateJobSpec(hcl, nil)
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
        volumes = ["/mnt/fast/minecraft/test/data:/data"]
      }
      resources { cpu = 1000; memory = 1024 }
    }
  }
}
`
	err := ValidateJobSpec(hcl, nil)
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
        volumes = ["/mnt/fast/data/test:/data"]
      }
      resources { cpu = 1000; memory = 1024 }
    }
  }
}
`
	err := ValidateJobSpec(hcl, []string{"custom.example.com/"})
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
        volumes = ["/mnt/fast/data/test:/data"]
      }
      resources { cpu = 1000; memory = 1024 }
    }
  }
}
`
	err := ValidateJobSpec(hcl, nil)
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
        volumes = ["/mnt/fast/data/test:/data"]
      }
      resources {
        cpu    = 100
        memory = 1024
      }
    }
  }
}
`
	err := ValidateJobSpec(hcl, nil)
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
        volumes = ["/mnt/fast/data/test:/data"]
      }
      resources {
        cpu    = 0
        memory = 1024
      }
    }
  }
}
`
	err := ValidateJobSpec(hcl, nil)
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
        volumes = ["/mnt/fast/data/test:/data"]
      }
      resources {
        cpu    = 1000
        memory = 256
      }
    }
  }
}
`
	err := ValidateJobSpec(hcl, nil)
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
        volumes = ["/mnt/fast/data/test:/data"]
      }
      resources {
        cpu    = 1000
        memory = 65536
      }
    }
  }
}
`
	err := ValidateJobSpec(hcl, nil)
	if err == nil {
		t.Fatal("expected error for memory > 32768")
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
