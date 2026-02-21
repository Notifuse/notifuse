package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgDatabase "github.com/Notifuse/notifuse/pkg/database"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/pkg/ratelimiter"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNotificationCenterHandler_RegisterRoutes(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockService := mocks.NewMockNotificationCenterService(ctrl)
	mockListService := mocks.NewMockListService(ctrl)
	mockLogger := &mockLogger{}
	handler := NewNotificationCenterHandler(mockService, mockListService, mockLogger, nil)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// Test that the endpoints are registered by making test requests
	// and checking that the request doesn't return 404

	// Test notification center endpoint
	req := httptest.NewRequest(http.MethodGet, "/preferences", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	assert.NotEqual(t, http.StatusNotFound, rec.Code)

	// Test subscribe endpoint
	req = httptest.NewRequest(http.MethodPost, "/subscribe", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	assert.NotEqual(t, http.StatusNotFound, rec.Code)

	// Test unsubscribe endpoint
	req = httptest.NewRequest(http.MethodPost, "/unsubscribe-oneclick", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	assert.NotEqual(t, http.StatusNotFound, rec.Code)
}

func TestNotificationCenterHandler_handleNotificationCenter(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockService := mocks.NewMockNotificationCenterService(ctrl)
	mockListService := mocks.NewMockListService(ctrl)
	mockLogger := &mockLogger{}
	handler := NewNotificationCenterHandler(mockService, mockListService, mockLogger, nil)

	tests := []struct {
		name               string
		method             string
		queryParams        string
		setupMock          func()
		expectedStatusCode int
		expectedResponse   string
	}{
		{
			name:               "method not allowed",
			method:             http.MethodPut,
			queryParams:        "",
			setupMock:          func() {},
			expectedStatusCode: http.StatusMethodNotAllowed,
			expectedResponse:   `{"error":"Method not allowed"}`,
		},
		{
			name:               "missing required parameters",
			method:             http.MethodGet,
			queryParams:        "",
			setupMock:          func() {},
			expectedStatusCode: http.StatusBadRequest,
			expectedResponse:   `{"error":"email is required"}`,
		},
		{
			name:        "service returns error - invalid verification",
			method:      http.MethodGet,
			queryParams: "?email=test@example.com&email_hmac=invalid&workspace_id=ws123",
			setupMock: func() {
				mockService.EXPECT().
					GetContactPreferences(gomock.Any(), "ws123", "test@example.com", "invalid").
					Return(nil, errors.New("invalid email verification"))
			},
			expectedStatusCode: http.StatusUnauthorized,
			expectedResponse:   `{"error":"Unauthorized: invalid verification"}`,
		},
		{
			name:        "service returns error - contact not found",
			method:      http.MethodGet,
			queryParams: "?email=test@example.com&email_hmac=valid&workspace_id=ws123",
			setupMock: func() {
				mockService.EXPECT().
					GetContactPreferences(gomock.Any(), "ws123", "test@example.com", "valid").
					Return(nil, errors.New("contact not found"))
			},
			expectedStatusCode: http.StatusNotFound,
			expectedResponse:   `{"error":"Contact not found"}`,
		},
		{
			name:        "service returns error - other error",
			method:      http.MethodGet,
			queryParams: "?email=test@example.com&email_hmac=valid&workspace_id=ws123",
			setupMock: func() {
				mockService.EXPECT().
					GetContactPreferences(gomock.Any(), "ws123", "test@example.com", "valid").
					Return(nil, errors.New("database error"))
			},
			expectedStatusCode: http.StatusInternalServerError,
			expectedResponse:   `{"error":"Failed to get contact preferences"}`,
		},
		{
			name:        "successful request",
			method:      http.MethodGet,
			queryParams: "?email=test@example.com&email_hmac=valid&workspace_id=ws123",
			setupMock: func() {
				response := &domain.ContactPreferencesResponse{
					Contact:      &domain.Contact{Email: "test@example.com"},
					PublicLists:  []*domain.List{{ID: "list1", Name: "Public List"}},
					ContactLists: []*domain.ContactList{{Email: "test@example.com", ListID: "list1"}},
					LogoURL:      "https://example.com/logo.png",
					WebsiteURL:   "https://example.com",
				}
				mockService.EXPECT().
					GetContactPreferences(gomock.Any(), "ws123", "test@example.com", "valid").
					Return(response, nil)
			},
			expectedStatusCode: http.StatusOK,
			// We'll do a partial match for the response
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.setupMock()

			req := httptest.NewRequest(tc.method, "/preferences"+tc.queryParams, nil)
			rec := httptest.NewRecorder()

			handler.handlePreferences(rec, req)

			assert.Equal(t, tc.expectedStatusCode, rec.Code)

			if tc.expectedResponse != "" {
				assert.JSONEq(t, tc.expectedResponse, rec.Body.String())
			} else if tc.expectedStatusCode == http.StatusOK {
				// For successful requests, verify that the response contains expected fields
				var response domain.ContactPreferencesResponse
				err := json.Unmarshal(rec.Body.Bytes(), &response)
				require.NoError(t, err)
				assert.Equal(t, "test@example.com", response.Contact.Email)
				assert.Len(t, response.PublicLists, 1)
				assert.Equal(t, "list1", response.PublicLists[0].ID)
				assert.Len(t, response.ContactLists, 1)
				assert.Equal(t, "test@example.com", response.ContactLists[0].Email)
				assert.Equal(t, "https://example.com/logo.png", response.LogoURL)
				assert.Equal(t, "https://example.com", response.WebsiteURL)
			}
		})
	}
}

func TestNotificationCenterHandler_handleSubscribe(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockService := mocks.NewMockNotificationCenterService(ctrl)
	mockListService := mocks.NewMockListService(ctrl)
	mockLogger := &mockLogger{}
	handler := NewNotificationCenterHandler(mockService, mockListService, mockLogger, nil)

	validRequest := domain.SubscribeToListsRequest{
		WorkspaceID: "ws123",
		Contact: domain.Contact{
			Email: "test@example.com",
		},
		ListIDs: []string{"list1", "list2"},
	}

	tests := []struct {
		name               string
		method             string
		requestBody        interface{}
		setupMock          func()
		expectedStatusCode int
		expectedResponse   string
	}{
		{
			name:               "method not allowed",
			method:             http.MethodGet,
			requestBody:        nil,
			setupMock:          func() {},
			expectedStatusCode: http.StatusMethodNotAllowed,
			expectedResponse:   `{"error":"Method not allowed"}`,
		},
		{
			name:               "invalid request body - not JSON",
			method:             http.MethodPost,
			requestBody:        "invalid json",
			setupMock:          func() {},
			expectedStatusCode: http.StatusBadRequest,
			expectedResponse:   `{"error":"Invalid request body"}`,
		},
		{
			name:               "invalid request body - missing fields",
			method:             http.MethodPost,
			requestBody:        map[string]interface{}{},
			setupMock:          func() {},
			expectedStatusCode: http.StatusBadRequest,
			expectedResponse:   `{"error":"workspace_id is required"}`,
		},
		{
			name:        "service returns error",
			method:      http.MethodPost,
			requestBody: validRequest,
			setupMock: func() {
				mockListService.EXPECT().
					SubscribeToLists(gomock.Any(), gomock.Any(), false).
					Return(errors.New("subscription failed"))
			},
			expectedStatusCode: http.StatusInternalServerError,
			expectedResponse:   `{"error":"Failed to subscribe to lists"}`,
		},
		{
			name:        "service returns non-public list error",
			method:      http.MethodPost,
			requestBody: validRequest,
			setupMock: func() {
				mockListService.EXPECT().
					SubscribeToLists(gomock.Any(), gomock.Any(), false).
					Return(errors.New("list is not public"))
			},
			expectedStatusCode: http.StatusBadRequest,
			expectedResponse:   `{"error":"list is not public"}`,
		},
		{
			name:        "successful request",
			method:      http.MethodPost,
			requestBody: validRequest,
			setupMock: func() {
				mockListService.EXPECT().
					SubscribeToLists(gomock.Any(), gomock.Any(), false).
					Return(nil)
			},
			expectedStatusCode: http.StatusOK,
			expectedResponse:   `{"success":true}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.setupMock()

			var body []byte
			var err error
			if tc.requestBody != nil {
				switch v := tc.requestBody.(type) {
				case string:
					body = []byte(v)
				default:
					body, err = json.Marshal(tc.requestBody)
					require.NoError(t, err)
				}
			}

			req := httptest.NewRequest(tc.method, "/subscribe", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			handler.handleSubscribe(rec, req)

			assert.Equal(t, tc.expectedStatusCode, rec.Code)
			assert.JSONEq(t, tc.expectedResponse, rec.Body.String())
		})
	}
}

func TestNotificationCenterHandler_handleUnsubscribeOneClick(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockService := mocks.NewMockNotificationCenterService(ctrl)
	mockListService := mocks.NewMockListService(ctrl)
	mockLogger := &mockLogger{}
	handler := NewNotificationCenterHandler(mockService, mockListService, mockLogger, nil)

	validRequest := domain.UnsubscribeFromListsRequest{
		WorkspaceID: "ws123",
		Email:       "test@example.com",
		EmailHMAC:   "valid-hmac",
		ListIDs:     []string{"list1", "list2"},
	}

	tests := []struct {
		name               string
		method             string
		requestBody        interface{}
		userAgent          string
		setupMock          func()
		expectedStatusCode int
		expectedResponse   string
	}{
		{
			name:               "method not allowed",
			method:             http.MethodGet,
			requestBody:        nil,
			setupMock:          func() {},
			expectedStatusCode: http.StatusMethodNotAllowed,
			expectedResponse:   `{"error":"Method not allowed"}`,
		},
		{
			name:               "invalid request body - not JSON",
			method:             http.MethodPost,
			requestBody:        "invalid json",
			setupMock:          func() {},
			expectedStatusCode: http.StatusBadRequest,
			expectedResponse:   `{"error":"Invalid request body"}`,
		},
		{
			name:        "service returns error",
			method:      http.MethodPost,
			requestBody: validRequest,
			userAgent:   "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
			setupMock: func() {
				mockListService.EXPECT().
					UnsubscribeFromLists(gomock.Any(), gomock.Any(), false).
					Return(errors.New("unsubscribe failed"))
			},
			expectedStatusCode: http.StatusInternalServerError,
			expectedResponse:   `{"error":"Failed to unsubscribe from lists"}`,
		},
		{
			name:        "successful request",
			method:      http.MethodPost,
			requestBody: validRequest,
			userAgent:   "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
			setupMock: func() {
				mockListService.EXPECT().
					UnsubscribeFromLists(gomock.Any(), gomock.Any(), false).
					Return(nil)
			},
			expectedStatusCode: http.StatusOK,
			expectedResponse:   `{"success":true}`,
		},
		{
			name:        "bot user agent - returns success without unsubscribing",
			method:      http.MethodPost,
			requestBody: validRequest,
			userAgent:   "Mozilla/5.0 (compatible; SafeLinks/1.0; +http://www.microsoft.com/safelinks)",
			setupMock: func() {
				// No mock expectation - service should not be called for bots
			},
			expectedStatusCode: http.StatusOK,
			expectedResponse:   `{"success":true}`,
		},
		{
			name:        "email scanner bot - returns success without unsubscribing",
			method:      http.MethodPost,
			requestBody: validRequest,
			userAgent:   "Proofpoint Email Security Scanner",
			setupMock: func() {
				// No mock expectation - service should not be called for bots
			},
			expectedStatusCode: http.StatusOK,
			expectedResponse:   `{"success":true}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.setupMock()

			var body []byte
			var err error
			if tc.requestBody != nil {
				switch v := tc.requestBody.(type) {
				case string:
					body = []byte(v)
				default:
					body, err = json.Marshal(tc.requestBody)
					require.NoError(t, err)
				}
			}

			req := httptest.NewRequest(tc.method, "/unsubscribe-oneclick", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			if tc.userAgent != "" {
				req.Header.Set("User-Agent", tc.userAgent)
			}
			rec := httptest.NewRecorder()

			handler.handleUnsubscribeOneClick(rec, req)

			assert.Equal(t, tc.expectedStatusCode, rec.Code)
			assert.JSONEq(t, tc.expectedResponse, rec.Body.String())
		})
	}
}

// Mock logger for testing
type mockLogger struct {
}

func (l *mockLogger) Debug(msg string) {}
func (l *mockLogger) Info(msg string)  {}
func (l *mockLogger) Warn(msg string)  {}
func (l *mockLogger) Error(msg string) {}
func (l *mockLogger) Fatal(msg string) {}

func (l *mockLogger) WithField(key string, value interface{}) logger.Logger {
	return l
}

func (l *mockLogger) WithFields(fields map[string]interface{}) logger.Logger {
	return l
}

func (l *mockLogger) WithError(err error) logger.Logger {
	return l
}

func (l *mockLogger) GetLevel() string {
	return "debug"
}

func (l *mockLogger) SetLevel(level string) {}

// Test NewNotificationCenterHandler function
func TestNewNotificationCenterHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockService := mocks.NewMockNotificationCenterService(ctrl)
	mockListService := mocks.NewMockListService(ctrl)
	mockLogger := &mockLogger{}

	handler := NewNotificationCenterHandler(mockService, mockListService, mockLogger, nil)

	assert.NotNil(t, handler)
	assert.Equal(t, mockService, handler.service)
	assert.Equal(t, mockListService, handler.listService)
	assert.Equal(t, mockLogger, handler.logger)
}

func TestNotificationCenterHandler_handleUpdatePreferences(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockService := mocks.NewMockNotificationCenterService(ctrl)
	mockListService := mocks.NewMockListService(ctrl)
	mockLogger := &mockLogger{}
	handler := NewNotificationCenterHandler(mockService, mockListService, mockLogger, nil)

	tests := []struct {
		name               string
		requestBody        interface{}
		setupMock          func()
		expectedStatusCode int
		expectedResponse   string
	}{
		{
			name:               "invalid JSON body",
			requestBody:        "not json",
			setupMock:          func() {},
			expectedStatusCode: http.StatusBadRequest,
			expectedResponse:   `{"error":"Invalid request body"}`,
		},
		{
			name:               "validation failure - missing fields",
			requestBody:        map[string]interface{}{"language": "fr"},
			setupMock:          func() {},
			expectedStatusCode: http.StatusBadRequest,
			expectedResponse:   `{"error":"workspace_id is required"}`,
		},
		{
			name: "validation failure - no language or timezone",
			requestBody: map[string]interface{}{
				"workspace_id": "ws123",
				"email":        "test@example.com",
				"email_hmac":   "hmac",
			},
			setupMock:          func() {},
			expectedStatusCode: http.StatusBadRequest,
			expectedResponse:   `{"error":"at least one of language or timezone must be provided"}`,
		},
		{
			name: "service returns HMAC error",
			requestBody: domain.UpdateContactPreferencesRequest{
				WorkspaceID: "ws123",
				Email:       "test@example.com",
				EmailHMAC:   "invalid",
				Language:    "fr",
			},
			setupMock: func() {
				mockService.EXPECT().
					UpdateContactPreferences(gomock.Any(), gomock.Any()).
					Return(errors.New("invalid email verification"))
			},
			expectedStatusCode: http.StatusUnauthorized,
			expectedResponse:   `{"error":"Unauthorized: invalid verification"}`,
		},
		{
			name: "service returns other error",
			requestBody: domain.UpdateContactPreferencesRequest{
				WorkspaceID: "ws123",
				Email:       "test@example.com",
				EmailHMAC:   "valid",
				Language:    "fr",
			},
			setupMock: func() {
				mockService.EXPECT().
					UpdateContactPreferences(gomock.Any(), gomock.Any()).
					Return(errors.New("database error"))
			},
			expectedStatusCode: http.StatusInternalServerError,
			expectedResponse:   `{"error":"Failed to update contact preferences"}`,
		},
		{
			name: "successful update",
			requestBody: domain.UpdateContactPreferencesRequest{
				WorkspaceID: "ws123",
				Email:       "test@example.com",
				EmailHMAC:   "valid",
				Language:    "fr",
				Timezone:    "Europe/Paris",
			},
			setupMock: func() {
				mockService.EXPECT().
					UpdateContactPreferences(gomock.Any(), gomock.Any()).
					Return(nil)
			},
			expectedStatusCode: http.StatusOK,
			expectedResponse:   `{"success":true}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.setupMock()

			var body []byte
			var err error
			switch v := tc.requestBody.(type) {
			case string:
				body = []byte(v)
			default:
				body, err = json.Marshal(tc.requestBody)
				require.NoError(t, err)
			}

			req := httptest.NewRequest(http.MethodPost, "/preferences", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			handler.handlePreferences(rec, req)

			assert.Equal(t, tc.expectedStatusCode, rec.Code)
			assert.JSONEq(t, tc.expectedResponse, rec.Body.String())
		})
	}
}

func TestNotificationCenterHandler_handleUpdatePreferences_RateLimiting(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockService := mocks.NewMockNotificationCenterService(ctrl)
	mockListService := mocks.NewMockListService(ctrl)
	mockLogger := &mockLogger{}

	rl := ratelimiter.NewRateLimiter()
	defer rl.Stop()
	// Allow only 1 request per 60s window
	rl.SetPolicy("preferences:email", 1, 60*time.Second)
	rl.SetPolicy("preferences:ip", 1, 60*time.Second)

	handler := NewNotificationCenterHandler(mockService, mockListService, mockLogger, rl)

	validReq := domain.UpdateContactPreferencesRequest{
		WorkspaceID: "ws123",
		Email:       "ratelimit@example.com",
		EmailHMAC:   "valid",
		Language:    "fr",
	}

	// First request should succeed
	mockService.EXPECT().
		UpdateContactPreferences(gomock.Any(), gomock.Any()).
		Return(nil)

	body, err := json.Marshal(validReq)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/preferences", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.handlePreferences(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Second request should be rate limited
	body, err = json.Marshal(validReq)
	require.NoError(t, err)

	req = httptest.NewRequest(http.MethodPost, "/preferences", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.handlePreferences(rec, req)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.NotEmpty(t, rec.Header().Get("Retry-After"))
}

func TestNotificationCenterHandler_handleHealth(t *testing.T) {
	// Test NotificationCenterHandler.handleHealth - this was at 0% coverage
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockService := mocks.NewMockNotificationCenterService(ctrl)
	mockListService := mocks.NewMockListService(ctrl)
	mockLogger := &mockLogger{}
	handler := NewNotificationCenterHandler(mockService, mockListService, mockLogger, nil)

	// Initialize connection manager for testing
	cfg := &config.Config{
		Database: config.DatabaseConfig{
			MaxConnections:      100,
			MaxConnectionsPerDB: 10,
		},
	}
	mockDB, _, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = mockDB.Close() }()

	err = pkgDatabase.InitializeConnectionManager(cfg, mockDB)
	require.NoError(t, err)
	defer pkgDatabase.ResetConnectionManager()

	t.Run("successful health check", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		w := httptest.NewRecorder()

		handler.handleHealth(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Contains(t, response, "max_connections")
		assert.Contains(t, response, "system_connections")
	})

	t.Run("method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/health", nil)
		w := httptest.NewRecorder()

		handler.handleHealth(w, req)

		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

func TestNotificationCenterHandler_handleHealthz(t *testing.T) {
	// Test NotificationCenterHandler.handleHealthz - this was at 0% coverage
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockService := mocks.NewMockNotificationCenterService(ctrl)
	mockListService := mocks.NewMockListService(ctrl)
	mockLogger := &mockLogger{}
	handler := NewNotificationCenterHandler(mockService, mockListService, mockLogger, nil)

	// Initialize connection manager for testing
	cfg := &config.Config{
		Database: config.DatabaseConfig{
			MaxConnections:      100,
			MaxConnectionsPerDB: 10,
		},
	}
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = mockDB.Close() }()

	// Mock ping query for healthz check
	mock.ExpectPing()

	err = pkgDatabase.InitializeConnectionManager(cfg, mockDB)
	require.NoError(t, err)
	defer pkgDatabase.ResetConnectionManager()

	t.Run("successful healthz check", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		w := httptest.NewRecorder()

		handler.handleHealthz(w, req)

		// May return OK or ServiceUnavailable depending on DB ping
		assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusServiceUnavailable)
	})

	t.Run("method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/healthz", nil)
		w := httptest.NewRecorder()

		handler.handleHealthz(w, req)

		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}
