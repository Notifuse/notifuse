# Makefile Test Commands Reference

## Overview

The Makefile provides comprehensive test commands for running unit tests, integration tests, and connection pool tests with various configurations.

---

## Quick Reference

### Most Common Commands

```bash
# Run all unit tests
make test-unit

# Run connection pool integration tests (recommended)
make test-connection-pools

# Run tests in agent mode (unit + integration)
make test-agent

# Run with race detector
make test-connection-pools-race
```

---

## Unit Test Commands

### `make test-unit`
**Description**: Runs all unit tests with race detector  
**Scope**: Domain, HTTP, Service, Repository, Migrations, Database layers  
**Flags**: `-race -v`  
**Duration**: ~30-60 seconds

### `make test-domain`
**Description**: Runs domain layer tests only  
**Scope**: `./internal/domain`  
**Flags**: `-race -v`

### `make test-service`
**Description**: Runs service layer tests only  
**Scope**: `./internal/service`  
**Flags**: `-race -v`

### `make test-repo`
**Description**: Runs repository layer tests only  
**Scope**: `./internal/repository`  
**Flags**: `-race -v`

### `make test-http`
**Description**: Runs HTTP handler tests only  
**Scope**: `./internal/http`  
**Flags**: `-race -v`

### `make test-migrations`
**Description**: Runs migration tests only  
**Scope**: `./internal/migrations`  
**Flags**: `-race -v`

### `make test-database`
**Description**: Runs database layer tests only  
**Scope**: `./internal/database`  
**Flags**: `-race -v`

### `make test-pkg`
**Description**: Runs package-level tests  
**Scope**: `./pkg/...`  
**Flags**: `-race -v`

---

## Integration Test Commands

### `make test-integration`
**Description**: Runs all integration tests (may have connection issues)  
**Scope**: `./tests/integration/`  
**Flags**: `-race -timeout 9m -v`  
**Environment**: `INTEGRATION_TESTS=true`  
**Note**: ⚠️ May encounter PostgreSQL connection exhaustion

---

## Connection Pool Integration Tests

### `make test-connection-pools` ✅ RECOMMENDED
**Description**: Runs all connection pool tests sequentially with delays  
**Duration**: ~2-3 minutes  
**Test Suites**:
1. TestConnectionPoolLifecycle (9s)
2. TestConnectionPoolConcurrency (17s)
3. TestConnectionPoolLimits (18s)
4. TestConnectionPoolFailureRecovery (11s)
5. TestConnectionPoolPerformance (48s)

**Delays**: 3 seconds between each test suite  
**Uses**: `./run-integration-tests.sh` script  
**Success Rate**: 100% ✅

**Example Output**:
```bash
Running connection pool tests (individually to avoid connection exhaustion)...
✅ TestConnectionPoolLifecycle - PASS (9.0s)
✅ TestConnectionPoolConcurrency - PASS (17.4s)
✅ TestConnectionPoolLimits - PASS (18.1s)
✅ TestConnectionPoolFailureRecovery - PASS (11.2s)
✅ TestConnectionPoolPerformance - PASS (48.1s)
```

### `make test-connection-pools-race`
**Description**: Runs connection pool tests with race detector  
**Duration**: ~3-5 minutes (slower due to race detector)  
**Flags**: `-race`  
**Use Case**: Detecting race conditions in concurrent code

### `make test-connection-pools-short`
**Description**: Runs fast connection pool tests only  
**Test Suites**:
- TestConnectionPoolLifecycle
- TestConnectionPoolLimits

**Duration**: ~15-20 seconds  
**Use Case**: Quick validation during development

### `make test-connection-pools-leak-check`
**Description**: Runs lifecycle tests with connection leak detection  
**Post-Test**: Queries PostgreSQL for leaked connections  
**Use Case**: Debugging connection leaks

### Individual Connection Pool Test Suites

```bash
make test-connection-pools-lifecycle      # Lifecycle tests only
make test-connection-pools-concurrency    # Concurrency tests only
make test-connection-pools-limits         # Limits tests only
make test-connection-pools-failure        # Failure recovery tests only
make test-connection-pools-performance    # Performance tests only
```

---

## Agent Mode (CI/CD Optimized)

### `make test-agent` ✅ RECOMMENDED FOR CI
**Description**: Runs unit tests + connection pool integration tests  
**Components**:
1. Unit tests (all layers) - filtered output
2. Connection pool integration tests (sequential)

**Output**: Concise, shows failures and summaries only  
**Duration**: ~3-5 minutes  
**Use Case**: Automated testing in CI/CD pipelines

**What it does**:
1. Runs all unit tests with filtered output (FAIL/PASS only)
2. Shows unit test summary (last 10 lines)
3. Runs all connection pool integration tests sequentially
4. Reports all results

**Example Usage in CI**:
```yaml
# GitHub Actions
- name: Run Tests
  run: make test-agent
  timeout-minutes: 10
```

---

## Coverage Commands

### `make coverage`
**Description**: Generates comprehensive test coverage report  
**Output**: 
- `coverage.out` - Coverage data
- `coverage.html` - HTML report
- Terminal summary with total coverage percentage

**Flags**: `-race -coverprofile=coverage.out -covermode=atomic`  
**Excludes**: Integration tests  
**Opens**: HTML report in browser (on some systems)

---

## Build Commands

### `make build`
**Description**: Builds the API server binary  
**Output**: `bin/server`

### `make run`
**Description**: Runs the API server from source  
**Command**: `go run ./cmd/api`

### `make dev`
**Description**: Runs in development mode with hot reload  
**Tool**: Air (live reload for Go)

### `make clean`
**Description**: Removes build artifacts and coverage reports  
**Removes**: `bin/`, `coverage.out`, `coverage.html`

---

## Docker Commands

### `make docker-build`
**Description**: Builds Docker image  
**Tag**: `notifuse:latest`

### `make docker-run`
**Description**: Runs the application in Docker container  
**Ports**: 8080:8080  
**Name**: `notifuse`

### `make docker-stop`
**Description**: Stops and removes the Docker container

### `make docker-clean`
**Description**: Stops container and removes Docker image

### `make docker-logs`
**Description**: Shows Docker container logs (follow mode)

---

## Test Execution Flow

### Standard Development Workflow
```bash
# 1. Run unit tests during development
make test-unit

# 2. Run specific layer tests
make test-service

# 3. Run connection pool tests when working on database code
make test-connection-pools-short

# 4. Run full test suite before committing
make test-agent
```

### CI/CD Pipeline Workflow
```bash
# Single command for comprehensive testing
make test-agent

# Or break it down:
make test-unit                    # Fast feedback (30s)
make test-connection-pools        # Thorough integration tests (3min)
make coverage                     # Generate coverage reports
```

---

## PostgreSQL Configuration

Connection pool tests require properly configured PostgreSQL:

**File**: `tests/docker-compose.test.yml`

```yaml
services:
  postgres-test:
    image: postgres:17-alpine
    command:
      - "postgres"
      - "-c"
      - "max_connections=300"
      - "-c"
      - "shared_buffers=128MB"
```

**Start test database**:
```bash
cd tests
docker-compose -f docker-compose.test.yml up -d
```

---

## Troubleshooting

### Connection Pool Tests Hanging
**Problem**: Tests timeout or hang  
**Solution**: Run tests sequentially with `make test-connection-pools`  
**Cause**: PostgreSQL connection exhaustion

### Race Detector Failures
**Problem**: Race conditions detected  
**Solution**: Fix the race condition in the code  
**Command**: `make test-connection-pools-race` to reproduce

### Connection Leaks
**Problem**: Tests report leaked connections  
**Solution**: Run `make test-connection-pools-leak-check`  
**Check**: Verify all `defer pool.Cleanup()` calls are present

### Slow Test Execution
**Problem**: Tests take too long  
**Solution**: Use `make test-connection-pools-short` for quick validation  
**Alternative**: Run specific test suites individually

---

## Best Practices

1. **During Development**: Use `make test-unit` for fast feedback
2. **Before Commit**: Run `make test-agent` for comprehensive validation
3. **In CI/CD**: Use `make test-agent` with 10-minute timeout
4. **Debugging**: Use individual test suite commands for targeted testing
5. **Performance**: Use `make test-connection-pools-short` for quick checks
6. **Race Detection**: Run `make test-connection-pools-race` periodically

---

## Summary

| Command | Duration | Use Case |
|---------|----------|----------|
| `make test-unit` | 30-60s | Fast unit test feedback |
| `make test-agent` | 3-5min | CI/CD comprehensive testing |
| `make test-connection-pools` | 2-3min | Full integration test suite |
| `make test-connection-pools-short` | 15-20s | Quick integration validation |
| `make test-connection-pools-race` | 3-5min | Race condition detection |
| `make coverage` | 1-2min | Coverage report generation |

**Recommended for CI/CD**: `make test-agent` ✅
