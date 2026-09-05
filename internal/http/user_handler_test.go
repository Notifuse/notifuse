package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/golang-jwt/jwt/v5"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
)

func setupUserHandlerTest(t *testing.T) (*UserHandler, *mocks.MockUserServiceInterface, *mocks.MockWorkspaceServiceInterface, []byte) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockUserSvc := mocks.NewMockUserServiceInterface(ctrl)
	mockWorkspaceSvc := mocks.NewMockWorkspaceServiceInterface(ctrl)
	cfg := &config.Config{}

	// Create key pair for testing
	jwtSecret := []byte("test-jwt-secret-key-for-testing-32bytes")
	mockLogger := &pkgmocks.MockLogger{}
	// An unlicensed deployment, which is what most of this file is about: the licence source
	// is required by the constructor, and the free tier is the state every failure degrades
	// to. Tests that care about the licence overwrite handler.licenseService — the field is
	// unexported and these tests are in-package, the same way the settings handler's licence
	// tests do it. mockLicenseService is declared in settings_handler_test.go.
	handler := NewUserHandler(mockUserSvc, mockWorkspaceSvc, newMockLicenseService(), cfg, func() ([]byte, error) { return jwtSecret, nil }, mockLogger)

	return handler, mockUserSvc, mockWorkspaceSvc, jwtSecret
}

func TestUserHandler_SignIn(t *testing.T) {
	_, mockUserSvc, mockWorkspaceSvc, jwtSecret := setupUserHandlerTest(t)

	// Test with different configs
	devConfig := &config.Config{Environment: "development"}
	prodConfig := &config.Config{Environment: "production"}

	// Create handlers with different configs
	getJWTSecret := func() ([]byte, error) { return jwtSecret, nil }
	devHandler := NewUserHandler(mockUserSvc, mockWorkspaceSvc, newMockLicenseService(), devConfig, getJWTSecret, &pkgmocks.MockLogger{})
	prodHandler := NewUserHandler(mockUserSvc, mockWorkspaceSvc, newMockLicenseService(), prodConfig, getJWTSecret, &pkgmocks.MockLogger{})

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

func TestUserHandler_GetCurrentUser_WithSessionVerification(t *testing.T) {
	handler, mockUserSvc, mockWorkspaceSvc, _ := setupUserHandlerTest(t)

	userID := "test-user"
	sessionID := "test-session"
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
	}

	t.Run("successful get current user with valid session", func(t *testing.T) {
		mockUserSvc.EXPECT().
			VerifyUserSession(gomock.Any(), userID, sessionID).
			Return(user, nil)
		mockUserSvc.EXPECT().
			GetUserByID(gomock.Any(), userID).
			Return(user, nil)
		mockWorkspaceSvc.EXPECT().
			ListWorkspaces(gomock.Any()).
			Return(workspaces, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/user.me", nil)
		ctx := context.WithValue(req.Context(), domain.UserIDKey, userID)
		ctx = context.WithValue(ctx, domain.UserTypeKey, string(domain.UserTypeUser))
		ctx = context.WithValue(ctx, domain.SessionIDKey, sessionID)
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()

		handler.GetCurrentUser(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		var response map[string]interface{}
		err := json.NewDecoder(rec.Body).Decode(&response)
		require.NoError(t, err)
		assert.Equal(t, user.Email, response["user"].(map[string]interface{})["email"])
	})

	t.Run("unauthorized when session verification fails", func(t *testing.T) {
		mockUserSvc.EXPECT().
			VerifyUserSession(gomock.Any(), userID, sessionID).
			Return(nil, fmt.Errorf("%w: session not found", domain.ErrSessionAuthFailed))

		req := httptest.NewRequest(http.MethodGet, "/api/user.me", nil)
		ctx := context.WithValue(req.Context(), domain.UserIDKey, userID)
		ctx = context.WithValue(ctx, domain.UserTypeKey, string(domain.UserTypeUser))
		ctx = context.WithValue(ctx, domain.SessionIDKey, sessionID)
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()

		handler.GetCurrentUser(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
		var response map[string]interface{}
		err := json.NewDecoder(rec.Body).Decode(&response)
		require.NoError(t, err)
		assert.Equal(t, "Session expired or invalid", response["error"])
	})

	// A 401 makes the console delete the stored token and leave the page it is on,
	// so an unanswerable session lookup — a database blip, not a rejected caller —
	// must not be reported that way.
	t.Run("server error when session verification is unanswerable", func(t *testing.T) {
		mockUserSvc.EXPECT().
			VerifyUserSession(gomock.Any(), userID, sessionID).
			Return(nil, fmt.Errorf("pq: password authentication failed for user"))

		req := httptest.NewRequest(http.MethodGet, "/api/user.me", nil)
		ctx := context.WithValue(req.Context(), domain.UserIDKey, userID)
		ctx = context.WithValue(ctx, domain.UserTypeKey, string(domain.UserTypeUser))
		ctx = context.WithValue(ctx, domain.SessionIDKey, sessionID)
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()

		handler.GetCurrentUser(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		var response map[string]interface{}
		err := json.NewDecoder(rec.Body).Decode(&response)
		require.NoError(t, err)
		assert.Equal(t, "Failed to verify session", response["error"])
	})

	t.Run("unauthorized when session ID missing for user type", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/user.me", nil)
		ctx := context.WithValue(req.Context(), domain.UserIDKey, userID)
		ctx = context.WithValue(ctx, domain.UserTypeKey, string(domain.UserTypeUser))
		// Intentionally not setting SessionIDKey
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()

		handler.GetCurrentUser(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
		var response map[string]interface{}
		err := json.NewDecoder(rec.Body).Decode(&response)
		require.NoError(t, err)
		assert.Equal(t, "Unauthorized", response["error"])
	})

	t.Run("skip session verification for API key type", func(t *testing.T) {
		// API keys don't have sessions, so session verification should be skipped
		mockUserSvc.EXPECT().
			GetUserByID(gomock.Any(), userID).
			Return(user, nil)
		mockWorkspaceSvc.EXPECT().
			ListWorkspaces(gomock.Any()).
			Return(workspaces, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/user.me", nil)
		ctx := context.WithValue(req.Context(), domain.UserIDKey, userID)
		ctx = context.WithValue(ctx, domain.UserTypeKey, string(domain.UserTypeAPIKey))
		// No SessionIDKey for API keys
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()

		handler.GetCurrentUser(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
	})
}

func TestUserHandler_Logout(t *testing.T) {
	handler, mockUserSvc, _, _ := setupUserHandlerTest(t)

	t.Run("successful logout", func(t *testing.T) {
		userID := "test-user"

		mockUserSvc.EXPECT().
			Logout(gomock.Any(), userID).
			Return(nil)

		req := httptest.NewRequest(http.MethodPost, "/api/user.logout", nil)
		ctx := context.WithValue(req.Context(), domain.UserIDKey, userID)
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()

		handler.Logout(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var response map[string]string
		err := json.NewDecoder(rec.Body).Decode(&response)
		require.NoError(t, err)
		assert.Equal(t, "Logged out successfully", response["message"])
	})

	t.Run("unauthorized when no user ID in context", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/user.logout", nil)
		// No user ID in context
		rec := httptest.NewRecorder()

		handler.Logout(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)

		var response map[string]string
		err := json.NewDecoder(rec.Body).Decode(&response)
		require.NoError(t, err)
		assert.Equal(t, "Unauthorized", response["error"])
	})

	t.Run("internal error when logout fails", func(t *testing.T) {
		userID := "test-user"

		mockUserSvc.EXPECT().
			Logout(gomock.Any(), userID).
			Return(fmt.Errorf("database error"))

		req := httptest.NewRequest(http.MethodPost, "/api/user.logout", nil)
		ctx := context.WithValue(req.Context(), domain.UserIDKey, userID)
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()

		handler.Logout(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)

		var response map[string]string
		err := json.NewDecoder(rec.Body).Decode(&response)
		require.NoError(t, err)
		assert.Equal(t, "Failed to logout", response["error"])
	})

	t.Run("unauthorized with empty user ID", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/user.logout", nil)
		ctx := context.WithValue(req.Context(), domain.UserIDKey, "")
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()

		handler.Logout(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})
}

func TestUserHandler_UpdateLanguage(t *testing.T) {
	handler, mockUserSvc, _, _ := setupUserHandlerTest(t)

	t.Run("successful update", func(t *testing.T) {
		userID := "test-user"

		mockUserSvc.EXPECT().
			UpdateUserLanguage(gomock.Any(), userID, "fr").
			Return(nil)

		body, _ := json.Marshal(map[string]string{"language": "fr"})
		req := httptest.NewRequest(http.MethodPost, "/api/user.updateLanguage", bytes.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), domain.UserIDKey, userID))
		rec := httptest.NewRecorder()

		handler.UpdateLanguage(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		var response map[string]string
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&response))
		assert.Equal(t, "Language updated successfully", response["message"])
	})

	t.Run("rejects unsupported language with 400", func(t *testing.T) {
		mockUserSvc.EXPECT().
			UpdateUserLanguage(gomock.Any(), "test-user", "xx").
			Return(&domain.ErrUnsupportedLanguage{Language: "xx"})

		body, _ := json.Marshal(map[string]string{"language": "xx"})
		req := httptest.NewRequest(http.MethodPost, "/api/user.updateLanguage", bytes.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), domain.UserIDKey, "test-user"))
		rec := httptest.NewRecorder()

		handler.UpdateLanguage(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("invalid request body with 400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/user.updateLanguage", bytes.NewReader([]byte("not json")))
		req = req.WithContext(context.WithValue(req.Context(), domain.UserIDKey, "test-user"))
		rec := httptest.NewRecorder()

		handler.UpdateLanguage(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("unauthorized when no user ID in context", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"language": "fr"})
		req := httptest.NewRequest(http.MethodPost, "/api/user.updateLanguage", bytes.NewReader(body))
		rec := httptest.NewRecorder()

		handler.UpdateLanguage(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		userID := "missing-user"

		mockUserSvc.EXPECT().
			UpdateUserLanguage(gomock.Any(), userID, "fr").
			Return(&domain.ErrUserNotFound{Message: "user not found"})

		body, _ := json.Marshal(map[string]string{"language": "fr"})
		req := httptest.NewRequest(http.MethodPost, "/api/user.updateLanguage", bytes.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), domain.UserIDKey, userID))
		rec := httptest.NewRecorder()

		handler.UpdateLanguage(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("internal error returns 500", func(t *testing.T) {
		userID := "test-user"

		mockUserSvc.EXPECT().
			UpdateUserLanguage(gomock.Any(), userID, "de").
			Return(fmt.Errorf("database error"))

		body, _ := json.Marshal(map[string]string{"language": "de"})
		req := httptest.NewRequest(http.MethodPost, "/api/user.updateLanguage", bytes.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), domain.UserIDKey, userID))
		rec := httptest.NewRecorder()

		handler.UpdateLanguage(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}

func TestUserHandler_RootSignIn(t *testing.T) {
	handler, mockUserSvc, _, _ := setupUserHandlerTest(t)

	t.Run("successful root signin", func(t *testing.T) {
		input := domain.RootSigninInput{
			Email:     "root@example.com",
			Timestamp: time.Now().Unix(),
			Signature: "valid-signature-abc123",
		}

		mockUserSvc.EXPECT().
			RootSignin(gomock.Any(), input).
			Return(&domain.AuthResponse{
				Token: "jwt-token",
				User: domain.User{
					ID:    "root-user-id",
					Email: "root@example.com",
				},
				ExpiresAt: time.Now().Add(24 * time.Hour),
			}, nil)

		body, err := json.Marshal(input)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/user.rootSignin", bytes.NewReader(body))
		rec := httptest.NewRecorder()

		handler.RootSignIn(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var response domain.AuthResponse
		err = json.NewDecoder(rec.Body).Decode(&response)
		require.NoError(t, err)
		assert.Equal(t, "jwt-token", response.Token)
		assert.Equal(t, "root@example.com", response.User.Email)
	})

	t.Run("wrong HTTP method returns 405", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/user.rootSignin", nil)
		rec := httptest.NewRecorder()

		handler.RootSignIn(rec, req)

		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)

		var response map[string]string
		err := json.NewDecoder(rec.Body).Decode(&response)
		require.NoError(t, err)
		assert.Equal(t, "Method not allowed", response["error"])
	})

	t.Run("invalid JSON body returns 400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/user.rootSignin", bytes.NewReader([]byte("invalid json")))
		rec := httptest.NewRecorder()

		handler.RootSignIn(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var response map[string]string
		err := json.NewDecoder(rec.Body).Decode(&response)
		require.NoError(t, err)
		assert.Equal(t, "Invalid request body", response["error"])
	})

	t.Run("missing required fields returns 400", func(t *testing.T) {
		// Missing signature
		input := map[string]interface{}{
			"email":     "root@example.com",
			"timestamp": time.Now().Unix(),
		}

		body, err := json.Marshal(input)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/user.rootSignin", bytes.NewReader(body))
		rec := httptest.NewRecorder()

		handler.RootSignIn(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var response map[string]string
		err = json.NewDecoder(rec.Body).Decode(&response)
		require.NoError(t, err)
		assert.Contains(t, response["error"], "Missing required fields")
	})

	t.Run("missing timestamp returns 400", func(t *testing.T) {
		input := map[string]interface{}{
			"email":     "root@example.com",
			"signature": "some-sig",
		}

		body, err := json.Marshal(input)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/user.rootSignin", bytes.NewReader(body))
		rec := httptest.NewRecorder()

		handler.RootSignIn(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("service error returns 401 with generic message", func(t *testing.T) {
		input := domain.RootSigninInput{
			Email:     "root@example.com",
			Timestamp: time.Now().Unix(),
			Signature: "invalid-signature",
		}

		mockUserSvc.EXPECT().
			RootSignin(gomock.Any(), input).
			Return(nil, fmt.Errorf("unauthorized: invalid credentials"))

		body, err := json.Marshal(input)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/user.rootSignin", bytes.NewReader(body))
		rec := httptest.NewRecorder()

		handler.RootSignIn(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)

		var response map[string]string
		err = json.NewDecoder(rec.Body).Decode(&response)
		require.NoError(t, err)
		// Should return generic error message, not the actual service error
		assert.Equal(t, "Invalid credentials", response["error"])
	})
}

func TestUserHandler_RegisterRoutes(t *testing.T) {
	handler, mockUserSvc, mockWorkspaceSvc, jwtSecret := setupUserHandlerTest(t)

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
				claims := &service.UserClaims{
					UserID:    "user1",
					SessionID: "session1",
					RegisteredClaims: jwt.RegisteredClaims{
						ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
						IssuedAt:  jwt.NewNumericDate(time.Now()),
					},
				}
				token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
				signedToken, err := token.SignedString(jwtSecret)
				require.NoError(t, err)
				req.Header.Set("Authorization", "Bearer "+signedToken)
			}

			// Send the request
			client := &http.Client{}
			resp, err := client.Do(req)

			// We don't care about the response content, just that a handler was registered
			// and it didn't return 404 Not Found
			if tc.route == "/api/user.me" {
				require.NoError(t, err)
				defer func() { _ = resp.Body.Close() }()
				assert.NotEqual(t, http.StatusNotFound, resp.StatusCode)
			}
		})
	}
}

// user.me returns every workspace the caller belongs to, each with its
// integrations. It is the widest of the workspace-serialising endpoints — one
// call, all workspaces — and it lives in a different handler from the others, so
// it is the one a reviewer of workspace_handler.go would never see.
func TestUserHandler_GetCurrentUserRedactsCredentials(t *testing.T) {
	handler, mockUserSvc, mockWorkspaceSvc, _ := setupUserHandlerTest(t)

	const userID = "test-user"
	const smtpPassword = "SENTINEL-user-me-smtp-password"

	workspaceWithSecret := func(id string) *domain.Workspace {
		return &domain.Workspace{
			ID:   id,
			Name: "Acme",
			Integrations: domain.Integrations{{
				ID:   "int-1",
				Type: domain.IntegrationTypeEmail,
				EmailProvider: domain.EmailProvider{
					Kind: domain.EmailProviderKindSMTP,
					SMTP: &domain.SMTPSettings{
						Host:              "smtp.example.com",
						Password:          smtpPassword,
						EncryptedPassword: "ciphertext",
					},
				},
			}},
		}
	}

	mockUserSvc.EXPECT().
		GetUserByID(gomock.Any(), userID).
		Return(&domain.User{ID: userID, Email: "u@example.com"}, nil)
	mockWorkspaceSvc.EXPECT().
		ListWorkspaces(gomock.Any()).
		Return([]*domain.Workspace{workspaceWithSecret("ws1"), workspaceWithSecret("ws2")}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/user.me", nil)
	req = req.WithContext(context.WithValue(req.Context(), domain.UserIDKey, userID))
	rec := httptest.NewRecorder()

	handler.GetCurrentUser(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.NotContains(t, body, smtpPassword, "a decrypted credential reached /api/user.me")
	assert.Contains(t, body, "ciphertext", "the encrypted form still goes out, so the console can tell configured from unset")
	assert.Contains(t, body, "smtp.example.com", "non-secret context survives")
}

// TestUserHandler_GetCurrentUserFileManagerSecretDependsOnTheCaller closes the
// door that redacting workspaces.list alone left open.
//
// user.me returns the same workspaces, and an API-key token passes RequireAuth
// here exactly as a session does — so a key that could no longer read the S3
// secret from workspaces.list simply read it from this endpoint, one call later.
// It is also where the console gets its workspaces: AuthContext fills them from
// this response and the file manager builds its browser S3 client out of them, so
// the session half has to keep working or the feature breaks.
//
// Asserted against the raw bytes, not a decoded struct: what leaks is what goes
// on the wire, and the whole body is what an integration platform logs.
func TestUserHandler_GetCurrentUserFileManagerSecretDependsOnTheCaller(t *testing.T) {
	const fileManagerSecret = "SENTINEL-live-bucket-secret"
	const userID = "test-user"
	const sessionID = "test-session"

	user := &domain.User{ID: userID, Email: "u@example.com"}

	newWorkspace := func() *domain.Workspace {
		workspace := &domain.Workspace{ID: "ws1", Name: "Acme"}
		workspace.Settings.FileManager = domain.FileManagerSettings{
			Bucket:    "assets",
			AccessKey: "AKIAEXAMPLE",
			SecretKey: fileManagerSecret,
		}
		return workspace
	}

	t.Run("an API key does not get it", func(t *testing.T) {
		handler, mockUserSvc, mockWorkspaceSvc, _ := setupUserHandlerTest(t)
		mockUserSvc.EXPECT().GetUserByID(gomock.Any(), userID).Return(user, nil)
		mockWorkspaceSvc.EXPECT().ListWorkspaces(gomock.Any()).Return([]*domain.Workspace{newWorkspace()}, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/user.me", nil)
		ctx := context.WithValue(req.Context(), domain.UserIDKey, userID)
		// A key carries no session id, and the handler's session check skips it for
		// exactly that reason — which is how it reaches the workspace listing at all.
		ctx = context.WithValue(ctx, domain.UserTypeKey, string(domain.UserTypeAPIKey))
		rec := httptest.NewRecorder()

		handler.GetCurrentUser(rec, req.WithContext(ctx))

		require.Equal(t, http.StatusOK, rec.Code)
		body := rec.Body.String()
		assert.NotContains(t, body, fileManagerSecret)
		// The workspace itself still comes back whole; only the credential goes.
		assert.Contains(t, body, "ws1")
		assert.Contains(t, body, "assets")
	})

	t.Run("a console session does", func(t *testing.T) {
		handler, mockUserSvc, mockWorkspaceSvc, _ := setupUserHandlerTest(t)
		mockUserSvc.EXPECT().VerifyUserSession(gomock.Any(), userID, sessionID).Return(user, nil)
		mockUserSvc.EXPECT().GetUserByID(gomock.Any(), userID).Return(user, nil)
		mockWorkspaceSvc.EXPECT().ListWorkspaces(gomock.Any()).Return([]*domain.Workspace{newWorkspace()}, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/user.me", nil)
		ctx := context.WithValue(req.Context(), domain.UserIDKey, userID)
		ctx = context.WithValue(ctx, domain.UserTypeKey, string(domain.UserTypeUser))
		ctx = context.WithValue(ctx, domain.SessionIDKey, sessionID)
		rec := httptest.NewRecorder()

		handler.GetCurrentUser(rec, req.WithContext(ctx))

		require.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), fileManagerSecret,
			"withholding this from the console breaks the browser file manager")
	})
}

// The console's licence state is fed from /api/user.me and from nowhere else.
//
// /api/licence.get is the other source and it is root-only, so a non-root session that could
// not read the licence off this payload would meet each gate as an unexplained refusal.
// console/src/contexts/LicenseContext.tsx prefers this response precisely because it costs no
// extra round trip.
func TestUserHandler_GetCurrentUserCarriesEntitlements(t *testing.T) {
	const userID = "test-user"
	const sessionID = "test-session"

	user := &domain.User{ID: userID, Email: "u@example.com"}

	// call drives one request and returns the decoded body. licence is applied to the
	// handler before the call, so a test states the deployment's licence state and nothing
	// else. callAs additionally says which address is root; call is callAs with none.
	var callAs func(t *testing.T, userType, rootEmail string, licence func(*mockLicenseService)) map[string]interface{}
	call := func(t *testing.T, userType string, licence func(*mockLicenseService)) map[string]interface{} {
		return callAs(t, userType, "", licence)
	}
	callAs = func(t *testing.T, userType, rootEmail string, licence func(*mockLicenseService)) map[string]interface{} {
		t.Helper()

		handler, mockUserSvc, mockWorkspaceSvc, _ := setupUserHandlerTest(t)
		handler.config.RootEmail = rootEmail
		licenseSvc := newMockLicenseService()
		licence(licenseSvc)
		handler.licenseService = licenseSvc

		ctx := context.WithValue(context.Background(), domain.UserIDKey, userID)
		ctx = context.WithValue(ctx, domain.UserTypeKey, userType)
		if userType == string(domain.UserTypeUser) {
			ctx = context.WithValue(ctx, domain.SessionIDKey, sessionID)
			mockUserSvc.EXPECT().VerifyUserSession(gomock.Any(), userID, sessionID).Return(user, nil)
		}
		mockUserSvc.EXPECT().GetUserByID(gomock.Any(), userID).Return(user, nil)
		mockWorkspaceSvc.EXPECT().ListWorkspaces(gomock.Any()).Return([]*domain.Workspace{}, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/user.me", nil).WithContext(ctx)
		rec := httptest.NewRecorder()
		handler.GetCurrentUser(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		var body map[string]interface{}
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&body))
		return body
	}

	// Absence is the regression this whole test exists for, so it is reported as a failed
	// assertion rather than as a nil map assertion panicking three subtests later.
	entitlementsOf := func(t *testing.T, body map[string]interface{}) map[string]interface{} {
		t.Helper()

		entitlements, ok := body["entitlements"].(map[string]interface{})
		require.True(t, ok, "user.me carried no entitlements, so the banner has no state to render")
		return entitlements
	}

	t.Run("a console session is told what the deployment is licensed for", func(t *testing.T) {
		body := call(t, string(domain.UserTypeUser), func(m *mockLicenseService) {
			m.entitlements = licensedEntitlements()
		})

		entitlements := entitlementsOf(t, body)

		assert.Equal(t, "agency", entitlements["tier"])
		// But not who bought it. The licensee's name and billing address are what make
		// /api/licence.get root-only, and a non-root session is told everything the
		// banner needs and nothing it does not.
		assert.Equal(t, "", entitlements["org"])
		assert.Equal(t, "", entitlements["sub"])
		assert.Equal(t, float64(15), entitlements["max_workspaces"])
		assert.Equal(t, []interface{}{"rbac", "ses_tenant"}, entitlements["features"])
		assert.Equal(t, "grace", entitlements["state"])
		assert.Equal(t, "2026-03-01T12:00:00Z", entitlements["expires_at"])
	})

	t.Run("the root session is told who the licensee is, for the settings card", func(t *testing.T) {
		body := callAs(t, string(domain.UserTypeUser), user.Email, func(m *mockLicenseService) {
			m.entitlements = licensedEntitlements()
		})

		entitlements := entitlementsOf(t, body)
		assert.Equal(t, "ACME SAS", entitlements["org"])
		assert.Equal(t, "billing@acme.com", entitlements["sub"])
	})

	// The free tier is a state, not an absence. A console told nothing cannot tell an
	// unlicensed deployment from a server too old to answer, and treats unknown as licensed.
	t.Run("an unlicensed deployment reports the community tier rather than nothing", func(t *testing.T) {
		body := call(t, string(domain.UserTypeUser), func(m *mockLicenseService) {
			m.entitlements = domain.CommunityEntitlements()
		})

		entitlements := entitlementsOf(t, body)
		assert.Equal(t, "none", entitlements["state"])
		assert.Equal(t, float64(domain.CommunityMaxWorkspaces), entitlements["max_workspaces"])
		assert.Equal(t, []interface{}{}, entitlements["features"],
			"null features would crash the console, which iterates the field")
	})

	// The shape is the contract. console/src/types/license.ts declares Entitlements field for
	// field against internal/domain/license.go, and this endpoint must not invent a third
	// shape — a console reading max_workspaces from a payload that spells it maxWorkspaces
	// silently reads undefined and greys nothing out.
	t.Run("the entitlements object is exactly the shape the console declares", func(t *testing.T) {
		body := call(t, string(domain.UserTypeUser), func(m *mockLicenseService) {
			m.entitlements = licensedEntitlements()
		})

		entitlements := entitlementsOf(t, body)
		fields := make([]string, 0, len(entitlements))
		for field := range entitlements {
			fields = append(fields, field)
		}
		sort.Strings(fields)

		assert.Equal(t,
			[]string{"expires_at", "features", "max_workspaces", "org", "state", "sub", "tier"},
			fields,
			"the payload no longer matches console/src/types/license.ts")
	})

	// A licence key is a bearer credential: whoever holds it can license their own
	// deployment with it. Nothing derived from one may carry it back out, and this endpoint
	// is answered to every signed-in session rather than to root alone.
	t.Run("the raw key is never in the payload", func(t *testing.T) {
		body := call(t, string(domain.UserTypeUser), func(m *mockLicenseService) {
			m.entitlements = licensedEntitlements()
		})

		entitlements := entitlementsOf(t, body)
		assert.NotContains(t, entitlements, "key")
		assert.NotContains(t, entitlements, "license_key")
	})

	// An API key authenticates this same endpoint — it is how an integration resolves its
	// workspaces. Withholding the entitlements keeps the licensee's name and billing address
	// out of bodies that integration platforms log whole, the same rule that keeps the S3
	// secret away from machine traffic. Absence reads as "not told" on the console side,
	// which types the field optional.
	t.Run("an api key is told nothing about the licence", func(t *testing.T) {
		body := call(t, string(domain.UserTypeAPIKey), func(m *mockLicenseService) {
			m.entitlements = licensedEntitlements()
		})

		assert.NotContains(t, body, "entitlements")
		// The rest of the payload is untouched: this is a redaction, not a refusal.
		assert.NotNil(t, body["user"])
	})
}

// user.me is a read, and it is the call the console makes to find out what the deployment is
// licensed for. It answers 405 to every other method, in every licence state — a read that
// behaved differently depending on the licence would be the one endpoint a console could not
// trust to tell it the truth.
func TestUserHandler_GetCurrentUserRefusesEveryMethodButGET(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		t.Run("refuses "+method, func(t *testing.T) {
			// No service expectations: the guard must run before anything is looked up.
			handler, _, _, _ := setupUserHandlerTest(t)

			ctx := context.WithValue(context.Background(), domain.UserIDKey, "test-user")
			ctx = context.WithValue(ctx, domain.UserTypeKey, string(domain.UserTypeUser))
			req := httptest.NewRequest(method, "/api/user.me", nil).WithContext(ctx)
			rec := httptest.NewRecorder()

			handler.GetCurrentUser(rec, req)

			assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
		})
	}
}
