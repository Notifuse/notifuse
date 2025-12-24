package queue

import (
	"os"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/pkg/emailerror"
	"github.com/stretchr/testify/assert"
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
	counted := icb.RecordFailure("integration1", recipientErr)
	assert.False(t, counted)

	counted = icb.RecordFailure("integration1", recipientErr)
	assert.False(t, counted)

	// Circuit should still be closed
	assert.False(t, icb.IsOpen("integration1"))

	// But provider errors should count
	counted = icb.RecordFailure("integration1", providerErr)
	assert.True(t, counted)

	counted = icb.RecordFailure("integration1", providerErr)
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
	counted := icb.RecordFailure("integration1", nil)
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
