package service_test

import (
	"testing"

	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

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

	tests := []struct {
		name      string
		config    *service.SetupConfig
		wantError string
	}{
		{
			name: "valid config",
			config: &service.SetupConfig{
				RootEmail:     "admin@example.com",
				APIEndpoint:   "https://api.example.com",
				SMTPHost:      "smtp.example.com",
				SMTPPort:      587,
				SMTPFromEmail: "noreply@example.com",
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
