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

func TestMarkEmailsAsBouncedUnchangedSince(t *testing.T) {
	ctx := context.Background()
	at := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	sinceA := time.Date(2026, 11, 17, 9, 0, 0, 123456000, time.UTC)
	sinceB := time.Date(2026, 11, 16, 8, 30, 0, 0, time.UTC)

	t.Run("bounces only the rows unchanged since each address's reference time", func(t *testing.T) {
		db, mock, cleanup := setupMockDB(t)
		defer cleanup()
		ctrl := gomock.NewController(t)
		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		workspaceRepo.EXPECT().GetConnection(gomock.Any(), "ws-123").Return(db, nil)

		// Each address is paired with its own reference time, in email order, and a
		// row changed after it, or already complained or bounced, keeps its status.
		mock.ExpectExec(`^UPDATE contact_lists AS cl SET status = 'bounced', updated_at = \$3 `+
			`FROM unnest\(\$1::text\[\], \$2::timestamptz\[\]\) AS r\(email, since\) `+
			`WHERE cl.email = r.email AND cl.updated_at <= r.since `+
			`AND cl.status NOT IN \('complained', 'bounced'\)$`).
			WithArgs(
				pq.Array([]string{"a@example.com", "b@example.com"}),
				pq.Array([]string{sinceA.Format(time.RFC3339Nano), sinceB.Format(time.RFC3339Nano)}),
				at,
			).
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := NewContactRepository(workspaceRepo).MarkEmailsAsBouncedUnchangedSince(ctx, "ws-123",
			map[string]time.Time{"b@example.com": sinceB, "a@example.com": sinceA}, at)
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("an empty map runs no query", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)

		err := NewContactRepository(workspaceRepo).MarkEmailsAsBouncedUnchangedSince(ctx, "ws-123", nil, at)
		require.NoError(t, err)
	})

	t.Run("a connection error is returned", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		workspaceRepo.EXPECT().GetConnection(gomock.Any(), "ws-123").Return(nil, errors.New("boom"))

		err := NewContactRepository(workspaceRepo).MarkEmailsAsBouncedUnchangedSince(ctx, "ws-123",
			map[string]time.Time{"a@example.com": sinceA}, at)
		assert.ErrorContains(t, err, "failed to get workspace connection")
	})

	t.Run("an exec error is returned", func(t *testing.T) {
		db, mock, cleanup := setupMockDB(t)
		defer cleanup()
		ctrl := gomock.NewController(t)
		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		workspaceRepo.EXPECT().GetConnection(gomock.Any(), "ws-123").Return(db, nil)
		mock.ExpectExec(`UPDATE contact_lists`).WillReturnError(errors.New("boom"))

		err := NewContactRepository(workspaceRepo).MarkEmailsAsBouncedUnchangedSince(ctx, "ws-123",
			map[string]time.Time{"a@example.com": sinceA}, at)
		assert.ErrorContains(t, err, "boom")
	})
}

func TestMarkEmailsAsBouncedAt(t *testing.T) {
	ctx := context.Background()
	atA := time.Date(2026, 11, 8, 10, 4, 0, 123456000, time.UTC)
	atB := time.Date(2026, 11, 12, 18, 30, 0, 0, time.FixedZone("CET", 3600))

	t.Run("each address is bounced at its own time, in one statement", func(t *testing.T) {
		db, mock, cleanup := setupMockDB(t)
		defer cleanup()
		ctrl := gomock.NewController(t)
		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		workspaceRepo.EXPECT().GetConnection(gomock.Any(), "ws-123").Return(db, nil)

		// Paired in email order, in UTC; a complained or bounced row keeps its status.
		mock.ExpectExec(`^UPDATE contact_lists AS cl SET status = 'bounced', updated_at = r.at `+
			`FROM unnest\(\$1::text\[\], \$2::timestamptz\[\]\) AS r\(email, at\) `+
			`WHERE cl.email = r.email AND cl.status NOT IN \('complained', 'bounced'\)$`).
			WithArgs(
				pq.Array([]string{"a@example.com", "b@example.com"}),
				pq.Array([]string{atA.Format(time.RFC3339Nano), atB.UTC().Format(time.RFC3339Nano)}),
			).
			WillReturnResult(sqlmock.NewResult(0, 2))

		err := NewContactRepository(workspaceRepo).MarkEmailsAsBouncedAt(ctx, "ws-123",
			map[string]time.Time{"b@example.com": atB, "a@example.com": atA})
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("an empty map runs no query", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)

		require.NoError(t, NewContactRepository(workspaceRepo).MarkEmailsAsBouncedAt(ctx, "ws-123", nil))
	})

	t.Run("a connection error is returned", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		workspaceRepo.EXPECT().GetConnection(gomock.Any(), "ws-123").Return(nil, errors.New("boom"))

		err := NewContactRepository(workspaceRepo).MarkEmailsAsBouncedAt(ctx, "ws-123",
			map[string]time.Time{"a@example.com": atA})
		assert.ErrorContains(t, err, "failed to get workspace connection")
	})

	t.Run("an exec error is returned", func(t *testing.T) {
		db, mock, cleanup := setupMockDB(t)
		defer cleanup()
		ctrl := gomock.NewController(t)
		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		workspaceRepo.EXPECT().GetConnection(gomock.Any(), "ws-123").Return(db, nil)
		mock.ExpectExec(`UPDATE contact_lists`).WillReturnError(errors.New("boom"))

		err := NewContactRepository(workspaceRepo).MarkEmailsAsBouncedAt(ctx, "ws-123",
			map[string]time.Time{"a@example.com": atA})
		assert.ErrorContains(t, err, "boom")
	})
}
