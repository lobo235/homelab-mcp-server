package resilience

import (
	"errors"
	"sync"
	"time"
)

// ErrCircuitOpen is returned when the circuit breaker is open.
var ErrCircuitOpen = errors.New("circuit breaker is open")

// CircuitBreaker implements the circuit breaker pattern.
// After failureThreshold consecutive failures within failureWindow,
// the circuit opens for openDuration. It closes after a successful health check.
type CircuitBreaker struct {
	mu               sync.Mutex
	failureThreshold int
	failureWindow    time.Duration
	openDuration     time.Duration

	failures  []time.Time
	openUntil time.Time
	state     string // "closed", "open", "half-open"
}

// NewCircuitBreaker creates a circuit breaker with the given parameters.
func NewCircuitBreaker(failureThreshold int, failureWindow, openDuration time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		failureThreshold: failureThreshold,
		failureWindow:    failureWindow,
		openDuration:     openDuration,
		state:            "closed",
	}
}

// DefaultCircuitBreaker returns a circuit breaker with the standard configuration:
// 5 failures in 30s opens for 60s.
func DefaultCircuitBreaker() *CircuitBreaker {
	return NewCircuitBreaker(5, 30*time.Second, 60*time.Second)
}

// Allow checks if a request should be allowed through.
// Returns ErrCircuitOpen if the circuit is open.
func (cb *CircuitBreaker) Allow() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()

	if cb.state == "open" {
		if now.After(cb.openUntil) {
			cb.state = "half-open"
			return nil
		}
		return ErrCircuitOpen
	}

	return nil
}

// RecordSuccess records a successful call and closes the circuit.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures = nil
	cb.state = "closed"
}

// RecordFailure records a failed call and may open the circuit.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-cb.failureWindow)

	// Remove old failures outside the window.
	var recent []time.Time
	for _, t := range cb.failures {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	recent = append(recent, now)
	cb.failures = recent

	if len(cb.failures) >= cb.failureThreshold {
		cb.state = "open"
		cb.openUntil = now.Add(cb.openDuration)
	}
}

// State returns the current circuit state.
func (cb *CircuitBreaker) State() string {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}
