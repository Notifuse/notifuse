package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/pkg/logger"
)

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
	}
}

// StatusResponse represents the installation status response
type StatusResponse struct {
	IsInstalled           bool `json:"is_installed"`
	SMTPConfigured        bool `json:"smtp_configured"`
	APIEndpointConfigured bool `json:"api_endpoint_configured"`
	RootEmailConfigured   bool `json:"root_email_configured"`
	SMTPRelayConfigured   bool `json:"smtp_relay_configured"`
}

// InitializeRequest represents the setup initialization request
type InitializeRequest struct {
	RootEmail              string `json:"root_email"`
	APIEndpoint            string `json:"api_endpoint"`
	SMTPHost               string `json:"smtp_host"`
	SMTPPort               int    `json:"smtp_port"`
	SMTPUsername           string `json:"smtp_username"`
	SMTPPassword           string `json:"smtp_password"`
	SMTPFromEmail          string `json:"smtp_from_email"`
	SMTPFromName           string `json:"smtp_from_name"`
	SMTPUseTLS             *bool  `json:"smtp_use_tls"`
	TelemetryEnabled       bool   `json:"telemetry_enabled"`
	CheckForUpdates        bool   `json:"check_for_updates"`
	SMTPRelayEnabled       bool   `json:"smtp_relay_enabled"`
	SMTPRelayHost          string `json:"smtp_relay_domain"`
	SMTPRelayPort          int    `json:"smtp_relay_port"`
	SMTPRelayTLSCertBase64 string `json:"smtp_relay_tls_cert_base64"`
	SMTPRelayTLSKeyBase64  string `json:"smtp_relay_tls_key_base64"`
}

// InitializeResponse represents the setup completion response
type InitializeResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// TestSMTPRequest represents the SMTP connection test request
type TestSMTPRequest struct {
	SMTPHost     string `json:"smtp_host"`
	SMTPPort     int    `json:"smtp_port"`
	SMTPUsername string `json:"smtp_username"`
	SMTPPassword string `json:"smtp_password"`
	SMTPUseTLS   *bool  `json:"smtp_use_tls"`
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
		SMTPRelayConfigured:   configStatus.SMTPRelayConfigured,
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

	// Convert request to service config
	setupConfig := &service.SetupConfig{
		RootEmail:              req.RootEmail,
		APIEndpoint:            req.APIEndpoint,
		SMTPHost:               req.SMTPHost,
		SMTPPort:               req.SMTPPort,
		SMTPUsername:           req.SMTPUsername,
		SMTPPassword:           req.SMTPPassword,
		SMTPFromEmail:          req.SMTPFromEmail,
		SMTPFromName:           req.SMTPFromName,
		SMTPUseTLS:             smtpUseTLS,
		TelemetryEnabled:       req.TelemetryEnabled,
		CheckForUpdates:        req.CheckForUpdates,
		SMTPRelayEnabled:       req.SMTPRelayEnabled,
		SMTPRelayDomain:        req.SMTPRelayHost,
		SMTPRelayPort:          req.SMTPRelayPort,
		SMTPRelayTLSCertBase64: req.SMTPRelayTLSCertBase64,
		SMTPRelayTLSKeyBase64:  req.SMTPRelayTLSKeyBase64,
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
		if err := h.app.Shutdown(context.Background()); err != nil {
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
		Host:     req.SMTPHost,
		Port:     req.SMTPPort,
		Username: req.SMTPUsername,
		Password: req.SMTPPassword,
		UseTLS:   useTLS,
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
