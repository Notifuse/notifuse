package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockUserServiceForSettings implements UserServiceInterface for settings handler tests
type mockUserServiceForSettings struct {
	users map[string]*domain.User
}

func newMockUserServiceForSettings() *mockUserServiceForSettings {
	return &mockUserServiceForSettings{
		users: make(map[string]*domain.User),
	}
}

func (m *mockUserServiceForSettings) SignIn(ctx context.Context, input domain.SignInInput) (string, error) {
	return "", nil
}

func (m *mockUserServiceForSettings) VerifyCode(ctx context.Context, input domain.VerifyCodeInput) (*domain.AuthResponse, error) {
	return nil, nil
}

func (m *mockUserServiceForSettings) RootSignin(ctx context.Context, input domain.RootSigninInput) (*domain.AuthResponse, error) {
	return nil, nil
}

func (m *mockUserServiceForSettings) VerifyUserSession(ctx context.Context, userID string, sessionID string) (*domain.User, error) {
	return nil, nil
}

func (m *mockUserServiceForSettings) GetUserByID(ctx context.Context, userID string) (*domain.User, error) {
	user, ok := m.users[userID]
	if !ok {
		return nil, fmt.Errorf("user not found")
	}
	return user, nil
}

func (m *mockUserServiceForSettings) Logout(ctx context.Context, userID string) error {
	return nil
}

const testRootEmail = "root@example.com"
const testSecretKey = "test-secret-key-32-bytes-long!!"

func setupSettingsHandler(t *testing.T) (*SettingsHandler, *mockSettingRepository, *mockUserServiceForSettings, *mockAppShutdowner) {
	t.Helper()

	settingRepo := newMockSettingRepository()
	settingService := service.NewSettingService(settingRepo)
	userSvc := newMockUserServiceForSettings()
	shutdowner := newMockAppShutdowner()

	envConfig := &service.EnvironmentConfig{}
	userRepo := newMockUserRepository()
	setupService := service.NewSetupService(
		settingService,
		&service.UserService{},
		userRepo,
		logger.NewLogger(),
		testSecretKey,
		nil,
		envConfig,
	)

	handler := NewSettingsHandler(
		setupService,
		settingService,
		userSvc,
		func() ([]byte, error) { return []byte("test-jwt-secret"), nil },
		logger.NewLogger(),
		testSecretKey,
		testRootEmail,
		shutdowner,
	)

	// Add root user to mock
	userSvc.users["root-user-id"] = &domain.User{
		ID:    "root-user-id",
		Email: testRootEmail,
	}

	// Add non-root user
	userSvc.users["other-user-id"] = &domain.User{
		ID:    "other-user-id",
		Email: "other@example.com",
	}

	return handler, settingRepo, userSvc, shutdowner
}

func reqWithUserContext(req *http.Request, userID string) *http.Request {
	ctx := context.WithValue(req.Context(), domain.UserIDKey, userID)
	return req.WithContext(ctx)
}

// ============================================================
// Tests for GET /api/settings.get
// ============================================================

func TestSettingsHandler_Get_MethodNotAllowed(t *testing.T) {
	handler, _, _, _ := setupSettingsHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/settings.get", nil)
	req = reqWithUserContext(req, "root-user-id")
	w := httptest.NewRecorder()

	handler.handleGet(w, req)
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestSettingsHandler_Get_Unauthorized(t *testing.T) {
	handler, _, _, _ := setupSettingsHandler(t)

	// No user ID in context
	req := httptest.NewRequest(http.MethodGet, "/api/settings.get", nil)
	w := httptest.NewRecorder()

	handler.handleGet(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestSettingsHandler_Get_Forbidden_NonRootUser(t *testing.T) {
	handler, _, _, _ := setupSettingsHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/settings.get", nil)
	req = reqWithUserContext(req, "other-user-id")
	w := httptest.NewRecorder()

	handler.handleGet(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestSettingsHandler_Get_Success(t *testing.T) {
	handler, settingRepo, _, _ := setupSettingsHandler(t)

	// Seed some settings
	ctx := context.Background()
	_ = settingRepo.Set(ctx, "is_installed", "true")
	_ = settingRepo.Set(ctx, "root_email", testRootEmail)
	_ = settingRepo.Set(ctx, "api_endpoint", "https://api.example.com")
	_ = settingRepo.Set(ctx, "smtp_host", "smtp.example.com")
	_ = settingRepo.Set(ctx, "smtp_port", "587")
	_ = settingRepo.Set(ctx, "smtp_from_email", "noreply@example.com")
	_ = settingRepo.Set(ctx, "smtp_from_name", "Notifuse")
	_ = settingRepo.Set(ctx, "smtp_use_tls", "true")
	_ = settingRepo.Set(ctx, "telemetry_enabled", "true")
	_ = settingRepo.Set(ctx, "check_for_updates", "false")

	req := httptest.NewRequest(http.MethodGet, "/api/settings.get", nil)
	req = reqWithUserContext(req, "root-user-id")
	w := httptest.NewRecorder()

	handler.handleGet(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response SystemSettingsResponse
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	assert.Equal(t, testRootEmail, response.Settings.RootEmail)
	assert.Equal(t, "https://api.example.com", response.Settings.APIEndpoint)
	assert.Equal(t, "smtp.example.com", response.Settings.SMTPHost)
	assert.Equal(t, 587, response.Settings.SMTPPort)
	assert.Equal(t, "noreply@example.com", response.Settings.SMTPFromEmail)
	assert.Equal(t, "Notifuse", response.Settings.SMTPFromName)
	assert.True(t, response.Settings.SMTPUseTLS)
	assert.True(t, response.Settings.TelemetryEnabled)
	assert.False(t, response.Settings.CheckForUpdates)
}

func TestSettingsHandler_Get_MaskedSensitiveFields(t *testing.T) {
	handler, settingRepo, _, _ := setupSettingsHandler(t)

	ctx := context.Background()
	_ = settingRepo.Set(ctx, "is_installed", "true")
	_ = settingRepo.Set(ctx, "root_email", testRootEmail)
	// Store encrypted password (we can't easily encrypt in test, but handler reads via settingService which decrypts)
	// For this test, the mock repo stores raw values and GetSystemConfig will try to decrypt
	// We need to test masking behavior at the handler level

	req := httptest.NewRequest(http.MethodGet, "/api/settings.get", nil)
	req = reqWithUserContext(req, "root-user-id")
	w := httptest.NewRecorder()

	handler.handleGet(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response SystemSettingsResponse
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	// Password should be empty (not set in DB), not masked
	assert.Empty(t, response.Settings.SMTPPassword)
	// EnvOverrides should be present (even if all false)
	assert.NotNil(t, response.EnvOverrides)
}

func TestSettingsHandler_Get_EnvOverrides(t *testing.T) {
	// Create handler with env config that has some values
	settingRepo := newMockSettingRepository()
	settingService := service.NewSettingService(settingRepo)
	userSvc := newMockUserServiceForSettings()
	shutdowner := newMockAppShutdowner()

	envConfig := &service.EnvironmentConfig{
		RootEmail:   "env-root@example.com",
		SMTPHost:    "env-smtp.example.com",
		SMTPPort:    465,
	}
	userRepo := newMockUserRepository()
	setupService := service.NewSetupService(
		settingService,
		&service.UserService{},
		userRepo,
		logger.NewLogger(),
		testSecretKey,
		nil,
		envConfig,
	)

	handler := NewSettingsHandler(
		setupService,
		settingService,
		userSvc,
		func() ([]byte, error) { return []byte("test-jwt-secret"), nil },
		logger.NewLogger(),
		testSecretKey,
		testRootEmail,
		shutdowner,
	)

	userSvc.users["root-user-id"] = &domain.User{
		ID:    "root-user-id",
		Email: testRootEmail,
	}

	ctx := context.Background()
	_ = settingRepo.Set(ctx, "is_installed", "true")

	req := httptest.NewRequest(http.MethodGet, "/api/settings.get", nil)
	req = reqWithUserContext(req, "root-user-id")
	w := httptest.NewRecorder()

	handler.handleGet(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response SystemSettingsResponse
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	assert.True(t, response.EnvOverrides["root_email"])
	assert.True(t, response.EnvOverrides["smtp_host"])
	assert.True(t, response.EnvOverrides["smtp_port"])
	assert.False(t, response.EnvOverrides["api_endpoint"])
	assert.False(t, response.EnvOverrides["smtp_password"])
}

// ============================================================
// Tests for POST /api/settings.update
// ============================================================

func TestSettingsHandler_Update_MethodNotAllowed(t *testing.T) {
	handler, _, _, _ := setupSettingsHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/settings.update", nil)
	req = reqWithUserContext(req, "root-user-id")
	w := httptest.NewRecorder()

	handler.handleUpdate(w, req)
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestSettingsHandler_Update_Unauthorized(t *testing.T) {
	handler, _, _, _ := setupSettingsHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/settings.update", nil)
	w := httptest.NewRecorder()

	handler.handleUpdate(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestSettingsHandler_Update_Forbidden_NonRootUser(t *testing.T) {
	handler, _, _, _ := setupSettingsHandler(t)

	body, _ := json.Marshal(SystemSettingsData{RootEmail: "new@example.com"})
	req := httptest.NewRequest(http.MethodPost, "/api/settings.update", bytes.NewBuffer(body))
	req = reqWithUserContext(req, "other-user-id")
	w := httptest.NewRecorder()

	handler.handleUpdate(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestSettingsHandler_Update_InvalidBody(t *testing.T) {
	handler, _, _, _ := setupSettingsHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/settings.update", bytes.NewBufferString("invalid-json"))
	req = reqWithUserContext(req, "root-user-id")
	w := httptest.NewRecorder()

	handler.handleUpdate(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSettingsHandler_Update_Success(t *testing.T) {
	handler, settingRepo, _, _ := setupSettingsHandler(t)

	// Seed initial settings
	ctx := context.Background()
	_ = settingRepo.Set(ctx, "is_installed", "true")
	_ = settingRepo.Set(ctx, "root_email", testRootEmail)

	updateData := SystemSettingsData{
		RootEmail:       testRootEmail,
		APIEndpoint:     "https://new-api.example.com",
		SMTPHost:        "new-smtp.example.com",
		SMTPPort:        465,
		SMTPFromEmail:   "new@example.com",
		SMTPFromName:    "NewName",
		SMTPUseTLS:      true,
		TelemetryEnabled: true,
		CheckForUpdates:  true,
	}
	body, _ := json.Marshal(updateData)
	req := httptest.NewRequest(http.MethodPost, "/api/settings.update", bytes.NewBuffer(body))
	req = reqWithUserContext(req, "root-user-id")
	w := httptest.NewRecorder()

	handler.handleUpdate(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, true, response["success"])

	// Verify settings were persisted
	assert.Equal(t, "https://new-api.example.com", settingRepo.settings["api_endpoint"])
	assert.Equal(t, "new-smtp.example.com", settingRepo.settings["smtp_host"])
	assert.Equal(t, "465", settingRepo.settings["smtp_port"])
	assert.Equal(t, "true", settingRepo.settings["telemetry_enabled"])
	assert.Equal(t, "true", settingRepo.settings["check_for_updates"])
}

func TestSettingsHandler_Update_MaskedPasswordRetainsExisting(t *testing.T) {
	handler, settingRepo, _, _ := setupSettingsHandler(t)

	ctx := context.Background()
	_ = settingRepo.Set(ctx, "is_installed", "true")
	_ = settingRepo.Set(ctx, "root_email", testRootEmail)
	_ = settingRepo.Set(ctx, "smtp_host", "smtp.example.com")
	_ = settingRepo.Set(ctx, "smtp_port", "587")
	_ = settingRepo.Set(ctx, "smtp_from_email", "noreply@example.com")

	// First, do a normal update to set a real password (will be encrypted by SetSystemConfig)
	updateData1 := SystemSettingsData{
		RootEmail:     testRootEmail,
		SMTPHost:      "smtp.example.com",
		SMTPPort:      587,
		SMTPPassword:  "real-secret-password",
		SMTPFromEmail: "noreply@example.com",
	}
	body1, _ := json.Marshal(updateData1)
	req1 := httptest.NewRequest(http.MethodPost, "/api/settings.update", bytes.NewBuffer(body1))
	req1 = reqWithUserContext(req1, "root-user-id")
	w1 := httptest.NewRecorder()
	handler.handleUpdate(w1, req1)
	require.Equal(t, http.StatusOK, w1.Code)

	// Capture the encrypted password stored in the mock repo
	encryptedPassword := settingRepo.settings["encrypted_smtp_password"]
	require.NotEmpty(t, encryptedPassword)

	// Now send update with masked password sentinel
	updateData2 := SystemSettingsData{
		RootEmail:     testRootEmail,
		SMTPHost:      "smtp.example.com",
		SMTPPort:      587,
		SMTPPassword:  passwordMask, // sentinel value - should retain existing
		SMTPFromEmail: "noreply@example.com",
	}
	body2, _ := json.Marshal(updateData2)
	req2 := httptest.NewRequest(http.MethodPost, "/api/settings.update", bytes.NewBuffer(body2))
	req2 = reqWithUserContext(req2, "root-user-id")
	w2 := httptest.NewRecorder()
	handler.handleUpdate(w2, req2)

	assert.Equal(t, http.StatusOK, w2.Code)

	// Verify via GET that the password is still set (masked in response)
	req3 := httptest.NewRequest(http.MethodGet, "/api/settings.get", nil)
	req3 = reqWithUserContext(req3, "root-user-id")
	w3 := httptest.NewRecorder()
	handler.handleGet(w3, req3)

	assert.Equal(t, http.StatusOK, w3.Code)
	var getResponse SystemSettingsResponse
	err := json.NewDecoder(w3.Body).Decode(&getResponse)
	require.NoError(t, err)
	// Password should still be masked (meaning it's still set, not cleared)
	assert.Equal(t, passwordMask, getResponse.Settings.SMTPPassword)
}

func TestSettingsHandler_Update_ClearOptionalField(t *testing.T) {
	handler, settingRepo, _, _ := setupSettingsHandler(t)

	ctx := context.Background()
	_ = settingRepo.Set(ctx, "is_installed", "true")
	_ = settingRepo.Set(ctx, "root_email", testRootEmail)
	_ = settingRepo.Set(ctx, "smtp_ehlo_hostname", "old-hostname.example.com")

	// Send update with empty EHLO hostname to clear it
	updateData := SystemSettingsData{
		RootEmail:       testRootEmail,
		SMTPHost:        "smtp.example.com",
		SMTPPort:        587,
		SMTPFromEmail:   "noreply@example.com",
		SMTPEHLOHostname: "", // clearing this field
	}
	body, _ := json.Marshal(updateData)
	req := httptest.NewRequest(http.MethodPost, "/api/settings.update", bytes.NewBuffer(body))
	req = reqWithUserContext(req, "root-user-id")
	w := httptest.NewRecorder()

	handler.handleUpdate(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// The field should be cleared (empty string)
	assert.Equal(t, "", settingRepo.settings["smtp_ehlo_hostname"])
}

// ============================================================
// Tests for POST /api/settings.testSmtp
// ============================================================

func TestSettingsHandler_TestSMTP_MethodNotAllowed(t *testing.T) {
	handler, _, _, _ := setupSettingsHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/settings.testSmtp", nil)
	req = reqWithUserContext(req, "root-user-id")
	w := httptest.NewRecorder()

	handler.handleTestSMTP(w, req)
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestSettingsHandler_TestSMTP_Unauthorized(t *testing.T) {
	handler, _, _, _ := setupSettingsHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/settings.testSmtp", nil)
	w := httptest.NewRecorder()

	handler.handleTestSMTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestSettingsHandler_TestSMTP_Forbidden_NonRootUser(t *testing.T) {
	handler, _, _, _ := setupSettingsHandler(t)

	body, _ := json.Marshal(TestSMTPRequest{SMTPHost: "smtp.example.com", SMTPPort: 587})
	req := httptest.NewRequest(http.MethodPost, "/api/settings.testSmtp", bytes.NewBuffer(body))
	req = reqWithUserContext(req, "other-user-id")
	w := httptest.NewRecorder()

	handler.handleTestSMTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestSettingsHandler_TestSMTP_InvalidBody(t *testing.T) {
	handler, _, _, _ := setupSettingsHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/settings.testSmtp", bytes.NewBufferString("invalid"))
	req = reqWithUserContext(req, "root-user-id")
	w := httptest.NewRecorder()

	handler.handleTestSMTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSettingsHandler_TestSMTP_MissingHost(t *testing.T) {
	handler, _, _, _ := setupSettingsHandler(t)

	body, _ := json.Marshal(TestSMTPRequest{SMTPPort: 587})
	req := httptest.NewRequest(http.MethodPost, "/api/settings.testSmtp", bytes.NewBuffer(body))
	req = reqWithUserContext(req, "root-user-id")
	w := httptest.NewRecorder()

	handler.handleTestSMTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSettingsHandler_TestSMTP_ConnectionFails(t *testing.T) {
	handler, _, _, _ := setupSettingsHandler(t)

	body, _ := json.Marshal(TestSMTPRequest{
		SMTPHost: "invalid-host.example.com",
		SMTPPort: 587,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/settings.testSmtp", bytes.NewBuffer(body))
	req = reqWithUserContext(req, "root-user-id")
	w := httptest.NewRecorder()

	handler.handleTestSMTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ============================================================
// Tests for RegisterRoutes
// ============================================================

func TestSettingsHandler_RegisterRoutes(t *testing.T) {
	handler, _, _, _ := setupSettingsHandler(t)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	routes := []string{
		"/api/settings.get",
		"/api/settings.update",
		"/api/settings.testSmtp",
	}

	for _, route := range routes {
		t.Run("Route "+route, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, route, nil)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)
			// Should not be 404 (route is registered)
			assert.NotEqual(t, http.StatusNotFound, w.Code)
		})
	}
}
