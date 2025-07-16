package repository

import (
	"context"
	"database/sql"
	"database/sql/driver"
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
		"broadcast_id",
	})

	// Direct row addition instead of building a slice
	return rows.AddRow(
		task.ID, task.WorkspaceID, task.Type, task.Status, task.Progress, stateJSON,
		task.ErrorMessage, task.CreatedAt, task.UpdatedAt, task.LastRunAt,
		task.CompletedAt, task.NextRunAfter, task.TimeoutAfter,
		task.MaxRuntime, task.MaxRetries, task.RetryCount, task.RetryInterval,
		task.BroadcastID,
	)
}

// For value comparison with sqlmock AnyArg
type anyTime struct{}

func (a anyTime) Match(v driver.Value) bool {
	_, ok := v.(time.Time)
	return ok
}

// For value comparison with sqlmock JSON payloads
type anyJSON struct{}

func (a anyJSON) Match(v driver.Value) bool {
	_, ok := v.([]byte)
	return ok
}

func TestTaskRepository_WithTransaction(t *testing.T) {
	db, mock, repo := setupTaskMock(t)
	defer db.Close()

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
	defer db.Close()

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
	defer db.Close()

	ctx := context.Background()
	workspace := "test-workspace"
	taskID := uuid.New().String()
	now := time.Now().UTC()

	rows := sqlmock.NewRows([]string{
		"id", "workspace_id", "type", "status", "progress", "state",
		"error_message", "created_at", "updated_at", "last_run_at",
		"completed_at", "next_run_after", "timeout_after",
		"max_runtime", "max_retries", "retry_count", "retry_interval",
		"broadcast_id",
	}).AddRow(
		taskID, workspace, "test-task", domain.TaskStatusPending, 0, "{}",
		"", now, now, nil,
		nil, nil, nil,
		60, 3, 0, 60,
		nil, // broadcast_id
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
	defer db.Close()

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
			task.BroadcastID,
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
	defer db.Close()

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

func TestTaskRepository_List(t *testing.T) {
	db, mock, repo := setupTaskMock(t)
	defer db.Close()

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
		"broadcast_id",
	})

	// Add task rows
	rows.AddRow(
		task1.ID, task1.WorkspaceID, task1.Type, task1.Status, task1.Progress, "{}",
		task1.ErrorMessage, task1.CreatedAt, task1.UpdatedAt, task1.LastRunAt,
		task1.CompletedAt, task1.NextRunAfter, task1.TimeoutAfter,
		task1.MaxRuntime, task1.MaxRetries, task1.RetryCount, task1.RetryInterval,
		task1.BroadcastID,
	)

	rows.AddRow(
		task2.ID, task2.WorkspaceID, task2.Type, task2.Status, task2.Progress, "{}",
		task2.ErrorMessage, task2.CreatedAt, task2.UpdatedAt, task2.LastRunAt,
		task2.CompletedAt, task2.NextRunAfter, task2.TimeoutAfter,
		task2.MaxRuntime, task2.MaxRetries, task2.RetryCount, task2.RetryInterval,
		task2.BroadcastID,
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
	defer db.Close()

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
		"broadcast_id",
	})

	// Add task rows
	rows.AddRow(
		task1.ID, task1.WorkspaceID, task1.Type, task1.Status, task1.Progress, "{}",
		task1.ErrorMessage, task1.CreatedAt, task1.UpdatedAt, task1.LastRunAt,
		task1.CompletedAt, task1.NextRunAfter, task1.TimeoutAfter,
		task1.MaxRuntime, task1.MaxRetries, task1.RetryCount, task1.RetryInterval,
		task1.BroadcastID,
	)

	rows.AddRow(
		task2.ID, task2.WorkspaceID, task2.Type, task2.Status, task2.Progress, "{}",
		task2.ErrorMessage, task2.CreatedAt, task2.UpdatedAt, task2.LastRunAt,
		task2.CompletedAt, task2.NextRunAfter, task2.TimeoutAfter,
		task2.MaxRuntime, task2.MaxRetries, task2.RetryCount, task2.RetryInterval,
		task2.BroadcastID,
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
	defer db.Close()

	ctx := context.Background()
	workspace := "test-workspace"
	taskID := uuid.New().String()
	timeoutAfter := time.Now().UTC().Add(5 * time.Minute)

	// Test successful mark as running
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE tasks SET").
		WithArgs(
			domain.TaskStatusRunning,
			sqlmock.AnyArg(), // updated_at
			sqlmock.AnyArg(), // last_run_at
			timeoutAfter,
			taskID,
			workspace,
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err := repo.MarkAsRunning(ctx, workspace, taskID, timeoutAfter)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())

	// Test mark as running for non-existent task
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE tasks SET").
		WithArgs(
			domain.TaskStatusRunning,
			sqlmock.AnyArg(), // updated_at
			sqlmock.AnyArg(), // last_run_at
			timeoutAfter,
			taskID,
			workspace,
		).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()

	err = repo.MarkAsRunning(ctx, workspace, taskID, timeoutAfter)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "task not found")
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
		).
		WillReturnError(fmt.Errorf("database error"))
	mock.ExpectRollback()

	err = repo.MarkAsRunning(ctx, workspace, taskID, timeoutAfter)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to mark task as running")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTaskRepository_MarkAsCompleted(t *testing.T) {
	db, mock, repo := setupTaskMock(t)
	defer db.Close()

	ctx := context.Background()
	workspace := "test-workspace"
	taskID := uuid.New().String()

	// Test successful mark as completed
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE tasks SET").
		WithArgs(
			string(domain.TaskStatusCompleted),
			int64(100),       // Change this to int64(100) to match what squirrel sends
			sqlmock.AnyArg(), // updated_at
			sqlmock.AnyArg(), // completed_at
			nil,              // timeout_after
			taskID,
			workspace,
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err := repo.MarkAsCompleted(ctx, workspace, taskID)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())

	// Test mark as completed for non-existent task
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE tasks SET").
		WithArgs(
			string(domain.TaskStatusCompleted),
			int64(100),       // Change this to int64(100) to match what squirrel sends
			sqlmock.AnyArg(), // updated_at
			sqlmock.AnyArg(), // completed_at
			nil,              // timeout_after
			taskID,
			workspace,
		).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()

	err = repo.MarkAsCompleted(ctx, workspace, taskID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "task not found")
	assert.NoError(t, mock.ExpectationsWereMet())

	// Test database error
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE tasks SET").
		WithArgs(
			string(domain.TaskStatusCompleted),
			int64(100),       // Change this to int64(100) to match what squirrel sends
			sqlmock.AnyArg(), // updated_at
			sqlmock.AnyArg(), // completed_at
			nil,              // timeout_after
			taskID,
			workspace,
		).
		WillReturnError(fmt.Errorf("database error"))
	mock.ExpectRollback()

	err = repo.MarkAsCompleted(ctx, workspace, taskID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to mark task as completed")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTaskRepository_MarkAsFailed(t *testing.T) {
	db, mock, repo := setupTaskMock(t)
	defer db.Close()

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
	defer db.Close()

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
	defer db.Close()

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
	defer db.Close()

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
}
