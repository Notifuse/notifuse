package database

import (
	"context"
	"database/sql"
	"fmt"
)

// MigrationLockKey is the advisory lock, single-bigint form, that serialises migrations
// across instances on the system database: held exclusively for the whole of a
// migration run, and shared by the background repair of empty workspace databases
// through each initialisation. It differs from the secret-key rotation lock
// (4_242_000_001) and from the name lock's two-key form.
const MigrationLockKey int64 = 4_242_000_003

// MigrationLockHolder is a session holding the migration lock.
type MigrationLockHolder struct {
	PID             int
	Shared          bool // held shared, by an empty workspace repair; otherwise by migrations
	ApplicationName string
	ClientAddr      string
}

// WithMigrationLock runs fn holding the migration lock exclusively, in a lock
// transaction of its own on the system database db, for as long as fn runs: a
// migration is never cut off by a bound. The lock is tried first; when another
// session holds it, waiting is called with the holders found (or the error looking
// them up), then the wait, bounded by ctx only, begins. See WithLockTransaction for
// the rest, the migration lock coming before any name lock, never after.
func WithMigrationLock(ctx context.Context, db *sql.DB, waiting func(holders []MigrationLockHolder, err error), fn func(ctx context.Context) error) error {
	return withLockTransaction(ctx, db, lockBounds{prepare: lockPrepareTimeout}, func(ctx context.Context, tx *sql.Tx) error {
		var granted bool
		if err := tx.QueryRowContext(ctx, `SELECT pg_try_advisory_xact_lock($1)`, MigrationLockKey).Scan(&granted); err != nil {
			return fmt.Errorf("failed to try the migration lock: %w", err)
		}
		if granted {
			return nil
		}
		holders, err := migrationLockHolders(ctx, db)
		if waiting != nil {
			waiting(holders, err)
		}
		if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, MigrationLockKey); err != nil {
			return fmt.Errorf("failed to take the migration lock: %w", err)
		}
		return nil
	}, fn)
}

// WithSharedMigrationLock runs fn holding the migration lock shared, so no migration
// starts until fn returns, within the bounds of WithLockTransaction.
func WithSharedMigrationLock(ctx context.Context, db *sql.DB, fn func(ctx context.Context) error) error {
	return WithLockTransaction(ctx, db, func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock_shared($1)`, MigrationLockKey); err != nil {
			return fmt.Errorf("failed to take the shared migration lock: %w", err)
		}
		return nil
	}, fn)
}

// migrationLockHolders lists the sessions granted the migration lock, exclusively or
// shared. A bigint key is stored in pg_locks as its high and low halves, objsubid 1.
// It runs on db, not on the lock transaction, which a failed lookup would abort.
func migrationLockHolders(ctx context.Context, db *sql.DB) ([]MigrationLockHolder, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT l.pid, l.mode = 'ShareLock', COALESCE(a.application_name, ''), COALESCE(host(a.client_addr), '')
		FROM pg_locks l
		LEFT JOIN pg_stat_activity a ON a.pid = l.pid
		WHERE l.locktype = 'advisory'
		AND l.database = (SELECT oid FROM pg_database WHERE datname = current_database())
		AND l.objsubid = 1
		AND l.classid = ($1::bigint >> 32)::oid
		AND l.objid = ($1::bigint & 4294967295)::oid
		AND l.granted
		ORDER BY l.pid`, MigrationLockKey)
	if err != nil {
		return nil, fmt.Errorf("failed to look up the migration lock's holders: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var holders []MigrationLockHolder
	for rows.Next() {
		var h MigrationLockHolder
		if err := rows.Scan(&h.PID, &h.Shared, &h.ApplicationName, &h.ClientAddr); err != nil {
			return nil, fmt.Errorf("failed to look up the migration lock's holders: %w", err)
		}
		holders = append(holders, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to look up the migration lock's holders: %w", err)
	}
	return holders, nil
}

// LockTransactionSettings returns the statements every lock transaction runs right
// after BEGIN, for tests of code that takes a lock to expect them.
func LockTransactionSettings() []string {
	return append([]string(nil), lockTransactionSettings...)
}
