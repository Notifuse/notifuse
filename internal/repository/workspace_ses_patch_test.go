package repository

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// PatchIntegrationSESSettings merges server-owned SES fields in a single
// statement. The race it exists to kill returns if this ever degrades to a
// read-modify-write of the whole integrations array, so these tests pin the
// single-statement shape, the empty-patch no-op, and the not-found contract.

func newSESPatchRepo(t *testing.T) (*workspaceRepository, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return &workspaceRepository{systemDB: db}, mock
}

func TestWorkspaceRepository_PatchIntegrationSESSettings(t *testing.T) {
	ctx := context.Background()

	t.Run("empty patch is a no-op that issues no SQL", func(t *testing.T) {
		repo, mock := newSESPatchRepo(t)
		require.NoError(t, repo.PatchIntegrationSESSettings(ctx, "ws-1", "int-1", nil))
		require.NoError(t, repo.PatchIntegrationSESSettings(ctx, "ws-1", "int-1", map[string]interface{}{}))
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("patch is JSON-encoded and merged in one UPDATE", func(t *testing.T) {
		repo, mock := newSESPatchRepo(t)
		mock.ExpectExec(regexp.QuoteMeta("jsonb_set")).
			WithArgs("ws-1", "int-1", []byte(`{"configuration_set":"cs-1","tenant":"acme"}`), sqlmock.AnyArg()).
			WillReturnResult(sqlmock.NewResult(0, 1))
		err := repo.PatchIntegrationSESSettings(ctx, "ws-1", "int-1", map[string]interface{}{
			"configuration_set": "cs-1",
			"tenant":            "acme",
		})
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("zero rows affected names the missing integration and workspace", func(t *testing.T) {
		repo, mock := newSESPatchRepo(t)
		mock.ExpectExec(regexp.QuoteMeta("jsonb_set")).
			WillReturnResult(sqlmock.NewResult(0, 0))
		err := repo.PatchIntegrationSESSettings(ctx, "ws-9", "int-ghost", map[string]interface{}{"tenant": "acme"})
		require.EqualError(t, err, "integration int-ghost not found in workspace ws-9")
	})

	t.Run("exec failure is wrapped", func(t *testing.T) {
		repo, mock := newSESPatchRepo(t)
		sentinel := errors.New("jsonb path exploded")
		mock.ExpectExec(regexp.QuoteMeta("jsonb_set")).WillReturnError(sentinel)
		err := repo.PatchIntegrationSESSettings(ctx, "ws-1", "int-1", map[string]interface{}{"tenant": "acme"})
		require.ErrorContains(t, err, "failed to patch SES settings:")
		assert.ErrorIs(t, err, sentinel)
	})
}
