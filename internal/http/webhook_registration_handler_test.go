package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/internal/http/middleware"
	"github.com/Notifuse/notifuse/internal/service"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang-jwt/jwt/v5"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/stretchr/testify/require"
)

func setupWebhookRegistrationHandlerTest(t *testing.T) (*mocks.MockWebhookRegistrationService, *pkgmocks.MockLogger, *WebhookRegistrationHandler, *http.ServeMux, []byte, *gomock.Controller, func()) {
	ctrl := gomock.NewController(t)
	mockService := mocks.NewMockWebhookRegistrationService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Create logger stubs for chained calls
	mockLoggerWithField := pkgmocks.NewMockLogger(ctrl)
	mockLoggerWithFields := pkgmocks.NewMockLogger(ctrl)

	// Set up more expectations for logger chains
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLoggerWithField).AnyTimes()
	mockLoggerWithField.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLoggerWithFields).AnyTimes()
	mockLoggerWithField.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLoggerWithFields.EXPECT().Error(gomock.Any()).AnyTimes()

	// Set up more chain expectations
	mockLoggerWithFields.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLoggerWithFields).AnyTimes()

	// Add missing expectations for frequently called logger methods
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()

	// Generate a key pair for testing
	jwtSecret := []byte("test-jwt-secret-key-for-testing-32bytes")
	handler := NewWebhookRegistrationHandler(mockService, func() ([]byte, error) { return jwtSecret, nil }, mockLogger)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	cleanup := func() {
		ctrl.Finish()
	}

	return mockService, mockLogger, handler, mux, jwtSecret, ctrl, cleanup
}

func createAuthToken(t *testing.T, jwtSecret []byte, workspaceID, userID string) string {
	claims := &service.UserClaims{
		UserID:    userID,
		Type:      string(domain.UserTypeUser),
		SessionID: "test-session",
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(10 * time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(jwtSecret)
	require.NoError(t, err)
	return signed
}

func TestWebhookRegistrationHandler_handleRegister(t *testing.T) {
	workspaceID := "workspace-123"
	userID := "user-123"

	t.Run("successful registration", func(t *testing.T) {
		mockService, _, _, mux, jwtSecret, _, cleanup := setupWebhookRegistrationHandlerTest(t)
		defer cleanup()

		// Prepare test data
		reqBody := domain.RegisterWebhookRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: "integration-123",
			EventTypes:    []domain.EmailEventType{domain.EmailEventDelivered, domain.EmailEventBounce},
		}

		// Expected status
		expectedStatus := &domain.WebhookRegistrationStatus{
			EmailProviderKind: domain.EmailProviderKindSES,
			IsRegistered:      true,
			Endpoints: []domain.WebhookEndpointStatus{
				{
					WebhookID: "webhook-123",
					URL:       "https://example.com/webhook",
					EventType: domain.EmailEventDelivered,
					Active:    true,
				},
			},
			ProviderDetails: map[string]interface{}{
				"integration_id": "integration-123",
			},
		}

		// Setup mock response
		mockService.EXPECT().
			RegisterWebhooks(
				gomock.Any(),
				reqBody.WorkspaceID,
				&domain.WebhookRegistrationConfig{
					IntegrationID: reqBody.IntegrationID,
					EventTypes:    reqBody.EventTypes,
				},
			).
			Return(expectedStatus, nil)

		// Create request
		jsonBody, err := json.Marshal(reqBody)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/webhooks.register", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+createAuthToken(t, jwtSecret, workspaceID, userID))

		// Create response recorder
		rr := httptest.NewRecorder()

		// Serve the request
		mux.ServeHTTP(rr, req)

		// Assert response
		assert.Equal(t, http.StatusOK, rr.Code)

		var respBody map[string]interface{}
		err = json.Unmarshal(rr.Body.Bytes(), &respBody)
		require.NoError(t, err)

		status, ok := respBody["status"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, true, status["is_registered"])
		assert.Equal(t, "ses", status["email_provider_kind"])
	})

	t.Run("method not allowed", func(t *testing.T) {
		_, _, _, mux, jwtSecret, _, cleanup := setupWebhookRegistrationHandlerTest(t)
		defer cleanup()

		req := httptest.NewRequest(http.MethodGet, "/api/webhooks.register", nil)
		req.Header.Set("Authorization", "Bearer "+createAuthToken(t, jwtSecret, workspaceID, userID))

		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("invalid request body", func(t *testing.T) {
		_, _, _, mux, jwtSecret, _, cleanup := setupWebhookRegistrationHandlerTest(t)
		defer cleanup()

		req := httptest.NewRequest(http.MethodPost, "/api/webhooks.register", bytes.NewBuffer([]byte("invalid json")))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+createAuthToken(t, jwtSecret, workspaceID, userID))

		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("invalid request payload", func(t *testing.T) {
		_, _, _, mux, jwtSecret, _, cleanup := setupWebhookRegistrationHandlerTest(t)
		defer cleanup()

		// Missing required fields
		reqBody := map[string]interface{}{
			"workspace_id": workspaceID,
			// Missing integration_id and event_types
		}

		jsonBody, err := json.Marshal(reqBody)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/webhooks.register", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+createAuthToken(t, jwtSecret, workspaceID, userID))

		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("service error", func(t *testing.T) {
		mockService, _, handler, _, jwtSecret, _, cleanup := setupWebhookRegistrationHandlerTest(t)
		defer cleanup()

		reqBody := domain.RegisterWebhookRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: "integration-123",
			EventTypes:    []domain.EmailEventType{domain.EmailEventDelivered},
		}

		// Setup a custom test with specific mocks
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// Mock the logger with specific expectations
		mockLogger := pkgmocks.NewMockLogger(ctrl)
		errorLogger := pkgmocks.NewMockLogger(ctrl)
		mockLogger.EXPECT().WithField("error", "service error").Return(errorLogger)
		errorLogger.EXPECT().WithField("workspace_id", workspaceID).Return(errorLogger)
		errorLogger.EXPECT().WithField("integration_id", "integration-123").Return(errorLogger)
		errorLogger.EXPECT().Error("Failed to register webhooks")

		// Use the real handler but with our new mock logger
		handler.logger = mockLogger

		// Setup service mock
		mockService.EXPECT().
			RegisterWebhooks(
				gomock.Any(),
				reqBody.WorkspaceID,
				&domain.WebhookRegistrationConfig{
					IntegrationID: reqBody.IntegrationID,
					EventTypes:    reqBody.EventTypes,
				},
			).
			Return(nil, errors.New("service error"))

		jsonBody, err := json.Marshal(reqBody)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/webhooks.register", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+createAuthToken(t, jwtSecret, workspaceID, userID))

		// Create a test server with our handler
		mux := http.NewServeMux()

		getJWTSecret := func() ([]byte, error) {
			return jwtSecret, nil
		}
		authMiddleware := middleware.NewAuthMiddleware(getJWTSecret)
		requireAuth := authMiddleware.RequireAuth()
		mux.Handle("/api/webhooks.register", requireAuth(http.HandlerFunc(handler.handleRegister)))

		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusInternalServerError, rr.Code)
		assert.Contains(t, rr.Body.String(), "Failed to register webhooks")
	})

	t.Run("unauthorized access", func(t *testing.T) {
		_, _, _, mux, _, _, cleanup := setupWebhookRegistrationHandlerTest(t)
		defer cleanup()

		reqBody := domain.RegisterWebhookRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: "integration-123",
			EventTypes:    []domain.EmailEventType{domain.EmailEventDelivered},
		}

		jsonBody, err := json.Marshal(reqBody)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/webhooks.register", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		// No authorization header

		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})
}

func TestWebhookRegistrationHandler_handleStatus(t *testing.T) {
	workspaceID := "workspace-123"
	integrationID := "integration-123"
	userID := "user-123"

	t.Run("successful status retrieval", func(t *testing.T) {
		mockService, _, _, mux, jwtSecret, _, cleanup := setupWebhookRegistrationHandlerTest(t)
		defer cleanup()

		// Expected status
		expectedStatus := &domain.WebhookRegistrationStatus{
			EmailProviderKind: domain.EmailProviderKindPostmark,
			IsRegistered:      true,
			Endpoints: []domain.WebhookEndpointStatus{
				{
					WebhookID: "webhook-123",
					URL:       "https://example.com/webhook",
					EventType: domain.EmailEventDelivered,
					Active:    true,
				},
			},
			ProviderDetails: map[string]interface{}{
				"integration_id": integrationID,
			},
		}

		// Setup mock response
		mockService.EXPECT().
			GetWebhookStatus(gomock.Any(), workspaceID, integrationID).
			Return(expectedStatus, nil)

		// Create request
		req := httptest.NewRequest(http.MethodGet, "/api/webhooks.status?workspace_id="+workspaceID+"&integration_id="+integrationID, nil)
		req.Header.Set("Authorization", "Bearer "+createAuthToken(t, jwtSecret, workspaceID, userID))

		// Create response recorder
		rr := httptest.NewRecorder()

		// Serve the request
		mux.ServeHTTP(rr, req)

		// Assert response
		assert.Equal(t, http.StatusOK, rr.Code)

		var respBody map[string]interface{}
		err := json.Unmarshal(rr.Body.Bytes(), &respBody)
		require.NoError(t, err)

		status, ok := respBody["status"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, true, status["is_registered"])
		assert.Equal(t, "postmark", status["email_provider_kind"])
	})

	t.Run("method not allowed", func(t *testing.T) {
		_, _, _, mux, jwtSecret, _, cleanup := setupWebhookRegistrationHandlerTest(t)
		defer cleanup()

		req := httptest.NewRequest(http.MethodPost, "/api/webhooks.status?workspace_id="+workspaceID+"&integration_id="+integrationID, nil)
		req.Header.Set("Authorization", "Bearer "+createAuthToken(t, jwtSecret, workspaceID, userID))

		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("missing workspace_id parameter", func(t *testing.T) {
		_, _, _, mux, jwtSecret, _, cleanup := setupWebhookRegistrationHandlerTest(t)
		defer cleanup()

		req := httptest.NewRequest(http.MethodGet, "/api/webhooks.status?integration_id="+integrationID, nil)
		req.Header.Set("Authorization", "Bearer "+createAuthToken(t, jwtSecret, workspaceID, userID))

		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("missing integration_id parameter", func(t *testing.T) {
		_, _, _, mux, jwtSecret, _, cleanup := setupWebhookRegistrationHandlerTest(t)
		defer cleanup()

		req := httptest.NewRequest(http.MethodGet, "/api/webhooks.status?workspace_id="+workspaceID, nil)
		req.Header.Set("Authorization", "Bearer "+createAuthToken(t, jwtSecret, workspaceID, userID))

		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("service error", func(t *testing.T) {
		mockService, _, handler, _, jwtSecret, _, cleanup := setupWebhookRegistrationHandlerTest(t)
		defer cleanup()

		// Setup a custom test with specific mocks
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// Mock the logger with specific expectations
		mockLogger := pkgmocks.NewMockLogger(ctrl)
		errorLogger := pkgmocks.NewMockLogger(ctrl)
		mockLogger.EXPECT().WithField("error", "service error").Return(errorLogger)
		errorLogger.EXPECT().WithField("workspace_id", workspaceID).Return(errorLogger)
		errorLogger.EXPECT().WithField("integration_id", integrationID).Return(errorLogger)
		errorLogger.EXPECT().Error("Failed to get webhook status")

		// Use the real handler but with our new mock logger
		handler.logger = mockLogger

		mockService.EXPECT().
			GetWebhookStatus(gomock.Any(), workspaceID, integrationID).
			Return(nil, errors.New("service error"))

		req := httptest.NewRequest(http.MethodGet, "/api/webhooks.status?workspace_id="+workspaceID+"&integration_id="+integrationID, nil)
		req.Header.Set("Authorization", "Bearer "+createAuthToken(t, jwtSecret, workspaceID, userID))

		// Create a test server with our handler
		mux := http.NewServeMux()

		getJWTSecret := func() ([]byte, error) {
			return jwtSecret, nil
		}
		authMiddleware := middleware.NewAuthMiddleware(getJWTSecret)
		requireAuth := authMiddleware.RequireAuth()
		mux.Handle("/api/webhooks.status", requireAuth(http.HandlerFunc(handler.handleStatus)))

		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusInternalServerError, rr.Code)
		assert.Contains(t, rr.Body.String(), "Failed to get webhook status")
	})

	t.Run("unauthorized access", func(t *testing.T) {
		_, _, _, mux, _, _, cleanup := setupWebhookRegistrationHandlerTest(t)
		defer cleanup()

		req := httptest.NewRequest(http.MethodGet, "/api/webhooks.status?workspace_id="+workspaceID+"&integration_id="+integrationID, nil)
		// No authorization header

		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})
}
