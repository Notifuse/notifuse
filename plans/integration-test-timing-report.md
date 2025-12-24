# Integration Test Timing Report

Generated: 2025-12-23 (Re-measured)

## Summary

| Metric | Value |
|--------|-------|
| Total Test Suites | 44 |
| Passed | 43 |
| Failed | 1 |
| Pass Rate | 97.7% |
| Total Duration | ~10 minutes |

## Test Suites by Duration (Slowest First)

| Rank | Test Suite | Duration | Status | Diff |
|------|------------|----------|--------|------|
| 1 | broadcast_esp_failures_test | 143.62s | PASS | +131.78s |
| 2 | broadcast_pagination_test | 53.68s | PASS | +44.84s |
| 3 | broadcast_race_condition_test | 41.57s | PASS | +30.24s |
| 4 | connection_pool_performance_test | 32.89s | PASS | -25.06s |
| 5 | segment_e2e_test | 22.18s | FAIL | +8.75s |
| 6 | connection_pool_concurrency_test | 21.42s | PASS | -15.68s |
| 7 | broadcast_handler_test | 14.43s | PASS | +10.12s |
| 8 | broadcast_task_timeout_test | 13.31s | PASS | +8.29s |
| 9 | workspace_test | 12.93s | PASS | -23.99s |
| 10 | smtp_relay_e2e_test | 12.72s | PASS | -5.02s |
| 11 | email_queue_e2e_test | 12.68s | PASS | -1.39s |
| 12 | contact_api_test | 12.64s | PASS | -2.63s |
| 13 | api_test | 12.06s | PASS | -5.60s |
| 14 | connection_pool_failure_test | 11.82s | PASS | -3.35s |
| 15 | blog_api_test | 10.21s | PASS | -14.66s |
| 16 | broadcast_ab_testing_e2e_test | 10.11s | PASS | +5.70s |
| 17 | webhook_registration_handler_test | 9.95s | PASS | -4.00s |
| 18 | connection_pool_limits_test | 9.81s | PASS | -12.60s |
| 19 | setup_wizard_test | 9.32s | PASS | -4.15s |
| 20 | contact_list_api_test | 8.77s | PASS | -4.81s |
| 21 | message_history_handler_test | 8.70s | PASS | -6.48s |
| 22 | transactional_handler_test | 8.25s | PASS | -19.26s |
| 23 | task_handler_test | 7.47s | PASS | -20.77s |
| 24 | database_test | 6.95s | PASS | -5.01s |
| 25 | automation_e2e_test | 6.56s | PASS | -43.65s |
| 26 | rate_limiter_test | 6.53s | PASS | -4.36s |
| 27 | user_auth_test | 6.46s | PASS | -3.72s |
| 28 | subscribe_rate_limiter_test | 6.06s | PASS | -0.56s |
| 29 | template_api_test | 5.72s | PASS | -1.04s |
| 30 | email_handler_test | 5.04s | PASS | -1.14s |
| 31 | supabase_integration_e2e_test | 4.88s | PASS | -0.47s |
| 32 | list_unsubscribe_headers_test | 4.76s | PASS | -2.77s |
| 33 | webhook_subscription_status_e2e_test | 4.47s | PASS | -1.16s |
| 34 | template_roundtrip_test | 4.46s | PASS | +0.67s |
| 35 | connection_pool_lifecycle_test | 4.29s | PASS | -17.79s |
| 36 | custom_event_api_test | 4.18s | PASS | -0.17s |
| 37 | contact_bulk_import_e2e_test | 4.03s | PASS | -0.32s |
| 38 | webhook_subscription_e2e_test | 3.97s | PASS | +0.09s |
| 39 | circuit_breaker_broadcast_test | 3.51s | PASS | -0.54s |
| 40 | broadcast_liquid_template_test | 3.45s | PASS | +1.46s |
| 41 | broadcast_scheduled_task_test | 3.45s | PASS | +0.15s |
| 42 | user_logout_test | 3.10s | PASS | -0.25s |
| 43 | migration_version_test | 3.01s | PASS | +0.64s |
| 44 | smtp_client_e2e_test | 2.27s | PASS | +1.11s |

## Slow Tests (>20s)

These tests take longer than 20 seconds and may benefit from optimization:

| Test Suite | Duration | Change |
|------------|----------|--------|
| broadcast_esp_failures_test | 143.62s | +131.78s |
| broadcast_pagination_test | 53.68s | +44.84s |
| broadcast_race_condition_test | 41.57s | +30.24s |
| connection_pool_performance_test | 32.89s | -25.06s |
| segment_e2e_test | 22.18s | +8.75s |
| connection_pool_concurrency_test | 21.42s | -15.68s |

## Failed Tests

### segment_e2e_test (22.18s)

**Subtest Failed**: `TestSegmentE2E/Segment_Rebuild_and_Membership_Updates/should_rebuild_segment_and_update_memberships`

**Error**: Race condition - "task already running" error. The segment stayed in "building" status and never completed.

**Root Cause**: Task concurrency issue where a task is marked as running by one goroutine while another tries to process it.

## Significant Changes

### Much Slower (>+10s)
| Test Suite | Old | New | Change |
|------------|-----|-----|--------|
| broadcast_esp_failures_test | 11.84s | 143.62s | +131.78s |
| broadcast_pagination_test | 8.84s | 53.68s | +44.84s |
| broadcast_race_condition_test | 11.33s | 41.57s | +30.24s |
| broadcast_handler_test | 4.31s | 14.43s | +10.12s |

### Much Faster (>-10s)
| Test Suite | Old | New | Change |
|------------|-----|-----|--------|
| automation_e2e_test | 50.21s | 6.56s | -43.65s |
| connection_pool_performance_test | 57.95s | 32.89s | -25.06s |
| workspace_test | 36.92s | 12.93s | -23.99s |
| task_handler_test | 28.24s | 7.47s | -20.77s |
| transactional_handler_test | 27.51s | 8.25s | -19.26s |
| connection_pool_lifecycle_test | 22.08s | 4.29s | -17.79s |
| connection_pool_concurrency_test | 37.10s | 21.42s | -15.68s |
| blog_api_test | 24.87s | 10.21s | -14.66s |
| connection_pool_limits_test | 22.41s | 9.81s | -12.60s |

## Duration Distribution

- **<5s (Fast)**: 14 tests
- **5-10s (Medium)**: 14 tests
- **10-20s (Slow)**: 10 tests
- **>20s (Very Slow)**: 6 tests

## Recommendations

1. **broadcast_esp_failures_test**: Now the slowest at 143s. Investigate why it's 12x slower than before.

2. **broadcast_pagination_test** and **broadcast_race_condition_test**: Significantly slower. May be related to email queue processing changes.

3. **segment_e2e_test**: Fix the race condition in task processing to make the test reliable.

4. **Connection Pool Tests**: Good improvement overall, now taking ~80s total instead of ~155s.

---
*Report generated: 2025-12-23*
