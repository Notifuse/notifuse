package repository

import (
	"context"
	"database/sql/driver"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/repository/testutil"
	"github.com/Notifuse/notifuse/pkg/crypto"
)

const (
	rotationTOTPUser  = "6f1c2a7e-0b5d-4c1e-9a3f-2d8e7b6c5a41"
	rotationTOTPOther = "0a9b8c7d-6e5f-4a3b-2c1d-0e9f8a7b6c5d"
)

func sealTOTP(t *testing.T, userID, secret, key string) string {
	t.Helper()
	aad, err := domain.TOTPAAD(userID)
	require.NoError(t, err)
	sealed, err := crypto.EncryptStringAAD(secret, key, aad)
	require.NoError(t, err)
	return sealed
}

// opensTOTPWithCurrent matches a sealed secret that the current key opens for userID only.
type opensTOTPWithCurrent struct {
	userID string
	want   string
}

func (m opensTOTPWithCurrent) Match(v driver.Value) bool {
	value, ok := v.(string)
	if !ok {
		return false
	}
	aad, err := domain.TOTPAAD(m.userID)
	if err != nil {
		return false
	}
	plain, err := crypto.DecryptFromHexStringAAD(value, rotationCurrentKey, aad)
	return err == nil && plain == m.want
}

func expectRotationUpToTOTPs(mock sqlmock.Sqlmock, totps *sqlmock.Rows) {
	mock.ExpectBegin()
	mock.ExpectExec(rotationLockQuery).WithArgs(secretKeyRotationLockID).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(rotationWorkspaceQuery).WillReturnRows(sqlmock.NewRows([]string{"id", "settings", "integrations"}))
	mock.ExpectQuery(rotationSettingsQuery).WillReturnRows(sqlmock.NewRows([]string{"key", "value"}))
	mock.ExpectQuery(rotationTOTPQuery).WillReturnRows(totps)
}

func TestRotateSecretKey_AuthenticatorSecrets(t *testing.T) {
	ctx := context.Background()
	columns := []string{"user_id", "secret_encrypted", "pending_secret_encrypted"}

	t.Run("secrets under a previous key are re-encrypted, bound to the same user", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()

		expectRotationUpToTOTPs(mock, sqlmock.NewRows(columns).
			AddRow(rotationTOTPUser, sealTOTP(t, rotationTOTPUser, "active", rotationPreviousKey), sealTOTP(t, rotationTOTPUser, "pending", rotationPreviousKey)).
			AddRow(rotationTOTPOther, sealTOTP(t, rotationTOTPOther, "current", rotationCurrentKey), nil))
		mock.ExpectExec(`UPDATE user_totp SET secret_encrypted = \$2, pending_secret_encrypted = \$3`).
			WithArgs(rotationTOTPUser, opensTOTPWithCurrent{rotationTOTPUser, "active"}, opensTOTPWithCurrent{rotationTOTPUser, "pending"}).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		result, err := RotateSecretKey(ctx, db, rotationCurrentKey, []string{rotationPreviousKey})
		require.NoError(t, err)
		assert.Equal(t, 1, result.UserTOTPs, "the secret already under the current key is left as it is")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	// Left so, that person could never pass their second factor again.
	t.Run("an active secret no key opens rolls back, naming the user", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()

		expectRotationUpToTOTPs(mock, sqlmock.NewRows(columns).
			AddRow(rotationTOTPUser, sealTOTP(t, rotationTOTPUser, "active", "some-other-key"), nil))
		mock.ExpectRollback()

		_, err := RotateSecretKey(ctx, db, rotationCurrentKey, []string{rotationPreviousKey})
		require.Error(t, err)
		assert.True(t, errors.Is(err, crypto.ErrUndecryptable))
		assert.Contains(t, err.Error(), rotationTOTPUser)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	// A secret copied onto another user's row does not open with that user.
	t.Run("a secret sealed for another user does not open", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()

		expectRotationUpToTOTPs(mock, sqlmock.NewRows(columns).
			AddRow(rotationTOTPUser, sealTOTP(t, rotationTOTPOther, "active", rotationPreviousKey), nil))
		mock.ExpectRollback()

		_, err := RotateSecretKey(ctx, db, rotationCurrentKey, []string{rotationPreviousKey})
		require.Error(t, err)
		assert.True(t, errors.Is(err, crypto.ErrUndecryptable))
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("with the keys proven, an active secret no key opens is named and the rest goes on", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()

		mock.ExpectBegin()
		mock.ExpectExec(rotationLockQuery).WithArgs(secretKeyRotationLockID).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(rotationWorkspaceQuery).WillReturnRows(sqlmock.NewRows([]string{"id", "settings", "integrations"}))
		mock.ExpectQuery(rotationSettingsQuery).WillReturnRows(sqlmock.NewRows([]string{"key", "value"}).
			AddRow("encrypted_smtp_password", encryptFor(t, "smtp-pass", rotationCurrentKey)))
		mock.ExpectQuery(rotationTOTPQuery).WillReturnRows(sqlmock.NewRows(columns).
			AddRow(rotationTOTPUser, sealTOTP(t, rotationTOTPUser, "active", "some-other-key"), nil))
		mock.ExpectCommit()

		result, err := RotateSecretKey(ctx, db, rotationCurrentKey, []string{rotationPreviousKey})
		require.NoError(t, err)
		assert.Equal(t, []string{rotationTOTPUser}, result.UnopenedTOTPUsers)
		assert.Zero(t, result.UserTOTPs)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	// With nothing to prove the keys right, a setup no key opens is more likely under the
	// right key than the keys are: it is left as it is.
	t.Run("with nothing proving the keys, a setup no key opens is left as it is", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()

		expectRotationUpToTOTPs(mock, sqlmock.NewRows(columns).
			AddRow(rotationTOTPUser, nil, sealTOTP(t, rotationTOTPUser, "pending", "some-other-key")))
		mock.ExpectCommit()

		result, err := RotateSecretKey(ctx, db, rotationCurrentKey, []string{rotationPreviousKey})
		require.NoError(t, err)
		assert.Zero(t, result.DroppedTOTPSetups)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("a secret being set up that no key opens is dropped with its fields", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()

		active := sealTOTP(t, rotationTOTPUser, "active", rotationCurrentKey)
		expectRotationUpToTOTPs(mock, sqlmock.NewRows(columns).
			AddRow(rotationTOTPUser, active, sealTOTP(t, rotationTOTPUser, "pending", "some-other-key")))
		mock.ExpectExec(`UPDATE user_totp SET secret_encrypted = \$2, pending_secret_encrypted = NULL, pending_expires_at = NULL,\s+pending_attempts = 0`).
			WithArgs(rotationTOTPUser, active).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		result, err := RotateSecretKey(ctx, db, rotationCurrentKey, []string{rotationPreviousKey})
		require.NoError(t, err)
		assert.Zero(t, result.UserTOTPs, "nothing was re-encrypted")
		assert.Equal(t, 1, result.DroppedTOTPSetups)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	// With no workspace, settings none of which opens roll back, unless an authenticator
	// secret opened: that shows the keys are right.
	t.Run("an opened authenticator secret vouches for the keys when no workspace is stored", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()

		mock.ExpectBegin()
		mock.ExpectExec(rotationLockQuery).WithArgs(secretKeyRotationLockID).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(rotationWorkspaceQuery).WillReturnRows(sqlmock.NewRows([]string{"id", "settings", "integrations"}))
		mock.ExpectQuery(rotationSettingsQuery).WillReturnRows(sqlmock.NewRows([]string{"key", "value"}).
			AddRow("encrypted_smtp_password", encryptFor(t, "smtp-pass", "some-other-key")))
		mock.ExpectQuery(rotationTOTPQuery).WillReturnRows(sqlmock.NewRows(columns).
			AddRow(rotationTOTPUser, sealTOTP(t, rotationTOTPUser, "active", rotationCurrentKey), nil))
		mock.ExpectCommit()

		result, err := RotateSecretKey(ctx, db, rotationCurrentKey, []string{rotationPreviousKey})
		require.NoError(t, err)
		assert.Equal(t, []string{"encrypted_smtp_password"}, result.UnopenedSettings)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestSecretKeyOpensAUserTOTP(t *testing.T) {
	ctx := context.Background()
	query := `SELECT user_id, secret_encrypted FROM user_totp\s+WHERE enabled_at IS NOT NULL AND secret_encrypted IS NOT NULL`

	t.Run("nothing stored", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()
		mock.ExpectQuery(query).WillReturnRows(sqlmock.NewRows([]string{"user_id", "secret_encrypted"}))
		opens, tried, err := SecretKeyOpensAUserTOTP(ctx, db, rotationCurrentKey)
		require.NoError(t, err)
		assert.True(t, opens)
		assert.Zero(t, tried)
	})

	t.Run("a secret the key opens", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()
		mock.ExpectQuery(query).WillReturnRows(sqlmock.NewRows([]string{"user_id", "secret_encrypted"}).
			AddRow(rotationTOTPOther, sealTOTP(t, rotationTOTPOther, "a", rotationPreviousKey)).
			AddRow(rotationTOTPUser, sealTOTP(t, rotationTOTPUser, "b", rotationCurrentKey)))
		opens, tried, err := SecretKeyOpensAUserTOTP(ctx, db, rotationCurrentKey)
		require.NoError(t, err)
		assert.True(t, opens)
		assert.Equal(t, 2, tried)
	})

	t.Run("none opens", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()
		mock.ExpectQuery(query).WillReturnRows(sqlmock.NewRows([]string{"user_id", "secret_encrypted"}).
			AddRow(rotationTOTPUser, sealTOTP(t, rotationTOTPUser, "b", rotationPreviousKey)))
		opens, tried, err := SecretKeyOpensAUserTOTP(ctx, db, rotationCurrentKey)
		require.NoError(t, err)
		assert.False(t, opens)
		assert.Equal(t, 1, tried)
	})
}
