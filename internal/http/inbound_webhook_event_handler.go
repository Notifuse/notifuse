package http

import (
	"io"
	"net/http"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/http/middleware"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// InboundWebhookEventHandler handles HTTP requests for inbound webhook events
type InboundWebhookEventHandler struct {
	service      domain.InboundWebhookEventServiceInterface
	logger       logger.Logger
	getJWTSecret func() ([]byte, error)
}

// NewInboundWebhookEventHandler creates a new inbound webhook event handler
func NewInboundWebhookEventHandler(service domain.InboundWebhookEventServiceInterface, getJWTSecret func() ([]byte, error), logger logger.Logger) *InboundWebhookEventHandler {
	return &InboundWebhookEventHandler{
		service:      service,
		logger:       logger,
		getJWTSecret: getJWTSecret,
	}
}

// RegisterRoutes registers the inbound webhook event HTTP endpoints
func (h *InboundWebhookEventHandler) RegisterRoutes(mux *http.ServeMux) {
	// Create auth middleware
	authMiddleware := middleware.NewAuthMiddleware(h.getJWTSecret)
	requireAuth := authMiddleware.RequireAuth()

	// Public webhooks endpoint for receiving events from email providers
	mux.Handle("/webhooks/email", http.HandlerFunc(h.handleIncomingWebhook))

	// Public endpoint for receiving inbound (reply) messages forwarded by a
	// provider's inbound parsing feature (e.g. Mailgun Routes).
	mux.Handle("/webhooks/email/inbound", http.HandlerFunc(h.handleIncomingReply))

	// Authenticated endpoints for accessing inbound webhook event data
	mux.Handle("/api/inboundWebhookEvents.list", requireAuth(http.HandlerFunc(h.handleList)))
}

// handleIncomingWebhook handles incoming webhook events from email providers
func (h *InboundWebhookEventHandler) handleIncomingWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract provider, workspace_id and integration_id from query parameters
	// Format: /webhooks/email?provider={provider}&workspace_id={id}&integration_id={id}
	provider := r.URL.Query().Get("provider")
	workspaceID := r.URL.Query().Get("workspace_id")
	integrationID := r.URL.Query().Get("integration_id")

	if provider == "" {
		WriteJSONError(w, "Provider is required", http.StatusBadRequest)
		return
	}

	if workspaceID == "" || integrationID == "" {
		WriteJSONError(w, "Workspace ID and integration ID are required", http.StatusBadRequest)
		return
	}

	// Log the incoming webhook
	h.logger.WithField("provider", provider).
		WithField("workspace_id", workspaceID).
		WithField("integration_id", integrationID).
		Info("Received webhook event")

	// Read and parse the request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to read webhook request body")
		WriteJSONError(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	// Process the webhook event
	err = h.service.ProcessWebhook(r.Context(), workspaceID, integrationID, body)
	if err != nil {
		h.logger.WithField("error", err.Error()).
			WithField("workspace_id", workspaceID).
			WithField("integration_id", integrationID).
			WithField("provider", provider).
			Error("Failed to process webhook")
		WriteJSONError(w, "Failed to process webhook", http.StatusBadRequest)
		return
	}

	// Return success
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
	})
}

// handleIncomingReply handles inbound (reply) messages forwarded by a provider's
// inbound parsing feature. Unlike event webhooks, these are posted as form data
// (multipart/form-data or application/x-www-form-urlencoded).
// Format: /webhooks/email/inbound?workspace_id={id}&integration_id={id}
func (h *InboundWebhookEventHandler) handleIncomingReply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	workspaceID := r.URL.Query().Get("workspace_id")
	integrationID := r.URL.Query().Get("integration_id")
	if workspaceID == "" || integrationID == "" {
		WriteJSONError(w, "Workspace ID and integration ID are required", http.StatusBadRequest)
		return
	}

	// Parse the form. ParseMultipartForm transparently handles
	// application/x-www-form-urlencoded bodies too, and populates r.PostForm.
	// 25MB matches the typical provider inbound message size limit.
	if err := r.ParseMultipartForm(25 << 20); err != nil && err != http.ErrNotMultipart {
		h.logger.WithField("error", err.Error()).
			WithField("workspace_id", workspaceID).
			Error("Failed to parse inbound reply form")
		WriteJSONError(w, "Failed to parse request body", http.StatusBadRequest)
		return
	}

	h.logger.WithField("workspace_id", workspaceID).
		WithField("integration_id", integrationID).
		Info("Received inbound reply")

	if err := h.service.ProcessInboundReply(r.Context(), workspaceID, integrationID, r.PostForm); err != nil {
		h.logger.WithField("error", err.Error()).
			WithField("workspace_id", workspaceID).
			WithField("integration_id", integrationID).
			Error("Failed to process inbound reply")
		WriteJSONError(w, "Failed to process inbound reply", http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
	})
}

// handleList handles requests to list inbound webhook events by type
func (h *InboundWebhookEventHandler) handleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse query parameters into InboundWebhookEventListParams
	params := domain.InboundWebhookEventListParams{}
	if err := params.FromQuery(r.URL.Query()); err != nil {
		h.logger.WithField("error", err.Error()).
			Error("Invalid inbound webhook event list parameters")
		WriteJSONError(w, "Invalid parameters: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Call the service to list events
	result, err := h.service.ListEvents(r.Context(), params.WorkspaceID, params)
	if err != nil {
		h.logger.WithField("error", err.Error()).
			WithField("workspace_id", params.WorkspaceID).
			Error("Failed to list inbound webhook events")
		WriteJSONError(w, "Failed to list inbound webhook events", http.StatusInternalServerError)
		return
	}

	// Return the results
	writeJSON(w, http.StatusOK, result)
}
