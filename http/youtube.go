package http

import (
	"fmt"
	"net/http"
	"strings"
)

// YouTubeRateLimitDetector detects YouTube-specific rate limiting signals.
type YouTubeRateLimitDetector struct{}

// NewYouTubeRateLimitDetector creates a new YouTube rate limit detector.
func NewYouTubeRateLimitDetector() *YouTubeRateLimitDetector {
	return &YouTubeRateLimitDetector{}
}

// IsRateLimited checks if the response indicates rate limiting from YouTube.
// It detects:
// - HTTP 429 (Too Many Requests)
// - HTTP 503 (Service Unavailable)
// - HTTP 403 with specific error patterns
// - Custom YouTube rate limit headers
func (d *YouTubeRateLimitDetector) IsRateLimited(statusCode int, header http.Header) bool {
	// Standard rate limit status codes
	if statusCode == http.StatusTooManyRequests || statusCode == http.StatusServiceUnavailable {
		return true
	}

	// YouTube sometimes returns 403 Forbidden with rate limit messages
	if statusCode == http.StatusForbidden {
		// Check for rate limit related headers
		if d.hasRateLimitHeaders(header) {
			return true
		}
	}

	return false
}

// hasRateLimitHeaders checks for custom rate limit headers that YouTube may send.
func (d *YouTubeRateLimitDetector) hasRateLimitHeaders(header http.Header) bool {
	// Standard retry-after header
	if header.Get("Retry-After") != "" {
		return true
	}

	// Check for various rate limit headers
	// When X-RateLimit-Remaining is 0, it indicates rate limiting
	if remaining := header.Get("X-RateLimit-Remaining"); remaining != "" {
		if remaining == "0" {
			return true
		}
	}

	rateLimitHeaders := []string{
		"X-RateLimit-Reset",
		"X-RateLimit-Limit",
	}

	for _, h := range rateLimitHeaders {
		if header.Get(h) != "" {
			return true
		}
	}

	return false
}

// GetRetryAfterDuration extracts retry-after duration from response headers.
// Checks Retry-After, X-RateLimit-Reset, and other timing headers.
func (d *YouTubeRateLimitDetector) GetRetryAfterDuration(header http.Header) int64 {
	// Standard Retry-After header (in seconds as integer)
	if retryAfter := header.Get("Retry-After"); retryAfter != "" {
		if seconds := parseSeconds(retryAfter); seconds > 0 {
			return seconds
		}
	}

	// YouTube-specific headers
	rateLimitHeaders := []string{
		"X-RateLimit-Reset",
		"X-RateLimit-Wait",
	}

	for _, h := range rateLimitHeaders {
		if val := header.Get(h); val != "" {
			if seconds := parseSeconds(val); seconds > 0 {
				return seconds
			}
		}
	}

	// Default retry time if no header specified
	return 60
}

// parseSeconds converts a string to seconds.
// Handles both integer seconds and ISO 8601 durations.
func parseSeconds(s string) int64 {
	// Try parsing as simple integer seconds
	var seconds int64
	if _, err := parseIntString(s, &seconds); err == nil {
		return seconds
	}

	// Could extend this to handle ISO 8601 durations if needed
	// For now, return 0 for unparseable values
	return 0
}

// parseIntString attempts to parse a string as an integer.
func parseIntString(s string, result *int64) (int, error) {
	// Trim whitespace
	s = strings.TrimSpace(s)

	// Simple integer parsing
	var n int64
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return 0, fmt.Errorf("invalid integer: %s", s)
		}
		n = n*10 + int64(ch-'0')
	}

	*result = n
	return int(n), nil
}

// IsClientError checks if status code is a client error (4xx).
func IsClientError(statusCode int) bool {
	return statusCode >= 400 && statusCode < 500
}

// IsServerError checks if status code is a server error (5xx).
func IsServerError(statusCode int) bool {
	return statusCode >= 500 && statusCode < 600
}

// ShouldRetry determines if a request should be retried based on status code.
func ShouldRetry(statusCode int) bool {
	// Retry on server errors
	if IsServerError(statusCode) {
		return true
	}

	// Retry on specific client errors
	retryableClientErrors := []int{
		http.StatusRequestTimeout,      // 408
		http.StatusTooManyRequests,     // 429
		http.StatusServiceUnavailable,  // 503
		http.StatusGatewayTimeout,      // 504
	}

	for _, code := range retryableClientErrors {
		if statusCode == code {
			return true
		}
	}

	return false
}
