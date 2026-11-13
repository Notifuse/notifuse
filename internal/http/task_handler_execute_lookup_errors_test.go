package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A dispatch whose task cannot be read because of a failure on our side answers 500
// with a generic message; the internal error stays in the logs.
func TestTaskHandler_ExecuteTask_LookupErrors(t *testing.T) {
	const secretKey = "test-secret-key"
	body := []byte(`{"workspace_id":"workspace1","id":"task123"}`)
	dbErr := errors.New("pq: connection refused")

	newHandler := func(t *testing.T) (*TaskHandler, *mocks.MockTaskService) {
		ctrl := gomock.NewController(t)
		mockTaskService := mocks.NewMockTaskService(ctrl)
		mockAuth := mocks.NewMockAuthService(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)
		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
		handler := NewTaskHandler(mockTaskService, mockAuth, func() ([]byte, error) { return nil, nil }, mockLogger, secretKey, true)
		return handler, mockTaskService
	}

	execute := func(t *testing.T, handler *TaskHandler) (int, string) {
		rec := httptest.NewRecorder()
		handler.ExecuteTask(rec, newSignedExecuteRequest(secretKey, body))
		var resp struct {
			Error string `json:"error"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp), rec.Body.String())
		return rec.Code, resp.Error
	}

	t.Run("the task cannot be read", func(t *testing.T) {
		handler, svc := newHandler(t)
		svc.EXPECT().GetTask(gomock.Any(), "workspace1", "task123").Return(nil, dbErr)

		code, msg := execute(t, handler)

		assert.Equal(t, http.StatusInternalServerError, code)
		assert.Equal(t, "Failed to get task", msg)
	})

	t.Run("the execution cannot read the task", func(t *testing.T) {
		handler, svc := newHandler(t)
		svc.EXPECT().GetTask(gomock.Any(), "workspace1", "task123").Return(&domain.Task{MaxRuntime: 60}, nil)
		svc.EXPECT().ExecuteTask(gomock.Any(), "workspace1", "task123", gomock.Any()).
			Return(fmt.Errorf("failed to load task for execution: %w", dbErr))

		code, msg := execute(t, handler)

		assert.Equal(t, http.StatusInternalServerError, code)
		assert.Equal(t, "Failed to execute task", msg)
	})

	t.Run("the task is gone when the execution reads it", func(t *testing.T) {
		handler, svc := newHandler(t)
		svc.EXPECT().GetTask(gomock.Any(), "workspace1", "task123").Return(&domain.Task{MaxRuntime: 60}, nil)
		svc.EXPECT().ExecuteTask(gomock.Any(), "workspace1", "task123", gomock.Any()).
			Return(&domain.ErrNotFound{Entity: "task", ID: "task123"})

		code, _ := execute(t, handler)

		assert.Equal(t, http.StatusNotFound, code)
	})
}
