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
	"github.com/asaskevich/govalidator"
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

	// Telemetry and CheckForUpdates are "true"/"false" when TELEMETRY /
	// CHECK_FOR_UPDATES is set, "" when unset.
	Telemetry       string
	CheckForUpdates string
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

	if s.envConfig.Telemetry != "" {
		result["telemetry_enabled"] = true
	}
	if s.envConfig.CheckForUpdates != "" {
		result["check_for_updates"] = true
	}

	return result
}

// applyEnvValues replaces, in place, every field of config whose environment variable
// is set, the way the runtime config does at boot (a non-empty env var wins, field by
// field). The setup wizard leaves a field set by the environment out of its request, so
// validation and storage must see the value the instance will actually run with. It is
// idempotent. Initialize stores the result for every group, set by the environment or not.
func (s *SetupService) applyEnvValues(config *SetupConfig) {
	env := s.envConfig
	if env == nil {
		return
	}
	str := func(field *string, value string) {
		if value != "" {
			*field = value
		}
	}
	tri := func(field *bool, value string) {
		if value != "" {
			*field = value == "true"
		}
	}

	str(&config.RootEmail, env.RootEmail)
	str(&config.APIEndpoint, env.APIEndpoint)

	str(&config.SMTPHost, env.SMTPHost)
	if env.SMTPPort > 0 {
		config.SMTPPort = env.SMTPPort
	}
	str(&config.SMTPUsername, env.SMTPUsername)
	str(&config.SMTPPassword, env.SMTPPassword)
	str(&config.SMTPFromEmail, env.SMTPFromEmail)
	str(&config.SMTPFromName, env.SMTPFromName)
	if env.SMTPUseTLS != "" {
		config.SMTPUseTLS = env.SMTPUseTLS != "false"
	}
	str(&config.SMTPEHLOHostname, env.SMTPEHLOHostname)

	tri(&config.SMTPBridgeEnabled, env.SMTPBridgeEnabled)
	str(&config.SMTPBridgeDomain, env.SMTPBridgeDomain)
	if env.SMTPBridgePort > 0 {
		config.SMTPBridgePort = env.SMTPBridgePort
	}
	str(&config.SMTPBridgeTLSCertBase64, env.SMTPBridgeTLSCertBase64)
	str(&config.SMTPBridgeTLSKeyBase64, env.SMTPBridgeTLSKeyBase64)

	tri(&config.OIDCEnabled, env.OIDCEnabled)
	str(&config.OIDCIssuerURL, env.OIDCIssuerURL)
	str(&config.OIDCClientID, env.OIDCClientID)
	str(&config.OIDCClientSecret, env.OIDCClientSecret)
	str(&config.OIDCRedirectURI, env.OIDCRedirectURI)
	str(&config.OIDCScopes, env.OIDCScopes)
	str(&config.OIDCButtonLabel, env.OIDCButtonLabel)
	tri(&config.OIDCAutoCreateUsers, env.OIDCAutoCreateUsers)
	str(&config.OIDCAllowedDomains, env.OIDCAllowedDomains)

	tri(&config.TelemetryEnabled, env.Telemetry)
	tri(&config.CheckForUpdates, env.CheckForUpdates)
}

// ValidateSetupConfig applies the environment to config (applyEnvValues), then checks
// the result for every group: Initialize stores it whether or not a group is set by the
// environment, so a value the request adds to an env-set group is checked too.
func (s *SetupService) ValidateSetupConfig(config *SetupConfig) error {
	s.applyEnvValues(config)

	// One or more root emails, separated as ROOT_EMAIL is (commas, semicolons or spaces).
	rootEmails := appconfig.ParseRootEmails(config.RootEmail)
	if len(rootEmails) == 0 {
		return fmt.Errorf("root_email is required")
	}
	for _, email := range rootEmails {
		if !govalidator.IsEmail(email) {
			return fmt.Errorf("root_email contains an invalid email: %s", email)
		}
	}

	if config.SMTPHost == "" {
		return fmt.Errorf("smtp_host is required")
	}
	if config.SMTPPort == 0 {
		config.SMTPPort = 587 // Default
	}
	if config.SMTPFromEmail == "" {
		return fmt.Errorf("smtp_from_email is required")
	}
	if err := s.ValidateSMTPCredentials(config.SMTPUsername, config.SMTPPassword); err != nil {
		return err
	}

	// The checks the boot makes (config.OIDCConfig.Validate), so that an incomplete SSO is
	// refused here rather than by a server that then fails to start.
	if config.OIDCEnabled {
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

	// config now holds what the instance will run with: the environment applied field
	// by field (a non-empty variable wins, as at boot), the request filling the rest,
	// whether or not a whole group is set by the environment. ValidateSetupConfig
	// applies it too; applying it again changes nothing.
	s.applyEnvValues(config)
	apiEndpoint := strings.TrimRight(config.APIEndpoint, "/")
	oidcScopes := config.OIDCScopes
	// Canonicalize before persisting: empty/token-less input becomes the full
	// default (see NormalizeScopesForStorage — a stored bare value would override
	// the richer default at boot), "openid" is always forced in.
	if config.OIDCEnabled {
		oidcScopes = appconfig.NormalizeScopesForStorage(oidcScopes)
	}

	// Store system settings
	systemConfig := &SystemConfig{
		IsInstalled:             true,
		AuditLogsRetentionDays:  domain.DefaultAuditLogRetentionDays,
		RootEmail:               config.RootEmail,
		APIEndpoint:             apiEndpoint,
		SMTPHost:                config.SMTPHost,
		SMTPPort:                config.SMTPPort,
		SMTPUsername:            config.SMTPUsername,
		SMTPPassword:            config.SMTPPassword,
		SMTPFromEmail:           config.SMTPFromEmail,
		SMTPFromName:            config.SMTPFromName,
		SMTPUseTLS:              config.SMTPUseTLS,
		SMTPEHLOHostname:        config.SMTPEHLOHostname,
		TelemetryEnabled:        config.TelemetryEnabled,
		CheckForUpdates:         config.CheckForUpdates,
		SMTPBridgeEnabled:       config.SMTPBridgeEnabled,
		SMTPBridgeDomain:        config.SMTPBridgeDomain,
		SMTPBridgePort:          config.SMTPBridgePort,
		SMTPBridgeTLSCertBase64: config.SMTPBridgeTLSCertBase64,
		SMTPBridgeTLSKeyBase64:  config.SMTPBridgeTLSKeyBase64,
		OIDCEnabled:             config.OIDCEnabled,
		OIDCIssuerURL:           config.OIDCIssuerURL,
		OIDCClientID:            config.OIDCClientID,
		OIDCClientSecret:        config.OIDCClientSecret,
		OIDCRedirectURI:         config.OIDCRedirectURI,
		OIDCScopes:              oidcScopes,
		OIDCButtonLabel:         config.OIDCButtonLabel,
		OIDCAutoCreateUsers:     config.OIDCAutoCreateUsers,
		OIDCAllowedDomains:      config.OIDCAllowedDomains,
	}

	if err := s.settingService.SetSystemConfig(ctx, systemConfig, s.secretKey); err != nil {
		return fmt.Errorf("failed to save system configuration: %w", err)
	}

	// Create the primary root user. When ROOT_EMAIL holds a list, the first
	// email is the primary; additional roots are created on startup by
	// InitializeDatabase. Using the raw list string here would create a user
	// with an invalid comma-joined email.
	primaryRootEmail := appconfig.PrimaryRootEmail(config.RootEmail)
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

	// Test the credentials the mailer will use, not just the ones typed: an env-provided
	// password the form cannot show would otherwise make a working pair look half-filled.
	username, password := s.effectiveSMTPCredentials(config.Username, config.Password)
	if err := ValidateSMTPCredentialPair(username, password); err != nil {
		return err
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
	if username != "" && password != "" {
		clientOptions = append(clientOptions,
			mail.WithUsername(username),
			mail.WithPassword(password),
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

// ValidateSMTPCredentials rejects a half-filled SMTP username/password pair, judged on the
// credentials the mailer will actually use once env overrides are applied.
func (s *SetupService) ValidateSMTPCredentials(username, password string) error {
	return ValidateSMTPCredentialPair(s.effectiveSMTPCredentials(username, password))
}

// effectiveSMTPCredentials applies env overrides field by field, as config.go does when it
// builds the runtime SMTP config: a non-empty env var wins over the stored or typed value.
func (s *SetupService) effectiveSMTPCredentials(username, password string) (string, string) {
	if s.envConfig == nil {
		return username, password
	}
	if s.envConfig.SMTPUsername != "" {
		username = s.envConfig.SMTPUsername
	}
	if s.envConfig.SMTPPassword != "" {
		password = s.envConfig.SMTPPassword
	}
	return username, password
}

// ValidateSMTPCredentialPair rejects a username without a password, or the reverse.
//
// The mailer only authenticates when both are set, so a half-filled pair silently connects
// anonymously: a connection test passes, then the server refuses the first real send.
// Both empty stays valid — that is how a relay that needs no login is configured.
func ValidateSMTPCredentialPair(username, password string) error {
	if username != "" && password == "" {
		return fmt.Errorf("SMTP password is required when a username is set: without it, Notifuse connects without logging in and the server can refuse to send. Leave both empty only if your server needs no login")
	}
	if username == "" && password != "" {
		return fmt.Errorf("SMTP username is required when a password is set: without it, Notifuse connects without logging in and the server can refuse to send")
	}
	return nil
}
