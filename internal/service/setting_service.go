package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/branding"
	"github.com/Notifuse/notifuse/pkg/crypto"
)

// InstanceBrandingSettingKey is the settings row holding the instance's own identity, as one
// JSON value so the four fields are written together.
const InstanceBrandingSettingKey = "instance_branding"

// SystemConfig holds all system-level configuration
type SystemConfig struct {
	IsInstalled      bool
	RootEmail        string
	APIEndpoint      string
	SMTPHost         string
	SMTPPort         int
	SMTPUsername     string
	SMTPPassword     string
	SMTPFromEmail    string
	SMTPFromName     string
	SMTPUseTLS       bool
	SMTPEHLOHostname string
	TelemetryEnabled bool
	CheckForUpdates  bool
	// RequireTwoFactor is the require_two_factor setting: every person on the instance
	// must pass a second factor. On write, nil keeps the stored value. REQUIRE_TWO_FACTOR,
	// when set, overrides it (config.RequireTwoFactor is the value in force).
	RequireTwoFactor *bool
	// AuditLogsRetentionDays is the deployment default retention for the audit
	// log: 0 keeps forever, absent means domain.DefaultAuditLogRetentionDays.
	AuditLogsRetentionDays  int
	SMTPBridgeEnabled       bool
	SMTPBridgeDomain        string
	SMTPBridgePort          int
	SMTPBridgeTLSCertBase64 string
	SMTPBridgeTLSKeyBase64  string

	// OIDC settings (client secret encrypted at rest).
	OIDCEnabled         bool
	OIDCIssuerURL       string
	OIDCClientID        string
	OIDCClientSecret    string
	OIDCRedirectURI     string
	OIDCScopes          string
	OIDCButtonLabel     string
	OIDCAutoCreateUsers bool
	OIDCAllowedDomains  string

	// Instance branding, stored together under InstanceBrandingSettingKey. Kept as flat
	// fields so the settings.update audit row records one change per field. SetSystemConfig
	// always writes them, so a caller passes the stored values for the fields it leaves alone.
	BrandingName          string
	BrandingLogoURL       string
	BrandingIconURL       string
	BrandingHideHelpMenu  bool
	BrandingBackgroundURL string
	// BrandingUnreadable is set by GetSystemConfig when the stored brand is not readable JSON:
	// the five fields above are then empty, and the rest of the settings load normally.
	BrandingUnreadable bool `json:"-"`
	// KeepStoredBranding makes SetSystemConfig leave the stored brand row exactly as it is,
	// for a save that does not touch the brand while that row is unreadable.
	KeepStoredBranding bool `json:"-"`
}

// brandingFrom reads the five branding fields of a config, normalised.
func brandingFrom(c *SystemConfig) branding.Branding {
	return branding.Branding{
		Name:          c.BrandingName,
		LogoURL:       c.BrandingLogoURL,
		IconURL:       c.BrandingIconURL,
		HideHelpMenu:  c.BrandingHideHelpMenu,
		BackgroundURL: c.BrandingBackgroundURL,
	}.Normalize()
}

// ErrTwoFactorRequiredForRoot: the root user turns on the instance's two-factor
// requirement only once their own account has it, so that the switch cannot lock them out.
var ErrTwoFactorRequiredForRoot = errors.New("turn on two-factor authentication on your own account first")

// CheckRequireTwoFactorChange refuses turning the instance's two-factor requirement on from
// a root session that would not meet it: the root user must have two-factor authentication,
// unless the session was opened by single sign-on or the root signature, which meet the
// requirement on their own. Turning it off, or leaving it as it is, is always allowed here.
func CheckRequireTwoFactorChange(current bool, next *bool, session domain.SessionAuth, rootHasTwoFactor bool) error {
	if next == nil || !*next || current {
		return nil
	}
	if session.AuthMethod == domain.AuthMethodSSO || session.AuthMethod == domain.AuthMethodRootSignature {
		return nil
	}
	if !rootHasTwoFactor || !domain.SessionSatisfiesTwoFactor(session) {
		return ErrTwoFactorRequiredForRoot
	}
	return nil
}

// SettingService provides methods for managing system settings
type SettingService struct {
	repo         domain.SettingRepository
	entitlements domain.EntitlementProvider
}

// NewSettingService creates a new SettingService. The entitlement provider gates instance
// branding writes; nil means unlicensed, so a construction that forgets it refuses a brand
// rather than granting one.
func NewSettingService(repo domain.SettingRepository, entitlements domain.EntitlementProvider) *SettingService {
	return &SettingService{
		repo:         repo,
		entitlements: entitlements,
	}
}

// isLicensedFor answers the one licence question this service asks. Unlike the older services,
// a nil provider counts as unlicensed: refusing a brand is the safe failure here.
func (s *SettingService) isLicensedFor(feature domain.Feature) bool {
	if s.entitlements == nil {
		return false
	}
	return s.entitlements.Entitlements().Has(feature)
}

// GetInstanceBranding reads the stored instance identity on its own, without decrypting any other
// setting. An absent row is the default identity. Only the JSON is checked, so a brand saved under
// older rules still reads. A database error is returned as is, and a row that is not JSON returns
// an error wrapping branding.ErrInvalidStored: neither is ever turned into an empty identity here.
func (s *SettingService) GetInstanceBranding(ctx context.Context) (branding.Branding, error) {
	setting, err := s.repo.Get(ctx, InstanceBrandingSettingKey)
	if err != nil {
		var notFound *domain.ErrSettingNotFound
		if errors.As(err, &notFound) {
			return branding.Branding{}, nil
		}
		return branding.Branding{}, fmt.Errorf("failed to get %s: %w", InstanceBrandingSettingKey, err)
	}
	if strings.TrimSpace(setting.Value) == "" {
		return branding.Branding{}, nil
	}
	return branding.Decode([]byte(setting.Value))
}

// checkBrandingWrite validates what a save changes in the brand and refuses, without the branding
// feature, any save that sets or changes a value. Clearing a field, unticking the Help switch and
// sending the stored values back unchanged are always allowed. An unreadable stored row counts as
// an empty brand here: the save replaces it.
func (s *SettingService) checkBrandingWrite(ctx context.Context, next branding.Branding) error {
	if next == (branding.Branding{}) {
		return nil
	}
	stored, err := s.GetInstanceBranding(ctx)
	if errors.Is(err, branding.ErrInvalidStored) {
		stored = branding.Branding{}
	} else if err != nil {
		return err
	}
	if err := next.ValidateChanged(stored); err != nil {
		return err
	}
	if s.isLicensedFor(domain.FeatureBranding) {
		return nil
	}
	widens := func(nextValue, storedValue string) bool {
		return nextValue != "" && nextValue != storedValue
	}
	if widens(next.Name, stored.Name) || widens(next.LogoURL, stored.LogoURL) ||
		widens(next.IconURL, stored.IconURL) || widens(next.BackgroundURL, stored.BackgroundURL) ||
		(next.HideHelpMenu && !stored.HideHelpMenu) {
		return domain.NewFeatureNotLicensedError(domain.FeatureBranding)
	}
	return nil
}

// GetSystemConfig loads all system settings from the database
func (s *SettingService) GetSystemConfig(ctx context.Context, secretKey string) (*SystemConfig, error) {
	config := &SystemConfig{
		// Absent means the default; the settings row, when present, overrides below.
		AuditLogsRetentionDays: domain.DefaultAuditLogRetentionDays,
		IsInstalled:            false,
		SMTPPort:               587,  // Default
		SMTPUseTLS:             true, // Default to TLS enabled
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

	instanceBranding, err := s.GetInstanceBranding(ctx)
	switch {
	case errors.Is(err, branding.ErrInvalidStored):
		// The rest of the settings stay readable and editable; the console says the brand is
		// unreadable, and a save that sets or clears it replaces the row.
		config.BrandingUnreadable = true
	case err != nil:
		return nil, err
	}
	config.BrandingName = instanceBranding.Name
	config.BrandingLogoURL = instanceBranding.LogoURL
	config.BrandingIconURL = instanceBranding.IconURL
	config.BrandingHideHelpMenu = instanceBranding.HideHelpMenu
	config.BrandingBackgroundURL = instanceBranding.BackgroundURL

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

	// Load the instance's two-factor requirement (REQUIRE_TWO_FACTOR overrides it)
	if setting, err := s.repo.Get(ctx, "require_two_factor"); err == nil {
		required := setting.Value == "true"
		config.RequireTwoFactor = &required
	}

	// Load check for updates setting
	if setting, err := s.repo.Get(ctx, "check_for_updates"); err == nil {
		config.CheckForUpdates = setting.Value == "true"
	}

	// Audit log retention: absent or unreadable keeps the default set above.
	if setting, err := s.repo.Get(ctx, domain.AuditLogsRetentionSettingKey); err == nil {
		if days, err := strconv.Atoi(strings.TrimSpace(setting.Value)); err == nil && domain.ValidateAuditRetentionDays(days) == nil {
			config.AuditLogsRetentionDays = days
		}
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

	// OIDC settings
	if setting, err := s.repo.Get(ctx, "oidc_enabled"); err == nil {
		config.OIDCEnabled = setting.Value == "true"
	}
	if setting, err := s.repo.Get(ctx, "oidc_issuer_url"); err == nil {
		config.OIDCIssuerURL = setting.Value
	}
	if setting, err := s.repo.Get(ctx, "oidc_client_id"); err == nil {
		config.OIDCClientID = setting.Value
	}
	if setting, err := s.repo.Get(ctx, "oidc_redirect_uri"); err == nil {
		config.OIDCRedirectURI = setting.Value
	}
	if setting, err := s.repo.Get(ctx, "oidc_scopes"); err == nil {
		config.OIDCScopes = setting.Value
	}
	if setting, err := s.repo.Get(ctx, "oidc_button_label"); err == nil {
		config.OIDCButtonLabel = setting.Value
	}
	if setting, err := s.repo.Get(ctx, "oidc_auto_create_users"); err == nil {
		config.OIDCAutoCreateUsers = setting.Value == "true"
	}
	if setting, err := s.repo.Get(ctx, "oidc_allowed_domains"); err == nil {
		config.OIDCAllowedDomains = setting.Value
	}
	if setting, err := s.repo.Get(ctx, "encrypted_oidc_client_secret"); err == nil && setting.Value != "" {
		decrypted, err := crypto.DecryptFromHexString(setting.Value, secretKey)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt OIDC client secret: %w", err)
		}
		config.OIDCClientSecret = decrypted
	}

	return config, nil
}

// SetSystemConfig stores all system settings in the database. Everything that can refuse the save
// is checked before the first write, so a refused save leaves every setting as it was.
func (s *SettingService) SetSystemConfig(ctx context.Context, config *SystemConfig, secretKey string) error {
	if err := domain.ValidateAuditRetentionDays(config.AuditLogsRetentionDays); err != nil {
		return err
	}
	var encodedBranding []byte
	if !config.KeepStoredBranding {
		nextBranding := brandingFrom(config)
		if err := s.checkBrandingWrite(ctx, nextBranding); err != nil {
			return err
		}
		encoded, err := nextBranding.Encode()
		if err != nil {
			return fmt.Errorf("failed to encode %s: %w", InstanceBrandingSettingKey, err)
		}
		encodedBranding = encoded
	}

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

	// Always write smtp_from_name (allow clearing)
	if err := s.repo.Set(ctx, "smtp_from_name", config.SMTPFromName); err != nil {
		return fmt.Errorf("failed to set smtp_from_name: %w", err)
	}

	// Always write smtp_ehlo_hostname (allow clearing)
	if err := s.repo.Set(ctx, "smtp_ehlo_hostname", config.SMTPEHLOHostname); err != nil {
		return fmt.Errorf("failed to set smtp_ehlo_hostname: %w", err)
	}

	// Set SMTP TLS setting
	smtpUseTLSValue := "true"
	if !config.SMTPUseTLS {
		smtpUseTLSValue = "false"
	}
	if err := s.repo.Set(ctx, "smtp_use_tls", smtpUseTLSValue); err != nil {
		return fmt.Errorf("failed to set smtp_use_tls: %w", err)
	}

	// Encrypt and store SMTP username (allow clearing)
	if config.SMTPUsername != "" {
		encrypted, err := crypto.EncryptString(config.SMTPUsername, secretKey)
		if err != nil {
			return fmt.Errorf("failed to encrypt SMTP username: %w", err)
		}
		if err := s.repo.Set(ctx, "encrypted_smtp_username", encrypted); err != nil {
			return fmt.Errorf("failed to set encrypted_smtp_username: %w", err)
		}
	} else {
		if err := s.repo.Set(ctx, "encrypted_smtp_username", ""); err != nil {
			return fmt.Errorf("failed to clear encrypted_smtp_username: %w", err)
		}
	}

	// Encrypt and store SMTP password (allow clearing)
	if config.SMTPPassword != "" {
		encrypted, err := crypto.EncryptString(config.SMTPPassword, secretKey)
		if err != nil {
			return fmt.Errorf("failed to encrypt SMTP password: %w", err)
		}
		if err := s.repo.Set(ctx, "encrypted_smtp_password", encrypted); err != nil {
			return fmt.Errorf("failed to set encrypted_smtp_password: %w", err)
		}
	} else {
		if err := s.repo.Set(ctx, "encrypted_smtp_password", ""); err != nil {
			return fmt.Errorf("failed to clear encrypted_smtp_password: %w", err)
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

	if config.RequireTwoFactor != nil {
		if err := s.repo.Set(ctx, "require_two_factor", strconv.FormatBool(*config.RequireTwoFactor)); err != nil {
			return fmt.Errorf("failed to set require_two_factor: %w", err)
		}
	}

	// Audit log retention (deployment default), validated above
	if err := s.repo.Set(ctx, domain.AuditLogsRetentionSettingKey, strconv.Itoa(config.AuditLogsRetentionDays)); err != nil {
		return fmt.Errorf("failed to set %s: %w", domain.AuditLogsRetentionSettingKey, err)
	}

	// Set SMTP Bridge enabled
	smtpBridgeEnabledValue := "false"
	if config.SMTPBridgeEnabled {
		smtpBridgeEnabledValue = "true"
	}
	if err := s.repo.Set(ctx, "smtp_bridge_enabled", smtpBridgeEnabledValue); err != nil {
		return fmt.Errorf("failed to set smtp_bridge_enabled: %w", err)
	}

	// Always write smtp_bridge_domain (allow clearing)
	if err := s.repo.Set(ctx, "smtp_bridge_domain", config.SMTPBridgeDomain); err != nil {
		return fmt.Errorf("failed to set smtp_bridge_domain: %w", err)
	}

	// Set SMTP Bridge port
	if config.SMTPBridgePort > 0 {
		if err := s.repo.Set(ctx, "smtp_bridge_port", strconv.Itoa(config.SMTPBridgePort)); err != nil {
			return fmt.Errorf("failed to set smtp_bridge_port: %w", err)
		}
	}

	// Encrypt and store SMTP Bridge TLS certificate (allow clearing)
	if config.SMTPBridgeTLSCertBase64 != "" {
		encrypted, err := crypto.EncryptString(config.SMTPBridgeTLSCertBase64, secretKey)
		if err != nil {
			return fmt.Errorf("failed to encrypt SMTP bridge TLS certificate: %w", err)
		}
		if err := s.repo.Set(ctx, "encrypted_smtp_bridge_tls_cert_base64", encrypted); err != nil {
			return fmt.Errorf("failed to set encrypted_smtp_bridge_tls_cert_base64: %w", err)
		}
	} else {
		if err := s.repo.Set(ctx, "encrypted_smtp_bridge_tls_cert_base64", ""); err != nil {
			return fmt.Errorf("failed to clear encrypted_smtp_bridge_tls_cert_base64: %w", err)
		}
	}

	// Encrypt and store SMTP Bridge TLS key (allow clearing)
	if config.SMTPBridgeTLSKeyBase64 != "" {
		encrypted, err := crypto.EncryptString(config.SMTPBridgeTLSKeyBase64, secretKey)
		if err != nil {
			return fmt.Errorf("failed to encrypt SMTP bridge TLS key: %w", err)
		}
		if err := s.repo.Set(ctx, "encrypted_smtp_bridge_tls_key_base64", encrypted); err != nil {
			return fmt.Errorf("failed to set encrypted_smtp_bridge_tls_key_base64: %w", err)
		}
	} else {
		if err := s.repo.Set(ctx, "encrypted_smtp_bridge_tls_key_base64", ""); err != nil {
			return fmt.Errorf("failed to clear encrypted_smtp_bridge_tls_key_base64: %w", err)
		}
	}

	// OIDC settings
	oidcEnabledVal := "false"
	if config.OIDCEnabled {
		oidcEnabledVal = "true"
	}
	if err := s.repo.Set(ctx, "oidc_enabled", oidcEnabledVal); err != nil {
		return fmt.Errorf("failed to set oidc_enabled: %w", err)
	}
	if err := s.repo.Set(ctx, "oidc_issuer_url", config.OIDCIssuerURL); err != nil {
		return fmt.Errorf("failed to set oidc_issuer_url: %w", err)
	}
	if err := s.repo.Set(ctx, "oidc_client_id", config.OIDCClientID); err != nil {
		return fmt.Errorf("failed to set oidc_client_id: %w", err)
	}
	if err := s.repo.Set(ctx, "oidc_redirect_uri", config.OIDCRedirectURI); err != nil {
		return fmt.Errorf("failed to set oidc_redirect_uri: %w", err)
	}
	if err := s.repo.Set(ctx, "oidc_scopes", config.OIDCScopes); err != nil {
		return fmt.Errorf("failed to set oidc_scopes: %w", err)
	}
	if err := s.repo.Set(ctx, "oidc_button_label", config.OIDCButtonLabel); err != nil {
		return fmt.Errorf("failed to set oidc_button_label: %w", err)
	}
	oidcAutoVal := "false"
	if config.OIDCAutoCreateUsers {
		oidcAutoVal = "true"
	}
	if err := s.repo.Set(ctx, "oidc_auto_create_users", oidcAutoVal); err != nil {
		return fmt.Errorf("failed to set oidc_auto_create_users: %w", err)
	}
	if err := s.repo.Set(ctx, "oidc_allowed_domains", config.OIDCAllowedDomains); err != nil {
		return fmt.Errorf("failed to set oidc_allowed_domains: %w", err)
	}
	// Encrypt and store OIDC client secret (allow clearing)
	if config.OIDCClientSecret != "" {
		encrypted, err := crypto.EncryptString(config.OIDCClientSecret, secretKey)
		if err != nil {
			return fmt.Errorf("failed to encrypt OIDC client secret: %w", err)
		}
		if err := s.repo.Set(ctx, "encrypted_oidc_client_secret", encrypted); err != nil {
			return fmt.Errorf("failed to set encrypted_oidc_client_secret: %w", err)
		}
	} else {
		if err := s.repo.Set(ctx, "encrypted_oidc_client_secret", ""); err != nil {
			return fmt.Errorf("failed to clear encrypted_oidc_client_secret: %w", err)
		}
	}

	if !config.KeepStoredBranding {
		if err := s.repo.Set(ctx, InstanceBrandingSettingKey, string(encodedBranding)); err != nil {
			return fmt.Errorf("failed to set %s: %w", InstanceBrandingSettingKey, err)
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
