package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errorMessage reads the "error" field of a JSON error response.
func errorMessage(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	msg, _ := body["error"].(string)
	return msg
}

// A failure to authenticate that is not a refusal is a server fault: 500, with no
// detail, and the task service is never reached (strict gomock, no expectation).
func TestTaskHandler_AuthenticationFailureIsAServerError(t *testing.T) {
	ctrl := gomock.NewController(t)
	handler, _, mockAuth := newTaskHandlerForAuth(t, ctrl)
	mockAuth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "ws-1").
		Return(nil, nil, nil, errors.New("database unavailable"))

	rec := httptest.NewRecorder()
	handler.GetTask(rec, httptest.NewRequest(http.MethodGet, "/api/tasks.get?workspace_id=ws-1&id=task-1", nil))

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Equal(t, "Failed to get task", errorMessage(t, rec))
}

// The task a mutation names is read to know which permission it needs: a failed read
// stops the mutation with a 500, and neither reset nor trigger reaches the service.
func TestTaskHandler_TaskLookupFailureStopsTheMutation(t *testing.T) {
	routes := map[string]func(h *TaskHandler, w http.ResponseWriter, r *http.Request){
		"reset":   func(h *TaskHandler, w http.ResponseWriter, r *http.Request) { h.ResetTask(w, r) },
		"trigger": func(h *TaskHandler, w http.ResponseWriter, r *http.Request) { h.TriggerTask(w, r) },
	}
	for name, call := range routes {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			handler, mockTaskService, mockAuth := newTaskHandlerForAuth(t, ctrl)
			allowOwner(mockAuth)
			mockTaskService.EXPECT().GetTask(gomock.Any(), "ws-1", "task-1").Return(nil, errors.New("connection reset"))

			rec := httptest.NewRecorder()
			call(handler, rec, httptest.NewRequest(http.MethodPost, "/api/tasks."+name,
				strings.NewReader(`{"workspace_id": "ws-1", "id": "task-1"}`)))

			assert.Equal(t, http.StatusInternalServerError, rec.Code)
			assert.Equal(t, "Failed to "+name+" task", errorMessage(t, rec))
		})
	}
}

func TestTaskHandler_DeleteTask_GoneBetweenCheckAndDelete(t *testing.T) {
	ctrl := gomock.NewController(t)
	handler, mockTaskService, mockAuth := newTaskHandlerForAuth(t, ctrl)
	allowOwner(mockAuth)
	mockTaskService.EXPECT().GetTask(gomock.Any(), "ws-1", "task-1").
		Return(&domain.Task{ID: "task-1", Type: "sync_integration"}, nil)
	mockTaskService.EXPECT().DeleteTask(gomock.Any(), "ws-1", "task-1").Return(errors.New("task not found"))

	rec := httptest.NewRecorder()
	handler.DeleteTask(rec, httptest.NewRequest(http.MethodPost, "/api/tasks.delete?workspace_id=ws-1&id=task-1", nil))

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// The dispatcher takes a 409 for a run already under way and logs anything else as an
// error, so a concurrent run must answer 409 and a real failure must not.
func TestTaskHandler_ExecuteTask_FailureStatuses(t *testing.T) {
	const secretKey = "test-secret-key"
	body := []byte(`{"workspace_id": "ws-1", "id": "task-1"}`)

	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantError  string
	}{
		{
			name:       "the task disappeared",
			err:        &domain.ErrNotFound{Entity: "task", ID: "task-1"},
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "already running",
			err:        &domain.ErrTaskAlreadyRunning{TaskID: "task-1"},
			wantStatus: http.StatusConflict,
		},
		{
			name:       "already running, wrapped",
			err:        &domain.ErrTaskExecution{TaskID: "task-1", Reason: "processor failed", Err: &domain.ErrTaskAlreadyRunning{TaskID: "task-1"}},
			wantStatus: http.StatusConflict,
			wantError:  "Task is already being processed",
		},
		{
			name:       "claimed by another executor",
			err:        &domain.ErrTaskExecution{TaskID: "task-1", Reason: "failed to mark task as running", Err: errors.New("no row updated")},
			wantStatus: http.StatusConflict,
			wantError:  "Task is already being processed",
		},
		{
			name:       "an unexpected error",
			err:        errors.New("boom"),
			wantStatus: http.StatusInternalServerError,
			wantError:  "Failed to execute task",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			handler, mockTaskService, _ := newTaskHandlerForAuth(t, ctrl)
			mockTaskService.EXPECT().GetTask(gomock.Any(), "ws-1", "task-1").Return(&domain.Task{ID: "task-1", MaxRuntime: 50}, nil)
			mockTaskService.EXPECT().ExecuteTask(gomock.Any(), "ws-1", "task-1", gomock.Any()).Return(tc.err)

			rec := httptest.NewRecorder()
			handler.ExecuteTask(rec, newSignedExecuteRequest(secretKey, body))

			assert.Equal(t, tc.wantStatus, rec.Code)
			if tc.wantError != "" {
				assert.Equal(t, tc.wantError, errorMessage(t, rec))
			}
		})
	}
}

// failingBody is a request body whose read breaks off.
type failingBody struct{}

func (failingBody) Read([]byte) (int, error) { return 0, errors.New("connection reset") }

// A body that cannot be read is refused before the signature is even checked.
func TestTaskHandler_ExecuteTask_UnreadableBody(t *testing.T) {
	ctrl := gomock.NewController(t)
	handler, _, _ := newTaskHandlerForAuth(t, ctrl)

	rec := httptest.NewRecorder()
	handler.ExecuteTask(rec, httptest.NewRequest(http.MethodPost, "/api/tasks.execute", failingBody{}))

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "Invalid request body", errorMessage(t, rec))
}

func TestTaskHandler_ResetAndTrigger_ServiceFailures(t *testing.T) {
	cases := []struct {
		name       string
		route      string
		err        error
		wantStatus int
		wantError  string
	}{
		{name: "reset: task gone", route: "reset", err: domain.ErrTaskNotFound, wantStatus: http.StatusNotFound, wantError: "Task not found"},
		{name: "reset: refused", route: "reset", err: domain.NewValidationError("task is not a recurring task"), wantStatus: http.StatusBadRequest, wantError: "validation error: task is not a recurring task"},
		{name: "trigger: task gone", route: "trigger", err: domain.ErrTaskNotFound, wantStatus: http.StatusNotFound, wantError: "Task not found"},
		{name: "trigger: refused", route: "trigger", err: domain.NewValidationError("task cannot be triggered in current status: completed"), wantStatus: http.StatusBadRequest, wantError: "validation error: task cannot be triggered in current status: completed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			handler, mockTaskService, mockAuth := newTaskHandlerForAuth(t, ctrl)
			allowOwner(mockAuth)
			mockTaskService.EXPECT().GetTask(gomock.Any(), "ws-1", "task-1").
				Return(&domain.Task{ID: "task-1", Type: "sync_integration"}, nil)

			req := httptest.NewRequest(http.MethodPost, "/api/tasks."+tc.route,
				strings.NewReader(`{"workspace_id": "ws-1", "id": "task-1"}`))
			rec := httptest.NewRecorder()
			if tc.route == "reset" {
				mockTaskService.EXPECT().ResetTask(gomock.Any(), "ws-1", "task-1").Return(tc.err)
				handler.ResetTask(rec, req)
			} else {
				mockTaskService.EXPECT().TriggerTask(gomock.Any(), "ws-1", "task-1").Return(tc.err)
				handler.TriggerTask(rec, req)
			}

			assert.Equal(t, tc.wantStatus, rec.Code)
			assert.Equal(t, tc.wantError, errorMessage(t, rec))
		})
	}
}
