package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

const trackingWorkspaceQuery = `SELECT COALESCE\(settings->>'encrypted_secret_key', ''\), COALESCE\(settings->>'custom_endpoint_url', ''\) FROM workspaces WHERE id = \$1`

func TestGetTrackingWorkspace(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	t.Run("reads the two fields", func(t *testing.T) {
		mock.ExpectQuery(trackingWorkspaceQuery).WithArgs("ws-1").
			WillReturnRows(sqlmock.NewRows([]string{"secret", "endpoint"}).AddRow("enc", "https://track.brand.com"))
		row, err := GetTrackingWorkspace(context.Background(), db, "ws-1")
		require.NoError(t, err)
		assert.Equal(t, TrackingWorkspaceRow{EncryptedSecretKey: "enc", CustomEndpointURL: "https://track.brand.com"}, row)
	})

	t.Run("no row is ErrWorkspaceNotFound", func(t *testing.T) {
		mock.ExpectQuery(trackingWorkspaceQuery).WithArgs("missing").WillReturnError(sql.ErrNoRows)
		_, err := GetTrackingWorkspace(context.Background(), db, "missing")
		var notFound *domain.ErrWorkspaceNotFound
		require.ErrorAs(t, err, &notFound)
		assert.Equal(t, "missing", notFound.WorkspaceID)
	})

	t.Run("database error propagates", func(t *testing.T) {
		boom := errors.New("connection refused")
		mock.ExpectQuery(trackingWorkspaceQuery).WithArgs("ws-1").WillReturnError(boom)
		_, err := GetTrackingWorkspace(context.Background(), db, "ws-1")
		require.ErrorIs(t, err, boom)
		var notFound *domain.ErrWorkspaceNotFound
		assert.False(t, errors.As(err, &notFound))
	})

	require.NoError(t, mock.ExpectationsWereMet())
}
