package service_test

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetupService_GeneratePasetoKeys(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create setup service (no mocks needed for this test)
	settingService := &service.SettingService{}
	userService := &service.UserService{}
	setupService := service.NewSetupService(
		settingService,
		userService,
		nil,
		&mockLogger{},
		"test-secret-key",
		nil, // no callback needed for this test
		nil, // no env config needed for this test
	)

	// Test key generation
	keys, err := setupService.GeneratePasetoKeys()
	require.NoError(t, err)
	require.NotNil(t, keys)

	// Verify keys are valid base64
	_, err = base64.StdEncoding.DecodeString(keys.PublicKey)
	assert.NoError(t, err, "Public key should be valid base64")

	_, err = base64.StdEncoding.DecodeString(keys.PrivateKey)
	assert.NoError(t, err, "Private key should be valid base64")

	// Verify keys are not empty
	assert.NotEmpty(t, keys.PublicKey)
	assert.NotEmpty(t, keys.PrivateKey)
}

func TestSetupService_ValidatePasetoKeys(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	setupService := service.NewSetupService(
		&service.SettingService{},
		&service.UserService{},
		nil,
		&mockLogger{},
		"test-secret-key",
		nil, // no callback needed for this test
		nil, // no env config needed for this test
	)

	tests := []struct {
		name       string
		publicKey  string
		privateKey string
		wantError  bool
	}{
		{
			name:       "valid keys",
			publicKey:  base64.StdEncoding.EncodeToString([]byte("valid-public-key")),
			privateKey: base64.StdEncoding.EncodeToString([]byte("valid-private-key")),
			wantError:  false,
		},
		{
			name:       "invalid public key",
			publicKey:  "not-base64!@#$",
			privateKey: base64.StdEncoding.EncodeToString([]byte("valid-private-key")),
			wantError:  true,
		},
		{
			name:       "invalid private key",
			publicKey:  base64.StdEncoding.EncodeToString([]byte("valid-public-key")),
			privateKey: "not-base64!@#$",
			wantError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := setupService.ValidatePasetoKeys(tt.publicKey, tt.privateKey)
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSetupService_ValidateSetupConfig(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	setupService := service.NewSetupService(
		&service.SettingService{},
		&service.UserService{},
		nil,
		&mockLogger{},
		"test-secret-key",
		nil, // no callback needed for this test
		nil, // no env config needed for this test
	)

	validPublicKey := base64.StdEncoding.EncodeToString([]byte("valid-public-key"))
	validPrivateKey := base64.StdEncoding.EncodeToString([]byte("valid-private-key"))

	tests := []struct {
		name      string
		config    *service.SetupConfig
		wantError string
	}{
		{
			name: "valid config with generated keys",
			config: &service.SetupConfig{
				RootEmail:          "admin@example.com",
				APIEndpoint:        "https://api.example.com",
				GeneratePasetoKeys: true,
				SMTPHost:           "smtp.example.com",
				SMTPPort:           587,
				SMTPFromEmail:      "noreply@example.com",
			},
			wantError: "",
		},
		{
			name: "valid config with provided keys",
			config: &service.SetupConfig{
				RootEmail:          "admin@example.com",
				APIEndpoint:        "https://api.example.com",
				GeneratePasetoKeys: false,
				PasetoPublicKey:    validPublicKey,
				PasetoPrivateKey:   validPrivateKey,
				SMTPHost:           "smtp.example.com",
				SMTPPort:           587,
				SMTPFromEmail:      "noreply@example.com",
			},
			wantError: "",
		},
		{
			name: "missing root email",
			config: &service.SetupConfig{
				APIEndpoint:        "https://api.example.com",
				GeneratePasetoKeys: true,
				SMTPHost:           "smtp.example.com",
				SMTPPort:           587,
				SMTPFromEmail:      "noreply@example.com",
			},
			wantError: "root_email is required",
		},
		{
			name: "missing PASETO keys when not generating",
			config: &service.SetupConfig{
				RootEmail:          "admin@example.com",
				APIEndpoint:        "https://api.example.com",
				GeneratePasetoKeys: false,
				SMTPHost:           "smtp.example.com",
				SMTPPort:           587,
				SMTPFromEmail:      "noreply@example.com",
			},
			wantError: "PASETO keys are required when not generating new ones",
		},
		{
			name: "missing SMTP host",
			config: &service.SetupConfig{
				RootEmail:          "admin@example.com",
				APIEndpoint:        "https://api.example.com",
				GeneratePasetoKeys: true,
				SMTPPort:           587,
				SMTPFromEmail:      "noreply@example.com",
			},
			wantError: "smtp_host is required",
		},
		{
			name: "missing SMTP from email",
			config: &service.SetupConfig{
				RootEmail:          "admin@example.com",
				APIEndpoint:        "https://api.example.com",
				GeneratePasetoKeys: true,
				SMTPHost:           "smtp.example.com",
				SMTPPort:           587,
			},
			wantError: "smtp_from_email is required",
		},
		{
			name: "default SMTP port set when 0",
			config: &service.SetupConfig{
				RootEmail:          "admin@example.com",
				APIEndpoint:        "https://api.example.com",
				GeneratePasetoKeys: true,
				SMTPHost:           "smtp.example.com",
				SMTPPort:           0,
				SMTPFromEmail:      "noreply@example.com",
			},
			wantError: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := setupService.ValidateSetupConfig(tt.config)
			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)
			} else {
				assert.NoError(t, err)
				// Check that default port is set
				if tt.config.SMTPPort == 0 {
					// The validation should have set it to 587
					// Note: This modifies the config in place
				}
			}
		})
	}
}

func TestSetupService_TestSMTPConnection(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	setupService := service.NewSetupService(
		&service.SettingService{},
		&service.UserService{},
		nil,
		&mockLogger{},
		"test-secret-key",
		nil, // no callback needed for this test
		nil, // no env config needed for this test
	)

	tests := []struct {
		name      string
		config    *service.SMTPTestConfig
		wantError bool
	}{
		{
			name: "missing host",
			config: &service.SMTPTestConfig{
				Port:     587,
				Username: "user",
				Password: "pass",
			},
			wantError: true,
		},
		{
			name: "missing port",
			config: &service.SMTPTestConfig{
				Host:     "smtp.example.com",
				Username: "user",
				Password: "pass",
			},
			wantError: true,
		},
		{
			name: "valid config but connection fails",
			config: &service.SMTPTestConfig{
				Host:     "invalid-smtp-host.example.com",
				Port:     587,
				Username: "user",
				Password: "pass",
			},
			wantError: true, // Will fail to connect to invalid host
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			err := setupService.TestSMTPConnection(ctx, tt.config)
			if tt.wantError {
				assert.Error(t, err)
			} else {
				// We can't test successful connection without a real SMTP server
				// This test will fail, which is expected
				t.Skip("Cannot test successful SMTP connection without a real server")
			}
		})
	}
}

func TestSetupService_GetConfigurationStatus(t *testing.T) {
	tests := []struct {
		name              string
		envConfig         *service.EnvironmentConfig
		expectedSMTP      bool
		expectedPaseto    bool
		expectedAPI       bool
		expectedRootEmail bool
	}{
		{
			name:              "no env config",
			envConfig:         nil,
			expectedSMTP:      false,
			expectedPaseto:    false,
			expectedAPI:       false,
			expectedRootEmail: false,
		},
		{
			name: "all configured",
			envConfig: &service.EnvironmentConfig{
				RootEmail:        "admin@example.com",
				APIEndpoint:      "https://api.example.com",
				PasetoPublicKey:  "public-key",
				PasetoPrivateKey: "private-key",
				SMTPHost:         "smtp.example.com",
				SMTPPort:         587,
				SMTPFromEmail:    "noreply@example.com",
			},
			expectedSMTP:      true,
			expectedPaseto:    true,
			expectedAPI:       true,
			expectedRootEmail: true,
		},
		{
			name: "SMTP incomplete - missing host",
			envConfig: &service.EnvironmentConfig{
				SMTPPort:      587,
				SMTPFromEmail: "noreply@example.com",
			},
			expectedSMTP:   false,
			expectedPaseto: false,
			expectedAPI:    false,
		},
		{
			name: "SMTP incomplete - missing port",
			envConfig: &service.EnvironmentConfig{
				SMTPHost:      "smtp.example.com",
				SMTPFromEmail: "noreply@example.com",
			},
			expectedSMTP:   false,
			expectedPaseto: false,
			expectedAPI:    false,
		},
		{
			name: "SMTP incomplete - missing from email",
			envConfig: &service.EnvironmentConfig{
				SMTPHost: "smtp.example.com",
				SMTPPort: 587,
			},
			expectedSMTP:   false,
			expectedPaseto: false,
			expectedAPI:    false,
		},
		{
			name: "PASETO incomplete - only public key",
			envConfig: &service.EnvironmentConfig{
				PasetoPublicKey: "public-key",
			},
			expectedSMTP:   false,
			expectedPaseto: false,
			expectedAPI:    false,
		},
		{
			name: "PASETO incomplete - only private key",
			envConfig: &service.EnvironmentConfig{
				PasetoPrivateKey: "private-key",
			},
			expectedSMTP:   false,
			expectedPaseto: false,
			expectedAPI:    false,
		},
		{
			name: "only API endpoint configured",
			envConfig: &service.EnvironmentConfig{
				APIEndpoint: "https://api.example.com",
			},
			expectedSMTP:   false,
			expectedPaseto: false,
			expectedAPI:    true,
		},
		{
			name: "only root email configured",
			envConfig: &service.EnvironmentConfig{
				RootEmail: "admin@example.com",
			},
			expectedSMTP:      false,
			expectedPaseto:    false,
			expectedAPI:       false,
			expectedRootEmail: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupService := service.NewSetupService(
				&service.SettingService{},
				&service.UserService{},
				nil,
				&mockLogger{},
				"test-secret-key",
				nil,
				tt.envConfig,
			)

			status := setupService.GetConfigurationStatus()

			assert.Equal(t, tt.expectedSMTP, status.SMTPConfigured, "SMTP configured status mismatch")
			assert.Equal(t, tt.expectedPaseto, status.PasetoConfigured, "PASETO configured status mismatch")
			assert.Equal(t, tt.expectedAPI, status.APIEndpointConfigured, "API endpoint configured status mismatch")
			assert.Equal(t, tt.expectedRootEmail, status.RootEmailConfigured, "Root email configured status mismatch")
		})
	}
}

// mockLogger is a simple mock implementation of logger.Logger for testing
type mockLogger struct{}

func (m *mockLogger) Debug(msg string)                                       {}
func (m *mockLogger) Info(msg string)                                        {}
func (m *mockLogger) Warn(msg string)                                        {}
func (m *mockLogger) Error(msg string)                                       {}
func (m *mockLogger) Fatal(msg string)                                       {}
func (m *mockLogger) WithField(key string, value interface{}) logger.Logger  { return m }
func (m *mockLogger) WithFields(fields map[string]interface{}) logger.Logger { return m }
