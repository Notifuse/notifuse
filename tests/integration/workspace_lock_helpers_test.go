package integration

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	intdb "github.com/Notifuse/notifuse/internal/database"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// barrierTimeout bounds every wait on a lock in these tests: a lock that is not taken
// where it should be otherwise shows up as a hang.
const barrierTimeout = 30 * time.Second

func openDatabase(t *testing.T, cfg *config.DatabaseConfig, dbName string) *sql.DB {
	t.Helper()
	db, err := sql.Open("postgres", dsnFor(cfg, dbName))
	require.NoError(t, err)
	require.NoError(t, db.Ping())
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func openAdmin(t *testing.T, cfg *config.DatabaseConfig) *sql.DB {
	t.Helper()
	return openDatabase(t, cfg, "postgres")
}

// uniqueID returns a workspace ID no other test uses.
func uniqueID(prefix string) string {
	return prefix + strings.ReplaceAll(uuid.New().String(), "-", "")[:10]
}

// createEmptyDatabase creates dbName and drops it when the test ends.
func createEmptyDatabase(t *testing.T, admin *sql.DB, dbName string) {
	t.Helper()
	_, err := admin.Exec("CREATE DATABASE " + pq.QuoteIdentifier(dbName))
	require.NoError(t, err)
	dropDatabaseAtCleanup(t, admin, dbName)
}

func dropDatabaseAtCleanup(t *testing.T, admin *sql.DB, dbName string) {
	t.Helper()
	t.Cleanup(func() {
		if _, err := admin.Exec("DROP DATABASE IF EXISTS " + pq.QuoteIdentifier(dbName) + " WITH (FORCE)"); err != nil {
			t.Logf("dropping %s: %v", dbName, err)
		}
	})
}

func databaseExists(t *testing.T, admin *sql.DB, dbName string) bool {
	t.Helper()
	var exists bool
	require.NoError(t, admin.QueryRow(`SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)`, dbName).Scan(&exists))
	return exists
}

// nameLockCount counts the name locks on dbName, granted or waiting. Every suite shares
// the postgres database, so the lock is always matched on its two keys.
func nameLockCount(t *testing.T, admin *sql.DB, dbName string, granted bool) int {
	t.Helper()
	n, err := queryNameLockCount(admin, dbName, granted)
	require.NoError(t, err)
	return n
}

func queryNameLockCount(admin *sql.DB, dbName string, granted bool) (int, error) {
	var n int
	err := admin.QueryRow(`
		SELECT count(*) FROM pg_locks
		WHERE locktype = 'advisory'
		AND database = (SELECT oid FROM pg_database WHERE datname = 'postgres')
		AND objsubid = 2
		AND classid = $1::bigint::oid
		AND objid = (hashtext($2)::bigint & 4294967295)::oid
		AND granted = $3`, int64(intdb.WorkspaceDatabaseLockClass), dbName, granted).Scan(&n)
	return n, err
}

// waitForNameLockWaiters waits until n sessions wait on dbName's name lock.
func waitForNameLockWaiters(t *testing.T, admin *sql.DB, dbName string, n int) {
	t.Helper()
	deadline := time.Now().Add(barrierTimeout)
	for {
		if got := nameLockCount(t, admin, dbName, false); got >= n {
			return
		}
		if time.Now().After(deadline) {
			require.FailNowf(t, "barrier timeout", "%d session(s) never waited on the name lock of %s", n, dbName)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func waitForNameLockReleased(t *testing.T, admin *sql.DB, dbName string) {
	t.Helper()
	deadline := time.Now().Add(barrierTimeout)
	for nameLockCount(t, admin, dbName, true) > 0 {
		if time.Now().After(deadline) {
			require.FailNowf(t, "barrier timeout", "the name lock of %s was never released", dbName)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// holdNameLock takes dbName's name lock through WithWorkspaceDatabaseLock and holds it
// until release is called. release returns WithWorkspaceDatabaseLock's result.
func holdNameLock(t *testing.T, cfg *config.DatabaseConfig, dbName string) (release func() error) {
	t.Helper()
	acquired := make(chan struct{})
	releaseCh := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- intdb.WithWorkspaceDatabaseLock(context.Background(), cfg, dbName, func(ctx context.Context, admin *sql.DB) error {
			close(acquired)
			<-releaseCh
			return nil
		})
	}()
	select {
	case <-acquired:
	case err := <-done:
		require.FailNowf(t, "holding the name lock", "%v", err)
	case <-time.After(barrierTimeout):
		require.FailNow(t, "barrier timeout taking the name lock")
	}
	released := false
	release = func() error {
		if released {
			return nil
		}
		released = true
		close(releaseCh)
		select {
		case err := <-done:
			return err
		case <-time.After(barrierTimeout):
			return fmt.Errorf("barrier timeout releasing the name lock of %s", dbName)
		}
	}
	t.Cleanup(func() { _ = release() })
	return release
}

// relationCount counts the relations a workspace schema check would see.
func relationCount(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	require.NoError(t, db.QueryRow(`
		SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname NOT LIKE 'pg\_%' AND n.nspname <> 'information_schema'`).Scan(&n))
	return n
}

// referenceSchemaRelationCount is the relation count of a freshly initialised workspace
// database, to tell a complete schema from a partial one.
func referenceSchemaRelationCount(t *testing.T, cfg *config.DatabaseConfig, admin *sql.DB) int {
	t.Helper()
	name := cfg.Prefix + "_ws_" + uniqueID("reference")
	createEmptyDatabase(t, admin, name)
	db := openDatabase(t, cfg, name)
	require.NoError(t, intdb.InitializeWorkspaceDatabase(db))
	return relationCount(t, db)
}

func testDatabaseConfig() *config.DatabaseConfig {
	return testutil.GetTestDatabaseConfig()
}

func quoteIdent(name string) string { return pq.QuoteIdentifier(name) }

func intdbHoldsTables(t *testing.T, cfg *config.DatabaseConfig, dbName string) (bool, error) {
	t.Helper()
	return intdb.WorkspaceDatabaseHoldsTables(context.Background(), openDatabase(t, cfg, dbName))
}
