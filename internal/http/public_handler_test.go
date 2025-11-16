package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"



	"github.com/Notifuse/notifuse/internal/domain"


	"github.com/Notifuse/notifuse/internal/domain/mocks"


	"github.com/Notifuse/notifuse/pkg/logger"


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
			method:             http.MethodPost,
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
			setupMock: func() {
				mockListService.EXPECT().
					UnsubscribeFromLists(gomock.Any(), gomock.Any(), false).
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

			req := httptest.NewRequest(tc.method, "/unsubscribe-oneclick", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
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
