package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/pkg/crypto"
	"github.com/Notifuse/notifuse/pkg/license"
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

func (m *mockUserServiceForSettings) UpdateUserLanguage(ctx context.Context, userID string, language string) error {
	return nil
}

const testRootEmail = "root@example.com"
const testSecretKey = "test-secret-key-32-bytes-long!!"

func setupSettingsHandler(t *testing.T) (*SettingsHandler, *mockSettingRepository, *mockUserServiceForSettings, *mockAppShutdowner) {
	return setupSettingsHandlerWithRootEmail(t, testRootEmail)
}

func setupSettingsHandlerWithRootEmail(t *testing.T, rootEmail string) (*SettingsHandler, *mockSettingRepository, *mockUserServiceForSettings, *mockAppShutdowner) {
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

	// The real licence service over the same settings repository the rest of the harness
	// uses, so a test can assert on what the store actually holds after a refused paste.
	// With no key anywhere it resolves to Community, which is the state every existing
	// settings test runs in.
	licenseSvc := service.NewLicenseService(service.LicenseServiceConfig{
		SettingRepo: settingRepo,
		Logger:      logger.NewLogger(),
	})

	handler := NewSettingsHandler(
		setupService,
		settingService,
		userSvc,
		func() ([]byte, error) { return []byte("test-jwt-secret"), nil },
		logger.NewLogger(),
		testSecretKey,
		rootEmail,
		shutdowner,
		licenseSvc,
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

func TestSettingsHandler_Get_MultipleRootEmails(t *testing.T) {
	handler, settingRepo, userSvc, _ := setupSettingsHandlerWithRootEmail(t, testRootEmail+",second@example.com")

	ctx := context.Background()
	_ = settingRepo.Set(ctx, "is_installed", "true")
	_ = settingRepo.Set(ctx, "root_email", testRootEmail+",second@example.com")

	// A second listed root user.
	userSvc.users["second-root-id"] = &domain.User{
		ID:    "second-root-id",
		Email: "second@example.com",
	}

	t.Run("second listed root is allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/settings.get", nil)
		req = reqWithUserContext(req, "second-root-id")
		w := httptest.NewRecorder()

		handler.handleGet(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("first listed root is still allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/settings.get", nil)
		req = reqWithUserContext(req, "root-user-id")
		w := httptest.NewRecorder()

		handler.handleGet(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("non-listed user is forbidden", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/settings.get", nil)
		req = reqWithUserContext(req, "other-user-id")
		w := httptest.NewRecorder()

		handler.handleGet(w, req)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})
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
		RootEmail: "env-root@example.com",
		SMTPHost:  "env-smtp.example.com",
		SMTPPort:  465,
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
		newMockLicenseService(),
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

// When ROOT_EMAIL is set via env var, the displayed value must reflect the live
// resolved config (which prefers the env var), not the value persisted to the DB at
// install time. Simulates an operator adding a second root email and restarting: the
// DB setting still holds only the original email, but the panel must show both.
func TestSettingsHandler_Get_EnvOverride_UsesLiveRootEmail(t *testing.T) {
	const resolvedRootEmails = "root@example.com,second@example.com"

	settingRepo := newMockSettingRepository()
	settingService := service.NewSettingService(settingRepo)
	userSvc := newMockUserServiceForSettings()
	shutdowner := newMockAppShutdowner()

	// Env var is set (override detected) and carries the full, up-to-date list.
	envConfig := &service.EnvironmentConfig{RootEmail: resolvedRootEmails}
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

	// h.rootEmail mirrors config.RootEmail (env wins) — the value the app uses for auth.
	handler := NewSettingsHandler(
		setupService,
		settingService,
		userSvc,
		func() ([]byte, error) { return []byte("test-jwt-secret"), nil },
		logger.NewLogger(),
		testSecretKey,
		resolvedRootEmails,
		shutdowner,
		newMockLicenseService(),
	)

	userSvc.users["root-user-id"] = &domain.User{ID: "root-user-id", Email: testRootEmail}

	ctx := context.Background()
	_ = settingRepo.Set(ctx, "is_installed", "true")
	// Stale DB value frozen at install time (only the first email).
	_ = settingRepo.Set(ctx, "root_email", testRootEmail)

	req := httptest.NewRequest(http.MethodGet, "/api/settings.get", nil)
	req = reqWithUserContext(req, "root-user-id")
	w := httptest.NewRecorder()

	handler.handleGet(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var response SystemSettingsResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&response))

	// Must surface the live resolved value, not the stale DB setting.
	assert.Equal(t, resolvedRootEmails, response.Settings.RootEmail)
	assert.True(t, response.EnvOverrides["root_email"])
}

// Every env-overridden field (not just root_email) must display the live env value
// instead of the value persisted to the DB at install time, and secrets must stay
// masked even when sourced from the env override.
func TestSettingsHandler_Get_EnvOverride_AllFieldsUseLiveValues(t *testing.T) {
	settingRepo := newMockSettingRepository()
	settingService := service.NewSettingService(settingRepo)
	userSvc := newMockUserServiceForSettings()
	shutdowner := newMockAppShutdowner()

	envConfig := &service.EnvironmentConfig{
		RootEmail:               "env-root@example.com",
		APIEndpoint:             "https://env.example.com",
		SMTPHost:                "env-smtp.example.com",
		SMTPPort:                465,
		SMTPUsername:            "env-user",
		SMTPPassword:            "env-secret",
		SMTPFromEmail:           "env-from@example.com",
		SMTPFromName:            "Env Sender",
		SMTPUseTLS:              "false", // explicit false must beat DB "true"
		SMTPEHLOHostname:        "ehlo.env.example.com",
		SMTPBridgeEnabled:       "true",
		SMTPBridgeDomain:        "bridge.env.example.com",
		SMTPBridgePort:          2525,
		SMTPBridgeTLSCertBase64: "env-cert",
		SMTPBridgeTLSKeyBase64:  "env-key",
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
		envConfig.RootEmail,
		shutdowner,
		newMockLicenseService(),
	)
	// Root user's email must match the env-configured root for authorization.
	userSvc.users["root-user-id"] = &domain.User{ID: "root-user-id", Email: "env-root@example.com"}

	ctx := context.Background()
	_ = settingRepo.Set(ctx, "is_installed", "true")
	// Stale DB values that must be shadowed by the env overrides.
	_ = settingRepo.Set(ctx, "root_email", "db-root@example.com")
	_ = settingRepo.Set(ctx, "smtp_host", "db-smtp.example.com")
	_ = settingRepo.Set(ctx, "smtp_port", "25")
	_ = settingRepo.Set(ctx, "smtp_use_tls", "true")
	_ = settingRepo.Set(ctx, "smtp_bridge_domain", "db-bridge.example.com")

	req := httptest.NewRequest(http.MethodGet, "/api/settings.get", nil)
	req = reqWithUserContext(req, "root-user-id")
	w := httptest.NewRecorder()

	handler.handleGet(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var response SystemSettingsResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&response))

	s := response.Settings
	assert.Equal(t, "env-root@example.com", s.RootEmail)
	assert.Equal(t, "https://env.example.com", s.APIEndpoint)
	assert.Equal(t, "env-smtp.example.com", s.SMTPHost)
	assert.Equal(t, 465, s.SMTPPort)
	assert.Equal(t, "env-user", s.SMTPUsername)
	assert.Equal(t, "env-from@example.com", s.SMTPFromEmail)
	assert.Equal(t, "Env Sender", s.SMTPFromName)
	assert.False(t, s.SMTPUseTLS) // env "false" beats DB "true"
	assert.Equal(t, "ehlo.env.example.com", s.SMTPEHLOHostname)
	assert.True(t, s.SMTPBridgeEnabled)
	assert.Equal(t, "bridge.env.example.com", s.SMTPBridgeDomain)
	assert.Equal(t, 2525, s.SMTPBridgePort)

	// Secrets sourced from env must be masked, never returned in the clear.
	assert.Equal(t, passwordMask, s.SMTPPassword)
	assert.NotEqual(t, "env-secret", s.SMTPPassword)
	assert.Equal(t, configuredMask, s.SMTPBridgeTLSCertBase64)
	assert.Equal(t, configuredMask, s.SMTPBridgeTLSKeyBase64)
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
		RootEmail:        testRootEmail,
		APIEndpoint:      "https://new-api.example.com",
		SMTPHost:         "new-smtp.example.com",
		SMTPPort:         465,
		SMTPFromEmail:    "new@example.com",
		SMTPFromName:     "NewName",
		SMTPUseTLS:       true,
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
		RootEmail:        testRootEmail,
		SMTPHost:         "smtp.example.com",
		SMTPPort:         587,
		SMTPFromEmail:    "noreply@example.com",
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

// TestSettingsHandler_Update_InvalidOIDCConfigRejected ensures an invalid OIDC config
// is rejected with 400 BEFORE persist+restart, instead of bricking the server on the
// next boot (config.OIDCConfig.Validate would abort startup).
func TestSettingsHandler_Update_InvalidOIDCConfigRejected(t *testing.T) {
	handler, settingRepo, _, shutdowner := setupSettingsHandler(t)

	ctx := context.Background()
	_ = settingRepo.Set(ctx, "is_installed", "true")
	_ = settingRepo.Set(ctx, "root_email", testRootEmail)

	// OIDC enabled but issuer/client/secret missing -> Validate must fail.
	bad := SystemSettingsData{
		RootEmail:    testRootEmail,
		APIEndpoint:  "https://app.example.com",
		OIDCEnabled:  true,
		OIDCClientID: "",
	}
	body, _ := json.Marshal(bad)
	req := httptest.NewRequest(http.MethodPost, "/api/settings.update", bytes.NewBuffer(body))
	req = reqWithUserContext(req, "root-user-id")
	w := httptest.NewRecorder()
	handler.handleUpdate(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code, "invalid OIDC config must be rejected, not persisted")
	assert.False(t, shutdowner.shutdownCalled, "no restart may be triggered for a rejected config")
	assert.Empty(t, settingRepo.settings["oidc_enabled"], "nothing may be persisted for a rejected config")

	// Auto-create with no allowlist must also be rejected.
	bad2 := SystemSettingsData{
		RootEmail:           testRootEmail,
		APIEndpoint:         "https://app.example.com",
		OIDCEnabled:         true,
		OIDCIssuerURL:       "https://idp.example.com",
		OIDCClientID:        "cid",
		OIDCClientSecret:    "secret",
		OIDCAutoCreateUsers: true,
		OIDCAllowedDomains:  "",
	}
	body2, _ := json.Marshal(bad2)
	req2 := httptest.NewRequest(http.MethodPost, "/api/settings.update", bytes.NewBuffer(body2))
	req2 = reqWithUserContext(req2, "root-user-id")
	w2 := httptest.NewRecorder()
	handler.handleUpdate(w2, req2)
	assert.Equal(t, http.StatusBadRequest, w2.Code, "JIT without an allowlist must be rejected")
}

// TestSettingsHandler_Update_OIDCSecretMaskRetainsExisting proves the OIDC client
// secret survives a settings save that submits the mask sentinel (the operator opened
// the drawer and clicked Save without re-typing the secret). A regression here would
// silently overwrite the stored secret with the mask literal and break SSO for everyone.
func TestSettingsHandler_Update_OIDCSecretMaskRetainsExisting(t *testing.T) {
	handler, settingRepo, _, _ := setupSettingsHandler(t)

	ctx := context.Background()
	_ = settingRepo.Set(ctx, "is_installed", "true")
	_ = settingRepo.Set(ctx, "root_email", testRootEmail)

	valid := SystemSettingsData{
		RootEmail:        testRootEmail,
		APIEndpoint:      "https://app.example.com",
		OIDCEnabled:      true,
		OIDCIssuerURL:    "https://idp.example.com",
		OIDCClientID:     "cid",
		OIDCClientSecret: "real-oidc-secret",
	}
	body, _ := json.Marshal(valid)
	req := httptest.NewRequest(http.MethodPost, "/api/settings.update", bytes.NewBuffer(body))
	req = reqWithUserContext(req, "root-user-id")
	w := httptest.NewRecorder()
	handler.handleUpdate(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	encrypted := settingRepo.settings["encrypted_oidc_client_secret"]
	require.NotEmpty(t, encrypted)
	require.NotEqual(t, "real-oidc-secret", encrypted, "secret must be encrypted at rest")

	// Second save with the mask sentinel + an unrelated change must NOT touch the secret.
	masked := valid
	masked.OIDCClientSecret = passwordMask
	masked.OIDCButtonLabel = "Sign in with Acme"
	body2, _ := json.Marshal(masked)
	req2 := httptest.NewRequest(http.MethodPost, "/api/settings.update", bytes.NewBuffer(body2))
	req2 = reqWithUserContext(req2, "root-user-id")
	w2 := httptest.NewRecorder()
	handler.handleUpdate(w2, req2)
	require.Equal(t, http.StatusOK, w2.Code)

	// Assert the underlying SECRET (plaintext) is preserved. We decrypt rather than
	// compare ciphertext because AES-GCM re-encrypts with a fresh random nonce, so the
	// ciphertext legitimately changes even when the secret is retained.
	stored, derr := crypto.DecryptFromHexString(settingRepo.settings["encrypted_oidc_client_secret"], testSecretKey)
	require.NoError(t, derr)
	assert.Equal(t, "real-oidc-secret", stored,
		"submitting the mask sentinel must retain the existing OIDC client secret")
	assert.Equal(t, "Sign in with Acme", settingRepo.settings["oidc_button_label"], "the unrelated change must persist")
}

// oidcScopesUpdate posts a settings update with the given OIDC enabled/scopes values
// and returns the persisted oidc_scopes. Empty scopes with OIDC enabled must persist
// the full default: a stored "" or bare "openid" overrides the richer default at boot.
func oidcScopesUpdate(t *testing.T, enabled bool, scopes string) string {
	t.Helper()
	handler, settingRepo, _, _ := setupSettingsHandler(t)

	ctx := context.Background()
	_ = settingRepo.Set(ctx, "is_installed", "true")
	_ = settingRepo.Set(ctx, "root_email", testRootEmail)

	update := SystemSettingsData{
		RootEmail:   testRootEmail,
		APIEndpoint: "https://app.example.com",
		OIDCEnabled: enabled,
		OIDCScopes:  scopes,
	}
	if enabled {
		update.OIDCIssuerURL = "https://idp.example.com"
		update.OIDCClientID = "cid"
		update.OIDCClientSecret = "secret"
	}
	body, _ := json.Marshal(update)
	req := httptest.NewRequest(http.MethodPost, "/api/settings.update", bytes.NewBuffer(body))
	req = reqWithUserContext(req, "root-user-id")
	w := httptest.NewRecorder()
	handler.handleUpdate(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	return settingRepo.settings["oidc_scopes"]
}

func TestSettingsHandler_Update_OIDCEmptyScopes_PersistsDefault(t *testing.T) {
	assert.Equal(t, "openid email profile", oidcScopesUpdate(t, true, ""))
}

func TestSettingsHandler_Update_OIDCSeparatorOnlyScopes_PersistsDefault(t *testing.T) {
	// A stray comma/semicolon left in a cleared field contains no scope tokens
	// and must not sneak a bare "openid" into the DB.
	assert.Equal(t, "openid email profile", oidcScopesUpdate(t, true, " , ; "))
}

func TestSettingsHandler_Update_OIDCCustomScopes_ForcesOpenID(t *testing.T) {
	assert.Equal(t, "openid email profile", oidcScopesUpdate(t, true, "email profile"))
}

func TestSettingsHandler_Update_OIDCDisabled_ScopesUntouched(t *testing.T) {
	assert.Equal(t, "", oidcScopesUpdate(t, false, ""))
}

// consoleSettingsSaveBody is the body the console actually posts: its form library submits
// only the fields it registered, and every registered field carries a value. Secrets come
// back as the mask sentinel because the operator did not retype them.
//
// It is raw JSON, not a marshalled SystemSettingsData, on purpose: no field on that struct
// carries omitempty, so a Go literal always emits every key and cannot express a key the
// wire never carried — which is the whole defect.
const consoleSettingsSaveBody = `{
	"root_email": "root@example.com",
	"api_endpoint": "https://app.example.com",
	"smtp_host": "smtp.example.com",
	"smtp_port": 587,
	"smtp_username": "smtp-user",
	"smtp_password": "` + passwordMask + `",
	"smtp_from_email": "noreply@example.com",
	"smtp_from_name": "Acme",
	"smtp_use_tls": true,
	"smtp_ehlo_hostname": "",
	"telemetry_enabled": true,
	"check_for_updates": true,
	"smtp_bridge_enabled": false,
	"smtp_bridge_domain": "",
	"smtp_bridge_port": 0,
	"smtp_bridge_tls_cert_base64": "",
	"smtp_bridge_tls_key_base64": "",
	"oidc_enabled": true,
	"oidc_issuer_url": "https://idp.example.com",
	"oidc_client_id": "cid",
	"oidc_client_secret": "` + passwordMask + `",
	"oidc_scopes": "openid email profile",
	"oidc_button_label": "Sign in with Acme",
	"oidc_auto_create_users": false,
	"oidc_allowed_domains": ""
}`

const storedOIDCRedirectURI = "https://sso.acme.example/api/user.oidc.callback"

// seedOIDCSettings installs a working OIDC configuration whose callback is a vanity URL
// that api_endpoint cannot derive — exactly the deployment the omitted key breaks.
func seedOIDCSettings(t *testing.T, settingRepo *mockSettingRepository) {
	t.Helper()
	ctx := context.Background()

	encryptedSecret, err := crypto.EncryptString("real-oidc-secret", testSecretKey)
	require.NoError(t, err)

	_ = settingRepo.Set(ctx, "is_installed", "true")
	_ = settingRepo.Set(ctx, "root_email", testRootEmail)
	_ = settingRepo.Set(ctx, "api_endpoint", "https://app.example.com")
	_ = settingRepo.Set(ctx, "oidc_enabled", "true")
	_ = settingRepo.Set(ctx, "oidc_issuer_url", "https://idp.example.com")
	_ = settingRepo.Set(ctx, "oidc_client_id", "cid")
	_ = settingRepo.Set(ctx, "encrypted_oidc_client_secret", encryptedSecret)
	_ = settingRepo.Set(ctx, "oidc_redirect_uri", storedOIDCRedirectURI)
	_ = settingRepo.Set(ctx, "oidc_scopes", "openid email profile")
	_ = settingRepo.Set(ctx, "oidc_button_label", "Sign in")
}

// postSettingsUpdate posts a raw settings.update body as the root user.
func postSettingsUpdate(t *testing.T, handler *SettingsHandler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/settings.update", bytes.NewBufferString(body))
	req = reqWithUserContext(req, "root-user-id")
	w := httptest.NewRecorder()
	handler.handleUpdate(w, req)
	return w
}

// TestSettingsHandler_Update_OmittedFieldsKeepStoredValues covers the redirect URI the
// console never sends. No <Form.Item name="oidc_redirect_uri"> exists in the console, so
// the key is absent from every save; decoded into a non-pointer string it arrives as "",
// which the handler cannot tell apart from an operator clearing the field. The stored
// callback is blanked on the first save of any unrelated setting, and SSO breaks wherever
// the callback is not derivable from api_endpoint — a reverse proxy or a vanity SSO domain.
func TestSettingsHandler_Update_OmittedFieldsKeepStoredValues(t *testing.T) {
	t.Run("a save that omits oidc_redirect_uri keeps the stored one", func(t *testing.T) {
		handler, settingRepo, _, _ := setupSettingsHandler(t)
		seedOIDCSettings(t, settingRepo)

		w := postSettingsUpdate(t, handler, consoleSettingsSaveBody)
		require.Equal(t, http.StatusOK, w.Code)

		assert.Equal(t, storedOIDCRedirectURI, settingRepo.settings["oidc_redirect_uri"],
			"a key the body never carried must leave the stored value alone")
		// The save itself must still have happened, or the assertion above proves nothing.
		assert.Equal(t, "Sign in with Acme", settingRepo.settings["oidc_button_label"])
	})

	// The same decode protects every other field, which matters for the API clients the
	// endpoint also serves: a body naming one setting used to blank the twenty-five it did
	// not name, SSO included.
	t.Run("a body naming one setting leaves the rest of the OIDC config alone", func(t *testing.T) {
		handler, settingRepo, _, _ := setupSettingsHandler(t)
		seedOIDCSettings(t, settingRepo)

		w := postSettingsUpdate(t, handler, `{"api_endpoint": "https://new.example.com"}`)
		require.Equal(t, http.StatusOK, w.Code)

		assert.Equal(t, "https://new.example.com", settingRepo.settings["api_endpoint"])
		assert.Equal(t, "true", settingRepo.settings["oidc_enabled"])
		assert.Equal(t, "https://idp.example.com", settingRepo.settings["oidc_issuer_url"])
		assert.Equal(t, "cid", settingRepo.settings["oidc_client_id"])
		assert.Equal(t, storedOIDCRedirectURI, settingRepo.settings["oidc_redirect_uri"])

		// Decrypted, not compared as ciphertext: AES-GCM re-encrypts with a fresh nonce, so
		// the stored bytes change even when the secret is kept.
		secret, err := crypto.DecryptFromHexString(settingRepo.settings["encrypted_oidc_client_secret"], testSecretKey)
		require.NoError(t, err)
		assert.Equal(t, "real-oidc-secret", secret)
	})

	t.Run("an explicitly empty oidc_redirect_uri still clears it", func(t *testing.T) {
		handler, settingRepo, _, _ := setupSettingsHandler(t)
		seedOIDCSettings(t, settingRepo)

		body := strings.Replace(consoleSettingsSaveBody,
			`"oidc_button_label": "Sign in with Acme",`,
			`"oidc_button_label": "Sign in with Acme",`+"\n\t"+`"oidc_redirect_uri": "",`, 1)
		require.Contains(t, body, `"oidc_redirect_uri": ""`)

		w := postSettingsUpdate(t, handler, body)
		require.Equal(t, http.StatusOK, w.Code)

		assert.Equal(t, "", settingRepo.settings["oidc_redirect_uri"],
			"clearing the field must stay expressible: the key is present, carrying an empty string")
	})
}

// ============================================================
// Tests for GET /api/licence.get and POST /api/licence.set
// ============================================================

// mockLicenseService is a hand-written LicenseServiceInterface, matching the mocks the rest
// of this file uses. It exists because a real service.LicenseService cannot be driven into a
// licensed state from a test: no valid key can be minted without the private half of the
// signing key, which this build deliberately does not carry. The real service is still used
// wherever the assertion is about what reaches the settings table.
type mockLicenseService struct {
	entitlements domain.Entitlements
	setErr       error
	// keysSubmitted records every key SetKey was handed, so a test can assert the guards
	// refuse before the service is ever consulted.
	keysSubmitted []string
}

func newMockLicenseService() *mockLicenseService {
	return &mockLicenseService{entitlements: domain.CommunityEntitlements()}
}

func (m *mockLicenseService) Entitlements() domain.Entitlements { return m.entitlements }

func (m *mockLicenseService) SetKey(ctx context.Context, raw string) error {
	m.keysSubmitted = append(m.keysSubmitted, raw)
	return m.setErr
}

// licensedEntitlements is an Agency key in its grace period: every field the console renders
// is populated and none of them is a zero value, so a field dropped from the response body
// fails the assertion rather than matching an empty one.
func licensedEntitlements() domain.Entitlements {
	return domain.Entitlements{
		Tier:          "agency",
		Org:           "ACME SAS",
		Sub:           "billing@acme.com",
		MaxWorkspaces: 15,
		Features:      []domain.Feature{domain.FeatureRBAC, domain.FeatureSESTenant},
		State:         domain.LicenseStateGrace,
		ExpiresAt:     time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC),
	}
}

// setupSettingsHandlerWithMockLicense swaps the harness's real licence service for one a test
// can drive. The field is unexported and these tests are in-package, which is the same reason
// they call the handler funcs directly.
func setupSettingsHandlerWithMockLicense(t *testing.T) (*SettingsHandler, *mockSettingRepository, *mockLicenseService) {
	t.Helper()

	handler, settingRepo, _, _ := setupSettingsHandler(t)
	licenseSvc := newMockLicenseService()
	handler.licenseService = licenseSvc

	return handler, settingRepo, licenseSvc
}

func getLicense(t *testing.T, handler *SettingsHandler, userID string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/licence.get", nil)
	req = reqWithUserContext(req, userID)
	w := httptest.NewRecorder()

	handler.handleLicenseGet(w, req)

	return w
}

func postLicense(t *testing.T, handler *SettingsHandler, userID string, body string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/api/licence.set", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = reqWithUserContext(req, userID)
	w := httptest.NewRecorder()

	handler.handleLicenseSet(w, req)

	return w
}

// The guards every root-only endpoint on this handler repeats. They are the whole reason
// the licence endpoints live here rather than in a file of their own.
func TestSettingsHandler_License_Guards(t *testing.T) {
	t.Run("licence.get refuses a method other than GET", func(t *testing.T) {
		handler, _, _, _ := setupSettingsHandler(t)

		req := httptest.NewRequest(http.MethodPost, "/api/licence.get", nil)
		req = reqWithUserContext(req, "root-user-id")
		w := httptest.NewRecorder()

		handler.handleLicenseGet(w, req)

		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("licence.set refuses a method other than POST", func(t *testing.T) {
		handler, _, _, _ := setupSettingsHandler(t)

		req := httptest.NewRequest(http.MethodGet, "/api/licence.set", nil)
		req = reqWithUserContext(req, "root-user-id")
		w := httptest.NewRecorder()

		handler.handleLicenseSet(w, req)

		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("licence.get refuses a request carrying no user", func(t *testing.T) {
		handler, _, _, _ := setupSettingsHandler(t)

		req := httptest.NewRequest(http.MethodGet, "/api/licence.get", nil)
		w := httptest.NewRecorder()

		handler.handleLicenseGet(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("licence.set refuses a request carrying no user", func(t *testing.T) {
		handler, _, license := setupSettingsHandlerWithMockLicense(t)

		req := httptest.NewRequest(http.MethodPost, "/api/licence.set", bytes.NewBufferString(`{"key":"NFUSE1.a.b"}`))
		w := httptest.NewRecorder()

		handler.handleLicenseSet(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.Empty(t, license.keysSubmitted, "the guard must run before the service is consulted")
	})

	t.Run("licence.get refuses a signed-in user who is not root", func(t *testing.T) {
		handler, _, _, _ := setupSettingsHandler(t)

		w := getLicense(t, handler, "other-user-id")

		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	// Only root can install a key. A member who could paste one could also replace a valid
	// licence with a key of their own choosing, and the org and billing address on the
	// current one are not theirs to read either.
	t.Run("licence.set refuses a signed-in user who is not root", func(t *testing.T) {
		handler, _, license := setupSettingsHandlerWithMockLicense(t)

		w := postLicense(t, handler, "other-user-id", `{"key":"NFUSE1.a.b"}`)

		assert.Equal(t, http.StatusForbidden, w.Code)
		assert.Empty(t, license.keysSubmitted, "a non-root paste must never reach the service")
	})
}

// The shape the console reads: the resolved grant, whole. Nothing else — the response is the
// same domain.Entitlements every gate consults, so the console cannot form a view the backend
// does not share.
func TestSettingsHandler_LicenseGet_ReportsTheResolvedLicence(t *testing.T) {
	handler, _, license := setupSettingsHandlerWithMockLicense(t)
	license.entitlements = licensedEntitlements()

	w := getLicense(t, handler, "root-user-id")
	require.Equal(t, http.StatusOK, w.Code)

	var resp LicenseResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))

	assert.Equal(t, licensedEntitlements(), resp.Entitlements)
}

// The JSON key names are a contract with the console, so they are pinned as strings rather
// than round-tripped through the struct that defines them.
func TestSettingsHandler_LicenseGet_BodyKeys(t *testing.T) {
	handler, _, license := setupSettingsHandlerWithMockLicense(t)
	license.entitlements = licensedEntitlements()

	w := getLicense(t, handler, "root-user-id")
	require.Equal(t, http.StatusOK, w.Code)

	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))

	require.Contains(t, body, "entitlements")

	entitlements, ok := body["entitlements"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "agency", entitlements["tier"])
	assert.Equal(t, "ACME SAS", entitlements["org"])
	assert.Equal(t, "billing@acme.com", entitlements["sub"])
	assert.Equal(t, string(domain.LicenseStateGrace), entitlements["state"])
	assert.Equal(t, float64(15), entitlements["max_workspaces"])
	assert.Equal(t, []interface{}{"rbac", "ses_tenant"}, entitlements["features"])
	assert.Equal(t, "2026-03-01T12:00:00Z", entitlements["expires_at"])
}

// A key is a bearer credential: whoever holds it can license their own deployment with it.
// An endpoint that echoed it back would copy it into every console session, browser cache and
// support screenshot, so the response is asserted against the stored value itself rather than
// against the absence of a field somebody could rename.
func TestSettingsHandler_LicenseGet_NeverReturnsTheRawKey(t *testing.T) {
	handler, settingRepo, _, _ := setupSettingsHandler(t)

	const storedKey = "NFUSE1.eyJ2IjoxfQ.c2lnbmF0dXJl"
	settingRepo.settings[service.LicenseSettingKey] = storedKey

	w := getLicense(t, handler, "root-user-id")
	require.Equal(t, http.StatusOK, w.Code)

	assert.NotContains(t, w.Body.String(), storedKey)
	assert.NotContains(t, w.Body.String(), "NFUSE1")
}

// An unwired licence service is a Community deployment, not a broken one. The settings page
// is the page an operator opens because something is already wrong; it must not be the second
// thing that fails.
func TestSettingsHandler_LicenseGet_WithoutALicenceService(t *testing.T) {
	handler, _, _, _ := setupSettingsHandler(t)
	handler.licenseService = nil

	w := getLicense(t, handler, "root-user-id")
	require.Equal(t, http.StatusOK, w.Code)

	var resp LicenseResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))

	assert.Equal(t, domain.CommunityEntitlements(), resp.Entitlements)
}

// A deployment with no key at all answers Community: max_workspaces 3, no features, state
// none. This one runs against the real service, so it also proves the harness wires it.
func TestSettingsHandler_LicenseGet_UnlicensedDeployment(t *testing.T) {
	handler, _, _, _ := setupSettingsHandler(t)

	w := getLicense(t, handler, "root-user-id")
	require.Equal(t, http.StatusOK, w.Code)

	var resp LicenseResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))

	assert.Equal(t, domain.LicenseStateNone, resp.Entitlements.State)
	assert.Equal(t, domain.CommunityMaxWorkspaces, resp.Entitlements.MaxWorkspaces)
	assert.Empty(t, resp.Entitlements.Features)
}

// The refusal that matters most: a bad paste must cost the deployment nothing. The service
// verifies before it writes, and this pins that the endpoint keeps that ordering — an
// operator fixing a typo must not discover that the first attempt already erased the licence
// they were running on.
func TestSettingsHandler_LicenseSet_MalformedKeyKeepsTheStoredOne(t *testing.T) {
	handler, settingRepo, _, _ := setupSettingsHandler(t)

	const storedKey = "NFUSE1.eyJ2IjoxfQ.c2lnbmF0dXJl"
	settingRepo.settings[service.LicenseSettingKey] = storedKey

	w := postLicense(t, handler, "root-user-id", `{"key":"this is not a licence key"}`)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, storedKey, settingRepo.settings[service.LicenseSettingKey],
		"a rejected paste must leave the stored key exactly as it was")

	var body map[string]string
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	// The sentinel's own sentence, so an operator can tell a truncated paste apart from a
	// key minted by a different signing authority.
	assert.Equal(t, license.ErrMalformedEnvelope.Error(), body["error"])
}

// Each refusal has a different remedy — fix the paste, edit the environment, call support —
// and an operator can only tell them apart if the statuses differ.
func TestSettingsHandler_LicenseSet_ErrorMapping(t *testing.T) {
	testCases := []struct {
		name            string
		setErr          error
		expectedStatus  int
		expectedMessage string
	}{
		{
			name:            "an empty key is the operator's mistake",
			setErr:          service.ErrLicenseKeyEmpty,
			expectedStatus:  http.StatusBadRequest,
			expectedMessage: "Licence key is required",
		},
		{
			// 409, not 400: the pasted key may be perfectly valid and simply cannot
			// win. Answering 400 would send an operator hunting for a typo that is
			// not there.
			name:            "a key locked by the environment conflicts with the deployment",
			setErr:          service.ErrLicenseKeyLockedByEnv,
			expectedStatus:  http.StatusConflict,
			expectedMessage: service.ErrLicenseKeyLockedByEnv.Error(),
		},
		{
			name:            "a signature that does not verify is the operator's mistake",
			setErr:          fmt.Errorf("invalid licence key: %w", license.ErrBadSignature),
			expectedStatus:  http.StatusBadRequest,
			expectedMessage: license.ErrBadSignature.Error(),
		},
		{
			name:            "a schema this build does not implement is the operator's mistake",
			setErr:          fmt.Errorf("invalid licence key: %w", license.ErrUnknownVersion),
			expectedStatus:  http.StatusBadRequest,
			expectedMessage: license.ErrUnknownVersion.Error(),
		},
		{
			// A build still carrying the placeholder public key rejects every key
			// ever minted. Naming it is the only way an operator finds that out.
			name:            "a binary with no signing key says so",
			setErr:          fmt.Errorf("invalid licence key: %w", license.ErrNoTrustedKey),
			expectedStatus:  http.StatusBadRequest,
			expectedMessage: license.ErrNoTrustedKey.Error(),
		},
		{
			// The settings table refusing the write. The key was already verified, so
			// this is the server's failure and not the operator's, and the internal
			// wording stays inside the process.
			name:            "a storage failure is the server's",
			setErr:          fmt.Errorf("failed to set %s: %w", service.LicenseSettingKey, errors.New("connection refused")),
			expectedStatus:  http.StatusInternalServerError,
			expectedMessage: "Failed to save licence key",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			handler, _, licenseSvc := setupSettingsHandlerWithMockLicense(t)
			licenseSvc.setErr = tc.setErr

			w := postLicense(t, handler, "root-user-id", `{"key":"NFUSE1.a.b"}`)

			assert.Equal(t, tc.expectedStatus, w.Code)

			var body map[string]string
			require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
			assert.Equal(t, tc.expectedMessage, body["error"])
		})
	}
}

// The service refuses rather than storing a key that would silently lose to the environment
// at the next restart. Run against the real service so the precedence is the real one.
func TestSettingsHandler_LicenseSet_LockedByEnvironment(t *testing.T) {
	handler, settingRepo, _, _ := setupSettingsHandler(t)
	handler.licenseService = service.NewLicenseService(service.LicenseServiceConfig{
		SettingRepo: settingRepo,
		EnvKey:      "NFUSE1.eyJ2IjoxfQ.c2lnbmF0dXJl",
		Logger:      logger.NewLogger(),
	})

	w := postLicense(t, handler, "root-user-id", `{"key":"NFUSE1.other.key"}`)

	assert.Equal(t, http.StatusConflict, w.Code)
	assert.NotContains(t, settingRepo.settings, service.LicenseSettingKey,
		"a key that could not take effect must not be stored")
}

func TestSettingsHandler_LicenseSet_EmptyKey(t *testing.T) {
	handler, settingRepo, _, _ := setupSettingsHandler(t)

	w := postLicense(t, handler, "root-user-id", `{"key":"   "}`)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.NotContains(t, settingRepo.settings, service.LicenseSettingKey)
}

func TestSettingsHandler_LicenseSet_InvalidBody(t *testing.T) {
	handler, _, licenseSvc := setupSettingsHandlerWithMockLicense(t)

	w := postLicense(t, handler, "root-user-id", `{"key":`)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Empty(t, licenseSvc.keysSubmitted)
}

// A successful install answers with the new state, so the console repaints its banner from
// the same round trip rather than from a follow-up read that could race the swap.
func TestSettingsHandler_LicenseSet_Success(t *testing.T) {
	handler, _, licenseSvc := setupSettingsHandlerWithMockLicense(t)

	w := postLicense(t, handler, "root-user-id", `{"key":"  NFUSE1.a.b  "}`)
	require.Equal(t, http.StatusOK, w.Code)

	// Passed through untouched: trimming, and every other judgement about the envelope,
	// belongs to the code that verifies the signature.
	require.Equal(t, []string{"  NFUSE1.a.b  "}, licenseSvc.keysSubmitted)

	// The state the service reports after the swap, not the state it had before it.
	licenseSvc.entitlements = licensedEntitlements()
	w = postLicense(t, handler, "root-user-id", `{"key":"NFUSE1.a.b"}`)
	require.Equal(t, http.StatusOK, w.Code)

	var resp LicenseResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, licensedEntitlements(), resp.Entitlements)
}

// Both routes are registered, and both under the same auth middleware as the rest of this
// handler. Everything a deployment buys is bought by pasting a key into /api/licence.set, so
// a route that failed to register would leave a paying customer unable to install what they
// paid for.
func TestSettingsHandler_RegistersTheLicenceRoutes(t *testing.T) {
	handler, _, _, _ := setupSettingsHandler(t)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	for _, route := range []string{"/api/licence.get", "/api/licence.set"} {
		t.Run(route, func(t *testing.T) {
			_, pattern := mux.Handler(httptest.NewRequest(http.MethodGet, route, nil))
			assert.Equal(t, route, pattern)
		})
	}
}
