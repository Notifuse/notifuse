package http

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/http/middleware"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/pkg/ratelimiter"
)

// maxInboundReplyBytes caps the inbound reply body size (matches typical provider
// inbound message limits) to bound memory/disk use.
const maxInboundReplyBytes = 25 << 20

// InboundWebhookEventHandler handles HTTP requests for inbound webhook events
type InboundWebhookEventHandler struct {
	service      domain.InboundWebhookEventServiceInterface
	logger       logger.Logger
	getJWTSecret func() ([]byte, error)
	rateLimiter  *ratelimiter.RateLimiter
}

// NewInboundWebhookEventHandler creates a new inbound webhook event handler
func NewInboundWebhookEventHandler(service domain.InboundWebhookEventServiceInterface, getJWTSecret func() ([]byte, error), rateLimiter *ratelimiter.RateLimiter, logger logger.Logger) *InboundWebhookEventHandler {
	return &InboundWebhookEventHandler{
		service:      service,
		logger:       logger,
		getJWTSecret: getJWTSecret,
		rateLimiter:  rateLimiter,
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

// handleIncomingReply handles inbound (reply) messages forwarded by a provider's
// inbound parsing feature. Unlike event webhooks, these arrive as form data
// (multipart/form-data or x-www-form-urlencoded) or JSON depending on the provider.
// Format: /webhooks/email/inbound?workspace_id={id}&integration_id={id}
// Authentication is the provider's native signature (verified in the service).
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

	// Rate-limit cheaply BEFORE reading/parsing the (up to 25MB) body or hitting the DB.
	// workspace_id/integration_id are embedded in the provider's forward() URL and can leak,
	// so bound the work an attacker can force per workspace and per source IP. Keyed first
	// by IP, then by workspace, so one abusive source can't exhaust a workspace's budget.
	if h.rateLimiter != nil {
		clientIP := getClientIP(r)
		if !h.rateLimiter.Allow("inbound:ip", clientIP) {
			w.Header().Set("Retry-After", "60")
			WriteJSONError(w, "Too many requests", http.StatusTooManyRequests)
			return
		}
		if !h.rateLimiter.Allow("inbound:workspace", workspaceID) {
			w.Header().Set("Retry-After", "60")
			WriteJSONError(w, "Too many requests", http.StatusTooManyRequests)
			return
		}
	}

	// Cap the total body size, then build a neutral request: form for
	// multipart/urlencoded providers, raw body for JSON providers.
	r.Body = http.MaxBytesReader(w, r.Body, maxInboundReplyBytes)
	inboundReq := &domain.InboundRequest{
		Header:      r.Header,
		ContentType: r.Header.Get("Content-Type"),
		Query:       r.URL.Query(),
	}
	if ct := inboundReq.ContentType; strings.HasPrefix(ct, "multipart/form-data") ||
		strings.HasPrefix(ct, "application/x-www-form-urlencoded") {
		if err := r.ParseMultipartForm(maxInboundReplyBytes); err != nil && err != http.ErrNotMultipart {
			h.logger.WithField("error", err.Error()).
				WithField("workspace_id", workspaceID).
				Error("Failed to parse inbound reply form")
			WriteJSONError(w, "Failed to parse request body", http.StatusBadRequest)
			return
		}
		inboundReq.Form = r.PostForm
	} else {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			h.logger.WithField("error", err.Error()).
				WithField("workspace_id", workspaceID).
				Error("Failed to read inbound reply body")
			WriteJSONError(w, "Failed to read request body", http.StatusBadRequest)
			return
		}
		inboundReq.Body = body
	}

	h.logger.WithField("workspace_id", workspaceID).
		WithField("integration_id", integrationID).
		Info("Received inbound reply")

	if err := h.service.ProcessInboundReply(r.Context(), workspaceID, integrationID, inboundReq); err != nil {
		h.logger.WithField("error", err.Error()).
			WithField("workspace_id", workspaceID).
			WithField("integration_id", integrationID).
			Error("Failed to process inbound reply")

		// Permanent client/config errors → 4xx so the provider stops retrying. Everything
		// else (infra, DB, parse) → 5xx so the provider retries transient failures. The
		// service returns nil for ignored/expected cases (unmatched, bounce, auto-reply).
		var wsNotFound *domain.ErrWorkspaceNotFound
		switch {
		case errors.Is(err, domain.ErrInboundIntegrationNotFound), errors.As(err, &wsNotFound):
			WriteJSONError(w, "Unknown workspace or integration", http.StatusNotFound)
		case errors.Is(err, domain.ErrInboundProviderUnsupported):
			WriteJSONError(w, "Provider does not support inbound replies", http.StatusBadRequest)
		default:
			WriteJSONError(w, "Failed to process inbound reply", http.StatusInternalServerError)
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
	})
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
		if writeServiceError(w, err, "You do not have permission to read webhook events") {
			return
		}
		WriteJSONError(w, "Failed to list inbound webhook events", http.StatusInternalServerError)
		return
	}

	// Return the results
	writeJSON(w, http.StatusOK, result)
}
