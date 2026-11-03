package integration

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	intdb "github.com/Notifuse/notifuse/internal/database"
	"github.com/Notifuse/notifuse/internal/migrations"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// migrationSystem is a throwaway system database, initialised as at startup.
type migrationSystem struct {
	db  *sql.DB
	cfg *config.Config
}

func newMigrationSystem(t *testing.T, admin *sql.DB) *migrationSystem {
	t.Helper()
	dbCfg := testDatabaseConfig()
	name := dbCfg.Prefix + "_" + uniqueID("migsys")
	createEmptyDatabase(t, admin, name)
	db := openDatabase(t, dbCfg, name)
	require.NoError(t, intdb.InitializeDatabase(db, nil))
	cfg := &config.Config{Database: *dbCfg}
	cfg.Database.DBName = name
	return &migrationSystem{db: db, cfg: cfg}
}

func (s *migrationSystem) setVersion(t *testing.T, version float64) {
	t.Helper()
	_, err := s.db.Exec(`INSERT INTO settings (key, value) VALUES ('db_version', $1)
		ON CONFLICT (key) DO UPDATE SET value = $1`, fmt.Sprintf("%.0f", version))
	require.NoError(t, err)
}

func (s *migrationSystem) version(t *testing.T) string {
	t.Helper()
	var v string
	require.NoError(t, s.db.QueryRow(`SELECT value FROM settings WHERE key = 'db_version'`).Scan(&v))
	return v
}

func (s *migrationSystem) register(t *testing.T, id string) {
	t.Helper()
	_, err := s.db.Exec(`INSERT INTO workspaces (id, name, created_at, updated_at) VALUES ($1, $1, now(), now())`, id)
	require.NoError(t, err)
}

func (s *migrationSystem) run(ctx context.Context) error {
	return migrations.NewManager(logger.NewLoggerWithLevel("disabled")).RunMigrations(ctx, s.cfg, s.db)
}

// versionBelowNewestWorkspaceMigration returns the database version that selects the
// newest registered migration with a workspace step. Fails when there is none, so a
// later system-only major cannot empty these tests silently.
func versionBelowNewestWorkspaceMigration(t *testing.T) float64 {
	t.Helper()
	codeVersion, err := migrations.GetCurrentCodeVersion()
	require.NoError(t, err)
	newest := 0.0
	for _, m := range migrations.GetRegisteredMigrations() {
		if m.HasWorkspaceUpdate() && m.GetMajorVersion() <= codeVersion && m.GetMajorVersion() > newest {
			newest = m.GetMajorVersion()
		}
	}
	require.NotZero(t, newest, "no registered migration with a workspace step")
	return newest - 1
}

func TestMigrationEmptyWorkspace(t *testing.T) {
	testutil.SkipIfShort(t)
	admin := openAdmin(t, testDatabaseConfig())
	codeVersion, err := migrations.GetCurrentCodeVersion()
	require.NoError(t, err)
	below := versionBelowNewestWorkspaceMigration(t)

	t.Run("an empty workspace database is skipped, the others migrated, the version stamped", func(t *testing.T) {
		sys := newMigrationSystem(t, admin)
		dbCfg := &sys.cfg.Database

		empty := uniqueID("mempty")
		createEmptyDatabase(t, admin, dbCfg.Prefix+"_ws_"+empty)
		sys.register(t, empty)

		full := uniqueID("mfull")
		dropDatabaseAtCleanup(t, admin, dbCfg.Prefix+"_ws_"+full)
		sys.register(t, full)
		require.NoError(t, intdb.EnsureWorkspaceDatabaseExists(context.Background(), dbCfg, full, nil))

		sys.setVersion(t, below)
		require.NoError(t, sys.run(context.Background()))
		assert.Equal(t, fmt.Sprintf("%.0f", codeVersion), sys.version(t))

		holds, err := intdbHoldsTables(t, dbCfg, dbCfg.Prefix+"_ws_"+empty)
		require.NoError(t, err)
		assert.False(t, holds, "migrations never build a schema")
	})

	t.Run("an empty workspace being initialised elsewhere is waited for, then migrated", func(t *testing.T) {
		sys := newMigrationSystem(t, admin)
		dbCfg := &sys.cfg.Database
		id := uniqueID("mwait")
		dbName := dbCfg.Prefix + "_ws_" + id
		createEmptyDatabase(t, admin, dbName)
		sys.register(t, id)
		sys.setVersion(t, below)

		release := holdNameLock(t, dbCfg, dbName)
		done := make(chan error, 1)
		go func() { done <- sys.run(context.Background()) }()
		waitForNameLockWaiters(t, admin, dbName, 1)

		// The initialisation another instance commits while it holds the lock.
		require.NoError(t, intdb.InitializeWorkspaceDatabase(openDatabase(t, dbCfg, dbName)))
		require.NoError(t, release())

		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(barrierTimeout):
			require.FailNow(t, "RunMigrations never returned")
		}
		assert.Equal(t, fmt.Sprintf("%.0f", codeVersion), sys.version(t))
	})

	t.Run("a registered workspace with no database stops the migration with what to do", func(t *testing.T) {
		sys := newMigrationSystem(t, admin)
		dbCfg := &sys.cfg.Database
		id := uniqueID("mgone")
		dbName := dbCfg.Prefix + "_ws_" + id
		sys.register(t, id)
		sys.setVersion(t, below)

		err := sys.run(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), fmt.Sprintf("workspace %s is registered but its database %s does not exist: if DB_PREFIX changed, restore it; otherwise create an empty database named %s owned by %s", id, dbName, dbName, dbCfg.User))
		assert.False(t, databaseExists(t, admin, dbName), "the database is never created at startup")
		assert.Equal(t, fmt.Sprintf("%.0f", below), sys.version(t))
	})
}

func (s *migrationSystem) repair(ctx context.Context) {
	migrations.NewManager(logger.NewLoggerWithLevel("disabled")).InitializeEmptyWorkspaces(ctx, s.cfg, s.db)
}

// migrationLockCount counts the sessions on the migration lock of s's system database,
// granted or waiting.
func (s *migrationSystem) migrationLockCount(t *testing.T, granted bool) int {
	t.Helper()
	var n int
	require.NoError(t, s.db.QueryRow(`
		SELECT count(*) FROM pg_locks
		WHERE locktype = 'advisory'
		AND database = (SELECT oid FROM pg_database WHERE datname = current_database())
		AND objsubid = 1
		AND classid = ($1::bigint >> 32)::oid
		AND objid = ($1::bigint & 4294967295)::oid
		AND granted = $2`, intdb.MigrationLockKey, granted).Scan(&n))
	return n
}

func (s *migrationSystem) waitForMigrationLockWaiters(t *testing.T, n int) {
	t.Helper()
	deadline := time.Now().Add(barrierTimeout)
	for s.migrationLockCount(t, false) < n {
		if time.Now().After(deadline) {
			require.FailNowf(t, "barrier timeout", "%d session(s) never waited on the migration lock", n)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// holdMigrationLock takes the migration lock exclusively, as another instance's
// migrations would, until release is called.
func (s *migrationSystem) holdMigrationLock(t *testing.T) (release func()) {
	t.Helper()
	tx, err := s.db.Begin()
	require.NoError(t, err)
	_, err = tx.Exec(`SELECT pg_advisory_xact_lock($1)`, intdb.MigrationLockKey)
	require.NoError(t, err)
	released := false
	release = func() {
		if !released {
			released = true
			require.NoError(t, tx.Commit())
		}
	}
	t.Cleanup(func() {
		if !released {
			_ = tx.Rollback()
		}
	})
	return release
}

func TestMigrationEmptyWorkspaceRepair(t *testing.T) {
	testutil.SkipIfShort(t)
	admin := openAdmin(t, testDatabaseConfig())
	codeVersion, err := migrations.GetCurrentCodeVersion()
	require.NoError(t, err)
	reference := referenceSchemaRelationCount(t, testDatabaseConfig(), admin)

	t.Run("at the current version: an empty database gets its schema, a missing one is not created", func(t *testing.T) {
		sys := newMigrationSystem(t, admin)
		dbCfg := &sys.cfg.Database
		empty := uniqueID("rempty")
		createEmptyDatabase(t, admin, dbCfg.Prefix+"_ws_"+empty)
		sys.register(t, empty)
		gone := uniqueID("rgone")
		dropDatabaseAtCleanup(t, admin, dbCfg.Prefix+"_ws_"+gone)
		sys.register(t, gone)
		sys.setVersion(t, codeVersion)

		require.NoError(t, sys.run(context.Background()))
		sys.repair(context.Background())

		assert.Equal(t, reference, relationCount(t, openDatabase(t, dbCfg, dbCfg.Prefix+"_ws_"+empty)))
		assert.False(t, databaseExists(t, admin, dbCfg.Prefix+"_ws_"+gone), "the repair never creates a database")
	})

	t.Run("one version below: the migration skips the empty database, the repair then builds it", func(t *testing.T) {
		sys := newMigrationSystem(t, admin)
		dbCfg := &sys.cfg.Database
		id := uniqueID("rbelow")
		dbName := dbCfg.Prefix + "_ws_" + id
		createEmptyDatabase(t, admin, dbName)
		sys.register(t, id)
		sys.setVersion(t, versionBelowNewestWorkspaceMigration(t))

		require.NoError(t, sys.run(context.Background()))
		assert.Equal(t, 0, relationCount(t, openDatabase(t, dbCfg, dbName)), "migrations never build a schema")
		sys.repair(context.Background())
		assert.Equal(t, reference, relationCount(t, openDatabase(t, dbCfg, dbName)))
	})

	t.Run("an instance that waited restarts when the holder applied a migration requiring one", func(t *testing.T) {
		var restartVersion float64
		for _, migration := range migrations.GetRegisteredMigrations() {
			if migration.ShouldRestartServer() && migration.GetMajorVersion() <= codeVersion {
				restartVersion = migration.GetMajorVersion()
			}
		}
		require.NotZero(t, restartVersion, "a registered migration requires a restart")
		sys := newMigrationSystem(t, admin)
		sys.setVersion(t, restartVersion-1)

		release := sys.holdMigrationLock(t)
		migrated := make(chan error, 1)
		go func() { migrated <- sys.run(context.Background()) }()
		sys.waitForMigrationLockWaiters(t, 1)
		sys.setVersion(t, codeVersion) // what the holder's migrations leave
		release()
		select {
		case err := <-migrated:
			assert.ErrorIs(t, err, migrations.ErrRestartRequired)
		case <-time.After(barrierTimeout):
			require.FailNow(t, "RunMigrations never returned")
		}
	})

	t.Run("migrations and the repair both wait for another instance's migrations", func(t *testing.T) {
		sys := newMigrationSystem(t, admin)
		dbCfg := &sys.cfg.Database
		id := uniqueID("rwait")
		dbName := dbCfg.Prefix + "_ws_" + id
		createEmptyDatabase(t, admin, dbName)
		sys.register(t, id)
		sys.setVersion(t, codeVersion)

		release := sys.holdMigrationLock(t)
		migrated := make(chan error, 1)
		go func() { migrated <- sys.run(context.Background()) }()
		repaired := make(chan struct{})
		go func() { sys.repair(context.Background()); close(repaired) }()
		sys.waitForMigrationLockWaiters(t, 2)
		assert.Equal(t, 0, relationCount(t, openDatabase(t, dbCfg, dbName)), "nothing ran while the lock was held")

		release()
		select {
		case err := <-migrated:
			require.NoError(t, err)
		case <-time.After(barrierTimeout):
			require.FailNow(t, "RunMigrations never returned")
		}
		select {
		case <-repaired:
		case <-time.After(barrierTimeout):
			require.FailNow(t, "the repair never returned")
		}
		assert.Equal(t, reference, relationCount(t, openDatabase(t, dbCfg, dbName)))
	})

	t.Run("migrations wait for a repair in progress, until its initialisation has committed", func(t *testing.T) {
		sys := newMigrationSystem(t, admin)
		dbCfg := &sys.cfg.Database
		id := uniqueID("rhold")
		dbName := dbCfg.Prefix + "_ws_" + id
		createEmptyDatabase(t, admin, dbName)
		sys.register(t, id)
		sys.setVersion(t, codeVersion)

		// The repair stops on the workspace's name lock, holding the shared migration lock.
		releaseName := holdNameLock(t, dbCfg, dbName)
		repaired := make(chan struct{})
		go func() { sys.repair(context.Background()); close(repaired) }()
		waitForNameLockWaiters(t, admin, dbName, 1)
		require.Equal(t, 1, sys.migrationLockCount(t, true), "the repair holds the shared migration lock")

		migrated := make(chan error, 1)
		go func() { migrated <- sys.run(context.Background()) }()
		sys.waitForMigrationLockWaiters(t, 1)

		require.NoError(t, releaseName())
		select {
		case err := <-migrated:
			require.NoError(t, err)
		case <-time.After(barrierTimeout):
			require.FailNow(t, "RunMigrations never returned")
		}
		assert.Equal(t, reference, relationCount(t, openDatabase(t, dbCfg, dbName)), "the migrations ran only once the repair committed")
		select {
		case <-repaired:
		case <-time.After(barrierTimeout):
			require.FailNow(t, "the repair never returned")
		}
	})

	t.Run("a database version above the code's: the repair initialises nothing", func(t *testing.T) {
		sys := newMigrationSystem(t, admin)
		dbCfg := &sys.cfg.Database
		id := uniqueID("rahead")
		dbName := dbCfg.Prefix + "_ws_" + id
		createEmptyDatabase(t, admin, dbName)
		sys.register(t, id)
		sys.setVersion(t, codeVersion+1)

		sys.repair(context.Background())
		assert.Equal(t, 0, relationCount(t, openDatabase(t, dbCfg, dbName)))
	})
}
