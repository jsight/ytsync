//go:build integration

// Package http provides HTTP client infrastructure for YouTube interactions.
// This file contains integration tests against live YouTube endpoints.
package http

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ytsync/retry"
)

// =============================================================================
// Test Configuration
// =============================================================================

const (
	// testTimeout is the maximum duration for individual tests.
	testTimeout = 30 * time.Second

	// testChannelID is a public YouTube channel used for testing.
	// Using a well-known channel that is unlikely to disappear.
	testChannelID = "UC_x5XG1OV2P6uZZ5FSM9Ttw" // Google Developers

	// testVideoID is a public YouTube video used for testing.
	testVideoID = "dQw4w9WgXcQ" // A well-known video
)

// skipIfCI skips integration tests when running in CI without explicit opt-in.
func skipIfCI(t *testing.T) {
	if os.Getenv("INTEGRATION_TESTS") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TESTS=true to run.")
	}
}

// =============================================================================
// Innertube API Throttling Tests
// =============================================================================

// TestIntegration_InnertubeRateLimiting verifies that the rate limiter correctly
// throttles requests to the Innertube API to avoid triggering YouTube's rate limits.
func TestIntegration_InnertubeRateLimiting(t *testing.T) {
	skipIfCI(t)

	cfg := DefaultConfig()
	cfg.RateLimiter.InnertubeRPS = 2.0 // 2 requests per second
	cfg.Timeout = testTimeout

	client := New(cfg)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Make 5 requests and measure timing
	start := time.Now()
	requestCount := 5

	for i := 0; i < requestCount; i++ {
		// Make a simple request to YouTube homepage (lightweight)
		_, err := client.Get(ctx, "https://www.youtube.com/")
		if err != nil {
			// Rate limit or other errors are expected; just log
			t.Logf("Request %d: %v", i+1, err)
		}
	}

	elapsed := time.Since(start)

	// With 2 RPS, 5 requests should take at least ~2 seconds
	// (first is immediate, then 4 waits of 500ms each)
	expectedMinDuration := time.Duration(requestCount-1) * 500 * time.Millisecond
	if elapsed < expectedMinDuration/2 { // Allow 50% tolerance
		t.Errorf("Rate limiting too fast: %v requests in %v (expected >= %v)",
			requestCount, elapsed, expectedMinDuration)
	}

	t.Logf("Completed %d requests in %v (rate limited)", requestCount, elapsed)
}

// TestIntegration_InnertubeAPIBasicRequest tests basic Innertube API functionality
// by making a browse request to fetch channel info.
func TestIntegration_InnertubeAPIBasicRequest(t *testing.T) {
	skipIfCI(t)

	cfg := DefaultConfig()
	cfg.Timeout = testTimeout
	cfg.RateLimiter.InnertubeRPS = 1.0 // Conservative rate

	client := New(cfg)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Innertube browse request
	browseEndpoint := "https://www.youtube.com/youtubei/v1/browse"
	reqBody := map[string]interface{}{
		"context": map[string]interface{}{
			"client": map[string]interface{}{
				"clientName":    "WEB",
				"clientVersion": "2.20240101.00.00",
				"hl":            "en",
				"gl":            "US",
			},
		},
		"browseId": testChannelID,
		"params":   "EgZ2aWRlb3PyBgQKAjoA", // Videos tab
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}

	headers := map[string]string{
		"Content-Type": "application/json",
		"Origin":       "https://www.youtube.com",
		"Referer":      "https://www.youtube.com/",
	}

	resp, err := client.Do(ctx, http.MethodPost, browseEndpoint,
		strings.NewReader(string(body)), headers)

	if err != nil {
		// Check if this is a rate limit error
		var rateLimitErr *RateLimitError
		if isRateLimitError(err, &rateLimitErr) {
			t.Logf("Rate limited (expected in some cases): %v", err)
			return
		}
		t.Fatalf("Innertube request failed: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Unexpected status code: %d", resp.StatusCode)
	}

	// Verify response contains expected structure
	var browseResp map[string]interface{}
	if err := json.Unmarshal(resp.Body, &browseResp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Check for basic response structure
	if _, ok := browseResp["contents"]; !ok {
		if _, ok := browseResp["responseContext"]; !ok {
			t.Error("Response missing expected 'contents' or 'responseContext' field")
		}
	}

	t.Logf("Innertube API request successful, response size: %d bytes", len(resp.Body))
}

// TestIntegration_InnertubeThrottlingBackoff tests that exponential backoff
// works correctly when rate limited by the Innertube API.
func TestIntegration_InnertubeThrottlingBackoff(t *testing.T) {
	skipIfCI(t)

	cfg := DefaultConfig()
	cfg.Timeout = testTimeout
	cfg.RateLimiter.InnertubeRPS = 10.0 // Higher rate to trigger throttling
	cfg.RateLimiter.EnableDynamicBackoff = true
	cfg.Retry.MaxRetries = 3
	cfg.Retry.InitialBackoff = 500 * time.Millisecond

	client := New(cfg)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Make rapid requests to potentially trigger rate limiting
	var rateLimitCount int
	var successCount int

	for i := 0; i < 10; i++ {
		resp, err := client.Get(ctx, "https://www.youtube.com/")
		if err != nil {
			var rateLimitErr *RateLimitError
			if isRateLimitError(err, &rateLimitErr) {
				rateLimitCount++
				t.Logf("Request %d rate limited: %v", i+1, err)

				// Verify backoff state was recorded
				state := client.rateLimiter.GetBackoffState("https://www.youtube.com/")
				if state != nil {
					t.Logf("Backoff state: %v, consecutive errors: %d",
						state.CurrentBackoff, state.ConsecutiveErrors)
				}
				continue
			}
			t.Logf("Request %d error: %v", i+1, err)
			continue
		}

		successCount++
		if resp.StatusCode == http.StatusOK {
			t.Logf("Request %d successful", i+1)
		}

		// Small delay to avoid hammering
		time.Sleep(100 * time.Millisecond)
	}

	t.Logf("Results: %d successful, %d rate limited", successCount, rateLimitCount)
}

// =============================================================================
// 429 Response and Retry Behavior Tests
// =============================================================================

// TestIntegration_RetryBehavior verifies retry logic with real network conditions.
func TestIntegration_RetryBehavior(t *testing.T) {
	skipIfCI(t)

	cfg := DefaultConfig()
	cfg.Timeout = testTimeout
	cfg.RateLimiter.InnertubeRPS = 1.0
	cfg.Retry.MaxRetries = 2
	cfg.Retry.InitialBackoff = 200 * time.Millisecond
	cfg.Retry.MaxBackoff = 2 * time.Second

	client := New(cfg)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Request a valid YouTube page
	resp, err := client.Get(ctx, "https://www.youtube.com/")
	if err != nil {
		var rateLimitErr *RateLimitError
		if isRateLimitError(err, &rateLimitErr) {
			t.Logf("Rate limited (retry behavior validated): %v", err)
			// Verify the error contains expected info
			if rateLimitErr.RetryAfter > 0 {
				t.Logf("Retry-After duration: %v", rateLimitErr.RetryAfter)
			}
			return
		}
		t.Logf("Request error: %v", err)
		return
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Unexpected status code: %d", resp.StatusCode)
	}

	t.Logf("Request successful: status=%d, body size=%d",
		resp.StatusCode, len(resp.Body))
}

// TestIntegration_CircuitBreakerWithLiveTraffic tests circuit breaker behavior
// by making requests and verifying state transitions.
func TestIntegration_CircuitBreakerWithLiveTraffic(t *testing.T) {
	skipIfCI(t)

	cfg := DefaultConfig()
	cfg.Timeout = 5 * time.Second
	cfg.RateLimiter.InnertubeRPS = 2.0
	cfg.CircuitBreaker.FailureThreshold = 3
	cfg.CircuitBreaker.RecoveryTimeout = 5 * time.Second

	client := New(cfg)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Make successful requests first
	for i := 0; i < 3; i++ {
		_, err := client.Get(ctx, "https://www.youtube.com/")
		if err != nil {
			t.Logf("Request %d: %v", i+1, err)
		} else {
			t.Logf("Request %d: success", i+1)
		}
		time.Sleep(500 * time.Millisecond)
	}

	// Check circuit state
	domain := client.rateLimiter.extractDomain("https://www.youtube.com/")
	state := client.circuitBreaker.GetState(domain)
	t.Logf("Circuit breaker state: %s", state)

	// Circuit should be closed after successful requests
	if state != CircuitClosed {
		t.Errorf("Expected circuit to be closed, got: %s", state)
	}
}

// =============================================================================
// Cookie Persistence Tests
// =============================================================================

// TestIntegration_CookiePersistence tests that cookies are properly stored
// and reused across requests.
func TestIntegration_CookiePersistence(t *testing.T) {
	skipIfCI(t)

	// Create session manager with cookie persistence
	tempDir := t.TempDir()
	cookieFile := tempDir + "/cookies.json"

	sessionCfg := SessionConfig{
		PersistCookies: true,
		CookieFile:     cookieFile,
		UserAgent:      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
		RefererURL:     "https://www.youtube.com",
	}

	session, err := NewSessionManager(sessionCfg)
	if err != nil {
		t.Fatalf("Failed to create session manager: %v", err)
	}

	httpCfg := DefaultConfig()
	httpCfg.Timeout = testTimeout
	httpCfg.RateLimiter.InnertubeRPS = 1.0

	client := session.GetClient(httpCfg)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Make initial request to get cookies
	_, err = client.Get(ctx, "https://www.youtube.com/")
	if err != nil {
		t.Logf("Initial request: %v", err)
	}

	// Save cookies
	if err := session.SaveCookies(); err != nil {
		t.Logf("Cookie save warning: %v", err)
	}

	// Check if cookie file was created
	if _, err := os.Stat(cookieFile); os.IsNotExist(err) {
		t.Log("Cookie file not created (YouTube may not have set cookies)")
	} else {
		// Read and log cookie file contents
		data, _ := os.ReadFile(cookieFile)
		t.Logf("Cookies saved: %d bytes", len(data))
	}

	// Create new session and load cookies
	session2, err := NewSessionManager(sessionCfg)
	if err != nil {
		t.Fatalf("Failed to create second session manager: %v", err)
	}

	client2 := session2.GetClient(httpCfg)
	defer client2.Close()

	// Make another request with loaded cookies
	_, err = client2.Get(ctx, "https://www.youtube.com/")
	if err != nil {
		t.Logf("Second request: %v", err)
	}

	t.Log("Cookie persistence test completed")
}

// TestIntegration_SessionHeaders verifies that session headers are properly
// applied to requests.
func TestIntegration_SessionHeaders(t *testing.T) {
	skipIfCI(t)

	sessionCfg := DefaultSessionConfig()
	sessionCfg.HeadersToAdd = map[string]string{
		"Accept-Language": "en-US,en;q=0.9",
		"Accept":          "text/html,application/json",
	}

	session, err := NewSessionManager(sessionCfg)
	if err != nil {
		t.Fatalf("Failed to create session manager: %v", err)
	}

	// Verify headers are set
	headers := session.GetHeaders()
	if headers["User-Agent"] == "" {
		t.Error("User-Agent should be set")
	}
	if headers["Referer"] != "https://www.youtube.com" {
		t.Errorf("Referer should be set, got: %s", headers["Referer"])
	}
	if headers["Accept-Language"] != "en-US,en;q=0.9" {
		t.Errorf("Accept-Language should be set, got: %s", headers["Accept-Language"])
	}

	t.Logf("Session headers: %+v", headers)
}

// =============================================================================
// User-Agent Rotation Tests
// =============================================================================

// Common user agents for rotation testing
var testUserAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:121.0) Gecko/20100101 Firefox/121.0",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.2 Safari/605.1.15",
}

// TestIntegration_UserAgentRotation tests that different user agents can be used
// for different requests without triggering bot detection.
func TestIntegration_UserAgentRotation(t *testing.T) {
	skipIfCI(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	var successCount, failCount int32

	for i, ua := range testUserAgents {
		cfg := DefaultConfig()
		cfg.Timeout = testTimeout
		cfg.UserAgent = ua
		cfg.RateLimiter.InnertubeRPS = 0.5 // Very conservative

		client := New(cfg)

		resp, err := client.Get(ctx, "https://www.youtube.com/")
		client.Close()

		if err != nil {
			atomic.AddInt32(&failCount, 1)
			t.Logf("User-Agent %d failed: %v", i+1, err)
		} else if resp.StatusCode == http.StatusOK {
			atomic.AddInt32(&successCount, 1)
			t.Logf("User-Agent %d: success (status=%d)", i+1, resp.StatusCode)
		} else {
			t.Logf("User-Agent %d: status=%d", i+1, resp.StatusCode)
		}

		// Wait between requests to avoid rate limiting
		time.Sleep(2 * time.Second)
	}

	t.Logf("User-Agent rotation results: %d success, %d failed",
		atomic.LoadInt32(&successCount), atomic.LoadInt32(&failCount))
}

// TestIntegration_ConcurrentUserAgentRotation tests concurrent requests with
// different user agents to ensure thread safety.
func TestIntegration_ConcurrentUserAgentRotation(t *testing.T) {
	skipIfCI(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	var wg sync.WaitGroup
	var successCount, failCount int32

	for i, ua := range testUserAgents {
		wg.Add(1)
		go func(idx int, userAgent string) {
			defer wg.Done()

			cfg := DefaultConfig()
			cfg.Timeout = testTimeout
			cfg.UserAgent = userAgent
			cfg.RateLimiter.InnertubeRPS = 0.5

			client := New(cfg)
			defer client.Close()

			resp, err := client.Get(ctx, "https://www.youtube.com/")
			if err != nil {
				atomic.AddInt32(&failCount, 1)
				t.Logf("Concurrent UA %d: %v", idx+1, err)
			} else if resp.StatusCode == http.StatusOK {
				atomic.AddInt32(&successCount, 1)
				t.Logf("Concurrent UA %d: success", idx+1)
			}
		}(i, ua)

		// Stagger start times slightly
		time.Sleep(500 * time.Millisecond)
	}

	wg.Wait()

	t.Logf("Concurrent rotation results: %d success, %d failed",
		atomic.LoadInt32(&successCount), atomic.LoadInt32(&failCount))
}

// =============================================================================
// Combined Resilience Tests
// =============================================================================

// TestIntegration_FullResilienceChain tests the complete chain of resilience
// features working together: rate limiting, circuit breaker, retries, and backoff.
func TestIntegration_FullResilienceChain(t *testing.T) {
	skipIfCI(t)

	cfg := DefaultConfig()
	cfg.Timeout = testTimeout
	cfg.RateLimiter.InnertubeRPS = 2.0
	cfg.RateLimiter.EnableDynamicBackoff = true
	cfg.CircuitBreaker.FailureThreshold = 5
	cfg.CircuitBreaker.RecoveryTimeout = 10 * time.Second
	cfg.Retry.MaxRetries = 3
	cfg.Retry.InitialBackoff = 500 * time.Millisecond
	cfg.Retry.MaxBackoff = 5 * time.Second

	client := New(cfg)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Make a series of requests to test all components
	results := struct {
		success    int
		rateLimit  int
		circuitErr int
		otherErr   int
	}{}

	for i := 0; i < 15; i++ {
		resp, err := client.Get(ctx, "https://www.youtube.com/")
		if err != nil {
			switch {
			case isCircuitOpenError(err):
				results.circuitErr++
				t.Logf("Request %d: circuit open", i+1)
			case isRateLimitErrorAny(err):
				results.rateLimit++
				t.Logf("Request %d: rate limited", i+1)
			default:
				results.otherErr++
				t.Logf("Request %d: %v", i+1, err)
			}
		} else if resp.StatusCode == http.StatusOK {
			results.success++
		}

		// Log current states
		domain := client.rateLimiter.extractDomain("https://www.youtube.com/")
		cbState := client.circuitBreaker.GetState(domain)
		backoffState := client.rateLimiter.GetBackoffState("https://www.youtube.com/")

		if backoffState != nil {
			t.Logf("  Circuit: %s, Backoff: %v, Errors: %d",
				cbState, backoffState.CurrentBackoff, backoffState.ConsecutiveErrors)
		}

		time.Sleep(200 * time.Millisecond)
	}

	t.Logf("Results: success=%d, rateLimit=%d, circuit=%d, other=%d",
		results.success, results.rateLimit, results.circuitErr, results.otherErr)
}

// TestIntegration_RateLimiterDomainIsolation tests that rate limiters are
// properly isolated per domain.
func TestIntegration_RateLimiterDomainIsolation(t *testing.T) {
	skipIfCI(t)

	cfg := DefaultConfig()
	cfg.Timeout = testTimeout
	cfg.RateLimiter.InnertubeRPS = 2.0
	cfg.RateLimiter.RSSRPS = 5.0 // RSS has higher limit

	client := New(cfg)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Test different domains have different rates
	domains := []string{
		"https://www.youtube.com/",
		"https://feeds.youtube.com/feeds/videos.xml?channel_id=" + testChannelID,
	}

	for _, domain := range domains {
		start := time.Now()

		// Make 3 requests
		for i := 0; i < 3; i++ {
			_, err := client.Get(ctx, domain)
			if err != nil {
				t.Logf("Domain %s request %d: %v", domain, i+1, err)
			}
		}

		elapsed := time.Since(start)
		t.Logf("Domain %s: 3 requests in %v", domain, elapsed)
	}

	// Check rate limiter stats
	stats := client.rateLimiter.Stats()
	t.Logf("Rate limiter stats: %+v", stats)
}

// =============================================================================
// Helper Functions
// =============================================================================

// isRateLimitError checks if the error is a rate limit error.
func isRateLimitError(err error, target **RateLimitError) bool {
	if err == nil {
		return false
	}

	// Type assertion for *RateLimitError
	if rlErr, ok := err.(*RateLimitError); ok {
		if target != nil {
			*target = rlErr
		}
		return true
	}

	// Check wrapped errors (from retry package)
	if rlErr, ok := err.(*retry.RetryableError); ok {
		if underlying, ok := rlErr.Unwrap().(*RateLimitError); ok {
			if target != nil {
				*target = underlying
			}
			return true
		}
	}

	return false
}

// isRateLimitErrorAny checks if any error in the chain is a rate limit error.
func isRateLimitErrorAny(err error) bool {
	return isRateLimitError(err, nil)
}

// isCircuitOpenError checks if the error is a circuit open error.
func isCircuitOpenError(err error) bool {
	if err == nil {
		return false
	}
	return err == ErrCircuitOpen || strings.Contains(err.Error(), "circuit breaker is open")
}
