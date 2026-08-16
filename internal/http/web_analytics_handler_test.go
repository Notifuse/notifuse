package http

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
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

	t.Run("oversized body gets 413, not a generic 400", func(t *testing.T) {
		// The distinction is actionable: a client can recover from "too large"
		// by trimming its oldest actions or rotating the session, and must,
		// because actions[] only grows and every later beat would fail too.
		_, _, mux := newWebAnalyticsHandlerForTest(t, nil)
		huge := `{"workspace_id":"` + strings.Repeat("a", 1<<20) + `"}`
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, trackRequest(t, huge, nil))
		assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
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

// compressibleSDK stands in for the real bundle in the encoding tests. The
// fixture TestWebAnalyticsHandlerSDK uses is 41 bytes and gzips to 61, so the
// handler keeps no compressed variant for it — every assertion below would
// pass vacuously against it while covering nothing.
func compressibleSDK() []byte {
	return bytes.Repeat([]byte("/* notifuse analytics */(function(){window.na=1;})();\n"), 40)
}

func sdkRequest(t *testing.T, mux *http.ServeMux, method, target string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func gunzip(t *testing.T, b []byte) []byte {
	t.Helper()
	zr, err := gzip.NewReader(bytes.NewReader(b))
	require.NoError(t, err)
	defer zr.Close()
	out, err := io.ReadAll(zr)
	require.NoError(t, err)
	return out
}

func TestWebAnalyticsHandlerSDKEncoding(t *testing.T) {
	sdk := compressibleSDK()

	t.Run("identity when the client does not accept gzip", func(t *testing.T) {
		handler, _, mux := newWebAnalyticsHandlerForTest(t, sdk)
		rec := sdkRequest(t, mux, http.MethodGet, "/na.js", nil)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.True(t, bytes.Equal(sdk, rec.Body.Bytes()))
		assert.Empty(t, rec.Header().Get("Content-Encoding"))
		assert.Equal(t, `"`+handler.SDKHash()+`"`, rec.Header().Get("ETag"))
		assert.Equal(t, strconv.Itoa(len(sdk)), rec.Header().Get("Content-Length"))
		assert.Contains(t, rec.Header().Values("Vary"), "Accept-Encoding")
	})

	t.Run("gzip when accepted", func(t *testing.T) {
		handler, _, mux := newWebAnalyticsHandlerForTest(t, sdk)
		rec := sdkRequest(t, mux, http.MethodGet, "/na.js", map[string]string{
			"Accept-Encoding": "gzip, deflate, br, zstd",
		})

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "gzip", rec.Header().Get("Content-Encoding"))
		assert.True(t, bytes.Equal(sdk, gunzip(t, rec.Body.Bytes())), "gzip body must inflate to the bundle")
		assert.Less(t, rec.Body.Len(), len(sdk))
		assert.Equal(t, strconv.Itoa(rec.Body.Len()), rec.Header().Get("Content-Length"))
		assert.Equal(t, `"`+handler.SDKHash()+`-gzip"`, rec.Header().Get("ETag"))
		assert.Contains(t, rec.Header().Values("Vary"), "Accept-Encoding")
	})

	t.Run("an explicit gzip refusal is honoured", func(t *testing.T) {
		_, _, mux := newWebAnalyticsHandlerForTest(t, sdk)
		rec := sdkRequest(t, mux, http.MethodGet, "/na.js", map[string]string{
			"Accept-Encoding": "gzip;q=0",
		})

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Empty(t, rec.Header().Get("Content-Encoding"))
		assert.True(t, bytes.Equal(sdk, rec.Body.Bytes()))
	})

	t.Run("a bundle gzip cannot shrink is served identity", func(t *testing.T) {
		tiny := []byte("(function(){/* notifuse analytics */})();")
		_, _, mux := newWebAnalyticsHandlerForTest(t, tiny)
		rec := sdkRequest(t, mux, http.MethodGet, "/na.js", map[string]string{"Accept-Encoding": "gzip"})

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Empty(t, rec.Header().Get("Content-Encoding"))
		assert.True(t, bytes.Equal(tiny, rec.Body.Bytes()))
	})

	t.Run("matching validator returns 304 without a body", func(t *testing.T) {
		handler, _, mux := newWebAnalyticsHandlerForTest(t, sdk)
		etag := `"` + handler.SDKHash() + `"`
		rec := sdkRequest(t, mux, http.MethodGet, "/na.js", map[string]string{"If-None-Match": etag})

		assert.Equal(t, http.StatusNotModified, rec.Code)
		assert.Empty(t, rec.Body.Bytes())
		assert.Equal(t, etag, rec.Header().Get("ETag"))
		assert.Contains(t, rec.Header().Values("Vary"), "Accept-Encoding")
		assert.Empty(t, rec.Header().Get("Content-Encoding"))
		assert.Empty(t, rec.Header().Get("Content-Length"))
		// A 304 leaving before the CORS override would ship the global
		// middleware's pinned origin and Allow-Credentials to a customer site.
		assert.Equal(t, "*", rec.Header().Get("Access-Control-Allow-Origin"))
		assert.Empty(t, rec.Header().Get("Access-Control-Allow-Credentials"))
	})

	t.Run("the two codings have separate validators", func(t *testing.T) {
		handler, _, mux := newWebAnalyticsHandlerForTest(t, sdk)
		identity := `"` + handler.SDKHash() + `"`
		compressed := `"` + handler.SDKHash() + `-gzip"`

		// The identity tag must not satisfy a request that will be answered
		// with gzip, or a cache hands a client the wrong representation.
		rec := sdkRequest(t, mux, http.MethodGet, "/na.js", map[string]string{
			"Accept-Encoding": "gzip",
			"If-None-Match":   identity,
		})
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "gzip", rec.Header().Get("Content-Encoding"))

		rec = sdkRequest(t, mux, http.MethodGet, "/na.js", map[string]string{
			"Accept-Encoding": "gzip",
			"If-None-Match":   compressed,
		})
		assert.Equal(t, http.StatusNotModified, rec.Code)
		assert.Equal(t, compressed, rec.Header().Get("ETag"))
	})

	t.Run("wildcard and weak validators match", func(t *testing.T) {
		handler, _, mux := newWebAnalyticsHandlerForTest(t, sdk)

		rec := sdkRequest(t, mux, http.MethodGet, "/na.js", map[string]string{"If-None-Match": "*"})
		assert.Equal(t, http.StatusNotModified, rec.Code)

		rec = sdkRequest(t, mux, http.MethodGet, "/na.js", map[string]string{
			"If-None-Match": `W/"` + handler.SDKHash() + `"`,
		})
		assert.Equal(t, http.StatusNotModified, rec.Code)
	})

	t.Run("HEAD reports the length of the negotiated encoding", func(t *testing.T) {
		_, _, mux := newWebAnalyticsHandlerForTest(t, sdk)

		rec := sdkRequest(t, mux, http.MethodHead, "/na.js", nil)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Empty(t, rec.Body.Bytes())
		assert.Equal(t, strconv.Itoa(len(sdk)), rec.Header().Get("Content-Length"))

		gzipped := sdkRequest(t, mux, http.MethodGet, "/na.js", map[string]string{"Accept-Encoding": "gzip"})
		rec = sdkRequest(t, mux, http.MethodHead, "/na.js", map[string]string{"Accept-Encoding": "gzip"})
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Empty(t, rec.Body.Bytes())
		assert.Equal(t, "gzip", rec.Header().Get("Content-Encoding"))
		assert.Equal(t, strconv.Itoa(gzipped.Body.Len()), rec.Header().Get("Content-Length"))
	})

	t.Run("both routes serve the same bytes under different cache policies", func(t *testing.T) {
		handler, _, mux := newWebAnalyticsHandlerForTest(t, sdk)
		headers := map[string]string{"Accept-Encoding": "gzip"}

		stable := sdkRequest(t, mux, http.MethodGet, "/na.js", headers)
		immutable := sdkRequest(t, mux, http.MethodGet, "/na."+handler.SDKHash()+".js", headers)

		// The SDK's double-install guard assumes a page loading both URLs
		// evaluates one and the same bundle.
		assert.True(t, bytes.Equal(stable.Body.Bytes(), immutable.Body.Bytes()))
		assert.Equal(t, stable.Header().Get("ETag"), immutable.Header().Get("ETag"))
		assert.Equal(t, "public, max-age=3600", stable.Header().Get("Cache-Control"))
		assert.Contains(t, immutable.Header().Get("Cache-Control"), "immutable")
	})

	t.Run("ranges are neither advertised nor served", func(t *testing.T) {
		_, _, mux := newWebAnalyticsHandlerForTest(t, sdk)
		rec := sdkRequest(t, mux, http.MethodGet, "/na.js", map[string]string{
			"Range":           "bytes=0-99",
			"Accept-Encoding": "gzip",
		})

		// A 206 slice of the compressed body could be spliced into a client's
		// identity buffer, so this route answers the full body or nothing.
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Empty(t, rec.Header().Get("Accept-Ranges"))
		assert.True(t, bytes.Equal(sdk, gunzip(t, rec.Body.Bytes())))
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
