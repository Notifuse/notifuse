package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/http/middleware"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/pkg/logger"
)

const (
	passwordMask   = "\u2022\u2022\u2022\u2022\u2022\u2022\u2022\u2022" // ••••••••
	configuredMask = "[configured]"
)

// SystemSettingsData represents the editable system settings
type SystemSettingsData struct {
	RootEmail               string `json:"root_email"`
	APIEndpoint             string `json:"api_endpoint"`
	SMTPHost                string `json:"smtp_host"`
	SMTPPort                int    `json:"smtp_port"`
	SMTPUsername            string `json:"smtp_username"`
	SMTPPassword            string `json:"smtp_password"`
	SMTPFromEmail           string `json:"smtp_from_email"`
	SMTPFromName            string `json:"smtp_from_name"`
	SMTPUseTLS              bool   `json:"smtp_use_tls"`
	SMTPEHLOHostname        string `json:"smtp_ehlo_hostname"`
	TelemetryEnabled        bool   `json:"telemetry_enabled"`
	CheckForUpdates         bool   `json:"check_for_updates"`
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
}

// SystemSettingsResponse wraps settings with env override info
type SystemSettingsResponse struct {
	Settings     SystemSettingsData `json:"settings"`
	EnvOverrides map[string]bool    `json:"env_overrides"`
}

// SettingsHandler handles system settings endpoints (root user only)
type SettingsHandler struct {
	setupService   *service.SetupService
	settingService *service.SettingService
	userService    UserServiceInterface
	getJWTSecret   func() ([]byte, error)
	logger         logger.Logger
	secretKey      string
	rootEmail      string
	app            AppShutdowner
}

// NewSettingsHandler creates a new settings handler
func NewSettingsHandler(
	setupService *service.SetupService,
	settingService *service.SettingService,
	userService UserServiceInterface,
	getJWTSecret func() ([]byte, error),
	logger logger.Logger,
	secretKey string,
	rootEmail string,
	app AppShutdowner,
) *SettingsHandler {
	return &SettingsHandler{
		setupService:   setupService,
		settingService: settingService,
		userService:    userService,
		getJWTSecret:   getJWTSecret,
		logger:         logger,
		secretKey:      secretKey,
		rootEmail:      rootEmail,
		app:            app,
	}
}

// requireRootUser checks that the authenticated user is the root user.
// Returns the user on success, or writes an error response and returns nil.
func (h *SettingsHandler) requireRootUser(w http.ResponseWriter, r *http.Request) *domain.User {
	ctx := r.Context()

	userID, ok := ctx.Value(domain.UserIDKey).(string)
	if !ok || userID == "" {
		WriteJSONError(w, "Unauthorized", http.StatusUnauthorized)
		return nil
	}

	user, err := h.userService.GetUserByID(ctx, userID)
	if err != nil {
		WriteJSONError(w, "Unauthorized", http.StatusUnauthorized)
		return nil
	}

	if !config.IsRootEmail(h.rootEmail, user.Email) {
		WriteJSONError(w, "Forbidden: root user access required", http.StatusForbidden)
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

	response := SystemSettingsResponse{
		Settings:     settings,
		EnvOverrides: envOverrides,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
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

	var reqData SystemSettingsData
	if err := json.NewDecoder(r.Body).Decode(&reqData); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Load current config to handle masked field round-trip
	currentConfig, err := h.settingService.GetSystemConfig(ctx, h.secretKey)
	if err != nil {
		h.logger.WithField("error", err).Error("Failed to load current system config")
		WriteJSONError(w, "Failed to load current settings", http.StatusInternalServerError)
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

	if err := h.settingService.SetSystemConfig(ctx, newConfig, h.secretKey); err != nil {
		h.logger.WithField("error", err).Error("Failed to save system settings")
		WriteJSONError(w, fmt.Sprintf("Failed to save settings: %v", err), http.StatusInternalServerError)
		return
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
		if err := h.app.Shutdown(context.Background()); err != nil {
			h.logger.WithField("error", err).Error("Error during graceful shutdown")
		}
	}()
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

// RegisterRoutes registers the settings handler routes
func (h *SettingsHandler) RegisterRoutes(mux *http.ServeMux) {
	authMiddleware := middleware.NewAuthMiddleware(h.getJWTSecret)
	requireAuth := authMiddleware.RequireAuth()

	mux.Handle("/api/settings.get", requireAuth(http.HandlerFunc(h.handleGet)))
	mux.Handle("/api/settings.update", requireAuth(http.HandlerFunc(h.handleUpdate)))
	mux.Handle("/api/settings.testSmtp", requireAuth(http.HandlerFunc(h.handleTestSMTP)))
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
