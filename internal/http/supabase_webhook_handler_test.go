package http

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/Notifuse/notifuse/internal/service"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
)

func TestSupabaseWebhookHandler_RegisterRoutes(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockService := &service.SupabaseService{}
	handler := NewSupabaseWebhookHandler(mockService, mockLogger)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// Verify routes exist by making test requests
	req1 := httptest.NewRequest(http.MethodPost, "/webhooks/supabase/auth-email", nil)
	rr1 := httptest.NewRecorder()
	mux.ServeHTTP(rr1, req1)
	// Handler exists, will return 400 due to missing params but that's ok for this test
	assert.NotEqual(t, http.StatusNotFound, rr1.Code)

	req2 := httptest.NewRequest(http.MethodPost, "/webhooks/supabase/before-user-created", nil)
	rr2 := httptest.NewRecorder()
	mux.ServeHTTP(rr2, req2)
	assert.NotEqual(t, http.StatusNotFound, rr2.Code)
}

func TestHandleAuthEmailWebhook_MethodNotAllowed(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := pkgmocks.NewMockLogger(ctrl)
	handler := NewSupabaseWebhookHandler(nil, mockLogger)

	req := httptest.NewRequest(http.MethodGet, "/webhooks/supabase/auth-email", nil)
	rr := httptest.NewRecorder()

	handler.handleAuthEmailWebhook(rr, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
}

func TestHandleAuthEmailWebhook_MissingQueryParams(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := pkgmocks.NewMockLogger(ctrl)
	handler := NewSupabaseWebhookHandler(nil, mockLogger)

	tests := []struct {
		name      string
		queryPath string
	}{
		{
			name:      "missing workspace_id",
			queryPath: "/webhooks/supabase/auth-email?integration_id=integration-456",
		},
		{
			name:      "missing integration_id",
			queryPath: "/webhooks/supabase/auth-email?workspace_id=workspace-123",
		},
		{
			name:      "missing both",
			queryPath: "/webhooks/supabase/auth-email",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tt.queryPath, nil)
			rr := httptest.NewRecorder()

			handler.handleAuthEmailWebhook(rr, req)

			assert.Equal(t, http.StatusBadRequest, rr.Code)
		})
	}
}

func TestHandleAuthEmailWebhook_MissingHeaders(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := pkgmocks.NewMockLogger(ctrl)
	handler := NewSupabaseWebhookHandler(nil, mockLogger)

	tests := []struct {
		name    string
		headers map[string]string
	}{
		{
			name: "missing webhook-id",
			headers: map[string]string{
				"webhook-timestamp": "1234567890",
				"webhook-signature": "v1,signature123",
			},
		},
		{
			name: "missing webhook-timestamp",
			headers: map[string]string{
				"webhook-id":        "webhook-id-123",
				"webhook-signature": "v1,signature123",
			},
		},
		{
			name: "missing webhook-signature",
			headers: map[string]string{
				"webhook-id":        "webhook-id-123",
				"webhook-timestamp": "1234567890",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/webhooks/supabase/auth-email?workspace_id=workspace-123&integration_id=integration-456", nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			rr := httptest.NewRecorder()
			handler.handleAuthEmailWebhook(rr, req)

			assert.Equal(t, http.StatusBadRequest, rr.Code)
		})
	}
}

func TestHandleUserCreatedWebhook_MethodNotAllowed(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := pkgmocks.NewMockLogger(ctrl)
	handler := NewSupabaseWebhookHandler(nil, mockLogger)

	req := httptest.NewRequest(http.MethodGet, "/webhooks/supabase/before-user-created", nil)
	rr := httptest.NewRecorder()

	handler.handleUserCreatedWebhook(rr, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
}

func TestHandleUserCreatedWebhook_MissingQueryParams(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := pkgmocks.NewMockLogger(ctrl)
	handler := NewSupabaseWebhookHandler(nil, mockLogger)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/supabase/before-user-created", nil)
	rr := httptest.NewRecorder()

	handler.handleUserCreatedWebhook(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleUserCreatedWebhook_MissingHeaders(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := pkgmocks.NewMockLogger(ctrl)
	handler := NewSupabaseWebhookHandler(nil, mockLogger)

	tests := []struct {
		name    string
		headers map[string]string
	}{
		{
			name: "missing webhook-id",
			headers: map[string]string{
				"webhook-timestamp": "1234567890",
				"webhook-signature": "v1,signature123",
			},
		},
		{
			name: "missing webhook-timestamp",
			headers: map[string]string{
				"webhook-id":        "webhook-id-123",
				"webhook-signature": "v1,signature123",
			},
		},
		{
			name: "missing webhook-signature",
			headers: map[string]string{
				"webhook-id":        "webhook-id-123",
				"webhook-timestamp": "1234567890",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/webhooks/supabase/before-user-created?workspace_id=workspace-123&integration_id=integration-456", nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			rr := httptest.NewRecorder()
			handler.handleUserCreatedWebhook(rr, req)

			assert.Equal(t, http.StatusBadRequest, rr.Code)
		})
	}
}

// newSupabaseBodyTestHandler returns a handler with no service: every case below
// must be answered before the service would be used.
func newSupabaseBodyTestHandler(t *testing.T) *SupabaseWebhookHandler {
	ctrl := gomock.NewController(t)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	return NewSupabaseWebhookHandler(nil, mockLogger)
}

func supabaseWebhookRoutes(h *SupabaseWebhookHandler) map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"/webhooks/supabase/auth-email":          h.handleAuthEmailWebhook,
		"/webhooks/supabase/before-user-created": h.handleUserCreatedWebhook,
	}
}

func TestSupabaseWebhooks_BodyOverOneMiBIsRefused(t *testing.T) {
	handler := newSupabaseBodyTestHandler(t)
	for path, handle := range supabaseWebhookRoutes(handler) {
		for name, declared := range map[string]bool{"with Content-Length": true, "chunked": false} {
			t.Run(path+" "+name, func(t *testing.T) {
				req := httptest.NewRequest(http.MethodPost, path+"?workspace_id=workspace-123&integration_id=integration-456",
					bytes.NewReader(bytes.Repeat([]byte(" "), (1<<20)+1)))
				if !declared {
					req.ContentLength = -1
				}
				req.Header.Set("webhook-id", "webhook-id-123")
				req.Header.Set("webhook-timestamp", "1234567890")
				req.Header.Set("webhook-signature", "v1,signature123")
				rr := httptest.NewRecorder()

				handle(rr, req)

				assert.Equal(t, http.StatusRequestEntityTooLarge, rr.Code)
			})
		}
	}
}

func TestSupabaseWebhooks_StalledBodyIsRefused(t *testing.T) {
	handler := newSupabaseBodyTestHandler(t)
	assert.Equal(t, 30*time.Second, handler.bodyReadTimeout)
	handler.bodyReadTimeout = 200 * time.Millisecond
	for path, handle := range supabaseWebhookRoutes(handler) {
		t.Run(path, func(t *testing.T) {
			srv := httptest.NewServer(handle)
			t.Cleanup(srv.Close)

			resp, _, closeErr := rawRequest(t, srv, "POST "+path+"?workspace_id=workspace-123&integration_id=integration-456 HTTP/1.1\r\n"+
				"Host: test\r\nwebhook-id: webhook-id-123\r\nwebhook-timestamp: 1234567890\r\nwebhook-signature: v1,signature123\r\n"+
				"Content-Length: 1024\r\n\r\n{\"user\"", nil)

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
			assert.ErrorIs(t, closeErr, io.EOF)
		})
	}
}

func TestSupabaseWebhooks_RefusedRequestWithAStalledBodyIsClosed(t *testing.T) {
	// The request is refused before its body is read; net/http drains the body
	// before answering, within the deadline set when the handler starts.
	handler := newSupabaseBodyTestHandler(t)
	handler.bodyReadTimeout = 200 * time.Millisecond
	for path, handle := range supabaseWebhookRoutes(handler) {
		t.Run(path, func(t *testing.T) {
			srv := httptest.NewServer(handle)
			t.Cleanup(srv.Close)

			start := time.Now()
			resp, _, closeErr := rawRequest(t, srv, "POST "+path+"?workspace_id=workspace-123&integration_id=integration-456 HTTP/1.1\r\n"+
				"Host: test\r\nContent-Length: 1024\r\n\r\n{\"user\"", nil)

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "the webhook-* headers are missing")
			assert.ErrorIs(t, closeErr, io.EOF)
			assert.Less(t, time.Since(start), 2*time.Second)
		})
	}
}
