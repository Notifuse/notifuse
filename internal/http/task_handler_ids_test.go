package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

const idsTaskID = "11111111-1111-4111-8111-111111111111"

// strictTaskHandler builds a handler whose logger accepts Error and Debug only where a test
// says so.
func strictTaskHandler(t *testing.T) (*TaskHandler, *mocks.MockTaskService, *mocks.MockAuthService, *pkgmocks.MockLogger) {
	t.Helper()
	ctrl := gomock.NewController(t)
	svc := mocks.NewMockTaskService(ctrl)
	auth := mocks.NewMockAuthService(ctrl)
	log := pkgmocks.NewMockLogger(ctrl)
	log.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(log).AnyTimes()
	log.EXPECT().WithFields(gomock.Any()).Return(log).AnyTimes()
	log.EXPECT().Info(gomock.Any()).AnyTimes()
	log.EXPECT().Warn(gomock.Any()).AnyTimes()
	handler := NewTaskHandler(svc, auth, func() ([]byte, error) { return []byte("test-jwt-secret-key-for-testing-32bytes"), nil },
		log, "test-secret-key", true)
	return handler, svc, auth, log
}

// recordingTaskHandler is strictTaskHandler with the fields of each WithFields call kept, so
// a test can check what an Error log carried.
func recordingTaskHandler(t *testing.T) (*TaskHandler, *mocks.MockTaskService, *mocks.MockAuthService, *pkgmocks.MockLogger, *[]map[string]interface{}) {
	t.Helper()
	ctrl := gomock.NewController(t)
	svc := mocks.NewMockTaskService(ctrl)
	auth := mocks.NewMockAuthService(ctrl)
	log := pkgmocks.NewMockLogger(ctrl)
	var fields []map[string]interface{}
	log.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(log).AnyTimes()
	log.EXPECT().WithFields(gomock.Any()).DoAndReturn(func(f map[string]interface{}) *pkgmocks.MockLogger {
		fields = append(fields, f)
		return log
	}).AnyTimes()
	log.EXPECT().Info(gomock.Any()).AnyTimes()
	log.EXPECT().Warn(gomock.Any()).AnyTimes()
	handler := NewTaskHandler(svc, auth, func() ([]byte, error) { return []byte("test-jwt-secret-key-for-testing-32bytes"), nil },
		log, "test-secret-key", true)
	return handler, svc, auth, log, &fields
}

// assertErrorFieldLogged checks that one of the logged field sets carries the internal error.
func assertErrorFieldLogged(t *testing.T, fields []map[string]interface{}, text string) {
	t.Helper()
	for _, f := range fields {
		if e, ok := f["error"].(string); ok && strings.Contains(e, text) {
			return
		}
	}
	t.Errorf("no log carried the error %q: %v", text, fields)
}

// Every task route refuses an ID that is not a UUID with a 400, before any service call.
func TestTaskHandler_TaskIDMustBeAUUID(t *testing.T) {
	routes := map[string]func(h *TaskHandler) *httptest.ResponseRecorder{
		"get": func(h *TaskHandler) *httptest.ResponseRecorder {
			rec := httptest.NewRecorder()
			h.GetTask(rec, httptest.NewRequest(http.MethodGet, "/api/tasks.get?workspace_id=ws-1&id=not-a-uuid", nil))
			return rec
		},
		"delete": func(h *TaskHandler) *httptest.ResponseRecorder {
			rec := httptest.NewRecorder()
			h.DeleteTask(rec, httptest.NewRequest(http.MethodPost, "/api/tasks.delete?workspace_id=ws-1&id=not-a-uuid", nil))
			return rec
		},
		"reset": func(h *TaskHandler) *httptest.ResponseRecorder {
			rec := httptest.NewRecorder()
			h.ResetTask(rec, httptest.NewRequest(http.MethodPost, "/api/tasks.reset", strings.NewReader(`{"workspace_id":"ws-1","id":"not-a-uuid"}`)))
			return rec
		},
		"trigger": func(h *TaskHandler) *httptest.ResponseRecorder {
			rec := httptest.NewRecorder()
			h.TriggerTask(rec, httptest.NewRequest(http.MethodPost, "/api/tasks.trigger", strings.NewReader(`{"workspace_id":"ws-1","id":"not-a-uuid"}`)))
			return rec
		},
		"execute": func(h *TaskHandler) *httptest.ResponseRecorder {
			rec := httptest.NewRecorder()
			h.ExecuteTask(rec, newSignedExecuteRequest("test-secret-key", []byte(`{"workspace_id":"ws-1","id":"not-a-uuid"}`)))
			return rec
		},
	}
	for name, call := range routes {
		t.Run(name, func(t *testing.T) {
			handler, _, _, _ := strictTaskHandler(t) // no service, auth or Error call expected
			rec := call(handler)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Equal(t, "task id must be a UUID", errorMessage(t, rec))
		})
	}
}

// Only domain.ErrTaskNotFound is a 404: another error whose text says "not found" is a
// failure on our side.
func TestTaskHandler_NotFoundOnlyForTheSentinel(t *testing.T) {
	lookalike := errors.New("relation \"tasks\" not found")

	t.Run("get", func(t *testing.T) {
		handler, svc, auth, log := strictTaskHandler(t)
		allowOwner(auth)
		svc.EXPECT().GetTask(gomock.Any(), "ws-1", idsTaskID).Return(nil, lookalike)
		log.EXPECT().Error("Failed to get task").Times(1)

		rec := httptest.NewRecorder()
		handler.GetTask(rec, httptest.NewRequest(http.MethodGet, "/api/tasks.get?workspace_id=ws-1&id="+idsTaskID, nil))
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})

	t.Run("delete", func(t *testing.T) {
		handler, svc, auth, log := strictTaskHandler(t)
		allowOwner(auth)
		svc.EXPECT().GetTask(gomock.Any(), "ws-1", idsTaskID).Return(&domain.Task{ID: idsTaskID, Type: "sync_integration"}, nil)
		svc.EXPECT().DeleteTask(gomock.Any(), "ws-1", idsTaskID).Return(lookalike)
		log.EXPECT().Error("Failed to delete task").Times(1)

		rec := httptest.NewRecorder()
		handler.DeleteTask(rec, httptest.NewRequest(http.MethodPost, "/api/tasks.delete?workspace_id=ws-1&id="+idsTaskID, nil))
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})

	t.Run("the authorization read", func(t *testing.T) {
		handler, svc, auth, log := strictTaskHandler(t)
		allowOwner(auth)
		svc.EXPECT().GetTask(gomock.Any(), "ws-1", idsTaskID).Return(nil, fmt.Errorf("failed to get task: %w", lookalike))
		log.EXPECT().Error("Failed to load task for authorization").Times(1)

		rec := httptest.NewRecorder()
		handler.ResetTask(rec, httptest.NewRequest(http.MethodPost, "/api/tasks.reset",
			strings.NewReader(`{"workspace_id":"ws-1","id":"`+idsTaskID+`"}`)))
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}

// A reset or trigger the client cancelled is logged at Debug, not Error: from the error, or
// from the request's context when the error is something else (a statement the driver
// cancelled). The authorization read follows the same rule.
func TestTaskHandler_CancelledByTheClient(t *testing.T) {
	body := `{"workspace_id":"ws-1","id":"` + idsTaskID + `"}`
	calls := map[string]struct {
		serve  func(h *TaskHandler, w http.ResponseWriter, r *http.Request)
		expect func(svc *mocks.MockTaskService, err error)
	}{
		"reset": {
			serve: func(h *TaskHandler, w http.ResponseWriter, r *http.Request) { h.ResetTask(w, r) },
			expect: func(svc *mocks.MockTaskService, err error) {
				svc.EXPECT().ResetTask(gomock.Any(), "ws-1", idsTaskID).Return(err)
			},
		},
		"trigger": {
			serve: func(h *TaskHandler, w http.ResponseWriter, r *http.Request) { h.TriggerTask(w, r) },
			expect: func(svc *mocks.MockTaskService, err error) {
				svc.EXPECT().TriggerTask(gomock.Any(), "ws-1", idsTaskID).Return(err)
			},
		},
	}
	for name, c := range calls {
		t.Run(name+": a wrapped context.Canceled", func(t *testing.T) {
			handler, svc, auth, log := strictTaskHandler(t)
			allowOwner(auth)
			svc.EXPECT().GetTask(gomock.Any(), "ws-1", idsTaskID).Return(&domain.Task{ID: idsTaskID, Type: "sync_integration"}, nil)
			c.expect(svc, fmt.Errorf("failed to %s task: %w", name, context.Canceled))
			log.EXPECT().Debug(gomock.Any()).Times(1)

			rec := httptest.NewRecorder()
			c.serve(handler, rec, httptest.NewRequest(http.MethodPost, "/api/tasks."+name, strings.NewReader(body)))
			assert.Equal(t, http.StatusInternalServerError, rec.Code)
		})

		t.Run(name+": the request's context is done", func(t *testing.T) {
			handler, svc, auth, log := strictTaskHandler(t)
			allowOwner(auth)
			ctx, cancel := context.WithCancel(context.Background())
			svc.EXPECT().GetTask(gomock.Any(), "ws-1", idsTaskID).DoAndReturn(func(context.Context, string, string) (*domain.Task, error) {
				cancel() // the client leaves while the request runs
				return &domain.Task{ID: idsTaskID, Type: "sync_integration"}, nil
			})
			c.expect(svc, errors.New("sql: transaction has already been committed or rolled back"))
			log.EXPECT().Debug(gomock.Any()).Times(1)

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/tasks."+name, strings.NewReader(body)).WithContext(ctx)
			c.serve(handler, rec, req)
			assert.Equal(t, http.StatusInternalServerError, rec.Code)
		})
	}

	t.Run("the authorization read, the request's context done", func(t *testing.T) {
		handler, svc, auth, log := strictTaskHandler(t)
		allowOwner(auth)
		ctx, cancel := context.WithCancel(context.Background())
		svc.EXPECT().GetTask(gomock.Any(), "ws-1", idsTaskID).DoAndReturn(func(context.Context, string, string) (*domain.Task, error) {
			cancel()
			return nil, errors.New("pq: canceling statement due to user request")
		})
		log.EXPECT().Debug(gomock.Any()).Times(1)

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/tasks.trigger", strings.NewReader(body)).WithContext(ctx)
		handler.TriggerTask(rec, req)
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}
