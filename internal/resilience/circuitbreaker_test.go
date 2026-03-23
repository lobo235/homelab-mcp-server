package resilience

import (
	"testing"
	"time"
)

func TestCircuitBreaker_Closed(t *testing.T) {
	cb := NewCircuitBreaker(5, 30*time.Second, 60*time.Second)

	if err := cb.Allow(); err != nil {
		t.Fatalf("expected Allow() to succeed in closed state: %v", err)
	}
	if cb.State() != "closed" {
		t.Errorf("state = %q, want %q", cb.State(), "closed")
	}
}

func TestCircuitBreaker_OpensAfterThreshold(t *testing.T) {
	cb := NewCircuitBreaker(3, 30*time.Second, 60*time.Second)

	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}

	if cb.State() != "open" {
		t.Errorf("state = %q, want %q", cb.State(), "open")
	}

	if err := cb.Allow(); err != ErrCircuitOpen {
		t.Errorf("Allow() = %v, want ErrCircuitOpen", err)
	}
}

func TestCircuitBreaker_ClosesOnSuccess(t *testing.T) {
	cb := NewCircuitBreaker(3, 30*time.Second, 60*time.Second)

	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}

	cb.RecordSuccess()

	if cb.State() != "closed" {
		t.Errorf("state = %q, want %q after success", cb.State(), "closed")
	}

	if err := cb.Allow(); err != nil {
		t.Errorf("Allow() failed after success: %v", err)
	}
}

func TestCircuitBreaker_HalfOpenAfterTimeout(t *testing.T) {
	cb := NewCircuitBreaker(3, 30*time.Second, 1*time.Millisecond)

	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}

	// Wait for circuit to expire.
	time.Sleep(5 * time.Millisecond)

	if err := cb.Allow(); err != nil {
		t.Errorf("Allow() should succeed in half-open state: %v", err)
	}
	if cb.State() != "half-open" {
		t.Errorf("state = %q, want %q", cb.State(), "half-open")
	}
}

func TestCircuitBreaker_FailuresOutsideWindowDontCount(t *testing.T) {
	cb := NewCircuitBreaker(3, 10*time.Millisecond, 60*time.Second)

	cb.RecordFailure()
	cb.RecordFailure()

	// Wait for failures to age out.
	time.Sleep(15 * time.Millisecond)

	cb.RecordFailure()

	// Should still be closed — only 1 failure in the window.
	if cb.State() != "closed" {
		t.Errorf("state = %q, want %q (old failures should not count)", cb.State(), "closed")
	}
}
