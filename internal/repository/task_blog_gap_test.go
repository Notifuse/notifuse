package repository

import (
	"context"
	"database/sql"
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
)

// Gap coverage for task_postgres.go / blog_postgres.go branches left below
// 100% by the existing suites: RowsAffected errors, rows.Err() iteration
// errors, Tx-variant not-found contracts, and downstream-must-not-run guards.
// Tx variants are exercised on a real sqlmock transaction (Begin only) so the
// WithTransaction wrapper cannot mask which layer produced an error.

var gapCatCols = []string{"id", "slug", "settings", "created_at", "updated_at", "deleted_at"}

func gapBeginTaskTx(t *testing.T) (*sql.DB, sqlmock.Sqlmock, *TaskRepository, *sql.Tx) {
	t.Helper()
	db, mock, repo := setupTaskMock(t)
	mock.ExpectBegin()
	tx, err := db.Begin()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db, mock, repo, tx
}

func gapBlogCategoryRepo(t *testing.T) (*blogCategoryRepository, *mocks.MockWorkspaceRepository) {
	t.Helper()
	ctrl := gomock.NewController(t)
	ws := mocks.NewMockWorkspaceRepository(ctrl)
	return NewBlogCategoryRepository(ws).(*blogCategoryRepository), ws
}

func gapBlogPostRepo(t *testing.T) (*blogPostRepository, *mocks.MockWorkspaceRepository) {
	t.Helper()
	ctrl := gomock.NewController(t)
	ws := mocks.NewMockWorkspaceRepository(ctrl)
	return NewBlogPostRepository(ws).(*blogPostRepository), ws
}

func gapBlogTx(t *testing.T) (*sql.DB, sqlmock.Sqlmock, *sql.Tx) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	mock.ExpectBegin()
	tx, err := db.Begin()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db, mock, tx
}

func TestTaskBlogGap_TaskCreateTx_PreservesIDStatusAndNormalizesUTC(t *testing.T) {
	_, mock, repo, tx := gapBeginTaskTx(t)
	defer func() { _ = tx.Rollback() }()
	ctx := context.Background()

	loc := time.FixedZone("UTC+2", 2*60*60)
	nextRun := time.Date(2026, 6, 1, 12, 0, 0, 0, loc)
	timeout := time.Date(2026, 6, 1, 13, 0, 0, 0, loc)
	lastRun := time.Date(2026, 5, 1, 9, 0, 0, 0, loc)
	completed := time.Date(2026, 5, 2, 9, 0, 0, 0, loc)
	errMsg := "previous failure"
	broadcastID := "b-7"
	integrationID := "int-9"
	var recurring int64 = 3600

	task := &domain.Task{
		ID: "task-fixed", Type: "send_broadcast", Status: domain.TaskStatusRunning,
		Progress: 0.5, State: &domain.TaskState{Progress: 0.5}, ErrorMessage: &errMsg,
		NextRunAfter: &nextRun, TimeoutAfter: &timeout, LastRunAt: &lastRun, CompletedAt: &completed,
		MaxRuntime: 120, MaxRetries: 3, RetryCount: 1, RetryInterval: 60,
		BroadcastID: &broadcastID, RecurringInterval: &recurring, IntegrationID: &integrationID,
	}

	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO tasks")).
		WithArgs(
			"task-fixed", "ws-1", "send_broadcast", domain.TaskStatusRunning, 0.5,
			[]byte(`{"progress":0.5}`), &errMsg, sqlmock.AnyArg(), sqlmock.AnyArg(),
			lastRun.UTC(), completed.UTC(), nextRun.UTC(), timeout.UTC(),
			int64(120), 3, 1, int64(60), &broadcastID, &recurring, &integrationID,
		).WillReturnResult(sqlmock.NewResult(1, 1))

	err := repo.CreateTx(ctx, tx, "ws-1", task)
	require.NoError(t, err)
	// Production normalizes the caller's pointers in place to UTC.
	assert.Equal(t, time.UTC, task.NextRunAfter.Location())
	assert.Equal(t, nextRun.UTC(), *task.NextRunAfter)
	assert.Equal(t, timeout.UTC(), *task.TimeoutAfter)
	assert.Equal(t, lastRun.UTC(), *task.LastRunAt)
	assert.Equal(t, completed.UTC(), *task.CompletedAt)
	assert.Equal(t, time.UTC, task.CreatedAt.Location())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestTaskBlogGap_TaskUpdateTx_RowsAffectedBranches(t *testing.T) {
	ctx := context.Background()
	newTask := func() *domain.Task {
		return &domain.Task{ID: "task-1", Type: "send_broadcast", Status: domain.TaskStatusPending,
			State: &domain.TaskState{Progress: 0.5}, MaxRuntime: 60, MaxRetries: 3, RetryInterval: 30}
	}

	t.Run("zero rows is the plain not-found error", func(t *testing.T) {
		_, mock, repo, tx := gapBeginTaskTx(t)
		defer func() { _ = tx.Rollback() }()
		mock.ExpectExec(regexp.QuoteMeta("UPDATE tasks")).
			WithArgs("task-1", "ws-1", "send_broadcast", domain.TaskStatusPending, 0.0,
				[]byte(`{"progress":0.5}`), (*string)(nil), sqlmock.AnyArg(),
				(*time.Time)(nil), (*time.Time)(nil), (*time.Time)(nil), (*time.Time)(nil),
				int64(60), 3, 0, int64(30), (*string)(nil), (*int64)(nil), (*string)(nil)).
			WillReturnResult(sqlmock.NewResult(0, 0))
		err := repo.UpdateTx(ctx, tx, "ws-1", newTask())
		require.EqualError(t, err, "task not found")
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("rows-affected failure is wrapped", func(t *testing.T) {
		_, mock, repo, tx := gapBeginTaskTx(t)
		defer func() { _ = tx.Rollback() }()
		sentinel := errors.New("result gone")
		mock.ExpectExec(regexp.QuoteMeta("UPDATE tasks")).
			WillReturnResult(sqlmock.NewErrorResult(sentinel))
		err := repo.UpdateTx(ctx, tx, "ws-1", newTask())
		require.ErrorContains(t, err, "failed to get rows affected:")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("exec failure is wrapped", func(t *testing.T) {
		_, mock, repo, tx := gapBeginTaskTx(t)
		defer func() { _ = tx.Rollback() }()
		sentinel := errors.New("deadlock")
		mock.ExpectExec(regexp.QuoteMeta("UPDATE tasks")).WillReturnError(sentinel)
		err := repo.UpdateTx(ctx, tx, "ws-1", newTask())
		require.ErrorContains(t, err, "failed to update task:")
		assert.ErrorIs(t, err, sentinel)
	})
}

func TestTaskBlogGap_TaskDelete_RowsAffectedBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("rows-affected failure is wrapped", func(t *testing.T) {
		db, mock, repo := setupTaskMock(t)
		defer func() { _ = db.Close() }()
		sentinel := errors.New("result gone")
		mock.ExpectExec(regexp.QuoteMeta("DELETE FROM tasks WHERE id = $1 AND workspace_id = $2")).
			WithArgs("task-1", "ws-1").WillReturnResult(sqlmock.NewErrorResult(sentinel))
		err := repo.Delete(ctx, "ws-1", "task-1")
		require.ErrorContains(t, err, "failed to get rows affected:")
		assert.ErrorIs(t, err, sentinel)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("zero rows is the plain not-found error", func(t *testing.T) {
		db, mock, repo := setupTaskMock(t)
		defer func() { _ = db.Close() }()
		mock.ExpectExec(regexp.QuoteMeta("DELETE FROM tasks")).
			WithArgs("task-1", "ws-1").WillReturnResult(sqlmock.NewResult(0, 0))
		err := repo.Delete(ctx, "ws-1", "task-1")
		require.EqualError(t, err, "task not found")
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestTaskBlogGap_TaskGetByIntegrationIDTx_Branches(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)

	t.Run("no row returns the ErrTaskNotFound sentinel", func(t *testing.T) {
		_, mock, repo, tx := gapBeginTaskTx(t)
		defer func() { _ = tx.Rollback() }()
		mock.ExpectQuery(regexp.QuoteMeta("integration_id = $2")).
			WithArgs("ws-1", "int-1").WillReturnRows(sqlmock.NewRows(taskCols))
		task, err := repo.GetTaskByIntegrationIDTx(ctx, tx, "ws-1", "int-1")
		assert.Nil(t, task)
		require.ErrorIs(t, err, domain.ErrTaskNotFound)
		require.EqualError(t, err, "task not found")
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("driver failure is wrapped", func(t *testing.T) {
		_, mock, repo, tx := gapBeginTaskTx(t)
		defer func() { _ = tx.Rollback() }()
		sentinel := errors.New("tx aborted")
		mock.ExpectQuery(regexp.QuoteMeta("FROM tasks")).WillReturnError(sentinel)
		_, err := repo.GetTaskByIntegrationIDTx(ctx, tx, "ws-1", "int-1")
		require.ErrorContains(t, err, "failed to get task by integration ID:")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("row with all-null optionals leaves pointers and state nil", func(t *testing.T) {
		_, mock, repo, tx := gapBeginTaskTx(t)
		defer func() { _ = tx.Rollback() }()
		mock.ExpectQuery(regexp.QuoteMeta("status NOT IN ('completed', 'failed')")).
			WithArgs("ws-1", "int-1").
			WillReturnRows(sqlmock.NewRows(taskCols).AddRow(
				"task-1", "ws-1", "sync", "pending", 0.0, nil,
				nil, now, now, nil, nil, nil, nil,
				int64(60), 3, 0, int64(30), nil, nil, "int-1"))
		task, err := repo.GetTaskByIntegrationIDTx(ctx, tx, "ws-1", "int-1")
		require.NoError(t, err)
		assert.Nil(t, task.State)
		assert.Nil(t, task.ErrorMessage)
		assert.Nil(t, task.BroadcastID)
		assert.Nil(t, task.LastRunAt)
		require.NotNil(t, task.IntegrationID)
		assert.Equal(t, "int-1", *task.IntegrationID)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("corrupt state is an unmarshal error", func(t *testing.T) {
		_, mock, repo, tx := gapBeginTaskTx(t)
		defer func() { _ = tx.Rollback() }()
		mock.ExpectQuery(regexp.QuoteMeta("FROM tasks")).
			WillReturnRows(sqlmock.NewRows(taskCols).AddRow(
				"task-1", "ws-1", "sync", "pending", 0.0, []byte(`{broken`),
				nil, now, now, nil, nil, nil, nil,
				int64(60), 3, 0, int64(30), nil, nil, "int-1"))
		_, err := repo.GetTaskByIntegrationIDTx(ctx, tx, "ws-1", "int-1")
		require.ErrorContains(t, err, "failed to unmarshal state:")
	})
}

func TestTaskBlogGap_TaskGetByBroadcastIDTx_Branches(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)

	t.Run("no row names the broadcast ID in the error", func(t *testing.T) {
		_, mock, repo, tx := gapBeginTaskTx(t)
		defer func() { _ = tx.Rollback() }()
		mock.ExpectQuery(regexp.QuoteMeta("AND type = 'send_broadcast'")).
			WithArgs("ws-1", "b-42").WillReturnRows(sqlmock.NewRows(taskCols))
		task, err := repo.GetTaskByBroadcastIDTx(ctx, tx, "ws-1", "b-42")
		assert.Nil(t, task)
		require.EqualError(t, err, "task not found for broadcast ID b-42")
	})

	t.Run("corrupt state is an unmarshal error", func(t *testing.T) {
		_, mock, repo, tx := gapBeginTaskTx(t)
		defer func() { _ = tx.Rollback() }()
		mock.ExpectQuery(regexp.QuoteMeta("FROM tasks")).
			WillReturnRows(sqlmock.NewRows(taskCols).AddRow(
				"task-1", "ws-1", "send_broadcast", "pending", 0.0, []byte(`{broken`),
				nil, now, now, nil, nil, nil, nil,
				int64(60), 3, 0, int64(30), "b-42", nil, nil))
		_, err := repo.GetTaskByBroadcastIDTx(ctx, tx, "ws-1", "b-42")
		require.ErrorContains(t, err, "failed to unmarshal state:")
	})
}

func TestTaskBlogGap_TaskSaveStateTx_ExactSQL(t *testing.T) {
	ctx := context.Background()

	t.Run("squirrel SQL, placeholder order and args are exact", func(t *testing.T) {
		var seen []string
		_, mock, repo := setupTaskMockCapturingSQL(t, &seen)
		mock.ExpectBegin()
		tx, err := repo.systemDB.Begin()
		require.NoError(t, err)
		defer func() { _ = tx.Rollback() }()
		mock.ExpectExec(regexp.QuoteMeta("UPDATE tasks")).
			WithArgs(0.75, []byte(`{"progress":0.75}`), sqlmock.AnyArg(),
				"task-1", domain.TaskStatusRunning, "ws-1").
			WillReturnResult(sqlmock.NewResult(0, 1))
		err = repo.SaveStateTx(ctx, tx, "ws-1", "task-1", 0.75, &domain.TaskState{Progress: 0.75})
		require.NoError(t, err)
		require.Len(t, seen, 1)
		assert.Equal(t,
			"UPDATE tasks SET progress = $1, state = $2, updated_at = $3 WHERE (id = $4 AND status = $5 AND workspace_id = $6)",
			seen[0])
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("exec failure is wrapped", func(t *testing.T) {
		_, mock, repo, tx := gapBeginTaskTx(t)
		defer func() { _ = tx.Rollback() }()
		sentinel := errors.New("write blocked")
		mock.ExpectExec(regexp.QuoteMeta("UPDATE tasks")).WillReturnError(sentinel)
		err := repo.SaveStateTx(ctx, tx, "ws-1", "task-1", 0.5, &domain.TaskState{Progress: 0.5})
		require.ErrorContains(t, err, "failed to save task state:")
		assert.ErrorIs(t, err, sentinel)
	})
}

func TestTaskBlogGap_TaskMarkTx_RowsAffectedBranches(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)

	t.Run("MarkAsCompletedTx rows-affected failure is wrapped", func(t *testing.T) {
		_, mock, repo, tx := gapBeginTaskTx(t)
		defer func() { _ = tx.Rollback() }()
		sentinel := errors.New("result gone")
		mock.ExpectExec(regexp.QuoteMeta("UPDATE tasks")).
			WillReturnResult(sqlmock.NewErrorResult(sentinel))
		err := repo.MarkAsCompletedTx(ctx, tx, "ws-1", "task-1", &domain.TaskState{Progress: 1})
		require.ErrorContains(t, err, "failed to get rows affected:")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("MarkAsCompletedTx zero rows is not-found", func(t *testing.T) {
		_, mock, repo, tx := gapBeginTaskTx(t)
		defer func() { _ = tx.Rollback() }()
		mock.ExpectExec(regexp.QuoteMeta("UPDATE tasks")).
			WillReturnResult(sqlmock.NewResult(0, 0))
		err := repo.MarkAsCompletedTx(ctx, tx, "ws-1", "task-1", nil)
		require.EqualError(t, err, "task not found")
	})

	t.Run("MarkAsRunningTx zero rows is ErrTaskAlreadyRunning with the ID", func(t *testing.T) {
		_, mock, repo, tx := gapBeginTaskTx(t)
		defer func() { _ = tx.Rollback() }()
		mock.ExpectExec(regexp.QuoteMeta("UPDATE tasks")).
			WillReturnResult(sqlmock.NewResult(0, 0))
		err := repo.MarkAsRunningTx(ctx, tx, "ws-1", "task-9", now)
		var already *domain.ErrTaskAlreadyRunning
		require.ErrorAs(t, err, &already)
		assert.Equal(t, "task-9", already.TaskID)
		require.EqualError(t, err, "task already running [task-9]")
	})

	t.Run("MarkAsRunningTx rows-affected failure is wrapped", func(t *testing.T) {
		_, mock, repo, tx := gapBeginTaskTx(t)
		defer func() { _ = tx.Rollback() }()
		sentinel := errors.New("result gone")
		mock.ExpectExec(regexp.QuoteMeta("UPDATE tasks")).
			WillReturnResult(sqlmock.NewErrorResult(sentinel))
		err := repo.MarkAsRunningTx(ctx, tx, "ws-1", "task-9", now)
		require.ErrorContains(t, err, "failed to get rows affected:")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("ReleaseTaskTx zero rows is ErrTaskNotRunning", func(t *testing.T) {
		_, mock, repo, tx := gapBeginTaskTx(t)
		defer func() { _ = tx.Rollback() }()
		mock.ExpectExec(regexp.QuoteMeta("UPDATE tasks")).
			WillReturnResult(sqlmock.NewResult(0, 0))
		err := repo.ReleaseTaskTx(ctx, tx, "ws-1", "task-1", "worker stopped", now)
		require.ErrorIs(t, err, domain.ErrTaskNotRunning)
		require.EqualError(t, err, "task is no longer running")
	})

	t.Run("ReleaseTaskTx rows-affected failure is wrapped", func(t *testing.T) {
		_, mock, repo, tx := gapBeginTaskTx(t)
		defer func() { _ = tx.Rollback() }()
		sentinel := errors.New("result gone")
		mock.ExpectExec(regexp.QuoteMeta("UPDATE tasks")).
			WillReturnResult(sqlmock.NewErrorResult(sentinel))
		err := repo.ReleaseTaskTx(ctx, tx, "ws-1", "task-1", "worker stopped", now)
		require.ErrorContains(t, err, "failed to get rows affected:")
		assert.ErrorIs(t, err, sentinel)
	})
}

func TestTaskBlogGap_TaskMarkAsFailedTx_RetryBranches(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	taskRow := func(retryCount, maxRetries int) *sqlmock.Rows {
		return sqlmock.NewRows(taskCols).AddRow(
			"task-1", "ws-1", "send_broadcast", "running", 0.5, []byte(`{"progress":0.5}`),
			nil, now, now, nil, nil, nil, nil,
			int64(60), maxRetries, retryCount, int64(60), nil, nil, nil)
	}

	t.Run("lookup failure aborts before any UPDATE", func(t *testing.T) {
		_, mock, repo, tx := gapBeginTaskTx(t)
		defer func() { _ = tx.Rollback() }()
		// No ExpectExec is registered: a downstream UPDATE would fail the
		// ExpectationsWereMet check and change the error text.
		mock.ExpectQuery(regexp.QuoteMeta("FOR UPDATE")).WillReturnError(sql.ErrNoRows)
		err := repo.MarkAsFailedTx(ctx, tx, "ws-1", "task-1", "boom")
		require.ErrorContains(t, err, "failed to get task for retry check:")
		require.ErrorContains(t, err, "task not found")
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("retries left reschedules as pending with retry_count + 1", func(t *testing.T) {
		_, mock, repo, tx := gapBeginTaskTx(t)
		defer func() { _ = tx.Rollback() }()
		mock.ExpectQuery(regexp.QuoteMeta("FOR UPDATE")).
			WithArgs("task-1", "ws-1").WillReturnRows(taskRow(0, 3))
		mock.ExpectExec(regexp.QuoteMeta("UPDATE tasks")).
			WithArgs(domain.TaskStatusPending, "boom", sqlmock.AnyArg(), sqlmock.AnyArg(), nil, "task-1", "ws-1").
			WillReturnResult(sqlmock.NewResult(0, 1))
		err := repo.MarkAsFailedTx(ctx, tx, "ws-1", "task-1", "boom")
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("budget exhausted marks failed with no next run", func(t *testing.T) {
		_, mock, repo, tx := gapBeginTaskTx(t)
		defer func() { _ = tx.Rollback() }()
		mock.ExpectQuery(regexp.QuoteMeta("FOR UPDATE")).
			WithArgs("task-1", "ws-1").WillReturnRows(taskRow(3, 3))
		mock.ExpectExec(regexp.QuoteMeta("UPDATE tasks")).
			WithArgs(domain.TaskStatusFailed, "boom", sqlmock.AnyArg(), nil, nil, "task-1", "ws-1").
			WillReturnResult(sqlmock.NewResult(0, 1))
		err := repo.MarkAsFailedTx(ctx, tx, "ws-1", "task-1", "boom")
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("update rows-affected failure is wrapped", func(t *testing.T) {
		_, mock, repo, tx := gapBeginTaskTx(t)
		defer func() { _ = tx.Rollback() }()
		sentinel := errors.New("result gone")
		mock.ExpectQuery(regexp.QuoteMeta("FOR UPDATE")).WillReturnRows(taskRow(0, 3))
		mock.ExpectExec(regexp.QuoteMeta("UPDATE tasks")).
			WillReturnResult(sqlmock.NewErrorResult(sentinel))
		err := repo.MarkAsFailedTx(ctx, tx, "ws-1", "task-1", "boom")
		require.ErrorContains(t, err, "failed to get rows affected:")
		assert.ErrorIs(t, err, sentinel)
	})
}

func TestTaskBlogGap_TaskExistsTx_Branches(t *testing.T) {
	ctx := context.Background()

	t.Run("existing row is true", func(t *testing.T) {
		_, mock, repo, tx := gapBeginTaskTx(t)
		defer func() { _ = tx.Rollback() }()
		mock.ExpectQuery(regexp.QuoteMeta("SELECT 1 FROM tasks WHERE id = $1 AND workspace_id = $2")).
			WithArgs("task-1", "ws-1").
			WillReturnRows(sqlmock.NewRows([]string{"?column?"}).AddRow(1))
		exists, err := repo.taskExistsTx(ctx, tx, "ws-1", "task-1")
		require.NoError(t, err)
		assert.True(t, exists)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("no row is false without error", func(t *testing.T) {
		_, mock, repo, tx := gapBeginTaskTx(t)
		defer func() { _ = tx.Rollback() }()
		mock.ExpectQuery(regexp.QuoteMeta("SELECT 1 FROM tasks")).
			WithArgs("task-1", "ws-1").WillReturnRows(sqlmock.NewRows([]string{"?column?"}))
		exists, err := repo.taskExistsTx(ctx, tx, "ws-1", "task-1")
		require.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("driver error is returned unwrapped", func(t *testing.T) {
		_, mock, repo, tx := gapBeginTaskTx(t)
		defer func() { _ = tx.Rollback() }()
		sentinel := errors.New("tx aborted")
		mock.ExpectQuery(regexp.QuoteMeta("SELECT 1 FROM tasks")).WillReturnError(sentinel)
		// Real contract: taskExistsTx returns the raw driver error (no fmt
		// wrap); callers such as MarkAsPausedTx add "failed to look up task:".
		exists, err := repo.taskExistsTx(ctx, tx, "ws-1", "task-1")
		assert.False(t, exists)
		require.EqualError(t, err, "tx aborted")
		assert.ErrorIs(t, err, sentinel)
	})
}

func TestTaskBlogGap_TaskListAndBatch_IterationErrors(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	validRow := func(rows *sqlmock.Rows) *sqlmock.Rows {
		return rows.AddRow("task-1", "ws-1", "send_broadcast", "pending", 0.0, []byte(`{}`),
			nil, now, now, nil, nil, nil, nil, int64(60), 3, 0, int64(30), nil, nil, nil)
	}

	t.Run("List surfaces rows.Err after the loop", func(t *testing.T) {
		db, mock, repo := setupTaskMock(t)
		defer func() { _ = db.Close() }()
		sentinel := errors.New("cursor died")
		mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM tasks")).
			WithArgs("ws-1").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
		rows := validRow(sqlmock.NewRows(taskCols))
		rows.RowError(0, sentinel)
		mock.ExpectQuery(regexp.QuoteMeta("FROM tasks")).WillReturnRows(rows)
		tasks, total, err := repo.List(ctx, "ws-1", domain.TaskFilter{Limit: 10})
		assert.Nil(t, tasks)
		assert.Equal(t, 0, total)
		require.ErrorContains(t, err, "error iterating task rows:")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("List data query failure is wrapped and count already consumed", func(t *testing.T) {
		db, mock, repo := setupTaskMock(t)
		defer func() { _ = db.Close() }()
		sentinel := errors.New("statement timeout")
		mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM tasks")).
			WithArgs("ws-1").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(4))
		mock.ExpectQuery(regexp.QuoteMeta("FROM tasks")).WillReturnError(sentinel)
		_, _, err := repo.List(ctx, "ws-1", domain.TaskFilter{Limit: 10})
		require.ErrorContains(t, err, "failed to list tasks:")
		assert.ErrorIs(t, err, sentinel)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("GetNextBatch surfaces rows.Err after the loop", func(t *testing.T) {
		db, mock, repo := setupTaskMock(t)
		defer func() { _ = db.Close() }()
		sentinel := errors.New("cursor died")
		rows := validRow(sqlmock.NewRows(taskCols))
		rows.RowError(0, sentinel)
		mock.ExpectQuery(regexp.QuoteMeta("FOR UPDATE SKIP LOCKED")).WillReturnRows(rows)
		tasks, err := repo.GetNextBatch(ctx, 5)
		assert.Nil(t, tasks)
		require.ErrorContains(t, err, "error iterating task rows:")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("GetNextBatch empty result is an empty success", func(t *testing.T) {
		db, mock, repo := setupTaskMock(t)
		defer func() { _ = db.Close() }()
		mock.ExpectQuery(regexp.QuoteMeta("FOR UPDATE SKIP LOCKED")).
			WillReturnRows(sqlmock.NewRows(taskCols))
		tasks, err := repo.GetNextBatch(ctx, 5)
		require.NoError(t, err)
		assert.Len(t, tasks, 0)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestTaskBlogGap_BlogCategoryTx_GetBranches(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)

	t.Run("GetCategoryTx no row is the plain not-found error", func(t *testing.T) {
		repo, _ := gapBlogCategoryRepo(t)
		_, mock, tx := gapBlogTx(t)
		defer func() { _ = tx.Rollback() }()
		mock.ExpectQuery(regexp.QuoteMeta("WHERE id = $1 AND deleted_at IS NULL")).
			WithArgs("cat-404").WillReturnRows(sqlmock.NewRows(gapCatCols))
		cat, err := repo.GetCategoryTx(ctx, tx, "cat-404")
		assert.Nil(t, cat)
		require.EqualError(t, err, "blog category not found")
		assert.NotErrorIs(t, err, sql.ErrNoRows)
	})

	t.Run("GetCategoryBySlugTx binds slug against the slug column", func(t *testing.T) {
		repo, _ := gapBlogCategoryRepo(t)
		_, mock, tx := gapBlogTx(t)
		defer func() { _ = tx.Rollback() }()
		mock.ExpectQuery(regexp.QuoteMeta("WHERE slug = $1 AND deleted_at IS NULL")).
			WithArgs("tech").
			WillReturnRows(sqlmock.NewRows(gapCatCols).
				AddRow("cat-1", "tech", []byte(`{"name":"Tech"}`), now, now, nil))
		cat, err := repo.GetCategoryBySlugTx(ctx, tx, "tech")
		require.NoError(t, err)
		assert.Equal(t, "cat-1", cat.ID)
		assert.Equal(t, "Tech", cat.Settings.Name)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("GetCategoryTx driver failure is wrapped", func(t *testing.T) {
		repo, _ := gapBlogCategoryRepo(t)
		_, mock, tx := gapBlogTx(t)
		defer func() { _ = tx.Rollback() }()
		sentinel := errors.New("tx aborted")
		mock.ExpectQuery(regexp.QuoteMeta("FROM blog_categories")).WillReturnError(sentinel)
		_, err := repo.GetCategoryTx(ctx, tx, "cat-1")
		require.ErrorContains(t, err, "failed to get blog category:")
		assert.ErrorIs(t, err, sentinel)
	})
}

func TestTaskBlogGap_BlogCategoryTx_UpdateDeleteBranches(t *testing.T) {
	ctx := context.Background()
	cat := func() *domain.BlogCategory {
		return &domain.BlogCategory{ID: "cat-1", Slug: "tech",
			Settings: domain.BlogCategorySettings{Name: "Tech"}}
	}

	t.Run("UpdateCategoryTx zero rows is not-found with slug-first args", func(t *testing.T) {
		repo, _ := gapBlogCategoryRepo(t)
		_, mock, tx := gapBlogTx(t)
		defer func() { _ = tx.Rollback() }()
		mock.ExpectExec(regexp.QuoteMeta("SET slug = $1, settings = $2, updated_at = $3")).
			WithArgs("tech", sqlmock.AnyArg(), sqlmock.AnyArg(), "cat-1").
			WillReturnResult(sqlmock.NewResult(0, 0))
		err := repo.UpdateCategoryTx(ctx, tx, cat())
		require.EqualError(t, err, "blog category not found")
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("UpdateCategoryTx rows-affected failure is wrapped", func(t *testing.T) {
		repo, _ := gapBlogCategoryRepo(t)
		_, mock, tx := gapBlogTx(t)
		defer func() { _ = tx.Rollback() }()
		sentinel := errors.New("result gone")
		mock.ExpectExec(regexp.QuoteMeta("UPDATE blog_categories")).
			WillReturnResult(sqlmock.NewErrorResult(sentinel))
		err := repo.UpdateCategoryTx(ctx, tx, cat())
		require.ErrorContains(t, err, "failed to get rows affected:")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("UpdateCategoryTx exec failure is wrapped", func(t *testing.T) {
		repo, _ := gapBlogCategoryRepo(t)
		_, mock, tx := gapBlogTx(t)
		defer func() { _ = tx.Rollback() }()
		sentinel := errors.New("deadlock")
		mock.ExpectExec(regexp.QuoteMeta("UPDATE blog_categories")).WillReturnError(sentinel)
		err := repo.UpdateCategoryTx(ctx, tx, cat())
		require.ErrorContains(t, err, "failed to update blog category:")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("DeleteCategoryTx zero rows is not-found", func(t *testing.T) {
		repo, _ := gapBlogCategoryRepo(t)
		_, mock, tx := gapBlogTx(t)
		defer func() { _ = tx.Rollback() }()
		mock.ExpectExec(regexp.QuoteMeta("SET deleted_at = $1")).
			WithArgs(sqlmock.AnyArg(), "cat-1").WillReturnResult(sqlmock.NewResult(0, 0))
		err := repo.DeleteCategoryTx(ctx, tx, "cat-1")
		require.EqualError(t, err, "blog category not found")
	})

	t.Run("DeleteCategoryTx rows-affected failure is wrapped", func(t *testing.T) {
		repo, _ := gapBlogCategoryRepo(t)
		_, mock, tx := gapBlogTx(t)
		defer func() { _ = tx.Rollback() }()
		sentinel := errors.New("result gone")
		mock.ExpectExec(regexp.QuoteMeta("UPDATE blog_categories")).
			WillReturnResult(sqlmock.NewErrorResult(sentinel))
		err := repo.DeleteCategoryTx(ctx, tx, "cat-1")
		require.ErrorContains(t, err, "failed to get rows affected:")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("DeleteCategoryTx exec failure is wrapped", func(t *testing.T) {
		repo, _ := gapBlogCategoryRepo(t)
		_, mock, tx := gapBlogTx(t)
		defer func() { _ = tx.Rollback() }()
		sentinel := errors.New("deadlock")
		mock.ExpectExec(regexp.QuoteMeta("UPDATE blog_categories")).WillReturnError(sentinel)
		err := repo.DeleteCategoryTx(ctx, tx, "cat-1")
		require.ErrorContains(t, err, "failed to delete blog category:")
		assert.ErrorIs(t, err, sentinel)
	})
}

func TestTaskBlogGap_BlogPostTx_GetBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("GetPostTx no row is the plain not-found error", func(t *testing.T) {
		repo, _ := gapBlogPostRepo(t)
		_, mock, tx := gapBlogTx(t)
		defer func() { _ = tx.Rollback() }()
		mock.ExpectQuery(regexp.QuoteMeta("WHERE id = $1 AND deleted_at IS NULL")).
			WithArgs("post-404").WillReturnRows(sqlmock.NewRows(blogPostCols))
		post, err := repo.GetPostTx(ctx, tx, "post-404")
		assert.Nil(t, post)
		require.EqualError(t, err, "blog post not found")
		assert.NotErrorIs(t, err, sql.ErrNoRows)
	})

	t.Run("GetPostBySlugTx no row is the plain not-found error", func(t *testing.T) {
		repo, _ := gapBlogPostRepo(t)
		_, mock, tx := gapBlogTx(t)
		defer func() { _ = tx.Rollback() }()
		mock.ExpectQuery(regexp.QuoteMeta("WHERE slug = $1 AND deleted_at IS NULL")).
			WithArgs("gone").WillReturnRows(sqlmock.NewRows(blogPostCols))
		post, err := repo.GetPostBySlugTx(ctx, tx, "gone")
		assert.Nil(t, post)
		require.EqualError(t, err, "blog post not found")
		assert.NotErrorIs(t, err, sql.ErrNoRows)
	})

	t.Run("GetPostTx driver failure is wrapped", func(t *testing.T) {
		repo, _ := gapBlogPostRepo(t)
		_, mock, tx := gapBlogTx(t)
		defer func() { _ = tx.Rollback() }()
		sentinel := errors.New("tx aborted")
		mock.ExpectQuery(regexp.QuoteMeta("FROM blog_posts")).WillReturnError(sentinel)
		_, err := repo.GetPostTx(ctx, tx, "post-1")
		require.ErrorContains(t, err, "failed to get blog post:")
		assert.ErrorIs(t, err, sentinel)
	})
}

func TestTaskBlogGap_BlogPostTx_WriteBranches(t *testing.T) {
	ctx := context.Background()
	published := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	post := func() *domain.BlogPost {
		return &domain.BlogPost{ID: "post-1", CategoryID: "cat-1", Slug: "hello",
			Settings: domain.BlogPostSettings{Title: "Hello"}, PublishedAt: &published}
	}

	t.Run("UpdatePostTx zero rows is not-found with id bound last", func(t *testing.T) {
		repo, _ := gapBlogPostRepo(t)
		_, mock, tx := gapBlogTx(t)
		defer func() { _ = tx.Rollback() }()
		mock.ExpectExec(regexp.QuoteMeta("SET category_id = $1, slug = $2, settings = $3, published_at = $4, updated_at = $5")).
			WithArgs("cat-1", "hello", sqlmock.AnyArg(), &published, sqlmock.AnyArg(), "post-1").
			WillReturnResult(sqlmock.NewResult(0, 0))
		err := repo.UpdatePostTx(ctx, tx, post())
		require.EqualError(t, err, "blog post not found")
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("UpdatePostTx rows-affected failure is wrapped", func(t *testing.T) {
		repo, _ := gapBlogPostRepo(t)
		_, mock, tx := gapBlogTx(t)
		defer func() { _ = tx.Rollback() }()
		sentinel := errors.New("result gone")
		mock.ExpectExec(regexp.QuoteMeta("UPDATE blog_posts")).
			WillReturnResult(sqlmock.NewErrorResult(sentinel))
		err := repo.UpdatePostTx(ctx, tx, post())
		require.ErrorContains(t, err, "failed to get rows affected:")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("DeletePostTx zero rows is not-found", func(t *testing.T) {
		repo, _ := gapBlogPostRepo(t)
		_, mock, tx := gapBlogTx(t)
		defer func() { _ = tx.Rollback() }()
		mock.ExpectExec(regexp.QuoteMeta("SET deleted_at = $1")).
			WithArgs(sqlmock.AnyArg(), "post-1").WillReturnResult(sqlmock.NewResult(0, 0))
		err := repo.DeletePostTx(ctx, tx, "post-1")
		require.EqualError(t, err, "blog post not found")
	})

	t.Run("DeletePostTx exec failure is wrapped", func(t *testing.T) {
		repo, _ := gapBlogPostRepo(t)
		_, mock, tx := gapBlogTx(t)
		defer func() { _ = tx.Rollback() }()
		sentinel := errors.New("deadlock")
		mock.ExpectExec(regexp.QuoteMeta("UPDATE blog_posts")).WillReturnError(sentinel)
		err := repo.DeletePostTx(ctx, tx, "post-1")
		require.ErrorContains(t, err, "failed to delete blog post:")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("PublishPostTx converts an explicit timestamp to UTC", func(t *testing.T) {
		repo, _ := gapBlogPostRepo(t)
		_, mock, tx := gapBlogTx(t)
		defer func() { _ = tx.Rollback() }()
		loc := time.FixedZone("UTC+2", 2*60*60)
		when := time.Date(2026, 6, 1, 12, 0, 0, 0, loc)
		mock.ExpectExec(regexp.QuoteMeta("SET published_at = $1, updated_at = $2")).
			WithArgs(when.UTC(), sqlmock.AnyArg(), "post-1").
			WillReturnResult(sqlmock.NewResult(0, 1))
		err := repo.PublishPostTx(ctx, tx, "post-1", &when)
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("PublishPostTx zero rows names the already-published case", func(t *testing.T) {
		repo, _ := gapBlogPostRepo(t)
		_, mock, tx := gapBlogTx(t)
		defer func() { _ = tx.Rollback() }()
		mock.ExpectExec(regexp.QuoteMeta("UPDATE blog_posts")).
			WillReturnResult(sqlmock.NewResult(0, 0))
		err := repo.PublishPostTx(ctx, tx, "post-1", nil)
		require.EqualError(t, err, "blog post not found or already published")
	})

	t.Run("UnpublishPostTx zero rows names the not-published case", func(t *testing.T) {
		repo, _ := gapBlogPostRepo(t)
		_, mock, tx := gapBlogTx(t)
		defer func() { _ = tx.Rollback() }()
		mock.ExpectExec(regexp.QuoteMeta("SET published_at = NULL, updated_at = $1")).
			WithArgs(sqlmock.AnyArg(), "post-1").WillReturnResult(sqlmock.NewResult(0, 0))
		err := repo.UnpublishPostTx(ctx, tx, "post-1")
		require.EqualError(t, err, "blog post not found or not published")
	})

	t.Run("UnpublishPostTx rows-affected failure is wrapped", func(t *testing.T) {
		repo, _ := gapBlogPostRepo(t)
		_, mock, tx := gapBlogTx(t)
		defer func() { _ = tx.Rollback() }()
		sentinel := errors.New("result gone")
		mock.ExpectExec(regexp.QuoteMeta("UPDATE blog_posts")).
			WillReturnResult(sqlmock.NewErrorResult(sentinel))
		err := repo.UnpublishPostTx(ctx, tx, "post-1")
		require.ErrorContains(t, err, "failed to get rows affected:")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("DeletePostsByCategoryIDTx returns the affected count", func(t *testing.T) {
		repo, _ := gapBlogPostRepo(t)
		_, mock, tx := gapBlogTx(t)
		defer func() { _ = tx.Rollback() }()
		mock.ExpectExec(regexp.QuoteMeta("WHERE category_id = $2 AND deleted_at IS NULL")).
			WithArgs(sqlmock.AnyArg(), "cat-1").WillReturnResult(sqlmock.NewResult(0, 3))
		n, err := repo.DeletePostsByCategoryIDTx(ctx, tx, "cat-1")
		require.NoError(t, err)
		assert.Equal(t, int64(3), n)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("DeletePostsByCategoryIDTx rows-affected failure is wrapped", func(t *testing.T) {
		repo, _ := gapBlogPostRepo(t)
		_, mock, tx := gapBlogTx(t)
		defer func() { _ = tx.Rollback() }()
		sentinel := errors.New("result gone")
		mock.ExpectExec(regexp.QuoteMeta("UPDATE blog_posts")).
			WillReturnResult(sqlmock.NewErrorResult(sentinel))
		n, err := repo.DeletePostsByCategoryIDTx(ctx, tx, "cat-1")
		assert.Equal(t, int64(0), n)
		require.ErrorContains(t, err, "failed to get rows affected:")
		assert.ErrorIs(t, err, sentinel)
	})
}

func TestTaskBlogGap_BlogGetCategoriesByIDs_Branches(t *testing.T) {
	now := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)

	t.Run("empty IDs short-circuit before any workspace lookup", func(t *testing.T) {
		repo, _ := gapBlogCategoryRepo(t) // no GetConnection expectation registered
		got, err := repo.GetCategoriesByIDs(blogCtx(), nil)
		require.NoError(t, err)
		assert.NotNil(t, got)
		assert.Len(t, got, 0)
		got, err = repo.GetCategoriesByIDs(blogCtx(), []string{})
		require.NoError(t, err)
		assert.Len(t, got, 0)
	})

	t.Run("placeholders are numbered in argument order", func(t *testing.T) {
		repo, ws := gapBlogCategoryRepo(t)
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		ws.EXPECT().GetConnection(gomock.Any(), "ws-1").Return(db, nil)
		// No deleted_at filter: deleted categories are returned on purpose
		// so callers can still build URLs for them.
		mock.ExpectQuery(regexp.QuoteMeta("WHERE id IN ($1, $2, $3)")).
			WithArgs("c1", "c2", "c3").
			WillReturnRows(sqlmock.NewRows(gapCatCols).
				AddRow("c1", "tech", []byte(`{"name":"Tech"}`), now, now, nil).
				AddRow("c2", "life", []byte(`{"name":"Life"}`), now, now, &now))
		got, err := repo.GetCategoriesByIDs(blogCtx(), []string{"c1", "c2", "c3"})
		require.NoError(t, err)
		require.Len(t, got, 2)
		assert.Equal(t, "c1", got[0].ID)
		require.NotNil(t, got[1].DeletedAt)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("connection failure is wrapped", func(t *testing.T) {
		repo, ws := gapBlogCategoryRepo(t)
		sentinel := errors.New("no pool")
		ws.EXPECT().GetConnection(gomock.Any(), "ws-1").Return(nil, sentinel)
		_, err := repo.GetCategoriesByIDs(blogCtx(), []string{"c1"})
		require.ErrorContains(t, err, "failed to get workspace connection:")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("query failure is wrapped", func(t *testing.T) {
		repo, ws := gapBlogCategoryRepo(t)
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		ws.EXPECT().GetConnection(gomock.Any(), "ws-1").Return(db, nil)
		sentinel := errors.New("statement timeout")
		mock.ExpectQuery(regexp.QuoteMeta("FROM blog_categories")).WillReturnError(sentinel)
		_, err = repo.GetCategoriesByIDs(blogCtx(), []string{"c1"})
		require.ErrorContains(t, err, "failed to get categories by IDs:")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("scan failure is wrapped", func(t *testing.T) {
		repo, ws := gapBlogCategoryRepo(t)
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		ws.EXPECT().GetConnection(gomock.Any(), "ws-1").Return(db, nil)
		// A string settings value fails BlogCategorySettings.Scan.
		mock.ExpectQuery(regexp.QuoteMeta("FROM blog_categories")).
			WillReturnRows(sqlmock.NewRows(gapCatCols).
				AddRow("c1", "tech", "not-bytes", now, now, nil))
		_, err = repo.GetCategoriesByIDs(blogCtx(), []string{"c1"})
		require.ErrorContains(t, err, "failed to scan blog category:")
	})

	t.Run("iteration failure is wrapped", func(t *testing.T) {
		repo, ws := gapBlogCategoryRepo(t)
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		ws.EXPECT().GetConnection(gomock.Any(), "ws-1").Return(db, nil)
		sentinel := errors.New("cursor died")
		rows := sqlmock.NewRows(gapCatCols).
			AddRow("c1", "tech", []byte(`{"name":"Tech"}`), now, now, nil)
		rows.RowError(0, sentinel)
		mock.ExpectQuery(regexp.QuoteMeta("FROM blog_categories")).WillReturnRows(rows)
		_, err = repo.GetCategoriesByIDs(blogCtx(), []string{"c1"})
		require.ErrorContains(t, err, "error iterating blog categories:")
		assert.ErrorIs(t, err, sentinel)
	})
}

func TestTaskBlogGap_BlogListCategories_Branches(t *testing.T) {
	now := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	withDB := func(t *testing.T) (*blogCategoryRepository, sqlmock.Sqlmock, *sql.DB) {
		repo, ws := gapBlogCategoryRepo(t)
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		ws.EXPECT().GetConnection(gomock.Any(), "ws-1").Return(db, nil)
		return repo, mock, db
	}

	t.Run("query failure is wrapped", func(t *testing.T) {
		repo, mock, db := withDB(t)
		defer func() { _ = db.Close() }()
		sentinel := errors.New("statement timeout")
		mock.ExpectQuery(regexp.QuoteMeta("ORDER BY created_at DESC")).WillReturnError(sentinel)
		_, err := repo.ListCategories(blogCtx())
		require.ErrorContains(t, err, "failed to list blog categories:")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("scan failure is wrapped", func(t *testing.T) {
		repo, mock, db := withDB(t)
		defer func() { _ = db.Close() }()
		mock.ExpectQuery(regexp.QuoteMeta("FROM blog_categories")).
			WillReturnRows(sqlmock.NewRows(gapCatCols).
				AddRow("c1", "tech", "not-bytes", now, now, nil))
		_, err := repo.ListCategories(blogCtx())
		require.ErrorContains(t, err, "failed to scan blog category:")
	})

	t.Run("iteration failure is wrapped", func(t *testing.T) {
		repo, mock, db := withDB(t)
		defer func() { _ = db.Close() }()
		sentinel := errors.New("cursor died")
		rows := sqlmock.NewRows(gapCatCols).
			AddRow("c1", "tech", []byte(`{"name":"Tech"}`), now, now, nil)
		rows.RowError(0, sentinel)
		mock.ExpectQuery(regexp.QuoteMeta("FROM blog_categories")).WillReturnRows(rows)
		_, err := repo.ListCategories(blogCtx())
		require.ErrorContains(t, err, "error iterating blog categories:")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("connection failure is wrapped", func(t *testing.T) {
		repo, ws := gapBlogCategoryRepo(t)
		sentinel := errors.New("no pool")
		ws.EXPECT().GetConnection(gomock.Any(), "ws-1").Return(nil, sentinel)
		_, err := repo.ListCategories(blogCtx())
		require.ErrorContains(t, err, "failed to get workspace connection:")
		assert.ErrorIs(t, err, sentinel)
	})
}

func TestTaskBlogGap_BlogListPosts_Branches(t *testing.T) {
	now := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	withDB := func(t *testing.T) (*blogPostRepository, sqlmock.Sqlmock, *sql.DB) {
		repo, ws := gapBlogPostRepo(t)
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		ws.EXPECT().GetConnection(gomock.Any(), "ws-1").Return(db, nil)
		return repo, mock, db
	}

	t.Run("data query failure is wrapped after a successful count", func(t *testing.T) {
		repo, mock, db := withDB(t)
		defer func() { _ = db.Close() }()
		sentinel := errors.New("statement timeout")
		mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*)")).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(4))
		mock.ExpectQuery(regexp.QuoteMeta("ORDER BY created_at DESC, id DESC")).WillReturnError(sentinel)
		_, err := repo.ListPosts(blogCtx(), domain.ListBlogPostsRequest{Limit: 10})
		require.ErrorContains(t, err, "failed to list blog posts:")
		assert.ErrorIs(t, err, sentinel)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("scan failure is wrapped", func(t *testing.T) {
		repo, mock, db := withDB(t)
		defer func() { _ = db.Close() }()
		mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*)")).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
		mock.ExpectQuery(regexp.QuoteMeta("FROM blog_posts")).
			WillReturnRows(sqlmock.NewRows(blogPostCols).
				AddRow("p1", "c1", "hello", "not-bytes", nil, now, now, nil))
		_, err := repo.ListPosts(blogCtx(), domain.ListBlogPostsRequest{Limit: 10})
		require.ErrorContains(t, err, "failed to scan blog post:")
	})

	t.Run("iteration failure is wrapped", func(t *testing.T) {
		repo, mock, db := withDB(t)
		defer func() { _ = db.Close() }()
		sentinel := errors.New("cursor died")
		mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*)")).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
		rows := sqlmock.NewRows(blogPostCols).
			AddRow("p1", "c1", "hello", []byte(`{}`), nil, now, now, nil)
		rows.RowError(0, sentinel)
		mock.ExpectQuery(regexp.QuoteMeta("FROM blog_posts")).WillReturnRows(rows)
		_, err := repo.ListPosts(blogCtx(), domain.ListBlogPostsRequest{Limit: 10})
		require.ErrorContains(t, err, "error iterating blog posts:")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("category and published filter number limit and offset after the filter", func(t *testing.T) {
		repo, mock, db := withDB(t)
		defer func() { _ = db.Close() }()
		mock.ExpectQuery(regexp.QuoteMeta("category_id = $1")).
			WithArgs("cat-1").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(5))
		mock.ExpectQuery(regexp.QuoteMeta("LIMIT $2 OFFSET $3")).
			WithArgs("cat-1", 2, 4).
			WillReturnRows(sqlmock.NewRows(blogPostCols).
				AddRow("p1", "cat-1", "hello", []byte(`{}`), &now, now, now, nil))
		resp, err := repo.ListPosts(blogCtx(), domain.ListBlogPostsRequest{
			CategoryID: "cat-1", Status: domain.BlogPostStatusPublished, Limit: 2, Offset: 4})
		require.NoError(t, err)
		assert.Equal(t, 5, resp.TotalCount)
		assert.Equal(t, 3, resp.TotalPages) // ceil(5/2)
		assert.Equal(t, 1, resp.CurrentPage)
		assert.True(t, resp.HasNextPage)
		assert.False(t, resp.HasPreviousPage)
		require.Len(t, resp.Posts, 1)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("zero limit yields zero total pages at the boundary", func(t *testing.T) {
		repo, mock, db := withDB(t)
		defer func() { _ = db.Close() }()
		mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*)")).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
		mock.ExpectQuery(regexp.QuoteMeta("LIMIT $1 OFFSET $2")).
			WithArgs(0, 0).WillReturnRows(sqlmock.NewRows(blogPostCols))
		resp, err := repo.ListPosts(blogCtx(), domain.ListBlogPostsRequest{Limit: 0})
		require.NoError(t, err)
		assert.Equal(t, 0, resp.TotalPages)
		assert.Equal(t, 1, resp.CurrentPage)
		assert.False(t, resp.HasNextPage)
	})
}

func TestTaskBlogGap_BlogFeed_Branches(t *testing.T) {
	now := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	withPostDB := func(t *testing.T) (*blogPostRepository, sqlmock.Sqlmock, *sql.DB) {
		repo, ws := gapBlogPostRepo(t)
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		ws.EXPECT().GetConnection(gomock.Any(), "ws-1").Return(db, nil)
		return repo, mock, db
	}

	t.Run("ListFeedPosts query failure is wrapped", func(t *testing.T) {
		repo, mock, db := withPostDB(t)
		defer func() { _ = db.Close() }()
		sentinel := errors.New("statement timeout")
		mock.ExpectQuery(regexp.QuoteMeta("FROM blog_posts p")).WillReturnError(sentinel)
		_, err := repo.ListFeedPosts(blogCtx(), nil, 10)
		require.ErrorContains(t, err, "failed to list feed posts:")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("ListFeedPosts empty-string slug adds no filter and limit is $1", func(t *testing.T) {
		repo, mock, db := withPostDB(t)
		defer func() { _ = db.Close() }()
		empty := ""
		mock.ExpectQuery(regexp.QuoteMeta("LIMIT $1")).
			WithArgs(3).WillReturnRows(sqlmock.NewRows(blogPostCols).
			AddRow("p1", "c1", "hello", []byte(`{}`), &now, now, now, nil))
		posts, err := repo.ListFeedPosts(blogCtx(), &empty, 3)
		require.NoError(t, err)
		require.Len(t, posts, 1)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("ListFeedPosts scan failure is wrapped", func(t *testing.T) {
		repo, mock, db := withPostDB(t)
		defer func() { _ = db.Close() }()
		mock.ExpectQuery(regexp.QuoteMeta("FROM blog_posts p")).
			WillReturnRows(sqlmock.NewRows(blogPostCols).
				AddRow("p1", "c1", "hello", "not-bytes", &now, now, now, nil))
		_, err := repo.ListFeedPosts(blogCtx(), nil, 10)
		require.ErrorContains(t, err, "failed to scan feed post:")
	})

	t.Run("ListFeedPosts iteration failure is wrapped", func(t *testing.T) {
		repo, mock, db := withPostDB(t)
		defer func() { _ = db.Close() }()
		sentinel := errors.New("cursor died")
		rows := sqlmock.NewRows(blogPostCols).
			AddRow("p1", "c1", "hello", []byte(`{}`), &now, now, now, nil)
		rows.RowError(0, sentinel)
		mock.ExpectQuery(regexp.QuoteMeta("FROM blog_posts p")).WillReturnRows(rows)
		_, err := repo.ListFeedPosts(blogCtx(), nil, 10)
		require.ErrorContains(t, err, "error iterating feed posts:")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("GetFeedFingerprint query failure is wrapped", func(t *testing.T) {
		repo, mock, db := withPostDB(t)
		defer func() { _ = db.Close() }()
		sentinel := errors.New("statement timeout")
		mock.ExpectQuery(regexp.QuoteMeta("WITH top_posts AS")).WillReturnError(sentinel)
		_, _, err := repo.GetFeedFingerprint(blogCtx(), nil, 10)
		require.ErrorContains(t, err, "failed to compute feed fingerprint:")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("GetFeedFingerprint with a category binds slug then limit", func(t *testing.T) {
		repo, mock, db := withPostDB(t)
		defer func() { _ = db.Close() }()
		slug := "tech"
		mock.ExpectQuery(regexp.QuoteMeta("WITH top_posts AS")).
			WithArgs("tech", 7).
			WillReturnRows(sqlmock.NewRows([]string{"max_updated_at", "ids_hash"}).
				AddRow(now, "deadbeef"))
		maxAt, hash, err := repo.GetFeedFingerprint(blogCtx(), &slug, 7)
		require.NoError(t, err)
		assert.Equal(t, now, maxAt)
		assert.Equal(t, "deadbeef", hash)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestTaskBlogGap_BlogWithTransaction_Branches(t *testing.T) {
	t.Run("category connection failure is wrapped", func(t *testing.T) {
		repo, ws := gapBlogCategoryRepo(t)
		sentinel := errors.New("no pool")
		ws.EXPECT().GetConnection(gomock.Any(), "ws-1").Return(nil, sentinel)
		err := repo.WithTransaction(context.Background(), "ws-1", func(tx *sql.Tx) error { return nil })
		require.ErrorContains(t, err, "failed to get workspace connection:")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("fn error passes through unwrapped and rolls back", func(t *testing.T) {
		repo, ws := gapBlogCategoryRepo(t)
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		ws.EXPECT().GetConnection(gomock.Any(), "ws-1").Return(db, nil)
		sentinel := errors.New("fn failed")
		mock.ExpectBegin()
		mock.ExpectRollback()
		err = repo.WithTransaction(context.Background(), "ws-1", func(tx *sql.Tx) error { return sentinel })
		require.EqualError(t, err, "fn failed")
		assert.ErrorIs(t, err, sentinel)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("commit failure is wrapped", func(t *testing.T) {
		repo, ws := gapBlogPostRepo(t)
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		ws.EXPECT().GetConnection(gomock.Any(), "ws-1").Return(db, nil)
		sentinel := errors.New("commit lost")
		mock.ExpectBegin()
		mock.ExpectCommit().WillReturnError(sentinel)
		err = repo.WithTransaction(context.Background(), "ws-1", func(tx *sql.Tx) error { return nil })
		require.ErrorContains(t, err, "failed to commit transaction:")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("begin failure is wrapped", func(t *testing.T) {
		repo, ws := gapBlogPostRepo(t)
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		ws.EXPECT().GetConnection(gomock.Any(), "ws-1").Return(db, nil)
		sentinel := errors.New("cannot begin")
		mock.ExpectBegin().WillReturnError(sentinel)
		err = repo.WithTransaction(context.Background(), "ws-1", func(tx *sql.Tx) error { return nil })
		require.ErrorContains(t, err, "failed to begin transaction:")
		assert.ErrorIs(t, err, sentinel)
	})
}

func TestTaskBlogGap_BlogWorkspaceGuards_DoNotTouchDB(t *testing.T) {
	// No GetConnection expectation is registered on any mock below: gomock
	// fails the test if a guard lets a call through to the workspace repo.
	bare := context.Background()

	t.Run("category methods refuse without a workspace", func(t *testing.T) {
		repo, _ := gapBlogCategoryRepo(t)
		cat := &domain.BlogCategory{ID: "cat-1", Slug: "tech"}
		require.EqualError(t, repo.UpdateCategory(bare, cat), "workspace_id not found in context")
		require.EqualError(t, repo.DeleteCategory(bare, "cat-1"), "workspace_id not found in context")
		_, err := repo.GetCategoryBySlug(bare, "tech")
		require.EqualError(t, err, "workspace_id not found in context")
		_, err = repo.GetCategoriesByIDs(bare, []string{"cat-1"})
		require.EqualError(t, err, "workspace_id not found in context")
	})

	t.Run("post methods refuse without a workspace", func(t *testing.T) {
		repo, _ := gapBlogPostRepo(t)
		post := &domain.BlogPost{ID: "post-1", CategoryID: "cat-1", Slug: "hello"}
		require.EqualError(t, repo.UpdatePost(bare, post), "workspace_id not found in context")
		require.EqualError(t, repo.DeletePost(bare, "post-1"), "workspace_id not found in context")
		require.EqualError(t, repo.PublishPost(bare, "post-1", nil), "workspace_id not found in context")
		require.EqualError(t, repo.UnpublishPost(bare, "post-1"), "workspace_id not found in context")
		_, err := repo.ListFeedPosts(bare, nil, 5)
		require.EqualError(t, err, "workspace_id not found in context")
		_, _, err = repo.GetFeedFingerprint(bare, nil, 5)
		require.EqualError(t, err, "workspace_id not found in context")
		_, err = repo.GetPostBySlug(bare, "hello")
		require.EqualError(t, err, "workspace_id not found in context")
	})
}
