package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/logger"
)

func newContactVisitsHandlerForTest(t *testing.T) (*WebAnalyticsHandler, *mocks.MockWebAnalyticsContactVisitsService) {
	t.Helper()
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)
	visits := mocks.NewMockWebAnalyticsContactVisitsService(ctrl)
	handler := NewWebAnalyticsHandler(mocks.NewMockWebAnalyticsService(ctrl), func() ([]byte, error) {
		return []byte("0123456789abcdef0123456789abcdef"), nil
	}, logger.NewLogger(), nil).WithContactVisits(visits)
	return handler, visits
}

const (
	contactVisitsQuery       = "/api/webAnalytics.contactVisits?workspace_id=ws1&email=ann@example.com"
	contactSessionPagesQuery = "/api/webAnalytics.contactSessionPages?workspace_id=ws1&email=ann@example.com&session_id=0190d5a4-1c2b-7000-8000-000000000001&session_date=2026-11-03"
)

func TestWebAnalyticsHandlerContactVisitsRoutes(t *testing.T) {
	t.Run("absent without the service", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		handler := NewWebAnalyticsHandler(mocks.NewMockWebAnalyticsService(ctrl), func() ([]byte, error) { return []byte("secret"), nil }, logger.NewLogger(), nil)
		mux := http.NewServeMux()
		handler.RegisterRoutes(mux)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, contactVisitsQuery, nil))
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("registered with the service, behind authentication", func(t *testing.T) {
		handler, _ := newContactVisitsHandlerForTest(t)
		mux := http.NewServeMux()
		handler.RegisterRoutes(mux)
		for _, target := range []string{contactVisitsQuery, contactSessionPagesQuery} {
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
			assert.Equal(t, http.StatusUnauthorized, rec.Code, target)
		}
	})
}

func TestWebAnalyticsHandlerContactVisits(t *testing.T) {
	serve := func(handler *WebAnalyticsHandler, method, target string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(method, target, nil)
		if target[:len("/api/webAnalytics.contactVisits")] == "/api/webAnalytics.contactVisits" {
			handler.handleContactVisits(rec, req)
		} else {
			handler.handleContactSessionPages(rec, req)
		}
		return rec
	}

	for _, target := range []string{contactVisitsQuery, contactSessionPagesQuery} {
		t.Run("non-GET is a 405: "+target, func(t *testing.T) {
			handler, _ := newContactVisitsHandlerForTest(t)
			assert.Equal(t, http.StatusMethodNotAllowed, serve(handler, http.MethodPost, target).Code)
		})
	}

	t.Run("a missing parameter is a 400", func(t *testing.T) {
		handler, _ := newContactVisitsHandlerForTest(t)
		rec := serve(handler, http.MethodGet, "/api/webAnalytics.contactVisits?workspace_id=ws1")
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "email is required")

		rec = serve(handler, http.MethodGet, "/api/webAnalytics.contactSessionPages?workspace_id=ws1&email=a@b.co&session_date=2026-11-03")
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "session_id is required")
	})

	t.Run("web analytics disabled is a 400 with the message", func(t *testing.T) {
		handler, visits := newContactVisitsHandlerForTest(t)
		visits.EXPECT().ContactVisits(gomock.Any(), gomock.Any()).Return(nil, domain.ErrWebAnalyticsNotEnabled)
		rec := serve(handler, http.MethodGet, contactVisitsQuery)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "web analytics is not enabled")
	})

	t.Run("a permission error is a 403", func(t *testing.T) {
		handler, visits := newContactVisitsHandlerForTest(t)
		visits.EXPECT().ContactSessionPages(gomock.Any(), gomock.Any()).
			Return(nil, domain.NewPermissionError(domain.PermissionResourceWebAnalytics, domain.PermissionTypeRead, "Insufficient permissions"))
		assert.Equal(t, http.StatusForbidden, serve(handler, http.MethodGet, contactSessionPagesQuery).Code)
	})

	t.Run("any other error is a 500 with a fixed message", func(t *testing.T) {
		handler, visits := newContactVisitsHandlerForTest(t)
		visits.EXPECT().ContactVisits(gomock.Any(), gomock.Any()).Return(nil, errors.New("pq: secret detail"))
		rec := serve(handler, http.MethodGet, contactVisitsQuery)
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.NotContains(t, rec.Body.String(), "secret detail")
	})

	t.Run("contactVisits: the parsed request and the JSON shape", func(t *testing.T) {
		handler, visits := newContactVisitsHandlerForTest(t)
		visits.EXPECT().ContactVisits(gomock.Any(), domain.ContactVisitsRequest{
			WorkspaceID: "ws1", Email: "ann@example.com", Filter: "goal", Limit: 5,
		}).Return(&domain.ContactVisitsResponse{
			Summary:    &domain.ContactVisitsSummary{Sessions: 1, Goals: []domain.ContactVisitGoal{}, TopPages: []domain.ContactVisitPage{}},
			Sessions:   []domain.ContactVisitSession{{ID: "s1", SessionDate: "2026-11-03"}},
			NextCursor: "abc",
		}, nil)

		rec := serve(handler, http.MethodGet, "/api/webAnalytics.contactVisits?workspace_id=ws1&email=Ann@example.com&filter=goal&limit=5")
		require.Equal(t, http.StatusOK, rec.Code)
		var body map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		assert.Equal(t, "abc", body["next_cursor"])
		assert.Equal(t, float64(1), body["summary"].(map[string]interface{})["sessions"])
		assert.Equal(t, "2026-11-03", body["sessions"].([]interface{})[0].(map[string]interface{})["session_date"])
	})

	t.Run("contactSessionPages: the parsed request and the JSON shape", func(t *testing.T) {
		handler, visits := newContactVisitsHandlerForTest(t)
		visits.EXPECT().ContactSessionPages(gomock.Any(), domain.ContactSessionPagesRequest{
			WorkspaceID: "ws1", Email: "ann@example.com", SessionID: "0190d5a4-1c2b-7000-8000-000000000001", SessionDate: "2026-11-03",
		}).Return(&domain.ContactSessionPagesResponse{Pages: []domain.ContactSessionPage{}, Goals: []domain.ContactSessionGoal{}}, nil)

		rec := serve(handler, http.MethodGet, contactSessionPagesQuery)
		require.Equal(t, http.StatusOK, rec.Code)
		assert.JSONEq(t, `{"pages":[],"goals":[]}`, rec.Body.String())
	})
}
