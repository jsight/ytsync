package http

import (
	"fmt"
	"time"
)

// RateLimitError indicates the server rate limited the request.
// It includes the status code and optional Retry-After duration.
type RateLimitError struct {
	// StatusCode is the HTTP status code (429, 403, or 503)
	StatusCode int
	// RetryAfter indicates how long to wait before retrying
	RetryAfter time.Duration
	// IsBotDetection indicates this may be anti-bot protection (403)
	IsBotDetection bool
}

// Error returns a string representation of the rate limit error.
func (e *RateLimitError) Error() string {
	if e.IsBotDetection {
		return fmt.Sprintf("bot detection (status %d): retry after %v", e.StatusCode, e.RetryAfter)
	}
	if e.RetryAfter > 0 {
		return fmt.Sprintf("rate limited (status %d): retry after %v", e.StatusCode, e.RetryAfter)
	}
	return fmt.Sprintf("rate limited (status %d)", e.StatusCode)
}

// HTTPError indicates an HTTP error response.
type HTTPError struct {
	// StatusCode is the HTTP status code
	StatusCode int
	// Body is the response body
	Body []byte
}

// Error returns a string representation of the HTTP error.
func (e *HTTPError) Error() string {
	return fmt.Sprintf("http error: status %d", e.StatusCode)
}

// Sentinel errors for HTTP operations.
var (
	// ErrNoResponse indicates no response was received from the server.
	ErrNoResponse = fmt.Errorf("no response received")

	// ErrRequestFailed indicates the request itself failed (network error).
	ErrRequestFailed = fmt.Errorf("http request failed")
)
