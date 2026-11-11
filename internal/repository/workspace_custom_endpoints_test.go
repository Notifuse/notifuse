package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
)

func TestWorkspaceRepository_ListCustomEndpoints(t *testing.T) {
	newRepo := func(t *testing.T) (*workspaceRepository, sqlmock.Sqlmock) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		t.Cleanup(func() { _ = db.Close() })
		repo := NewWorkspaceRepository(db, &config.DatabaseConfig{}, "test-secret-key-32-bytes-long!!", newMockConnectionManager(db))
		return repo.(*workspaceRepository), mock
	}

	t.Run("returns each workspace's custom endpoint URL", func(t *testing.T) {
		repo, mock := newRepo(t)
		mock.ExpectQuery(`SELECT id, settings->>'custom_endpoint_url' FROM workspaces WHERE COALESCE\(settings->>'custom_endpoint_url', ''\) != ''`).
			WillReturnRows(sqlmock.NewRows([]string{"id", "custom_endpoint_url"}).
				AddRow("brand_a", "https://mail.brand-a.example").
				AddRow("brand_b", "https://Mail.Brand-B.example:8443"))

		got, err := repo.ListCustomEndpoints(context.Background())
		require.NoError(t, err)
		assert.Equal(t, map[string]string{
			"brand_a": "https://mail.brand-a.example",
			"brand_b": "https://Mail.Brand-B.example:8443",
		}, got)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("surfaces a query failure", func(t *testing.T) {
		repo, mock := newRepo(t)
		mock.ExpectQuery(`SELECT id`).WillReturnError(errors.New("db down"))
		_, err := repo.ListCustomEndpoints(context.Background())
		assert.Error(t, err)
	})
}
