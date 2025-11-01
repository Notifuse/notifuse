package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockSettingRepository is a mock implementation of domain.SettingRepository
type MockSettingRepository struct {
	settings map[string]string
	getError error
	setError error
}

func NewMockSettingRepository() *MockSettingRepository {
	return &MockSettingRepository{
		settings: make(map[string]string),
	}
}

func (m *MockSettingRepository) Get(ctx context.Context, key string) (*domain.Setting, error) {
	if m.getError != nil {
		return nil, m.getError
	}
	value, exists := m.settings[key]
	if !exists {
		return nil, &domain.ErrSettingNotFound{Key: key}
	}
	return &domain.Setting{Key: key, Value: value}, nil
}

func (m *MockSettingRepository) Set(ctx context.Context, key, value string) error {
	if m.setError != nil {
		return m.setError
	}
	m.settings[key] = value
	return nil
}

func (m *MockSettingRepository) Delete(ctx context.Context, key string) error {
	delete(m.settings, key)
	return nil
}

func (m *MockSettingRepository) List(ctx context.Context) ([]*domain.Setting, error) {
	settings := make([]*domain.Setting, 0, len(m.settings))
	for k, v := range m.settings {
		settings = append(settings, &domain.Setting{Key: k, Value: v})
	}
	return settings, nil
}

func (m *MockSettingRepository) GetLastCronRun(ctx context.Context) (*time.Time, error) {
	if m.getError != nil {
		return nil, m.getError
	}
	// Not used in setting_service tests, return nil
	return nil, nil
}

func (m *MockSettingRepository) SetLastCronRun(ctx context.Context) error {
	if m.setError != nil {
		return m.setError
	}
	// Not used in setting_service tests
	return nil
}

const testSecretKey = "test-secret-key-for-encryption-32b"

func TestSettingService_GetSystemConfig(t *testing.T) {
	ctx := context.Background()

	t.Run("not_installed", func(t *testing.T) {
		repo := NewMockSettingRepository()
		service := NewSettingService(repo)

		config, err := service.GetSystemConfig(ctx, testSecretKey)

		require.NoError(t, err)
		assert.False(t, config.IsInstalled)
		assert.Equal(t, 587, config.SMTPPort) // Default port
	})

	t.Run("installed_with_basic_settings", func(t *testing.T) {
		repo := NewMockSettingRepository()
		repo.settings["is_installed"] = "true"
		repo.settings["root_email"] = "admin@example.com"
		repo.settings["api_endpoint"] = "https://api.example.com"
		repo.settings["smtp_host"] = "smtp.example.com"
		repo.settings["smtp_port"] = "587"
		repo.settings["smtp_from_email"] = "noreply@example.com"
		repo.settings["smtp_from_name"] = "Test App"
		repo.settings["telemetry_enabled"] = "true"
		repo.settings["check_for_updates"] = "false"

		service := NewSettingService(repo)
		config, err := service.GetSystemConfig(ctx, testSecretKey)

		require.NoError(t, err)
		assert.True(t, config.IsInstalled)
		assert.Equal(t, "admin@example.com", config.RootEmail)
		assert.Equal(t, "https://api.example.com", config.APIEndpoint)
		assert.Equal(t, "smtp.example.com", config.SMTPHost)
		assert.Equal(t, 587, config.SMTPPort)
		assert.Equal(t, "noreply@example.com", config.SMTPFromEmail)
		assert.Equal(t, "Test App", config.SMTPFromName)
		assert.True(t, config.TelemetryEnabled)
		assert.False(t, config.CheckForUpdates)
	})

	t.Run("installed_with_encrypted_smtp_credentials", func(t *testing.T) {
		repo := NewMockSettingRepository()
		repo.settings["is_installed"] = "true"

		// Encrypt SMTP credentials
		encryptedUsername, err := crypto.EncryptString("smtp_user", testSecretKey)
		require.NoError(t, err)
		encryptedPassword, err := crypto.EncryptString("smtp_pass", testSecretKey)
		require.NoError(t, err)

		repo.settings["encrypted_smtp_username"] = encryptedUsername
		repo.settings["encrypted_smtp_password"] = encryptedPassword

		service := NewSettingService(repo)
		config, err := service.GetSystemConfig(ctx, testSecretKey)

		require.NoError(t, err)
		assert.True(t, config.IsInstalled)
		assert.Equal(t, "smtp_user", config.SMTPUsername)
		assert.Equal(t, "smtp_pass", config.SMTPPassword)
	})

	t.Run("decryption_error_for_smtp_username", func(t *testing.T) {
		repo := NewMockSettingRepository()
		repo.settings["is_installed"] = "true"
		repo.settings["encrypted_smtp_username"] = "invalid-encrypted-data"

		service := NewSettingService(repo)
		config, err := service.GetSystemConfig(ctx, testSecretKey)

		require.Error(t, err)
		assert.Nil(t, config)
		assert.Contains(t, err.Error(), "failed to decrypt SMTP username")
	})

	t.Run("decryption_error_for_smtp_password", func(t *testing.T) {
		repo := NewMockSettingRepository()
		repo.settings["is_installed"] = "true"
		repo.settings["encrypted_smtp_password"] = "invalid-encrypted-data"

		service := NewSettingService(repo)
		config, err := service.GetSystemConfig(ctx, testSecretKey)

		require.Error(t, err)
		assert.Nil(t, config)
		assert.Contains(t, err.Error(), "failed to decrypt SMTP password")
	})

	t.Run("installed_false_returns_early", func(t *testing.T) {
		repo := NewMockSettingRepository()
		repo.settings["is_installed"] = "false"
		repo.settings["root_email"] = "admin@example.com" // Should not be loaded

		service := NewSettingService(repo)
		config, err := service.GetSystemConfig(ctx, testSecretKey)

		require.NoError(t, err)
		assert.False(t, config.IsInstalled)
		assert.Empty(t, config.RootEmail) // Not loaded because not installed
	})

	t.Run("repository_error", func(t *testing.T) {
		repo := NewMockSettingRepository()
		repo.getError = errors.New("database connection failed")

		service := NewSettingService(repo)
		config, err := service.GetSystemConfig(ctx, testSecretKey)

		require.Error(t, err)
		assert.Nil(t, config)
		assert.Contains(t, err.Error(), "failed to get is_installed setting")
	})

	t.Run("invalid_port_uses_default", func(t *testing.T) {
		repo := NewMockSettingRepository()
		repo.settings["is_installed"] = "true"
		repo.settings["smtp_port"] = "invalid"

		service := NewSettingService(repo)
		config, err := service.GetSystemConfig(ctx, testSecretKey)

		require.NoError(t, err)
		assert.Equal(t, 587, config.SMTPPort) // Uses default
	})
}

func TestSettingService_SetSystemConfig(t *testing.T) {
	ctx := context.Background()

	t.Run("successful_set_complete_config", func(t *testing.T) {
		repo := NewMockSettingRepository()
		service := NewSettingService(repo)

		config := &SystemConfig{
			IsInstalled:      true,
			RootEmail:        "admin@example.com",
			APIEndpoint:      "https://api.example.com",
			SMTPHost:         "smtp.example.com",
			SMTPPort:         587,
			SMTPUsername:     "smtp_user",
			SMTPPassword:     "smtp_pass",
			SMTPFromEmail:    "noreply@example.com",
			SMTPFromName:     "Test App",
			TelemetryEnabled: true,
			CheckForUpdates:  false,
		}

		err := service.SetSystemConfig(ctx, config, testSecretKey)
		require.NoError(t, err)

		// Verify all settings were stored
		assert.Equal(t, "true", repo.settings["is_installed"])
		assert.Equal(t, "admin@example.com", repo.settings["root_email"])
		assert.Equal(t, "https://api.example.com", repo.settings["api_endpoint"])
		assert.Equal(t, "smtp.example.com", repo.settings["smtp_host"])
		assert.Equal(t, "587", repo.settings["smtp_port"])
		assert.Equal(t, "noreply@example.com", repo.settings["smtp_from_email"])
		assert.Equal(t, "Test App", repo.settings["smtp_from_name"])
		assert.Equal(t, "true", repo.settings["telemetry_enabled"])
		assert.Equal(t, "false", repo.settings["check_for_updates"])

		// Verify SMTP credentials are encrypted
		assert.NotEmpty(t, repo.settings["encrypted_smtp_username"])
		assert.NotEmpty(t, repo.settings["encrypted_smtp_password"])
		assert.NotEqual(t, "smtp_user", repo.settings["encrypted_smtp_username"])
		assert.NotEqual(t, "smtp_pass", repo.settings["encrypted_smtp_password"])

		// Verify we can decrypt them
		decryptedUsername, err := crypto.DecryptFromHexString(repo.settings["encrypted_smtp_username"], testSecretKey)
		require.NoError(t, err)
		assert.Equal(t, "smtp_user", decryptedUsername)

		decryptedPassword, err := crypto.DecryptFromHexString(repo.settings["encrypted_smtp_password"], testSecretKey)
		require.NoError(t, err)
		assert.Equal(t, "smtp_pass", decryptedPassword)
	})

	t.Run("set_minimal_config", func(t *testing.T) {
		repo := NewMockSettingRepository()
		service := NewSettingService(repo)

		config := &SystemConfig{
			IsInstalled: false,
		}

		err := service.SetSystemConfig(ctx, config, testSecretKey)
		require.NoError(t, err)

		assert.Equal(t, "false", repo.settings["is_installed"])
		assert.Equal(t, "false", repo.settings["telemetry_enabled"])
		assert.Equal(t, "false", repo.settings["check_for_updates"])
		// Other fields should not be set (empty strings not stored)
		assert.Empty(t, repo.settings["root_email"])
		assert.Empty(t, repo.settings["smtp_host"])
	})

	t.Run("repository_error", func(t *testing.T) {
		repo := NewMockSettingRepository()
		repo.setError = errors.New("database write failed")
		service := NewSettingService(repo)

		config := &SystemConfig{
			IsInstalled: true,
		}

		err := service.SetSystemConfig(ctx, config, testSecretKey)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to set is_installed")
	})

	t.Run("roundtrip_test", func(t *testing.T) {
		repo := NewMockSettingRepository()
		service := NewSettingService(repo)

		originalConfig := &SystemConfig{
			IsInstalled:      true,
			RootEmail:        "admin@example.com",
			APIEndpoint:      "https://api.example.com",
			SMTPHost:         "smtp.example.com",
			SMTPPort:         2525,
			SMTPUsername:     "smtp_user",
			SMTPPassword:     "smtp_pass",
			SMTPFromEmail:    "noreply@example.com",
			SMTPFromName:     "Test App",
			TelemetryEnabled: true,
			CheckForUpdates:  true,
		}

		// Set the config
		err := service.SetSystemConfig(ctx, originalConfig, testSecretKey)
		require.NoError(t, err)

		// Get the config back
		retrievedConfig, err := service.GetSystemConfig(ctx, testSecretKey)
		require.NoError(t, err)

		// Verify all fields match
		assert.Equal(t, originalConfig.IsInstalled, retrievedConfig.IsInstalled)
		assert.Equal(t, originalConfig.RootEmail, retrievedConfig.RootEmail)
		assert.Equal(t, originalConfig.APIEndpoint, retrievedConfig.APIEndpoint)
		assert.Equal(t, originalConfig.SMTPHost, retrievedConfig.SMTPHost)
		assert.Equal(t, originalConfig.SMTPPort, retrievedConfig.SMTPPort)
		assert.Equal(t, originalConfig.SMTPUsername, retrievedConfig.SMTPUsername)
		assert.Equal(t, originalConfig.SMTPPassword, retrievedConfig.SMTPPassword)
		assert.Equal(t, originalConfig.SMTPFromEmail, retrievedConfig.SMTPFromEmail)
		assert.Equal(t, originalConfig.SMTPFromName, retrievedConfig.SMTPFromName)
		assert.Equal(t, originalConfig.TelemetryEnabled, retrievedConfig.TelemetryEnabled)
		assert.Equal(t, originalConfig.CheckForUpdates, retrievedConfig.CheckForUpdates)
	})
}

func TestSettingService_IsInstalled(t *testing.T) {
	ctx := context.Background()

	t.Run("not_installed_setting_not_found", func(t *testing.T) {
		repo := NewMockSettingRepository()
		service := NewSettingService(repo)

		isInstalled, err := service.IsInstalled(ctx)

		require.NoError(t, err)
		assert.False(t, isInstalled)
	})

	t.Run("installed_true", func(t *testing.T) {
		repo := NewMockSettingRepository()
		repo.settings["is_installed"] = "true"
		service := NewSettingService(repo)

		isInstalled, err := service.IsInstalled(ctx)

		require.NoError(t, err)
		assert.True(t, isInstalled)
	})

	t.Run("installed_false", func(t *testing.T) {
		repo := NewMockSettingRepository()
		repo.settings["is_installed"] = "false"
		service := NewSettingService(repo)

		isInstalled, err := service.IsInstalled(ctx)

		require.NoError(t, err)
		assert.False(t, isInstalled)
	})

	t.Run("repository_error", func(t *testing.T) {
		repo := NewMockSettingRepository()
		repo.getError = errors.New("database connection failed")
		service := NewSettingService(repo)

		isInstalled, err := service.IsInstalled(ctx)

		require.Error(t, err)
		assert.False(t, isInstalled)
		assert.Contains(t, err.Error(), "database connection failed")
	})
}

func TestSettingService_GetSetting(t *testing.T) {
	ctx := context.Background()

	t.Run("successful_get", func(t *testing.T) {
		repo := NewMockSettingRepository()
		repo.settings["test_key"] = "test_value"
		service := NewSettingService(repo)

		value, err := service.GetSetting(ctx, "test_key")

		require.NoError(t, err)
		assert.Equal(t, "test_value", value)
	})

	t.Run("setting_not_found", func(t *testing.T) {
		repo := NewMockSettingRepository()
		service := NewSettingService(repo)

		value, err := service.GetSetting(ctx, "nonexistent_key")

		require.Error(t, err)
		assert.Empty(t, value)
		assert.IsType(t, &domain.ErrSettingNotFound{}, err)
	})

	t.Run("repository_error", func(t *testing.T) {
		repo := NewMockSettingRepository()
		repo.getError = errors.New("database read failed")
		service := NewSettingService(repo)

		value, err := service.GetSetting(ctx, "test_key")

		require.Error(t, err)
		assert.Empty(t, value)
	})
}

func TestSettingService_SetSetting(t *testing.T) {
	ctx := context.Background()

	t.Run("successful_set", func(t *testing.T) {
		repo := NewMockSettingRepository()
		service := NewSettingService(repo)

		err := service.SetSetting(ctx, "test_key", "test_value")

		require.NoError(t, err)
		assert.Equal(t, "test_value", repo.settings["test_key"])
	})

	t.Run("repository_error", func(t *testing.T) {
		repo := NewMockSettingRepository()
		repo.setError = errors.New("database write failed")
		service := NewSettingService(repo)

		err := service.SetSetting(ctx, "test_key", "test_value")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "database write failed")
	})

	t.Run("overwrite_existing_value", func(t *testing.T) {
		repo := NewMockSettingRepository()
		repo.settings["test_key"] = "old_value"
		service := NewSettingService(repo)

		err := service.SetSetting(ctx, "test_key", "new_value")

		require.NoError(t, err)
		assert.Equal(t, "new_value", repo.settings["test_key"])
	})
}

