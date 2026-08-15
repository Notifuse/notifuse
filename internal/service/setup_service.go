package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	appconfig "github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/google/uuid"
	"github.com/wneessen/go-mail"
)

// SetupConfig represents the setup initialization configuration
type SetupConfig struct {
	RootEmail               string
	APIEndpoint             string
	SMTPHost                string
	SMTPPort                int
	SMTPUsername            string
	SMTPPassword            string
	SMTPFromEmail           string
	SMTPFromName            string
	SMTPUseTLS              bool
	SMTPEHLOHostname        string
	TelemetryEnabled        bool
	CheckForUpdates         bool
	SMTPBridgeEnabled       bool
	SMTPBridgeDomain        string
	SMTPBridgePort          int
	SMTPBridgeTLSCertBase64 string
	SMTPBridgeTLSKeyBase64  string

	// OIDC (optional SSO)
	OIDCEnabled         bool
	OIDCIssuerURL       string
	OIDCClientID        string
	OIDCClientSecret    string
	OIDCRedirectURI     string
	OIDCScopes          string
	OIDCButtonLabel     string
	OIDCAutoCreateUsers bool
	OIDCAllowedDomains  string
}

// SMTPTestConfig represents SMTP configuration for testing
type SMTPTestConfig struct {
	Host         string
	Port         int
	Username     string
	Password     string
	UseTLS       bool
	EHLOHostname string
}

// ConfigurationStatus represents which configuration groups are set via environment
type ConfigurationStatus struct {
	SMTPConfigured        bool
	APIEndpointConfigured bool
	RootEmailConfigured   bool
	SMTPBridgeConfigured  bool
	OIDCConfigured        bool
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
	RootEmail               string
	APIEndpoint             string
	SMTPHost                string
	SMTPPort                int
	SMTPUsername            string
	SMTPPassword            string
	SMTPFromEmail           string
	SMTPFromName            string
	SMTPUseTLS              string // "true", "false", or "" (empty = not set, defaults to true)
	SMTPEHLOHostname        string
	SMTPBridgeEnabled       string // "true", "false", or "" (empty = not set, allows setup wizard to configure)
	SMTPBridgeDomain        string
	SMTPBridgePort          int
	SMTPBridgeTLSCertBase64 string
	SMTPBridgeTLSKeyBase64  string

	// OIDC env values (Enabled/AutoCreateUsers are tri-state strings).
	OIDCEnabled         string
	OIDCIssuerURL       string
	OIDCClientID        string
	OIDCClientSecret    string
	OIDCRedirectURI     string
	OIDCScopes          string
	OIDCButtonLabel     string
	OIDCAutoCreateUsers string
	OIDCAllowedDomains  string
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
			SMTPBridgeConfigured:  false,
			OIDCConfigured:        false,
		}
	}

	// SMTP is configured if ALL required SMTP fields are present
	// Note: Username/Password are optional (some SMTP servers don't require auth)
	smtpConfigured := s.envConfig.SMTPHost != "" &&
		s.envConfig.SMTPPort > 0 &&
		s.envConfig.SMTPFromEmail != ""

	// SMTP Bridge is configured if:
	// 1. SMTP_BRIDGE_ENABLED env var is explicitly set (even if "" or "false") - this prevents setup wizard from enabling it
	// 2. OR if enabled ("true") and has all required fields
	smtpBridgeConfigured := s.envConfig.SMTPBridgeEnabled != "" ||
		(s.envConfig.SMTPBridgeEnabled == "true" &&
			s.envConfig.SMTPBridgeDomain != "" &&
			s.envConfig.SMTPBridgeTLSCertBase64 != "" &&
			s.envConfig.SMTPBridgeTLSKeyBase64 != "")

	// OIDC is "configured via env" (locking the wizard out) if any core env var is set.
	oidcConfigured := s.envConfig.OIDCEnabled != "" ||
		s.envConfig.OIDCIssuerURL != "" ||
		s.envConfig.OIDCClientID != ""

	return &ConfigurationStatus{
		SMTPConfigured:        smtpConfigured,
		APIEndpointConfigured: s.envConfig.APIEndpoint != "",
		RootEmailConfigured:   s.envConfig.RootEmail != "",
		SMTPBridgeConfigured:  smtpBridgeConfigured,
		OIDCConfigured:        oidcConfigured,
	}
}

// GetEnvConfig returns the environment-variable configuration the service was
// constructed with (may be nil). Used to surface live env-overridden values that
// differ from the values persisted to the database at install time.
func (s *SetupService) GetEnvConfig() *EnvironmentConfig {
	return s.envConfig
}

// GetEnvOverrides returns a map of setting keys that are overridden by environment variables.
// A key is present (true) if the corresponding env var has a non-empty/non-zero value.
func (s *SetupService) GetEnvOverrides() map[string]bool {
	result := make(map[string]bool)

	if s.envConfig == nil {
		return result
	}

	if s.envConfig.RootEmail != "" {
		result["root_email"] = true
	}
	if s.envConfig.APIEndpoint != "" {
		result["api_endpoint"] = true
	}
	if s.envConfig.SMTPHost != "" {
		result["smtp_host"] = true
	}
	if s.envConfig.SMTPPort > 0 {
		result["smtp_port"] = true
	}
	if s.envConfig.SMTPUsername != "" {
		result["smtp_username"] = true
	}
	if s.envConfig.SMTPPassword != "" {
		result["smtp_password"] = true
	}
	if s.envConfig.SMTPFromEmail != "" {
		result["smtp_from_email"] = true
	}
	if s.envConfig.SMTPFromName != "" {
		result["smtp_from_name"] = true
	}
	if s.envConfig.SMTPUseTLS != "" {
		result["smtp_use_tls"] = true
	}
	if s.envConfig.SMTPEHLOHostname != "" {
		result["smtp_ehlo_hostname"] = true
	}
	if s.envConfig.SMTPBridgeEnabled != "" {
		result["smtp_bridge_enabled"] = true
	}
	if s.envConfig.SMTPBridgeDomain != "" {
		result["smtp_bridge_domain"] = true
	}
	if s.envConfig.SMTPBridgePort > 0 {
		result["smtp_bridge_port"] = true
	}
	if s.envConfig.SMTPBridgeTLSCertBase64 != "" {
		result["smtp_bridge_tls_cert_base64"] = true
	}
	if s.envConfig.SMTPBridgeTLSKeyBase64 != "" {
		result["smtp_bridge_tls_key_base64"] = true
	}

	// OIDC overrides (keys match the settings keys so the UI can lock fields).
	if s.envConfig.OIDCEnabled != "" {
		result["oidc_enabled"] = true
	}
	if s.envConfig.OIDCIssuerURL != "" {
		result["oidc_issuer_url"] = true
	}
	if s.envConfig.OIDCClientID != "" {
		result["oidc_client_id"] = true
	}
	if s.envConfig.OIDCClientSecret != "" {
		result["oidc_client_secret"] = true
	}
	if s.envConfig.OIDCRedirectURI != "" {
		result["oidc_redirect_uri"] = true
	}
	if s.envConfig.OIDCScopes != "" {
		result["oidc_scopes"] = true
	}
	if s.envConfig.OIDCButtonLabel != "" {
		result["oidc_button_label"] = true
	}
	if s.envConfig.OIDCAutoCreateUsers != "" {
		result["oidc_auto_create_users"] = true
	}
	if s.envConfig.OIDCAllowedDomains != "" {
		result["oidc_allowed_domains"] = true
	}

	return result
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

	// Validate OIDC when the user enables it in the wizard (and it's not env-configured).
	if !status.OIDCConfigured && config.OIDCEnabled {
		if config.OIDCIssuerURL == "" {
			return fmt.Errorf("oidc_issuer_url is required when OIDC is enabled")
		}
		if config.OIDCClientID == "" {
			return fmt.Errorf("oidc_client_id is required when OIDC is enabled")
		}
		if config.OIDCClientSecret == "" {
			return fmt.Errorf("oidc_client_secret is required when OIDC is enabled")
		}
		if config.OIDCAutoCreateUsers && config.OIDCAllowedDomains == "" {
			return fmt.Errorf("oidc_allowed_domains is required when OIDC auto-create is enabled")
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
	var smtpHost, smtpUsername, smtpPassword, smtpFromEmail, smtpFromName, smtpEHLOHostname string
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
		smtpEHLOHostname = s.envConfig.SMTPEHLOHostname
	} else {
		// Use user-provided SMTP
		smtpHost = config.SMTPHost
		smtpPort = config.SMTPPort
		smtpUsername = config.SMTPUsername
		smtpPassword = config.SMTPPassword
		smtpFromEmail = config.SMTPFromEmail
		smtpFromName = config.SMTPFromName
		smtpUseTLS = config.SMTPUseTLS
		smtpEHLOHostname = config.SMTPEHLOHostname
	}

	// Handle SMTP Bridge configuration
	var smtpBridgeEnabled bool
	var smtpBridgeDomain, smtpBridgeTLSCertBase64, smtpBridgeTLSKeyBase64 string
	var smtpBridgePort int

	if status.SMTPBridgeConfigured {
		// Use env-configured SMTP Bridge (parse string to bool)
		smtpBridgeEnabled = s.envConfig.SMTPBridgeEnabled == "true"
		smtpBridgeDomain = s.envConfig.SMTPBridgeDomain
		smtpBridgePort = s.envConfig.SMTPBridgePort
		smtpBridgeTLSCertBase64 = s.envConfig.SMTPBridgeTLSCertBase64
		smtpBridgeTLSKeyBase64 = s.envConfig.SMTPBridgeTLSKeyBase64
	} else {
		// Use user-provided SMTP Bridge
		smtpBridgeEnabled = config.SMTPBridgeEnabled
		smtpBridgeDomain = config.SMTPBridgeDomain
		smtpBridgePort = config.SMTPBridgePort
		smtpBridgeTLSCertBase64 = config.SMTPBridgeTLSCertBase64
		smtpBridgeTLSKeyBase64 = config.SMTPBridgeTLSKeyBase64
	}

	// Handle OIDC configuration (env wins, else user-provided).
	var oidcEnabled, oidcAutoCreate bool
	var oidcIssuerURL, oidcClientID, oidcClientSecret, oidcRedirectURI, oidcScopes, oidcButtonLabel, oidcAllowedDomains string
	if status.OIDCConfigured {
		oidcEnabled = s.envConfig.OIDCEnabled == "true"
		oidcIssuerURL = s.envConfig.OIDCIssuerURL
		oidcClientID = s.envConfig.OIDCClientID
		oidcClientSecret = s.envConfig.OIDCClientSecret
		oidcRedirectURI = s.envConfig.OIDCRedirectURI
		oidcScopes = s.envConfig.OIDCScopes
		oidcButtonLabel = s.envConfig.OIDCButtonLabel
		oidcAutoCreate = s.envConfig.OIDCAutoCreateUsers == "true"
		oidcAllowedDomains = s.envConfig.OIDCAllowedDomains
	} else {
		oidcEnabled = config.OIDCEnabled
		oidcIssuerURL = config.OIDCIssuerURL
		oidcClientID = config.OIDCClientID
		oidcClientSecret = config.OIDCClientSecret
		oidcRedirectURI = config.OIDCRedirectURI
		oidcScopes = config.OIDCScopes
		oidcButtonLabel = config.OIDCButtonLabel
		oidcAutoCreate = config.OIDCAutoCreateUsers
		oidcAllowedDomains = config.OIDCAllowedDomains
	}
	// Canonicalize before persisting: empty/token-less input becomes the full
	// default (see NormalizeScopesForStorage — a stored bare value would override
	// the richer default at boot), "openid" is always forced in.
	if oidcEnabled {
		oidcScopes = appconfig.NormalizeScopesForStorage(oidcScopes)
	}

	// Store system settings
	systemConfig := &SystemConfig{
		IsInstalled:             true,
		RootEmail:               finalConfig.RootEmail,
		APIEndpoint:             finalConfig.APIEndpoint,
		SMTPHost:                smtpHost,
		SMTPPort:                smtpPort,
		SMTPUsername:            smtpUsername,
		SMTPPassword:            smtpPassword,
		SMTPFromEmail:           smtpFromEmail,
		SMTPFromName:            smtpFromName,
		SMTPUseTLS:              smtpUseTLS,
		SMTPEHLOHostname:        smtpEHLOHostname,
		TelemetryEnabled:        config.TelemetryEnabled,
		CheckForUpdates:         config.CheckForUpdates,
		SMTPBridgeEnabled:       smtpBridgeEnabled,
		SMTPBridgeDomain:        smtpBridgeDomain,
		SMTPBridgePort:          smtpBridgePort,
		SMTPBridgeTLSCertBase64: smtpBridgeTLSCertBase64,
		SMTPBridgeTLSKeyBase64:  smtpBridgeTLSKeyBase64,
		OIDCEnabled:             oidcEnabled,
		OIDCIssuerURL:           oidcIssuerURL,
		OIDCClientID:            oidcClientID,
		OIDCClientSecret:        oidcClientSecret,
		OIDCRedirectURI:         oidcRedirectURI,
		OIDCScopes:              oidcScopes,
		OIDCButtonLabel:         oidcButtonLabel,
		OIDCAutoCreateUsers:     oidcAutoCreate,
		OIDCAllowedDomains:      oidcAllowedDomains,
	}

	if err := s.settingService.SetSystemConfig(ctx, systemConfig, s.secretKey); err != nil {
		return fmt.Errorf("failed to save system configuration: %w", err)
	}

	// Create the primary root user. When ROOT_EMAIL holds a list, the first
	// email is the primary; additional roots are created on startup by
	// InitializeDatabase. Using the raw list string here would create a user
	// with an invalid comma-joined email.
	primaryRootEmail := appconfig.PrimaryRootEmail(finalConfig.RootEmail)
	rootUser := &domain.User{
		ID:        uuid.New().String(),
		Email:     primaryRootEmail,
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
		s.logger.WithField("email", primaryRootEmail).Info("Root user already exists, skipping creation")
	}

	s.logger.WithField("email", primaryRootEmail).Info("Setup wizard completed successfully")

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

	// On the SMTPS port the server expects TLS from the first byte; WithSSL
	// makes go-mail dial TLS-first and skip STARTTLS negotiation.
	if config.UseTLS && config.Port == implicitTLSPort {
		clientOptions = append(clientOptions, mail.WithSSL())
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

	// Set custom EHLO hostname if configured
	if config.EHLOHostname != "" {
		clientOptions = append(clientOptions, mail.WithHELO(config.EHLOHostname))
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
