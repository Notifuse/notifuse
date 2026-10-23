package queue

import (
	"os"
	"sync"
	"time"

	"github.com/Notifuse/notifuse/pkg/emailerror"
)

// getCircuitBreakerCooldown returns the circuit breaker cooldown period.
// Can be overridden via CIRCUIT_BREAKER_COOLDOWN environment variable for testing.
// Default is 1 minute.
func getCircuitBreakerCooldown() time.Duration {
	if cooldown := os.Getenv("CIRCUIT_BREAKER_COOLDOWN"); cooldown != "" {
		if d, err := time.ParseDuration(cooldown); err == nil {
			return d
		}
	}
	return 1 * time.Minute
}

// CircuitBreakerConfig holds configuration for circuit breakers
type CircuitBreakerConfig struct {
	// Threshold is the number of provider failures before opening the circuit
	Threshold int

	// CooldownPeriod is how long to wait before attempting to close the circuit
	CooldownPeriod time.Duration
}

// DefaultCircuitBreakerConfig returns sensible defaults
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		Threshold:      5,
		CooldownPeriod: getCircuitBreakerCooldown(),
	}
}

// Ticket is what a send carries from Allow to the result it records. Sends run
// concurrently, so a result can arrive after the circuit changed state: the ticket
// says which opening the send predates, however late it actually reached the provider.
type Ticket struct {
	// Epoch is the number of times the circuit had opened when the send was allowed.
	Epoch uint64
	// Probe marks the single send let through an open circuit once its cooldown is
	// over. Only its result can close the circuit.
	Probe bool
}

// CircuitBreaker represents a single integration's circuit state
type CircuitBreaker struct {
	failures       int
	threshold      int
	cooldownPeriod time.Duration
	lastFailure    time.Time
	lastError      *emailerror.ClassifiedError
	isOpen         bool
	openedAt       time.Time
	epoch          uint64
	probeInFlight  bool
	mutex          sync.RWMutex
}

// NewCircuitBreaker creates a new circuit breaker with the given configuration
func NewCircuitBreaker(threshold int, cooldownPeriod time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		threshold:      threshold,
		cooldownPeriod: cooldownPeriod,
	}
}

// IsOpen reports whether the circuit is open. It changes nothing: only a probe's
// result closes an open circuit.
func (cb *CircuitBreaker) IsOpen() bool {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()
	return cb.isOpen
}

// Allow asks to send. A closed circuit allows every send. An open one refuses until
// its cooldown is over, then allows one send, the probe, and refuses the others until
// that probe reports or is cancelled.
func (cb *CircuitBreaker) Allow() (Ticket, bool) {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	if !cb.isOpen {
		return Ticket{Epoch: cb.epoch}, true
	}
	if cb.probeInFlight || time.Since(cb.openedAt) < cb.cooldownPeriod {
		return Ticket{}, false
	}
	cb.probeInFlight = true
	return Ticket{Epoch: cb.epoch, Probe: true}, true
}

// CancelProbe gives back a probe that was never sent, so the next Allow can probe at
// once instead of after another cooldown. Any other ticket is ignored.
func (cb *CircuitBreaker) CancelProbe(ticket Ticket) {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	if cb.isCurrentProbe(ticket) {
		cb.probeInFlight = false
	}
}

// RecordSuccess records a send the provider accepted. On an open circuit only the
// probe closes it; a send allowed before the latest opening says nothing about the
// provider now and changes nothing.
func (cb *CircuitBreaker) RecordSuccess(ticket Ticket) {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	if ticket.Epoch != cb.epoch {
		return
	}
	if cb.isOpen {
		if ticket.Probe {
			cb.close()
		}
		return
	}
	cb.failures = 0
	cb.lastError = nil
}

// RecordFailure records a failed send and reports whether it counted against the
// provider, and whether it opened the circuit — the transition, not the state.
//
// The distinction matters to anything acting on an open circuit. The breaker is
// consulted per send, so a large queue observes "open" many times for one outage.
// And a provider that stays down fails each probe, which opens the circuit again once
// per cooldown period, so acting on it has to be idempotent.
//
// Only provider errors count: a rejection that blames the recipient says nothing about
// the provider's health. On a probe, though, it shows the provider answered, and
// closes the circuit. Results from sends allowed before the latest opening are ignored.
func (cb *CircuitBreaker) RecordFailure(ticket Ticket, classifiedErr *emailerror.ClassifiedError) (counted bool, opened bool) {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	if classifiedErr == nil || classifiedErr.IsRecipientError() {
		if cb.isCurrentProbe(ticket) {
			if classifiedErr == nil {
				// No verdict either way: release the probe rather than hold it out.
				cb.probeInFlight = false
			} else {
				cb.close()
			}
		}
		return false, false
	}

	if ticket.Epoch != cb.epoch {
		return false, false
	}
	if cb.isOpen {
		if !ticket.Probe {
			return false, false
		}
		cb.open(classifiedErr)
		return true, true
	}

	cb.failures++
	cb.lastFailure = time.Now()
	cb.lastError = classifiedErr
	if cb.failures >= cb.threshold {
		cb.open(classifiedErr)
		return true, true
	}
	return true, false
}

// isCurrentProbe reports whether ticket is the probe out on the current opening.
// Callers hold the mutex.
func (cb *CircuitBreaker) isCurrentProbe(ticket Ticket) bool {
	return cb.isOpen && ticket.Probe && ticket.Epoch == cb.epoch
}

// open starts a new epoch, which voids every ticket handed out before it. Callers
// hold the mutex.
func (cb *CircuitBreaker) open(classifiedErr *emailerror.ClassifiedError) {
	now := time.Now()
	cb.isOpen = true
	cb.epoch++
	cb.openedAt = now
	cb.probeInFlight = false
	cb.lastFailure = now
	cb.lastError = classifiedErr
}

// close is called on a probe's verdict. Callers hold the mutex.
func (cb *CircuitBreaker) close() {
	cb.isOpen = false
	cb.probeInFlight = false
	cb.failures = 0
	cb.lastError = nil
}

// GetLastError returns the last error that caused a failure
func (cb *CircuitBreaker) GetLastError() *emailerror.ClassifiedError {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()
	return cb.lastError
}

// GetFailures returns the current failure count
func (cb *CircuitBreaker) GetFailures() int {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()
	return cb.failures
}

// IntegrationCircuitBreaker manages circuit breakers per integration
type IntegrationCircuitBreaker struct {
	breakers sync.Map // map[integrationID]*CircuitBreaker
	config   CircuitBreakerConfig
}

// NewIntegrationCircuitBreaker creates a new per-integration circuit breaker manager
func NewIntegrationCircuitBreaker(config CircuitBreakerConfig) *IntegrationCircuitBreaker {
	if config.Threshold == 0 {
		config.Threshold = 5
	}
	if config.CooldownPeriod == 0 {
		config.CooldownPeriod = getCircuitBreakerCooldown()
	}

	return &IntegrationCircuitBreaker{
		config: config,
	}
}

// getOrCreateBreaker gets or creates a circuit breaker for an integration
func (icb *IntegrationCircuitBreaker) getOrCreateBreaker(integrationID string) *CircuitBreaker {
	if cb, ok := icb.breakers.Load(integrationID); ok {
		return cb.(*CircuitBreaker)
	}

	newCB := NewCircuitBreaker(icb.config.Threshold, icb.config.CooldownPeriod)
	actual, _ := icb.breakers.LoadOrStore(integrationID, newCB)
	return actual.(*CircuitBreaker)
}

// IsOpen reports whether the circuit for an integration is open
func (icb *IntegrationCircuitBreaker) IsOpen(integrationID string) bool {
	if cb, ok := icb.breakers.Load(integrationID); ok {
		return cb.(*CircuitBreaker).IsOpen()
	}
	return false
}

// Allow asks to send through an integration (see CircuitBreaker.Allow)
func (icb *IntegrationCircuitBreaker) Allow(integrationID string) (Ticket, bool) {
	if cb, ok := icb.breakers.Load(integrationID); ok {
		return cb.(*CircuitBreaker).Allow()
	}
	// No failure recorded yet: closed, at the first epoch.
	return Ticket{}, true
}

// CancelProbe gives back a probe that was never sent
func (icb *IntegrationCircuitBreaker) CancelProbe(integrationID string, ticket Ticket) {
	if cb, ok := icb.breakers.Load(integrationID); ok {
		cb.(*CircuitBreaker).CancelProbe(ticket)
	}
}

// RecordSuccess records a successful send for an integration
func (icb *IntegrationCircuitBreaker) RecordSuccess(integrationID string, ticket Ticket) {
	if cb, ok := icb.breakers.Load(integrationID); ok {
		cb.(*CircuitBreaker).RecordSuccess(ticket)
	}
}

// RecordFailure records a failure for an integration.
// Only provider errors count toward the threshold: a rejection that blames the
// recipient says nothing about the provider's health, and letting those open the
// circuit would let a handful of dead addresses throttle a whole workspace. A probe's
// result always reaches the breaker, since any answer from the provider settles it.
//
// counted reports whether the error was attributed to the provider at all; opened
// reports the moment the circuit flipped, which is the only usable trigger for acting
// on an outage.
func (icb *IntegrationCircuitBreaker) RecordFailure(integrationID string, ticket Ticket, classifiedErr *emailerror.ClassifiedError) (counted bool, opened bool) {
	if !ticket.Probe && (classifiedErr == nil || classifiedErr.IsRecipientError()) {
		return false, false
	}

	cb := icb.getOrCreateBreaker(integrationID)
	return cb.RecordFailure(ticket, classifiedErr)
}

// GetLastError returns the last error for an integration
func (icb *IntegrationCircuitBreaker) GetLastError(integrationID string) *emailerror.ClassifiedError {
	if cb, ok := icb.breakers.Load(integrationID); ok {
		return cb.(*CircuitBreaker).GetLastError()
	}
	return nil
}

// GetConfig returns the circuit breaker configuration
func (icb *IntegrationCircuitBreaker) GetConfig() CircuitBreakerConfig {
	return icb.config
}

// CircuitBreakerStats contains statistics for a circuit breaker
type CircuitBreakerStats struct {
	IsOpen       bool          `json:"is_open"`
	Failures     int           `json:"failures"`
	Threshold    int           `json:"threshold"`
	LastFailure  time.Time     `json:"last_failure,omitempty"`
	CooldownLeft time.Duration `json:"cooldown_left,omitempty"`
	// ProbeInFlight is set while the one send let through after the cooldown is out.
	ProbeInFlight bool `json:"probe_in_flight,omitempty"`
}

// GetStats returns statistics for all circuit breakers
func (icb *IntegrationCircuitBreaker) GetStats() map[string]CircuitBreakerStats {
	stats := make(map[string]CircuitBreakerStats)
	icb.breakers.Range(func(key, value interface{}) bool {
		integrationID := key.(string)
		cb := value.(*CircuitBreaker)

		cb.mutex.RLock()
		stat := CircuitBreakerStats{
			IsOpen:        cb.isOpen,
			Failures:      cb.failures,
			Threshold:     cb.threshold,
			ProbeInFlight: cb.probeInFlight,
		}
		if !cb.lastFailure.IsZero() {
			stat.LastFailure = cb.lastFailure
		}
		if cb.isOpen {
			cooldownLeft := cb.cooldownPeriod - time.Since(cb.openedAt)
			if cooldownLeft > 0 {
				stat.CooldownLeft = cooldownLeft
			}
		}
		cb.mutex.RUnlock()

		stats[integrationID] = stat
		return true
	})
	return stats
}

// Clear removes all circuit breakers (useful for testing)
func (icb *IntegrationCircuitBreaker) Clear() {
	icb.breakers.Range(func(key, value interface{}) bool {
		icb.breakers.Delete(key)
		return true
	})
}

// Remove removes the circuit breaker for a specific integration
func (icb *IntegrationCircuitBreaker) Remove(integrationID string) {
	icb.breakers.Delete(integrationID)
}
