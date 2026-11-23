package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/http/middleware"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/pkg/license"
	"github.com/Notifuse/notifuse/pkg/logger"
)

const (
	passwordMask   = "\u2022\u2022\u2022\u2022\u2022\u2022\u2022\u2022" // ••••••••
	configuredMask = "[configured]"
)

// SystemSettingsData represents the editable system settings
type SystemSettingsData struct {
	RootEmail        string `json:"root_email"`
	APIEndpoint      string `json:"api_endpoint"`
	SMTPHost         string `json:"smtp_host"`
	SMTPPort         int    `json:"smtp_port"`
	SMTPUsername     string `json:"smtp_username"`
	SMTPPassword     string `json:"smtp_password"`
	SMTPFromEmail    string `json:"smtp_from_email"`
	SMTPFromName     string `json:"smtp_from_name"`
	SMTPUseTLS       bool   `json:"smtp_use_tls"`
	SMTPEHLOHostname string `json:"smtp_ehlo_hostname"`
	TelemetryEnabled bool   `json:"telemetry_enabled"`
	CheckForUpdates  bool   `json:"check_for_updates"`
	// Deployment default retention for the audit log, in days; 0 keeps forever.
	AuditLogsRetentionDays  int    `json:"audit_logs_retention_days"`
	SMTPBridgeEnabled       bool   `json:"smtp_bridge_enabled"`
	SMTPBridgeDomain        string `json:"smtp_bridge_domain"`
	SMTPBridgePort          int    `json:"smtp_bridge_port"`
	SMTPBridgeTLSCertBase64 string `json:"smtp_bridge_tls_cert_base64"`
	SMTPBridgeTLSKeyBase64  string `json:"smtp_bridge_tls_key_base64"`
	OIDCEnabled             bool   `json:"oidc_enabled"`
	OIDCIssuerURL           string `json:"oidc_issuer_url"`
	OIDCClientID            string `json:"oidc_client_id"`
	OIDCClientSecret        string `json:"oidc_client_secret"` // masked on GET
	OIDCRedirectURI         string `json:"oidc_redirect_uri"`
	OIDCScopes              string `json:"oidc_scopes"`
	OIDCButtonLabel         string `json:"oidc_button_label"`
	OIDCAutoCreateUsers     bool   `json:"oidc_auto_create_users"`
	OIDCAllowedDomains      string `json:"oidc_allowed_domains"`
	// RequireTwoFactor makes every person on the instance pass a second factor. On GET it
	// is the value in force, REQUIRE_TWO_FACTOR included.
	RequireTwoFactor bool `json:"require_two_factor"`
}

// TwoFactorSettings is what the Security panel shows beside the switch.
type TwoFactorSettings struct {
	Required bool `json:"required"`
	// FromEnv: REQUIRE_TWO_FACTOR sets the value, and the switch is read-only.
	FromEnv bool `json:"from_env"`
	// UsersWithout are the people (not API keys) who have not turned it on, for the root
	// user to warn before switching the requirement on.
	UsersWithout []string `json:"users_without"`
}

// SystemSettingsResponse wraps settings with env override info
type SystemSettingsResponse struct {
	Settings     SystemSettingsData `json:"settings"`
	EnvOverrides map[string]bool    `json:"env_overrides"`
	TwoFactor    TwoFactorSettings  `json:"two_factor"`
}

// Error code of a root user turning on the requirement without meeting it.
const twoFactorRequiredForRootCode = "two_factor_required_for_root"

// LicenseServiceInterface is the slice of the licence service the two licence endpoints
// need, declared here in the consuming package the way UserServiceInterface is.
//
// Two methods and no more. The handler reads the resolved grant and installs a pasted key;
// it has no way to reach the raw key, because the only safe answer to "what is my licence
// key" over HTTP is that the endpoint cannot say.
//
// service.LicenseService satisfies it.
type LicenseServiceInterface interface {
	Entitlements() domain.Entitlements
	SetKey(ctx context.Context, raw string) error
}

// LicenseResponse is the body of GET /api/licence.get, and of a successful
// POST /api/licence.set so the console can repaint from one round trip.
//
// It never carries the key. A licence key is a bearer credential — whoever holds it can
// license their own deployment with it — and an endpoint that echoed it back would copy it
// into every console session, browser cache, proxy log and support screenshot. Everything a
// console needs is already in the verified claims: who it is licensed to, what it grants,
// and until when.
type LicenseResponse struct {
	// Entitlements is the very value every gate reads, so what the console greys out and
	// what the backend refuses cannot drift apart.
	Entitlements domain.Entitlements `json:"entitlements"`
}

// SetLicenseRequest is the body of POST /api/licence.set: the envelope, verbatim.
//
// Nothing else is accepted. Tier, features and quota are read from the signature, never from
// the request — a body that could set them would be a licence generator with a JSON API.
type SetLicenseRequest struct {
	Key string `json:"key"`
}

// licenseKeyRejections are the pkg/license failures a pasted key can produce. All of them
// mean the paste is wrong rather than the server broken, so all of them answer 400 carrying
// the sentence pkg/license already wrote — those sentences are the entire support story for
// "I pasted a key and nothing happened", which is otherwise indistinguishable from "the key
// never arrived".
//
// Enumerated rather than inferred, because the only way to infer it is from the other side —
// treating everything that is not a storage failure as a bad key — and a storage failure has
// no sentinel to match on. A sentinel added to pkg/license and not added here degrades to a
// 500: the wrong status, but a safe one. Nothing is stored on either path.
var licenseKeyRejections = []error{
	license.ErrMalformedEnvelope,
	license.ErrBadEncoding,
	license.ErrNoTrustedKey,
	license.ErrBadSignature,
	license.ErrMalformedPayload,
	license.ErrUnknownVersion,
	license.ErrFutureIssuedAt,
}

// SettingsHandler handles system settings endpoints (root user only)
type SettingsHandler struct {
	setupService   *service.SetupService
	settingService *service.SettingService
	userService    UserServiceInterface
	licenseService LicenseServiceInterface
	getJWTSecret   func() ([]byte, error)
	logger         logger.Logger
	secretKey      string
	rootEmail      string
	app            AppShutdowner
	authService    domain.AuthService
	twoFactor      TwoFactorServiceInterface
	// requireTwoFactorEnv is REQUIRE_TWO_FACTOR: "true", "false", or "" when unset.
	requireTwoFactorEnv string
	// retentionDefaults serves dataRetention.getDefaults and setDefaults; nil answers 503.
	retentionDefaults retentionDefaultsService
}

// retentionDefaultsService is what the settings handler needs from DataRetentionService.
type retentionDefaultsService interface {
	Defaults(ctx context.Context) (*domain.RetentionDefaults, error)
	SetDefaults(ctx context.Context, categories map[domain.RetentionCategory]int) (*domain.RetentionDefaults, error)
}

// SetDataRetention supplies the deployment's data retention defaults (root only).
func (h *SettingsHandler) SetDataRetention(svc retentionDefaultsService) {
	h.retentionDefaults = svc
}

// GET /api/dataRetention.getDefaults — the deployment's default periods (root), and the
// change that waits, if any. Not part of settings.update: saving it needs no restart.
func (h *SettingsHandler) handleGetRetentionDefaults(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.requireRootUser(w, r) == nil {
		return
	}
	if h.retentionDefaults == nil {
		WriteJSONError(w, "Data retention is not available", http.StatusServiceUnavailable)
		return
	}
	stored, err := h.retentionDefaults.Defaults(r.Context())
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to read the data retention defaults")
		WriteJSONError(w, "Failed to read the data retention defaults", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"defaults": stored.ResolvedDefaults(time.Now()),
		"stored":   stored,
	})
}

// POST /api/dataRetention.setDefaults {"categories": {...}} — root. A default that deletes
// more waits 7 days and the owners of the workspaces it reaches are told.
func (h *SettingsHandler) handleSetRetentionDefaults(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.requireRootUser(w, r) == nil {
		return
	}
	if h.retentionDefaults == nil {
		WriteJSONError(w, "Data retention is not available", http.StatusServiceUnavailable)
		return
	}
	var req struct {
		Categories map[domain.RetentionCategory]int `json:"categories"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.Categories == nil {
		WriteJSONError(w, "categories is required", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	before, _ := h.retentionDefaults.Defaults(ctx) // for the audit diff only; SetDefaults reports a failed read
	saved, err := h.retentionDefaults.SetDefaults(ctx, req.Categories)
	if err != nil {
		if writeServiceError(w, err, "Failed to set the data retention defaults") {
			return
		}
		h.logger.WithField("error", err.Error()).Error("Failed to set the data retention defaults")
		WriteJSONError(w, "Failed to set the data retention defaults", http.StatusInternalServerError)
		return
	}
	if audit := domain.AuditFromContext(ctx); audit != nil {
		audit.SetTarget(domain.AuditTargetSettings, "data_retention_defaults", "")
		audit.SetChanges(domain.AuditDiff(before, saved))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"stored": saved, "defaults": saved.ResolvedDefaults(time.Now())})
}

// NewSettingsHandler creates a new settings handler
//
// The licence service is a REQUIRED positional parameter, not a trailing variadic. It used to
// be variadic, app.go omitted it, and the result was the worst outcome this design exists to
// prevent: gates refusing capabilities while /api/licence.set answered 500, so the operator
// had no way to buy their way out. Omitting the argument must be a compile error.
//
// The handler still tolerates a nil interface value at RUNTIME — see licenseResponse — because
// the settings page is the page an operator opens when something is already wrong and it must
// not be the second thing that fails. That is a fail-safe for a service that failed to build,
// not a licence for a caller to skip the argument.
func NewSettingsHandler(
	setupService *service.SetupService,
	settingService *service.SettingService,
	userService UserServiceInterface,
	getJWTSecret func() ([]byte, error),
	logger logger.Logger,
	secretKey string,
	rootEmail string,
	app AppShutdowner,
	licenseService LicenseServiceInterface,
	authService domain.AuthService,
	twoFactor TwoFactorServiceInterface,
	requireTwoFactorEnv string,
) *SettingsHandler {
	return &SettingsHandler{
		setupService:   setupService,
		settingService: settingService,
		userService:    userService,
		licenseService: licenseService,
		getJWTSecret:   getJWTSecret,
		logger:         logger,
		secretKey:      secretKey,
		rootEmail:      rootEmail,
		app:            app,
		authService:    authService,
		twoFactor:      twoFactor,

		requireTwoFactorEnv: requireTwoFactorEnv,
	}
}

// requireRootUser checks that the authenticated user is the root user.
// Returns the user on success, or writes an error response and returns nil.
func (h *SettingsHandler) requireRootUser(w http.ResponseWriter, r *http.Request) *domain.User {
	ctx := r.Context()

	userID, _ := ctx.Value(domain.UserIDKey).(string)
	if userID == "" {
		WriteJSONError(w, "Unauthorized", http.StatusUnauthorized)
		return nil
	}

	// Root is a person, so only a live session of a person's account gets through. An
	// API key is never root, whatever account it names: refused as a credential that
	// lacks the access, not as a dead one.
	if !isConsoleSession(ctx) {
		WriteJSONError(w, "Forbidden: root user access required", http.StatusForbidden)
		return nil
	}
	sessionID, _ := ctx.Value(domain.SessionIDKey).(string)
	if sessionID == "" {
		WriteJSONError(w, "Unauthorized", http.StatusUnauthorized)
		return nil
	}

	user, err := h.userService.VerifyUserSession(ctx, userID, sessionID)
	if err != nil {
		if sessionCheckStatus(err) != http.StatusUnauthorized {
			h.logger.WithField("error", err.Error()).Error("Failed to verify session")
			WriteJSONError(w, "Failed to verify session", http.StatusInternalServerError)
			return nil
		}
		WriteJSONError(w, "Unauthorized", http.StatusUnauthorized)
		return nil
	}

	if !config.IsRootEmail(h.rootEmail, user.Email) {
		WriteJSONError(w, "Forbidden: root user access required", http.StatusForbidden)
		return nil
	}

	// The instance's two-factor requirement, and, for a root user who turned two-factor
	// authentication on, a session that passed it: these routes change the root email, SMTP
	// and single sign-on.
	if err := h.authService.RequireCompliantRootSession(ctx); err != nil {
		if !writePermissionError(w, err) {
			h.logger.WithField("error", err.Error()).Error("Failed to verify the root session")
			WriteJSONError(w, "Failed to verify session", http.StatusInternalServerError)
		}
		return nil
	}

	return user
}

// handleGet returns the current system settings with env override info
func (h *SettingsHandler) handleGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.requireRootUser(w, r) == nil {
		return
	}

	ctx := r.Context()

	sysConfig, err := h.settingService.GetSystemConfig(ctx, h.secretKey)
	if err != nil {
		h.logger.WithField("error", err).Error("Failed to load system config")
		WriteJSONError(w, "Failed to load system settings", http.StatusInternalServerError)
		return
	}

	// Build response with masked sensitive fields
	settings := SystemSettingsData{
		RootEmail:         sysConfig.RootEmail,
		APIEndpoint:       sysConfig.APIEndpoint,
		SMTPHost:          sysConfig.SMTPHost,
		SMTPPort:          sysConfig.SMTPPort,
		SMTPUsername:      sysConfig.SMTPUsername,
		SMTPFromEmail:     sysConfig.SMTPFromEmail,
		SMTPFromName:      sysConfig.SMTPFromName,
		SMTPUseTLS:        sysConfig.SMTPUseTLS,
		SMTPEHLOHostname:  sysConfig.SMTPEHLOHostname,
		TelemetryEnabled:  sysConfig.TelemetryEnabled,
		CheckForUpdates:   sysConfig.CheckForUpdates,
		SMTPBridgeEnabled: sysConfig.SMTPBridgeEnabled,
		SMTPBridgeDomain:  sysConfig.SMTPBridgeDomain,
		SMTPBridgePort:    sysConfig.SMTPBridgePort,
		// OIDC non-secret fields (the client secret is masked below).
		OIDCEnabled:         sysConfig.OIDCEnabled,
		OIDCIssuerURL:       sysConfig.OIDCIssuerURL,
		OIDCClientID:        sysConfig.OIDCClientID,
		OIDCRedirectURI:     sysConfig.OIDCRedirectURI,
		OIDCScopes:          sysConfig.OIDCScopes,
		OIDCButtonLabel:     sysConfig.OIDCButtonLabel,
		OIDCAutoCreateUsers: sysConfig.OIDCAutoCreateUsers,
		OIDCAllowedDomains:  sysConfig.OIDCAllowedDomains,
	}

	envOverrides := h.setupService.GetEnvOverrides()

	// Env vars are authoritative and may differ from the values persisted to the DB at
	// install time (e.g. an operator added a second ROOT_EMAIL, or changed SMTP_HOST,
	// and restarted). GetSystemConfig only reflects the DB, so a field overridden by an
	// env var would otherwise show a stale value in the (disabled) panel. For every
	// env-overridden field, surface the live env value the rest of the app actually uses.
	// Secrets are overlaid into local vars and masked below — never returned in clear.
	smtpPassword := sysConfig.SMTPPassword
	tlsCert := sysConfig.SMTPBridgeTLSCertBase64
	tlsKey := sysConfig.SMTPBridgeTLSKeyBase64
	oidcSecret := sysConfig.OIDCClientSecret
	if env := h.setupService.GetEnvConfig(); env != nil {
		if envOverrides["root_email"] {
			settings.RootEmail = env.RootEmail
		}
		if envOverrides["api_endpoint"] {
			settings.APIEndpoint = env.APIEndpoint
		}
		if envOverrides["smtp_host"] {
			settings.SMTPHost = env.SMTPHost
		}
		if envOverrides["smtp_port"] {
			settings.SMTPPort = env.SMTPPort
		}
		if envOverrides["smtp_username"] {
			settings.SMTPUsername = env.SMTPUsername
		}
		if envOverrides["smtp_from_email"] {
			settings.SMTPFromEmail = env.SMTPFromEmail
		}
		if envOverrides["smtp_from_name"] {
			settings.SMTPFromName = env.SMTPFromName
		}
		if envOverrides["smtp_use_tls"] {
			settings.SMTPUseTLS = env.SMTPUseTLS != "false"
		}
		if envOverrides["smtp_ehlo_hostname"] {
			settings.SMTPEHLOHostname = env.SMTPEHLOHostname
		}
		if envOverrides["smtp_bridge_enabled"] {
			settings.SMTPBridgeEnabled = env.SMTPBridgeEnabled == "true"
		}
		if envOverrides["smtp_bridge_domain"] {
			settings.SMTPBridgeDomain = env.SMTPBridgeDomain
		}
		if envOverrides["smtp_bridge_port"] {
			settings.SMTPBridgePort = env.SMTPBridgePort
		}
		if envOverrides["smtp_password"] {
			smtpPassword = env.SMTPPassword
		}
		if envOverrides["smtp_bridge_tls_cert_base64"] {
			tlsCert = env.SMTPBridgeTLSCertBase64
		}
		if envOverrides["smtp_bridge_tls_key_base64"] {
			tlsKey = env.SMTPBridgeTLSKeyBase64
		}
		if envOverrides["oidc_enabled"] {
			settings.OIDCEnabled = env.OIDCEnabled == "true"
		}
		if envOverrides["oidc_issuer_url"] {
			settings.OIDCIssuerURL = env.OIDCIssuerURL
		}
		if envOverrides["oidc_client_id"] {
			settings.OIDCClientID = env.OIDCClientID
		}
		if envOverrides["oidc_redirect_uri"] {
			settings.OIDCRedirectURI = env.OIDCRedirectURI
		}
		if envOverrides["oidc_scopes"] {
			settings.OIDCScopes = env.OIDCScopes
		}
		if envOverrides["oidc_button_label"] {
			settings.OIDCButtonLabel = env.OIDCButtonLabel
		}
		if envOverrides["oidc_auto_create_users"] {
			settings.OIDCAutoCreateUsers = env.OIDCAutoCreateUsers == "true"
		}
		if envOverrides["oidc_allowed_domains"] {
			settings.OIDCAllowedDomains = env.OIDCAllowedDomains
		}
		if envOverrides["oidc_client_secret"] {
			oidcSecret = env.OIDCClientSecret
		}
	}

	// Mask sensitive fields based on the effective (post-override) value
	if smtpPassword != "" {
		settings.SMTPPassword = passwordMask
	}
	if tlsCert != "" {
		settings.SMTPBridgeTLSCertBase64 = configuredMask
	}
	if tlsKey != "" {
		settings.SMTPBridgeTLSKeyBase64 = configuredMask
	}
	if oidcSecret != "" {
		settings.OIDCClientSecret = passwordMask
	}

	settings.RequireTwoFactor = h.twoFactor.RequireTwoFactor()
	envOverrides["require_two_factor"] = h.requireTwoFactorEnv != ""
	// A list for the root user's information: a failure to read it must not close the
	// settings, which is where the root user repairs things.
	usersWithout, err := h.twoFactor.UsersWithoutTwoFactor(ctx)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to list the people without two-factor authentication")
		usersWithout = nil
	}
	if usersWithout == nil {
		usersWithout = []string{}
	}

	response := SystemSettingsResponse{
		Settings:     settings,
		EnvOverrides: envOverrides,
		TwoFactor: TwoFactorSettings{
			Required:     settings.RequireTwoFactor,
			FromEnv:      h.requireTwoFactorEnv != "",
			UsersWithout: usersWithout,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

// systemSettingsFromConfig projects the stored config onto the wire shape, so an update
// body can be decoded over it and only the keys it carries take effect. Secrets are
// included in the clear because the result is the base of a write path that stays in
// memory — unlike handleGet's response struct, this one is never serialized back out.
func systemSettingsFromConfig(c *service.SystemConfig) SystemSettingsData {
	return SystemSettingsData{
		RootEmail:               c.RootEmail,
		APIEndpoint:             c.APIEndpoint,
		SMTPHost:                c.SMTPHost,
		SMTPPort:                c.SMTPPort,
		SMTPUsername:            c.SMTPUsername,
		SMTPPassword:            c.SMTPPassword,
		SMTPFromEmail:           c.SMTPFromEmail,
		SMTPFromName:            c.SMTPFromName,
		SMTPUseTLS:              c.SMTPUseTLS,
		SMTPEHLOHostname:        c.SMTPEHLOHostname,
		TelemetryEnabled:        c.TelemetryEnabled,
		CheckForUpdates:         c.CheckForUpdates,
		AuditLogsRetentionDays:  c.AuditLogsRetentionDays,
		SMTPBridgeEnabled:       c.SMTPBridgeEnabled,
		SMTPBridgeDomain:        c.SMTPBridgeDomain,
		SMTPBridgePort:          c.SMTPBridgePort,
		SMTPBridgeTLSCertBase64: c.SMTPBridgeTLSCertBase64,
		SMTPBridgeTLSKeyBase64:  c.SMTPBridgeTLSKeyBase64,
		OIDCEnabled:             c.OIDCEnabled,
		OIDCIssuerURL:           c.OIDCIssuerURL,
		OIDCClientID:            c.OIDCClientID,
		OIDCClientSecret:        c.OIDCClientSecret,
		OIDCRedirectURI:         c.OIDCRedirectURI,
		OIDCScopes:              c.OIDCScopes,
		OIDCButtonLabel:         c.OIDCButtonLabel,
		OIDCAutoCreateUsers:     c.OIDCAutoCreateUsers,
		OIDCAllowedDomains:      c.OIDCAllowedDomains,
		RequireTwoFactor:        c.RequireTwoFactor != nil && *c.RequireTwoFactor,
	}
}

// handleUpdate updates system settings and triggers a server restart
func (h *SettingsHandler) handleUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.requireRootUser(w, r) == nil {
		return
	}

	ctx := r.Context()

	// Load current config to handle masked field round-trip, and to seed the request below
	currentConfig, err := h.settingService.GetSystemConfig(ctx, h.secretKey)
	if err != nil {
		h.logger.WithField("error", err).Error("Failed to load current system config")
		WriteJSONError(w, "Failed to load current settings", http.StatusInternalServerError)
		return
	}

	// Decode ONTO the stored settings rather than into a zero struct: json.Decode only
	// touches the keys the body actually carries, so an absent key keeps its stored value
	// instead of arriving as "" or false. No field here is optional-looking — every one has
	// a meaningful zero — and the console's form library submits only the fields it
	// registered, so a key it never registers is absent from every save. That is how
	// oidc_redirect_uri got blanked on each save, breaking SSO wherever the callback is not
	// derivable from api_endpoint (a reverse proxy, or a vanity SSO domain).
	//
	// Clearing a field stays expressible: the key is present, carrying the empty value.
	reqData := systemSettingsFromConfig(currentConfig)
	// The value in force, as settings.get shows it: a body without the key then changes
	// nothing, whether REQUIRE_TWO_FACTOR or the stored setting decides it.
	if h.requireTwoFactorEnv != "" {
		reqData.RequireTwoFactor = h.requireTwoFactorEnv == "true"
	}
	if err := json.NewDecoder(r.Body).Decode(&reqData); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Handle masked password: if sentinel value, retain existing
	smtpPassword := reqData.SMTPPassword
	if smtpPassword == passwordMask {
		smtpPassword = currentConfig.SMTPPassword
	}

	// Handle masked TLS cert/key: if sentinel value, retain existing
	tlsCert := reqData.SMTPBridgeTLSCertBase64
	if tlsCert == configuredMask {
		tlsCert = currentConfig.SMTPBridgeTLSCertBase64
	}

	tlsKey := reqData.SMTPBridgeTLSKeyBase64
	if tlsKey == configuredMask {
		tlsKey = currentConfig.SMTPBridgeTLSKeyBase64
	}

	// Handle masked OIDC client secret: if sentinel value, retain existing.
	oidcClientSecret := reqData.OIDCClientSecret
	if oidcClientSecret == passwordMask {
		oidcClientSecret = currentConfig.OIDCClientSecret
	}

	// Canonicalize scopes exactly like the setup wizard: empty/token-less input
	// becomes the full default (see NormalizeScopesForStorage — a stored bare
	// value would override the richer default at boot), "openid" forced in.
	oidcScopes := reqData.OIDCScopes
	if reqData.OIDCEnabled {
		oidcScopes = config.NormalizeScopesForStorage(oidcScopes)
	}

	// Build SystemConfig for persistence
	newConfig := &service.SystemConfig{
		IsInstalled:             true,
		RootEmail:               reqData.RootEmail,
		APIEndpoint:             reqData.APIEndpoint,
		SMTPHost:                reqData.SMTPHost,
		SMTPPort:                reqData.SMTPPort,
		SMTPUsername:            reqData.SMTPUsername,
		SMTPPassword:            smtpPassword,
		SMTPFromEmail:           reqData.SMTPFromEmail,
		SMTPFromName:            reqData.SMTPFromName,
		SMTPUseTLS:              reqData.SMTPUseTLS,
		SMTPEHLOHostname:        reqData.SMTPEHLOHostname,
		TelemetryEnabled:        reqData.TelemetryEnabled,
		CheckForUpdates:         reqData.CheckForUpdates,
		AuditLogsRetentionDays:  reqData.AuditLogsRetentionDays,
		SMTPBridgeEnabled:       reqData.SMTPBridgeEnabled,
		SMTPBridgeDomain:        reqData.SMTPBridgeDomain,
		SMTPBridgePort:          reqData.SMTPBridgePort,
		SMTPBridgeTLSCertBase64: tlsCert,
		SMTPBridgeTLSKeyBase64:  tlsKey,
		OIDCEnabled:             reqData.OIDCEnabled,
		OIDCIssuerURL:           reqData.OIDCIssuerURL,
		OIDCClientID:            reqData.OIDCClientID,
		OIDCClientSecret:        oidcClientSecret,
		OIDCRedirectURI:         reqData.OIDCRedirectURI,
		OIDCScopes:              oidcScopes,
		OIDCButtonLabel:         reqData.OIDCButtonLabel,
		OIDCAutoCreateUsers:     reqData.OIDCAutoCreateUsers,
		OIDCAllowedDomains:      reqData.OIDCAllowedDomains,
	}

	// The two-factor requirement: written only when the body changes it. When
	// REQUIRE_TWO_FACTOR is set, the form shows its value: sending it back changes
	// nothing, sending another is refused.
	stored := currentConfig.RequireTwoFactor != nil && *currentConfig.RequireTwoFactor
	if h.requireTwoFactorEnv != "" {
		if reqData.RequireTwoFactor != (h.requireTwoFactorEnv == "true") {
			WriteJSONError(w, "require_two_factor is set by REQUIRE_TWO_FACTOR", http.StatusConflict)
			return
		}
	} else if reqData.RequireTwoFactor != stored {
		next := reqData.RequireTwoFactor
		if !h.allowRequireTwoFactorChange(w, r, stored, next) {
			return
		}
		newConfig.RequireTwoFactor = &next
	}
	if newConfig.RequireTwoFactor == nil {
		// Unchanged: rewritten as it is, so the audit diff shows no change.
		newConfig.RequireTwoFactor = currentConfig.RequireTwoFactor
	}

	// Fail fast on an invalid OIDC config BEFORE persisting + restarting. Without this
	// guard, a bad config (OIDC enabled with no issuer/client/secret, a non-https
	// issuer, or auto-create with no allowlist) would be saved, the server would
	// restart, and config.OIDCConfig.Validate() at boot would abort startup —
	// bricking the instance (login + admin UI down) with no in-product recovery.
	// This mirrors the setup wizard's ValidateSetupConfig.
	if err := validateOIDCForUpdate(newConfig); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := domain.ValidateAuditRetentionDays(newConfig.AuditLogsRetentionDays); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := h.setupService.ValidateSMTPCredentials(newConfig.SMTPUsername, newConfig.SMTPPassword); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.settingService.SetSystemConfig(ctx, newConfig, h.secretKey); err != nil {
		h.logger.WithField("error", err).Error("Failed to save system settings")
		WriteJSONError(w, fmt.Sprintf("Failed to save settings: %v", err), http.StatusInternalServerError)
		return
	}

	// The audit row for settings.update carries what changed. AuditDiff marks
	// every password, secret and key as changed-but-redacted; the values never
	// reach the log.
	if audit := domain.AuditFromContext(ctx); audit != nil {
		audit.SetTarget(domain.AuditTargetSettings, "system", "")
		audit.SetChanges(domain.AuditDiff(currentConfig, newConfig))
	}

	response := map[string]interface{}{
		"success": true,
		"message": "Settings saved successfully. Server is restarting...",
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)

	// Flush the response to ensure client receives it before shutdown
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	// Trigger graceful shutdown in background after a brief delay
	go func() {
		time.Sleep(500 * time.Millisecond)
		h.logger.Info("Settings updated - initiating graceful shutdown for configuration reload")
		if err := reloadShutdown(h.app); err != nil {
			h.logger.WithField("error", err).Error("Error during graceful shutdown")
		}
	}()
}

// allowRequireTwoFactorChange refuses turning the requirement on from a root session that
// would not meet it (service.CheckRequireTwoFactorChange): the next sign-in would lock the
// root user out.
func (h *SettingsHandler) allowRequireTwoFactorChange(w http.ResponseWriter, r *http.Request, current, next bool) bool {
	ctx := r.Context()
	user, session, err := h.authService.AuthenticateSession(ctx)
	if err != nil {
		if !writePermissionError(w, err) {
			WriteJSONError(w, "Failed to verify session", http.StatusInternalServerError)
		}
		return false
	}
	status, err := h.twoFactor.Status(ctx, user.ID)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to read the root user's two-factor state")
		WriteJSONError(w, "Failed to read the two-factor state", http.StatusInternalServerError)
		return false
	}
	if err := service.CheckRequireTwoFactorChange(current, &next, *session, status.Enabled); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": twoFactorRequiredForRootCode, "message": err.Error()})
		return false
	}
	return true
}

// handleTestSMTP tests SMTP connection with the provided configuration
func (h *SettingsHandler) handleTestSMTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.requireRootUser(w, r) == nil {
		return
	}

	var req TestSMTPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Default TLS to true if not specified
	useTLS := true
	if req.SMTPUseTLS != nil {
		useTLS = *req.SMTPUseTLS
	}

	// If password is the mask sentinel, substitute the real password from DB
	smtpPassword := req.SMTPPassword
	if smtpPassword == passwordMask {
		currentConfig, err := h.settingService.GetSystemConfig(r.Context(), h.secretKey)
		if err != nil {
			h.logger.WithField("error", err).Error("Failed to load current config for SMTP test")
			WriteJSONError(w, "Failed to load current settings", http.StatusInternalServerError)
			return
		}
		smtpPassword = currentConfig.SMTPPassword
	}

	testConfig := &service.SMTPTestConfig{
		Host:         req.SMTPHost,
		Port:         req.SMTPPort,
		Username:     req.SMTPUsername,
		Password:     smtpPassword,
		UseTLS:       useTLS,
		EHLOHostname: req.SMTPEHLOHostname,
	}

	if err := h.setupService.TestSMTPConnection(r.Context(), testConfig); err != nil {
		h.logger.WithField("error", err).Warn("SMTP connection test failed")
		WriteJSONError(w, fmt.Sprintf("SMTP connection failed: %v", err), http.StatusBadRequest)
		return
	}

	response := TestSMTPResponse{
		Success: true,
		Message: "SMTP connection test successful",
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

// handleLicenseGet returns what this deployment is licensed for, and whether the console is
// currently walled.
//
// It lives on the settings handler rather than in a handler of its own because this one is
// already root-only and already carries requireRootUser: a new file would mean a second root
// guard to keep correct, and a licence page is a settings page.
//
// Root-only although the banner it feeds is shown to everyone. Non-root users learn their
// deployment is unlicensed from the banner and the 402s, which is all they can act on — the
// licensee's name and billing address are not theirs to read, and only root can paste a key.
func (h *SettingsHandler) handleLicenseGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.requireRootUser(w, r) == nil {
		return
	}

	writeJSON(w, http.StatusOK, h.licenseResponse())
}

// handleLicenseSet installs a pasted licence key: verify, persist, swap, in that order.
//
// The ordering is the service's, and it is what makes a bad paste free: nothing is written
// and nothing in memory moves until the signature verifies, so a typo, a truncated copy or a
// key minted by somebody else leaves the deployment on exactly the licence it had a second
// earlier.
//
// No licence gate stands in front of this route, and none ever may: every licensed capability
// is unlocked by pasting a key here, so an endpoint that refused the unlicensed would be a
// lock with the key on the inside.
func (h *SettingsHandler) handleLicenseSet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.requireRootUser(w, r) == nil {
		return
	}

	var req SetLicenseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if h.licenseService == nil {
		h.logger.Error("Licence key submitted but no licence service is wired")
		WriteJSONError(w, "Licence management is unavailable", http.StatusInternalServerError)
		return
	}

	if err := h.licenseService.SetKey(r.Context(), req.Key); err != nil {
		h.writeLicenseSetError(w, err)
		return
	}

	// The new state, so the console repaints its banner from the same round trip that
	// installed the key rather than from a follow-up read that could race the swap.
	writeJSON(w, http.StatusOK, h.licenseResponse())
}

// licenseResponse builds the body both licence endpoints answer with.
//
// A nil service reports Community rather than an error. That is the same fail-safe rule the
// licence service itself follows — every failure degrades to the free tier, none of them
// breaks the deployment — applied one layer up so that a dependency which failed to wire
// cannot 500 the page an operator opens precisely because something is wrong.
func (h *SettingsHandler) licenseResponse() LicenseResponse {
	if h.licenseService == nil {
		return LicenseResponse{Entitlements: domain.CommunityEntitlements()}
	}

	return LicenseResponse{Entitlements: h.licenseService.Entitlements()}
}

// writeLicenseSetError maps a refused key to a status the console can act on.
//
// Three outcomes, three remedies, and the operator can only tell them apart if the statuses
// differ: fix the paste (400), edit the environment and restart (409), or call support
// (500). Collapsing them into one 400 would send an operator hunting for a typo in a key
// that is perfectly good and simply cannot take effect.
func (h *SettingsHandler) writeLicenseSetError(w http.ResponseWriter, err error) {
	if errors.Is(err, service.ErrLicenseKeyEmpty) {
		WriteJSONError(w, "Licence key is required", http.StatusBadRequest)
		return
	}

	// 409 rather than 400: the key may well be valid, it just cannot win. The deployment's
	// own configuration is what conflicts, and the remedy is in NOTIFUSE_LICENSE_KEY rather
	// than in the box the operator just typed into. The service refuses instead of storing
	// a row that would silently lose to the environment at the next restart — a discrepancy
	// that would surface weeks later, to somebody else.
	if errors.Is(err, service.ErrLicenseKeyLockedByEnv) {
		WriteJSONError(w, err.Error(), http.StatusConflict)
		return
	}

	for _, rejection := range licenseKeyRejections {
		if errors.Is(err, rejection) {
			// The sentinel's own sentence, not the wrapping prose: "licence key
			// signature does not verify" is the difference between a truncated paste
			// and a key for a different signing authority.
			WriteJSONError(w, rejection.Error(), http.StatusBadRequest)
			return
		}
	}

	// Anything left is the settings table failing to accept the write. The stored key and
	// the running licence are both untouched, so the deployment is exactly where it was.
	h.logger.WithField("error", err.Error()).Error("Failed to store licence key")
	WriteJSONError(w, "Failed to save licence key", http.StatusInternalServerError)
}

// RegisterRoutes registers the settings handler routes
func (h *SettingsHandler) RegisterRoutes(mux *http.ServeMux) {
	authMiddleware := middleware.NewAuthMiddleware(h.getJWTSecret)
	requireAuth := authMiddleware.RequireAuth()

	mux.Handle("/api/settings.get", requireAuth(http.HandlerFunc(h.handleGet)))
	mux.Handle("/api/settings.update", requireAuth(http.HandlerFunc(h.handleUpdate)))
	mux.Handle("/api/settings.testSmtp", requireAuth(http.HandlerFunc(h.handleTestSMTP)))
	mux.Handle("/api/dataRetention.getDefaults", requireAuth(http.HandlerFunc(h.handleGetRetentionDefaults)))
	mux.Handle("/api/dataRetention.setDefaults", requireAuth(http.HandlerFunc(h.handleSetRetentionDefaults)))

	// licence.* rather than settings.licence.*: the console reads the licence on pages that
	// have nothing to do with settings, so filing it under the settings namespace would name
	// it after the one screen it is least often read from.
	mux.Handle("/api/licence.get", requireAuth(http.HandlerFunc(h.handleLicenseGet)))
	mux.Handle("/api/licence.set", requireAuth(http.HandlerFunc(h.handleLicenseSet)))
}

// validateOIDCForUpdate runs the canonical config.OIDCConfig.Validate() against a
// settings-update payload BEFORE it is persisted, converting what would otherwise be
// a fatal post-restart boot failure into a recoverable HTTP 400. The redirect URI is
// derived the same way config.resolveOIDCConfig does (from API_ENDPOINT) so a blank
// redirect — which boot re-derives — is not a false-positive rejection.
func validateOIDCForUpdate(c *service.SystemConfig) error {
	redirect := c.OIDCRedirectURI
	if redirect == "" {
		if c.APIEndpoint != "" {
			redirect = strings.TrimRight(c.APIEndpoint, "/") + "/api/user.oidc.callback"
		} else {
			// API_ENDPOINT may be supplied via env at boot; don't reject a URI that
			// will be derivable then. The other (real) checks still run.
			redirect = "https://placeholder.invalid/api/user.oidc.callback"
		}
	}
	oidcCfg := config.OIDCConfig{
		Enabled:         c.OIDCEnabled,
		IssuerURL:       c.OIDCIssuerURL,
		ClientID:        c.OIDCClientID,
		ClientSecret:    c.OIDCClientSecret,
		RedirectURI:     redirect,
		AutoCreateUsers: c.OIDCAutoCreateUsers,
		AllowedDomains:  config.ParseRootEmails(c.OIDCAllowedDomains),
	}
	return oidcCfg.Validate()
}
