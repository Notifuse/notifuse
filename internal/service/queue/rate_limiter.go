package queue

import (
	"math"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// How an integration's pace follows its provider. A refusal for the sending pace keeps
// throttleDecrease of the current pace, never below throttleFloorShare of the configured
// rate (and never below 1/min). Once the provider accepts again, the pace climbs back by
// recoveryShare of the configured rate every recoveryInterval. A refusal at the floor
// counts against the provider only once the pace has sat there for floorDwell, so a
// burst of refusals of a few seconds, which reaches the floor in under two seconds at
// high rates, does not pause broadcasts.
const (
	throttleDecrease        = 0.75
	throttleFloorShare      = 0.10
	recoveryShare           = 0.05
	defaultRecoveryInterval = 10 * time.Second
	defaultFloorDwell       = 30 * time.Second
)

// IntegrationRateLimiter paces each email integration: at its configured rate, or more
// slowly while its provider refuses emails for arriving too fast.
type IntegrationRateLimiter struct {
	limiters sync.Map // map[integrationID]*adaptiveLimiter

	// now, recoveryInterval and floorDwell are fixed at construction; tests shorten or
	// replace them.
	now              func() time.Time
	recoveryInterval time.Duration
	floorDwell       time.Duration
}

// adaptiveLimiter is one integration's pace. Rates are per minute; x/time/rate gets them
// per second.
type adaptiveLimiter struct {
	mu      sync.Mutex
	limiter *rate.Limiter
	// ceiling is the configured rate; floor <= effective <= ceiling.
	ceiling   float64
	effective float64
	// generation counts decreases and resets. A send carries the generation it was
	// reserved in, so the sends throttled together cut the pace once, and a send
	// reserved before a decrease is not mistaken for one sent at the new pace.
	generation uint64
	// recovering is set by an accepted send after a throttle: the pace climbs back only
	// once the provider has taken an email at the lower pace, not while it waits.
	recovering bool
	stepFrom   time.Time
	// floorSince is when the pace reached the floor, zero while above it.
	floorSince time.Time
}

// Slot is a reserved token: when it falls due, and the generation it was taken in.
type Slot struct {
	Reservation *rate.Reservation
	Delay       time.Duration
	Generation  uint64
}

// ThrottleOutcome is what a refusal for the sending pace did to the integration's pace.
type ThrottleOutcome struct {
	// AtFloor means the pace has sat at its floor for the dwell: slowing down has been
	// tried to the end, and the refusal should count against the provider.
	AtFloor bool
	// Decreased reports a cut of the pace, From and To per minute.
	Decreased bool
	From      int
	To        int
}

// NewIntegrationRateLimiter creates a new IntegrationRateLimiter
func NewIntegrationRateLimiter() *IntegrationRateLimiter {
	return newIntegrationRateLimiter(time.Now)
}

func newIntegrationRateLimiter(now func() time.Time) *IntegrationRateLimiter {
	return &IntegrationRateLimiter{
		now:              now,
		recoveryInterval: defaultRecoveryInterval,
		floorDwell:       defaultFloorDwell,
	}
}

// ceilingFor is the configured rate per minute, at least 1.
func ceilingFor(ratePerMinute int) float64 {
	if ratePerMinute < 1 {
		return 1
	}
	return float64(ratePerMinute)
}

func perSecond(perMinute float64) rate.Limit {
	return rate.Limit(perMinute / 60)
}

// state returns the integration's pace, creating it at the configured rate. Callers
// lock it and call sync before reading it.
func (irl *IntegrationRateLimiter) state(integrationID string, ratePerMinute int) *adaptiveLimiter {
	if existing, ok := irl.limiters.Load(integrationID); ok {
		return existing.(*adaptiveLimiter)
	}
	al := &adaptiveLimiter{}
	al.reset(irl.now(), ceilingFor(ratePerMinute))
	actual, _ := irl.limiters.LoadOrStore(integrationID, al)
	return actual.(*adaptiveLimiter)
}

func (al *adaptiveLimiter) floor() float64 {
	return math.Min(al.ceiling, math.Max(throttleFloorShare*al.ceiling, 1))
}

// reset starts over at the configured rate. Editing the limit is a fresh start in both
// directions. The generation keeps counting, so no send reserved before the reset is
// taken for one reserved after it.
func (al *adaptiveLimiter) reset(now time.Time, ceiling float64) {
	al.ceiling = ceiling
	al.effective = ceiling
	al.limiter = rate.NewLimiter(perSecond(ceiling), 1)
	al.generation++
	al.recovering = false
	al.stepFrom = now
	al.floorSince = time.Time{}
	if al.effective <= al.floor() {
		// At 1/min the configured rate is its own floor.
		al.floorSince = now
	}
}

// sync applies a change of the configured rate and the recovery due by now. Callers hold
// the mutex.
func (al *adaptiveLimiter) sync(now time.Time, ceiling float64, interval time.Duration) {
	if ceiling != al.ceiling {
		al.reset(now, ceiling)
		return
	}
	al.recover(now, interval)
}

// recover climbs back towards the ceiling, one step per full interval since stepFrom.
// Raising the limit has no reserved tokens to stretch. Callers hold the mutex.
func (al *adaptiveLimiter) recover(now time.Time, interval time.Duration) {
	if !al.recovering || al.effective >= al.ceiling {
		return
	}
	steps := int64(now.Sub(al.stepFrom) / interval)
	if steps <= 0 {
		return
	}
	al.effective = math.Min(al.ceiling, al.effective+float64(steps)*recoveryShare*al.ceiling)
	al.stepFrom = al.stepFrom.Add(time.Duration(steps) * interval)
	al.limiter.SetLimitAt(now, perSecond(al.effective))
	if al.effective > al.floor() {
		al.floorSince = time.Time{}
	}
}

// lock returns the integration's pace, locked and brought up to date.
func (irl *IntegrationRateLimiter) lock(integrationID string, ratePerMinute int) (*adaptiveLimiter, time.Time) {
	al := irl.state(integrationID, ratePerMinute)
	al.mu.Lock()
	now := irl.now()
	al.sync(now, ceilingFor(ratePerMinute), irl.recoveryInterval)
	return al, now
}

// ReserveBefore reserves the integration's next token if it falls due by deadline, and
// returns it with how long to wait for it. A token due after the deadline is not taken:
// its reservation is cancelled at once, so the next caller is offered the same token
// instead of the one after it.
func (irl *IntegrationRateLimiter) ReserveBefore(integrationID string, ratePerMinute int, deadline time.Time) (Slot, bool) {
	al, now := irl.lock(integrationID, ratePerMinute)
	defer al.mu.Unlock()

	r := al.limiter.ReserveN(now, 1)
	if !r.OK() {
		return Slot{}, false
	}
	delay := r.DelayFrom(now)
	if now.Add(delay).After(deadline) {
		r.CancelAt(now)
		return Slot{}, false
	}
	return Slot{Reservation: r, Delay: delay, Generation: al.generation}, true
}

// TokenDueBy reports whether the integration's next token falls due by t. It takes
// nothing, so a caller can skip work that would find no token to send with.
func (irl *IntegrationRateLimiter) TokenDueBy(integrationID string, ratePerMinute int, t time.Time) bool {
	al, _ := irl.lock(integrationID, ratePerMinute)
	defer al.mu.Unlock()
	return al.limiter.TokensAt(t) >= 1
}

// EffectivePerMinute is the integration's current pace, per minute.
func (irl *IntegrationRateLimiter) EffectivePerMinute(integrationID string, ratePerMinute int) int {
	al, _ := irl.lock(integrationID, ratePerMinute)
	defer al.mu.Unlock()
	return int(math.Round(al.effective))
}

// CurrentGeneration is the generation the integration's next token would be reserved
// in, 0 for an integration with no pace yet.
func (irl *IntegrationRateLimiter) CurrentGeneration(integrationID string) uint64 {
	existing, ok := irl.limiters.Load(integrationID)
	if !ok {
		return 0
	}
	al := existing.(*adaptiveLimiter)
	al.mu.Lock()
	defer al.mu.Unlock()
	return al.generation
}

// RecordThrottle records that the provider refused a send of the given generation for
// arriving too fast. A refusal of an older generation changes nothing: it comes from a
// send reserved before the latest decrease, and the sends throttled together cut the
// pace once. A refusal of the current generation stops any recovery, then cuts the pace,
// or at the floor reports AtFloor once the pace has sat there for the dwell.
//
// The cut replaces the limiter rather than lowering its limit: the tokens reserved ahead
// by the sends waiting in this round are a debt that a lowered limit keeps and
// stretches, which at the floor would hold the integration for minutes. The new limiter
// starts with its token taken, so the next send waits one interval at the new pace.
func (irl *IntegrationRateLimiter) RecordThrottle(integrationID string, ratePerMinute int, generation uint64) ThrottleOutcome {
	al, now := irl.lock(integrationID, ratePerMinute)
	defer al.mu.Unlock()

	if generation != al.generation {
		return ThrottleOutcome{}
	}
	al.recovering = false

	floor := al.floor()
	if al.effective <= floor {
		if al.floorSince.IsZero() {
			al.floorSince = now
		}
		return ThrottleOutcome{AtFloor: now.Sub(al.floorSince) >= irl.floorDwell}
	}

	from := al.effective
	al.effective = math.Max(floor, al.effective*throttleDecrease)
	if al.effective <= floor {
		al.floorSince = now
	}
	al.generation++
	al.limiter = rate.NewLimiter(perSecond(al.effective), 1)
	al.limiter.ReserveN(now, 1)
	return ThrottleOutcome{Decreased: true, From: int(math.Round(from)), To: int(math.Round(al.effective))}
}

// RecordAccepted records that the provider took a send of the given generation at the
// current pace: it accepted the email, or refused it for its recipient. After a
// throttle, the first one starts the climb back. One of an older generation was sent at
// a pace that no longer applies and starts nothing.
func (irl *IntegrationRateLimiter) RecordAccepted(integrationID string, generation uint64) {
	existing, ok := irl.limiters.Load(integrationID)
	if !ok {
		return
	}
	al := existing.(*adaptiveLimiter)
	al.mu.Lock()
	defer al.mu.Unlock()

	now := irl.now()
	al.recover(now, irl.recoveryInterval)
	if generation != al.generation || al.recovering || al.effective >= al.ceiling {
		return
	}
	al.recovering = true
	al.stepFrom = now
}

// GetCurrentRate returns the current pace of an integration in emails per second
// Returns 0 if no limiter exists for the integration
func (irl *IntegrationRateLimiter) GetCurrentRate(integrationID string) float64 {
	existing, ok := irl.limiters.Load(integrationID)
	if !ok {
		return 0
	}
	al := existing.(*adaptiveLimiter)
	al.mu.Lock()
	defer al.mu.Unlock()
	al.recover(irl.now(), irl.recoveryInterval)
	return float64(perSecond(al.effective))
}

// GetStats returns statistics about all rate limiters
func (irl *IntegrationRateLimiter) GetStats() map[string]RateLimiterStats {
	stats := make(map[string]RateLimiterStats)
	irl.limiters.Range(func(key, value interface{}) bool {
		integrationID := key.(string)
		al := value.(*adaptiveLimiter)
		al.mu.Lock()
		now := irl.now()
		al.recover(now, irl.recoveryInterval)
		stats[integrationID] = RateLimiterStats{
			RatePerSecond:       float64(perSecond(al.effective)),
			RatePerMinute:       al.effective,
			ConfiguredPerMinute: al.ceiling,
			TokensAvailable:     al.limiter.TokensAt(now),
			Burst:               al.limiter.Burst(),
		}
		al.mu.Unlock()
		return true
	})
	return stats
}

// RateLimiterStats contains statistics for a single rate limiter. RatePerSecond and
// RatePerMinute are the current pace, below ConfiguredPerMinute while the provider is
// asking to slow down.
type RateLimiterStats struct {
	RatePerSecond       float64 `json:"rate_per_second"`
	RatePerMinute       float64 `json:"rate_per_minute"`
	ConfiguredPerMinute float64 `json:"configured_per_minute"`
	TokensAvailable     float64 `json:"tokens_available"`
	Burst               int     `json:"burst"`
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
