package database

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithMigrationLock(t *testing.T) {
	tryLock := regexp.QuoteMeta(`SELECT pg_try_advisory_xact_lock($1)`)
	holders := `SELECT l.pid, l.mode = 'ShareLock', COALESCE\(a.application_name, ''\), COALESCE\(host\(a.client_addr\), ''\)\s+FROM pg_locks l`
	waitLock := regexp.QuoteMeta(`SELECT pg_advisory_xact_lock($1)`)

	t.Run("granted at once: no holder lookup, the work runs with no bound, then the lock goes", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		mock.ExpectBegin()
		expectLockTransactionSettings(mock)
		mock.ExpectQuery(tryLock).WithArgs(MigrationLockKey).WillReturnRows(sqlmock.NewRows([]string{"granted"}).AddRow(true))
		mock.ExpectExec(`SELECT work`).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectCommit()

		waited := false
		err = WithMigrationLock(context.Background(), db, func([]MigrationLockHolder, error) { waited = true }, func(ctx context.Context) error {
			_, hasDeadline := ctx.Deadline()
			assert.False(t, hasDeadline, "a migration is never cut off by the lock's bound")
			_, underHold := lockHoldDeadline(ctx)
			assert.False(t, underHold)
			_, err := db.ExecContext(ctx, `SELECT work`)
			return err
		})
		require.NoError(t, err)
		assert.False(t, waited)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("held elsewhere: the holders are reported, then the lock waited for", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		mock.ExpectBegin()
		expectLockTransactionSettings(mock)
		mock.ExpectQuery(tryLock).WithArgs(MigrationLockKey).WillReturnRows(sqlmock.NewRows([]string{"granted"}).AddRow(false))
		mock.ExpectQuery(holders).WithArgs(MigrationLockKey).
			WillReturnRows(sqlmock.NewRows([]string{"pid", "shared", "app", "addr"}).AddRow(41, false, "notifuse", "10.0.0.2").AddRow(42, true, "", ""))
		mock.ExpectExec(waitLock).WithArgs(MigrationLockKey).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectCommit()

		var reported []MigrationLockHolder
		err = WithMigrationLock(context.Background(), db, func(h []MigrationLockHolder, err error) {
			require.NoError(t, err)
			reported = h
		}, func(ctx context.Context) error { return nil })
		require.NoError(t, err)
		assert.Equal(t, []MigrationLockHolder{{PID: 41, ApplicationName: "notifuse", ClientAddr: "10.0.0.2"}, {PID: 42, Shared: true}}, reported)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("a failed holder lookup is reported, and the lock still waited for", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		mock.ExpectBegin()
		expectLockTransactionSettings(mock)
		mock.ExpectQuery(tryLock).WillReturnRows(sqlmock.NewRows([]string{"granted"}).AddRow(false))
		mock.ExpectQuery(holders).WillReturnError(errors.New("permission denied"))
		mock.ExpectExec(waitLock).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectCommit()

		var lookupErr error
		err = WithMigrationLock(context.Background(), db, func(_ []MigrationLockHolder, err error) { lookupErr = err }, func(ctx context.Context) error { return nil })
		require.NoError(t, err)
		assert.ErrorContains(t, lookupErr, "permission denied")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("the work's error comes back and the lock goes with a rollback", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		mock.ExpectBegin()
		expectLockTransactionSettings(mock)
		mock.ExpectQuery(tryLock).WillReturnRows(sqlmock.NewRows([]string{"granted"}).AddRow(true))
		mock.ExpectRollback()

		restart := errors.New("restart required")
		err = WithMigrationLock(context.Background(), db, nil, func(ctx context.Context) error { return restart })
		assert.ErrorIs(t, err, restart)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("the wait ends with ctx", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		mock.ExpectBegin()
		expectLockTransactionSettings(mock)
		mock.ExpectQuery(tryLock).WillReturnRows(sqlmock.NewRows([]string{"granted"}).AddRow(false))
		mock.ExpectQuery(holders).WillReturnRows(sqlmock.NewRows([]string{"pid", "app", "addr"}))
		mock.ExpectExec(waitLock).WillDelayFor(time.Second).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectRollback()

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		ran := false
		err = WithMigrationLock(ctx, db, nil, func(ctx context.Context) error { ran = true; return nil })
		assert.ErrorContains(t, err, "failed to take the migration lock")
		assert.False(t, ran)
	})
}

func TestWithSharedMigrationLock(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	mock.ExpectBegin()
	expectLockTransactionSettings(mock)
	mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_xact_lock_shared($1)`)).WithArgs(MigrationLockKey).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`SELECT work`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	err = WithSharedMigrationLock(context.Background(), db, func(ctx context.Context) error {
		deadline, ok := ctx.Deadline()
		require.True(t, ok, "the shared lock keeps the bounds of a lock transaction")
		assert.WithinDuration(t, time.Now().Add(lockHoldTimeout-lockWorkMargin), deadline, 5*time.Second)
		_, err := db.ExecContext(ctx, `SELECT work`)
		return err
	})
	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestLockTransactionSettings_IsACopy(t *testing.T) {
	got := LockTransactionSettings()
	require.Equal(t, lockTransactionSettings, got)
	got[0] = "changed"
	assert.NotEqual(t, "changed", lockTransactionSettings[0])
}
