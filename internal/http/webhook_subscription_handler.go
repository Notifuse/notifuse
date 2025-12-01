package http

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/http/middleware"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// WebhookSubscriptionHandler handles HTTP requests for webhook subscriptions
type WebhookSubscriptionHandler struct {
	service      *service.WebhookSubscriptionService
	worker       *service.WebhookDeliveryWorker
	logger       logger.Logger
	getJWTSecret func() ([]byte, error)
}

// NewWebhookSubscriptionHandler creates a new webhook subscription handler
func NewWebhookSubscriptionHandler(
	svc *service.WebhookSubscriptionService,
	worker *service.WebhookDeliveryWorker,
	getJWTSecret func() ([]byte, error),
	logger logger.Logger,
) *WebhookSubscriptionHandler {
	return &WebhookSubscriptionHandler{
		service:      svc,
		worker:       worker,
		logger:       logger,
		getJWTSecret: getJWTSecret,
	}
}

// RegisterRoutes registers the webhook subscription routes
func (h *WebhookSubscriptionHandler) RegisterRoutes(mux *http.ServeMux) {
	authMiddleware := middleware.NewAuthMiddleware(h.getJWTSecret)
	requireAuth := authMiddleware.RequireAuth()

	mux.Handle("/api/webhookSubscriptions.create", requireAuth(http.HandlerFunc(h.handleCreate)))
	mux.Handle("/api/webhookSubscriptions.list", requireAuth(http.HandlerFunc(h.handleList)))
	mux.Handle("/api/webhookSubscriptions.get", requireAuth(http.HandlerFunc(h.handleGet)))
	mux.Handle("/api/webhookSubscriptions.update", requireAuth(http.HandlerFunc(h.handleUpdate)))
	mux.Handle("/api/webhookSubscriptions.delete", requireAuth(http.HandlerFunc(h.handleDelete)))
	mux.Handle("/api/webhookSubscriptions.toggle", requireAuth(http.HandlerFunc(h.handleToggle)))
	mux.Handle("/api/webhookSubscriptions.regenerateSecret", requireAuth(http.HandlerFunc(h.handleRegenerateSecret)))
	mux.Handle("/api/webhookSubscriptions.deliveries", requireAuth(http.HandlerFunc(h.handleGetDeliveries)))
	mux.Handle("/api/webhookSubscriptions.test", requireAuth(http.HandlerFunc(h.handleTest)))
	mux.Handle("/api/webhookSubscriptions.eventTypes", requireAuth(http.HandlerFunc(h.handleGetEventTypes)))
}

// handleCreate handles POST /api/webhookSubscriptions.create
func (h *WebhookSubscriptionHandler) handleCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		WorkspaceID        string                     `json:"workspace_id"`
		Name               string                     `json:"name"`
		URL                string                     `json:"url"`
		EventTypes         []string                   `json:"event_types"`
		CustomEventFilters *domain.CustomEventFilters `json:"custom_event_filters,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.WorkspaceID == "" {
		WriteJSONError(w, "workspace_id is required", http.StatusBadRequest)
		return
	}

	sub, err := h.service.Create(r.Context(), req.WorkspaceID, req.Name, req.URL, req.EventTypes, req.CustomEventFilters)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to create webhook subscription")
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"subscription": sub,
	})
}

// handleList handles GET /api/webhookSubscriptions.list
func (h *WebhookSubscriptionHandler) handleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	workspaceID := r.URL.Query().Get("workspace_id")
	if workspaceID == "" {
		WriteJSONError(w, "workspace_id is required", http.StatusBadRequest)
		return
	}

	subs, err := h.service.List(r.Context(), workspaceID)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to list webhook subscriptions")
		WriteJSONError(w, "Failed to list webhook subscriptions", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"subscriptions": subs,
	})
}

// handleGet handles GET /api/webhookSubscriptions.get
func (h *WebhookSubscriptionHandler) handleGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	workspaceID := r.URL.Query().Get("workspace_id")
	id := r.URL.Query().Get("id")

	if workspaceID == "" {
		WriteJSONError(w, "workspace_id is required", http.StatusBadRequest)
		return
	}
	if id == "" {
		WriteJSONError(w, "id is required", http.StatusBadRequest)
		return
	}

	sub, err := h.service.GetByID(r.Context(), workspaceID, id)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to get webhook subscription")
		WriteJSONError(w, "Webhook subscription not found", http.StatusNotFound)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"subscription": sub,
	})
}

// handleUpdate handles POST /api/webhookSubscriptions.update
func (h *WebhookSubscriptionHandler) handleUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		WorkspaceID        string                     `json:"workspace_id"`
		ID                 string                     `json:"id"`
		Name               string                     `json:"name"`
		URL                string                     `json:"url"`
		EventTypes         []string                   `json:"event_types"`
		CustomEventFilters *domain.CustomEventFilters `json:"custom_event_filters,omitempty"`
		Enabled            bool                       `json:"enabled"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.WorkspaceID == "" {
		WriteJSONError(w, "workspace_id is required", http.StatusBadRequest)
		return
	}
	if req.ID == "" {
		WriteJSONError(w, "id is required", http.StatusBadRequest)
		return
	}

	sub, err := h.service.Update(r.Context(), req.WorkspaceID, req.ID, req.Name, req.URL, req.EventTypes, req.CustomEventFilters, req.Enabled)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to update webhook subscription")
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"subscription": sub,
	})
}

// handleDelete handles POST /api/webhookSubscriptions.delete
func (h *WebhookSubscriptionHandler) handleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		WorkspaceID string `json:"workspace_id"`
		ID          string `json:"id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.WorkspaceID == "" {
		WriteJSONError(w, "workspace_id is required", http.StatusBadRequest)
		return
	}
	if req.ID == "" {
		WriteJSONError(w, "id is required", http.StatusBadRequest)
		return
	}

	if err := h.service.Delete(r.Context(), req.WorkspaceID, req.ID); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to delete webhook subscription")
		WriteJSONError(w, "Failed to delete webhook subscription", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
	})
}

// handleToggle handles POST /api/webhookSubscriptions.toggle
func (h *WebhookSubscriptionHandler) handleToggle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		WorkspaceID string `json:"workspace_id"`
		ID          string `json:"id"`
		Enabled     bool   `json:"enabled"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.WorkspaceID == "" {
		WriteJSONError(w, "workspace_id is required", http.StatusBadRequest)
		return
	}
	if req.ID == "" {
		WriteJSONError(w, "id is required", http.StatusBadRequest)
		return
	}

	sub, err := h.service.Toggle(r.Context(), req.WorkspaceID, req.ID, req.Enabled)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to toggle webhook subscription")
		WriteJSONError(w, "Failed to toggle webhook subscription", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"subscription": sub,
	})
}

// handleRegenerateSecret handles POST /api/webhookSubscriptions.regenerateSecret
func (h *WebhookSubscriptionHandler) handleRegenerateSecret(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		WorkspaceID string `json:"workspace_id"`
		ID          string `json:"id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.WorkspaceID == "" {
		WriteJSONError(w, "workspace_id is required", http.StatusBadRequest)
		return
	}
	if req.ID == "" {
		WriteJSONError(w, "id is required", http.StatusBadRequest)
		return
	}

	sub, err := h.service.RegenerateSecret(r.Context(), req.WorkspaceID, req.ID)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to regenerate webhook secret")
		WriteJSONError(w, "Failed to regenerate webhook secret", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"subscription": sub,
	})
}

// handleGetDeliveries handles GET /api/webhookSubscriptions.deliveries
func (h *WebhookSubscriptionHandler) handleGetDeliveries(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	workspaceID := r.URL.Query().Get("workspace_id")
	subscriptionID := r.URL.Query().Get("subscription_id")

	if workspaceID == "" {
		WriteJSONError(w, "workspace_id is required", http.StatusBadRequest)
		return
	}

	// subscription_id is optional - if not provided, returns all deliveries
	var subscriptionIDPtr *string
	if subscriptionID != "" {
		subscriptionIDPtr = &subscriptionID
	}

	limit := 20
	offset := 0

	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	deliveries, total, err := h.service.GetDeliveries(r.Context(), workspaceID, subscriptionIDPtr, limit, offset)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to get webhook deliveries")
		WriteJSONError(w, "Failed to get webhook deliveries", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"deliveries": deliveries,
		"total":      total,
		"limit":      limit,
		"offset":     offset,
	})
}

// handleTest handles POST /api/webhookSubscriptions.test
func (h *WebhookSubscriptionHandler) handleTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		WorkspaceID string `json:"workspace_id"`
		ID          string `json:"id"`
		EventType   string `json:"event_type"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.WorkspaceID == "" {
		WriteJSONError(w, "workspace_id is required", http.StatusBadRequest)
		return
	}
	if req.ID == "" {
		WriteJSONError(w, "id is required", http.StatusBadRequest)
		return
	}

	// Get the subscription
	sub, err := h.service.GetByID(r.Context(), req.WorkspaceID, req.ID)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to get webhook subscription")
		WriteJSONError(w, "Webhook subscription not found", http.StatusNotFound)
		return
	}

	// Send test webhook with event type
	statusCode, responseBody, err := h.worker.SendTestWebhook(r.Context(), req.WorkspaceID, sub, req.EventType)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"success":       false,
			"error":         err.Error(),
			"status_code":   0,
			"response_body": "",
		})
		return
	}

	success := statusCode >= 200 && statusCode < 300

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":       success,
		"status_code":   statusCode,
		"response_body": responseBody,
	})
}

// handleGetEventTypes handles GET /api/webhookSubscriptions.eventTypes
func (h *WebhookSubscriptionHandler) handleGetEventTypes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	eventTypes := h.service.GetEventTypes()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"event_types": eventTypes,
	})
}
