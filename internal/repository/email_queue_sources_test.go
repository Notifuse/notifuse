package repository

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/repository/testutil"
)

func TestEmailQueueRepository_ListActiveSourcesByIntegration(t *testing.T) {
	ctx := context.Background()

	t.Run("returns distinct active sources for the integration, in row order", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()
		repo := NewEmailQueueRepositoryWithDB(db)

		// 'paused' must stay excluded: a stopped source needs nothing done to
		// it, and listing it would make the breaker rewrite its pause reason.
		mock.ExpectQuery(regexp.QuoteMeta("status IN ('pending', 'processing', 'failed')")).
			WithArgs(domain.EmailQueueSourceBroadcast, "int-1").
			WillReturnRows(sqlmock.NewRows([]string{"source_id"}).
				AddRow("broadcast-1").AddRow("broadcast-2"))

		got, err := repo.ListActiveSourcesByIntegration(ctx, "ws-1", domain.EmailQueueSourceBroadcast, "int-1")
		require.NoError(t, err)
		assert.Equal(t, []string{"broadcast-1", "broadcast-2"}, got)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("no active sources yields an empty result, not an error", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()
		repo := NewEmailQueueRepositoryWithDB(db)

		mock.ExpectQuery("SELECT DISTINCT source_id").
			WillReturnRows(sqlmock.NewRows([]string{"source_id"}))

		got, err := repo.ListActiveSourcesByIntegration(ctx, "ws-1", domain.EmailQueueSourceAutomation, "int-9")
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("query failure is wrapped", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()
		repo := NewEmailQueueRepositoryWithDB(db)

		sentinel := errors.New("queue table gone")
		mock.ExpectQuery("SELECT DISTINCT source_id").WillReturnError(sentinel)

		_, err := repo.ListActiveSourcesByIntegration(ctx, "ws-1", domain.EmailQueueSourceBroadcast, "int-1")
		require.ErrorContains(t, err, "failed to list active sources by integration:")
		assert.ErrorIs(t, err, sentinel)
	})
}
