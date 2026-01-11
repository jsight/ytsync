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
// It supports configurable rates for different endpoints and dynamic rate adjustment.
type RateLimiter struct {
	limiters     map[string]*rate.Limiter
	backoffState map[string]*BackoffState
	mu           sync.RWMutex
	config       RateLimiterConfig
}

// BackoffState tracks rate limit backoff for a domain.
type BackoffState struct {
	// CurrentBackoff is the current backoff duration
	CurrentBackoff time.Duration
	// LastError is when the last rate limit error occurred
	LastError time.Time
	// ConsecutiveErrors is the count of consecutive rate limit errors
	ConsecutiveErrors int
	// OriginalRPS is the original configured rate to restore after cooldown
	OriginalRPS float64
	// ReducedRPS is the current reduced rate (0 means using original)
	ReducedRPS float64
}

// Default backoff values for Innertube rate limiting
const (
	// InnertubeInitialBackoff is the initial backoff for Innertube rate limits
	InnertubeInitialBackoff = 1 * time.Second
	// InnertubeMaxBackoff is the maximum backoff for Innertube rate limits
	InnertubeMaxBackoff = 60 * time.Second
	// InnertubeBackoffMultiplier is the multiplier for exponential backoff
	InnertubeBackoffMultiplier = 2.0
	// BackoffCooldownPeriod is how long after last error before resetting backoff
	BackoffCooldownPeriod = 5 * time.Minute
	// MinRPSMultiplier is the minimum rate reduction (0.25 = 25% of original)
	MinRPSMultiplier = 0.25
)

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
	// EnableDynamicBackoff enables automatic rate reduction on errors
	EnableDynamicBackoff bool
}

// DefaultRateLimiterConfig returns sensible defaults aligned with YouTube's rate limits.
func DefaultRateLimiterConfig() RateLimiterConfig {
	return RateLimiterConfig{
		InnertubeRPS:         2.5,  // Conservative: 2-3 req/s
		DataAPIRPS:           1.0,  // Conservative per quota
		RSSRPS:               10.0, // RSS is generous with rate limits
		CustomRates:          make(map[string]float64),
		EnableDynamicBackoff: true,
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
		limiters:     make(map[string]*rate.Limiter),
		backoffState: make(map[string]*BackoffState),
		config:       cfg,
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

// RecordRateLimitError records a rate limit error for a domain and updates backoff state.
// Call this when a 429/403 response is received.
// Returns the recommended backoff duration before retrying.
func (rl *RateLimiter) RecordRateLimitError(urlStr string, retryAfter time.Duration) time.Duration {
	if rl == nil || !rl.config.EnableDynamicBackoff {
		if retryAfter > 0 {
			return retryAfter
		}
		return InnertubeInitialBackoff
	}

	domain := rl.extractDomain(urlStr)

	rl.mu.Lock()
	defer rl.mu.Unlock()

	state, exists := rl.backoffState[domain]
	if !exists {
		// Initialize backoff state
		originalRPS := rl.getRPS(domain)
		state = &BackoffState{
			CurrentBackoff: InnertubeInitialBackoff,
			LastError:      time.Now(),
			OriginalRPS:    originalRPS,
		}
		rl.backoffState[domain] = state
	}

	// Update state
	state.LastError = time.Now()
	state.ConsecutiveErrors++

	// Calculate new backoff: 1s → 2s → 4s → 8s → ... → max
	if state.ConsecutiveErrors > 1 {
		state.CurrentBackoff = time.Duration(float64(state.CurrentBackoff) * InnertubeBackoffMultiplier)
		if state.CurrentBackoff > InnertubeMaxBackoff {
			state.CurrentBackoff = InnertubeMaxBackoff
		}
	}

	// Use server-specified Retry-After if longer than our calculated backoff
	effectiveBackoff := state.CurrentBackoff
	if retryAfter > effectiveBackoff {
		effectiveBackoff = retryAfter
		state.CurrentBackoff = retryAfter
	}

	// Reduce the rate limit for this domain
	rl.reduceRate(domain, state)

	return effectiveBackoff
}

// reduceRate reduces the rate limit for a domain based on backoff state.
// Must be called with mutex held.
func (rl *RateLimiter) reduceRate(domain string, state *BackoffState) {
	// Calculate reduction factor based on consecutive errors
	// 1 error: 75%, 2 errors: 50%, 3+ errors: 25%
	reductionFactor := 1.0
	switch {
	case state.ConsecutiveErrors >= 3:
		reductionFactor = MinRPSMultiplier
	case state.ConsecutiveErrors == 2:
		reductionFactor = 0.5
	case state.ConsecutiveErrors == 1:
		reductionFactor = 0.75
	}

	newRPS := state.OriginalRPS * reductionFactor
	if newRPS < state.OriginalRPS*MinRPSMultiplier {
		newRPS = state.OriginalRPS * MinRPSMultiplier
	}

	state.ReducedRPS = newRPS

	// Update the limiter with the new rate
	if limiter, ok := rl.limiters[domain]; ok {
		limiter.SetLimit(rate.Limit(newRPS))
	}
}

// RecordSuccess records a successful request, potentially resetting backoff state.
func (rl *RateLimiter) RecordSuccess(urlStr string) {
	if rl == nil || !rl.config.EnableDynamicBackoff {
		return
	}

	domain := rl.extractDomain(urlStr)

	rl.mu.Lock()
	defer rl.mu.Unlock()

	state, exists := rl.backoffState[domain]
	if !exists {
		return
	}

	// If enough time has passed since last error, start recovering
	if time.Since(state.LastError) > BackoffCooldownPeriod {
		// Reset to original rate
		if limiter, ok := rl.limiters[domain]; ok && state.ReducedRPS > 0 {
			limiter.SetLimit(rate.Limit(state.OriginalRPS))
		}
		delete(rl.backoffState, domain)
		return
	}

	// Gradually reduce consecutive error count after successful requests
	if state.ConsecutiveErrors > 0 {
		state.ConsecutiveErrors--

		// Increase rate slightly if we're recovering
		if state.ReducedRPS > 0 && state.ConsecutiveErrors == 0 {
			// Recover to 50% of original, then full recovery after cooldown
			newRPS := state.OriginalRPS * 0.5
			if newRPS > state.ReducedRPS {
				state.ReducedRPS = newRPS
				if limiter, ok := rl.limiters[domain]; ok {
					limiter.SetLimit(rate.Limit(newRPS))
				}
			}
		}
	}
}

// GetBackoffState returns the current backoff state for a domain.
// Returns nil if no backoff state exists.
func (rl *RateLimiter) GetBackoffState(urlStr string) *BackoffState {
	if rl == nil {
		return nil
	}

	domain := rl.extractDomain(urlStr)

	rl.mu.RLock()
	defer rl.mu.RUnlock()

	if state, ok := rl.backoffState[domain]; ok {
		// Return a copy to prevent external modification
		return &BackoffState{
			CurrentBackoff:    state.CurrentBackoff,
			LastError:         state.LastError,
			ConsecutiveErrors: state.ConsecutiveErrors,
			OriginalRPS:       state.OriginalRPS,
			ReducedRPS:        state.ReducedRPS,
		}
	}
	return nil
}

// IsBackedOff returns true if the domain is currently in a backoff state.
func (rl *RateLimiter) IsBackedOff(urlStr string) bool {
	state := rl.GetBackoffState(urlStr)
	if state == nil {
		return false
	}
	return time.Since(state.LastError) < state.CurrentBackoff
}

// WaitForBackoff waits for the current backoff period to expire.
// Returns immediately if not in backoff state.
func (rl *RateLimiter) WaitForBackoff(ctx context.Context, urlStr string) error {
	state := rl.GetBackoffState(urlStr)
	if state == nil {
		return nil
	}

	remaining := state.CurrentBackoff - time.Since(state.LastError)
	if remaining <= 0 {
		return nil
	}

	select {
	case <-time.After(remaining):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
