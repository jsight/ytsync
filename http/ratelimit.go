// Package http provides HTTP client infrastructure for YouTube interactions
package http

import (
	"context"
	"fmt"
	"net/url"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// RateLimiter manages per-domain request rate limiting using token bucket algorithm.
// It supports configurable rates for different endpoints.
type RateLimiter struct {
	limiters map[string]*rate.Limiter
	mu       sync.RWMutex
	config   RateLimiterConfig
}

// RateLimiterConfig defines rate limiting behavior.
type RateLimiterConfig struct {
	// InnertubeRPS is requests per second for innertube API (default: 2.5)
	InnertubeRPS float64
	// DataAPIRPS is requests per second for YouTube Data API
	// If 0, uses quota-based limiting instead (no token bucket)
	DataAPIRPS float64
	// RSSRPS is requests per second for RSS feeds (0 = unlimited)
	RSSRPS float64
	// CustomRates maps domain patterns to RPS values
	CustomRates map[string]float64
}

// DefaultRateLimiterConfig returns sensible defaults aligned with YouTube's rate limits.
func DefaultRateLimiterConfig() RateLimiterConfig {
	return RateLimiterConfig{
		InnertubeRPS: 2.5,    // Conservative: 2-3 req/s
		DataAPIRPS:   1.0,    // Conservative per quota
		RSSRPS:       10.0,   // RSS is generous with rate limits
		CustomRates:  make(map[string]float64),
	}
}

// NewRateLimiter creates a new rate limiter with the given configuration.
func NewRateLimiter(cfg RateLimiterConfig) *RateLimiter {
	if cfg.InnertubeRPS == 0 {
		cfg.InnertubeRPS = DefaultRateLimiterConfig().InnertubeRPS
	}
	if cfg.DataAPIRPS == 0 {
		cfg.DataAPIRPS = DefaultRateLimiterConfig().DataAPIRPS
	}
	if cfg.RSSRPS == 0 {
		cfg.RSSRPS = DefaultRateLimiterConfig().RSSRPS
	}
	if cfg.CustomRates == nil {
		cfg.CustomRates = make(map[string]float64)
	}

	return &RateLimiter{
		limiters: make(map[string]*rate.Limiter),
		config:   cfg,
	}
}

// Wait waits until the rate limit allows a request for the given URL.
// Returns an error if the context is canceled or exceeded deadline.
func (rl *RateLimiter) Wait(ctx context.Context, urlStr string) error {
	if rl == nil {
		return nil
	}

	limiter := rl.getLimiter(urlStr)
	if limiter == nil {
		// No rate limiting for this domain
		return nil
	}

	if !limiter.Allow() {
		// Calculate wait time and use reservation for accurate timing
		reservation := limiter.Reserve()
		if !reservation.OK() {
			return fmt.Errorf("rate limit: cannot reserve token")
		}

		// Wait for the reservation or context cancellation
		select {
		case <-time.After(reservation.Delay()):
			return nil
		case <-ctx.Done():
			reservation.Cancel()
			return ctx.Err()
		}
	}

	return nil
}

// getLimiter returns the rate limiter for a given URL, creating one if necessary.
func (rl *RateLimiter) getLimiter(urlStr string) *rate.Limiter {
	domain := rl.extractDomain(urlStr)
	rps := rl.getRPS(domain)

	// Unlimited rate limit (0 RPS)
	if rps == 0 {
		return nil
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Return existing limiter
	if limiter, ok := rl.limiters[domain]; ok {
		return limiter
	}

	// Create new limiter with token bucket: tokens=1 (burst of 1), rate=rps
	limiter := rate.NewLimiter(rate.Limit(rps), 1)
	rl.limiters[domain] = limiter
	return limiter
}

// getRPS returns the requests per second for a given domain.
func (rl *RateLimiter) getRPS(domain string) float64 {
	// Check custom rates first
	if rps, ok := rl.config.CustomRates[domain]; ok {
		return rps
	}

	// Check well-known domains
	switch domain {
	case "www.youtube.com", "youtube.com":
		// YouTube intertube API
		return rl.config.InnertubeRPS
	case "www.googleapis.com", "googleapis.com":
		// YouTube Data API
		return rl.config.DataAPIRPS
	case "feeds.youtube.com":
		// YouTube RSS feeds
		return rl.config.RSSRPS
	default:
		// Default to innertube conservative rate
		return rl.config.InnertubeRPS
	}
}

// extractDomain extracts the domain from a URL string.
func (rl *RateLimiter) extractDomain(urlStr string) string {
	u, err := url.Parse(urlStr)
	if err != nil {
		return "unknown"
	}

	host := u.Host
	if host == "" {
		return "unknown"
	}

	// Remove port if present
	if idx := indexAny(host, ":"); idx != -1 {
		host = host[:idx]
	}

	return host
}

// indexAny returns the index of the first occurrence of any character in s in the string.
func indexAny(s, chars string) int {
	for i, c := range s {
		for _, ch := range chars {
			if c == ch {
				return i
			}
		}
	}
	return -1
}

// SetCustomRate sets a custom rate limit for a specific domain.
func (rl *RateLimiter) SetCustomRate(domain string, rps float64) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.config.CustomRates[domain] = rps

	// Clear existing limiter to force recreation with new rate
	delete(rl.limiters, domain)
}

// Stats returns statistics about the rate limiters.
// Useful for monitoring and debugging.
func (rl *RateLimiter) Stats() map[string]float64 {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	stats := make(map[string]float64)
	for domain := range rl.limiters {
		stats[domain] = rl.getRPS(domain)
	}
	return stats
}
