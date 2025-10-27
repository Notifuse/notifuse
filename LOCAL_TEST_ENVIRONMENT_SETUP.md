# Local Test Environment Setup - Notifuse

## Issue Discovery

When running integration tests in the Cursor development container (Docker-in-Docker environment), tests were failing due to network connectivity issues between containers.

## Root Cause

The Notifuse workspace runs inside a **Cursor development container**, which is itself running in Docker. When integration tests start PostgreSQL and MailHog containers via `docker compose`, these test containers run on the **host's Docker daemon**, not inside the Cursor container.

This creates a network isolation problem:
- **Cursor container**: Can't reach `localhost:5433` (PostgreSQL)
- **Test containers**: Running on host's Docker network
- **Result**: Connection timeouts and test failures

## Solution: Test Runner Script

Created `/workspace/run-integration-tests.sh` that:

1. **Starts test infrastructure** (PostgreSQL + MailHog)
2. **Dynamically retrieves container IPs** using `docker inspect`
3. **Sets environment variables** for test connectivity:
   - `TEST_DB_HOST` → PostgreSQL container IP
   - `TEST_DB_PORT` → Internal port (5432)
   - `TEST_SMTP_HOST` → MailHog container IP
4. **Runs integration tests** with proper network configuration

## Usage

### Run Integration Tests (Local Containerized Environment)

```bash
# Run specific test
./run-integration-tests.sh TestSetupWizardSigninImmediatelyAfterCompletion

# Run all integration tests
./run-integration-tests.sh ".*"
```

### Run Integration Tests (Direct/CI Environment)

```bash
# Start test infrastructure
docker compose -f tests/docker-compose.test.yml up -d

# Run tests (localhost works in direct environments)
go test -v ./tests/integration -run TestName
```

## Test Utilities Updates

Updated test utilities to support both environments:

### `/workspace/tests/testutil/connection_pool.go`
- Defaults to `localhost:5433` for normal environments
- Respects `TEST_DB_HOST` and `TEST_DB_PORT` environment variables
- Automatically uses internal port (5432) when custom host is set

### `/workspace/tests/testutil/database.go`
- Same flexible host/port configuration
- Works in both containerized and direct environments

### `/workspace/tests/testutil/helpers.go`
- Removed hardcoded `TEST_DB_HOST` setting
- Allows external configuration via environment variables

### `/workspace/tests/integration/setup_wizard_test.go`
- Added `TEST_SMTP_HOST` environment variable support
- Defaults to `localhost` for non-containerized environments
- Uses container IP when running in Cursor container

## Architecture

```
┌─────────────────────────────────────┐
│   Cursor Development Container      │
│  ┌──────────────────────────────┐  │
│  │  Notifuse Workspace          │  │
│  │  - Go code                   │  │
│  │  - Integration tests         │  │
│  │  - run-integration-tests.sh  │  │
│  └───────────┬──────────────────┘  │
└──────────────┼──────────────────────┘
               │ Docker socket
               │ (/var/run/docker.sock)
               ▼
┌──────────────────────────────────────┐
│     Host Docker Daemon               │
│  ┌────────────────────────────────┐ │
│  │  Test Network (tests_default)  │ │
│  │  ┌────────────────────────┐   │ │
│  │  │ tests-postgres-test-1  │   │ │
│  │  │ IP: 172.17.0.3:5432   │   │ │
│  │  └────────────────────────┘   │ │
│  │  ┌────────────────────────┐   │ │
│  │  │ tests-mailhog-1        │   │ │
│  │  │ IP: 172.17.0.2:1025   │   │ │
│  │  └────────────────────────┘   │ │
│  └────────────────────────────────┘ │
└──────────────────────────────────────┘
```

## Key Learnings

1. **Docker-in-Docker Networking**: Containers started from within a container can't use `localhost` to communicate
2. **Dynamic Configuration**: Use container IPs or service names depending on the network setup
3. **Environment Flexibility**: Design tests to work in both local and CI environments
4. **Test Utilities**: Create abstractions that handle environment differences transparently

## Files Modified

1. **New**: `/workspace/run-integration-tests.sh` - Test runner script
2. **Updated**: `/workspace/tests/testutil/connection_pool.go` - Flexible DB connectivity
3. **Updated**: `/workspace/tests/testutil/database.go` - Flexible DB configuration
4. **Updated**: `/workspace/tests/testutil/helpers.go` - Removed hardcoded hosts
5. **Updated**: `/workspace/tests/integration/setup_wizard_test.go` - Flexible SMTP host

## Testing

All integration and unit tests pass successfully with the new setup:

```bash
✅ TestSetupWizardSigninImmediatelyAfterCompletion - PASS
✅ TestAppInitMailer - PASS
✅ TestNewApp - PASS
```

## Future Improvements

1. Consider using Docker Compose networks for better isolation
2. Add health checks before running tests
3. Create separate test profiles for local vs CI environments
4. Document CI environment setup (GitHub Actions uses native Docker)
