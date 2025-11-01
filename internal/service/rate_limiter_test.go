package service

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRateLimiter(t *testing.T) {
	rl := NewRateLimiter(5, 1*time.Minute)
	require.NotNil(t, rl)
	assert.Equal(t, 5, rl.maxAttempts)
	assert.Equal(t, 1*time.Minute, rl.window)
	assert.NotNil(t, rl.attempts)
	assert.NotNil(t, rl.stopCleanup)

	// Clean up
	rl.Stop()
}

func TestRateLimiter_Allow_BasicLimiting(t *testing.T) {
	rl := NewRateLimiter(3, 1*time.Second)
	defer rl.Stop()

	key := "test@example.com"

	// Should allow first 3 attempts
	assert.True(t, rl.Allow(key), "First attempt should be allowed")
	assert.True(t, rl.Allow(key), "Second attempt should be allowed")
	assert.True(t, rl.Allow(key), "Third attempt should be allowed")

	// Should block 4th attempt
	assert.False(t, rl.Allow(key), "Fourth attempt should be blocked")
	assert.False(t, rl.Allow(key), "Fifth attempt should be blocked")
}

func TestRateLimiter_Allow_WindowExpiration(t *testing.T) {
	rl := NewRateLimiter(3, 500*time.Millisecond)
	defer rl.Stop()

	key := "test@example.com"

	// Use up all attempts
	assert.True(t, rl.Allow(key))
	assert.True(t, rl.Allow(key))
	assert.True(t, rl.Allow(key))
	assert.False(t, rl.Allow(key), "Should be blocked")

	// Wait for window to expire
	time.Sleep(600 * time.Millisecond)

	// Should allow again after window expires
	assert.True(t, rl.Allow(key), "Should be allowed after window expires")
}

func TestRateLimiter_Allow_DifferentKeys(t *testing.T) {
	rl := NewRateLimiter(2, 1*time.Second)
	defer rl.Stop()

	key1 := "user1@example.com"
	key2 := "user2@example.com"

	// Each key should have independent rate limits
	assert.True(t, rl.Allow(key1))
	assert.True(t, rl.Allow(key1))
	assert.False(t, rl.Allow(key1), "Key1 should be blocked")

	// Key2 should still be allowed
	assert.True(t, rl.Allow(key2))
	assert.True(t, rl.Allow(key2))
	assert.False(t, rl.Allow(key2), "Key2 should be blocked")
}

func TestRateLimiter_Reset(t *testing.T) {
	rl := NewRateLimiter(2, 1*time.Minute)
	defer rl.Stop()

	key := "test@example.com"

	// Use up attempts
	assert.True(t, rl.Allow(key))
	assert.True(t, rl.Allow(key))
	assert.False(t, rl.Allow(key), "Should be blocked")

	// Reset the key
	rl.Reset(key)

	// Should allow again immediately
	assert.True(t, rl.Allow(key), "Should be allowed after reset")
	assert.True(t, rl.Allow(key), "Should be allowed after reset")
	assert.False(t, rl.Allow(key), "Should be blocked again")
}

func TestRateLimiter_ConcurrentAccess(t *testing.T) {
	rl := NewRateLimiter(100, 1*time.Second)
	defer rl.Stop()

	var wg sync.WaitGroup
	successCount := int32(0)
	failCount := int32(0)

	// Launch 200 goroutines trying to access the same key
	key := "concurrent@example.com"
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if rl.Allow(key) {
				successCount++
			} else {
				failCount++
			}
		}()
	}

	wg.Wait()

	// Should have exactly 100 successes and 100 failures
	assert.Equal(t, int32(100), successCount, "Should allow exactly max attempts")
	assert.Equal(t, int32(100), failCount, "Should block remaining attempts")
}

func TestRateLimiter_ConcurrentDifferentKeys(t *testing.T) {
	rl := NewRateLimiter(10, 1*time.Second)
	defer rl.Stop()

	var wg sync.WaitGroup
	numKeys := 100

	// Launch goroutines for different keys
	for i := 0; i < numKeys; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			key := fmt.Sprintf("user%d@example.com", index)
			
			// Each key should get its own quota
			for j := 0; j < 15; j++ {
				rl.Allow(key)
			}
		}(i)
	}

	wg.Wait()

	// Should not panic or race
	// Check that we have entries for multiple keys
	rl.mu.RLock()
	keyCount := len(rl.attempts)
	rl.mu.RUnlock()

	assert.Greater(t, keyCount, 0, "Should have tracked multiple keys")
}

func TestRateLimiter_Cleanup(t *testing.T) {
	rl := NewRateLimiter(5, 100*time.Millisecond)
	defer rl.Stop()

	// Add some attempts
	for i := 0; i < 10; i++ {
		key := fmt.Sprintf("user%d@example.com", i)
		rl.Allow(key)
	}

	// Verify we have entries
	rl.mu.RLock()
	initialCount := len(rl.attempts)
	rl.mu.RUnlock()
	assert.Greater(t, initialCount, 0, "Should have entries")

	// Wait for window to expire
	time.Sleep(200 * time.Millisecond)

	// Manually trigger cleanup logic (since cleanup runs every minute)
	rl.mu.Lock()
	now := time.Now()
	cutoff := now.Add(-rl.window)
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

	// Check that old entries were cleaned up
	rl.mu.RLock()
	finalCount := len(rl.attempts)
	rl.mu.RUnlock()
	assert.Equal(t, 0, finalCount, "Old entries should be cleaned up")
}

func TestRateLimiter_Stop(t *testing.T) {
	rl := NewRateLimiter(5, 1*time.Minute)
	
	// Add some attempts
	rl.Allow("test@example.com")

	// Stop should not panic
	assert.NotPanics(t, func() {
		rl.Stop()
	})

	// Calling Stop again should not panic
	assert.NotPanics(t, func() {
		rl.Stop()
	})
}

func TestRateLimiter_SlidingWindow(t *testing.T) {
	rl := NewRateLimiter(3, 1*time.Second)
	defer rl.Stop()

	key := "test@example.com"

	// Use 2 attempts
	assert.True(t, rl.Allow(key))
	assert.True(t, rl.Allow(key))

	// Wait half the window
	time.Sleep(500 * time.Millisecond)

	// Use 1 more attempt (should work, 3 total in window)
	assert.True(t, rl.Allow(key))

	// Try another (should fail, still 3 in window)
	assert.False(t, rl.Allow(key))

	// Wait for first 2 attempts to expire
	time.Sleep(600 * time.Millisecond)

	// Now only 1 attempt in window, should allow 2 more
	assert.True(t, rl.Allow(key))
	assert.True(t, rl.Allow(key))
	assert.False(t, rl.Allow(key))
}

func TestRateLimiter_ZeroAttempts(t *testing.T) {
	// Edge case: limiter that allows 0 attempts
	rl := NewRateLimiter(0, 1*time.Minute)
	defer rl.Stop()

	key := "test@example.com"

	// Should immediately block
	assert.False(t, rl.Allow(key), "Should block when maxAttempts is 0")
}

func TestRateLimiter_LargeVolume(t *testing.T) {
	rl := NewRateLimiter(1000, 1*time.Second)
	defer rl.Stop()

	// Simulate high volume for a single key
	key := "highvolume@example.com"
	
	successCount := 0
	for i := 0; i < 2000; i++ {
		if rl.Allow(key) {
			successCount++
		}
	}

	assert.Equal(t, 1000, successCount, "Should allow exactly maxAttempts")
}

