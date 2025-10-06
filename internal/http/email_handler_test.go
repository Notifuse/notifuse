package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"aidanwoods.dev/go-paseto"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupEmailHandlerTest prepares a test environment for email handler tests
func setupEmailHandlerTest(t *testing.T) (*mocks.MockEmailServiceInterface, *pkgmocks.MockLogger, *EmailHandler, paseto.V4AsymmetricSecretKey) {
	ctrl := gomock.NewController(t)
	t.Cleanup(func() { ctrl.Finish() })

	mockService := mocks.NewMockEmailServiceInterface(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Setup common logger expectations
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Fatal(gomock.Any()).AnyTimes()

	// Create key pair for testing
	secretKey := paseto.NewV4AsymmetricSecretKey()
	publicKey := secretKey.Public()

	handler := NewEmailHandler(mockService, publicKey, mockLogger, "test-secret-key")

	return mockService, mockLogger, handler, secretKey
}

func TestNewEmailHandler(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockService := mocks.NewMockEmailServiceInterface(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	secretKey := paseto.NewV4AsymmetricSecretKey()
	publicKey := secretKey.Public()

	// Act
	handler := NewEmailHandler(mockService, publicKey, mockLogger, "test-secret-key")

	// Assert
	assert.NotNil(t, handler)
	assert.Equal(t, mockService, handler.emailService)
	assert.Equal(t, publicKey, handler.publicKey)
	assert.Equal(t, mockLogger, handler.logger)
	assert.Equal(t, "test-secret-key", handler.secretKey)
}

func TestEmailHandler_RegisterRoutes(t *testing.T) {
	// Arrange
	_, _, handler, _ := setupEmailHandlerTest(t)

	// Create a multiplexer to register routes with
	mux := http.NewServeMux()

	// Register routes with the mux
	handler.RegisterRoutes(mux)

	// Create a test server with the mux
	server := httptest.NewServer(mux)
	defer server.Close()

	// Create an authenticated request to verify the route exists
	reqBody := bytes.NewReader([]byte("{}"))
	req, err := http.NewRequest(http.MethodPost, server.URL+"/api/email.testProvider", reqBody)
	require.NoError(t, err)

	// Set content type
	req.Header.Set("Content-Type", "application/json")

	// Act
	client := &http.Client{}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Assert
	// We expect a 401 Unauthorized since we didn't provide authentication
	// The important part is that the route exists and returns a response
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestEmailHandler_HandleTestEmailProvider(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		reqBody        interface{}
		setupMock      func(*mocks.MockEmailServiceInterface)
		expectedStatus int
		expectedResp   *domain.TestEmailProviderResponse
	}{
		{
			name:           "Method not allowed",
			method:         http.MethodGet,
			reqBody:        nil,
			setupMock:      func(m *mocks.MockEmailServiceInterface) {},
			expectedStatus: http.StatusMethodNotAllowed,
			expectedResp:   nil,
		},
		{
			name:           "Invalid request body",
			method:         http.MethodPost,
			reqBody:        "invalid json",
			setupMock:      func(m *mocks.MockEmailServiceInterface) {},
			expectedStatus: http.StatusBadRequest,
			expectedResp:   nil,
		},
		{
			name:   "Missing recipient email",
			method: http.MethodPost,
			reqBody: domain.TestEmailProviderRequest{
				WorkspaceID: "workspace123",
				Provider: domain.EmailProvider{
					Kind: domain.EmailProviderKindSMTP,
				},
				// Missing To field
			},
			setupMock:      func(m *mocks.MockEmailServiceInterface) {},
			expectedStatus: http.StatusBadRequest,
			expectedResp:   nil,
		},
		{
			name:   "Missing workspace ID",
			method: http.MethodPost,
			reqBody: domain.TestEmailProviderRequest{
				To: "test@example.com",
				Provider: domain.EmailProvider{
					Kind: domain.EmailProviderKindSMTP,
				},
				// Missing WorkspaceID field
			},
			setupMock:      func(m *mocks.MockEmailServiceInterface) {},
			expectedStatus: http.StatusBadRequest,
			expectedResp:   nil,
		},
		{
			name:   "Service error",
			method: http.MethodPost,
			reqBody: domain.TestEmailProviderRequest{
				WorkspaceID: "workspace123",
				To:          "test@example.com",
				Provider: domain.EmailProvider{
					Kind: domain.EmailProviderKindSMTP,
					Senders: []domain.EmailSender{
						domain.NewEmailSender("sender@example.com", "Test Sender"),
					},
					SMTP: &domain.SMTPSettings{
						Host:     "smtp.example.com",
						Port:     587,
						Username: "user@example.com",
					},
				},
			},
			setupMock: func(m *mocks.MockEmailServiceInterface) {
				m.EXPECT().
					TestEmailProvider(
						gomock.Any(),
						"workspace123",
						gomock.Any(),
						"test@example.com",
					).
					Return(errors.New("service error"))
			},
			expectedStatus: http.StatusOK,
			expectedResp: &domain.TestEmailProviderResponse{
				Success: false,
				Error:   "service error",
			},
		},
		{
			name:   "Success",
			method: http.MethodPost,
			reqBody: domain.TestEmailProviderRequest{
				WorkspaceID: "workspace123",
				To:          "test@example.com",
				Provider: domain.EmailProvider{
					Kind: domain.EmailProviderKindSMTP,
					Senders: []domain.EmailSender{
						domain.NewEmailSender("sender@example.com", "Test Sender"),
					},
					SMTP: &domain.SMTPSettings{
						Host:     "smtp.example.com",
						Port:     587,
						Username: "user@example.com",
					},
				},
			},
			setupMock: func(m *mocks.MockEmailServiceInterface) {
				m.EXPECT().
					TestEmailProvider(
						gomock.Any(),
						"workspace123",
						gomock.Any(),
						"test@example.com",
					).
					Return(nil)
			},
			expectedStatus: http.StatusOK,
			expectedResp: &domain.TestEmailProviderResponse{
				Success: true,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			mockService, _, handler, _ := setupEmailHandlerTest(t)
			tc.setupMock(mockService)

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

			req := httptest.NewRequest(tc.method, "/api/email.testProvider", bytes.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")

			// Create a response recorder
			w := httptest.NewRecorder()

			// Act - call the handler directly instead of through the mux
			handler.handleTestEmailProvider(w, req)

			// Assert
			assert.Equal(t, tc.expectedStatus, w.Code)

			if tc.expectedResp != nil {
				var response domain.TestEmailProviderResponse
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

func TestEmailHandler_HandleClickRedirection(t *testing.T) {
	// Setup
	mockEmailService, _, handler, _ := setupEmailHandlerTest(t)

	tests := []struct {
		name               string
		queryParams        map[string]string
		setupExpectations  func()
		expectedStatusCode int
		expectedRedirectTo string
		expectedBody       string
	}{
		{
			name: "Success with all parameters",
			queryParams: map[string]string{
				"mid": "message-123",
				"wid": "workspace-123",
				"url": "https://example.com",
			},
			setupExpectations: func() {
				mockEmailService.EXPECT().
					VisitLink(gomock.Any(), "message-123", "workspace-123").
					Return(nil)
			},
			expectedStatusCode: http.StatusSeeOther,
			expectedRedirectTo: "https://example.com",
		},
		{
			name: "Missing message ID or workspace ID",
			queryParams: map[string]string{
				"url": "https://example.com",
			},
			setupExpectations:  func() {},
			expectedStatusCode: http.StatusSeeOther,
			expectedRedirectTo: "https://example.com",
		},
		{
			name:               "Missing URL parameter",
			queryParams:        map[string]string{},
			setupExpectations:  func() {},
			expectedStatusCode: http.StatusBadRequest,
			expectedBody:       "Missing redirect URL\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup request
			req := httptest.NewRequest(http.MethodGet, "/visit", nil)
			q := req.URL.Query()
			for key, value := range tt.queryParams {
				q.Add(key, value)
			}
			req.URL.RawQuery = q.Encode()

			// Setup expectations
			tt.setupExpectations()

			// Create response recorder
			w := httptest.NewRecorder()

			// Call the handler
			handler.handleClickRedirection(w, req.WithContext(context.Background()))

			// Assertions
			assert.Equal(t, tt.expectedStatusCode, w.Code)

			if tt.expectedRedirectTo != "" {
				location := w.Header().Get("Location")
				assert.Equal(t, tt.expectedRedirectTo, location)
			}

			if tt.expectedBody != "" {
				assert.Equal(t, tt.expectedBody, w.Body.String())
			}
		})
	}
}

func TestEmailHandler_HandleOpens(t *testing.T) {
	// Setup
	mockEmailService, _, handler, _ := setupEmailHandlerTest(t)

	tests := []struct {
		name                string
		queryParams         map[string]string
		setupExpectations   func()
		expectedStatusCode  int
		expectedBody        string
		expectedContentType string
	}{
		{
			name: "Success with all parameters",
			queryParams: map[string]string{
				"mid": "message-123",
				"wid": "workspace-123",
			},
			setupExpectations: func() {
				mockEmailService.EXPECT().
					OpenEmail(gomock.Any(), "message-123", "workspace-123").
					Return(nil)
			},
			expectedStatusCode:  http.StatusOK,
			expectedContentType: "image/png",
		},
		{
			name: "Missing message ID",
			queryParams: map[string]string{
				"wid": "workspace-123",
			},
			setupExpectations:  func() {},
			expectedStatusCode: http.StatusBadRequest,
			expectedBody:       "Missing message ID or workspace ID\n",
		},
		{
			name: "Missing workspace ID",
			queryParams: map[string]string{
				"mid": "message-123",
			},
			setupExpectations:  func() {},
			expectedStatusCode: http.StatusBadRequest,
			expectedBody:       "Missing message ID or workspace ID\n",
		},
		{
			name:               "Missing both IDs",
			queryParams:        map[string]string{},
			setupExpectations:  func() {},
			expectedStatusCode: http.StatusBadRequest,
			expectedBody:       "Missing message ID or workspace ID\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup request
			req := httptest.NewRequest(http.MethodGet, "/opens", nil)
			q := req.URL.Query()
			for key, value := range tt.queryParams {
				q.Add(key, value)
			}
			req.URL.RawQuery = q.Encode()

			// Setup expectations
			tt.setupExpectations()

			// Create response recorder
			w := httptest.NewRecorder()

			// Call the handler
			handler.handleOpens(w, req.WithContext(context.Background()))

			// Assertions
			assert.Equal(t, tt.expectedStatusCode, w.Code)

			if tt.expectedBody != "" {
				assert.Equal(t, tt.expectedBody, w.Body.String())
			}

			if tt.expectedContentType != "" {
				assert.Equal(t, tt.expectedContentType, w.Header().Get("Content-Type"))
			}

			// If it's a successful response, verify it returned a PNG image
			// The transparent pixel is 67 bytes long
			if tt.expectedStatusCode == http.StatusOK {
				assert.Equal(t, 67, len(w.Body.Bytes()))
				// Verify PNG signature in the first 8 bytes
				assert.Equal(t, []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, w.Body.Bytes()[:8])
			}
		})
	}
}
