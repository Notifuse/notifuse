package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func coverageWebAnalyticsHandler(t *testing.T) (*WebAnalyticsHandler, *mocks.MockWebAnalyticsService) {
	t.Helper()
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)
	svc := mocks.NewMockWebAnalyticsService(ctrl)
	return NewWebAnalyticsHandler(svc, nil, logger.NewLogger(), nil), svc
}

func TestCoverage_DecodeBackfillRequest(t *testing.T) {
	h, _ := coverageWebAnalyticsHandler(t)

	t.Run("GET is 405 and not ok", func(t *testing.T) {
		rr := httptest.NewRecorder()
		ws, ok := h.decodeBackfillRequest(rr, httptest.NewRequest(http.MethodGet, "/api/webAnalytics.backfillStart", strings.NewReader(`{"workspace_id":"ws1"}`)))
		assert.False(t, ok)
		assert.Empty(t, ws)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
		var body map[string]string
		require.NoError(t, json.NewDecoder(rr.Body).Decode(&body))
		assert.Equal(t, "Method not allowed", body["error"])
	})

	t.Run("invalid JSON is 400 and not ok", func(t *testing.T) {
		rr := httptest.NewRecorder()
		ws, ok := h.decodeBackfillRequest(rr, httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{not json`)))
		assert.False(t, ok)
		assert.Empty(t, ws)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
		var body map[string]string
		require.NoError(t, json.NewDecoder(rr.Body).Decode(&body))
		assert.Equal(t, "Invalid request body", body["error"])
	})

	t.Run("missing workspace_id is 400 and not ok", func(t *testing.T) {
		rr := httptest.NewRecorder()
		ws, ok := h.decodeBackfillRequest(rr, httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{}`)))
		assert.False(t, ok)
		assert.Empty(t, ws)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
		var body map[string]string
		require.NoError(t, json.NewDecoder(rr.Body).Decode(&body))
		assert.Equal(t, "workspace_id is required", body["error"])
	})

	t.Run("empty body is 400 invalid request body", func(t *testing.T) {
		rr := httptest.NewRecorder()
		_, ok := h.decodeBackfillRequest(rr, httptest.NewRequest(http.MethodPost, "/x", strings.NewReader("")))
		assert.False(t, ok)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
		var body map[string]string
		require.NoError(t, json.NewDecoder(rr.Body).Decode(&body))
		assert.Equal(t, "Invalid request body", body["error"])
	})

	t.Run("valid body returns workspace id", func(t *testing.T) {
		rr := httptest.NewRecorder()
		ws, ok := h.decodeBackfillRequest(rr, httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"workspace_id":"ws42"}`)))
		assert.True(t, ok)
		assert.Equal(t, "ws42", ws)
		assert.Equal(t, http.StatusOK, rr.Code, "no error status should be written on success")
	})
}

func TestCoverage_WriteBackfillError(t *testing.T) {
	h, _ := coverageWebAnalyticsHandler(t)

	t.Run("permission error is 403 with resource and permission", func(t *testing.T) {
		rr := httptest.NewRecorder()
		h.writeBackfillError(rr, "ws1", domain.NewPermissionError(domain.PermissionResourceWebAnalytics, domain.PermissionTypeWrite, "Insufficient permissions: write access to web analytics required"))
		assert.Equal(t, http.StatusForbidden, rr.Code)
		var body map[string]interface{}
		require.NoError(t, json.NewDecoder(rr.Body).Decode(&body))
		assert.Equal(t, "Insufficient permissions: write access to web analytics required", body["error"])
		assert.Equal(t, string(domain.PermissionResourceWebAnalytics), body["resource"])
		assert.Equal(t, string(domain.PermissionTypeWrite), body["permission"])
	})

	t.Run("revoked API key is 401", func(t *testing.T) {
		rr := httptest.NewRecorder()
		h.writeBackfillError(rr, "ws1", domain.ErrAPIKeyRevoked)
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
		var body map[string]string
		require.NoError(t, json.NewDecoder(rr.Body).Decode(&body))
		assert.Equal(t, "API key has been revoked", body["error"])
	})

	t.Run("generic error is 400 with the exact service message", func(t *testing.T) {
		rr := httptest.NewRecorder()
		h.writeBackfillError(rr, "ws1", errors.New("a backfill is already running"))
		assert.Equal(t, http.StatusBadRequest, rr.Code)
		var body map[string]string
		require.NoError(t, json.NewDecoder(rr.Body).Decode(&body))
		assert.Equal(t, "a backfill is already running", body["error"])
	})
}

func TestCoverage_HandleBackfillStart(t *testing.T) {
	t.Run("success forwards workspace and returns status verbatim", func(t *testing.T) {
		h, svc := coverageWebAnalyticsHandler(t)
		svc.EXPECT().BackfillStart(gomock.Any(), "ws1").
			Return(&domain.WebAnalyticsBackfillStatus{TaskID: "task-9", Status: "running", Progress: 0.25}, nil)
		rr := httptest.NewRecorder()
		h.handleBackfillStart(rr, httptest.NewRequest(http.MethodPost, "/api/webAnalytics.backfillStart", strings.NewReader(`{"workspace_id":"ws1"}`)))
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		var body struct {
			Backfill *domain.WebAnalyticsBackfillStatus `json:"backfill"`
		}
		require.NoError(t, json.NewDecoder(rr.Body).Decode(&body))
		require.NotNil(t, body.Backfill)
		assert.Equal(t, "task-9", body.Backfill.TaskID)
		assert.Equal(t, "running", body.Backfill.Status)
		assert.Equal(t, 0.25, body.Backfill.Progress)
	})

	t.Run("permission error is 403", func(t *testing.T) {
		h, svc := coverageWebAnalyticsHandler(t)
		svc.EXPECT().BackfillStart(gomock.Any(), "ws1").
			Return(nil, domain.NewPermissionError(domain.PermissionResourceWebAnalytics, domain.PermissionTypeWrite, "no write"))
		rr := httptest.NewRecorder()
		h.handleBackfillStart(rr, httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"workspace_id":"ws1"}`)))
		assert.Equal(t, http.StatusForbidden, rr.Code)
		var body map[string]interface{}
		require.NoError(t, json.NewDecoder(rr.Body).Decode(&body))
		assert.Equal(t, string(domain.PermissionTypeWrite), body["permission"])
	})

	t.Run("generic service error is 400 with exact message", func(t *testing.T) {
		h, svc := coverageWebAnalyticsHandler(t)
		svc.EXPECT().BackfillStart(gomock.Any(), "ws1").Return(nil, errors.New("already running"))
		rr := httptest.NewRecorder()
		h.handleBackfillStart(rr, httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"workspace_id":"ws1"}`)))
		assert.Equal(t, http.StatusBadRequest, rr.Code)
		var body map[string]string
		require.NoError(t, json.NewDecoder(rr.Body).Decode(&body))
		assert.Equal(t, "already running", body["error"])
	})

	t.Run("missing workspace never reaches service", func(t *testing.T) {
		h, _ := coverageWebAnalyticsHandler(t) // no EXPECT: any service call fails
		rr := httptest.NewRecorder()
		h.handleBackfillStart(rr, httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{}`)))
		assert.Equal(t, http.StatusBadRequest, rr.Code)
		var body map[string]string
		require.NoError(t, json.NewDecoder(rr.Body).Decode(&body))
		assert.Equal(t, "workspace_id is required", body["error"])
	})

	t.Run("GET never reaches service", func(t *testing.T) {
		h, _ := coverageWebAnalyticsHandler(t)
		rr := httptest.NewRecorder()
		h.handleBackfillStart(rr, httptest.NewRequest(http.MethodGet, "/x", nil))
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})
}

func TestCoverage_HandleBackfillStatus(t *testing.T) {
	t.Run("success returns the latest status", func(t *testing.T) {
		h, svc := coverageWebAnalyticsHandler(t)
		svc.EXPECT().BackfillStatus(gomock.Any(), "ws7").
			Return(&domain.WebAnalyticsBackfillStatus{TaskID: "task-1", Status: "completed", Progress: 1}, nil)
		rr := httptest.NewRecorder()
		h.handleBackfillStatus(rr, httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"workspace_id":"ws7"}`)))
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		var body struct {
			Backfill *domain.WebAnalyticsBackfillStatus `json:"backfill"`
		}
		require.NoError(t, json.NewDecoder(rr.Body).Decode(&body))
		require.NotNil(t, body.Backfill)
		assert.Equal(t, "completed", body.Backfill.Status)
		assert.Equal(t, float64(1), body.Backfill.Progress)
	})

	t.Run("no prior run encodes backfill as null", func(t *testing.T) {
		h, svc := coverageWebAnalyticsHandler(t)
		svc.EXPECT().BackfillStatus(gomock.Any(), "ws7").Return(nil, nil)
		rr := httptest.NewRecorder()
		h.handleBackfillStatus(rr, httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"workspace_id":"ws7"}`)))
		require.Equal(t, http.StatusOK, rr.Code)
		assert.JSONEq(t, `{"backfill":null}`, rr.Body.String())
	})

	t.Run("service error is 400 with exact message", func(t *testing.T) {
		h, svc := coverageWebAnalyticsHandler(t)
		svc.EXPECT().BackfillStatus(gomock.Any(), "ws7").Return(nil, errors.New("task store down"))
		rr := httptest.NewRecorder()
		h.handleBackfillStatus(rr, httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"workspace_id":"ws7"}`)))
		assert.Equal(t, http.StatusBadRequest, rr.Code)
		var body map[string]string
		require.NoError(t, json.NewDecoder(rr.Body).Decode(&body))
		assert.Equal(t, "task store down", body["error"])
	})

	t.Run("invalid JSON never reaches service", func(t *testing.T) {
		h, _ := coverageWebAnalyticsHandler(t)
		rr := httptest.NewRecorder()
		h.handleBackfillStatus(rr, httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{bad`)))
		assert.Equal(t, http.StatusBadRequest, rr.Code)
		var body map[string]string
		require.NoError(t, json.NewDecoder(rr.Body).Decode(&body))
		assert.Equal(t, "Invalid request body", body["error"])
	})
}

func TestCoverage_HandleBackfillCancel(t *testing.T) {
	t.Run("success returns status success and forwards workspace", func(t *testing.T) {
		h, svc := coverageWebAnalyticsHandler(t)
		svc.EXPECT().BackfillCancel(gomock.Any(), "ws3").Return(nil)
		rr := httptest.NewRecorder()
		h.handleBackfillCancel(rr, httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"workspace_id":"ws3"}`)))
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		assert.JSONEq(t, `{"status":"success"}`, rr.Body.String())
	})

	t.Run("permission error is 403", func(t *testing.T) {
		h, svc := coverageWebAnalyticsHandler(t)
		svc.EXPECT().BackfillCancel(gomock.Any(), "ws3").
			Return(domain.NewPermissionError(domain.PermissionResourceWebAnalytics, domain.PermissionTypeWrite, "no write"))
		rr := httptest.NewRecorder()
		h.handleBackfillCancel(rr, httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"workspace_id":"ws3"}`)))
		assert.Equal(t, http.StatusForbidden, rr.Code)
		var body map[string]interface{}
		require.NoError(t, json.NewDecoder(rr.Body).Decode(&body))
		assert.Equal(t, string(domain.PermissionResourceWebAnalytics), body["resource"])
	})

	t.Run("generic error is 400 with exact message", func(t *testing.T) {
		h, svc := coverageWebAnalyticsHandler(t)
		svc.EXPECT().BackfillCancel(gomock.Any(), "ws3").Return(errors.New("no running backfill"))
		rr := httptest.NewRecorder()
		h.handleBackfillCancel(rr, httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"workspace_id":"ws3"}`)))
		assert.Equal(t, http.StatusBadRequest, rr.Code)
		var body map[string]string
		require.NoError(t, json.NewDecoder(rr.Body).Decode(&body))
		assert.Equal(t, "no running backfill", body["error"])
	})

	t.Run("missing workspace never reaches service", func(t *testing.T) {
		h, _ := coverageWebAnalyticsHandler(t)
		rr := httptest.NewRecorder()
		h.handleBackfillCancel(rr, httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"workspace_id":""}`)))
		assert.Equal(t, http.StatusBadRequest, rr.Code)
		var body map[string]string
		require.NoError(t, json.NewDecoder(rr.Body).Decode(&body))
		assert.Equal(t, "workspace_id is required", body["error"])
	})
}
