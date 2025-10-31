package service

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"aidanwoods.dev/go-paseto"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/google/uuid"
	"github.com/wneessen/go-mail"
)

// PasetoKeys represents generated PASETO keys
type PasetoKeys struct {
	PublicKey  string
	PrivateKey string
}

// SetupConfig represents the setup initialization configuration
type SetupConfig struct {
	RootEmail          string
	APIEndpoint        string
	GeneratePasetoKeys bool
	PasetoPublicKey    string
	PasetoPrivateKey   string
	SMTPHost           string
	SMTPPort           int
	SMTPUsername       string
	SMTPPassword       string
	SMTPFromEmail      string
	SMTPFromName       string
	TelemetryEnabled   bool
	CheckForUpdates    bool
}

// SMTPTestConfig represents SMTP configuration for testing
type SMTPTestConfig struct {
	Host     string
	Port     int
	Username string
	Password string
}

// ConfigurationStatus represents which configuration groups are set via environment
type ConfigurationStatus struct {
	SMTPConfigured        bool
	PasetoConfigured      bool
	APIEndpointConfigured bool
	RootEmailConfigured   bool
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
	RootEmail        string
	APIEndpoint      string
	PasetoPublicKey  string
	PasetoPrivateKey string
	SMTPHost         string
	SMTPPort         int
	SMTPUsername     string
	SMTPPassword     string
	SMTPFromEmail    string
	SMTPFromName     string
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
			PasetoConfigured:      false,
			APIEndpointConfigured: false,
			RootEmailConfigured:   false,
		}
	}

	// SMTP is configured if ALL required SMTP fields are present
	// Note: Username/Password are optional (some SMTP servers don't require auth)
	smtpConfigured := s.envConfig.SMTPHost != "" &&
		s.envConfig.SMTPPort > 0 &&
		s.envConfig.SMTPFromEmail != ""

	// PASETO is configured if BOTH keys are present
	pasetoConfigured := s.envConfig.PasetoPublicKey != "" &&
		s.envConfig.PasetoPrivateKey != ""

	return &ConfigurationStatus{
		SMTPConfigured:        smtpConfigured,
		PasetoConfigured:      pasetoConfigured,
		APIEndpointConfigured: s.envConfig.APIEndpoint != "",
		RootEmailConfigured:   s.envConfig.RootEmail != "",
	}
}

// GeneratePasetoKeys generates new PASETO v4 asymmetric keys
func (s *SetupService) GeneratePasetoKeys() (*PasetoKeys, error) {
	secretKey := paseto.NewV4AsymmetricSecretKey()
	publicKey := secretKey.Public()

	privateKeyBase64 := base64.StdEncoding.EncodeToString(secretKey.ExportBytes())
	publicKeyBase64 := base64.StdEncoding.EncodeToString(publicKey.ExportBytes())

	return &PasetoKeys{
		PublicKey:  publicKeyBase64,
		PrivateKey: privateKeyBase64,
	}, nil
}

// ValidatePasetoKeys validates the format of provided PASETO keys
func (s *SetupService) ValidatePasetoKeys(publicKey, privateKey string) error {
	if _, err := base64.StdEncoding.DecodeString(privateKey); err != nil {
		return fmt.Errorf("invalid PASETO private key format: %w", err)
	}
	if _, err := base64.StdEncoding.DecodeString(publicKey); err != nil {
		return fmt.Errorf("invalid PASETO public key format: %w", err)
	}
	return nil
}

// ValidateSetupConfig validates the setup configuration, only checking user-provided fields
func (s *SetupService) ValidateSetupConfig(config *SetupConfig) error {
	status := s.GetConfigurationStatus()

	// Validate root_email if not configured via env
	if !status.RootEmailConfigured && config.RootEmail == "" {
		return fmt.Errorf("root_email is required")
	}

	// Validate PASETO keys if not configured via env
	if !status.PasetoConfigured {
		if !config.GeneratePasetoKeys {
			if config.PasetoPrivateKey == "" || config.PasetoPublicKey == "" {
				return fmt.Errorf("PASETO keys are required when not generating new ones")
			}
			if err := s.ValidatePasetoKeys(config.PasetoPublicKey, config.PasetoPrivateKey); err != nil {
				return err
			}
		}
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
func (s *SetupService) Initialize(ctx context.Context, config *SetupConfig) (*PasetoKeys, error) {
	// Validate configuration
	if err := s.ValidateSetupConfig(config); err != nil {
		return nil, err
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

	// Handle PASETO keys
	var generatedKeys *PasetoKeys
	var privateKeyBase64, publicKeyBase64 string

	if status.PasetoConfigured {
		// Use env-configured keys
		privateKeyBase64 = s.envConfig.PasetoPrivateKey
		publicKeyBase64 = s.envConfig.PasetoPublicKey
	} else if config.GeneratePasetoKeys {
		// Generate new keys
		keys, err := s.GeneratePasetoKeys()
		if err != nil {
			return nil, fmt.Errorf("failed to generate PASETO keys: %w", err)
		}
		privateKeyBase64 = keys.PrivateKey
		publicKeyBase64 = keys.PublicKey
		generatedKeys = keys
	} else {
		// Use provided keys
		privateKeyBase64 = config.PasetoPrivateKey
		publicKeyBase64 = config.PasetoPublicKey
	}

	// Handle SMTP configuration
	var smtpHost, smtpUsername, smtpPassword, smtpFromEmail, smtpFromName string
	var smtpPort int

	if status.SMTPConfigured {
		// Use env-configured SMTP
		smtpHost = s.envConfig.SMTPHost
		smtpPort = s.envConfig.SMTPPort
		smtpUsername = s.envConfig.SMTPUsername
		smtpPassword = s.envConfig.SMTPPassword
		smtpFromEmail = s.envConfig.SMTPFromEmail
		smtpFromName = s.envConfig.SMTPFromName
	} else {
		// Use user-provided SMTP
		smtpHost = config.SMTPHost
		smtpPort = config.SMTPPort
		smtpUsername = config.SMTPUsername
		smtpPassword = config.SMTPPassword
		smtpFromEmail = config.SMTPFromEmail
		smtpFromName = config.SMTPFromName
	}

	// Store system settings
	systemConfig := &SystemConfig{
		IsInstalled:      true,
		RootEmail:        finalConfig.RootEmail,
		APIEndpoint:      finalConfig.APIEndpoint,
		PasetoPrivateKey: privateKeyBase64,
		PasetoPublicKey:  publicKeyBase64,
		SMTPHost:         smtpHost,
		SMTPPort:         smtpPort,
		SMTPUsername:     smtpUsername,
		SMTPPassword:     smtpPassword,
		SMTPFromEmail:    smtpFromEmail,
		SMTPFromName:     smtpFromName,
		TelemetryEnabled: config.TelemetryEnabled,
		CheckForUpdates:  config.CheckForUpdates,
	}

	if err := s.settingService.SetSystemConfig(ctx, systemConfig, s.secretKey); err != nil {
		return nil, fmt.Errorf("failed to save system configuration: %w", err)
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
			return nil, fmt.Errorf("failed to create root user: %w", err)
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

	return generatedKeys, nil
}

// TestSMTPConnection tests the SMTP connection with the provided configuration
func (s *SetupService) TestSMTPConnection(ctx context.Context, config *SMTPTestConfig) error {
	if config.Host == "" {
		return fmt.Errorf("SMTP host is required")
	}

	if config.Port == 0 {
		return fmt.Errorf("SMTP port is required")
	}

	// Build client options
	clientOptions := []mail.Option{
		mail.WithPort(config.Port),
		mail.WithTLSPolicy(mail.TLSMandatory),
	}

	// Only add authentication if username and password are provided
	// This allows for unauthenticated SMTP servers (e.g., local relays, port 25)
	if config.Username != "" && config.Password != "" {
		clientOptions = append(clientOptions,
			mail.WithUsername(config.Username),
			mail.WithPassword(config.Password),
			mail.WithSMTPAuth(mail.SMTPAuthPlain),
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
