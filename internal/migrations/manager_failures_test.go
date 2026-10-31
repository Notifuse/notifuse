package migrations

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/database"
)

func funcPointer(fn interface{}) uintptr { return reflect.ValueOf(fn).Pointer() }

func TestManager_DefaultSeams(t *testing.T) {
	m := NewManager(newRecordingLogger())
	assert.Equal(t, funcPointer(database.WithWorkspaceDatabaseLock), funcPointer(m.lockWorkspaceDatabase()))
	assert.Equal(t, funcPointer(database.WithMigrationLock), funcPointer(m.lockMigrations()))
	assert.Equal(t, funcPointer(database.WithSharedMigrationLock), funcPointer(m.shareMigrationLock()))
	assert.Equal(t, funcPointer(database.InitializeWorkspaceDatabaseContext), funcPointer(m.initializer()))

	codeVersion, err := GetCurrentCodeVersion()
	require.NoError(t, err)
	got, err := m.currentCodeVersion()
	require.NoError(t, err)
	assert.Equal(t, codeVersion, got)
}

func TestOrDefault(t *testing.T) {
	assert.Equal(t, 2*time.Second, orDefault(2*time.Second, time.Second))
	assert.Equal(t, time.Second, orDefault(0, time.Second))
}

func TestManager_RunMigrations_EarlyFailures(t *testing.T) {
	codeVersion, err := GetCurrentCodeVersion()
	require.NoError(t, err)

	t.Run("an injected migration lock is the one taken, and its error comes back", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		expectVersionRow(mock, codeVersion)

		m := NewManager(newRecordingLogger())
		refused := errors.New("lock refused")
		var lockedDB *sql.DB
		m.migrationLock = func(ctx context.Context, d *sql.DB, waiting func([]database.MigrationLockHolder, error), fn func(context.Context) error) error {
			lockedDB = d
			return refused
		}
		assert.ErrorIs(t, m.RunMigrations(context.Background(), &config.Config{}, db), refused)
		assert.Same(t, db, lockedDB)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("the injected code version is the one compared under the lock", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		expectVersionRow(mock, 40)
		expectMigrationLockTaken(mock)
		expectVersionRow(mock, 40) // the injected build's: up to date, nothing to migrate
		mock.ExpectCommit()

		m := NewManager(newRecordingLogger())
		m.codeVersion = func() (float64, error) { return 40, nil }
		require.NoError(t, m.RunMigrations(context.Background(), &config.Config{}, db))
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("a code version that does not parse stops before the lock", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		expectVersionRow(mock, codeVersion)

		m := NewManager(newRecordingLogger())
		m.codeVersion = func() (float64, error) { return 0, errors.New("bad version") }
		m.migrationLock = func(context.Context, *sql.DB, func([]database.MigrationLockHolder, error), func(context.Context) error) error {
			t.Fatal("the lock must not be tried")
			return nil
		}
		err = m.RunMigrations(context.Background(), &config.Config{}, db)
		assert.EqualError(t, err, "failed to get current code version: bad version")
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestManager_LogMigrationLockWait_LookupError(t *testing.T) {
	log := newRecordingLogger()
	NewManager(log).logMigrationLockWait(nil, errors.New("permission denied for pg_stat_activity"))

	warns := log.at("warn")
	require.Len(t, warns, 1)
	assert.Equal(t, "permission denied for pg_stat_activity", warns[0].fields["error"])
	assert.NotContains(t, warns[0].fields, "holders")
}

// migrationTx returns a transaction on a sqlmock system database, and the mock.
func migrationTx(t *testing.T) (*sql.Tx, sqlmock.Sqlmock) {
	t.Helper()
	sys, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = sys.Close()
		assert.NoError(t, mock.ExpectationsWereMet())
	})
	mock.ExpectBegin()
	tx, err := sys.Begin()
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	return tx, mock
}

// failingTablesDB returns a workspace database whose tables check fails, then must be
// closed.
func failingTablesDB(t *testing.T) *sql.DB {
	t.Helper()
	db, mock := workspaceDB(t)
	mock.ExpectQuery(`SELECT EXISTS \(\s*SELECT 1\s*FROM pg_class c`).WillReturnError(errors.New("relation lookup failed"))
	mock.ExpectClose()
	return db
}

func TestManager_ConnectForMigration_Failures(t *testing.T) {
	cfg := emptyWorkspaceTestConfig()
	ctx := context.Background()

	t.Run("a tables check that fails closes the database", func(t *testing.T) {
		tx, _ := migrationTx(t)
		conn := newScriptedConnector(t)
		conn.script("ws1", connectorAnswer{db: failingTablesDB(t)})
		m := NewManager(newRecordingLogger())
		m.connector = conn

		db, migrate, err := m.connectForMigration(ctx, cfg, tx, "ws1")
		assert.ErrorContains(t, err, "failed to inspect workspace database ws1: ")
		assert.ErrorContains(t, err, "relation lookup failed")
		assert.Nil(t, db)
		assert.False(t, migrate)
	})

	t.Run("a database initialised meanwhile that cannot be reached again", func(t *testing.T) {
		tx, sysMock := migrationTx(t)
		expectRegisteredInTx(sysMock, "ws1", true)
		lock := newFakeNameLock(t)
		lock.expectDatabaseExists("notifuse_ws_ws1", true)
		conn := newScriptedConnector(t)
		conn.script("ws1",
			connectorAnswer{err: missingDatabase},
			connectorAnswer{db: emptyDB(t, true)},
			connectorAnswer{err: errors.New("too many clients")})
		m := NewManager(newRecordingLogger())
		m.connector = conn
		m.workspaceLock = lock.lock

		db, migrate, err := m.connectForMigration(ctx, cfg, tx, "ws1")
		assert.EqualError(t, err, "failed to connect to workspace database ws1: too many clients")
		assert.Nil(t, db)
		assert.False(t, migrate)
		assert.Equal(t, []string{"notifuse_ws_ws1"}, lock.names)
	})
}

func TestManager_LookAgainUnderNameLock_Failures(t *testing.T) {
	cfg := emptyWorkspaceTestConfig()
	ctx := context.Background()

	t.Run("an ID that names no database", func(t *testing.T) {
		tx, _ := migrationTx(t)
		lock := newFakeNameLock(t)
		m := NewManager(newRecordingLogger())
		m.workspaceLock = lock.lock

		migrate, err := m.lookAgainUnderNameLock(ctx, cfg, tx, "bad id")
		assert.ErrorContains(t, err, "invalid workspace ID for a database name")
		assert.False(t, migrate)
		assert.Empty(t, lock.names, "no lock is taken")
	})

	t.Run("the registration check fails", func(t *testing.T) {
		tx, sysMock := migrationTx(t)
		sysMock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM workspaces WHERE id = \$1\)`).WillReturnError(errors.New("system db down"))
		lock := newFakeNameLock(t)
		m := NewManager(newRecordingLogger())
		m.workspaceLock = lock.lock

		migrate, err := m.lookAgainUnderNameLock(ctx, cfg, tx, "ws1")
		assert.EqualError(t, err, "failed to check workspace ws1: system db down")
		assert.False(t, migrate)
	})

	t.Run("the database check fails", func(t *testing.T) {
		tx, sysMock := migrationTx(t)
		expectRegisteredInTx(sysMock, "ws1", true)
		lock := newFakeNameLock(t)
		lock.mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM pg_database WHERE datname = \$1\)`).WillReturnError(errors.New("admin db down"))
		m := NewManager(newRecordingLogger())
		m.workspaceLock = lock.lock

		migrate, err := m.lookAgainUnderNameLock(ctx, cfg, tx, "ws1")
		assert.ErrorContains(t, err, "failed to check whether database notifuse_ws_ws1 exists: admin db down")
		assert.False(t, migrate)
	})

	t.Run("the database cannot be reached", func(t *testing.T) {
		tx, sysMock := migrationTx(t)
		expectRegisteredInTx(sysMock, "ws1", true)
		lock := newFakeNameLock(t)
		lock.expectDatabaseExists("notifuse_ws_ws1", true)
		conn := newScriptedConnector(t)
		conn.script("ws1", connectorAnswer{err: errors.New("too many clients")})
		m := NewManager(newRecordingLogger())
		m.workspaceLock = lock.lock
		m.connector = conn

		migrate, err := m.lookAgainUnderNameLock(ctx, cfg, tx, "ws1")
		assert.EqualError(t, err, "failed to connect to workspace database ws1: too many clients")
		assert.False(t, migrate)
	})

	t.Run("the tables check fails", func(t *testing.T) {
		tx, sysMock := migrationTx(t)
		expectRegisteredInTx(sysMock, "ws1", true)
		lock := newFakeNameLock(t)
		lock.expectDatabaseExists("notifuse_ws_ws1", true)
		conn := newScriptedConnector(t)
		conn.script("ws1", connectorAnswer{db: failingTablesDB(t)})
		m := NewManager(newRecordingLogger())
		m.workspaceLock = lock.lock
		m.connector = conn

		migrate, err := m.lookAgainUnderNameLock(ctx, cfg, tx, "ws1")
		assert.ErrorContains(t, err, "failed to inspect workspace database ws1: ")
		assert.False(t, migrate)
	})
}

func TestManager_InitializeEmptyWorkspaces_Failures(t *testing.T) {
	ctx := context.Background()

	t.Run("a code version that does not parse runs nothing", func(t *testing.T) {
		f := newRepairFixture(t)
		f.manager.codeVersion = func() (float64, error) { return 0, errors.New("bad version") }

		f.run(ctx)
		errs := f.log.at("error")
		require.Len(t, errs, 1)
		assert.Equal(t, "Empty workspace repair not run: failed to get the code version", errs[0].msg)
		assert.Equal(t, "bad version", errs[0].fields["error"])
		assert.Zero(t, f.sharedTaken)
	})

	t.Run("a version read that fails under the shared lock skips the workspace", func(t *testing.T) {
		f := newRepairFixture(t)
		conn := newScriptedConnector(t)
		conn.script("a", connectorAnswer{db: emptyDB(t, false)})
		conn.script("b", connectorAnswer{db: emptyDB(t, true)})
		f.manager.connector = conn
		f.expectWorkspaces("a", "b")
		f.sysMock.ExpectQuery("SELECT value FROM settings WHERE key = 'db_version'").WillReturnError(errors.New("settings unreadable"))

		f.run(ctx)
		assert.Empty(t, f.initialised)
		errs := f.log.at("error")
		require.Len(t, errs, 1)
		assert.Equal(t, "a", errs[0].fields["workspace"])
		assert.Contains(t, errs[0].fields["error"], "settings unreadable")
		assert.Equal(t, 2, conn.total(), "the next workspace is still probed")
	})

	t.Run("a cut probe of a workspace still registered whose database exists is an error", func(t *testing.T) {
		f := newRepairFixture(t)
		conn := newScriptedConnector(t)
		conn.script("cut", connectorAnswer{err: fmt.Errorf("probe: %w", &pq.Error{Code: "57P01", Message: "terminating connection due to administrator command"})})
		f.manager.connector = conn
		f.expectWorkspaces("cut")
		expectRegisteredInTx(f.sysMock, "cut", true)
		f.nameLock.expectDatabaseExists("notifuse_ws_cut", true)

		f.run(ctx)
		assert.Empty(t, f.log.at("warn"))
		errs := f.log.at("error")
		require.Len(t, errs, 1)
		assert.Equal(t, "Failed to check or initialise the workspace database; skipped", errs[0].msg)
		assert.Equal(t, "cut", errs[0].fields["workspace"])
		assert.Contains(t, errs[0].fields["error"], "terminating connection")
		assert.NotContains(t, errs[0].fields, "recheck_error")
		assert.Equal(t, []string{"notifuse_ws_cut"}, f.nameLock.names)
	})
}

func TestManager_RepairWorkspace_Failures(t *testing.T) {
	cfg := emptyWorkspaceTestConfig()
	ctx := context.Background()

	newManager := func(t *testing.T) (*Manager, *sql.DB, sqlmock.Sqlmock, *fakeNameLock) {
		sys, sysMock, err := sqlmock.New()
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = sys.Close()
			assert.NoError(t, sysMock.ExpectationsWereMet())
		})
		lock := newFakeNameLock(t)
		m := NewManager(newRecordingLogger())
		m.workspaceLock = lock.lock
		m.initializeWorkspace = func(context.Context, *sql.DB) error {
			t.Fatal("nothing is initialised")
			return nil
		}
		return m, sys, sysMock, lock
	}

	t.Run("an ID that names no database", func(t *testing.T) {
		m, sys, _, lock := newManager(t)
		err := m.repairWorkspace(ctx, cfg, sys, "bad id")
		assert.ErrorContains(t, err, "invalid workspace ID for a database name")
		assert.Empty(t, lock.names)
	})

	t.Run("the registration check fails", func(t *testing.T) {
		m, sys, sysMock, _ := newManager(t)
		sysMock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM workspaces WHERE id = \$1\)`).WillReturnError(errors.New("system db down"))
		err := m.repairWorkspace(ctx, cfg, sys, "ws1")
		assert.EqualError(t, err, "failed to check workspace ws1: system db down")
	})

	t.Run("the database cannot be reached", func(t *testing.T) {
		m, sys, sysMock, lock := newManager(t)
		expectRegisteredInTx(sysMock, "ws1", true)
		lock.expectDatabaseExists("notifuse_ws_ws1", true)
		conn := newScriptedConnector(t)
		conn.script("ws1", connectorAnswer{err: errors.New("too many clients")})
		m.connector = conn

		err := m.repairWorkspace(ctx, cfg, sys, "ws1")
		assert.EqualError(t, err, "failed to connect to workspace database ws1: too many clients")
	})

	t.Run("the tables check fails, and the database is closed", func(t *testing.T) {
		m, sys, sysMock, lock := newManager(t)
		expectRegisteredInTx(sysMock, "ws1", true)
		lock.expectDatabaseExists("notifuse_ws_ws1", true)
		conn := newScriptedConnector(t)
		conn.script("ws1", connectorAnswer{db: failingTablesDB(t)})
		m.connector = conn

		err := m.repairWorkspace(ctx, cfg, sys, "ws1")
		assert.ErrorContains(t, err, "relation lookup failed")
	})
}

func TestManager_WorkspaceDatabaseEmpty_TablesCheckFails(t *testing.T) {
	conn := newScriptedConnector(t)
	conn.script("ws1", connectorAnswer{db: failingTablesDB(t)})
	m := NewManager(newRecordingLogger())
	m.connector = conn

	empty, err := m.workspaceDatabaseEmpty(context.Background(), emptyWorkspaceTestConfig(), "ws1")
	assert.ErrorContains(t, err, "failed to inspect workspace database ws1: ")
	assert.False(t, empty)
}

func TestManager_WorkspaceIDs_RowThatDoesNotScan(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	mock.ExpectQuery(`SELECT id FROM workspaces ORDER BY id`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("a").AddRow(nil))

	ids, err := NewManager(newRecordingLogger()).workspaceIDs(context.Background(), db)
	assert.Error(t, err)
	assert.Nil(t, ids)
	assert.NoError(t, mock.ExpectationsWereMet())
}
