package http

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// SetupHandler handles setup wizard endpoints
type SetupHandler struct {
	setupService   *service.SetupService
	settingService *service.SettingService
	logger         logger.Logger
}

// NewSetupHandler creates a new setup handler
func NewSetupHandler(
	setupService *service.SetupService,
	settingService *service.SettingService,
	logger logger.Logger,
) *SetupHandler {
	return &SetupHandler{
		setupService:   setupService,
		settingService: settingService,
		logger:         logger,
	}
}

// StatusResponse represents the installation status response
type StatusResponse struct {
	IsInstalled           bool `json:"is_installed"`
	SMTPConfigured        bool `json:"smtp_configured"`
	PasetoConfigured      bool `json:"paseto_configured"`
	APIEndpointConfigured bool `json:"api_endpoint_configured"`
	RootEmailConfigured   bool `json:"root_email_configured"`
}

// PasetoKeysResponse represents generated PASETO keys
type PasetoKeysResponse struct {
	PublicKey  string `json:"public_key"`
	PrivateKey string `json:"private_key"`
}

// InitializeRequest represents the setup initialization request
type InitializeRequest struct {
	RootEmail          string `json:"root_email"`
	APIEndpoint        string `json:"api_endpoint"`
	GeneratePasetoKeys bool   `json:"generate_paseto_keys"`
	PasetoPublicKey    string `json:"paseto_public_key,omitempty"`
	PasetoPrivateKey   string `json:"paseto_private_key,omitempty"`
	SMTPHost           string `json:"smtp_host"`
	SMTPPort           int    `json:"smtp_port"`
	SMTPUsername       string `json:"smtp_username"`
	SMTPPassword       string `json:"smtp_password"`
	SMTPFromEmail      string `json:"smtp_from_email"`
	SMTPFromName       string `json:"smtp_from_name"`
}

// InitializeResponse represents the setup completion response
type InitializeResponse struct {
	Success    bool                `json:"success"`
	Message    string              `json:"message"`
	PasetoKeys *PasetoKeysResponse `json:"paseto_keys,omitempty"`
}

// TestSMTPRequest represents the SMTP connection test request
type TestSMTPRequest struct {
	SMTPHost     string `json:"smtp_host"`
	SMTPPort     int    `json:"smtp_port"`
	SMTPUsername string `json:"smtp_username"`
	SMTPPassword string `json:"smtp_password"`
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
		PasetoConfigured:      configStatus.PasetoConfigured,
		APIEndpointConfigured: configStatus.APIEndpointConfigured,
		RootEmailConfigured:   configStatus.RootEmailConfigured,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
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
		json.NewEncoder(w).Encode(response)
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

	// Convert request to service config
	setupConfig := &service.SetupConfig{
		RootEmail:          req.RootEmail,
		APIEndpoint:        req.APIEndpoint,
		GeneratePasetoKeys: req.GeneratePasetoKeys,
		PasetoPublicKey:    req.PasetoPublicKey,
		PasetoPrivateKey:   req.PasetoPrivateKey,
		SMTPHost:           req.SMTPHost,
		SMTPPort:           req.SMTPPort,
		SMTPUsername:       req.SMTPUsername,
		SMTPPassword:       req.SMTPPassword,
		SMTPFromEmail:      req.SMTPFromEmail,
		SMTPFromName:       req.SMTPFromName,
	}

	// Initialize using service (callback will be called in service)
	generatedKeys, err := h.setupService.Initialize(ctx, setupConfig)
	if err != nil {
		h.logger.WithField("error", err).Error("Failed to initialize system")
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	response := InitializeResponse{
		Success: true,
		Message: "Setup completed successfully. You can now sign in with your email.",
	}

	// Include generated keys in response if they were generated
	if generatedKeys != nil {
		response.PasetoKeys = &PasetoKeysResponse{
			PublicKey:  generatedKeys.PublicKey,
			PrivateKey: generatedKeys.PrivateKey,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
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

	// Test SMTP connection using service
	testConfig := &service.SMTPTestConfig{
		Host:     req.SMTPHost,
		Port:     req.SMTPPort,
		Username: req.SMTPUsername,
		Password: req.SMTPPassword,
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
	json.NewEncoder(w).Encode(response)
}

// RegisterRoutes registers the setup handler routes
func (h *SetupHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/setup.status", h.Status)
	mux.HandleFunc("/api/setup.initialize", h.Initialize)
	mux.HandleFunc("/api/setup.testSmtp", h.TestSMTP)
}
