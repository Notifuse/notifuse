package repository

import (
	"context"
	"database/sql/driver"
	"encoding/json"
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

// document is an argument matcher for a JSON document the function accepts.
type document func(tree any) bool

func (m document) Match(v driver.Value) bool {
	b, ok := v.([]byte)
	if !ok {
		return false
	}
	var tree any
	return json.Unmarshal(b, &tree) == nil && m(tree)
}

// at returns the string at path, object keys and array indexes, in a decoded document.
func at(tree any, path ...any) string {
	for _, step := range path {
		switch s := step.(type) {
		case string:
			object, ok := tree.(map[string]any)
			if !ok {
				return ""
			}
			tree = object[s]
		case int:
			array, ok := tree.([]any)
			if !ok || s >= len(array) {
				return ""
			}
			tree = array[s]
		}
	}
	value, _ := tree.(string)
	return value
}

// rotationIntegrations is an SMTP integration whose password is smtpPassword, and whose
// SES settings, which it does not use, hold a value no key opens.
func rotationIntegrations(id, smtpPassword string) string {
	return fmt.Sprintf(`[{"id": %q, "type": "email", "email_provider": {"kind": "smtp",
		"smtp": {"host": "smtp.example.com", "port": 587, "encrypted_password": %q},
		"ses": {"region": "eu-west-1", "encrypted_secret_key": "not-a-ciphertext"}}}]`, id, smtpPassword)
}

func TestRotateSecretKey(t *testing.T) {
	ctx := context.Background()

	t.Run("values under a previous key are re-encrypted, others left as they are", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()

		rotated := fmt.Sprintf(`{"timezone": "UTC", "encrypted_secret_key": %q}`, encryptFor(t, "ws-key", rotationPreviousKey))
		// Free-form content is never taken for a credential, whatever its key is named.
		untouched := fmt.Sprintf(`{"timezone": "UTC", "encrypted_secret_key": %q, "custom_field_labels": {"encrypted_note": "Note"}}`,
			encryptFor(t, "ws-key", rotationCurrentKey))
		integrations := rotationIntegrations("smtp", encryptFor(t, "pass", rotationPreviousKey))

		mock.ExpectBegin()
		mock.ExpectExec(rotationLockQuery).WithArgs(secretKeyRotationLockID).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(rotationWorkspaceQuery).WillReturnRows(
			sqlmock.NewRows([]string{"id", "settings", "integrations"}).
				AddRow("aaa", []byte(rotated), []byte(integrations)).
				AddRow("bbb", []byte(untouched), nil))
		mock.ExpectExec(rotationWorkspaceWrite).
			WithArgs(
				document(func(tree any) bool {
					return opensWithOnly{want: "ws-key"}.Match(at(tree, "encrypted_secret_key"))
				}),
				document(func(tree any) bool {
					return opensWithOnly{want: "pass"}.Match(at(tree, 0, "email_provider", "smtp", "encrypted_password")) &&
						at(tree, 0, "email_provider", "ses", "encrypted_secret_key") == "not-a-ciphertext"
				}),
				"aaa").
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

	t.Run("a credential the workspace uses that no key opens rolls back, naming the workspace", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()

		first := fmt.Sprintf(`{"encrypted_secret_key": %q}`, encryptFor(t, "ws-key", rotationPreviousKey))
		second := fmt.Sprintf(`{"encrypted_secret_key": %q}`, encryptFor(t, "ws-key", rotationCurrentKey))
		lost := rotationIntegrations("lost-smtp", encryptFor(t, "pass", "a-key-nobody-listed-giving-32-bytes"))

		mock.ExpectBegin()
		mock.ExpectExec(rotationLockQuery).WithArgs(secretKeyRotationLockID).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(rotationWorkspaceQuery).WillReturnRows(
			sqlmock.NewRows([]string{"id", "settings", "integrations"}).
				AddRow("aaa", []byte(first), nil).
				AddRow("bbb", []byte(second), []byte(lost)))
		mock.ExpectExec(rotationWorkspaceWrite).WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), "aaa").
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectRollback()

		_, err := RotateSecretKey(ctx, db, rotationCurrentKey, []string{rotationPreviousKey})

		require.Error(t, err)
		assert.True(t, errors.Is(err, crypto.ErrUndecryptable))
		assert.Contains(t, err.Error(), "workspace bbb")
		assert.Contains(t, err.Error(), "lost-smtp")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("a workspace key no key opens rolls back", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()

		lost := fmt.Sprintf(`{"encrypted_secret_key": %q}`, encryptFor(t, "ws-key", "a-key-nobody-listed-giving-32-bytes"))

		mock.ExpectBegin()
		mock.ExpectExec(rotationLockQuery).WithArgs(secretKeyRotationLockID).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(rotationWorkspaceQuery).WillReturnRows(
			sqlmock.NewRows([]string{"id", "settings", "integrations"}).AddRow("aaa", []byte(lost), nil))
		mock.ExpectRollback()

		_, err := RotateSecretKey(ctx, db, rotationCurrentKey, []string{rotationPreviousKey})

		require.Error(t, err)
		assert.True(t, errors.Is(err, crypto.ErrUndecryptable))
		assert.Contains(t, err.Error(), "workspace aaa")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	// The server skips a setting it cannot open when it loads them, and v29 left
	// encrypted_smtp_relay_* rows that nothing reads any more.
	t.Run("a setting no key opens is left as it is", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()

		mock.ExpectBegin()
		mock.ExpectExec(rotationLockQuery).WithArgs(secretKeyRotationLockID).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(rotationWorkspaceQuery).WillReturnRows(
			sqlmock.NewRows([]string{"id", "settings", "integrations"}))
		mock.ExpectQuery(rotationSettingsQuery).WillReturnRows(
			sqlmock.NewRows([]string{"key", "value"}).
				AddRow("encrypted_smtp_password", encryptFor(t, "smtp-pass", rotationPreviousKey)).
				AddRow("encrypted_smtp_relay_tls_cert_base64", encryptFor(t, "cert", "a-key-nobody-listed-giving-32-bytes")))
		mock.ExpectExec(rotationSettingWrite).
			WithArgs(opensWithOnly{want: "smtp-pass"}, "encrypted_smtp_password").
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		result, err := RotateSecretKey(ctx, db, rotationCurrentKey, []string{rotationPreviousKey})

		require.NoError(t, err)
		assert.Equal(t, SecretKeyRotationResult{Settings: 1, UnopenedSettings: []string{"encrypted_smtp_relay_tls_cert_base64"}}, result)
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

func TestSecretKeyOpensAWorkspace(t *testing.T) {
	ctx := context.Background()
	const query = `SELECT settings->>'encrypted_secret_key' FROM workspaces WHERE COALESCE\(settings->>'encrypted_secret_key', ''\) <> ''`
	unknown := "a-key-nobody-listed-giving-32-bytes"

	t.Run("true when there is no workspace", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()
		mock.ExpectQuery(query).WillReturnRows(sqlmock.NewRows([]string{"key"}))

		opens, examined, err := SecretKeyOpensAWorkspace(ctx, db, rotationCurrentKey)

		require.NoError(t, err)
		assert.True(t, opens)
		assert.Zero(t, examined)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("true when one workspace key of several opens", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()
		mock.ExpectQuery(query).WillReturnRows(sqlmock.NewRows([]string{"key"}).
			AddRow(encryptFor(t, "ws-1", unknown)).
			AddRow(encryptFor(t, "ws-2", rotationCurrentKey)).
			AddRow(encryptFor(t, "ws-3", unknown)))

		opens, _, err := SecretKeyOpensAWorkspace(ctx, db, rotationCurrentKey)

		require.NoError(t, err)
		assert.True(t, opens)
	})

	t.Run("false, with how many it tried, when none opens", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()
		mock.ExpectQuery(query).WillReturnRows(sqlmock.NewRows([]string{"key"}).
			AddRow(encryptFor(t, "ws-1", unknown)).
			AddRow(encryptFor(t, "ws-2", rotationPreviousKey)))

		opens, examined, err := SecretKeyOpensAWorkspace(ctx, db, rotationCurrentKey)

		require.NoError(t, err)
		assert.False(t, opens)
		assert.Equal(t, 2, examined)
	})

	t.Run("a database failure is returned", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()
		mock.ExpectQuery(query).WillReturnError(assert.AnError)

		_, _, err := SecretKeyOpensAWorkspace(ctx, db, rotationCurrentKey)

		require.ErrorIs(t, err, assert.AnError)
	})
}
