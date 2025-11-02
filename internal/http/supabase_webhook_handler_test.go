package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

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
