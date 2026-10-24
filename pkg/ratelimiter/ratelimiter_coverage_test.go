package ratelimiter

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCoverage_AllowBoundaries(t *testing.T) {
	t.Run("maxAttempts 1 allows exactly one", func(t *testing.T) {
		rl := NewRateLimiter()
		defer rl.Stop()
		rl.SetPolicy("ns", 1, time.Minute)
		assert.True(t, rl.Allow("ns", "k"))
		assert.False(t, rl.Allow("ns", "k"))
		assert.False(t, rl.Allow("ns", "k"))
		rl.mu.RLock()
		assert.Len(t, rl.attempts["ns:k"], 1, "denied attempts must not be recorded")
		rl.mu.RUnlock()
	})

	t.Run("attempt exactly at cutoff is expired, just inside is not", func(t *testing.T) {
		rl := NewRateLimiter()
		defer rl.Stop()
		window := time.Minute
		rl.SetPolicy("ns", 1, window)
		now := time.Now()
		rl.mu.Lock()
		rl.attempts["ns:edge"] = []time.Time{now.Add(-window)} // t.After(cutoff) is false at equality
		rl.mu.Unlock()
		assert.True(t, rl.Allow("ns", "edge"), "attempt at exactly now-window must not count")

		rl.mu.Lock()
		rl.attempts["ns:inside"] = []time.Time{now.Add(-window + time.Second)}
		rl.mu.Unlock()
		assert.False(t, rl.Allow("ns", "inside"), "attempt inside the window must count")
	})

	t.Run("denied call prunes expired but does not grow the list", func(t *testing.T) {
		rl := NewRateLimiter()
		defer rl.Stop()
		rl.SetPolicy("ns", 2, time.Minute)
		now := time.Now()
		rl.mu.Lock()
		rl.attempts["ns:k"] = []time.Time{now.Add(-2 * time.Minute), now.Add(-time.Second), now}
		rl.mu.Unlock()
		// Two valid attempts at max 2: denied, expired entry pruned, no append.
		assert.False(t, rl.Allow("ns", "k"))
		rl.mu.RLock()
		assert.Len(t, rl.attempts["ns:k"], 2)
		rl.mu.RUnlock()
		assert.False(t, rl.Allow("ns", "k"))
		rl.mu.RLock()
		assert.Len(t, rl.attempts["ns:k"], 2)
		rl.mu.RUnlock()
	})

	t.Run("empty key is a distinct bucket and empty namespace fails closed", func(t *testing.T) {
		rl := NewRateLimiter()
		defer rl.Stop()
		rl.SetPolicy("ns", 1, time.Minute)
		assert.False(t, rl.Allow("", "k"))
		assert.True(t, rl.Allow("ns", ""))
		assert.False(t, rl.Allow("ns", ""))
		assert.True(t, rl.Allow("ns", "other"))
	})

	t.Run("SetPolicy overwrite raises the limit in place", func(t *testing.T) {
		rl := NewRateLimiter()
		defer rl.Stop()
		rl.SetPolicy("ns", 1, time.Minute)
		assert.True(t, rl.Allow("ns", "k"))
		assert.False(t, rl.Allow("ns", "k"))
		rl.SetPolicy("ns", 3, time.Minute)
		assert.True(t, rl.Allow("ns", "k"))
		assert.True(t, rl.Allow("ns", "k"))
		assert.False(t, rl.Allow("ns", "k"))
		rl.mu.RLock()
		p := rl.policies["ns"]
		rl.mu.RUnlock()
		assert.Equal(t, RatePolicy{MaxAttempts: 3, Window: time.Minute}, p)
	})
}

func TestCoverage_GetRemainingWindowBranches(t *testing.T) {
	t.Run("all attempts expired yields zero", func(t *testing.T) {
		rl := NewRateLimiter()
		defer rl.Stop()
		rl.SetPolicy("ns", 5, time.Minute)
		rl.mu.Lock()
		rl.attempts["ns:k"] = []time.Time{time.Now().Add(-2 * time.Minute)}
		rl.mu.Unlock()
		assert.Equal(t, 0, rl.GetRemainingWindow("ns", "k"))
	})

	t.Run("oldest valid attempt drives the value even when list is unsorted", func(t *testing.T) {
		rl := NewRateLimiter()
		defer rl.Stop()
		rl.SetPolicy("ns", 5, 60*time.Second)
		now := time.Now()
		rl.mu.Lock()
		// Newest first on purpose: the scan must still pick the oldest valid.
		rl.attempts["ns:k"] = []time.Time{now, now.Add(-10 * time.Second)}
		rl.mu.Unlock()
		// Oldest expires 50s from now: int(49.99..)+1 == 50 (allows 1s scheduling slack -> 49 or 50).
		got := rl.GetRemainingWindow("ns", "k")
		assert.GreaterOrEqual(t, got, 49)
		assert.LessOrEqual(t, got, 50)
	})

	t.Run("fresh attempt in 60s window rounds up to the full window", func(t *testing.T) {
		rl := NewRateLimiter()
		defer rl.Stop()
		rl.SetPolicy("ns", 5, 60*time.Second)
		require.True(t, rl.Allow("ns", "k"))
		assert.Equal(t, 60, rl.GetRemainingWindow("ns", "k"))
	})

	t.Run("unknown key with policy yields zero", func(t *testing.T) {
		rl := NewRateLimiter()
		defer rl.Stop()
		rl.SetPolicy("ns", 5, time.Minute)
		assert.Equal(t, 0, rl.GetRemainingWindow("ns", "never-seen"))
	})
}

func TestCoverage_ResetIsolation(t *testing.T) {
	rl := NewRateLimiter()
	defer rl.Stop()
	rl.SetPolicy("ns", 1, time.Minute)
	assert.True(t, rl.Allow("ns", "a"))
	assert.True(t, rl.Allow("ns", "b"))
	rl.Reset("ns", "a")
	assert.True(t, rl.Allow("ns", "a"), "reset key must be allowed again")
	assert.False(t, rl.Allow("ns", "b"), "other key must stay exhausted")
	assert.NotPanics(t, func() { rl.Reset("ns", "never-seen") })
	assert.NotPanics(t, func() { rl.Reset("no-policy", "a") })
}

// TestCoverage_CleanupTick exercises the real cleanup goroutine's tick branch:
// expired entries are dropped, live entries survive, and entries whose
// namespace lost its policy are dropped. The ticker period is one minute, so
// this test polls for up to 75s.
func TestCoverage_CleanupTick(t *testing.T) {
	if testing.Short() {
		t.Skip("cleanup tick period is one minute")
	}
	rl := NewRateLimiter()
	defer rl.Stop()
	rl.SetPolicy("keep", 5, time.Hour)
	rl.SetPolicy("short", 5, time.Minute)

	now := time.Now()
	rl.mu.Lock()
	rl.attempts["short:expired"] = []time.Time{now.Add(-2 * time.Minute)}
	rl.attempts["keep:fresh"] = []time.Time{now}
	rl.attempts["orphan:key"] = []time.Time{now} // no policy for "orphan"
	rl.mu.Unlock()

	deadline := time.Now().Add(75 * time.Second)
	for {
		rl.mu.RLock()
		_, expiredPresent := rl.attempts["short:expired"]
		_, orphanPresent := rl.attempts["orphan:key"]
		_, freshPresent := rl.attempts["keep:fresh"]
		rl.mu.RUnlock()
		if !expiredPresent && !orphanPresent {
			assert.True(t, freshPresent, "cleanup must keep entries with recent attempts")
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("cleanup tick did not prune within 75s (expired=%v orphan=%v)", expiredPresent, orphanPresent)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func TestCoverage_CleanupStops(t *testing.T) {
	rl := &RateLimiter{
		attempts:    make(map[string][]time.Time),
		policies:    make(map[string]RatePolicy),
		stopCleanup: make(chan struct{}),
	}
	done := make(chan struct{})
	go func() {
		rl.cleanup()
		close(done)
	}()
	rl.Stop()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("cleanup did not return after Stop")
	}
	assert.NotPanics(t, func() { rl.Stop() }, "double Stop must be safe")
}
