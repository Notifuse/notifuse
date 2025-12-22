package queue

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewIntegrationRateLimiter(t *testing.T) {
	limiter := NewIntegrationRateLimiter()

	require.NotNil(t, limiter)
	// Should have empty stats initially
	stats := limiter.GetStats()
	assert.Empty(t, stats)
}

func TestIntegrationRateLimiter_GetOrCreateLimiter(t *testing.T) {
	t.Run("creates new limiter for unknown integration", func(t *testing.T) {
		irl := NewIntegrationRateLimiter()

		limiter := irl.GetOrCreateLimiter("integration-1", 60) // 60 per minute = 1 per second

		require.NotNil(t, limiter)
		// Rate should be 1 per second (60/60)
		assert.InDelta(t, 1.0, float64(limiter.Limit()), 0.001)
		// Burst should be 1
		assert.Equal(t, 1, limiter.Burst())
	})

	t.Run("returns existing limiter for known integration", func(t *testing.T) {
		irl := NewIntegrationRateLimiter()

		limiter1 := irl.GetOrCreateLimiter("integration-1", 60)
		limiter2 := irl.GetOrCreateLimiter("integration-1", 60)

		// Should return the same limiter instance
		assert.Same(t, limiter1, limiter2)
	})

	t.Run("updates rate when rate changes", func(t *testing.T) {
		irl := NewIntegrationRateLimiter()

		// Create with initial rate
		limiter1 := irl.GetOrCreateLimiter("integration-1", 60) // 1 per second
		initialRate := float64(limiter1.Limit())

		// Update with new rate
		limiter2 := irl.GetOrCreateLimiter("integration-1", 120) // 2 per second

		// Should be same limiter but with updated rate
		assert.Same(t, limiter1, limiter2)
		assert.NotEqual(t, initialRate, float64(limiter2.Limit()))
		assert.InDelta(t, 2.0, float64(limiter2.Limit()), 0.001)
	})

	t.Run("enforces minimum rate of 1 per minute", func(t *testing.T) {
		irl := NewIntegrationRateLimiter()

		// Try with zero rate
		limiter := irl.GetOrCreateLimiter("integration-1", 0)

		// Should have minimum rate of 1 per minute (1/60 per second)
		minRate := 1.0 / 60.0
		assert.InDelta(t, minRate, float64(limiter.Limit()), 0.001)
	})

	t.Run("handles negative rate", func(t *testing.T) {
		irl := NewIntegrationRateLimiter()

		// Try with negative rate
		limiter := irl.GetOrCreateLimiter("integration-1", -100)

		// Should have minimum rate of 1 per minute (1/60 per second)
		minRate := 1.0 / 60.0
		assert.InDelta(t, minRate, float64(limiter.Limit()), 0.001)
	})

	t.Run("handles high rate correctly", func(t *testing.T) {
		irl := NewIntegrationRateLimiter()

		// 6000 per minute = 100 per second
		limiter := irl.GetOrCreateLimiter("integration-1", 6000)

		assert.InDelta(t, 100.0, float64(limiter.Limit()), 0.001)
	})
}

func TestIntegrationRateLimiter_Wait(t *testing.T) {
	t.Run("allows immediate execution when under limit", func(t *testing.T) {
		irl := NewIntegrationRateLimiter()
		ctx := context.Background()

		// High rate to ensure we're under limit
		start := time.Now()
		err := irl.Wait(ctx, "integration-1", 6000)
		elapsed := time.Since(start)

		assert.NoError(t, err)
		// Should complete almost immediately (< 100ms)
		assert.Less(t, elapsed, 100*time.Millisecond)
	})

	t.Run("returns error on context cancellation", func(t *testing.T) {
		irl := NewIntegrationRateLimiter()

		// Very low rate - 1 per minute
		// First call should succeed
		ctx1 := context.Background()
		err := irl.Wait(ctx1, "integration-1", 1)
		assert.NoError(t, err)

		// Second call should block, but context will be cancelled
		ctx2, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		err = irl.Wait(ctx2, "integration-1", 1)
		assert.Error(t, err)
	})

	t.Run("returns error on context timeout", func(t *testing.T) {
		irl := NewIntegrationRateLimiter()

		// Very low rate - 1 per minute
		// First call consumes the burst
		ctx1 := context.Background()
		err := irl.Wait(ctx1, "integration-1", 1)
		assert.NoError(t, err)

		// Second call with short timeout
		ctx2, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()

		err = irl.Wait(ctx2, "integration-1", 1)
		assert.Error(t, err)
	})
}

func TestIntegrationRateLimiter_Allow(t *testing.T) {
	t.Run("returns true when under limit", func(t *testing.T) {
		irl := NewIntegrationRateLimiter()

		// First request should be allowed (burst = 1)
		allowed := irl.Allow("integration-1", 60)
		assert.True(t, allowed)
	})

	t.Run("returns false when at limit", func(t *testing.T) {
		irl := NewIntegrationRateLimiter()

		// First request consumes the burst
		allowed1 := irl.Allow("integration-1", 60)
		assert.True(t, allowed1)

		// Second immediate request should be rate limited
		allowed2 := irl.Allow("integration-1", 60)
		assert.False(t, allowed2)
	})

	t.Run("allows again after rate period", func(t *testing.T) {
		irl := NewIntegrationRateLimiter()

		// High rate for quick test - 6000 per minute = 100 per second
		// Token replenishes every 10ms
		allowed1 := irl.Allow("integration-1", 6000)
		assert.True(t, allowed1)

		// Wait for token to replenish
		time.Sleep(15 * time.Millisecond)

		allowed2 := irl.Allow("integration-1", 6000)
		assert.True(t, allowed2)
	})
}

func TestIntegrationRateLimiter_Reserve(t *testing.T) {
	t.Run("returns zero delay when under limit", func(t *testing.T) {
		irl := NewIntegrationRateLimiter()

		reservation := irl.Reserve("integration-1", 60)

		require.NotNil(t, reservation)
		assert.True(t, reservation.OK())
		// First reservation should have zero or near-zero delay
		assert.Less(t, reservation.Delay(), 100*time.Millisecond)
	})

	t.Run("returns positive delay when at limit", func(t *testing.T) {
		irl := NewIntegrationRateLimiter()

		// First reservation consumes burst
		reservation1 := irl.Reserve("integration-1", 60) // 1 per second
		require.NotNil(t, reservation1)
		assert.True(t, reservation1.OK())

		// Second reservation should have a delay
		reservation2 := irl.Reserve("integration-1", 60)
		require.NotNil(t, reservation2)
		assert.True(t, reservation2.OK())
		// Should have approximately 1 second delay (1 per second rate)
		assert.Greater(t, reservation2.Delay(), 500*time.Millisecond)
	})
}

func TestIntegrationRateLimiter_GetCurrentRate(t *testing.T) {
	t.Run("returns rate for known integration", func(t *testing.T) {
		irl := NewIntegrationRateLimiter()

		// Create a limiter first
		irl.GetOrCreateLimiter("integration-1", 120) // 2 per second

		rate := irl.GetCurrentRate("integration-1")
		assert.InDelta(t, 2.0, rate, 0.001)
	})

	t.Run("returns 0 for unknown integration", func(t *testing.T) {
		irl := NewIntegrationRateLimiter()

		rate := irl.GetCurrentRate("unknown-integration")
		assert.Equal(t, 0.0, rate)
	})
}

func TestIntegrationRateLimiter_GetStats(t *testing.T) {
	t.Run("returns stats for all limiters", func(t *testing.T) {
		irl := NewIntegrationRateLimiter()

		// Create multiple limiters
		irl.GetOrCreateLimiter("integration-1", 60)  // 1 per second
		irl.GetOrCreateLimiter("integration-2", 120) // 2 per second
		irl.GetOrCreateLimiter("integration-3", 180) // 3 per second

		stats := irl.GetStats()

		assert.Len(t, stats, 3)

		// Check integration-1 stats
		stat1, ok := stats["integration-1"]
		require.True(t, ok)
		assert.InDelta(t, 1.0, stat1.RatePerSecond, 0.001)
		assert.InDelta(t, 60.0, stat1.RatePerMinute, 0.1)
		assert.Equal(t, 1, stat1.Burst)

		// Check integration-2 stats
		stat2, ok := stats["integration-2"]
		require.True(t, ok)
		assert.InDelta(t, 2.0, stat2.RatePerSecond, 0.001)
		assert.InDelta(t, 120.0, stat2.RatePerMinute, 0.1)

		// Check integration-3 stats
		stat3, ok := stats["integration-3"]
		require.True(t, ok)
		assert.InDelta(t, 3.0, stat3.RatePerSecond, 0.001)
	})

	t.Run("returns empty map when no limiters", func(t *testing.T) {
		irl := NewIntegrationRateLimiter()

		stats := irl.GetStats()
		assert.Empty(t, stats)
	})
}

func TestIntegrationRateLimiter_Clear(t *testing.T) {
	t.Run("removes all limiters", func(t *testing.T) {
		irl := NewIntegrationRateLimiter()

		// Create multiple limiters
		irl.GetOrCreateLimiter("integration-1", 60)
		irl.GetOrCreateLimiter("integration-2", 120)
		irl.GetOrCreateLimiter("integration-3", 180)

		// Verify they exist
		assert.Len(t, irl.GetStats(), 3)

		// Clear all
		irl.Clear()

		// Verify all removed
		assert.Empty(t, irl.GetStats())
	})

	t.Run("stats returns empty after clear", func(t *testing.T) {
		irl := NewIntegrationRateLimiter()

		irl.GetOrCreateLimiter("integration-1", 60)
		irl.Clear()

		stats := irl.GetStats()
		assert.Empty(t, stats)
	})
}

func TestIntegrationRateLimiter_Remove(t *testing.T) {
	t.Run("removes specific limiter", func(t *testing.T) {
		irl := NewIntegrationRateLimiter()

		// Create multiple limiters
		irl.GetOrCreateLimiter("integration-1", 60)
		irl.GetOrCreateLimiter("integration-2", 120)
		irl.GetOrCreateLimiter("integration-3", 180)

		// Remove one
		irl.Remove("integration-2")

		// Verify only integration-2 is removed
		stats := irl.GetStats()
		assert.Len(t, stats, 2)
		_, ok1 := stats["integration-1"]
		assert.True(t, ok1)
		_, ok2 := stats["integration-2"]
		assert.False(t, ok2)
		_, ok3 := stats["integration-3"]
		assert.True(t, ok3)
	})

	t.Run("no-op for unknown integration", func(t *testing.T) {
		irl := NewIntegrationRateLimiter()

		// Create a limiter
		irl.GetOrCreateLimiter("integration-1", 60)

		// Remove unknown - should not panic or affect existing
		irl.Remove("unknown-integration")

		stats := irl.GetStats()
		assert.Len(t, stats, 1)
	})
}

func TestIntegrationRateLimiter_ConcurrentAccess(t *testing.T) {
	t.Run("handles concurrent limiter creation safely", func(t *testing.T) {
		irl := NewIntegrationRateLimiter()

		var wg sync.WaitGroup
		numGoroutines := 100
		integrationID := "integration-1"

		wg.Add(numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			go func(rate int) {
				defer wg.Done()
				limiter := irl.GetOrCreateLimiter(integrationID, rate)
				assert.NotNil(t, limiter)
			}(60 + i)
		}
		wg.Wait()

		// Should have exactly one limiter
		stats := irl.GetStats()
		assert.Len(t, stats, 1)
	})

	t.Run("handles concurrent access to different integrations", func(t *testing.T) {
		irl := NewIntegrationRateLimiter()

		var wg sync.WaitGroup
		numGoroutines := 50

		wg.Add(numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			go func(idx int) {
				defer wg.Done()
				integrationID := string(rune('A' + idx%26))
				limiter := irl.GetOrCreateLimiter(integrationID, 60+idx)
				assert.NotNil(t, limiter)
			}(i)
		}
		wg.Wait()

		// Should have up to 26 unique limiters (A-Z)
		stats := irl.GetStats()
		assert.LessOrEqual(t, len(stats), 26)
		assert.Greater(t, len(stats), 0)
	})

	t.Run("concurrent Wait calls complete without deadlock", func(t *testing.T) {
		irl := NewIntegrationRateLimiter()

		var wg sync.WaitGroup
		numGoroutines := 10
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		wg.Add(numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			go func() {
				defer wg.Done()
				// High rate to ensure quick completion
				err := irl.Wait(ctx, "integration-1", 60000)
				// May succeed or fail based on context, but should not hang
				_ = err
			}()
		}

		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-done:
			// Test passed - all goroutines completed
		case <-time.After(6 * time.Second):
			t.Fatal("test timed out - possible deadlock")
		}
	})

	t.Run("concurrent Allow calls return consistent results", func(t *testing.T) {
		irl := NewIntegrationRateLimiter()

		var wg sync.WaitGroup
		var allowedCount int64
		var mu sync.Mutex
		numGoroutines := 100

		wg.Add(numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			go func() {
				defer wg.Done()
				if irl.Allow("integration-1", 60) {
					mu.Lock()
					allowedCount++
					mu.Unlock()
				}
			}()
		}
		wg.Wait()

		// With burst of 1, at most a few should be allowed in immediate succession
		// The exact number depends on timing, but should be much less than 100
		assert.Less(t, allowedCount, int64(numGoroutines))
		assert.Greater(t, allowedCount, int64(0))
	})
}

func TestRateLimiterStats(t *testing.T) {
	t.Run("contains expected fields", func(t *testing.T) {
		stats := RateLimiterStats{
			RatePerSecond:   2.5,
			RatePerMinute:   150.0,
			TokensAvailable: 0.5,
			Burst:           1,
		}

		assert.Equal(t, 2.5, stats.RatePerSecond)
		assert.Equal(t, 150.0, stats.RatePerMinute)
		assert.Equal(t, 0.5, stats.TokensAvailable)
		assert.Equal(t, 1, stats.Burst)
	})
}
