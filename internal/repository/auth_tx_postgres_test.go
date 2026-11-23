package repository

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

var sessionRowColumns = []string{"id", "user_id", "expires_at", "created_at", "magic_code", "magic_code_expires_at",
	"auth_method", "second_factor", "second_factor_at", "authenticated_at",
	"challenge_hash", "challenge_expires_at", "challenge_attempts", "pending_invitation_id"}

func TestAuthTxRepository_LockUser(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewAuthTxRepository()

	mock.ExpectQuery(`SELECT id FROM users WHERE id = \$1 FOR UPDATE`).WithArgs("u1").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("u1"))
	require.NoError(t, repo.LockUser(context.Background(), db, "u1"))

	mock.ExpectQuery(`SELECT id FROM users WHERE id = \$1 FOR UPDATE`).WithArgs("gone").WillReturnError(sql.ErrNoRows)
	err := repo.LockUser(context.Background(), db, "gone")
	var notFound *domain.ErrUserNotFound
	assert.ErrorAs(t, err, &notFound)
}

func TestAuthTxRepository_CreateUserIfAbsentNeverAbortsOnAConflict(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewAuthTxRepository()

	mock.ExpectExec(`INSERT INTO users .* ON CONFLICT \(email\) DO NOTHING`).WillReturnResult(sqlmock.NewResult(0, 1))
	created, err := repo.CreateUserIfAbsent(context.Background(), db, &domain.User{Email: "new@x.io"})
	require.NoError(t, err)
	assert.True(t, created)

	mock.ExpectExec(`INSERT INTO users .* ON CONFLICT \(email\) DO NOTHING`).WillReturnResult(sqlmock.NewResult(0, 0))
	created, err = repo.CreateUserIfAbsent(context.Background(), db, &domain.User{Email: "taken@x.io"})
	require.NoError(t, err)
	assert.False(t, created)
}

// The code's expiry is a TIMESTAMP written in UTC: compared at UTC, so that a connection in
// another time zone neither expires codes early nor keeps them late. Only the newest
// matching session is taken.
func TestAuthTxRepository_ConsumeMagicCode(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewAuthTxRepository()
	now := time.Now().UTC().Truncate(time.Second)

	mock.ExpectQuery(`UPDATE user_sessions SET magic_code = NULL, magic_code_expires_at = NULL\s+WHERE id = \(\s+`+
		`SELECT id FROM user_sessions\s+WHERE user_id = \$1 AND magic_code = \$2 AND magic_code_expires_at > \(now\(\) AT TIME ZONE 'UTC'\)\s+`+
		`ORDER BY created_at DESC\s+LIMIT 1\s+FOR UPDATE\s+\)\s+RETURNING id, user_id`).
		WithArgs("u1", "hash").
		WillReturnRows(sqlmock.NewRows(sessionRowColumns).
			AddRow("s1", "u1", now.Add(time.Hour), now, nil, nil, nil, nil, nil, nil, nil, nil, 0, nil))
	session, err := repo.ConsumeMagicCode(context.Background(), db, "u1", "hash")
	require.NoError(t, err)
	assert.Equal(t, "s1", session.ID)
	assert.Nil(t, session.MagicCode)

	mock.ExpectQuery(`UPDATE user_sessions SET magic_code = NULL`).WithArgs("u1", "wrong").WillReturnError(sql.ErrNoRows)
	_, err = repo.ConsumeMagicCode(context.Background(), db, "u1", "wrong")
	var notFound *domain.ErrSessionNotFound
	assert.ErrorAs(t, err, &notFound)
}

func TestAuthTxRepository_PurgeExpiredPendingSessions(t *testing.T) {
	db, mock := newSQLMock(t)
	mock.ExpectExec(`DELETE FROM user_sessions\s+WHERE user_id = \$1 AND \(\s+` +
		`\(challenge_hash IS NOT NULL AND challenge_expires_at < now\(\)\)\s+` +
		`OR \(magic_code IS NOT NULL AND magic_code_expires_at < \(now\(\) AT TIME ZONE 'UTC'\)\)`).
		WithArgs("u1").WillReturnResult(sqlmock.NewResult(0, 2))
	require.NoError(t, NewAuthTxRepository().PurgeExpiredPendingSessions(context.Background(), db, "u1"))
}

func TestAuthTxRepository_Challenge(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewAuthTxRepository()
	now := time.Now().UTC().Truncate(time.Second)
	invitation := "inv-1"

	mock.ExpectExec(`SET auth_method = \$2, challenge_hash = \$3, challenge_expires_at = \$4, challenge_attempts = 0, pending_invitation_id = \$5`).
		WithArgs("s1", "invitation", "chash", now, &invitation).WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, repo.SetSessionChallenge(context.Background(), db, "s1", domain.AuthMethodInvitation, "chash", now, &invitation))

	mock.ExpectQuery(`SELECT user_id FROM user_sessions WHERE challenge_hash = \$1 AND challenge_expires_at > now\(\)`).WithArgs("chash").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u1"))
	userID, err := repo.FindChallengeUser(context.Background(), db, "chash")
	require.NoError(t, err)
	assert.Equal(t, "u1", userID)

	mock.ExpectQuery(`SELECT user_id FROM user_sessions WHERE challenge_hash = \$1`).WithArgs("nope").WillReturnError(sql.ErrNoRows)
	_, err = repo.FindChallengeUser(context.Background(), db, "nope")
	assert.ErrorIs(t, err, domain.ErrTwoFactorChallengeInvalid)

	// The lock re-checks the hash: another request may have completed the challenge.
	mock.ExpectQuery(`FROM user_sessions\s+WHERE user_id = \$1 AND challenge_hash = \$2\s+FOR UPDATE`).WithArgs("u1", "chash").
		WillReturnError(sql.ErrNoRows)
	_, err = repo.GetChallengeSessionForUpdate(context.Background(), db, "u1", "chash")
	assert.ErrorIs(t, err, domain.ErrTwoFactorChallengeInvalid)

	mock.ExpectExec(`UPDATE user_sessions SET challenge_attempts = challenge_attempts \+ 1 WHERE id = \$1`).WithArgs("s1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, repo.FailChallenge(context.Background(), db, "s1"))

	mock.ExpectExec(`SET challenge_hash = NULL, challenge_expires_at = NULL, challenge_attempts = 0, pending_invitation_id = NULL,\s+`+
		`second_factor = \$2, second_factor_at = \$3, authenticated_at = \$3`).
		WithArgs("s1", "recovery_code", now).WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, repo.CompleteChallenge(context.Background(), db, "s1", domain.SecondFactorRecoveryCode, now))
}

func TestAuthTxRepository_SessionSecondFactor(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewAuthTxRepository()
	now := time.Now().UTC()

	// A session revoked meanwhile is reported, not silently skipped.
	mock.ExpectExec(`UPDATE user_sessions SET second_factor = \$2, second_factor_at = \$3\s+WHERE id = \$1 AND challenge_hash IS NULL AND magic_code IS NULL`).
		WithArgs("s1", "totp", now).WillReturnResult(sqlmock.NewResult(0, 0))
	err := repo.SetSessionSecondFactor(context.Background(), db, "s1", domain.SecondFactorTOTP, now)
	var notFound *domain.ErrSessionNotFound
	assert.ErrorAs(t, err, &notFound)

	mock.ExpectExec(`UPDATE user_sessions SET second_factor = NULL, second_factor_at = NULL\s+WHERE user_id = \$1`).WithArgs("u1").
		WillReturnResult(sqlmock.NewResult(0, 3))
	require.NoError(t, repo.ClearSecondFactorOnSessions(context.Background(), db, "u1"))

	mock.ExpectExec(`DELETE FROM user_sessions WHERE user_id = \$1 AND id <> \$2`).WithArgs("u1", "s1").WillReturnResult(sqlmock.NewResult(0, 2))
	require.NoError(t, repo.DeleteSessionsExcept(context.Background(), db, "u1", "s1"))

	mock.ExpectExec(`DELETE FROM user_sessions WHERE user_id = \$1`).WithArgs("u1").WillReturnResult(sqlmock.NewResult(0, 2))
	require.NoError(t, repo.DeleteAllSessions(context.Background(), db, "u1"))
}

func TestAuthTxRepository_ActivateAndCreateSession(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewAuthTxRepository()
	now := time.Now().UTC()

	mock.ExpectExec(`UPDATE user_sessions SET auth_method = \$2, authenticated_at = \$3 WHERE id = \$1`).
		WithArgs("s1", "magic_code", now).WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, repo.ActivateSession(context.Background(), db, "s1", domain.AuthMethodMagicCode, now))

	mock.ExpectExec(`INSERT INTO user_sessions \(id, user_id, expires_at, created_at, magic_code, magic_code_expires_at, auth_method, authenticated_at\)`).
		WithArgs("s2", "u1", sqlmock.AnyArg(), sqlmock.AnyArg(), (*string)(nil), nil, "invitation", &now).WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, repo.CreateSession(context.Background(), db, &domain.Session{
		ID: "s2", UserID: "u1", ExpiresAt: now.Add(time.Hour), AuthMethod: domain.AuthMethodInvitation, AuthenticatedAt: &now,
	}))

	// A session waiting for its magic code keeps it: stored without it, it would be usable.
	code := "hash"
	codeExpires := now.Add(15 * time.Minute)
	mock.ExpectExec(`INSERT INTO user_sessions`).
		WithArgs("s3", "u1", sqlmock.AnyArg(), sqlmock.AnyArg(), &code, codeExpires.UTC(), nil, (*time.Time)(nil)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, repo.CreateSession(context.Background(), db, &domain.Session{
		ID: "s3", UserID: "u1", ExpiresAt: now.Add(time.Hour), MagicCode: &code, MagicCodeExpires: &codeExpires,
	}))

	// A challenge is never set at creation.
	challenge := "chash"
	assert.Error(t, repo.CreateSession(context.Background(), db, &domain.Session{ID: "s4", UserID: "u1", ChallengeHash: &challenge}))
}

func TestAuthTxRepository_GetSessionForUpdateOnlyTakesUsableSessions(t *testing.T) {
	db, mock := newSQLMock(t)
	mock.ExpectQuery(`WHERE id = \$1 AND user_id = \$2 AND challenge_hash IS NULL AND magic_code IS NULL\s+FOR UPDATE`).
		WithArgs("s1", "u1").WillReturnError(sql.ErrNoRows)
	_, err := NewAuthTxRepository().GetSessionForUpdate(context.Background(), db, "u1", "s1")
	var notFound *domain.ErrSessionNotFound
	assert.ErrorAs(t, err, &notFound)
}
