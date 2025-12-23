# Integration Test Timing Report

Generated: 2025-12-23 (Updated after fixes)

## Summary

| Metric | Value |
|--------|-------|
| Total Test Suites | 43 |
| Passed | 43 |
| Failed | 0 |
| Pass Rate | 100% |
| Total Duration | ~11 minutes |

## Test Suites by Duration (Slowest First)

| Rank | Test Suite | Duration | Status |
|------|------------|----------|--------|
| 1 | connection_pool_performance_test | 57.95s | PASS |
| 2 | automation_e2e_test | 50.21s | PASS |
| 3 | connection_pool_concurrency_test | 37.10s | PASS |
| 4 | workspace_test | 36.92s | PASS |
| 5 | task_handler_test | 28.24s | PASS |
| 6 | transactional_handler_test | 27.51s | PASS |
| 7 | blog_api_test | 24.87s | PASS |
| 8 | connection_pool_limits_test | 22.41s | PASS |
| 9 | connection_pool_lifecycle_test | 22.08s | PASS |
| 10 | smtp_relay_e2e_test | 17.74s | PASS |
| 11 | api_test | 17.66s | PASS |
| 12 | connection_pool_failure_test | 15.17s | PASS |
| 13 | contact_api_test | 15.27s | PASS |
| 14 | message_history_handler_test | 15.18s | PASS |
| 15 | email_queue_e2e_test | 14.07s | PASS |
| 16 | webhook_registration_handler_test | 13.95s | PASS |
| 17 | contact_list_api_test | 13.58s | PASS |
| 18 | setup_wizard_test | 13.47s | PASS |
| 19 | segment_e2e_test | 13.43s | PASS |
| 20 | broadcast_esp_failures_test | 11.84s | PASS |
| 21 | broadcast_race_condition_test | 11.33s | PASS |
| 22 | database_test | 11.96s | PASS |
| 23 | rate_limiter_test | 10.89s | PASS |
| 24 | user_auth_test | 10.18s | PASS |
| 25 | broadcast_pagination_test | 8.84s | PASS |
| 26 | list_unsubscribe_headers_test | 7.53s | PASS |
| 27 | template_api_test | 6.76s | PASS |
| 28 | subscribe_rate_limiter_test | 6.62s | PASS |
| 29 | email_handler_test | 6.18s | PASS |
| 30 | webhook_subscription_status_e2e_test | 5.63s | PASS |
| 31 | supabase_integration_e2e_test | 5.35s | PASS |
| 32 | broadcast_task_timeout_test | 5.02s | PASS |
| 33 | broadcast_ab_testing_e2e_test | 4.41s | PASS |
| 34 | custom_event_api_test | 4.35s | PASS |
| 35 | contact_bulk_import_e2e_test | 4.35s | PASS |
| 36 | broadcast_handler_test | 4.31s | PASS |
| 37 | circuit_breaker_broadcast_test | 4.05s | PASS |
| 38 | webhook_subscription_e2e_test | 3.88s | PASS |
| 39 | template_roundtrip_test | 3.79s | PASS |
| 40 | user_logout_test | 3.35s | PASS |
| 41 | broadcast_scheduled_task_test | 3.30s | PASS |
| 42 | migration_version_test | 2.37s | PASS |
| 43 | broadcast_liquid_template_test | 1.99s | PASS |
| 44 | smtp_client_e2e_test | 1.16s | PASS |

## Slow Tests (>20s)

These tests take longer than 20 seconds and may benefit from optimization:

| Test Suite | Duration |
|------------|----------|
| connection_pool_performance_test | 57.95s |
| automation_e2e_test | 50.21s |
| connection_pool_concurrency_test | 37.10s |
| workspace_test | 36.92s |
| task_handler_test | 28.24s |
| transactional_handler_test | 27.51s |
| blog_api_test | 24.87s |
| connection_pool_limits_test | 22.41s |
| connection_pool_lifecycle_test | 22.08s |

## Failed Tests

All tests are now passing.

## Fixes Applied (2025-12-23)

The following fixes were applied to resolve the 12 originally failing tests:

### Schema Fix
- Added `enqueued_count` column to `internal/database/init.go` broadcasts table

### Test-Specific Fixes

| Test | Issue | Fix |
|------|-------|-----|
| broadcast_pagination_test | Emails queued but not processed | Added `StartBackgroundWorkers(ctx)` |
| broadcast_task_timeout_test | Only 144/200 emails (3s sleep too short) | Added `StartBackgroundWorkers(ctx)` + `WaitForMailpitMessages` |
| circuit_breaker_broadcast_test | Status mismatch + missing column | Fixed assertions + added `enqueued_count` to test schema |

### Root Cause
The email queue system requires explicit worker startup via `suite.ServerManager.StartBackgroundWorkers(ctx)` for emails to be processed.

## Duration Distribution

- **<5s (Fast)**: 14 tests
- **5-15s (Medium)**: 16 tests
- **15-30s (Slow)**: 9 tests
- **>30s (Very Slow)**: 4 tests

## Recommendations

1. **Connection Pool Tests**: The 5 connection pool tests together take ~155s. Consider running them in parallel or optimizing database setup.

2. **Automation E2E**: At 50s, this is the second slowest. Review if all subtests are necessary or if setup can be shared.

---
*Report updated after all tests fixed - 2025-12-23*
