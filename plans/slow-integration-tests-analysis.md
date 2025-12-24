# Slow Integration Tests Analysis

**Date:** 2025-12-23
**Tests Analyzed:** broadcast_esp_failures_test, broadcast_pagination_test, broadcast_race_condition_test

## Executive Summary

Three integration tests became significantly slower after the email queue architecture change:

| Test | Old Time | New Time | Increase |
|------|----------|----------|----------|
| broadcast_esp_failures_test | 11.84s | 143.62s | +131.78s (12x) |
| broadcast_pagination_test | 8.84s | 53.68s | +44.84s (6x) |
| broadcast_race_condition_test | 11.33s | 41.57s | +30.24s (4x) |

**Root Cause:** Tests now verify end-to-end email delivery through background workers instead of just verifying emails are enqueued.

## Detailed Analysis

### 1. broadcast_esp_failures_test (143.62s)

#### What It Tests
- ESP (Email Service Provider) failure handling when SMTP is unreachable
- Circuit breaker activation after consecutive failures
- Proper error tracking in message history

#### Root Cause: TCP Connection Timeout

The SMTP service uses a **30-second TCP dial timeout**:

```go
// internal/service/smtp_service.go:107
dialer := &net.Dialer{Timeout: 30 * time.Second}
```

When connecting to port 9999 (nothing listening), each connection attempt waits **30 seconds** before timing out.

#### Timeline Breakdown

| Phase | Duration | Explanation |
|-------|----------|-------------|
| Setup (user, workspace, contacts) | ~3s | Database operations |
| 5 SMTP connection timeouts | **~150s** | 5 × 30s TCP timeout |
| Circuit breaker triggers | 0s | Opens after 5 failures |
| Queue drain + verification | ~3s | Remaining emails skipped |

The circuit breaker threshold is 5 (`CircuitBreakerThreshold: 5` in worker.go:22), so after 5 failed connection attempts (potentially 150 seconds), it stops trying.

### 2. broadcast_pagination_test (53.68s)

#### What It Tests
- Issue #157: All contacts receive emails even with identical `created_at` timestamps
- Pagination works correctly across multiple batches (50 per batch)
- Tests with 100 and 1000 contacts

#### Root Cause: Polling Overhead

Multiple polling loops with conservative intervals:

```go
// helpers.go:456-457
pollInterval := 1 * time.Second
taskExecutionInterval := 2 * time.Second

// helpers.go:1193
pollInterval := 2 * time.Second  // WaitForMailpitMessages

// worker.go:30
PollInterval: 1 * time.Second    // Email queue worker
```

#### Timeline Breakdown (1000 contacts)

| Phase | Duration | Explanation |
|-------|----------|-------------|
| Setup + bulk import | ~5s | Database operations |
| Broadcast task execution | ~10s | Polling every 2s |
| Queue processing | ~20s | 1000 ÷ 50 batches × 1s poll |
| Mailpit verification | ~15s | Polling every 2s |

### 3. broadcast_race_condition_test (41.57s)

#### What It Tests
- Concurrent task execution doesn't cause duplicate emails
- `MarkAsRunningTx` properly prevents race conditions
- 5 concurrent executors trying to process the same task

#### Root Cause: Hardcoded Sleep

```go
// broadcast_race_condition_test.go:253
time.Sleep(5 * time.Second)  // Unnecessary hardcoded wait
```

Plus queue drain polling:
```go
// helpers.go:1274
pollInterval := 500 * time.Millisecond
```

#### Timeline Breakdown

| Phase | Duration | Explanation |
|-------|----------|-------------|
| Setup + contacts | ~3s | Database operations |
| Concurrent execution | ~3s | 5 goroutines |
| Broadcast completion | ~15s | 1s polling |
| **Hardcoded sleep** | **5s** | Unnecessary! |
| Queue drain | ~10s | 500ms polling |
| Verification | ~3s | Mailpit check |

## Configuration Values Explained

| Configuration | Value | Reason |
|---------------|-------|--------|
| TCP dial timeout | 30s | Industry standard for SMTP connections |
| Worker poll interval | 1s | Balance between latency and CPU |
| Circuit breaker threshold | 5 | Enough to detect real vs transient issues |
| Circuit breaker cooldown | 1 min | Time for external service recovery |
| Mailpit poll interval | 2s | Reduce HTTP overhead |
| Queue empty poll | 500ms | Local DB, can be faster |

## Why Tests Were Faster Before

The previous architecture:
1. **Didn't start background workers** - emails just enqueued
2. **Didn't wait for delivery** - only verified enqueue count
3. **No real SMTP connections** - no TCP timeouts

## Optimizations Implemented

### 1. Configurable SMTP Dial Timeout

Added environment variable support for SMTP dial timeout:

```go
// internal/service/smtp_service.go
func getSMTPDialTimeout() time.Duration {
    if timeout := os.Getenv("SMTP_DIAL_TIMEOUT"); timeout != "" {
        if d, err := time.ParseDuration(timeout); err == nil {
            return d
        }
    }
    return 30 * time.Second  // Default for production
}
```

Test environment sets `SMTP_DIAL_TIMEOUT=2s` for faster failure detection.

### 2. Remove Hardcoded Sleep

Replaced the hardcoded `time.Sleep(5 * time.Second)` with proper queue drain waiting:

```go
// Before:
time.Sleep(5 * time.Second)

// After:
// Start worker and wait for queue to drain properly
err = suite.ServerManager.StartBackgroundWorkers(workerCtx)
require.NoError(t, err)
err = testutil.WaitForQueueEmpty(t, queueRepo, workspace.ID, 2*time.Minute)
```

### 3. Test Environment Configuration

Updated test setup to use faster timeouts:

```go
// tests/testutil/helpers.go - SetupTestEnvironment()
os.Setenv("SMTP_DIAL_TIMEOUT", "2s")
```

## Correction: TCP Timeout Was Not the Issue

After testing, the TCP timeout optimization had **no effect** because:

1. **Connection refused is immediate**: When connecting to port 9999 with nothing listening, the OS returns "connection refused" immediately (~0ms), not a timeout.

2. **The real bottleneck is retry/cooldown mechanisms**:
   - Circuit breaker cooldown: **1 minute** after 5 consecutive failures
   - Retry exponential backoff: **1min, 2min, 4min** between retry attempts
   - 20 emails × 3 retries = potentially waiting for multiple cooldown periods

## Actual Test Timeline (broadcast_esp_failures_test)

Looking at the logs:
```
17:50:17 - First 5 emails fail (immediate connection refused)
           Circuit breaker opens
17:50:17 to 17:51:17 - Waiting for 1-minute cooldown
17:51:18 - Second attempt for same 5 emails
           ... cycle continues
```

The ~143 seconds comes from:
- Immediate failures (< 1s)
- 2 × 1-minute circuit breaker cooldowns (~120s)
- Queue polling and processing overhead (~20s)

## Why These Tests Are Inherently Slow

The retry and circuit breaker mechanisms are designed for production use:

| Mechanism | Production Value | Purpose |
|-----------|-----------------|---------|
| Circuit breaker cooldown | 1 minute | Allow external service to recover |
| Retry backoff | 1, 2, 4 minutes | Avoid hammering failing service |
| Max retries | 3 | Eventual failure after reasonable attempts |

These are **not configurable** in the current implementation and are hardcoded in:
- `internal/service/queue/worker.go:34` - `CircuitBreakerCooldown: 1 * time.Minute`
- `internal/domain/email_queue.go:158` - `CalculateNextRetryTime` with 1-minute base

## Changes Made (Limited Effect)

1. **`internal/service/smtp_service.go`** - Added configurable SMTP dial timeout (2s for tests)
   - Effect: None for "connection refused" scenarios
   - Would help if server was slow to respond (not the case here)

2. **`tests/testutil/helpers.go`** - Set `SMTP_DIAL_TIMEOUT=2s`
   - Effect: None for immediate connection failures

3. **`tests/integration/broadcast_race_condition_test.go`** - Replaced `time.Sleep(5s)` with proper queue draining
   - Effect: Minor improvement, test now waits for actual queue completion

## Actual Test Results After Initial Changes

| Test | Before | After | Change |
|------|--------|-------|--------|
| broadcast_esp_failures_test | 143.62s | 143.11s | -0.51s (no effect) |
| broadcast_pagination_test | 53.68s | 55.44s | +1.76s (variance) |
| broadcast_race_condition_test | 41.57s | 40.92s | -0.65s (slight improvement) |

## Optimizations Implemented (Second Round)

Added configurable environment variables for circuit breaker and retry backoff:

### 1. Circuit Breaker Cooldown

**File: `internal/service/queue/circuit_breaker.go`**
```go
func getCircuitBreakerCooldown() time.Duration {
    if cooldown := os.Getenv("CIRCUIT_BREAKER_COOLDOWN"); cooldown != "" {
        if d, err := time.ParseDuration(cooldown); err == nil {
            return d
        }
    }
    return 1 * time.Minute
}
```

Updated `DefaultCircuitBreakerConfig()`, `NewIntegrationCircuitBreaker()`, and `worker.go` to use this.

### 2. Retry Backoff Base

**File: `internal/domain/email_queue.go`**
```go
func getEmailQueueRetryBase() time.Duration {
    if base := os.Getenv("EMAIL_QUEUE_RETRY_BASE"); base != "" {
        if d, err := time.ParseDuration(base); err == nil {
            return d
        }
    }
    return 1 * time.Minute
}
```

### 3. Test Environment Configuration

**File: `tests/testutil/helpers.go`**
```go
// In SetupTestEnvironment():
os.Setenv("CIRCUIT_BREAKER_COOLDOWN", "2s")
os.Setenv("EMAIL_QUEUE_RETRY_BASE", "2s")
```

## Final Test Results

| Test | Before | After Optimization | Improvement |
|------|--------|-------------------|-------------|
| broadcast_esp_failures_test | 143.62s | **4.77s** | **~30x faster** |
| broadcast_pagination_test | 53.68s | **38.60s** | ~28% faster |
| broadcast_race_condition_test | 41.57s | **9.28s** | **~4.5x faster** |

### Summary

The configurable circuit breaker cooldown and retry backoff base dramatically improved test performance:
- **ESP failures test**: 143s → 5s (the circuit breaker cooldown was the main bottleneck)
- **Race condition test**: 42s → 9s (proper queue draining + faster cooldown)
- **Pagination test**: 54s → 39s (minor improvement, test is I/O bound)

---
*Analysis completed: 2025-12-23*
*Final update after implementing configurable circuit breaker and retry backoff*
