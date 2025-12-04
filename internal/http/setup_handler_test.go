package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"

	"github.com/Notifuse/notifuse/internal/service"

	"github.com/Notifuse/notifuse/pkg/logger"

	"github.com/stretchr/testify/assert"

	"github.com/stretchr/testify/require"
)

// mockAppShutdowner implements AppShutdowner for testing
type mockAppShutdowner struct {
	shutdownCalled bool
	shutdownError  error
}

func (m *mockAppShutdowner) Shutdown(ctx context.Context) error {
	m.shutdownCalled = true
	return m.shutdownError
}

func newMockAppShutdowner() *mockAppShutdowner {
	return &mockAppShutdowner{}
}

// mockSettingRepository is a mock implementation of domain.SettingRepository
type mockSettingRepository struct {
	settings map[string]string
}

func newMockSettingRepository() *mockSettingRepository {
	return &mockSettingRepository{
		settings: make(map[string]string),
	}
}

func (m *mockSettingRepository) Get(ctx context.Context, key string) (*domain.Setting, error) {
	value, ok := m.settings[key]
	if !ok {
		return nil, &domain.ErrSettingNotFound{Key: key}
	}
	return &domain.Setting{Key: key, Value: value}, nil
}

func (m *mockSettingRepository) Set(ctx context.Context, key, value string) error {
	m.settings[key] = value
	return nil
}

func (m *mockSettingRepository) Delete(ctx context.Context, key string) error {
	delete(m.settings, key)
	return nil
}

func (m *mockSettingRepository) List(ctx context.Context) ([]*domain.Setting, error) {
	settings := make([]*domain.Setting, 0, len(m.settings))
	for k, v := range m.settings {
		settings = append(settings, &domain.Setting{Key: k, Value: v})
	}
	return settings, nil
}

func (m *mockSettingRepository) GetLastCronRun(ctx context.Context) (*time.Time, error) {
	return nil, nil
}

func (m *mockSettingRepository) SetLastCronRun(ctx context.Context) error {
	return nil
}

// mockUserRepository is a mock implementation of domain.UserRepository
type mockUserRepository struct {
	users []*domain.User
}

func newMockUserRepository() *mockUserRepository {
	return &mockUserRepository{
		users: make([]*domain.User, 0),
	}
}

func (m *mockUserRepository) CreateUser(ctx context.Context, user *domain.User) error {
	m.users = append(m.users, user)
	return nil
}

func (m *mockUserRepository) GetUserByID(ctx context.Context, id string) (*domain.User, error) {
	return nil, nil
}

func (m *mockUserRepository) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	return nil, nil
}

func (m *mockUserRepository) CreateSession(ctx context.Context, session *domain.Session) error {
	return nil
}

func (m *mockUserRepository) GetSessionByID(ctx context.Context, id string) (*domain.Session, error) {
	return nil, nil
}

func (m *mockUserRepository) GetSessionsByUserID(ctx context.Context, userID string) ([]*domain.Session, error) {
	return nil, nil
}

func (m *mockUserRepository) UpdateSession(ctx context.Context, session *domain.Session) error {
	return nil
}

func (m *mockUserRepository) DeleteSession(ctx context.Context, id string) error {
	return nil
}

func (m *mockUserRepository) DeleteAllSessionsByUserID(ctx context.Context, userID string) error {
	return nil
}

func (m *mockUserRepository) Delete(ctx context.Context, id string) error {
	return nil
}

func TestSetupHandler_Status(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		isInstalled    bool
		expectedStatus int
		expectBody     bool
	}{
		{
			name:           "GET request returns not installed status",
			method:         http.MethodGet,
			isInstalled:    false,
			expectedStatus: http.StatusOK,
			expectBody:     true,
		},
		{
			name:           "GET request returns installed status",
			method:         http.MethodGet,
			isInstalled:    true,
			expectedStatus: http.StatusOK,
			expectBody:     true,
		},
		{
			name:           "POST request returns method not allowed",
			method:         http.MethodPost,
			isInstalled:    false,
			expectedStatus: http.StatusMethodNotAllowed,
			expectBody:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			settingRepo := newMockSettingRepository()
			if tt.isInstalled {
				_ = settingRepo.Set(context.Background(), "is_installed", "true")
			}

			settingService := service.NewSettingService(settingRepo)
			userRepo := newMockUserRepository()
			userService := &service.UserService{}
			setupService := service.NewSetupService(settingService, userService, userRepo, logger.NewLogger(), "secret-key", nil, nil)

			handler := NewSetupHandler(
				setupService,
				settingService,
				logger.NewLogger(),
				newMockAppShutdowner(),
			)

			// Create request
			req := httptest.NewRequest(tt.method, "/api/setup.status", nil)
			w := httptest.NewRecorder()

			// Execute
			handler.Status(w, req)

			// Assert
			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectBody {
				var response StatusResponse
				err := json.NewDecoder(w.Body).Decode(&response)
				require.NoError(t, err)
				assert.Equal(t, tt.isInstalled, response.IsInstalled)
			}
		})
	}
}

func TestSetupHandler_TestSMTP(t *testing.T) {
	tlsTrue := true
	tlsFalse := false

	tests := []struct {
		name           string
		method         string
		isInstalled    bool
		requestBody    interface{}
		expectedStatus int
	}{
		{
			name:        "Valid SMTP config with TLS but connection fails (expected)",
			method:      http.MethodPost,
			isInstalled: false,
			requestBody: TestSMTPRequest{
				SMTPHost:     "invalid-host.example.com",
				SMTPPort:     587,
				SMTPUsername: "user",
				SMTPPassword: "pass",
				SMTPUseTLS:   &tlsTrue,
			},
			expectedStatus: http.StatusBadRequest, // Will fail to connect
		},
		{
			name:        "Valid SMTP config without TLS but connection fails (expected)",
			method:      http.MethodPost,
			isInstalled: false,
			requestBody: TestSMTPRequest{
				SMTPHost:     "invalid-host.example.com",
				SMTPPort:     25,
				SMTPUsername: "user",
				SMTPPassword: "pass",
				SMTPUseTLS:   &tlsFalse,
			},
			expectedStatus: http.StatusBadRequest, // Will fail to connect
		},
		{
			name:        "Valid SMTP config without TLS field (defaults to true)",
			method:      http.MethodPost,
			isInstalled: false,
			requestBody: TestSMTPRequest{
				SMTPHost:     "invalid-host.example.com",
				SMTPPort:     587,
				SMTPUsername: "user",
				SMTPPassword: "pass",
			},
			expectedStatus: http.StatusBadRequest, // Will fail to connect
		},
		{
			name:           "Test SMTP forbidden when installed",
			method:         http.MethodPost,
			isInstalled:    true,
			requestBody:    TestSMTPRequest{SMTPHost: "smtp.example.com", SMTPPort: 587},
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "GET request returns method not allowed",
			method:         http.MethodGet,
			isInstalled:    false,
			requestBody:    nil,
			expectedStatus: http.StatusMethodNotAllowed,
		},
		{
			name:           "Invalid request body",
			method:         http.MethodPost,
			isInstalled:    false,
			requestBody:    "invalid-json",
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			settingRepo := newMockSettingRepository()
			if tt.isInstalled {
				_ = settingRepo.Set(context.Background(), "is_installed", "true")
			}

			settingService := service.NewSettingService(settingRepo)
			userRepo := newMockUserRepository()
			userService := &service.UserService{}
			setupService := service.NewSetupService(settingService, userService, userRepo, logger.NewLogger(), "secret-key", nil, nil)

			handler := NewSetupHandler(
				setupService,
				settingService,
				logger.NewLogger(),
				newMockAppShutdowner(),
			)

			// Create request
			var body *bytes.Buffer
			if tt.requestBody != nil {
				if str, ok := tt.requestBody.(string); ok {
					body = bytes.NewBufferString(str)
				} else {
					jsonBody, _ := json.Marshal(tt.requestBody)
					body = bytes.NewBuffer(jsonBody)
				}
			} else {
				body = bytes.NewBuffer([]byte{})
			}

			req := httptest.NewRequest(tt.method, "/api/setup.testSmtp", body)
			w := httptest.NewRecorder()

			// Execute
			handler.TestSMTP(w, req)

			// Assert
			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestSetupHandler_Initialize(t *testing.T) {
	tlsTrue := true
	tlsFalse := false

	tests := []struct {
		name           string
		method         string
		isInstalled    bool
		requestBody    interface{}
		expectedStatus int
	}{
		{
			name:        "Valid initialization request with TLS enabled",
			method:      http.MethodPost,
			isInstalled: false,
			requestBody: InitializeRequest{
				RootEmail:     "admin@example.com",
				APIEndpoint:   "https://api.example.com",
				SMTPHost:      "smtp.example.com",
				SMTPPort:      587,
				SMTPFromEmail: "noreply@example.com",
				SMTPFromName:  "Test",
				SMTPUseTLS:    &tlsTrue,
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:        "Valid initialization request with TLS disabled",
			method:      http.MethodPost,
			isInstalled: false,
			requestBody: InitializeRequest{
				RootEmail:     "admin@example.com",
				APIEndpoint:   "https://api.example.com",
				SMTPHost:      "smtp.example.com",
				SMTPPort:      25,
				SMTPFromEmail: "noreply@example.com",
				SMTPFromName:  "Test",
				SMTPUseTLS:    &tlsFalse,
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:        "Valid initialization request without TLS field (defaults to true)",
			method:      http.MethodPost,
			isInstalled: false,
			requestBody: InitializeRequest{
				RootEmail:     "admin@example.com",
				APIEndpoint:   "https://api.example.com",
				SMTPHost:      "smtp.example.com",
				SMTPPort:      587,
				SMTPFromEmail: "noreply@example.com",
				SMTPFromName:  "Test",
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Already installed returns success response",
			method:         http.MethodPost,
			isInstalled:    true,
			requestBody:    InitializeRequest{RootEmail: "admin@example.com"},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "GET request returns method not allowed",
			method:         http.MethodGet,
			isInstalled:    false,
			requestBody:    nil,
			expectedStatus: http.StatusMethodNotAllowed,
		},
		{
			name:           "Invalid request body",
			method:         http.MethodPost,
			isInstalled:    false,
			requestBody:    "invalid-json",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:        "Missing required fields",
			method:      http.MethodPost,
			isInstalled: false,
			requestBody: InitializeRequest{
				APIEndpoint: "https://api.example.com",
				// Missing RootEmail
			},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			settingRepo := newMockSettingRepository()
			if tt.isInstalled {
				_ = settingRepo.Set(context.Background(), "is_installed", "true")
			}

			settingService := service.NewSettingService(settingRepo)
			userRepo := newMockUserRepository()
			userService := &service.UserService{}

			callbackCalled := false
			setupService := service.NewSetupService(
				settingService,
				userService,
				userRepo,
				logger.NewLogger(),
				"secret-key",
				func() error {
					callbackCalled = true
					return nil
				},
				nil,
			)

			handler := NewSetupHandler(
				setupService,
				settingService,
				logger.NewLogger(),
				newMockAppShutdowner(),
			)

			// Create request
			var body *bytes.Buffer
			if tt.requestBody != nil {
				if str, ok := tt.requestBody.(string); ok {
					body = bytes.NewBufferString(str)
				} else {
					jsonBody, _ := json.Marshal(tt.requestBody)
					body = bytes.NewBuffer(jsonBody)
				}
			} else {
				body = bytes.NewBuffer([]byte{})
			}

			req := httptest.NewRequest(tt.method, "/api/setup.initialize", body)
			w := httptest.NewRecorder()

			// Execute
			handler.Initialize(w, req)

			// Assert
			assert.Equal(t, tt.expectedStatus, w.Code)

			// Check callback was called for successful initialization
			if tt.expectedStatus == http.StatusOK && !tt.isInstalled {
				assert.True(t, callbackCalled, "Callback should be called on successful initialization")

				// Verify response
				var response InitializeResponse
				err := json.NewDecoder(w.Body).Decode(&response)
				require.NoError(t, err)
				assert.True(t, response.Success)
				assert.NotEmpty(t, response.Message)
			}

			// Verify response for already installed case
			if tt.expectedStatus == http.StatusOK && tt.isInstalled {
				var response InitializeResponse
				err := json.NewDecoder(w.Body).Decode(&response)
				require.NoError(t, err)
				assert.True(t, response.Success)
				assert.Contains(t, response.Message, "already completed")
			}
		})
	}
}

func TestSetupHandler_RegisterRoutes(t *testing.T) {
	// Setup
	settingRepo := newMockSettingRepository()
	settingService := service.NewSettingService(settingRepo)
	userRepo := newMockUserRepository()
	userService := &service.UserService{}
	setupService := service.NewSetupService(settingService, userService, userRepo, logger.NewLogger(), "secret-key", nil, nil)

	handler := NewSetupHandler(
		setupService,
		settingService,
		logger.NewLogger(),
		newMockAppShutdowner(),
	)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// Test that routes are accessible
	routes := []string{
		"/api/setup.status",
		"/api/setup.initialize",
		"/api/setup.testSmtp",
	}

	for _, route := range routes {
		t.Run("Route "+route, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, route, nil)
			w := httptest.NewRecorder()

			mux.ServeHTTP(w, req)

			// Should not be 404 (route is registered)
			assert.NotEqual(t, http.StatusNotFound, w.Code, "Route should be registered")
		})
	}
}
