package ratelimiter

import (
	"sync"
	"time"
)

// RatePolicy defines the rate limit configuration for a namespace
type RatePolicy struct {
	MaxAttempts int
	Window      time.Duration
}

// RateLimiter provides in-memory rate limiting with namespace support.
// It tracks attempts per namespace:key combination and enforces different
// rate limits for different namespaces.
//
// Example usage:
//
//	rl := ratelimiter.NewRateLimiter()
//	rl.SetPolicy("api", 100, 1*time.Minute)
//	rl.SetPolicy("auth", 5, 5*time.Minute)
//
//	if !rl.Allow("api", userID) {
//	    return http.StatusTooManyRequests
//	}
type RateLimiter struct {
	mu          sync.RWMutex
	attempts    map[string][]time.Time // "namespace:key" -> timestamps of attempts
	policies    map[string]RatePolicy  // namespace -> policy
	stopCleanup chan struct{}          // channel to stop cleanup goroutine
	stopped     bool                   // flag to prevent double-close
}

// NewRateLimiter creates a new rate limiter with namespace support.
// It automatically starts a background cleanup goroutine to prevent
// memory leaks by removing expired entries.
//
// After creation, use SetPolicy to configure rate limits for each namespace.
func NewRateLimiter() *RateLimiter {
	rl := &RateLimiter{
		attempts:    make(map[string][]time.Time),
		policies:    make(map[string]RatePolicy),
		stopCleanup: make(chan struct{}),
	}

	// Start background cleanup goroutine
	go rl.cleanup()

	return rl
}

// SetPolicy configures the rate limit policy for a specific namespace.
// This method should be called during initialization before Allow is used.
//
// Example:
//
//	rl.SetPolicy("signin", 5, 5*time.Minute) // 5 attempts per 5 minutes
func (rl *RateLimiter) SetPolicy(namespace string, maxAttempts int, window time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.policies[namespace] = RatePolicy{
		MaxAttempts: maxAttempts,
		Window:      window,
	}
}

// Allow checks if a request for the given namespace and key should be allowed
// based on the rate limit. It returns true if the request is allowed, false if
// the rate limit has been exceeded or if the namespace has no policy configured.
//
// This method is thread-safe and can be called concurrently.
//
// If the namespace doesn't have a configured policy, this method returns false
// (fail closed for security) and the request should be denied.
func (rl *RateLimiter) Allow(namespace, key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Check if policy exists for this namespace
	policy, exists := rl.policies[namespace]
	if !exists {
		// Fail closed - deny request if no policy configured
		// In production, this should be logged
		return false
	}

	now := time.Now()
	cutoff := now.Add(-policy.Window)

	// Create composite key
	compositeKey := namespace + ":" + key

	// Get existing attempts for this key
	attemptsList := rl.attempts[compositeKey]

	// Filter out expired attempts (outside the time window)
	valid := make([]time.Time, 0, len(attemptsList))
	for _, t := range attemptsList {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}

	// Check if limit exceeded
	if len(valid) >= policy.MaxAttempts {
		rl.attempts[compositeKey] = valid // Update with filtered list
		return false
	}

	// Record this attempt
	valid = append(valid, now)
	rl.attempts[compositeKey] = valid

	return true
}

// Reset clears all recorded attempts for the given namespace and key,
// effectively resetting the rate limit for that combination.
//
// This is useful when you want to clear the rate limit after a successful
// operation (e.g., successful authentication).
func (rl *RateLimiter) Reset(namespace, key string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	compositeKey := namespace + ":" + key
	delete(rl.attempts, compositeKey)
}

// GetRemainingWindow returns the number of seconds until the rate limit
// window expires for the given namespace and key. This is useful for
// setting the Retry-After header in HTTP responses.
//
// Returns 0 if there are no attempts or if the namespace has no policy.
func (rl *RateLimiter) GetRemainingWindow(namespace, key string) int {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	// Check if policy exists
	policy, exists := rl.policies[namespace]
	if !exists {
		return 0
	}

	compositeKey := namespace + ":" + key
	attemptsList := rl.attempts[compositeKey]

	if len(attemptsList) == 0 {
		return 0
	}

	// Find the oldest attempt that's still valid
	now := time.Now()
	cutoff := now.Add(-policy.Window)

	var oldestValid time.Time
	for _, t := range attemptsList {
		if t.After(cutoff) {
			if oldestValid.IsZero() || t.Before(oldestValid) {
				oldestValid = t
			}
		}
	}

	if oldestValid.IsZero() {
		return 0
	}

	// Calculate when this attempt will expire
	expiresAt := oldestValid.Add(policy.Window)
	remaining := time.Until(expiresAt)

	if remaining <= 0 {
		return 0
	}

	return int(remaining.Seconds()) + 1 // Round up
}

// cleanup runs in a background goroutine and periodically removes entries
// that have no recent attempts within their namespace's time window.
// This prevents unbounded memory growth.
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rl.mu.Lock()
			now := time.Now()

			// Remove entries with no recent attempts
			for compositeKey, attemptsList := range rl.attempts {
				// Extract namespace from composite key
				namespace := compositeKey
				if colonIdx := len(compositeKey); colonIdx > 0 {
					for i, c := range compositeKey {
						if c == ':' {
							namespace = compositeKey[:i]
							break
						}
					}
				}

				// Get policy for this namespace
				policy, exists := rl.policies[namespace]
				if !exists {
					// No policy for this namespace, remove the entry
					delete(rl.attempts, compositeKey)
					continue
				}

				cutoff := now.Add(-policy.Window)
				hasRecent := false
				for _, t := range attemptsList {
					if t.After(cutoff) {
						hasRecent = true
						break
					}
				}

				if !hasRecent {
					delete(rl.attempts, compositeKey)
				}
			}
			rl.mu.Unlock()

		case <-rl.stopCleanup:
			return
		}
	}
}

// Stop stops the background cleanup goroutine. This should be called
// when the rate limiter is no longer needed to prevent goroutine leaks.
// It is safe to call Stop multiple times.
func (rl *RateLimiter) Stop() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if !rl.stopped {
		close(rl.stopCleanup)
		rl.stopped = true
	}
}
