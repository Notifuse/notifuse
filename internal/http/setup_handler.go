package http

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"aidanwoods.dev/go-paseto"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/google/uuid"
)

// SetupHandler handles setup wizard endpoints
type SetupHandler struct {
	settingService *service.SettingService
	userRepo       domain.UserRepository
	authService    *service.AuthService
	logger         logger.Logger
	secretKey      string
}

// NewSetupHandler creates a new setup handler
func NewSetupHandler(
	settingService *service.SettingService,
	userRepo domain.UserRepository,
	authService *service.AuthService,
	logger logger.Logger,
	secretKey string,
) *SetupHandler {
	return &SetupHandler{
		settingService: settingService,
		userRepo:       userRepo,
		authService:    authService,
		logger:         logger,
		secretKey:      secretKey,
	}
}

// StatusResponse represents the installation status response
type StatusResponse struct {
	IsInstalled bool `json:"is_installed"`
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
	Success bool   `json:"success"`
	Token   string `json:"token"`
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

	response := StatusResponse{
		IsInstalled: isInstalled,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GeneratePasetoKeys generates new PASETO keys
func (h *SetupHandler) GeneratePasetoKeys(w http.ResponseWriter, r *http.Request) {
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

	if isInstalled {
		WriteJSONError(w, "System is already installed", http.StatusForbidden)
		return
	}

	// Generate new PASETO keys
	secretKey := paseto.NewV4AsymmetricSecretKey()
	publicKey := secretKey.Public()

	privateKeyBase64 := base64.StdEncoding.EncodeToString(secretKey.ExportBytes())
	publicKeyBase64 := base64.StdEncoding.EncodeToString(publicKey.ExportBytes())

	response := PasetoKeysResponse{
		PublicKey:  publicKeyBase64,
		PrivateKey: privateKeyBase64,
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
		// Already installed, return 204 No Content
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Parse request body
	var req InitializeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.RootEmail == "" {
		WriteJSONError(w, "root_email is required", http.StatusBadRequest)
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

	// Handle PASETO keys
	var privateKeyBase64, publicKeyBase64 string
	if req.GeneratePasetoKeys {
		// Generate new keys
		secretKey := paseto.NewV4AsymmetricSecretKey()
		publicKey := secretKey.Public()
		privateKeyBase64 = base64.StdEncoding.EncodeToString(secretKey.ExportBytes())
		publicKeyBase64 = base64.StdEncoding.EncodeToString(publicKey.ExportBytes())
	} else {
		// Use provided keys
		if req.PasetoPrivateKey == "" || req.PasetoPublicKey == "" {
			WriteJSONError(w, "PASETO keys are required when not generating new ones", http.StatusBadRequest)
			return
		}
		privateKeyBase64 = req.PasetoPrivateKey
		publicKeyBase64 = req.PasetoPublicKey

		// Validate provided keys
		if _, err := base64.StdEncoding.DecodeString(privateKeyBase64); err != nil {
			WriteJSONError(w, "Invalid PASETO private key format", http.StatusBadRequest)
			return
		}
		if _, err := base64.StdEncoding.DecodeString(publicKeyBase64); err != nil {
			WriteJSONError(w, "Invalid PASETO public key format", http.StatusBadRequest)
			return
		}
	}

	// Validate SMTP settings (basic validation)
	if req.SMTPHost == "" {
		WriteJSONError(w, "smtp_host is required", http.StatusBadRequest)
		return
	}
	if req.SMTPPort == 0 {
		req.SMTPPort = 587 // Default
	}
	if req.SMTPFromEmail == "" {
		WriteJSONError(w, "smtp_from_email is required", http.StatusBadRequest)
		return
	}

	// Store system settings
	systemConfig := &service.SystemConfig{
		IsInstalled:      true,
		RootEmail:        req.RootEmail,
		APIEndpoint:      req.APIEndpoint,
		PasetoPrivateKey: privateKeyBase64,
		PasetoPublicKey:  publicKeyBase64,
		SMTPHost:         req.SMTPHost,
		SMTPPort:         req.SMTPPort,
		SMTPUsername:     req.SMTPUsername,
		SMTPPassword:     req.SMTPPassword,
		SMTPFromEmail:    req.SMTPFromEmail,
		SMTPFromName:     req.SMTPFromName,
	}

	if err := h.settingService.SetSystemConfig(ctx, systemConfig, h.secretKey); err != nil {
		h.logger.WithField("error", err).Error("Failed to save system configuration")
		WriteJSONError(w, "Failed to save system configuration", http.StatusInternalServerError)
		return
	}

	// Create root user
	rootUser := &domain.User{
		ID:        uuid.New().String(),
		Email:     req.RootEmail,
		Name:      "Root User",
		Type:      domain.UserTypeUser,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	if err := h.userRepo.CreateUser(ctx, rootUser); err != nil {
		// Check if user already exists
		if !strings.Contains(err.Error(), "duplicate key") {
			h.logger.WithField("error", err).Error("Failed to create root user")
			WriteJSONError(w, "Failed to create root user", http.StatusInternalServerError)
			return
		}
		// User already exists, fetch it
		existingUser, err := h.userRepo.GetUserByEmail(ctx, req.RootEmail)
		if err != nil {
			h.logger.WithField("error", err).Error("Failed to fetch existing root user")
			WriteJSONError(w, "Failed to fetch existing root user", http.StatusInternalServerError)
			return
		}
		rootUser = existingUser
	}

	// Create a session for the root user
	session := &domain.Session{
		ID:        uuid.New().String(),
		UserID:    rootUser.ID,
		ExpiresAt: time.Now().UTC().Add(30 * 24 * time.Hour), // 30 days
		CreatedAt: time.Now().UTC(),
	}

	if err := h.userRepo.CreateSession(ctx, session); err != nil {
		h.logger.WithField("error", err).Error("Failed to create session for root user")
		WriteJSONError(w, "Failed to create session for root user", http.StatusInternalServerError)
		return
	}

	// Generate auth token for immediate login
	token := h.authService.GenerateUserAuthToken(rootUser, session.ID, session.ExpiresAt)

	h.logger.WithField("email", req.RootEmail).Info("Setup wizard completed successfully")

	response := InitializeResponse{
		Success: true,
		Token:   token,
		Message: "Setup completed successfully",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// RegisterRoutes registers the setup handler routes
func (h *SetupHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/setup.status", h.Status)
	mux.HandleFunc("/api/setup.pasetoKeys", h.GeneratePasetoKeys)
	mux.HandleFunc("/api/setup.initialize", h.Initialize)
}
