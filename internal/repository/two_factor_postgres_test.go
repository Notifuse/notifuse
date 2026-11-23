package repository

import (
	"context"
	"database/sql"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

func newSQLMock(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, mock.ExpectationsWereMet())
		_ = db.Close()
	})
	return db, mock
}

var userTOTPRowColumns = []string{"user_id", "secret_encrypted", "enabled_at", "last_used_step", "pending_secret_encrypted",
	"pending_expires_at", "pending_attempts", "failed_attempts", "locked_until", "created_at", "updated_at"}

func TestTwoFactorRepository_ClockTimestamp(t *testing.T) {
	db, mock := newSQLMock(t)
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT clock_timestamp()`)).WillReturnRows(sqlmock.NewRows([]string{"now"}).AddRow(now))

	got, err := NewTwoFactorRepository().ClockTimestamp(context.Background(), db)
	require.NoError(t, err)
	assert.Equal(t, now, got)
}

func TestTwoFactorRepository_GetTOTP(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewTwoFactorRepository()
	now := time.Now().UTC().Truncate(time.Second)

	mock.ExpectQuery(`SELECT user_id, .* FROM user_totp WHERE user_id = \$1 FOR UPDATE`).WithArgs("u1").
		WillReturnRows(sqlmock.NewRows(userTOTPRowColumns).
			AddRow("u1", "sealed", now, int64(42), nil, nil, 0, 3, nil, now, now))
	totp, err := repo.GetTOTPForUpdate(context.Background(), db, "u1")
	require.NoError(t, err)
	require.NotNil(t, totp)
	assert.True(t, totp.Enabled())
	assert.Equal(t, int64(42), totp.LastUsedStep)
	assert.Equal(t, 3, totp.FailedAttempts)
	assert.Nil(t, totp.PendingSecretEncrypted)

	// A setup started and not confirmed is not two-factor authentication.
	mock.ExpectQuery(`SELECT user_id, .* FROM user_totp WHERE user_id = \$1$`).WithArgs("u2").
		WillReturnRows(sqlmock.NewRows(userTOTPRowColumns).
			AddRow("u2", nil, nil, int64(0), "pending", now, 1, 0, nil, now, now))
	totp, err = repo.GetTOTP(context.Background(), db, "u2")
	require.NoError(t, err)
	assert.False(t, totp.Enabled())
	require.NotNil(t, totp.PendingSecretEncrypted)

	mock.ExpectQuery(`FROM user_totp`).WithArgs("u3").WillReturnError(sql.ErrNoRows)
	totp, err = repo.GetTOTP(context.Background(), db, "u3")
	require.NoError(t, err)
	assert.Nil(t, totp, "no row is no two-factor authentication, not an error")
}

func TestTwoFactorRepository_SavePendingUpserts(t *testing.T) {
	db, mock := newSQLMock(t)
	now := time.Now().UTC()
	expires := now.Add(domain.PendingSetupTTL)
	mock.ExpectExec(`INSERT INTO user_totp .* ON CONFLICT \(user_id\) DO UPDATE\s+SET pending_secret_encrypted = \$2, pending_expires_at = \$3, pending_attempts = 0`).
		WithArgs("u1", "sealed", expires, now).WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, NewTwoFactorRepository().SavePending(context.Background(), db, "u1", "sealed", expires, now))
}

func TestTwoFactorRepository_RecordPendingFailureDropsTheSecretAtTheLimit(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewTwoFactorRepository()
	now := time.Now().UTC()

	mock.ExpectQuery(`UPDATE user_totp\s+SET pending_attempts = pending_attempts \+ 1`).WithArgs("u1", now).
		WillReturnRows(sqlmock.NewRows([]string{"pending_attempts"}).AddRow(4))
	dropped, err := repo.RecordPendingFailure(context.Background(), db, "u1", domain.MaxPendingSetupAttempts, now)
	require.NoError(t, err)
	assert.False(t, dropped)

	mock.ExpectQuery(`UPDATE user_totp\s+SET pending_attempts = pending_attempts \+ 1`).WithArgs("u1", now).
		WillReturnRows(sqlmock.NewRows([]string{"pending_attempts"}).AddRow(5))
	mock.ExpectExec(`SET pending_secret_encrypted = NULL, pending_expires_at = NULL, pending_attempts = 0`).WithArgs("u1", now).
		WillReturnResult(sqlmock.NewResult(0, 1))
	dropped, err = repo.RecordPendingFailure(context.Background(), db, "u1", domain.MaxPendingSetupAttempts, now)
	require.NoError(t, err)
	assert.True(t, dropped)

	// The row was deleted meanwhile (turned off or reset): nothing is pending any more.
	mock.ExpectQuery(`UPDATE user_totp\s+SET pending_attempts = pending_attempts \+ 1`).WithArgs("u1", now).WillReturnError(sql.ErrNoRows)
	dropped, err = repo.RecordPendingFailure(context.Background(), db, "u1", domain.MaxPendingSetupAttempts, now)
	require.NoError(t, err)
	assert.True(t, dropped)
}

func TestTwoFactorRepository_EnableClearsThePendingSecretAndTheCounters(t *testing.T) {
	db, mock := newSQLMock(t)
	now := time.Now().UTC()
	mock.ExpectExec(`SET secret_encrypted = \$2, enabled_at = COALESCE\(enabled_at, \$4\), last_used_step = \$3,\s+`+
		`pending_secret_encrypted = NULL, pending_expires_at = NULL, pending_attempts = 0,\s+`+
		`failed_attempts = 0, locked_until = NULL`).
		WithArgs("u1", "sealed", int64(99), now).WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, NewTwoFactorRepository().Enable(context.Background(), db, "u1", "sealed", 99, now))

	mock.ExpectExec(`UPDATE user_totp`).WillReturnResult(sqlmock.NewResult(0, 0))
	assert.Error(t, NewTwoFactorRepository().Enable(context.Background(), db, "u1", "sealed", 99, now), "no row to enable")
}

func TestTwoFactorRepository_FailureAndSuccess(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewTwoFactorRepository()
	now := time.Now().UTC()
	until := now.Add(time.Minute)

	mock.ExpectExec(`UPDATE user_totp SET failed_attempts = \$2, locked_until = \$3`).WithArgs("u1", 5, &until, now).
		WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, repo.RecordFailure(context.Background(), db, "u1", 5, &until, now))

	step := int64(7)
	mock.ExpectExec(`SET failed_attempts = 0, locked_until = NULL, last_used_step = COALESCE\(\$2, last_used_step\)`).
		WithArgs("u1", &step, now).WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, repo.RecordSuccess(context.Background(), db, "u1", &step, now))
}

func TestTwoFactorRepository_RecoveryCodes(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewTwoFactorRepository()
	now := time.Now().UTC()

	mock.ExpectExec(`DELETE FROM user_recovery_codes WHERE user_id = \$1`).WithArgs("u1").WillReturnResult(sqlmock.NewResult(0, 10))
	mock.ExpectExec(`INSERT INTO user_recovery_codes`).WithArgs(sqlmock.AnyArg(), "u1", "h1", now).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO user_recovery_codes`).WithArgs(sqlmock.AnyArg(), "u1", "h2", now).WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, repo.ReplaceRecoveryCodes(context.Background(), db, "u1", []string{"h1", "h2"}, now))

	// A code is used once: the update only matches an unused one.
	mock.ExpectExec(`UPDATE user_recovery_codes SET used_at = \$3\s+WHERE user_id = \$1 AND code_hash = \$2 AND used_at IS NULL`).
		WithArgs("u1", "h1", now).WillReturnResult(sqlmock.NewResult(0, 1))
	ok, err := repo.ConsumeRecoveryCode(context.Background(), db, "u1", "h1", now)
	require.NoError(t, err)
	assert.True(t, ok)

	mock.ExpectExec(`UPDATE user_recovery_codes`).WithArgs("u1", "h1", now).WillReturnResult(sqlmock.NewResult(0, 0))
	ok, err = repo.ConsumeRecoveryCode(context.Background(), db, "u1", "h1", now)
	require.NoError(t, err)
	assert.False(t, ok)

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM user_recovery_codes WHERE user_id = \$1 AND used_at IS NULL`).WithArgs("u1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(9))
	left, err := repo.CountRemainingRecoveryCodes(context.Background(), db, "u1")
	require.NoError(t, err)
	assert.Equal(t, 9, left)
}

func TestTwoFactorRepository_ListUsersWithoutTwoFactorSkipsAPIKeys(t *testing.T) {
	db, mock := newSQLMock(t)
	mock.ExpectQuery(`LEFT JOIN user_totp t ON t.user_id = u.id\s+WHERE u.type = 'user' AND \(t.enabled_at IS NULL OR t.secret_encrypted IS NULL\)`).
		WillReturnRows(sqlmock.NewRows([]string{"email"}).AddRow("a@x.io").AddRow("b@x.io"))
	emails, err := NewTwoFactorRepository().ListUsersWithoutTwoFactor(context.Background(), db)
	require.NoError(t, err)
	assert.Equal(t, []string{"a@x.io", "b@x.io"}, emails)
}
