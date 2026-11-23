package repository

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/dkim"
)

const (
	dkimLockQuery    = `SELECT settings->'dkim_keys', integrations FROM workspaces WHERE id = \$1 FOR UPDATE`
	dkimWrite        = `UPDATE workspaces SET settings = jsonb_set\(COALESCE\(settings, '\{\}'::jsonb\), '\{dkim_keys\}', \$1::jsonb, true\) WHERE id = \$2`
	dkimWriteTouched = `UPDATE workspaces SET settings = jsonb_set\(COALESCE\(settings, '\{\}'::jsonb\), '\{dkim_keys\}', \$1::jsonb, true\), updated_at = \$3 WHERE id = \$2`
	dkimRepoSecret   = "dkim-repository-test-secret-32-bytes"
)

func newDKIMRepo(t *testing.T) (*workspaceRepository, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return &workspaceRepository{systemDB: db, secretKey: dkimRepoSecret}, mock
}

func sealedDKIMKey(t *testing.T) string {
	t.Helper()
	privPEM, _, err := dkim.GenerateRSA2048()
	require.NoError(t, err)
	sealed, err := dkim.EncryptPrivateKey(privPEM, dkimRepoSecret)
	require.NoError(t, err)
	return sealed
}

// keysArg matches the JSON of the keys written.
type keysArg func(keys []domain.DKIMKey) bool

func (m keysArg) Match(v driver.Value) bool {
	b, ok := v.([]byte)
	if !ok {
		return false
	}
	var keys []domain.DKIMKey
	return json.Unmarshal(b, &keys) == nil && m(keys)
}

func TestWorkspaceRepository_Update_KeepsStoredDKIMKeys(t *testing.T) {
	// The statement itself is what keeps the stored keys: the settings the caller
	// sends are merged with the row's dkim_keys under its lock. This pins that shape;
	// the integration test checks it against PostgreSQL.
	repo, mock := newDKIMRepo(t)
	mock.ExpectExec(`(?s)SET name = \$1,\s+settings = CASE\s+WHEN workspaces\.settings \? 'dkim_keys'\s+THEN jsonb_set\(\$2::jsonb, '\{dkim_keys\}', workspaces\.settings->'dkim_keys', true\)\s+ELSE \$2::jsonb - 'dkim_keys'\s+END`).
		WillReturnError(errors.New("stop here"))

	w := &domain.Workspace{ID: "ws-1", Name: "Acme", Settings: domain.WorkspaceSettings{
		Timezone:  "UTC",
		SecretKey: "workspace-secret",
		DKIMKeys:  []domain.DKIMKey{{Domain: "stale.io"}},
	}}
	err := repo.Update(context.Background(), w)
	assert.EqualError(t, err, "stop here")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestWorkspaceRepository_UpdateDKIMKeys(t *testing.T) {
	ctx := context.Background()
	stored := `[{"id":"k1","revision":2,"domain":"acme.io","selector":"s1","public_key":"P","status":"pending"}]`
	integrations := `[{"id":"smtp1","name":"Relay","type":"email","email_provider":{"kind":"smtp","senders":[{"email":"a@acme.io"}],"smtp":{"encrypted_password":"opaque"}}}]`

	t.Run("hands the stored keys and integrations to mutate, writes only dkim_keys", func(t *testing.T) {
		repo, mock := newDKIMRepo(t)
		sealed := sealedDKIMKey(t)
		mock.ExpectBegin()
		mock.ExpectQuery(dkimLockQuery).WithArgs("ws-1").
			WillReturnRows(sqlmock.NewRows([]string{"keys", "integrations"}).AddRow([]byte(stored), []byte(integrations)))
		mock.ExpectExec(dkimWriteTouched).
			WithArgs(keysArg(func(keys []domain.DKIMKey) bool {
				return len(keys) == 2 && keys[1].Domain == "new.io" && keys[1].EncryptedPrivateKey == sealed
			}), "ws-1", sqlmock.AnyArg()).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		got, err := repo.UpdateDKIMKeys(ctx, "ws-1", func(keys []domain.DKIMKey, ints domain.Integrations) (domain.DKIMKeysMutation, error) {
			require.Len(t, keys, 1)
			assert.Equal(t, int64(2), keys[0].Revision)
			require.Len(t, ints, 1)
			assert.Equal(t, "a@acme.io", ints[0].EmailProvider.Senders[0].Email)
			assert.Equal(t, "opaque", ints[0].EmailProvider.SMTP.EncryptedPassword, "integrations are not decrypted")
			return domain.DKIMKeysMutation{
				Keys:           append(keys, domain.DKIMKey{Domain: "new.io", EncryptedPrivateKey: sealed}),
				Changed:        true,
				TouchUpdatedAt: true,
			}, nil
		})
		require.NoError(t, err)
		assert.Len(t, got, 2)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("leaves updated_at alone when asked", func(t *testing.T) {
		repo, mock := newDKIMRepo(t)
		mock.ExpectBegin()
		mock.ExpectQuery(dkimLockQuery).WithArgs("ws-1").
			WillReturnRows(sqlmock.NewRows([]string{"keys", "integrations"}).AddRow([]byte(stored), nil))
		mock.ExpectExec(dkimWrite).WithArgs(sqlmock.AnyArg(), "ws-1").WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		_, err := repo.UpdateDKIMKeys(ctx, "ws-1", func(keys []domain.DKIMKey, _ domain.Integrations) (domain.DKIMKeysMutation, error) {
			return domain.DKIMKeysMutation{Keys: keys, Changed: true}, nil
		})
		require.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("deleting the last key writes an empty list", func(t *testing.T) {
		repo, mock := newDKIMRepo(t)
		mock.ExpectBegin()
		mock.ExpectQuery(dkimLockQuery).WithArgs("ws-1").
			WillReturnRows(sqlmock.NewRows([]string{"keys", "integrations"}).AddRow([]byte(stored), nil))
		mock.ExpectExec(dkimWriteTouched).WithArgs([]byte("[]"), "ws-1", sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		got, err := repo.UpdateDKIMKeys(ctx, "ws-1", func([]domain.DKIMKey, domain.Integrations) (domain.DKIMKeysMutation, error) {
			return domain.DKIMKeysMutation{Changed: true, TouchUpdatedAt: true}, nil
		})
		require.NoError(t, err)
		assert.Empty(t, got)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("no change: no write, the stored keys returned", func(t *testing.T) {
		repo, mock := newDKIMRepo(t)
		mock.ExpectBegin()
		mock.ExpectQuery(dkimLockQuery).WithArgs("ws-1").
			WillReturnRows(sqlmock.NewRows([]string{"keys", "integrations"}).AddRow(nil, nil))
		mock.ExpectCommit()

		got, err := repo.UpdateDKIMKeys(ctx, "ws-1", func(keys []domain.DKIMKey, _ domain.Integrations) (domain.DKIMKeysMutation, error) {
			assert.Empty(t, keys, "a workspace without dkim_keys")
			return domain.DKIMKeysMutation{}, nil
		})
		require.NoError(t, err)
		assert.Empty(t, got)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("an error from mutate rolls back", func(t *testing.T) {
		repo, mock := newDKIMRepo(t)
		mock.ExpectBegin()
		mock.ExpectQuery(dkimLockQuery).WithArgs("ws-1").
			WillReturnRows(sqlmock.NewRows([]string{"keys", "integrations"}).AddRow([]byte(stored), nil))
		mock.ExpectRollback()

		sentinel := errors.New("conflict")
		_, err := repo.UpdateDKIMKeys(ctx, "ws-1", func([]domain.DKIMKey, domain.Integrations) (domain.DKIMKeysMutation, error) {
			return domain.DKIMKeysMutation{}, sentinel
		})
		assert.ErrorIs(t, err, sentinel)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("a private key that does not open is refused before writing", func(t *testing.T) {
		repo, mock := newDKIMRepo(t)
		mock.ExpectBegin()
		mock.ExpectQuery(dkimLockQuery).WithArgs("ws-1").
			WillReturnRows(sqlmock.NewRows([]string{"keys", "integrations"}).AddRow([]byte(stored), nil))
		mock.ExpectRollback()

		_, err := repo.UpdateDKIMKeys(ctx, "ws-1", func(keys []domain.DKIMKey, _ domain.Integrations) (domain.DKIMKeysMutation, error) {
			return domain.DKIMKeysMutation{Keys: []domain.DKIMKey{{Domain: "bad.io", EncryptedPrivateKey: "zz"}}, Changed: true}, nil
		})
		assert.ErrorContains(t, err, "bad.io")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("a stored key that no longer opens does not block a write about another domain", func(t *testing.T) {
		repo, mock := newDKIMRepo(t)
		broken := `[{"id":"k1","domain":"broken.io","encrypted_private_key":"zz","status":"published"},{"id":"k2","domain":"acme.io","status":"pending"}]`
		mock.ExpectBegin()
		mock.ExpectQuery(dkimLockQuery).WithArgs("ws-1").
			WillReturnRows(sqlmock.NewRows([]string{"keys", "integrations"}).AddRow([]byte(broken), nil))
		mock.ExpectExec(dkimWrite).WithArgs(sqlmock.AnyArg(), "ws-1").WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		_, err := repo.UpdateDKIMKeys(ctx, "ws-1", func(keys []domain.DKIMKey, _ domain.Integrations) (domain.DKIMKeysMutation, error) {
			keys[1].Status = domain.DKIMKeyStatusPublished
			return domain.DKIMKeysMutation{Keys: keys, Changed: true}, nil
		})
		require.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("a new key written in place of an old one is still checked", func(t *testing.T) {
		repo, mock := newDKIMRepo(t)
		mock.ExpectBegin()
		mock.ExpectQuery(dkimLockQuery).WithArgs("ws-1").
			WillReturnRows(sqlmock.NewRows([]string{"keys", "integrations"}).AddRow([]byte(stored), nil))
		mock.ExpectRollback()

		_, err := repo.UpdateDKIMKeys(ctx, "ws-1", func(keys []domain.DKIMKey, _ domain.Integrations) (domain.DKIMKeysMutation, error) {
			keys[0] = domain.DKIMKey{Domain: "acme.io", EncryptedPrivateKey: "zz"}
			return domain.DKIMKeysMutation{Keys: keys, Changed: true}, nil
		})
		assert.ErrorContains(t, err, "acme.io")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("unknown workspace", func(t *testing.T) {
		repo, mock := newDKIMRepo(t)
		mock.ExpectBegin()
		mock.ExpectQuery(dkimLockQuery).WithArgs("ghost").WillReturnRows(sqlmock.NewRows([]string{"keys", "integrations"}))
		mock.ExpectRollback()

		_, err := repo.UpdateDKIMKeys(ctx, "ghost", func([]domain.DKIMKey, domain.Integrations) (domain.DKIMKeysMutation, error) {
			t.Fatal("mutate must not run")
			return domain.DKIMKeysMutation{}, nil
		})
		var notFound *domain.ErrWorkspaceNotFound
		assert.ErrorAs(t, err, &notFound)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestWorkspaceRepository_GetWorkspaceDKIMKeys(t *testing.T) {
	ctx := context.Background()
	query := `SELECT settings->'dkim_keys' FROM workspaces WHERE id = \$1`

	repo, mock := newDKIMRepo(t)
	mock.ExpectQuery(query).WithArgs("ws-1").
		WillReturnRows(sqlmock.NewRows([]string{"keys"}).AddRow([]byte(`[{"domain":"acme.io","status":"published","encrypted_private_key":"zz"}]`)))
	keys, err := repo.GetWorkspaceDKIMKeys(ctx, "ws-1")
	require.NoError(t, err)
	require.Len(t, keys, 1)
	assert.Equal(t, "zz", keys[0].EncryptedPrivateKey, "returned sealed, a broken key fails nothing here")

	mock.ExpectQuery(query).WithArgs("ws-2").WillReturnRows(sqlmock.NewRows([]string{"keys"}).AddRow(nil))
	keys, err = repo.GetWorkspaceDKIMKeys(ctx, "ws-2")
	require.NoError(t, err)
	assert.Empty(t, keys)

	mock.ExpectQuery(query).WithArgs("ghost").WillReturnRows(sqlmock.NewRows([]string{"keys"}))
	_, err = repo.GetWorkspaceDKIMKeys(ctx, "ghost")
	var notFound *domain.ErrWorkspaceNotFound
	assert.ErrorAs(t, err, &notFound)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestWorkspaceRepository_ListWorkspacesDKIMMetadata(t *testing.T) {
	repo, mock := newDKIMRepo(t)
	mock.ExpectQuery(`(?s)SELECT id, \(\s+SELECT jsonb_agg\(k - 'encrypted_private_key'\).*WHERE jsonb_typeof\(settings->'dkim_keys'\) = 'array'`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "keys"}).
			AddRow("ws-1", []byte(`[{"domain":"acme.io","selector":"s1","status":"published"}]`)).
			AddRow("ws-2", []byte(`[{"domain":"b.io"},{"domain":"c.io"}]`)))

	got, err := repo.ListWorkspacesDKIMMetadata(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "ws-1", got[0].WorkspaceID)
	assert.Equal(t, domain.DKIMKeyStatusPublished, got[0].Keys[0].Status)
	assert.Len(t, got[1].Keys, 2)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestWorkspaceRepository_ListWorkspacesDKIMMetadata_SkipsAnUndecodableWorkspace(t *testing.T) {
	repo, mock := newDKIMRepo(t)
	mock.ExpectQuery(`SELECT id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "keys"}).
			AddRow("ws-bad", []byte(`[{"domain":"acme.io","revision":"two"}]`)).
			AddRow("ws-2", []byte(`[{"domain":"b.io"}]`)))

	got, err := repo.ListWorkspacesDKIMMetadata(context.Background())
	assert.ErrorContains(t, err, "ws-bad")
	require.Len(t, got, 1, "the other workspaces are still returned")
	assert.Equal(t, "ws-2", got[0].WorkspaceID)
}
