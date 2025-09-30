package broadcast

import "time"

// Config contains configuration for broadcast processing
type Config struct {
	// Concurrency settings
	MaxParallelism int           `json:"max_parallelism"`
	MaxProcessTime time.Duration `json:"max_process_time"`

	// Batch processing
	FetchBatchSize   int `json:"fetch_batch_size"`
	ProcessBatchSize int `json:"process_batch_size"`

	// Logging and metrics
	ProgressLogInterval time.Duration `json:"progress_log_interval"`

	// Circuit breaker settings
	EnableCircuitBreaker    bool          `json:"enable_circuit_breaker"`
	CircuitBreakerThreshold int           `json:"circuit_breaker_threshold"`
	CircuitBreakerCooldown  time.Duration `json:"circuit_breaker_cooldown"`

	// Rate limiting
	DefaultRateLimit int `json:"default_rate_limit"` // Emails per minute (fallback when broadcast doesn't specify rate limit)

	// Retry settings
	MaxRetries    int           `json:"max_retries"`
	RetryInterval time.Duration `json:"retry_interval"`
}

// DefaultConfig returns a configuration with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		MaxParallelism:          10,
		MaxProcessTime:          50 * time.Second,
		FetchBatchSize:          50,
		ProcessBatchSize:        25,
		ProgressLogInterval:     5 * time.Second,
		EnableCircuitBreaker:    true,
		CircuitBreakerThreshold: 5,
		CircuitBreakerCooldown:  1 * time.Minute,
		DefaultRateLimit:        600, // 10 per second
		MaxRetries:              3,
		RetryInterval:           30 * time.Second,
	}
}
