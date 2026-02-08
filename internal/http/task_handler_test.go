package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"

	"github.com/stretchr/testify/assert"
)

func TestTaskHandler_ExecuteTask(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTaskService := mocks.NewMockTaskService(ctrl)
	// For tests we don't need the actual key, we can use a mock or nil since we're not validating auth
	var jwtSecret []byte
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Set up common logger expectations
	mockLoggerWithField := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLoggerWithField).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLoggerWithField).AnyTimes()
	mockLoggerWithField.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLoggerWithField.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockLoggerWithField.EXPECT().Debug(gomock.Any()).AnyTimes()

	secretKey := "test-secret-key"

	handler := NewTaskHandler(mockTaskService, func() ([]byte, error) { return jwtSecret, nil }, mockLogger, secretKey)

	t.Run("Successful execution", func(t *testing.T) {
		// Setup
		reqBody := domain.ExecuteTaskRequest{
			WorkspaceID: "workspace1",
			ID:          "task123",
		}

		reqJSON, _ := json.Marshal(reqBody)

		// Configure service mock to return success
		mockTaskService.EXPECT().
			GetTask(gomock.Any(), reqBody.WorkspaceID, reqBody.ID).
			Return(&domain.Task{MaxRuntime: 60}, nil)
		mockTaskService.EXPECT().
			ExecuteTask(gomock.Any(), reqBody.WorkspaceID, reqBody.ID, gomock.Any()).
			Return(nil)

		// Call handler
		req := httptest.NewRequest(http.MethodPost, "/api/tasks.execute", bytes.NewBuffer(reqJSON))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler.ExecuteTask(rec, req)

		// Verify response
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		err := json.NewDecoder(rec.Body).Decode(&resp)
		assert.NoError(t, err)
		assert.True(t, resp["success"].(bool))
	})

	t.Run("Method not allowed", func(t *testing.T) {
		// Call handler with wrong method
		req := httptest.NewRequest(http.MethodGet, "/api/tasks.execute", nil)
		rec := httptest.NewRecorder()

		handler.ExecuteTask(rec, req)

		// Verify response
		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	})

	t.Run("Invalid request body", func(t *testing.T) {
		// Call handler with invalid JSON
		req := httptest.NewRequest(http.MethodPost, "/api/tasks.execute", bytes.NewBuffer([]byte("invalid json")))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler.ExecuteTask(rec, req)

		// Verify response
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("Missing required fields", func(t *testing.T) {
		// Setup request with missing fields
		reqBody := map[string]interface{}{
			"WorkspaceID": "workspace1",
			// missing ID field
		}

		reqJSON, _ := json.Marshal(reqBody)

		// Call handler
		req := httptest.NewRequest(http.MethodPost, "/api/tasks.execute", bytes.NewBuffer(reqJSON))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler.ExecuteTask(rec, req)

		// Verify response
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("Task not found error", func(t *testing.T) {
		// Setup
		reqBody := domain.ExecuteTaskRequest{
			WorkspaceID: "workspace1",
			ID:          "task123",
		}

		reqJSON, _ := json.Marshal(reqBody)

		// Configure service mock to return a NotFound error
		notFoundErr := &domain.ErrNotFound{
			Entity: "task",
			ID:     reqBody.ID,
		}
		mockTaskService.EXPECT().
			GetTask(gomock.Any(), reqBody.WorkspaceID, reqBody.ID).
			Return(nil, notFoundErr)

		// Call handler
		req := httptest.NewRequest(http.MethodPost, "/api/tasks.execute", bytes.NewBuffer(reqJSON))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler.ExecuteTask(rec, req)

		// Verify response has correct status code for not found
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("Task execution error - unsupported task type", func(t *testing.T) {
		// Setup
		reqBody := domain.ExecuteTaskRequest{
			WorkspaceID: "workspace1",
			ID:          "task123",
		}

		reqJSON, _ := json.Marshal(reqBody)

		// Configure service mock to return a TaskExecution error for unsupported type
		execErr := &domain.ErrTaskExecution{
			TaskID: reqBody.ID,
			Reason: "no processor registered for task type",
			Err:    errors.New("unsupported_task_type"),
		}
		mockTaskService.EXPECT().
			GetTask(gomock.Any(), reqBody.WorkspaceID, reqBody.ID).
			Return(&domain.Task{MaxRuntime: 60}, nil)
		mockTaskService.EXPECT().
			ExecuteTask(gomock.Any(), reqBody.WorkspaceID, reqBody.ID, gomock.Any()).
			Return(execErr)

		// Call handler
		req := httptest.NewRequest(http.MethodPost, "/api/tasks.execute", bytes.NewBuffer(reqJSON))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler.ExecuteTask(rec, req)

		// Verify response has correct status code for bad request (client error)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("Task execution error - general error", func(t *testing.T) {
		// Setup
		reqBody := domain.ExecuteTaskRequest{
			WorkspaceID: "workspace1",
			ID:          "task123",
		}

		reqJSON, _ := json.Marshal(reqBody)

		// Configure service mock to return a general task execution error
		execErr := &domain.ErrTaskExecution{
			TaskID: reqBody.ID,
			Reason: "processing failed",
			Err:    errors.New("internal error"),
		}
		mockTaskService.EXPECT().
			GetTask(gomock.Any(), reqBody.WorkspaceID, reqBody.ID).
			Return(&domain.Task{MaxRuntime: 60}, nil)
		mockTaskService.EXPECT().
			ExecuteTask(gomock.Any(), reqBody.WorkspaceID, reqBody.ID, gomock.Any()).
			Return(execErr)

		// Call handler
		req := httptest.NewRequest(http.MethodPost, "/api/tasks.execute", bytes.NewBuffer(reqJSON))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler.ExecuteTask(rec, req)

		// Verify response has correct status code for internal server error
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})

	t.Run("Task timeout error", func(t *testing.T) {
		// Setup
		reqBody := domain.ExecuteTaskRequest{
			WorkspaceID: "workspace1",
			ID:          "task123",
		}

		reqJSON, _ := json.Marshal(reqBody)

		// Configure service mock to return a timeout error
		timeoutErr := &domain.ErrTaskTimeout{
			TaskID:     reqBody.ID,
			MaxRuntime: 60,
		}
		mockTaskService.EXPECT().
			GetTask(gomock.Any(), reqBody.WorkspaceID, reqBody.ID).
			Return(&domain.Task{MaxRuntime: 60}, nil)
		mockTaskService.EXPECT().
			ExecuteTask(gomock.Any(), reqBody.WorkspaceID, reqBody.ID, gomock.Any()).
			Return(timeoutErr)

		// Call handler
		req := httptest.NewRequest(http.MethodPost, "/api/tasks.execute", bytes.NewBuffer(reqJSON))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler.ExecuteTask(rec, req)

		// Verify response has correct status code for timeout
		assert.Equal(t, http.StatusGatewayTimeout, rec.Code)
	})
}

func TestTaskHandler_RegisterRoutes(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTaskService := mocks.NewMockTaskService(ctrl)
	// For tests we don't need the actual key, we can use a generated one
	jwtSecret := []byte("test-jwt-secret-key-for-testing-32bytes")
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	secretKey := "test-secret-key"

	handler := NewTaskHandler(mockTaskService, func() ([]byte, error) { return jwtSecret, nil }, mockLogger, secretKey)

	// Create a new mux
	mux := http.NewServeMux()

	// Register routes
	handler.RegisterRoutes(mux)

	// Test routes by checking if mux can match all expected patterns
	routes := []string{
		"/api/tasks.create",
		"/api/tasks.list",
		"/api/tasks.get",
		"/api/tasks.delete",
		"/api/tasks.executePending",
		"/api/tasks.execute",
	}

	for _, route := range routes {
		req := httptest.NewRequest(http.MethodGet, route, nil)
		match, _ := mux.Handler(req)
		assert.NotNil(t, match, "Route should be registered: "+route)
	}
}

func TestTaskHandler_CreateTask(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTaskService := mocks.NewMockTaskService(ctrl)
	var jwtSecret []byte
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Set up common logger expectations
	mockLoggerWithField := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLoggerWithField).AnyTimes()
	mockLoggerWithField.EXPECT().Error(gomock.Any()).AnyTimes()

	secretKey := "test-secret-key"

	handler := NewTaskHandler(mockTaskService, func() ([]byte, error) { return jwtSecret, nil }, mockLogger, secretKey)

	t.Run("Successful creation", func(t *testing.T) {
		// Setup
		taskRequest := domain.CreateTaskRequest{
			WorkspaceID:   "workspace1",
			Type:          "email_broadcast",
			MaxRuntime:    300,
			MaxRetries:    3,
			RetryInterval: 300,
		}

		reqJSON, _ := json.Marshal(taskRequest)

		// Configure service mock to return success
		mockTaskService.EXPECT().
			CreateTask(gomock.Any(), taskRequest.WorkspaceID, gomock.Any()).
			Return(nil)

		// Call handler
		req := httptest.NewRequest(http.MethodPost, "/api/tasks.create", bytes.NewBuffer(reqJSON))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler.CreateTask(rec, req)

		// Verify response
		assert.Equal(t, http.StatusCreated, rec.Code)

		var resp map[string]interface{}
		err := json.NewDecoder(rec.Body).Decode(&resp)
		assert.NoError(t, err)
		assert.NotNil(t, resp["task"])
	})

	t.Run("Method not allowed", func(t *testing.T) {
		// Call handler with wrong method
		req := httptest.NewRequest(http.MethodGet, "/api/tasks.create", nil)
		rec := httptest.NewRecorder()

		handler.CreateTask(rec, req)

		// Verify response
		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	})

	t.Run("Invalid request body", func(t *testing.T) {
		// Call handler with invalid JSON
		req := httptest.NewRequest(http.MethodPost, "/api/tasks.create", bytes.NewBuffer([]byte("invalid json")))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler.CreateTask(rec, req)

		// Verify response
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("Invalid task data", func(t *testing.T) {
		// Setup request with invalid task data
		reqBody := map[string]string{
			"workspace_id": "workspace1",
			// Missing Type field which is required
		}

		reqJSON, _ := json.Marshal(reqBody)

		// Call handler
		req := httptest.NewRequest(http.MethodPost, "/api/tasks.create", bytes.NewBuffer(reqJSON))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler.CreateTask(rec, req)

		// Verify response
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("Service error", func(t *testing.T) {
		// Setup
		taskRequest := domain.CreateTaskRequest{
			WorkspaceID:   "workspace1",
			Type:          "email_broadcast",
			MaxRuntime:    300,
			MaxRetries:    3,
			RetryInterval: 300,
		}

		reqJSON, _ := json.Marshal(taskRequest)

		// Configure service mock to return an error
		mockTaskService.EXPECT().
			CreateTask(gomock.Any(), taskRequest.WorkspaceID, gomock.Any()).
			Return(errors.New("service error"))

		// Call handler
		req := httptest.NewRequest(http.MethodPost, "/api/tasks.create", bytes.NewBuffer(reqJSON))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler.CreateTask(rec, req)

		// Verify response
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}

func TestTaskHandler_GetTask(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTaskService := mocks.NewMockTaskService(ctrl)
	var jwtSecret []byte
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Set up common logger expectations
	mockLoggerWithField := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLoggerWithField).AnyTimes()
	mockLoggerWithField.EXPECT().Error(gomock.Any()).AnyTimes()

	secretKey := "test-secret-key"

	handler := NewTaskHandler(mockTaskService, func() ([]byte, error) { return jwtSecret, nil }, mockLogger, secretKey)

	t.Run("Successful retrieval", func(t *testing.T) {
		// Setup expected task
		now := time.Now()
		task := &domain.Task{
			ID:          "task123",
			Type:        "email_broadcast",
			Status:      domain.TaskStatusPending,
			WorkspaceID: "workspace1",
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		// Configure service mock to return the task
		mockTaskService.EXPECT().
			GetTask(gomock.Any(), "workspace1", "task123").
			Return(task, nil)

		// Call handler
		req := httptest.NewRequest(http.MethodGet, "/api/tasks.get?workspace_id=workspace1&id=task123", nil)
		rec := httptest.NewRecorder()

		handler.GetTask(rec, req)

		// Verify response
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		err := json.NewDecoder(rec.Body).Decode(&resp)
		assert.NoError(t, err)
		assert.NotNil(t, resp["task"])
	})

	t.Run("Method not allowed", func(t *testing.T) {
		// Call handler with wrong method
		req := httptest.NewRequest(http.MethodPost, "/api/tasks.get", nil)
		rec := httptest.NewRecorder()

		handler.GetTask(rec, req)

		// Verify response
		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	})

	t.Run("Missing parameters", func(t *testing.T) {
		// Call handler with missing required parameters
		req := httptest.NewRequest(http.MethodGet, "/api/tasks.get", nil)
		rec := httptest.NewRecorder()

		handler.GetTask(rec, req)

		// Verify response
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("Task not found", func(t *testing.T) {
		// Configure service mock to return a not found error
		mockTaskService.EXPECT().
			GetTask(gomock.Any(), "workspace1", "nonexistent").
			Return(nil, errors.New("task not found"))

		// Call handler
		req := httptest.NewRequest(http.MethodGet, "/api/tasks.get?workspace_id=workspace1&id=nonexistent", nil)
		rec := httptest.NewRecorder()

		handler.GetTask(rec, req)

		// Verify response
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("Service error (not not-found)", func(t *testing.T) {
		// Configure service mock to return a service error
		mockTaskService.EXPECT().
			GetTask(gomock.Any(), "workspace1", "task123").
			Return(nil, errors.New("database error"))

		// Call handler
		req := httptest.NewRequest(http.MethodGet, "/api/tasks.get?workspace_id=workspace1&id=task123", nil)
		rec := httptest.NewRecorder()

		handler.GetTask(rec, req)

		// Verify response
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}

func TestTaskHandler_ListTasks(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTaskService := mocks.NewMockTaskService(ctrl)
	var jwtSecret []byte
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Set up common logger expectations
	mockLoggerWithField := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLoggerWithField).AnyTimes()
	mockLoggerWithField.EXPECT().Error(gomock.Any()).AnyTimes()

	secretKey := "test-secret-key"

	handler := NewTaskHandler(mockTaskService, func() ([]byte, error) { return jwtSecret, nil }, mockLogger, secretKey)

	t.Run("Successful list", func(t *testing.T) {
		// Setup expected response
		now := time.Now()
		response := &domain.TaskListResponse{
			Tasks: []*domain.Task{
				{
					ID:          "task123",
					Type:        "email_broadcast",
					Status:      domain.TaskStatusPending,
					WorkspaceID: "workspace1",
					CreatedAt:   now,
					UpdatedAt:   now,
				},
				{
					ID:          "task456",
					Type:        "sms_broadcast",
					Status:      domain.TaskStatusCompleted,
					WorkspaceID: "workspace1",
					CreatedAt:   now,
					UpdatedAt:   now,
				},
			},
			TotalCount: 2,
		}

		// Configure service mock to return the response
		mockTaskService.EXPECT().
			ListTasks(gomock.Any(), "workspace1", gomock.Any()).
			DoAndReturn(func(_ context.Context, workspaceID string, filter domain.TaskFilter) (*domain.TaskListResponse, error) {
				assert.Contains(t, filter.Status, domain.TaskStatusPending)
				assert.Contains(t, filter.Type, "email_broadcast")
				assert.Equal(t, 10, filter.Limit)
				assert.Equal(t, 0, filter.Offset)
				return response, nil
			})

		// Call handler
		req := httptest.NewRequest(http.MethodGet, "/api/tasks.list?workspace_id=workspace1&status=pending&type=email_broadcast&limit=10", nil)
		rec := httptest.NewRecorder()

		handler.ListTasks(rec, req)

		// Verify response
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		err := json.NewDecoder(rec.Body).Decode(&resp)
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, float64(2), resp["total_count"].(float64))
		assert.Len(t, resp["tasks"].([]interface{}), 2)
	})

	t.Run("Method not allowed", func(t *testing.T) {
		// Call handler with wrong method
		req := httptest.NewRequest(http.MethodPost, "/api/tasks.list", nil)
		rec := httptest.NewRecorder()

		handler.ListTasks(rec, req)

		// Verify response
		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	})

	t.Run("Missing workspace_id", func(t *testing.T) {
		// Call handler with missing required workspace_id
		req := httptest.NewRequest(http.MethodGet, "/api/tasks.list", nil)
		rec := httptest.NewRecorder()

		handler.ListTasks(rec, req)

		// Verify response
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("Invalid filter parameters", func(t *testing.T) {
		// Call handler with invalid filter parameters
		req := httptest.NewRequest(http.MethodGet, "/api/tasks.list?workspace_id=workspace1&limit=invalid", nil)
		rec := httptest.NewRecorder()

		handler.ListTasks(rec, req)

		// Verify response
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("Service error", func(t *testing.T) {
		// Configure service mock to return an error
		mockTaskService.EXPECT().
			ListTasks(gomock.Any(), "workspace1", gomock.Any()).
			Return(nil, errors.New("service error"))

		// Call handler
		req := httptest.NewRequest(http.MethodGet, "/api/tasks.list?workspace_id=workspace1", nil)
		rec := httptest.NewRecorder()

		handler.ListTasks(rec, req)

		// Verify response
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}

func TestTaskHandler_DeleteTask(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTaskService := mocks.NewMockTaskService(ctrl)
	var jwtSecret []byte
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Set up common logger expectations
	mockLoggerWithField := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLoggerWithField).AnyTimes()
	mockLoggerWithField.EXPECT().Error(gomock.Any()).AnyTimes()

	secretKey := "test-secret-key"

	handler := NewTaskHandler(mockTaskService, func() ([]byte, error) { return jwtSecret, nil }, mockLogger, secretKey)

	t.Run("Successful deletion", func(t *testing.T) {
		// Configure service mock to return success
		mockTaskService.EXPECT().
			DeleteTask(gomock.Any(), "workspace1", "task123").
			Return(nil)

		// Call handler
		req := httptest.NewRequest(http.MethodPost, "/api/tasks.delete?workspace_id=workspace1&id=task123", nil)
		rec := httptest.NewRecorder()

		handler.DeleteTask(rec, req)

		// Verify response
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		err := json.NewDecoder(rec.Body).Decode(&resp)
		assert.NoError(t, err)
		assert.True(t, resp["success"].(bool))
	})

	t.Run("Method not allowed", func(t *testing.T) {
		// Call handler with wrong method
		req := httptest.NewRequest(http.MethodGet, "/api/tasks.delete", nil)
		rec := httptest.NewRecorder()

		handler.DeleteTask(rec, req)

		// Verify response
		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	})

	t.Run("Missing parameters", func(t *testing.T) {
		// Call handler with missing required parameters
		req := httptest.NewRequest(http.MethodPost, "/api/tasks.delete", nil)
		rec := httptest.NewRecorder()

		handler.DeleteTask(rec, req)

		// Verify response
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("Task not found", func(t *testing.T) {
		// Configure service mock to return a not found error
		mockTaskService.EXPECT().
			DeleteTask(gomock.Any(), "workspace1", "nonexistent").
			Return(errors.New("task not found"))

		// Call handler
		req := httptest.NewRequest(http.MethodPost, "/api/tasks.delete?workspace_id=workspace1&id=nonexistent", nil)
		rec := httptest.NewRecorder()

		handler.DeleteTask(rec, req)

		// Verify response
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("Service error (not not-found)", func(t *testing.T) {
		// Configure service mock to return a service error
		mockTaskService.EXPECT().
			DeleteTask(gomock.Any(), "workspace1", "task123").
			Return(errors.New("database error"))

		// Call handler
		req := httptest.NewRequest(http.MethodPost, "/api/tasks.delete?workspace_id=workspace1&id=task123", nil)
		rec := httptest.NewRecorder()

		handler.DeleteTask(rec, req)

		// Verify response
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}

func TestTaskHandler_ExecutePendingTasks(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTaskService := mocks.NewMockTaskService(ctrl)
	var jwtSecret []byte
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Set up common logger expectations
	mockLoggerWithField := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLoggerWithField).AnyTimes()
	mockLoggerWithField.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes() // For manual trigger logging

	secretKey := "test-secret-key"

	handler := NewTaskHandler(mockTaskService, func() ([]byte, error) { return jwtSecret, nil }, mockLogger, secretKey)

	t.Run("Successful execution", func(t *testing.T) {
		// Configure service mock to return success
		mockTaskService.EXPECT().
			ExecutePendingTasks(gomock.Any(), 10).
			Return(nil)

		// Call handler
		req := httptest.NewRequest(http.MethodGet, "/api/tasks.executePending?max_tasks=10", nil)
		rec := httptest.NewRecorder()

		handler.ExecutePendingTasks(rec, req)

		// Verify response
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		err := json.NewDecoder(rec.Body).Decode(&resp)
		assert.NoError(t, err)
		assert.True(t, resp["success"].(bool))
		assert.Equal(t, float64(10), resp["max_tasks"])
	})

	t.Run("Method not allowed", func(t *testing.T) {
		// Call handler with wrong method
		req := httptest.NewRequest(http.MethodPost, "/api/tasks.executePending", nil)
		rec := httptest.NewRecorder()

		handler.ExecutePendingTasks(rec, req)

		// Verify response
		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	})

	t.Run("Invalid max_tasks parameter", func(t *testing.T) {
		// Call handler with invalid max_tasks parameter
		req := httptest.NewRequest(http.MethodGet, "/api/tasks.executePending?max_tasks=invalid", nil)
		rec := httptest.NewRecorder()

		handler.ExecutePendingTasks(rec, req)

		// Verify response
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("Default max_tasks (omitted)", func(t *testing.T) {
		// Configure service mock to return success with default max_tasks (should be 100)
		mockTaskService.EXPECT().
			ExecutePendingTasks(gomock.Any(), 100).
			Return(nil)

		// Call handler without max_tasks
		req := httptest.NewRequest(http.MethodGet, "/api/tasks.executePending", nil)
		rec := httptest.NewRecorder()

		handler.ExecutePendingTasks(rec, req)

		// Verify response
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("Service error", func(t *testing.T) {
		// Configure service mock to return an error
		mockTaskService.EXPECT().
			ExecutePendingTasks(gomock.Any(), 10).
			Return(errors.New("service error"))

		// Call handler
		req := httptest.NewRequest(http.MethodGet, "/api/tasks.executePending?max_tasks=10", nil)
		rec := httptest.NewRecorder()

		handler.ExecutePendingTasks(rec, req)

		// Verify response
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}

func TestTaskHandler_GetCronStatus(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTaskService := mocks.NewMockTaskService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Configure logger expectations
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	// Create test JWT secret
	jwtSecret := []byte("test-jwt-secret-key-for-testing-32bytes")

	handler := NewTaskHandler(
		mockTaskService,
		func() ([]byte, error) { return jwtSecret, nil },
		mockLogger,
		"test-secret",
	)

	t.Run("Returns last cron run when available", func(t *testing.T) {
		// Setup
		lastRun := time.Now().Add(-30 * time.Minute).UTC()

		mockTaskService.EXPECT().
			GetLastCronRun(gomock.Any()).
			Return(&lastRun, nil)

		// Create request
		req := httptest.NewRequest(http.MethodGet, "/api/cron.status", nil)
		w := httptest.NewRecorder()

		// Call handler
		handler.GetCronStatus(w, req)

		// Assert
		assert.Equal(t, http.StatusOK, w.Code)

		// Check response contains expected fields
		body := w.Body.String()
		assert.Contains(t, body, `"success":true`)
		assert.Contains(t, body, `"last_run"`)
		assert.Contains(t, body, `"last_run_unix"`)
		assert.Contains(t, body, `"time_since_last_run"`)
		assert.Contains(t, body, `"time_since_last_run_seconds"`)
		assert.Contains(t, body, lastRun.Format(time.RFC3339))
	})

	t.Run("Returns null when no cron run recorded", func(t *testing.T) {
		// Setup
		mockTaskService.EXPECT().
			GetLastCronRun(gomock.Any()).
			Return(nil, nil)

		// Create request
		req := httptest.NewRequest(http.MethodGet, "/api/cron.status", nil)
		w := httptest.NewRecorder()

		// Call handler
		handler.GetCronStatus(w, req)

		// Assert
		assert.Equal(t, http.StatusOK, w.Code)

		// Check response
		body := w.Body.String()
		assert.Contains(t, body, `"success":true`)
		assert.Contains(t, body, `"last_run":null`)
		assert.Contains(t, body, `"last_run_unix":null`)
		assert.Contains(t, body, `"time_since_last_run":null`)
		assert.Contains(t, body, `"time_since_last_run_seconds":null`)
		assert.Contains(t, body, `"No cron run recorded yet"`)
	})

	t.Run("Handles service error", func(t *testing.T) {
		// Setup
		mockTaskService.EXPECT().
			GetLastCronRun(gomock.Any()).
			Return(nil, assert.AnError)

		// Create request
		req := httptest.NewRequest(http.MethodGet, "/api/cron.status", nil)
		w := httptest.NewRecorder()

		// Call handler
		handler.GetCronStatus(w, req)

		// Assert
		assert.Equal(t, http.StatusInternalServerError, w.Code)

		// Check error response
		body := w.Body.String()
		assert.Contains(t, body, `"error"`)
		assert.Contains(t, body, `"Failed to get cron status"`)
	})

	t.Run("Rejects non-GET methods", func(t *testing.T) {
		// Create POST request
		req := httptest.NewRequest(http.MethodPost, "/api/cron.status", nil)
		w := httptest.NewRecorder()

		// Call handler
		handler.GetCronStatus(w, req)

		// Assert
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)

		// Check error response
		body := w.Body.String()
		assert.Contains(t, body, `"Method not allowed"`)
	})
}

func TestTaskHandler_GetCronStatus_Integration(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTaskService := mocks.NewMockTaskService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Configure logger expectations
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()

	// Create test JWT secret
	jwtSecret := []byte("test-jwt-secret-key-for-testing-32bytes")

	handler := NewTaskHandler(
		mockTaskService,
		func() ([]byte, error) { return jwtSecret, nil },
		mockLogger,
		"test-secret",
	)

	// Test the endpoint is properly registered
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// Setup mock
	lastRun := time.Now().Add(-1 * time.Hour).UTC()
	mockTaskService.EXPECT().
		GetLastCronRun(gomock.Any()).
		Return(&lastRun, nil)

	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/cron.status", nil)
	w := httptest.NewRecorder()

	// Call through mux
	mux.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusOK, w.Code)

	// Check response contains expected timestamp
	body := w.Body.String()
	assert.Contains(t, body, `"success":true`)
	assert.Contains(t, body, lastRun.Format(time.RFC3339))
}

func TestTaskHandler_ResetTask(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTaskService := mocks.NewMockTaskService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Configure logger
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	jwtSecret := []byte("test-jwt-secret-key-for-testing-32bytes")
	handler := NewTaskHandler(
		mockTaskService,
		func() ([]byte, error) { return jwtSecret, nil },
		mockLogger,
		"test-secret",
	)

	t.Run("Success", func(t *testing.T) {
		mockTaskService.EXPECT().
			ResetTask(gomock.Any(), "ws-1", "task-1").
			Return(nil)

		body := `{"workspace_id": "ws-1", "id": "task-1"}`
		req := httptest.NewRequest(http.MethodPost, "/api/tasks.reset", strings.NewReader(body))
		w := httptest.NewRecorder()

		handler.ResetTask(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"success":true`)
	})

	t.Run("Method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/tasks.reset", nil)
		w := httptest.NewRecorder()

		handler.ResetTask(w, req)

		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("Invalid body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/tasks.reset", strings.NewReader("invalid"))
		w := httptest.NewRecorder()

		handler.ResetTask(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Missing workspace_id", func(t *testing.T) {
		body := `{"id": "task-1"}`
		req := httptest.NewRequest(http.MethodPost, "/api/tasks.reset", strings.NewReader(body))
		w := httptest.NewRecorder()

		handler.ResetTask(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "workspace_id is required")
	})

	t.Run("Task not found", func(t *testing.T) {
		mockTaskService.EXPECT().
			ResetTask(gomock.Any(), "ws-1", "task-not-found").
			Return(domain.ErrTaskNotFound)

		body := `{"workspace_id": "ws-1", "id": "task-not-found"}`
		req := httptest.NewRequest(http.MethodPost, "/api/tasks.reset", strings.NewReader(body))
		w := httptest.NewRecorder()

		handler.ResetTask(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestTaskHandler_TriggerTask(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTaskService := mocks.NewMockTaskService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Configure logger
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	jwtSecret := []byte("test-jwt-secret-key-for-testing-32bytes")
	handler := NewTaskHandler(
		mockTaskService,
		func() ([]byte, error) { return jwtSecret, nil },
		mockLogger,
		"test-secret",
	)

	t.Run("Success", func(t *testing.T) {
		mockTaskService.EXPECT().
			TriggerTask(gomock.Any(), "ws-1", "task-1").
			Return(nil)

		body := `{"workspace_id": "ws-1", "id": "task-1"}`
		req := httptest.NewRequest(http.MethodPost, "/api/tasks.trigger", strings.NewReader(body))
		w := httptest.NewRecorder()

		handler.TriggerTask(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"success":true`)
	})

	t.Run("Method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/tasks.trigger", nil)
		w := httptest.NewRecorder()

		handler.TriggerTask(w, req)

		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("Invalid body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/tasks.trigger", strings.NewReader("invalid"))
		w := httptest.NewRecorder()

		handler.TriggerTask(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Missing id", func(t *testing.T) {
		body := `{"workspace_id": "ws-1"}`
		req := httptest.NewRequest(http.MethodPost, "/api/tasks.trigger", strings.NewReader(body))
		w := httptest.NewRecorder()

		handler.TriggerTask(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "id is required")
	})

	t.Run("Task not found", func(t *testing.T) {
		mockTaskService.EXPECT().
			TriggerTask(gomock.Any(), "ws-1", "task-not-found").
			Return(domain.ErrTaskNotFound)

		body := `{"workspace_id": "ws-1", "id": "task-not-found"}`
		req := httptest.NewRequest(http.MethodPost, "/api/tasks.trigger", strings.NewReader(body))
		w := httptest.NewRecorder()

		handler.TriggerTask(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("Task already running", func(t *testing.T) {
		mockTaskService.EXPECT().
			TriggerTask(gomock.Any(), "ws-1", "task-running").
			Return(&domain.ErrTaskAlreadyRunning{TaskID: "task-running"})

		body := `{"workspace_id": "ws-1", "id": "task-running"}`
		req := httptest.NewRequest(http.MethodPost, "/api/tasks.trigger", strings.NewReader(body))
		w := httptest.NewRecorder()

		handler.TriggerTask(w, req)

		assert.Equal(t, http.StatusConflict, w.Code)
		assert.Contains(t, w.Body.String(), "already running")
	})
}
