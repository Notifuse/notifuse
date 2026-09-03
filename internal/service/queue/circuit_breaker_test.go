package queue

import (
	"os"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/pkg/emailerror"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCircuitBreaker_OpenAfterThreshold(t *testing.T) {
	cb := NewCircuitBreaker(3, 1*time.Minute)

	// Should start closed
	assert.False(t, cb.IsOpen())

	// Record failures
	providerErr := &emailerror.ClassifiedError{Type: emailerror.ErrorTypeProvider}

	cb.RecordFailure(providerErr)
	assert.False(t, cb.IsOpen())
	assert.Equal(t, 1, cb.GetFailures())

	cb.RecordFailure(providerErr)
	assert.False(t, cb.IsOpen())
	assert.Equal(t, 2, cb.GetFailures())

	// Third failure should open the circuit
	cb.RecordFailure(providerErr)
	assert.True(t, cb.IsOpen())
	assert.Equal(t, 3, cb.GetFailures())
}

func TestCircuitBreaker_ResetOnSuccess(t *testing.T) {
	cb := NewCircuitBreaker(3, 1*time.Minute)
	providerErr := &emailerror.ClassifiedError{Type: emailerror.ErrorTypeProvider}

	// Record some failures
	cb.RecordFailure(providerErr)
	cb.RecordFailure(providerErr)
	assert.Equal(t, 2, cb.GetFailures())

	// Success should reset
	cb.RecordSuccess()
	assert.Equal(t, 0, cb.GetFailures())
	assert.False(t, cb.IsOpen())
}

func TestCircuitBreaker_AutoResetAfterCooldown(t *testing.T) {
	// Use a very short cooldown for testing
	cb := NewCircuitBreaker(2, 10*time.Millisecond)
	providerErr := &emailerror.ClassifiedError{Type: emailerror.ErrorTypeProvider}

	// Open the circuit
	cb.RecordFailure(providerErr)
	cb.RecordFailure(providerErr)
	assert.True(t, cb.IsOpen())

	// Wait for cooldown
	time.Sleep(20 * time.Millisecond)

	// Should auto-reset
	assert.False(t, cb.IsOpen())
	assert.Equal(t, 0, cb.GetFailures())
}

func TestCircuitBreaker_GetLastError(t *testing.T) {
	cb := NewCircuitBreaker(3, 1*time.Minute)

	// Initially nil
	assert.Nil(t, cb.GetLastError())

	// After failure, should have last error
	providerErr := &emailerror.ClassifiedError{
		Type:     emailerror.ErrorTypeProvider,
		Provider: "ses",
	}
	cb.RecordFailure(providerErr)
	assert.Equal(t, providerErr, cb.GetLastError())

	// After success, should be cleared
	cb.RecordSuccess()
	assert.Nil(t, cb.GetLastError())
}

func TestIntegrationCircuitBreaker_PerIntegration(t *testing.T) {
	config := CircuitBreakerConfig{
		Threshold:      2,
		CooldownPeriod: 1 * time.Minute,
	}
	icb := NewIntegrationCircuitBreaker(config)

	providerErr := &emailerror.ClassifiedError{Type: emailerror.ErrorTypeProvider}

	// Open circuit for integration1
	icb.RecordFailure("integration1", providerErr)
	icb.RecordFailure("integration1", providerErr)
	assert.True(t, icb.IsOpen("integration1"))

	// integration2 should still be closed
	assert.False(t, icb.IsOpen("integration2"))

	// Success on integration1 should close it
	icb.RecordSuccess("integration1")
	assert.False(t, icb.IsOpen("integration1"))
}

func TestIntegrationCircuitBreaker_IgnoresRecipientErrors(t *testing.T) {
	config := CircuitBreakerConfig{
		Threshold:      2,
		CooldownPeriod: 1 * time.Minute,
	}
	icb := NewIntegrationCircuitBreaker(config)

	recipientErr := &emailerror.ClassifiedError{Type: emailerror.ErrorTypeRecipient}
	providerErr := &emailerror.ClassifiedError{Type: emailerror.ErrorTypeProvider}

	// Recipient errors should not count
	counted, _ := icb.RecordFailure("integration1", recipientErr)
	assert.False(t, counted)

	counted, _ = icb.RecordFailure("integration1", recipientErr)
	assert.False(t, counted)

	// Circuit should still be closed
	assert.False(t, icb.IsOpen("integration1"))

	// But provider errors should count
	counted, _ = icb.RecordFailure("integration1", providerErr)
	assert.True(t, counted)

	counted, _ = icb.RecordFailure("integration1", providerErr)
	assert.True(t, counted)

	// Now circuit should be open
	assert.True(t, icb.IsOpen("integration1"))
}

func TestIntegrationCircuitBreaker_NilError(t *testing.T) {
	config := CircuitBreakerConfig{
		Threshold:      2,
		CooldownPeriod: 1 * time.Minute,
	}
	icb := NewIntegrationCircuitBreaker(config)

	// Nil error should not count
	counted, _ := icb.RecordFailure("integration1", nil)
	assert.False(t, counted)

	// Circuit should still be closed
	assert.False(t, icb.IsOpen("integration1"))
}

func TestIntegrationCircuitBreaker_GetStats(t *testing.T) {
	config := CircuitBreakerConfig{
		Threshold:      3,
		CooldownPeriod: 1 * time.Minute,
	}
	icb := NewIntegrationCircuitBreaker(config)

	providerErr := &emailerror.ClassifiedError{Type: emailerror.ErrorTypeProvider}

	// Record failures for integration1
	icb.RecordFailure("integration1", providerErr)
	icb.RecordFailure("integration1", providerErr)

	// Open circuit for integration2
	icb.RecordFailure("integration2", providerErr)
	icb.RecordFailure("integration2", providerErr)
	icb.RecordFailure("integration2", providerErr)

	stats := icb.GetStats()

	// Check integration1 stats
	stat1, ok := stats["integration1"]
	assert.True(t, ok)
	assert.False(t, stat1.IsOpen)
	assert.Equal(t, 2, stat1.Failures)
	assert.Equal(t, 3, stat1.Threshold)

	// Check integration2 stats
	stat2, ok := stats["integration2"]
	assert.True(t, ok)
	assert.True(t, stat2.IsOpen)
	assert.Equal(t, 3, stat2.Failures)
	assert.Equal(t, 3, stat2.Threshold)
	assert.True(t, stat2.CooldownLeft > 0)
}

func TestIntegrationCircuitBreaker_GetLastError(t *testing.T) {
	config := CircuitBreakerConfig{
		Threshold:      3,
		CooldownPeriod: 1 * time.Minute,
	}
	icb := NewIntegrationCircuitBreaker(config)

	// Initially nil
	assert.Nil(t, icb.GetLastError("integration1"))

	// After failure
	providerErr := &emailerror.ClassifiedError{
		Type:     emailerror.ErrorTypeProvider,
		Provider: "ses",
	}
	icb.RecordFailure("integration1", providerErr)
	assert.Equal(t, providerErr, icb.GetLastError("integration1"))

	// Different integration should still be nil
	assert.Nil(t, icb.GetLastError("integration2"))
}

func TestIntegrationCircuitBreaker_GetConfig(t *testing.T) {
	config := CircuitBreakerConfig{
		Threshold:      10,
		CooldownPeriod: 5 * time.Minute,
	}
	icb := NewIntegrationCircuitBreaker(config)

	returnedConfig := icb.GetConfig()
	assert.Equal(t, 10, returnedConfig.Threshold)
	assert.Equal(t, 5*time.Minute, returnedConfig.CooldownPeriod)
}

func TestIntegrationCircuitBreaker_Clear(t *testing.T) {
	config := CircuitBreakerConfig{
		Threshold:      2,
		CooldownPeriod: 1 * time.Minute,
	}
	icb := NewIntegrationCircuitBreaker(config)

	providerErr := &emailerror.ClassifiedError{Type: emailerror.ErrorTypeProvider}

	// Open some circuits
	icb.RecordFailure("integration1", providerErr)
	icb.RecordFailure("integration1", providerErr)
	icb.RecordFailure("integration2", providerErr)
	icb.RecordFailure("integration2", providerErr)

	assert.True(t, icb.IsOpen("integration1"))
	assert.True(t, icb.IsOpen("integration2"))

	// Clear all
	icb.Clear()

	// Stats should be empty
	stats := icb.GetStats()
	assert.Empty(t, stats)

	// New checks should not be open
	assert.False(t, icb.IsOpen("integration1"))
	assert.False(t, icb.IsOpen("integration2"))
}

func TestIntegrationCircuitBreaker_Remove(t *testing.T) {
	config := CircuitBreakerConfig{
		Threshold:      2,
		CooldownPeriod: 1 * time.Minute,
	}
	icb := NewIntegrationCircuitBreaker(config)

	providerErr := &emailerror.ClassifiedError{Type: emailerror.ErrorTypeProvider}

	// Open circuit for integration1
	icb.RecordFailure("integration1", providerErr)
	icb.RecordFailure("integration1", providerErr)
	assert.True(t, icb.IsOpen("integration1"))

	// Remove integration1
	icb.Remove("integration1")

	// Should be closed again (no breaker)
	assert.False(t, icb.IsOpen("integration1"))
}

func TestIntegrationCircuitBreaker_DefaultConfig(t *testing.T) {
	// Ensure env var is not set for this test
	os.Unsetenv("CIRCUIT_BREAKER_COOLDOWN")

	// Test with zero config values - should use defaults
	icb := NewIntegrationCircuitBreaker(CircuitBreakerConfig{})

	config := icb.GetConfig()
	assert.Equal(t, 5, config.Threshold)
	assert.Equal(t, 1*time.Minute, config.CooldownPeriod)
}

func TestDefaultCircuitBreakerConfig(t *testing.T) {
	// Ensure env var is not set for this test
	os.Unsetenv("CIRCUIT_BREAKER_COOLDOWN")

	config := DefaultCircuitBreakerConfig()

	assert.Equal(t, 5, config.Threshold)
	assert.Equal(t, 1*time.Minute, config.CooldownPeriod)
}

func TestGetCircuitBreakerCooldown(t *testing.T) {
	t.Run("default value when not set", func(t *testing.T) {
		os.Unsetenv("CIRCUIT_BREAKER_COOLDOWN")
		assert.Equal(t, 1*time.Minute, getCircuitBreakerCooldown())
	})

	t.Run("custom value from environment", func(t *testing.T) {
		os.Setenv("CIRCUIT_BREAKER_COOLDOWN", "2s")
		defer os.Unsetenv("CIRCUIT_BREAKER_COOLDOWN")
		assert.Equal(t, 2*time.Second, getCircuitBreakerCooldown())
	})

	t.Run("custom value with different duration", func(t *testing.T) {
		os.Setenv("CIRCUIT_BREAKER_COOLDOWN", "30s")
		defer os.Unsetenv("CIRCUIT_BREAKER_COOLDOWN")
		assert.Equal(t, 30*time.Second, getCircuitBreakerCooldown())
	})

	t.Run("invalid value uses default", func(t *testing.T) {
		os.Setenv("CIRCUIT_BREAKER_COOLDOWN", "invalid")
		defer os.Unsetenv("CIRCUIT_BREAKER_COOLDOWN")
		assert.Equal(t, 1*time.Minute, getCircuitBreakerCooldown())
	})

	t.Run("empty value uses default", func(t *testing.T) {
		os.Setenv("CIRCUIT_BREAKER_COOLDOWN", "")
		defer os.Unsetenv("CIRCUIT_BREAKER_COOLDOWN")
		assert.Equal(t, 1*time.Minute, getCircuitBreakerCooldown())
	})
}

// TestCircuitBreaker_ReportsTheOpenTransition covers the signal the broadcast pause
// hangs on. The breaker is checked per entry and a large queue can skip thousands of
// them while it is open, so "the circuit is open" is not usable as a trigger — only
// the moment it flips is. And it flips more than once per outage: the breaker does not
// latch, so it resets after its cooldown and opens again, which is why whatever acts
// on this has to be idempotent.
func TestCircuitBreaker_ReportsTheOpenTransition(t *testing.T) {
	providerErr := &emailerror.ClassifiedError{Type: emailerror.ErrorTypeProvider}

	t.Run("only the failure that reaches the threshold reports it", func(t *testing.T) {
		cb := NewCircuitBreaker(5, time.Minute)

		for i := 1; i < 5; i++ {
			assert.False(t, cb.RecordFailure(providerErr), "failure %d is below the threshold", i)
		}
		assert.True(t, cb.RecordFailure(providerErr), "the fifth failure opens the circuit")
	})

	t.Run("does not report again while already open", func(t *testing.T) {
		cb := NewCircuitBreaker(2, time.Minute)

		cb.RecordFailure(providerErr)
		assert.True(t, cb.RecordFailure(providerErr))
		assert.False(t, cb.RecordFailure(providerErr), "already open is not a transition")
	})

	t.Run("reports again after a cooldown reset", func(t *testing.T) {
		// The breaker restores a full failure budget once its cooldown elapses, so a
		// provider that stays down opens it once per cooldown period, not once.
		cb := NewCircuitBreaker(2, 20*time.Millisecond)

		cb.RecordFailure(providerErr)
		require.True(t, cb.RecordFailure(providerErr))

		time.Sleep(30 * time.Millisecond)
		require.False(t, cb.IsOpen(), "the cooldown resets it rather than latching")

		cb.RecordFailure(providerErr)
		assert.True(t, cb.RecordFailure(providerErr), "it opens again on the next run of failures")
	})

	t.Run("a recipient error neither counts nor reports", func(t *testing.T) {
		icb := NewIntegrationCircuitBreaker(CircuitBreakerConfig{Threshold: 1, CooldownPeriod: time.Minute})
		recipientErr := &emailerror.ClassifiedError{Type: emailerror.ErrorTypeRecipient}

		counted, opened := icb.RecordFailure("integration1", recipientErr)
		assert.False(t, counted)
		assert.False(t, opened)
		assert.False(t, icb.IsOpen("integration1"))
	})

	t.Run("the per-integration wrapper passes the transition through", func(t *testing.T) {
		icb := NewIntegrationCircuitBreaker(CircuitBreakerConfig{Threshold: 2, CooldownPeriod: time.Minute})

		counted, opened := icb.RecordFailure("integration1", providerErr)
		assert.True(t, counted)
		assert.False(t, opened)

		counted, opened = icb.RecordFailure("integration1", providerErr)
		assert.True(t, counted)
		assert.True(t, opened)
	})
}
