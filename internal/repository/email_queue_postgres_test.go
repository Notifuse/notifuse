package repository

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/repository/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewEmailQueueRepository(t *testing.T) {
	repo := NewEmailQueueRepository(nil)
	require.NotNil(t, repo)
}

func TestNewEmailQueueRepositoryWithDB(t *testing.T) {
	db, _, cleanup := testutil.SetupMockDB(t)
	defer cleanup()

	repo := NewEmailQueueRepositoryWithDB(db)
	require.NotNil(t, repo)
}

func TestEmailQueueRepository_Enqueue(t *testing.T) {
	ctx := context.Background()

	t.Run("successfully enqueues single entry", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()

		repo := NewEmailQueueRepositoryWithDB(db)

		entry := &domain.EmailQueueEntry{
			ID:            "entry-123",
			SourceType:    domain.EmailQueueSourceBroadcast,
			SourceID:      "broadcast-456",
			IntegrationID: "integration-789",
			ProviderKind:  domain.EmailProviderKindSMTP,
			ContactEmail:  "test@example.com",
			MessageID:     "msg-001",
			TemplateID:    "tpl-001",
			Payload: domain.EmailQueuePayload{
				FromAddress: "sender@example.com",
				Subject:     "Test Subject",
			},
		}

		mock.ExpectBegin()
		mock.ExpectExec(`INSERT INTO email_queue`).
			WithArgs(
				sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
				sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
				sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
				sqlmock.AnyArg(), sqlmock.AnyArg(),
			).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		err := repo.Enqueue(ctx, "workspace-123", []*domain.EmailQueueEntry{entry})
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("handles empty entries slice", func(t *testing.T) {
		db, _, cleanup := testutil.SetupMockDB(t)
		defer cleanup()

		repo := NewEmailQueueRepositoryWithDB(db)

		err := repo.Enqueue(ctx, "workspace-123", []*domain.EmailQueueEntry{})
		assert.NoError(t, err)
	})

	t.Run("returns error on begin transaction failure", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()

		repo := NewEmailQueueRepositoryWithDB(db)

		entry := &domain.EmailQueueEntry{
			ID:         "entry-123",
			SourceType: domain.EmailQueueSourceBroadcast,
		}

		mock.ExpectBegin().WillReturnError(errors.New("connection error"))

		err := repo.Enqueue(ctx, "workspace-123", []*domain.EmailQueueEntry{entry})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to begin transaction")
	})

	t.Run("returns error on insert failure", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()

		repo := NewEmailQueueRepositoryWithDB(db)

		entry := &domain.EmailQueueEntry{
			ID:         "entry-123",
			SourceType: domain.EmailQueueSourceBroadcast,
		}

		mock.ExpectBegin()
		mock.ExpectExec(`INSERT INTO email_queue`).
			WillReturnError(errors.New("insert error"))
		mock.ExpectRollback()

		err := repo.Enqueue(ctx, "workspace-123", []*domain.EmailQueueEntry{entry})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to insert queue entries")
	})

	t.Run("sets default values when not provided", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()

		repo := NewEmailQueueRepositoryWithDB(db)

		// Entry without ID, status, priority, max_attempts
		entry := &domain.EmailQueueEntry{
			SourceType:   domain.EmailQueueSourceAutomation,
			SourceID:     "automation-001",
			ContactEmail: "test@example.com",
		}

		mock.ExpectBegin()
		mock.ExpectExec(`INSERT INTO email_queue`).
			WithArgs(
				sqlmock.AnyArg(), // ID should be generated
				domain.EmailQueueStatusPending,
				domain.EmailQueuePriorityMarketing,
				sqlmock.AnyArg(), sqlmock.AnyArg(),
				sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
				sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
				3, // max_attempts default
				sqlmock.AnyArg(), sqlmock.AnyArg(),
			).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		err := repo.Enqueue(ctx, "workspace-123", []*domain.EmailQueueEntry{entry})
		assert.NoError(t, err)

		// Verify defaults were set on the entry
		assert.NotEmpty(t, entry.ID)
		assert.Equal(t, domain.EmailQueueStatusPending, entry.Status)
		assert.Equal(t, domain.EmailQueuePriorityMarketing, entry.Priority)
		assert.Equal(t, 3, entry.MaxAttempts)
	})

	t.Run("successfully enqueues multiple entries batch", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()

		repo := NewEmailQueueRepositoryWithDB(db)

		entries := []*domain.EmailQueueEntry{
			{ID: "entry-1", SourceType: domain.EmailQueueSourceBroadcast, ContactEmail: "user1@example.com"},
			{ID: "entry-2", SourceType: domain.EmailQueueSourceBroadcast, ContactEmail: "user2@example.com"},
		}

		mock.ExpectBegin()
		mock.ExpectExec(`INSERT INTO email_queue`).
			WillReturnResult(sqlmock.NewResult(2, 2))
		mock.ExpectCommit()

		err := repo.Enqueue(ctx, "workspace-123", entries)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestEmailQueueRepository_FetchPending(t *testing.T) {
	ctx := context.Background()

	t.Run("returns pending entries ordered by priority", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()

		repo := NewEmailQueueRepositoryWithDB(db)

		now := time.Now().UTC()
		payload := domain.EmailQueuePayload{FromAddress: "sender@example.com"}
		payloadJSON, _ := json.Marshal(payload)

		rows := sqlmock.NewRows([]string{
			"id", "status", "priority", "source_type", "source_id", "integration_id", "provider_kind",
			"contact_email", "message_id", "template_id", "payload", "attempts", "max_attempts",
			"last_error", "next_retry_at", "created_at", "updated_at", "processed_at",
		}).AddRow(
			"entry-1", "pending", 1, "broadcast", "bcast-1", "integ-1", "smtp",
			"user@example.com", "msg-1", "tpl-1", payloadJSON, 0, 3,
			nil, nil, now, now, nil,
		).AddRow(
			"entry-2", "pending", 5, "automation", "auto-1", "integ-2", "ses",
			"user2@example.com", "msg-2", "tpl-2", payloadJSON, 0, 3,
			nil, nil, now, now, nil,
		)

		mock.ExpectQuery(`SELECT .+ FROM email_queue WHERE`).
			WithArgs(10).
			WillReturnRows(rows)

		entries, err := repo.FetchPending(ctx, "workspace-123", 10)
		require.NoError(t, err)
		assert.Len(t, entries, 2)
		assert.Equal(t, "entry-1", entries[0].ID)
		assert.Equal(t, 1, entries[0].Priority)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns empty slice when no pending entries", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()

		repo := NewEmailQueueRepositoryWithDB(db)

		rows := sqlmock.NewRows([]string{
			"id", "status", "priority", "source_type", "source_id", "integration_id", "provider_kind",
			"contact_email", "message_id", "template_id", "payload", "attempts", "max_attempts",
			"last_error", "next_retry_at", "created_at", "updated_at", "processed_at",
		})

		mock.ExpectQuery(`SELECT .+ FROM email_queue WHERE`).
			WithArgs(10).
			WillReturnRows(rows)

		entries, err := repo.FetchPending(ctx, "workspace-123", 10)
		require.NoError(t, err)
		assert.Empty(t, entries)
	})

	t.Run("handles database error", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()

		repo := NewEmailQueueRepositoryWithDB(db)

		mock.ExpectQuery(`SELECT .+ FROM email_queue WHERE`).
			WithArgs(10).
			WillReturnError(errors.New("database error"))

		entries, err := repo.FetchPending(ctx, "workspace-123", 10)
		assert.Error(t, err)
		assert.Nil(t, entries)
		assert.Contains(t, err.Error(), "failed to query pending emails")
	})
}

func TestEmailQueueRepository_MarkAsProcessing(t *testing.T) {
	ctx := context.Background()

	t.Run("successfully marks pending entry as processing", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()

		repo := NewEmailQueueRepositoryWithDB(db)

		mock.ExpectExec(`UPDATE email_queue SET status = 'processing'`).
			WithArgs("entry-123").
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := repo.MarkAsProcessing(ctx, "workspace-123", "entry-123")
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns error if entry not found", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()

		repo := NewEmailQueueRepositoryWithDB(db)

		mock.ExpectExec(`UPDATE email_queue SET status = 'processing'`).
			WithArgs("nonexistent").
			WillReturnResult(sqlmock.NewResult(0, 0))

		err := repo.MarkAsProcessing(ctx, "workspace-123", "nonexistent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "email not found or already processing")
	})

	t.Run("handles database error", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()

		repo := NewEmailQueueRepositoryWithDB(db)

		mock.ExpectExec(`UPDATE email_queue SET status = 'processing'`).
			WithArgs("entry-123").
			WillReturnError(errors.New("database error"))

		err := repo.MarkAsProcessing(ctx, "workspace-123", "entry-123")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to mark email as processing")
	})
}

func TestEmailQueueRepository_MarkAsSent(t *testing.T) {
	ctx := context.Background()

	t.Run("successfully deletes entry after send", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()

		repo := NewEmailQueueRepositoryWithDB(db)

		mock.ExpectExec(`DELETE FROM email_queue WHERE id = \$1`).
			WithArgs("entry-123").
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := repo.MarkAsSent(ctx, "workspace-123", "entry-123")
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("handles database error", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()

		repo := NewEmailQueueRepositoryWithDB(db)

		mock.ExpectExec(`DELETE FROM email_queue WHERE id = \$1`).
			WithArgs("entry-123").
			WillReturnError(errors.New("database error"))

		err := repo.MarkAsSent(ctx, "workspace-123", "entry-123")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to delete sent email")
	})
}

func TestEmailQueueRepository_MarkAsFailed(t *testing.T) {
	ctx := context.Background()

	t.Run("marks as failed with error message and retry time", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()

		repo := NewEmailQueueRepositoryWithDB(db)

		nextRetry := time.Now().Add(time.Minute)

		mock.ExpectExec(`UPDATE email_queue SET status = 'failed'`).
			WithArgs("entry-123", sqlmock.AnyArg(), "send failed", &nextRetry).
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := repo.MarkAsFailed(ctx, "workspace-123", "entry-123", "send failed", &nextRetry)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("handles nil nextRetryAt", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()

		repo := NewEmailQueueRepositoryWithDB(db)

		mock.ExpectExec(`UPDATE email_queue SET status = 'failed'`).
			WithArgs("entry-123", sqlmock.AnyArg(), "permanent failure", nil).
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := repo.MarkAsFailed(ctx, "workspace-123", "entry-123", "permanent failure", nil)
		assert.NoError(t, err)
	})

	t.Run("handles database error", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()

		repo := NewEmailQueueRepositoryWithDB(db)

		mock.ExpectExec(`UPDATE email_queue SET status = 'failed'`).
			WillReturnError(errors.New("database error"))

		err := repo.MarkAsFailed(ctx, "workspace-123", "entry-123", "error", nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to mark email as failed")
	})
}

func TestEmailQueueRepository_Delete(t *testing.T) {
	ctx := context.Background()

	t.Run("successfully deletes entry", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()

		repo := NewEmailQueueRepositoryWithDB(db)

		mock.ExpectExec(`DELETE FROM email_queue WHERE id = \$1`).
			WithArgs("entry-123").
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := repo.Delete(ctx, "workspace-123", "entry-123")
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("handles database error", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()

		repo := NewEmailQueueRepositoryWithDB(db)

		mock.ExpectExec(`DELETE FROM email_queue WHERE id = \$1`).
			WithArgs("entry-123").
			WillReturnError(errors.New("database error"))

		err := repo.Delete(ctx, "workspace-123", "entry-123")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to delete queue entry")
	})
}

func TestEmailQueueRepository_GetStats(t *testing.T) {
	ctx := context.Background()

	t.Run("returns correct counts by status", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()

		repo := NewEmailQueueRepositoryWithDB(db)

		// Queue stats (no "sent" column - sent entries are deleted immediately)
		mock.ExpectQuery(`SELECT .+ FROM email_queue`).
			WillReturnRows(sqlmock.NewRows([]string{"pending", "processing", "failed"}).
				AddRow(10, 5, 3))

		stats, err := repo.GetStats(ctx, "workspace-123")
		require.NoError(t, err)
		assert.Equal(t, int64(10), stats.Pending)
		assert.Equal(t, int64(5), stats.Processing)
		assert.Equal(t, int64(3), stats.Failed)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("handles database error", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()

		repo := NewEmailQueueRepositoryWithDB(db)

		mock.ExpectQuery(`SELECT .+ FROM email_queue`).
			WillReturnError(errors.New("database error"))

		stats, err := repo.GetStats(ctx, "workspace-123")
		assert.Error(t, err)
		assert.Nil(t, stats)
	})
}

func TestEmailQueueRepository_GetBySourceID(t *testing.T) {
	ctx := context.Background()

	t.Run("returns entries for broadcast source", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()

		repo := NewEmailQueueRepositoryWithDB(db)

		now := time.Now().UTC()
		payload := domain.EmailQueuePayload{FromAddress: "sender@example.com"}
		payloadJSON, _ := json.Marshal(payload)

		rows := sqlmock.NewRows([]string{
			"id", "status", "priority", "source_type", "source_id", "integration_id", "provider_kind",
			"contact_email", "message_id", "template_id", "payload", "attempts", "max_attempts",
			"last_error", "next_retry_at", "created_at", "updated_at", "processed_at",
		}).AddRow(
			"entry-1", "pending", 5, "broadcast", "bcast-123", "integ-1", "smtp",
			"user@example.com", "msg-1", "tpl-1", payloadJSON, 0, 3,
			nil, nil, now, now, nil,
		)

		mock.ExpectQuery(`SELECT .+ FROM email_queue WHERE source_type = \$1 AND source_id = \$2`).
			WithArgs(domain.EmailQueueSourceBroadcast, "bcast-123").
			WillReturnRows(rows)

		entries, err := repo.GetBySourceID(ctx, "workspace-123", domain.EmailQueueSourceBroadcast, "bcast-123")
		require.NoError(t, err)
		assert.Len(t, entries, 1)
		assert.Equal(t, "entry-1", entries[0].ID)
	})

	t.Run("returns empty for unknown source", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()

		repo := NewEmailQueueRepositoryWithDB(db)

		rows := sqlmock.NewRows([]string{
			"id", "status", "priority", "source_type", "source_id", "integration_id", "provider_kind",
			"contact_email", "message_id", "template_id", "payload", "attempts", "max_attempts",
			"last_error", "next_retry_at", "created_at", "updated_at", "processed_at",
		})

		mock.ExpectQuery(`SELECT .+ FROM email_queue WHERE source_type = \$1 AND source_id = \$2`).
			WithArgs(domain.EmailQueueSourceBroadcast, "nonexistent").
			WillReturnRows(rows)

		entries, err := repo.GetBySourceID(ctx, "workspace-123", domain.EmailQueueSourceBroadcast, "nonexistent")
		require.NoError(t, err)
		assert.Empty(t, entries)
	})
}

func TestEmailQueueRepository_CountBySourceAndStatus(t *testing.T) {
	ctx := context.Background()

	t.Run("counts correctly by source and status", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()

		repo := NewEmailQueueRepositoryWithDB(db)

		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM email_queue WHERE source_type = \$1 AND source_id = \$2 AND status = \$3`).
			WithArgs(domain.EmailQueueSourceBroadcast, "bcast-123", domain.EmailQueueStatusPending).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(42))

		count, err := repo.CountBySourceAndStatus(ctx, "workspace-123", domain.EmailQueueSourceBroadcast, "bcast-123", domain.EmailQueueStatusPending)
		require.NoError(t, err)
		assert.Equal(t, int64(42), count)
	})

	t.Run("handles database error", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()

		repo := NewEmailQueueRepositoryWithDB(db)

		mock.ExpectQuery(`SELECT COUNT\(\*\)`).
			WillReturnError(errors.New("database error"))

		count, err := repo.CountBySourceAndStatus(ctx, "workspace-123", domain.EmailQueueSourceBroadcast, "bcast-123", domain.EmailQueueStatusPending)
		assert.Error(t, err)
		assert.Equal(t, int64(0), count)
	})
}

// Note: CleanupSent test removed - sent entries are now deleted immediately
// so there's no need for a cleanup operation

func TestEmailQueueRepository_EnqueueTx(t *testing.T) {
	ctx := context.Background()

	t.Run("enqueues within existing transaction", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()

		repo := NewEmailQueueRepositoryWithDB(db).(*EmailQueueRepository)

		entry := &domain.EmailQueueEntry{
			ID:           "entry-123",
			SourceType:   domain.EmailQueueSourceAutomation,
			ContactEmail: "test@example.com",
		}

		mock.ExpectBegin()
		tx, err := db.BeginTx(ctx, nil)
		require.NoError(t, err)

		mock.ExpectExec(`INSERT INTO email_queue`).
			WillReturnResult(sqlmock.NewResult(1, 1))

		err = repo.EnqueueTx(ctx, tx, []*domain.EmailQueueEntry{entry})
		assert.NoError(t, err)
	})

	t.Run("handles empty entries in transaction", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()

		repo := NewEmailQueueRepositoryWithDB(db).(*EmailQueueRepository)

		mock.ExpectBegin()
		tx, err := db.BeginTx(ctx, nil)
		require.NoError(t, err)

		err = repo.EnqueueTx(ctx, tx, []*domain.EmailQueueEntry{})
		assert.NoError(t, err)
	})
}
