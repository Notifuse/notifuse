package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/http/middleware"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// TaskHandler handles HTTP requests related to tasks
type TaskHandler struct {
	taskService  domain.TaskService
	getJWTSecret func() ([]byte, error)
	logger       logger.Logger
	secretKey    string
}

// NewTaskHandler creates a new task handler
func NewTaskHandler(
	taskService domain.TaskService,
	getJWTSecret func() ([]byte, error),
	logger logger.Logger,
	secretKey string,
) *TaskHandler {
	return &TaskHandler{
		taskService:  taskService,
		getJWTSecret: getJWTSecret,
		logger:       logger,
		secretKey:    secretKey,
	}
}

// RegisterRoutes registers the task-related routes
func (h *TaskHandler) RegisterRoutes(mux *http.ServeMux) {
	// Create auth middleware
	authMiddleware := middleware.NewAuthMiddleware(h.getJWTSecret)
	requireAuth := authMiddleware.RequireAuth()

	// Register RPC-style endpoints with dot notation
	mux.Handle("/api/tasks.create", requireAuth(http.HandlerFunc(h.CreateTask)))
	mux.Handle("/api/tasks.list", requireAuth(http.HandlerFunc(h.ListTasks)))
	mux.Handle("/api/tasks.get", requireAuth(http.HandlerFunc(h.GetTask)))
	mux.Handle("/api/tasks.delete", requireAuth(http.HandlerFunc(h.DeleteTask)))
	mux.Handle("/api/tasks.reset", requireAuth(http.HandlerFunc(h.ResetTask)))
	mux.Handle("/api/tasks.trigger", requireAuth(http.HandlerFunc(h.TriggerTask)))
	// public routes for external systems to trigger task execution
	mux.Handle("/api/tasks.execute", http.HandlerFunc(h.ExecuteTask))
	mux.Handle("/api/cron", http.HandlerFunc(h.ExecutePendingTasks))
	mux.Handle("/api/cron.status", http.HandlerFunc(h.GetCronStatus))
}

// CreateTask handles creation of a new task
func (h *TaskHandler) CreateTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var createRequest domain.CreateTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&createRequest); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	task, err := createRequest.Validate()
	if err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.taskService.CreateTask(r.Context(), createRequest.WorkspaceID, task); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to create task")
		WriteJSONError(w, "Failed to create task", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"task": task,
	})
}

// GetTask handles retrieval of a task by ID
func (h *TaskHandler) GetTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var getRequest domain.GetTaskRequest
	if err := getRequest.FromURLParams(r.URL.Query()); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	task, err := h.taskService.GetTask(r.Context(), getRequest.WorkspaceID, getRequest.ID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			WriteJSONError(w, "Task not found", http.StatusNotFound)
		} else {
			h.logger.WithField("error", err.Error()).Error("Failed to get task")
			WriteJSONError(w, "Failed to get task", http.StatusInternalServerError)
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"task": task,
	})
}

// ListTasks handles listing tasks with optional filtering
func (h *TaskHandler) ListTasks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var listRequest domain.ListTasksRequest
	if err := listRequest.FromURLParams(r.URL.Query()); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	filter := listRequest.ToFilter()

	response, err := h.taskService.ListTasks(r.Context(), listRequest.WorkspaceID, filter)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to list tasks")
		WriteJSONError(w, "Failed to list tasks", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, response)
}

// DeleteTask handles deletion of a task
func (h *TaskHandler) DeleteTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var deleteRequest domain.DeleteTaskRequest
	if err := deleteRequest.FromURLParams(r.URL.Query()); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.taskService.DeleteTask(r.Context(), deleteRequest.WorkspaceID, deleteRequest.ID); err != nil {
		if strings.Contains(err.Error(), "not found") {
			WriteJSONError(w, "Task not found", http.StatusNotFound)
		} else {
			h.logger.WithField("error", err.Error()).Error("Failed to delete task")
			WriteJSONError(w, "Failed to delete task", http.StatusInternalServerError)
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
	})
}

// ExecutePendingTasks handles the cron-triggered task execution
func (h *TaskHandler) ExecutePendingTasks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Log that manual trigger is being used (internal scheduler should handle this)
	h.logger.Info("Manual cron trigger via HTTP endpoint - internal scheduler should handle this automatically")

	startTime := time.Now()

	var executeRequest domain.ExecutePendingTasksRequest
	if err := executeRequest.FromURLParams(r.URL.Query()); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Execute tasks
	if err := h.taskService.ExecutePendingTasks(r.Context(), executeRequest.MaxTasks); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to execute tasks")
		WriteJSONError(w, "Failed to execute tasks", http.StatusInternalServerError)
		return
	}

	elapsed := time.Since(startTime)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":   true,
		"message":   "Task execution initiated",
		"max_tasks": executeRequest.MaxTasks,
		"elapsed":   elapsed.String(),
	})
}

// ExecuteTask handles execution of a single task
func (h *TaskHandler) ExecuteTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var executeRequest domain.ExecuteTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&executeRequest); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := executeRequest.Validate(); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Get the task to calculate timeout
	task, err := h.taskService.GetTask(r.Context(), executeRequest.WorkspaceID, executeRequest.ID)
	if err != nil {
		WriteJSONError(w, err.Error(), http.StatusNotFound)
		return
	}

	// Calculate timeout based on task's MaxRuntime
	timeoutAt := time.Now().Add(time.Duration(task.MaxRuntime) * time.Second)

	if err := h.taskService.ExecuteTask(r.Context(), executeRequest.WorkspaceID, executeRequest.ID, timeoutAt); err != nil {
		// Handle different error types with appropriate status codes
		switch e := err.(type) {
		case *domain.ErrNotFound:
			WriteJSONError(w, e.Error(), http.StatusNotFound)
		case *domain.ErrTaskAlreadyRunning:
			// Task is already being executed by another worker - this is expected in concurrent scenarios
			h.logger.WithFields(map[string]interface{}{
				"task_id":      executeRequest.ID,
				"workspace_id": executeRequest.WorkspaceID,
			}).Debug("Task already running, rejecting concurrent execution")
			WriteJSONError(w, e.Error(), http.StatusConflict)
		case *domain.ErrTaskExecution:
			// Check if this is an "already running" error - expected in concurrent environments
			if _, ok := e.Err.(*domain.ErrTaskAlreadyRunning); ok {
				h.logger.WithFields(map[string]interface{}{
					"task_id":      executeRequest.ID,
					"workspace_id": executeRequest.WorkspaceID,
				}).Debug("Task already running (wrapped), rejecting concurrent execution")
				WriteJSONError(w, "Task is already being processed", http.StatusConflict)
			} else if e.Reason == "failed to mark task as running" {
				// This happens when another executor already claimed the task
				h.logger.WithFields(map[string]interface{}{
					"task_id":      executeRequest.ID,
					"workspace_id": executeRequest.WorkspaceID,
				}).Debug("Task already claimed by another executor")
				WriteJSONError(w, "Task is already being processed", http.StatusConflict)
			} else if e.Reason == "no processor registered for task type" {
				h.logger.WithFields(map[string]interface{}{
					"task_id":      executeRequest.ID,
					"workspace_id": executeRequest.WorkspaceID,
					"error":        err.Error(),
				}).Warn("No processor registered for task type")
				WriteJSONError(w, "Unsupported task type", http.StatusBadRequest)
			} else {
				// Log genuine errors at ERROR level
				h.logger.WithFields(map[string]interface{}{
					"task_id":      executeRequest.ID,
					"workspace_id": executeRequest.WorkspaceID,
					"reason":       e.Reason,
					"error":        err.Error(),
				}).Error("Task execution failed")
				WriteJSONError(w, "Task execution failed: "+e.Reason, http.StatusInternalServerError)
			}
		case *domain.ErrTaskTimeout:
			WriteJSONError(w, e.Error(), http.StatusGatewayTimeout)
		default:
			h.logger.WithFields(map[string]interface{}{
				"task_id":      executeRequest.ID,
				"workspace_id": executeRequest.WorkspaceID,
				"error":        err.Error(),
			}).Error("Failed to execute task")
			WriteJSONError(w, "Failed to execute task", http.StatusInternalServerError)
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Task execution initiated",
	})
}

// GetCronStatus returns the last cron run timestamp from settings
func (h *TaskHandler) GetCronStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	lastRun, err := h.taskService.GetLastCronRun(r.Context())
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to get last cron run")
		WriteJSONError(w, "Failed to get cron status", http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"success": true,
	}

	if lastRun != nil {
		response["last_run"] = lastRun.Format(time.RFC3339)
		response["last_run_unix"] = lastRun.Unix()

		// Calculate time since last run
		timeSince := time.Since(*lastRun)
		response["time_since_last_run"] = timeSince.String()
		response["time_since_last_run_seconds"] = int64(timeSince.Seconds())
	} else {
		response["last_run"] = nil
		response["last_run_unix"] = nil
		response["time_since_last_run"] = nil
		response["time_since_last_run_seconds"] = nil
		response["message"] = "No cron run recorded yet"
	}

	writeJSON(w, http.StatusOK, response)
}

// ResetTask resets a failed recurring task, clearing error state and scheduling for immediate execution
func (h *TaskHandler) ResetTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.ResetTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := req.Validate(); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.taskService.ResetTask(r.Context(), req.WorkspaceID, req.ID); err != nil {
		if errors.Is(err, domain.ErrTaskNotFound) {
			WriteJSONError(w, "Task not found", http.StatusNotFound)
			return
		}
		h.logger.WithFields(map[string]interface{}{
			"task_id":      req.ID,
			"workspace_id": req.WorkspaceID,
			"error":        err.Error(),
		}).Error("Failed to reset task")
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true})
}

// TriggerTask triggers an immediate execution of a recurring task
func (h *TaskHandler) TriggerTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.TriggerTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := req.Validate(); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.taskService.TriggerTask(r.Context(), req.WorkspaceID, req.ID); err != nil {
		if errors.Is(err, domain.ErrTaskNotFound) {
			WriteJSONError(w, "Task not found", http.StatusNotFound)
			return
		}
		var alreadyRunningErr *domain.ErrTaskAlreadyRunning
		if errors.As(err, &alreadyRunningErr) {
			WriteJSONError(w, "Task is already running", http.StatusConflict)
			return
		}
		h.logger.WithFields(map[string]interface{}{
			"task_id":      req.ID,
			"workspace_id": req.WorkspaceID,
			"error":        err.Error(),
		}).Error("Failed to trigger task")
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true})
}
