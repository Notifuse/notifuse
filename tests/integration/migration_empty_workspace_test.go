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
