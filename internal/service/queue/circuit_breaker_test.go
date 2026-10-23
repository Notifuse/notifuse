package queue

import (
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/pkg/emailerror"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	providerErr  = &emailerror.ClassifiedError{Type: emailerror.ErrorTypeProvider}
	recipientErr = &emailerror.ClassifiedError{Type: emailerror.ErrorTypeRecipient}
)

// openBreaker returns a breaker opened by threshold failures, and the ticket a send
// took while it was still closed.
func openBreaker(t *testing.T, threshold int, cooldown time.Duration) (*CircuitBreaker, Ticket) {
	t.Helper()
	cb := NewCircuitBreaker(threshold, cooldown)
	ticket, ok := cb.Allow()
	require.True(t, ok)
	for i := 0; i < threshold; i++ {
		cb.RecordFailure(ticket, providerErr)
	}
	require.True(t, cb.IsOpen())
	return cb, ticket
}

func TestCircuitBreaker_OpenAfterThreshold(t *testing.T) {
	cb := NewCircuitBreaker(3, 1*time.Minute)
	assert.False(t, cb.IsOpen())

	ticket, ok := cb.Allow()
	require.True(t, ok)
	assert.False(t, ticket.Probe)

	counted, opened := cb.RecordFailure(ticket, providerErr)
	assert.True(t, counted)
	assert.False(t, opened)
	assert.Equal(t, 1, cb.GetFailures())

	cb.RecordFailure(ticket, providerErr)
	assert.False(t, cb.IsOpen())

	counted, opened = cb.RecordFailure(ticket, providerErr)
	assert.True(t, counted)
	assert.True(t, opened, "the failure that reaches the threshold opens the circuit")
	assert.True(t, cb.IsOpen())
}

func TestCircuitBreaker_ResetOnSuccess(t *testing.T) {
	cb := NewCircuitBreaker(3, 1*time.Minute)
	ticket, _ := cb.Allow()

	cb.RecordFailure(ticket, providerErr)
	cb.RecordFailure(ticket, providerErr)
	assert.Equal(t, 2, cb.GetFailures())

	cb.RecordSuccess(ticket)
	assert.Equal(t, 0, cb.GetFailures())
	assert.Nil(t, cb.GetLastError())
	assert.False(t, cb.IsOpen())
}

func TestCircuitBreaker_GetLastError(t *testing.T) {
	cb := NewCircuitBreaker(3, 1*time.Minute)
	assert.Nil(t, cb.GetLastError())

	sesErr := &emailerror.ClassifiedError{Type: emailerror.ErrorTypeProvider, Provider: "ses"}
	ticket, _ := cb.Allow()
	cb.RecordFailure(ticket, sesErr)
	assert.Equal(t, sesErr, cb.GetLastError())

	cb.RecordSuccess(ticket)
	assert.Nil(t, cb.GetLastError())
}

func TestCircuitBreaker_Allow(t *testing.T) {
	t.Run("a closed circuit allows every send, with the current epoch", func(t *testing.T) {
		cb := NewCircuitBreaker(2, time.Minute)
		first, ok := cb.Allow()
		require.True(t, ok)
		second, ok := cb.Allow()
		require.True(t, ok)
		assert.Equal(t, first, second)
		assert.False(t, first.Probe)
	})

	t.Run("an open circuit refuses within its cooldown", func(t *testing.T) {
		cb, _ := openBreaker(t, 2, time.Minute)
		_, ok := cb.Allow()
		assert.False(t, ok)
	})

	t.Run("after the cooldown, one concurrent caller gets the probe and the others are refused", func(t *testing.T) {
		cb, _ := openBreaker(t, 2, 20*time.Millisecond)
		time.Sleep(30 * time.Millisecond)

		var probes atomic.Int32
		var wg sync.WaitGroup
		start := make(chan struct{})
		for i := 0; i < 20; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				if ticket, ok := cb.Allow(); ok {
					assert.True(t, ticket.Probe)
					probes.Add(1)
				}
			}()
		}
		close(start)
		wg.Wait()
		assert.Equal(t, int32(1), probes.Load())

		_, ok := cb.Allow()
		assert.False(t, ok, "no second probe while the first is out")
	})

	t.Run("IsOpen changes nothing, even past the cooldown", func(t *testing.T) {
		cb, _ := openBreaker(t, 2, 10*time.Millisecond)
		time.Sleep(20 * time.Millisecond)

		assert.True(t, cb.IsOpen())
		assert.True(t, cb.IsOpen())
		ticket, ok := cb.Allow()
		require.True(t, ok)
		assert.True(t, ticket.Probe, "reading the state did not close the circuit")
	})
}

func TestCircuitBreaker_Probe(t *testing.T) {
	probe := func(t *testing.T, cb *CircuitBreaker) Ticket {
		t.Helper()
		ticket, ok := cb.Allow()
		require.True(t, ok)
		require.True(t, ticket.Probe)
		return ticket
	}

	t.Run("a cancelled probe can be granted again at once", func(t *testing.T) {
		cb, _ := openBreaker(t, 2, 10*time.Millisecond)
		time.Sleep(20 * time.Millisecond)

		cb.CancelProbe(probe(t, cb))
		probe(t, cb)
	})

	t.Run("a cancel from an earlier epoch frees nothing", func(t *testing.T) {
		cb, closedTicket := openBreaker(t, 2, 10*time.Millisecond)
		time.Sleep(20 * time.Millisecond)
		probe(t, cb)

		cb.CancelProbe(Ticket{Epoch: closedTicket.Epoch, Probe: true})
		_, ok := cb.Allow()
		assert.False(t, ok)
	})

	t.Run("the probe's success closes the circuit", func(t *testing.T) {
		cb, _ := openBreaker(t, 2, 10*time.Millisecond)
		time.Sleep(20 * time.Millisecond)

		cb.RecordSuccess(probe(t, cb))
		assert.False(t, cb.IsOpen())
		assert.Equal(t, 0, cb.GetFailures())
		assert.Nil(t, cb.GetLastError())
	})

	t.Run("the probe's recipient error closes the circuit: the provider answered", func(t *testing.T) {
		cb, _ := openBreaker(t, 2, 10*time.Millisecond)
		time.Sleep(20 * time.Millisecond)

		counted, opened := cb.RecordFailure(probe(t, cb), recipientErr)
		assert.False(t, counted)
		assert.False(t, opened)
		assert.False(t, cb.IsOpen())
	})

	t.Run("the probe's provider failure re-opens the circuit, reports it and restarts the cooldown", func(t *testing.T) {
		cb, _ := openBreaker(t, 2, 30*time.Millisecond)
		time.Sleep(40 * time.Millisecond)

		counted, opened := cb.RecordFailure(probe(t, cb), providerErr)
		assert.True(t, counted)
		assert.True(t, opened, "the pause hangs on this, and has to fire again for each failed probe")
		assert.True(t, cb.IsOpen())

		_, ok := cb.Allow()
		assert.False(t, ok, "a new cooldown starts at the re-opening")
		time.Sleep(40 * time.Millisecond)
		probe(t, cb)
	})

	t.Run("after a probe closes the circuit, the next opening grants a probe after its own cooldown", func(t *testing.T) {
		cb, _ := openBreaker(t, 1, 10*time.Millisecond)
		time.Sleep(20 * time.Millisecond)
		cb.RecordSuccess(probe(t, cb))

		ticket, ok := cb.Allow()
		require.True(t, ok)
		_, opened := cb.RecordFailure(ticket, providerErr)
		require.True(t, opened)

		time.Sleep(20 * time.Millisecond)
		probe(t, cb)
	})
}

func TestCircuitBreaker_LateResults(t *testing.T) {
	t.Run("a success allowed before the opening does not close it", func(t *testing.T) {
		// The send was allowed while the circuit was closed but got to the provider
		// after five others had failed: one success then says nothing about the
		// provider now, and closing on it would end the cooldown at once.
		cb, closedTicket := openBreaker(t, 2, time.Minute)

		cb.RecordSuccess(closedTicket)
		assert.True(t, cb.IsOpen())
	})

	t.Run("a failure allowed before the opening neither counts nor extends the cooldown", func(t *testing.T) {
		cb, closedTicket := openBreaker(t, 2, 40*time.Millisecond)
		time.Sleep(20 * time.Millisecond)

		counted, opened := cb.RecordFailure(closedTicket, providerErr)
		assert.False(t, counted)
		assert.False(t, opened)

		time.Sleep(30 * time.Millisecond)
		ticket, ok := cb.Allow()
		require.True(t, ok, "the cooldown runs from the opening, not from the late failure")
		assert.True(t, ticket.Probe)
	})

	t.Run("a failure allowed before an opening does not count once a probe has closed it", func(t *testing.T) {
		cb, closedTicket := openBreaker(t, 2, 10*time.Millisecond)
		time.Sleep(20 * time.Millisecond)
		probeTicket, ok := cb.Allow()
		require.True(t, ok)
		cb.RecordSuccess(probeTicket)

		counted, _ := cb.RecordFailure(closedTicket, providerErr)
		assert.False(t, counted)
		assert.Equal(t, 0, cb.GetFailures())
	})

	t.Run("concurrent failures report the opening once", func(t *testing.T) {
		cb := NewCircuitBreaker(5, time.Minute)

		var opened atomic.Int32
		var wg sync.WaitGroup
		start := make(chan struct{})
		for i := 0; i < 20; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				ticket, ok := cb.Allow()
				<-start
				if !ok {
					return
				}
				if _, o := cb.RecordFailure(ticket, providerErr); o {
					opened.Add(1)
				}
			}()
		}
		close(start)
		wg.Wait()
		assert.Equal(t, int32(1), opened.Load())
	})
}

func TestIntegrationCircuitBreaker_PerIntegration(t *testing.T) {
	icb := NewIntegrationCircuitBreaker(CircuitBreakerConfig{Threshold: 2, CooldownPeriod: 10 * time.Millisecond})

	ticket, ok := icb.Allow("integration1")
	require.True(t, ok)
	icb.RecordFailure("integration1", ticket, providerErr)
	icb.RecordFailure("integration1", ticket, providerErr)
	assert.True(t, icb.IsOpen("integration1"))

	_, ok = icb.Allow("integration2")
	assert.True(t, ok, "another integration is unaffected")
	assert.False(t, icb.IsOpen("integration2"))

	time.Sleep(20 * time.Millisecond)
	probe, ok := icb.Allow("integration1")
	require.True(t, ok)
	require.True(t, probe.Probe)
	icb.RecordSuccess("integration1", probe)
	assert.False(t, icb.IsOpen("integration1"))
}

func TestIntegrationCircuitBreaker_IgnoresRecipientErrors(t *testing.T) {
	icb := NewIntegrationCircuitBreaker(CircuitBreakerConfig{Threshold: 2, CooldownPeriod: time.Minute})
	ticket, _ := icb.Allow("integration1")

	counted, opened := icb.RecordFailure("integration1", ticket, recipientErr)
	assert.False(t, counted)
	assert.False(t, opened)
	counted, _ = icb.RecordFailure("integration1", ticket, recipientErr)
	assert.False(t, counted)
	assert.False(t, icb.IsOpen("integration1"))

	counted, _ = icb.RecordFailure("integration1", ticket, providerErr)
	assert.True(t, counted)
	counted, opened = icb.RecordFailure("integration1", ticket, providerErr)
	assert.True(t, counted)
	assert.True(t, opened)
	assert.True(t, icb.IsOpen("integration1"))
}

func TestIntegrationCircuitBreaker_ProbeRecipientErrorReachesTheBreaker(t *testing.T) {
	// The manager used to drop recipient errors before the breaker. A probe needs
	// one to get through: it is the provider answering.
	icb := NewIntegrationCircuitBreaker(CircuitBreakerConfig{Threshold: 1, CooldownPeriod: 10 * time.Millisecond})
	ticket, _ := icb.Allow("integration1")
	icb.RecordFailure("integration1", ticket, providerErr)
	time.Sleep(20 * time.Millisecond)

	probe, ok := icb.Allow("integration1")
	require.True(t, ok)
	icb.RecordFailure("integration1", probe, recipientErr)
	assert.False(t, icb.IsOpen("integration1"))
}

func TestIntegrationCircuitBreaker_NilError(t *testing.T) {
	icb := NewIntegrationCircuitBreaker(CircuitBreakerConfig{Threshold: 2, CooldownPeriod: time.Minute})
	ticket, _ := icb.Allow("integration1")

	counted, _ := icb.RecordFailure("integration1", ticket, nil)
	assert.False(t, counted)
	assert.False(t, icb.IsOpen("integration1"))

	t.Run("a probe recorded with no error is released, not left out", func(t *testing.T) {
		icb := NewIntegrationCircuitBreaker(CircuitBreakerConfig{Threshold: 1, CooldownPeriod: 10 * time.Millisecond})
		ticket, _ := icb.Allow("integration1")
		icb.RecordFailure("integration1", ticket, providerErr)
		time.Sleep(20 * time.Millisecond)

		probe, _ := icb.Allow("integration1")
		require.True(t, probe.Probe)
		icb.RecordFailure("integration1", probe, nil)
		again, ok := icb.Allow("integration1")
		require.True(t, ok)
		assert.True(t, again.Probe)
	})
}

func TestIntegrationCircuitBreaker_CancelProbe(t *testing.T) {
	icb := NewIntegrationCircuitBreaker(CircuitBreakerConfig{Threshold: 1, CooldownPeriod: 10 * time.Millisecond})

	icb.CancelProbe("unknown", Ticket{Probe: true}) // no breaker: nothing to do

	ticket, _ := icb.Allow("integration1")
	icb.RecordFailure("integration1", ticket, providerErr)
	time.Sleep(20 * time.Millisecond)

	probe, ok := icb.Allow("integration1")
	require.True(t, ok)
	icb.CancelProbe("integration1", probe)
	again, ok := icb.Allow("integration1")
	require.True(t, ok)
	assert.True(t, again.Probe)
}

func TestIntegrationCircuitBreaker_GetStats(t *testing.T) {
	icb := NewIntegrationCircuitBreaker(CircuitBreakerConfig{Threshold: 3, CooldownPeriod: 100 * time.Millisecond})

	t1, _ := icb.Allow("integration1")
	icb.RecordFailure("integration1", t1, providerErr)
	icb.RecordFailure("integration1", t1, providerErr)

	t2, _ := icb.Allow("integration2")
	icb.RecordFailure("integration2", t2, providerErr)
	icb.RecordFailure("integration2", t2, providerErr)
	icb.RecordFailure("integration2", t2, providerErr)

	time.Sleep(40 * time.Millisecond)
	// A late failure: it must not move the cooldown the stats report.
	icb.RecordFailure("integration2", t2, providerErr)

	stats := icb.GetStats()

	stat1, ok := stats["integration1"]
	require.True(t, ok)
	assert.False(t, stat1.IsOpen)
	assert.Equal(t, 2, stat1.Failures)
	assert.Equal(t, 3, stat1.Threshold)

	stat2, ok := stats["integration2"]
	require.True(t, ok)
	assert.True(t, stat2.IsOpen)
	assert.Equal(t, 3, stat2.Threshold)
	assert.Greater(t, stat2.CooldownLeft, time.Duration(0))
	assert.LessOrEqual(t, stat2.CooldownLeft, 60*time.Millisecond, "counted from the opening")
	assert.False(t, stat2.ProbeInFlight)

	time.Sleep(70 * time.Millisecond)
	_, ok = icb.Allow("integration2")
	require.True(t, ok)
	assert.True(t, icb.GetStats()["integration2"].ProbeInFlight)
}

func TestIntegrationCircuitBreaker_GetLastError(t *testing.T) {
	icb := NewIntegrationCircuitBreaker(CircuitBreakerConfig{Threshold: 3, CooldownPeriod: time.Minute})
	assert.Nil(t, icb.GetLastError("integration1"))

	sesErr := &emailerror.ClassifiedError{Type: emailerror.ErrorTypeProvider, Provider: "ses"}
	ticket, _ := icb.Allow("integration1")
	icb.RecordFailure("integration1", ticket, sesErr)
	assert.Equal(t, sesErr, icb.GetLastError("integration1"))
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
	icb := NewIntegrationCircuitBreaker(CircuitBreakerConfig{Threshold: 2, CooldownPeriod: time.Minute})

	for _, id := range []string{"integration1", "integration2"} {
		ticket, _ := icb.Allow(id)
		icb.RecordFailure(id, ticket, providerErr)
		icb.RecordFailure(id, ticket, providerErr)
		require.True(t, icb.IsOpen(id))
	}

	icb.Clear()

	assert.Empty(t, icb.GetStats())
	assert.False(t, icb.IsOpen("integration1"))
	assert.False(t, icb.IsOpen("integration2"))
}

func TestIntegrationCircuitBreaker_Remove(t *testing.T) {
	icb := NewIntegrationCircuitBreaker(CircuitBreakerConfig{Threshold: 2, CooldownPeriod: time.Minute})

	ticket, _ := icb.Allow("integration1")
	icb.RecordFailure("integration1", ticket, providerErr)
	icb.RecordFailure("integration1", ticket, providerErr)
	require.True(t, icb.IsOpen("integration1"))

	icb.Remove("integration1")
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
