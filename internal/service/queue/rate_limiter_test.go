package queue

import (
	"sort"
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

func TestIntegrationRateLimiter_ConfiguredRate(t *testing.T) {
	t.Run("a new integration starts at its configured rate, one token at a time", func(t *testing.T) {
		irl := NewIntegrationRateLimiter()

		assert.Equal(t, 60, irl.EffectivePerMinute("integration-1", 60))
		assert.InDelta(t, 1.0, irl.GetCurrentRate("integration-1"), 0.001)
		assert.Equal(t, 1, irl.GetStats()["integration-1"].Burst)
	})

	t.Run("an edited rate applies at once", func(t *testing.T) {
		irl := NewIntegrationRateLimiter()

		irl.EffectivePerMinute("integration-1", 60)
		assert.Equal(t, 120, irl.EffectivePerMinute("integration-1", 120))
		assert.InDelta(t, 2.0, irl.GetCurrentRate("integration-1"), 0.001)
	})

	t.Run("enforces minimum rate of 1 per minute", func(t *testing.T) {
		irl := NewIntegrationRateLimiter()

		assert.Equal(t, 1, irl.EffectivePerMinute("zero", 0))
		assert.Equal(t, 1, irl.EffectivePerMinute("negative", -100))
		assert.InDelta(t, 1.0/60.0, irl.GetCurrentRate("zero"), 0.0001)
	})

	t.Run("handles high rate correctly", func(t *testing.T) {
		irl := NewIntegrationRateLimiter()

		assert.Equal(t, 6000, irl.EffectivePerMinute("integration-1", 6000))
		assert.InDelta(t, 100.0, irl.GetCurrentRate("integration-1"), 0.001)
	})

	t.Run("a rate that does not divide by 60 is reported as configured", func(t *testing.T) {
		irl := NewIntegrationRateLimiter()

		assert.Equal(t, 29, irl.EffectivePerMinute("integration-1", 29))
	})
}

func TestIntegrationRateLimiter_TokenDueBy(t *testing.T) {
	irl := NewIntegrationRateLimiter()
	now := time.Now()
	assert.True(t, irl.TokenDueBy("integ", 60, now), "a new limiter has its token now")

	_, ok := irl.ReserveBefore("integ", 60, now.Add(time.Second))
	require.True(t, ok)
	// 60/min: the next token is a second after the one just taken.
	assert.False(t, irl.TokenDueBy("integ", 60, now.Add(500*time.Millisecond)))
	assert.True(t, irl.TokenDueBy("integ", 60, now.Add(1100*time.Millisecond)))

	// Had a check taken the next token, the one after would be two seconds away.
	slot, ok := irl.ReserveBefore("integ", 60, now.Add(1500*time.Millisecond))
	require.True(t, ok, "the checks took nothing")
	assert.LessOrEqual(t, slot.Delay, 1100*time.Millisecond)
}

func TestIntegrationRateLimiter_ReserveBefore(t *testing.T) {
	t.Run("concurrent callers get tokens spaced by the rate", func(t *testing.T) {
		irl := NewIntegrationRateLimiter()
		deadline := time.Now().Add(time.Minute)

		const callers = 10
		delays := make([]time.Duration, callers)
		var wg sync.WaitGroup
		for i := 0; i < callers; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				slot, ok := irl.ReserveBefore("integration-1", 600, deadline) // one every 100ms
				assert.True(t, ok)
				assert.NotNil(t, slot.Reservation)
				delays[i] = slot.Delay
			}(i)
		}
		wg.Wait()

		sorted := append([]time.Duration(nil), delays...)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
		assert.Less(t, sorted[0], 10*time.Millisecond, "the first caller gets the banked token")
		for i := 1; i < callers; i++ {
			assert.InDelta(t, float64(100*time.Millisecond), float64(sorted[i]-sorted[i-1]), float64(10*time.Millisecond),
				"each token comes one interval after the previous one")
		}
	})

	t.Run("a token due after the deadline is refused and given back", func(t *testing.T) {
		irl := NewIntegrationRateLimiter()
		deadline := time.Now().Add(1500 * time.Millisecond)

		// 60/min: tokens at 0s and 1s fall within the deadline, the one at 2s does not.
		first, ok := irl.ReserveBefore("integration-1", 60, deadline)
		require.True(t, ok)
		second, ok := irl.ReserveBefore("integration-1", 60, deadline)
		require.True(t, ok)
		assert.Less(t, first.Delay, 10*time.Millisecond)
		assert.InDelta(t, float64(time.Second), float64(second.Delay), float64(20*time.Millisecond))

		refused, ok := irl.ReserveBefore("integration-1", 60, deadline)
		assert.False(t, ok)
		assert.Nil(t, refused.Reservation)
		assert.Zero(t, refused.Delay)

		// The refused token was cancelled, so the next caller is offered the same one
		// rather than the one after it.
		next, ok := irl.ReserveBefore("integration-1", 60, time.Now().Add(time.Minute))
		require.True(t, ok)
		assert.InDelta(t, float64(2*time.Second), float64(next.Delay), float64(50*time.Millisecond))
	})
}

func TestIntegrationRateLimiter_GetCurrentRate(t *testing.T) {
	t.Run("returns rate for known integration", func(t *testing.T) {
		irl := NewIntegrationRateLimiter()

		// Create a limiter first
		irl.EffectivePerMinute("integration-1", 120) // 2 per second

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
		irl.EffectivePerMinute("integration-1", 60)  // 1 per second
		irl.EffectivePerMinute("integration-2", 120) // 2 per second
		irl.EffectivePerMinute("integration-3", 180) // 3 per second

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
		irl.EffectivePerMinute("integration-1", 60)
		irl.EffectivePerMinute("integration-2", 120)
		irl.EffectivePerMinute("integration-3", 180)

		// Verify they exist
		assert.Len(t, irl.GetStats(), 3)

		// Clear all
		irl.Clear()

		// Verify all removed
		assert.Empty(t, irl.GetStats())
	})

	t.Run("stats returns empty after clear", func(t *testing.T) {
		irl := NewIntegrationRateLimiter()

		irl.EffectivePerMinute("integration-1", 60)
		irl.Clear()

		stats := irl.GetStats()
		assert.Empty(t, stats)
	})
}

func TestIntegrationRateLimiter_Remove(t *testing.T) {
	t.Run("removes specific limiter", func(t *testing.T) {
		irl := NewIntegrationRateLimiter()

		// Create multiple limiters
		irl.EffectivePerMinute("integration-1", 60)
		irl.EffectivePerMinute("integration-2", 120)
		irl.EffectivePerMinute("integration-3", 180)

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
		irl.EffectivePerMinute("integration-1", 60)

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
				assert.Positive(t, irl.EffectivePerMinute(integrationID, rate))
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
				assert.Positive(t, irl.EffectivePerMinute(integrationID, 60+idx))
			}(i)
		}
		wg.Wait()

		// Should have up to 26 unique limiters (A-Z)
		stats := irl.GetStats()
		assert.LessOrEqual(t, len(stats), 26)
		assert.Greater(t, len(stats), 0)
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
