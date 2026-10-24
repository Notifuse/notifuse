package repository

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/internal/service"
)

// WithTransaction is a documented placeholder: workspace-scoped transactions
// must go through GetConnection, so the generic helper must keep refusing
// instead of silently running fn outside a transaction.
func TestAutomationRepository_WithTransaction_Refuses(t *testing.T) {
	_, _, repo := setupAutomationMock(t)
	called := false
	err := repo.WithTransaction(context.Background(), func(tx *sql.Tx) error {
		called = true
		return nil
	})
	require.EqualError(t, err, "use workspace-specific transactions via GetConnection")
	assert.False(t, called, "fn must not run outside a real transaction")
}

// NewAutomationRepository (the production constructor) leaves db nil, so
// getDB must resolve connections through the workspace repository.
func TestAutomationRepository_NewRoutesThroughWorkspaceRepo(t *testing.T) {
	ctrl := gomock.NewController(t)
	wsRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewAutomationRepository(wsRepo, service.NewAutomationTriggerGenerator(service.NewQueryBuilder()))

	sentinel := errors.New("no pool for workspace")
	wsRepo.EXPECT().GetConnection(gomock.Any(), "ws-1").Return(nil, sentinel).Times(1)

	err := repo.IncrementAutomationStat(context.Background(), "ws-1", "auto-1", "enrolled")
	require.ErrorContains(t, err, "failed to get database connection:")
	assert.ErrorIs(t, err, sentinel)
}

func TestAutomationRepository_IncrementAutomationStat(t *testing.T) {
	ctx := context.Background()

	t.Run("invalid stat names are rejected before any SQL runs", func(t *testing.T) {
		_, mock, repo := setupAutomationMock(t)
		for _, stat := range []string{"", "opened", "ENROLLED", "enrolled; DROP TABLE automations"} {
			err := repo.IncrementAutomationStat(ctx, "ws-1", "auto-1", stat)
			require.EqualError(t, err, "invalid stat name: "+stat, "stat %q", stat)
		}
		// No expectations were set: any Exec would have failed the test, and
		// this confirms nothing was left pending either. The stat name is
		// interpolated into the query, so this whitelist is the injection guard.
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("each valid stat increments and reports success", func(t *testing.T) {
		for _, stat := range []string{"enrolled", "completed", "exited", "failed"} {
			_, mock, repo := setupAutomationMock(t)
			mock.ExpectExec(regexp.QuoteMeta("UPDATE automations")).
				WithArgs(sqlmock.AnyArg(), "auto-1", "ws-1").
				WillReturnResult(sqlmock.NewResult(0, 1))
			require.NoError(t, repo.IncrementAutomationStat(ctx, "ws-1", "auto-1", stat), "stat %s", stat)
			require.NoError(t, mock.ExpectationsWereMet())
		}
	})

	t.Run("zero rows affected means the automation does not exist", func(t *testing.T) {
		_, mock, repo := setupAutomationMock(t)
		mock.ExpectExec(regexp.QuoteMeta("UPDATE automations")).
			WillReturnResult(sqlmock.NewResult(0, 0))
		err := repo.IncrementAutomationStat(ctx, "ws-1", "ghost", "enrolled")
		require.EqualError(t, err, "automation not found: ghost")
	})

	t.Run("exec failure is wrapped", func(t *testing.T) {
		_, mock, repo := setupAutomationMock(t)
		sentinel := errors.New("connection reset")
		mock.ExpectExec(regexp.QuoteMeta("UPDATE automations")).WillReturnError(sentinel)
		err := repo.IncrementAutomationStat(ctx, "ws-1", "auto-1", "failed")
		require.ErrorContains(t, err, "failed to increment automation stat:")
		assert.ErrorIs(t, err, sentinel)
	})
}

func TestAutomationRepository_UpdateContactAutomationIfActive(t *testing.T) {
	ctx := context.Background()
	ca := func() *domain.ContactAutomation {
		return &domain.ContactAutomation{
			ID: "ca-1", AutomationID: "auto-1", ContactEmail: "a@example.com",
			Status: domain.ContactAutomationStatusActive, MaxRetries: 3,
			Context: map[string]interface{}{"step": float64(2)},
		}
	}

	t.Run("one row updated means the caller still owns the journey", func(t *testing.T) {
		_, mock, repo := setupAutomationMock(t)
		mock.ExpectExec(regexp.QuoteMeta("UPDATE contact_automations")).
			WillReturnResult(sqlmock.NewResult(0, 1))
		updated, err := repo.UpdateContactAutomationIfActive(ctx, "ws-1", ca())
		require.NoError(t, err)
		assert.True(t, updated)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("zero rows updated means a concurrent exit won, without error", func(t *testing.T) {
		// This false-without-error contract is what stops the executor from
		// resurrecting an exited journey; turning it into an error (or true)
		// would break the reply-interrupt race handling.
		_, mock, repo := setupAutomationMock(t)
		mock.ExpectExec(regexp.QuoteMeta("UPDATE contact_automations")).
			WillReturnResult(sqlmock.NewResult(0, 0))
		updated, err := repo.UpdateContactAutomationIfActive(ctx, "ws-1", ca())
		require.NoError(t, err)
		assert.False(t, updated)
	})

	t.Run("the update only applies while the stored row is active", func(t *testing.T) {
		_, mock, repo := setupAutomationMock(t)
		// Capture the generated SQL to prove the optimistic-lock predicate is
		// present; without status='active' the lock is fictional.
		mock.ExpectExec(regexp.QuoteMeta("UPDATE contact_automations")).
			WithArgs(
				sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
				sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
				"ca-1", domain.ContactAutomationStatusActive,
			).
			WillReturnResult(sqlmock.NewResult(0, 1))
		_, err := repo.UpdateContactAutomationIfActive(ctx, "ws-1", ca())
		require.NoError(t, err)
	})

	t.Run("exec failure is wrapped and reports not-updated", func(t *testing.T) {
		_, mock, repo := setupAutomationMock(t)
		sentinel := errors.New("deadlock detected")
		mock.ExpectExec(regexp.QuoteMeta("UPDATE contact_automations")).WillReturnError(sentinel)
		updated, err := repo.UpdateContactAutomationIfActive(ctx, "ws-1", ca())
		assert.False(t, updated)
		require.ErrorContains(t, err, "failed to update contact automation:")
		assert.ErrorIs(t, err, sentinel)
	})
}

func scheduledRow(id, email string, entered time.Time) []driver.Value {
	nodeID := "node-1"
	return []driver.Value{
		id, "auto-1", email, nodeID, string(domain.ContactAutomationStatusActive),
		nil, entered, entered, []byte(`{"step":1}`), 0, nil, nil, 3,
	}
}

var scheduledCols = []string{
	"id", "automation_id", "contact_email", "current_node_id", "status",
	"exit_reason", "entered_at", "scheduled_at", "context", "retry_count",
	"last_error", "last_retry_at", "max_retries",
}

func TestAutomationRepository_GetScheduledContactAutomationsGlobal(t *testing.T) {
	ctx := context.Background()
	before := time.Now().UTC()

	newGlobalRepo := func(t *testing.T, wsRepo *mocks.MockWorkspaceRepository) (sqlmock.Sqlmock, *AutomationRepository) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		repo := &AutomationRepository{
			workspaceRepo:    wsRepo,
			db:               db,
			triggerGenerator: service.NewAutomationTriggerGenerator(service.NewQueryBuilder()),
		}
		return mock, repo
	}

	t.Run("a missing workspace repository is an explicit error", func(t *testing.T) {
		_, _, repo := setupAutomationMock(t) // db-only repo: workspaceRepo is nil
		_, err := repo.GetScheduledContactAutomationsGlobal(ctx, before, 10)
		require.EqualError(t, err, "workspace repository is required for global scheduling")
	})

	t.Run("workspace list failure is wrapped", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		wsRepo := mocks.NewMockWorkspaceRepository(ctrl)
		_, repo := newGlobalRepo(t, wsRepo)
		sentinel := errors.New("cannot list workspaces")
		wsRepo.EXPECT().List(gomock.Any()).Return(nil, sentinel).Times(1)
		_, err := repo.GetScheduledContactAutomationsGlobal(ctx, before, 10)
		require.ErrorContains(t, err, "failed to get workspaces:")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("no workspaces means no work, not an error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		wsRepo := mocks.NewMockWorkspaceRepository(ctrl)
		_, repo := newGlobalRepo(t, wsRepo)
		wsRepo.EXPECT().List(gomock.Any()).Return(nil, nil).Times(1)
		got, err := repo.GetScheduledContactAutomationsGlobal(ctx, before, 10)
		require.NoError(t, err)
		assert.Nil(t, got)
	})

	t.Run("round-robin trims to the limit and tags each row with its workspace", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		wsRepo := mocks.NewMockWorkspaceRepository(ctrl)
		mock, repo := newGlobalRepo(t, wsRepo)
		wsRepo.EXPECT().List(gomock.Any()).Return([]*domain.Workspace{
			{ID: "ws-a"}, {ID: "ws-b"},
		}, nil).Times(1)

		// limit=3 over 2 workspaces -> perWorkspace = 3/2+1 = 2 from each;
		// the combined 4 must be trimmed to 3, ws-a first (list order).
		entered := time.Now().UTC()
		mock.ExpectQuery(regexp.QuoteMeta("FROM contact_automations")).
			WithArgs(before, 2).
			WillReturnRows(sqlmock.NewRows(scheduledCols).
				AddRow(scheduledRow("ca-1", "a1@example.com", entered)...).
				AddRow(scheduledRow("ca-2", "a2@example.com", entered)...))
		mock.ExpectQuery(regexp.QuoteMeta("FROM contact_automations")).
			WithArgs(before, 2).
			WillReturnRows(sqlmock.NewRows(scheduledCols).
				AddRow(scheduledRow("ca-3", "b1@example.com", entered)...).
				AddRow(scheduledRow("ca-4", "b2@example.com", entered)...))

		got, err := repo.GetScheduledContactAutomationsGlobal(ctx, before, 3)
		require.NoError(t, err)
		require.Len(t, got, 3)
		assert.Equal(t, "ws-a", got[0].WorkspaceID)
		assert.Equal(t, "ca-1", got[0].ID)
		assert.Equal(t, "ws-a", got[1].WorkspaceID)
		assert.Equal(t, "ws-b", got[2].WorkspaceID)
		assert.Equal(t, "ca-3", got[2].ID)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("a failing workspace is skipped, the others still contribute", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		wsRepo := mocks.NewMockWorkspaceRepository(ctrl)
		mock, repo := newGlobalRepo(t, wsRepo)
		wsRepo.EXPECT().List(gomock.Any()).Return([]*domain.Workspace{
			{ID: "ws-bad"}, {ID: "ws-good"},
		}, nil).Times(1)

		entered := time.Now().UTC()
		mock.ExpectQuery(regexp.QuoteMeta("FROM contact_automations")).
			WillReturnError(errors.New("workspace db down"))
		mock.ExpectQuery(regexp.QuoteMeta("FROM contact_automations")).
			WillReturnRows(sqlmock.NewRows(scheduledCols).
				AddRow(scheduledRow("ca-9", "g1@example.com", entered)...))

		got, err := repo.GetScheduledContactAutomationsGlobal(ctx, before, 10)
		require.NoError(t, err, "one bad workspace must not starve the scheduler")
		require.Len(t, got, 1)
		assert.Equal(t, "ws-good", got[0].WorkspaceID)
		assert.Equal(t, "g1@example.com", got[0].ContactEmail)
	})
}

// ListContactAutomations builds count and data queries from the same filter:
// the status list and scheduled bound must reach both, with context decoded
// from JSONB. UpdateContactAutomationTx pins the full column set and maps a
// zero-row update to not-found.
func TestAutomationRepository_ListAndUpdateContactAutomations(t *testing.T) {
	ctx := context.Background()
	cols := []string{"id", "automation_id", "contact_email", "current_node_id", "status", "exit_reason", "entered_at", "scheduled_at", "context", "retry_count", "last_error", "last_retry_at", "max_retries"}
	now := time.Now().Truncate(time.Second)
	row := func(id, status string) []driver.Value {
		return []driver.Value{id, "auto-1", "a@example.com", "n1", status, nil, now, nil, []byte(`{"k":1}`), int64(0), nil, nil, int64(3)}
	}

	t.Run("filter reaches both queries and rows decode", func(t *testing.T) {
		_, mock, repo := setupAutomationMock(t)
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM contact_automations WHERE`).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
		mock.ExpectQuery(regexp.QuoteMeta("ORDER BY entered_at DESC")).
			WillReturnRows(sqlmock.NewRows(cols).
				AddRow(row("ca-1", "active")...).
				AddRow(row("ca-2", "completed")...))

		scheduledBy := now.Add(time.Hour)
		cas, count, err := repo.ListContactAutomations(ctx, "ws-1", domain.ContactAutomationFilter{
			AutomationID: "auto-1", Status: []domain.ContactAutomationStatus{domain.ContactAutomationStatusActive},
			ScheduledBy: &scheduledBy, Limit: 10, Offset: 5,
		})
		require.NoError(t, err)
		assert.Equal(t, 2, count)
		require.Len(t, cas, 2)
		assert.Equal(t, "active", string(cas[0].Status))
		assert.Equal(t, float64(1), cas[0].Context["k"])
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("update with a nil transaction writes all mutable columns", func(t *testing.T) {
		_, mock, repo := setupAutomationMock(t)
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE contact_automations SET current_node_id = $1, status = $2, exit_reason = $3, scheduled_at = $4, context = $5, retry_count = $6, last_error = $7, last_retry_at = $8, max_retries = $9 WHERE id = $10`)).
			WithArgs("n2", domain.ContactAutomationStatus("paused"), nil, nil, []byte(`{"a":2}`), 1, nil, nil, 3, "ca-9").
			WillReturnResult(sqlmock.NewResult(0, 1))
		node := "n2"
		err := repo.UpdateContactAutomationTx(ctx, nil, "ws-1", &domain.ContactAutomation{
			ID: "ca-9", CurrentNodeID: &node, Status: domain.ContactAutomationStatus("paused"),
			Context: map[string]interface{}{"a": float64(2)}, RetryCount: 1, MaxRetries: 3,
		})
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("zero-row update reports not found by id", func(t *testing.T) {
		_, mock, repo := setupAutomationMock(t)
		mock.ExpectExec(`UPDATE contact_automations`).WillReturnResult(sqlmock.NewResult(0, 0))
		err := repo.UpdateContactAutomationTx(ctx, nil, "ws-1", &domain.ContactAutomation{ID: "gone"})
		require.EqualError(t, err, "contact automation not found: gone")
		require.NoError(t, mock.ExpectationsWereMet())
	})
}
