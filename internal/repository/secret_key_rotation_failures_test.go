package repository

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/repository/testutil"
)

// A rotation that fails part way rolls back and commits nothing: every case below sets
// no ExpectCommit, so a commit would fail ExpectationsWereMet.
func TestRotateSecretKey_FailureRollsBack(t *testing.T) {
	ctx := context.Background()
	previous := []string{rotationPreviousKey}

	workspaceColumns := []string{"id", "settings", "integrations"}
	settingColumns := []string{"key", "value"}

	// lockAndRead expects the transaction, the lock and the given workspace rows.
	lockAndRead := func(mock sqlmock.Sqlmock, rows *sqlmock.Rows) {
		mock.ExpectBegin()
		mock.ExpectExec(rotationLockQuery).WithArgs(secretKeyRotationLockID).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(rotationWorkspaceQuery).WillReturnRows(rows)
	}
	// rotatedWorkspace holds a workspace key under the previous key, so it is written.
	rotatedWorkspace := func(t *testing.T) []byte {
		return []byte(fmt.Sprintf(`{"encrypted_secret_key": %q}`, encryptFor(t, "ws-key", rotationPreviousKey)))
	}

	cases := []struct {
		name    string
		expect  func(t *testing.T, mock sqlmock.Sqlmock)
		wantErr string
	}{
		{
			name: "the transaction does not start",
			expect: func(t *testing.T, mock sqlmock.Sqlmock) {
				mock.ExpectBegin().WillReturnError(assert.AnError)
			},
			wantErr: "failed to begin transaction",
		},
		{
			name: "the workspaces cannot be read",
			expect: func(t *testing.T, mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectExec(rotationLockQuery).WithArgs(secretKeyRotationLockID).WillReturnResult(sqlmock.NewResult(0, 0))
				mock.ExpectQuery(rotationWorkspaceQuery).WillReturnError(assert.AnError)
				mock.ExpectRollback()
			},
			wantErr: "failed to read workspaces",
		},
		{
			name: "a workspace row does not scan",
			expect: func(t *testing.T, mock sqlmock.Sqlmock) {
				lockAndRead(mock, sqlmock.NewRows([]string{"id"}).AddRow("aaa"))
				mock.ExpectRollback()
			},
			wantErr: "failed to scan workspace",
		},
		{
			name: "reading the workspaces breaks off",
			expect: func(t *testing.T, mock sqlmock.Sqlmock) {
				lockAndRead(mock, sqlmock.NewRows(workspaceColumns).
					AddRow("aaa", rotatedWorkspace(t), nil).
					RowError(0, assert.AnError))
				mock.ExpectRollback()
			},
			wantErr: "failed to read workspaces",
		},
		{
			name: "workspace settings that are not JSON",
			expect: func(t *testing.T, mock sqlmock.Sqlmock) {
				lockAndRead(mock, sqlmock.NewRows(workspaceColumns).AddRow("aaa", []byte("{not json"), nil))
				mock.ExpectRollback()
			},
			wantErr: "workspace aaa, settings",
		},
		{
			name: "workspace integrations that are not JSON",
			expect: func(t *testing.T, mock sqlmock.Sqlmock) {
				lockAndRead(mock, sqlmock.NewRows(workspaceColumns).AddRow("aaa", rotatedWorkspace(t), []byte("[not json")))
				mock.ExpectRollback()
			},
			wantErr: "workspace aaa, integrations",
		},
		{
			name: "a re-encrypted workspace cannot be written",
			expect: func(t *testing.T, mock sqlmock.Sqlmock) {
				lockAndRead(mock, sqlmock.NewRows(workspaceColumns).AddRow("aaa", rotatedWorkspace(t), nil))
				mock.ExpectExec(rotationWorkspaceWrite).WithArgs(sqlmock.AnyArg(), nil, "aaa").WillReturnError(assert.AnError)
				mock.ExpectRollback()
			},
			wantErr: "failed to update workspace aaa",
		},
		{
			name: "the settings cannot be read",
			expect: func(t *testing.T, mock sqlmock.Sqlmock) {
				lockAndRead(mock, sqlmock.NewRows(workspaceColumns))
				mock.ExpectQuery(rotationSettingsQuery).WillReturnError(assert.AnError)
				mock.ExpectRollback()
			},
			wantErr: "failed to read settings",
		},
		{
			name: "a setting row does not scan",
			expect: func(t *testing.T, mock sqlmock.Sqlmock) {
				lockAndRead(mock, sqlmock.NewRows(workspaceColumns))
				mock.ExpectQuery(rotationSettingsQuery).WillReturnRows(sqlmock.NewRows([]string{"key"}).AddRow("encrypted_smtp_password"))
				mock.ExpectRollback()
			},
			wantErr: "failed to scan setting",
		},
		{
			name: "reading the settings breaks off",
			expect: func(t *testing.T, mock sqlmock.Sqlmock) {
				lockAndRead(mock, sqlmock.NewRows(workspaceColumns))
				mock.ExpectQuery(rotationSettingsQuery).WillReturnRows(sqlmock.NewRows(settingColumns).
					AddRow("encrypted_smtp_password", encryptFor(t, "smtp-pass", rotationCurrentKey)).
					RowError(0, assert.AnError))
				mock.ExpectRollback()
			},
			wantErr: "failed to read settings",
		},
		{
			// The workspace already written goes back with the setting that failed.
			name: "a re-encrypted setting cannot be written",
			expect: func(t *testing.T, mock sqlmock.Sqlmock) {
				lockAndRead(mock, sqlmock.NewRows(workspaceColumns).AddRow("aaa", rotatedWorkspace(t), nil))
				mock.ExpectExec(rotationWorkspaceWrite).WithArgs(sqlmock.AnyArg(), nil, "aaa").WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectQuery(rotationSettingsQuery).WillReturnRows(sqlmock.NewRows(settingColumns).
					AddRow("encrypted_smtp_password", encryptFor(t, "smtp-pass", rotationPreviousKey)))
				mock.ExpectExec(rotationSettingWrite).
					WithArgs(opensWithOnly{want: "smtp-pass"}, "encrypted_smtp_password").
					WillReturnError(assert.AnError)
				mock.ExpectRollback()
			},
			wantErr: "failed to update setting encrypted_smtp_password",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, cleanup := testutil.SetupMockDB(t)
			defer cleanup()
			tc.expect(t, mock)

			_, err := RotateSecretKey(ctx, db, rotationCurrentKey, previous)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// A commit that fails reports nothing as rotated: none of the writes it counted landed.
func TestRotateSecretKey_CommitFailureReportsNothingRotated(t *testing.T) {
	db, mock, cleanup := testutil.SetupMockDB(t)
	defer cleanup()

	settings := fmt.Sprintf(`{"encrypted_secret_key": %q}`, encryptFor(t, "ws-key", rotationPreviousKey))
	mock.ExpectBegin()
	mock.ExpectExec(rotationLockQuery).WithArgs(secretKeyRotationLockID).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(rotationWorkspaceQuery).WillReturnRows(
		sqlmock.NewRows([]string{"id", "settings", "integrations"}).AddRow("aaa", []byte(settings), nil))
	mock.ExpectExec(rotationWorkspaceWrite).WithArgs(sqlmock.AnyArg(), nil, "aaa").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(rotationSettingsQuery).WillReturnRows(sqlmock.NewRows([]string{"key", "value"}))
	mock.ExpectCommit().WillReturnError(assert.AnError)

	result, err := RotateSecretKey(context.Background(), db, rotationCurrentKey, []string{rotationPreviousKey})

	require.ErrorIs(t, err, assert.AnError)
	assert.Contains(t, err.Error(), "failed to commit the rotation")
	assert.Equal(t, SecretKeyRotationResult{}, result)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// The single-key checks refuse to answer when the rows cannot be read in full: an
// answer from a partial read could let a wrong key through.
func TestSecretKeyChecks_ReadFailures(t *testing.T) {
	ctx := context.Background()
	const workspaceKeysQuery = `SELECT id, settings->>'encrypted_secret_key' FROM workspaces`
	const settingsQuery = `SELECT key, value FROM settings WHERE key LIKE 'encrypted\\_%' AND value <> '' ORDER BY key`
	unknown := "a-key-nobody-listed-giving-32-bytes"

	checks := []struct {
		name  string
		query string
		run   func(db *sql.DB) (bool, error)
	}{
		{
			name:  "workspace keys",
			query: workspaceKeysQuery,
			run: func(db *sql.DB) (bool, error) {
				opens, _, err := SecretKeyOpensAWorkspace(ctx, db, rotationCurrentKey)
				return opens, err
			},
		},
		{
			name:  "settings",
			query: settingsQuery,
			run: func(db *sql.DB) (bool, error) {
				opens, _, err := SecretKeyOpensASetting(ctx, db, rotationCurrentKey)
				return opens, err
			},
		},
	}
	for _, check := range checks {
		t.Run(check.name+": a row that does not scan", func(t *testing.T) {
			db, mock, cleanup := testutil.SetupMockDB(t)
			defer cleanup()
			mock.ExpectQuery(check.query).WillReturnRows(sqlmock.NewRows([]string{"name"}).AddRow("one"))

			opens, err := check.run(db)

			require.Error(t, err)
			assert.False(t, opens)
		})
		t.Run(check.name+": reading breaks off", func(t *testing.T) {
			db, mock, cleanup := testutil.SetupMockDB(t)
			defer cleanup()
			mock.ExpectQuery(check.query).WillReturnRows(sqlmock.NewRows([]string{"name", "value"}).
				AddRow("one", encryptFor(t, "v", unknown)).
				RowError(0, assert.AnError))

			opens, err := check.run(db)

			require.ErrorIs(t, err, assert.AnError)
			assert.False(t, opens)
		})
	}
}
