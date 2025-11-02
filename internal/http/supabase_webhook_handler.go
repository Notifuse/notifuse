package http

import (
	"io"
	"net/http"
	"strings"

	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// SupabaseWebhookHandler handles HTTP requests for Supabase webhooks
type SupabaseWebhookHandler struct {
	supabaseService *service.SupabaseService
	logger          logger.Logger
}

// NewSupabaseWebhookHandler creates a new Supabase webhook handler
func NewSupabaseWebhookHandler(supabaseService *service.SupabaseService, logger logger.Logger) *SupabaseWebhookHandler {
	return &SupabaseWebhookHandler{
		supabaseService: supabaseService,
		logger:          logger,
	}
}

// RegisterRoutes registers the Supabase webhook HTTP endpoints
func (h *SupabaseWebhookHandler) RegisterRoutes(mux *http.ServeMux) {
	// Public webhook endpoints for receiving events from Supabase
	mux.Handle("/webhooks/supabase/auth-email", http.HandlerFunc(h.handleAuthEmailWebhook))
	mux.Handle("/webhooks/supabase/before-user-created", http.HandlerFunc(h.handleUserCreatedWebhook))
}

// handleAuthEmailWebhook handles incoming Supabase Send Email Hook webhooks
func (h *SupabaseWebhookHandler) handleAuthEmailWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract workspace_id and integration_id from query parameters
	// Format: /webhooks/supabase/auth-email?workspace_id={id}&integration_id={id}
	workspaceID := r.URL.Query().Get("workspace_id")
	integrationID := r.URL.Query().Get("integration_id")

	if workspaceID == "" || integrationID == "" {
		WriteJSONError(w, "workspace_id and integration_id are required", http.StatusBadRequest)
		return
	}

	// Extract Supabase webhook headers
	webhookID := r.Header.Get("webhook-id")
	webhookTimestamp := r.Header.Get("webhook-timestamp")
	webhookSignature := r.Header.Get("webhook-signature")

	if webhookID == "" || webhookTimestamp == "" || webhookSignature == "" {
		WriteJSONError(w, "Missing required webhook headers", http.StatusBadRequest)
		return
	}

	// Log the incoming webhook
	h.logger.WithField("workspace_id", workspaceID).
		WithField("integration_id", integrationID).
		Info("Received Supabase auth email webhook")

	// Read the request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to read webhook request body")
		WriteJSONError(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	// Process the webhook
	err = h.supabaseService.ProcessAuthEmailHook(r.Context(), workspaceID, integrationID, body, webhookID, webhookTimestamp, webhookSignature)
	if err != nil {
		h.logger.WithField("error", err.Error()).
			WithField("workspace_id", workspaceID).
			WithField("integration_id", integrationID).
			Error("Failed to process auth email webhook")
		WriteJSONError(w, "Failed to process webhook", http.StatusBadRequest)
		return
	}

	// Return success
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
	})
}

// handleUserCreatedWebhook handles incoming Supabase Before User Created Hook webhooks
func (h *SupabaseWebhookHandler) handleUserCreatedWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract workspace_id and integration_id from query parameters
	// Format: /webhooks/supabase/before-user-created?workspace_id={id}&integration_id={id}
	workspaceID := r.URL.Query().Get("workspace_id")
	integrationID := r.URL.Query().Get("integration_id")

	if workspaceID == "" || integrationID == "" {
		WriteJSONError(w, "workspace_id and integration_id are required", http.StatusBadRequest)
		return
	}

	// Extract Supabase webhook headers
	webhookID := r.Header.Get("webhook-id")
	webhookTimestamp := r.Header.Get("webhook-timestamp")
	webhookSignature := r.Header.Get("webhook-signature")

	if webhookID == "" || webhookTimestamp == "" || webhookSignature == "" {
		WriteJSONError(w, "Missing required webhook headers", http.StatusBadRequest)
		return
	}

	// Log the incoming webhook
	h.logger.WithField("workspace_id", workspaceID).
		WithField("integration_id", integrationID).
		Info("Received Supabase user created webhook")

	// Read the request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to read webhook request body")
		WriteJSONError(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	// Process the webhook
	err = h.supabaseService.ProcessUserCreatedHook(r.Context(), workspaceID, integrationID, body, webhookID, webhookTimestamp, webhookSignature)

	if err != nil {
		// Check if this is a rejection error (e.g., disposable email)
		// These should return 4xx to block user creation in Supabase
		if strings.Contains(err.Error(), "disposable email") ||
			strings.Contains(err.Error(), "not allowed") ||
			strings.Contains(err.Error(), "rejected") {
			h.logger.WithField("error", err.Error()).
				WithField("workspace_id", workspaceID).
				WithField("integration_id", integrationID).
				Info("User creation rejected by policy")

			// Return error to block signup in Supabase
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{
				"error": map[string]interface{}{
					"http_code": 400,
					"message":   err.Error(),
				},
			})
			return
		}

		// For other errors, log but don't block user creation
		h.logger.WithField("error", err.Error()).
			WithField("workspace_id", workspaceID).
			WithField("integration_id", integrationID).
			Error("Error in user created webhook processing, but returning success to not block user creation")
	}

	// Return success with 204 No Content to allow signup
	w.WriteHeader(http.StatusNoContent)
}
