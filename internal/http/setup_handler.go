package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/google/uuid"
)

// reloadShutdownTimeout bounds the shutdown a handler starts to load a new configuration. It is
// shorter than the 70 s cmd/api waits for that shutdown, so the databases close before the exit.
const reloadShutdownTimeout = 65 * time.Second

// reloadShutdown shuts the app down to load a new configuration, within reloadShutdownTimeout.
func reloadShutdown(app AppShutdowner) error {
	ctx, cancel := context.WithTimeout(context.Background(), reloadShutdownTimeout)
	defer cancel()
	return app.Shutdown(ctx)
}

// AppShutdowner defines the interface for triggering app shutdown
type AppShutdowner interface {
	Shutdown(ctx context.Context) error
}

// SetupHandler handles setup wizard endpoints
type SetupHandler struct {
	setupService   *service.SetupService
	settingService *service.SettingService
	logger         logger.Logger
	app            AppShutdowner
	// bootID is drawn once per handler, so once per process: the setup page tells
	// the restarted server from the one shutting down by a different value.
	bootID string
}

// NewSetupHandler creates a new setup handler
func NewSetupHandler(
	setupService *service.SetupService,
	settingService *service.SettingService,
	logger logger.Logger,
	app AppShutdowner,
) *SetupHandler {
	return &SetupHandler{
		setupService:   setupService,
		settingService: settingService,
		logger:         logger,
		app:            app,
		bootID:         uuid.NewString(),
	}
}

// StatusResponse represents the installation status response
type StatusResponse struct {
	IsInstalled           bool `json:"is_installed"`
	SMTPConfigured        bool `json:"smtp_configured"`
	APIEndpointConfigured bool `json:"api_endpoint_configured"`
	RootEmailConfigured   bool `json:"root_email_configured"`
	SMTPBridgeConfigured  bool `json:"smtp_bridge_configured"`
	OIDCConfigured        bool `json:"oidc_configured"`
	// EnvOverrides lists, before install only, the setting keys whose environment
	// variable is set (names, never values), so the wizard can lock those fields.
	EnvOverrides []string `json:"env_overrides,omitempty"`
	// BootID is random and changes at each start of the process; it says nothing
	// about the host or the time.
	BootID string `json:"boot_id"`
}

// InitializeRequest represents the setup initialization request
type InitializeRequest struct {
	RootEmail               string `json:"root_email"`
	APIEndpoint             string `json:"api_endpoint"`
	SMTPHost                string `json:"smtp_host"`
	SMTPPort                int    `json:"smtp_port"`
	SMTPUsername            string `json:"smtp_username"`
	SMTPPassword            string `json:"smtp_password"`
	SMTPFromEmail           string `json:"smtp_from_email"`
	SMTPFromName            string `json:"smtp_from_name"`
	SMTPUseTLS              *bool  `json:"smtp_use_tls"`
	SMTPEHLOHostname        string `json:"smtp_ehlo_hostname"`
	TelemetryEnabled        bool   `json:"telemetry_enabled"`
	CheckForUpdates         *bool  `json:"check_for_updates"`
	SMTPBridgeEnabled       bool   `json:"smtp_bridge_enabled"`
	SMTPBridgeHost          string `json:"smtp_bridge_domain"`
	SMTPBridgePort          int    `json:"smtp_bridge_port"`
	SMTPBridgeTLSCertBase64 string `json:"smtp_bridge_tls_cert_base64"`
	SMTPBridgeTLSKeyBase64  string `json:"smtp_bridge_tls_key_base64"`
	OIDCEnabled             bool   `json:"oidc_enabled"`
	OIDCIssuerURL           string `json:"oidc_issuer_url"`
	OIDCClientID            string `json:"oidc_client_id"`
	OIDCClientSecret        string `json:"oidc_client_secret"`
	OIDCRedirectURI         string `json:"oidc_redirect_uri"`
	OIDCScopes              string `json:"oidc_scopes"`
	OIDCButtonLabel         string `json:"oidc_button_label"`
	OIDCAutoCreateUsers     bool   `json:"oidc_auto_create_users"`
	OIDCAllowedDomains      string `json:"oidc_allowed_domains"`
}

// InitializeResponse represents the setup completion response
type InitializeResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	// BootID of the process that handled the request: the setup page waits for the
	// server to answer another one, since this process restarts to apply the setup.
	BootID string `json:"boot_id"`
}

// TestSMTPRequest represents the SMTP connection test request
type TestSMTPRequest struct {
	SMTPHost         string `json:"smtp_host"`
	SMTPPort         int    `json:"smtp_port"`
	SMTPUsername     string `json:"smtp_username"`
	SMTPPassword     string `json:"smtp_password"`
	SMTPUseTLS       *bool  `json:"smtp_use_tls"`
	SMTPEHLOHostname string `json:"smtp_ehlo_hostname"`
}

// TestSMTPResponse represents the SMTP connection test response
type TestSMTPResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// Status returns the current installation status
func (h *SetupHandler) Status(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()
	isInstalled, err := h.settingService.IsInstalled(ctx)
	if err != nil {
		h.logger.WithField("error", err).Error("Failed to check installation status")
		WriteJSONError(w, "Failed to check installation status", http.StatusInternalServerError)
		return
	}

	// Get configuration status to tell frontend what's configured via env
	configStatus := h.setupService.GetConfigurationStatus()

	response := StatusResponse{
		IsInstalled:           isInstalled,
		SMTPConfigured:        configStatus.SMTPConfigured,
		APIEndpointConfigured: configStatus.APIEndpointConfigured,
		RootEmailConfigured:   configStatus.RootEmailConfigured,
		SMTPBridgeConfigured:  configStatus.SMTPBridgeConfigured,
		OIDCConfigured:        configStatus.OIDCConfigured,
		BootID:                h.bootID,
	}
	if !isInstalled {
		for key, set := range h.setupService.GetEnvOverrides() {
			if set {
				response.EnvOverrides = append(response.EnvOverrides, key)
			}
		}
		slices.Sort(response.EnvOverrides)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

// Initialize completes the setup wizard
func (h *SetupHandler) Initialize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()

	// Check if already installed
	isInstalled, err := h.settingService.IsInstalled(ctx)
	if err != nil {
		h.logger.WithField("error", err).Error("Failed to check installation status")
		WriteJSONError(w, "Failed to check installation status", http.StatusInternalServerError)
		return
	}

	if isInstalled {
		// Already installed, return success response
		response := InitializeResponse{
			Success: true,
			Message: "Setup already completed. System is ready to use.",
			BootID:  h.bootID,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(response)
		return
	}

	// Parse request body
	var req InitializeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Auto-detect API endpoint if not provided
	if req.APIEndpoint == "" {
		// Use the Host header to construct the API endpoint
		scheme := "https"
		if r.TLS == nil {
			scheme = "http"
		}
		req.APIEndpoint = fmt.Sprintf("%s://%s", scheme, r.Host)
	}

	// Default TLS to true if not specified
	smtpUseTLS := true
	if req.SMTPUseTLS != nil {
		smtpUseTLS = *req.SMTPUseTLS
	}

	// Absent means on, as the setup wizard and the documentation default it: a plain bool
	// would install a JSON client that leaves the field out with it off.
	checkForUpdates := true
	if req.CheckForUpdates != nil {
		checkForUpdates = *req.CheckForUpdates
	}

	// Convert request to service config
	setupConfig := &service.SetupConfig{
		RootEmail:               req.RootEmail,
		APIEndpoint:             req.APIEndpoint,
		SMTPHost:                req.SMTPHost,
		SMTPPort:                req.SMTPPort,
		SMTPUsername:            req.SMTPUsername,
		SMTPPassword:            req.SMTPPassword,
		SMTPFromEmail:           req.SMTPFromEmail,
		SMTPFromName:            req.SMTPFromName,
		SMTPUseTLS:              smtpUseTLS,
		SMTPEHLOHostname:        req.SMTPEHLOHostname,
		TelemetryEnabled:        req.TelemetryEnabled,
		CheckForUpdates:         checkForUpdates,
		SMTPBridgeEnabled:       req.SMTPBridgeEnabled,
		SMTPBridgeDomain:        req.SMTPBridgeHost,
		SMTPBridgePort:          req.SMTPBridgePort,
		SMTPBridgeTLSCertBase64: req.SMTPBridgeTLSCertBase64,
		SMTPBridgeTLSKeyBase64:  req.SMTPBridgeTLSKeyBase64,
		OIDCEnabled:             req.OIDCEnabled,
		OIDCIssuerURL:           req.OIDCIssuerURL,
		OIDCClientID:            req.OIDCClientID,
		OIDCClientSecret:        req.OIDCClientSecret,
		OIDCRedirectURI:         req.OIDCRedirectURI,
		OIDCScopes:              req.OIDCScopes,
		OIDCButtonLabel:         req.OIDCButtonLabel,
		OIDCAutoCreateUsers:     req.OIDCAutoCreateUsers,
		OIDCAllowedDomains:      req.OIDCAllowedDomains,
	}

	// Initialize using service (callback will be called in service)
	err = h.setupService.Initialize(ctx, setupConfig)
	if err != nil {
		h.logger.WithField("error", err).Error("Failed to initialize system")
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	response := InitializeResponse{
		Success: true,
		Message: "Setup completed successfully. Server is restarting with new configuration...",
		BootID:  h.bootID,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)

	// Flush the response to ensure client receives it before shutdown
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	// Trigger graceful shutdown in background after a brief delay
	// This allows the response to reach the client
	go func() {
		time.Sleep(500 * time.Millisecond)
		h.logger.Info("Setup completed - initiating graceful shutdown for configuration reload")
		if err := reloadShutdown(h.app); err != nil {
			h.logger.WithField("error", err).Error("Error during graceful shutdown")
		}
	}()
}

// TestSMTP tests the SMTP connection with the provided configuration
func (h *SetupHandler) TestSMTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()

	// Check if already installed - disable this endpoint if installed
	isInstalled, err := h.settingService.IsInstalled(ctx)
	if err != nil {
		h.logger.WithField("error", err).Error("Failed to check installation status")
		WriteJSONError(w, "Failed to check installation status", http.StatusInternalServerError)
		return
	}

	if isInstalled {
		WriteJSONError(w, "System is already installed", http.StatusForbidden)
		return
	}

	// Parse request body
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

	// Test SMTP connection using service
	testConfig := &service.SMTPTestConfig{
		Host:         req.SMTPHost,
		Port:         req.SMTPPort,
		Username:     req.SMTPUsername,
		Password:     req.SMTPPassword,
		UseTLS:       useTLS,
		EHLOHostname: req.SMTPEHLOHostname,
	}

	if err := h.setupService.TestSMTPConnection(ctx, testConfig); err != nil {
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

// RegisterRoutes registers the setup handler routes
func (h *SetupHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/setup.status", h.Status)
	mux.HandleFunc("/api/setup.initialize", h.Initialize)
	mux.HandleFunc("/api/setup.testSmtp", h.TestSMTP)
}
