// Package validation provides pre-flight validation for Nomad job specs.
package validation

import (
	"fmt"
	"regexp"
	"strings"
)

// DefaultArtifactAllowlist contains trusted artifact source prefixes.
var DefaultArtifactAllowlist = []string{
	"raw.githubusercontent.com/lobo235/",
	"github.com/lobo235/",
}

// jobNamePattern validates Nomad job IDs.
var jobNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,48}[a-z0-9]$`)

// serverNamePattern validates server names used in URL paths.
// Must match vault-gateway's pattern: ^[a-z0-9][a-z0-9-]{0,47}$
var serverNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,47}$`)

// allocIDPattern validates Nomad allocation IDs — full UUIDs or short prefixes (min 8 hex chars).
var allocIDPattern = regexp.MustCompile(`^[0-9a-f]{8}[0-9a-f-]*$`)

// ValidateServerName checks if a server name is safe for use in URL paths.
func ValidateServerName(name string) error {
	if !serverNamePattern.MatchString(name) {
		return fmt.Errorf("invalid server name %q: must match ^[a-z0-9][a-z0-9-]{0,47}$", name)
	}
	return nil
}

// MCServerDir returns the NFS directory name for a Minecraft server job.
// Convention: job IDs are prefixed with "mc-" but filesystem and DNS use the
// bare name. For example, job "mc-atm9" uses directory "atm9" and hostname
// "atm9.<domain>".
func MCServerDir(jobID string) string {
	return strings.TrimPrefix(jobID, "mc-")
}

// ValidateAllocID checks if an allocation ID is a valid UUID or UUID prefix.
// Nomad supports prefix matching, so short IDs (min 8 hex chars) are accepted.
func ValidateAllocID(id string) error {
	if !allocIDPattern.MatchString(id) {
		return fmt.Errorf("invalid allocation ID %q: must be a UUID or hex prefix (min 8 chars)", id)
	}
	return nil
}

// Error holds all validation failures.
type Error struct {
	Errors []string
}

func (e *Error) Error() string {
	return fmt.Sprintf("job spec validation failed: %s", strings.Join(e.Errors, "; "))
}

// ValidateJobSpec performs pre-flight checks on a raw HCL job spec string.
// It checks for:
// 1. Required fields: job, group, task, driver = "docker", resources block
// 2. Security: no network_mode = "host", no privileged = true, volume mounts under allowed prefixes only, artifact allowlist
// 3. Naming: job ID matches ^[a-z0-9][a-z0-9-]{1,48}[a-z0-9]$
// 4. Resources: CPU >= 500 MHz (or 0), memory >= 512 MB and <= 32768 MB
// 5. Cluster: datacenter and node_pool must match expected values
func ValidateJobSpec(hcl string, extraAllowlist, volumePrefixes []string, expectedDatacenter, expectedNodePool string) error {
	var errs []string

	// Build full allowlist.
	allowlist := append([]string{}, DefaultArtifactAllowlist...)
	allowlist = append(allowlist, extraAllowlist...)

	// 1. Required fields.
	if !containsBlock(hcl, "job") {
		errs = append(errs, "missing required 'job' block")
	}
	if !containsBlock(hcl, "group") {
		errs = append(errs, "missing required 'group' block")
	}
	if !containsBlock(hcl, "task") {
		errs = append(errs, "missing required 'task' block")
	}
	if !containsField(hcl, "driver", "docker") {
		errs = append(errs, "missing or invalid 'driver = \"docker\"'")
	}
	if !containsBlock(hcl, "resources") {
		errs = append(errs, "missing required 'resources' block")
	}

	// 2. Security rules.
	// Reject network_mode unless it's a known-safe value.
	if nmVal := extractFieldValue(hcl, "network_mode"); nmVal != "" {
		if nmVal != "bridge" && nmVal != "none" {
			errs = append(errs, fmt.Sprintf("network_mode = %q is not allowed (must be \"bridge\" or \"none\")", nmVal))
		}
	}
	// Reject privileged unless explicitly false.
	if privVal := extractFieldValue(hcl, "privileged"); privVal != "" {
		if privVal != "false" {
			errs = append(errs, fmt.Sprintf("privileged = %q is not allowed (must be false or omitted)", privVal))
		}
	}

	// Check volume mounts.
	volumeErrs := validateVolumeMounts(hcl, volumePrefixes)
	errs = append(errs, volumeErrs...)

	// Check artifact sources.
	artifactErrs := validateArtifactSources(hcl, allowlist)
	errs = append(errs, artifactErrs...)

	// 3. Naming rules.
	jobName := extractJobName(hcl)
	if jobName != "" && !jobNamePattern.MatchString(jobName) {
		errs = append(errs, fmt.Sprintf("job ID %q does not match naming rules: must be 3-50 lowercase alphanumeric with hyphens", jobName))
	}

	// 4. Cluster validation.
	clusterErrs := validateCluster(hcl, expectedDatacenter, expectedNodePool)
	errs = append(errs, clusterErrs...)

	// 5. Resource sanity.
	resourceErrs := validateResources(hcl)
	errs = append(errs, resourceErrs...)

	if len(errs) > 0 {
		return &Error{Errors: errs}
	}
	return nil
}

// ValidateJobName checks if a job name is valid.
func ValidateJobName(name string) bool {
	return jobNamePattern.MatchString(name)
}

// containsBlock checks if the HCL contains a named block.
func containsBlock(hcl, blockName string) bool {
	// Match patterns like: blockName "name" { or blockName {
	pattern := regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(blockName) + `\s+`)
	return pattern.MatchString(hcl)
}

// containsField checks if the HCL contains a field with a specific value.
func containsField(hcl, field, value string) bool {
	// Match field = "value" or field = value (for booleans)
	pattern := regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(field) + `\s*=\s*"?` + regexp.QuoteMeta(value) + `"?`)
	return pattern.MatchString(hcl)
}

// fieldValueRe matches a field assignment and captures the raw value (quoted or unquoted).
var fieldValueRe = func(field string) *regexp.Regexp {
	return regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(field) + `\s*=\s*"?([^"\s]+)"?`)
}

// extractFieldValue returns the value of a field assignment, or "" if not found.
// Strips quotes so "host" returns "host" and true returns "true".
func extractFieldValue(hcl, field string) string {
	re := fieldValueRe(field)
	m := re.FindStringSubmatch(hcl)
	if len(m) >= 2 {
		return m[1]
	}
	return ""
}

var jobNameRe = regexp.MustCompile(`(?m)^\s*job\s+"([^"]+)"`)

// extractJobName pulls the job ID from a job "name" { block.
func extractJobName(hcl string) string {
	m := jobNameRe.FindStringSubmatch(hcl)
	if len(m) >= 2 {
		return m[1]
	}
	return ""
}

var volumeRe = regexp.MustCompile(`(?m)"(/[^"]+):`)

// validateVolumeMounts ensures all volume mount sources are under one of the allowed prefixes.
func validateVolumeMounts(hcl string, volumePrefixes []string) []string {
	var errs []string
	// Look for volumes = ["<host_path>:<container_path>"] patterns.
	matches := volumeRe.FindAllStringSubmatch(hcl, -1)
	for _, m := range matches {
		hostPath := m[1]
		allowed := false
		for _, prefix := range volumePrefixes {
			if strings.HasPrefix(hostPath, prefix) {
				allowed = true
				break
			}
		}
		if !allowed {
			errs = append(errs, fmt.Sprintf("volume mount %q is not under any allowed prefix %v", hostPath, volumePrefixes))
		}
	}
	return errs
}

var artifactSourceRe = regexp.MustCompile(`(?m)source\s*=\s*"(https?://[^"]+)"`)

// validateArtifactSources ensures artifact sources are from trusted domains.
func validateArtifactSources(hcl string, allowlist []string) []string {
	var errs []string
	matches := artifactSourceRe.FindAllStringSubmatch(hcl, -1)
	for _, m := range matches {
		source := m[1]
		// Strip protocol for comparison.
		stripped := strings.TrimPrefix(strings.TrimPrefix(source, "https://"), "http://")
		allowed := false
		for _, prefix := range allowlist {
			if strings.HasPrefix(stripped, prefix) {
				allowed = true
				break
			}
		}
		if !allowed {
			errs = append(errs, fmt.Sprintf("artifact source %q is not in the allowlist", source))
		}
	}
	return errs
}

var cpuRe = regexp.MustCompile(`(?m)^\s*cpu\s*=\s*(\d+)`)
var memoryRe = regexp.MustCompile(`(?m)^\s*memory\s*=\s*(\d+)`)

// validateResources checks CPU and memory constraints.
func validateResources(hcl string) []string {
	var errs []string

	// CPU: must be 0 or >= 500.
	cpuMatch := cpuRe.FindStringSubmatch(hcl)
	if len(cpuMatch) >= 2 {
		cpu := parseIntFromString(cpuMatch[1])
		if cpu != 0 && cpu < 500 {
			errs = append(errs, fmt.Sprintf("cpu = %d is below minimum (must be 0 or >= 500 MHz)", cpu))
		}
	}

	// Memory: must be >= 512 and <= 32768.
	memMatch := memoryRe.FindStringSubmatch(hcl)
	if len(memMatch) >= 2 {
		mem := parseIntFromString(memMatch[1])
		if mem < 512 {
			errs = append(errs, fmt.Sprintf("memory = %d is below minimum (must be >= 512 MB)", mem))
		}
		if mem > 32768 {
			errs = append(errs, fmt.Sprintf("memory = %d exceeds maximum (must be <= 32768 MB)", mem))
		}
	}

	return errs
}

func parseIntFromString(s string) int {
	n := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		}
	}
	return n
}

var dcArrayRe = regexp.MustCompile(`datacenters\s*=\s*\[\s*"([^"]+)"`)

// validateCluster checks datacenter and node_pool against expected values.
func validateCluster(hcl, expectedDatacenter, expectedNodePool string) []string {
	var errs []string
	if expectedDatacenter != "" {
		// datacenters is always an array: datacenters = ["dc1"]
		// Do NOT use extractFieldValue which would match the "[" bracket.
		var dcVal string
		if m := dcArrayRe.FindStringSubmatch(hcl); len(m) >= 2 {
			dcVal = m[1]
		}
		if dcVal != "" && dcVal != expectedDatacenter {
			errs = append(errs, fmt.Sprintf("datacenter %q does not match expected %q", dcVal, expectedDatacenter))
		}
	}
	if expectedNodePool != "" {
		npVal := extractFieldValue(hcl, "node_pool")
		if npVal != "" && npVal != expectedNodePool {
			errs = append(errs, fmt.Sprintf("node_pool %q does not match expected %q", npVal, expectedNodePool))
		}
	}
	return errs
}
