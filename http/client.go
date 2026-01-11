// Package http provides HTTP client infrastructure for YouTube interactions
// with built-in retry logic, rate limiting, and error handling.
package http

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
	"ytsync/retry"
)

// Client wraps an HTTP client with retry logic and rate limit handling.
type Client struct {
	base   *http.Client
	config *Config
}

// Config holds HTTP client configuration including retry and rate limit settings.
type Config struct {
	// Timeout for individual HTTP requests
	Timeout time.Duration

	// Retry configuration
	Retry retry.Config

	// Maximum concurrent requests
	MaxConcurrent int

	// User agent for HTTP requests
	UserAgent string
}

// DefaultConfig returns sensible defaults for HTTP client configuration.
func DefaultConfig() *Config {
	return &Config{
		Timeout:       30 * time.Second,
		Retry:         retry.DefaultConfig(),
		MaxConcurrent: 10,
		UserAgent:     "ytsync/1.0",
	}
}

// New creates a new HTTP client with the given configuration.
func New(cfg *Config) *Client {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	base := &http.Client{
		Timeout: cfg.Timeout,
	}

	return &Client{
		base:   base,
		config: cfg,
	}
}

// Response represents an HTTP response with status code and body.
type Response struct {
	StatusCode int
	Header     http.Header
	Body       []byte
}

// Get performs a GET request with retry logic.
func (c *Client) Get(ctx context.Context, url string) (*Response, error) {
	return c.Do(ctx, http.MethodGet, url, nil, nil)
}

// Do performs an HTTP request with retry logic and rate limit handling.
// It automatically retries on transient failures and detects rate limiting.
func (c *Client) Do(ctx context.Context, method, url string, body io.Reader, headers map[string]string) (*Response, error) {
	var lastResp *http.Response

	err := retry.Do(ctx, c.config.Retry, c.isRetryableHTTPError, func(ctx context.Context) error {
		req, err := http.NewRequestWithContext(ctx, method, url, body)
		if err != nil {
			return err
		}

		// Set default user agent
		if req.Header.Get("User-Agent") == "" {
			req.Header.Set("User-Agent", c.config.UserAgent)
		}

		// Apply custom headers
		for k, v := range headers {
			req.Header.Set(k, v)
		}

		resp, err := c.base.Do(req)
		if err != nil {
			return fmt.Errorf("http request failed: %w", err)
		}

		// Check for rate limiting
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable {
			defer resp.Body.Close()
			retryAfter := c.parseRetryAfter(resp.Header)
			return &RateLimitError{
				StatusCode: resp.StatusCode,
				RetryAfter: retryAfter,
			}
		}

		// Non-2xx status codes
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			defer resp.Body.Close()
			bodyBytes, _ := io.ReadAll(resp.Body)
			return &HTTPError{
				StatusCode: resp.StatusCode,
				Body:       bodyBytes,
			}
		}

		lastResp = resp
		return nil
	})

	if err != nil {
		if lastResp != nil {
			lastResp.Body.Close()
		}
		return nil, err
	}

	if lastResp == nil {
		return nil, fmt.Errorf("no response received")
	}

	defer lastResp.Body.Close()
	respBody, err := io.ReadAll(lastResp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	return &Response{
		StatusCode: lastResp.StatusCode,
		Header:     lastResp.Header,
		Body:       respBody,
	}, nil
}

// isRetryableHTTPError determines if an HTTP error is retryable.
func (c *Client) isRetryableHTTPError(err error) bool {
	// Use default retry classifier for generic errors
	if !retry.IsRetryable(err) {
		return false
	}

	// Rate limit errors are retryable
	if _, ok := err.(*RateLimitError); ok {
		return true
	}

	// HTTP errors are retryable if status code is 5xx
	if httpErr, ok := err.(*HTTPError); ok {
		return httpErr.StatusCode >= 500
	}

	return true
}

// parseRetryAfter extracts the Retry-After header value.
// Returns the number of seconds to wait, or 0 if not present.
func (c *Client) parseRetryAfter(header http.Header) time.Duration {
	retryAfter := header.Get("Retry-After")
	if retryAfter == "" {
		return 0
	}

	// Try parsing as seconds (integer)
	if seconds, err := strconv.Atoi(retryAfter); err == nil {
		return time.Duration(seconds) * time.Second
	}

	// Try parsing as HTTP date
	if t, err := http.ParseTime(retryAfter); err == nil {
		return time.Until(t)
	}

	return 0
}

// Close closes the HTTP client connections.
func (c *Client) Close() error {
	c.base.CloseIdleConnections()
	return nil
}
