package service

import (
	"context"
	"fmt"
	"strconv"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/crypto"
)

// SystemConfig holds all system-level configuration
type SystemConfig struct {
	IsInstalled            bool
	RootEmail              string
	APIEndpoint            string
	SMTPHost               string
	SMTPPort               int
	SMTPUsername           string
	SMTPPassword           string
	SMTPFromEmail          string
	SMTPFromName           string
	SMTPUseTLS             bool
	SMTPEHLOHostname       string
	TelemetryEnabled       bool
	CheckForUpdates        bool
	SMTPBridgeEnabled       bool
	SMTPBridgeDomain        string
	SMTPBridgePort          int
	SMTPBridgeTLSCertBase64 string
	SMTPBridgeTLSKeyBase64  string
}

// SettingService provides methods for managing system settings
type SettingService struct {
	repo domain.SettingRepository
}

// NewSettingService creates a new SettingService
func NewSettingService(repo domain.SettingRepository) *SettingService {
	return &SettingService{
		repo: repo,
	}
}

// GetSystemConfig loads all system settings from the database
func (s *SettingService) GetSystemConfig(ctx context.Context, secretKey string) (*SystemConfig, error) {
	config := &SystemConfig{
		IsInstalled: false,
		SMTPPort:    587,  // Default
		SMTPUseTLS:  true, // Default to TLS enabled
	}

	// Check if system is installed
	isInstalledSetting, err := s.repo.Get(ctx, "is_installed")
	if err != nil {
		if _, ok := err.(*domain.ErrSettingNotFound); !ok {
			return nil, fmt.Errorf("failed to get is_installed setting: %w", err)
		}
		// Not installed yet
		return config, nil
	}

	config.IsInstalled = isInstalledSetting.Value == "true"
	if !config.IsInstalled {
		return config, nil
	}

	// Load root email
	if setting, err := s.repo.Get(ctx, "root_email"); err == nil {
		config.RootEmail = setting.Value
	}

	// Load API endpoint
	if setting, err := s.repo.Get(ctx, "api_endpoint"); err == nil {
		config.APIEndpoint = setting.Value
	}

	// Load SMTP settings
	if setting, err := s.repo.Get(ctx, "smtp_host"); err == nil {
		config.SMTPHost = setting.Value
	}

	if setting, err := s.repo.Get(ctx, "smtp_port"); err == nil && setting.Value != "" {
		if port, err := strconv.Atoi(setting.Value); err == nil {
			config.SMTPPort = port
		}
	}

	if setting, err := s.repo.Get(ctx, "smtp_from_email"); err == nil {
		config.SMTPFromEmail = setting.Value
	}

	if setting, err := s.repo.Get(ctx, "smtp_from_name"); err == nil {
		config.SMTPFromName = setting.Value
	}

	// Load SMTP TLS setting (default to true if not set)
	if setting, err := s.repo.Get(ctx, "smtp_use_tls"); err == nil {
		config.SMTPUseTLS = setting.Value != "false"
	}

	// Load SMTP EHLO hostname
	if setting, err := s.repo.Get(ctx, "smtp_ehlo_hostname"); err == nil {
		config.SMTPEHLOHostname = setting.Value
	}

	// Load and decrypt SMTP username
	if setting, err := s.repo.Get(ctx, "encrypted_smtp_username"); err == nil && setting.Value != "" {
		decrypted, err := crypto.DecryptFromHexString(setting.Value, secretKey)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt SMTP username: %w", err)
		}
		config.SMTPUsername = decrypted
	}

	// Load and decrypt SMTP password
	if setting, err := s.repo.Get(ctx, "encrypted_smtp_password"); err == nil && setting.Value != "" {
		decrypted, err := crypto.DecryptFromHexString(setting.Value, secretKey)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt SMTP password: %w", err)
		}
		config.SMTPPassword = decrypted
	}

	// Load telemetry setting
	if setting, err := s.repo.Get(ctx, "telemetry_enabled"); err == nil {
		config.TelemetryEnabled = setting.Value == "true"
	}

	// Load check for updates setting
	if setting, err := s.repo.Get(ctx, "check_for_updates"); err == nil {
		config.CheckForUpdates = setting.Value == "true"
	}

	// Load SMTP Bridge settings
	if setting, err := s.repo.Get(ctx, "smtp_bridge_enabled"); err == nil {
		config.SMTPBridgeEnabled = setting.Value == "true"
	}

	if setting, err := s.repo.Get(ctx, "smtp_bridge_domain"); err == nil {
		config.SMTPBridgeDomain = setting.Value
	}

	if setting, err := s.repo.Get(ctx, "smtp_bridge_port"); err == nil && setting.Value != "" {
		if port, err := strconv.Atoi(setting.Value); err == nil {
			config.SMTPBridgePort = port
		}
	}

	// Load and decrypt SMTP Bridge TLS certificate
	if setting, err := s.repo.Get(ctx, "encrypted_smtp_bridge_tls_cert_base64"); err == nil && setting.Value != "" {
		decrypted, err := crypto.DecryptFromHexString(setting.Value, secretKey)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt SMTP bridge TLS certificate: %w", err)
		}
		config.SMTPBridgeTLSCertBase64 = decrypted
	}

	// Load and decrypt SMTP Bridge TLS key
	if setting, err := s.repo.Get(ctx, "encrypted_smtp_bridge_tls_key_base64"); err == nil && setting.Value != "" {
		decrypted, err := crypto.DecryptFromHexString(setting.Value, secretKey)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt SMTP bridge TLS key: %w", err)
		}
		config.SMTPBridgeTLSKeyBase64 = decrypted
	}

	return config, nil
}

// SetSystemConfig stores all system settings in the database
func (s *SettingService) SetSystemConfig(ctx context.Context, config *SystemConfig, secretKey string) error {
	// Set is_installed flag
	isInstalled := "false"
	if config.IsInstalled {
		isInstalled = "true"
	}
	if err := s.repo.Set(ctx, "is_installed", isInstalled); err != nil {
		return fmt.Errorf("failed to set is_installed: %w", err)
	}

	// Set root email
	if config.RootEmail != "" {
		if err := s.repo.Set(ctx, "root_email", config.RootEmail); err != nil {
			return fmt.Errorf("failed to set root_email: %w", err)
		}
	}

	// Set API endpoint
	if config.APIEndpoint != "" {
		if err := s.repo.Set(ctx, "api_endpoint", config.APIEndpoint); err != nil {
			return fmt.Errorf("failed to set api_endpoint: %w", err)
		}
	}

	// Set SMTP settings
	if config.SMTPHost != "" {
		if err := s.repo.Set(ctx, "smtp_host", config.SMTPHost); err != nil {
			return fmt.Errorf("failed to set smtp_host: %w", err)
		}
	}

	if config.SMTPPort > 0 {
		if err := s.repo.Set(ctx, "smtp_port", strconv.Itoa(config.SMTPPort)); err != nil {
			return fmt.Errorf("failed to set smtp_port: %w", err)
		}
	}

	if config.SMTPFromEmail != "" {
		if err := s.repo.Set(ctx, "smtp_from_email", config.SMTPFromEmail); err != nil {
			return fmt.Errorf("failed to set smtp_from_email: %w", err)
		}
	}

	if config.SMTPFromName != "" {
		if err := s.repo.Set(ctx, "smtp_from_name", config.SMTPFromName); err != nil {
			return fmt.Errorf("failed to set smtp_from_name: %w", err)
		}
	}

	// Set SMTP EHLO hostname
	if config.SMTPEHLOHostname != "" {
		if err := s.repo.Set(ctx, "smtp_ehlo_hostname", config.SMTPEHLOHostname); err != nil {
			return fmt.Errorf("failed to set smtp_ehlo_hostname: %w", err)
		}
	}

	// Set SMTP TLS setting
	smtpUseTLSValue := "true"
	if !config.SMTPUseTLS {
		smtpUseTLSValue = "false"
	}
	if err := s.repo.Set(ctx, "smtp_use_tls", smtpUseTLSValue); err != nil {
		return fmt.Errorf("failed to set smtp_use_tls: %w", err)
	}

	// Encrypt and store SMTP username
	if config.SMTPUsername != "" {
		encrypted, err := crypto.EncryptString(config.SMTPUsername, secretKey)
		if err != nil {
			return fmt.Errorf("failed to encrypt SMTP username: %w", err)
		}
		if err := s.repo.Set(ctx, "encrypted_smtp_username", encrypted); err != nil {
			return fmt.Errorf("failed to set encrypted_smtp_username: %w", err)
		}
	}

	// Encrypt and store SMTP password
	if config.SMTPPassword != "" {
		encrypted, err := crypto.EncryptString(config.SMTPPassword, secretKey)
		if err != nil {
			return fmt.Errorf("failed to encrypt SMTP password: %w", err)
		}
		if err := s.repo.Set(ctx, "encrypted_smtp_password", encrypted); err != nil {
			return fmt.Errorf("failed to set encrypted_smtp_password: %w", err)
		}
	}

	// Set telemetry enabled
	telemetryValue := "false"
	if config.TelemetryEnabled {
		telemetryValue = "true"
	}
	if err := s.repo.Set(ctx, "telemetry_enabled", telemetryValue); err != nil {
		return fmt.Errorf("failed to set telemetry_enabled: %w", err)
	}

	// Set check for updates
	checkUpdatesValue := "false"
	if config.CheckForUpdates {
		checkUpdatesValue = "true"
	}
	if err := s.repo.Set(ctx, "check_for_updates", checkUpdatesValue); err != nil {
		return fmt.Errorf("failed to set check_for_updates: %w", err)
	}

	// Set SMTP Bridge enabled
	smtpBridgeEnabledValue := "false"
	if config.SMTPBridgeEnabled {
		smtpBridgeEnabledValue = "true"
	}
	if err := s.repo.Set(ctx, "smtp_bridge_enabled", smtpBridgeEnabledValue); err != nil {
		return fmt.Errorf("failed to set smtp_bridge_enabled: %w", err)
	}

	// Set SMTP Bridge domain
	if config.SMTPBridgeDomain != "" {
		if err := s.repo.Set(ctx, "smtp_bridge_domain", config.SMTPBridgeDomain); err != nil {
			return fmt.Errorf("failed to set smtp_bridge_domain: %w", err)
		}
	}

	// Set SMTP Bridge port
	if config.SMTPBridgePort > 0 {
		if err := s.repo.Set(ctx, "smtp_bridge_port", strconv.Itoa(config.SMTPBridgePort)); err != nil {
			return fmt.Errorf("failed to set smtp_bridge_port: %w", err)
		}
	}

	// Encrypt and store SMTP Bridge TLS certificate
	if config.SMTPBridgeTLSCertBase64 != "" {
		encrypted, err := crypto.EncryptString(config.SMTPBridgeTLSCertBase64, secretKey)
		if err != nil {
			return fmt.Errorf("failed to encrypt SMTP bridge TLS certificate: %w", err)
		}
		if err := s.repo.Set(ctx, "encrypted_smtp_bridge_tls_cert_base64", encrypted); err != nil {
			return fmt.Errorf("failed to set encrypted_smtp_bridge_tls_cert_base64: %w", err)
		}
	}

	// Encrypt and store SMTP Bridge TLS key
	if config.SMTPBridgeTLSKeyBase64 != "" {
		encrypted, err := crypto.EncryptString(config.SMTPBridgeTLSKeyBase64, secretKey)
		if err != nil {
			return fmt.Errorf("failed to encrypt SMTP bridge TLS key: %w", err)
		}
		if err := s.repo.Set(ctx, "encrypted_smtp_bridge_tls_key_base64", encrypted); err != nil {
			return fmt.Errorf("failed to set encrypted_smtp_bridge_tls_key_base64: %w", err)
		}
	}

	return nil
}

// IsInstalled checks if the system has been installed
func (s *SettingService) IsInstalled(ctx context.Context) (bool, error) {
	setting, err := s.repo.Get(ctx, "is_installed")
	if err != nil {
		if _, ok := err.(*domain.ErrSettingNotFound); ok {
			return false, nil
		}
		return false, err
	}
	return setting.Value == "true", nil
}

// GetSetting retrieves a single setting by key
func (s *SettingService) GetSetting(ctx context.Context, key string) (string, error) {
	setting, err := s.repo.Get(ctx, key)
	if err != nil {
		return "", err
	}
	return setting.Value, nil
}

// SetSetting sets a single setting
func (s *SettingService) SetSetting(ctx context.Context, key, value string) error {
	return s.repo.Set(ctx, key, value)
}
