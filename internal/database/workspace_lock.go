package database

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// WorkspaceDatabaseLockClass is the first key of the workspace name lock,
// pg_advisory_xact_lock(class, hashtext(database name)). The two-key form takes int4
// keys, hence int32. It cannot collide with the secret-key rotation lock, which uses
// the single-bigint form on the system database.
const WorkspaceDatabaseLockClass int32 = 424_200_002

const (
	// lockHoldTimeout bounds how long a lock is held, from its grant. database/sql rolls
	// the lock transaction back when it fires, which releases the lock.
	lockHoldTimeout = 10 * time.Minute
	// lockWorkMargin is how much earlier than the lock transaction the work under it
	// must end, so the lock never goes while its work still runs.
	lockWorkMargin = time.Minute
	// dropDatabaseTimeout bounds a cleanup drop, as it bounds the workspace deletion's.
	dropDatabaseTimeout = 5 * time.Minute
	// cleanupHoldMargin keeps a cleanup drop ending before its lock transaction does.
	cleanupHoldMargin = 30 * time.Second
	// lockPrepareTimeout bounds the statements that prepare a lock transaction, before
	// the wait for the lock.
	lockPrepareTimeout = time.Minute
	// lockConnectTimeoutSeconds bounds the whole connection establishment of the paths
	// that take the name lock: lib/pq's context only bounds the TCP dial, not TLS or
	// the startup exchange.
	lockConnectTimeoutSeconds = 10
)

// lockTransactionSettings run right after BEGIN on every lock transaction. No server
// setting may end the holder early or abort its wait: the bounds come from the
// contexts. transaction_timeout exists from PostgreSQL 17 only, hence the guard. The
// keepalives make the server notice a holder whose node disappeared within about a
// minute instead of about two hours (ignored on a Unix socket).
var lockTransactionSettings = []string{
	`SET LOCAL statement_timeout = 0`,
	`SET LOCAL lock_timeout = 0`,
	`SET LOCAL idle_in_transaction_session_timeout = 0`,
	`SELECT set_config('transaction_timeout', '0', true) WHERE current_setting('transaction_timeout', true) IS NOT NULL`,
	`SET LOCAL tcp_keepalives_idle = 30`,
	`SET LOCAL tcp_keepalives_interval = 10`,
	`SET LOCAL tcp_keepalives_count = 3`,
}

// logLockReleaseError reports a lock transaction that could not be committed after its
// work succeeded. The work is done and the server releases the lock with the
// transaction or the connection anyway, so it is not returned. lapsed is true when the
// lock transaction's own bound had ended it: the lock went before the work under it
// returned. A variable for tests.
var logLockReleaseError = func(err error, lapsed bool) {
	if lapsed {
		logger.NewLogger().WithField("error", err.Error()).Error("A database advisory lock was released before the work under it ended")
		return
	}
	logger.NewLogger().WithField("error", err.Error()).Warn("Failed to release a database advisory lock cleanly; the server releases it with the connection")
}

type lockHoldDeadlineKey struct{}

func withLockHoldDeadline(ctx context.Context, deadline time.Time) context.Context {
	return context.WithValue(ctx, lockHoldDeadlineKey{}, deadline)
}

// lockHoldDeadline returns the deadline of the lock transaction ctx runs under.
func lockHoldDeadline(ctx context.Context) (time.Time, bool) {
	deadline, ok := ctx.Value(lockHoldDeadlineKey{}).(time.Time)
	return deadline, ok
}

// DetachedUnderLock returns a context for work under a lock that must not be cut off by
// the caller's deadline or cancellation, such as a drop, or the row deletion that has to
// follow one: it ends after max, or 30 seconds before the lock transaction ctx runs
// under, whichever comes first, so the work never outlives the lock.
func DetachedUnderLock(ctx context.Context, max time.Duration) (context.Context, context.CancelFunc) {
	return detachedBefore(ctx, max, cleanupHoldMargin)
}

// detachedBefore is DetachedUnderLock ending margin before the lock's hold bound.
func detachedBefore(ctx context.Context, max, margin time.Duration) (context.Context, context.CancelFunc) {
	deadline := time.Now().Add(max)
	if hold, ok := lockHoldDeadline(ctx); ok {
		if limit := hold.Add(-margin); limit.Before(deadline) {
			deadline = limit
		}
	}
	return context.WithDeadline(context.WithoutCancel(ctx), deadline)
}

// cleanupContext returns the context of a drop after a failure under a lock: the failure
// may be the caller's deadline passing, and a drop under it would be refused before it
// is sent.
func cleanupContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return DetachedUnderLock(ctx, dropDatabaseTimeout)
}

// WithLockTransaction runs fn while a transaction-level advisory lock taken by acquire
// is held by a transaction of its own on db. fn runs its statements on other
// connections.
//
// The lock transaction is begun detached from ctx and only its preparation and acquire
// run under ctx: a cancelled caller stops waiting but never releases a lock whose work
// still runs (database/sql rolls a transaction back the moment its BeginTx context
// ends). A lock reported granted while ctx ended is not trusted: lib/pq may have closed
// the connection, and the lock with it. Once granted, the lock is held for at most
// lockHoldTimeout, counted from the grant, so a long wait never eats into the work.
// fn gets a context ending lockWorkMargin before that bound, and by ctx's own deadline;
// work that must outlive ctx's deadline detaches with DetachedUnderLock.
//
// Code running under the lock never calls a function that takes it again: another
// connection of the same process would wait on itself until its context ends.
func WithLockTransaction(ctx context.Context, db *sql.DB, acquire func(ctx context.Context, tx *sql.Tx) error, fn func(ctx context.Context) error) error {
	return withLockTransaction(ctx, db, lockBounds{prepare: lockPrepareTimeout, hold: lockHoldTimeout}, acquire, fn)
}

// lockBounds are the bounds of a lock transaction: prepare covers the settings, before
// the wait; hold covers the lock, from its grant.
type lockBounds struct {
	prepare time.Duration
	hold    time.Duration
}

func withLockTransaction(ctx context.Context, db *sql.DB, bounds lockBounds, acquire func(ctx context.Context, tx *sql.Tx) error, fn func(ctx context.Context) error) error {
	// Getting a connection and the settings are bounded by prepare and ctx (lib/pq
	// does not interrupt BEGIN itself), the wait by ctx, and the lock by hold from its
	// grant, when holdCtx is cancelled.
	prepareCtx, cancelPrepare := context.WithTimeout(ctx, bounds.prepare)
	defer cancelPrepare()
	holdCtx, cancelHold := context.WithCancel(context.WithoutCancel(ctx))
	defer cancelHold()

	conn, tx, err := beginOnOwnConnection(prepareCtx, holdCtx, db)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	finished := false
	defer func() {
		if !finished {
			_ = tx.Rollback()
		}
	}()

	for _, statement := range lockTransactionSettings {
		if _, err := tx.ExecContext(prepareCtx, statement); err != nil {
			return fmt.Errorf("failed to prepare lock transaction: %w", err)
		}
	}
	// lib/pq may close the connection when the bound ends as a statement returns.
	if err := prepareCtx.Err(); err != nil {
		return fmt.Errorf("failed to prepare lock transaction: %w", err)
	}
	cancelPrepare()

	if err := acquire(ctx, tx); err != nil {
		return err
	}
	// Once acquire has returned, lib/pq's cancel watcher has settled: a nil ctx.Err()
	// here means the connection, and the lock, were kept.
	if err := ctx.Err(); err != nil {
		return err
	}

	holdDeadline := time.Now().Add(bounds.hold)
	holdTimer := time.AfterFunc(time.Until(holdDeadline), cancelHold)
	defer holdTimer.Stop()

	workCtx, cancelWork := context.WithDeadline(withLockHoldDeadline(ctx, holdDeadline), holdDeadline.Add(-lockWorkMargin))
	defer cancelWork()
	if err := fn(workCtx); err != nil {
		return err
	}

	finished = true
	if err := tx.Commit(); err != nil {
		// Only the hold bound ends the transaction before this commit.
		logLockReleaseError(err, holdCtx.Err() != nil)
	}
	return nil
}

// beginOnOwnConnection begins a transaction on holdCtx, on a live connection of its own
// taken from db within getCtx. A BEGIN that fails on the connection itself, which dies
// between its ping and BEGIN, is tried once more, on another since the driver discards
// a broken connection; a refusal by the server is not. BEGIN changes nothing either way.
func beginOnOwnConnection(getCtx, holdCtx context.Context, db *sql.DB) (*sql.Conn, *sql.Tx, error) {
	var beginErr error
	for {
		conn, err := liveConnection(getCtx, db)
		if err != nil {
			if beginErr != nil {
				return nil, nil, fmt.Errorf("failed to begin lock transaction: %w", thenErr(beginErr, fmt.Errorf("failed to get another connection: %w", err)))
			}
			return nil, nil, fmt.Errorf("failed to get a connection for the lock transaction: %w", err)
		}
		tx, err := conn.BeginTx(holdCtx, nil)
		if err == nil {
			return conn, tx, nil
		}
		_ = conn.Close()
		if beginErr != nil || refusedByServer(err) {
			return nil, nil, fmt.Errorf("failed to begin lock transaction: %w", thenErr(beginErr, err))
		}
		beginErr = err
	}
}

// liveConnection takes a connection of its own from db and pings it. A pooled
// connection found closed (the server restarted, a proxy closed it) fails its ping and
// is discarded; there are attempts enough to go through every connection idle at the
// start, and a few more for ones handed back meanwhile.
func liveConnection(ctx context.Context, db *sql.DB) (*sql.Conn, error) {
	attempts := db.Stats().Idle + 3
	for attempt := 1; ; attempt++ {
		conn, err := db.Conn(ctx)
		if err != nil {
			return nil, err
		}
		err = conn.PingContext(ctx)
		if err == nil {
			return conn, nil
		}
		_ = conn.Close()
		if !errors.Is(err, driver.ErrBadConn) || attempt >= attempts {
			return nil, err
		}
	}
}

// WithConnectTimeout adds connect_timeout to a URL DSN: lib/pq's context only bounds the
// TCP dial, while connect_timeout bounds the whole establishment, TLS and startup
// included. A DSN that does not parse is returned as it is, for lib/pq to refuse.
func WithConnectTimeout(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil {
		return dsn
	}
	query := u.Query()
	query.Set("connect_timeout", strconv.Itoa(lockConnectTimeoutSeconds))
	u.RawQuery = query.Encode()
	return u.String()
}

// lockAdminDSN is the admin DSN of the paths that take the name lock. GetPostgresDSN
// stays as it is for its other callers.
func lockAdminDSN(cfg *config.DatabaseConfig) string {
	return WithConnectTimeout(GetPostgresDSN(cfg))
}

// WithWorkspaceDatabaseLock runs fn under the name lock of the workspace database
// dbName, across instances: everything that checks, creates, initialises or drops a
// workspace database, with the workspace-row checks that decide it, runs under it. The
// lock is held on the `postgres` admin database, which every caller can reach; fn gets
// that admin database, opened for this call with room for the lock's connection and
// fn's own. A hash collision only serialises two unrelated names.
func WithWorkspaceDatabaseLock(ctx context.Context, cfg *config.DatabaseConfig, dbName string, fn func(ctx context.Context, admin *sql.DB) error) error {
	admin, err := sql.Open("postgres", lockAdminDSN(cfg))
	if err != nil {
		return fmt.Errorf("failed to connect to PostgreSQL server: %w", err)
	}
	defer func() { _ = admin.Close() }()

	if err := admin.PingContext(ctx); err != nil {
		return fmt.Errorf("failed to ping PostgreSQL server: %w", err)
	}
	return withWorkspaceDatabaseLockOn(ctx, admin, dbName, fn)
}

func withWorkspaceDatabaseLockOn(ctx context.Context, admin *sql.DB, dbName string, fn func(ctx context.Context, admin *sql.DB) error) error {
	return WithLockTransaction(ctx, admin,
		func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1::integer, hashtext($2))`, WorkspaceDatabaseLockClass, dbName); err != nil {
				return fmt.Errorf("failed to take the lock on workspace database %s: %w", dbName, err)
			}
			return nil
		},
		func(ctx context.Context) error { return fn(ctx, admin) },
	)
}
