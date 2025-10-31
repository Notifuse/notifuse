# Integration Testing Framework for Notifuse API

This directory contains the integration testing framework for the Notifuse API, providing comprehensive end-to-end testing capabilities.

## Directory Structure

```
tests/
├── README.md                    # This file
├── compose.test.yaml            # Test infrastructure (PostgreSQL, MailHog)
├── testutil/                    # Test utilities and helpers
│   ├── database.go              # Database management for tests
│   ├── server.go                # Test server management
│   ├── client.go                # HTTP client for API testing
│   ├── factory.go               # Test data factory
│   └── helpers.go               # General test helpers
└── integration/                 # Integration test files
    ├── database_test.go         # Database integration tests
    ├── api_test.go              # Basic API integration tests
    └── contact_api_test.go      # Contact API integration tests
```

## Quick Start

### 1. Prerequisites

- Docker and Docker Compose
- Go 1.21+
- Make (optional, but recommended)

### 2. Run Integration Tests

```bash
# Start test infrastructure
make -f Makefile.integration test-integration-setup

# Run all integration tests
make -f Makefile.integration test-integration

# Or run the full cycle (setup + test + teardown)
make -f Makefile.integration test-integration-full
```

### 3. Manual Setup (Alternative)

```bash
# Start test database
cd tests && docker compose -f compose.test.yaml up -d

# Run tests with environment variables
INTEGRATION_TESTS=true \
TEST_DB_HOST=localhost \
TEST_DB_PORT=5433 \
TEST_DB_USER=notifuse_test \
TEST_DB_PASSWORD=test_password \
ENVIRONMENT=test \
go test -v ./tests/integration/...

# Stop test infrastructure
cd tests && docker compose -f compose.test.yaml down -v
```

## Test Infrastructure

### Database (PostgreSQL)

- **Host**: localhost
- **Port**: 5433
- **User**: notifuse_test
- **Password**: test_password
- **Max Connections**: 500 (optimized for concurrent testing)
- **Shared Buffers**: 256MB
- **Optimizations**: Configured for high-concurrency integration testing

The test PostgreSQL instance is automatically configured with optimized settings for concurrent testing:

- Increased connection limits (500 max connections)
- Optimized memory settings
- Relaxed durability settings for better performance
- Enhanced logging for debugging

### Performance Optimizations

The test database includes several optimizations:

- `synchronous_commit = 'off'` - Faster commits for tests
- `work_mem = '32MB'` - More memory for complex queries
- `max_locks_per_transaction = 256` - Support for concurrent operations
- `checkpoint_timeout = '15min'` - Reduced checkpoint frequency
- **Port**: 5433 (to avoid conflicts with development database)
- **User**: notifuse_test
- **Password**: test_password
- **Database**: Created dynamically per test

### Email Testing (MailHog)

- **SMTP**: localhost:1025
- **Web UI**: http://localhost:8025

The integration tests now include proper SMTP email provider configuration using MailHog for testing email functionality. This eliminates the need to skip email-related tests and provides comprehensive testing of broadcast sending, scheduling, and A/B testing features.

#### MailHog Configuration in Tests

The test suite automatically configures a MailHog SMTP provider for each workspace:

- **Host**: localhost
- **Port**: 1025 (SMTP)
- **Authentication**: None (MailHog doesn't require auth)
- **TLS**: Disabled (for local testing)
- **Default Sender**: noreply@notifuse.test

You can view sent emails by accessing the MailHog web UI at http://localhost:8025 during test execution.

## Test Components

### DatabaseManager (`testutil/database.go`)

- Creates isolated test databases
- Runs migrations automatically
- Provides test data seeding
- Handles cleanup

### ServerManager (`testutil/server.go`)

- Starts API server on random port
- Manages server lifecycle
- Provides graceful shutdown

### APIClient (`testutil/client.go`)

- HTTP client with authentication support
- Built-in retry logic
- Helper methods for common API operations

### TestDataFactory (`testutil/factory.go`)

- Creates test entities (users, workspaces, contacts, etc.)
- Supports customization through options pattern
- Persists data to database

### IntegrationTestSuite (`testutil/helpers.go`)

- Complete testing environment setup
- Combines all test utilities
- Provides cleanup and reset functionality

## Writing Integration Tests

### Basic Test Structure

```go
func TestMyFeature(t *testing.T) {
    testutil.SkipIfShort(t)
    testutil.SetupTestEnvironment()
    defer testutil.CleanupTestEnvironment()

    suite := testutil.NewIntegrationTestSuite(t, appFactory)
    defer suite.Cleanup()

    // Your test code here
}
```

### Using the Data Factory

```go
// Create test data
user, err := suite.DataFactory.CreateUser(
    testutil.WithUserEmail("test@example.com"),
    testutil.WithUserName("Test User"),
)
require.NoError(t, err)

contact, err := suite.DataFactory.CreateContact(
    testutil.WithContactEmail("contact@example.com"),
    testutil.WithContactName("John", "Doe"),
)
require.NoError(t, err)
```

### Making API Requests

```go
// List contacts
resp, err := suite.APIClient.ListContacts(map[string]string{
    "limit": "10",
})
require.NoError(t, err)
defer resp.Body.Close()

// Create contact
contact := map[string]interface{}{
    "email": "new@example.com",
    "first_name": "Jane",
    "last_name": "Smith",
}
resp, err = suite.APIClient.CreateContact(contact)
require.NoError(t, err)
defer resp.Body.Close()
```

## Available Make Commands

```bash
# Setup and teardown
make test-integration-setup     # Start test infrastructure
make test-integration-teardown  # Stop test infrastructure
make test-integration-reset     # Reset infrastructure

# Running tests
make test-integration           # Run all integration tests
make test-integration-full      # Full cycle (setup + test + teardown)
make test-integration-quick     # Run subset of tests
make test-integration-watch     # Run tests in watch mode

# Specific test categories
make test-integration-database  # Database tests only
make test-integration-api       # API tests only
make test-integration-contacts  # Contact API tests only

# Utilities
make test-integration-health    # Check infrastructure health
make test-integration-logs      # View infrastructure logs
make test-integration-coverage  # Run with coverage report
make test-integration-debug     # Run with debug logging
```

## Configuration

### Environment Variables

- `INTEGRATION_TESTS=true` - Required to run integration tests
- `TEST_DB_HOST` - Test database host (default: localhost)
- `TEST_DB_PORT` - Test database port (default: 5433)
- `TEST_DB_USER` - Test database user (default: notifuse_test)
- `TEST_DB_PASSWORD` - Test database password (default: test_password)
- `ENVIRONMENT=test` - Set application environment to test mode

### Test Database

Each test creates its own isolated database with a unique name based on timestamp. This ensures tests don't interfere with each other.

## Best Practices

### 1. Test Isolation

- Each test should be independent
- Use `suite.ResetData()` between test cases if needed
- Clean up resources in `defer` statements

### 2. Error Handling

- Always check for errors with `require.NoError(t, err)`
- Use descriptive error messages
- Handle edge cases explicitly

### 3. Data Management

- Use the DataFactory for creating test data
- Don't hardcode IDs or sensitive values
- Use helper functions for common data patterns

### 4. Performance

- Use `testutil.SkipIfShort(t)` for long-running tests
- Set appropriate timeouts
- Consider parallel test execution where appropriate

### 5. Debugging

- Use `t.Logf()` for debugging output
- Check test infrastructure health before debugging
- Use the debug mode for verbose logging

## Common Issues

### Database Connection Issues

```bash
# Check if database is running
make test-integration-health

# Reset infrastructure
make test-integration-reset
```

### Port Conflicts

The test infrastructure uses ports 5433 and 8025. Make sure these ports are available.

### Permission Issues

Ensure Docker has permission to create volumes and networks.

## Adding New Tests

1. Create test file in `tests/integration/`
2. Follow the test structure pattern
3. Use the IntegrationTestSuite for setup
4. Add specific Make targets if needed
5. Update this README with new test categories

## Future Enhancements

- [ ] Performance testing framework
- [ ] Load testing utilities
- [ ] Mock external services
- [ ] Test data generators
- [ ] Parallel test execution
- [ ] Test result reporting
- [ ] CI/CD integration helpers
