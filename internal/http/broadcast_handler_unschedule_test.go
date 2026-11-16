package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	http_handler "github.com/Notifuse/notifuse/internal/http"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// unschedule posts a raw JSON body, as an API client sends it, to HandleUnschedule.
func unschedule(handler *http_handler.BroadcastHandler, method, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/api/broadcasts.unschedule", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.HandleUnschedule(w, req)
	return w
}

func errorMessage(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	message, _ := body["error"].(string)
	return message
}

func TestHandleUnschedule(t *testing.T) {
	t.Run("sends the broadcast back to draft", func(t *testing.T) {
		handler, service, _, ctrl := setupBroadcastHandler(t)
		defer ctrl.Finish()
		service.EXPECT().UnscheduleBroadcast(gomock.Any(), &domain.UnscheduleBroadcastRequest{WorkspaceID: "w1", ID: "b1"}).Return(nil)

		w := unschedule(handler, http.MethodPost, `{"workspace_id":"w1","id":"b1"}`)
		assert.Equal(t, http.StatusOK, w.Code)
		var body map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Equal(t, true, body["success"])
	})

	t.Run("answers 400 with the reason when the sending has started", func(t *testing.T) {
		handler, service, _, ctrl := setupBroadcastHandler(t)
		defer ctrl.Finish()
		service.EXPECT().UnscheduleBroadcast(gomock.Any(), gomock.Any()).
			Return(domain.NewValidationError("the broadcast has started sending: cancel it instead"))

		w := unschedule(handler, http.MethodPost, `{"workspace_id":"w1","id":"b1"}`)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Equal(t, "the broadcast has started sending: cancel it instead", errorMessage(t, w))
	})

	t.Run("answers 400 without calling the service when the id is missing", func(t *testing.T) {
		handler, _, _, ctrl := setupBroadcastHandler(t)
		defer ctrl.Finish()

		w := unschedule(handler, http.MethodPost, `{"workspace_id":"w1"}`)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Equal(t, "broadcast id is required", errorMessage(t, w))
	})

	t.Run("answers 400 to a body that is not JSON", func(t *testing.T) {
		handler, _, _, ctrl := setupBroadcastHandler(t)
		defer ctrl.Finish()

		w := unschedule(handler, http.MethodPost, `{"workspace_id":`)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("answers 404 for a broadcast that does not exist", func(t *testing.T) {
		handler, service, _, ctrl := setupBroadcastHandler(t)
		defer ctrl.Finish()
		service.EXPECT().UnscheduleBroadcast(gomock.Any(), gomock.Any()).Return(&domain.ErrBroadcastNotFound{ID: "b1"})

		w := unschedule(handler, http.MethodPost, `{"workspace_id":"w1","id":"b1"}`)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("answers 403 without write access to broadcasts", func(t *testing.T) {
		handler, service, _, ctrl := setupBroadcastHandler(t)
		defer ctrl.Finish()
		service.EXPECT().UnscheduleBroadcast(gomock.Any(), gomock.Any()).Return(domain.NewPermissionError(
			domain.PermissionResourceBroadcasts, domain.PermissionTypeWrite, "Insufficient permissions"))

		w := unschedule(handler, http.MethodPost, `{"workspace_id":"w1","id":"b1"}`)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("answers 500 on any other failure", func(t *testing.T) {
		handler, service, logger, ctrl := setupBroadcastHandler(t)
		defer ctrl.Finish()
		withField := pkgmocks.NewMockLogger(ctrl)
		logger.EXPECT().WithField("error", "db down").Return(withField)
		withField.EXPECT().Error("Failed to send the broadcast back to draft")
		service.EXPECT().UnscheduleBroadcast(gomock.Any(), gomock.Any()).Return(errors.New("db down"))

		w := unschedule(handler, http.MethodPost, `{"workspace_id":"w1","id":"b1"}`)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("answers 405 to a GET", func(t *testing.T) {
		handler, _, _, ctrl := setupBroadcastHandler(t)
		defer ctrl.Finish()

		w := unschedule(handler, http.MethodGet, ``)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

// In demo, the route refuses before authentication, as broadcasts.schedule does.
func TestHandleUnschedule_RefusedInDemo(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	service := mocks.NewMockBroadcastService(ctrl)
	jwtSecret := []byte("test-jwt-secret-key-for-testing-32bytes")
	handler := http_handler.NewBroadcastHandler(service, func() ([]byte, error) { return jwtSecret, nil }, pkgmocks.NewMockLogger(ctrl), true)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/broadcasts.unschedule", strings.NewReader(`{"workspace_id":"w1","id":"b1"}`)).
		WithContext(context.Background())
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, errorMessage(t, w), "demo mode")
}
