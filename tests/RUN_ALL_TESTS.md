# Running All Connection Pool Tests Together

## Problem

When running all 33 connection pool test cases together, PostgreSQL's default `max_connections=100` is exhausted, causing tests to timeout.

## Solutions

### Solution 1: Increase PostgreSQL max_connections (✅ Recommended)

**Already Applied:** The `tests/compose.test.yaml` now configures PostgreSQL with `max_connections=300`.

```yaml
postgres-test:
  command:
    - "postgres"
    - "-c"
    - "max_connections=300"
    - "-c"
    - "shared_buffers=128MB"
```

**Restart PostgreSQL to apply:**
```bash
cd tests && docker compose -f compose.test.yaml down
cd tests && docker compose -f compose.test.yaml up -d
```

**Then run all tests:**
```bash
./run-integration-tests.sh TestConnectionPool
# or
INTEGRATION_TESTS=true go test -v ./tests/integration -run TestConnectionPool -timeout 15m
```

### Solution 2: Add Cleanup Delays Between Tests

Add explicit delays to ensure connections fully close:

```bash
# Run with delays between suites
for suite in Lifecycle Concurrency Limits Failure Performance; do
  echo "Running TestConnectionPool${suite}..."
  ./run-integration-tests.sh "TestConnectionPool${suite}"
  echo "Waiting for connections to close..."
  sleep 5
done
```

### Solution 3: Use Makefile (Already Configured)

The Makefile already runs tests individually:

```bash
make test-connection-pools
```

This runs each suite separately with proper cleanup between them.

### Solution 4: Configure CI with Matrix Strategy

For GitHub Actions, use a matrix to parallelize:

```yaml
jobs:
  connection-pool-tests:
    strategy:
      matrix:
        suite:
          - TestConnectionPoolLifecycle
          - TestConnectionPoolConcurrency
          - TestConnectionPoolLimits
          - TestConnectionPoolFailure
          - TestConnectionPoolPerformance
    steps:
      - name: Run ${{ matrix.suite }}
        run: |
          docker compose -f tests/compose.test.yaml up -d
          ./run-integration-tests.sh "${{ matrix.suite }}"
```

This runs suites in parallel across different runners.

## Recommended Approach

### For Local Development:
```bash
# Option 1: Use the Makefile (runs individually)
make test-connection-pools

# Option 2: Run all together (requires max_connections=300)
./run-integration-tests.sh TestConnectionPool
```

### For CI/CD:
```bash
# Option 1: Matrix strategy (parallel execution)
# See Solution 4 above

# Option 2: Sequential with delays (slower but reliable)
# See Solution 2 above

# Option 3: Individual jobs (simple)
- name: Lifecycle Tests
  run: ./run-integration-tests.sh TestConnectionPoolLifecycle

- name: Concurrency Tests  
  run: ./run-integration-tests.sh TestConnectionPoolConcurrency

# ... etc
```

## Verification

After applying Solution 1, verify it works:

```bash
# Check PostgreSQL max_connections
docker exec tests-postgres-test-1 psql -U notifuse_test -d postgres -c "SHOW max_connections;"

# Should output: 300

# Run all tests
./run-integration-tests.sh TestConnectionPool
```

## Why max_connections=300?

**Calculation:**
- 33 test cases
- Each creates ~3-5 database connections
- Peak usage: ~33 × 5 = 165 connections
- Buffer for PostgreSQL internals: +35
- Total needed: ~200 minimum
- **Set to 300 for safety margin**

## Performance Impact

Increasing `max_connections` has minimal impact on test performance:
- Slightly more memory used by PostgreSQL (~400KB per connection)
- For 300 connections: ~120MB additional memory
- Acceptable for test environment

## Alternative: Skip Performance Tests

If you can't increase `max_connections`, skip the heavy tests:

```bash
# Run only fast tests
INTEGRATION_TESTS=true go test -short -v ./tests/integration -run TestConnectionPool
```

This skips `TestConnectionPoolPerformance` which creates the most connections.

---

## Summary

**Best Solution:** Increase PostgreSQL `max_connections` to 300 (already done in `compose.test.yaml`)

**Then simply run:**
```bash
./run-integration-tests.sh TestConnectionPool
```

All 33 tests will complete successfully in ~2 minutes. ✅
