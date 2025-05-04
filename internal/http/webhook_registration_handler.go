package http

import (
	"encoding/json"
	"net/http"

	"aidanwoods.dev/go-paseto"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/http/middleware"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// WebhookRegistrationHandler handles webhook registration HTTP requests
type WebhookRegistrationHandler struct {
	service   domain.WebhookRegistrationService
	logger    logger.Logger
	publicKey paseto.V4AsymmetricPublicKey
}

// NewWebhookRegistrationHandler creates a new webhook registration handler
func NewWebhookRegistrationHandler(
	service domain.WebhookRegistrationService,
	publicKey paseto.V4AsymmetricPublicKey,
	logger logger.Logger,
) *WebhookRegistrationHandler {
	return &WebhookRegistrationHandler{
		service:   service,
		logger:    logger,
		publicKey: publicKey,
	}
}

// RegisterRoutes registers the webhook registration HTTP endpoints
func (h *WebhookRegistrationHandler) RegisterRoutes(mux *http.ServeMux) {
	// Create auth middleware
	authMiddleware := middleware.NewAuthMiddleware(h.publicKey)
	requireAuth := authMiddleware.RequireAuth()

	// Register webhook endpoints
	mux.Handle("/api/webhooks.register", requireAuth(http.HandlerFunc(h.handleRegister)))
	mux.Handle("/api/webhooks.status", requireAuth(http.HandlerFunc(h.handleStatus)))
}

// handleRegister handles requests to register webhooks with an email provider
func (h *WebhookRegistrationHandler) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	var req domain.RegisterWebhookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to decode request body")
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate request
	if err := req.Validate(); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Create webhook registration config
	config := &domain.WebhookRegistrationConfig{
		BaseURL:       req.BaseURL,
		IntegrationID: req.IntegrationID,
		EventTypes:    req.EventTypes,
	}

	// Register webhooks
	status, err := h.service.RegisterWebhooks(r.Context(), req.WorkspaceID, config)
	if err != nil {
		h.logger.WithField("error", err.Error()).
			WithField("workspace_id", req.WorkspaceID).
			WithField("integration_id", req.IntegrationID).
			Error("Failed to register webhooks")
		WriteJSONError(w, "Failed to register webhooks: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Return webhook registration status
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": status,
	})
}

// handleStatus handles requests to get the status of webhooks for an email provider
func (h *WebhookRegistrationHandler) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request parameters
	workspaceID := r.URL.Query().Get("workspace_id")
	if workspaceID == "" {
		WriteJSONError(w, "workspace_id is required", http.StatusBadRequest)
		return
	}

	providerKind := domain.EmailProviderKind(r.URL.Query().Get("provider"))
	if providerKind == "" {
		WriteJSONError(w, "provider is required", http.StatusBadRequest)
		return
	}

	emailType := r.URL.Query().Get("email_type")
	if emailType == "" {
		WriteJSONError(w, "email_type is required", http.StatusBadRequest)
		return
	}

	// Create and validate request
	req := &domain.GetWebhookStatusRequest{
		WorkspaceID:   workspaceID,
		IntegrationID: emailType,
	}

	if err := req.Validate(); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Get webhook status
	status, err := h.service.GetWebhookStatus(r.Context(), req.WorkspaceID, req.IntegrationID)
	if err != nil {
		h.logger.WithField("error", err.Error()).
			WithField("workspace_id", req.WorkspaceID).
			WithField("integration_id", req.IntegrationID).
			Error("Failed to get webhook status")
		WriteJSONError(w, "Failed to get webhook status: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Return webhook status
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": status,
	})
}
