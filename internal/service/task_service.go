package service

import (
	"bytes"
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/pkg/tracing"
)

// Maximum time a task can run before timing out
const defaultMaxTaskRuntime = 50 // 50 seconds

// TaskService manages task execution and state
type TaskService struct {
	repo        domain.TaskRepository
	settingRepo domain.SettingRepository
	logger      logger.Logger
	authService *AuthService
	processors  map[string]domain.TaskProcessor
	lock        sync.RWMutex
	apiEndpoint string
	// autoExecuteImmediate controls whether tasks are automatically executed when set to immediate
	// This is mainly used to disable auto-execution during testing
	autoExecuteImmediate bool
}

// WithTransaction executes a function within a transaction
func (s *TaskService) WithTransaction(ctx context.Context, fn func(*sql.Tx) error) error {
	// The repository should handle the transaction
	repo, ok := s.repo.(interface {
		WithTransaction(ctx context.Context, fn func(*sql.Tx) error) error
	})
	if !ok {
		return fmt.Errorf("repository does not support transactions")
	}

	return repo.WithTransaction(ctx, fn)
}

// NewTaskService creates a new task service instance
func NewTaskService(repository domain.TaskRepository, settingRepo domain.SettingRepository, logger logger.Logger, authService *AuthService, apiEndpoint string) *TaskService {

	return &TaskService{
		repo:                 repository,
		settingRepo:          settingRepo,
		logger:               logger,
		authService:          authService,
		processors:           make(map[string]domain.TaskProcessor),
		apiEndpoint:          apiEndpoint,
		autoExecuteImmediate: true, // Enable auto-execution by default
	}
}

// SetAutoExecuteImmediate sets whether tasks should be automatically executed immediately
// This is mainly used for testing to disable auto-execution
func (s *TaskService) SetAutoExecuteImmediate(enabled bool) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.autoExecuteImmediate = enabled
}

// IsAutoExecuteEnabled returns whether automatic task execution is enabled
func (s *TaskService) IsAutoExecuteEnabled() bool {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.autoExecuteImmediate
}

// RegisterProcessor registers a task processor for a specific task type
func (s *TaskService) RegisterProcessor(processor domain.TaskProcessor) {
	s.lock.Lock()
	defer s.lock.Unlock()

	// Determine which task types this processor can handle
	for _, taskType := range getTaskTypes() {
		if processor.CanProcess(taskType) {
			s.processors[taskType] = processor
			s.logger.WithField("task_type", taskType).Info("Registered task processor")
		}
	}
}

// getTaskTypes returns all supported task types
func getTaskTypes() []string {
	// This could be expanded with more task types as needed
	return []string{
		"import_contacts",
		"export_contacts",
		"send_broadcast",
		"generate_report",
		"build_segment",
		"process_contact_segment_queue",
		"check_segment_recompute",
		"sync_integration",
	}
}

// GetProcessor returns the processor for a given task type
func (s *TaskService) GetProcessor(taskType string) (domain.TaskProcessor, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	processor, ok := s.processors[taskType]
	if !ok {
		return nil, fmt.Errorf("no processor registered for task type: %s", taskType)
	}

	return processor, nil
}

// CreateTask creates a new task
func (s *TaskService) CreateTask(ctx context.Context, workspace string, task *domain.Task) error {
	ctx, span := tracing.StartServiceSpan(ctx, "TaskService", "CreateTask")
	defer tracing.EndSpan(span, nil)

	tracing.AddAttribute(ctx, "workspace_id", workspace)
	if task.BroadcastID != nil {
		tracing.AddAttribute(ctx, "broadcast_id", *task.BroadcastID)
	}
	tracing.AddAttribute(ctx, "task_type", task.Type)

	if task.MaxRuntime <= 0 {
		task.MaxRuntime = defaultMaxTaskRuntime
	}

	// Set default retry settings if not provided
	if task.MaxRetries <= 0 {
		task.MaxRetries = 3
	}
	if task.RetryInterval <= 0 {
		task.RetryInterval = 60 // Default to 1 minute between retries
	}

	err := s.repo.Create(ctx, workspace, task)
	if err != nil {
		tracing.MarkSpanError(ctx, err)
	}
	return err
}

// GetTask retrieves a task by ID
func (s *TaskService) GetTask(ctx context.Context, workspace, id string) (*domain.Task, error) {
	ctx, span := tracing.StartServiceSpan(ctx, "TaskService", "GetTask")
	defer tracing.EndSpan(span, nil)

	tracing.AddAttribute(ctx, "workspace_id", workspace)
	tracing.AddAttribute(ctx, "task_id", id)

	task, err := s.repo.Get(ctx, workspace, id)
	if err != nil {
		tracing.MarkSpanError(ctx, err)
	} else if task != nil && task.BroadcastID != nil {
		tracing.AddAttribute(ctx, "broadcast_id", *task.BroadcastID)
	}

	return task, err
}

// ListTasks lists tasks based on filter criteria
func (s *TaskService) ListTasks(ctx context.Context, workspace string, filter domain.TaskFilter) (*domain.TaskListResponse, error) {
	ctx, span := tracing.StartServiceSpan(ctx, "TaskService", "ListTasks")
	defer tracing.EndSpan(span, nil)

	tracing.AddAttribute(ctx, "workspace_id", workspace)
	tracing.AddAttribute(ctx, "limit", filter.Limit)
	tracing.AddAttribute(ctx, "offset", filter.Offset)

	// Removed Status and Type tracing attributes to fix compilation issues

	tasks, totalCount, err := s.repo.List(ctx, workspace, filter)
	if err != nil {
		tracing.MarkSpanError(ctx, err)
		return nil, err
	}

	// Calculate if there are more results
	hasMore := (filter.Offset + len(tasks)) < totalCount
	tracing.AddAttribute(ctx, "total_count", totalCount)
	tracing.AddAttribute(ctx, "result_count", len(tasks))
	tracing.AddAttribute(ctx, "has_more", hasMore)

	return &domain.TaskListResponse{
		Tasks:      tasks,
		TotalCount: totalCount,
		Limit:      filter.Limit,
		Offset:     filter.Offset,
		HasMore:    hasMore,
	}, nil
}

// DeleteTask removes a task
func (s *TaskService) DeleteTask(ctx context.Context, workspace, id string) error {
	ctx, span := tracing.StartServiceSpan(ctx, "TaskService", "DeleteTask")
	defer tracing.EndSpan(span, nil)

	tracing.AddAttribute(ctx, "workspace_id", workspace)
	tracing.AddAttribute(ctx, "task_id", id)

	err := s.repo.Delete(ctx, workspace, id)
	if err != nil {
		tracing.MarkSpanError(ctx, err)
	}
	return err
}

// ExecutePendingTasks processes a batch of pending tasks
func (s *TaskService) ExecutePendingTasks(ctx context.Context, maxTasks int) error {
	ctx, span := tracing.StartServiceSpan(ctx, "TaskService", "ExecutePendingTasks")
	defer tracing.EndSpan(span, nil)

	// Set the last cron run timestamp before processing tasks
	if err := s.settingRepo.SetLastCronRun(ctx); err != nil {
		tracing.MarkSpanError(ctx, err)
		s.logger.WithField("error", err.Error()).Error("Failed to set last cron run timestamp")
		// Continue processing tasks even if setting update fails
	} else {
		s.logger.Debug("Updated last cron run timestamp")
	}

	// Get the next batch of tasks
	// each workspace has a permanent task to process the contact segment queue
	// TODO/problem: if new number of workspaces is above 100, this will not work
	// we need to have a way to scale this
	if maxTasks <= 0 {
		maxTasks = 100 // Default value
	}

	tracing.AddAttribute(ctx, "max_tasks", maxTasks)

	tasks, err := s.repo.GetNextBatch(ctx, maxTasks)
	if err != nil {
		tracing.MarkSpanError(ctx, err)
		return fmt.Errorf("failed to get next batch of tasks: %w", err)
	}

	tracing.AddAttribute(ctx, "task_count", len(tasks))
	s.logger.WithField("task_count", len(tasks)).Info("Retrieved batch of tasks to process")

	if s.apiEndpoint == "" {
		tracing.AddAttribute(ctx, "execution_mode", "direct")
		s.logger.Warn("API endpoint not configured, falling back to direct execution")
		return s.executeTasksDirectly(ctx, tasks)
	}

	tracing.AddAttribute(ctx, "execution_mode", "http")

	// Use a wait group to wait for all HTTP requests to complete
	var wg sync.WaitGroup

	// Create HTTP client with connection pooling for reuse across tasks
	// Per Go docs: "Clients and Transports are safe for concurrent use by multiple
	// goroutines and for efficiency should only be created once and re-used."
	httpClient := &http.Client{
		Timeout: 53 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 100,
			IdleConnTimeout:     90 * time.Second,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}
	httpClient = tracing.WrapHTTPClient(httpClient)

	// Execute tasks using HTTP roundtrips
	for _, task := range tasks {
		// Add to wait group before launching goroutine
		wg.Add(1)

		go func(t *domain.Task) {
			defer wg.Done() // Signal completion when goroutine finishes

			taskCtx, taskSpan := tracing.StartServiceSpan(ctx, "TaskService", "DispatchTaskExecution")
			defer tracing.EndSpan(taskSpan, nil)

			tracing.AddAttribute(taskCtx, "task_id", t.ID)
			tracing.AddAttribute(taskCtx, "workspace_id", t.WorkspaceID)
			tracing.AddAttribute(taskCtx, "task_type", t.Type)
			if t.BroadcastID != nil {
				tracing.AddAttribute(taskCtx, "broadcast_id", *t.BroadcastID)
			}

			s.logger.WithField("task_id", t.ID).
				WithField("workspace_id", t.WorkspaceID).
				Info("Dispatching task execution via HTTP")

			// Create request payload
			reqBody, err := json.Marshal(domain.ExecuteTaskRequest{
				WorkspaceID: t.WorkspaceID,
				ID:          t.ID,
			})
			if err != nil {
				tracing.MarkSpanError(taskCtx, err)
				s.logger.WithField("task_id", t.ID).
					WithField("workspace_id", t.WorkspaceID).
					WithField("error", err.Error()).
					Error("Failed to marshal task execution request")
				return
			}

			// Create request with tracing context
			endpoint := fmt.Sprintf("%s/api/tasks.execute", s.apiEndpoint)
			req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewBuffer(reqBody))
			if err != nil {
				tracing.MarkSpanError(taskCtx, err)
				s.logger.WithField("task_id", t.ID).
					WithField("workspace_id", t.WorkspaceID).
					WithField("error", err.Error()).
					Error("Failed to create HTTP request for task execution")
				return
			}

			// Set content type
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Task-ID", t.ID) // Add task ID for tracing

			// Execute request
			resp, err := httpClient.Do(req)
			if err != nil {
				tracing.MarkSpanError(taskCtx, err)
				s.logger.WithField("task_id", t.ID).
					WithField("workspace_id", t.WorkspaceID).
					WithField("error", err.Error()).
					Error("HTTP request for task execution failed")
				return
			}
			defer func() { _ = resp.Body.Close() }()

			// Check response
			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)

				// 409 Conflict means task is already running - this is expected in concurrent scenarios
				if resp.StatusCode == http.StatusConflict {
					s.logger.WithField("task_id", t.ID).
						WithField("workspace_id", t.WorkspaceID).
						Debug("Task already running, skipping duplicate dispatch")
					return
				}

				// Other non-OK statuses are actual errors
				err := fmt.Errorf("non-OK status: %d, response: %s", resp.StatusCode, string(body))
				tracing.MarkSpanError(taskCtx, err)
				s.logger.WithField("task_id", t.ID).
					WithField("workspace_id", t.WorkspaceID).
					WithField("status_code", resp.StatusCode).
					WithField("response", string(body)).
					Error("HTTP request for task execution returned non-OK status")
				return
			}

			s.logger.WithField("task_id", t.ID).
				WithField("workspace_id", t.WorkspaceID).
				Info("Task execution request dispatched successfully")
		}(task)
	}

	// Wait for all HTTP requests to complete
	wg.Wait()

	return nil
}

// executeTasksDirectly processes tasks directly without HTTP roundtrips
// This is used as a fallback when API endpoint is not configured
func (s *TaskService) executeTasksDirectly(ctx context.Context, tasks []*domain.Task) error {
	ctx, span := tracing.StartServiceSpan(ctx, "TaskService", "executeTasksDirectly")
	defer tracing.EndSpan(span, nil)

	now := time.Now().UTC()

	tracing.AddAttribute(ctx, "task_count", len(tasks))

	// Use a wait group to wait for all goroutines to complete
	var wg sync.WaitGroup

	for _, task := range tasks {
		// Calculate timeout time instead of using context timeout
		timeoutAt := now.Add(time.Duration(task.MaxRuntime) * time.Second)

		// Add to wait group before launching goroutine
		wg.Add(1)

		// Handle the task in a goroutine
		go func(t *domain.Task, timeout time.Time) {
			defer wg.Done() // Signal completion when goroutine finishes

			execCtx, execSpan := tracing.StartServiceSpan(ctx, "TaskService", "executeTaskDirectly")

			// Set task attributes
			tracing.AddAttribute(execCtx, "task_id", t.ID)
			tracing.AddAttribute(execCtx, "workspace_id", t.WorkspaceID)
			tracing.AddAttribute(execCtx, "task_type", t.Type)
			tracing.AddAttribute(execCtx, "timeout_at", timeout.Format(time.RFC3339))
			if t.BroadcastID != nil {
				tracing.AddAttribute(execCtx, "broadcast_id", *t.BroadcastID)
			}

			// Ensure we clean up and handle timeout
			defer func() {
				tracing.EndSpan(execSpan, nil)
			}()

			if err := s.ExecuteTask(execCtx, t.WorkspaceID, t.ID, timeout); err != nil {
				tracing.MarkSpanError(execCtx, err)
				s.logger.WithField("task_id", t.ID).
					WithField("workspace_id", t.WorkspaceID).
					WithField("error", err.Error()).
					Error("Failed to execute task")
			}
		}(task, timeoutAt)
	}

	// Wait for all goroutines to complete
	wg.Wait()

	return nil
}

// ExecuteTask executes a specific task
func (s *TaskService) ExecuteTask(ctx context.Context, workspace, taskID string, timeoutAt time.Time) error {
	ctx, span := tracing.StartServiceSpan(ctx, "TaskService", "ExecuteTask")
	defer tracing.EndSpan(span, nil)

	tracing.AddAttribute(ctx, "workspace_id", workspace)
	tracing.AddAttribute(ctx, "task_id", taskID)

	// First check if the context is already cancelled
	if ctx.Err() != nil {
		tracing.MarkSpanError(ctx, ctx.Err())
		return ctx.Err()
	}

	// Get the task
	var task *domain.Task
	var processor domain.TaskProcessor

	// Wrap the initial setup operations in a transaction
	err := s.WithTransaction(ctx, func(tx *sql.Tx) error {
		txCtx, txSpan := tracing.StartServiceSpan(ctx, "TaskService", "ExecuteTaskTransaction")
		defer tracing.EndSpan(txSpan, nil)

		var taskErr error
		task, taskErr = s.repo.GetTx(txCtx, tx, workspace, taskID)
		if taskErr != nil {
			tracing.MarkSpanError(txCtx, taskErr)
			s.logger.WithFields(map[string]interface{}{
				"task_id":      taskID,
				"workspace_id": workspace,
				"error":        taskErr.Error(),
			}).Error("Failed to get task for execution")
			return &domain.ErrNotFound{
				Entity: "task",
				ID:     taskID,
			}
		}

		if task != nil {
			tracing.AddAttribute(txCtx, "task_type", task.Type)
			if task.BroadcastID != nil {
				tracing.AddAttribute(txCtx, "broadcast_id", *task.BroadcastID)
			}
		}

		// Get the processor for this task type
		var procErr error
		processor, procErr = s.GetProcessor(task.Type)
		if procErr != nil {
			tracing.MarkSpanError(txCtx, procErr)
			s.logger.WithFields(map[string]interface{}{
				"task_id":      taskID,
				"workspace_id": workspace,
				"task_type":    task.Type,
				"error":        procErr.Error(),
			}).Error("Failed to get processor for task type")
			return &domain.ErrTaskExecution{
				TaskID: taskID,
				Reason: "no processor registered for task type",
				Err:    procErr,
			}
		}

		// Use the passed timeoutAt parameter
		tracing.AddAttribute(txCtx, "timeout_at", timeoutAt.Format(time.RFC3339))

		// Mark task as running within the same transaction
		if markErr := s.repo.MarkAsRunningTx(txCtx, tx, workspace, taskID, timeoutAt); markErr != nil {
			tracing.MarkSpanError(txCtx, markErr)
			s.logger.WithFields(map[string]interface{}{
				"task_id":      taskID,
				"workspace_id": workspace,
				"error":        markErr.Error(),
			}).Error("Failed to mark task as running")
			return &domain.ErrTaskExecution{
				TaskID: taskID,
				Reason: "failed to mark task as running",
				Err:    markErr,
			}
		}

		// Store timeoutAt for later use in processor
		task.TimeoutAfter = &timeoutAt

		return nil
	})

	if err != nil {
		tracing.MarkSpanError(ctx, err)
		return err
	}

	// For non-parallel tasks, use the standard execution flow
	// Set up completion channel and context handling
	done := make(chan bool, 1)
	processErr := make(chan error, 1)
	bgCtx := context.Background()

	// Process the task in a goroutine
	go func() {
		procCtx, procSpan := tracing.StartServiceSpan(ctx, "TaskService", "ProcessTask")
		defer tracing.EndSpan(procSpan, nil)

		tracing.AddAttribute(procCtx, "task_id", taskID)
		tracing.AddAttribute(procCtx, "workspace_id", workspace)
		tracing.AddAttribute(procCtx, "task_type", task.Type)
		if task.BroadcastID != nil {
			tracing.AddAttribute(procCtx, "broadcast_id", *task.BroadcastID)
		}

		// Check if context was cancelled before we even start
		if procCtx.Err() != nil {
			tracing.MarkSpanError(procCtx, procCtx.Err())
			processErr <- procCtx.Err()
			return
		}

		// Track task execution time
		startTime := time.Now()

		// Call the processor
		completed, err := processor.Process(procCtx, task, *task.TimeoutAfter)

		// Calculate elapsed time
		elapsed := time.Since(startTime)
		tracing.AddAttribute(procCtx, "elapsed_time_ms", elapsed.Milliseconds())
		tracing.AddAttribute(procCtx, "task_completed", completed)

		if err != nil {
			tracing.MarkSpanError(procCtx, err)
			s.logger.WithFields(map[string]interface{}{
				"task_id":      taskID,
				"workspace_id": workspace,
				"elapsed_time": elapsed,
				"error":        err.Error(),
			}).Error("Task processing failed")
			processErr <- &domain.ErrTaskExecution{
				TaskID: taskID,
				Reason: "processing failed",
				Err:    err,
			}
			return
		}

		done <- completed
	}()

	// Wait for completion, error, or timeout
	select {
	case completed := <-done:
		if completed {
			// Check if this is a recurring task
			if task.IsRecurring() {
				// Recurring task: reschedule instead of completing
				rescheduleCtx, rescheduleSpan := tracing.StartServiceSpan(ctx, "TaskService", "RescheduleRecurringTask")
				defer tracing.EndSpan(rescheduleSpan, nil)

				tracing.AddAttribute(rescheduleCtx, "task_id", taskID)
				tracing.AddAttribute(rescheduleCtx, "workspace_id", workspace)
				tracing.AddAttribute(rescheduleCtx, "recurring_interval", *task.RecurringInterval)

				// Calculate next run with backoff and jitter
				interval := *task.RecurringInterval
				if task.State != nil && task.State.IntegrationSync != nil {
					if task.State.IntegrationSync.ConsecErrors > 0 {
						// Quadratic backoff: errors^2 * 10 seconds, capped at 1 hour
						backoff := int64(task.State.IntegrationSync.ConsecErrors * task.State.IntegrationSync.ConsecErrors * 10)
						if backoff > 3600 {
							backoff = 3600
						}
						interval += backoff
						tracing.AddAttribute(rescheduleCtx, "backoff_seconds", backoff)
					}
				}

				// Add jitter (10% of interval) to prevent thundering herd
				jitter := time.Duration(rand.Int63n(interval/10+1)) * time.Second
				nextRun := time.Now().UTC().Add(time.Duration(interval)*time.Second + jitter)

				tracing.AddAttribute(rescheduleCtx, "next_run", nextRun.Format(time.RFC3339))

				if err := s.repo.MarkAsPending(bgCtx, workspace, taskID, nextRun, 0, task.State); err != nil {
					if errors.Is(err, domain.ErrTaskNotFound) {
						// Task was deleted during execution, this is fine
						s.logger.WithFields(map[string]interface{}{
							"task_id":      taskID,
							"workspace_id": workspace,
						}).Info("Recurring task deleted during execution")
						return nil
					}
					tracing.MarkSpanError(rescheduleCtx, err)
					s.logger.WithFields(map[string]interface{}{
						"task_id":      taskID,
						"workspace_id": workspace,
						"error":        err.Error(),
					}).Error("Failed to reschedule recurring task")
					return &domain.ErrTaskExecution{
						TaskID: taskID,
						Reason: "failed to reschedule recurring task",
						Err:    err,
					}
				}
				s.logger.WithFields(map[string]interface{}{
					"task_id":      taskID,
					"workspace_id": workspace,
					"next_run":     nextRun,
					"interval":     interval,
				}).Info("Recurring task rescheduled")
			} else {
				// Non-recurring task: mark as completed
				completeCtx, completeSpan := tracing.StartServiceSpan(ctx, "TaskService", "MarkTaskCompleted")
				defer tracing.EndSpan(completeSpan, nil)

				tracing.AddAttribute(completeCtx, "task_id", taskID)
				tracing.AddAttribute(completeCtx, "workspace_id", workspace)

				if err := s.repo.MarkAsCompleted(bgCtx, workspace, taskID, task.State); err != nil {
					tracing.MarkSpanError(completeCtx, err)
					s.logger.WithFields(map[string]interface{}{
						"task_id":      taskID,
						"workspace_id": workspace,
						"error":        err.Error(),
					}).Error("Failed to mark task as completed")
					return &domain.ErrTaskExecution{
						TaskID: taskID,
						Reason: "failed to mark task as completed",
						Err:    err,
					}
				}
				s.logger.WithFields(map[string]interface{}{
					"task_id":      taskID,
					"workspace_id": workspace,
				}).Info("Task completed successfully")
			}
		} else {
			// Mark task as pending for next run
			pendingCtx, pendingSpan := tracing.StartServiceSpan(ctx, "TaskService", "MarkTaskPending")
			defer tracing.EndSpan(pendingSpan, nil)

			tracing.AddAttribute(pendingCtx, "task_id", taskID)
			tracing.AddAttribute(pendingCtx, "workspace_id", workspace)

			nextRun := time.Now().UTC()
			tracing.AddAttribute(pendingCtx, "next_run", nextRun.Format(time.RFC3339))
			tracing.AddAttribute(pendingCtx, "progress", task.Progress)

			if err := s.repo.MarkAsPending(bgCtx, task.WorkspaceID, task.ID, nextRun, task.Progress, task.State); err != nil {
				tracing.MarkSpanError(pendingCtx, err)
				s.logger.WithFields(map[string]interface{}{
					"task_id":      taskID,
					"workspace_id": workspace,
					"error":        err.Error(),
				}).Error("Failed to mark task as pending")
				return &domain.ErrTaskExecution{
					TaskID: taskID,
					Reason: "failed to mark task as pending",
					Err:    err,
				}
			}
			s.logger.WithFields(map[string]interface{}{
				"task_id":      taskID,
				"workspace_id": workspace,
				"next_run":     nextRun,
			}).Info("Task pending and will continue in next run")
		}
	case err := <-processErr:
		// Task failed with an error
		failCtx, failSpan := tracing.StartServiceSpan(ctx, "TaskService", "MarkTaskFailed")
		defer tracing.EndSpan(failSpan, nil)

		tracing.AddAttribute(failCtx, "task_id", taskID)
		tracing.AddAttribute(failCtx, "workspace_id", workspace)
		tracing.AddAttribute(failCtx, "error", err.Error())

		if markErr := s.repo.MarkAsFailed(bgCtx, workspace, taskID, err.Error()); markErr != nil {
			tracing.MarkSpanError(failCtx, markErr)
			s.logger.WithFields(map[string]interface{}{
				"task_id":      taskID,
				"workspace_id": workspace,
				"error":        markErr.Error(),
				"process_err":  err.Error(),
			}).Error("Failed to mark task as failed")
			return fmt.Errorf("failed to mark task as failed: %w", markErr)
		}
		return err
	}

	return nil
}

// GetLastCronRun retrieves the last cron execution timestamp
func (s *TaskService) GetLastCronRun(ctx context.Context) (*time.Time, error) {
	ctx, span := tracing.StartServiceSpan(ctx, "TaskService", "GetLastCronRun")
	defer tracing.EndSpan(span, nil)

	lastRun, err := s.settingRepo.GetLastCronRun(ctx)
	if err != nil {
		tracing.MarkSpanError(ctx, err)
		s.logger.WithField("error", err.Error()).Error("Failed to get last cron run")
		return nil, err
	}

	if lastRun != nil {
		tracing.AddAttribute(ctx, "last_run", lastRun.Format(time.RFC3339))
	}

	return lastRun, nil
}

// SubscribeToBroadcastEvents registers handlers for broadcast-related events
func (s *TaskService) SubscribeToBroadcastEvents(eventBus domain.EventBus) {
	// Subscribe to broadcast events
	eventBus.Subscribe(domain.EventBroadcastScheduled, s.handleBroadcastScheduled)
	eventBus.Subscribe(domain.EventBroadcastPaused, s.handleBroadcastPaused)
	eventBus.Subscribe(domain.EventBroadcastResumed, s.handleBroadcastResumed)
	eventBus.Subscribe(domain.EventBroadcastSent, s.handleBroadcastSent)
	eventBus.Subscribe(domain.EventBroadcastFailed, s.handleBroadcastFailed)
	eventBus.Subscribe(domain.EventBroadcastCancelled, s.handleBroadcastCancelled)

	s.logger.Info("TaskService subscribed to broadcast events")
}

// Event handlers for broadcast events
func (s *TaskService) handleBroadcastScheduled(ctx context.Context, payload domain.EventPayload) {
	ctx, span := tracing.StartServiceSpan(ctx, "TaskService", "handleBroadcastScheduled")
	defer tracing.EndSpan(span, nil)

	broadcastID := payload.EntityID
	tracing.AddAttribute(ctx, "broadcast_id", broadcastID)
	tracing.AddAttribute(ctx, "workspace_id", payload.WorkspaceID)
	tracing.AddAttribute(ctx, "event_type", payload.Type)

	s.logger.WithFields(map[string]interface{}{
		"broadcast_id": broadcastID,
		"workspace_id": payload.WorkspaceID,
	}).Info("Handling broadcast scheduled event")

	// Extract payload data before transaction (needed after commit for immediate execution)
	sendNow, _ := payload.Data["send_now"].(bool)
	status, _ := payload.Data["status"].(string)

	// Track whether we should trigger immediate execution after commit
	shouldExecuteImmediately := false

	// Use a transaction for checking and potentially updating/creating task
	err := s.WithTransaction(ctx, func(tx *sql.Tx) error {
		txCtx, txSpan := tracing.StartServiceSpan(ctx, "TaskService", "BroadcastScheduledTransaction")
		defer tracing.EndSpan(txSpan, nil)

		tracing.AddAttribute(txCtx, "send_now", sendNow)
		tracing.AddAttribute(txCtx, "broadcast_status", status)

		// Try to find the task for this broadcast ID directly
		existingTask, err := s.repo.GetTaskByBroadcastID(txCtx, payload.WorkspaceID, broadcastID)
		if err != nil {
			// If no task exists, we'll create one later
			tracing.AddAttribute(txCtx, "task_exists", false)
			s.logger.WithField("broadcast_id", broadcastID).
				WithField("error", err.Error()).
				Debug("No existing task found for broadcast, will create new one")
		}

		// Update existing task if found
		if existingTask != nil {
			tracing.AddAttribute(txCtx, "task_exists", true)
			tracing.AddAttribute(txCtx, "task_id", existingTask.ID)
			tracing.AddAttribute(txCtx, "current_status", string(existingTask.Status))

			s.logger.WithFields(map[string]interface{}{
				"broadcast_id": broadcastID,
				"task_id":      existingTask.ID,
			}).Info("Task already exists for broadcast, updating status")

			if sendNow && status == string(domain.BroadcastStatusProcessing) {
				// If broadcast is being sent immediately, mark task as pending and set next run to now
				nextRunAfter := time.Now()
				existingTask.NextRunAfter = &nextRunAfter
				existingTask.Status = domain.TaskStatusPending

				// Ensure BroadcastID is set
				if existingTask.BroadcastID == nil {
					broadcastIDCopy := broadcastID
					existingTask.BroadcastID = &broadcastIDCopy
				}

				if updateErr := s.repo.Update(txCtx, payload.WorkspaceID, existingTask); updateErr != nil {
					tracing.MarkSpanError(txCtx, updateErr)
					s.logger.WithFields(map[string]interface{}{
						"broadcast_id": broadcastID,
						"task_id":      existingTask.ID,
						"error":        updateErr.Error(),
					}).Error("Failed to update task for scheduled broadcast")
					return updateErr
				}

				// Flag for immediate execution after transaction commits
				shouldExecuteImmediately = true
			}

			return nil
		}

		// If no task exists, create one
		s.logger.WithField("broadcast_id", broadcastID).Info("Creating new task for scheduled broadcast")

		tracing.AddAttribute(txCtx, "creating_new_task", true)

		// Create a copy of the broadcast ID for the pointer
		broadcastIDCopy := broadcastID

		task := &domain.Task{
			WorkspaceID: payload.WorkspaceID,
			Type:        "send_broadcast",
			Status:      domain.TaskStatusPending,
			BroadcastID: &broadcastIDCopy,
			State: &domain.TaskState{
				Progress: 0,
				Message:  "Starting broadcast",
				SendBroadcast: &domain.SendBroadcastState{
					BroadcastID:     broadcastID,
					ChannelType:     "email",
					EnqueuedCount:   0,
					FailedCount:     0,
					RecipientOffset: 0,
				},
			},
			MaxRuntime:    50, // 50 seconds
			MaxRetries:    3,
			RetryInterval: 300, // 5 minutes
		}

		// If the broadcast is set to send immediately, we don't need to set NextRunAfter
		// If it's scheduled for the future, we should set NextRunAfter based on the schedule
		if !sendNow && status == string(domain.BroadcastStatusScheduled) {

			// Get broadcast schedule info from payload
			if scheduledTimeStr, hasTime := payload.Data["scheduled_time"].(string); hasTime {

				// Parse the scheduled time string
				if scheduledTime, parseErr := time.Parse(time.RFC3339, scheduledTimeStr); parseErr == nil {
					// Use the actual scheduled time from the broadcast
					task.NextRunAfter = &scheduledTime
					tracing.AddAttribute(txCtx, "next_run_after", scheduledTime.Format(time.RFC3339))
					tracing.AddAttribute(txCtx, "scheduled_time_source", "payload")
				} else {
					log.Printf("Failed to parse scheduled_time: %v", parseErr)
				}
			}
		}

		if createErr := s.CreateTask(txCtx, payload.WorkspaceID, task); createErr != nil {
			tracing.MarkSpanError(txCtx, createErr)
			s.logger.WithFields(map[string]interface{}{
				"broadcast_id": broadcastID,
				"error":        createErr.Error(),
			}).Error("Failed to create task for scheduled broadcast")
			return createErr
		}

		s.logger.WithFields(map[string]interface{}{
			"broadcast_id": broadcastID,
			"workspace_id": payload.WorkspaceID,
		}).Info("Successfully created task for scheduled broadcast")

		// Flag for immediate execution if this is a send-now broadcast
		if sendNow && status == string(domain.BroadcastStatusProcessing) {
			shouldExecuteImmediately = true
		}

		return nil
	})

	if err != nil {
		tracing.MarkSpanError(ctx, err)
		s.logger.WithFields(map[string]interface{}{
			"broadcast_id": broadcastID,
			"workspace_id": payload.WorkspaceID,
			"error":        err.Error(),
		}).Error("Failed to handle broadcast scheduled event")
		return
	}

	// Trigger immediate task execution after transaction commits (no sleep needed)
	if shouldExecuteImmediately && s.autoExecuteImmediate {
		go func() {
			if execErr := s.ExecutePendingTasks(context.Background(), 1); execErr != nil {
				s.logger.WithFields(map[string]interface{}{
					"broadcast_id": broadcastID,
					"error":        execErr.Error(),
				}).Error("Failed to trigger immediate task execution")
			}
		}()
	}
}

func (s *TaskService) handleBroadcastPaused(ctx context.Context, payload domain.EventPayload) {
	ctx, span := tracing.StartServiceSpan(ctx, "TaskService", "handleBroadcastPaused")
	defer tracing.EndSpan(span, nil)

	tracing.AddAttribute(ctx, "workspace_id", payload.WorkspaceID)
	tracing.AddAttribute(ctx, "event_type", payload.Type)

	broadcastID, ok := payload.Data["broadcast_id"].(string)
	if !ok || broadcastID == "" {
		err := fmt.Errorf("missing or invalid broadcast_id")
		tracing.MarkSpanError(ctx, err)
		s.logger.Error("Failed to handle broadcast paused event: missing or invalid broadcast_id")
		return
	}

	tracing.AddAttribute(ctx, "broadcast_id", broadcastID)

	s.logger.WithFields(map[string]interface{}{
		"broadcast_id": broadcastID,
		"workspace_id": payload.WorkspaceID,
	}).Info("Handling broadcast paused event")

	// Find associated task by broadcast ID
	task, err := s.repo.GetTaskByBroadcastID(ctx, payload.WorkspaceID, broadcastID)
	if err != nil {
		tracing.MarkSpanError(ctx, err)
		s.logger.WithField("error", err.Error()).Debug("No task found for paused broadcast")
		return
	}

	tracing.AddAttribute(ctx, "task_id", task.ID)
	tracing.AddAttribute(ctx, "current_status", string(task.Status))

	// Pause the task
	nextRunAfter := time.Now().Add(24 * time.Hour) // Pause for 24 hours
	tracing.AddAttribute(ctx, "next_run_after", nextRunAfter.Format(time.RFC3339))

	if err := s.repo.MarkAsPaused(ctx, payload.WorkspaceID, task.ID, nextRunAfter, task.Progress, task.State); err != nil {
		tracing.MarkSpanError(ctx, err)
		s.logger.WithFields(map[string]interface{}{
			"broadcast_id": broadcastID,
			"task_id":      task.ID,
			"error":        err.Error(),
		}).Error("Failed to pause task for paused broadcast")
	} else {
		s.logger.WithFields(map[string]interface{}{
			"broadcast_id": broadcastID,
			"task_id":      task.ID,
		}).Info("Successfully paused task for paused broadcast")
	}
}

func (s *TaskService) handleBroadcastResumed(ctx context.Context, payload domain.EventPayload) {
	ctx, span := tracing.StartServiceSpan(ctx, "TaskService", "handleBroadcastResumed")
	defer tracing.EndSpan(span, nil)

	tracing.AddAttribute(ctx, "workspace_id", payload.WorkspaceID)
	tracing.AddAttribute(ctx, "event_type", payload.Type)

	broadcastID, ok := payload.Data["broadcast_id"].(string)
	if !ok || broadcastID == "" {
		err := fmt.Errorf("missing or invalid broadcast_id")
		tracing.MarkSpanError(ctx, err)
		s.logger.Error("Failed to handle broadcast resumed event: missing or invalid broadcast_id")
		return
	}

	tracing.AddAttribute(ctx, "broadcast_id", broadcastID)

	s.logger.WithFields(map[string]interface{}{
		"broadcast_id": broadcastID,
		"workspace_id": payload.WorkspaceID,
	}).Info("Handling broadcast resumed event")

	// Find associated task by broadcast ID
	task, err := s.repo.GetTaskByBroadcastID(ctx, payload.WorkspaceID, broadcastID)
	if err != nil {
		tracing.MarkSpanError(ctx, err)
		s.logger.WithField("error", err.Error()).Debug("No task found for resumed broadcast")
		return
	}

	tracing.AddAttribute(ctx, "task_id", task.ID)
	tracing.AddAttribute(ctx, "current_status", string(task.Status))

	// Resume the task
	nextRunAfter := time.Now().UTC()
	task.NextRunAfter = &nextRunAfter
	task.Status = domain.TaskStatusPending

	tracing.AddAttribute(ctx, "next_run_after", nextRunAfter.Format(time.RFC3339))
	tracing.AddAttribute(ctx, "new_status", string(task.Status))

	if err := s.repo.Update(ctx, payload.WorkspaceID, task); err != nil {
		tracing.MarkSpanError(ctx, err)
		s.logger.WithFields(map[string]interface{}{
			"broadcast_id": broadcastID,
			"task_id":      task.ID,
			"error":        err.Error(),
		}).Error("Failed to resume task for resumed broadcast")
	} else {
		s.logger.WithFields(map[string]interface{}{
			"broadcast_id": broadcastID,
			"task_id":      task.ID,
		}).Info("Successfully resumed task for resumed broadcast")

		// Check if broadcast should start immediately
		startNow, _ := payload.Data["start_now"].(bool)
		if startNow && s.autoExecuteImmediate {
			// Immediately trigger task execution
			go func() {
				// Small delay to ensure transaction is committed
				time.Sleep(100 * time.Millisecond)
				if execErr := s.ExecutePendingTasks(context.Background(), 1); execErr != nil {
					s.logger.WithFields(map[string]interface{}{
						"broadcast_id": broadcastID,
						"task_id":      task.ID,
						"error":        execErr.Error(),
					}).Error("Failed to trigger immediate task execution for resumed broadcast")
				}
			}()
		}
	}
}

func (s *TaskService) handleBroadcastSent(ctx context.Context, payload domain.EventPayload) {
	ctx, span := tracing.StartServiceSpan(ctx, "TaskService", "handleBroadcastSent")
	defer tracing.EndSpan(span, nil)

	tracing.AddAttribute(ctx, "workspace_id", payload.WorkspaceID)
	tracing.AddAttribute(ctx, "event_type", payload.Type)

	broadcastID, ok := payload.Data["broadcast_id"].(string)
	if !ok || broadcastID == "" {
		err := fmt.Errorf("missing or invalid broadcast_id")
		tracing.MarkSpanError(ctx, err)
		s.logger.Error("Failed to handle broadcast sent event: missing or invalid broadcast_id")
		return
	}

	tracing.AddAttribute(ctx, "broadcast_id", broadcastID)

	s.logger.WithFields(map[string]interface{}{
		"broadcast_id": broadcastID,
		"workspace_id": payload.WorkspaceID,
	}).Info("Handling broadcast sent event")

	// Find associated task by broadcast ID
	task, err := s.repo.GetTaskByBroadcastID(ctx, payload.WorkspaceID, broadcastID)
	if err != nil {
		tracing.MarkSpanError(ctx, err)
		s.logger.WithField("error", err.Error()).Debug("No task found for sent broadcast")
		return
	}

	tracing.AddAttribute(ctx, "task_id", task.ID)
	tracing.AddAttribute(ctx, "current_status", string(task.Status))

	// Mark the task as completed
	if err := s.repo.MarkAsCompleted(ctx, payload.WorkspaceID, task.ID, task.State); err != nil {
		tracing.MarkSpanError(ctx, err)
		s.logger.WithFields(map[string]interface{}{
			"broadcast_id": broadcastID,
			"task_id":      task.ID,
			"error":        err.Error(),
		}).Error("Failed to complete task for sent broadcast")
	} else {
		s.logger.WithFields(map[string]interface{}{
			"broadcast_id": broadcastID,
			"task_id":      task.ID,
		}).Info("Successfully completed task for sent broadcast")
	}
}

func (s *TaskService) handleBroadcastFailed(ctx context.Context, payload domain.EventPayload) {
	ctx, span := tracing.StartServiceSpan(ctx, "TaskService", "handleBroadcastFailed")
	defer tracing.EndSpan(span, nil)

	tracing.AddAttribute(ctx, "workspace_id", payload.WorkspaceID)
	tracing.AddAttribute(ctx, "event_type", payload.Type)

	broadcastID, ok := payload.Data["broadcast_id"].(string)
	if !ok || broadcastID == "" {
		err := fmt.Errorf("missing or invalid broadcast_id")
		tracing.MarkSpanError(ctx, err)
		s.logger.Error("Failed to handle broadcast failed event: missing or invalid broadcast_id")
		return
	}

	tracing.AddAttribute(ctx, "broadcast_id", broadcastID)

	reason, _ := payload.Data["reason"].(string)
	if reason == "" {
		reason = "Broadcast failed"
	}

	tracing.AddAttribute(ctx, "failure_reason", reason)

	s.logger.WithFields(map[string]interface{}{
		"broadcast_id": broadcastID,
		"workspace_id": payload.WorkspaceID,
		"reason":       reason,
	}).Info("Handling broadcast failed event")

	// Find associated task by broadcast ID
	task, err := s.repo.GetTaskByBroadcastID(ctx, payload.WorkspaceID, broadcastID)
	if err != nil {
		tracing.MarkSpanError(ctx, err)
		s.logger.WithField("error", err.Error()).Debug("No task found for failed broadcast")
		return
	}

	tracing.AddAttribute(ctx, "task_id", task.ID)
	tracing.AddAttribute(ctx, "current_status", string(task.Status))

	// Mark the task as failed
	if err := s.repo.MarkAsFailed(ctx, payload.WorkspaceID, task.ID, reason); err != nil {
		tracing.MarkSpanError(ctx, err)
		s.logger.WithFields(map[string]interface{}{
			"broadcast_id": broadcastID,
			"task_id":      task.ID,
			"error":        err.Error(),
		}).Error("Failed to mark task as failed for failed broadcast")
	} else {
		s.logger.WithFields(map[string]interface{}{
			"broadcast_id": broadcastID,
			"task_id":      task.ID,
		}).Info("Successfully marked task as failed for failed broadcast")
	}
}

func (s *TaskService) handleBroadcastCancelled(ctx context.Context, payload domain.EventPayload) {
	ctx, span := tracing.StartServiceSpan(ctx, "TaskService", "handleBroadcastCancelled")
	defer tracing.EndSpan(span, nil)

	tracing.AddAttribute(ctx, "workspace_id", payload.WorkspaceID)
	tracing.AddAttribute(ctx, "event_type", payload.Type)

	broadcastID, ok := payload.Data["broadcast_id"].(string)
	if !ok || broadcastID == "" {
		err := fmt.Errorf("missing or invalid broadcast_id")
		tracing.MarkSpanError(ctx, err)
		s.logger.Error("Failed to handle broadcast cancelled event: missing or invalid broadcast_id")
		return
	}

	tracing.AddAttribute(ctx, "broadcast_id", broadcastID)

	s.logger.WithFields(map[string]interface{}{
		"broadcast_id": broadcastID,
		"workspace_id": payload.WorkspaceID,
	}).Info("Handling broadcast cancelled event")

	// Find associated task by broadcast ID
	task, err := s.repo.GetTaskByBroadcastID(ctx, payload.WorkspaceID, broadcastID)
	if err != nil {
		tracing.MarkSpanError(ctx, err)
		s.logger.WithField("error", err.Error()).Debug("No task found for cancelled broadcast")
		return
	}

	tracing.AddAttribute(ctx, "task_id", task.ID)
	tracing.AddAttribute(ctx, "current_status", string(task.Status))

	// Mark the task as failed with cancellation reason
	cancelReason := "Broadcast was cancelled"
	tracing.AddAttribute(ctx, "cancel_reason", cancelReason)

	if err := s.repo.MarkAsFailed(ctx, payload.WorkspaceID, task.ID, cancelReason); err != nil {
		tracing.MarkSpanError(ctx, err)
		s.logger.WithFields(map[string]interface{}{
			"broadcast_id": broadcastID,
			"task_id":      task.ID,
			"error":        err.Error(),
		}).Error("Failed to mark task as failed for cancelled broadcast")
	} else {
		s.logger.WithFields(map[string]interface{}{
			"broadcast_id": broadcastID,
			"task_id":      task.ID,
		}).Info("Successfully marked task as failed for cancelled broadcast")
	}
}

// ResetTask resets a failed recurring task, clearing error state and rescheduling for immediate execution
func (s *TaskService) ResetTask(ctx context.Context, workspace, taskID string) error {
	ctx, span := tracing.StartServiceSpan(ctx, "TaskService", "ResetTask")
	defer func() { tracing.EndSpan(span, nil) }()

	tracing.AddAttribute(ctx, "task_id", taskID)
	tracing.AddAttribute(ctx, "workspace_id", workspace)

	// Get the task
	task, err := s.repo.Get(ctx, workspace, taskID)
	if err != nil {
		tracing.MarkSpanError(ctx, err)
		return domain.ErrTaskNotFound
	}

	// Verify task is in failed state
	if task.Status != domain.TaskStatusFailed {
		return fmt.Errorf("task is not in failed state, current status: %s", task.Status)
	}

	// Verify task is recurring
	if !task.IsRecurring() {
		return fmt.Errorf("task is not a recurring task")
	}

	// Reset error state in IntegrationSync
	if task.State != nil && task.State.IntegrationSync != nil {
		task.State.IntegrationSync.ConsecErrors = 0
		task.State.IntegrationSync.LastError = nil
		task.State.IntegrationSync.LastErrorType = ""
	}

	// Schedule for immediate execution
	nextRun := time.Now().UTC()

	if err := s.repo.MarkAsPending(ctx, workspace, taskID, nextRun, 0, task.State); err != nil {
		tracing.MarkSpanError(ctx, err)
		s.logger.WithFields(map[string]interface{}{
			"task_id":      taskID,
			"workspace_id": workspace,
			"error":        err.Error(),
		}).Error("Failed to reset task")
		return fmt.Errorf("failed to reset task: %w", err)
	}

	s.logger.WithFields(map[string]interface{}{
		"task_id":      taskID,
		"workspace_id": workspace,
	}).Info("Task reset successfully")

	return nil
}

// TriggerTask triggers an immediate execution of a recurring task
func (s *TaskService) TriggerTask(ctx context.Context, workspace, taskID string) error {
	ctx, span := tracing.StartServiceSpan(ctx, "TaskService", "TriggerTask")
	defer func() { tracing.EndSpan(span, nil) }()

	tracing.AddAttribute(ctx, "task_id", taskID)
	tracing.AddAttribute(ctx, "workspace_id", workspace)

	// Get the task
	task, err := s.repo.Get(ctx, workspace, taskID)
	if err != nil {
		tracing.MarkSpanError(ctx, err)
		return domain.ErrTaskNotFound
	}

	// Verify task is recurring
	if !task.IsRecurring() {
		return fmt.Errorf("task is not a recurring task")
	}

	// Check if task is already running
	if task.Status == domain.TaskStatusRunning {
		return &domain.ErrTaskAlreadyRunning{TaskID: taskID}
	}

	// Check if task is in a state that can be triggered
	if task.Status != domain.TaskStatusPending && task.Status != domain.TaskStatusPaused {
		return fmt.Errorf("task cannot be triggered in current status: %s", task.Status)
	}

	// Schedule for immediate execution
	nextRun := time.Now().UTC()

	if err := s.repo.MarkAsPending(ctx, workspace, taskID, nextRun, task.Progress, task.State); err != nil {
		tracing.MarkSpanError(ctx, err)
		s.logger.WithFields(map[string]interface{}{
			"task_id":      taskID,
			"workspace_id": workspace,
			"error":        err.Error(),
		}).Error("Failed to trigger task")
		return fmt.Errorf("failed to trigger task: %w", err)
	}

	s.logger.WithFields(map[string]interface{}{
		"task_id":      taskID,
		"workspace_id": workspace,
	}).Info("Task triggered for immediate execution")

	return nil
}
