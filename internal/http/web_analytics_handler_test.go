package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/pkg/logger"
)

func newWebAnalyticsHandlerForTest(t *testing.T, sdkJS []byte) (*WebAnalyticsHandler, *mocks.MockWebAnalyticsService, *http.ServeMux) {
	t.Helper()
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	svc := mocks.NewMockWebAnalyticsService(ctrl)
	handler := NewWebAnalyticsHandler(svc, nil, logger.NewLogger(), sdkJS)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	return handler, svc, mux
}

func trackRequest(t *testing.T, body string, headers map[string]string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/track", strings.NewReader(body))
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh) Chrome/126.0")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return req
}

func validBeatBody() string {
	return `{"workspace_id":"ws1","session_id":"01912345-6789-7abc-8def-0123456789ab","actions":[],"created_at":1,"updated_at":1,"seq":0}`
}

func TestWebAnalyticsHandlerTrack(t *testing.T) {
	t.Run("method not allowed", func(t *testing.T) {
		_, _, mux := newWebAnalyticsHandlerForTest(t, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/track", nil))
		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	})

	t.Run("invalid JSON gets a 400 with success:false", func(t *testing.T) {
		_, _, mux := newWebAnalyticsHandlerForTest(t, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, trackRequest(t, "{not json", nil))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), `"success":false`)
	})

	t.Run("oversized body rejected", func(t *testing.T) {
		_, _, mux := newWebAnalyticsHandlerForTest(t, nil)
		huge := `{"workspace_id":"` + strings.Repeat("a", 1<<20) + `"}`
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, trackRequest(t, huge, nil))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("bot user agents short-circuit without touching the service", func(t *testing.T) {
		_, svc, mux := newWebAnalyticsHandlerForTest(t, nil)
		// No svc.EXPECT(): any call would fail the test.
		_ = svc
		rec := httptest.NewRecorder()
		req := trackRequest(t, validBeatBody(), map[string]string{"User-Agent": "Googlebot/2.1"})
		mux.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), `"success":true`)
	})

	t.Run("content-type is irrelevant (text/plain beats decode)", func(t *testing.T) {
		_, svc, mux := newWebAnalyticsHandlerForTest(t, nil)
		svc.EXPECT().Track(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, payload *domain.WebTrackPayload, meta domain.WebRequestMeta) error {
				assert.Equal(t, "ws1", payload.WorkspaceID)
				assert.Equal(t, "https://shop.example.com", meta.Origin)
				assert.Equal(t, "203.0.113.9", meta.ClientIP)
				assert.False(t, meta.ReceivedAt.IsZero())
				return nil
			})

		rec := httptest.NewRecorder()
		req := trackRequest(t, validBeatBody(), map[string]string{
			"Content-Type":    "text/plain;charset=UTF-8",
			"Origin":          "https://shop.example.com",
			"X-Forwarded-For": "203.0.113.9, 10.0.0.1",
		})
		mux.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("validation errors from the service map to 400", func(t *testing.T) {
		_, svc, mux := newWebAnalyticsHandlerForTest(t, nil)
		svc.EXPECT().Track(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(&service.ErrWebTrackInvalidPayload{Err: fmt.Errorf("seq must be >= 0")})

		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, trackRequest(t, validBeatBody(), nil))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "seq must be")
	})

	t.Run("internal errors stay invisible to the client", func(t *testing.T) {
		_, svc, mux := newWebAnalyticsHandlerForTest(t, nil)
		svc.EXPECT().Track(gomock.Any(), gomock.Any(), gomock.Any()).Return(fmt.Errorf("db exploded"))

		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, trackRequest(t, validBeatBody(), nil))
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), `"success":true`)
		assert.NotContains(t, rec.Body.String(), "db exploded")
	})

	t.Run("a panic in the pipeline still answers 200", func(t *testing.T) {
		_, svc, mux := newWebAnalyticsHandlerForTest(t, nil)
		svc.EXPECT().Track(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(context.Context, *domain.WebTrackPayload, domain.WebRequestMeta) error {
				panic("boom")
			})

		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, trackRequest(t, validBeatBody(), nil))
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), `"success":true`)
	})

	t.Run("CORS: origin reflected, credentials header stripped, wildcard fallback", func(t *testing.T) {
		_, svc, mux := newWebAnalyticsHandlerForTest(t, nil)
		svc.EXPECT().Track(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(2)

		rec := httptest.NewRecorder()
		// Simulate the global CORS middleware having pinned unusable values.
		rec.Header().Set("Access-Control-Allow-Origin", "https://console.internal")
		rec.Header().Set("Access-Control-Allow-Credentials", "true")
		mux.ServeHTTP(rec, trackRequest(t, validBeatBody(), map[string]string{"Origin": "https://customer-site.com"}))
		assert.Equal(t, "https://customer-site.com", rec.Header().Get("Access-Control-Allow-Origin"))
		assert.Empty(t, rec.Header().Get("Access-Control-Allow-Credentials"))
		assert.Contains(t, rec.Header().Values("Vary"), "Origin")

		rec = httptest.NewRecorder()
		mux.ServeHTTP(rec, trackRequest(t, validBeatBody(), nil))
		assert.Equal(t, "*", rec.Header().Get("Access-Control-Allow-Origin"))
	})
}

func TestWebAnalyticsHandlerSDK(t *testing.T) {
	sdk := []byte("(function(){/* notifuse analytics */})();")

	t.Run("no embedded SDK, no routes", func(t *testing.T) {
		_, _, mux := newWebAnalyticsHandlerForTest(t, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/na.js", nil))
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("stable URL with short cache", func(t *testing.T) {
		_, _, mux := newWebAnalyticsHandlerForTest(t, sdk)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/na.js", nil))
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.True(t, bytes.Equal(sdk, rec.Body.Bytes()))
		assert.Equal(t, "public, max-age=3600", rec.Header().Get("Cache-Control"))
		assert.Equal(t, "*", rec.Header().Get("Access-Control-Allow-Origin"))
	})

	t.Run("hash-addressed URL is immutable", func(t *testing.T) {
		handler, _, mux := newWebAnalyticsHandlerForTest(t, sdk)
		require.NotEmpty(t, handler.SDKHash())

		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/na."+handler.SDKHash()+".js", nil))
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Header().Get("Cache-Control"), "immutable")
	})

	t.Run("POST to the SDK URL rejected", func(t *testing.T) {
		_, _, mux := newWebAnalyticsHandlerForTest(t, sdk)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/na.js", nil))
		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	})
}

func TestWebAnalyticsHandlerBackfillRoutes(t *testing.T) {
	t.Run("routes absent without a JWT secret provider", func(t *testing.T) {
		_, _, mux := newWebAnalyticsHandlerForTest(t, nil) // getJWTSecret == nil
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/webAnalytics.backfillStart", strings.NewReader("{}")))
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("registered routes demand authentication", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		t.Cleanup(ctrl.Finish)
		svc := mocks.NewMockWebAnalyticsService(ctrl)
		handler := NewWebAnalyticsHandler(svc, func() ([]byte, error) { return []byte("0123456789abcdef0123456789abcdef"), nil }, logger.NewLogger(), nil)
		mux := http.NewServeMux()
		handler.RegisterRoutes(mux)

		for _, route := range []string{"/api/webAnalytics.backfillStart", "/api/webAnalytics.backfillStatus", "/api/webAnalytics.backfillCancel"} {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, route, strings.NewReader(`{"workspace_id":"ws1"}`))
			mux.ServeHTTP(rec, req)
			assert.Equal(t, http.StatusUnauthorized, rec.Code, route)
		}
	})
}

func TestWebAnalyticsHandlerResponseShape(t *testing.T) {
	_, svc, mux := newWebAnalyticsHandlerForTest(t, nil)
	svc.EXPECT().Track(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, trackRequest(t, validBeatBody(), nil))

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, map[string]interface{}{"success": true}, body)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
}
