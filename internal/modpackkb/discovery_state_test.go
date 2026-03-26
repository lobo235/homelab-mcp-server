package modpackkb

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStartDiscovery(t *testing.T) {
	kb := testKB(t)

	state, err := kb.StartDiscovery("test-pack", "user1", "1.0.0")
	if err != nil {
		t.Fatalf("StartDiscovery: %v", err)
	}
	if state.Slug != "test-pack" {
		t.Errorf("Slug = %q, want %q", state.Slug, "test-pack")
	}
	if state.Status != "queued" {
		t.Errorf("Status = %q, want %q", state.Status, "queued")
	}
	if state.RequestedBy != "user1" {
		t.Errorf("RequestedBy = %q, want %q", state.RequestedBy, "user1")
	}
	if state.RequestedVersion != "1.0.0" {
		t.Errorf("RequestedVersion = %q, want %q", state.RequestedVersion, "1.0.0")
	}
	if state.StartedAt == "" {
		t.Error("StartedAt should be set")
	}
	if len(state.StepsRemaining) != 7 {
		t.Errorf("StepsRemaining count = %d, want 7", len(state.StepsRemaining))
	}
	if len(state.StepsCompleted) != 0 {
		t.Errorf("StepsCompleted count = %d, want 0", len(state.StepsCompleted))
	}

	// File should exist on disk.
	statePath := filepath.Join(kb.dir, ".discovery", "test-pack.state.json")
	if _, err := os.Stat(statePath); os.IsNotExist(err) {
		t.Error("state file should exist on disk")
	}
}

func TestStartDiscoveryAlreadyInProgress(t *testing.T) {
	kb := testKB(t)

	_, err := kb.StartDiscovery("test-pack", "user1", "")
	if err != nil {
		t.Fatalf("StartDiscovery: %v", err)
	}

	// Second call should fail.
	_, err = kb.StartDiscovery("test-pack", "user2", "")
	if err == nil {
		t.Fatal("expected error for duplicate in-progress discovery")
	}
}

func TestStartDiscoveryAllowsAfterFailed(t *testing.T) {
	kb := testKB(t)

	_, err := kb.StartDiscovery("test-pack", "user1", "")
	if err != nil {
		t.Fatalf("StartDiscovery: %v", err)
	}

	// Mark as failed.
	if err := kb.UpdateDiscoveryState("test-pack", "failed", "", "something broke"); err != nil {
		t.Fatalf("UpdateDiscoveryState: %v", err)
	}

	// Should now be able to start again.
	state, err := kb.StartDiscovery("test-pack", "user2", "2.0.0")
	if err != nil {
		t.Fatalf("StartDiscovery after failed: %v", err)
	}
	if state.RequestedBy != "user2" {
		t.Errorf("RequestedBy = %q, want %q", state.RequestedBy, "user2")
	}
}

func TestUpdateDiscoveryState(t *testing.T) {
	kb := testKB(t)

	_, err := kb.StartDiscovery("test-pack", "user1", "")
	if err != nil {
		t.Fatalf("StartDiscovery: %v", err)
	}

	// Complete the "resolve" step.
	if err := kb.UpdateDiscoveryState("test-pack", "downloading", "resolve", ""); err != nil {
		t.Fatalf("UpdateDiscoveryState: %v", err)
	}

	state, err := kb.GetDiscoveryState("test-pack")
	if err != nil {
		t.Fatalf("GetDiscoveryState: %v", err)
	}
	if state.Status != "downloading" {
		t.Errorf("Status = %q, want %q", state.Status, "downloading")
	}
	if len(state.StepsCompleted) != 1 || state.StepsCompleted[0] != "resolve" {
		t.Errorf("StepsCompleted = %v, want [resolve]", state.StepsCompleted)
	}
	if len(state.StepsRemaining) != 6 {
		t.Errorf("StepsRemaining count = %d, want 6", len(state.StepsRemaining))
	}
}

func TestUpdateDiscoveryStateWithError(t *testing.T) {
	kb := testKB(t)

	_, err := kb.StartDiscovery("test-pack", "user1", "")
	if err != nil {
		t.Fatalf("StartDiscovery: %v", err)
	}

	if err := kb.UpdateDiscoveryState("test-pack", "failed", "resolve", "download failed"); err != nil {
		t.Fatalf("UpdateDiscoveryState: %v", err)
	}

	state, err := kb.GetDiscoveryState("test-pack")
	if err != nil {
		t.Fatalf("GetDiscoveryState: %v", err)
	}
	if state.Error != "download failed" {
		t.Errorf("Error = %q, want %q", state.Error, "download failed")
	}
	if state.Status != "failed" {
		t.Errorf("Status = %q, want %q", state.Status, "failed")
	}
}

func TestUpdateDiscoveryStateNoState(t *testing.T) {
	kb := testKB(t)

	err := kb.UpdateDiscoveryState("nonexistent", "downloading", "", "")
	if err == nil {
		t.Fatal("expected error for nonexistent state")
	}
}

func TestGetDiscoveryStateNotFound(t *testing.T) {
	kb := testKB(t)

	state, err := kb.GetDiscoveryState("nonexistent")
	if err != nil {
		t.Fatalf("GetDiscoveryState: %v", err)
	}
	if state != nil {
		t.Error("expected nil state for nonexistent slug")
	}
}

func TestIsDiscoveryInProgress(t *testing.T) {
	kb := testKB(t)

	// No state file => not in progress.
	if kb.IsDiscoveryInProgress("test-pack") {
		t.Error("expected false for nonexistent state")
	}

	// Start discovery => in progress.
	_, err := kb.StartDiscovery("test-pack", "user1", "")
	if err != nil {
		t.Fatalf("StartDiscovery: %v", err)
	}
	if !kb.IsDiscoveryInProgress("test-pack") {
		t.Error("expected true for queued state")
	}

	// Mark as ready => not in progress.
	if err := kb.UpdateDiscoveryState("test-pack", "ready", "", ""); err != nil {
		t.Fatalf("UpdateDiscoveryState: %v", err)
	}
	if kb.IsDiscoveryInProgress("test-pack") {
		t.Error("expected false for ready state")
	}
}

func TestIsDiscoveryInProgressStale(t *testing.T) {
	kb := testKB(t)

	// Create a state file with old timestamp (>30 min ago).
	staleTime := time.Now().UTC().Add(-31 * time.Minute).Format(time.RFC3339)
	state := &DiscoveryState{
		Slug:           "test-pack",
		Status:         "downloading",
		RequestedBy:    "user1",
		StartedAt:      staleTime,
		UpdatedAt:      staleTime,
		StepsCompleted: []string{"resolve"},
		StepsRemaining: []string{"download", "extract", "analyze", "api_enrich", "web_search", "finalize"},
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	statePath := filepath.Join(kb.dir, ".discovery", "test-pack.state.json")
	if err := os.WriteFile(statePath, data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Stale state should not be considered in progress.
	if kb.IsDiscoveryInProgress("test-pack") {
		t.Error("expected false for stale state (>30 min old)")
	}

	// Starting new discovery should succeed for stale state.
	_, err = kb.StartDiscovery("test-pack", "user2", "")
	if err != nil {
		t.Fatalf("StartDiscovery after stale: %v", err)
	}
}

func TestDiscoveryLifecycle(t *testing.T) {
	kb := testKB(t)

	// Start.
	state, err := kb.StartDiscovery("my-pack", "bot", "1.2.3")
	if err != nil {
		t.Fatalf("StartDiscovery: %v", err)
	}
	if state.Status != "queued" {
		t.Fatalf("Status = %q, want queued", state.Status)
	}

	// Walk through all steps.
	steps := []struct {
		status string
		step   string
	}{
		{"downloading", "resolve"},
		{"extracting", "download"},
		{"analyzing", "extract"},
		{"api_enriching", "analyze"},
		{"web_searching", "api_enrich"},
		{"finalizing", "web_search"},
		{"ready", "finalize"},
	}

	for _, s := range steps {
		if err := kb.UpdateDiscoveryState("my-pack", s.status, s.step, ""); err != nil {
			t.Fatalf("UpdateDiscoveryState(%s, %s): %v", s.status, s.step, err)
		}
	}

	final, err := kb.GetDiscoveryState("my-pack")
	if err != nil {
		t.Fatalf("GetDiscoveryState: %v", err)
	}
	if final.Status != "ready" {
		t.Errorf("Status = %q, want ready", final.Status)
	}
	if len(final.StepsCompleted) != 7 {
		t.Errorf("StepsCompleted = %d, want 7", len(final.StepsCompleted))
	}
	if len(final.StepsRemaining) != 0 {
		t.Errorf("StepsRemaining = %d, want 0", len(final.StepsRemaining))
	}
	if kb.IsDiscoveryInProgress("my-pack") {
		t.Error("expected false for ready state")
	}
}
