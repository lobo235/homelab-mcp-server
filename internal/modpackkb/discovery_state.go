package modpackkb

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

// validSlugRe matches only safe slug characters (lowercase alphanum + hyphens).
var validSlugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*[a-z0-9]$|^[a-z0-9]$`)

// validateSlug checks that a slug is safe for use in file paths.
func validateSlug(slug string) error {
	if slug == "" {
		return fmt.Errorf("slug is required")
	}
	if len(slug) > 128 {
		return fmt.Errorf("slug too long: %d characters (max 128)", len(slug))
	}
	if !validSlugRe.MatchString(slug) {
		return fmt.Errorf("invalid slug %q: must contain only lowercase letters, numbers, and hyphens", slug)
	}
	return nil
}

// DiscoveryState tracks the progress of a modpack discovery pipeline.
type DiscoveryState struct {
	Slug             string   `json:"slug"`
	Status           string   `json:"status"` // queued|downloading|extracting|analyzing|api_enriching|web_searching|finalizing|ready|failed
	RequestedBy      string   `json:"requested_by"`
	RequestedVersion string   `json:"requested_version,omitempty"`
	StartedAt        string   `json:"started_at"`
	UpdatedAt        string   `json:"updated_at"`
	Error            string   `json:"error,omitempty"`
	StepsCompleted   []string `json:"steps_completed"`
	StepsRemaining   []string `json:"steps_remaining"`
}

// discoverySteps lists the stages of the discovery pipeline in order.
var discoverySteps = []string{"resolve", "download", "extract", "analyze", "api_enrich", "web_search", "finalize"}

// StartDiscovery creates a new discovery state file with status "queued".
// Returns an error if a discovery is already in progress for this slug.
func (kb *KB) StartDiscovery(slug, requestedBy, requestedVersion string) (*DiscoveryState, error) {
	if err := validateSlug(slug); err != nil {
		return nil, err
	}

	kb.mu.Lock()
	defer kb.mu.Unlock()

	if kb.isDiscoveryInProgressUnlocked(slug) {
		return nil, fmt.Errorf("discovery already in progress for %q", slug)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	remaining := make([]string, len(discoverySteps))
	copy(remaining, discoverySteps)

	state := &DiscoveryState{
		Slug:             slug,
		Status:           "queued",
		RequestedBy:      requestedBy,
		RequestedVersion: requestedVersion,
		StartedAt:        now,
		UpdatedAt:        now,
		StepsCompleted:   []string{},
		StepsRemaining:   remaining,
	}

	if err := kb.writeDiscoveryState(state); err != nil {
		return nil, err
	}
	return state, nil
}

// UpdateDiscoveryState updates the status of a discovery, moves a completed step,
// and optionally records an error message.
func (kb *KB) UpdateDiscoveryState(slug, status string, stepCompleted string, errMsg string) error {
	kb.mu.Lock()
	defer kb.mu.Unlock()

	state, err := kb.readDiscoveryStateUnlocked(slug)
	if err != nil {
		return fmt.Errorf("read discovery state for %q: %w", slug, err)
	}
	if state == nil {
		return fmt.Errorf("no discovery state for %q", slug)
	}

	state.Status = status
	state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	if errMsg != "" {
		state.Error = errMsg
	}

	if stepCompleted != "" {
		// Move step from remaining to completed.
		newRemaining := make([]string, 0, len(state.StepsRemaining))
		for _, s := range state.StepsRemaining {
			if s != stepCompleted {
				newRemaining = append(newRemaining, s)
			}
		}
		state.StepsRemaining = newRemaining
		state.StepsCompleted = append(state.StepsCompleted, stepCompleted)
	}

	return kb.writeDiscoveryState(state)
}

// GetDiscoveryState reads the discovery state for a slug.
// Returns nil, nil if no state file exists.
func (kb *KB) GetDiscoveryState(slug string) (*DiscoveryState, error) {
	if err := validateSlug(slug); err != nil {
		return nil, err
	}

	kb.mu.RLock()
	defer kb.mu.RUnlock()
	return kb.readDiscoveryStateUnlocked(slug)
}

// IsDiscoveryInProgress returns true if a discovery is active for the slug
// (status is not "ready" or "failed", and the state is less than 30 minutes old).
func (kb *KB) IsDiscoveryInProgress(slug string) bool {
	if validateSlug(slug) != nil {
		return false
	}

	kb.mu.RLock()
	defer kb.mu.RUnlock()
	return kb.isDiscoveryInProgressUnlocked(slug)
}

func (kb *KB) isDiscoveryInProgressUnlocked(slug string) bool {
	state, err := kb.readDiscoveryStateUnlocked(slug)
	if err != nil || state == nil {
		return false
	}
	if state.Status == "ready" || state.Status == "failed" {
		return false
	}
	updatedAt, err := time.Parse(time.RFC3339, state.UpdatedAt)
	if err != nil {
		return false
	}
	return time.Since(updatedAt) < 30*time.Minute
}

func (kb *KB) discoveryStatePath(slug string) string {
	return filepath.Join(kb.dir, ".discovery", slug+".state.json")
}

func (kb *KB) readDiscoveryStateUnlocked(slug string) (*DiscoveryState, error) {
	data, err := os.ReadFile(kb.discoveryStatePath(slug))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var state DiscoveryState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parse discovery state %q: %w", slug, err)
	}
	return &state, nil
}

func (kb *KB) writeDiscoveryState(state *DiscoveryState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal discovery state %q: %w", state.Slug, err)
	}
	if err := os.WriteFile(kb.discoveryStatePath(state.Slug), data, 0o644); err != nil {
		return fmt.Errorf("write discovery state %q: %w", state.Slug, err)
	}
	return nil
}
