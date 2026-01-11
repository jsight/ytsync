package http

import (
	"errors"
	"testing"
	"time"
)

func TestCircuitBreakerInitialState(t *testing.T) {
	cb := NewCircuitBreaker(DefaultCircuitBreakerConfig())

	state := cb.GetState("example.com")
	if state != CircuitClosed {
		t.Errorf("initial state = %v, want CircuitClosed", state)
	}
}

func TestCircuitBreakerAllowInClosedState(t *testing.T) {
	cb := NewCircuitBreaker(DefaultCircuitBreakerConfig())

	err := cb.Allow("example.com")
	if err != nil {
		t.Errorf("Allow() in closed state returned error: %v", err)
	}
}

func TestCircuitBreakerOpensAfterThreshold(t *testing.T) {
	cfg := CircuitBreakerConfig{
		FailureThreshold:    3,
		RecoveryTimeout:     30 * time.Second,
		HalfOpenMaxRequests: 1,
	}
	cb := NewCircuitBreaker(cfg)

	testErr := errors.New("test error")

	// Record 2 failures - should stay closed
	cb.RecordFailure("example.com", testErr)
	cb.RecordFailure("example.com", testErr)

	if cb.GetState("example.com") != CircuitClosed {
		t.Error("circuit should still be closed after 2 failures")
	}

	// 3rd failure should open the circuit
	cb.RecordFailure("example.com", testErr)

	if cb.GetState("example.com") != CircuitOpen {
		t.Error("circuit should be open after 3 failures")
	}
}

func TestCircuitBreakerRejectsWhenOpen(t *testing.T) {
	cfg := CircuitBreakerConfig{
		FailureThreshold:    2,
		RecoveryTimeout:     30 * time.Second,
		HalfOpenMaxRequests: 1,
	}
	cb := NewCircuitBreaker(cfg)

	testErr := errors.New("test error")

	// Open the circuit
	cb.RecordFailure("example.com", testErr)
	cb.RecordFailure("example.com", testErr)

	// Should reject requests
	err := cb.Allow("example.com")
	if !errors.Is(err, ErrCircuitOpen) {
		t.Errorf("Allow() = %v, want ErrCircuitOpen", err)
	}
}

func TestCircuitBreakerTransitionsToHalfOpen(t *testing.T) {
	cfg := CircuitBreakerConfig{
		FailureThreshold:    2,
		RecoveryTimeout:     50 * time.Millisecond,
		HalfOpenMaxRequests: 1,
	}
	cb := NewCircuitBreaker(cfg)

	testErr := errors.New("test error")

	// Open the circuit
	cb.RecordFailure("example.com", testErr)
	cb.RecordFailure("example.com", testErr)

	if cb.GetState("example.com") != CircuitOpen {
		t.Fatal("circuit should be open")
	}

	// Wait for recovery timeout
	time.Sleep(60 * time.Millisecond)

	// Should now be half-open (checked via GetState which detects timeout)
	if cb.GetState("example.com") != CircuitHalfOpen {
		t.Error("circuit should transition to half-open after recovery timeout")
	}

	// Allow should succeed in half-open state
	err := cb.Allow("example.com")
	if err != nil {
		t.Errorf("Allow() in half-open state returned error: %v", err)
	}
}

func TestCircuitBreakerClosesOnSuccessInHalfOpen(t *testing.T) {
	cfg := CircuitBreakerConfig{
		FailureThreshold:    2,
		RecoveryTimeout:     50 * time.Millisecond,
		HalfOpenMaxRequests: 1,
	}
	cb := NewCircuitBreaker(cfg)

	testErr := errors.New("test error")

	// Open the circuit
	cb.RecordFailure("example.com", testErr)
	cb.RecordFailure("example.com", testErr)

	// Wait for recovery timeout
	time.Sleep(60 * time.Millisecond)

	// Trigger half-open state check and allow a test request
	cb.Allow("example.com")

	// Record success
	cb.RecordSuccess("example.com")

	// Should be closed now
	if cb.GetState("example.com") != CircuitClosed {
		t.Error("circuit should close after success in half-open state")
	}
}

func TestCircuitBreakerReopensOnFailureInHalfOpen(t *testing.T) {
	cfg := CircuitBreakerConfig{
		FailureThreshold:    2,
		RecoveryTimeout:     50 * time.Millisecond,
		HalfOpenMaxRequests: 1,
	}
	cb := NewCircuitBreaker(cfg)

	testErr := errors.New("test error")

	// Open the circuit
	cb.RecordFailure("example.com", testErr)
	cb.RecordFailure("example.com", testErr)

	// Wait for recovery timeout
	time.Sleep(60 * time.Millisecond)

	// Trigger half-open state check
	cb.Allow("example.com")

	// Record failure in half-open state
	cb.RecordFailure("example.com", testErr)

	// Should be open again
	if cb.GetState("example.com") != CircuitOpen {
		t.Error("circuit should reopen after failure in half-open state")
	}
}

func TestCircuitBreakerResetsConsecutiveErrorsOnSuccess(t *testing.T) {
	cfg := CircuitBreakerConfig{
		FailureThreshold:    5,
		RecoveryTimeout:     30 * time.Second,
		HalfOpenMaxRequests: 1,
	}
	cb := NewCircuitBreaker(cfg)

	testErr := errors.New("test error")

	// Record 3 failures
	cb.RecordFailure("example.com", testErr)
	cb.RecordFailure("example.com", testErr)
	cb.RecordFailure("example.com", testErr)

	stats := cb.GetStats("example.com")
	if stats.ConsecutiveErrors != 3 {
		t.Errorf("consecutive errors = %d, want 3", stats.ConsecutiveErrors)
	}

	// Record success - should reset counter
	cb.RecordSuccess("example.com")

	stats = cb.GetStats("example.com")
	if stats.ConsecutiveErrors != 0 {
		t.Errorf("consecutive errors after success = %d, want 0", stats.ConsecutiveErrors)
	}
}

func TestCircuitBreakerIgnoresPermanentErrors(t *testing.T) {
	permanentErr := errors.New("permanent error")
	transientErr := errors.New("transient error")

	cfg := CircuitBreakerConfig{
		FailureThreshold:    3,
		RecoveryTimeout:     30 * time.Second,
		HalfOpenMaxRequests: 1,
		IsTransientError: func(err error) bool {
			return err == transientErr
		},
	}
	cb := NewCircuitBreaker(cfg)

	// Record permanent errors - should not affect circuit
	cb.RecordFailure("example.com", permanentErr)
	cb.RecordFailure("example.com", permanentErr)
	cb.RecordFailure("example.com", permanentErr)

	if cb.GetState("example.com") != CircuitClosed {
		t.Error("circuit should remain closed for permanent errors")
	}

	// Record transient errors - should open circuit
	cb.RecordFailure("example.com", transientErr)
	cb.RecordFailure("example.com", transientErr)
	cb.RecordFailure("example.com", transientErr)

	if cb.GetState("example.com") != CircuitOpen {
		t.Error("circuit should open for transient errors")
	}
}

func TestCircuitBreakerPerDomainIsolation(t *testing.T) {
	cfg := CircuitBreakerConfig{
		FailureThreshold:    2,
		RecoveryTimeout:     30 * time.Second,
		HalfOpenMaxRequests: 1,
	}
	cb := NewCircuitBreaker(cfg)

	testErr := errors.New("test error")

	// Open circuit for domain1
	cb.RecordFailure("domain1.com", testErr)
	cb.RecordFailure("domain1.com", testErr)

	if cb.GetState("domain1.com") != CircuitOpen {
		t.Error("domain1 circuit should be open")
	}

	// domain2 should still be closed
	if cb.GetState("domain2.com") != CircuitClosed {
		t.Error("domain2 circuit should be closed")
	}

	// Should be able to make requests to domain2
	err := cb.Allow("domain2.com")
	if err != nil {
		t.Errorf("domain2 Allow() returned error: %v", err)
	}
}

func TestCircuitBreakerReset(t *testing.T) {
	cfg := CircuitBreakerConfig{
		FailureThreshold:    2,
		RecoveryTimeout:     30 * time.Second,
		HalfOpenMaxRequests: 1,
	}
	cb := NewCircuitBreaker(cfg)

	testErr := errors.New("test error")

	// Open circuit
	cb.RecordFailure("example.com", testErr)
	cb.RecordFailure("example.com", testErr)

	if cb.GetState("example.com") != CircuitOpen {
		t.Fatal("circuit should be open")
	}

	// Reset
	cb.Reset("example.com")

	// Should be closed and allow requests
	if cb.GetState("example.com") != CircuitClosed {
		t.Error("circuit should be closed after reset")
	}

	err := cb.Allow("example.com")
	if err != nil {
		t.Errorf("Allow() after reset returned error: %v", err)
	}
}

func TestCircuitBreakerResetAll(t *testing.T) {
	cfg := CircuitBreakerConfig{
		FailureThreshold:    2,
		RecoveryTimeout:     30 * time.Second,
		HalfOpenMaxRequests: 1,
	}
	cb := NewCircuitBreaker(cfg)

	testErr := errors.New("test error")

	// Open circuits for multiple domains
	cb.RecordFailure("domain1.com", testErr)
	cb.RecordFailure("domain1.com", testErr)
	cb.RecordFailure("domain2.com", testErr)
	cb.RecordFailure("domain2.com", testErr)

	// Reset all
	cb.ResetAll()

	// All should be closed
	if cb.GetState("domain1.com") != CircuitClosed {
		t.Error("domain1 circuit should be closed after ResetAll")
	}
	if cb.GetState("domain2.com") != CircuitClosed {
		t.Error("domain2 circuit should be closed after ResetAll")
	}
}

func TestCircuitBreakerHalfOpenLimitsRequests(t *testing.T) {
	cfg := CircuitBreakerConfig{
		FailureThreshold:    2,
		RecoveryTimeout:     50 * time.Millisecond,
		HalfOpenMaxRequests: 1,
	}
	cb := NewCircuitBreaker(cfg)

	testErr := errors.New("test error")

	// Open circuit
	cb.RecordFailure("example.com", testErr)
	cb.RecordFailure("example.com", testErr)

	// Wait for half-open
	time.Sleep(60 * time.Millisecond)

	// First request should be allowed
	err := cb.Allow("example.com")
	if err != nil {
		t.Errorf("first Allow() in half-open returned error: %v", err)
	}

	// Second request should be rejected (only 1 test request allowed)
	err = cb.Allow("example.com")
	if !errors.Is(err, ErrCircuitOpen) {
		t.Errorf("second Allow() in half-open = %v, want ErrCircuitOpen", err)
	}
}

func TestCircuitStateString(t *testing.T) {
	tests := []struct {
		state CircuitState
		want  string
	}{
		{CircuitClosed, "closed"},
		{CircuitOpen, "open"},
		{CircuitHalfOpen, "half-open"},
		{CircuitState(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.state.String(); got != tt.want {
			t.Errorf("CircuitState(%d).String() = %q, want %q", tt.state, got, tt.want)
		}
	}
}

func TestCircuitBreakerGetStats(t *testing.T) {
	cfg := CircuitBreakerConfig{
		FailureThreshold:    5,
		RecoveryTimeout:     30 * time.Second,
		HalfOpenMaxRequests: 1,
	}
	cb := NewCircuitBreaker(cfg)

	testErr := errors.New("test error")

	// Record some failures
	cb.RecordFailure("example.com", testErr)
	cb.RecordFailure("example.com", testErr)
	cb.RecordFailure("example.com", testErr)

	stats := cb.GetStats("example.com")

	if stats.State != CircuitClosed {
		t.Errorf("State = %v, want CircuitClosed", stats.State)
	}
	if stats.ConsecutiveErrors != 3 {
		t.Errorf("ConsecutiveErrors = %d, want 3", stats.ConsecutiveErrors)
	}
	if stats.LastError.IsZero() {
		t.Error("LastError should be set")
	}
}

func TestCircuitBreakerNilSafety(t *testing.T) {
	var cb *CircuitBreaker

	// All methods should be safe to call on nil
	err := cb.Allow("example.com")
	if err != nil {
		t.Errorf("nil Allow() returned error: %v", err)
	}

	cb.RecordSuccess("example.com")
	cb.RecordFailure("example.com", errors.New("test"))
	cb.Reset("example.com")
	cb.ResetAll()

	state := cb.GetState("example.com")
	if state != CircuitClosed {
		t.Errorf("nil GetState() = %v, want CircuitClosed", state)
	}

	stats := cb.GetStats("example.com")
	if stats.State != CircuitClosed {
		t.Errorf("nil GetStats() State = %v, want CircuitClosed", stats.State)
	}
}

func TestIsTransientHTTPError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"rate limit error", &RateLimitError{StatusCode: 429}, true},
		{"5xx error", &HTTPError{StatusCode: 500}, true},
		{"503 error", &HTTPError{StatusCode: 503}, true},
		{"429 error", &HTTPError{StatusCode: 429}, true},
		{"400 error", &HTTPError{StatusCode: 400}, false},
		{"404 error", &HTTPError{StatusCode: 404}, false},
		{"generic error", errors.New("network error"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsTransientHTTPError(tt.err); got != tt.want {
				t.Errorf("IsTransientHTTPError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestDefaultCircuitBreakerConfig(t *testing.T) {
	cfg := DefaultCircuitBreakerConfig()

	if cfg.FailureThreshold != DefaultFailureThreshold {
		t.Errorf("FailureThreshold = %d, want %d", cfg.FailureThreshold, DefaultFailureThreshold)
	}
	if cfg.RecoveryTimeout != DefaultRecoveryTimeout {
		t.Errorf("RecoveryTimeout = %v, want %v", cfg.RecoveryTimeout, DefaultRecoveryTimeout)
	}
	if cfg.HalfOpenMaxRequests != DefaultHalfOpenMaxRequests {
		t.Errorf("HalfOpenMaxRequests = %d, want %d", cfg.HalfOpenMaxRequests, DefaultHalfOpenMaxRequests)
	}
}
