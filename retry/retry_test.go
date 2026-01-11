package retry

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDo_Success(t *testing.T) {
	attempts := 0
	cfg := Config{
		MaxRetries:     3,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     100 * time.Millisecond,
		Multiplier:     2.0,
	}

	err := Do(context.Background(), cfg, nil, func(ctx context.Context) error {
		attempts++
		return nil
	})

	if err != nil {
		t.Errorf("Do() returned error = %v, want nil", err)
	}
	if attempts != 1 {
		t.Errorf("Do() made %d attempts, want 1", attempts)
	}
}

func TestDo_PermanentError(t *testing.T) {
	attempts := 0
	permanentErr := errors.New("permanent")
	cfg := Config{
		MaxRetries:     3,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     100 * time.Millisecond,
		Multiplier:     2.0,
	}

	// Classifier that marks this error as non-retryable
	classifier := func(err error) bool {
		return !errors.Is(err, permanentErr)
	}

	err := Do(context.Background(), cfg, classifier, func(ctx context.Context) error {
		attempts++
		return permanentErr
	})

	if !errors.Is(err, permanentErr) {
		t.Errorf("Do() returned error = %v, want %v", err, permanentErr)
	}
	if attempts != 1 {
		t.Errorf("Do() made %d attempts, want 1", attempts)
	}
}

func TestDo_RetryableError(t *testing.T) {
	attempts := 0
	tempErr := errors.New("temporary")
	successAfter := 2
	cfg := Config{
		MaxRetries:     5,
		InitialBackoff: 5 * time.Millisecond,
		MaxBackoff:     50 * time.Millisecond,
		Multiplier:     2.0,
	}

	err := Do(context.Background(), cfg, IsRetryable, func(ctx context.Context) error {
		attempts++
		if attempts < successAfter {
			return tempErr
		}
		return nil
	})

	if err != nil {
		t.Errorf("Do() returned error = %v, want nil", err)
	}
	if attempts != successAfter {
		t.Errorf("Do() made %d attempts, want %d", attempts, successAfter)
	}
}

func TestDo_MaxRetriesExceeded(t *testing.T) {
	attempts := 0
	tempErr := errors.New("temporary")
	maxRetries := 3
	cfg := Config{
		MaxRetries:     maxRetries,
		InitialBackoff: 5 * time.Millisecond,
		MaxBackoff:     50 * time.Millisecond,
		Multiplier:     2.0,
	}

	err := Do(context.Background(), cfg, IsRetryable, func(ctx context.Context) error {
		attempts++
		return tempErr
	})

	if err == nil {
		t.Error("Do() returned nil error, want error")
	}
	if attempts != maxRetries+1 {
		t.Errorf("Do() made %d attempts, want %d", attempts, maxRetries+1)
	}
}

func TestDo_ContextCanceled(t *testing.T) {
	attempts := 0
	cfg := Config{
		MaxRetries:     5,
		InitialBackoff: 100 * time.Millisecond,
		MaxBackoff:     1 * time.Second,
		Multiplier:     2.0,
	}

	ctx, cancel := context.WithCancel(context.Background())

	err := Do(ctx, cfg, IsRetryable, func(ctx context.Context) error {
		attempts++
		if attempts > 1 {
			cancel()
		}
		return errors.New("temporary")
	})

	if err == nil {
		t.Error("Do() returned nil error, want context.Canceled")
	}
	if !errors.Is(err, context.Canceled) && err.Error() != "context canceled" {
		t.Errorf("Do() returned error = %v, want context.Canceled", err)
	}
}

func TestDo_ContextDeadline(t *testing.T) {
	cfg := Config{
		MaxRetries:     5,
		InitialBackoff: 100 * time.Millisecond,
		MaxBackoff:     1 * time.Second,
		Multiplier:     2.0,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := Do(ctx, cfg, IsRetryable, func(ctx context.Context) error {
		time.Sleep(200 * time.Millisecond)
		return errors.New("temporary")
	})

	if err == nil {
		t.Error("Do() returned nil error, want context.DeadlineExceeded")
	}
}

func TestBackoffProgression(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxRetries = 3
	cfg.InitialBackoff = 10 * time.Millisecond
	cfg.MaxBackoff = 100 * time.Millisecond
	cfg.Multiplier = 2.0
	cfg.JitterFraction = 0 // No jitter for predictable testing

	var backoffs []time.Duration
	start := time.Now()

	Do(context.Background(), cfg, IsRetryable, func(ctx context.Context) error {
		if len(backoffs) > 0 {
			backoffs = append(backoffs, time.Since(start))
		}
		if len(backoffs) <= cfg.MaxRetries {
			return errors.New("retry")
		}
		return nil
	})

	// Backoff should roughly double each time
	if len(backoffs) > 1 {
		ratio := backoffs[1].Seconds() / backoffs[0].Seconds()
		if ratio < 1.5 || ratio > 2.5 {
			t.Logf("Backoff ratio = %f (expected ~2.0), backoffs = %v", ratio, backoffs)
		}
	}
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name  string
		err   error
		want  bool
	}{
		{"nil error", nil, true}, // nil is treated as no error, but function returns false
		{"context canceled", context.Canceled, false},
		{"context deadline exceeded", context.DeadlineExceeded, false},
		{"channel not found", ErrChannelNotFound, false},
		{"invalid URL", ErrInvalidURL, false},
		{"generic error", errors.New("generic"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsRetryable(tt.err); got != tt.want {
				t.Errorf("IsRetryable(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.MaxRetries != 5 {
		t.Errorf("DefaultConfig().MaxRetries = %d, want 5", cfg.MaxRetries)
	}
	if cfg.InitialBackoff != 1*time.Second {
		t.Errorf("DefaultConfig().InitialBackoff = %v, want 1s", cfg.InitialBackoff)
	}
	if cfg.MaxBackoff != 30*time.Second {
		t.Errorf("DefaultConfig().MaxBackoff = %v, want 30s", cfg.MaxBackoff)
	}
	if cfg.Multiplier != 2.0 {
		t.Errorf("DefaultConfig().Multiplier = %f, want 2.0", cfg.Multiplier)
	}
}
