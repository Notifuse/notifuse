# Makefile Test Commands Reference

## Overview

The Makefile provides comprehensive test commands for running unit tests, integration tests, and connection pool tests with various configurations.

---

## Quick Reference

### Most Common Commands

```bash
# Run all unit tests
make test-unit

# Run all integration tests within Cursor Agent (non-verbose)
make e2e-test-within-cursor-agent

# Run all integration tests (verbose)
make test-integration
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
**Description**: Runs all integration tests (verbose)  
**Scope**: `./tests/integration/`  
**Flags**: `-race -timeout 9m -v`  
**Environment**: `INTEGRATION_TESTS=true`  
**Duration**: ~2-3 minutes  
**Use Case**: Detailed debugging with full output

---

## End-to-End Testing (Cursor Agent / CI/CD Optimized)

### `make e2e-test-within-cursor-agent` ✅ RECOMMENDED FOR CURSOR AGENT
**Description**: Runs all integration tests (non-verbose)  
**Scope**: All tests in `./tests/integration/`  
**Output**: Non-verbose, shows only PASS/FAIL/test names  
**Duration**: ~2-3 minutes  
**Use Case**: 
- Cursor Agent automated testing
- CI/CD pipelines
- Quick validation with clean output

**What it does**:
1. Uses `./run-integration-tests.sh` to run all integration tests
2. Filters output to show only test names, PASS/FAIL, and timing
3. Reports completion status

**Example Output**:
```bash
Running all integration tests (non-verbose)...
=== RUN   TestConnectionPoolLifecycle
--- PASS: TestConnectionPoolLifecycle (8.45s)
=== RUN   TestConnectionPoolConcurrency
--- PASS: TestConnectionPoolConcurrency (16.91s)
=== RUN   TestAPIServerShutdown
--- PASS: TestAPIServerShutdown (2.15s)
ok      github.com/Notifuse/notifuse/tests/integration

✅ All integration tests completed
```

**Example Usage**:
```bash
# Cursor Agent (recommended)
make e2e-test-within-cursor-agent

# GitHub Actions
- name: Run E2E Tests
  run: make e2e-test-within-cursor-agent
  timeout-minutes: 5
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

# 3. Run integration tests before committing
make e2e-test-within-cursor-agent
```

### CI/CD Pipeline Workflow
```bash
# Single command for comprehensive testing
make e2e-test-within-cursor-agent

# Or break it down:
make test-unit                    # Fast feedback (30s)
make test-integration             # Verbose integration tests (3min)
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
2. **Before Commit**: Run `make e2e-test-within-cursor-agent` for comprehensive validation
3. **In CI/CD**: Use `make e2e-test-within-cursor-agent` with 5-minute timeout
4. **In Cursor Agent**: Use `make e2e-test-within-cursor-agent` for automated testing
5. **Debugging**: Use `make test-integration` for verbose output
6. **Coverage**: Run `make coverage` to generate coverage reports

---

## Summary

| Command | Duration | Use Case |
|---------|----------|----------|
| `make test-unit` | 30-60s | Fast unit test feedback |
| `make e2e-test-within-cursor-agent` | 2-3min | All integration tests (non-verbose, Cursor Agent) |
| `make test-integration` | 2-3min | All integration tests (verbose, debugging) |
| `make coverage` | 1-2min | Coverage report generation |

**Recommended for Cursor Agent / CI/CD**: `make e2e-test-within-cursor-agent` ✅  
**Recommended for detailed debugging**: `make test-integration` ✅
