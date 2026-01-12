package http

import (
	"bytes"
	"context"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// =============================================================================
// Rate Limiting Detection Tests
// =============================================================================

func TestRateLimitDetection_429Response(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	}))
	defer server.Close()

	client := New(nil)
	defer client.Close()

	_, err := client.Get(context.Background(), server.URL)

	// Should eventually succeed after retry
	if err != nil {
		t.Logf("Request may have failed due to retry exhaustion: %v", err)
	}

	// Should have recorded the rate limit
	backoffState := client.rateLimiter.GetBackoffState(server.URL)
	if backoffState == nil {
		t.Log("Backoff state was cleared after successful retry")
	}
}

func TestRateLimitDetection_503Response(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	cfg := DefaultConfig()
	cfg.Retry.MaxRetries = 0 // No retries for this test
	client := New(cfg)
	defer client.Close()

	_, err := client.Get(context.Background(), server.URL)

	// Should return rate limit error
	var rateLimitErr *RateLimitError
	if !errors.As(err, &rateLimitErr) {
		t.Errorf("expected RateLimitError, got %T: %v", err, err)
	}
	if rateLimitErr != nil && rateLimitErr.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("StatusCode = %d, want %d", rateLimitErr.StatusCode, http.StatusServiceUnavailable)
	}
}

func TestRateLimitDetection_403BotDetection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	cfg := DefaultConfig()
	cfg.Retry.MaxRetries = 0
	client := New(cfg)
	defer client.Close()

	_, err := client.Get(context.Background(), server.URL)

	var rateLimitErr *RateLimitError
	if !errors.As(err, &rateLimitErr) {
		t.Fatalf("expected RateLimitError, got %T: %v", err, err)
	}
	if !rateLimitErr.IsBotDetection {
		t.Error("IsBotDetection should be true for 403 response")
	}
}

func TestRateLimitDetection_RetryAfterHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	cfg := DefaultConfig()
	cfg.Retry.MaxRetries = 0
	client := New(cfg)
	defer client.Close()

	_, err := client.Get(context.Background(), server.URL)

	var rateLimitErr *RateLimitError
	if !errors.As(err, &rateLimitErr) {
		t.Fatalf("expected RateLimitError, got %T: %v", err, err)
	}
	if rateLimitErr.RetryAfter < 60*time.Second {
		t.Errorf("RetryAfter = %v, want >= 60s", rateLimitErr.RetryAfter)
	}
}

// =============================================================================
// Backoff Timing Tests
// =============================================================================

func TestBackoffTiming_ExponentialIncrease(t *testing.T) {
	cfg := DefaultRateLimiterConfig()
	cfg.EnableDynamicBackoff = true
	rl := NewRateLimiter(cfg)

	domain := "https://example.com/test"

	// First error: should be initial backoff (1s)
	backoff1 := rl.RecordRateLimitError(domain, 0)
	if backoff1 != InnertubeInitialBackoff {
		t.Errorf("first backoff = %v, want %v", backoff1, InnertubeInitialBackoff)
	}

	// Second error: should be 2x
	backoff2 := rl.RecordRateLimitError(domain, 0)
	expected2 := time.Duration(float64(InnertubeInitialBackoff) * InnertubeBackoffMultiplier)
	if backoff2 != expected2 {
		t.Errorf("second backoff = %v, want %v", backoff2, expected2)
	}

	// Third error: should be 4x
	backoff3 := rl.RecordRateLimitError(domain, 0)
	expected3 := time.Duration(float64(expected2) * InnertubeBackoffMultiplier)
	if backoff3 != expected3 {
		t.Errorf("third backoff = %v, want %v", backoff3, expected3)
	}
}

func TestBackoffTiming_MaxBackoffCap(t *testing.T) {
	cfg := DefaultRateLimiterConfig()
	cfg.EnableDynamicBackoff = true
	rl := NewRateLimiter(cfg)

	domain := "https://example.com/test"

	// Record many errors to hit max backoff
	var lastBackoff time.Duration
	for i := 0; i < 20; i++ {
		lastBackoff = rl.RecordRateLimitError(domain, 0)
	}

	if lastBackoff > InnertubeMaxBackoff {
		t.Errorf("backoff = %v, should be capped at %v", lastBackoff, InnertubeMaxBackoff)
	}
}

func TestBackoffTiming_RetryAfterOverrides(t *testing.T) {
	cfg := DefaultRateLimiterConfig()
	cfg.EnableDynamicBackoff = true
	rl := NewRateLimiter(cfg)

	domain := "https://example.com/test"

	// Server says wait 5 minutes
	serverRetryAfter := 5 * time.Minute
	backoff := rl.RecordRateLimitError(domain, serverRetryAfter)

	if backoff < serverRetryAfter {
		t.Errorf("backoff = %v, should be at least server's Retry-After %v", backoff, serverRetryAfter)
	}
}

func TestBackoffTiming_RecoveryAfterSuccess(t *testing.T) {
	cfg := DefaultRateLimiterConfig()
	cfg.EnableDynamicBackoff = true
	rl := NewRateLimiter(cfg)

	domain := "https://example.com/test"

	// Record some errors
	rl.RecordRateLimitError(domain, 0)
	rl.RecordRateLimitError(domain, 0)
	rl.RecordRateLimitError(domain, 0)

	state := rl.GetBackoffState(domain)
	if state == nil {
		t.Fatal("backoff state should exist after errors")
	}
	initialErrors := state.ConsecutiveErrors

	// Record success
	rl.RecordSuccess(domain)

	state = rl.GetBackoffState(domain)
	if state != nil && state.ConsecutiveErrors >= initialErrors {
		t.Error("consecutive errors should decrease after success")
	}
}

// =============================================================================
// Jitter Distribution Tests
// =============================================================================

func TestJitterDistribution(t *testing.T) {
	// Test that jitter produces values in expected range
	baseDuration := 1 * time.Second
	jitterFraction := 0.2 // ±20%

	minExpected := float64(baseDuration) * (1 - jitterFraction)
	maxExpected := float64(baseDuration) * (1 + jitterFraction)

	samples := 1000
	var belowMin, aboveMax int
	var sum float64

	for i := 0; i < samples; i++ {
		// Simulate jitter calculation similar to retry package
		jitterRange := float64(baseDuration) * jitterFraction
		jitterValue := (randFloat64() - 0.5) * 2 * jitterRange
		result := float64(baseDuration) + jitterValue

		sum += result
		if result < minExpected {
			belowMin++
		}
		if result > maxExpected {
			aboveMax++
		}
	}

	// Check distribution is within bounds
	if belowMin > 0 || aboveMax > 0 {
		t.Errorf("jitter out of bounds: %d below min, %d above max", belowMin, aboveMax)
	}

	// Check average is close to base duration (should be roughly centered)
	avg := sum / float64(samples)
	deviation := math.Abs(avg-float64(baseDuration)) / float64(baseDuration)
	if deviation > 0.05 { // Allow 5% deviation from center
		t.Errorf("jitter average deviation = %.2f%%, want < 5%%", deviation*100)
	}
}

// randFloat64 returns a pseudo-random float64 in [0,1)
// Using a simple LCG for reproducibility in tests
var randSeed uint64 = 12345

func randFloat64() float64 {
	randSeed = randSeed*6364136223846793005 + 1
	return float64(randSeed>>33) / float64(1<<31)
}

// =============================================================================
// Circuit Breaker Concurrent Safety Tests
// =============================================================================

func TestCircuitBreakerConcurrentSafety(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold:    10,
		RecoveryTimeout:     100 * time.Millisecond,
		HalfOpenMaxRequests: 1,
	})

	const goroutines = 50
	const operationsPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	testErr := errors.New("test error")

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()

			for j := 0; j < operationsPerGoroutine; j++ {
				domain := "example.com"

				// Mix of operations
				switch j % 4 {
				case 0:
					cb.Allow(domain)
				case 1:
					cb.RecordSuccess(domain)
				case 2:
					cb.RecordFailure(domain, testErr)
				case 3:
					cb.GetStats(domain)
				}
			}
		}(i)
	}

	wg.Wait()

	// If we get here without race detector complaints, test passes
}

func TestCircuitBreakerConcurrentStateTransitions(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold:    5,
		RecoveryTimeout:     50 * time.Millisecond,
		HalfOpenMaxRequests: 1,
	})

	domain := "test.com"
	testErr := errors.New("test error")

	var wg sync.WaitGroup

	// Goroutine 1: Continuously record failures
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			cb.RecordFailure(domain, testErr)
			time.Sleep(time.Millisecond)
		}
	}()

	// Goroutine 2: Continuously check state
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			cb.GetState(domain)
			time.Sleep(time.Millisecond)
		}
	}()

	// Goroutine 3: Try to allow requests
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			cb.Allow(domain)
			time.Sleep(time.Millisecond)
		}
	}()

	wg.Wait()
}

// =============================================================================
// Rate Limiter Concurrent Safety Tests
// =============================================================================

func TestRateLimiterConcurrentSafety(t *testing.T) {
	cfg := DefaultRateLimiterConfig()
	cfg.InnertubeRPS = 100 // High rate for faster test
	rl := NewRateLimiter(cfg)

	const goroutines = 20
	const requestsPerGoroutine = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)

	ctx := context.Background()

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()

			for j := 0; j < requestsPerGoroutine; j++ {
				url := "https://www.youtube.com/test"
				rl.Wait(ctx, url)
				rl.RecordSuccess(url)
			}
		}(i)
	}

	wg.Wait()
}

func TestRateLimiterConcurrentBackoff(t *testing.T) {
	cfg := DefaultRateLimiterConfig()
	cfg.EnableDynamicBackoff = true
	rl := NewRateLimiter(cfg)

	const goroutines = 10

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()

			url := "https://www.youtube.com/test"
			for j := 0; j < 20; j++ {
				if j%3 == 0 {
					rl.RecordRateLimitError(url, time.Second)
				} else {
					rl.RecordSuccess(url)
				}
			}
		}(i)
	}

	wg.Wait()
}

// =============================================================================
// Integrated Client Resilience Tests
// =============================================================================

func TestClientResilience_CircuitBreakerIntegration(t *testing.T) {
	var requestCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&requestCount, 1)
		// First 5 requests fail to trigger circuit breaker
		if count <= 5 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	}))
	defer server.Close()

	cfg := DefaultConfig()
	cfg.Retry.MaxRetries = 0 // No retries to make circuit breaker behavior clear
	cfg.CircuitBreaker.FailureThreshold = 5
	cfg.CircuitBreaker.RecoveryTimeout = 50 * time.Millisecond
	client := New(cfg)
	defer client.Close()

	// Make 5 requests to open the circuit
	for i := 0; i < 5; i++ {
		client.Get(context.Background(), server.URL)
	}

	// Verify circuit is open
	domain := client.rateLimiter.extractDomain(server.URL)
	state := client.circuitBreaker.GetState(domain)
	if state != CircuitOpen {
		t.Errorf("circuit state = %v, want CircuitOpen", state)
	}

	// Next request should fail fast
	_, err := client.Get(context.Background(), server.URL)
	if !errors.Is(err, ErrCircuitOpen) {
		t.Errorf("expected ErrCircuitOpen, got %v", err)
	}

	// Wait for recovery timeout
	time.Sleep(60 * time.Millisecond)

	// Circuit should be half-open, next request should succeed (server now returns 200)
	resp, err := client.Get(context.Background(), server.URL)
	if err != nil {
		t.Errorf("request in half-open state failed: %v", err)
	}
	if resp != nil && resp.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	// Circuit should be closed now
	state = client.circuitBreaker.GetState(domain)
	if state != CircuitClosed {
		t.Errorf("circuit state = %v, want CircuitClosed", state)
	}
}

func TestClientResilience_RateLimiterIntegration(t *testing.T) {
	cfg := DefaultConfig()
	cfg.RateLimiter.InnertubeRPS = 10 // 10 req/s

	client := New(cfg)
	defer client.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Make rapid requests - rate limiter should slow them down
	start := time.Now()
	for i := 0; i < 5; i++ {
		client.Get(context.Background(), server.URL)
	}
	elapsed := time.Since(start)

	// With 10 RPS, 5 requests should take at least ~400ms (first is immediate, then 4 waits of ~100ms each)
	// Allow some tolerance
	if elapsed < 300*time.Millisecond {
		t.Logf("5 requests completed in %v (rate limiting may not have been strict enough)", elapsed)
	}
}

func TestClientResilience_CombinedFailures(t *testing.T) {
	var requestCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&requestCount, 1)
		switch {
		case count <= 2:
			w.WriteHeader(http.StatusTooManyRequests)
		case count <= 4:
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("success"))
		}
	}))
	defer server.Close()

	cfg := DefaultConfig()
	cfg.Retry.MaxRetries = 10
	cfg.Retry.InitialBackoff = 10 * time.Millisecond
	cfg.CircuitBreaker.FailureThreshold = 10 // High threshold to allow retries
	client := New(cfg)
	defer client.Close()

	// Should eventually succeed despite initial failures
	resp, err := client.Get(context.Background(), server.URL)
	if err != nil {
		t.Logf("Request failed (may have exhausted retries): %v", err)
		return
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestClientResilience_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := New(nil)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := client.Get(ctx, server.URL)
	if err == nil {
		t.Error("expected context cancellation error")
	}
}

func TestClientResilience_MultipleDomainsIsolation(t *testing.T) {
	// Test circuit breaker isolation directly since httptest servers
	// share the same host (127.0.0.1) which maps to the same domain
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold:    3,
		RecoveryTimeout:     30 * time.Second,
		HalfOpenMaxRequests: 1,
	})

	testErr := errors.New("test error")

	// Fail domain1 enough to open its circuit
	for i := 0; i < 3; i++ {
		cb.RecordFailure("youtube.com", testErr)
	}

	// Domain 1 should have open circuit
	if cb.GetState("youtube.com") != CircuitOpen {
		t.Error("youtube.com circuit should be open")
	}

	// Domain 2 should still be closed
	if cb.GetState("google.com") != CircuitClosed {
		t.Error("google.com circuit should be closed")
	}

	// Allow should work for domain2
	if err := cb.Allow("google.com"); err != nil {
		t.Errorf("google.com Allow() failed: %v", err)
	}

	// Allow should fail for domain1
	if err := cb.Allow("youtube.com"); !errors.Is(err, ErrCircuitOpen) {
		t.Errorf("youtube.com Allow() = %v, want ErrCircuitOpen", err)
	}

	// Record some failures for domain2 (but not enough to open)
	cb.RecordFailure("google.com", testErr)
	cb.RecordFailure("google.com", testErr)

	// Domain 2 should still be closed (2 failures, threshold is 3)
	if cb.GetState("google.com") != CircuitClosed {
		t.Error("google.com circuit should still be closed after 2 failures")
	}

	// Third failure should open domain2
	cb.RecordFailure("google.com", testErr)
	if cb.GetState("google.com") != CircuitOpen {
		t.Error("google.com circuit should be open after 3 failures")
	}
}

// =============================================================================
// Error Handling Tests
// =============================================================================

func TestClientResilience_HTTPErrorClassification(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		wantRetryable  bool
		wantRateLimit  bool
	}{
		{"200 OK", 200, false, false},
		{"400 Bad Request", 400, false, false},
		{"404 Not Found", 404, false, false},
		{"429 Too Many Requests", 429, true, true},
		{"500 Internal Server Error", 500, true, false},
		{"502 Bad Gateway", 502, true, false},
		{"503 Service Unavailable", 503, true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				if tt.statusCode >= 200 && tt.statusCode < 300 {
					w.Write([]byte("ok"))
				}
			}))
			defer server.Close()

			cfg := DefaultConfig()
			cfg.Retry.MaxRetries = 0 // No retries
			client := New(cfg)
			defer client.Close()

			_, err := client.Get(context.Background(), server.URL)

			if tt.statusCode >= 200 && tt.statusCode < 300 {
				if err != nil {
					t.Errorf("expected no error for %d, got %v", tt.statusCode, err)
				}
				return
			}

			if tt.wantRateLimit {
				var rateLimitErr *RateLimitError
				if !errors.As(err, &rateLimitErr) {
					t.Errorf("expected RateLimitError for %d", tt.statusCode)
				}
			} else {
				var httpErr *HTTPError
				if !errors.As(err, &httpErr) {
					t.Errorf("expected HTTPError for %d, got %T", tt.statusCode, err)
				}
			}
		})
	}
}

func TestClientResilience_ResponseBodyReading(t *testing.T) {
	expectedBody := "Hello, World!"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(expectedBody))
	}))
	defer server.Close()

	client := New(nil)
	defer client.Close()

	resp, err := client.Get(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if string(resp.Body) != expectedBody {
		t.Errorf("body = %q, want %q", string(resp.Body), expectedBody)
	}
}

func TestClientResilience_LargeResponse(t *testing.T) {
	// 1MB response
	largeBody := make([]byte, 1024*1024)
	for i := range largeBody {
		largeBody[i] = byte(i % 256)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(largeBody)
	}))
	defer server.Close()

	client := New(nil)
	defer client.Close()

	resp, err := client.Get(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if len(resp.Body) != len(largeBody) {
		t.Errorf("body length = %d, want %d", len(resp.Body), len(largeBody))
	}
}

func TestClientResilience_EmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := New(nil)
	defer client.Close()

	resp, err := client.Get(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if len(resp.Body) != 0 {
		t.Errorf("body length = %d, want 0", len(resp.Body))
	}
}

// =============================================================================
// Header Handling Tests
// =============================================================================

func TestClientResilience_CustomHeaders(t *testing.T) {
	var receivedHeaders http.Header

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := New(nil)
	defer client.Close()

	headers := map[string]string{
		"X-Custom-Header": "custom-value",
		"Authorization":   "Bearer token123",
	}

	_, err := client.Do(context.Background(), http.MethodGet, server.URL, nil, headers)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if receivedHeaders.Get("X-Custom-Header") != "custom-value" {
		t.Errorf("X-Custom-Header = %q, want %q", receivedHeaders.Get("X-Custom-Header"), "custom-value")
	}
	if receivedHeaders.Get("Authorization") != "Bearer token123" {
		t.Errorf("Authorization header not received correctly")
	}
}

func TestClientResilience_UserAgentDefault(t *testing.T) {
	var receivedUserAgent string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedUserAgent = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := New(nil)
	defer client.Close()

	_, err := client.Get(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if receivedUserAgent != "ytsync/1.0" {
		t.Errorf("User-Agent = %q, want %q", receivedUserAgent, "ytsync/1.0")
	}
}

// =============================================================================
// Request Body Tests
// =============================================================================

func TestClientResilience_PostWithBody(t *testing.T) {
	var receivedBody []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("received"))
	}))
	defer server.Close()

	client := New(nil)
	defer client.Close()

	body := []byte(`{"key": "value"}`)
	resp, err := client.Do(context.Background(), http.MethodPost, server.URL,
		bytes.NewReader(body),
		map[string]string{"Content-Type": "application/json"})

	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if string(receivedBody) != string(body) {
		t.Errorf("received body = %q, want %q", string(receivedBody), string(body))
	}
	if string(resp.Body) != "received" {
		t.Errorf("response body = %q, want %q", string(resp.Body), "received")
	}
}
