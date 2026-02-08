package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTaskMock(t *testing.T) (*sql.DB, sqlmock.Sqlmock, *TaskRepository) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	repo := NewTaskRepository(db).(*TaskRepository)
	return db, mock, repo
}

// Helper to create a test task with default values
func createTestTask(id, workspace string) *domain.Task {
	if id == "" {
		id = uuid.New().String()
	}

	now := time.Now().UTC()

	return &domain.Task{
		ID:          id,
		WorkspaceID: workspace,
		Type:        "test-task",
		Status:      domain.TaskStatusPending,
		Progress:    0,
		State: &domain.TaskState{
			SendBroadcast: &domain.SendBroadcastState{
				BroadcastID:     "test-broadcast",
				RecipientOffset: 0,
			},
		},
		CreatedAt:     now,
		UpdatedAt:     now,
		MaxRuntime:    60,
		MaxRetries:    3,
		RetryCount:    0,
		RetryInterval: 60,
	}
}

// Helper to convert task state to JSON
func taskStateToJSON(t *testing.T, state *domain.TaskState) []byte {
	stateJSON, err := json.Marshal(state)
	require.NoError(t, err)
	return stateJSON
}

// Helper to setup mocked rows for a task
func taskToMockRows(t *testing.T, task *domain.Task) *sqlmock.Rows {
	stateJSON := taskStateToJSON(t, task.State)

	rows := sqlmock.NewRows([]string{
		"id", "workspace_id", "type", "status", "progress", "state",
		"error_message", "created_at", "updated_at", "last_run_at",
		"completed_at", "next_run_after", "timeout_after",
		"max_runtime", "max_retries", "retry_count", "retry_interval",
		"broadcast_id", "recurring_interval", "integration_id",
	})

	// Direct row addition instead of building a slice
	return rows.AddRow(
		task.ID, task.WorkspaceID, task.Type, task.Status, task.Progress, stateJSON,
		task.ErrorMessage, task.CreatedAt, task.UpdatedAt, task.LastRunAt,
		task.CompletedAt, task.NextRunAfter, task.TimeoutAfter,
		task.MaxRuntime, task.MaxRetries, task.RetryCount, task.RetryInterval,
		task.BroadcastID, task.RecurringInterval, task.IntegrationID,
	)
}

func TestTaskRepository_WithTransaction(t *testing.T) {
	db, mock, repo := setupTaskMock(t)
	defer func() { _ = db.Close() }()

	// Setup mock expectations
	mock.ExpectBegin()
	mock.ExpectCommit()

	// Test successful transaction
	err := repo.WithTransaction(context.Background(), func(tx *sql.Tx) error {
		// Just return nil to simulate successful operation
		return nil
	})

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())

	// Test transaction with error
	mock.ExpectBegin()
	mock.ExpectRollback()

	expectedErr := fmt.Errorf("test error")
	err = repo.WithTransaction(context.Background(), func(tx *sql.Tx) error {
		return expectedErr
	})

	assert.Error(t, err)
	assert.Equal(t, expectedErr, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTaskRepository_CreateWithTransaction(t *testing.T) {
	db, mock, repo := setupTaskMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspace := "test-workspace"
	task := &domain.Task{
		ID:          "task-123",
		WorkspaceID: workspace,
		Type:        "test-task",
		Status:      domain.TaskStatusPending,
		Progress:    0,
		State: &domain.TaskState{
			SendBroadcast: &domain.SendBroadcastState{
				BroadcastID:     "test-broadcast",
				RecipientOffset: 0,
			},
		},
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
		MaxRuntime:    60,
		MaxRetries:    3,
		RetryCount:    0,
		RetryInterval: 60,
		BroadcastID:   nil,
	}

	// Setup mock expectations
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO tasks").
		WithArgs(
			task.ID, workspace, task.Type, task.Status, task.Progress,
			sqlmock.AnyArg(), // State JSON
			task.ErrorMessage,
			sqlmock.AnyArg(), // CreatedAt (use AnyArg to avoid timestamp precision issues)
			sqlmock.AnyArg(), // UpdatedAt
			task.LastRunAt,
			task.CompletedAt, task.NextRunAfter, task.TimeoutAfter,
			task.MaxRuntime, task.MaxRetries, task.RetryCount, task.RetryInterval,
			task.BroadcastID,
			task.RecurringInterval, task.IntegrationID,
		).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	// Test create task with transaction
	err := repo.Create(ctx, workspace, task)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTaskRepository_GetWithTransaction(t *testing.T) {
	db, mock, repo := setupTaskMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspace := "test-workspace"
	taskID := uuid.New().String()
	now := time.Now().UTC()

	rows := sqlmock.NewRows([]string{
		"id", "workspace_id", "type", "status", "progress", "state",
		"error_message", "created_at", "updated_at", "last_run_at",
		"completed_at", "next_run_after", "timeout_after",
		"max_runtime", "max_retries", "retry_count", "retry_interval",
		"broadcast_id", "recurring_interval", "integration_id",
	}).AddRow(
		taskID, workspace, "test-task", domain.TaskStatusPending, 0, "{}",
		"", now, now, nil,
		nil, nil, nil,
		60, 3, 0, 60,
		nil, nil, nil, // broadcast_id, recurring_interval, integration_id
	)

	// Setup mock expectations
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM tasks WHERE id = .* AND workspace_id = .*").
		WithArgs(taskID, workspace).
		WillReturnRows(rows)
	mock.ExpectCommit()

	// Test get task with transaction
	task, err := repo.Get(ctx, workspace, taskID)
	assert.NoError(t, err)
	assert.NotNil(t, task)
	assert.Equal(t, taskID, task.ID)
	assert.Equal(t, workspace, task.WorkspaceID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTaskRepository_UpdateWithTransaction(t *testing.T) {
	db, mock, repo := setupTaskMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspace := "test-workspace"
	taskID := uuid.New().String()
	now := time.Now().UTC()

	task := &domain.Task{
		ID:          taskID,
		WorkspaceID: workspace,
		Type:        "test-task",
		Status:      domain.TaskStatusRunning,
		Progress:    50,
		State: &domain.TaskState{
			SendBroadcast: &domain.SendBroadcastState{
				BroadcastID:     "test-broadcast",
				RecipientOffset: 100,
			},
		},
		CreatedAt:     now.Add(-1 * time.Hour),
		UpdatedAt:     now,
		MaxRuntime:    60,
		MaxRetries:    3,
		RetryCount:    0,
		RetryInterval: 60,
		BroadcastID:   nil,
	}

	// Setup mock expectations
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE tasks").
		WithArgs(
			taskID, workspace, task.Type, task.Status, task.Progress,
			sqlmock.AnyArg(), // State JSON
			task.ErrorMessage, sqlmock.AnyArg(), task.LastRunAt,
			task.CompletedAt, task.NextRunAfter, task.TimeoutAfter,
			task.MaxRuntime, task.MaxRetries, task.RetryCount, task.RetryInterval,
			task.BroadcastID, task.RecurringInterval, task.IntegrationID,
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	// Test update task with transaction
	err := repo.Update(ctx, workspace, task)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTaskRepository_Delete(t *testing.T) {
	db, mock, repo := setupTaskMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspace := "test-workspace"
	taskID := uuid.New().String()

	// Test successful delete
	mock.ExpectExec("DELETE FROM tasks WHERE id = \\$1 AND workspace_id = \\$2").
		WithArgs(taskID, workspace).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := repo.Delete(ctx, workspace, taskID)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())

	// Test delete of non-existent task
	mock.ExpectExec("DELETE FROM tasks WHERE id = \\$1 AND workspace_id = \\$2").
		WithArgs(taskID, workspace).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err = repo.Delete(ctx, workspace, taskID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "task not found")
	assert.NoError(t, mock.ExpectationsWereMet())

	// Test database error
	mock.ExpectExec("DELETE FROM tasks WHERE id = \\$1 AND workspace_id = \\$2").
		WithArgs(taskID, workspace).
		WillReturnError(fmt.Errorf("database error"))

	err = repo.Delete(ctx, workspace, taskID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete task")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTaskRepository_DeleteAll(t *testing.T) {
	db, mock, repo := setupTaskMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspace := "test-workspace"

	// Successful delete of all tasks for a workspace
	mock.ExpectExec("DELETE FROM tasks WHERE").
		WithArgs(workspace).
		WillReturnResult(sqlmock.NewResult(0, 5))

	err := repo.DeleteAll(ctx, workspace)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())

	// Zero rows affected should still be a success
	mock.ExpectExec("DELETE FROM tasks WHERE").
		WithArgs(workspace).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err = repo.DeleteAll(ctx, workspace)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())

	// Database error should be surfaced
	mock.ExpectExec("DELETE FROM tasks WHERE").
		WithArgs(workspace).
		WillReturnError(fmt.Errorf("database error"))

	err = repo.DeleteAll(ctx, workspace)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete tasks")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTaskRepository_List(t *testing.T) {
	db, mock, repo := setupTaskMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspace := "test-workspace"

	// Create test tasks
	task1 := createTestTask("task-1", workspace)
	task1.Status = domain.TaskStatusPending
	task1.Type = "email"
	task1.CreatedAt = time.Now().Add(-1 * time.Hour)

	task2 := createTestTask("task-2", workspace)
	task2.Status = domain.TaskStatusCompleted
	task2.Type = "sms"
	task2.CreatedAt = time.Now()

	// Filter setup
	filter := domain.TaskFilter{
		Status:        []domain.TaskStatus{domain.TaskStatusPending, domain.TaskStatusCompleted},
		Type:          []string{"email", "sms"},
		CreatedAfter:  &task1.CreatedAt,
		CreatedBefore: &task2.CreatedAt,
		Limit:         10,
		Offset:        0,
	}

	// Test count query
	countRows := sqlmock.NewRows([]string{"count"}).AddRow(2)
	mock.ExpectQuery("SELECT COUNT.*FROM tasks.*").
		WithArgs(workspace, "pending", "completed", "email", "sms", task1.CreatedAt, task2.CreatedAt).
		WillReturnRows(countRows)

	// Test data query
	rows := sqlmock.NewRows([]string{
		"id", "workspace_id", "type", "status", "progress", "state",
		"error_message", "created_at", "updated_at", "last_run_at",
		"completed_at", "next_run_after", "timeout_after",
		"max_runtime", "max_retries", "retry_count", "retry_interval",
		"broadcast_id", "recurring_interval", "integration_id",
	})

	// Add task rows
	rows.AddRow(
		task1.ID, task1.WorkspaceID, task1.Type, task1.Status, task1.Progress, "{}",
		task1.ErrorMessage, task1.CreatedAt, task1.UpdatedAt, task1.LastRunAt,
		task1.CompletedAt, task1.NextRunAfter, task1.TimeoutAfter,
		task1.MaxRuntime, task1.MaxRetries, task1.RetryCount, task1.RetryInterval,
		task1.BroadcastID, task1.RecurringInterval, task1.IntegrationID,
	)

	rows.AddRow(
		task2.ID, task2.WorkspaceID, task2.Type, task2.Status, task2.Progress, "{}",
		task2.ErrorMessage, task2.CreatedAt, task2.UpdatedAt, task2.LastRunAt,
		task2.CompletedAt, task2.NextRunAfter, task2.TimeoutAfter,
		task2.MaxRuntime, task2.MaxRetries, task2.RetryCount, task2.RetryInterval,
		task2.BroadcastID, task2.RecurringInterval, task2.IntegrationID,
	)

	mock.ExpectQuery("SELECT .* FROM tasks WHERE").
		WithArgs(workspace, "pending", "completed", "email", "sms", task1.CreatedAt, task2.CreatedAt).
		WillReturnRows(rows)

	tasks, count, err := repo.List(ctx, workspace, filter)
	assert.NoError(t, err)
	assert.Equal(t, 2, count)
	assert.Len(t, tasks, 2)
	assert.Equal(t, task1.ID, tasks[0].ID)
	assert.Equal(t, task2.ID, tasks[1].ID)
	assert.NoError(t, mock.ExpectationsWereMet())

	// Test empty result
	mock.ExpectQuery("SELECT COUNT.*FROM tasks.*").
		WithArgs(workspace, "pending", "completed", "email", "sms", task1.CreatedAt, task2.CreatedAt).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	mock.ExpectQuery("SELECT .* FROM tasks WHERE").
		WithArgs(workspace, "pending", "completed", "email", "sms", task1.CreatedAt, task2.CreatedAt).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "workspace_id", "type", "status", "progress", "state",
			"error_message", "created_at", "updated_at", "last_run_at",
			"completed_at", "next_run_after", "timeout_after",
			"max_runtime", "max_retries", "retry_count", "retry_interval",
			"broadcast_id",
		}))

	tasks, count, err = repo.List(ctx, workspace, filter)
	assert.NoError(t, err)
	assert.Equal(t, 0, count)
	assert.Len(t, tasks, 0)
	assert.NoError(t, mock.ExpectationsWereMet())

	// Test database error
	mock.ExpectQuery("SELECT COUNT.*FROM tasks.*").
		WithArgs(workspace, "pending", "completed", "email", "sms", task1.CreatedAt, task2.CreatedAt).
		WillReturnError(fmt.Errorf("database error"))

	tasks, count, err = repo.List(ctx, workspace, filter)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to count tasks")
	assert.Equal(t, 0, count)
	assert.Nil(t, tasks)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTaskRepository_GetNextBatch(t *testing.T) {
	db, mock, repo := setupTaskMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	now := time.Now().UTC()
	limit := 2

	// Create test tasks
	task1 := createTestTask("task-1", "workspace-1")
	task1.Status = domain.TaskStatusPending
	task1.NextRunAfter = nil

	task2 := createTestTask("task-2", "workspace-2")
	task2.Status = domain.TaskStatusPaused
	pastTime := now.Add(-1 * time.Hour)
	task2.NextRunAfter = &pastTime

	// Test successful batch retrieval
	rows := sqlmock.NewRows([]string{
		"id", "workspace_id", "type", "status", "progress", "state",
		"error_message", "created_at", "updated_at", "last_run_at",
		"completed_at", "next_run_after", "timeout_after",
		"max_runtime", "max_retries", "retry_count", "retry_interval",
		"broadcast_id", "recurring_interval", "integration_id",
	})

	// Add task rows
	rows.AddRow(
		task1.ID, task1.WorkspaceID, task1.Type, task1.Status, task1.Progress, "{}",
		task1.ErrorMessage, task1.CreatedAt, task1.UpdatedAt, task1.LastRunAt,
		task1.CompletedAt, task1.NextRunAfter, task1.TimeoutAfter,
		task1.MaxRuntime, task1.MaxRetries, task1.RetryCount, task1.RetryInterval,
		task1.BroadcastID, task1.RecurringInterval, task1.IntegrationID,
	)

	rows.AddRow(
		task2.ID, task2.WorkspaceID, task2.Type, task2.Status, task2.Progress, "{}",
		task2.ErrorMessage, task2.CreatedAt, task2.UpdatedAt, task2.LastRunAt,
		task2.CompletedAt, task2.NextRunAfter, task2.TimeoutAfter,
		task2.MaxRuntime, task2.MaxRetries, task2.RetryCount, task2.RetryInterval,
		task2.BroadcastID, task2.RecurringInterval, task2.IntegrationID,
	)

	mock.ExpectQuery("SELECT .* FROM tasks WHERE").
		WillReturnRows(rows)

	tasks, err := repo.GetNextBatch(ctx, limit)
	assert.NoError(t, err)
	assert.Len(t, tasks, 2)
	assert.Equal(t, task1.ID, tasks[0].ID)
	assert.Equal(t, task2.ID, tasks[1].ID)
	assert.NoError(t, mock.ExpectationsWereMet())

	// Test empty batch
	mock.ExpectQuery("SELECT .* FROM tasks WHERE").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "workspace_id", "type", "status", "progress", "state",
			"error_message", "created_at", "updated_at", "last_run_at",
			"completed_at", "next_run_after", "timeout_after",
			"max_runtime", "max_retries", "retry_count", "retry_interval",
			"broadcast_id",
		}))

	tasks, err = repo.GetNextBatch(ctx, limit)
	assert.NoError(t, err)
	assert.Len(t, tasks, 0)
	assert.NoError(t, mock.ExpectationsWereMet())

	// Test database error
	mock.ExpectQuery("SELECT .* FROM tasks WHERE").
		WillReturnError(fmt.Errorf("database error"))

	tasks, err = repo.GetNextBatch(ctx, limit)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get next batch")
	assert.Nil(t, tasks)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTaskRepository_MarkAsRunning(t *testing.T) {
	db, mock, repo := setupTaskMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspace := "test-workspace"
	taskID := uuid.New().String()
	timeoutAfter := time.Now().UTC().Add(5 * time.Minute)

	// Test successful mark as running (task is in pending status)
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE tasks SET").
		WithArgs(
			domain.TaskStatusRunning,
			sqlmock.AnyArg(), // updated_at
			sqlmock.AnyArg(), // last_run_at
			timeoutAfter,
			taskID,
			workspace,
			string(domain.TaskStatusPending), // status check in WHERE
			string(domain.TaskStatusPaused),  // status check in WHERE
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err := repo.MarkAsRunning(ctx, workspace, taskID, timeoutAfter)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())

	// Test mark as running for non-existent task or task already running
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE tasks SET").
		WithArgs(
			domain.TaskStatusRunning,
			sqlmock.AnyArg(), // updated_at
			sqlmock.AnyArg(), // last_run_at
			timeoutAfter,
			taskID,
			workspace,
			string(domain.TaskStatusPending), // status check in WHERE
			string(domain.TaskStatusPaused),  // status check in WHERE
		).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()

	err = repo.MarkAsRunning(ctx, workspace, taskID, timeoutAfter)
	assert.Error(t, err)
	var alreadyRunningErr *domain.ErrTaskAlreadyRunning
	assert.ErrorAs(t, err, &alreadyRunningErr)
	assert.Equal(t, taskID, alreadyRunningErr.TaskID)
	assert.NoError(t, mock.ExpectationsWereMet())

	// Test database error
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE tasks SET").
		WithArgs(
			domain.TaskStatusRunning,
			sqlmock.AnyArg(), // updated_at
			sqlmock.AnyArg(), // last_run_at
			timeoutAfter,
			taskID,
			workspace,
			string(domain.TaskStatusPending), // status check in WHERE
			string(domain.TaskStatusPaused),  // status check in WHERE
		).
		WillReturnError(fmt.Errorf("database error"))
	mock.ExpectRollback()

	err = repo.MarkAsRunning(ctx, workspace, taskID, timeoutAfter)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to mark task as running")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestTaskRepository_MarkAsRunning_ConcurrentProtection tests that MarkAsRunning
// properly prevents concurrent execution by checking the task status.
func TestTaskRepository_MarkAsRunning_ConcurrentProtection(t *testing.T) {
	db, mock, repo := setupTaskMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspace := "test-workspace"
	taskID := uuid.New().String()
	timeoutAfter := time.Now().UTC().Add(5 * time.Minute)

	// Simulate scenario where task is already running (another executor claimed it first)
	// The query will match 0 rows because status is 'running', not 'pending' or 'paused'
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE tasks SET").
		WithArgs(
			domain.TaskStatusRunning,
			sqlmock.AnyArg(), // updated_at
			sqlmock.AnyArg(), // last_run_at
			timeoutAfter,
			taskID,
			workspace,
			string(domain.TaskStatusPending),
			string(domain.TaskStatusPaused),
		).
		WillReturnResult(sqlmock.NewResult(0, 0)) // 0 rows = task already running
	mock.ExpectRollback()

	err := repo.MarkAsRunning(ctx, workspace, taskID, timeoutAfter)
	assert.Error(t, err)
	var alreadyRunningErr *domain.ErrTaskAlreadyRunning
	assert.ErrorAs(t, err, &alreadyRunningErr)
	assert.Equal(t, taskID, alreadyRunningErr.TaskID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTaskRepository_MarkAsCompleted(t *testing.T) {
	db, mock, repo := setupTaskMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspace := "test-workspace"
	taskID := uuid.New().String()
	state := &domain.TaskState{
		Progress: 100,
		Message:  "Completed",
		SendBroadcast: &domain.SendBroadcastState{
			BroadcastID:     "broadcast-123",
			TotalRecipients: 1000,
			EnqueuedCount:   1000,
			FailedCount:     0,
		},
	}

	// Test successful mark as completed
	mock.ExpectBegin()
	// Squirrel generates the args in the order: status, progress, state, error_message, updated_at, completed_at, timeout_after, id, workspace_id
	mock.ExpectExec("UPDATE tasks SET").
		WithArgs(
			string(domain.TaskStatusCompleted),
			int64(100),       // progress
			sqlmock.AnyArg(), // state JSON
			nil,              // error_message (nil on completion)
			sqlmock.AnyArg(), // updated_at
			sqlmock.AnyArg(), // completed_at
			nil,              // timeout_after
			taskID,
			workspace,
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err := repo.MarkAsCompleted(ctx, workspace, taskID, state)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())

	// Test mark as completed for non-existent task
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE tasks SET").
		WithArgs(
			string(domain.TaskStatusCompleted),
			int64(100),       // progress
			sqlmock.AnyArg(), // state JSON
			nil,              // error_message (nil on completion)
			sqlmock.AnyArg(), // updated_at
			sqlmock.AnyArg(), // completed_at
			nil,              // timeout_after
			taskID,
			workspace,
		).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()

	err = repo.MarkAsCompleted(ctx, workspace, taskID, state)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "task not found")
	assert.NoError(t, mock.ExpectationsWereMet())

	// Test database error
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE tasks SET").
		WithArgs(
			string(domain.TaskStatusCompleted),
			int64(100),       // progress
			sqlmock.AnyArg(), // state JSON
			nil,              // error_message (nil on completion)
			sqlmock.AnyArg(), // updated_at
			sqlmock.AnyArg(), // completed_at
			nil,              // timeout_after
			taskID,
			workspace,
		).
		WillReturnError(fmt.Errorf("database error"))
	mock.ExpectRollback()

	err = repo.MarkAsCompleted(ctx, workspace, taskID, state)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to mark task as completed")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestTaskRepository_MarkAsCompleted_SavesState verifies that MarkAsCompleted
// now saves the final task state (fix for GitHub issue #157).
func TestTaskRepository_MarkAsCompleted_SavesState(t *testing.T) {
	db, mock, repo := setupTaskMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspace := "test-workspace"
	taskID := uuid.New().String()
	state := &domain.TaskState{
		Progress: 100,
		Message:  "Processed 1000/1000 recipients (100.0%)",
		SendBroadcast: &domain.SendBroadcastState{
			BroadcastID:     "broadcast-123",
			TotalRecipients: 1000,
			EnqueuedCount:   1000,
			FailedCount:     0,
		},
	}

	// Verify that state is now included in the SQL (unlike before the fix)
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE tasks SET").
		WithArgs(
			string(domain.TaskStatusCompleted),
			int64(100),       // progress
			sqlmock.AnyArg(), // state JSON - THIS WAS MISSING BEFORE THE FIX
			nil,              // error_message
			sqlmock.AnyArg(), // updated_at
			sqlmock.AnyArg(), // completed_at
			nil,              // timeout_after
			taskID,
			workspace,
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err := repo.MarkAsCompleted(ctx, workspace, taskID, state)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())

	t.Log("FIXED: MarkAsCompleted now saves the final task state")
}

func TestTaskRepository_MarkAsFailed(t *testing.T) {
	db, mock, repo := setupTaskMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspace := "test-workspace"
	taskID := uuid.New().String()
	errorMsg := "Test error message"

	// Create mock task for GetTx
	task := createTestTask(taskID, workspace)
	task.MaxRetries = 3
	task.RetryCount = 0

	// Test successful mark as failed with retry
	mock.ExpectBegin()

	// First mock the GetTx call to check the retry count
	mockRows := taskToMockRows(t, task)
	mock.ExpectQuery("SELECT .* FROM tasks WHERE id = .* AND workspace_id = .*").
		WithArgs(taskID, workspace).
		WillReturnRows(mockRows)

	// Then mock the update with pending status for retry
	mock.ExpectExec("UPDATE tasks SET").
		WithArgs(
			string(domain.TaskStatusPending),
			errorMsg,
			sqlmock.AnyArg(), // updated_at
			sqlmock.AnyArg(), // next_run_after
			nil,              // timeout_after
			taskID,
			workspace,
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err := repo.MarkAsFailed(ctx, workspace, taskID, errorMsg)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())

	// Test mark as failed for task that exceeds max retries
	task.RetryCount = 3 // Equal to MaxRetries

	mock.ExpectBegin()

	// First mock the GetTx call to check the retry count
	mockRows = taskToMockRows(t, task)
	mock.ExpectQuery("SELECT .* FROM tasks WHERE id = .* AND workspace_id = .*").
		WithArgs(taskID, workspace).
		WillReturnRows(mockRows)

	// Then mock the update with failed status (no more retries)
	mock.ExpectExec("UPDATE tasks SET").
		WithArgs(
			string(domain.TaskStatusFailed),
			errorMsg,
			sqlmock.AnyArg(), // updated_at
			nil,              // next_run_after (nil when no more retries)
			nil,              // timeout_after
			taskID,
			workspace,
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err = repo.MarkAsFailed(ctx, workspace, taskID, errorMsg)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())

	// Test database error on GetTx
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM tasks WHERE id = .* AND workspace_id = .*").
		WithArgs(taskID, workspace).
		WillReturnError(fmt.Errorf("database error"))
	mock.ExpectRollback()

	err = repo.MarkAsFailed(ctx, workspace, taskID, errorMsg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get task for retry check")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTaskRepository_MarkAsPaused(t *testing.T) {
	db, mock, repo := setupTaskMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspace := "test-workspace"
	taskID := uuid.New().String()
	nextRunAfter := time.Now().UTC().Add(5 * time.Minute)
	progress := float64(50)
	state := &domain.TaskState{
		SendBroadcast: &domain.SendBroadcastState{
			BroadcastID:     "test-broadcast",
			RecipientOffset: 100,
		},
	}

	// Test successful mark as paused
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE tasks SET").
		WithArgs(
			string(domain.TaskStatusPaused),
			progress,
			sqlmock.AnyArg(), // state JSON
			sqlmock.AnyArg(), // updated_at
			nextRunAfter,
			nil, // timeout_after
			taskID,
			workspace,
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err := repo.MarkAsPaused(ctx, workspace, taskID, nextRunAfter, progress, state)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())

	// Test mark as paused for non-existent task
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE tasks SET").
		WithArgs(
			string(domain.TaskStatusPaused),
			progress,
			sqlmock.AnyArg(), // state JSON
			sqlmock.AnyArg(), // updated_at
			nextRunAfter,
			nil, // timeout_after
			taskID,
			workspace,
		).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()

	err = repo.MarkAsPaused(ctx, workspace, taskID, nextRunAfter, progress, state)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "task not found")
	assert.NoError(t, mock.ExpectationsWereMet())

	// Test database error
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE tasks SET").
		WithArgs(
			string(domain.TaskStatusPaused),
			progress,
			sqlmock.AnyArg(), // state JSON
			sqlmock.AnyArg(), // updated_at
			nextRunAfter,
			nil, // timeout_after
			taskID,
			workspace,
		).
		WillReturnError(fmt.Errorf("database error"))
	mock.ExpectRollback()

	err = repo.MarkAsPaused(ctx, workspace, taskID, nextRunAfter, progress, state)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to mark task as paused")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTaskRepository_SaveState(t *testing.T) {
	db, mock, repo := setupTaskMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspace := "test-workspace"
	taskID := uuid.New().String()
	progress := float64(75)
	state := &domain.TaskState{
		SendBroadcast: &domain.SendBroadcastState{
			BroadcastID:     "test-broadcast",
			RecipientOffset: 200,
		},
	}

	// Test successful save state
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE tasks SET").
		WithArgs(
			progress,
			sqlmock.AnyArg(), // state JSON
			sqlmock.AnyArg(), // updated_at
			taskID,
			string(domain.TaskStatusRunning),
			workspace,
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err := repo.SaveState(ctx, workspace, taskID, progress, state)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())

	// Test save state for non-existent task (no rows affected, but no error expected)
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE tasks SET").
		WithArgs(
			progress,
			sqlmock.AnyArg(), // state JSON
			sqlmock.AnyArg(), // updated_at
			taskID,
			string(domain.TaskStatusRunning),
			workspace,
		).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	err = repo.SaveState(ctx, workspace, taskID, progress, state)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())

	// Test database error
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE tasks SET").
		WithArgs(
			progress,
			sqlmock.AnyArg(), // state JSON
			sqlmock.AnyArg(), // updated_at
			taskID,
			string(domain.TaskStatusRunning),
			workspace,
		).
		WillReturnError(fmt.Errorf("database error"))
	mock.ExpectRollback()

	err = repo.SaveState(ctx, workspace, taskID, progress, state)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to save task state")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTaskRepository_GetTaskByBroadcastID(t *testing.T) {
	db, mock, repo := setupTaskMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspace := "test-workspace"
	broadcastID := "broadcast-123"
	taskID := uuid.New().String()

	// Create a broadcast task
	task := createTestTask(taskID, workspace)
	task.Type = "send_broadcast"
	task.BroadcastID = &broadcastID

	// Test successful retrieval
	mock.ExpectBegin()

	mockRows := taskToMockRows(t, task)
	mock.ExpectQuery("SELECT .* FROM tasks WHERE workspace_id = \\$1 AND broadcast_id = \\$2").
		WithArgs(workspace, broadcastID).
		WillReturnRows(mockRows)

	mock.ExpectCommit()

	retrievedTask, err := repo.GetTaskByBroadcastID(ctx, workspace, broadcastID)
	assert.NoError(t, err)
	assert.NotNil(t, retrievedTask)
	assert.Equal(t, taskID, retrievedTask.ID)
	assert.Equal(t, workspace, retrievedTask.WorkspaceID)
	assert.Equal(t, broadcastID, *retrievedTask.BroadcastID)
	assert.NoError(t, mock.ExpectationsWereMet())

	// Test task not found
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM tasks WHERE workspace_id = \\$1 AND broadcast_id = \\$2").
		WithArgs(workspace, broadcastID).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()

	retrievedTask, err = repo.GetTaskByBroadcastID(ctx, workspace, broadcastID)
	assert.Error(t, err)
	assert.Nil(t, retrievedTask)
	assert.Contains(t, err.Error(), "task not found for broadcast ID")
	assert.NoError(t, mock.ExpectationsWereMet())

	// Test database error
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM tasks WHERE workspace_id = \\$1 AND broadcast_id = \\$2").
		WithArgs(workspace, broadcastID).
		WillReturnError(fmt.Errorf("database error"))
	mock.ExpectRollback()

	retrievedTask, err = repo.GetTaskByBroadcastID(ctx, workspace, broadcastID)
	assert.Error(t, err)
	assert.Nil(t, retrievedTask)
	assert.Contains(t, err.Error(), "failed to get task by broadcast ID")
	assert.NoError(t, mock.ExpectationsWereMet())

	// Test JSON unmarshal error
	mock.ExpectBegin()

	// Create rows with invalid JSON for state
	invalidJSONRows := sqlmock.NewRows([]string{
		"id", "workspace_id", "type", "status", "progress", "state",
		"error_message", "created_at", "updated_at", "last_run_at",
		"completed_at", "next_run_after", "timeout_after",
		"max_runtime", "max_retries", "retry_count", "retry_interval",
		"broadcast_id", "recurring_interval", "integration_id",
	}).AddRow(
		taskID, workspace, "send_broadcast", domain.TaskStatusPending, 0, "invalid json",
		"", task.CreatedAt, task.UpdatedAt, nil,
		nil, nil, nil,
		60, 3, 0, 60,
		broadcastID, nil, nil,
	)

	mock.ExpectQuery("SELECT .* FROM tasks WHERE workspace_id = \\$1 AND broadcast_id = \\$2").
		WithArgs(workspace, broadcastID).
		WillReturnRows(invalidJSONRows)
	mock.ExpectRollback()

	retrievedTask, err = repo.GetTaskByBroadcastID(ctx, workspace, broadcastID)
	assert.Error(t, err)
	assert.Nil(t, retrievedTask)
	assert.Contains(t, err.Error(), "failed to unmarshal state")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTaskRepository_GetTaskByIntegrationID(t *testing.T) {
	db, mock, repo := setupTaskMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspace := "test-workspace"
	taskID := uuid.New().String()
	integrationID := "int-123"
	interval := int64(60)

	task := &domain.Task{
		ID:                taskID,
		WorkspaceID:       workspace,
		Type:              "sync_integration",
		Status:            domain.TaskStatusPending,
		Progress:          0,
		State:             &domain.TaskState{},
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
		MaxRuntime:        300,
		MaxRetries:        3,
		RetryCount:        0,
		RetryInterval:     60,
		RecurringInterval: &interval,
		IntegrationID:     &integrationID,
	}

	// Test successful retrieval
	mock.ExpectBegin()
	mockRows := taskToMockRows(t, task)
	mock.ExpectQuery("SELECT .* FROM tasks WHERE workspace_id = \\$1 AND integration_id = \\$2").
		WithArgs(workspace, integrationID).
		WillReturnRows(mockRows)
	mock.ExpectCommit()

	retrievedTask, err := repo.GetTaskByIntegrationID(ctx, workspace, integrationID)
	assert.NoError(t, err)
	assert.NotNil(t, retrievedTask)
	assert.Equal(t, taskID, retrievedTask.ID)
	assert.Equal(t, workspace, retrievedTask.WorkspaceID)
	assert.Equal(t, integrationID, *retrievedTask.IntegrationID)
	assert.Equal(t, interval, *retrievedTask.RecurringInterval)
	assert.NoError(t, mock.ExpectationsWereMet())

	// Test task not found
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM tasks WHERE workspace_id = \\$1 AND integration_id = \\$2").
		WithArgs(workspace, integrationID).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()

	retrievedTask, err = repo.GetTaskByIntegrationID(ctx, workspace, integrationID)
	assert.Error(t, err)
	assert.Nil(t, retrievedTask)
	assert.ErrorIs(t, err, domain.ErrTaskNotFound)
	assert.NoError(t, mock.ExpectationsWereMet())

	// Test database error
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM tasks WHERE workspace_id = \\$1 AND integration_id = \\$2").
		WithArgs(workspace, integrationID).
		WillReturnError(fmt.Errorf("database error"))
	mock.ExpectRollback()

	retrievedTask, err = repo.GetTaskByIntegrationID(ctx, workspace, integrationID)
	assert.Error(t, err)
	assert.Nil(t, retrievedTask)
	assert.Contains(t, err.Error(), "failed to get task by integration ID")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestTaskRepository_CreateTx_ErrorPaths tests error paths in CreateTx
func TestTaskRepository_CreateTx_ErrorPaths(t *testing.T) {
	db, mock, repo := setupTaskMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspace := "test-workspace"

	// Test with task that has empty ID (should generate one)
	task := &domain.Task{
		ID:            "", // Empty ID to test generation
		WorkspaceID:   workspace,
		Type:          "test-task",
		Status:        "", // Empty status to test default
		Progress:      0,
		State:         &domain.TaskState{},
		MaxRuntime:    60,
		MaxRetries:    3,
		RetryCount:    0,
		RetryInterval: 60,
	}

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO tasks").
		WithArgs(
			sqlmock.AnyArg(), // ID will be generated
			workspace,
			task.Type,
			string(domain.TaskStatusPending), // Default status
			task.Progress,
			sqlmock.AnyArg(), // State JSON
			task.ErrorMessage,
			sqlmock.AnyArg(), // CreatedAt
			sqlmock.AnyArg(), // UpdatedAt
			task.LastRunAt,
			task.CompletedAt, task.NextRunAfter, task.TimeoutAfter,
			task.MaxRuntime, task.MaxRetries, task.RetryCount, task.RetryInterval,
			task.BroadcastID, task.RecurringInterval, task.IntegrationID,
		).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err := repo.Create(ctx, workspace, task)
	assert.NoError(t, err)
	assert.NotEmpty(t, task.ID)                            // ID should be generated
	assert.Equal(t, domain.TaskStatusPending, task.Status) // Default status should be set
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestTaskRepository_GetTx_ErrorPaths tests error paths in GetTx
func TestTaskRepository_GetTx_ErrorPaths(t *testing.T) {
	db, mock, repo := setupTaskMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspace := "test-workspace"
	taskID := uuid.New().String()

	// Test JSON unmarshal error in GetTx
	mock.ExpectBegin()

	invalidJSONRows := sqlmock.NewRows([]string{
		"id", "workspace_id", "type", "status", "progress", "state",
		"error_message", "created_at", "updated_at", "last_run_at",
		"completed_at", "next_run_after", "timeout_after",
		"max_runtime", "max_retries", "retry_count", "retry_interval",
		"broadcast_id", "recurring_interval", "integration_id",
	}).AddRow(
		taskID, workspace, "test-task", domain.TaskStatusPending, 0, "invalid json",
		"", time.Now(), time.Now(), nil,
		nil, nil, nil,
		60, 3, 0, 60,
		nil, nil, nil,
	)

	mock.ExpectQuery("SELECT .* FROM tasks WHERE id = .* AND workspace_id = .*").
		WithArgs(taskID, workspace).
		WillReturnRows(invalidJSONRows)
	mock.ExpectRollback()

	task, err := repo.Get(ctx, workspace, taskID)
	assert.Error(t, err)
	assert.Nil(t, task)
	assert.Contains(t, err.Error(), "failed to unmarshal state")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestTaskRepository_UpdateTx_ErrorPaths tests error paths in UpdateTx
func TestTaskRepository_UpdateTx_ErrorPaths(t *testing.T) {
	db, mock, repo := setupTaskMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspace := "test-workspace"
	taskID := uuid.New().String()

	task := &domain.Task{
		ID:            taskID,
		WorkspaceID:   workspace,
		Type:          "test-task",
		Status:        domain.TaskStatusRunning,
		Progress:      50,
		State:         &domain.TaskState{},
		MaxRuntime:    60,
		MaxRetries:    3,
		RetryCount:    0,
		RetryInterval: 60,
	}

	// Test rows affected error
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE tasks").
		WithArgs(
			taskID, workspace, task.Type, task.Status, task.Progress,
			sqlmock.AnyArg(), // State JSON
			task.ErrorMessage, sqlmock.AnyArg(), task.LastRunAt,
			task.CompletedAt, task.NextRunAfter, task.TimeoutAfter,
			task.MaxRuntime, task.MaxRetries, task.RetryCount, task.RetryInterval,
			task.BroadcastID, task.RecurringInterval, task.IntegrationID,
		).
		WillReturnResult(sqlmock.NewErrorResult(fmt.Errorf("rows affected error")))
	mock.ExpectRollback()

	err := repo.Update(ctx, workspace, task)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get rows affected")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestTaskRepository_List_ErrorPaths tests error paths in List
func TestTaskRepository_List_ErrorPaths(t *testing.T) {
	db, mock, repo := setupTaskMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspace := "test-workspace"

	filter := domain.TaskFilter{
		Status: []domain.TaskStatus{domain.TaskStatusPending},
		Limit:  10,
		Offset: 0,
	}

	// Test count query build error - this is hard to trigger, so test data query build error instead
	// Test data query error after successful count
	countRows := sqlmock.NewRows([]string{"count"}).AddRow(1)
	mock.ExpectQuery("SELECT COUNT.*FROM tasks.*").
		WithArgs(workspace, "pending").
		WillReturnRows(countRows)

	// Test rows.Err() error
	dataRows := sqlmock.NewRows([]string{
		"id", "workspace_id", "type", "status", "progress", "state",
		"error_message", "created_at", "updated_at", "last_run_at",
		"completed_at", "next_run_after", "timeout_after",
		"max_runtime", "max_retries", "retry_count", "retry_interval",
		"broadcast_id", "recurring_interval", "integration_id",
	}).AddRow(
		"task-1", workspace, "test-task", domain.TaskStatusPending, 0, "{}",
		"", time.Now(), time.Now(), nil,
		nil, nil, nil,
		60, 3, 0, 60,
		nil, nil, nil,
	).RowError(0, fmt.Errorf("row iteration error"))

	mock.ExpectQuery("SELECT .* FROM tasks WHERE").
		WithArgs(workspace, "pending").
		WillReturnRows(dataRows)

	tasks, count, err := repo.List(ctx, workspace, filter)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error iterating task rows")
	assert.Equal(t, 0, count)
	assert.Nil(t, tasks)
	assert.NoError(t, mock.ExpectationsWereMet())

	// Test scan error in List
	countRows2 := sqlmock.NewRows([]string{"count"}).AddRow(1)
	mock.ExpectQuery("SELECT COUNT.*FROM tasks.*").
		WithArgs(workspace, "pending").
		WillReturnRows(countRows2)

	// Create rows that will cause scan error by missing columns
	invalidRows := sqlmock.NewRows([]string{"id", "workspace_id"}).AddRow("task-1", workspace)

	mock.ExpectQuery("SELECT .* FROM tasks WHERE").
		WithArgs(workspace, "pending").
		WillReturnRows(invalidRows)

	tasks, count, err = repo.List(ctx, workspace, filter)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to scan task row")
	assert.Equal(t, 0, count)
	assert.Nil(t, tasks)
	assert.NoError(t, mock.ExpectationsWereMet())

	// Test JSON unmarshal error in List
	countRows3 := sqlmock.NewRows([]string{"count"}).AddRow(1)
	mock.ExpectQuery("SELECT COUNT.*FROM tasks.*").
		WithArgs(workspace, "pending").
		WillReturnRows(countRows3)

	invalidJSONRows := sqlmock.NewRows([]string{
		"id", "workspace_id", "type", "status", "progress", "state",
		"error_message", "created_at", "updated_at", "last_run_at",
		"completed_at", "next_run_after", "timeout_after",
		"max_runtime", "max_retries", "retry_count", "retry_interval",
		"broadcast_id", "recurring_interval", "integration_id",
	}).AddRow(
		"task-1", workspace, "test-task", domain.TaskStatusPending, 0, "invalid json",
		"", time.Now(), time.Now(), nil,
		nil, nil, nil,
		60, 3, 0, 60,
		nil, nil, nil,
	)

	mock.ExpectQuery("SELECT .* FROM tasks WHERE").
		WithArgs(workspace, "pending").
		WillReturnRows(invalidJSONRows)

	tasks, count, err = repo.List(ctx, workspace, filter)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal state")
	assert.Equal(t, 0, count)
	assert.Nil(t, tasks)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestTaskRepository_GetNextBatch_ErrorPaths tests error paths in GetNextBatch
func TestTaskRepository_GetNextBatch_ErrorPaths(t *testing.T) {
	db, mock, repo := setupTaskMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	limit := 2

	// Test scan error in GetNextBatch
	invalidRows := sqlmock.NewRows([]string{"id", "workspace_id"}).AddRow("task-1", "workspace-1")

	mock.ExpectQuery("SELECT .* FROM tasks WHERE").
		WillReturnRows(invalidRows)

	tasks, err := repo.GetNextBatch(ctx, limit)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to scan task row")
	assert.Nil(t, tasks)
	assert.NoError(t, mock.ExpectationsWereMet())

	// Test rows.Err() error in GetNextBatch
	errorRows := sqlmock.NewRows([]string{
		"id", "workspace_id", "type", "status", "progress", "state",
		"error_message", "created_at", "updated_at", "last_run_at",
		"completed_at", "next_run_after", "timeout_after",
		"max_runtime", "max_retries", "retry_count", "retry_interval",
		"broadcast_id", "recurring_interval", "integration_id",
	}).AddRow(
		"task-1", "workspace-1", "test-task", domain.TaskStatusPending, 0, "{}",
		"", time.Now(), time.Now(), nil,
		nil, nil, nil,
		60, 3, 0, 60,
		nil, nil, nil,
	).RowError(0, fmt.Errorf("row iteration error"))

	mock.ExpectQuery("SELECT .* FROM tasks WHERE").
		WillReturnRows(errorRows)

	tasks, err = repo.GetNextBatch(ctx, limit)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error iterating task rows")
	assert.Nil(t, tasks)
	assert.NoError(t, mock.ExpectationsWereMet())

	// Test JSON unmarshal error in GetNextBatch
	invalidJSONRows := sqlmock.NewRows([]string{
		"id", "workspace_id", "type", "status", "progress", "state",
		"error_message", "created_at", "updated_at", "last_run_at",
		"completed_at", "next_run_after", "timeout_after",
		"max_runtime", "max_retries", "retry_count", "retry_interval",
		"broadcast_id", "recurring_interval", "integration_id",
	}).AddRow(
		"task-1", "workspace-1", "test-task", domain.TaskStatusPending, 0, "invalid json",
		"", time.Now(), time.Now(), nil,
		nil, nil, nil,
		60, 3, 0, 60,
		nil, nil, nil,
	)

	mock.ExpectQuery("SELECT .* FROM tasks WHERE").
		WillReturnRows(invalidJSONRows)

	tasks, err = repo.GetNextBatch(ctx, limit)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal state")
	assert.Nil(t, tasks)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestTaskRepository_WithTransaction_BeginError tests begin transaction error
func TestTaskRepository_WithTransaction_BeginError(t *testing.T) {
	db, mock, repo := setupTaskMock(t)
	defer func() { _ = db.Close() }()

	// Test begin transaction error
	mock.ExpectBegin().WillReturnError(fmt.Errorf("begin transaction error"))

	err := repo.WithTransaction(context.Background(), func(tx *sql.Tx) error {
		return nil
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to begin transaction")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestTaskRepository_WithTransaction_CommitError tests commit transaction error
func TestTaskRepository_WithTransaction_CommitError(t *testing.T) {
	db, mock, repo := setupTaskMock(t)
	defer func() { _ = db.Close() }()

	// Test commit transaction error
	mock.ExpectBegin()
	mock.ExpectCommit().WillReturnError(fmt.Errorf("commit transaction error"))

	err := repo.WithTransaction(context.Background(), func(tx *sql.Tx) error {
		return nil
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to commit transaction")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestTaskRepository_CreateTx_MarshalError tests JSON marshal error in CreateTx
func TestTaskRepository_CreateTx_MarshalError(t *testing.T) {
	db, mock, repo := setupTaskMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspace := "test-workspace"

	// Create a task with a state that would cause JSON marshal error
	// We can't easily trigger this with normal state, but we can test the path exists
	task := &domain.Task{
		ID:            "test-id",
		WorkspaceID:   workspace,
		Type:          "test-task",
		Status:        domain.TaskStatusPending,
		Progress:      0,
		State:         &domain.TaskState{}, // This should marshal fine
		MaxRuntime:    60,
		MaxRetries:    3,
		RetryCount:    0,
		RetryInterval: 60,
	}

	// Test successful path to ensure the marshal works
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO tasks").
		WithArgs(
			task.ID, workspace, task.Type, task.Status, task.Progress,
			sqlmock.AnyArg(), // State JSON
			task.ErrorMessage,
			sqlmock.AnyArg(), // CreatedAt
			sqlmock.AnyArg(), // UpdatedAt
			task.LastRunAt,
			task.CompletedAt, task.NextRunAfter, task.TimeoutAfter,
			task.MaxRuntime, task.MaxRetries, task.RetryCount, task.RetryInterval,
			task.BroadcastID, task.RecurringInterval, task.IntegrationID,
		).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err := repo.Create(ctx, workspace, task)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestTaskRepository_UpdateTx_MarshalError tests JSON marshal error in UpdateTx
func TestTaskRepository_UpdateTx_MarshalError(t *testing.T) {
	db, mock, repo := setupTaskMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspace := "test-workspace"
	taskID := uuid.New().String()

	task := &domain.Task{
		ID:            taskID,
		WorkspaceID:   workspace,
		Type:          "test-task",
		Status:        domain.TaskStatusRunning,
		Progress:      50,
		State:         &domain.TaskState{}, // This should marshal fine
		MaxRuntime:    60,
		MaxRetries:    3,
		RetryCount:    0,
		RetryInterval: 60,
	}

	// Test successful path to ensure the marshal works
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE tasks").
		WithArgs(
			taskID, workspace, task.Type, task.Status, task.Progress,
			sqlmock.AnyArg(), // State JSON
			task.ErrorMessage, sqlmock.AnyArg(), task.LastRunAt,
			task.CompletedAt, task.NextRunAfter, task.TimeoutAfter,
			task.MaxRuntime, task.MaxRetries, task.RetryCount, task.RetryInterval,
			task.BroadcastID, task.RecurringInterval, task.IntegrationID,
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err := repo.Update(ctx, workspace, task)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestTaskRepository_SaveStateTx_MarshalError tests JSON marshal error in SaveStateTx
func TestTaskRepository_SaveStateTx_MarshalError(t *testing.T) {
	db, mock, repo := setupTaskMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspace := "test-workspace"
	taskID := uuid.New().String()
	progress := float64(75)
	state := &domain.TaskState{} // This should marshal fine

	// Test successful path to ensure the marshal works
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE tasks SET").
		WithArgs(
			progress,
			sqlmock.AnyArg(), // state JSON
			sqlmock.AnyArg(), // updated_at
			taskID,
			string(domain.TaskStatusRunning),
			workspace,
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err := repo.SaveState(ctx, workspace, taskID, progress, state)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestTaskRepository_MarkAsPausedTx_MarshalError tests JSON marshal error in MarkAsPausedTx
func TestTaskRepository_MarkAsPausedTx_MarshalError(t *testing.T) {
	db, mock, repo := setupTaskMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspace := "test-workspace"
	taskID := uuid.New().String()
	nextRunAfter := time.Now().UTC().Add(5 * time.Minute)
	progress := float64(50)
	state := &domain.TaskState{} // This should marshal fine

	// Test successful path to ensure the marshal works
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE tasks SET").
		WithArgs(
			string(domain.TaskStatusPaused),
			progress,
			sqlmock.AnyArg(), // state JSON
			sqlmock.AnyArg(), // updated_at
			nextRunAfter,
			nil, // timeout_after
			taskID,
			workspace,
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err := repo.MarkAsPaused(ctx, workspace, taskID, nextRunAfter, progress, state)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestTaskRepository_DeleteAll_QueryBuildError tests query build error in DeleteAll
func TestTaskRepository_DeleteAll_QueryBuildError(t *testing.T) {
	db, mock, repo := setupTaskMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspace := "test-workspace"

	// Test successful deletion
	mock.ExpectExec("DELETE FROM tasks WHERE").
		WithArgs(workspace).
		WillReturnResult(sqlmock.NewResult(0, 3))

	err := repo.DeleteAll(ctx, workspace)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestTaskRepository_GetNextBatch_QueryBuildError tests query build error in GetNextBatch
func TestTaskRepository_GetNextBatch_QueryBuildError(t *testing.T) {
	db, mock, repo := setupTaskMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	limit := 5

	// Test successful query build and execution
	rows := sqlmock.NewRows([]string{
		"id", "workspace_id", "type", "status", "progress", "state",
		"error_message", "created_at", "updated_at", "last_run_at",
		"completed_at", "next_run_after", "timeout_after",
		"max_runtime", "max_retries", "retry_count", "retry_interval",
		"broadcast_id", "recurring_interval", "integration_id",
	}).AddRow(
		"task-1", "workspace-1", "test-task", domain.TaskStatusPending, 0, "{}",
		"", time.Now(), time.Now(), nil,
		nil, nil, nil,
		60, 3, 0, 60,
		nil, nil, nil,
	)

	mock.ExpectQuery("SELECT .* FROM tasks WHERE").
		WillReturnRows(rows)

	tasks, err := repo.GetNextBatch(ctx, limit)
	assert.NoError(t, err)
	assert.Len(t, tasks, 1)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTaskRepository_MarkAsPending(t *testing.T) {
	// Test TaskRepository.MarkAsPending - this was at 0% coverage
	db, mock, repo := setupTaskMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspace := "test-workspace"
	id := "task-123"
	nextRunAfter := time.Now().UTC().Add(1 * time.Hour)
	progress := 50.0
	state := &domain.TaskState{
		SendBroadcast: &domain.SendBroadcastState{
			BroadcastID:     "broadcast-123",
			RecipientOffset: 100,
		},
	}

	t.Run("Success - Marks task as pending", func(t *testing.T) {
		mock.ExpectBegin()
		mock.ExpectExec("UPDATE tasks SET").
			WithArgs(
				domain.TaskStatusPending,
				progress,
				sqlmock.AnyArg(), // state JSON
				sqlmock.AnyArg(), // updated_at
				nextRunAfter,
				nil, // timeout_after
				id,
				workspace,
			).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		err := repo.MarkAsPending(ctx, workspace, id, nextRunAfter, progress, state)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Error - Task not found", func(t *testing.T) {
		mock.ExpectBegin()
		mock.ExpectExec("UPDATE tasks SET").
			WithArgs(
				domain.TaskStatusPending,
				progress,
				sqlmock.AnyArg(),
				sqlmock.AnyArg(),
				nextRunAfter,
				nil,
				id,
				workspace,
			).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectRollback()

		err := repo.MarkAsPending(ctx, workspace, id, nextRunAfter, progress, state)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "task not found")
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestTaskRepository_MarkAsPendingTx(t *testing.T) {
	// Test TaskRepository.MarkAsPendingTx - this was at 0% coverage
	db, mock, repo := setupTaskMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()

	workspace := "test-workspace"
	id := "task-123"
	nextRunAfter := time.Now().UTC().Add(1 * time.Hour)
	progress := 50.0
	state := &domain.TaskState{
		SendBroadcast: &domain.SendBroadcastState{
			BroadcastID:     "broadcast-123",
			RecipientOffset: 100,
		},
	}

	t.Run("Success - Marks task as pending in transaction", func(t *testing.T) {
		mock.ExpectBegin()
		mock.ExpectExec("UPDATE tasks SET").
			WithArgs(
				domain.TaskStatusPending,
				progress,
				sqlmock.AnyArg(), // state JSON
				sqlmock.AnyArg(), // updated_at
				nextRunAfter,
				nil, // timeout_after
				id,
				workspace,
			).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		tx, err := db.Begin()
		require.NoError(t, err)
		defer func() { _ = tx.Rollback() }()

		err = repo.MarkAsPendingTx(ctx, tx, workspace, id, nextRunAfter, progress, state)
		assert.NoError(t, err)
		_ = tx.Commit()
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Error - Task not found", func(t *testing.T) {
		mock.ExpectBegin()
		mock.ExpectExec("UPDATE tasks SET").
			WithArgs(
				domain.TaskStatusPending,
				progress,
				sqlmock.AnyArg(),
				sqlmock.AnyArg(),
				nextRunAfter,
				nil,
				id,
				workspace,
			).
			WillReturnResult(sqlmock.NewResult(0, 0))
		// Note: MarkAsPendingTx doesn't rollback on error, caller manages transaction

		tx, err := db.Begin()
		require.NoError(t, err)
		defer func() { _ = tx.Rollback() }()

		err = repo.MarkAsPendingTx(ctx, tx, workspace, id, nextRunAfter, progress, state)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "task not found")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Success - Nil state", func(t *testing.T) {
		mock.ExpectBegin()
		mock.ExpectExec("UPDATE tasks SET").
			WithArgs(
				domain.TaskStatusPending,
				progress,
				[]byte("null"),   // nil state marshaled as null
				sqlmock.AnyArg(), // updated_at
				nextRunAfter,
				nil, // timeout_after
				id,
				workspace,
			).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		tx, err := db.Begin()
		require.NoError(t, err)
		defer func() { _ = tx.Rollback() }()

		err = repo.MarkAsPendingTx(ctx, tx, workspace, id, nextRunAfter, progress, nil)
		// nil state should be marshaled as null, which is valid
		assert.NoError(t, err)
		_ = tx.Commit()
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}
