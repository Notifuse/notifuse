package database

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
)

// expectLockTransactionSettings queues the statements every lock transaction runs right
// after BEGIN: the four server timeouts reset, then the TCP keepalives.
func expectLockTransactionSettings(mock sqlmock.Sqlmock) {
	mock.ExpectExec(`SET LOCAL statement_timeout = 0`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`SET LOCAL lock_timeout = 0`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`SET LOCAL idle_in_transaction_session_timeout = 0`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`SELECT set_config\('transaction_timeout', '0', true\) WHERE current_setting\('transaction_timeout', true\) IS NOT NULL`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`SET LOCAL tcp_keepalives_idle = 30`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`SET LOCAL tcp_keepalives_interval = 10`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`SET LOCAL tcp_keepalives_count = 3`).WillReturnResult(sqlmock.NewResult(0, 0))
}

func expectWorkspaceNameLock(mock sqlmock.Sqlmock, dbName string) {
	mock.ExpectExec(`SELECT pg_advisory_xact_lock\(\$1::integer, hashtext\(\$2\)\)`).
		WithArgs(int64(WorkspaceDatabaseLockClass), dbName).
		WillReturnResult(sqlmock.NewResult(0, 0))
}

func TestWorkspaceDatabaseLockClassFitsInt4(t *testing.T) {
	// The two-key advisory lock takes int4 keys: a class above 2^31-1 fails at runtime,
	// and sqlmock would not notice. The type pins it; this pins it differs from the
	// secret-key rotation lock's single-bigint key.
	var class int32 = WorkspaceDatabaseLockClass
	assert.NotZero(t, class)
	assert.NotEqual(t, int64(4_242_000_001), int64(class))
}

func TestWithWorkspaceDatabaseLockOn(t *testing.T) {
	t.Run("takes the name lock after the settings, runs fn, then commits", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectBegin()
		expectLockTransactionSettings(mock)
		expectWorkspaceNameLock(mock, "notifuse_ws_acme")
		mock.ExpectExec(`SELECT 'work'`).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectCommit()

		var gotAdmin *sql.DB
		err = withWorkspaceDatabaseLockOn(context.Background(), db, "notifuse_ws_acme", func(ctx context.Context, admin *sql.DB) error {
			gotAdmin = admin
			_, err := admin.ExecContext(ctx, `SELECT 'work'`)
			return err
		})
		require.NoError(t, err)
		assert.Same(t, db, gotAdmin)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestWithLockTransaction(t *testing.T) {
	acquireWith := func(query string) func(ctx context.Context, tx *sql.Tx) error {
		return func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, query)
			return err
		}
	}

	t.Run("fn gets a deadline a minute short of the lock transaction's", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectBegin()
		expectLockTransactionSettings(mock)
		mock.ExpectExec(`SELECT lock`).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectCommit()

		start := time.Now()
		err = WithLockTransaction(context.Background(), db, acquireWith(`SELECT lock`), func(ctx context.Context) error {
			deadline, ok := ctx.Deadline()
			require.True(t, ok)
			assert.WithinDuration(t, start.Add(lockHoldTimeout-lockWorkMargin), deadline, 5*time.Second)
			hold, ok := lockHoldDeadline(ctx)
			require.True(t, ok)
			assert.WithinDuration(t, start.Add(lockHoldTimeout), hold, 5*time.Second)
			assert.True(t, deadline.Before(hold))
			return nil
		})
		require.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("fn keeps the caller's earlier deadline", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectBegin()
		expectLockTransactionSettings(mock)
		mock.ExpectExec(`SELECT lock`).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectCommit()

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		callerDeadline, _ := ctx.Deadline()
		err = WithLockTransaction(ctx, db, acquireWith(`SELECT lock`), func(ctx context.Context) error {
			deadline, ok := ctx.Deadline()
			require.True(t, ok)
			assert.Equal(t, callerDeadline, deadline)
			return nil
		})
		require.NoError(t, err)
	})

	t.Run("fn error rolls back and is returned", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectBegin()
		expectLockTransactionSettings(mock)
		mock.ExpectExec(`SELECT lock`).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectRollback()

		boom := errors.New("boom")
		err = WithLockTransaction(context.Background(), db, acquireWith(`SELECT lock`), func(ctx context.Context) error {
			return boom
		})
		assert.ErrorIs(t, err, boom)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("a panicking fn rolls back and the panic goes on", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectBegin()
		expectLockTransactionSettings(mock)
		mock.ExpectExec(`SELECT lock`).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectRollback()

		assert.PanicsWithValue(t, "kaboom", func() {
			_ = WithLockTransaction(context.Background(), db, acquireWith(`SELECT lock`), func(ctx context.Context) error {
				panic("kaboom")
			})
		})
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("a lock error never calls fn and rolls back", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectBegin()
		expectLockTransactionSettings(mock)
		mock.ExpectExec(`SELECT lock`).WillReturnError(context.Canceled)
		mock.ExpectRollback()

		called := false
		err = WithLockTransaction(context.Background(), db, acquireWith(`SELECT lock`), func(ctx context.Context) error {
			called = true
			return nil
		})
		assert.ErrorIs(t, err, context.Canceled)
		assert.False(t, called)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	// lib/pq can return nil from a lock statement whose context ended as the lock was
	// granted, after closing the connection and with it the lock: such a lock is not held.
	t.Run("a lock granted as the caller's context ends is not trusted", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectBegin()
		expectLockTransactionSettings(mock)
		mock.ExpectExec(`SELECT lock`).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectRollback()

		ctx, cancel := context.WithCancel(context.Background())
		called := false
		err = WithLockTransaction(ctx, db, func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `SELECT lock`)
			cancel()
			return err
		}, func(ctx context.Context) error {
			called = true
			return nil
		})
		assert.ErrorIs(t, err, context.Canceled)
		assert.False(t, called)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("a caller cancelled while fn runs does not end the lock transaction", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectBegin()
		expectLockTransactionSettings(mock)
		mock.ExpectExec(`SELECT lock`).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectCommit()

		ctx, cancel := context.WithCancel(context.Background())
		err = WithLockTransaction(ctx, db, acquireWith(`SELECT lock`), func(ctx context.Context) error {
			cancel()
			// database/sql rolls a transaction back when its BeginTx context ends; give
			// it the chance to, had the lock transaction been begun on the caller's.
			time.Sleep(50 * time.Millisecond)
			return nil
		})
		require.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("a commit error after fn succeeded is logged, not returned", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectBegin()
		expectLockTransactionSettings(mock)
		mock.ExpectExec(`SELECT lock`).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectCommit().WillReturnError(errors.New("connection reset"))

		var logged error
		lapsed := true
		previous := logLockReleaseError
		logLockReleaseError = func(err error, l bool) { logged, lapsed = err, l }
		defer func() { logLockReleaseError = previous }()

		err = WithLockTransaction(context.Background(), db, acquireWith(`SELECT lock`), func(ctx context.Context) error {
			return nil
		})
		require.NoError(t, err)
		require.Error(t, logged)
		assert.Contains(t, logged.Error(), "connection reset")
		assert.False(t, lapsed, "the lock was still within its bound")
	})

	// Work that ignores its context does not keep the lock past the hold bound: the
	// lock transaction is rolled back when it fires, and the late commit is reported.
	t.Run("the hold bound rolls the lock transaction back", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectBegin()
		expectLockTransactionSettings(mock)
		mock.ExpectExec(`SELECT lock`).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectRollback()

		var logged error
		lapsed := false
		previous := logLockReleaseError
		logLockReleaseError = func(err error, l bool) { logged, lapsed = err, l }
		defer func() { logLockReleaseError = previous }()

		const hold = 200 * time.Millisecond
		err = withLockTransaction(context.Background(), db, lockBounds{prepare: time.Minute, hold: hold}, acquireWith(`SELECT lock`), func(ctx context.Context) error {
			time.Sleep(3 * hold)
			return nil
		})
		require.NoError(t, err, "the work's own result")
		require.Error(t, logged)
		assert.True(t, lapsed)
		assert.NoError(t, mock.ExpectationsWereMet(), "rolled back by the bound")
	})

	t.Run("the hold bound counts from the grant, not from the wait", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectBegin()
		expectLockTransactionSettings(mock)
		mock.ExpectExec(`SELECT lock`).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectCommit()

		const wait = 300 * time.Millisecond
		var granted time.Time
		acquire := func(ctx context.Context, tx *sql.Tx) error {
			time.Sleep(wait) // the wait for another holder
			_, err := tx.ExecContext(ctx, `SELECT lock`)
			granted = time.Now()
			return err
		}
		err = WithLockTransaction(context.Background(), db, acquire, func(ctx context.Context) error {
			hold, ok := lockHoldDeadline(ctx)
			require.True(t, ok)
			assert.False(t, hold.Before(granted.Add(lockHoldTimeout)), "hold %v is earlier than grant + bound", hold)
			deadline, _ := ctx.Deadline()
			assert.Equal(t, hold.Add(-lockWorkMargin), deadline)
			return nil
		})
		require.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("a preparation that overruns its bound never takes the lock", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectBegin()
		mock.ExpectExec(`SET LOCAL statement_timeout = 0`).WillDelayFor(time.Second).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectRollback()

		start := time.Now()
		err = withLockTransaction(context.Background(), db, lockBounds{prepare: 100 * time.Millisecond, hold: time.Minute}, acquireWith(`SELECT lock`), func(ctx context.Context) error {
			t.Fatal("fn must not run")
			return nil
		})
		require.ErrorContains(t, err, "failed to prepare lock transaction")
		assert.Less(t, time.Since(start), 900*time.Millisecond, "the statement is cancelled at the bound")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("a pool with no free connection fails at the preparation bound", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		db.SetMaxOpenConns(1)
		busy, err := db.Conn(context.Background())
		require.NoError(t, err)
		defer func() { _ = busy.Close() }()

		start := time.Now()
		err = withLockTransaction(context.Background(), db, lockBounds{prepare: 100 * time.Millisecond, hold: time.Minute}, acquireWith(`SELECT lock`), func(ctx context.Context) error {
			t.Fatal("fn must not run")
			return nil
		})
		require.ErrorContains(t, err, "failed to get a connection for the lock transaction")
		assert.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Less(t, time.Since(start), 900*time.Millisecond)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("pooled connections found closed are replaced", func(t *testing.T) {
		const stale = 4
		c := &staleConnector{stale: stale}
		db := sql.OpenDB(c)
		defer func() { _ = db.Close() }()
		fillWithIdleConnections(t, db, stale)

		ran := false
		err := WithLockTransaction(context.Background(), db, acquireWith(`SELECT lock`), func(ctx context.Context) error {
			ran = true
			return nil
		})
		require.NoError(t, err)
		assert.True(t, ran)
		assert.Equal(t, int32(stale+1), c.opened.Load(), "every closed connection was replaced")
		assert.Equal(t, int32(1), c.committed.Load())
	})

	// None idle when the attempts are counted, yet closed connections met meanwhile
	// (here, ones the server closes as they open): a few attempts more cover them.
	t.Run("with no idle connection, closed ones are still replaced", func(t *testing.T) {
		c := &staleConnector{stale: 2}
		db := sql.OpenDB(c)
		defer func() { _ = db.Close() }()
		require.Equal(t, 0, db.Stats().Idle)

		err := WithLockTransaction(context.Background(), db, acquireWith(`SELECT lock`), func(ctx context.Context) error { return nil })
		require.NoError(t, err)
		assert.Equal(t, int32(3), c.opened.Load())
		assert.Equal(t, int32(1), c.committed.Load())
	})

	t.Run("a connection reset between its ping and BEGIN is replaced", func(t *testing.T) {
		c := &staleConnector{}
		c.failBegins.Store(1)
		db := sql.OpenDB(c)
		defer func() { _ = db.Close() }()

		err := WithLockTransaction(context.Background(), db, acquireWith(`SELECT lock`), func(ctx context.Context) error { return nil })
		require.NoError(t, err)
		assert.Equal(t, int32(2), c.opened.Load())
		assert.Equal(t, int32(1), c.committed.Load())
	})

	t.Run("BEGIN failing on a second live connection too gives up", func(t *testing.T) {
		c := &staleConnector{}
		c.failBegins.Store(2)
		db := sql.OpenDB(c)
		defer func() { _ = db.Close() }()

		err := WithLockTransaction(context.Background(), db, acquireWith(`SELECT lock`), func(ctx context.Context) error { return nil })
		require.ErrorContains(t, err, "failed to begin lock transaction")
		assert.ErrorIs(t, err, errConnectionReset)
		assert.Contains(t, err.Error(), "; then ")
		assert.Equal(t, int32(2), c.opened.Load())
		assert.Equal(t, int32(0), c.committed.Load())
	})

	t.Run("a BEGIN refused by the server is not tried again", func(t *testing.T) {
		c := &staleConnector{refuseBegin: &pq.Error{Code: "XX000", Message: "internal error"}}
		db := sql.OpenDB(c)
		defer func() { _ = db.Close() }()

		err := WithLockTransaction(context.Background(), db, acquireWith(`SELECT lock`), func(ctx context.Context) error { return nil })
		require.ErrorContains(t, err, "failed to begin lock transaction: pq: internal error")
		assert.Equal(t, int32(1), c.begins.Load())
	})

	t.Run("no other connection after a failed BEGIN: both failures reported", func(t *testing.T) {
		c := &staleConnector{maxOpens: 1}
		c.failBegins.Store(1)
		db := sql.OpenDB(c)
		defer func() { _ = db.Close() }()

		err := WithLockTransaction(context.Background(), db, acquireWith(`SELECT lock`), func(ctx context.Context) error { return nil })
		require.ErrorContains(t, err, "failed to begin lock transaction")
		assert.ErrorIs(t, err, errConnectionReset)
		assert.ErrorIs(t, err, errConnectorExhausted)
		assert.ErrorContains(t, err, "then failed to get another connection")
	})

	t.Run("a settings error never takes the lock", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectBegin()
		mock.ExpectExec(`SET LOCAL statement_timeout = 0`).WillReturnError(errors.New("denied"))
		mock.ExpectRollback()

		err = WithLockTransaction(context.Background(), db, acquireWith(`SELECT lock`), func(ctx context.Context) error {
			t.Fatal("fn must not run")
			return nil
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "denied")
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestCleanupContext(t *testing.T) {
	t.Run("ignores an expired caller deadline", func(t *testing.T) {
		parent, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		defer cancel()
		ctx, cancelCleanup := cleanupContext(parent)
		defer cancelCleanup()
		require.NoError(t, ctx.Err())
		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		assert.WithinDuration(t, time.Now().Add(dropDatabaseTimeout), deadline, 5*time.Second)
	})

	t.Run("stays below the lock transaction's deadline", func(t *testing.T) {
		hold := time.Now().Add(2 * time.Minute)
		parent := withLockHoldDeadline(context.Background(), hold)
		ctx, cancel := cleanupContext(parent)
		defer cancel()
		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		assert.Equal(t, hold.Add(-cleanupHoldMargin), deadline)
	})
}

func TestDetachedUnderLock(t *testing.T) {
	t.Run("ignores the caller's expired deadline and cancellation", func(t *testing.T) {
		parent, cancel := context.WithCancel(context.Background())
		cancel()
		ctx, cancelWork := DetachedUnderLock(parent, time.Minute)
		defer cancelWork()
		require.NoError(t, ctx.Err())
		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		assert.WithinDuration(t, time.Now().Add(time.Minute), deadline, 5*time.Second)
	})

	t.Run("ends before the lock transaction", func(t *testing.T) {
		hold := time.Now().Add(3 * time.Minute)
		ctx, cancel := DetachedUnderLock(withLockHoldDeadline(context.Background(), hold), 10*time.Minute)
		defer cancel()
		deadline, _ := ctx.Deadline()
		assert.Equal(t, hold.Add(-cleanupHoldMargin), deadline)
	})
}

func TestWithConnectTimeout(t *testing.T) {
	assert.Equal(t, "postgres://u:p@h:5432/db?connect_timeout=10&sslmode=disable", WithConnectTimeout("postgres://u:p@h:5432/db?sslmode=disable"))
	assert.Equal(t, "postgres://u:p@h:5432/db?connect_timeout=10", WithConnectTimeout("postgres://u:p@h:5432/db"))
	assert.Equal(t, "postgres://u:pa%3Fss@h:5432/db?connect_timeout=10", WithConnectTimeout("postgres://u:pa%3Fss@h:5432/db"), "an escaped password stays escaped")
	assert.Equal(t, "postgres://u:p%zz@h/db", WithConnectTimeout("postgres://u:p%zz@h/db"), "a DSN that does not parse is left for lib/pq to refuse")
}

func TestLockAdminDSN(t *testing.T) {
	cfg := &config.DatabaseConfig{User: "u", Password: "p", Host: "h", Port: 5432, SSLMode: "disable"}
	dsn := lockAdminDSN(cfg)
	assert.Equal(t, WithConnectTimeout(GetPostgresDSN(cfg)), dsn)
	assert.Equal(t, "postgres://u:p@h:5432/postgres?connect_timeout=10&sslmode=disable", dsn)
}

func TestWithWorkspaceDatabaseLock_Unreachable(t *testing.T) {
	cfg := &config.DatabaseConfig{User: "invalid", Password: "invalid", Host: "nonexistent-host", Port: 9999, SSLMode: "disable"}
	called := false
	err := WithWorkspaceDatabaseLock(context.Background(), cfg, "x_ws_y", func(ctx context.Context, admin *sql.DB) error {
		called = true
		return nil
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to ping PostgreSQL server")
	assert.False(t, called)
}

// staleConnector hands out connections, the first stale of which are closed by the
// server: their ping fails as such a connection's does.
type staleConnector struct {
	stale                    int32
	opened, committed, execs atomic.Int32
	failBegins               atomic.Int32 // BEGINs on live connections that meet a reset
	begins                   atomic.Int32
	refuseBegin              error // every BEGIN refused so by the server
	maxOpens                 int32 // no connection opens beyond this many; zero means no limit
}

// errConnectionReset is what lib/pq returns for a connection reset mid-statement: not
// driver.ErrBadConn, though lib/pq then reports the connection invalid, so database/sql
// discards it.
var errConnectionReset = errors.New("read tcp: connection reset by peer")

var errConnectorExhausted = errors.New("dial tcp: connection refused")

// fillWithIdleConnections opens n connections on db and hands them back to its pool.
func fillWithIdleConnections(t *testing.T, db *sql.DB, n int) {
	t.Helper()
	db.SetMaxIdleConns(n)
	var conns []*sql.Conn
	for i := 0; i < n; i++ {
		conn, err := db.Conn(context.Background())
		require.NoError(t, err)
		conns = append(conns, conn)
	}
	for _, conn := range conns {
		require.NoError(t, conn.Close())
	}
	require.Equal(t, n, db.Stats().Idle)
}

func (c *staleConnector) Connect(context.Context) (driver.Conn, error) {
	if c.maxOpens > 0 && c.opened.Load() >= c.maxOpens {
		return nil, errConnectorExhausted
	}
	n := c.opened.Add(1)
	return &staleConn{c: c, closed: n <= c.stale}, nil
}

func (c *staleConnector) Driver() driver.Driver { return nil }

type staleConn struct {
	c      *staleConnector
	closed bool // by the server, unnoticed
	broken bool // by a reset the driver saw
}

func (n *staleConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("not supported") }
func (n *staleConn) Close() error                        { return nil }
func (n *staleConn) Begin() (driver.Tx, error)           { return nil, errors.New("not supported") }

func (n *staleConn) Ping(context.Context) error {
	if n.closed {
		return driver.ErrBadConn
	}
	return nil
}

// IsValid reports a connection broken by a reset as lib/pq does. One the server closed
// unnoticed still looks valid until it is used.
func (n *staleConn) IsValid() bool { return !n.broken }

func (n *staleConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	if n.closed {
		return nil, driver.ErrBadConn
	}
	n.c.begins.Add(1)
	if n.c.refuseBegin != nil {
		return nil, n.c.refuseBegin
	}
	if n.c.failBegins.Add(-1) >= 0 {
		n.broken = true
		return nil, errConnectionReset
	}
	return staleTx{c: n.c}, nil
}

func (n *staleConn) ExecContext(context.Context, string, []driver.NamedValue) (driver.Result, error) {
	if n.closed {
		return nil, driver.ErrBadConn
	}
	n.c.execs.Add(1)
	return driver.RowsAffected(1), nil
}

type staleTx struct{ c *staleConnector }

func (t staleTx) Commit() error   { t.c.committed.Add(1); return nil }
func (t staleTx) Rollback() error { return nil }
