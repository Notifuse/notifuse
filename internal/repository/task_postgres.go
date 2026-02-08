package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/google/uuid"
)

// TaskRepository implements the domain.TaskRepository interface using PostgreSQL
type TaskRepository struct {
	systemDB *sql.DB
}

// NewTaskRepository creates a new TaskRepository instance
func NewTaskRepository(db *sql.DB) domain.TaskRepository {
	return &TaskRepository{
		systemDB: db,
	}
}

// WithTransaction executes a function within a transaction
// This is used to ensure database operations are atomic and properly locked
// All task processing follows this pattern:
// 1. Get the task with FOR UPDATE to acquire a row-level lock
// 2. Perform operations on the task
// 3. Update or mark the task with a new status
// This prevents multiple workers from processing the same task simultaneously
func (r *TaskRepository) WithTransaction(ctx context.Context, fn func(*sql.Tx) error) error {
	// Begin a transaction
	tx, err := r.systemDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	// Defer rollback - this will be a no-op if we successfully commit
	defer func() { _ = tx.Rollback() }()

	// Execute the provided function with the transaction
	if err := fn(tx); err != nil {
		return err
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// Create adds a new task
func (r *TaskRepository) Create(ctx context.Context, workspace string, task *domain.Task) error {
	return r.WithTransaction(ctx, func(tx *sql.Tx) error {
		return r.CreateTx(ctx, tx, workspace, task)
	})
}

// CreateTx adds a new task within a transaction
func (r *TaskRepository) CreateTx(ctx context.Context, tx *sql.Tx, workspace string, task *domain.Task) error {
	// Generate ID if not provided
	if task.ID == "" {
		task.ID = uuid.New().String()
	}

	// Initialize timestamps
	now := time.Now().UTC()
	task.CreatedAt = now
	task.UpdatedAt = now

	// Set default status if not set
	if task.Status == "" {
		task.Status = domain.TaskStatusPending
	}

	// Convert state to JSON
	stateJSON, err := json.Marshal(task.State)
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	// make sure dates are in UTC
	if task.NextRunAfter != nil {
		nextRunAfter := task.NextRunAfter.UTC()
		task.NextRunAfter = &nextRunAfter
	}
	if task.TimeoutAfter != nil {
		timeoutAfter := task.TimeoutAfter.UTC()
		task.TimeoutAfter = &timeoutAfter
	}

	if task.LastRunAt != nil {
		lastRunAt := task.LastRunAt.UTC()
		task.LastRunAt = &lastRunAt
	}

	if task.CompletedAt != nil {
		completedAt := task.CompletedAt.UTC()
		task.CompletedAt = &completedAt
	}

	// Insert the task
	query := `
		INSERT INTO tasks (
			id, workspace_id, type, status, progress, state,
			error_message, created_at, updated_at, last_run_at,
			completed_at, next_run_after, timeout_after,
			max_runtime, max_retries, retry_count, retry_interval,
			broadcast_id, recurring_interval, integration_id
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20
		)
	`

	_, err = tx.ExecContext(
		ctx,
		query,
		task.ID,
		workspace,
		task.Type,
		task.Status,
		task.Progress,
		stateJSON,
		task.ErrorMessage,
		task.CreatedAt,
		task.UpdatedAt,
		task.LastRunAt,
		task.CompletedAt,
		task.NextRunAfter,
		task.TimeoutAfter,
		task.MaxRuntime,
		task.MaxRetries,
		task.RetryCount,
		task.RetryInterval,
		task.BroadcastID,
		task.RecurringInterval,
		task.IntegrationID,
	)

	if err != nil {
		return fmt.Errorf("failed to insert task: %w", err)
	}

	return nil
}

// Get retrieves a task by ID
func (r *TaskRepository) Get(ctx context.Context, workspace, id string) (*domain.Task, error) {
	var task *domain.Task
	var err error

	err = r.WithTransaction(ctx, func(tx *sql.Tx) error {
		task, err = r.GetTx(ctx, tx, workspace, id)
		return err
	})

	return task, err
}

// GetTx retrieves a task by ID within a transaction
func (r *TaskRepository) GetTx(ctx context.Context, tx *sql.Tx, workspace, id string) (*domain.Task, error) {
	query := `
		SELECT
			id, workspace_id, type, status, progress, state,
			error_message, created_at, updated_at, last_run_at,
			completed_at, next_run_after, timeout_after,
			max_runtime, max_retries, retry_count, retry_interval,
			broadcast_id, recurring_interval, integration_id
		FROM tasks
		WHERE id = $1 AND workspace_id = $2
		FOR UPDATE
	`

	var task domain.Task
	var stateJSON []byte
	var lastRunAt, completedAt, nextRunAfter, timeoutAfter sql.NullTime
	var broadcastID sql.NullString
	var errorMessage sql.NullString
	var recurringInterval sql.NullInt64
	var integrationID sql.NullString

	err := tx.QueryRowContext(ctx, query, id, workspace).Scan(
		&task.ID,
		&task.WorkspaceID,
		&task.Type,
		&task.Status,
		&task.Progress,
		&stateJSON,
		&errorMessage,
		&task.CreatedAt,
		&task.UpdatedAt,
		&lastRunAt,
		&completedAt,
		&nextRunAfter,
		&timeoutAfter,
		&task.MaxRuntime,
		&task.MaxRetries,
		&task.RetryCount,
		&task.RetryInterval,
		&broadcastID,
		&recurringInterval,
		&integrationID,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("task not found")
		}
		return nil, fmt.Errorf("failed to get task: %w", err)
	}

	// Handle nullable times
	if lastRunAt.Valid {
		task.LastRunAt = &lastRunAt.Time
	}
	if completedAt.Valid {
		task.CompletedAt = &completedAt.Time
	}
	if nextRunAfter.Valid {
		task.NextRunAfter = &nextRunAfter.Time
	}
	if timeoutAfter.Valid {
		task.TimeoutAfter = &timeoutAfter.Time
	}

	// Handle nullable error message
	if errorMessage.Valid {
		task.ErrorMessage = &errorMessage.String
	}

	// Handle optional broadcast ID
	if broadcastID.Valid {
		task.BroadcastID = &broadcastID.String
	}

	// Handle optional recurring fields
	if recurringInterval.Valid {
		task.RecurringInterval = &recurringInterval.Int64
	}
	if integrationID.Valid {
		task.IntegrationID = &integrationID.String
	}

	// Unmarshal state
	if stateJSON != nil {
		task.State = &domain.TaskState{}
		if err := json.Unmarshal(stateJSON, task.State); err != nil {
			return nil, fmt.Errorf("failed to unmarshal state: %w", err)
		}
	}

	return &task, nil
}

// Update updates an existing task
func (r *TaskRepository) Update(ctx context.Context, workspace string, task *domain.Task) error {
	return r.WithTransaction(ctx, func(tx *sql.Tx) error {
		return r.UpdateTx(ctx, tx, workspace, task)
	})
}

// UpdateTx updates an existing task within a transaction
func (r *TaskRepository) UpdateTx(ctx context.Context, tx *sql.Tx, workspace string, task *domain.Task) error {
	// Update timestamp
	task.UpdatedAt = time.Now().UTC()

	// Convert state to JSON
	stateJSON, err := json.Marshal(task.State)
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	// Update the task
	query := `
		UPDATE tasks
		SET
			type = $3,
			status = $4,
			progress = $5,
			state = $6,
			error_message = $7,
			updated_at = $8,
			last_run_at = $9,
			completed_at = $10,
			next_run_after = $11,
			timeout_after = $12,
			max_runtime = $13,
			max_retries = $14,
			retry_count = $15,
			retry_interval = $16,
			broadcast_id = $17,
			recurring_interval = $18,
			integration_id = $19
		WHERE id = $1 AND workspace_id = $2
	`

	result, err := tx.ExecContext(
		ctx,
		query,
		task.ID,
		workspace,
		task.Type,
		task.Status,
		task.Progress,
		stateJSON,
		task.ErrorMessage,
		task.UpdatedAt,
		task.LastRunAt,
		task.CompletedAt,
		task.NextRunAfter,
		task.TimeoutAfter,
		task.MaxRuntime,
		task.MaxRetries,
		task.RetryCount,
		task.RetryInterval,
		task.BroadcastID,
		task.RecurringInterval,
		task.IntegrationID,
	)

	if err != nil {
		return fmt.Errorf("failed to update task: %w", err)
	}

	// Check if the task was found
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("task not found")
	}

	return nil
}

// Delete removes a task
func (r *TaskRepository) Delete(ctx context.Context, workspace, id string) error {
	query := `DELETE FROM tasks WHERE id = $1 AND workspace_id = $2`
	result, err := r.systemDB.ExecContext(ctx, query, id, workspace)
	if err != nil {
		return fmt.Errorf("failed to delete task: %w", err)
	}

	// Check if the task was found
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("task not found")
	}

	return nil
}

// List retrieves tasks with optional filtering
func (r *TaskRepository) List(ctx context.Context, workspace string, filter domain.TaskFilter) ([]*domain.Task, int, error) {
	// First, build a query to get the total count
	psql := sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

	// Base query conditions
	countQuery := psql.Select("COUNT(*)").
		From("tasks").
		Where(sq.Eq{"workspace_id": workspace})

	// Apply filters
	if len(filter.Status) > 0 {
		// Convert domain.TaskStatus to strings for SQL
		statusStrings := make([]string, len(filter.Status))
		for i, s := range filter.Status {
			statusStrings[i] = string(s)
		}
		countQuery = countQuery.Where(sq.Eq{"status": statusStrings})
	}

	if len(filter.Type) > 0 {
		countQuery = countQuery.Where(sq.Eq{"type": filter.Type})
	}

	if filter.CreatedAfter != nil {
		countQuery = countQuery.Where(sq.GtOrEq{"created_at": filter.CreatedAfter})
	}

	if filter.CreatedBefore != nil {
		countQuery = countQuery.Where(sq.LtOrEq{"created_at": filter.CreatedBefore})
	}

	// Execute count query
	countSQL, countArgs, err := countQuery.ToSql()
	if err != nil {
		return nil, 0, fmt.Errorf("failed to build count query: %w", err)
	}

	var totalCount int
	err = r.systemDB.QueryRowContext(ctx, countSQL, countArgs...).Scan(&totalCount)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count tasks: %w", err)
	}

	// Build the data query
	dataQuery := psql.Select(
		"id", "workspace_id", "type", "status", "progress", "state",
		"error_message", "created_at", "updated_at", "last_run_at",
		"completed_at", "next_run_after", "timeout_after",
		"max_runtime", "max_retries", "retry_count", "retry_interval",
		"broadcast_id", "recurring_interval", "integration_id",
	).
		From("tasks").
		Where(sq.Eq{"workspace_id": workspace})

	// Apply the same filters
	if len(filter.Status) > 0 {
		statusStrings := make([]string, len(filter.Status))
		for i, s := range filter.Status {
			statusStrings[i] = string(s)
		}
		dataQuery = dataQuery.Where(sq.Eq{"status": statusStrings})
	}

	if len(filter.Type) > 0 {
		dataQuery = dataQuery.Where(sq.Eq{"type": filter.Type})
	}

	if filter.CreatedAfter != nil {
		dataQuery = dataQuery.Where(sq.GtOrEq{"created_at": filter.CreatedAfter})
	}

	if filter.CreatedBefore != nil {
		dataQuery = dataQuery.Where(sq.LtOrEq{"created_at": filter.CreatedBefore})
	}

	// Add order, limit and offset
	dataQuery = dataQuery.
		OrderBy("created_at DESC").
		Limit(uint64(filter.Limit))

	if filter.Offset > 0 {
		dataQuery = dataQuery.Offset(uint64(filter.Offset))
	}

	// Execute data query
	dataSql, dataArgs, err := dataQuery.ToSql()
	if err != nil {
		return nil, 0, fmt.Errorf("failed to build tasks query: %w", err)
	}

	rows, err := r.systemDB.QueryContext(ctx, dataSql, dataArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list tasks: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var tasks []*domain.Task
	for rows.Next() {
		var task domain.Task
		var stateJSON []byte
		var lastRunAt, completedAt, nextRunAfter, timeoutAfter sql.NullTime
		var broadcastID sql.NullString
		var errorMessage sql.NullString
		var recurringInterval sql.NullInt64
		var integrationID sql.NullString

		err := rows.Scan(
			&task.ID,
			&task.WorkspaceID,
			&task.Type,
			&task.Status,
			&task.Progress,
			&stateJSON,
			&errorMessage,
			&task.CreatedAt,
			&task.UpdatedAt,
			&lastRunAt,
			&completedAt,
			&nextRunAfter,
			&timeoutAfter,
			&task.MaxRuntime,
			&task.MaxRetries,
			&task.RetryCount,
			&task.RetryInterval,
			&broadcastID,
			&recurringInterval,
			&integrationID,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan task row: %w", err)
		}

		// Handle nullable times
		if lastRunAt.Valid {
			task.LastRunAt = &lastRunAt.Time
		}
		if completedAt.Valid {
			task.CompletedAt = &completedAt.Time
		}
		if nextRunAfter.Valid {
			task.NextRunAfter = &nextRunAfter.Time
		}
		if timeoutAfter.Valid {
			task.TimeoutAfter = &timeoutAfter.Time
		}

		// Handle nullable error message
		if errorMessage.Valid {
			task.ErrorMessage = &errorMessage.String
		}

		// Handle optional broadcast ID
		if broadcastID.Valid {
			task.BroadcastID = &broadcastID.String
		}

		// Handle optional recurring fields
		if recurringInterval.Valid {
			task.RecurringInterval = &recurringInterval.Int64
		}
		if integrationID.Valid {
			task.IntegrationID = &integrationID.String
		}

		// Unmarshal state
		if stateJSON != nil {
			task.State = &domain.TaskState{}
			if err := json.Unmarshal(stateJSON, task.State); err != nil {
				return nil, 0, fmt.Errorf("failed to unmarshal state: %w", err)
			}
		}

		tasks = append(tasks, &task)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating task rows: %w", err)
	}

	return tasks, totalCount, nil
}

// GetNextBatch retrieves tasks that are ready to be processed
func (r *TaskRepository) GetNextBatch(ctx context.Context, limit int) ([]*domain.Task, error) {
	now := time.Now().UTC()
	psql := sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

	// We want tasks that are:
	// 1. Pending and ready to run (next_run_after is null or in the past)
	// 2. Paused but ready to resume (next_run_after in the past)
	// 3. Running but have timed out (timeout_after in the past)
	query := psql.Select(
		"id", "workspace_id", "type", "status", "progress", "state",
		"error_message", "created_at", "updated_at", "last_run_at",
		"completed_at", "next_run_after", "timeout_after",
		"max_runtime", "max_retries", "retry_count", "retry_interval",
		"broadcast_id", "recurring_interval", "integration_id",
	).
		From("tasks").
		Where(sq.Or{
			sq.And{
				sq.Eq{"status": string(domain.TaskStatusPending)},
				sq.Or{
					sq.Eq{"next_run_after": nil},
					sq.LtOrEq{"next_run_after": now},
				},
			},
			sq.And{
				sq.Eq{"status": string(domain.TaskStatusPaused)},
				sq.LtOrEq{"next_run_after": now},
			},
			sq.And{
				sq.Eq{"status": string(domain.TaskStatusRunning)},
				sq.LtOrEq{"timeout_after": now},
			},
		}).
		OrderBy("next_run_after NULLS FIRST, created_at").
		Limit(uint64(limit)).
		Suffix("FOR UPDATE SKIP LOCKED")

	sqlQuery, args, err := query.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build next batch query: %w", err)
	}

	// log.Println("sqlQuery", sqlQuery)
	// log.Println("args", args)

	rows, err := r.systemDB.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get next batch of tasks: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var tasks []*domain.Task
	for rows.Next() {
		var task domain.Task
		var stateJSON []byte
		var lastRunAt, completedAt, nextRunAfter, timeoutAfter sql.NullTime
		var broadcastID sql.NullString
		var errorMessage sql.NullString
		var recurringInterval sql.NullInt64
		var integrationID sql.NullString

		err := rows.Scan(
			&task.ID,
			&task.WorkspaceID,
			&task.Type,
			&task.Status,
			&task.Progress,
			&stateJSON,
			&errorMessage,
			&task.CreatedAt,
			&task.UpdatedAt,
			&lastRunAt,
			&completedAt,
			&nextRunAfter,
			&timeoutAfter,
			&task.MaxRuntime,
			&task.MaxRetries,
			&task.RetryCount,
			&task.RetryInterval,
			&broadcastID,
			&recurringInterval,
			&integrationID,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan task row: %w", err)
		}

		// Handle nullable times
		if lastRunAt.Valid {
			task.LastRunAt = &lastRunAt.Time
		}
		if completedAt.Valid {
			task.CompletedAt = &completedAt.Time
		}
		if nextRunAfter.Valid {
			task.NextRunAfter = &nextRunAfter.Time
		}
		if timeoutAfter.Valid {
			task.TimeoutAfter = &timeoutAfter.Time
		}

		// Handle nullable error message
		if errorMessage.Valid {
			task.ErrorMessage = &errorMessage.String
		}

		// Handle optional broadcast ID
		if broadcastID.Valid {
			task.BroadcastID = &broadcastID.String
		}

		// Handle optional recurring fields
		if recurringInterval.Valid {
			task.RecurringInterval = &recurringInterval.Int64
		}
		if integrationID.Valid {
			task.IntegrationID = &integrationID.String
		}

		// Unmarshal state
		if stateJSON != nil {
			task.State = &domain.TaskState{}
			if err := json.Unmarshal(stateJSON, task.State); err != nil {
				return nil, fmt.Errorf("failed to unmarshal state: %w", err)
			}
		}

		tasks = append(tasks, &task)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating task rows: %w", err)
	}

	return tasks, nil
}

// SaveState saves the current state of a running task
func (r *TaskRepository) SaveState(ctx context.Context, workspace, id string, progress float64, state *domain.TaskState) error {
	return r.WithTransaction(ctx, func(tx *sql.Tx) error {
		return r.SaveStateTx(ctx, tx, workspace, id, progress, state)
	})
}

// SaveStateTx saves the current state of a running task within a transaction
func (r *TaskRepository) SaveStateTx(ctx context.Context, tx *sql.Tx, workspace, id string, progress float64, state *domain.TaskState) error {
	// Convert state to JSON
	stateJSON, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	now := time.Now().UTC()
	psql := sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

	query := psql.Update("tasks").
		Set("progress", progress).
		Set("state", stateJSON).
		Set("updated_at", now).
		Where(sq.And{
			sq.Eq{
				"id":           id,
				"workspace_id": workspace,
				"status":       domain.TaskStatusRunning,
			},
		})

	sqlQuery, args, err := query.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build update query: %w", err)
	}

	_, err = tx.ExecContext(ctx, sqlQuery, args...)
	if err != nil {
		return fmt.Errorf("failed to save task state: %w", err)
	}

	return nil
}

// MarkAsRunning marks a task as running and sets timeout
func (r *TaskRepository) MarkAsRunning(ctx context.Context, workspace, id string, timeoutAfter time.Time) error {
	return r.WithTransaction(ctx, func(tx *sql.Tx) error {
		return r.MarkAsRunningTx(ctx, tx, workspace, id, timeoutAfter)
	})
}

// MarkAsRunningTx marks a task as running and sets timeout within a transaction.
// Only tasks in "pending" or "paused" status can be marked as running.
// This prevents duplicate execution when multiple schedulers try to execute the same task.
func (r *TaskRepository) MarkAsRunningTx(ctx context.Context, tx *sql.Tx, workspace, id string, timeoutAfter time.Time) error {
	now := time.Now().UTC()
	psql := sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

	// Only mark as running if task is in pending or paused status
	// This prevents race conditions where multiple executors try to run the same task
	query := psql.Update("tasks").
		Set("status", domain.TaskStatusRunning).
		Set("updated_at", now).
		Set("last_run_at", now).
		Set("timeout_after", timeoutAfter).
		Where(sq.And{
			sq.Eq{"id": id},
			sq.Eq{"workspace_id": workspace},
			sq.Or{
				sq.Eq{"status": string(domain.TaskStatusPending)},
				sq.Eq{"status": string(domain.TaskStatusPaused)},
			},
		})

	sqlQuery, args, err := query.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build update query: %w", err)
	}

	result, err := tx.ExecContext(ctx, sqlQuery, args...)
	if err != nil {
		return fmt.Errorf("failed to mark task as running: %w", err)
	}

	// Check if the task was found and was in an executable state
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return &domain.ErrTaskAlreadyRunning{TaskID: id}
	}

	return nil
}

// MarkAsCompleted marks a task as completed and saves the final state
func (r *TaskRepository) MarkAsCompleted(ctx context.Context, workspace, id string, state *domain.TaskState) error {
	return r.WithTransaction(ctx, func(tx *sql.Tx) error {
		return r.MarkAsCompletedTx(ctx, tx, workspace, id, state)
	})
}

// MarkAsCompletedTx marks a task as completed within a transaction
func (r *TaskRepository) MarkAsCompletedTx(ctx context.Context, tx *sql.Tx, workspace, id string, state *domain.TaskState) error {
	now := time.Now().UTC()

	// Convert state to JSON
	stateJSON, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	psql := sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

	query := psql.Update("tasks").
		Set("status", domain.TaskStatusCompleted).
		Set("progress", 100).
		Set("state", stateJSON).
		Set("error_message", nil).
		Set("updated_at", now).
		Set("completed_at", now).
		Set("timeout_after", nil).
		Where(sq.Eq{
			"id":           id,
			"workspace_id": workspace,
		})

	sqlQuery, args, err := query.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build update query: %w", err)
	}

	result, err := tx.ExecContext(ctx, sqlQuery, args...)
	if err != nil {
		return fmt.Errorf("failed to mark task as completed: %w", err)
	}

	// Check if the task was found
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("task not found")
	}

	return nil
}

// MarkAsFailed marks a task as failed
func (r *TaskRepository) MarkAsFailed(ctx context.Context, workspace, id string, errorMsg string) error {
	return r.WithTransaction(ctx, func(tx *sql.Tx) error {
		return r.MarkAsFailedTx(ctx, tx, workspace, id, errorMsg)
	})
}

// MarkAsFailedTx marks a task as failed within a transaction
func (r *TaskRepository) MarkAsFailedTx(ctx context.Context, tx *sql.Tx, workspace, id string, errorMsg string) error {
	// Get current task to check retry counts
	task, err := r.GetTx(ctx, tx, workspace, id)
	if err != nil {
		return fmt.Errorf("failed to get task for retry check: %w", err)
	}

	now := time.Now().UTC()
	newStatus := domain.TaskStatusFailed
	psql := sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

	// Handle retries if applicable
	var nextRunAfter *time.Time
	if task.RetryCount < task.MaxRetries {
		// Calculate next retry time
		retryTime := now.Add(time.Duration(task.RetryInterval) * time.Second)
		nextRunAfter = &retryTime
		newStatus = domain.TaskStatusPending
	}

	query := psql.Update("tasks").
		Set("status", newStatus).
		Set("error_message", errorMsg).
		Set("updated_at", now).
		Set("retry_count", sq.Expr("retry_count + 1")).
		Set("next_run_after", nextRunAfter).
		Set("timeout_after", nil).
		Where(sq.Eq{
			"id":           id,
			"workspace_id": workspace,
		})

	sqlQuery, args, err := query.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build update query: %w", err)
	}

	result, err := tx.ExecContext(ctx, sqlQuery, args...)
	if err != nil {
		return fmt.Errorf("failed to mark task as failed: %w", err)
	}

	// Check if the task was found
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("task not found")
	}

	return nil
}

// MarkAsPaused marks a task as paused and sets the next run time
func (r *TaskRepository) MarkAsPaused(ctx context.Context, workspace, id string, nextRunAfter time.Time, progress float64, state *domain.TaskState) error {
	return r.WithTransaction(ctx, func(tx *sql.Tx) error {
		return r.MarkAsPausedTx(ctx, tx, workspace, id, nextRunAfter, progress, state)
	})
}

// MarkAsPausedTx marks a task as paused and sets the next run time within a transaction
func (r *TaskRepository) MarkAsPausedTx(ctx context.Context, tx *sql.Tx, workspace, id string, nextRunAfter time.Time, progress float64, state *domain.TaskState) error {
	now := time.Now().UTC()

	// Convert state to JSON
	stateJSON, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	psql := sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

	// Update task with the provided progress and state
	query := psql.Update("tasks").
		Set("status", domain.TaskStatusPaused).
		Set("progress", progress). // Use the provided progress
		Set("state", stateJSON).   // Use the provided state
		Set("updated_at", now).
		Set("next_run_after", nextRunAfter).
		Set("timeout_after", nil).
		Set("retry_count", sq.Expr("retry_count + 1")). // Increment retry count
		Where(sq.Eq{
			"id":           id,
			"workspace_id": workspace,
		})

	sqlQuery, args, err := query.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build update query: %w", err)
	}

	result, err := tx.ExecContext(ctx, sqlQuery, args...)
	if err != nil {
		return fmt.Errorf("failed to mark task as paused: %w", err)
	}

	// Check if the task was found
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("task not found")
	}

	return nil
}

// MarkAsPending marks a task as pending and sets the next run time
func (r *TaskRepository) MarkAsPending(ctx context.Context, workspace, id string, nextRunAfter time.Time, progress float64, state *domain.TaskState) error {
	return r.WithTransaction(ctx, func(tx *sql.Tx) error {
		return r.MarkAsPendingTx(ctx, tx, workspace, id, nextRunAfter, progress, state)
	})
}

// MarkAsPendingTx marks a task as pending and sets the next run time within a transaction
func (r *TaskRepository) MarkAsPendingTx(ctx context.Context, tx *sql.Tx, workspace, id string, nextRunAfter time.Time, progress float64, state *domain.TaskState) error {
	now := time.Now().UTC()

	// Convert state to JSON
	stateJSON, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	psql := sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

	// Update task with the provided progress and state
	query := psql.Update("tasks").
		Set("status", domain.TaskStatusPending).
		Set("progress", progress).
		Set("state", stateJSON).
		Set("updated_at", now).
		Set("next_run_after", nextRunAfter).
		Set("timeout_after", nil).
		Where(sq.Eq{
			"id":           id,
			"workspace_id": workspace,
		})

	sqlQuery, args, err := query.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build update query: %w", err)
	}

	result, err := tx.ExecContext(ctx, sqlQuery, args...)
	if err != nil {
		return fmt.Errorf("failed to mark task as pending: %w", err)
	}

	// Check if the task was found
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("task not found")
	}

	return nil
}

// GetTaskByBroadcastID retrieves a task associated with a specific broadcast ID
func (r *TaskRepository) GetTaskByBroadcastID(ctx context.Context, workspace, broadcastID string) (*domain.Task, error) {
	var task *domain.Task
	var err error

	err = r.WithTransaction(ctx, func(tx *sql.Tx) error {
		task, err = r.GetTaskByBroadcastIDTx(ctx, tx, workspace, broadcastID)
		return err
	})

	return task, err
}

// GetTaskByBroadcastIDTx retrieves a task by broadcast ID within a transaction
func (r *TaskRepository) GetTaskByBroadcastIDTx(ctx context.Context, tx *sql.Tx, workspace, broadcastID string) (*domain.Task, error) {
	query := `
		SELECT
			id, workspace_id, type, status, progress, state,
			error_message, created_at, updated_at, last_run_at,
			completed_at, next_run_after, timeout_after,
			max_runtime, max_retries, retry_count, retry_interval,
			broadcast_id, recurring_interval, integration_id
		FROM tasks
		WHERE workspace_id = $1 AND broadcast_id = $2
		AND type = 'send_broadcast'
		LIMIT 1
		FOR UPDATE
	`

	var task domain.Task
	var stateJSON []byte
	var lastRunAt, completedAt, nextRunAfter, timeoutAfter sql.NullTime
	var dbBroadcastID sql.NullString
	var errorMessage sql.NullString
	var recurringInterval sql.NullInt64
	var integrationID sql.NullString

	err := tx.QueryRowContext(ctx, query, workspace, broadcastID).Scan(
		&task.ID,
		&task.WorkspaceID,
		&task.Type,
		&task.Status,
		&task.Progress,
		&stateJSON,
		&errorMessage,
		&task.CreatedAt,
		&task.UpdatedAt,
		&lastRunAt,
		&completedAt,
		&nextRunAfter,
		&timeoutAfter,
		&task.MaxRuntime,
		&task.MaxRetries,
		&task.RetryCount,
		&task.RetryInterval,
		&dbBroadcastID,
		&recurringInterval,
		&integrationID,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("task not found for broadcast ID %s", broadcastID)
		}
		return nil, fmt.Errorf("failed to get task by broadcast ID: %w", err)
	}

	// Handle nullable times
	if lastRunAt.Valid {
		task.LastRunAt = &lastRunAt.Time
	}
	if completedAt.Valid {
		task.CompletedAt = &completedAt.Time
	}
	if nextRunAfter.Valid {
		task.NextRunAfter = &nextRunAfter.Time
	}
	if timeoutAfter.Valid {
		task.TimeoutAfter = &timeoutAfter.Time
	}

	// Handle nullable error message
	if errorMessage.Valid {
		task.ErrorMessage = &errorMessage.String
	}

	// Handle optional broadcast ID
	if dbBroadcastID.Valid {
		task.BroadcastID = &dbBroadcastID.String
	}

	// Handle optional recurring fields
	if recurringInterval.Valid {
		task.RecurringInterval = &recurringInterval.Int64
	}
	if integrationID.Valid {
		task.IntegrationID = &integrationID.String
	}

	// Unmarshal state
	if stateJSON != nil {
		task.State = &domain.TaskState{}
		if err := json.Unmarshal(stateJSON, task.State); err != nil {
			return nil, fmt.Errorf("failed to unmarshal state: %w", err)
		}
	}

	return &task, nil
}

// GetTaskByIntegrationID retrieves an active task by integration ID
func (r *TaskRepository) GetTaskByIntegrationID(ctx context.Context, workspace, integrationID string) (*domain.Task, error) {
	var task *domain.Task
	var err error

	err = r.WithTransaction(ctx, func(tx *sql.Tx) error {
		task, err = r.GetTaskByIntegrationIDTx(ctx, tx, workspace, integrationID)
		return err
	})

	return task, err
}

// GetTaskByIntegrationIDTx retrieves an active task by integration ID within a transaction
func (r *TaskRepository) GetTaskByIntegrationIDTx(ctx context.Context, tx *sql.Tx, workspace, integrationID string) (*domain.Task, error) {
	query := `
		SELECT
			id, workspace_id, type, status, progress, state,
			error_message, created_at, updated_at, last_run_at,
			completed_at, next_run_after, timeout_after,
			max_runtime, max_retries, retry_count, retry_interval,
			broadcast_id, recurring_interval, integration_id
		FROM tasks
		WHERE workspace_id = $1 AND integration_id = $2
		AND status NOT IN ('completed', 'failed')
		LIMIT 1
		FOR UPDATE
	`

	var task domain.Task
	var stateJSON []byte
	var lastRunAt, completedAt, nextRunAfter, timeoutAfter sql.NullTime
	var broadcastID sql.NullString
	var errorMessage sql.NullString
	var recurringInterval sql.NullInt64
	var dbIntegrationID sql.NullString

	err := tx.QueryRowContext(ctx, query, workspace, integrationID).Scan(
		&task.ID,
		&task.WorkspaceID,
		&task.Type,
		&task.Status,
		&task.Progress,
		&stateJSON,
		&errorMessage,
		&task.CreatedAt,
		&task.UpdatedAt,
		&lastRunAt,
		&completedAt,
		&nextRunAfter,
		&timeoutAfter,
		&task.MaxRuntime,
		&task.MaxRetries,
		&task.RetryCount,
		&task.RetryInterval,
		&broadcastID,
		&recurringInterval,
		&dbIntegrationID,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrTaskNotFound
		}
		return nil, fmt.Errorf("failed to get task by integration ID: %w", err)
	}

	// Handle nullable times
	if lastRunAt.Valid {
		task.LastRunAt = &lastRunAt.Time
	}
	if completedAt.Valid {
		task.CompletedAt = &completedAt.Time
	}
	if nextRunAfter.Valid {
		task.NextRunAfter = &nextRunAfter.Time
	}
	if timeoutAfter.Valid {
		task.TimeoutAfter = &timeoutAfter.Time
	}

	// Handle nullable error message
	if errorMessage.Valid {
		task.ErrorMessage = &errorMessage.String
	}

	// Handle optional broadcast ID
	if broadcastID.Valid {
		task.BroadcastID = &broadcastID.String
	}

	// Handle optional recurring fields
	if recurringInterval.Valid {
		task.RecurringInterval = &recurringInterval.Int64
	}
	if dbIntegrationID.Valid {
		task.IntegrationID = &dbIntegrationID.String
	}

	// Unmarshal state
	if stateJSON != nil {
		task.State = &domain.TaskState{}
		if err := json.Unmarshal(stateJSON, task.State); err != nil {
			return nil, fmt.Errorf("failed to unmarshal state: %w", err)
		}
	}

	return &task, nil
}

func (r *TaskRepository) DeleteAll(ctx context.Context, workspace string) error {
	psql := sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

	query := psql.Delete("tasks").
		Where(sq.Eq{"workspace_id": workspace})

	sqlQuery, args, err := query.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build delete query: %w", err)
	}

	_, err = r.systemDB.ExecContext(ctx, sqlQuery, args...)
	if err != nil {
		return fmt.Errorf("failed to delete tasks: %w", err)
	}

	return nil
}
