package orchestration

import (
	"sync"
	"time"
)

// DestroyState tracks the progress of an async server destruction.
type DestroyState struct {
	Server         string   `json:"server"`
	Status         string   `json:"status"` // destroying|destroyed|partially_destroyed
	DeleteDir      bool     `json:"delete_directory"`
	StepsCompleted []string `json:"steps_completed"`
	Errors         []string `json:"errors,omitempty"`
	StartedAt      string   `json:"started_at"`
	CompletedAt    string   `json:"completed_at,omitempty"`
}

// destroyTracker tracks in-flight and recently completed destroy operations.
type destroyTracker struct {
	mu     sync.RWMutex
	states map[string]*DestroyState // keyed by server name
}

var tracker = &destroyTracker{
	states: make(map[string]*DestroyState),
}

// startDestroy registers a new destroy operation.
func (t *destroyTracker) startDestroy(name string, deleteDir bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.states[name] = &DestroyState{
		Server:    name,
		Status:    "destroying",
		DeleteDir: deleteDir,
		StartedAt: time.Now().UTC().Format(time.RFC3339),
	}
}

// addStep records a completed step.
func (t *destroyTracker) addStep(name, step string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if s, ok := t.states[name]; ok {
		s.StepsCompleted = append(s.StepsCompleted, step)
	}
}

// addError records a step failure.
func (t *destroyTracker) addError(name, errMsg string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if s, ok := t.states[name]; ok {
		s.Errors = append(s.Errors, errMsg)
	}
}

// complete marks the operation as finished.
func (t *destroyTracker) complete(name string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	s, ok := t.states[name]
	if !ok {
		return
	}
	s.CompletedAt = time.Now().UTC().Format(time.RFC3339)
	if len(s.Errors) > 0 {
		s.Status = "partially_destroyed"
	} else {
		s.Status = "destroyed"
	}
}

// get returns the current state for a server, or nil if not tracked.
func (t *destroyTracker) get(name string) *DestroyState {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if s, ok := t.states[name]; ok {
		// Return a copy to avoid races.
		cp := *s
		cp.StepsCompleted = append([]string(nil), s.StepsCompleted...)
		cp.Errors = append([]string(nil), s.Errors...)
		return &cp
	}
	return nil
}

// GetDestroyState returns the destroy state for a server (package-level accessor).
func GetDestroyState(name string) *DestroyState {
	return tracker.get(name)
}
