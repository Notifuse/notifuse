package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/golang/mock/gomock"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInboundWebhookEventRepository_FirstStoredAt(t *testing.T) {
	ctx := context.Background()
	workspaceID := "ws-123"

	t.Run("returns when each stored event was first stored", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		workspaceRepo.EXPECT().GetConnection(gomock.Any(), workspaceID).Return(db, nil)

		first := time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC)
		mock.ExpectQuery(`SELECT id, created_at FROM inbound_webhook_events WHERE id = ANY\(\$1\)`).
			WithArgs(pq.Array([]string{"evt-1", "evt-2"})).
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow("evt-1", first))

		got, err := NewInboundWebhookEventRepository(workspaceRepo).FirstStoredAt(ctx, workspaceID, []string{"evt-1", "evt-2"})
		require.NoError(t, err)
		assert.Equal(t, map[string]time.Time{"evt-1": first}, got)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("no IDs runs no query", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)

		got, err := NewInboundWebhookEventRepository(workspaceRepo).FirstStoredAt(ctx, workspaceID, nil)
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("a query error is returned", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		workspaceRepo.EXPECT().GetConnection(gomock.Any(), workspaceID).Return(db, nil)
		mock.ExpectQuery(`SELECT id, created_at FROM inbound_webhook_events`).WillReturnError(errors.New("boom"))

		_, err = NewInboundWebhookEventRepository(workspaceRepo).FirstStoredAt(ctx, workspaceID, []string{"evt-1"})
		assert.ErrorContains(t, err, "boom")
	})

	t.Run("a connection error is returned", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		workspaceRepo.EXPECT().GetConnection(gomock.Any(), workspaceID).Return(nil, errors.New("no connection"))

		_, err := NewInboundWebhookEventRepository(workspaceRepo).FirstStoredAt(ctx, workspaceID, []string{"evt-1"})
		assert.ErrorContains(t, err, "no connection")
	})
}
