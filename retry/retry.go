// Package retry provides exponential backoff retry logic with jitter.
package retry

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"time"
)

// Config holds retry configuration.
type Config struct {
	// MaxRetries is the maximum number of retry attempts.
	MaxRetries int
	// InitialBackoff is the initial delay before retrying.
	InitialBackoff time.Duration
	// MaxBackoff is the maximum delay between retries.
	MaxBackoff time.Duration
	// Multiplier is the exponential backoff multiplier.
	Multiplier float64
	// JitterFraction is the fraction of backoff used for jitter (0.0-1.0).
	JitterFraction float64
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		MaxRetries:     5,
		InitialBackoff: 1 * time.Second,
		MaxBackoff:     30 * time.Second,
		Multiplier:     2.0,
		JitterFraction: 0.2, // +/- 20% jitter
	}
}

// ErrorClassifier determines if an error is retryable.
type ErrorClassifier func(error) bool

// IsRetryable is a default error classifier that checks for common retryable errors.
func IsRetryable(err error) bool {
	// Check for context errors (not retryable)
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	// Check for permanent errors
	if errors.Is(err, ErrChannelNotFound) || errors.Is(err, ErrInvalidURL) {
		return false
	}

	// Everything else is retryable
	return true
}

// Sentinel errors that are permanent.
var (
	ErrChannelNotFound = errors.New("channel not found")
	ErrInvalidURL      = errors.New("invalid url")
)

// Do executes fn with retry logic, using the provided classifier to determine
// if errors are retryable.
func Do(ctx context.Context, cfg Config, classifier ErrorClassifier, fn func(context.Context) error) error {
	if classifier == nil {
		classifier = IsRetryable
	}

	var lastErr error
	backoff := cfg.InitialBackoff

	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		// Attempt the operation
		if err := fn(ctx); err == nil {
			return nil
		} else {
			lastErr = err
			if !classifier(err) {
				// Permanent error, don't retry
				return err
			}
		}

		// Last attempt, don't sleep
		if attempt == cfg.MaxRetries {
			break
		}

		// Calculate backoff with jitter
		sleep := backoff + jitter(backoff, cfg.JitterFraction)
		if sleep > cfg.MaxBackoff {
			sleep = cfg.MaxBackoff
		}

		// Sleep or return if context is canceled
		select {
		case <-time.After(sleep):
			// Continue to next attempt
		case <-ctx.Done():
			return ctx.Err()
		}

		// Increase backoff for next attempt
		backoff = time.Duration(float64(backoff) * cfg.Multiplier)
		if backoff > cfg.MaxBackoff {
			backoff = cfg.MaxBackoff
		}
	}

	return fmt.Errorf("max retries exceeded: %w", lastErr)
}

// jitter returns a random duration in range [-jitterFraction*d, +jitterFraction*d].
func jitter(d time.Duration, fraction float64) time.Duration {
	if fraction <= 0 {
		return 0
	}
	jitterRange := float64(d) * fraction
	jitterValue := (rand.Float64() - 0.5) * 2 * jitterRange
	return time.Duration(jitterValue)
}

// RetryableError wraps an error and indicates it's retryable.
type RetryableError struct {
	Err    error
	Retries int
}

func (e *RetryableError) Error() string {
	return fmt.Sprintf("failed after %d retries: %v", e.Retries, e.Err)
}

func (e *RetryableError) Unwrap() error {
	return e.Err
}
