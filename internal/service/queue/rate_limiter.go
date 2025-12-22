package queue

import (
	"context"
	"sync"

	"golang.org/x/time/rate"
)

// IntegrationRateLimiter manages rate limits per integration
// Each integration has its own rate limiter based on its configured RateLimitPerMinute
type IntegrationRateLimiter struct {
	limiters sync.Map // map[integrationID]*rate.Limiter
}

// NewIntegrationRateLimiter creates a new IntegrationRateLimiter
func NewIntegrationRateLimiter() *IntegrationRateLimiter {
	return &IntegrationRateLimiter{}
}

// GetOrCreateLimiter returns a rate limiter for the integration, creating one if needed
// The rate limiter is updated if the rate has changed
func (irl *IntegrationRateLimiter) GetOrCreateLimiter(integrationID string, ratePerMinute int) *rate.Limiter {
	// Convert rate per minute to rate per second
	ratePerSecond := float64(ratePerMinute) / 60.0

	// Ensure minimum rate of 1 per minute
	if ratePerSecond < 1.0/60.0 {
		ratePerSecond = 1.0 / 60.0
	}

	if existing, ok := irl.limiters.Load(integrationID); ok {
		limiter := existing.(*rate.Limiter)
		// Update rate if changed (SetLimit is thread-safe)
		if limiter.Limit() != rate.Limit(ratePerSecond) {
			limiter.SetLimit(rate.Limit(ratePerSecond))
		}
		return limiter
	}

	// Create new limiter with burst of 1 (strict rate limiting)
	// Burst of 1 means we allow at most 1 email to be sent immediately,
	// then subsequent emails must wait for the rate limit
	limiter := rate.NewLimiter(rate.Limit(ratePerSecond), 1)
	actual, _ := irl.limiters.LoadOrStore(integrationID, limiter)
	return actual.(*rate.Limiter)
}

// Wait blocks until the integration's rate limiter allows an event
// Returns an error if the context is cancelled
func (irl *IntegrationRateLimiter) Wait(ctx context.Context, integrationID string, ratePerMinute int) error {
	limiter := irl.GetOrCreateLimiter(integrationID, ratePerMinute)
	return limiter.Wait(ctx)
}

// Allow checks if sending is allowed immediately without blocking
// Returns true if allowed, false if rate limited
func (irl *IntegrationRateLimiter) Allow(integrationID string, ratePerMinute int) bool {
	limiter := irl.GetOrCreateLimiter(integrationID, ratePerMinute)
	return limiter.Allow()
}

// Reserve reserves a token without blocking and returns the delay needed
// The caller should wait for the returned duration before sending
func (irl *IntegrationRateLimiter) Reserve(integrationID string, ratePerMinute int) *rate.Reservation {
	limiter := irl.GetOrCreateLimiter(integrationID, ratePerMinute)
	return limiter.Reserve()
}

// GetCurrentRate returns the current rate limit for an integration in emails per second
// Returns 0 if no limiter exists for the integration
func (irl *IntegrationRateLimiter) GetCurrentRate(integrationID string) float64 {
	if existing, ok := irl.limiters.Load(integrationID); ok {
		limiter := existing.(*rate.Limiter)
		return float64(limiter.Limit())
	}
	return 0
}

// GetStats returns statistics about all rate limiters
func (irl *IntegrationRateLimiter) GetStats() map[string]RateLimiterStats {
	stats := make(map[string]RateLimiterStats)
	irl.limiters.Range(func(key, value interface{}) bool {
		integrationID := key.(string)
		limiter := value.(*rate.Limiter)
		stats[integrationID] = RateLimiterStats{
			RatePerSecond:   float64(limiter.Limit()),
			RatePerMinute:   float64(limiter.Limit()) * 60,
			TokensAvailable: limiter.Tokens(),
			Burst:           limiter.Burst(),
		}
		return true
	})
	return stats
}

// RateLimiterStats contains statistics for a single rate limiter
type RateLimiterStats struct {
	RatePerSecond   float64 `json:"rate_per_second"`
	RatePerMinute   float64 `json:"rate_per_minute"`
	TokensAvailable float64 `json:"tokens_available"`
	Burst           int     `json:"burst"`
}

// Clear removes all rate limiters (useful for testing or shutdown)
func (irl *IntegrationRateLimiter) Clear() {
	irl.limiters.Range(func(key, value interface{}) bool {
		irl.limiters.Delete(key)
		return true
	})
}

// Remove removes the rate limiter for a specific integration
func (irl *IntegrationRateLimiter) Remove(integrationID string) {
	irl.limiters.Delete(integrationID)
}
