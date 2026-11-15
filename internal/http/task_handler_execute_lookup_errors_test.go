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
	body := []byte(`{"workspace_id":"workspace1","id":"12312312-3123-4123-8123-123123123123"}`)
	dbErr := errors.New("pq: connection refused")

	newHandler := func(t *testing.T) (*TaskHandler, *mocks.MockTaskService, *pkgmocks.MockLogger, *[]map[string]interface{}) {
		handler, svc, _, log, fields := recordingTaskHandler(t)
		return handler, svc, log, fields
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
		handler, svc, log, fields := newHandler(t)
		svc.EXPECT().GetTask(gomock.Any(), "workspace1", "12312312-3123-4123-8123-123123123123").Return(nil, dbErr)
		log.EXPECT().Error("Failed to get task for execution").Times(1)

		code, msg := execute(t, handler)

		assert.Equal(t, http.StatusInternalServerError, code)
		assert.Equal(t, "Failed to get task", msg)
		assertErrorFieldLogged(t, *fields, dbErr.Error())
	})

	t.Run("the execution cannot read the task", func(t *testing.T) {
		handler, svc, log, fields := newHandler(t)
		log.EXPECT().Error("Failed to execute task").Times(1)
		svc.EXPECT().GetTask(gomock.Any(), "workspace1", "12312312-3123-4123-8123-123123123123").Return(&domain.Task{MaxRuntime: 60}, nil)
		svc.EXPECT().ExecuteTask(gomock.Any(), "workspace1", "12312312-3123-4123-8123-123123123123", gomock.Any()).
			Return(fmt.Errorf("failed to load task for execution: %w", dbErr))

		code, msg := execute(t, handler)

		assert.Equal(t, http.StatusInternalServerError, code)
		assert.Equal(t, "Failed to execute task", msg)
		assertErrorFieldLogged(t, *fields, dbErr.Error())
	})

	t.Run("the task is gone when the execution reads it", func(t *testing.T) {
		// Logged at Warn once, never at Error: the strict logger refuses anything else.
		ctrl := gomock.NewController(t)
		svc := mocks.NewMockTaskService(ctrl)
		log := pkgmocks.NewMockLogger(ctrl)
		log.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(log).AnyTimes()
		log.EXPECT().WithFields(gomock.Any()).Return(log).AnyTimes()
		log.EXPECT().Info(gomock.Any()).AnyTimes()
		log.EXPECT().Debug(gomock.Any()).AnyTimes()
		log.EXPECT().Warn("Task to execute not found").Times(1)
		handler := NewTaskHandler(svc, mocks.NewMockAuthService(ctrl),
			func() ([]byte, error) { return []byte("test-jwt-secret-key-for-testing-32bytes"), nil },
			log, secretKey, true)
		svc.EXPECT().GetTask(gomock.Any(), "workspace1", "12312312-3123-4123-8123-123123123123").Return(&domain.Task{MaxRuntime: 60}, nil)
		svc.EXPECT().ExecuteTask(gomock.Any(), "workspace1", "12312312-3123-4123-8123-123123123123", gomock.Any()).
			Return(&domain.ErrNotFound{Entity: "task", ID: "12312312-3123-4123-8123-123123123123"})

		code, msg := execute(t, handler)

		assert.Equal(t, http.StatusNotFound, code)
		assert.Equal(t, (&domain.ErrNotFound{Entity: "task", ID: "12312312-3123-4123-8123-123123123123"}).Error(), msg)
	})
}
