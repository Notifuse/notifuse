package service_test

import (
	"context"
	"testing"

	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/stretchr/testify/assert"
)

func TestSetupService_ValidateSetupConfig(t *testing.T) {

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
		config    *service.SetupConfig
		wantError string
	}{
		{
			name: "valid config with TLS enabled",
			config: &service.SetupConfig{
				RootEmail:     "admin@example.com",
				APIEndpoint:   "https://api.example.com",
				SMTPHost:      "smtp.example.com",
				SMTPPort:      587,
				SMTPFromEmail: "noreply@example.com",
				SMTPUseTLS:    true,
			},
			wantError: "",
		},
		{
			name: "valid config with TLS disabled",
			config: &service.SetupConfig{
				RootEmail:     "admin@example.com",
				APIEndpoint:   "https://api.example.com",
				SMTPHost:      "smtp.example.com",
				SMTPPort:      25,
				SMTPFromEmail: "noreply@example.com",
				SMTPUseTLS:    false,
			},
			wantError: "",
		},
		{
			name: "missing root email",
			config: &service.SetupConfig{
				APIEndpoint:   "https://api.example.com",
				SMTPHost:      "smtp.example.com",
				SMTPPort:      587,
				SMTPFromEmail: "noreply@example.com",
			},
			wantError: "root_email is required",
		},
		{
			name: "missing SMTP host",
			config: &service.SetupConfig{
				RootEmail:     "admin@example.com",
				APIEndpoint:   "https://api.example.com",
				SMTPPort:      587,
				SMTPFromEmail: "noreply@example.com",
			},
			wantError: "smtp_host is required",
		},
		{
			name: "missing SMTP from email",
			config: &service.SetupConfig{
				RootEmail:   "admin@example.com",
				APIEndpoint: "https://api.example.com",
				SMTPHost:    "smtp.example.com",
				SMTPPort:    587,
			},
			wantError: "smtp_from_email is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := setupService.ValidateSetupConfig(tt.config)
			if tt.wantError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Mock logger for testing
type mockLogger struct{}

func (m *mockLogger) Debug(msg string)                                       {}
func (m *mockLogger) Info(msg string)                                        {}
func (m *mockLogger) Warn(msg string)                                        {}
func (m *mockLogger) Error(msg string)                                       {}
func (m *mockLogger) Fatal(msg string)                                       {}
func (m *mockLogger) Panic(msg string)                                       {}
func (m *mockLogger) WithField(key string, value interface{}) logger.Logger  { return m }
func (m *mockLogger) WithFields(fields map[string]interface{}) logger.Logger { return m }
func (m *mockLogger) WithError(err error) logger.Logger                      { return m }

func TestSetupService_Initialize(t *testing.T) {
	// Test SetupService.Initialize - this was at 0% coverage
	// Note: This is a complex method that requires proper mocks for SettingService and UserRepository
	// For basic coverage, we test validation error path
	setupService := service.NewSetupService(
		&service.SettingService{},
		&service.UserService{},
		nil,
		&mockLogger{},
		"test-secret-key",
		nil,
		nil,
	)

	ctx := context.Background()

	t.Run("Error - Validation fails", func(t *testing.T) {
		config := &service.SetupConfig{
			// Missing required fields
		}

		err := setupService.Initialize(ctx, config)
		assert.Error(t, err)
	})
}

func TestSetupService_TestSMTPConnection(t *testing.T) {
	// Test SetupService.TestSMTPConnection - this was at 0% coverage
	setupService := service.NewSetupService(
		&service.SettingService{},
		&service.UserService{},
		nil,
		&mockLogger{},
		"test-secret-key",
		nil,
		nil,
	)

	ctx := context.Background()

	t.Run("Error - Missing host", func(t *testing.T) {
		config := &service.SMTPTestConfig{
			Port: 587,
		}

		err := setupService.TestSMTPConnection(ctx, config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "SMTP host is required")
	})

	t.Run("Error - Missing port", func(t *testing.T) {
		config := &service.SMTPTestConfig{
			Host: "smtp.example.com",
		}

		err := setupService.TestSMTPConnection(ctx, config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "SMTP port is required")
	})

	t.Run("Error - Connection fails with TLS enabled", func(t *testing.T) {
		config := &service.SMTPTestConfig{
			Host:     "invalid-host.example.com",
			Port:     587,
			Username: "user",
			Password: "pass",
			UseTLS:   true,
		}

		err := setupService.TestSMTPConnection(ctx, config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to connect to SMTP server")
	})

	t.Run("Error - Connection fails with TLS disabled", func(t *testing.T) {
		config := &service.SMTPTestConfig{
			Host:     "invalid-host.example.com",
			Port:     25,
			Username: "user",
			Password: "pass",
			UseTLS:   false,
		}

		err := setupService.TestSMTPConnection(ctx, config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to connect to SMTP server")
	})

	// Note: Actual SMTP connection test would require a real SMTP server or more complex mocking
	// For coverage purposes, we test the validation logic
}

func TestSetupService_GetEnvOverrides(t *testing.T) {
	tests := []struct {
		name      string
		envConfig *service.EnvironmentConfig
		expected  map[string]bool
	}{
		{
			name:      "nil env config returns all false",
			envConfig: nil,
			expected:  map[string]bool{},
		},
		{
			name:      "empty env config returns all false",
			envConfig: &service.EnvironmentConfig{},
			expected:  map[string]bool{},
		},
		{
			name: "only root_email set",
			envConfig: &service.EnvironmentConfig{
				RootEmail: "admin@example.com",
			},
			expected: map[string]bool{
				"root_email": true,
			},
		},
		{
			name: "SMTP fields set",
			envConfig: &service.EnvironmentConfig{
				SMTPHost:     "smtp.example.com",
				SMTPPort:     587,
				SMTPUsername: "user",
				SMTPPassword: "pass",
			},
			expected: map[string]bool{
				"smtp_host":     true,
				"smtp_port":     true,
				"smtp_username": true,
				"smtp_password": true,
			},
		},
		{
			name: "all fields set",
			envConfig: &service.EnvironmentConfig{
				RootEmail:               "admin@example.com",
				APIEndpoint:             "https://api.example.com",
				SMTPHost:                "smtp.example.com",
				SMTPPort:                587,
				SMTPUsername:            "user",
				SMTPPassword:            "pass",
				SMTPFromEmail:           "noreply@example.com",
				SMTPFromName:            "Test",
				SMTPUseTLS:              "true",
				SMTPEHLOHostname:        "mail.example.com",
				SMTPBridgeEnabled:       "true",
				SMTPBridgeDomain:        "bridge.example.com",
				SMTPBridgePort:          587,
				SMTPBridgeTLSCertBase64: "cert-data",
				SMTPBridgeTLSKeyBase64:  "key-data",
				SMTPBridgeTLSMode:       "starttls",
			},
			expected: map[string]bool{
				"root_email":                  true,
				"api_endpoint":                true,
				"smtp_host":                   true,
				"smtp_port":                   true,
				"smtp_username":               true,
				"smtp_password":               true,
				"smtp_from_email":             true,
				"smtp_from_name":              true,
				"smtp_use_tls":                true,
				"smtp_ehlo_hostname":          true,
				"smtp_bridge_enabled":         true,
				"smtp_bridge_domain":          true,
				"smtp_bridge_port":            true,
				"smtp_bridge_tls_cert_base64": true,
				"smtp_bridge_tls_key_base64":  true,
				"smtp_bridge_tls_mode":        true,
			},
		},
		{
			name: "smtp_bridge_tls_mode set alone",
			envConfig: &service.EnvironmentConfig{
				SMTPBridgeTLSMode: "off",
			},
			expected: map[string]bool{
				"smtp_bridge_tls_mode": true,
			},
		},
		{
			name: "smtp_use_tls set to false is still an override",
			envConfig: &service.EnvironmentConfig{
				SMTPUseTLS: "false",
			},
			expected: map[string]bool{
				"smtp_use_tls": true,
			},
		},
		{
			name: "smtp_bridge_enabled set to false is still an override",
			envConfig: &service.EnvironmentConfig{
				SMTPBridgeEnabled: "false",
			},
			expected: map[string]bool{
				"smtp_bridge_enabled": true,
			},
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

			result := setupService.GetEnvOverrides()

			// Check that expected overrides are present
			for key, val := range tt.expected {
				assert.Equal(t, val, result[key], "expected %s to be %v", key, val)
			}

			// Check that non-expected keys are NOT present (or false)
			allKeys := []string{
				"root_email", "api_endpoint",
				"smtp_host", "smtp_port", "smtp_username", "smtp_password",
				"smtp_from_email", "smtp_from_name", "smtp_use_tls", "smtp_ehlo_hostname",
				"smtp_bridge_enabled", "smtp_bridge_domain", "smtp_bridge_port",
				"smtp_bridge_tls_cert_base64", "smtp_bridge_tls_key_base64",
			}
			for _, key := range allKeys {
				if _, expected := tt.expected[key]; !expected {
					assert.False(t, result[key], "expected %s to be false/absent", key)
				}
			}
		})
	}
}
