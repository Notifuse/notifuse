package ratelimiter

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRateLimiter(t *testing.T) {
	rl := NewRateLimiter()
	require.NotNil(t, rl)
	assert.NotNil(t, rl.attempts)
	assert.NotNil(t, rl.policies)
	assert.NotNil(t, rl.stopCleanup)

	// Clean up
	rl.Stop()
}

func TestRateLimiter_SetPolicy(t *testing.T) {
	rl := NewRateLimiter()
	defer rl.Stop()

	rl.SetPolicy("test", 5, 1*time.Minute)

	rl.mu.RLock()
	policy, exists := rl.policies["test"]
	rl.mu.RUnlock()

	assert.True(t, exists, "Policy should be set")
	assert.Equal(t, 5, policy.MaxAttempts)
	assert.Equal(t, 1*time.Minute, policy.Window)
}

func TestRateLimiter_Allow_BasicLimiting(t *testing.T) {
	rl := NewRateLimiter()
	defer rl.Stop()

	rl.SetPolicy("test", 3, 1*time.Second)
	key := "test@example.com"

	// Should allow first 3 attempts
	assert.True(t, rl.Allow("test", key), "First attempt should be allowed")
	assert.True(t, rl.Allow("test", key), "Second attempt should be allowed")
	assert.True(t, rl.Allow("test", key), "Third attempt should be allowed")

	// Should block 4th attempt
	assert.False(t, rl.Allow("test", key), "Fourth attempt should be blocked")
	assert.False(t, rl.Allow("test", key), "Fifth attempt should be blocked")
}

func TestRateLimiter_Allow_MissingPolicy(t *testing.T) {
	rl := NewRateLimiter()
	defer rl.Stop()

	// Don't set any policy
	key := "test@example.com"

	// Should deny (fail closed) when no policy is configured
	assert.False(t, rl.Allow("nonexistent", key), "Should deny when no policy exists")
}

func TestRateLimiter_Allow_WindowExpiration(t *testing.T) {
	rl := NewRateLimiter()
	defer rl.Stop()

	rl.SetPolicy("test", 3, 500*time.Millisecond)
	key := "test@example.com"

	// Use up all attempts
	assert.True(t, rl.Allow("test", key))
	assert.True(t, rl.Allow("test", key))
	assert.True(t, rl.Allow("test", key))
	assert.False(t, rl.Allow("test", key), "Should be blocked")

	// Wait for window to expire
	time.Sleep(600 * time.Millisecond)

	// Should allow again after window expires
	assert.True(t, rl.Allow("test", key), "Should be allowed after window expires")
}

func TestRateLimiter_Allow_DifferentKeys(t *testing.T) {
	rl := NewRateLimiter()
	defer rl.Stop()

	rl.SetPolicy("test", 2, 1*time.Second)
	key1 := "user1@example.com"
	key2 := "user2@example.com"

	// Each key should have independent rate limits
	assert.True(t, rl.Allow("test", key1))
	assert.True(t, rl.Allow("test", key1))
	assert.False(t, rl.Allow("test", key1), "Key1 should be blocked")

	// Key2 should still be allowed
	assert.True(t, rl.Allow("test", key2))
	assert.True(t, rl.Allow("test", key2))
	assert.False(t, rl.Allow("test", key2), "Key2 should be blocked")
}

func TestRateLimiter_MultipleNamespaces(t *testing.T) {
	rl := NewRateLimiter()
	defer rl.Stop()

	rl.SetPolicy("strict", 2, 1*time.Second)
	rl.SetPolicy("permissive", 10, 1*time.Second)

	email := "test@example.com"

	// Exhaust strict namespace
	assert.True(t, rl.Allow("strict", email))
	assert.True(t, rl.Allow("strict", email))
	assert.False(t, rl.Allow("strict", email), "Strict namespace should be exhausted")

	// Permissive namespace should still allow
	assert.True(t, rl.Allow("permissive", email), "Permissive namespace should still allow")

	// Use more attempts in permissive
	for i := 0; i < 9; i++ {
		assert.True(t, rl.Allow("permissive", email), fmt.Sprintf("Attempt %d should be allowed", i+2))
	}

	// 11th attempt should be blocked
	assert.False(t, rl.Allow("permissive", email), "Permissive namespace should now be exhausted")
}

func TestRateLimiter_NamespaceIndependence(t *testing.T) {
	rl := NewRateLimiter()
	defer rl.Stop()

	rl.SetPolicy("signin", 5, 5*time.Minute)
	rl.SetPolicy("api", 100, 1*time.Minute)

	email := "test@example.com"

	// Use up signin namespace
	for i := 0; i < 5; i++ {
		assert.True(t, rl.Allow("signin", email))
	}
	assert.False(t, rl.Allow("signin", email), "Signin should be blocked")

	// API namespace should still allow
	assert.True(t, rl.Allow("api", email), "API namespace should be independent")
}

func TestRateLimiter_Reset(t *testing.T) {
	rl := NewRateLimiter()
	defer rl.Stop()

	rl.SetPolicy("test", 2, 1*time.Minute)
	key := "test@example.com"

	// Use up attempts
	assert.True(t, rl.Allow("test", key))
	assert.True(t, rl.Allow("test", key))
	assert.False(t, rl.Allow("test", key), "Should be blocked")

	// Reset the key
	rl.Reset("test", key)

	// Should allow again immediately
	assert.True(t, rl.Allow("test", key), "Should be allowed after reset")
	assert.True(t, rl.Allow("test", key), "Should be allowed after reset")
	assert.False(t, rl.Allow("test", key), "Should be blocked again")
}

func TestRateLimiter_GetRemainingWindow(t *testing.T) {
	rl := NewRateLimiter()
	defer rl.Stop()

	rl.SetPolicy("test", 3, 10*time.Second)
	key := "test@example.com"

	// No attempts yet
	assert.Equal(t, 0, rl.GetRemainingWindow("test", key))

	// Make one attempt
	assert.True(t, rl.Allow("test", key))
	remaining := rl.GetRemainingWindow("test", key)
	assert.Greater(t, remaining, 0, "Should have remaining time")
	assert.LessOrEqual(t, remaining, 10, "Should not exceed window")

	// Wait a bit and check again
	time.Sleep(1 * time.Second)
	remaining2 := rl.GetRemainingWindow("test", key)
	assert.Less(t, remaining2, remaining, "Remaining time should decrease")
}

func TestRateLimiter_GetRemainingWindow_NoPolicy(t *testing.T) {
	rl := NewRateLimiter()
	defer rl.Stop()

	key := "test@example.com"

	// Should return 0 for namespace with no policy
	assert.Equal(t, 0, rl.GetRemainingWindow("nonexistent", key))
}

func TestRateLimiter_ConcurrentAccess(t *testing.T) {
	rl := NewRateLimiter()
	defer rl.Stop()

	rl.SetPolicy("test", 100, 1*time.Second)

	var wg sync.WaitGroup
	successCount := int32(0)
	failCount := int32(0)

	// Launch 200 goroutines trying to access the same key
	key := "concurrent@example.com"
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if rl.Allow("test", key) {
				atomic.AddInt32(&successCount, 1)
			} else {
				atomic.AddInt32(&failCount, 1)
			}
		}()
	}

	wg.Wait()

	// Should have exactly 100 successes and 100 failures
	assert.Equal(t, int32(100), atomic.LoadInt32(&successCount), "Should allow exactly max attempts")
	assert.Equal(t, int32(100), atomic.LoadInt32(&failCount), "Should block remaining attempts")
}

func TestRateLimiter_ConcurrentDifferentKeys(t *testing.T) {
	rl := NewRateLimiter()
	defer rl.Stop()

	rl.SetPolicy("test", 10, 1*time.Second)

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
				rl.Allow("test", key)
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

func TestRateLimiter_ConcurrentNamespaces(t *testing.T) {
	rl := NewRateLimiter()
	defer rl.Stop()

	rl.SetPolicy("ns1", 5, 1*time.Second)
	rl.SetPolicy("ns2", 10, 1*time.Second)
	rl.SetPolicy("ns3", 15, 1*time.Second)

	var wg sync.WaitGroup
	key := "test@example.com"

	// Concurrent access to different namespaces
	for _, ns := range []string{"ns1", "ns2", "ns3"} {
		wg.Add(1)
		go func(namespace string) {
			defer wg.Done()
			for i := 0; i < 20; i++ {
				rl.Allow(namespace, key)
			}
		}(ns)
	}

	wg.Wait()

	// Should not panic or race
	rl.mu.RLock()
	keyCount := len(rl.attempts)
	rl.mu.RUnlock()

	assert.Greater(t, keyCount, 0, "Should have tracked attempts")
}

func TestRateLimiter_Cleanup(t *testing.T) {
	rl := NewRateLimiter()
	defer rl.Stop()

	rl.SetPolicy("test", 5, 100*time.Millisecond)

	// Add some attempts
	for i := 0; i < 10; i++ {
		key := fmt.Sprintf("user%d@example.com", i)
		rl.Allow("test", key)
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
	for compositeKey, attemptsList := range rl.attempts {
		// Extract namespace
		namespace := compositeKey
		for i, c := range compositeKey {
			if c == ':' {
				namespace = compositeKey[:i]
				break
			}
		}

		policy, exists := rl.policies[namespace]
		if !exists {
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

	// Check that old entries were cleaned up
	rl.mu.RLock()
	finalCount := len(rl.attempts)
	rl.mu.RUnlock()
	assert.Equal(t, 0, finalCount, "Old entries should be cleaned up")
}

func TestRateLimiter_Stop(t *testing.T) {
	rl := NewRateLimiter()
	rl.SetPolicy("test", 5, 1*time.Minute)

	// Add some attempts
	rl.Allow("test", "test@example.com")

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
	rl := NewRateLimiter()
	defer rl.Stop()

	rl.SetPolicy("test", 3, 1*time.Second)
	key := "test@example.com"

	// Use 2 attempts
	assert.True(t, rl.Allow("test", key))
	assert.True(t, rl.Allow("test", key))

	// Wait half the window
	time.Sleep(500 * time.Millisecond)

	// Use 1 more attempt (should work, 3 total in window)
	assert.True(t, rl.Allow("test", key))

	// Try another (should fail, still 3 in window)
	assert.False(t, rl.Allow("test", key))

	// Wait for first 2 attempts to expire
	time.Sleep(600 * time.Millisecond)

	// Now only 1 attempt in window, should allow 2 more
	assert.True(t, rl.Allow("test", key))
	assert.True(t, rl.Allow("test", key))
	assert.False(t, rl.Allow("test", key))
}

func TestRateLimiter_ZeroAttempts(t *testing.T) {
	// Edge case: limiter that allows 0 attempts
	rl := NewRateLimiter()
	defer rl.Stop()

	rl.SetPolicy("test", 0, 1*time.Minute)
	key := "test@example.com"

	// Should immediately block
	assert.False(t, rl.Allow("test", key), "Should block when maxAttempts is 0")
}

func TestRateLimiter_LargeVolume(t *testing.T) {
	rl := NewRateLimiter()
	defer rl.Stop()

	// Use a longer window to ensure attempts don't expire during the test loop
	rl.SetPolicy("test", 1000, 1*time.Minute)

	// Simulate high volume for a single key
	key := "highvolume@example.com"

	successCount := 0
	for i := 0; i < 2000; i++ {
		if rl.Allow("test", key) {
			successCount++
		}
	}

	assert.Equal(t, 1000, successCount, "Should allow exactly maxAttempts")
}

func TestRateLimiter_DifferentWindowsPerNamespace(t *testing.T) {
	rl := NewRateLimiter()
	defer rl.Stop()

	// Short window
	rl.SetPolicy("short", 2, 500*time.Millisecond)
	// Long window
	rl.SetPolicy("long", 2, 2*time.Second)

	key := "test@example.com"

	// Use both namespaces
	assert.True(t, rl.Allow("short", key))
	assert.True(t, rl.Allow("short", key))
	assert.False(t, rl.Allow("short", key))

	assert.True(t, rl.Allow("long", key))
	assert.True(t, rl.Allow("long", key))
	assert.False(t, rl.Allow("long", key))

	// Wait for short window to expire
	time.Sleep(600 * time.Millisecond)

	// Short should allow again
	assert.True(t, rl.Allow("short", key), "Short window should have expired")

	// Long should still be blocked
	assert.False(t, rl.Allow("long", key), "Long window should still be active")

	// Wait for long window to expire
	time.Sleep(1500 * time.Millisecond)

	// Long should allow again
	assert.True(t, rl.Allow("long", key), "Long window should have expired")
}
