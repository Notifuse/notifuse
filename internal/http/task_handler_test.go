package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// allowOwner authenticates every workspace as an owner, whose HasPermission
// short-circuits to true. Tests that care about a specific grant stub the auth
// service themselves.
func allowOwner(mockAuth *mocks.MockAuthService) {
	mockAuth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, workspaceID string) (context.Context, *domain.User, *domain.UserWorkspace, error) {
			return ctx, &domain.User{ID: "user-1"},
				&domain.UserWorkspace{UserID: "user-1", WorkspaceID: workspaceID, Role: "owner"}, nil
		}).AnyTimes()
}

// memberWith builds a non-owner membership holding exactly the given grants.
func memberWith(workspaceID string, permissions domain.UserPermissions) *domain.UserWorkspace {
	return &domain.UserWorkspace{
		UserID:      "user-1",
		WorkspaceID: workspaceID,
		Role:        "member",
		Permissions: permissions,
	}
}

// allowMember authenticates one workspace as the given membership, and refuses
// every other workspace the way AuthenticateUserForWorkspace does.
func allowMember(mockAuth *mocks.MockAuthService, userWorkspace *domain.UserWorkspace) {
	mockAuth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, workspaceID string) (context.Context, *domain.User, *domain.UserWorkspace, error) {
			if workspaceID != userWorkspace.WorkspaceID {
				return ctx, nil, nil, domain.ErrUserNotInWorkspace
			}
			return ctx, &domain.User{ID: userWorkspace.UserID}, userWorkspace, nil
		}).AnyTimes()
}

// signExecuteRequest stamps the headers a dispatch of /api/tasks.execute carries.
func signExecuteRequest(req *http.Request, secretKey string, body []byte, at time.Time) {
	timestamp := at.UTC().Unix()
	req.Header.Set(domain.TaskExecuteTimestampHeader, strconv.FormatInt(timestamp, 10))
	req.Header.Set(domain.TaskExecuteSignatureHeader,
		domain.SignTaskExecuteRequest(domain.TaskExecuteSigningKey(secretKey), timestamp, req.URL.Path, body))
}

// newSignedExecuteRequest builds a dispatch signed over its own body, as the
// scheduler emits it.
func newSignedExecuteRequest(secretKey string, body []byte) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/tasks.execute", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	signExecuteRequest(req, secretKey, body, time.Now())
	return req
}

// newTaskHandlerForAuth builds a handler whose logger accepts anything, for the
// authorization tests.
func newTaskHandlerForAuth(t *testing.T, ctrl *gomock.Controller) (*TaskHandler, *mocks.MockTaskService, *mocks.MockAuthService) {
	t.Helper()

	mockTaskService := mocks.NewMockTaskService(ctrl)
	mockAuth := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	handler := NewTaskHandler(mockTaskService, mockAuth,
		func() ([]byte, error) { return []byte("test-jwt-secret-key-for-testing-32bytes"), nil },
		mockLogger, "test-secret-key", true)

	return handler, mockTaskService, mockAuth
}

func TestTaskHandler_ExecuteTask(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTaskService := mocks.NewMockTaskService(ctrl)
	mockAuth := mocks.NewMockAuthService(ctrl)
	allowOwner(mockAuth)
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
	mockLoggerWithField.EXPECT().Info(gomock.Any()).AnyTimes()

	secretKey := "test-secret-key"

	handler := NewTaskHandler(mockTaskService, mockAuth, func() ([]byte, error) { return jwtSecret, nil }, mockLogger, secretKey, true)

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
		req := newSignedExecuteRequest(secretKey, reqJSON)
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
		req := newSignedExecuteRequest(secretKey, []byte("invalid json"))
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
		req := newSignedExecuteRequest(secretKey, reqJSON)
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
		req := newSignedExecuteRequest(secretKey, reqJSON)
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
		req := newSignedExecuteRequest(secretKey, reqJSON)
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
		req := newSignedExecuteRequest(secretKey, reqJSON)
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
		req := newSignedExecuteRequest(secretKey, reqJSON)
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
	mockAuth := mocks.NewMockAuthService(ctrl)
	allowOwner(mockAuth)
	// For tests we don't need the actual key, we can use a generated one
	jwtSecret := []byte("test-jwt-secret-key-for-testing-32bytes")
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	secretKey := "test-secret-key"

	handler := NewTaskHandler(mockTaskService, mockAuth, func() ([]byte, error) { return jwtSecret, nil }, mockLogger, secretKey, true)

	// Create a new mux
	mux := http.NewServeMux()

	// Register routes
	handler.RegisterRoutes(mux)

	// mux.Handler returns a non-nil handler for anything (the 404 handler), so
	// the pattern is what says whether a route exists.
	registered := []string{
		"/api/tasks.list",
		"/api/tasks.get",
		"/api/tasks.delete",
		"/api/tasks.reset",
		"/api/tasks.trigger",
		"/api/tasks.execute",
		// tasks.create is retired but still routed, so it can answer 410 instead of
		// falling through to the SPA catch-all's empty 200.
		"/api/tasks.create",
		"/api/cron",
		"/api/cron.status",
	}

	for _, route := range registered {
		req := httptest.NewRequest(http.MethodGet, route, nil)
		_, pattern := mux.Handler(req)
		assert.Equal(t, route, pattern, "Route should be registered: "+route)
	}
}

func TestTaskHandler_CreateTaskGone(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	handler, _, _ := newTaskHandlerWithDispatch(t, ctrl, true)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// Whatever the caller sends, and whether or not it is authenticated: the
	// endpoint is gone, and the body has to say so — an empty 200 would read as a
	// created task.
	for _, method := range []string{http.MethodPost, http.MethodGet} {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/api/tasks.create",
				strings.NewReader(`{"workspace_id":"workspace123","type":"send_broadcast"}`))
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			assert.Equal(t, http.StatusGone, w.Code)

			var response map[string]string
			require.NoError(t, json.NewDecoder(w.Body).Decode(&response))
			assert.NotEmpty(t, response["error"])
			assert.Contains(t, response["error"], "tasks.create")
		})
	}
}

// newTaskHandlerWithDispatch builds a handler with the HTTP-dispatch flag set
// either way, over a logger that accepts anything.
func newTaskHandlerWithDispatch(t *testing.T, ctrl *gomock.Controller, httpDispatchEnabled bool) (*TaskHandler, *mocks.MockTaskService, *mocks.MockAuthService) {
	t.Helper()

	mockTaskService := mocks.NewMockTaskService(ctrl)
	mockAuth := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	handler := NewTaskHandler(mockTaskService, mockAuth,
		func() ([]byte, error) { return []byte("test-jwt-secret-key-for-testing-32bytes"), nil },
		mockLogger, "test-secret-key", httpDispatchEnabled)

	return handler, mockTaskService, mockAuth
}

// TestTaskHandler_ExecuteTask_SchedulerEnabled pins what a default install — one
// running its own scheduler, which executes tasks in-process — answers on the
// dispatch endpoint.
//
// The route is registered either way. Leaving it unregistered handed the request
// to the console SPA catch-all, which answers 200 with an empty body for any
// unrouted /api path, so a dispatcher aimed at the wrong instance saw success and
// dropped the execution. Owning the path is what makes the misconfiguration loud.
func TestTaskHandler_ExecuteTask_SchedulerEnabled(t *testing.T) {
	body := []byte(`{"workspace_id":"ws-1","id":"task-1"}`)

	t.Run("the route is registered so nothing falls through to the SPA", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		handler, _, _ := newTaskHandlerWithDispatch(t, ctrl, false)

		mux := http.NewServeMux()
		handler.RegisterRoutes(mux)

		_, pattern := mux.Handler(httptest.NewRequest(http.MethodPost, "/api/tasks.execute", nil))
		assert.Equal(t, "/api/tasks.execute", pattern)

		// The cron endpoints stay public either way — external crontabs rely on them.
		_, cronPattern := mux.Handler(httptest.NewRequest(http.MethodGet, "/api/cron", nil))
		assert.Equal(t, "/api/cron", cronPattern)
	})

	t.Run("a dispatch answers 404 with a JSON body", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// Nothing reaches the service: the endpoint behaves as if it were absent.
		handler, mockTaskService, _ := newTaskHandlerWithDispatch(t, ctrl, false)
		mockTaskService.EXPECT().GetTask(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
		mockTaskService.EXPECT().ExecuteTask(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		mux := http.NewServeMux()
		handler.RegisterRoutes(mux)

		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, newSignedExecuteRequest("test-secret-key", body))

		assert.Equal(t, http.StatusNotFound, rec.Code)
		assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
		var payload map[string]string
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
		assert.NotEmpty(t, payload["error"], "the body has to be a JSON error, not the SPA's empty 200")
	})

	t.Run("a correct signature does not unlock it", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		handler, mockTaskService, _ := newTaskHandlerWithDispatch(t, ctrl, false)
		mockTaskService.EXPECT().ExecuteTask(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		rec := httptest.NewRecorder()
		handler.ExecuteTask(rec, newSignedExecuteRequest("test-secret-key", body))

		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("an unsigned dispatch gets the same 404, not a 401", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// An unauthenticated prober must not be able to tell a disabled endpoint
		// from a missing one — that is the posture the conditional registration
		// had, and it is unchanged.
		handler, _, _ := newTaskHandlerWithDispatch(t, ctrl, false)

		rec := httptest.NewRecorder()
		handler.ExecuteTask(rec, httptest.NewRequest(http.MethodPost, "/api/tasks.execute", bytes.NewReader(body)))

		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("signature verification still applies when dispatch is enabled", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		handler, mockTaskService, _ := newTaskHandlerWithDispatch(t, ctrl, true)
		mockTaskService.EXPECT().ExecuteTask(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		rec := httptest.NewRecorder()
		handler.ExecuteTask(rec, httptest.NewRequest(http.MethodPost, "/api/tasks.execute", bytes.NewReader(body)))

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})
}

func TestTaskHandler_GetTask(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTaskService := mocks.NewMockTaskService(ctrl)
	mockAuth := mocks.NewMockAuthService(ctrl)
	allowOwner(mockAuth)
	var jwtSecret []byte
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Set up common logger expectations
	mockLoggerWithField := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLoggerWithField).AnyTimes()
	mockLoggerWithField.EXPECT().Error(gomock.Any()).AnyTimes()

	secretKey := "test-secret-key"

	handler := NewTaskHandler(mockTaskService, mockAuth, func() ([]byte, error) { return jwtSecret, nil }, mockLogger, secretKey, true)

	t.Run("Successful retrieval", func(t *testing.T) {
		// Setup expected task
		now := time.Now()
		task := &domain.Task{
			ID:          "task123",
			Type:        "send_broadcast",
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
	mockAuth := mocks.NewMockAuthService(ctrl)
	allowOwner(mockAuth)
	var jwtSecret []byte
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Set up common logger expectations
	mockLoggerWithField := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLoggerWithField).AnyTimes()
	mockLoggerWithField.EXPECT().Error(gomock.Any()).AnyTimes()

	secretKey := "test-secret-key"

	handler := NewTaskHandler(mockTaskService, mockAuth, func() ([]byte, error) { return jwtSecret, nil }, mockLogger, secretKey, true)

	t.Run("Successful list", func(t *testing.T) {
		// Setup expected response
		now := time.Now()
		response := &domain.TaskListResponse{
			Tasks: []*domain.Task{
				{
					ID:          "task123",
					Type:        "send_broadcast",
					Status:      domain.TaskStatusPending,
					WorkspaceID: "workspace1",
					CreatedAt:   now,
					UpdatedAt:   now,
				},
				{
					ID:          "task456",
					Type:        "build_segment",
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
				assert.Contains(t, filter.Type, "send_broadcast")
				assert.Equal(t, 10, filter.Limit)
				assert.Equal(t, 0, filter.Offset)
				return response, nil
			})

		// Call handler
		req := httptest.NewRequest(http.MethodGet, "/api/tasks.list?workspace_id=workspace1&status=pending&type=send_broadcast&limit=10", nil)
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
			DoAndReturn(func(_ context.Context, _ string, filter domain.TaskFilter) (*domain.TaskListResponse, error) {
				// No type named, so the filter carries the types the caller can read.
				assert.Contains(t, filter.Type, "send_broadcast")
				return nil, errors.New("service error")
			})

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
	mockAuth := mocks.NewMockAuthService(ctrl)
	allowOwner(mockAuth)
	var jwtSecret []byte
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Set up common logger expectations
	mockLoggerWithField := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLoggerWithField).AnyTimes()
	mockLoggerWithField.EXPECT().Error(gomock.Any()).AnyTimes()

	secretKey := "test-secret-key"

	handler := NewTaskHandler(mockTaskService, mockAuth, func() ([]byte, error) { return jwtSecret, nil }, mockLogger, secretKey, true)

	t.Run("Successful deletion", func(t *testing.T) {
		// Configure service mock to return success
		mockTaskService.EXPECT().
			GetTask(gomock.Any(), "workspace1", "task123").
			Return(&domain.Task{ID: "task123", Type: "send_broadcast"}, nil)
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
		// The authorization read is what discovers the task is gone, so the
		// delete never runs.
		mockTaskService.EXPECT().
			GetTask(gomock.Any(), "workspace1", "nonexistent").
			Return(nil, errors.New("task not found"))

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
			GetTask(gomock.Any(), "workspace1", "task123").
			Return(&domain.Task{ID: "task123", Type: "send_broadcast"}, nil)
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
	mockAuth := mocks.NewMockAuthService(ctrl)
	allowOwner(mockAuth)
	var jwtSecret []byte
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Set up common logger expectations
	mockLoggerWithField := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLoggerWithField).AnyTimes()
	mockLoggerWithField.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes() // For manual trigger logging

	secretKey := "test-secret-key"

	handler := NewTaskHandler(mockTaskService, mockAuth, func() ([]byte, error) { return jwtSecret, nil }, mockLogger, secretKey, true)

	// waitForCronIdle waits until the background run has finished, so the
	// single-flight guard is clear before the next subtest runs.
	waitForCronIdle := func(t *testing.T) {
		t.Helper()
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			if !handler.cronRunning.Load() {
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
		t.Fatal("background cron run never finished")
	}

	t.Run("Accepts and runs in the background", func(t *testing.T) {
		started := make(chan struct{}, 1)
		mockTaskService.EXPECT().
			ExecutePendingTasks(gomock.Any(), 10).
			DoAndReturn(func(context.Context, int) error {
				started <- struct{}{}
				return nil
			})

		req := httptest.NewRequest(http.MethodGet, "/api/cron?max_tasks=10", nil)
		rec := httptest.NewRecorder()

		handler.ExecutePendingTasks(rec, req)

		// The endpoint acknowledges immediately instead of holding the caller
		// open for the whole run — an external cron with a short timeout used
		// to give up and take the running tasks down with it.
		assert.Equal(t, http.StatusAccepted, rec.Code)

		var resp map[string]interface{}
		err := json.NewDecoder(rec.Body).Decode(&resp)
		assert.NoError(t, err)
		assert.True(t, resp["success"].(bool))
		assert.Equal(t, float64(10), resp["max_tasks"])

		select {
		case <-started:
		case <-time.After(2 * time.Second):
			t.Fatal("background execution never started")
		}
		waitForCronIdle(t)
	})

	t.Run("Runs on a context detached from the request", func(t *testing.T) {
		captured := make(chan context.Context, 1)
		mockTaskService.EXPECT().
			ExecutePendingTasks(gomock.Any(), 10).
			DoAndReturn(func(ctx context.Context, _ int) error {
				captured <- ctx
				return nil
			})

		reqCtx, cancel := context.WithCancel(context.Background())
		req := httptest.NewRequest(http.MethodGet, "/api/cron?max_tasks=10", nil).WithContext(reqCtx)
		rec := httptest.NewRecorder()

		handler.ExecutePendingTasks(rec, req)
		assert.Equal(t, http.StatusAccepted, rec.Code)

		// The caller hangs up. In-process execution means the tasks run right
		// here, so this must not cancel them mid database transaction.
		cancel()

		select {
		case execCtx := <-captured:
			assert.NoError(t, execCtx.Err(),
				"cancelling the request must not cancel the run it started")
		case <-time.After(2 * time.Second):
			t.Fatal("background execution never started")
		}
		waitForCronIdle(t)
	})

	t.Run("Refuses to start a concurrent run", func(t *testing.T) {
		release := make(chan struct{})
		mockTaskService.EXPECT().
			ExecutePendingTasks(gomock.Any(), 10).
			DoAndReturn(func(context.Context, int) error {
				<-release
				return nil
			})

		rec1 := httptest.NewRecorder()
		handler.ExecutePendingTasks(rec1, httptest.NewRequest(http.MethodGet, "/api/cron?max_tasks=10", nil))
		assert.Equal(t, http.StatusAccepted, rec1.Code)

		// A second trigger while the first is in flight must not start
		// anything: the endpoint is public, so answering immediately without
		// this guard would let any caller spawn unbounded background runs.
		rec2 := httptest.NewRecorder()
		handler.ExecutePendingTasks(rec2, httptest.NewRequest(http.MethodGet, "/api/cron?max_tasks=10", nil))
		assert.Equal(t, http.StatusAccepted, rec2.Code)

		var resp map[string]interface{}
		assert.NoError(t, json.NewDecoder(rec2.Body).Decode(&resp))
		assert.Equal(t, true, resp["already_running"])

		close(release)
		waitForCronIdle(t)
	})

	t.Run("Recovers from a panic in the background run", func(t *testing.T) {
		mockTaskService.EXPECT().
			ExecutePendingTasks(gomock.Any(), 10).
			DoAndReturn(func(context.Context, int) error {
				panic("boom")
			})

		rec := httptest.NewRecorder()
		handler.ExecutePendingTasks(rec, httptest.NewRequest(http.MethodGet, "/api/cron?max_tasks=10", nil))
		assert.Equal(t, http.StatusAccepted, rec.Code)

		// The panic must not escape the goroutine, and must still release the
		// single-flight guard.
		waitForCronIdle(t)
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
		started := make(chan struct{}, 1)
		mockTaskService.EXPECT().
			ExecutePendingTasks(gomock.Any(), 100).
			DoAndReturn(func(context.Context, int) error {
				started <- struct{}{}
				return nil
			})

		// Call handler without max_tasks
		req := httptest.NewRequest(http.MethodGet, "/api/cron", nil)
		rec := httptest.NewRecorder()

		handler.ExecutePendingTasks(rec, req)

		assert.Equal(t, http.StatusAccepted, rec.Code)
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			t.Fatal("background execution never started")
		}
		waitForCronIdle(t)
	})

	t.Run("Service error is logged, not returned", func(t *testing.T) {
		started := make(chan struct{}, 1)
		mockTaskService.EXPECT().
			ExecutePendingTasks(gomock.Any(), 10).
			DoAndReturn(func(context.Context, int) error {
				started <- struct{}{}
				return errors.New("service error")
			})

		req := httptest.NewRequest(http.MethodGet, "/api/cron?max_tasks=10", nil)
		rec := httptest.NewRecorder()

		handler.ExecutePendingTasks(rec, req)

		// The response is sent before the run finishes, so a failure can only
		// be reported in the logs.
		assert.Equal(t, http.StatusAccepted, rec.Code)
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			t.Fatal("background execution never started")
		}
		waitForCronIdle(t)
	})
}

func TestTaskHandler_GetCronStatus(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTaskService := mocks.NewMockTaskService(ctrl)
	mockAuth := mocks.NewMockAuthService(ctrl)
	allowOwner(mockAuth)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Configure logger expectations
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	// Create test JWT secret
	jwtSecret := []byte("test-jwt-secret-key-for-testing-32bytes")

	handler := NewTaskHandler(
		mockTaskService,
		mockAuth,
		func() ([]byte, error) { return jwtSecret, nil },
		mockLogger,
		"test-secret",
		true,
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
	mockAuth := mocks.NewMockAuthService(ctrl)
	allowOwner(mockAuth)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Configure logger expectations
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()

	// Create test JWT secret
	jwtSecret := []byte("test-jwt-secret-key-for-testing-32bytes")

	handler := NewTaskHandler(
		mockTaskService,
		mockAuth,
		func() ([]byte, error) { return jwtSecret, nil },
		mockLogger,
		"test-secret",
		true,
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
	mockAuth := mocks.NewMockAuthService(ctrl)
	allowOwner(mockAuth)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Configure logger
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	jwtSecret := []byte("test-jwt-secret-key-for-testing-32bytes")
	handler := NewTaskHandler(
		mockTaskService,
		mockAuth,
		func() ([]byte, error) { return jwtSecret, nil },
		mockLogger,
		"test-secret",
		true,
	)

	t.Run("Success", func(t *testing.T) {
		mockTaskService.EXPECT().
			GetTask(gomock.Any(), "ws-1", "task-1").
			Return(&domain.Task{ID: "task-1", Type: "sync_integration"}, nil)
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
			GetTask(gomock.Any(), "ws-1", "task-not-found").
			Return(nil, domain.ErrTaskNotFound)

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
	mockAuth := mocks.NewMockAuthService(ctrl)
	allowOwner(mockAuth)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Configure logger
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	jwtSecret := []byte("test-jwt-secret-key-for-testing-32bytes")
	handler := NewTaskHandler(
		mockTaskService,
		mockAuth,
		func() ([]byte, error) { return jwtSecret, nil },
		mockLogger,
		"test-secret",
		true,
	)

	t.Run("Success", func(t *testing.T) {
		mockTaskService.EXPECT().
			GetTask(gomock.Any(), "ws-1", "task-1").
			Return(&domain.Task{ID: "task-1", Type: "send_broadcast"}, nil)
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
			GetTask(gomock.Any(), "ws-1", "task-not-found").
			Return(nil, domain.ErrTaskNotFound)

		body := `{"workspace_id": "ws-1", "id": "task-not-found"}`
		req := httptest.NewRequest(http.MethodPost, "/api/tasks.trigger", strings.NewReader(body))
		w := httptest.NewRecorder()

		handler.TriggerTask(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("Task already running", func(t *testing.T) {
		mockTaskService.EXPECT().
			GetTask(gomock.Any(), "ws-1", "task-running").
			Return(&domain.Task{ID: "task-running", Type: "send_broadcast"}, nil)
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

// TestTaskHandler_ExecuteTask_DetachedFromRequestContext pins the defect at the
// root of the incident: the handler ran the task on the request's context, so
// the dispatcher's HTTP client timeout cancelled a running broadcast mid-batch,
// aborting the enqueue transaction it was in.
func TestTaskHandler_ExecuteTask_DetachedFromRequestContext(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTaskService := mocks.NewMockTaskService(ctrl)
	mockAuth := mocks.NewMockAuthService(ctrl)
	allowOwner(mockAuth)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLoggerWithFields := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLoggerWithFields).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLoggerWithFields).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLoggerWithFields.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLoggerWithFields.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLoggerWithFields.EXPECT().Error(gomock.Any()).AnyTimes()

	handler := NewTaskHandler(mockTaskService, mockAuth, func() ([]byte, error) { return nil, nil }, mockLogger, "test-secret", true)

	task := &domain.Task{ID: "task-1", WorkspaceID: "ws-1", MaxRuntime: 50}
	mockTaskService.EXPECT().GetTask(gomock.Any(), "ws-1", "task-1").Return(task, nil)

	captured := make(chan context.Context, 1)
	mockTaskService.EXPECT().
		ExecuteTask(gomock.Any(), "ws-1", "task-1", gomock.Any()).
		DoAndReturn(func(ctx context.Context, _, _ string, _ time.Time) error {
			captured <- ctx
			return nil
		})

	// The dispatcher hangs up while the task is still being set up.
	reqCtx, cancel := context.WithCancel(context.Background())
	cancel()

	body := []byte(`{"workspace_id":"ws-1","id":"task-1"}`)
	req := newSignedExecuteRequest("test-secret", body).WithContext(reqCtx)
	rec := httptest.NewRecorder()

	handler.ExecuteTask(rec, req)

	execCtx := <-captured
	assert.NoError(t, execCtx.Err(),
		"task execution must outlive the dispatcher's connection; it is bounded by the task's own deadline")
}

// TestTaskHandler_TaskTypeResourceMapping pins the type→resource table the five
// authenticated routes authorize against. Tasks have no resource of their own,
// so this table is the whole gate: get the mapping wrong and triggering a
// broadcast send needs a segment grant.
func TestTaskHandler_TaskTypeResourceMapping(t *testing.T) {
	cases := []struct {
		taskType string
		resource domain.PermissionResource
	}{
		{"send_broadcast", domain.PermissionResourceBroadcasts},
		{"build_segment", domain.PermissionResourceSegments},
		{"check_segment_recompute", domain.PermissionResourceSegments},
		{"process_contact_segment_queue", domain.PermissionResourceSegments},
		{"sync_integration", domain.PermissionResourceWorkspace},
		{domain.WebAnalyticsBackfillTaskType, domain.PermissionResourceWebAnalytics},
	}

	for _, tc := range cases {
		t.Run(tc.taskType, func(t *testing.T) {
			resource, ok := taskTypeResource(tc.taskType)
			assert.True(t, ok)
			assert.Equal(t, tc.resource, resource)

			// The owning grant authorizes it...
			granted := memberWith("ws-1", domain.UserPermissions{
				tc.resource: domain.ResourcePermissions{Read: true, Write: true},
			})
			assert.NoError(t, authorizeTaskType(granted, tc.taskType, domain.PermissionTypeRead))
			assert.NoError(t, authorizeTaskType(granted, tc.taskType, domain.PermissionTypeWrite))

			// ...and every other grant does not.
			for _, other := range domain.AllPermissionResources {
				if other == tc.resource {
					continue
				}
				denied := memberWith("ws-1", domain.UserPermissions{
					other: domain.ResourcePermissions{Read: true, Write: true},
				})
				err := authorizeTaskType(denied, tc.taskType, domain.PermissionTypeRead)
				assert.IsType(t, &domain.PermissionError{}, err, "%s must not authorize %s", other, tc.taskType)
			}
		})
	}
}

// TestTaskHandler_UnknownTaskTypeFailsClosed pins that a type with no owning
// resource is denied for everyone, the workspace owner included: there is no
// grant that describes it, and consulting HasPermission would hand it to the
// role that short-circuits to true.
func TestTaskHandler_UnknownTaskTypeFailsClosed(t *testing.T) {
	_, ok := taskTypeResource("import_contacts")
	assert.False(t, ok, "a type with no processor and no mapping must not resolve")

	owner := &domain.UserWorkspace{UserID: "user-1", WorkspaceID: "ws-1", Role: "owner"}
	full := memberWith("ws-1", domain.NewFullPermissions())

	for _, uw := range []*domain.UserWorkspace{owner, full} {
		for _, permission := range []domain.PermissionType{domain.PermissionTypeRead, domain.PermissionTypeWrite} {
			err := authorizeTaskType(uw, "import_contacts", permission)
			assert.IsType(t, &domain.PermissionError{}, err)
		}
	}

	// And it reaches the routes: a get on an unmapped task is 403, not 200.
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	handler, mockTaskService, mockAuth := newTaskHandlerForAuth(t, ctrl)
	allowOwner(mockAuth)
	mockTaskService.EXPECT().GetTask(gomock.Any(), "ws-1", "task-1").
		Return(&domain.Task{ID: "task-1", Type: "import_contacts"}, nil)

	rec := httptest.NewRecorder()
	handler.GetTask(rec, httptest.NewRequest(http.MethodGet, "/api/tasks.get?workspace_id=ws-1&id=task-1", nil))

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// TestTaskHandler_PermissionEnforcement grants the OPPOSITE permission per case
// and asserts the route denies. read for get/list, write for delete/reset/trigger.
func TestTaskHandler_PermissionEnforcement(t *testing.T) {
	sendBroadcast := &domain.Task{ID: "task-1", WorkspaceID: "ws-1", Type: "send_broadcast"}

	testCases := []struct {
		name string
		// granted is what the caller holds — never what the route needs.
		granted domain.UserPermissions
		expect  func(*mocks.MockTaskService)
		call    func(*TaskHandler, *httptest.ResponseRecorder)
	}{
		{
			name:    "get needs broadcasts read",
			granted: domain.UserPermissions{domain.PermissionResourceBroadcasts: {Write: true}},
			expect: func(m *mocks.MockTaskService) {
				m.EXPECT().GetTask(gomock.Any(), "ws-1", "task-1").Return(sendBroadcast, nil)
			},
			call: func(h *TaskHandler, rec *httptest.ResponseRecorder) {
				h.GetTask(rec, httptest.NewRequest(http.MethodGet, "/api/tasks.get?workspace_id=ws-1&id=task-1", nil))
			},
		},
		{
			name:    "delete needs broadcasts write",
			granted: domain.UserPermissions{domain.PermissionResourceBroadcasts: {Read: true}},
			expect: func(m *mocks.MockTaskService) {
				m.EXPECT().GetTask(gomock.Any(), "ws-1", "task-1").Return(sendBroadcast, nil)
			},
			call: func(h *TaskHandler, rec *httptest.ResponseRecorder) {
				h.DeleteTask(rec, httptest.NewRequest(http.MethodPost, "/api/tasks.delete?workspace_id=ws-1&id=task-1", nil))
			},
		},
		{
			name:    "reset needs broadcasts write",
			granted: domain.UserPermissions{domain.PermissionResourceBroadcasts: {Read: true}},
			expect: func(m *mocks.MockTaskService) {
				m.EXPECT().GetTask(gomock.Any(), "ws-1", "task-1").Return(sendBroadcast, nil)
			},
			call: func(h *TaskHandler, rec *httptest.ResponseRecorder) {
				h.ResetTask(rec, httptest.NewRequest(http.MethodPost, "/api/tasks.reset",
					strings.NewReader(`{"workspace_id":"ws-1","id":"task-1"}`)))
			},
		},
		{
			name:    "trigger needs broadcasts write",
			granted: domain.UserPermissions{domain.PermissionResourceBroadcasts: {Read: true}},
			expect: func(m *mocks.MockTaskService) {
				m.EXPECT().GetTask(gomock.Any(), "ws-1", "task-1").Return(sendBroadcast, nil)
			},
			call: func(h *TaskHandler, rec *httptest.ResponseRecorder) {
				h.TriggerTask(rec, httptest.NewRequest(http.MethodPost, "/api/tasks.trigger",
					strings.NewReader(`{"workspace_id":"ws-1","id":"task-1"}`)))
			},
		},
		{
			name:    "list of a named type needs that type's read",
			granted: domain.UserPermissions{domain.PermissionResourceBroadcasts: {Write: true}},
			expect:  func(m *mocks.MockTaskService) {},
			call: func(h *TaskHandler, rec *httptest.ResponseRecorder) {
				h.ListTasks(rec, httptest.NewRequest(http.MethodGet,
					"/api/tasks.list?workspace_id=ws-1&type=send_broadcast", nil))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			handler, mockTaskService, mockAuth := newTaskHandlerForAuth(t, ctrl)
			allowMember(mockAuth, memberWith("ws-1", tc.granted))
			tc.expect(mockTaskService)

			rec := httptest.NewRecorder()
			tc.call(handler, rec)

			assert.Equal(t, http.StatusForbidden, rec.Code)

			var body map[string]interface{}
			assert.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			assert.Equal(t, string(domain.PermissionResourceBroadcasts), body["resource"])
		})
	}
}

// TestTaskHandler_CrossTenantDenied pins the hole this closes: the workspace is
// named in the request, so before these gates any valid token reached any
// workspace's tasks by asking for it.
func TestTaskHandler_CrossTenantDenied(t *testing.T) {
	routes := []struct {
		name string
		call func(*TaskHandler, *httptest.ResponseRecorder)
	}{
		{"list", func(h *TaskHandler, rec *httptest.ResponseRecorder) {
			h.ListTasks(rec, httptest.NewRequest(http.MethodGet, "/api/tasks.list?workspace_id=ws-b", nil))
		}},
		{"get", func(h *TaskHandler, rec *httptest.ResponseRecorder) {
			h.GetTask(rec, httptest.NewRequest(http.MethodGet, "/api/tasks.get?workspace_id=ws-b&id=task-1", nil))
		}},
		{"delete", func(h *TaskHandler, rec *httptest.ResponseRecorder) {
			h.DeleteTask(rec, httptest.NewRequest(http.MethodPost, "/api/tasks.delete?workspace_id=ws-b&id=task-1", nil))
		}},
		{"reset", func(h *TaskHandler, rec *httptest.ResponseRecorder) {
			h.ResetTask(rec, httptest.NewRequest(http.MethodPost, "/api/tasks.reset",
				strings.NewReader(`{"workspace_id":"ws-b","id":"task-1"}`)))
		}},
		{"trigger", func(h *TaskHandler, rec *httptest.ResponseRecorder) {
			h.TriggerTask(rec, httptest.NewRequest(http.MethodPost, "/api/tasks.trigger",
				strings.NewReader(`{"workspace_id":"ws-b","id":"task-1"}`)))
		}},
	}

	for _, route := range routes {
		t.Run(route.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			handler, _, mockAuth := newTaskHandlerForAuth(t, ctrl)
			// A full-access membership — of the other workspace.
			allowMember(mockAuth, memberWith("ws-a", domain.NewFullPermissions()))

			rec := httptest.NewRecorder()
			route.call(handler, rec)

			assert.Equal(t, http.StatusForbidden, rec.Code)
		})
	}
}

// TestTaskHandler_ListTasks_NarrowsToReadableTypes pins how a listing that names
// no type is answered: the filter is narrowed to what the caller can read, so
// total_count stays honest, and a caller that can read nothing gets nothing.
func TestTaskHandler_ListTasks_NarrowsToReadableTypes(t *testing.T) {
	t.Run("filter carries only the readable types", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		handler, mockTaskService, mockAuth := newTaskHandlerForAuth(t, ctrl)
		allowMember(mockAuth, memberWith("ws-1", domain.UserPermissions{
			domain.PermissionResourceSegments: {Read: true},
		}))

		mockTaskService.EXPECT().ListTasks(gomock.Any(), "ws-1", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, filter domain.TaskFilter) (*domain.TaskListResponse, error) {
				assert.Equal(t, []string{"build_segment", "check_segment_recompute", "process_contact_segment_queue"}, filter.Type)
				return &domain.TaskListResponse{Tasks: []*domain.Task{}}, nil
			})

		rec := httptest.NewRecorder()
		handler.ListTasks(rec, httptest.NewRequest(http.MethodGet, "/api/tasks.list?workspace_id=ws-1", nil))

		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("a caller that can read no type is answered with nothing", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		handler, _, mockAuth := newTaskHandlerForAuth(t, ctrl)
		allowMember(mockAuth, memberWith("ws-1", domain.UserPermissions{
			domain.PermissionResourceContacts: {Read: true},
		}))

		rec := httptest.NewRecorder()
		handler.ListTasks(rec, httptest.NewRequest(http.MethodGet, "/api/tasks.list?workspace_id=ws-1", nil))

		// The service is never called: an empty type filter would have meant
		// "every type".
		assert.Equal(t, http.StatusOK, rec.Code)

		var response domain.TaskListResponse
		assert.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
		assert.Empty(t, response.Tasks)
		assert.Equal(t, 0, response.TotalCount)
	})
}

// TestTaskHandler_ExecuteTaskSignature covers the dispatch endpoint's only
// authentication.
func TestTaskHandler_ExecuteTaskSignature(t *testing.T) {
	const secretKey = "test-secret-key"
	body := []byte(`{"workspace_id":"ws-1","id":"task-1"}`)

	t.Run("a correctly signed dispatch runs the task", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		handler, mockTaskService, _ := newTaskHandlerForAuth(t, ctrl)
		mockTaskService.EXPECT().GetTask(gomock.Any(), "ws-1", "task-1").
			Return(&domain.Task{ID: "task-1", MaxRuntime: 60}, nil)
		mockTaskService.EXPECT().ExecuteTask(gomock.Any(), "ws-1", "task-1", gomock.Any()).Return(nil)

		rec := httptest.NewRecorder()
		handler.ExecuteTask(rec, newSignedExecuteRequest(secretKey, body))

		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("an unsigned dispatch is rejected", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		handler, _, _ := newTaskHandlerForAuth(t, ctrl)

		req := httptest.NewRequest(http.MethodPost, "/api/tasks.execute", bytes.NewReader(body))
		rec := httptest.NewRecorder()
		handler.ExecuteTask(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("a stale timestamp is rejected", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		handler, _, _ := newTaskHandlerForAuth(t, ctrl)

		req := httptest.NewRequest(http.MethodPost, "/api/tasks.execute", bytes.NewReader(body))
		signExecuteRequest(req, secretKey, body,
			time.Now().Add(-domain.TaskExecuteSignatureMaxSkew-time.Minute))
		rec := httptest.NewRecorder()
		handler.ExecuteTask(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("a signature for another task id is rejected", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		handler, _, _ := newTaskHandlerForAuth(t, ctrl)

		// The captured dispatch's headers, replayed over a different body. This
		// is what the bare-path signature form would have allowed for five
		// minutes: same path, any task id.
		captured := newSignedExecuteRequest(secretKey, body)
		other := []byte(`{"workspace_id":"ws-1","id":"task-2"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/tasks.execute", bytes.NewReader(other))
		req.Header.Set(domain.TaskExecuteTimestampHeader, captured.Header.Get(domain.TaskExecuteTimestampHeader))
		req.Header.Set(domain.TaskExecuteSignatureHeader, captured.Header.Get(domain.TaskExecuteSignatureHeader))

		rec := httptest.NewRecorder()
		handler.ExecuteTask(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("a signature under another key is rejected", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		handler, _, _ := newTaskHandlerForAuth(t, ctrl)

		req := httptest.NewRequest(http.MethodPost, "/api/tasks.execute", bytes.NewReader(body))
		signExecuteRequest(req, "some-other-installations-secret", body, time.Now())
		rec := httptest.NewRecorder()
		handler.ExecuteTask(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("an installation with no secret key refuses every dispatch", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		handler, _, _ := newTaskHandlerForAuth(t, ctrl)
		handler.secretKey = ""

		rec := httptest.NewRecorder()
		handler.ExecuteTask(rec, newSignedExecuteRequest("", body))

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("execution carries no user context", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// The auth service is never consulted: the dispatcher has no session,
		// which is the whole reason the five gates above live in the handler.
		handler, mockTaskService, mockAuth := newTaskHandlerForAuth(t, ctrl)
		mockAuth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), gomock.Any()).Times(0)

		mockTaskService.EXPECT().GetTask(gomock.Any(), "ws-1", "task-1").
			Return(&domain.Task{ID: "task-1", MaxRuntime: 60, Type: "send_broadcast"}, nil)
		mockTaskService.EXPECT().ExecuteTask(gomock.Any(), "ws-1", "task-1", gomock.Any()).Return(nil)

		rec := httptest.NewRecorder()
		handler.ExecuteTask(rec, newSignedExecuteRequest(secretKey, body))

		assert.Equal(t, http.StatusOK, rec.Code)
	})
}
