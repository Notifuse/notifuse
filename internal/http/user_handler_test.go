package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"aidanwoods.dev/go-paseto"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
)

func setupUserHandlerTest(t *testing.T) (*UserHandler, *mocks.MockUserServiceInterface, *mocks.MockWorkspaceServiceInterface, paseto.V4AsymmetricSecretKey) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockUserSvc := mocks.NewMockUserServiceInterface(ctrl)
	mockWorkspaceSvc := mocks.NewMockWorkspaceServiceInterface(ctrl)
	cfg := &config.Config{}

	// Create key pair for testing
	secretKey := paseto.NewV4AsymmetricSecretKey()
	publicKey := secretKey.Public()

	mockLogger := &pkgmocks.MockLogger{}
	handler := NewUserHandler(mockUserSvc, mockWorkspaceSvc, cfg, publicKey, mockLogger)

	return handler, mockUserSvc, mockWorkspaceSvc, secretKey
}

func TestUserHandler_SignIn(t *testing.T) {
	_, mockUserSvc, mockWorkspaceSvc, secretKey := setupUserHandlerTest(t)

	// Test with different configs
	devConfig := &config.Config{Environment: "development"}
	prodConfig := &config.Config{Environment: "production"}

	// Create handlers with different configs
	devHandler := NewUserHandler(mockUserSvc, mockWorkspaceSvc, devConfig, secretKey.Public(), &pkgmocks.MockLogger{})
	prodHandler := NewUserHandler(mockUserSvc, mockWorkspaceSvc, prodConfig, secretKey.Public(), &pkgmocks.MockLogger{})

	tests := []struct {
		name         string
		handler      *UserHandler
		input        domain.SignInInput
		setupMock    func()
		expectedCode int
		expectedBody map[string]string
	}{
		{
			name:    "successful sign in production",
			handler: prodHandler,
			input: domain.SignInInput{
				Email: "test@example.com",
			},
			setupMock: func() {
				mockUserSvc.EXPECT().
					SignIn(gomock.Any(), domain.SignInInput{
						Email: "test@example.com",
					}).Return("", nil)
			},
			expectedCode: http.StatusOK,
			expectedBody: map[string]string{
				"message": "Magic code sent to your email",
			},
		},
		{
			name:    "successful sign in development",
			handler: devHandler,
			input: domain.SignInInput{
				Email: "test@example.com",
			},
			setupMock: func() {
				mockUserSvc.EXPECT().
					SignIn(gomock.Any(), domain.SignInInput{
						Email: "test@example.com",
					}).Return("123456", nil)
			},
			expectedCode: http.StatusOK,
			expectedBody: map[string]string{
				"message": "Magic code sent to your email",
				"code":    "123456",
			},
		},
		{
			name:    "invalid email production",
			handler: prodHandler,
			input: domain.SignInInput{
				Email: "",
			},
			setupMock: func() {
				mockUserSvc.EXPECT().
					SignIn(gomock.Any(), domain.SignInInput{
						Email: "",
					}).Return("", fmt.Errorf("invalid email"))
			},
			expectedCode: http.StatusInternalServerError,
			expectedBody: map[string]string{
				"error": "invalid email",
			},
		},
		{
			name:    "invalid email development",
			handler: devHandler,
			input: domain.SignInInput{
				Email: "",
			},
			setupMock: func() {
				mockUserSvc.EXPECT().
					SignIn(gomock.Any(), domain.SignInInput{
						Email: "",
					}).Return("", fmt.Errorf("invalid email"))
			},
			expectedCode: http.StatusInternalServerError,
			expectedBody: map[string]string{
				"error": "invalid email",
			},
		},
		{
			name:    "user does not exist",
			handler: prodHandler,
			input: domain.SignInInput{
				Email: "nonexistent@example.com",
			},
			setupMock: func() {
				mockUserSvc.EXPECT().
					SignIn(gomock.Any(), domain.SignInInput{
						Email: "nonexistent@example.com",
					}).Return("", &domain.ErrUserNotFound{Message: "user does not exist"})
			},
			expectedCode: http.StatusBadRequest,
			expectedBody: map[string]string{
				"error": "user does not exist",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			body, err := json.Marshal(tt.input)
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPost, "/api/user.signin", bytes.NewReader(body))
			rec := httptest.NewRecorder()

			tt.handler.SignIn(rec, req)

			assert.Equal(t, tt.expectedCode, rec.Code)

			var response map[string]string
			err = json.NewDecoder(rec.Body).Decode(&response)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedBody, response)
		})
	}
}

func TestUserHandler_VerifyCode(t *testing.T) {
	handler, mockUserSvc, _, _ := setupUserHandlerTest(t)

	user := &domain.User{
		ID:    uuid.New().String(),
		Email: "test@example.com",
		Name:  "Test User",
	}

	tests := []struct {
		name          string
		input         domain.VerifyCodeInput
		setupMock     func()
		expectedCode  int
		checkResponse func(t *testing.T, response map[string]interface{})
	}{
		{
			name: "successful verification",
			input: domain.VerifyCodeInput{
				Email: "test@example.com",
				Code:  "123456",
			},
			setupMock: func() {
				mockUserSvc.EXPECT().
					VerifyCode(gomock.Any(), domain.VerifyCodeInput{
						Email: "test@example.com",
						Code:  "123456",
					}).Return(&domain.AuthResponse{
					Token:     "auth-token",
					User:      *user,
					ExpiresAt: time.Now().Add(24 * time.Hour),
				}, nil)
			},
			expectedCode: http.StatusOK,
			checkResponse: func(t *testing.T, response map[string]interface{}) {
				assert.Equal(t, "auth-token", response["token"])
				assert.Equal(t, user.ID, response["user"].(map[string]interface{})["id"])
				assert.Equal(t, user.Email, response["user"].(map[string]interface{})["email"])
				assert.Equal(t, user.Name, response["user"].(map[string]interface{})["name"])
				assert.NotEmpty(t, response["expires_at"])
			},
		},
		{
			name: "invalid code",
			input: domain.VerifyCodeInput{
				Email: "test@example.com",
				Code:  "000000",
			},
			setupMock: func() {
				mockUserSvc.EXPECT().
					VerifyCode(gomock.Any(), domain.VerifyCodeInput{
						Email: "test@example.com",
						Code:  "000000",
					}).Return(nil, fmt.Errorf("invalid or expired code"))
			},
			expectedCode: http.StatusUnauthorized,
			checkResponse: func(t *testing.T, response map[string]interface{}) {
				assert.Equal(t, "invalid or expired code", response["error"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			body, err := json.Marshal(tt.input)
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPost, "/api/user.verify", bytes.NewReader(body))
			rec := httptest.NewRecorder()

			handler.VerifyCode(rec, req)

			assert.Equal(t, tt.expectedCode, rec.Code)

			var response map[string]interface{}
			err = json.NewDecoder(rec.Body).Decode(&response)
			require.NoError(t, err)
			tt.checkResponse(t, response)
		})
	}
}

func TestUserHandler_GetCurrentUser(t *testing.T) {
	handler, mockUserSvc, mockWorkspaceSvc, _ := setupUserHandlerTest(t)

	// Test successful case
	userID := "test-user"
	user := &domain.User{
		ID:    userID,
		Email: "test@example.com",
		Name:  "Test User",
	}
	workspaces := []*domain.Workspace{
		{
			ID:   "workspace1",
			Name: "Workspace 1",
		},
		{
			ID:   "workspace2",
			Name: "Workspace 2",
		},
	}

	mockUserSvc.EXPECT().
		GetUserByID(gomock.Any(), userID).
		Return(user, nil)
	mockWorkspaceSvc.EXPECT().
		ListWorkspaces(gomock.Any()).
		Return(workspaces, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/user.me", nil)
	req = req.WithContext(context.WithValue(req.Context(), domain.UserIDKey, userID))
	rec := httptest.NewRecorder()

	handler.GetCurrentUser(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	var response map[string]interface{}
	err := json.NewDecoder(rec.Body).Decode(&response)
	require.NoError(t, err)

	userData := response["user"].(map[string]interface{})
	assert.Equal(t, user.Email, userData["email"])
	assert.Equal(t, user.Name, userData["name"])

	workspacesData := response["workspaces"].([]interface{})
	assert.Equal(t, 2, len(workspacesData))

	// Test unauthorized access
	req = httptest.NewRequest(http.MethodGet, "/api/user.me", nil)
	rec = httptest.NewRecorder()

	handler.GetCurrentUser(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)

	// Test user not found
	notFoundUserID := "unknown-user-id"
	mockUserSvc.EXPECT().
		GetUserByID(gomock.Any(), notFoundUserID).
		Return(nil, fmt.Errorf("user not found"))

	req = httptest.NewRequest(http.MethodGet, "/api/user.me", nil)
	req = req.WithContext(context.WithValue(req.Context(), domain.UserIDKey, notFoundUserID))
	rec = httptest.NewRecorder()

	handler.GetCurrentUser(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)

	// Test workspaces retrieval error
	errorUserID := "error-workspace-user"
	mockUserSvc.EXPECT().
		GetUserByID(gomock.Any(), errorUserID).
		Return(user, nil)
	mockWorkspaceSvc.EXPECT().
		ListWorkspaces(gomock.Any()).
		Return(nil, fmt.Errorf("database error"))

	req = httptest.NewRequest(http.MethodGet, "/api/user.me", nil)
	req = req.WithContext(context.WithValue(req.Context(), domain.UserIDKey, errorUserID))
	rec = httptest.NewRecorder()

	handler.GetCurrentUser(rec, req)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, rec.Body.String(), "Failed to retrieve workspaces")
}

func TestUserHandler_RegisterRoutes(t *testing.T) {
	handler, mockUserSvc, mockWorkspaceSvc, secretKey := setupUserHandlerTest(t)

	// Set up mock expectation for VerifyUserSession to prevent unexpected call error
	mockUserSvc.EXPECT().
		VerifyUserSession(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&domain.User{ID: "user1", Email: "user@example.com"}, nil)

	// Set up mock expectation for GetUserByID with specific user ID
	mockUserSvc.EXPECT().
		GetUserByID(gomock.Any(), "user1").
		Return(&domain.User{ID: "user1", Email: "user@example.com"}, nil)

	// Set up mock expectation for ListWorkspaces
	mockWorkspaceSvc.EXPECT().
		ListWorkspaces(gomock.Any()).
		Return([]*domain.Workspace{}, nil)

	// Test cases for different scenarios
	testCases := []struct {
		name       string
		route      string
		setupMocks func()
	}{
		{
			name:  "public routes",
			route: "/api/user.signin",
			setupMocks: func() {
				// No mock setup needed for testing route registration
			},
		},
		{
			name:  "protected routes with auth service",
			route: "/api/user.me",
			setupMocks: func() {
				// Setup mock for auth middleware
				mockUserSvc.EXPECT().
					GetUserByID(gomock.Any(), gomock.Any()).
					Return(&domain.User{
						ID:    "user1",
						Email: "user@example.com",
					}, nil)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a new HTTP multiplexer for each test case
			mux := http.NewServeMux()

			// Register routes
			handler.RegisterRoutes(mux)

			// Test server for this multiplexer
			server := httptest.NewServer(mux)
			defer server.Close()

			// Make a request to the test path
			req, err := http.NewRequest("GET", server.URL+tc.route, nil)
			require.NoError(t, err)

			// For protected routes, we need to add a valid token
			if tc.route == "/api/user.me" {
				token := paseto.NewToken()
				token.SetString(string(domain.UserIDKey), "user1")
				token.SetString(string(domain.SessionIDKey), "session1")
				token.SetExpiration(time.Now().Add(time.Hour))

				signedToken := token.V4Sign(secretKey, nil)
				req.Header.Set("Authorization", "Bearer "+signedToken)
			}

			// Send the request
			client := &http.Client{}
			resp, err := client.Do(req)

			// We don't care about the response content, just that a handler was registered
			// and it didn't return 404 Not Found
			if tc.route == "/api/user.me" {
				require.NoError(t, err)
				defer resp.Body.Close()
				assert.NotEqual(t, http.StatusNotFound, resp.StatusCode)
			}
		})
	}
}
