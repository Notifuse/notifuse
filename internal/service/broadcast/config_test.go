package broadcast_test

import (
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/service/broadcast"
	"github.com/stretchr/testify/assert"
)

// TestDefaultConfig ensures the default config has expected values
func TestDefaultConfig(t *testing.T) {
	config := broadcast.DefaultConfig()

	// Verify default config values
	assert.NotNil(t, config)
	assert.Equal(t, 10, config.MaxParallelism)
	assert.Equal(t, 50*time.Second, config.MaxProcessTime)
	assert.Equal(t, 50, config.FetchBatchSize)
	assert.Equal(t, 25, config.ProcessBatchSize)
	assert.Equal(t, 5*time.Second, config.ProgressLogInterval)
	assert.Equal(t, true, config.EnableCircuitBreaker)
	assert.Equal(t, 5, config.CircuitBreakerThreshold)
	assert.Equal(t, 1*time.Minute, config.CircuitBreakerCooldown)
	assert.Equal(t, 600, config.DefaultRateLimit)
	assert.Equal(t, 3, config.MaxRetries)
	assert.Equal(t, 30*time.Second, config.RetryInterval)
}
