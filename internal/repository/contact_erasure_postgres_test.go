package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupContactErasureTest(t *testing.T) (*mocks.MockWorkspaceRepository, domain.ContactErasureRepository, sqlmock.Sqlmock) {
	t.Helper()
	ctrl := gomock.NewController(t)
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, mock.ExpectationsWereMet())
		_ = db.Close()
	})
	workspaceRepo.EXPECT().GetConnection(gomock.Any(), "ws").Return(db, nil).AnyTimes()
	return workspaceRepo, NewContactErasureRepository(workspaceRepo), mock
}

func TestContactErasureRepository_DeleteListRowsKeepingOptOuts(t *testing.T) {
	t.Run("deletes the rows and hashes the three opt-out statuses in one statement", func(t *testing.T) {
		_, repo, mock := setupContactErasureTest(t)
		mock.ExpectExec(`(?s)WITH gone AS \(\s*DELETE FROM contact_lists WHERE email = \$1\s+RETURNING list_id, status, deleted_at, updated_at\s*\)`+
			`\s*INSERT INTO contact_opt_out_hashes .*SELECT contact_opt_out_hash\(\$1\), gone.list_id, gone.status, gone.deleted_at IS NOT NULL, gone.updated_at, NOW\(\), \$2`+
			`\s+FROM gone\s+WHERE gone.status IN \('unsubscribed', 'bounced', 'complained'\)`+
			`.*ON CONFLICT \(email_hash, list_id\) DO UPDATE.*WHERE EXCLUDED.opted_out_at >= contact_opt_out_hashes.opted_out_at`).
			WithArgs("a@example.com", "delete").
			WillReturnResult(sqlmock.NewResult(0, 2))

		n, err := repo.DeleteListRowsKeepingOptOuts(context.Background(), "ws", "a@example.com", domain.OptOutHashSourceDelete)
		require.NoError(t, err)
		assert.Equal(t, int64(2), n)
	})

	t.Run("an error is returned", func(t *testing.T) {
		_, repo, mock := setupContactErasureTest(t)
		mock.ExpectExec(`WITH gone AS`).WillReturnError(errors.New("boom"))

		_, err := repo.DeleteListRowsKeepingOptOuts(context.Background(), "ws", "a@example.com", domain.OptOutHashSourceRetention)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to record opt-out hashes")
	})
}

func TestContactErasureRepository_ForgetOptOutHashes(t *testing.T) {
	t.Run("deletes every hash of the address", func(t *testing.T) {
		_, repo, mock := setupContactErasureTest(t)
		mock.ExpectExec(`DELETE FROM contact_opt_out_hashes WHERE email_hash = contact_opt_out_hash\(\$1\)`).
			WithArgs("a@example.com").WillReturnResult(sqlmock.NewResult(0, 1))
		require.NoError(t, repo.ForgetOptOutHashes(context.Background(), "ws", "a@example.com"))
	})

	t.Run("an error is returned", func(t *testing.T) {
		_, repo, mock := setupContactErasureTest(t)
		mock.ExpectExec(`DELETE FROM contact_opt_out_hashes`).WillReturnError(errors.New("boom"))
		err := repo.ForgetOptOutHashes(context.Background(), "ws", "a@example.com")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to forget opt-out hashes")
	})
}

func TestContactErasureRepository_DeleteJourneysForEmail(t *testing.T) {
	t.Run("deletes the journeys, then the trigger log", func(t *testing.T) {
		_, repo, mock := setupContactErasureTest(t)
		mock.ExpectExec(`DELETE FROM contact_automations WHERE contact_email = \$1`).
			WithArgs("a@example.com").WillReturnResult(sqlmock.NewResult(0, 3))
		mock.ExpectExec(`DELETE FROM automation_trigger_log WHERE contact_email = \$1`).
			WithArgs("a@example.com").WillReturnResult(sqlmock.NewResult(0, 1))

		require.NoError(t, repo.DeleteJourneysForEmail(context.Background(), "ws", "a@example.com"))
	})

	t.Run("a failed journey delete stops before the trigger log", func(t *testing.T) {
		_, repo, mock := setupContactErasureTest(t)
		mock.ExpectExec(`DELETE FROM contact_automations`).WillReturnError(errors.New("boom"))

		err := repo.DeleteJourneysForEmail(context.Background(), "ws", "a@example.com")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to delete automation journeys")
	})

	t.Run("a failed trigger log delete is reported", func(t *testing.T) {
		_, repo, mock := setupContactErasureTest(t)
		mock.ExpectExec(`DELETE FROM contact_automations`).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`DELETE FROM automation_trigger_log`).WillReturnError(errors.New("boom"))

		err := repo.DeleteJourneysForEmail(context.Background(), "ws", "a@example.com")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to delete automation trigger log")
	})
}

func TestContactErasureRepository_DeleteWebhookDeliveriesForEmail(t *testing.T) {
	t.Run("matches the address where each event kind puts it, and keeps what the deletion sends", func(t *testing.T) {
		_, repo, mock := setupContactErasureTest(t)
		mock.ExpectExec(`(?s)DELETE FROM webhook_deliveries\s+WHERE event_type NOT IN \('contact.deleted', 'segment.left'\)` +
			`.*payload->>'email' = \$1.*payload->'contact'->>'email' = \$1.*payload->'custom_event'->>'email' = \$1`).
			WithArgs("a@example.com").WillReturnResult(sqlmock.NewResult(0, 4))

		n, err := repo.DeleteWebhookDeliveriesForEmail(context.Background(), "ws", "a@example.com")
		require.NoError(t, err)
		assert.Equal(t, int64(4), n)
	})

	t.Run("an error is returned", func(t *testing.T) {
		_, repo, mock := setupContactErasureTest(t)
		mock.ExpectExec(`DELETE FROM webhook_deliveries`).WillReturnError(errors.New("boom"))

		_, err := repo.DeleteWebhookDeliveriesForEmail(context.Background(), "ws", "a@example.com")
		require.Error(t, err)
	})
}

func TestContactErasureRepository_PurgeOptOutHashes(t *testing.T) {
	t.Run("deletes expired hashes and those of deleted lists", func(t *testing.T) {
		_, repo, mock := setupContactErasureTest(t)
		before := time.Date(2023, 11, 23, 0, 0, 0, 0, time.UTC)
		mock.ExpectExec(`(?s)DELETE FROM contact_opt_out_hashes o\s+WHERE o.recorded_at < \$1` +
			`\s+OR NOT EXISTS \(SELECT 1 FROM lists l WHERE l.id = o.list_id AND l.deleted_at IS NULL\)`).
			WithArgs(before).WillReturnResult(sqlmock.NewResult(0, 5))

		n, err := repo.PurgeOptOutHashes(context.Background(), "ws", before)
		require.NoError(t, err)
		assert.Equal(t, int64(5), n)
	})

	t.Run("an error is returned", func(t *testing.T) {
		_, repo, mock := setupContactErasureTest(t)
		mock.ExpectExec(`DELETE FROM contact_opt_out_hashes`).WillReturnError(errors.New("boom"))

		_, err := repo.PurgeOptOutHashes(context.Background(), "ws", time.Now())
		require.Error(t, err)
	})
}

func TestContactErasureRepository_ConnectionError(t *testing.T) {
	ctrl := gomock.NewController(t)
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	workspaceRepo.EXPECT().GetConnection(gomock.Any(), "ws").Return(nil, errors.New("down")).Times(5)
	repo := NewContactErasureRepository(workspaceRepo)
	ctx := context.Background()

	_, err := repo.DeleteListRowsKeepingOptOuts(ctx, "ws", "a@example.com", domain.OptOutHashSourceDelete)
	assert.Error(t, err)
	assert.Error(t, repo.ForgetOptOutHashes(ctx, "ws", "a@example.com"))
	assert.Error(t, repo.DeleteJourneysForEmail(ctx, "ws", "a@example.com"))
	_, err = repo.DeleteWebhookDeliveriesForEmail(ctx, "ws", "a@example.com")
	assert.Error(t, err)
	_, err = repo.PurgeOptOutHashes(ctx, "ws", time.Now())
	assert.Error(t, err)
}
