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

// CircuitBreaker represents a single integration's circuit state
type CircuitBreaker struct {
	failures       int
	threshold      int
	cooldownPeriod time.Duration
	lastFailure    time.Time
	lastError      *emailerror.ClassifiedError
	isOpen         bool
	mutex          sync.RWMutex
}

// NewCircuitBreaker creates a new circuit breaker with the given configuration
func NewCircuitBreaker(threshold int, cooldownPeriod time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		threshold:      threshold,
		cooldownPeriod: cooldownPeriod,
	}
}

// IsOpen checks if the circuit is open (preventing further calls)
// If the circuit is open and the cooldown period has passed, it automatically resets
func (cb *CircuitBreaker) IsOpen() bool {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()

	if !cb.isOpen {
		return false
	}

	// Check if cooldown period has passed
	if time.Since(cb.lastFailure) > cb.cooldownPeriod {
		// Need to upgrade to write lock to reset
		cb.mutex.RUnlock()
		cb.mutex.Lock()
		// Double-check after acquiring write lock
		if cb.isOpen && time.Since(cb.lastFailure) > cb.cooldownPeriod {
			cb.isOpen = false
			cb.failures = 0
			cb.lastError = nil
		}
		cb.mutex.Unlock()
		cb.mutex.RLock()
	}

	return cb.isOpen
}

// RecordSuccess records a successful call and resets the circuit breaker
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	cb.failures = 0
	cb.lastError = nil
	cb.isOpen = false
}

// RecordFailure records a failed call and opens the circuit if threshold is reached
func (cb *CircuitBreaker) RecordFailure(classifiedErr *emailerror.ClassifiedError) {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	cb.failures++
	cb.lastFailure = time.Now()
	cb.lastError = classifiedErr

	if cb.failures >= cb.threshold {
		cb.isOpen = true
	}
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

// IsOpen checks if the circuit for an integration is open
func (icb *IntegrationCircuitBreaker) IsOpen(integrationID string) bool {
	if cb, ok := icb.breakers.Load(integrationID); ok {
		return cb.(*CircuitBreaker).IsOpen()
	}
	return false
}

// RecordSuccess records a successful send for an integration
func (icb *IntegrationCircuitBreaker) RecordSuccess(integrationID string) {
	cb := icb.getOrCreateBreaker(integrationID)
	cb.RecordSuccess()
}

// RecordFailure records a failure for an integration
// Only counts provider errors toward the circuit breaker threshold
// Returns true if the error was counted (provider error), false if ignored (recipient error)
func (icb *IntegrationCircuitBreaker) RecordFailure(integrationID string, classifiedErr *emailerror.ClassifiedError) bool {
	// Only count provider errors toward circuit breaker
	if classifiedErr == nil || classifiedErr.IsRecipientError() {
		return false
	}

	cb := icb.getOrCreateBreaker(integrationID)
	cb.RecordFailure(classifiedErr)
	return true
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
}

// GetStats returns statistics for all circuit breakers
func (icb *IntegrationCircuitBreaker) GetStats() map[string]CircuitBreakerStats {
	stats := make(map[string]CircuitBreakerStats)
	icb.breakers.Range(func(key, value interface{}) bool {
		integrationID := key.(string)
		cb := value.(*CircuitBreaker)

		cb.mutex.RLock()
		stat := CircuitBreakerStats{
			IsOpen:    cb.isOpen,
			Failures:  cb.failures,
			Threshold: cb.threshold,
		}
		if !cb.lastFailure.IsZero() {
			stat.LastFailure = cb.lastFailure
			if cb.isOpen {
				cooldownLeft := cb.cooldownPeriod - time.Since(cb.lastFailure)
				if cooldownLeft > 0 {
					stat.CooldownLeft = cooldownLeft
				}
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
