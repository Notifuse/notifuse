package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/google/uuid"
	"github.com/wneessen/go-mail"
)

// SetupConfig represents the setup initialization configuration
type SetupConfig struct {
	RootEmail              string
	APIEndpoint            string
	SMTPHost               string
	SMTPPort               int
	SMTPUsername           string
	SMTPPassword           string
	SMTPFromEmail          string
	SMTPFromName           string
	SMTPUseTLS             bool
	TelemetryEnabled       bool
	CheckForUpdates        bool
	SMTPRelayEnabled       bool
	SMTPRelayDomain        string
	SMTPRelayPort          int
	SMTPRelayTLSCertBase64 string
	SMTPRelayTLSKeyBase64  string
}

// SMTPTestConfig represents SMTP configuration for testing
type SMTPTestConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	UseTLS   bool
}

// ConfigurationStatus represents which configuration groups are set via environment
type ConfigurationStatus struct {
	SMTPConfigured        bool
	APIEndpointConfigured bool
	RootEmailConfigured   bool
	SMTPRelayConfigured   bool
}

// SetupService handles setup wizard operations
type SetupService struct {
	settingService   *SettingService
	userService      *UserService
	userRepo         domain.UserRepository
	logger           logger.Logger
	secretKey        string
	onSetupCompleted func() error // Callback to reload config after setup
	envConfig        *EnvironmentConfig
}

// EnvironmentConfig holds configuration from environment variables
type EnvironmentConfig struct {
	RootEmail              string
	APIEndpoint            string
	SMTPHost               string
	SMTPPort               int
	SMTPUsername           string
	SMTPPassword           string
	SMTPFromEmail          string
	SMTPFromName           string
	SMTPUseTLS             string // "true", "false", or "" (empty = not set, defaults to true)
	SMTPRelayEnabled       string // "true", "false", or "" (empty = not set, allows setup wizard to configure)
	SMTPRelayDomain        string
	SMTPRelayPort          int
	SMTPRelayTLSCertBase64 string
	SMTPRelayTLSKeyBase64  string
}

// NewSetupService creates a new setup service
func NewSetupService(
	settingService *SettingService,
	userService *UserService,
	userRepo domain.UserRepository,
	logger logger.Logger,
	secretKey string,
	onSetupCompleted func() error,
	envConfig *EnvironmentConfig,
) *SetupService {
	return &SetupService{
		settingService:   settingService,
		userService:      userService,
		userRepo:         userRepo,
		logger:           logger,
		secretKey:        secretKey,
		onSetupCompleted: onSetupCompleted,
		envConfig:        envConfig,
	}
}

// GetConfigurationStatus checks which configuration groups are set via environment
func (s *SetupService) GetConfigurationStatus() *ConfigurationStatus {
	if s.envConfig == nil {
		return &ConfigurationStatus{
			SMTPConfigured:        false,
			APIEndpointConfigured: false,
			RootEmailConfigured:   false,
			SMTPRelayConfigured:   false,
		}
	}

	// SMTP is configured if ALL required SMTP fields are present
	// Note: Username/Password are optional (some SMTP servers don't require auth)
	smtpConfigured := s.envConfig.SMTPHost != "" &&
		s.envConfig.SMTPPort > 0 &&
		s.envConfig.SMTPFromEmail != ""

	// SMTP Relay is configured if:
	// 1. SMTP_RELAY_ENABLED env var is explicitly set (even if "" or "false") - this prevents setup wizard from enabling it
	// 2. OR if enabled ("true") and has all required fields
	smtpRelayConfigured := s.envConfig.SMTPRelayEnabled != "" ||
		(s.envConfig.SMTPRelayEnabled == "true" &&
			s.envConfig.SMTPRelayDomain != "" &&
			s.envConfig.SMTPRelayTLSCertBase64 != "" &&
			s.envConfig.SMTPRelayTLSKeyBase64 != "")

	return &ConfigurationStatus{
		SMTPConfigured:        smtpConfigured,
		APIEndpointConfigured: s.envConfig.APIEndpoint != "",
		RootEmailConfigured:   s.envConfig.RootEmail != "",
		SMTPRelayConfigured:   smtpRelayConfigured,
	}
}

// ValidateSetupConfig validates the setup configuration, only checking user-provided fields
func (s *SetupService) ValidateSetupConfig(config *SetupConfig) error {
	status := s.GetConfigurationStatus()

	// Validate root_email if not configured via env
	if !status.RootEmailConfigured && config.RootEmail == "" {
		return fmt.Errorf("root_email is required")
	}

	// Validate SMTP if not configured via env
	if !status.SMTPConfigured {
		if config.SMTPHost == "" {
			return fmt.Errorf("smtp_host is required")
		}

		if config.SMTPPort == 0 {
			config.SMTPPort = 587 // Default
		}

		if config.SMTPFromEmail == "" {
			return fmt.Errorf("smtp_from_email is required")
		}
	}

	return nil
}

// Initialize completes the setup wizard
func (s *SetupService) Initialize(ctx context.Context, config *SetupConfig) error {
	// Validate configuration
	if err := s.ValidateSetupConfig(config); err != nil {
		return err
	}

	status := s.GetConfigurationStatus()

	// Merge configuration: env vars always win
	finalConfig := &SetupConfig{
		RootEmail:   config.RootEmail,
		APIEndpoint: config.APIEndpoint,
	}

	// Override with env values if configured
	if status.RootEmailConfigured {
		finalConfig.RootEmail = s.envConfig.RootEmail
	}
	if status.APIEndpointConfigured {
		finalConfig.APIEndpoint = s.envConfig.APIEndpoint
	}

	// Sanitize API endpoint
	finalConfig.APIEndpoint = strings.TrimRight(finalConfig.APIEndpoint, "/")

	// Handle SMTP configuration
	var smtpHost, smtpUsername, smtpPassword, smtpFromEmail, smtpFromName string
	var smtpPort int
	var smtpUseTLS bool

	if status.SMTPConfigured {
		// Use env-configured SMTP
		smtpHost = s.envConfig.SMTPHost
		smtpPort = s.envConfig.SMTPPort
		smtpUsername = s.envConfig.SMTPUsername
		smtpPassword = s.envConfig.SMTPPassword
		smtpFromEmail = s.envConfig.SMTPFromEmail
		smtpFromName = s.envConfig.SMTPFromName
		// TLS defaults to true unless explicitly set to false via env var
		smtpUseTLS = s.envConfig.SMTPUseTLS != "false"
	} else {
		// Use user-provided SMTP
		smtpHost = config.SMTPHost
		smtpPort = config.SMTPPort
		smtpUsername = config.SMTPUsername
		smtpPassword = config.SMTPPassword
		smtpFromEmail = config.SMTPFromEmail
		smtpFromName = config.SMTPFromName
		smtpUseTLS = config.SMTPUseTLS
	}

	// Handle SMTP Relay configuration
	var smtpRelayEnabled bool
	var smtpRelayDomain, smtpRelayTLSCertBase64, smtpRelayTLSKeyBase64 string
	var smtpRelayPort int

	if status.SMTPRelayConfigured {
		// Use env-configured SMTP Relay (parse string to bool)
		smtpRelayEnabled = s.envConfig.SMTPRelayEnabled == "true"
		smtpRelayDomain = s.envConfig.SMTPRelayDomain
		smtpRelayPort = s.envConfig.SMTPRelayPort
		smtpRelayTLSCertBase64 = s.envConfig.SMTPRelayTLSCertBase64
		smtpRelayTLSKeyBase64 = s.envConfig.SMTPRelayTLSKeyBase64
	} else {
		// Use user-provided SMTP Relay
		smtpRelayEnabled = config.SMTPRelayEnabled
		smtpRelayDomain = config.SMTPRelayDomain
		smtpRelayPort = config.SMTPRelayPort
		smtpRelayTLSCertBase64 = config.SMTPRelayTLSCertBase64
		smtpRelayTLSKeyBase64 = config.SMTPRelayTLSKeyBase64
	}

	// Store system settings
	systemConfig := &SystemConfig{
		IsInstalled:            true,
		RootEmail:              finalConfig.RootEmail,
		APIEndpoint:            finalConfig.APIEndpoint,
		SMTPHost:               smtpHost,
		SMTPPort:               smtpPort,
		SMTPUsername:           smtpUsername,
		SMTPPassword:           smtpPassword,
		SMTPFromEmail:          smtpFromEmail,
		SMTPFromName:           smtpFromName,
		SMTPUseTLS:             smtpUseTLS,
		TelemetryEnabled:       config.TelemetryEnabled,
		CheckForUpdates:        config.CheckForUpdates,
		SMTPRelayEnabled:       smtpRelayEnabled,
		SMTPRelayDomain:        smtpRelayDomain,
		SMTPRelayPort:          smtpRelayPort,
		SMTPRelayTLSCertBase64: smtpRelayTLSCertBase64,
		SMTPRelayTLSKeyBase64:  smtpRelayTLSKeyBase64,
	}

	if err := s.settingService.SetSystemConfig(ctx, systemConfig, s.secretKey); err != nil {
		return fmt.Errorf("failed to save system configuration: %w", err)
	}

	// Create root user (use final merged email)
	rootUser := &domain.User{
		ID:        uuid.New().String(),
		Email:     finalConfig.RootEmail,
		Name:      "Root User",
		Type:      domain.UserTypeUser,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	if err := s.userRepo.CreateUser(ctx, rootUser); err != nil {
		// Check if user already exists - if so, that's okay during setup
		var errUserExists *domain.ErrUserExists
		if !errors.As(err, &errUserExists) {
			return fmt.Errorf("failed to create root user: %w", err)
		}
		// User already exists - this is fine during setup, continue
		s.logger.WithField("email", finalConfig.RootEmail).Info("Root user already exists, skipping creation")
	}

	s.logger.WithField("email", finalConfig.RootEmail).Info("Setup wizard completed successfully")

	// Reload configuration if callback is provided
	if s.onSetupCompleted != nil {
		if err := s.onSetupCompleted(); err != nil {
			s.logger.WithField("error", err).Error("Failed to reload configuration after setup")
			// Don't fail the request - setup was successful, just log the error
		}
	}

	return nil
}

// TestSMTPConnection tests the SMTP connection with the provided configuration
func (s *SetupService) TestSMTPConnection(ctx context.Context, config *SMTPTestConfig) error {
	if config.Host == "" {
		return fmt.Errorf("SMTP host is required")
	}

	if config.Port == 0 {
		return fmt.Errorf("SMTP port is required")
	}

	// Determine TLS policy based on config
	tlsPolicy := mail.TLSMandatory
	if !config.UseTLS {
		tlsPolicy = mail.NoTLS
	}

	// Build client options
	clientOptions := []mail.Option{
		mail.WithPort(config.Port),
		mail.WithTLSPolicy(tlsPolicy),
	}

	// Only add authentication if username and password are provided
	// This allows for unauthenticated SMTP servers (e.g., local relays, port 25)
	if config.Username != "" && config.Password != "" {
		clientOptions = append(clientOptions,
			mail.WithUsername(config.Username),
			mail.WithPassword(config.Password),
			mail.WithSMTPAuth(mail.SMTPAuthAutoDiscover),
		)
	}

	// Create mail client with timeout from context
	client, err := mail.NewClient(config.Host, clientOptions...)
	if err != nil {
		return fmt.Errorf("failed to create SMTP client: %w", err)
	}

	// Test the connection by dialing
	if err := client.DialWithContext(ctx); err != nil {
		return fmt.Errorf("failed to connect to SMTP server: %w", err)
	}

	// Close the connection
	if err := client.Close(); err != nil {
		s.logger.WithField("error", err).Warn("Failed to close SMTP connection gracefully")
	}

	return nil
}
