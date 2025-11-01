package service

import (
	"sync"
	"time"
)

// RateLimiter provides in-memory rate limiting based on a sliding time window.
// It tracks attempts per key (e.g., email address) and enforces a maximum
// number of attempts within a time window.
type RateLimiter struct {
	mu          sync.RWMutex
	attempts    map[string][]time.Time // key -> timestamps of attempts
	maxAttempts int                    // maximum attempts allowed
	window      time.Duration          // time window for rate limiting
	stopCleanup chan struct{}          // channel to stop cleanup goroutine
	stopped     bool                   // flag to prevent double-close
}

// NewRateLimiter creates a new rate limiter with the specified maximum attempts
// and time window. It automatically starts a background cleanup goroutine to
// prevent memory leaks by removing expired entries.
//
// Example:
//   limiter := NewRateLimiter(5, 5*time.Minute) // 5 attempts per 5 minutes
func NewRateLimiter(maxAttempts int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		attempts:    make(map[string][]time.Time),
		maxAttempts: maxAttempts,
		window:      window,
		stopCleanup: make(chan struct{}),
	}

	// Start background cleanup goroutine
	go rl.cleanup()

	return rl
}

// Allow checks if a request for the given key should be allowed based on
// the rate limit. It returns true if the request is allowed, false if the
// rate limit has been exceeded.
//
// This method is thread-safe and can be called concurrently.
func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	// Get existing attempts for this key
	attempts := rl.attempts[key]

	// Filter out expired attempts (outside the time window)
	valid := make([]time.Time, 0, len(attempts))
	for _, t := range attempts {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}

	// Check if limit exceeded
	if len(valid) >= rl.maxAttempts {
		rl.attempts[key] = valid // Update with filtered list
		return false
	}

	// Record this attempt
	valid = append(valid, now)
	rl.attempts[key] = valid

	return true
}

// Reset clears all recorded attempts for the given key, effectively
// resetting the rate limit for that key.
//
// This is useful when you want to clear the rate limit after a successful
// operation (e.g., successful authentication).
func (rl *RateLimiter) Reset(key string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.attempts, key)
}

// cleanup runs in a background goroutine and periodically removes entries
// that have no recent attempts within the time window. This prevents
// unbounded memory growth.
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rl.mu.Lock()
			now := time.Now()
			cutoff := now.Add(-rl.window)

			// Remove entries with no recent attempts
			for key, attempts := range rl.attempts {
				hasRecent := false
				for _, t := range attempts {
					if t.After(cutoff) {
						hasRecent = true
						break
					}
				}
				if !hasRecent {
					delete(rl.attempts, key)
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

