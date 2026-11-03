package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
)

// provisioningFixture injects every dependency of workspace database provisioning (D5):
// a fake name lock that hands fn a sqlmock admin database and records each
// acquisition, a workspace opener, an initialiser and an emptiness check.
type provisioningFixture struct {
	t         *testing.T
	admin     *sql.DB
	adminMock sqlmock.Sqlmock

	events []string // what happened, in order

	lockAcquisitions int
	inLock           bool

	openErr        error
	initErr        error
	initErrs       []error // per initialisation, in order; initErr once they run out
	initBlockOnCtx bool
	holdsTables    bool
	holdsErr       error

	opened      int
	initialized int
	holdsCalls  int
	openedDBs   []*sql.DB

	initCtxDeadline time.Time

	hold         time.Duration // the lock's hold bound; zero means lockHoldTimeout
	holdDeadline time.Time
}

func newProvisioningFixture(t *testing.T) *provisioningFixture {
	t.Helper()
	admin, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = admin.Close() })
	return &provisioningFixture{t: t, admin: admin, adminMock: mock}
}

func (f *provisioningFixture) provisioner() workspaceDatabaseProvisioner {
	return workspaceDatabaseProvisioner{
		lock: func(ctx context.Context, cfg *config.DatabaseConfig, dbName string, fn func(ctx context.Context, admin *sql.DB) error) error {
			f.lockAcquisitions++
			f.events = append(f.events, "lock "+dbName)
			f.inLock = true
			defer func() { f.inLock = false; f.events = append(f.events, "unlock") }()
			hold := f.hold
			if hold == 0 {
				hold = lockHoldTimeout
			}
			f.holdDeadline = time.Now().Add(hold)
			return fn(withLockHoldDeadline(ctx, f.holdDeadline), f.admin)
		},
		openWorkspace: func(ctx context.Context, cfg *config.DatabaseConfig, dbName string) (*sql.DB, error) {
			require.True(f.t, f.inLock, "workspace database opened outside the lock")
			f.opened++
			f.events = append(f.events, "open "+dbName)
			if f.openErr != nil {
				return nil, f.openErr
			}
			db, mock, err := sqlmock.New()
			require.NoError(f.t, err)
			mock.ExpectClose()
			f.openedDBs = append(f.openedDBs, db)
			f.t.Cleanup(func() { assert.NoError(f.t, mock.ExpectationsWereMet(), "workspace connection not closed") })
			return db, nil
		},
		initialize: func(ctx context.Context, db *sql.DB) error {
			require.True(f.t, f.inLock, "initialised outside the lock")
			f.initialized++
			f.events = append(f.events, "initialize")
			f.initCtxDeadline, _ = ctx.Deadline()
			if f.initBlockOnCtx {
				<-ctx.Done()
				return ctx.Err()
			}
			if f.initialized <= len(f.initErrs) {
				return f.initErrs[f.initialized-1]
			}
			return f.initErr
		},
		holdsTables: func(ctx context.Context, db *sql.DB) (bool, error) {
			require.True(f.t, f.inLock, "emptiness checked outside the lock")
			f.holdsCalls++
			f.events = append(f.events, "holds-tables")
			return f.holdsTables, f.holdsErr
		},
	}
}

func (f *provisioningFixture) expectDatabaseExists(name string, exists bool) {
	f.adminMock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM pg_database WHERE datname = \$1\)`).
		WithArgs(name).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(exists))
}

func (f *provisioningFixture) expectCreate(name string) {
	f.adminMock.ExpectExec(`CREATE DATABASE "` + name + `"`).WillReturnResult(sqlmock.NewResult(0, 0))
}

func (f *provisioningFixture) expectDrop(name string) *sqlmock.ExpectedExec {
	return f.adminMock.ExpectExec(`DROP DATABASE IF EXISTS "` + name + `" WITH \(FORCE\)`)
}

func (f *provisioningFixture) verify() {
	f.t.Helper()
	assert.NoError(f.t, f.adminMock.ExpectationsWereMet())
}

var provisioningConfig = &config.DatabaseConfig{User: "u", Password: "p", Host: "h", Port: 5432, Prefix: "notifuse", SSLMode: "disable"}

func TestEnsureWorkspaceDatabaseExists_Provisioning(t *testing.T) {
	ctx := context.Background()

	t.Run("checks registration inside the lock, and its error stops everything", func(t *testing.T) {
		f := newProvisioningFixture(t)
		gone := &domain.ErrWorkspaceNotFound{WorkspaceID: "acme"}
		err := f.provisioner().ensure(ctx, provisioningConfig, "acme", func(ctx context.Context) error {
			require.True(t, f.inLock, "registration checked outside the lock")
			return gone
		})
		assert.ErrorIs(t, err, gone)
		assert.Equal(t, 0, f.opened)
		f.verify() // no query on the admin database either
	})

	t.Run("missing: creates and initialises, without the emptiness check", func(t *testing.T) {
		f := newProvisioningFixture(t)
		f.expectDatabaseExists("notifuse_ws_acme", false)
		f.expectCreate("notifuse_ws_acme")

		err := f.provisioner().ensure(ctx, provisioningConfig, "acme", func(ctx context.Context) error { return nil })
		require.NoError(t, err)
		assert.Equal(t, 1, f.initialized)
		// A database copied from a template1 that holds a user table must still get
		// its schema, so a new database is never checked for emptiness.
		assert.Equal(t, 0, f.holdsCalls)
		assert.Equal(t, 1, f.lockAcquisitions)
		f.verify()
	})

	t.Run("missing and initialisation fails: drops what it created", func(t *testing.T) {
		f := newProvisioningFixture(t)
		f.initErr = errors.New("cannot change return type of existing function")
		f.expectDatabaseExists("notifuse_ws_acme", false)
		f.expectCreate("notifuse_ws_acme")
		f.expectDrop("notifuse_ws_acme").WillReturnResult(sqlmock.NewResult(0, 0))

		err := f.provisioner().ensure(ctx, provisioningConfig, "acme", nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, f.initErr)
		assert.Contains(t, err.Error(), "failed to initialize workspace database schema")
		f.verify()
	})

	t.Run("missing and the caller's deadline expires during initialisation: the drop still runs", func(t *testing.T) {
		f := newProvisioningFixture(t)
		f.initBlockOnCtx = true
		f.expectDatabaseExists("notifuse_ws_acme", false)
		f.expectCreate("notifuse_ws_acme")
		f.expectDrop("notifuse_ws_acme").WillReturnResult(sqlmock.NewResult(0, 0))

		shortCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
		defer cancel()
		err := f.provisioner().ensure(shortCtx, provisioningConfig, "acme", nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
		f.verify() // a drop under the expired context would never reach the driver
	})

	t.Run("missing and the connection fails: drops what it created", func(t *testing.T) {
		f := newProvisioningFixture(t)
		f.openErr = errors.New("connection refused")
		f.expectDatabaseExists("notifuse_ws_acme", false)
		f.expectCreate("notifuse_ws_acme")
		f.expectDrop("notifuse_ws_acme").WillReturnResult(sqlmock.NewResult(0, 0))

		err := f.provisioner().ensure(ctx, provisioningConfig, "acme", nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, f.openErr)
		assert.Equal(t, 0, f.initialized)
		f.verify()
	})

	t.Run("the drop after a failure fails too: the error says both", func(t *testing.T) {
		f := newProvisioningFixture(t)
		f.initErr = errors.New("init broke")
		f.expectDatabaseExists("notifuse_ws_acme", false)
		f.expectCreate("notifuse_ws_acme")
		f.expectDrop("notifuse_ws_acme").WillReturnError(errors.New("drop broke"))

		err := f.provisioner().ensure(ctx, provisioningConfig, "acme", nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, f.initErr)
		assert.Contains(t, err.Error(), "drop broke")
		f.verify()
	})

	t.Run("a failed CREATE DATABASE drops nothing", func(t *testing.T) {
		f := newProvisioningFixture(t)
		f.expectDatabaseExists("notifuse_ws_acme", false)
		f.adminMock.ExpectExec(`CREATE DATABASE`).WillReturnError(errors.New("permission denied to create database"))

		err := f.provisioner().ensure(ctx, provisioningConfig, "acme", nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create workspace database")
		assert.Equal(t, 0, f.opened)
		f.verify()
	})

	t.Run("exists and empty: initialises, creates and drops nothing", func(t *testing.T) {
		f := newProvisioningFixture(t)
		f.expectDatabaseExists("notifuse_ws_acme", true)

		err := f.provisioner().ensure(ctx, provisioningConfig, "acme", nil)
		require.NoError(t, err)
		assert.Equal(t, 1, f.holdsCalls)
		assert.Equal(t, 1, f.initialized)
		f.verify()
	})

	t.Run("exists and empty, initialisation fails: nothing dropped", func(t *testing.T) {
		f := newProvisioningFixture(t)
		f.initErr = errors.New("init broke")
		f.expectDatabaseExists("notifuse_ws_acme", true)

		err := f.provisioner().ensure(ctx, provisioningConfig, "acme", nil)
		assert.ErrorIs(t, err, f.initErr)
		f.verify()
	})

	t.Run("exists with tables: nothing", func(t *testing.T) {
		f := newProvisioningFixture(t)
		f.holdsTables = true
		f.expectDatabaseExists("notifuse_ws_acme", true)

		err := f.provisioner().ensure(ctx, provisioningConfig, "acme", nil)
		require.NoError(t, err)
		assert.Equal(t, 0, f.initialized)
		f.verify()
	})

	t.Run("invalid ID: refused before the lock", func(t *testing.T) {
		f := newProvisioningFixture(t)
		err := f.provisioner().ensure(ctx, provisioningConfig, "a?b=c", nil)
		assert.EqualError(t, err, `invalid workspace ID for a database name: "a?b=c"`)
		assert.Equal(t, 0, f.lockAcquisitions)
	})
}

func TestCreateWorkspaceDatabase_Provisioning(t *testing.T) {
	ctx := context.Background()
	previousDelays := insertRetryDelays
	insertRetryDelays = []time.Duration{0, 0}
	defer func() { insertRetryDelays = previousDelays }()

	type insertAnswer struct {
		found bool // a row was there: nothing inserted
		err   error
	}
	type rowState struct {
		rows     []WorkspaceRow // answers in order; the last one repeats
		rowErrs  []error
		calls    int
		outcomes []insertAnswer // answers of insert in order; none means it inserts
		inserts  int
		inserted bool          // an insert inserted: lookups answer afterInsert from then on
		after    *WorkspaceRow // the answer then; nil means WorkspaceRowOurs
		afterErr error
	}
	rowFn := func(f *provisioningFixture, s *rowState) func(ctx context.Context) (WorkspaceRow, error) {
		return func(ctx context.Context) (WorkspaceRow, error) {
			require.True(t, f.inLock, "registration checked outside the lock")
			i := s.calls
			s.calls++
			f.events = append(f.events, "registered?")
			if s.inserted {
				if s.after != nil {
					return *s.after, s.afterErr
				}
				return WorkspaceRowOurs, s.afterErr
			}
			var err error
			if i < len(s.rowErrs) {
				err = s.rowErrs[i]
			}
			if i >= len(s.rows) {
				i = len(s.rows) - 1
			}
			return s.rows[i], err
		}
	}
	insertFn := func(f *provisioningFixture, s *rowState) func(ctx context.Context) (bool, error) {
		return func(ctx context.Context) (bool, error) {
			require.True(t, f.inLock, "row inserted outside the lock")
			i := s.inserts
			s.inserts++
			f.events = append(f.events, "insert")
			if i >= len(s.outcomes) {
				s.inserted = true
				return true, nil
			}
			o := s.outcomes[i]
			s.inserted = !o.found && o.err == nil
			return s.inserted, o.err
		}
	}

	t.Run("already registered: ErrWorkspaceAlreadyExists, no query on any database", func(t *testing.T) {
		f := newProvisioningFixture(t)
		s := &rowState{rows: []WorkspaceRow{WorkspaceRowOther}}
		err := f.provisioner().create(ctx, provisioningConfig, "acme", rowFn(f, s), insertFn(f, s))
		assert.ErrorIs(t, err, domain.ErrWorkspaceAlreadyExists)
		assert.Equal(t, 0, s.inserts)
		assert.Equal(t, 0, f.opened)
		f.verify()
	})

	t.Run("a row matching this creation before its insert is refused too", func(t *testing.T) {
		f := newProvisioningFixture(t)
		s := &rowState{rows: []WorkspaceRow{WorkspaceRowOurs}}
		err := f.provisioner().create(ctx, provisioningConfig, "acme", rowFn(f, s), insertFn(f, s))
		assert.ErrorIs(t, err, domain.ErrWorkspaceAlreadyExists)
		assert.Equal(t, 0, s.inserts)
		f.verify()
	})

	t.Run("registration check fails: nothing touched", func(t *testing.T) {
		f := newProvisioningFixture(t)
		boom := errors.New("system database down")
		s := &rowState{rows: []WorkspaceRow{WorkspaceRowAbsent}, rowErrs: []error{boom}}
		err := f.provisioner().create(ctx, provisioningConfig, "acme", rowFn(f, s), insertFn(f, s))
		assert.ErrorIs(t, err, boom)
		assert.Equal(t, 0, s.inserts)
		f.verify()
	})

	t.Run("missing: creates, initialises, inserts once, all inside the lock", func(t *testing.T) {
		f := newProvisioningFixture(t)
		s := &rowState{rows: []WorkspaceRow{WorkspaceRowAbsent}}
		f.expectDatabaseExists("notifuse_ws_acme", false)
		f.expectCreate("notifuse_ws_acme")

		err := f.provisioner().create(ctx, provisioningConfig, "acme", rowFn(f, s), insertFn(f, s))
		require.NoError(t, err)
		assert.Equal(t, 1, s.inserts)
		assert.Equal(t, 0, f.holdsCalls)
		assert.Equal(t, []string{"lock notifuse_ws_acme", "registered?", "open notifuse_ws_acme", "initialize", "insert", "unlock"}, f.events)
		f.verify()
	})

	t.Run("exists and empty: initialises and inserts", func(t *testing.T) {
		f := newProvisioningFixture(t)
		s := &rowState{rows: []WorkspaceRow{WorkspaceRowAbsent}}
		f.expectDatabaseExists("notifuse_ws_acme", true)

		err := f.provisioner().create(ctx, provisioningConfig, "acme", rowFn(f, s), insertFn(f, s))
		require.NoError(t, err)
		assert.Equal(t, 1, f.initialized)
		assert.Equal(t, 1, s.inserts)
		f.verify()
	})

	t.Run("exists with tables: ErrWorkspaceDatabaseNotEmpty, no DDL, no drop, no insert", func(t *testing.T) {
		f := newProvisioningFixture(t)
		f.holdsTables = true
		s := &rowState{rows: []WorkspaceRow{WorkspaceRowAbsent}}
		f.expectDatabaseExists("notifuse_ws_acme", true)

		err := f.provisioner().create(ctx, provisioningConfig, "acme", rowFn(f, s), insertFn(f, s))
		var notEmpty *domain.ErrWorkspaceDatabaseNotEmpty
		require.ErrorAs(t, err, &notEmpty)
		assert.Equal(t, "acme", notEmpty.WorkspaceID)
		assert.Equal(t, "notifuse_ws_acme", notEmpty.DatabaseName)
		assert.Equal(t, 0, f.initialized)
		assert.Equal(t, 0, s.inserts)
		f.verify()
	})

	refused := &pq.Error{Code: "23502", Message: "null value in column violates not-null constraint"}
	lost := errors.New("connection reset by peer")
	readOnly := &pq.Error{Code: "25006", Message: "cannot execute INSERT in a read-only transaction"}

	t.Run("an insert that can never succeed: dropped through the unlocked drop", func(t *testing.T) {
		f := newProvisioningFixture(t)
		s := &rowState{rows: []WorkspaceRow{WorkspaceRowAbsent}, outcomes: []insertAnswer{{err: refused}}}
		f.expectDatabaseExists("notifuse_ws_acme", false)
		f.expectCreate("notifuse_ws_acme")
		f.expectDrop("notifuse_ws_acme").WillReturnResult(sqlmock.NewResult(0, 0))

		err := f.provisioner().create(ctx, provisioningConfig, "acme", rowFn(f, s), insertFn(f, s))
		assert.ErrorIs(t, err, refused)
		assert.Equal(t, 1, s.inserts, "no point inserting again")
		assert.Equal(t, 1, f.lockAcquisitions, "the cleanup drop must not take the lock again")
		f.verify()
	})

	t.Run("can never succeed on an adopted empty database: dropped too", func(t *testing.T) {
		f := newProvisioningFixture(t)
		s := &rowState{rows: []WorkspaceRow{WorkspaceRowAbsent}, outcomes: []insertAnswer{{err: refused}}}
		f.expectDatabaseExists("notifuse_ws_acme", true)
		f.expectDrop("notifuse_ws_acme").WillReturnResult(sqlmock.NewResult(0, 0))

		err := f.provisioner().create(ctx, provisioningConfig, "acme", rowFn(f, s), insertFn(f, s))
		assert.ErrorIs(t, err, refused)
		f.verify()
	})

	// Every way the insert can end, attempt by attempt. A row found is this creation's
	// own when an earlier attempt committed, or a writer's that does not take the name
	// lock (an earlier release, during a rolling deploy). After a failure, or a row gone
	// before its lookup, the insert runs again. A lost connection may follow a commit;
	// an insert not sent, or refused by the server, changed nothing.
	captureRepairLog := func(t *testing.T) *[]error {
		var logged []error
		saved := logRetryCheckError
		logRetryCheckError = func(workspaceID, dbName, _ string, err error) {
			assert.Equal(t, "acme", workspaceID)
			assert.Equal(t, "notifuse_ws_acme", dbName)
			logged = append(logged, err)
		}
		t.Cleanup(func() { logRetryCheckError = saved })
		return &logged
	}
	found := insertAnswer{found: true}
	notSent := fmt.Errorf("%w: %w", errInsertNotSent, errors.New("dial tcp: connection refused"))
	type attemptCase struct {
		outcomes []insertAnswer
		rows     []WorkspaceRow // lookups after the registration check
		rowErrs  []error
		inserts  int
		want     error  // nil: success
		wantMsg  string // the outcome stays unknown, the database not dropped
		drop     bool
		exists   *bool // the database looked up after a retried insert; by default, there with tables
	}
	yes, no := true, false
	for name, tc := range map[string]attemptCase{
		"inserted":                {inserts: 1},
		"this creation's own row": {outcomes: []insertAnswer{found}, rows: []WorkspaceRow{WorkspaceRowOurs}, inserts: 1},
		"another writer's row":    {outcomes: []insertAnswer{found}, rows: []WorkspaceRow{WorkspaceRowOther}, inserts: 1, want: domain.ErrWorkspaceAlreadyExists},
		"a row gone before its lookup, then inserted, the database still there": {
			outcomes: []insertAnswer{found}, rows: []WorkspaceRow{WorkspaceRowAbsent}, inserts: 2, exists: &yes,
		},
		"a row gone before its lookup, then inserted, the database dropped with it: created again": {
			outcomes: []insertAnswer{found}, rows: []WorkspaceRow{WorkspaceRowAbsent}, inserts: 2, exists: &no,
		},
		"a row gone before its lookup, then another writer's row": {
			outcomes: []insertAnswer{found, found}, rows: []WorkspaceRow{WorkspaceRowAbsent, WorkspaceRowOther}, inserts: 2, want: domain.ErrWorkspaceAlreadyExists,
		},
		"an unknown lookup answer": {outcomes: []insertAnswer{found}, rows: []WorkspaceRow{WorkspaceRow(99)}, inserts: 1, wantMsg: "unexpected answer 99"},
		"a row gone before its lookup, then its own row: the database checked again": {
			outcomes: []insertAnswer{found, found}, rows: []WorkspaceRow{WorkspaceRowAbsent, WorkspaceRowOurs}, inserts: 2, exists: &yes,
		},
		"a failed lookup, then inserted: the database checked again": {
			outcomes: []insertAnswer{found}, rows: []WorkspaceRow{WorkspaceRowAbsent}, rowErrs: []error{errors.New("down")}, inserts: 2, exists: &yes,
		},
		"a failed lookup, then the row again": {outcomes: []insertAnswer{found, found}, rows: []WorkspaceRow{WorkspaceRowAbsent, WorkspaceRowOurs}, rowErrs: []error{errors.New("down")}, inserts: 2},
		"a lost connection, then inserted":    {outcomes: []insertAnswer{{err: lost}}, inserts: 2},
		"a lost connection, then its own row": {outcomes: []insertAnswer{{err: lost}, found}, rows: []WorkspaceRow{WorkspaceRowOurs}, inserts: 2},
		"a lost connection, then another's row": {
			outcomes: []insertAnswer{{err: lost}, found}, rows: []WorkspaceRow{WorkspaceRowOther}, inserts: 2, want: domain.ErrWorkspaceAlreadyExists,
		},
		"a transient refusal, then inserted":   {outcomes: []insertAnswer{{err: readOnly}}, inserts: 2},
		"a cancelled insert, then its own row": {outcomes: []insertAnswer{{err: &pq.Error{Code: "57014"}}, found}, rows: []WorkspaceRow{WorkspaceRowOurs}, inserts: 2},
		"not sent, then inserted":              {outcomes: []insertAnswer{{err: notSent}}, inserts: 2},
		"not sent, then inserted, the database dropped meanwhile: created again": {
			outcomes: []insertAnswer{{err: notSent}}, inserts: 2, exists: &no,
		},
		"a lost connection, then an insert that can never succeed: none committed": {
			outcomes: []insertAnswer{{err: lost}, {err: refused}}, inserts: 2, want: refused, drop: true,
		},
		"a row seen, then an insert that can never succeed": {
			outcomes: []insertAnswer{found, {err: refused}}, rows: []WorkspaceRow{WorkspaceRowAbsent}, rowErrs: []error{errors.New("down")}, inserts: 2, wantMsg: "null value",
		},
		"not sent on every attempt: none committed": {
			outcomes: []insertAnswer{{err: notSent}, {err: notSent}, {err: notSent}}, inserts: 3, want: errInsertNotSent, drop: true,
		},
		"a transient refusal on every attempt: none committed": {
			outcomes: []insertAnswer{{err: readOnly}, {err: readOnly}, {err: readOnly}}, inserts: 3, want: readOnly, drop: true,
		},
		"a lost connection on every attempt": {
			outcomes: []insertAnswer{{err: lost}, {err: lost}, {err: lost}}, inserts: 3, wantMsg: lost.Error(),
		},
		"a lost connection, then not sent": {
			outcomes: []insertAnswer{{err: lost}, {err: notSent}, {err: notSent}}, inserts: 3, wantMsg: "insert not sent",
		},
		"a failed lookup on every attempt": {
			outcomes: []insertAnswer{found, found, found}, rows: []WorkspaceRow{WorkspaceRowAbsent}, rowErrs: []error{errors.New("down"), errors.New("down"), errors.New("down")}, inserts: 3,
			wantMsg: "after its insert found a row: down",
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := newProvisioningFixture(t)
			s := &rowState{
				rows:     append([]WorkspaceRow{WorkspaceRowAbsent}, tc.rows...),
				rowErrs:  append([]error{nil}, tc.rowErrs...),
				outcomes: tc.outcomes,
			}
			f.expectDatabaseExists("notifuse_ws_acme", false)
			f.expectCreate("notifuse_ws_acme")
			if tc.drop {
				f.expectDrop("notifuse_ws_acme").WillReturnResult(sqlmock.NewResult(0, 0))
			}
			// A retried insert that registered the workspace checks its database again:
			// by default, it finds the tables this creation gave it.
			defaulted := tc.exists == nil && tc.want == nil && tc.wantMsg == "" && tc.inserts > 1
			if defaulted {
				tc.exists = &yes
				f.holdsTables = true
			}
			if tc.exists != nil {
				f.expectDatabaseExists("notifuse_ws_acme", *tc.exists)
				if !*tc.exists {
					f.expectCreate("notifuse_ws_acme")
				}
			}
			logged := captureRepairLog(t)

			err := f.provisioner().create(ctx, provisioningConfig, "acme", rowFn(f, s), insertFn(f, s))
			switch {
			case tc.wantMsg != "":
				assert.ErrorContains(t, err, tc.wantMsg)
				assert.ErrorContains(t, err, "so its database notifuse_ws_acme was not dropped")
			case tc.want != nil:
				assert.ErrorIs(t, err, tc.want)
			default:
				require.NoError(t, err)
			}
			if err != nil && !tc.drop {
				// sqlmock fails an unexpected drop only through the error it returns.
				assert.NotContains(t, err.Error(), "could not be dropped")
				assert.NotContains(t, err.Error(), "all expectations were already fulfilled")
			}
			assert.Equal(t, tc.inserts, s.inserts)
			assert.Empty(t, *logged)
			if defaulted {
				assert.Equal(t, 1, f.initialized, "a database holding its tables is left as it is")
			}
			f.verify()
		})
	}

	// Once a row went and this creation's was inserted after it, the workspace is
	// registered: what fails while its database is checked again is only logged.
	rowWentThenOurs := func() *rowState {
		return &rowState{
			rows:     []WorkspaceRow{WorkspaceRowAbsent, WorkspaceRowAbsent},
			rowErrs:  []error{nil},
			outcomes: []insertAnswer{found},
		}
	}
	// A retried insert's row that is no longer this creation's at the check: the
	// creation fails, so its caller never takes the workspace, and no database is touched.
	for name, tc := range map[string]struct {
		after   WorkspaceRow
		found   bool // the retry found the row, its lookup said ours
		drop    bool
		want    error
		wantMsg string
	}{
		"removed since: its database dropped":       {after: WorkspaceRowAbsent, drop: true, wantMsg: "workspace acme was removed while it was being created"},
		"another creation's row since":              {after: WorkspaceRowOther, want: domain.ErrWorkspaceAlreadyExists},
		"another creation's row after it was found": {after: WorkspaceRowOther, found: true, want: domain.ErrWorkspaceAlreadyExists},
		"an unexpected answer":                      {after: WorkspaceRow(99), wantMsg: "unexpected answer 99 looking up workspace acme after its insert"},
	} {
		t.Run("a retried insert whose row was "+name+": refused, its database left alone", func(t *testing.T) {
			f := newProvisioningFixture(t)
			after := tc.after
			s := &rowState{rows: []WorkspaceRow{WorkspaceRowAbsent}, outcomes: []insertAnswer{{err: notSent}}, after: &after}
			if tc.found {
				// Lookups: before the insert, after the retry found the row, at the check.
				s = &rowState{rows: []WorkspaceRow{WorkspaceRowAbsent, WorkspaceRowOurs, tc.after}, outcomes: []insertAnswer{{err: lost}, {found: true}}}
			}
			f.expectDatabaseExists("notifuse_ws_acme", false)
			f.expectCreate("notifuse_ws_acme")
			if tc.drop {
				f.expectDrop("notifuse_ws_acme").WillReturnResult(sqlmock.NewResult(0, 0))
			}
			logged := captureRepairLog(t)

			err := f.provisioner().create(ctx, provisioningConfig, "acme", rowFn(f, s), insertFn(f, s))
			if tc.want != nil {
				assert.ErrorIs(t, err, tc.want)
			} else {
				assert.EqualError(t, err, tc.wantMsg)
			}
			// sqlmock fails an unexpected drop only through the error it returns.
			assert.NotContains(t, err.Error(), "could not be dropped")
			assert.NotContains(t, err.Error(), "all expectations were already fulfilled")
			assert.Equal(t, 2, s.inserts)
			assert.Empty(t, *logged)
			f.verify()
		})
	}
	t.Run("a retried insert whose row cannot be looked up before the check: logged, nothing touched", func(t *testing.T) {
		f := newProvisioningFixture(t)
		s := &rowState{rows: []WorkspaceRow{WorkspaceRowAbsent}, outcomes: []insertAnswer{{err: notSent}}, afterErr: errors.New("system database down")}
		f.expectDatabaseExists("notifuse_ws_acme", false)
		f.expectCreate("notifuse_ws_acme")
		logged := captureRepairLog(t)

		require.NoError(t, f.provisioner().create(ctx, provisioningConfig, "acme", rowFn(f, s), insertFn(f, s)))
		require.Len(t, *logged, 1)
		assert.ErrorContains(t, (*logged)[0], "system database down")
		f.verify()
	})
	t.Run("the bounds under the lock end in order: work and retries, check, drop", func(t *testing.T) {
		assert.Greater(t, lockWorkMargin, checkHoldMargin)
		assert.Greater(t, checkHoldMargin, cleanupHoldMargin)
		assert.Greater(t, cleanupHoldMargin, time.Duration(0))
	})
	t.Run("a row gone, then inserted: a failed database check is only logged", func(t *testing.T) {
		f := newProvisioningFixture(t)
		s := rowWentThenOurs()
		f.expectDatabaseExists("notifuse_ws_acme", false)
		f.expectCreate("notifuse_ws_acme")
		f.adminMock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM pg_database WHERE datname = \$1\)`).
			WithArgs("notifuse_ws_acme").WillReturnError(errors.New("admin connection reset"))
		logged := captureRepairLog(t)

		require.NoError(t, f.provisioner().create(ctx, provisioningConfig, "acme", rowFn(f, s), insertFn(f, s)))
		assert.Equal(t, 2, s.inserts)
		require.Len(t, *logged, 1)
		assert.ErrorContains(t, (*logged)[0], "admin connection reset")
		f.verify()
	})
	t.Run("a row gone, then inserted: a failed recreation is only logged, its database dropped", func(t *testing.T) {
		f := newProvisioningFixture(t)
		f.initErrs = []error{nil, errors.New("init broke")}
		s := rowWentThenOurs()
		f.expectDatabaseExists("notifuse_ws_acme", false)
		f.expectCreate("notifuse_ws_acme")
		f.expectDatabaseExists("notifuse_ws_acme", false)
		f.expectCreate("notifuse_ws_acme")
		f.expectDrop("notifuse_ws_acme").WillReturnResult(sqlmock.NewResult(0, 0))
		logged := captureRepairLog(t)

		require.NoError(t, f.provisioner().create(ctx, provisioningConfig, "acme", rowFn(f, s), insertFn(f, s)))
		assert.Equal(t, 2, f.initialized)
		require.Len(t, *logged, 1)
		assert.ErrorContains(t, (*logged)[0], "init broke")
		f.verify()
	})
	t.Run("a row gone, then inserted: an existing empty database gets its schema", func(t *testing.T) {
		f := newProvisioningFixture(t)
		f.hold = lockWorkMargin + time.Second // so the lock's bound limits the check
		s := rowWentThenOurs()
		f.expectDatabaseExists("notifuse_ws_acme", false)
		f.expectCreate("notifuse_ws_acme")
		f.expectDatabaseExists("notifuse_ws_acme", true)
		logged := captureRepairLog(t)

		require.NoError(t, f.provisioner().create(ctx, provisioningConfig, "acme", rowFn(f, s), insertFn(f, s)))
		assert.Equal(t, 1, f.holdsCalls)
		assert.Equal(t, 2, f.initialized)
		assert.Empty(t, *logged)
		assert.WithinDuration(t, f.holdDeadline.Add(-checkHoldMargin), f.initCtxDeadline, 100*time.Millisecond,
			"the check ends after the retries, before a drop")
		f.verify()
	})
	t.Run("a row gone, then inserted: a database holding tables is left as it is", func(t *testing.T) {
		f := newProvisioningFixture(t)
		f.holdsTables = true
		s := rowWentThenOurs()
		f.expectDatabaseExists("notifuse_ws_acme", false)
		f.expectCreate("notifuse_ws_acme")
		f.expectDatabaseExists("notifuse_ws_acme", true)
		logged := captureRepairLog(t)

		require.NoError(t, f.provisioner().create(ctx, provisioningConfig, "acme", rowFn(f, s), insertFn(f, s)))
		assert.Equal(t, 1, f.initialized, "only the first setup initialised")
		assert.Empty(t, *logged)
		f.verify()
	})

	t.Run("the pauses between attempts grow, and end with the retry bound", func(t *testing.T) {
		insertRetryDelays = []time.Duration{50 * time.Millisecond, time.Hour}
		defer func() { insertRetryDelays = []time.Duration{0, 0} }()
		f := newProvisioningFixture(t)
		// The retry bound ends lockWorkMargin before the lock's, with the work: 200ms from now.
		f.hold = lockWorkMargin + 200*time.Millisecond
		s := &rowState{rows: []WorkspaceRow{WorkspaceRowAbsent}, outcomes: []insertAnswer{{err: lost}, {err: lost}}}
		f.expectDatabaseExists("notifuse_ws_acme", false)
		f.expectCreate("notifuse_ws_acme")

		start := time.Now()
		err := f.provisioner().create(ctx, provisioningConfig, "acme", rowFn(f, s), insertFn(f, s))
		elapsed := time.Since(start)
		assert.ErrorContains(t, err, "no time left to insert again")
		assert.ErrorIs(t, err, context.DeadlineExceeded)
		assert.ErrorContains(t, err, "so its database notifuse_ws_acme was not dropped")
		assert.Equal(t, 2, s.inserts, "the first pause ran, the second ended with the bound")
		assert.GreaterOrEqual(t, elapsed, 150*time.Millisecond)
		assert.Less(t, elapsed, 5*time.Second)
		f.verify()
	})

	t.Run("retries get a bound of their own once the first attempt used up the caller's", func(t *testing.T) {
		defer func(saved time.Duration) { insertRetryTimeout = saved }(insertRetryTimeout)
		insertRetryTimeout = 300 * time.Millisecond
		f := newProvisioningFixture(t)
		s := &rowState{rows: []WorkspaceRow{WorkspaceRowAbsent, WorkspaceRowOurs}} // before the insert, then at the check
		f.expectDatabaseExists("notifuse_ws_acme", false)
		f.expectCreate("notifuse_ws_acme")
		attempts := 0
		var firstEnded time.Time
		insert := func(ctx context.Context) (bool, error) {
			attempts++
			if attempts == 1 {
				<-ctx.Done() // waits on a row lock until the caller's deadline
				firstEnded = time.Now()
				return false, errors.New("connection reset by peer")
			}
			deadline, ok := ctx.Deadline()
			require.True(t, ok)
			assert.WithinDuration(t, firstEnded.Add(insertRetryTimeout), deadline, 100*time.Millisecond, "the retry bound starts once the first attempt ended")
			return true, ctx.Err()
		}

		f.expectDatabaseExists("notifuse_ws_acme", true) // checked again after the retry
		logged := captureRepairLog(t)

		callerCtx, cancel := context.WithTimeout(ctx, 400*time.Millisecond)
		defer cancel()
		require.NoError(t, f.provisioner().create(callerCtx, provisioningConfig, "acme", rowFn(f, s), insert))
		assert.Equal(t, 2, attempts)
		assert.Empty(t, *logged)
		f.verify()
	})

	t.Run("the lookup after a first insert that used up the caller's bound runs on the retries'", func(t *testing.T) {
		defer func(saved time.Duration) { insertRetryTimeout = saved }(insertRetryTimeout)
		insertRetryTimeout = 300 * time.Millisecond
		f := newProvisioningFixture(t)
		f.expectDatabaseExists("notifuse_ws_acme", false)
		f.expectCreate("notifuse_ws_acme")
		lookups, inserts := 0, 0
		row := func(ctx context.Context) (WorkspaceRow, error) {
			lookups++
			if lookups == 1 {
				return WorkspaceRowAbsent, nil // the check before the insert
			}
			return WorkspaceRowOther, ctx.Err()
		}
		insert := func(ctx context.Context) (bool, error) {
			inserts++
			<-ctx.Done() // waits on another's insert until the caller's deadline, then finds its row
			return false, nil
		}

		callerCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
		defer cancel()
		err := f.provisioner().create(callerCtx, provisioningConfig, "acme", row, insert)
		assert.ErrorIs(t, err, domain.ErrWorkspaceAlreadyExists)
		assert.Equal(t, 1, inserts)
		f.verify()
	})

	t.Run("a first insert ending close to the work's bound still has its lookup", func(t *testing.T) {
		f := newProvisioningFixture(t)
		f.hold = lockWorkMargin + 500*time.Millisecond // the work ends 500ms from now
		f.expectDatabaseExists("notifuse_ws_acme", false)
		f.expectCreate("notifuse_ws_acme")
		lookups := 0
		row := func(ctx context.Context) (WorkspaceRow, error) {
			lookups++
			if lookups == 1 {
				return WorkspaceRowAbsent, nil
			}
			return WorkspaceRowOther, ctx.Err()
		}
		insert := func(ctx context.Context) (bool, error) {
			time.Sleep(200 * time.Millisecond) // waited on another's insert, then found its row
			return false, nil
		}

		err := f.provisioner().create(ctx, provisioningConfig, "acme", row, insert)
		assert.ErrorIs(t, err, domain.ErrWorkspaceAlreadyExists)
		f.verify()
	})

	t.Run("the drop gets a bound of its own once the retries used up theirs", func(t *testing.T) {
		defer func(saved time.Duration) { insertRetryTimeout = saved }(insertRetryTimeout)
		insertRetryTimeout = 100 * time.Millisecond
		insertRetryDelays = []time.Duration{time.Hour, time.Hour}
		defer func() { insertRetryDelays = []time.Duration{0, 0} }()
		f := newProvisioningFixture(t)
		s := &rowState{rows: []WorkspaceRow{WorkspaceRowAbsent}, outcomes: []insertAnswer{{err: readOnly}}}
		f.expectDatabaseExists("notifuse_ws_acme", false)
		f.expectCreate("notifuse_ws_acme")
		f.expectDrop("notifuse_ws_acme").WillReturnResult(sqlmock.NewResult(0, 0))

		err := f.provisioner().create(ctx, provisioningConfig, "acme", rowFn(f, s), insertFn(f, s))
		assert.ErrorIs(t, err, readOnly)
		assert.ErrorContains(t, err, "no time left to insert again")
		assert.NotContains(t, err.Error(), "could not be dropped")
		f.verify()
	})

	t.Run("retries ending with the lock's bound leave the drop its time", func(t *testing.T) {
		insertRetryDelays = []time.Duration{time.Hour, time.Hour}
		defer func() { insertRetryDelays = []time.Duration{0, 0} }()
		f := newProvisioningFixture(t)
		f.hold = lockWorkMargin + 200*time.Millisecond
		s := &rowState{rows: []WorkspaceRow{WorkspaceRowAbsent}, outcomes: []insertAnswer{{err: readOnly}}}
		f.expectDatabaseExists("notifuse_ws_acme", false)
		f.expectCreate("notifuse_ws_acme")
		f.expectDrop("notifuse_ws_acme").WillReturnResult(sqlmock.NewResult(0, 0))

		err := f.provisioner().create(ctx, provisioningConfig, "acme", rowFn(f, s), insertFn(f, s))
		assert.ErrorIs(t, err, readOnly)
		assert.ErrorContains(t, err, "no time left to insert again")
		assert.NotContains(t, err.Error(), "could not be dropped")
		f.verify()
	})

	t.Run("only a refusal for the values can never succeed", func(t *testing.T) {
		assert.False(t, insertCanNeverSucceed(&pq.Error{Message: "no SQLSTATE"}))
		assert.False(t, insertCanNeverSucceed(&pq.Error{Code: "2"}))
		assert.True(t, insertCanNeverSucceed(fmt.Errorf("insert: %w", &pq.Error{Code: "22001"})))
		assert.False(t, insertCanNeverSucceed(&pq.Error{Code: "42501"}), "a privilege can be granted between attempts")
		assert.False(t, insertCanNeverSucceed(&pq.Error{Code: "42703"}), "a column can be added between attempts")
		assert.False(t, insertCanNeverSucceed(&pq.Error{Code: "40P01"}))
		assert.False(t, insertCanNeverSucceed(&pq.Error{Code: "25006"}))
	})

	t.Run("creation's setup fails: dropped, no insert", func(t *testing.T) {
		f := newProvisioningFixture(t)
		f.initErr = errors.New("init broke")
		s := &rowState{rows: []WorkspaceRow{WorkspaceRowAbsent}}
		f.expectDatabaseExists("notifuse_ws_acme", false)
		f.expectCreate("notifuse_ws_acme")
		f.expectDrop("notifuse_ws_acme").WillReturnResult(sqlmock.NewResult(0, 0))

		err := f.provisioner().create(ctx, provisioningConfig, "acme", rowFn(f, s), insertFn(f, s))
		assert.ErrorIs(t, err, f.initErr)
		assert.Equal(t, 0, s.inserts)
		f.verify()
	})

	t.Run("invalid ID: the #477 errors before any connection", func(t *testing.T) {
		f := newProvisioningFixture(t)
		s := &rowState{rows: []WorkspaceRow{WorkspaceRowAbsent}}
		err := f.provisioner().create(ctx, provisioningConfig, "AcmeCo", rowFn(f, s), insertFn(f, s))
		assert.EqualError(t, err, "workspace database name must be lowercase: notifuse_ws_AcmeCo")
		assert.Equal(t, 0, f.lockAcquisitions)
	})
}

func TestWorkspaceDatabaseDSNWithConnectTimeout(t *testing.T) {
	dsn := lockWorkspaceDSN(provisioningConfig, "notifuse_ws_acme")
	assert.Equal(t, "postgres://u:p@h:5432/notifuse_ws_acme?connect_timeout=10&sslmode=disable", dsn)
}

func TestWorkspaceInsert_Run(t *testing.T) {
	ctx := context.Background()
	statement := regexp.QuoteMeta(`INSERT INTO workspaces ("id", "name") VALUES ($1, $2) ON CONFLICT (id) DO NOTHING`)
	t.Run("id alone", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO workspaces ("id") VALUES ($1) ON CONFLICT (id) DO NOTHING`)).WithArgs("acme").
			WillReturnResult(sqlmock.NewResult(0, 1))
		run, err := WorkspaceInsert{DB: db}.prepare("acme")
		require.NoError(t, err)
		inserted, err := run(ctx)
		require.NoError(t, err)
		assert.True(t, inserted)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
	insertOn := func(t *testing.T, db *sql.DB) func(ctx context.Context) (bool, error) {
		run, err := WorkspaceInsert{DB: db, Columns: []string{"name"}, Values: []any{"Acme"}}.prepare("acme")
		require.NoError(t, err)
		return run
	}

	for name, tc := range map[string]struct {
		affected int64
		inserted bool
	}{"inserted": {1, true}, "a row already there": {0, false}} {
		t.Run(name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() { _ = db.Close() }()
			mock.ExpectExec(statement).WithArgs("acme", "Acme").WillReturnResult(sqlmock.NewResult(0, tc.affected))

			inserted, err := insertOn(t, db)(ctx)
			require.NoError(t, err)
			assert.Equal(t, tc.inserted, inserted)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}

	t.Run("a statement error comes back as it is", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		refused := &pq.Error{Code: "23502"}
		mock.ExpectExec(statement).WillReturnError(refused)

		_, err = insertOn(t, db)(ctx)
		assert.Same(t, refused, err)
		assert.NotErrorIs(t, err, errInsertNotSent)
	})

	t.Run("no connection: not sent", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		mock.ExpectClose()
		require.NoError(t, db.Close())

		_, err = insertOn(t, db)(ctx)
		assert.ErrorIs(t, err, errInsertNotSent)
	})

	t.Run("a pooled connection found closed is replaced, the statement sent once", func(t *testing.T) {
		const stale = 3
		connector := &staleConnector{stale: stale}
		db := sql.OpenDB(connector)
		defer func() { _ = db.Close() }()
		fillWithIdleConnections(t, db, stale)

		inserted, err := insertOn(t, db)(ctx)
		require.NoError(t, err)
		assert.True(t, inserted)
		assert.Equal(t, int32(stale+1), connector.opened.Load(), "every closed connection was replaced")
		assert.Equal(t, int32(1), connector.execs.Load(), "the statement runs once, on the live connection")
	})

	t.Run("an invalid insert is refused before anything", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		for _, tc := range []struct {
			insert WorkspaceInsert
			want   string
		}{
			{WorkspaceInsert{Columns: []string{"name"}, Values: []any{"Acme"}}, "no database"},
			{WorkspaceInsert{DB: db, Columns: []string{"name"}}, "1 columns, 0 values"},
			{WorkspaceInsert{DB: db, Columns: []string{"name"}, Values: []any{"Acme", "Acme"}}, "1 columns, 2 values"},
			{WorkspaceInsert{DB: db, Columns: []string{"id"}, Values: []any{"acme"}}, "column id, set from the workspace ID"},
			{WorkspaceInsert{DB: db, Columns: []string{"ID"}, Values: []any{"acme"}}, "column ID, set from the workspace ID"},
			{WorkspaceInsert{DB: db, Columns: []string{"Name"}, Values: []any{"Acme"}}, `column "Name" is not a lower-case name`},
			{WorkspaceInsert{DB: db, Columns: []string{""}, Values: []any{"Acme"}}, `column "" is not a lower-case name`},
			{WorkspaceInsert{DB: db, Columns: []string{"name; DROP"}, Values: []any{"Acme"}}, `column "name; DROP" is not a lower-case name`},
			{WorkspaceInsert{DB: db, Columns: []string{"name", "name"}, Values: []any{"a", "b"}}, "column name set twice"},
		} {
			want := "invalid workspace insert: " + tc.want
			_, err := tc.insert.prepare("acme")
			assert.EqualError(t, err, want)
			assert.EqualError(t, CreateWorkspaceDatabase(ctx, provisioningConfig, "acme", nil, tc.insert), want)
		}
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}
