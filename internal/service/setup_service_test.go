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
