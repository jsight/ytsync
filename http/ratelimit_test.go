package http

import (
	"context"
	"testing"
	"time"
)

func TestNewRateLimiter(t *testing.T) {
	cfg := DefaultRateLimiterConfig()
	rl := NewRateLimiter(cfg)

	if rl == nil {
		t.Fatal("NewRateLimiter returned nil")
	}
}

func TestRateLimiterWait(t *testing.T) {
	cfg := RateLimiterConfig{
		InnertubeRPS: 10.0, // 10 req/s = 100ms per request
	}
	rl := NewRateLimiter(cfg)

	ctx := context.Background()
	url := "https://www.youtube.com/api/test"

	// First request should not wait
	start := time.Now()
	if err := rl.Wait(ctx, url); err != nil {
		t.Fatalf("Wait failed: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed > 50*time.Millisecond {
		t.Logf("First request took %v (expected ~0ms)", elapsed)
	}

	// Second request should wait ~100ms
	start = time.Now()
	if err := rl.Wait(ctx, url); err != nil {
		t.Fatalf("Wait failed: %v", err)
	}
	elapsed = time.Since(start)
	if elapsed < 50*time.Millisecond {
		t.Logf("Second request took %v (expected ~100ms)", elapsed)
	}
}

func TestRateLimiterContextCanceled(t *testing.T) {
	cfg := RateLimiterConfig{
		InnertubeRPS: 1.0, // 1 req/s = 1s per request
	}
	rl := NewRateLimiter(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	url := "https://www.youtube.com/api/test"

	// First request succeeds
	if err := rl.Wait(ctx, url); err != nil {
		t.Fatalf("First Wait failed: %v", err)
	}

	// Cancel context
	cancel()

	// Second request should fail with context canceled
	if err := rl.Wait(ctx, url); err == nil {
		t.Fatal("Expected context canceled error")
	}
}

func TestRateLimiterMultipleDomains(t *testing.T) {
	cfg := DefaultRateLimiterConfig()
	cfg.CustomRates["api.example.com"] = 5.0
	rl := NewRateLimiter(cfg)

	ctx := context.Background()

	// Different domains should have different limiters
	url1 := "https://www.youtube.com/api/test"
	url2 := "https://api.example.com/test"

	// Both should succeed immediately (first request on each)
	if err := rl.Wait(ctx, url1); err != nil {
		t.Fatalf("Wait for youtube.com failed: %v", err)
	}
	if err := rl.Wait(ctx, url2); err != nil {
		t.Fatalf("Wait for example.com failed: %v", err)
	}
}

func TestRateLimiterUnlimitedDomain(t *testing.T) {
	cfg := RateLimiterConfig{
		RSSRPS: 0, // Unlimited
	}
	rl := NewRateLimiter(cfg)

	ctx := context.Background()
	url := "https://feeds.youtube.com/test"

	// Multiple requests should all succeed without waiting
	for i := 0; i < 100; i++ {
		if err := rl.Wait(ctx, url); err != nil {
			t.Fatalf("Wait failed on iteration %d: %v", i, err)
		}
	}
}

func TestRateLimiterExtractDomain(t *testing.T) {
	cfg := DefaultRateLimiterConfig()
	rl := NewRateLimiter(cfg)

	tests := []struct {
		url    string
		domain string
	}{
		{"https://www.youtube.com/api/test", "www.youtube.com"},
		{"https://googleapis.com:443/test", "googleapis.com"},
		{"http://feeds.youtube.com/test?param=value", "feeds.youtube.com"},
		{"invalid url", "unknown"},
	}

	for _, tt := range tests {
		got := rl.extractDomain(tt.url)
		if got != tt.domain {
			t.Errorf("extractDomain(%q) = %q, want %q", tt.url, got, tt.domain)
		}
	}
}

func TestRateLimiterGetRPS(t *testing.T) {
	cfg := RateLimiterConfig{
		InnertubeRPS: 2.5,
		DataAPIRPS:   1.0,
		RSSRPS:       10.0,
		CustomRates:  map[string]float64{"custom.com": 5.0},
	}
	rl := NewRateLimiter(cfg)

	tests := []struct {
		domain string
		rps    float64
	}{
		{"www.youtube.com", 2.5},
		{"googleapis.com", 1.0},
		{"feeds.youtube.com", 10.0},
		{"custom.com", 5.0},
		{"unknown.com", 2.5}, // defaults to innertube
	}

	for _, tt := range tests {
		got := rl.getRPS(tt.domain)
		if got != tt.rps {
			t.Errorf("getRPS(%q) = %v, want %v", tt.domain, got, tt.rps)
		}
	}
}

func TestRateLimiterSetCustomRate(t *testing.T) {
	cfg := DefaultRateLimiterConfig()
	rl := NewRateLimiter(cfg)

	domain := "test.com"
	newRate := 20.0

	rl.SetCustomRate(domain, newRate)

	got := rl.getRPS(domain)
	if got != newRate {
		t.Errorf("After SetCustomRate, getRPS(%q) = %v, want %v", domain, got, newRate)
	}
}

func TestRateLimiterStats(t *testing.T) {
	cfg := DefaultRateLimiterConfig()
	rl := NewRateLimiter(cfg)

	ctx := context.Background()

	// Create some limiters
	rl.Wait(ctx, "https://www.youtube.com/test")
	rl.Wait(ctx, "https://feeds.youtube.com/test")

	stats := rl.Stats()

	// Check that stats are returned for created limiters
	if _, ok := stats["www.youtube.com"]; !ok {
		t.Error("Stats missing www.youtube.com")
	}
	if _, ok := stats["feeds.youtube.com"]; !ok {
		t.Error("Stats missing feeds.youtube.com")
	}
}

func TestRateLimiterRecordRateLimitError(t *testing.T) {
	cfg := DefaultRateLimiterConfig()
	cfg.EnableDynamicBackoff = true
	rl := NewRateLimiter(cfg)

	url := "https://www.youtube.com/api/test"

	// First error should return initial backoff (1s)
	backoff := rl.RecordRateLimitError(url, 0)
	if backoff != InnertubeInitialBackoff {
		t.Errorf("First error backoff = %v, want %v", backoff, InnertubeInitialBackoff)
	}

	// Second error should return doubled backoff (2s)
	backoff = rl.RecordRateLimitError(url, 0)
	expectedBackoff := time.Duration(float64(InnertubeInitialBackoff) * InnertubeBackoffMultiplier)
	if backoff != expectedBackoff {
		t.Errorf("Second error backoff = %v, want %v", backoff, expectedBackoff)
	}

	// Third error should return 4s
	backoff = rl.RecordRateLimitError(url, 0)
	expectedBackoff = time.Duration(float64(expectedBackoff) * InnertubeBackoffMultiplier)
	if backoff != expectedBackoff {
		t.Errorf("Third error backoff = %v, want %v", backoff, expectedBackoff)
	}
}

func TestRateLimiterRecordRateLimitError_RetryAfterRespected(t *testing.T) {
	cfg := DefaultRateLimiterConfig()
	cfg.EnableDynamicBackoff = true
	rl := NewRateLimiter(cfg)

	url := "https://www.youtube.com/api/test"
	serverRetryAfter := 30 * time.Second

	// Server's Retry-After should be used if longer than calculated backoff
	backoff := rl.RecordRateLimitError(url, serverRetryAfter)
	if backoff != serverRetryAfter {
		t.Errorf("Backoff = %v, want server's Retry-After %v", backoff, serverRetryAfter)
	}
}

func TestRateLimiterBackoffState(t *testing.T) {
	cfg := DefaultRateLimiterConfig()
	cfg.EnableDynamicBackoff = true
	rl := NewRateLimiter(cfg)

	url := "https://www.youtube.com/api/test"

	// Initially no backoff state
	if state := rl.GetBackoffState(url); state != nil {
		t.Error("Expected no backoff state initially")
	}

	// Record an error
	rl.RecordRateLimitError(url, 0)

	// Now should have backoff state
	state := rl.GetBackoffState(url)
	if state == nil {
		t.Fatal("Expected backoff state after error")
	}
	if state.ConsecutiveErrors != 1 {
		t.Errorf("ConsecutiveErrors = %d, want 1", state.ConsecutiveErrors)
	}
	if state.OriginalRPS == 0 {
		t.Error("OriginalRPS should be set")
	}
}

func TestRateLimiterRecordSuccess(t *testing.T) {
	cfg := DefaultRateLimiterConfig()
	cfg.EnableDynamicBackoff = true
	rl := NewRateLimiter(cfg)

	url := "https://www.youtube.com/api/test"

	// Record multiple errors
	rl.RecordRateLimitError(url, 0)
	rl.RecordRateLimitError(url, 0)
	rl.RecordRateLimitError(url, 0)

	state := rl.GetBackoffState(url)
	if state.ConsecutiveErrors != 3 {
		t.Errorf("After 3 errors, ConsecutiveErrors = %d, want 3", state.ConsecutiveErrors)
	}

	// Record a success
	rl.RecordSuccess(url)

	state = rl.GetBackoffState(url)
	if state.ConsecutiveErrors != 2 {
		t.Errorf("After success, ConsecutiveErrors = %d, want 2", state.ConsecutiveErrors)
	}
}

func TestRateLimiterIsBackedOff(t *testing.T) {
	cfg := DefaultRateLimiterConfig()
	cfg.EnableDynamicBackoff = true
	rl := NewRateLimiter(cfg)

	url := "https://www.youtube.com/api/test"

	// Initially not backed off
	if rl.IsBackedOff(url) {
		t.Error("Should not be backed off initially")
	}

	// Record an error
	rl.RecordRateLimitError(url, 0)

	// Should be backed off
	if !rl.IsBackedOff(url) {
		t.Error("Should be backed off after error")
	}
}

func TestRateLimiterRateReduction(t *testing.T) {
	cfg := RateLimiterConfig{
		InnertubeRPS:         2.5,
		EnableDynamicBackoff: true,
	}
	rl := NewRateLimiter(cfg)

	url := "https://www.youtube.com/api/test"
	ctx := context.Background()

	// Initialize the limiter
	rl.Wait(ctx, url)

	// Record multiple errors to trigger rate reduction
	rl.RecordRateLimitError(url, 0)

	state := rl.GetBackoffState(url)
	if state.ReducedRPS == 0 {
		t.Error("ReducedRPS should be set after error")
	}
	if state.ReducedRPS >= state.OriginalRPS {
		t.Errorf("ReducedRPS (%v) should be less than OriginalRPS (%v)", state.ReducedRPS, state.OriginalRPS)
	}
}

func TestRateLimiterDisabledDynamicBackoff(t *testing.T) {
	cfg := DefaultRateLimiterConfig()
	cfg.EnableDynamicBackoff = false
	rl := NewRateLimiter(cfg)

	url := "https://www.youtube.com/api/test"

	// Record an error
	backoff := rl.RecordRateLimitError(url, 0)

	// Should still return initial backoff
	if backoff != InnertubeInitialBackoff {
		t.Errorf("Backoff = %v, want %v", backoff, InnertubeInitialBackoff)
	}

	// But no backoff state should be tracked
	if state := rl.GetBackoffState(url); state != nil {
		t.Error("No backoff state should be tracked when disabled")
	}
}

func TestBackoffStateConstants(t *testing.T) {
	// Verify constants are sensible
	if InnertubeInitialBackoff != 1*time.Second {
		t.Errorf("InnertubeInitialBackoff = %v, want 1s", InnertubeInitialBackoff)
	}
	if InnertubeMaxBackoff != 60*time.Second {
		t.Errorf("InnertubeMaxBackoff = %v, want 60s", InnertubeMaxBackoff)
	}
	if InnertubeBackoffMultiplier != 2.0 {
		t.Errorf("InnertubeBackoffMultiplier = %v, want 2.0", InnertubeBackoffMultiplier)
	}
	if MinRPSMultiplier != 0.25 {
		t.Errorf("MinRPSMultiplier = %v, want 0.25", MinRPSMultiplier)
	}
}
