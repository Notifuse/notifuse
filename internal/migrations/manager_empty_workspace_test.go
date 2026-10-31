package migrations

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// recordingMigration is a workspace migration recording the workspaces it migrated.
type recordingMigration struct {
	mockMigration
	migrated []string
}

func (m *recordingMigration) UpdateWorkspace(ctx context.Context, cfg *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	m.migrated = append(m.migrated, workspace.ID)
	return nil
}

// scriptedConnector answers each connection to a workspace with the next scripted
// database or error, and counts the connections.
type scriptedConnector struct {
	t       *testing.T
	answers map[string][]connectorAnswer
	calls   map[string]int
}

type connectorAnswer struct {
	db  *sql.DB
	err error
}

func newScriptedConnector(t *testing.T) *scriptedConnector {
	return &scriptedConnector{t: t, answers: map[string][]connectorAnswer{}, calls: map[string]int{}}
}

func (c *scriptedConnector) script(workspaceID string, answers ...connectorAnswer) {
	c.answers[workspaceID] = append(c.answers[workspaceID], answers...)
}

func (c *scriptedConnector) connectToWorkspace(ctx context.Context, cfg *config.DatabaseConfig, workspaceID string) (*sql.DB, error) {
	i := c.calls[workspaceID]
	c.calls[workspaceID]++
	require.Less(c.t, i, len(c.answers[workspaceID]), "unexpected connection %d to workspace %s", i+1, workspaceID)
	a := c.answers[workspaceID][i]
	return a.db, a.err
}

func (c *scriptedConnector) total() int {
	n := 0
	for _, v := range c.calls {
		n += v
	}
	return n
}

// workspaceDB returns a sqlmock workspace database that must be closed.
func workspaceDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = db.Close()
		assert.NoError(t, mock.ExpectationsWereMet())
	})
	return db, mock
}

// fakeNameLock records the name locks taken and hands fn a sqlmock admin database.
type fakeNameLock struct {
	admin  *sql.DB
	mock   sqlmock.Sqlmock
	names  []string
	inLock bool
}

func newFakeNameLock(t *testing.T) *fakeNameLock {
	admin, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = admin.Close()
		assert.NoError(t, mock.ExpectationsWereMet())
	})
	return &fakeNameLock{admin: admin, mock: mock}
}

func (l *fakeNameLock) lock(ctx context.Context, cfg *config.DatabaseConfig, dbName string, fn func(ctx context.Context, admin *sql.DB) error) error {
	l.names = append(l.names, dbName)
	l.inLock = true
	defer func() { l.inLock = false }()
	return fn(ctx, l.admin)
}

func (l *fakeNameLock) expectDatabaseExists(name string, exists bool) {
	l.mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM pg_database WHERE datname = \$1\)`).WithArgs(name).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(exists))
}

func expectWorkspaceRows(mock sqlmock.Sqlmock, ids ...string) {
	rows := sqlmock.NewRows([]string{"id", "name", "settings", "integrations", "created_at", "updated_at"})
	for _, id := range ids {
		rows.AddRow(id, id, []byte("{}"), []byte("[]"), time.Now(), time.Now())
	}
	mock.ExpectQuery("SELECT id, name, settings, integrations, created_at, updated_at FROM workspaces").WillReturnRows(rows)
}

func expectRegisteredInTx(mock sqlmock.Sqlmock, id string, registered bool) {
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM workspaces WHERE id = \$1\)`).WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(registered))
}

var missingDatabase = fmt.Errorf("failed to ping workspace database: %w", &pq.Error{Code: "3D000", Message: `database "notifuse_ws_gone" does not exist`})

func emptyWorkspaceTestConfig() *config.Config {
	return &config.Config{Database: config.DatabaseConfig{Prefix: "notifuse", User: "notifuse_app"}}
}

func TestManager_EmptyWorkspaceMigration(t *testing.T) {
	ctx := context.Background()
	migration := func() *recordingMigration {
		return &recordingMigration{mockMigration: mockMigration{version: 99, hasWorkspaceUpdate: true}}
	}

	t.Run("an empty workspace is looked at again under its lock and skipped; the others are migrated", func(t *testing.T) {
		sysDB, sysMock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = sysDB.Close() }()
		lock := newFakeNameLock(t)
		conn := newScriptedConnector(t)
		manager := NewManager(&mockLogger{})
		manager.connector = conn
		manager.workspaceLock = lock.lock

		emptyFirst, emptyFirstMock := workspaceDB(t)
		expectWorkspaceHoldsTables(emptyFirstMock, false)
		emptyFirstMock.ExpectClose()
		emptyAgain, emptyAgainMock := workspaceDB(t)
		expectWorkspaceHoldsTables(emptyAgainMock, false)
		emptyAgainMock.ExpectClose()
		conn.script("empty", connectorAnswer{db: emptyFirst}, connectorAnswer{db: emptyAgain})

		full, fullMock := workspaceDB(t)
		expectWorkspaceHoldsTables(fullMock, true)
		fullMock.ExpectBegin()
		fullMock.ExpectCommit()
		fullMock.ExpectClose()
		conn.script("full", connectorAnswer{db: full})

		sysMock.ExpectBegin()
		expectWorkspaceRows(sysMock, "empty", "full")
		expectRegisteredInTx(sysMock, "empty", true)
		lock.expectDatabaseExists("notifuse_ws_empty", true)
		sysMock.ExpectCommit()

		m := migration()
		require.NoError(t, manager.executeMigration(ctx, emptyWorkspaceTestConfig(), sysDB, m))
		assert.Equal(t, []string{"full"}, m.migrated)
		assert.Equal(t, []string{"notifuse_ws_empty"}, lock.names, "no lock for a workspace that holds tables")
		assert.NoError(t, sysMock.ExpectationsWereMet())
	})

	t.Run("empty at first, holding tables inside the lock: migrated", func(t *testing.T) {
		sysDB, sysMock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = sysDB.Close() }()
		lock := newFakeNameLock(t)
		conn := newScriptedConnector(t)
		manager := NewManager(&mockLogger{})
		manager.connector = conn
		manager.workspaceLock = lock.lock

		first, firstMock := workspaceDB(t)
		expectWorkspaceHoldsTables(firstMock, false)
		firstMock.ExpectClose()
		again, againMock := workspaceDB(t)
		expectWorkspaceHoldsTables(againMock, true)
		againMock.ExpectClose()
		migrateDB, migrateMock := workspaceDB(t)
		migrateMock.ExpectBegin()
		migrateMock.ExpectCommit()
		migrateMock.ExpectClose()
		conn.script("ws", connectorAnswer{db: first}, connectorAnswer{db: again}, connectorAnswer{db: migrateDB})

		sysMock.ExpectBegin()
		expectWorkspaceRows(sysMock, "ws")
		expectRegisteredInTx(sysMock, "ws", true)
		lock.expectDatabaseExists("notifuse_ws_ws", true)
		sysMock.ExpectCommit()

		m := migration()
		require.NoError(t, manager.executeMigration(ctx, emptyWorkspaceTestConfig(), sysDB, m))
		assert.Equal(t, []string{"ws"}, m.migrated)
		assert.NoError(t, sysMock.ExpectationsWereMet())
	})

	t.Run("a missing database with its row present fails with what to do, decided inside the lock", func(t *testing.T) {
		sysDB, sysMock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = sysDB.Close() }()
		lock := newFakeNameLock(t)
		conn := newScriptedConnector(t)
		conn.script("gone", connectorAnswer{err: missingDatabase})
		manager := NewManager(&mockLogger{})
		manager.connector = conn
		manager.workspaceLock = func(ctx context.Context, cfg *config.DatabaseConfig, dbName string, fn func(ctx context.Context, admin *sql.DB) error) error {
			return lock.lock(ctx, cfg, dbName, fn)
		}

		sysMock.ExpectBegin()
		expectWorkspaceRows(sysMock, "gone")
		expectRegisteredInTx(sysMock, "gone", true)
		lock.expectDatabaseExists("notifuse_ws_gone", false)
		sysMock.ExpectRollback()

		err = manager.executeMigration(ctx, emptyWorkspaceTestConfig(), sysDB, migration())
		require.Error(t, err)
		assert.Equal(t, "workspace gone is registered but its database notifuse_ws_gone does not exist: if DB_PREFIX changed, restore it; otherwise create an empty database named notifuse_ws_gone owned by notifuse_app (startup skips empty databases) or delete the workspace's rows", err.Error())
		assert.Equal(t, []string{"notifuse_ws_gone"}, lock.names)
		assert.Equal(t, 1, conn.total(), "the database is never created, nor connected to again")
		assert.NoError(t, sysMock.ExpectationsWereMet())
	})

	t.Run("a missing database whose row is gone is skipped", func(t *testing.T) {
		sysDB, sysMock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = sysDB.Close() }()
		lock := newFakeNameLock(t)
		conn := newScriptedConnector(t)
		conn.script("gone", connectorAnswer{err: missingDatabase})
		manager := NewManager(&mockLogger{})
		manager.connector = conn
		manager.workspaceLock = lock.lock

		sysMock.ExpectBegin()
		expectWorkspaceRows(sysMock, "gone")
		expectRegisteredInTx(sysMock, "gone", false)
		sysMock.ExpectCommit()

		m := migration()
		require.NoError(t, manager.executeMigration(ctx, emptyWorkspaceTestConfig(), sysDB, m))
		assert.Empty(t, m.migrated)
		assert.NoError(t, sysMock.ExpectationsWereMet())
	})

	t.Run("another connection error is returned as before, without the lock", func(t *testing.T) {
		sysDB, sysMock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = sysDB.Close() }()
		lock := newFakeNameLock(t)
		conn := newScriptedConnector(t)
		sentinel := errors.New("connection refused")
		conn.script("ws", connectorAnswer{err: sentinel})
		manager := NewManager(&mockLogger{})
		manager.connector = conn
		manager.workspaceLock = lock.lock

		sysMock.ExpectBegin()
		expectWorkspaceRows(sysMock, "ws")
		sysMock.ExpectRollback()

		err = manager.executeMigration(ctx, emptyWorkspaceTestConfig(), sysDB, migration())
		assert.ErrorIs(t, err, sentinel)
		assert.ErrorContains(t, err, "failed to connect to workspace database ws:")
		assert.Empty(t, lock.names)
	})

	t.Run("a system-only migration never connects to a workspace", func(t *testing.T) {
		sysDB, sysMock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = sysDB.Close() }()
		conn := newScriptedConnector(t) // any connection fails the test
		manager := NewManager(&mockLogger{})
		manager.connector = conn

		sysMock.ExpectBegin()
		sysMock.ExpectCommit()

		require.NoError(t, manager.executeMigration(ctx, emptyWorkspaceTestConfig(), sysDB, &mockMigration{version: 99, hasSystemUpdate: true}))
		assert.Equal(t, 0, conn.total())
	})
}

// Against the real registry, one version behind: v42's workspace step fails on an empty
// database, so migrating the empty workspace would fail this test.
func TestManager_RunMigrations_SkipsAnEmptyWorkspaceAndStampsTheVersion(t *testing.T) {
	manager, mock, db, codeVersion := managerOneVersionBehind(t)
	lock := newFakeNameLock(t)
	conn := newScriptedConnector(t)
	manager.connector = conn
	manager.workspaceLock = lock.lock

	first, firstMock := workspaceDB(t)
	expectWorkspaceHoldsTables(firstMock, false)
	firstMock.ExpectClose()
	again, againMock := workspaceDB(t)
	expectWorkspaceHoldsTables(againMock, false)
	againMock.ExpectClose()
	conn.script("empty", connectorAnswer{db: first}, connectorAnswer{db: again})

	mock.ExpectBegin()
	expectWorkspaceRows(mock, "empty")
	expectRegisteredInTx(mock, "empty", true)
	lock.expectDatabaseExists("notifuse_ws_empty", true)
	mock.ExpectCommit()
	mock.ExpectExec("INSERT INTO settings").WithArgs(fmt.Sprintf("%.0f", codeVersion)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit() // the migration lock

	require.NoError(t, manager.RunMigrations(context.Background(), emptyWorkspaceTestConfig(), db))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestManager_RunMigrations_NoWorkspaceConnectionWithoutPendingMigrations(t *testing.T) {
	codeVersion, err := GetCurrentCodeVersion()
	require.NoError(t, err)

	for name, dbVersion := range map[string]float64{"equal versions": codeVersion, "database ahead of the code": codeVersion + 1} {
		t.Run(name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() { _ = db.Close() }()
			expectMigrationLockGranted(mock)
			mock.ExpectQuery("SELECT value FROM settings WHERE key = 'db_version'").
				WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(fmt.Sprintf("%.0f", dbVersion)))
			if dbVersion > codeVersion {
				mock.ExpectRollback() // the migration lock
			} else {
				mock.ExpectCommit()
			}

			conn := newScriptedConnector(t)
			manager := NewManager(logger.NewLoggerWithLevel("disabled"))
			manager.connector = conn
			err = manager.RunMigrations(context.Background(), emptyWorkspaceTestConfig(), db)
			if dbVersion > codeVersion {
				assert.ErrorIs(t, err, ErrDatabaseAheadOfCode)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, 0, conn.total())
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestWorkspaceMigrationDSN(t *testing.T) {
	cfg := &config.DatabaseConfig{User: "u", Password: "p", Host: "h", Port: 5432, Prefix: "notifuse", SSLMode: "disable"}
	dsn, err := workspaceMigrationDSN(cfg, "acme")
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(dsn, "/notifuse_ws_acme?connect_timeout=10&sslmode=disable"), dsn)

	_, err = workspaceMigrationDSN(cfg, "a?b=c")
	assert.Error(t, err)
}
