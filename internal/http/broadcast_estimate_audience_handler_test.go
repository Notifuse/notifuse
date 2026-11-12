package http_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
)

func TestHandleEstimateAudience(t *testing.T) {
	serve := func(t *testing.T, method, body string, expect func(m *mocks.MockBroadcastService)) *httptest.ResponseRecorder {
		t.Helper()
		handler, service, _, logger, ctrl := setupBroadcastHandler(t)
		t.Cleanup(ctrl.Finish)
		logger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(logger).AnyTimes()
		logger.EXPECT().Error(gomock.Any()).AnyTimes()
		if expect != nil {
			expect(service)
		}
		rec := httptest.NewRecorder()
		handler.HandleEstimateAudience(rec, httptest.NewRequest(method, "/api/broadcasts.estimateAudience", strings.NewReader(body)))
		return rec
	}

	t.Run("non-POST is a 405", func(t *testing.T) {
		assert.Equal(t, http.StatusMethodNotAllowed, serve(t, http.MethodGet, "", nil).Code)
	})

	t.Run("an unreadable body is a 400", func(t *testing.T) {
		assert.Equal(t, http.StatusBadRequest, serve(t, http.MethodPost, "{", nil).Code)
	})

	t.Run("a missing list is a 400 with the reason", func(t *testing.T) {
		rec := serve(t, http.MethodPost, `{"workspace_id":"ws","audience":{}}`, nil)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "audience.list is required")
	})

	t.Run("success: the decoded audience, exclude_unsubscribed defaulted, and the count", func(t *testing.T) {
		rec := serve(t, http.MethodPost, `{"workspace_id":"ws","audience":{"list":"l1","segments":["s1"]}}`, func(m *mocks.MockBroadcastService) {
			m.EXPECT().EstimateAudience(gomock.Any(), "ws", domain.AudienceSettings{List: "l1", Segments: []string{"s1"}, ExcludeUnsubscribed: true}).Return(2214, nil)
		})
		require.Equal(t, http.StatusOK, rec.Code)
		assert.JSONEq(t, `{"count":2214}`, rec.Body.String())
	})

	t.Run("a permission error is a 403", func(t *testing.T) {
		rec := serve(t, http.MethodPost, `{"workspace_id":"ws","audience":{"list":"l1"}}`, func(m *mocks.MockBroadcastService) {
			m.EXPECT().EstimateAudience(gomock.Any(), "ws", gomock.Any()).
				Return(0, domain.NewPermissionError(domain.PermissionResourceLists, domain.PermissionTypeRead, "Insufficient permissions"))
		})
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("a service error is a 500", func(t *testing.T) {
		rec := serve(t, http.MethodPost, `{"workspace_id":"ws","audience":{"list":"l1"}}`, func(m *mocks.MockBroadcastService) {
			m.EXPECT().EstimateAudience(gomock.Any(), "ws", gomock.Any()).Return(0, errors.New("db down"))
		})
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.NotContains(t, rec.Body.String(), "db down")
	})

	t.Run("the route is registered", func(t *testing.T) {
		handler, _, _, _, ctrl := setupBroadcastHandler(t)
		defer ctrl.Finish()
		mux := http.NewServeMux()
		handler.RegisterRoutes(mux)
		_, pattern := mux.Handler(httptest.NewRequest(http.MethodPost, "/api/broadcasts.estimateAudience", nil))
		assert.Equal(t, "/api/broadcasts.estimateAudience", pattern)
	})
}

func TestHandleList_StatusCountsAndStartedAfter(t *testing.T) {
	startedAfter := time.Date(2026, 10, 12, 22, 0, 0, 0, time.UTC)

	t.Run("the new parameters reach the service and status_counts is written", func(t *testing.T) {
		handler, service, _, _, ctrl := setupBroadcastHandler(t)
		defer ctrl.Finish()
		service.EXPECT().ListBroadcasts(gomock.Any(), gomock.Eq(domain.ListBroadcastsParams{
			WorkspaceID: "ws", Status: domain.BroadcastStatusDraft, Statuses: []domain.BroadcastStatus{domain.BroadcastStatusDraft},
			Search: "promo", Limit: 10, WithStatusCounts: true, StartedAfter: &startedAfter,
		})).Return(&domain.BroadcastListResponse{TotalCount: 0, StatusCounts: map[domain.BroadcastStatus]int{domain.BroadcastStatusDraft: 4}}, nil)

		rec := httptest.NewRecorder()
		handler.HandleList(rec, httptest.NewRequest(http.MethodGet,
			"/api/broadcasts.list?workspace_id=ws&status=draft&search=promo&limit=10&with_status_counts=true&started_after=2026-10-13T00:00:00%2B02:00", nil))
		require.Equal(t, http.StatusOK, rec.Code)
		assert.JSONEq(t, `{"broadcasts":[],"total_count":0,"status_counts":{"draft":4}}`, rec.Body.String())
	})

	t.Run("status_counts is an object even when nothing matches", func(t *testing.T) {
		handler, service, _, _, ctrl := setupBroadcastHandler(t)
		defer ctrl.Finish()
		service.EXPECT().ListBroadcasts(gomock.Any(), gomock.Any()).Return(&domain.BroadcastListResponse{}, nil)

		rec := httptest.NewRecorder()
		handler.HandleList(rec, httptest.NewRequest(http.MethodGet, "/api/broadcasts.list?workspace_id=ws&with_status_counts=true", nil))
		require.Equal(t, http.StatusOK, rec.Code)
		var body map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		assert.Equal(t, map[string]interface{}{}, body["status_counts"])
	})

	t.Run("without with_status_counts there is no status_counts key", func(t *testing.T) {
		handler, service, _, _, ctrl := setupBroadcastHandler(t)
		defer ctrl.Finish()
		service.EXPECT().ListBroadcasts(gomock.Any(), gomock.Any()).Return(&domain.BroadcastListResponse{}, nil)

		rec := httptest.NewRecorder()
		handler.HandleList(rec, httptest.NewRequest(http.MethodGet, "/api/broadcasts.list?workspace_id=ws", nil))
		require.Equal(t, http.StatusOK, rec.Code)
		assert.NotContains(t, rec.Body.String(), "status_counts")
	})

	t.Run("an unreadable started_after is a 400", func(t *testing.T) {
		handler, _, _, _, ctrl := setupBroadcastHandler(t)
		defer ctrl.Finish()
		rec := httptest.NewRecorder()
		handler.HandleList(rec, httptest.NewRequest(http.MethodGet, "/api/broadcasts.list?workspace_id=ws&started_after=yesterday", nil))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}
