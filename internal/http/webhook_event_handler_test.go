package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"aidanwoods.dev/go-paseto"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupWebhookEventHandlerTest(t *testing.T) (*WebhookEventHandler, *mocks.MockWebhookEventServiceInterface, paseto.V4AsymmetricSecretKey) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockService := mocks.NewMockWebhookEventServiceInterface(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Set up logger mock expectations
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	// Create key pair for testing
	secretKey := paseto.NewV4AsymmetricSecretKey()
	publicKey := secretKey.Public()

	handler := NewWebhookEventHandler(
		mockService,
		publicKey,
		mockLogger,
	)

	return handler, mockService, secretKey
}

func createWebhookTestToken(t *testing.T, secretKey paseto.V4AsymmetricSecretKey, userID string) string {
	token := paseto.NewToken()
	token.SetExpiration(time.Now().Add(time.Hour))
	token.SetString(string(domain.UserIDKey), userID)
	token.SetString(string(domain.UserTypeKey), string(domain.UserTypeUser))
	token.SetString(string(domain.SessionIDKey), "test-session")

	signedToken := token.V4Sign(secretKey, nil)
	require.NotEmpty(t, signedToken)
	return signedToken
}

// Tests for handleIncomingWebhook

func TestWebhookEventHandler_handleIncomingWebhook_MethodNotAllowed(t *testing.T) {
	handler, _, _ := setupWebhookEventHandlerTest(t)

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

func TestWebhookEventHandler_handleIncomingWebhook_MissingProvider(t *testing.T) {
	handler, _, _ := setupWebhookEventHandlerTest(t)

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

func TestWebhookEventHandler_handleIncomingWebhook_MissingWorkspaceOrIntegrationID(t *testing.T) {
	handler, _, _ := setupWebhookEventHandlerTest(t)

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

func TestWebhookEventHandler_handleIncomingWebhook_BodyReadError(t *testing.T) {
	handler, _, _ := setupWebhookEventHandlerTest(t)

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

func TestWebhookEventHandler_handleIncomingWebhook_ProcessError(t *testing.T) {
	handler, mockService, _ := setupWebhookEventHandlerTest(t)

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

func TestWebhookEventHandler_handleIncomingWebhook_Success(t *testing.T) {
	handler, mockService, _ := setupWebhookEventHandlerTest(t)

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

func TestWebhookEventHandler_handleList_MethodNotAllowed(t *testing.T) {
	handler, _, _ := setupWebhookEventHandlerTest(t)

	// Create a non-GET request
	req := httptest.NewRequest(http.MethodPost, "/api/webhookEvents.list", nil)
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

func TestWebhookEventHandler_handleList_InvalidParameters(t *testing.T) {
	handler, _, _ := setupWebhookEventHandlerTest(t)

	// Create a request with invalid parameters (missing workspace_id)
	req := httptest.NewRequest(http.MethodGet, "/api/webhookEvents.list", nil)
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

func TestWebhookEventHandler_handleList_ServiceError(t *testing.T) {
	handler, mockService, _ := setupWebhookEventHandlerTest(t)

	// Create a valid request
	req := httptest.NewRequest(http.MethodGet, "/api/webhookEvents.list?workspace_id=ws123", nil)
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
	assert.Equal(t, "Failed to list webhook events", response["error"])
}

func TestWebhookEventHandler_handleList_Success(t *testing.T) {
	handler, mockService, _ := setupWebhookEventHandlerTest(t)

	// Create a valid request with filter parameters
	now := time.Now().UTC()
	reqURL := "/api/webhookEvents.list?workspace_id=ws123&limit=10&event_type=bounce&recipient_email=test@example.com"
	req := httptest.NewRequest(http.MethodGet, reqURL, nil)
	w := httptest.NewRecorder()

	// Create expected events
	events := []*domain.WebhookEvent{
		{
			ID:                "evt1",
			Type:              domain.EmailEventBounce,
			EmailProviderKind: domain.EmailProviderKindSES,
			IntegrationID:     "integration1",
			RecipientEmail:    "test@example.com",
			MessageID:         "message1",
			Timestamp:         now,
			BounceType:        "Permanent",
			BounceCategory:    "General",
			BounceDiagnostic:  "550 User unknown",
			CreatedAt:         now,
		},
	}

	// Create expected result
	expectedResult := &domain.WebhookEventListResult{
		Events:     events,
		NextCursor: "next-cursor",
		HasMore:    true,
	}

	// Mock service to return success
	mockService.EXPECT().
		ListEvents(gomock.Any(), "ws123", gomock.Any()).
		DoAndReturn(func(_ context.Context, workspaceID string, params domain.WebhookEventListParams) (*domain.WebhookEventListResult, error) {
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

	var result domain.WebhookEventListResult
	err := json.NewDecoder(w.Body).Decode(&result)
	require.NoError(t, err)

	assert.Equal(t, 1, len(result.Events))
	assert.Equal(t, "evt1", result.Events[0].ID)
	assert.Equal(t, domain.EmailEventBounce, result.Events[0].Type)
	assert.Equal(t, "test@example.com", result.Events[0].RecipientEmail)
	assert.Equal(t, "next-cursor", result.NextCursor)
	assert.True(t, result.HasMore)
}

func TestWebhookEventHandler_RegisterRoutes(t *testing.T) {
	handler, _, _ := setupWebhookEventHandlerTest(t)

	// Create a new test ServeMux
	mux := http.NewServeMux()

	// Register the routes
	handler.RegisterRoutes(mux)

	// Create test requests
	webhookReq := httptest.NewRequest(http.MethodPost, "/webhooks/email", nil)
	listReq := httptest.NewRequest(http.MethodGet, "/api/webhookEvents.list", nil)

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
