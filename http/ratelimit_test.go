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
