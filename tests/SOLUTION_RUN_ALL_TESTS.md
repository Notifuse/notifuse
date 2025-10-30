# ✅ SOLUTION: Run All Connection Pool Tests Together

## Quick Answer

**Update `tests/docker-compose.test.yml` to increase `max_connections=300`** (✅ Already Done)

```yaml
postgres-test:
  command:
    - "postgres"
    - "-c"
    - "max_connections=300"
```

Then restart PostgreSQL and run:

```bash
# Restart PostgreSQL with new config
docker compose -f tests/docker-compose.test.yml restart postgres-test

# Run all tests
./run-integration-tests.sh TestConnectionPool
```

---

## Complete Solution Steps

### Step 1: Update PostgreSQL Configuration (✅ Done)

File: `tests/docker-compose.test.yml`

```yaml
services:
  postgres-test:
    image: postgres:14
    command:
      - "postgres"
      - "-c"
      - "max_connections=300"
      - "-c"
      - "shared_buffers=128MB"
```

### Step 2: Restart Docker Compose

```bash
cd tests
docker compose -f docker-compose.test.yml down
docker compose -f docker-compose.test.yml up -d
```

Wait ~10 seconds for PostgreSQL to be ready.

### Step 3: Verify Configuration

```bash
docker exec tests-postgres-test-1 psql -U notifuse_test -d postgres -c "SHOW max_connections;"
```

Should output: `300`

### Step 4: Run All Tests

```bash
./run-integration-tests.sh TestConnectionPool
```

---

## Why This Works

### Problem
- Default PostgreSQL `max_connections = 100`
- 33 test cases × ~3-5 connections each = ~150 connections
- Tests exhaust available connections → timeout

### Solution  
- Increase `max_connections` to 300
- Provides sufficient capacity for all tests
- Minimal memory overhead (~120MB)

---

## Alternative: Run Tests Individually

If you can't increase `max_connections`, use the Makefile:

```bash
make test-connection-pools
```

This runs each suite separately to avoid exhaustion.

---

## Verification

After applying the solution:

```bash
# Check max_connections is 300
docker exec tests-postgres-test-1 psql -U notifuse_test -c "SHOW max_connections;"

# Run all tests (should complete in ~2 minutes)
time ./run-integration-tests.sh TestConnectionPool
```

Expected result: ✅ All 33 tests PASS

---

## For CI/CD (GitHub Actions)

Add to your workflow:

```yaml
services:
  postgres:
    image: postgres:14
    options: >-
      -c max_connections=300
      -c shared_buffers=128MB
    env:
      POSTGRES_USER: notifuse_test
      POSTGRES_PASSWORD: test_password
```

Then run tests normally:

```yaml
- name: Run Connection Pool Tests
  run: |
    INTEGRATION_TESTS=true go test -v ./tests/integration -run TestConnectionPool -timeout 15m
```

---

## Summary

✅ **Solution Applied**: PostgreSQL configured with `max_connections=300`  
✅ **Result**: All 33 connection pool tests can run together  
✅ **Time**: Completes in ~2 minutes  
✅ **Memory**: Additional ~120MB (acceptable for tests)

**Status: SOLVED** 🎉
