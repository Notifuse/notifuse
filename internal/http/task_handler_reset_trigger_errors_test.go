package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// How reset and trigger answer each kind of service error, once the task was found
// and the caller may change it. Only the failure on our side is logged at Error, once, with
// the internal error (the logger mock refuses any other Error).
func TestTaskHandler_ResetAndTrigger_ErrorStatuses(t *testing.T) {
	dbFailure := errors.New("pq: connection refused")
	cases := []struct {
		name       string
		err        func(route string) error
		wantStatus int
		wantError  func(route string) string
	}{
		{
			name:       "the task is gone",
			err:        func(string) error { return domain.ErrTaskNotFound },
			wantStatus: http.StatusNotFound,
			wantError:  func(string) string { return "Task not found" },
		},
		{
			name:       "the task is gone, wrapped",
			err:        func(route string) error { return fmt.Errorf("failed to %s task: %w", route, domain.ErrTaskNotFound) },
			wantStatus: http.StatusNotFound,
			wantError:  func(string) string { return "Task not found" },
		},
		{
			name:       "the task's state refuses it",
			err:        func(string) error { return domain.NewValidationError("task is not a recurring task") },
			wantStatus: http.StatusBadRequest,
			wantError:  func(string) string { return "validation error: task is not a recurring task" },
		},
		{
			// A failure on our side: a 500, and the internal text stays in the logs.
			name:       "the database fails",
			err:        func(route string) error { return fmt.Errorf("failed to %s task: %w", route, dbFailure) },
			wantStatus: http.StatusInternalServerError,
			wantError:  func(route string) string { return "Failed to " + route + " task" },
		},
	}
	for _, route := range []string{"reset", "trigger"} {
		for _, tc := range cases {
			t.Run(route+": "+tc.name, func(t *testing.T) {
				handler, mockTaskService, mockAuth, log, fields := recordingTaskHandler(t)
				allowOwner(mockAuth)
				if tc.wantStatus == http.StatusInternalServerError {
					log.EXPECT().Error("Failed to " + route + " task").Times(1)
				}
				mockTaskService.EXPECT().GetTask(gomock.Any(), "ws-1", "11111111-1111-4111-8111-111111111111").
					Return(&domain.Task{ID: "11111111-1111-4111-8111-111111111111", Type: "sync_integration"}, nil)
				err := tc.err(route)

				req := httptest.NewRequest(http.MethodPost, "/api/tasks."+route,
					strings.NewReader(`{"workspace_id": "ws-1", "id": "11111111-1111-4111-8111-111111111111"}`))
				rec := httptest.NewRecorder()
				if route == "reset" {
					mockTaskService.EXPECT().ResetTask(gomock.Any(), "ws-1", "11111111-1111-4111-8111-111111111111").Return(err)
					handler.ResetTask(rec, req)
				} else {
					mockTaskService.EXPECT().TriggerTask(gomock.Any(), "ws-1", "11111111-1111-4111-8111-111111111111").Return(err)
					handler.TriggerTask(rec, req)
				}

				assert.Equal(t, tc.wantStatus, rec.Code)
				var body map[string]interface{}
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
				assert.Equal(t, tc.wantError(route), body["error"])
				assert.NotContains(t, rec.Body.String(), "pq:", "no internal error text in the body")
				if tc.wantStatus == http.StatusInternalServerError {
					assertErrorFieldLogged(t, *fields, dbFailure.Error())
				}
			})
		}
	}
}
