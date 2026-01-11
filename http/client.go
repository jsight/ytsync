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
	base           *http.Client
	config         *Config
	rateLimiter    *RateLimiter
	circuitBreaker *CircuitBreaker
	session        *SessionManager
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

	// Rate limiter configuration
	RateLimiter RateLimiterConfig

	// Circuit breaker configuration
	CircuitBreaker CircuitBreakerConfig

	// Connection pool configuration
	Transport TransportConfig
}

// TransportConfig configures the HTTP transport (connection pooling).
type TransportConfig struct {
	// MaxIdleConns is the maximum number of idle connections across all hosts.
	// Default: 20
	MaxIdleConns int

	// MaxIdleConnsPerHost is the maximum idle connections per host.
	// Default: 10
	MaxIdleConnsPerHost int

	// MaxConnsPerHost is the maximum concurrent connections per host.
	// Default: 20
	MaxConnsPerHost int

	// IdleConnTimeout is the maximum amount of time an idle connection can remain open.
	// Default: 90 seconds
	IdleConnTimeout time.Duration

	// ForceAttemptHTTP2 forces HTTP/2 for connections to servers that don't explicitly support it.
	// Default: true
	ForceAttemptHTTP2 bool

	// DisableKeepAlives disables HTTP keep-alives (connection reuse).
	// Default: false (keep-alives enabled)
	DisableKeepAlives bool
}

// DefaultConfig returns sensible defaults for HTTP client configuration.
func DefaultConfig() *Config {
	cbConfig := DefaultCircuitBreakerConfig()
	cbConfig.IsTransientError = IsTransientHTTPError
	return &Config{
		Timeout:        30 * time.Second,
		Retry:          retry.DefaultConfig(),
		MaxConcurrent:  10,
		UserAgent:      "ytsync/1.0",
		RateLimiter:    DefaultRateLimiterConfig(),
		CircuitBreaker: cbConfig,
		Transport:      DefaultTransportConfig(),
	}
}

// DefaultTransportConfig returns sensible defaults for HTTP transport configuration.
func DefaultTransportConfig() TransportConfig {
	return TransportConfig{
		MaxIdleConns:        20,
		MaxIdleConnsPerHost: 10,
		MaxConnsPerHost:     20,
		IdleConnTimeout:     90 * time.Second,
		ForceAttemptHTTP2:   true,
		DisableKeepAlives:   false,
	}
}

// New creates a new HTTP client with the given configuration.
func New(cfg *Config) *Client {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	// Configure transport with optimized settings for YouTube interactions
	transport := &http.Transport{
		// Connection pool settings
		MaxIdleConns:        cfg.Transport.MaxIdleConns,
		MaxIdleConnsPerHost: cfg.Transport.MaxIdleConnsPerHost,
		MaxConnsPerHost:     cfg.Transport.MaxConnsPerHost,
		IdleConnTimeout:     cfg.Transport.IdleConnTimeout,

		// HTTP/2 support
		ForceAttemptHTTP2: cfg.Transport.ForceAttemptHTTP2,

		// TCP keepalive
		DisableKeepAlives: cfg.Transport.DisableKeepAlives,
	}

	base := &http.Client{
		Timeout:   cfg.Timeout,
		Transport: transport,
	}

	return &Client{
		base:           base,
		config:         cfg,
		rateLimiter:    NewRateLimiter(cfg.RateLimiter),
		circuitBreaker: NewCircuitBreaker(cfg.CircuitBreaker),
		session:        nil,
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
// The circuit breaker pattern is used to fail fast when a domain is unresponsive.
func (c *Client) Do(ctx context.Context, method, urlStr string, body io.Reader, headers map[string]string) (*Response, error) {
	// Extract domain for circuit breaker
	domain := c.rateLimiter.extractDomain(urlStr)

	// Check circuit breaker first - fail fast if circuit is open
	if err := c.circuitBreaker.Allow(domain); err != nil {
		return nil, err
	}

	// Wait for any backoff period from previous rate limit errors
	if err := c.rateLimiter.WaitForBackoff(ctx, urlStr); err != nil {
		c.circuitBreaker.RecordFailure(domain, err)
		return nil, err
	}

	// Wait for rate limit before attempting request
	if err := c.rateLimiter.Wait(ctx, urlStr); err != nil {
		c.circuitBreaker.RecordFailure(domain, err)
		return nil, err
	}

	var lastResp *http.Response

	err := retry.Do(ctx, c.config.Retry, c.isRetryableHTTPError, func(ctx context.Context) error {
		req, err := http.NewRequestWithContext(ctx, method, urlStr, body)
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

		// Apply session headers if available
		if c.session != nil {
			for k, v := range c.session.GetHeaders() {
				if req.Header.Get(k) == "" { // Don't override explicitly set headers
					req.Header.Set(k, v)
				}
			}
		}

		resp, err := c.base.Do(req)
		if err != nil {
			return fmt.Errorf("http request failed: %w", err)
		}

		// Check for rate limiting (429) or anti-bot detection (403)
		if resp.StatusCode == http.StatusTooManyRequests ||
			resp.StatusCode == http.StatusServiceUnavailable ||
			resp.StatusCode == http.StatusForbidden {
			defer resp.Body.Close()

			// Parse Retry-After header
			retryAfter := c.parseRetryAfter(resp.Header)

			// Record rate limit error and get recommended backoff
			recommendedBackoff := c.rateLimiter.RecordRateLimitError(urlStr, retryAfter)
			if recommendedBackoff > retryAfter {
				retryAfter = recommendedBackoff
			}

			isBotDetection := resp.StatusCode == http.StatusForbidden
			return &RateLimitError{
				StatusCode:     resp.StatusCode,
				RetryAfter:     retryAfter,
				IsBotDetection: isBotDetection,
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
		// Record failure to circuit breaker
		c.circuitBreaker.RecordFailure(domain, err)
		return nil, err
	}

	if lastResp == nil {
		err := fmt.Errorf("no response received")
		c.circuitBreaker.RecordFailure(domain, err)
		return nil, err
	}

	defer lastResp.Body.Close()
	respBody, err := io.ReadAll(lastResp.Body)
	if err != nil {
		c.circuitBreaker.RecordFailure(domain, err)
		return nil, fmt.Errorf("read response body: %w", err)
	}

	// Record successful request to help recover from backoff and circuit breaker
	c.rateLimiter.RecordSuccess(urlStr)
	c.circuitBreaker.RecordSuccess(domain)

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

// Close closes the HTTP client connections and releases all resources.
func (c *Client) Close() error {
	if c.base != nil && c.base.Transport != nil {
		c.base.CloseIdleConnections()
	}
	return nil
}

// GetTransportConfig returns the transport configuration being used.
func (c *Client) GetTransportConfig() TransportConfig {
	return c.config.Transport
}
