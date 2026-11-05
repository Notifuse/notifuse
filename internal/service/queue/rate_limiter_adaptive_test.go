package queue

import (
	"cmp"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeClock is a clock the tests move by hand.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{t: time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func newClockedLimiter() (*IntegrationRateLimiter, *fakeClock) {
	clock := newFakeClock()
	return newIntegrationRateLimiter(clock.Now), clock
}

// throttleNow reserves a token, waits for it on the fake clock, and reports the send as
// throttled: what one refused send does.
func throttleNow(t *testing.T, irl *IntegrationRateLimiter, clock *fakeClock, id string, rate int) ThrottleOutcome {
	t.Helper()
	slot, ok := irl.ReserveBefore(id, rate, clock.Now().Add(24*time.Hour))
	require.True(t, ok)
	clock.Advance(slot.Delay)
	return irl.RecordThrottle(id, rate, slot.Generation)
}

// decreasesToFloor is how many cuts take the configured rate to its floor.
func decreasesToFloor() int {
	n := 0
	for share := 1.0; share > throttleFloorShare; share *= throttleDecrease {
		n++
	}
	return n
}

func TestRateLimiter_Throttle(t *testing.T) {
	t.Run("a throttle of the current generation keeps three quarters of the pace", func(t *testing.T) {
		irl, clock := newClockedLimiter()
		irl.EffectivePerMinute("i", 1000)
		gen := irl.CurrentGeneration("i")

		out := throttleNow(t, irl, clock, "i", 1000)

		assert.True(t, out.Decreased)
		assert.False(t, out.AtFloor)
		assert.Equal(t, 1000, out.From)
		assert.Equal(t, 750, out.To)
		assert.Equal(t, 750, irl.EffectivePerMinute("i", 1000))
		assert.Equal(t, gen+1, irl.CurrentGeneration("i"))
	})

	t.Run("a throttle of an older generation changes nothing", func(t *testing.T) {
		irl, clock := newClockedLimiter()
		irl.EffectivePerMinute("i", 1000)
		old := irl.CurrentGeneration("i")
		throttleNow(t, irl, clock, "i", 1000)

		out := irl.RecordThrottle("i", 1000, old)

		assert.Equal(t, ThrottleOutcome{}, out)
		assert.Equal(t, 750, irl.EffectivePerMinute("i", 1000))
		assert.Equal(t, old+1, irl.CurrentGeneration("i"))
	})

	t.Run("the cut drops the tokens reserved ahead at the old pace", func(t *testing.T) {
		irl, clock := newClockedLimiter()
		// 600/min: ten reservations reach a second ahead.
		var last Slot
		for i := 0; i < 10; i++ {
			slot, ok := irl.ReserveBefore("i", 600, clock.Now().Add(time.Hour))
			require.True(t, ok)
			last = slot
		}
		require.Greater(t, last.Delay, 800*time.Millisecond)

		irl.RecordThrottle("i", 600, last.Generation)

		// At 450/min the next token is one interval (133 ms) away, not the debt of the
		// nine reserved ahead stretched to the new pace.
		next, ok := irl.ReserveBefore("i", 600, clock.Now().Add(time.Hour))
		require.True(t, ok)
		assert.InDelta(t, float64(60*time.Second/450), float64(next.Delay), float64(time.Millisecond))
	})

	t.Run("cuts stop at a tenth of the configured rate, and count once the pace has sat there", func(t *testing.T) {
		irl, clock := newClockedLimiter()

		for i := 0; i < decreasesToFloor(); i++ {
			out := throttleNow(t, irl, clock, "i", 1000)
			require.True(t, out.Decreased, "cut %d", i+1)
		}
		assert.Equal(t, 100, irl.EffectivePerMinute("i", 1000))

		out := throttleNow(t, irl, clock, "i", 1000)
		assert.False(t, out.Decreased)
		assert.False(t, out.AtFloor, "just reached the floor")

		clock.Advance(defaultFloorDwell)
		out = throttleNow(t, irl, clock, "i", 1000)
		assert.False(t, out.Decreased)
		assert.True(t, out.AtFloor)
		assert.Equal(t, 100, irl.EffectivePerMinute("i", 1000))
	})

	t.Run("a throttle of the generation before the floor never counts as one at the floor", func(t *testing.T) {
		irl, clock := newClockedLimiter()
		for i := 0; i < decreasesToFloor()-1; i++ {
			throttleNow(t, irl, clock, "i", 1000)
		}
		before := irl.CurrentGeneration("i")
		require.True(t, throttleNow(t, irl, clock, "i", 1000).Decreased)
		clock.Advance(time.Hour)

		assert.False(t, irl.RecordThrottle("i", 1000, before).AtFloor)
	})

	t.Run("the floor is at least one per minute, and at 1/min the first refusal starts the dwell", func(t *testing.T) {
		irl, clock := newClockedLimiter()
		irl.EffectivePerMinute("one", 1)
		gen := irl.CurrentGeneration("one")

		// Created long before its first refusal: the dwell counts from the refusal.
		clock.Advance(time.Hour)
		out := irl.RecordThrottle("one", 1, gen)
		assert.False(t, out.Decreased)
		assert.False(t, out.AtFloor)

		// Refusals of every email, a minute apart at 1/min, are one run.
		clock.Advance(time.Minute)
		assert.True(t, irl.RecordThrottle("one", 1, gen).AtFloor)

		irl.EffectivePerMinute("five", 5)
		for i := 0; i < 10; i++ {
			throttleNow(t, irl, clock, "five", 5)
		}
		assert.Equal(t, 1, irl.EffectivePerMinute("five", 5))
	})

	t.Run("a refusal after a quiet spell at the floor starts a new dwell", func(t *testing.T) {
		irl, clock := newClockedLimiter()
		for i := 0; i < decreasesToFloor(); i++ {
			require.True(t, throttleNow(t, irl, clock, "i", 1000).Decreased)
		}
		gen := irl.CurrentGeneration("i")

		// Left at the floor with nothing sent, as when a broadcast ends right after the
		// refusals, then refused again much later.
		clock.Advance(time.Hour)
		assert.False(t, irl.RecordThrottle("i", 1000, gen).AtFloor, "a new run")

		clock.Advance(defaultFloorDwell / 2)
		assert.False(t, irl.RecordThrottle("i", 1000, gen).AtFloor)
		clock.Advance(defaultFloorDwell / 2)
		assert.True(t, irl.RecordThrottle("i", 1000, gen).AtFloor, "the run has lasted the dwell")
	})

	t.Run("a quiet spell after the cut to the floor is the dwell plus two sends at the floor", func(t *testing.T) {
		// The send interval at the floor falls below, at and above the dwell.
		cases := []struct {
			name     string
			rate     int
			oneRun   time.Duration
			newRun   time.Duration
			floorGap time.Duration
			vsDwell  int
		}{
			{"1000/min, floor 100/min", 1000, defaultFloorDwell + time.Second, defaultFloorDwell + 1300*time.Millisecond, 600 * time.Millisecond, -1},
			{"20/min, floor 2/min", 20, defaultFloorDwell + 50*time.Second, defaultFloorDwell + 61*time.Second, 30 * time.Second, 0},
			{"10/min, floor 1/min", 10, defaultFloorDwell + 100*time.Second, defaultFloorDwell + 121*time.Second, time.Minute, 1},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				require.Equal(t, tc.vsDwell, cmp.Compare(tc.floorGap, defaultFloorDwell))
				for _, gap := range []time.Duration{tc.oneRun, tc.newRun} {
					irl, clock := newClockedLimiter()
					// The last cut reaches the floor: the run counts from it.
					for i := 0; i < decreasesToFloor(); i++ {
						require.True(t, throttleNow(t, irl, clock, "i", tc.rate).Decreased)
					}
					require.Equal(t, tc.floorGap, time.Duration(float64(time.Minute)/float64(irl.EffectivePerMinute("i", tc.rate))))
					gen := irl.CurrentGeneration("i")

					clock.Advance(gap)
					assert.Equal(t, gap == tc.oneRun, irl.RecordThrottle("i", tc.rate, gen).AtFloor, gap)
				}
			})
		}
	})

	t.Run("an accepted email and a recovery interval without a refusal end the run, also at 1/min", func(t *testing.T) {
		irl, clock := newClockedLimiter()
		irl.EffectivePerMinute("one", 1)
		gen := irl.CurrentGeneration("one")

		require.False(t, irl.RecordThrottle("one", 1, gen).AtFloor)
		clock.Advance(time.Minute)
		irl.RecordAccepted("one", gen)
		clock.Advance(time.Minute)
		assert.False(t, irl.RecordThrottle("one", 1, gen).AtFloor, "a new run")
		clock.Advance(time.Minute)
		assert.True(t, irl.RecordThrottle("one", 1, gen).AtFloor)
	})

	t.Run("emails accepted between refusals at the floor do not end the run", func(t *testing.T) {
		irl, clock := newClockedLimiter()
		for i := 0; i < decreasesToFloor(); i++ {
			require.True(t, throttleNow(t, irl, clock, "i", 1000).Decreased)
		}
		gen := irl.CurrentGeneration("i")

		// The provider takes an email now and then, never for a whole recovery interval:
		// the pace stays at the floor.
		for elapsed := time.Duration(0); elapsed < defaultFloorDwell; elapsed += 12 * time.Second {
			irl.RecordAccepted("i", gen)
			clock.Advance(time.Second)
			assert.False(t, irl.RecordThrottle("i", 1000, gen).AtFloor)
			clock.Advance(11 * time.Second)
		}
		assert.True(t, irl.RecordThrottle("i", 1000, gen).AtFloor)
		assert.Equal(t, 100, irl.EffectivePerMinute("i", 1000))
	})

	t.Run("concurrent throttles of one generation cut the pace once", func(t *testing.T) {
		irl, _ := newClockedLimiter()
		irl.EffectivePerMinute("i", 1000)
		gen := irl.CurrentGeneration("i")

		var wg sync.WaitGroup
		for i := 0; i < 20; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				irl.RecordThrottle("i", 1000, gen)
			}()
		}
		wg.Wait()

		assert.Equal(t, 750, irl.EffectivePerMinute("i", 1000))
		assert.Equal(t, gen+1, irl.CurrentGeneration("i"))
	})
}

func TestRateLimiter_Recovery(t *testing.T) {
	t.Run("the pace does not climb back while the provider takes nothing", func(t *testing.T) {
		irl, clock := newClockedLimiter()
		throttleNow(t, irl, clock, "i", 1000)

		clock.Advance(time.Hour)
		assert.Equal(t, 750, irl.EffectivePerMinute("i", 1000))
	})

	t.Run("after an accepted send it climbs by 5 % of the limit per full interval, up to the limit", func(t *testing.T) {
		irl, clock := newClockedLimiter()
		throttleNow(t, irl, clock, "i", 1000)
		irl.RecordAccepted("i", irl.CurrentGeneration("i"))

		clock.Advance(defaultRecoveryInterval - time.Millisecond)
		assert.Equal(t, 750, irl.EffectivePerMinute("i", 1000), "a partial interval counts nothing")
		clock.Advance(time.Millisecond)
		assert.Equal(t, 800, irl.EffectivePerMinute("i", 1000))
		clock.Advance(2 * defaultRecoveryInterval)
		assert.Equal(t, 900, irl.EffectivePerMinute("i", 1000))

		// Idle time after an accepted send counts too.
		clock.Advance(time.Hour)
		assert.Equal(t, 1000, irl.EffectivePerMinute("i", 1000))
		assert.InDelta(t, 1000.0/60, irl.GetCurrentRate("i"), 0.001)
	})

	t.Run("an accepted send of an older generation starts nothing", func(t *testing.T) {
		irl, clock := newClockedLimiter()
		irl.EffectivePerMinute("i", 1000)
		old := irl.CurrentGeneration("i")
		throttleNow(t, irl, clock, "i", 1000)

		irl.RecordAccepted("i", old)
		clock.Advance(time.Hour)
		assert.Equal(t, 750, irl.EffectivePerMinute("i", 1000))
	})

	t.Run("a throttle at the floor stops a recovery an accepted send started", func(t *testing.T) {
		irl, clock := newClockedLimiter()
		for i := 0; i < decreasesToFloor(); i++ {
			throttleNow(t, irl, clock, "i", 1000)
		}
		irl.RecordAccepted("i", irl.CurrentGeneration("i"))

		clock.Advance(5 * time.Second)
		require.False(t, throttleNow(t, irl, clock, "i", 1000).Decreased, "already at the floor")
		clock.Advance(defaultRecoveryInterval)

		assert.Equal(t, 100, irl.EffectivePerMinute("i", 1000))
		assert.False(t, throttleNow(t, irl, clock, "i", 1000).AtFloor)
		clock.Advance(defaultFloorDwell / 2)
		assert.True(t, throttleNow(t, irl, clock, "i", 1000).AtFloor, "the run of refusals went on from the floor")
	})

	t.Run("climbing off the floor and falling back starts a new dwell", func(t *testing.T) {
		irl, clock := newClockedLimiter()
		for i := 0; i < decreasesToFloor(); i++ {
			throttleNow(t, irl, clock, "i", 1000)
		}
		irl.RecordAccepted("i", irl.CurrentGeneration("i"))
		clock.Advance(defaultFloorDwell)
		require.Equal(t, 250, irl.EffectivePerMinute("i", 1000))

		for irl.EffectivePerMinute("i", 1000) > 100 {
			require.True(t, throttleNow(t, irl, clock, "i", 1000).Decreased)
		}
		assert.False(t, throttleNow(t, irl, clock, "i", 1000).AtFloor)
	})

	t.Run("an accepted send with no pace recorded does nothing", func(t *testing.T) {
		irl, _ := newClockedLimiter()
		irl.RecordAccepted("unknown", 3)
		assert.Empty(t, irl.GetStats())
	})
}

func TestRateLimiter_ConfiguredRateChange(t *testing.T) {
	for _, rate := range []int{500, 2000} {
		irl, clock := newClockedLimiter()
		throttleNow(t, irl, clock, "i", 1000)
		before := irl.CurrentGeneration("i")

		assert.Equal(t, rate, irl.EffectivePerMinute("i", rate), "an edited limit starts over at %d", rate)
		assert.Equal(t, before+1, irl.CurrentGeneration("i"))
		assert.Equal(t, ThrottleOutcome{}, irl.RecordThrottle("i", rate, before))
	}
}

func TestRateLimiter_EffectivePaceDrivesTokens(t *testing.T) {
	irl, clock := newClockedLimiter()
	throttleNow(t, irl, clock, "i", 600) // 450/min: one token every 133 ms

	assert.False(t, irl.TokenDueBy("i", 600, clock.Now().Add(100*time.Millisecond)))
	assert.True(t, irl.TokenDueBy("i", 600, clock.Now().Add(140*time.Millisecond)))

	slot, ok := irl.ReserveBefore("i", 600, clock.Now().Add(time.Second))
	require.True(t, ok)
	assert.Equal(t, irl.CurrentGeneration("i"), slot.Generation)
	assert.InDelta(t, float64(60*time.Second/450), float64(slot.Delay), float64(time.Millisecond))

	stats := irl.GetStats()["i"]
	assert.InDelta(t, 450.0, stats.RatePerMinute, 0.001)
	assert.InDelta(t, 600.0, stats.ConfiguredPerMinute, 0.001)
}

// A provider that refuses every send for its pace must end up counted against, at every
// rate: the climb back never starts, so the pace walks down to its floor and sits there.
func TestRateLimiter_ProviderRefusingEverythingReachesTheFloor(t *testing.T) {
	for _, rate := range []int{1, 25, 60, 600, 6000} {
		irl, clock := newClockedLimiter()
		decreases := 0
		start := clock.Now()
		for {
			out := throttleNow(t, irl, clock, "i", rate)
			if out.Decreased {
				decreases++
			}
			if out.AtFloor {
				break
			}
			require.Less(t, clock.Now().Sub(start), time.Hour, "rate %d never reached the floor", rate)
		}
		assert.LessOrEqual(t, decreases, decreasesToFloor(), "rate %d", rate)
		floor := math.Max(1, throttleFloorShare*float64(rate))
		assert.Equal(t, int(math.Round(math.Min(floor, float64(rate)))), irl.EffectivePerMinute("i", rate), "rate %d", rate)
	}
}
