package repository

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

// Task Tx variants share the scheduler's row contract: not-found is a plain
// "task not found", updates verify affected rows where the caller depends
// on it, and state is stored as the exact marshaled JSON.

var taskCols = []string{
	"id", "workspace_id", "type", "status", "progress", "state",
	"error_message", "created_at", "updated_at", "last_run_at",
	"completed_at", "next_run_after", "timeout_after",
	"max_runtime", "max_retries", "retry_count", "retry_interval",
	"broadcast_id", "recurring_interval", "integration_id",
}

func TestTaskRepository_GetTx_Branches(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)

	run := func(t *testing.T, expect func(mock sqlmock.Sqlmock)) (*domain.Task, error) {
		_, mock, repo := setupTaskMock(t)
		mock.ExpectBegin()
		expect(mock)
		mock.ExpectCommit()
		var got *domain.Task
		var gotErr error
		err := repo.WithTransaction(ctx, func(tx *sql.Tx) error {
			got, gotErr = repo.GetTx(ctx, tx, "ws-1", "task-1")
			return gotErr
		})
		// WithTransaction wraps fn errors; unwrap one level for assertions.
		if gotErr != nil {
			return nil, gotErr
		}
		require.NoError(t, err)
		return got, nil
	}

	t.Run("no row is the plain not-found error", func(t *testing.T) {
		_, err := run(t, func(mock sqlmock.Sqlmock) {
			mock.ExpectQuery(regexp.QuoteMeta("FOR UPDATE")).
				WithArgs("task-1", "ws-1").
				WillReturnRows(sqlmock.NewRows(taskCols))
		})
		require.EqualError(t, err, "task not found")
	})

	t.Run("driver failure is wrapped", func(t *testing.T) {
		sentinel := errors.New("tx aborted")
		_, err := run(t, func(mock sqlmock.Sqlmock) {
			mock.ExpectQuery(regexp.QuoteMeta("FOR UPDATE")).WillReturnError(sentinel)
		})
		require.ErrorContains(t, err, "failed to get task:")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("a corrupt state blob is an unmarshal error", func(t *testing.T) {
		_, err := run(t, func(mock sqlmock.Sqlmock) {
			mock.ExpectQuery(regexp.QuoteMeta("FOR UPDATE")).
				WillReturnRows(sqlmock.NewRows(taskCols).AddRow(
					"task-1", "ws-1", "send_broadcast", "pending", 0.5, []byte(`{broken`),
					nil, now, now, nil, nil, nil, nil,
					int64(3600), 3, 0, int64(300), nil, nil, nil))
		})
		require.ErrorContains(t, err, "failed to unmarshal state:")
	})

	t.Run("the row maps verbatim including nullables", func(t *testing.T) {
		got, err := run(t, func(mock sqlmock.Sqlmock) {
			mock.ExpectQuery(regexp.QuoteMeta("FOR UPDATE")).
				WillReturnRows(sqlmock.NewRows(taskCols).AddRow(
					"task-1", "ws-1", "send_broadcast", "running", 0.75, []byte(`{"progress":0.75}`),
					"last error", now, now, now, nil, now, nil,
					int64(3600), 3, 1, int64(300), "b-1", int64(86400), "int-1"))
		})
		require.NoError(t, err)
		assert.Equal(t, domain.TaskStatusRunning, got.Status)
		assert.Equal(t, 0.75, got.Progress)
		require.NotNil(t, got.State)
		require.NotNil(t, got.ErrorMessage)
		assert.Equal(t, "last error", *got.ErrorMessage)
		require.NotNil(t, got.BroadcastID)
		assert.Equal(t, "b-1", *got.BroadcastID)
		require.NotNil(t, got.RecurringInterval)
		assert.Equal(t, int64(86400), *got.RecurringInterval)
		require.NotNil(t, got.IntegrationID)
		assert.Equal(t, "int-1", *got.IntegrationID)
	})
}

func TestTaskRepository_MarkAsPendingTx_Branches(t *testing.T) {
	ctx := context.Background()

	t.Run("marks pending, resets retries, and stores the state JSON", func(t *testing.T) {
		_, mock, repo := setupTaskMock(t)
		next := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta("UPDATE tasks")).
			WithArgs(domain.TaskStatusPending, 0.5, []byte(`{"progress":0.5}`),
				sqlmock.AnyArg(), next, nil, 0, "task-1", "ws-1").
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		err := repo.WithTransaction(ctx, func(tx *sql.Tx) error {
			return repo.MarkAsPendingTx(ctx, tx, "ws-1", "task-1", next, 0.5,
				&domain.TaskState{Progress: 0.5})
		})
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("exec failure is wrapped with the operation", func(t *testing.T) {
		_, mock, repo := setupTaskMock(t)
		sentinel := errors.New("deadlock")
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta("UPDATE tasks")).WillReturnError(sentinel)
		mock.ExpectRollback()

		err := repo.WithTransaction(ctx, func(tx *sql.Tx) error {
			return repo.MarkAsPendingTx(ctx, tx, "ws-1", "task-1", time.Now(), 0, nil)
		})
		require.ErrorContains(t, err, "failed to mark task as pending:")
		assert.ErrorIs(t, err, sentinel)
	})
}

func TestTaskRepository_ReleaseTaskTx_Branches(t *testing.T) {
	ctx := context.Background()

	t.Run("release clears the run markers back to pending", func(t *testing.T) {
		_, mock, repo := setupTaskMock(t)
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta("UPDATE tasks")).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		err := repo.WithTransaction(ctx, func(tx *sql.Tx) error {
			return repo.ReleaseTaskTx(ctx, tx, "ws-1", "task-1", "worker stopped", time.Now())
		})
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("exec failure is wrapped with the operation", func(t *testing.T) {
		_, mock, repo := setupTaskMock(t)
		sentinel := errors.New("gone")
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta("UPDATE tasks")).WillReturnError(sentinel)
		mock.ExpectRollback()

		err := repo.WithTransaction(ctx, func(tx *sql.Tx) error {
			return repo.ReleaseTaskTx(ctx, tx, "ws-1", "task-1", "worker stopped", time.Now())
		})
		require.ErrorContains(t, err, "failed to release task:")
		assert.ErrorIs(t, err, sentinel)
	})
}
