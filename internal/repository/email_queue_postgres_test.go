package repository

import (
	"context"
	"database/sql"
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

	t.Run("successfully marks as sent", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()

		repo := NewEmailQueueRepositoryWithDB(db)

		mock.ExpectExec(`UPDATE email_queue SET status = 'sent'`).
			WithArgs("entry-123", sqlmock.AnyArg()).
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := repo.MarkAsSent(ctx, "workspace-123", "entry-123")
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("handles database error", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()

		repo := NewEmailQueueRepositoryWithDB(db)

		mock.ExpectExec(`UPDATE email_queue SET status = 'sent'`).
			WithArgs("entry-123", sqlmock.AnyArg()).
			WillReturnError(errors.New("database error"))

		err := repo.MarkAsSent(ctx, "workspace-123", "entry-123")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to mark email as sent")
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

func TestEmailQueueRepository_MoveToDeadLetter(t *testing.T) {
	ctx := context.Background()

	t.Run("successfully moves entry to dead letter", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()

		repo := NewEmailQueueRepositoryWithDB(db)

		entry := &domain.EmailQueueEntry{
			ID:           "entry-123",
			SourceType:   domain.EmailQueueSourceBroadcast,
			SourceID:     "broadcast-456",
			ContactEmail: "user@example.com",
			MessageID:    "msg-001",
			Attempts:     3,
			CreatedAt:    time.Now().UTC(),
			Payload:      domain.EmailQueuePayload{Subject: "Test"},
		}

		mock.ExpectBegin()
		mock.ExpectExec(`INSERT INTO email_queue_dead_letter`).
			WithArgs(
				sqlmock.AnyArg(), // deadLetterID
				"entry-123", domain.EmailQueueSourceBroadcast, "broadcast-456", "user@example.com",
				"msg-001", sqlmock.AnyArg(), "max retries exceeded", 3, sqlmock.AnyArg(), sqlmock.AnyArg(),
			).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectExec(`DELETE FROM email_queue WHERE id = \$1`).
			WithArgs("entry-123").
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		err := repo.MoveToDeadLetter(ctx, "workspace-123", entry, "max retries exceeded")
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("rolls back on insert failure", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()

		repo := NewEmailQueueRepositoryWithDB(db)

		entry := &domain.EmailQueueEntry{
			ID:           "entry-123",
			SourceType:   domain.EmailQueueSourceBroadcast,
			ContactEmail: "user@example.com",
			Payload:      domain.EmailQueuePayload{},
		}

		mock.ExpectBegin()
		mock.ExpectExec(`INSERT INTO email_queue_dead_letter`).
			WillReturnError(errors.New("insert failed"))
		mock.ExpectRollback()

		err := repo.MoveToDeadLetter(ctx, "workspace-123", entry, "error")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to insert into dead letter queue")
	})
}

func TestEmailQueueRepository_GetStats(t *testing.T) {
	ctx := context.Background()

	t.Run("returns correct counts by status", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()

		repo := NewEmailQueueRepositoryWithDB(db)

		// Queue stats
		mock.ExpectQuery(`SELECT .+ FROM email_queue`).
			WillReturnRows(sqlmock.NewRows([]string{"pending", "processing", "sent", "failed"}).
				AddRow(10, 5, 100, 3))

		// Dead letter count
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM email_queue_dead_letter`).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

		stats, err := repo.GetStats(ctx, "workspace-123")
		require.NoError(t, err)
		assert.Equal(t, int64(10), stats.Pending)
		assert.Equal(t, int64(5), stats.Processing)
		assert.Equal(t, int64(100), stats.Sent)
		assert.Equal(t, int64(3), stats.Failed)
		assert.Equal(t, int64(2), stats.DeadLetter)
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
			WithArgs(domain.EmailQueueSourceBroadcast, "bcast-123", domain.EmailQueueStatusSent).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(42))

		count, err := repo.CountBySourceAndStatus(ctx, "workspace-123", domain.EmailQueueSourceBroadcast, "bcast-123", domain.EmailQueueStatusSent)
		require.NoError(t, err)
		assert.Equal(t, int64(42), count)
	})

	t.Run("handles database error", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()

		repo := NewEmailQueueRepositoryWithDB(db)

		mock.ExpectQuery(`SELECT COUNT\(\*\)`).
			WillReturnError(errors.New("database error"))

		count, err := repo.CountBySourceAndStatus(ctx, "workspace-123", domain.EmailQueueSourceBroadcast, "bcast-123", domain.EmailQueueStatusSent)
		assert.Error(t, err)
		assert.Equal(t, int64(0), count)
	})
}

func TestEmailQueueRepository_CleanupSent(t *testing.T) {
	ctx := context.Background()

	t.Run("deletes sent entries older than duration", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()

		repo := NewEmailQueueRepositoryWithDB(db)

		mock.ExpectExec(`DELETE FROM email_queue WHERE status = 'sent' AND processed_at < \$1`).
			WithArgs(sqlmock.AnyArg()).
			WillReturnResult(sqlmock.NewResult(0, 50))

		count, err := repo.CleanupSent(ctx, "workspace-123", 24*time.Hour)
		require.NoError(t, err)
		assert.Equal(t, int64(50), count)
	})

	t.Run("handles database error", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()

		repo := NewEmailQueueRepositoryWithDB(db)

		mock.ExpectExec(`DELETE FROM email_queue`).
			WillReturnError(errors.New("database error"))

		count, err := repo.CleanupSent(ctx, "workspace-123", 24*time.Hour)
		assert.Error(t, err)
		assert.Equal(t, int64(0), count)
	})
}

func TestEmailQueueRepository_CleanupDeadLetter(t *testing.T) {
	ctx := context.Background()

	t.Run("deletes dead letter entries older than duration", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()

		repo := NewEmailQueueRepositoryWithDB(db)

		mock.ExpectExec(`DELETE FROM email_queue_dead_letter WHERE failed_at < \$1`).
			WithArgs(sqlmock.AnyArg()).
			WillReturnResult(sqlmock.NewResult(0, 10))

		count, err := repo.CleanupDeadLetter(ctx, "workspace-123", 30*24*time.Hour)
		require.NoError(t, err)
		assert.Equal(t, int64(10), count)
	})
}

func TestEmailQueueRepository_GetDeadLetterEntries(t *testing.T) {
	ctx := context.Background()

	t.Run("returns paginated dead letter entries", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()

		repo := NewEmailQueueRepositoryWithDB(db)

		now := time.Now().UTC()
		payload := domain.EmailQueuePayload{Subject: "Test"}
		payloadJSON, _ := json.Marshal(payload)

		// Count query
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM email_queue_dead_letter`).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(25))

		// Data query
		rows := sqlmock.NewRows([]string{
			"id", "original_entry_id", "source_type", "source_id", "contact_email",
			"message_id", "payload", "final_error", "attempts", "created_at", "failed_at",
		}).AddRow(
			"dl-1", "entry-1", "broadcast", "bcast-1", "user@example.com",
			"msg-1", payloadJSON, "max retries", 3, now, now,
		)

		mock.ExpectQuery(`SELECT .+ FROM email_queue_dead_letter ORDER BY failed_at DESC`).
			WithArgs(10, 0).
			WillReturnRows(rows)

		entries, total, err := repo.GetDeadLetterEntries(ctx, "workspace-123", 10, 0)
		require.NoError(t, err)
		assert.Equal(t, int64(25), total)
		assert.Len(t, entries, 1)
		assert.Equal(t, "dl-1", entries[0].ID)
		assert.Equal(t, "max retries", entries[0].FinalError)
	})

	t.Run("handles empty results", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()

		repo := NewEmailQueueRepositoryWithDB(db)

		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM email_queue_dead_letter`).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

		mock.ExpectQuery(`SELECT .+ FROM email_queue_dead_letter`).
			WithArgs(10, 0).
			WillReturnRows(sqlmock.NewRows([]string{
				"id", "original_entry_id", "source_type", "source_id", "contact_email",
				"message_id", "payload", "final_error", "attempts", "created_at", "failed_at",
			}))

		entries, total, err := repo.GetDeadLetterEntries(ctx, "workspace-123", 10, 0)
		require.NoError(t, err)
		assert.Equal(t, int64(0), total)
		assert.Empty(t, entries)
	})
}

func TestEmailQueueRepository_RetryDeadLetter(t *testing.T) {
	ctx := context.Background()

	t.Run("moves entry back to queue", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()

		repo := NewEmailQueueRepositoryWithDB(db)

		now := time.Now().UTC()
		payload := domain.EmailQueuePayload{Subject: "Test"}
		payloadJSON, _ := json.Marshal(payload)

		mock.ExpectBegin()

		// Get dead letter entry
		mock.ExpectQuery(`SELECT .+ FROM email_queue_dead_letter WHERE id = \$1`).
			WithArgs("dl-123").
			WillReturnRows(sqlmock.NewRows([]string{
				"id", "original_entry_id", "source_type", "source_id", "contact_email",
				"message_id", "payload", "final_error", "attempts", "created_at", "failed_at",
			}).AddRow(
				"dl-123", "entry-123", "broadcast", "bcast-1", "user@example.com",
				"msg-1", payloadJSON, "error", 3, now, now,
			))

		// Insert back to queue
		mock.ExpectExec(`INSERT INTO email_queue`).
			WillReturnResult(sqlmock.NewResult(1, 1))

		// Delete from dead letter
		mock.ExpectExec(`DELETE FROM email_queue_dead_letter WHERE id = \$1`).
			WithArgs("dl-123").
			WillReturnResult(sqlmock.NewResult(0, 1))

		mock.ExpectCommit()

		err := repo.RetryDeadLetter(ctx, "workspace-123", "dl-123")
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns error when entry not found", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()

		repo := NewEmailQueueRepositoryWithDB(db)

		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT .+ FROM email_queue_dead_letter WHERE id = \$1`).
			WithArgs("nonexistent").
			WillReturnError(sql.ErrNoRows)
		mock.ExpectRollback()

		err := repo.RetryDeadLetter(ctx, "workspace-123", "nonexistent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "dead letter entry not found")
	})
}

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
