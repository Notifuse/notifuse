package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
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

// Helper function to set up the test environment for transactional handler tests
func setupTransactionalHandlerTest(t *testing.T) (*mocks.MockTransactionalNotificationService, *pkgmocks.MockLogger, *TransactionalNotificationHandler) {
	ctrl := gomock.NewController(t)
	mockService := mocks.NewMockTransactionalNotificationService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// For tests we don't need the actual key, we can create a new one
	secretKey := paseto.NewV4AsymmetricSecretKey()
	publicKey := secretKey.Public()

	handler := NewTransactionalNotificationHandler(mockService, func() (paseto.V4AsymmetricPublicKey, error) { return publicKey, nil }, mockLogger, false)

	return mockService, mockLogger, handler
}

// Helper to create a sample transactional notification
func createTestTransactionalNotification() *domain.TransactionalNotification {
	now := time.Now().UTC()
	return &domain.TransactionalNotification{
		ID:          "test-notification",
		Name:        "Test Notification",
		Description: "Test notification description",
		Channels: domain.ChannelTemplates{
			domain.TransactionalChannelEmail: {
				TemplateID: "template-123",
				Settings: domain.MapOfAny{
					"subject": "Test Subject",
				},
			},
		},
		Metadata:  domain.MapOfAny{"category": "test"},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func TestNewTransactionalNotificationHandler(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	mockService := mocks.NewMockTransactionalNotificationService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	secretKey := paseto.NewV4AsymmetricSecretKey()
	publicKey := secretKey.Public()

	// Act
	handler := NewTransactionalNotificationHandler(mockService, func() (paseto.V4AsymmetricPublicKey, error) { return publicKey, nil }, mockLogger, false)

	// Assert
	assert.NotNil(t, handler)
	assert.Equal(t, mockService, handler.service)
	assert.NotNil(t, handler.getPublicKey)
	assert.Equal(t, mockLogger, handler.logger)
}

func TestTransactionalNotificationHandler_RegisterRoutes(t *testing.T) {
	// Arrange
	_, _, handler := setupTransactionalHandlerTest(t)

	// Create a multiplexer to register routes with
	mux := http.NewServeMux()

	// Act - Register routes with the mux
	handler.RegisterRoutes(mux)

	// Create a test server with the mux
	server := httptest.NewServer(mux)
	defer server.Close()

	// Test routes (we'll make unauthenticated requests which should return 401)
	routesToTest := []string{
		"/api/transactional.list",
		"/api/transactional.get",
		"/api/transactional.create",
		"/api/transactional.update",
		"/api/transactional.delete",
		"/api/transactional.send",
	}

	// Make requests to verify routes are registered
	for _, route := range routesToTest {
		t.Run(route, func(t *testing.T) {
			req, err := http.NewRequest(routeMethod(route), server.URL+route, nil)
			require.NoError(t, err)

			client := &http.Client{}
			resp, err := client.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			// We expect 401 Unauthorized since we didn't authenticate
			// The important part is that the route exists and returns a response
			assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		})
	}
}

// Helper to determine appropriate method for testing routes
func routeMethod(route string) string {
	if strings.HasSuffix(route, ".list") || strings.HasSuffix(route, ".get") {
		return http.MethodGet
	}
	return http.MethodPost
}

func TestTransactionalNotificationHandler_HandleList(t *testing.T) {
	mockService, mockLogger, handler := setupTransactionalHandlerTest(t)

	workspaceID := "workspace1"

	testCases := []struct {
		name           string
		method         string
		queryParams    url.Values
		setupMock      func()
		expectedStatus int
		checkResponse  func(t *testing.T, response map[string]interface{})
	}{
		{
			name:   "method not allowed",
			method: http.MethodPost,
			queryParams: url.Values{
				"workspace_id": []string{workspaceID},
			},
			setupMock:      func() {},
			expectedStatus: http.StatusMethodNotAllowed,
			checkResponse:  nil,
		},
		{
			name:           "missing workspace ID",
			method:         http.MethodGet,
			queryParams:    url.Values{},
			setupMock:      func() {},
			expectedStatus: http.StatusBadRequest,
			checkResponse:  nil,
		},
		{
			name:   "successful empty list",
			method: http.MethodGet,
			queryParams: url.Values{
				"workspace_id": []string{workspaceID},
			},
			setupMock: func() {
				mockService.EXPECT().
					ListNotifications(gomock.Any(), workspaceID, gomock.Any(), gomock.Any(), gomock.Any()).
					Return([]*domain.TransactionalNotification{}, 0, nil)
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, response map[string]interface{}) {
				notifications, ok := response["notifications"].([]interface{})
				assert.True(t, ok)
				assert.Empty(t, notifications)
				assert.Equal(t, float64(0), response["total"])
			},
		},
		{
			name:   "successful list with notifications",
			method: http.MethodGet,
			queryParams: url.Values{
				"workspace_id": []string{workspaceID},
				"limit":        []string{"10"},
				"offset":       []string{"0"},
			},
			setupMock: func() {
				notification := createTestTransactionalNotification()
				mockService.EXPECT().
					ListNotifications(
						gomock.Any(),
						workspaceID,
						gomock.Any(),
						10,
						0,
					).
					Return([]*domain.TransactionalNotification{notification}, 1, nil)
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, response map[string]interface{}) {
				notifications, ok := response["notifications"].([]interface{})
				assert.True(t, ok)
				assert.Len(t, notifications, 1)
				assert.Equal(t, float64(1), response["total"])

				notification := notifications[0].(map[string]interface{})
				assert.Equal(t, "test-notification", notification["id"])
				assert.Equal(t, "Test Notification", notification["name"])
			},
		},
		{
			name:   "service error",
			method: http.MethodGet,
			queryParams: url.Values{
				"workspace_id": []string{workspaceID},
			},
			setupMock: func() {
				mockService.EXPECT().
					ListNotifications(gomock.Any(), workspaceID, gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, 0, errors.New("service error"))

				mockLogger.EXPECT().
					WithField("error", "service error").
					Return(mockLogger)
				mockLogger.EXPECT().
					Error("Failed to list transactional notifications")
			},
			expectedStatus: http.StatusInternalServerError,
			checkResponse:  nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup mock expectations
			tc.setupMock()

			// Create request
			req := httptest.NewRequest(tc.method, "/api/transactional.list?"+tc.queryParams.Encode(), nil)
			w := httptest.NewRecorder()

			// Call handler method
			handler.handleList(w, req)

			// Check status code
			assert.Equal(t, tc.expectedStatus, w.Code)

			// If success case, check response content
			if tc.expectedStatus == http.StatusOK && tc.checkResponse != nil {
				var response map[string]interface{}
				err := json.NewDecoder(w.Body).Decode(&response)
				require.NoError(t, err)
				tc.checkResponse(t, response)
			}
		})
	}
}

func TestTransactionalNotificationHandler_HandleGet(t *testing.T) {
	mockService, mockLogger, handler := setupTransactionalHandlerTest(t)

	workspaceID := "workspace1"
	notificationID := "test-notification"

	testCases := []struct {
		name           string
		method         string
		queryParams    url.Values
		setupMock      func()
		expectedStatus int
		checkResponse  func(t *testing.T, response map[string]interface{})
	}{
		{
			name:   "method not allowed",
			method: http.MethodPost,
			queryParams: url.Values{
				"workspace_id": []string{workspaceID},
				"id":           []string{notificationID},
			},
			setupMock:      func() {},
			expectedStatus: http.StatusMethodNotAllowed,
			checkResponse:  nil,
		},
		{
			name:   "missing workspace ID",
			method: http.MethodGet,
			queryParams: url.Values{
				"id": []string{notificationID},
			},
			setupMock:      func() {},
			expectedStatus: http.StatusBadRequest,
			checkResponse:  nil,
		},
		{
			name:   "missing notification ID",
			method: http.MethodGet,
			queryParams: url.Values{
				"workspace_id": []string{workspaceID},
			},
			setupMock:      func() {},
			expectedStatus: http.StatusBadRequest,
			checkResponse:  nil,
		},
		{
			name:   "notification not found",
			method: http.MethodGet,
			queryParams: url.Values{
				"workspace_id": []string{workspaceID},
				"id":           []string{notificationID},
			},
			setupMock: func() {
				mockService.EXPECT().
					GetNotification(gomock.Any(), workspaceID, notificationID).
					Return(nil, errors.New("notification not found"))
			},
			expectedStatus: http.StatusNotFound,
			checkResponse:  nil,
		},
		{
			name:   "successful get",
			method: http.MethodGet,
			queryParams: url.Values{
				"workspace_id": []string{workspaceID},
				"id":           []string{notificationID},
			},
			setupMock: func() {
				notification := createTestTransactionalNotification()
				mockService.EXPECT().
					GetNotification(gomock.Any(), workspaceID, notificationID).
					Return(notification, nil)
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, response map[string]interface{}) {
				notification, ok := response["notification"].(map[string]interface{})
				assert.True(t, ok)
				assert.Equal(t, notificationID, notification["id"])
				assert.Equal(t, "Test Notification", notification["name"])
				assert.Equal(t, "Test notification description", notification["description"])
			},
		},
		{
			name:   "service error",
			method: http.MethodGet,
			queryParams: url.Values{
				"workspace_id": []string{workspaceID},
				"id":           []string{notificationID},
			},
			setupMock: func() {
				mockService.EXPECT().
					GetNotification(gomock.Any(), workspaceID, notificationID).
					Return(nil, errors.New("service error"))

				mockLogger.EXPECT().
					WithField(gomock.Eq("error"), gomock.Eq("service error")).
					Return(mockLogger)
				mockLogger.EXPECT().
					Error(gomock.Eq("Failed to get transactional notification"))
			},
			expectedStatus: http.StatusInternalServerError,
			checkResponse:  nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup mock expectations
			tc.setupMock()

			// Create request
			req := httptest.NewRequest(tc.method, "/api/transactional.get?"+tc.queryParams.Encode(), nil)
			w := httptest.NewRecorder()

			// Call handler method
			handler.handleGet(w, req)

			// Check status code
			assert.Equal(t, tc.expectedStatus, w.Code)

			// If success case, check response content
			if tc.expectedStatus == http.StatusOK && tc.checkResponse != nil {
				var response map[string]interface{}
				err := json.NewDecoder(w.Body).Decode(&response)
				require.NoError(t, err)
				tc.checkResponse(t, response)
			}
		})
	}
}

func TestTransactionalNotificationHandler_HandleCreate(t *testing.T) {
	mockService, mockLogger, handler := setupTransactionalHandlerTest(t)

	workspaceID := "workspace1"

	// Valid request body for creation
	validCreateParams := domain.TransactionalNotificationCreateParams{
		ID:          "test-notification",
		Name:        "Test Notification",
		Description: "Test notification description",
		Channels: domain.ChannelTemplates{
			domain.TransactionalChannelEmail: {
				TemplateID: "template-123",
				Settings: domain.MapOfAny{
					"subject": "Test Subject",
				},
			},
		},
	}

	validReqBody := domain.CreateTransactionalRequest{
		WorkspaceID:  workspaceID,
		Notification: validCreateParams,
	}

	testCases := []struct {
		name           string
		method         string
		requestBody    interface{}
		setupMock      func()
		expectedStatus int
		checkResponse  func(t *testing.T, response map[string]interface{})
	}{
		{
			name:           "method not allowed",
			method:         http.MethodGet,
			requestBody:    nil,
			setupMock:      func() {},
			expectedStatus: http.StatusMethodNotAllowed,
			checkResponse:  nil,
		},
		{
			name:        "invalid request body",
			method:      http.MethodPost,
			requestBody: "invalid json",
			setupMock: func() {
				mockLogger.EXPECT().
					WithField("error", gomock.Any()).
					Return(mockLogger)
				mockLogger.EXPECT().
					Error("Failed to decode request body")
			},
			expectedStatus: http.StatusBadRequest,
			checkResponse:  nil,
		},
		{
			name:   "missing required fields",
			method: http.MethodPost,
			requestBody: domain.CreateTransactionalRequest{
				WorkspaceID:  workspaceID,
				Notification: domain.TransactionalNotificationCreateParams{
					// Missing required fields
				},
			},
			setupMock:      func() {},
			expectedStatus: http.StatusBadRequest,
			checkResponse:  nil,
		},
		{
			name:        "successful creation",
			method:      http.MethodPost,
			requestBody: validReqBody,
			setupMock: func() {
				notification := createTestTransactionalNotification()
				mockService.EXPECT().
					CreateNotification(gomock.Any(), workspaceID, gomock.Any()).
					Return(notification, nil)
			},
			expectedStatus: http.StatusCreated,
			checkResponse: func(t *testing.T, response map[string]interface{}) {
				notification, ok := response["notification"].(map[string]interface{})
				assert.True(t, ok)
				assert.Equal(t, "test-notification", notification["id"])
				assert.Equal(t, "Test Notification", notification["name"])
			},
		},
		{
			name:        "template validation error",
			method:      http.MethodPost,
			requestBody: validReqBody,
			setupMock: func() {
				mockService.EXPECT().
					CreateNotification(gomock.Any(), workspaceID, gomock.Any()).
					Return(nil, errors.New("invalid template: missing required field"))

				mockLogger.EXPECT().
					WithField("error", "invalid template: missing required field").
					Return(mockLogger)
				mockLogger.EXPECT().
					Error("Failed to create transactional notification")
			},
			expectedStatus: http.StatusBadRequest,
			checkResponse:  nil,
		},
		{
			name:        "service error",
			method:      http.MethodPost,
			requestBody: validReqBody,
			setupMock: func() {
				mockService.EXPECT().
					CreateNotification(gomock.Any(), workspaceID, gomock.Any()).
					Return(nil, errors.New("service error"))

				mockLogger.EXPECT().
					WithField("error", "service error").
					Return(mockLogger)
				mockLogger.EXPECT().
					Error("Failed to create transactional notification")
			},
			expectedStatus: http.StatusInternalServerError,
			checkResponse:  nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup mock expectations
			tc.setupMock()

			// Create request body
			var reqBody []byte
			var err error

			switch body := tc.requestBody.(type) {
			case string:
				reqBody = []byte(body)
			default:
				reqBody, err = json.Marshal(body)
				require.NoError(t, err)
			}

			// Create request
			req := httptest.NewRequest(tc.method, "/api/transactional.create", bytes.NewBuffer(reqBody))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			// Call handler method
			handler.handleCreate(w, req)

			// Check status code
			assert.Equal(t, tc.expectedStatus, w.Code)

			// If success case, check response content
			if tc.expectedStatus == http.StatusCreated && tc.checkResponse != nil {
				var response map[string]interface{}
				err := json.NewDecoder(w.Body).Decode(&response)
				require.NoError(t, err)
				tc.checkResponse(t, response)
			}
		})
	}
}

func TestTransactionalNotificationHandler_HandleUpdate(t *testing.T) {
	mockService, mockLogger, handler := setupTransactionalHandlerTest(t)

	workspaceID := "workspace1"
	notificationID := "test-notification"

	// Valid request body for update
	validUpdateParams := domain.TransactionalNotificationUpdateParams{
		Name:        "Updated Notification",
		Description: "Updated description",
	}

	validReqBody := domain.UpdateTransactionalRequest{
		WorkspaceID: workspaceID,
		ID:          notificationID,
		Updates:     validUpdateParams,
	}

	testCases := []struct {
		name           string
		method         string
		requestBody    interface{}
		setupMock      func()
		expectedStatus int
		checkResponse  func(t *testing.T, response map[string]interface{})
	}{
		{
			name:           "method not allowed",
			method:         http.MethodGet,
			requestBody:    nil,
			setupMock:      func() {},
			expectedStatus: http.StatusMethodNotAllowed,
			checkResponse:  nil,
		},
		{
			name:        "invalid request body",
			method:      http.MethodPost,
			requestBody: "invalid json",
			setupMock: func() {
				mockLogger.EXPECT().
					WithField(gomock.Eq("error"), gomock.Any()).
					Return(mockLogger)
				mockLogger.EXPECT().
					Error(gomock.Eq("Failed to decode request body"))
			},
			expectedStatus: http.StatusBadRequest,
			checkResponse:  nil,
		},
		{
			name:        "missing required fields",
			method:      http.MethodPost,
			requestBody: domain.UpdateTransactionalRequest{
				// Missing required fields
			},
			setupMock:      func() {},
			expectedStatus: http.StatusBadRequest,
			checkResponse:  nil,
		},
		{
			name:        "notification not found",
			method:      http.MethodPost,
			requestBody: validReqBody,
			setupMock: func() {
				mockService.EXPECT().
					UpdateNotification(gomock.Any(), workspaceID, notificationID, gomock.Any()).
					Return(nil, errors.New("notification not found"))
			},
			expectedStatus: http.StatusNotFound,
			checkResponse:  nil,
		},
		{
			name:        "successful update",
			method:      http.MethodPost,
			requestBody: validReqBody,
			setupMock: func() {
				updatedNotification := createTestTransactionalNotification()
				updatedNotification.Name = "Updated Notification"
				updatedNotification.Description = "Updated description"

				mockService.EXPECT().
					UpdateNotification(gomock.Any(), workspaceID, notificationID, gomock.Any()).
					Return(updatedNotification, nil)
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, response map[string]interface{}) {
				notification, ok := response["notification"].(map[string]interface{})
				assert.True(t, ok)
				assert.Equal(t, notificationID, notification["id"])
				assert.Equal(t, "Updated Notification", notification["name"])
				assert.Equal(t, "Updated description", notification["description"])
			},
		},
		{
			name:        "template validation error",
			method:      http.MethodPost,
			requestBody: validReqBody,
			setupMock: func() {
				mockService.EXPECT().
					UpdateNotification(gomock.Any(), workspaceID, notificationID, gomock.Any()).
					Return(nil, errors.New("invalid template: missing required field"))
			},
			expectedStatus: http.StatusBadRequest,
			checkResponse:  nil,
		},
		{
			name:        "service error",
			method:      http.MethodPost,
			requestBody: validReqBody,
			setupMock: func() {
				mockService.EXPECT().
					UpdateNotification(gomock.Any(), workspaceID, notificationID, gomock.Any()).
					Return(nil, errors.New("service error"))

				mockLogger.EXPECT().
					WithField(gomock.Eq("error"), gomock.Eq("service error")).
					Return(mockLogger)
				mockLogger.EXPECT().
					Error(gomock.Eq("Failed to update transactional notification"))
			},
			expectedStatus: http.StatusInternalServerError,
			checkResponse:  nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup mock expectations
			tc.setupMock()

			// Create request body
			var reqBody []byte
			var err error

			switch body := tc.requestBody.(type) {
			case string:
				reqBody = []byte(body)
			default:
				reqBody, err = json.Marshal(body)
				require.NoError(t, err)
			}

			// Create request
			req := httptest.NewRequest(tc.method, "/api/transactional.update", bytes.NewBuffer(reqBody))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			// Call handler method
			handler.handleUpdate(w, req)

			// Check status code
			assert.Equal(t, tc.expectedStatus, w.Code)

			// If success case, check response content
			if tc.expectedStatus == http.StatusOK && tc.checkResponse != nil {
				var response map[string]interface{}
				err := json.NewDecoder(w.Body).Decode(&response)
				require.NoError(t, err)
				tc.checkResponse(t, response)
			}
		})
	}
}

func TestTransactionalNotificationHandler_HandleDelete(t *testing.T) {
	mockService, mockLogger, handler := setupTransactionalHandlerTest(t)

	workspaceID := "workspace1"
	notificationID := "test-notification"

	validReqBody := domain.DeleteTransactionalRequest{
		WorkspaceID: workspaceID,
		ID:          notificationID,
	}

	testCases := []struct {
		name           string
		method         string
		requestBody    interface{}
		setupMock      func()
		expectedStatus int
		checkResponse  func(t *testing.T, response map[string]interface{})
	}{
		{
			name:           "method not allowed",
			method:         http.MethodGet,
			requestBody:    nil,
			setupMock:      func() {},
			expectedStatus: http.StatusMethodNotAllowed,
			checkResponse:  nil,
		},
		{
			name:        "invalid request body",
			method:      http.MethodPost,
			requestBody: "invalid json",
			setupMock: func() {
				mockLogger.EXPECT().
					WithField(gomock.Eq("error"), gomock.Any()).
					Return(mockLogger)
				mockLogger.EXPECT().
					Error(gomock.Eq("Failed to decode request body"))
			},
			expectedStatus: http.StatusBadRequest,
			checkResponse:  nil,
		},
		{
			name:        "missing required fields",
			method:      http.MethodPost,
			requestBody: domain.DeleteTransactionalRequest{
				// Missing required fields
			},
			setupMock:      func() {},
			expectedStatus: http.StatusBadRequest,
			checkResponse:  nil,
		},
		{
			name:        "notification not found",
			method:      http.MethodPost,
			requestBody: validReqBody,
			setupMock: func() {
				mockService.EXPECT().
					DeleteNotification(gomock.Any(), workspaceID, notificationID).
					Return(errors.New("notification not found"))
			},
			expectedStatus: http.StatusNotFound,
			checkResponse:  nil,
		},
		{
			name:        "successful deletion",
			method:      http.MethodPost,
			requestBody: validReqBody,
			setupMock: func() {
				mockService.EXPECT().
					DeleteNotification(gomock.Any(), workspaceID, notificationID).
					Return(nil)
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, response map[string]interface{}) {
				success, ok := response["success"].(bool)
				assert.True(t, ok)
				assert.True(t, success)
			},
		},
		{
			name:        "service error",
			method:      http.MethodPost,
			requestBody: validReqBody,
			setupMock: func() {
				mockService.EXPECT().
					DeleteNotification(gomock.Any(), workspaceID, notificationID).
					Return(errors.New("service error"))

				mockLogger.EXPECT().
					WithField(gomock.Eq("error"), gomock.Eq("service error")).
					Return(mockLogger)
				mockLogger.EXPECT().
					Error(gomock.Eq("Failed to delete transactional notification"))
			},
			expectedStatus: http.StatusInternalServerError,
			checkResponse:  nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup mock expectations
			tc.setupMock()

			// Create request body
			var reqBody []byte
			var err error

			switch body := tc.requestBody.(type) {
			case string:
				reqBody = []byte(body)
			default:
				reqBody, err = json.Marshal(body)
				require.NoError(t, err)
			}

			// Create request
			req := httptest.NewRequest(tc.method, "/api/transactional.delete", bytes.NewBuffer(reqBody))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			// Call handler method
			handler.handleDelete(w, req)

			// Check status code
			assert.Equal(t, tc.expectedStatus, w.Code)

			// If success case, check response content
			if tc.expectedStatus == http.StatusOK && tc.checkResponse != nil {
				var response map[string]interface{}
				err := json.NewDecoder(w.Body).Decode(&response)
				require.NoError(t, err)
				tc.checkResponse(t, response)
			}
		})
	}
}

func TestTransactionalNotificationHandler_HandleSend(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockService := mocks.NewMockTransactionalNotificationService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// For tests we don't need the actual key, we can create a new one
	secretKey := paseto.NewV4AsymmetricSecretKey()
	publicKey := secretKey.Public()

	handler := NewTransactionalNotificationHandler(mockService, func() (paseto.V4AsymmetricPublicKey, error) { return publicKey, nil }, mockLogger, false)

	workspaceID := "workspace1"
	notificationID := "test-notification"

	validReqBody := domain.SendTransactionalRequest{
		WorkspaceID: workspaceID,
		Notification: domain.TransactionalNotificationSendParams{
			ID: notificationID,
			Contact: &domain.Contact{
				Email: "test@example.com",
			},
			Data: domain.MapOfAny{
				"name": "Test User",
			},
			Channels: []domain.TransactionalChannel{domain.TransactionalChannelEmail}, // Required field
		},
	}

	testCases := []struct {
		name           string
		method         string
		requestBody    interface{}
		setupMock      func()
		expectedStatus int
		checkResponse  func(t *testing.T, response map[string]interface{})
	}{
		{
			name:           "method not allowed",
			method:         http.MethodGet,
			requestBody:    nil,
			setupMock:      func() {},
			expectedStatus: http.StatusMethodNotAllowed,
			checkResponse:  nil,
		},
		{
			name:        "invalid request body",
			method:      http.MethodPost,
			requestBody: "invalid json",
			setupMock: func() {
				// Set expectation for logger mock
				mockLogger.EXPECT().
					WithField(gomock.Eq("error"), gomock.Any()).
					Return(mockLogger)
				mockLogger.EXPECT().
					Error(gomock.Eq("Failed to decode request body"))
			},
			expectedStatus: http.StatusBadRequest,
			checkResponse:  nil,
		},
		{
			name:   "missing required fields",
			method: http.MethodPost,
			requestBody: domain.SendTransactionalRequest{
				WorkspaceID:  workspaceID,
				Notification: domain.TransactionalNotificationSendParams{
					// Missing ID and Contact
				},
			},
			setupMock:      func() {},
			expectedStatus: http.StatusBadRequest,
			checkResponse:  nil,
		},
		{
			name:   "invalid contact",
			method: http.MethodPost,
			requestBody: domain.SendTransactionalRequest{
				WorkspaceID: workspaceID,
				Notification: domain.TransactionalNotificationSendParams{
					ID:       notificationID,
					Contact:  &domain.Contact{}, // Empty contact
					Channels: []domain.TransactionalChannel{domain.TransactionalChannelEmail},
				},
			},
			setupMock:      func() {},
			expectedStatus: http.StatusBadRequest,
			checkResponse:  nil,
		},
		{
			name:        "notification not found",
			method:      http.MethodPost,
			requestBody: validReqBody,
			setupMock: func() {
				mockService.EXPECT().
					SendNotification(gomock.Any(), gomock.Eq(workspaceID), gomock.Any()).
					Return("", errors.New("notification not found"))

				mockLogger.EXPECT().
					WithField(gomock.Eq("error"), gomock.Any()).
					Return(mockLogger)
				mockLogger.EXPECT().
					Error(gomock.Eq("Failed to send transactional notification"))
			},
			expectedStatus: http.StatusBadRequest,
			checkResponse:  nil,
		},
		{
			name:        "notification inactive",
			method:      http.MethodPost,
			requestBody: validReqBody,
			setupMock: func() {
				mockService.EXPECT().
					SendNotification(gomock.Any(), gomock.Eq(workspaceID), gomock.Any()).
					Return("", errors.New("notification not active"))

				mockLogger.EXPECT().
					WithField(gomock.Eq("error"), gomock.Any()).
					Return(mockLogger)
				mockLogger.EXPECT().
					Error(gomock.Eq("Failed to send transactional notification"))
			},
			expectedStatus: http.StatusBadRequest,
			checkResponse:  nil,
		},
		{
			name:        "no valid channels",
			method:      http.MethodPost,
			requestBody: validReqBody,
			setupMock: func() {
				mockService.EXPECT().
					SendNotification(gomock.Any(), gomock.Eq(workspaceID), gomock.Any()).
					Return("", errors.New("no valid channels"))

				mockLogger.EXPECT().
					WithField(gomock.Eq("error"), gomock.Any()).
					Return(mockLogger)
				mockLogger.EXPECT().
					Error(gomock.Eq("Failed to send transactional notification"))
			},
			expectedStatus: http.StatusBadRequest,
			checkResponse:  nil,
		},
		{
			name:        "successful send",
			method:      http.MethodPost,
			requestBody: validReqBody,
			setupMock: func() {
				mockService.EXPECT().
					SendNotification(gomock.Any(), gomock.Eq(workspaceID), gomock.Any()).
					Return("msg_123", nil)
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, response map[string]interface{}) {
				success, ok := response["success"].(bool)
				assert.True(t, ok)
				assert.True(t, success)

				messageID, ok := response["message_id"].(string)
				assert.True(t, ok)
				assert.Equal(t, "msg_123", messageID)
			},
		},
		{
			name:   "with cc and bcc valid emails",
			method: http.MethodPost,
			requestBody: domain.SendTransactionalRequest{
				WorkspaceID: workspaceID,
				Notification: domain.TransactionalNotificationSendParams{
					ID: notificationID,
					Contact: &domain.Contact{
						Email: "test@example.com",
					},
					Data: domain.MapOfAny{
						"name": "Test User",
					},
					Channels: []domain.TransactionalChannel{domain.TransactionalChannelEmail},
					EmailOptions: domain.EmailOptions{
						CC:  []string{"cc1@example.com", "cc2@example.com"},
						BCC: []string{"bcc@example.com"},
					},
				},
			},
			setupMock: func() {
				mockService.EXPECT().
					SendNotification(gomock.Any(), gomock.Eq(workspaceID), gomock.Any()).
					DoAndReturn(func(ctx context.Context, wsID string, params domain.TransactionalNotificationSendParams) (string, error) {
						// Verify cc and bcc are passed correctly
						assert.Equal(t, []string{"cc1@example.com", "cc2@example.com"}, params.EmailOptions.CC)
						assert.Equal(t, []string{"bcc@example.com"}, params.EmailOptions.BCC)
						assert.Contains(t, params.Channels, domain.TransactionalChannelEmail)
						return "msg_with_cc_bcc", nil
					})
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, response map[string]interface{}) {
				success, ok := response["success"].(bool)
				assert.True(t, ok)
				assert.True(t, success)

				messageID, ok := response["message_id"].(string)
				assert.True(t, ok)
				assert.Equal(t, "msg_with_cc_bcc", messageID)
			},
		},
		{
			name:   "with invalid cc email",
			method: http.MethodPost,
			requestBody: domain.SendTransactionalRequest{
				WorkspaceID: workspaceID,
				Notification: domain.TransactionalNotificationSendParams{
					ID: notificationID,
					Contact: &domain.Contact{
						Email: "test@example.com",
					},
					Channels: []domain.TransactionalChannel{domain.TransactionalChannelEmail},
					EmailOptions: domain.EmailOptions{
						CC: []string{"invalid-email"},
					},
				},
			},
			setupMock:      func() {},
			expectedStatus: http.StatusBadRequest,
			checkResponse:  nil,
		},
		{
			name:   "with invalid bcc email",
			method: http.MethodPost,
			requestBody: domain.SendTransactionalRequest{
				WorkspaceID: workspaceID,
				Notification: domain.TransactionalNotificationSendParams{
					ID: notificationID,
					Contact: &domain.Contact{
						Email: "test@example.com",
					},
					Channels: []domain.TransactionalChannel{domain.TransactionalChannelEmail},
					EmailOptions: domain.EmailOptions{
						BCC: []string{"invalid-email"},
					},
				},
			},
			setupMock:      func() {},
			expectedStatus: http.StatusBadRequest,
			checkResponse:  nil,
		},
		{
			name:        "service error",
			method:      http.MethodPost,
			requestBody: validReqBody,
			setupMock: func() {
				mockService.EXPECT().
					SendNotification(gomock.Any(), gomock.Eq(workspaceID), gomock.Any()).
					Return("", errors.New("service error"))

				mockLogger.EXPECT().
					WithField(gomock.Eq("error"), gomock.Any()).
					Return(mockLogger)
				mockLogger.EXPECT().
					Error(gomock.Eq("Failed to send transactional notification"))
			},
			expectedStatus: http.StatusInternalServerError,
			checkResponse:  nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup mock expectations
			tc.setupMock()

			// Create request body
			var reqBody []byte
			var err error

			switch body := tc.requestBody.(type) {
			case string:
				reqBody = []byte(body)
			default:
				reqBody, err = json.Marshal(body)
				require.NoError(t, err)
			}

			// Create request
			req := httptest.NewRequest(tc.method, "/api/transactional.send", bytes.NewBuffer(reqBody))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			// Call handler method
			handler.handleSend(w, req)

			// Check status code
			assert.Equal(t, tc.expectedStatus, w.Code)

			// If success case, check response content
			if tc.expectedStatus == http.StatusOK && tc.checkResponse != nil {
				var response map[string]interface{}
				err := json.NewDecoder(w.Body).Decode(&response)
				require.NoError(t, err)
				tc.checkResponse(t, response)
			}
		})
	}
}

func TestTransactionalNotificationHandler_HandleTestTemplate(t *testing.T) {
	// Create a mock controller for the entire test function
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create the mock logger once for the entire test function
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	tests := []struct {
		name           string
		method         string
		reqBody        interface{}
		setupMock      func(*mocks.MockTransactionalNotificationService, *pkgmocks.MockLogger)
		expectedStatus int
		expectedResp   *domain.TestTemplateResponse
	}{
		{
			name:           "Method not allowed",
			method:         http.MethodGet,
			reqBody:        nil,
			setupMock:      func(m *mocks.MockTransactionalNotificationService, l *pkgmocks.MockLogger) {},
			expectedStatus: http.StatusMethodNotAllowed,
			expectedResp:   nil,
		},
		{
			name:    "Invalid request body",
			method:  http.MethodPost,
			reqBody: "invalid json",
			setupMock: func(m *mocks.MockTransactionalNotificationService, l *pkgmocks.MockLogger) {
				l.EXPECT().
					WithField("error", "invalid character 'i' looking for beginning of value").
					Return(l)
				l.EXPECT().
					Error("Failed to decode request body")
			},
			expectedStatus: http.StatusBadRequest,
			expectedResp:   nil,
		},
		{
			name:   "Missing recipient email",
			method: http.MethodPost,
			reqBody: domain.TestTemplateRequest{
				WorkspaceID:   "workspace123",
				TemplateID:    "template123",
				IntegrationID: "marketing",
				// Missing RecipientEmail field
			},
			setupMock:      func(m *mocks.MockTransactionalNotificationService, l *pkgmocks.MockLogger) {},
			expectedStatus: http.StatusBadRequest,
			expectedResp:   nil,
		},
		{
			name:   "Missing workspace ID",
			method: http.MethodPost,
			reqBody: domain.TestTemplateRequest{
				TemplateID:     "template123",
				IntegrationID:  "marketing",
				RecipientEmail: "test@example.com",
				// Missing WorkspaceID field
			},
			setupMock:      func(m *mocks.MockTransactionalNotificationService, l *pkgmocks.MockLogger) {},
			expectedStatus: http.StatusBadRequest,
			expectedResp:   nil,
		},
		{
			name:   "Invalid provider type",
			method: http.MethodPost,
			reqBody: domain.TestTemplateRequest{
				WorkspaceID:    "workspace123",
				TemplateID:     "template123",
				IntegrationID:  "invalid",
				SenderID:       "sender123",
				RecipientEmail: "test@example.com",
			},
			setupMock: func(m *mocks.MockTransactionalNotificationService, l *pkgmocks.MockLogger) {
				m.EXPECT().
					TestTemplate(
						gomock.Any(),
						"workspace123",
						"template123",
						"invalid",
						"sender123",
						"test@example.com",
						domain.EmailOptions{},
					).
					Return(fmt.Errorf("integration not found: invalid"))

				l.EXPECT().
					WithFields(map[string]interface{}{
						"error":        "integration not found: invalid",
						"workspace_id": "workspace123",
						"template_id":  "template123",
					}).
					Return(l)
				l.EXPECT().
					Error("Failed to test template")
			},
			expectedStatus: http.StatusOK,
			expectedResp: &domain.TestTemplateResponse{
				Success: false,
				Error:   "integration not found: invalid",
			},
		},
		{
			name:   "Service error",
			method: http.MethodPost,
			reqBody: domain.TestTemplateRequest{
				WorkspaceID:    "workspace123",
				TemplateID:     "template123",
				IntegrationID:  "marketing",
				SenderID:       "sender123",
				RecipientEmail: "test@example.com",
			},
			setupMock: func(m *mocks.MockTransactionalNotificationService, l *pkgmocks.MockLogger) {
				m.EXPECT().
					TestTemplate(
						gomock.Any(),
						"workspace123",
						"template123",
						"marketing",
						"sender123",
						"test@example.com",
						domain.EmailOptions{},
					).
					Return(errors.New("service error"))

				l.EXPECT().
					WithFields(map[string]interface{}{
						"error":        "service error",
						"workspace_id": "workspace123",
						"template_id":  "template123",
					}).
					Return(l)
				l.EXPECT().
					Error("Failed to test template")
			},
			expectedStatus: http.StatusOK,
			expectedResp: &domain.TestTemplateResponse{
				Success: false,
				Error:   "service error",
			},
		},
		{
			name:   "Template not found",
			method: http.MethodPost,
			reqBody: domain.TestTemplateRequest{
				WorkspaceID:    "workspace123",
				TemplateID:     "template123",
				IntegrationID:  "marketing",
				SenderID:       "sender123",
				RecipientEmail: "test@example.com",
			},
			setupMock: func(m *mocks.MockTransactionalNotificationService, l *pkgmocks.MockLogger) {
				m.EXPECT().
					TestTemplate(
						gomock.Any(),
						"workspace123",
						"template123",
						"marketing",
						"sender123",
						"test@example.com",
						domain.EmailOptions{},
					).
					Return(&domain.ErrTemplateNotFound{Message: "not found"})
			},
			expectedStatus: http.StatusNotFound,
			expectedResp:   nil,
		},
		{
			name:   "Success with CC and BCC",
			method: http.MethodPost,
			reqBody: domain.TestTemplateRequest{
				WorkspaceID:    "workspace123",
				TemplateID:     "template123",
				IntegrationID:  "marketing",
				SenderID:       "sender123",
				RecipientEmail: "test@example.com",
				EmailOptions: domain.EmailOptions{
					CC:  []string{"cc1@example.com", "cc2@example.com"},
					BCC: []string{"bcc@example.com"},
				},
			},
			setupMock: func(m *mocks.MockTransactionalNotificationService, l *pkgmocks.MockLogger) {
				m.EXPECT().
					TestTemplate(
						gomock.Any(),
						"workspace123",
						"template123",
						"marketing",
						"sender123",
						"test@example.com",
						domain.EmailOptions{
							CC:  []string{"cc1@example.com", "cc2@example.com"},
							BCC: []string{"bcc@example.com"},
						},
					).
					Return(nil)
			},
			expectedStatus: http.StatusOK,
			expectedResp: &domain.TestTemplateResponse{
				Success: true,
			},
		},
		{
			name:   "Success",
			method: http.MethodPost,
			reqBody: domain.TestTemplateRequest{
				WorkspaceID:    "workspace123",
				TemplateID:     "template123",
				IntegrationID:  "marketing",
				SenderID:       "sender123",
				RecipientEmail: "test@example.com",
			},
			setupMock: func(m *mocks.MockTransactionalNotificationService, l *pkgmocks.MockLogger) {
				m.EXPECT().
					TestTemplate(
						gomock.Any(),
						"workspace123",
						"template123",
						"marketing",
						"sender123",
						"test@example.com",
						domain.EmailOptions{},
					).
					Return(nil)
			},
			expectedStatus: http.StatusOK,
			expectedResp: &domain.TestTemplateResponse{
				Success: true,
			},
		},
		{
			name:   "Success with ReplyTo",
			method: http.MethodPost,
			reqBody: domain.TestTemplateRequest{
				WorkspaceID:    "workspace123",
				TemplateID:     "template123",
				IntegrationID:  "marketing",
				SenderID:       "sender123",
				RecipientEmail: "test@example.com",
				EmailOptions: domain.EmailOptions{
					ReplyTo: "custom-reply@example.com",
				},
			},
			setupMock: func(m *mocks.MockTransactionalNotificationService, l *pkgmocks.MockLogger) {
				m.EXPECT().
					TestTemplate(
						gomock.Any(),
						"workspace123",
						"template123",
						"marketing",
						"sender123",
						"test@example.com",
						domain.EmailOptions{
							ReplyTo: "custom-reply@example.com",
						},
					).
					Return(nil)
			},
			expectedStatus: http.StatusOK,
			expectedResp: &domain.TestTemplateResponse{
				Success: true,
			},
		},
		{
			name:   "Invalid ReplyTo format",
			method: http.MethodPost,
			reqBody: domain.TestTemplateRequest{
				WorkspaceID:    "workspace123",
				TemplateID:     "template123",
				IntegrationID:  "marketing",
				RecipientEmail: "test@example.com",
				EmailOptions: domain.EmailOptions{
					ReplyTo: "invalid-email", // Invalid email format
				},
			},
			setupMock:      func(m *mocks.MockTransactionalNotificationService, l *pkgmocks.MockLogger) {},
			expectedStatus: http.StatusBadRequest,
			expectedResp:   nil,
		},
		{
			name:   "Success with all parameters",
			method: http.MethodPost,
			reqBody: domain.TestTemplateRequest{
				WorkspaceID:    "workspace123",
				TemplateID:     "template123",
				IntegrationID:  "marketing",
				SenderID:       "sender123",
				RecipientEmail: "test@example.com",
				EmailOptions: domain.EmailOptions{
					CC:      []string{"cc@example.com"},
					BCC:     []string{"bcc@example.com"},
					ReplyTo: "reply@example.com",
				},
			},
			setupMock: func(m *mocks.MockTransactionalNotificationService, l *pkgmocks.MockLogger) {
				m.EXPECT().
					TestTemplate(
						gomock.Any(),
						"workspace123",
						"template123",
						"marketing",
						"sender123",
						"test@example.com",
						domain.EmailOptions{
							CC:      []string{"cc@example.com"},
							BCC:     []string{"bcc@example.com"},
							ReplyTo: "reply@example.com",
						},
					).
					Return(nil)
			},
			expectedStatus: http.StatusOK,
			expectedResp: &domain.TestTemplateResponse{
				Success: true,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			mockService := mocks.NewMockTransactionalNotificationService(ctrl)
			tc.setupMock(mockService, mockLogger)

			// Create a paseto secret key
			secretKey := paseto.NewV4AsymmetricSecretKey()
			publicKey := secretKey.Public()

			// Create the handler
			handler := &TransactionalNotificationHandler{
				service:      mockService,
				logger:       mockLogger,
				getPublicKey: func() (paseto.V4AsymmetricPublicKey, error) { return publicKey, nil },
			}

			// Create request
			var reqBody []byte
			var err error

			if tc.reqBody != nil {
				if strBody, ok := tc.reqBody.(string); ok {
					reqBody = []byte(strBody)
				} else {
					reqBody, err = json.Marshal(tc.reqBody)
					require.NoError(t, err)
				}
			}

			req := httptest.NewRequest(tc.method, "/api/email.testTemplate", bytes.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")

			// Create a response recorder
			w := httptest.NewRecorder()

			// Act - call the handler directly instead of through the mux
			handler.handleTestTemplate(w, req)

			// Assert
			assert.Equal(t, tc.expectedStatus, w.Code)

			if tc.expectedResp != nil {
				var response domain.TestTemplateResponse
				err = json.NewDecoder(w.Body).Decode(&response)
				require.NoError(t, err)
				assert.Equal(t, tc.expectedResp.Success, response.Success)
				if tc.expectedResp.Error != "" {
					assert.Equal(t, tc.expectedResp.Error, response.Error)
				}
			}
		})
	}
}
