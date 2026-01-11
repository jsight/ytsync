// Package http provides HTTP client infrastructure for YouTube interactions
package http

import (
	"errors"
	"sync"
	"time"
)

// CircuitState represents the state of a circuit breaker.
type CircuitState int

const (
	// CircuitClosed is the normal state where requests are allowed.
	CircuitClosed CircuitState = iota
	// CircuitOpen is the state where requests fail fast.
	CircuitOpen
	// CircuitHalfOpen is the testing state where one request is allowed.
	CircuitHalfOpen
)

// String returns the string representation of a circuit state.
func (s CircuitState) String() string {
	switch s {
	case CircuitClosed:
		return "closed"
	case CircuitOpen:
		return "open"
	case CircuitHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// Circuit breaker configuration constants
const (
	// DefaultFailureThreshold is the number of consecutive failures to open the circuit.
	DefaultFailureThreshold = 5
	// DefaultRecoveryTimeout is how long the circuit stays open before testing.
	DefaultRecoveryTimeout = 30 * time.Second
	// DefaultHalfOpenMaxRequests is the number of test requests allowed in half-open state.
	DefaultHalfOpenMaxRequests = 1
)

// ErrCircuitOpen is returned when the circuit breaker is open.
var ErrCircuitOpen = errors.New("circuit breaker is open")

// CircuitBreakerConfig configures circuit breaker behavior.
type CircuitBreakerConfig struct {
	// FailureThreshold is the number of consecutive failures to open the circuit.
	// Default: 5
	FailureThreshold int
	// RecoveryTimeout is how long the circuit stays open before transitioning to half-open.
	// Default: 30 seconds
	RecoveryTimeout time.Duration
	// HalfOpenMaxRequests is the number of test requests allowed in half-open state.
	// Default: 1
	HalfOpenMaxRequests int
	// IsTransientError is a function that determines if an error is transient (retryable).
	// Transient errors increment the failure count; permanent errors don't affect the circuit.
	// If nil, all errors are treated as transient.
	IsTransientError func(error) bool
}

// DefaultCircuitBreakerConfig returns sensible defaults for circuit breaker configuration.
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		FailureThreshold:    DefaultFailureThreshold,
		RecoveryTimeout:     DefaultRecoveryTimeout,
		HalfOpenMaxRequests: DefaultHalfOpenMaxRequests,
		IsTransientError:    nil, // All errors are transient by default
	}
}

// circuitState holds the state for a single circuit.
type circuitState struct {
	state             CircuitState
	consecutiveErrors int
	lastError         time.Time
	lastStateChange   time.Time
	halfOpenRequests  int
}

// CircuitBreaker implements the circuit breaker pattern for fault tolerance.
// It tracks failures per domain and opens the circuit to fail fast when
// too many consecutive failures occur.
type CircuitBreaker struct {
	circuits map[string]*circuitState
	mu       sync.RWMutex
	config   CircuitBreakerConfig
}

// NewCircuitBreaker creates a new circuit breaker with the given configuration.
func NewCircuitBreaker(cfg CircuitBreakerConfig) *CircuitBreaker {
	if cfg.FailureThreshold <= 0 {
		cfg.FailureThreshold = DefaultFailureThreshold
	}
	if cfg.RecoveryTimeout <= 0 {
		cfg.RecoveryTimeout = DefaultRecoveryTimeout
	}
	if cfg.HalfOpenMaxRequests <= 0 {
		cfg.HalfOpenMaxRequests = DefaultHalfOpenMaxRequests
	}

	return &CircuitBreaker{
		circuits: make(map[string]*circuitState),
		config:   cfg,
	}
}

// Allow checks if a request to the given domain should be allowed.
// Returns nil if the request is allowed, or ErrCircuitOpen if the circuit is open.
func (cb *CircuitBreaker) Allow(domain string) error {
	if cb == nil {
		return nil
	}

	cb.mu.Lock()
	defer cb.mu.Unlock()

	circuit := cb.getOrCreateCircuit(domain)

	switch circuit.state {
	case CircuitClosed:
		return nil

	case CircuitOpen:
		// Check if recovery timeout has elapsed
		if time.Since(circuit.lastStateChange) >= cb.config.RecoveryTimeout {
			// Transition to half-open and count this as the first test request
			circuit.state = CircuitHalfOpen
			circuit.lastStateChange = time.Now()
			circuit.halfOpenRequests = 1 // This request counts as the first test
			return nil
		}
		return ErrCircuitOpen

	case CircuitHalfOpen:
		// Allow limited requests in half-open state
		if circuit.halfOpenRequests < cb.config.HalfOpenMaxRequests {
			circuit.halfOpenRequests++
			return nil
		}
		return ErrCircuitOpen

	default:
		return nil
	}
}

// RecordSuccess records a successful request for the given domain.
// In half-open state, this closes the circuit.
func (cb *CircuitBreaker) RecordSuccess(domain string) {
	if cb == nil {
		return
	}

	cb.mu.Lock()
	defer cb.mu.Unlock()

	circuit := cb.getOrCreateCircuit(domain)

	switch circuit.state {
	case CircuitHalfOpen:
		// Success in half-open state closes the circuit
		circuit.state = CircuitClosed
		circuit.lastStateChange = time.Now()
		circuit.consecutiveErrors = 0
		circuit.halfOpenRequests = 0

	case CircuitClosed:
		// Reset consecutive errors on success
		circuit.consecutiveErrors = 0
	}
}

// RecordFailure records a failed request for the given domain.
// If the failure threshold is reached, the circuit opens.
func (cb *CircuitBreaker) RecordFailure(domain string, err error) {
	if cb == nil {
		return
	}

	// Check if this is a permanent error (shouldn't affect circuit)
	if cb.config.IsTransientError != nil && !cb.config.IsTransientError(err) {
		// Permanent error - don't affect circuit state
		return
	}

	cb.mu.Lock()
	defer cb.mu.Unlock()

	circuit := cb.getOrCreateCircuit(domain)

	switch circuit.state {
	case CircuitClosed:
		circuit.consecutiveErrors++
		circuit.lastError = time.Now()

		// Open the circuit if threshold reached
		if circuit.consecutiveErrors >= cb.config.FailureThreshold {
			circuit.state = CircuitOpen
			circuit.lastStateChange = time.Now()
		}

	case CircuitHalfOpen:
		// Failure in half-open state reopens the circuit
		circuit.state = CircuitOpen
		circuit.lastStateChange = time.Now()
		circuit.consecutiveErrors++
	}
}

// GetState returns the current state of the circuit for a domain.
func (cb *CircuitBreaker) GetState(domain string) CircuitState {
	if cb == nil {
		return CircuitClosed
	}

	cb.mu.RLock()
	defer cb.mu.RUnlock()

	circuit, exists := cb.circuits[domain]
	if !exists {
		return CircuitClosed
	}

	// Check for automatic state transitions
	if circuit.state == CircuitOpen {
		if time.Since(circuit.lastStateChange) >= cb.config.RecoveryTimeout {
			return CircuitHalfOpen
		}
	}

	return circuit.state
}

// GetStats returns statistics for a domain's circuit.
func (cb *CircuitBreaker) GetStats(domain string) CircuitStats {
	if cb == nil {
		return CircuitStats{State: CircuitClosed}
	}

	cb.mu.RLock()
	defer cb.mu.RUnlock()

	circuit, exists := cb.circuits[domain]
	if !exists {
		return CircuitStats{State: CircuitClosed}
	}

	state := circuit.state
	// Check for automatic state transitions
	if state == CircuitOpen && time.Since(circuit.lastStateChange) >= cb.config.RecoveryTimeout {
		state = CircuitHalfOpen
	}

	return CircuitStats{
		State:             state,
		ConsecutiveErrors: circuit.consecutiveErrors,
		LastError:         circuit.lastError,
		LastStateChange:   circuit.lastStateChange,
	}
}

// CircuitStats contains statistics about a circuit's state.
type CircuitStats struct {
	State             CircuitState
	ConsecutiveErrors int
	LastError         time.Time
	LastStateChange   time.Time
}

// Reset resets the circuit for a domain to the closed state.
func (cb *CircuitBreaker) Reset(domain string) {
	if cb == nil {
		return
	}

	cb.mu.Lock()
	defer cb.mu.Unlock()

	delete(cb.circuits, domain)
}

// ResetAll resets all circuits to the closed state.
func (cb *CircuitBreaker) ResetAll() {
	if cb == nil {
		return
	}

	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.circuits = make(map[string]*circuitState)
}

// getOrCreateCircuit gets or creates a circuit for a domain.
// Must be called with mutex held.
func (cb *CircuitBreaker) getOrCreateCircuit(domain string) *circuitState {
	circuit, exists := cb.circuits[domain]
	if !exists {
		circuit = &circuitState{
			state:           CircuitClosed,
			lastStateChange: time.Now(),
		}
		cb.circuits[domain] = circuit
	}
	return circuit
}

// IsTransientHTTPError is a helper function to determine if an HTTP error is transient.
// Use this as the IsTransientError function in CircuitBreakerConfig.
func IsTransientHTTPError(err error) bool {
	if err == nil {
		return false
	}

	// Rate limit errors are transient
	var rateLimitErr *RateLimitError
	if errors.As(err, &rateLimitErr) {
		return true
	}

	// HTTP errors: 5xx are transient, 4xx are permanent (except 429)
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		// 5xx errors are transient
		if httpErr.StatusCode >= 500 {
			return true
		}
		// 429 Too Many Requests is transient
		if httpErr.StatusCode == 429 {
			return true
		}
		// Other 4xx errors are permanent
		return false
	}

	// Network errors, timeouts, etc. are transient
	return true
}
