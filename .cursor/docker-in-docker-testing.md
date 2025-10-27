# Docker-in-Docker Integration Testing Solution

## Problem Discovery

While investigating a signin bug after setup, I needed to run integration tests locally in the Cursor development environment. The tests were hanging indefinitely with no clear error messages.

### Initial Symptoms

```bash
# Running integration test
go test -v ./tests/integration -run TestSetupWizardSigninImmediatelyAfterCompletion

# Result: Timeout after 10s with stack trace showing:
panic: test timed out after 10s
goroutine 23 [IO wait]:
  database/sql.(*DB).PingContext
  # Stuck trying to ping database at localhost:5433
```

## Root Cause Analysis

### Investigation Steps

1. **Checked if Docker was running**: ✅ Services started successfully
2. **Verified port mappings**: ✅ PostgreSQL exposed on `0.0.0.0:5433`
3. **Tested direct connection**: ❌ Connection refused/timeout
4. **Realized the architecture**: We're in Docker-in-Docker!

### The Architecture

```
┌─────────────────────────────────────────────────┐
│   HOST MACHINE                                   │
│                                                   │
│  ┌────────────────────────────────────────────┐ │
│  │   Docker Daemon                             │ │
│  │                                             │ │
│  │  ┌──────────────────────────────────────┐ │ │
│  │  │  Cursor Dev Container (cursor)       │ │ │
│  │  │  - Go workspace                      │ │ │
│  │  │  - Running tests                     │ │ │
│  │  │  - Network: cursor_default           │ │ │
│  │  │                                       │ │ │
│  │  │  Try: localhost:5433                 │ │ │
│  │  │       127.0.0.1:5433  ❌ FAIL       │ │ │
│  │  │       [::1]:5433      ❌ FAIL       │ │ │
│  │  └──────────────────────────────────────┘ │ │
│  │                                             │ │
│  │  ┌──────────────────────────────────────┐ │ │
│  │  │  Test Network (tests_default)        │ │ │
│  │  │                                       │ │ │
│  │  │  ┌────────────────────────────────┐ │ │ │
│  │  │  │ tests-postgres-test-1          │ │ │ │
│  │  │  │ IP: 172.17.0.3                 │ │ │ │
│  │  │  │ Port: 5432 (internal)          │ │ │ │
│  │  │  │ Exposed: 0.0.0.0:5433 (host)   │ │ │ │
│  │  │  └────────────────────────────────┘ │ │ │
│  │  │                                       │ │ │
│  │  │  ┌────────────────────────────────┐ │ │ │
│  │  │  │ tests-mailhog-1                │ │ │ │
│  │  │  │ IP: 172.17.0.2                 │ │ │ │
│  │  │  │ Port: 1025 (internal)          │ │ │ │
│  │  │  │ Exposed: 0.0.0.0:1025 (host)   │ │ │ │
│  │  │  └────────────────────────────────┘ │ │ │
│  │  └──────────────────────────────────────┘ │ │
│  └────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────┘
```

### The Problem

**Port mapping misconception:**
- Test containers expose ports to `0.0.0.0:5433` on the **HOST**
- Cursor container tries to connect to `localhost:5433` (its own network namespace)
- `localhost` inside Cursor container ≠ HOST machine
- Result: Connection refused

**Why it happens:**
1. `docker compose up` runs on HOST's Docker daemon (via `/var/run/docker.sock`)
2. Test containers are created on HOST's network
3. Cursor container has its own isolated network namespace
4. Port `5433` on HOST is not accessible from inside Cursor container

## Solution: Dynamic Container IP Resolution

### The Fix

Created `/workspace/run-integration-tests.sh` that:

1. **Discovers container IPs dynamically** using `docker inspect`
2. **Sets environment variables** for test connectivity
3. **Runs tests** with proper network configuration

### Script Implementation

```bash
#!/bin/bash
set -e

echo "🐳 Integration Test Runner"

# Start test infrastructure if not running
if ! docker ps | grep -q "tests-postgres-test-1"; then
    docker compose -f tests/docker-compose.test.yml up -d
    sleep 8
fi

# Get container IPs dynamically
POSTGRES_IP=$(docker inspect tests-postgres-test-1 \
    --format='{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}')
MAILHOG_IP=$(docker inspect tests-mailhog-1 \
    --format='{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}')

echo "📊 PostgreSQL: $POSTGRES_IP:5432"
echo "📬 MailHog: $MAILHOG_IP:1025"

# Export for tests
export TEST_DB_HOST="$POSTGRES_IP"
export TEST_DB_PORT="5432"          # Internal port, not exposed port!
export TEST_SMTP_HOST="$MAILHOG_IP"
export TEST_DB_USER="notifuse_test"
export TEST_DB_PASSWORD="test_password"

# Run tests
go test -v ./tests/integration -run "${1:-.*}" -timeout 120s
```

### Key Insights

1. **Use Internal Ports**: Connect to `5432`, not `5433`
2. **Use Container IPs**: Direct IP addressing bypasses port mapping
3. **Dynamic Discovery**: Don't hardcode IPs (they change)
4. **Environment Variables**: Flexible configuration for tests

## Testing the Solution

### Verification Commands

```bash
# 1. Confirm we're in a container
cat /proc/1/cgroup | head -5
# Output: Shows docker container ID

# 2. Check container IPs
docker inspect tests-postgres-test-1 \
    --format='{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}'
# Output: 172.17.0.3

# 3. Test direct connection
psql "host=172.17.0.3 port=5432 user=notifuse_test password=test_password dbname=postgres"
# Output: Connected successfully ✅

# 4. Why localhost fails
psql "host=localhost port=5433 user=notifuse_test password=test_password dbname=postgres"
# Output: Connection refused ❌
```

## Code Changes for Flexibility

### Test Utilities Updated

#### 1. `tests/testutil/connection_pool.go`

```go
func GetGlobalTestPool() *TestConnectionPool {
    poolOnce.Do(func() {
        // Default for normal environments
        defaultHost := "localhost"
        defaultPort := 5433
        
        // Check for container environment
        testHost := getEnvOrDefault("TEST_DB_HOST", defaultHost)
        testPort := defaultPort
        if testHost != defaultHost {
            // Custom host = likely containerized
            testPort = 5432  // Use internal port
        }
        
        config := &config.DatabaseConfig{
            Host: testHost,
            Port: testPort,
            // ...
        }
    })
}
```

#### 2. `tests/integration/setup_wizard_test.go`

```go
// Use environment variable for SMTP host
smtpHost := os.Getenv("TEST_SMTP_HOST")
if smtpHost == "" {
    smtpHost = "localhost"  // Default for direct environments
}

initReq := map[string]interface{}{
    "smtp_host": smtpHost,  // Dynamic based on environment
    // ...
}
```

### Design Pattern: Environment Detection

```go
// Pattern used throughout test utilities
func getTestDatabaseHost() string {
    // Try environment variable first
    if host := os.Getenv("TEST_DB_HOST"); host != "" {
        return host  // Containerized environment
    }
    return "localhost"  // Direct environment
}
```

## Usage

### Running Integration Tests

```bash
# From workspace root
./run-integration-tests.sh

# Run specific test
./run-integration-tests.sh TestSetupWizardFlow

# Run all tests matching pattern
./run-integration-tests.sh "TestSetup.*"
```

### Expected Output

```
🐳 Integration Test Runner
==========================

📊 PostgreSQL container IP: 172.17.0.3
📬 MailHog container IP: 172.17.0.2
🔧 Test configuration:
   DB Host: 172.17.0.3
   DB Port: 5432
   DB User: notifuse_test
   SMTP Host: 172.17.0.2

🧪 Running test: TestSetupWizardSigninImmediatelyAfterCompletion

=== RUN   TestSetupWizardSigninImmediatelyAfterCompletion
=== RUN   TestSetupWizardSigninImmediatelyAfterCompletion/Complete_Setup_and_Signin_Without_Restart
=== RUN   TestSetupWizardSigninImmediatelyAfterCompletion/Verify_Mailer_Config_Updated_After_Setup
--- PASS: TestSetupWizardSigninImmediatelyAfterCompletion (0.69s)
    --- PASS: TestSetupWizardSigninImmediatelyAfterCompletion/Complete_Setup_and_Signin_Without_Restart (0.02s)
    --- PASS: TestSetupWizardSigninImmediatelyAfterCompletion/Verify_Mailer_Config_Updated_After_Setup (0.00s)
PASS
ok  	github.com/Notifuse/notifuse/tests/integration	1.148s

✅ Tests passed!
```

## Alternative Solutions Considered

### 1. Docker Network Connection

**Attempt**: Connect Cursor container to test network

```bash
# Get Cursor container name
docker ps | grep cursor

# Try to connect
docker network connect tests_default cursor
# Error: No such container: cursor
```

**Problem**: Can't reference Cursor container from within itself

### 2. Host Network Mode

**Attempt**: Run tests with `--network=host`

```bash
go test --network=host ./tests/integration
```

**Problem**: Go test doesn't support Docker flags

### 3. Service Names

**Attempt**: Use Docker service names

```go
dsn := "host=tests-postgres-test-1 port=5432 ..."
```

**Problem**: DNS resolution fails across different networks

**Why Dynamic IPs Work**: Direct IP routing works even across network boundaries

## CI/CD Compatibility

### GitHub Actions Environment

GitHub Actions runs differently:

```yaml
# In GitHub Actions
- name: Run integration tests
  run: |
    docker compose -f tests/docker-compose.test.yml up -d
    go test -v ./tests/integration  # Works with localhost!
```

**Why it works**:
- Tests run directly on runner (not in container)
- `localhost:5433` correctly routes to exposed port
- No Docker-in-Docker complexity

### Environment Detection

```go
// Tests work in both environments
func getTestDBHost() string {
    if host := os.Getenv("TEST_DB_HOST"); host != "" {
        return host  // Cursor: 172.17.0.3
    }
    return "localhost"  // GitHub Actions: localhost
}
```

## Troubleshooting

### Test Hangs/Timeouts

```bash
# Check if containers are running
docker ps | grep tests

# Check container logs
docker logs tests-postgres-test-1
docker logs tests-mailhog-1

# Verify connectivity
POSTGRES_IP=$(docker inspect tests-postgres-test-1 --format='{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}')
nc -zv $POSTGRES_IP 5432  # Should succeed
```

### IPv6 Issues

If you see `dial tcp [::1]:5433: connect: connection refused`:

```bash
# Problem: Go trying IPv6 first
host=localhost  # Resolves to ::1 (IPv6) or 127.0.0.1 (IPv4)

# Solution: Use explicit IPv4
host=127.0.0.1  # Forces IPv4

# Or: Use container IP (best)
host=172.17.0.3  # Bypasses DNS entirely
```

### Port Confusion

```
Exposed Port (host): 5433
Internal Port (container): 5432

From HOST:        localhost:5433     ✅
From Cursor:      localhost:5433     ❌
From Cursor:      172.17.0.3:5432    ✅ (internal port!)
```

## Lessons Learned

### 1. Network Namespaces Matter

`localhost` is not universal—it's relative to the network namespace.

### 2. Port Mapping vs. Port Forwarding

- **Port mapping**: HOST:5433 → Container:5432
- **Direct IP**: Container:5432 (no mapping)

### 3. Container Discovery

`docker inspect` is your friend for dynamic environments.

### 4. Test Flexibility

Design tests to work in multiple environments (local, containerized, CI).

### 5. Documentation is Key

Complex setups need clear documentation for future developers.

## References

### Docker Networking Docs

- [Docker Networks](https://docs.docker.com/network/)
- [Container Networking](https://docs.docker.com/config/containers/container-networking/)
- [Docker Compose Networking](https://docs.docker.com/compose/networking/)

### Commands Used

```bash
# Container inspection
docker inspect <container>
docker ps --format '{{.Names}}\t{{.Networks}}'
docker network ls

# Network debugging
docker exec <container> ping <target>
docker logs <container>
netstat -tulpn | grep 5432
```

## Future Improvements

### 1. Shared Docker Network

Create a shared network for Cursor and test containers:

```yaml
# docker-compose.yml
networks:
  dev:
    external: true  # Created by Cursor
```

### 2. Service Discovery

Use Docker DNS for service discovery:

```bash
# If on same network
host=postgres-test  # DNS resolves automatically
```

### 3. Development Containers

Use VS Code Dev Containers with proper network configuration:

```json
// .devcontainer/devcontainer.json
{
  "runArgs": ["--network=tests_default"]
}
```

### 4. Test Harness

Create a test harness that auto-detects environment:

```go
func NewTestEnvironment() *TestEnv {
    if isDockerInDocker() {
        return &TestEnv{useContainerIPs: true}
    }
    return &TestEnv{useLocalhost: true}
}
```

## Conclusion

Docker-in-Docker testing requires understanding:
1. Network namespaces and isolation
2. Port mapping vs. direct IP access
3. Container discovery and dynamic configuration
4. Environment-agnostic test design

**Solution**: Dynamic IP discovery + environment variables = Portable tests

**Result**: Integration tests now work seamlessly in Cursor development containers! 🎉
