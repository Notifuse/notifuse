package domain

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

//go:generate mockgen -destination mocks/mock_task_service.go -package mocks github.com/Notifuse/notifuse/internal/domain TaskService
//go:generate mockgen -destination mocks/mock_task_repository.go -package mocks github.com/Notifuse/notifuse/internal/domain TaskRepository
//go:generate mockgen -destination mocks/mock_task_processor.go -package mocks github.com/Notifuse/notifuse/internal/domain TaskProcessor

// TaskStatus represents the current state of a task
type TaskStatus string

const (
	// TaskStatusPending is for tasks that haven't started yet
	TaskStatusPending TaskStatus = "pending"
	// TaskStatusRunning is for tasks that are currently running
	TaskStatusRunning TaskStatus = "running"
	// TaskStatusCompleted is for tasks that have completed successfully
	TaskStatusCompleted TaskStatus = "completed"
	// TaskStatusFailed is for tasks that have failed
	TaskStatusFailed TaskStatus = "failed"
	// TaskStatusPaused is for tasks that were paused due to timeout
	TaskStatusPaused TaskStatus = "paused"
)

// TaskState represents the state of a task, with specialized fields for different task types
type TaskState struct {
	// Common fields for all task types
	Progress float64 `json:"progress,omitempty"`
	Message  string  `json:"message,omitempty"`

	// Specialized states for different task types - only one will be used based on task type
	SendBroadcast *SendBroadcastState `json:"send_broadcast,omitempty"`
}

// Value implements the driver.Valuer interface for TaskState
func (s TaskState) Value() (driver.Value, error) {
	return json.Marshal(s)
}

// Scan implements the sql.Scanner interface for TaskState
func (s *TaskState) Scan(value interface{}) error {
	if value == nil {
		*s = TaskState{}
		return nil
	}

	b, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("expected []byte, got %T", value)
	}

	cloned := bytes.Clone(b)
	return json.Unmarshal(cloned, &s)
}

// SendBroadcastState contains state specific to broadcast sending tasks
type SendBroadcastState struct {
	BroadcastID     string `json:"broadcast_id"`
	TotalRecipients int    `json:"total_recipients"`
	SentCount       int    `json:"sent_count"`
	FailedCount     int    `json:"failed_count"`
	ChannelType     string `json:"channel_type"`
	RecipientOffset int64  `json:"recipient_offset"`
	// New fields for A/B testing phases
	Phase                     string `json:"phase"` // "test", "winner", or "single"
	TestPhaseCompleted        bool   `json:"test_phase_completed"`
	TestPhaseRecipientCount   int    `json:"test_phase_recipient_count"`
	WinnerPhaseRecipientCount int    `json:"winner_phase_recipient_count"`
}

// Task represents a background task that can be executed in multiple steps
type Task struct {
	ID            string     `json:"id"`
	WorkspaceID   string     `json:"workspace_id"`
	Type          string     `json:"type"`
	Status        TaskStatus `json:"status"`
	Progress      float64    `json:"progress"`
	State         *TaskState `json:"state,omitempty"` // Typed state struct
	ErrorMessage  string     `json:"error_message,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	LastRunAt     *time.Time `json:"last_run_at,omitempty"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
	NextRunAfter  *time.Time `json:"next_run_after,omitempty"`
	TimeoutAfter  *time.Time `json:"timeout_after,omitempty"`
	MaxRuntime    int        `json:"max_runtime"` // Maximum runtime in seconds
	MaxRetries    int        `json:"max_retries"`
	RetryCount    int        `json:"retry_count"`
	RetryInterval int        `json:"retry_interval"`         // Retry interval in seconds
	BroadcastID   *string    `json:"broadcast_id,omitempty"` // Optional reference to a broadcast
}

type TaskService interface {
	RegisterProcessor(processor TaskProcessor)
	GetProcessor(taskType string) (TaskProcessor, error)
	CreateTask(ctx context.Context, workspace string, task *Task) error
	GetTask(ctx context.Context, workspace, id string) (*Task, error)
	ListTasks(ctx context.Context, workspace string, filter TaskFilter) (*TaskListResponse, error)
	DeleteTask(ctx context.Context, workspace, id string) error
	ExecutePendingTasks(ctx context.Context, maxTasks int) error
	ExecuteTask(ctx context.Context, workspace, taskID string, timeoutAt time.Time) error
	GetLastCronRun(ctx context.Context) (*time.Time, error)
	SubscribeToBroadcastEvents(eventBus EventBus)
}

// TaskRepository defines methods for task persistence
type TaskRepository interface {
	// Transaction support
	WithTransaction(ctx context.Context, fn func(*sql.Tx) error) error

	// Create adds a new task
	Create(ctx context.Context, workspace string, task *Task) error
	CreateTx(ctx context.Context, tx *sql.Tx, workspace string, task *Task) error

	// Get retrieves a task by ID
	Get(ctx context.Context, workspace, id string) (*Task, error)
	GetTx(ctx context.Context, tx *sql.Tx, workspace, id string) (*Task, error)

	// Get a task by broadcast ID
	GetTaskByBroadcastID(ctx context.Context, workspace, broadcastID string) (*Task, error)
	GetTaskByBroadcastIDTx(ctx context.Context, tx *sql.Tx, workspace, broadcastID string) (*Task, error)

	// Update updates an existing task
	Update(ctx context.Context, workspace string, task *Task) error
	UpdateTx(ctx context.Context, tx *sql.Tx, workspace string, task *Task) error

	// Delete removes a task
	Delete(ctx context.Context, workspace, id string) error
	DeleteAll(ctx context.Context, workspace string) error

	// List retrieves tasks with optional filtering
	List(ctx context.Context, workspace string, filter TaskFilter) ([]*Task, int, error)

	// GetNextBatch retrieves tasks that are ready to be processed
	GetNextBatch(ctx context.Context, limit int) ([]*Task, error)

	// MarkAsRunning marks a task as running and sets timeout
	MarkAsRunning(ctx context.Context, workspace, id string, timeoutAfter time.Time) error
	MarkAsRunningTx(ctx context.Context, tx *sql.Tx, workspace, id string, timeoutAfter time.Time) error

	// SaveState saves the current state of a running task
	SaveState(ctx context.Context, workspace, id string, progress float64, state *TaskState) error
	SaveStateTx(ctx context.Context, tx *sql.Tx, workspace, id string, progress float64, state *TaskState) error

	// MarkAsCompleted marks a task as completed
	MarkAsCompleted(ctx context.Context, workspace, id string) error
	MarkAsCompletedTx(ctx context.Context, tx *sql.Tx, workspace, id string) error

	// MarkAsFailed marks a task as failed
	MarkAsFailed(ctx context.Context, workspace, id string, errorMsg string) error
	MarkAsFailedTx(ctx context.Context, tx *sql.Tx, workspace, id string, errorMsg string) error

	// MarkAsPaused marks a task as paused (e.g., due to timeout)
	MarkAsPaused(ctx context.Context, workspace, id string, nextRunAfter time.Time, progress float64, state *TaskState) error
	MarkAsPausedTx(ctx context.Context, tx *sql.Tx, workspace, id string, nextRunAfter time.Time, progress float64, state *TaskState) error
}

// TaskFilter defines the filtering criteria for task listing
type TaskFilter struct {
	Status        []TaskStatus
	Type          []string
	CreatedAfter  *time.Time
	CreatedBefore *time.Time
	Limit         int
	Offset        int
}

// TaskProcessor defines the interface for task execution
type TaskProcessor interface {
	// Process executes or continues a task, returns whether the task has been completed
	Process(ctx context.Context, task *Task, timeoutAt time.Time) (completed bool, err error)

	// CanProcess returns whether this processor can handle the given task type
	CanProcess(taskType string) bool
}

// TaskExecutor is responsible for executing tasks
type TaskExecutor interface {
	// ExecutePendingTasks finds and executes pending tasks
	ExecutePendingTasks(ctx context.Context, maxTasks int) error

	// ExecuteTask executes a specific task
	ExecuteTask(ctx context.Context, workspaceID, taskID string, timeoutAt time.Time) error

	// RegisterProcessor registers a task processor for a specific task type
	RegisterProcessor(processor TaskProcessor)
}

// CreateTaskRequest defines the request to create a new task
type CreateTaskRequest struct {
	WorkspaceID   string     `json:"workspace_id"`
	Type          string     `json:"type"`
	State         *TaskState `json:"state,omitempty"` // New typed state struct
	MaxRuntime    int        `json:"max_runtime"`
	MaxRetries    int        `json:"max_retries"`
	RetryInterval int        `json:"retry_interval"`
	NextRunAfter  *time.Time `json:"next_run_after,omitempty"`
}

// Validate validates the create task request
func (r *CreateTaskRequest) Validate() (*Task, error) {
	if r.WorkspaceID == "" {
		return nil, fmt.Errorf("workspace_id is required")
	}

	if r.Type == "" {
		return nil, fmt.Errorf("task type is required")
	}

	task := &Task{
		WorkspaceID:   r.WorkspaceID,
		Type:          r.Type,
		Status:        TaskStatusPending,
		State:         r.State,
		MaxRuntime:    r.MaxRuntime,
		MaxRetries:    r.MaxRetries,
		RetryInterval: r.RetryInterval,
		NextRunAfter:  r.NextRunAfter,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}

	// Set defaults if not provided
	if task.MaxRuntime <= 0 {
		task.MaxRuntime = 50 // 50 seconds default
	}

	if task.MaxRetries <= 0 {
		task.MaxRetries = 3
	}

	if task.RetryInterval <= 0 {
		task.RetryInterval = 300 // 5 minutes default
	}

	return task, nil
}

// TaskListResponse defines the response for listing tasks
type TaskListResponse struct {
	Tasks      []*Task `json:"tasks"`
	TotalCount int     `json:"total_count"`
	Limit      int     `json:"limit"`
	Offset     int     `json:"offset"`
	HasMore    bool    `json:"has_more"`
}

// GetTaskRequest is used to extract query parameters for getting a single task
type GetTaskRequest struct {
	WorkspaceID string `json:"workspace_id"`
	ID          string `json:"id"`
}

// FromURLParams parses URL query parameters into the request
func (r *GetTaskRequest) FromURLParams(values url.Values) error {
	r.WorkspaceID = values.Get("workspace_id")
	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}

	r.ID = values.Get("id")
	if r.ID == "" {
		return fmt.Errorf("id is required")
	}

	return nil
}

// DeleteTaskRequest defines the request to delete a task
type DeleteTaskRequest struct {
	WorkspaceID string `json:"workspace_id"`
	ID          string `json:"id"`
}

// FromURLParams parses URL query parameters into the request
func (r *DeleteTaskRequest) FromURLParams(values url.Values) error {
	r.WorkspaceID = values.Get("workspace_id")
	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}

	r.ID = values.Get("id")
	if r.ID == "" {
		return fmt.Errorf("id is required")
	}

	return nil
}

// ListTasksRequest is used to extract query parameters for listing tasks
type ListTasksRequest struct {
	WorkspaceID   string   `json:"workspace_id"`
	Status        []string `json:"status,omitempty"`
	Type          []string `json:"type,omitempty"`
	CreatedAfter  string   `json:"created_after,omitempty"`
	CreatedBefore string   `json:"created_before,omitempty"`
	Limit         int      `json:"limit,omitempty"`
	Offset        int      `json:"offset,omitempty"`
}

// FromURLParams parses URL query parameters into the request
func (r *ListTasksRequest) FromURLParams(values url.Values) error {
	r.WorkspaceID = values.Get("workspace_id")
	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}

	if statusParam := values.Get("status"); statusParam != "" {
		r.Status = splitAndTrim(statusParam)
	}

	if typeParam := values.Get("type"); typeParam != "" {
		r.Type = splitAndTrim(typeParam)
	}

	r.CreatedAfter = values.Get("created_after")
	r.CreatedBefore = values.Get("created_before")

	if limitStr := values.Get("limit"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil {
			return fmt.Errorf("invalid limit parameter: %w", err)
		}
		r.Limit = limit
	}

	if offsetStr := values.Get("offset"); offsetStr != "" {
		offset, err := strconv.Atoi(offsetStr)
		if err != nil {
			return fmt.Errorf("invalid offset parameter: %w", err)
		}
		r.Offset = offset
	}

	return nil
}

// ToFilter converts the request to a TaskFilter
func (r *ListTasksRequest) ToFilter() TaskFilter {
	filter := TaskFilter{
		Limit:  r.Limit,
		Offset: r.Offset,
	}

	// Set default limit if not provided
	if filter.Limit <= 0 {
		filter.Limit = 100 // Default page size
	}

	// Convert status strings to TaskStatus
	if len(r.Status) > 0 {
		filter.Status = make([]TaskStatus, len(r.Status))
		for i, s := range r.Status {
			filter.Status[i] = TaskStatus(s)
		}
	}

	filter.Type = r.Type

	// Parse date filters
	if r.CreatedAfter != "" {
		if t, err := time.Parse(time.RFC3339, r.CreatedAfter); err == nil {
			filter.CreatedAfter = &t
		}
	}

	if r.CreatedBefore != "" {
		if t, err := time.Parse(time.RFC3339, r.CreatedBefore); err == nil {
			filter.CreatedBefore = &t
		}
	}

	return filter
}

// splitAndTrim splits a comma-separated string into an array and trims spaces
func splitAndTrim(s string) []string {
	if s == "" {
		return nil
	}

	parts := make([]string, 0)
	for _, part := range strings.Split(s, ",") {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return parts
}

// ExecutePendingTasksRequest defines the request to execute pending tasks
type ExecutePendingTasksRequest struct {
	MaxTasks int `json:"max_tasks,omitempty"`
}

// FromURLParams parses URL query parameters into the request
func (r *ExecutePendingTasksRequest) FromURLParams(values url.Values) error {
	if maxTasksStr := values.Get("max_tasks"); maxTasksStr != "" {
		maxTasks, err := strconv.Atoi(maxTasksStr)
		if err != nil {
			return fmt.Errorf("invalid max_tasks parameter: %w", err)
		}
		r.MaxTasks = maxTasks
	} else {
		r.MaxTasks = 10 // default value
	}

	return nil
}

// ExecuteTaskRequest defines the request to execute a specific task
type ExecuteTaskRequest struct {
	WorkspaceID string `json:"workspace_id"`
	ID          string `json:"id"`
}

// Validate validates the execute task request
func (r *ExecuteTaskRequest) Validate() error {
	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}

	if r.ID == "" {
		return fmt.Errorf("task id is required")
	}

	return nil
}
