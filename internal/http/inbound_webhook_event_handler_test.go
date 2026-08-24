package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/Notifuse/notifuse/pkg/ratelimiter"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/stretchr/testify/require"
)

func setupInboundWebhookEventHandlerTest(t *testing.T) (*InboundWebhookEventHandler, *mocks.MockInboundWebhookEventServiceInterface, []byte) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockService := mocks.NewMockInboundWebhookEventServiceInterface(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Set up logger mock expectations
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	// Create key pair for testing
	jwtSecret := []byte("test-jwt-secret-key-for-testing-32bytes")
	handler := NewInboundWebhookEventHandler(
		mockService,
		func() ([]byte, error) { return jwtSecret, nil },
		nil, // rate limiter disabled by default in unit tests
		mockLogger,
	)

	return handler, mockService, jwtSecret
}

// Tests for handleIncomingWebhook

func TestInboundWebhookEventHandler_handleIncomingWebhook_MethodNotAllowed(t *testing.T) {
	handler, _, _ := setupInboundWebhookEventHandlerTest(t)

	// Create a non-POST request
	req := httptest.NewRequest(http.MethodGet, "/webhooks/email", nil)
	w := httptest.NewRecorder()

	// Call the handler
	handler.handleIncomingWebhook(w, req)

	// Check response
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)

	var response map[string]string
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "Method not allowed", response["error"])
}

func TestInboundWebhookEventHandler_handleIncomingWebhook_MissingProvider(t *testing.T) {
	handler, _, _ := setupInboundWebhookEventHandlerTest(t)

	// Create a request with no provider
	req := httptest.NewRequest(http.MethodPost, "/webhooks/email?workspace_id=ws123&integration_id=int123", nil)
	w := httptest.NewRecorder()

	// Call the handler
	handler.handleIncomingWebhook(w, req)

	// Check response
	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]string
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "Provider is required", response["error"])
}

func TestInboundWebhookEventHandler_handleIncomingWebhook_MissingWorkspaceOrIntegrationID(t *testing.T) {
	handler, _, _ := setupInboundWebhookEventHandlerTest(t)

	// Create a request with provider but missing workspace_id and integration_id
	req := httptest.NewRequest(http.MethodPost, "/webhooks/email?provider=ses", nil)
	w := httptest.NewRecorder()

	// Call the handler
	handler.handleIncomingWebhook(w, req)

	// Check response
	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]string
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "Workspace ID and integration ID are required", response["error"])
}

func TestInboundWebhookEventHandler_handleIncomingWebhook_BodyReadError(t *testing.T) {
	handler, _, _ := setupInboundWebhookEventHandlerTest(t)

	// Create a request with an erroring body
	req := httptest.NewRequest(http.MethodPost, "/webhooks/email?provider=ses&workspace_id=ws123&integration_id=int123", nil)
	req.Body = io.NopCloser(&errorReader{}) // Use a reader that always returns an error
	w := httptest.NewRecorder()

	// Call the handler
	handler.handleIncomingWebhook(w, req)

	// Check response
	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]string
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "Failed to read request body", response["error"])
}

func TestInboundWebhookEventHandler_handleIncomingWebhook_ProcessError(t *testing.T) {
	handler, mockService, _ := setupInboundWebhookEventHandlerTest(t)

	// Create a valid request
	payload := []byte(`{"event": "test"}`)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/email?provider=ses&workspace_id=ws123&integration_id=int123", bytes.NewReader(payload))
	w := httptest.NewRecorder()

	// Mock service to return an error
	mockService.EXPECT().
		ProcessWebhook(gomock.Any(), "ws123", "int123", payload).
		Return(errors.New("processing error"))

	// Call the handler
	handler.handleIncomingWebhook(w, req)

	// Check response
	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]string
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "Failed to process webhook", response["error"])
}

func TestInboundWebhookEventHandler_handleIncomingWebhook_Success(t *testing.T) {
	handler, mockService, _ := setupInboundWebhookEventHandlerTest(t)

	// Create a valid request
	payload := []byte(`{"event": "test"}`)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/email?provider=ses&workspace_id=ws123&integration_id=int123", bytes.NewReader(payload))
	w := httptest.NewRecorder()

	// Mock service to return success
	mockService.EXPECT().
		ProcessWebhook(gomock.Any(), "ws123", "int123", payload).
		Return(nil)

	// Call the handler
	handler.handleIncomingWebhook(w, req)

	// Check response
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, true, response["success"])
}

// Tests for handleList

func TestInboundWebhookEventHandler_handleList_MethodNotAllowed(t *testing.T) {
	handler, _, _ := setupInboundWebhookEventHandlerTest(t)

	// Create a non-GET request
	req := httptest.NewRequest(http.MethodPost, "/api/inboundWebhookEvents.list", nil)
	w := httptest.NewRecorder()

	// Call the handler
	handler.handleList(w, req)

	// Check response
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)

	var response map[string]string
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "Method not allowed", response["error"])
}

func TestInboundWebhookEventHandler_handleList_InvalidParameters(t *testing.T) {
	handler, _, _ := setupInboundWebhookEventHandlerTest(t)

	// Create a request with invalid parameters (missing workspace_id)
	req := httptest.NewRequest(http.MethodGet, "/api/inboundWebhookEvents.list", nil)
	w := httptest.NewRecorder()

	// Call the handler
	handler.handleList(w, req)

	// Check response
	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]string
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Contains(t, response["error"], "Invalid parameters")
}

func TestInboundWebhookEventHandler_handleList_ServiceError(t *testing.T) {
	handler, mockService, _ := setupInboundWebhookEventHandlerTest(t)

	// Create a valid request
	req := httptest.NewRequest(http.MethodGet, "/api/inboundWebhookEvents.list?workspace_id=ws123", nil)
	w := httptest.NewRecorder()

	// Mock service to return an error
	mockService.EXPECT().
		ListEvents(gomock.Any(), "ws123", gomock.Any()).
		Return(nil, errors.New("service error"))

	// Call the handler
	handler.handleList(w, req)

	// Check response
	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var response map[string]string
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "Failed to list inbound webhook events", response["error"])
}

func TestInboundWebhookEventHandler_handleList_Success(t *testing.T) {
	handler, mockService, _ := setupInboundWebhookEventHandlerTest(t)

	// Create a valid request with filter parameters
	now := time.Now().UTC()
	reqURL := "/api/inboundWebhookEvents.list?workspace_id=ws123&limit=10&event_type=bounce&recipient_email=test@example.com"
	req := httptest.NewRequest(http.MethodGet, reqURL, nil)
	w := httptest.NewRecorder()

	// Create expected events
	messageID := "message1"
	events := []*domain.InboundWebhookEvent{
		{
			ID:               "evt1",
			Type:             domain.EmailEventBounce,
			Source:           domain.WebhookSourceSES,
			IntegrationID:    "integration1",
			RecipientEmail:   "test@example.com",
			MessageID:        &messageID,
			Timestamp:        now,
			BounceType:       "Permanent",
			BounceCategory:   "General",
			BounceDiagnostic: "550 User unknown",
			CreatedAt:        now,
		},
	}

	// Create expected result
	expectedResult := &domain.InboundWebhookEventListResult{
		Events:     events,
		NextCursor: "next-cursor",
		HasMore:    true,
	}

	// Mock service to return success
	mockService.EXPECT().
		ListEvents(gomock.Any(), "ws123", gomock.Any()).
		DoAndReturn(func(_ context.Context, workspaceID string, params domain.InboundWebhookEventListParams) (*domain.InboundWebhookEventListResult, error) {
			assert.Equal(t, "ws123", workspaceID)
			assert.Equal(t, "ws123", params.WorkspaceID)
			assert.Equal(t, 10, params.Limit)
			assert.Equal(t, domain.EmailEventBounce, params.EventType)
			assert.Equal(t, "test@example.com", params.RecipientEmail)
			return expectedResult, nil
		})

	// Call the handler
	handler.handleList(w, req)

	// Check response
	assert.Equal(t, http.StatusOK, w.Code)

	var result domain.InboundWebhookEventListResult
	err := json.NewDecoder(w.Body).Decode(&result)
	require.NoError(t, err)

	assert.Equal(t, 1, len(result.Events))
	assert.Equal(t, "evt1", result.Events[0].ID)
	assert.Equal(t, domain.EmailEventBounce, result.Events[0].Type)
	assert.Equal(t, "test@example.com", result.Events[0].RecipientEmail)
	assert.Equal(t, "next-cursor", result.NextCursor)
	assert.True(t, result.HasMore)
}

func TestInboundWebhookEventHandler_RegisterRoutes(t *testing.T) {
	handler, _, _ := setupInboundWebhookEventHandlerTest(t)

	// Create a new test ServeMux
	mux := http.NewServeMux()

	// Register the routes
	handler.RegisterRoutes(mux)

	// Create test requests
	webhookReq := httptest.NewRequest(http.MethodPost, "/webhooks/email", nil)
	listReq := httptest.NewRequest(http.MethodGet, "/api/inboundWebhookEvents.list", nil)

	// Test that the routes were registered (just checking for no panic)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, webhookReq)

	w = httptest.NewRecorder()
	mux.ServeHTTP(w, listReq)
}

// Custom error reader for testing read errors
type errorReader struct{}

func (r *errorReader) Read(p []byte) (n int, err error) {
	return 0, errors.New("simulated read error")
}

// Tests for handleIncomingReply

func TestInboundWebhookEventHandler_handleIncomingReply_MethodNotAllowed(t *testing.T) {
	handler, _, _ := setupInboundWebhookEventHandlerTest(t)
	req := httptest.NewRequest(http.MethodGet, "/webhooks/email/inbound", nil)
	w := httptest.NewRecorder()
	handler.handleIncomingReply(w, req)
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestInboundWebhookEventHandler_handleIncomingReply_MissingParams(t *testing.T) {
	handler, _, _ := setupInboundWebhookEventHandlerTest(t)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/email/inbound", nil)
	w := httptest.NewRecorder()
	handler.handleIncomingReply(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestInboundWebhookEventHandler_handleIncomingReply_Success(t *testing.T) {
	handler, mockService, _ := setupInboundWebhookEventHandlerTest(t)

	mockService.EXPECT().
		ProcessInboundReply(gomock.Any(), "ws1", "int1", gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ string, parsed *domain.InboundRequest) error {
			assert.Equal(t, "jane@example.com", parsed.Form.Get("sender"))
			return nil
		})

	body := bytes.NewBufferString("sender=jane%40example.com&subject=Re%3A+Hi")
	req := httptest.NewRequest(http.MethodPost, "/webhooks/email/inbound?workspace_id=ws1&integration_id=int1", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	handler.handleIncomingReply(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, true, resp["success"])
}

func TestInboundWebhookEventHandler_handleIncomingReply_ServiceError(t *testing.T) {
	handler, mockService, _ := setupInboundWebhookEventHandlerTest(t)

	mockService.EXPECT().
		ProcessInboundReply(gomock.Any(), "ws1", "int1", gomock.Any()).
		Return(errors.New("db down"))

	req := httptest.NewRequest(http.MethodPost, "/webhooks/email/inbound?workspace_id=ws1&integration_id=int1", bytes.NewBufferString("sender=jane%40example.com"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	handler.handleIncomingReply(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestInboundWebhookEventHandler_handleIncomingReply_PermanentErrorsMapTo4xx(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"integration not found -> 404", fmt.Errorf("x: %w", domain.ErrInboundIntegrationNotFound), http.StatusNotFound},
		{"workspace not found -> 404", fmt.Errorf("x: %w", &domain.ErrWorkspaceNotFound{WorkspaceID: "ws1"}), http.StatusNotFound},
		{"unsupported provider -> 400", fmt.Errorf("x: %w", domain.ErrInboundProviderUnsupported), http.StatusBadRequest},
		{"transient infra -> 500", errors.New("db down"), http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler, mockService, _ := setupInboundWebhookEventHandlerTest(t)
			mockService.EXPECT().ProcessInboundReply(gomock.Any(), "ws1", "int1", gomock.Any()).Return(tc.err)
			req := httptest.NewRequest(http.MethodPost, "/webhooks/email/inbound?workspace_id=ws1&integration_id=int1", bytes.NewBufferString("sender=j%40e.com"))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			w := httptest.NewRecorder()
			handler.handleIncomingReply(w, req)
			assert.Equal(t, tc.want, w.Code)
		})
	}
}

func TestInboundWebhookEventHandler_handleIncomingReply_RateLimited(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockService := mocks.NewMockInboundWebhookEventServiceInterface(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	rl := ratelimiter.NewRateLimiter()
	defer rl.Stop()
	rl.SetPolicy("inbound:ip", 1, time.Minute) // strict: 1 per window
	rl.SetPolicy("inbound:workspace", 100, time.Minute)
	handler := NewInboundWebhookEventHandler(mockService, func() ([]byte, error) { return []byte("k"), nil }, rl, mockLogger)

	// The service must be invoked only for the first (allowed) request, proving the
	// rate-limit rejection happens BEFORE any service/DB work.
	mockService.EXPECT().ProcessInboundReply(gomock.Any(), "ws1", "int1", gomock.Any()).Return(nil).Times(1)

	doReq := func() int {
		req := httptest.NewRequest(http.MethodPost, "/webhooks/email/inbound?workspace_id=ws1&integration_id=int1", bytes.NewBufferString("sender=j%40e.com"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.RemoteAddr = "203.0.113.9:1234"
		w := httptest.NewRecorder()
		handler.handleIncomingReply(w, req)
		return w.Code
	}
	assert.Equal(t, http.StatusOK, doReq())
	assert.Equal(t, http.StatusTooManyRequests, doReq(), "second request from the same IP must be rate-limited")
}

// A missing webhook_events grant is the caller's problem, so it answers 403 with
// the resource and verb it wanted rather than an opaque 500.
func TestInboundWebhookEventHandler_handleList_PermissionDenied(t *testing.T) {
	handler, mockService, _ := setupInboundWebhookEventHandlerTest(t)

	mockService.EXPECT().
		ListEvents(gomock.Any(), "ws123", gomock.Any()).
		Return(nil, domain.NewPermissionError(
			domain.PermissionResourceWebhookEvents,
			domain.PermissionTypeRead,
			"Insufficient permissions: read access to webhook events required",
		))

	req := httptest.NewRequest(http.MethodGet, "/api/inboundWebhookEvents.list?workspace_id=ws123", nil)
	w := httptest.NewRecorder()

	handler.handleList(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)

	var response map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&response))
	assert.Equal(t, string(domain.PermissionResourceWebhookEvents), response["resource"])
	assert.Equal(t, string(domain.PermissionTypeRead), response["permission"])
}
