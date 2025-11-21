package testutil

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/stretchr/testify/require"
)

// VerifyNoLeakedConnections queries PostgreSQL for leaked connections
// This should be called after cleaning up a test to ensure no connections remain
func VerifyNoLeakedConnections(t *testing.T, systemDB *sql.DB, testUser string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var count int
	query := `
		SELECT COUNT(*) 
		FROM pg_stat_activity 
		WHERE usename = $1 
		  AND pid != pg_backend_pid()
	`

	err := systemDB.QueryRowContext(ctx, query, testUser).Scan(&count)
	require.NoError(t, err, "Failed to query connection count")

	if count > 0 {
		// Get details about leaked connections
		detailsQuery := `
			SELECT pid, datname, application_name, state, query_start, state_change
			FROM pg_stat_activity 
			WHERE usename = $1 
			  AND pid != pg_backend_pid()
		`

		rows, err := systemDB.QueryContext(ctx, detailsQuery, testUser)
		require.NoError(t, err, "Failed to query leaked connection details")
		defer rows.Close()

		t.Errorf("LEAK DETECTED: %d connections remain active for user %s", count, testUser)
		for rows.Next() {
			var pid int
			var datname, app, state string
			var queryStart, stateChange time.Time
			if err := rows.Scan(&pid, &datname, &app, &state, &queryStart, &stateChange); err == nil {
				t.Logf("  - PID %d: DB=%s, App=%s, State=%s, Started=%v",
					pid, datname, app, state, queryStart)
			}
		}

		t.FailNow()
	}
}

// WaitForConnectionClose waits for a connection to be closed with timeout
func WaitForConnectionClose(t *testing.T, systemDB *sql.DB, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		// Try to ping - if it fails, connection is closed
		if err := systemDB.Ping(); err != nil {
			return // Connection is closed
		}
		time.Sleep(50 * time.Millisecond)
	}

	t.Fatalf("Connection did not close within %v", timeout)
}

// GetActiveConnectionCount returns current connection count for a user
func GetActiveConnectionCount(t *testing.T, systemDB *sql.DB, user string) int {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var count int
	query := `
		SELECT COUNT(*) 
		FROM pg_stat_activity 
		WHERE usename = $1 
		  AND pid != pg_backend_pid()
	`

	err := systemDB.QueryRowContext(ctx, query, user).Scan(&count)
	require.NoError(t, err, "Failed to query connection count")

	return count
}

// CreateTestWorkspaces creates N test workspaces and returns their IDs
func CreateTestWorkspaces(t *testing.T, pool *TestConnectionPool, count int) []string {
	t.Helper()

	workspaceIDs := make([]string, count)
	for i := 0; i < count; i++ {
		workspaceID := fmt.Sprintf("test_ws_%d_%d", time.Now().UnixNano(), i)
		err := pool.EnsureWorkspaceDatabase(workspaceID)
		require.NoError(t, err, "Failed to create workspace %s", workspaceID)
		workspaceIDs[i] = workspaceID
	}

	return workspaceIDs
}

// CleanupTestWorkspaces removes test workspaces
func CleanupTestWorkspaces(t *testing.T, pool *TestConnectionPool, workspaceIDs []string) {
	t.Helper()

	for _, workspaceID := range workspaceIDs {
		err := pool.CleanupWorkspace(workspaceID)
		if err != nil {
			t.Logf("Warning: failed to cleanup workspace %s: %v", workspaceID, err)
		}
	}
}

// MeasureOperationTime measures and returns operation duration
func MeasureOperationTime(t *testing.T, operation string, fn func()) time.Duration {
	t.Helper()

	start := time.Now()
	fn()
	duration := time.Since(start)
	t.Logf("Operation '%s' took %v", operation, duration)
	return duration
}

// WaitForConditionWithContext waits for a condition to be true within a timeout
func WaitForConditionWithContext(ctx context.Context, t *testing.T, condition func() bool, checkInterval time.Duration, message string) error {
	t.Helper()

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for condition: %s", message)
		case <-ticker.C:
			if condition() {
				return nil
			}
		}
	}
}

// GetTestDatabaseConfig returns a database config for testing
func GetTestDatabaseConfig() *config.DatabaseConfig {
	// Default to localhost for normal environments
	// In containerized environments, set TEST_DB_HOST env var
	defaultHost := "localhost"
	defaultPort := 5433

	// Check if we're likely in a containerized environment
	testHost := getEnvOrDefault("TEST_DB_HOST", defaultHost)
	testPort := defaultPort
	if testHost != defaultHost {
		// If custom host is set, likely need internal port
		if portStr := getEnvOrDefault("TEST_DB_PORT", ""); portStr != "" {
			fmt.Sscanf(portStr, "%d", &testPort)
		} else {
			testPort = 5432 // Default to internal port when using custom host
		}
	}

	cfg := &config.DatabaseConfig{
		Host:     testHost,
		Port:     testPort,
		User:     getEnvOrDefault("TEST_DB_USER", "notifuse_test"),
		Password: getEnvOrDefault("TEST_DB_PASSWORD", "test_password"),
		Prefix:   "notifuse_test",
		SSLMode:  "disable",
	}

	// Debug logging to troubleshoot connection issues
	if os.Getenv("DEBUG_TEST_CONFIG") == "true" {
		fmt.Printf("[DEBUG] Test DB Config: host=%s port=%d user=%s\n", cfg.Host, cfg.Port, cfg.User)
	}

	return cfg
}

// TerminateAllConnections terminates all connections to a database (for testing)
func TerminateAllConnections(t *testing.T, systemDB *sql.DB, dbName string) error {
	t.Helper()

	query := fmt.Sprintf(`
		SELECT pg_terminate_backend(pid) 
		FROM pg_stat_activity 
		WHERE datname = '%s' 
		  AND pid <> pg_backend_pid()
	`, dbName)

	_, err := systemDB.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to terminate connections to %s: %w", dbName, err)
	}

	// Wait for connections to actually close
	time.Sleep(100 * time.Millisecond)

	return nil
}

// GetDatabaseConnectionStats returns detailed connection statistics for a database
func GetDatabaseConnectionStats(t *testing.T, systemDB *sql.DB, dbName string) (int, error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var count int
	query := `
		SELECT COUNT(*) 
		FROM pg_stat_activity 
		WHERE datname = $1 
		  AND pid != pg_backend_pid()
	`

	err := systemDB.QueryRowContext(ctx, query, dbName).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to query connection count for %s: %w", dbName, err)
	}

	return count, nil
}

// WaitForDatabaseReady waits for a database to be ready for connections
func WaitForDatabaseReady(t *testing.T, db *sql.DB, timeout time.Duration) error {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("database not ready within %v", timeout)
		case <-ticker.C:
			if err := db.PingContext(ctx); err == nil {
				return nil
			}
		}
	}
}
