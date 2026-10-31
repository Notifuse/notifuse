package migrations

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// recordingLogger records the messages logged at each level.
type recordingLogger struct {
	mu     sync.Mutex
	fields map[string]interface{}
	logs   *[]loggedLine
}

type loggedLine struct {
	level, msg string
	fields     map[string]interface{}
}

func newRecordingLogger() *recordingLogger {
	return &recordingLogger{fields: map[string]interface{}{}, logs: &[]loggedLine{}}
}

func (l *recordingLogger) with(fields map[string]interface{}) *recordingLogger {
	merged := map[string]interface{}{}
	for k, v := range l.fields {
		merged[k] = v
	}
	for k, v := range fields {
		merged[k] = v
	}
	return &recordingLogger{fields: merged, logs: l.logs}
}

func (l *recordingLogger) WithField(key string, value interface{}) logger.Logger {
	return l.with(map[string]interface{}{key: value})
}
func (l *recordingLogger) WithFields(fields map[string]interface{}) logger.Logger {
	return l.with(fields)
}
func (l *recordingLogger) record(level, msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	*l.logs = append(*l.logs, loggedLine{level: level, msg: msg, fields: l.fields})
}
func (l *recordingLogger) Debug(msg string) { l.record("debug", msg) }
func (l *recordingLogger) Info(msg string)  { l.record("info", msg) }
func (l *recordingLogger) Warn(msg string)  { l.record("warn", msg) }
func (l *recordingLogger) Error(msg string) { l.record("error", msg) }
func (l *recordingLogger) Fatal(msg string) { l.record("fatal", msg) }

func (l *recordingLogger) at(level string) []loggedLine {
	var out []loggedLine
	for _, line := range *l.logs {
		if line.level == level {
			out = append(out, line)
		}
	}
	return out
}

// connectorFunc is a workspace connector answering through a function.
type connectorFunc func(ctx context.Context, workspaceID string) (*sql.DB, error)

func (f connectorFunc) connectToWorkspace(ctx context.Context, cfg *config.DatabaseConfig, workspaceID string) (*sql.DB, error) {
	return f(ctx, workspaceID)
}

// repairFixture runs InitializeEmptyWorkspaces against a sqlmock system database, a
// fake shared migration lock and name lock, and a recording initialiser.
type repairFixture struct {
	t        *testing.T
	sys      *sql.DB
	sysMock  sqlmock.Sqlmock
	nameLock *fakeNameLock
	log      *recordingLogger
	manager  *Manager

	events      []string
	inShared    bool
	sharedTaken int
	initErr     error
	initialised []time.Time // deadline of each initialisation's context
	onInit      func()
}

func newRepairFixture(t *testing.T) *repairFixture {
	t.Helper()
	sys, sysMock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sys.Close() })
	f := &repairFixture{t: t, sys: sys, sysMock: sysMock, nameLock: newFakeNameLock(t), log: newRecordingLogger()}
	f.manager = NewManager(f.log)
	f.manager.workspaceLock = func(ctx context.Context, cfg *config.DatabaseConfig, dbName string, fn func(ctx context.Context, admin *sql.DB) error) error {
		f.events = append(f.events, "name lock "+dbName)
		return f.nameLock.lock(ctx, cfg, dbName, fn)
	}
	f.manager.sharedMigrationLock = func(ctx context.Context, db *sql.DB, fn func(ctx context.Context) error) error {
		assert.Same(t, f.sys, db)
		f.sharedTaken++
		f.inShared = true
		f.events = append(f.events, "shared lock")
		defer func() { f.inShared = false; f.events = append(f.events, "shared unlock") }()
		return fn(ctx)
	}
	f.manager.initializeWorkspace = func(ctx context.Context, db *sql.DB) error {
		require.True(t, f.inShared, "initialised under the shared migration lock")
		require.True(t, f.nameLock.inLock, "initialised under the name lock")
		deadline, _ := ctx.Deadline()
		f.initialised = append(f.initialised, deadline)
		f.events = append(f.events, "initialise")
		if f.onInit != nil {
			f.onInit()
		}
		return f.initErr
	}
	f.manager.repairCheckTimeout = 100 * time.Millisecond
	f.manager.repairInitTimeout = time.Hour
	return f
}

func (f *repairFixture) expectWorkspaces(ids ...string) {
	rows := sqlmock.NewRows([]string{"id"})
	for _, id := range ids {
		rows.AddRow(id)
	}
	f.sysMock.ExpectQuery(`SELECT id FROM workspaces ORDER BY id`).WillReturnRows(rows)
}

func (f *repairFixture) expectVersion(version float64) {
	f.sysMock.ExpectQuery("SELECT value FROM settings WHERE key = 'db_version'").
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(fmt.Sprintf("%.0f", version)))
}

func (f *repairFixture) run(ctx context.Context) {
	f.manager.InitializeEmptyWorkspaces(ctx, emptyWorkspaceTestConfig(), f.sys)
	assert.NoError(f.t, f.sysMock.ExpectationsWereMet())
}

// emptyDB returns a workspace database answering its emptiness check once.
func emptyDB(t *testing.T, holdsTables bool) *sql.DB {
	db, mock := workspaceDB(t)
	expectWorkspaceHoldsTables(mock, holdsTables)
	mock.ExpectClose()
	return db
}

func TestManager_InitializeEmptyWorkspaces(t *testing.T) {
	codeVersion, err := GetCurrentCodeVersion()
	require.NoError(t, err)
	ctx := context.Background()

	t.Run("an empty workspace is checked again under its name lock and initialised before the shared lock goes", func(t *testing.T) {
		f := newRepairFixture(t)
		conn := newScriptedConnector(t)
		conn.script("empty", connectorAnswer{db: emptyDB(t, false)}, connectorAnswer{db: emptyDB(t, false)})
		f.manager.connector = conn
		f.expectWorkspaces("empty")
		f.expectVersion(codeVersion)
		expectRegisteredInTx(f.sysMock, "empty", true)
		f.nameLock.expectDatabaseExists("notifuse_ws_empty", true)

		start := time.Now()
		f.run(ctx)
		assert.Equal(t, []string{"shared lock", "name lock notifuse_ws_empty", "initialise", "shared unlock"}, f.events)
		require.Len(t, f.initialised, 1)
		assert.True(t, f.initialised[0].After(start.Add(30*time.Minute)), "the initialisation runs under its own bound, not the check's")
		require.Len(t, f.log.at("info"), 1)
		assert.Equal(t, "empty", f.log.at("info")[0].fields["workspace"])
	})

	t.Run("a database holding tables is left alone, without any lock", func(t *testing.T) {
		f := newRepairFixture(t)
		conn := newScriptedConnector(t)
		conn.script("full", connectorAnswer{db: emptyDB(t, true)})
		f.manager.connector = conn
		f.expectWorkspaces("full")

		f.run(ctx)
		assert.Empty(t, f.events)
		assert.Empty(t, f.log.at("error"))
	})

	t.Run("a version that is not the code's stops at the first empty workspace", func(t *testing.T) {
		for name, version := range map[string]float64{"below": codeVersion - 1, "above": codeVersion + 1} {
			t.Run(name, func(t *testing.T) {
				f := newRepairFixture(t)
				conn := newScriptedConnector(t)
				conn.script("a", connectorAnswer{db: emptyDB(t, false)}) // no connection to b
				f.manager.connector = conn
				f.expectWorkspaces("a", "b")
				f.expectVersion(version)

				f.run(ctx)
				assert.Equal(t, 1, f.sharedTaken)
				assert.Empty(t, f.initialised)
			})
		}
	})

	t.Run("no version stamped stops too", func(t *testing.T) {
		f := newRepairFixture(t)
		conn := newScriptedConnector(t)
		conn.script("a", connectorAnswer{db: emptyDB(t, false)})
		f.manager.connector = conn
		f.expectWorkspaces("a")
		f.sysMock.ExpectQuery("SELECT value FROM settings WHERE key = 'db_version'").WillReturnError(sql.ErrNoRows)

		f.run(ctx)
		assert.Empty(t, f.initialised)
	})

	t.Run("failures are logged and the next workspace still handled", func(t *testing.T) {
		f := newRepairFixture(t)
		f.initErr = errors.New("init broke")
		conn := newScriptedConnector(t)
		conn.script("a", connectorAnswer{err: errors.New("unreachable")})
		conn.script("b", connectorAnswer{db: emptyDB(t, false)}, connectorAnswer{db: emptyDB(t, false)})
		conn.script("c", connectorAnswer{db: emptyDB(t, true)})
		f.manager.connector = conn
		f.expectWorkspaces("a", "b", "c")
		f.expectVersion(codeVersion)
		expectRegisteredInTx(f.sysMock, "b", true)
		f.nameLock.expectDatabaseExists("notifuse_ws_b", true)

		f.run(ctx)
		assert.Equal(t, 1, f.sharedTaken, "only the empty database is handled under the lock")
		errs := f.log.at("error")
		require.Len(t, errs, 2)
		assert.Equal(t, "a", errs[0].fields["workspace"])
		assert.Contains(t, errs[0].fields["error"], "unreachable")
		assert.Equal(t, "b", errs[1].fields["workspace"])
		assert.Contains(t, errs[1].fields["error"], "init broke")
	})

	t.Run("a connection hanging until its deadline is given up, and the next workspace handled", func(t *testing.T) {
		f := newRepairFixture(t)
		var deadline time.Time
		f.manager.connector = connectorFunc(func(ctx context.Context, id string) (*sql.DB, error) {
			if id == "hangs" {
				deadline, _ = ctx.Deadline()
				<-ctx.Done()
				return nil, ctx.Err()
			}
			return emptyDB(t, true), nil
		})
		f.expectWorkspaces("hangs", "next")

		start := time.Now()
		f.run(ctx)
		assert.WithinDuration(t, start.Add(f.manager.repairCheckTimeout), deadline, 50*time.Millisecond)
		assert.Less(t, time.Since(start), 5*time.Second)
		assert.Equal(t, 0, f.sharedTaken)
		require.Len(t, f.log.at("error"), 1)
	})

	t.Run("inside the name lock, a workspace deleted or a database gone is not initialised", func(t *testing.T) {
		f := newRepairFixture(t)
		conn := newScriptedConnector(t)
		conn.script("deleted", connectorAnswer{db: emptyDB(t, false)})
		conn.script("gone", connectorAnswer{db: emptyDB(t, false)})
		f.manager.connector = conn
		f.expectWorkspaces("deleted", "gone")
		f.expectVersion(codeVersion)
		expectRegisteredInTx(f.sysMock, "deleted", false)
		f.expectVersion(codeVersion)
		expectRegisteredInTx(f.sysMock, "gone", true)
		f.nameLock.expectDatabaseExists("notifuse_ws_gone", false) // never created here

		f.run(ctx)
		assert.Empty(t, f.initialised)
		assert.Empty(t, f.log.at("error"))
	})

	t.Run("tables appearing before the name lock was granted: not initialised", func(t *testing.T) {
		f := newRepairFixture(t)
		conn := newScriptedConnector(t)
		conn.script("raced", connectorAnswer{db: emptyDB(t, false)}, connectorAnswer{db: emptyDB(t, true)})
		f.manager.connector = conn
		f.expectWorkspaces("raced")
		f.expectVersion(codeVersion)
		expectRegisteredInTx(f.sysMock, "raced", true)
		f.nameLock.expectDatabaseExists("notifuse_ws_raced", true)

		f.run(ctx)
		assert.Empty(t, f.initialised)
	})

	t.Run("a cancelled context stops the loop", func(t *testing.T) {
		f := newRepairFixture(t)
		runCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		f.onInit = cancel
		conn := newScriptedConnector(t)
		conn.script("a", connectorAnswer{db: emptyDB(t, false)}, connectorAnswer{db: emptyDB(t, false)})
		f.manager.connector = conn // no connection to b
		f.expectWorkspaces("a", "b")
		f.expectVersion(codeVersion)
		expectRegisteredInTx(f.sysMock, "a", true)
		f.nameLock.expectDatabaseExists("notifuse_ws_a", true)

		f.run(runCtx)
		assert.Equal(t, 1, f.sharedTaken)
	})

	t.Run("a failed workspace listing runs nothing", func(t *testing.T) {
		f := newRepairFixture(t)
		f.sysMock.ExpectQuery(`SELECT id FROM workspaces ORDER BY id`).WillReturnError(errors.New("down"))
		f.run(ctx)
		assert.Equal(t, 0, f.sharedTaken)
		require.Len(t, f.log.at("error"), 1)
	})

	t.Run("a listing ended by a cancelled context logs nothing", func(t *testing.T) {
		f := newRepairFixture(t)
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		f.run(cancelled)
		assert.Empty(t, *f.log.logs)
	})

	t.Run("a workspace deleted since the listing is skipped without a word", func(t *testing.T) {
		f := newRepairFixture(t)
		conn := newScriptedConnector(t)
		conn.script("deleted", connectorAnswer{err: fmt.Errorf("ping: %w", &pq.Error{Code: "3D000"})})
		f.manager.connector = conn
		f.expectWorkspaces("deleted")
		expectRegisteredInTx(f.sysMock, "deleted", false)

		f.run(ctx)
		assert.Empty(t, *f.log.logs)
		assert.Equal(t, []string{"name lock notifuse_ws_deleted"}, f.events, "looked at again under the name lock, which waits for a deletion in progress")
	})

	t.Run("a probe cut by a deletion that has since removed the workspace is skipped without a word", func(t *testing.T) {
		f := newRepairFixture(t)
		conn := newScriptedConnector(t)
		conn.script("dropped", connectorAnswer{err: fmt.Errorf("probe: %w", &pq.Error{Code: "57P01"})})
		f.manager.connector = conn
		f.expectWorkspaces("dropped")
		expectRegisteredInTx(f.sysMock, "dropped", false)

		f.run(ctx)
		assert.Empty(t, *f.log.logs)
	})

	t.Run("a failed look under the name lock logs the probe's error, not the missing-database advice", func(t *testing.T) {
		f := newRepairFixture(t)
		conn := newScriptedConnector(t)
		conn.script("lost", connectorAnswer{err: fmt.Errorf("ping: %w", &pq.Error{Code: "3D000"})})
		f.manager.connector = conn
		f.expectWorkspaces("lost")
		f.sysMock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM workspaces WHERE id = \$1\)`).WillReturnError(errors.New("system db down"))

		f.run(ctx)
		assert.Empty(t, f.log.at("warn"))
		require.Len(t, f.log.at("error"), 1)
		assert.Equal(t, "lost", f.log.at("error")[0].fields["workspace"])
		assert.Contains(t, f.log.at("error")[0].fields["recheck_error"], "system db down")
	})

	t.Run("a database created between the probe and the look is left for the next start, without a word", func(t *testing.T) {
		f := newRepairFixture(t)
		conn := newScriptedConnector(t)
		conn.script("recreated", connectorAnswer{err: fmt.Errorf("ping: %w", &pq.Error{Code: "3D000"})})
		f.manager.connector = conn
		f.expectWorkspaces("recreated")
		expectRegisteredInTx(f.sysMock, "recreated", true)
		f.nameLock.expectDatabaseExists("notifuse_ws_recreated", true)

		f.run(ctx)
		assert.Empty(t, *f.log.logs)
	})

	t.Run("a failure no deletion causes is logged without a look under the name lock", func(t *testing.T) {
		f := newRepairFixture(t)
		conn := newScriptedConnector(t)
		conn.script("busy", connectorAnswer{err: fmt.Errorf("ping: %w", &pq.Error{Code: "53300"})})
		f.manager.connector = conn
		f.expectWorkspaces("busy")

		f.run(ctx)
		assert.Empty(t, f.events)
		require.Len(t, f.log.at("error"), 1)
	})

	t.Run("a missing database is reported with what to do, without the migration lock", func(t *testing.T) {
		f := newRepairFixture(t)
		conn := newScriptedConnector(t)
		conn.script("lost", connectorAnswer{err: fmt.Errorf("ping: %w", &pq.Error{Code: "3D000"})})
		conn.script("next", connectorAnswer{db: emptyDB(t, true)})
		f.manager.connector = conn
		f.expectWorkspaces("lost", "next")
		expectRegisteredInTx(f.sysMock, "lost", true)
		f.nameLock.expectDatabaseExists("notifuse_ws_lost", false)

		f.run(ctx)
		assert.Equal(t, 0, f.sharedTaken)
		assert.Empty(t, f.log.at("error"))
		warns := f.log.at("warn")
		require.Len(t, warns, 1)
		assert.Equal(t, "lost", warns[0].fields["workspace"])
		assert.Contains(t, warns[0].msg, "workspace lost is registered but its database notifuse_ws_lost does not exist: if DB_PREFIX changed, restore it")
	})
}

func TestManager_RunMigrations_WaitsForTheMigrationLockAndNamesItsHolders(t *testing.T) {
	codeVersion, err := GetCurrentCodeVersion()
	require.NoError(t, err)
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	// The version is read before the lock is tried, and again once it is held.
	expectVersionRow(mock, codeVersion)
	expectMigrationLockAwaited(mock, sqlmock.NewRows([]string{"pid", "shared", "app", "addr"}).
		AddRow(4242, false, "notifuse", "10.0.0.7").AddRow(4243, true, "", ""))
	expectVersionRow(mock, codeVersion) // up to date, unchanged since before the wait
	mock.ExpectCommit()

	log := newRecordingLogger()
	require.NoError(t, NewManager(log).RunMigrations(context.Background(), &config.Config{}, db))
	require.NoError(t, mock.ExpectationsWereMet())
	warns := log.at("warn")
	require.Len(t, warns, 1)
	assert.Equal(t, "pid 4242 running migrations (notifuse) from 10.0.0.7, pid 4243 repairing an empty workspace database", warns[0].fields["holders"])
}

func expectVersionRow(mock sqlmock.Sqlmock, version float64) {
	mock.ExpectQuery("SELECT value FROM settings WHERE key = 'db_version'").
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(fmt.Sprintf("%.0f", version)))
}

// expectMigrationLockAwaited expects the migration lock refused at first, its holders
// looked up, then a blocking wait.
func expectMigrationLockAwaited(mock sqlmock.Sqlmock, holders *sqlmock.Rows) {
	mock.ExpectBegin()
	for _, statement := range lockSettingsForTest() {
		mock.ExpectExec(statement).WillReturnResult(sqlmock.NewResult(0, 0))
	}
	mock.ExpectQuery(`SELECT pg_try_advisory_xact_lock`).WillReturnRows(sqlmock.NewRows([]string{"granted"}).AddRow(false))
	mock.ExpectQuery(`FROM pg_locks l`).WillReturnRows(holders)
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).WillReturnResult(sqlmock.NewResult(0, 0))
}

func TestManager_RunMigrations_RestartsWhenAnotherInstanceMigrated(t *testing.T) {
	codeVersion, err := GetCurrentCodeVersion()
	require.NoError(t, err)
	noHolders := func() *sqlmock.Rows { return sqlmock.NewRows([]string{"pid", "shared", "app", "addr"}) }
	granted := func(mock sqlmock.Sqlmock) {
		mock.ExpectBegin()
		for _, statement := range lockSettingsForTest() {
			mock.ExpectExec(statement).WillReturnResult(sqlmock.NewResult(0, 0))
		}
		mock.ExpectQuery(`SELECT pg_try_advisory_xact_lock`).WillReturnRows(sqlmock.NewRows([]string{"granted"}).AddRow(true))
	}

	run := func(t *testing.T, expect func(mock sqlmock.Sqlmock)) error {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		expect(mock)
		err = NewManager(logger.NewLoggerWithLevel("disabled")).RunMigrations(context.Background(), &config.Config{}, db)
		require.NoError(t, mock.ExpectationsWereMet())
		return err
	}

	t.Run("the holder waited for applied V36, which may require a restart", func(t *testing.T) {
		err := run(t, func(mock sqlmock.Sqlmock) {
			expectVersionRow(mock, 35)
			expectMigrationLockAwaited(mock, noHolders())
			expectVersionRow(mock, codeVersion)
			mock.ExpectRollback()
		})
		assert.ErrorIs(t, err, ErrRestartRequired)
		assert.ErrorContains(t, err, fmt.Sprintf("another instance migrated the database from version 35 to %.0f, across version 36", codeVersion))
	})

	t.Run("another instance applied V14, which requires one, between the read and a lock granted at once", func(t *testing.T) {
		err := run(t, func(mock sqlmock.Sqlmock) {
			expectVersionRow(mock, 13)
			granted(mock)
			expectVersionRow(mock, 14)
			mock.ExpectRollback()
		})
		assert.ErrorIs(t, err, ErrRestartRequired)
	})

	t.Run("the version moved across no migration that can require a restart", func(t *testing.T) {
		err := run(t, func(mock sqlmock.Sqlmock) {
			expectVersionRow(mock, 36)
			expectMigrationLockAwaited(mock, noHolders())
			expectVersionRow(mock, codeVersion)
			mock.ExpectCommit()
		})
		assert.NoError(t, err)
	})

	t.Run("the version moved down (a restore): no restart", func(t *testing.T) {
		err := run(t, func(mock sqlmock.Sqlmock) {
			expectVersionRow(mock, codeVersion)
			granted(mock)
			expectVersionRow(mock, 13)
			mock.ExpectRollback() // the run goes on to migrate from 13 and fails on the next query
		})
		assert.NotErrorIs(t, err, ErrRestartRequired)
	})

	t.Run("the version did not move", func(t *testing.T) {
		err := run(t, func(mock sqlmock.Sqlmock) {
			expectVersionRow(mock, codeVersion)
			expectMigrationLockAwaited(mock, noHolders())
			expectVersionRow(mock, codeVersion)
			mock.ExpectCommit()
		})
		assert.NoError(t, err)
	})

	t.Run("a database ahead of the code is refused before the lock", func(t *testing.T) {
		err := run(t, func(mock sqlmock.Sqlmock) {
			expectVersionRow(mock, codeVersion+1)
		})
		assert.ErrorIs(t, err, ErrDatabaseAheadOfCode)
	})

	t.Run("a failed read before the lock stops startup with the error, before the lock", func(t *testing.T) {
		err := run(t, func(mock sqlmock.Sqlmock) {
			mock.ExpectQuery("SELECT value FROM settings WHERE key = 'db_version'").WillReturnError(errors.New("blip"))
		})
		require.EqualError(t, err, "failed to get current database version: blip")
	})

	t.Run("no version before the lock: a first run, nothing to reload", func(t *testing.T) {
		err := run(t, func(mock sqlmock.Sqlmock) {
			mock.ExpectQuery("SELECT value FROM settings WHERE key = 'db_version'").WillReturnError(sql.ErrNoRows)
			expectMigrationLockAwaited(mock, noHolders())
			expectVersionRow(mock, codeVersion)
			mock.ExpectCommit()
		})
		assert.NoError(t, err)
	})
}
