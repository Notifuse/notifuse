package repository

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/repository/testutil"
	"github.com/Notifuse/notifuse/pkg/crypto"
)

const (
	rotationCurrentKey  = "rotation-current-key-giving-32-bytes"
	rotationPreviousKey = "rotation-previous-key-giving-32-bytes"
)

const (
	rotationLockQuery      = `SELECT pg_advisory_xact_lock\(\$1\)`
	rotationWorkspaceQuery = `SELECT id, settings, integrations FROM workspaces ORDER BY id FOR UPDATE`
	rotationSettingsQuery  = `SELECT key, value FROM settings WHERE key LIKE 'encrypted\\_%' AND value <> '' ORDER BY key FOR UPDATE`
	rotationWorkspaceWrite = `UPDATE workspaces SET settings = \$1, integrations = \$2 WHERE id = \$3`
	rotationSettingWrite   = `UPDATE settings SET value = \$1 WHERE key = \$2`
)

func encryptFor(t *testing.T, plaintext, key string) string {
	t.Helper()
	enc, err := crypto.EncryptString(plaintext, key)
	require.NoError(t, err)
	return enc
}

// opensWithOnly is an argument matcher for a value that the current key opens to
// want, and the previous key does not open.
type opensWithOnly struct {
	want string
}

func (m opensWithOnly) Match(v driver.Value) bool {
	s, ok := v.(string)
	if !ok {
		return false
	}
	plain, err := crypto.DecryptFromHexString(s, rotationCurrentKey)
	if err != nil || plain != m.want {
		return false
	}
	_, err = crypto.DecryptFromHexString(s, rotationPreviousKey)
	return err != nil
}

// documentOpensWithCurrentOnly matches a JSON document whose encrypted_ values the
// current key opens and the previous key does not.
type documentOpensWithCurrentOnly struct{}

func (documentOpensWithCurrentOnly) Match(v driver.Value) bool {
	b, ok := v.([]byte)
	if !ok {
		return false
	}
	if _, changed, err := crypto.ReencryptFields(b, rotationCurrentKey, nil); err != nil || changed {
		return false
	}
	_, _, err := crypto.ReencryptFields(b, rotationPreviousKey, nil)
	return errors.Is(err, crypto.ErrUndecryptable)
}

func TestRotateSecretKey(t *testing.T) {
	ctx := context.Background()

	t.Run("values under a previous key are re-encrypted, others left as they are", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()

		rotated := fmt.Sprintf(`{"timezone": "UTC", "encrypted_secret_key": %q}`, encryptFor(t, "ws-key", rotationPreviousKey))
		untouched := fmt.Sprintf(`{"timezone": "UTC", "encrypted_secret_key": %q}`, encryptFor(t, "ws-key", rotationCurrentKey))
		integrations := fmt.Sprintf(`[{"id": "smtp", "email_provider": {"smtp": {"encrypted_password": %q}}}]`,
			encryptFor(t, "pass", rotationPreviousKey))

		mock.ExpectBegin()
		mock.ExpectExec(rotationLockQuery).WithArgs(secretKeyRotationLockID).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(rotationWorkspaceQuery).WillReturnRows(
			sqlmock.NewRows([]string{"id", "settings", "integrations"}).
				AddRow("aaa", []byte(rotated), []byte(integrations)).
				AddRow("bbb", []byte(untouched), nil))
		mock.ExpectExec(rotationWorkspaceWrite).
			WithArgs(documentOpensWithCurrentOnly{}, documentOpensWithCurrentOnly{}, "aaa").
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectQuery(rotationSettingsQuery).WillReturnRows(
			sqlmock.NewRows([]string{"key", "value"}).
				AddRow("encrypted_oidc_client_secret", encryptFor(t, "oidc", rotationCurrentKey)).
				AddRow("encrypted_smtp_password", encryptFor(t, "smtp-pass", rotationPreviousKey)))
		mock.ExpectExec(rotationSettingWrite).
			WithArgs(opensWithOnly{want: "smtp-pass"}, "encrypted_smtp_password").
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		result, err := RotateSecretKey(ctx, db, rotationCurrentKey, []string{rotationPreviousKey})

		require.NoError(t, err)
		assert.Equal(t, SecretKeyRotationResult{Workspaces: 1, Settings: 1}, result)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("without previous keys, values are only checked", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()

		settings := fmt.Sprintf(`{"encrypted_secret_key": %q}`, encryptFor(t, "ws-key", rotationCurrentKey))

		mock.ExpectBegin()
		mock.ExpectExec(rotationLockQuery).WithArgs(secretKeyRotationLockID).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(rotationWorkspaceQuery).WillReturnRows(
			sqlmock.NewRows([]string{"id", "settings", "integrations"}).AddRow("aaa", []byte(settings), []byte("[]")))
		mock.ExpectQuery(rotationSettingsQuery).WillReturnRows(
			sqlmock.NewRows([]string{"key", "value"}).
				AddRow("encrypted_smtp_password", encryptFor(t, "smtp-pass", rotationCurrentKey)))
		mock.ExpectCommit()

		result, err := RotateSecretKey(ctx, db, rotationCurrentKey, nil)

		require.NoError(t, err)
		assert.Equal(t, SecretKeyRotationResult{}, result)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("a workspace value no key opens rolls back, naming the workspace and the path", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()

		first := fmt.Sprintf(`{"encrypted_secret_key": %q}`, encryptFor(t, "ws-key", rotationPreviousKey))
		lost := fmt.Sprintf(`[{"email_provider": {"smtp": {"encrypted_password": %q}}}]`,
			encryptFor(t, "pass", "a-key-nobody-listed-giving-32-bytes"))

		mock.ExpectBegin()
		mock.ExpectExec(rotationLockQuery).WithArgs(secretKeyRotationLockID).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(rotationWorkspaceQuery).WillReturnRows(
			sqlmock.NewRows([]string{"id", "settings", "integrations"}).
				AddRow("aaa", []byte(first), nil).
				AddRow("bbb", []byte(`{}`), []byte(lost)))
		mock.ExpectExec(rotationWorkspaceWrite).WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), "aaa").
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectRollback()

		_, err := RotateSecretKey(ctx, db, rotationCurrentKey, []string{rotationPreviousKey})

		require.Error(t, err)
		assert.True(t, errors.Is(err, crypto.ErrUndecryptable))
		assert.Contains(t, err.Error(), "workspace bbb")
		assert.Contains(t, err.Error(), "integrations [0].email_provider.smtp.encrypted_password")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("a setting no key opens rolls back, naming the setting", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()

		mock.ExpectBegin()
		mock.ExpectExec(rotationLockQuery).WithArgs(secretKeyRotationLockID).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(rotationWorkspaceQuery).WillReturnRows(
			sqlmock.NewRows([]string{"id", "settings", "integrations"}))
		mock.ExpectQuery(rotationSettingsQuery).WillReturnRows(
			sqlmock.NewRows([]string{"key", "value"}).
				AddRow("encrypted_smtp_password", encryptFor(t, "smtp-pass", "a-key-nobody-listed-giving-32-bytes")))
		mock.ExpectRollback()

		_, err := RotateSecretKey(ctx, db, rotationCurrentKey, []string{rotationPreviousKey})

		require.Error(t, err)
		assert.True(t, errors.Is(err, crypto.ErrUndecryptable))
		assert.Contains(t, err.Error(), "setting encrypted_smtp_password")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("the lock is taken before anything is read", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()

		mock.ExpectBegin()
		mock.ExpectExec(rotationLockQuery).WithArgs(secretKeyRotationLockID).WillReturnError(assert.AnError)
		mock.ExpectRollback()

		_, err := RotateSecretKey(ctx, db, rotationCurrentKey, []string{rotationPreviousKey})

		require.ErrorIs(t, err, assert.AnError)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}
