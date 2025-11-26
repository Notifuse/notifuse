package http

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/http/middleware"
	"github.com/Notifuse/notifuse/pkg/logger"
)

type CustomEventHandler struct {
	service      domain.CustomEventService
	logger       logger.Logger
	getJWTSecret func() ([]byte, error)
}

func NewCustomEventHandler(service domain.CustomEventService, getJWTSecret func() ([]byte, error), logger logger.Logger) *CustomEventHandler {
	return &CustomEventHandler{
		service:      service,
		getJWTSecret: getJWTSecret,
		logger:       logger,
	}
}

// RegisterRoutes registers the custom event HTTP endpoints
func (h *CustomEventHandler) RegisterRoutes(mux *http.ServeMux) {
	// Create auth middleware
	authMiddleware := middleware.NewAuthMiddleware(h.getJWTSecret)
	requireAuth := authMiddleware.RequireAuth()

	// Register RPC-style endpoints with dot notation
	mux.Handle("/api/customEvent.create", requireAuth(http.HandlerFunc(h.CreateCustomEvent)))
	mux.Handle("/api/customEvent.batchCreate", requireAuth(http.HandlerFunc(h.BatchCreateCustomEvents)))
	mux.Handle("/api/customEvent.get", requireAuth(http.HandlerFunc(h.GetCustomEvent)))
	mux.Handle("/api/customEvent.list", requireAuth(http.HandlerFunc(h.ListCustomEvents)))
}

// POST /api/customEvent.create
func (h *CustomEventHandler) CreateCustomEvent(w http.ResponseWriter, r *http.Request) {
	var req domain.CreateCustomEventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to decode request")
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	event, err := h.service.CreateEvent(r.Context(), &req)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to create custom event")
		if _, ok := err.(*domain.PermissionError); ok {
			WriteJSONError(w, err.Error(), http.StatusForbidden)
			return
		}
		WriteJSONError(w, "Failed to create custom event", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"event": event,
	})
}

// POST /api/customEvent.batchCreate
func (h *CustomEventHandler) BatchCreateCustomEvents(w http.ResponseWriter, r *http.Request) {
	var req domain.BatchCreateCustomEventsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to decode request")
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	eventIDs, err := h.service.BatchCreateEvents(r.Context(), &req)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to batch create custom events")
		if _, ok := err.(*domain.PermissionError); ok {
			WriteJSONError(w, err.Error(), http.StatusForbidden)
			return
		}
		WriteJSONError(w, "Failed to create custom events", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"event_ids": eventIDs,
		"count":     len(eventIDs),
	})
}

// GET /api/customEvent.get
func (h *CustomEventHandler) GetCustomEvent(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.URL.Query().Get("workspace_id")
	eventName := r.URL.Query().Get("event_name")
	externalID := r.URL.Query().Get("external_id")

	if workspaceID == "" || eventName == "" || externalID == "" {
		WriteJSONError(w, "workspace_id, event_name, and external_id are required", http.StatusBadRequest)
		return
	}

	event, err := h.service.GetEvent(r.Context(), workspaceID, eventName, externalID)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to get custom event")
		if _, ok := err.(*domain.PermissionError); ok {
			WriteJSONError(w, err.Error(), http.StatusForbidden)
			return
		}
		WriteJSONError(w, "Custom event not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"event": event,
	})
}

// GET /api/customEvent.list
func (h *CustomEventHandler) ListCustomEvents(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.URL.Query().Get("workspace_id")
	email := r.URL.Query().Get("email")
	eventName := r.URL.Query().Get("event_name")

	if workspaceID == "" {
		WriteJSONError(w, "workspace_id is required", http.StatusBadRequest)
		return
	}

	if email == "" && eventName == "" {
		WriteJSONError(w, "either email or event_name is required", http.StatusBadRequest)
		return
	}

	// Parse limit and offset
	limit := 50
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil {
			limit = parsedLimit
		}
	}

	offset := 0
	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if parsedOffset, err := strconv.Atoi(offsetStr); err == nil {
			offset = parsedOffset
		}
	}

	req := &domain.ListCustomEventsRequest{
		WorkspaceID: workspaceID,
		Email:       email,
		Limit:       limit,
		Offset:      offset,
	}

	if eventName != "" {
		req.EventName = &eventName
	}

	events, err := h.service.ListEvents(r.Context(), req)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to list custom events")
		if _, ok := err.(*domain.PermissionError); ok {
			WriteJSONError(w, err.Error(), http.StatusForbidden)
			return
		}
		WriteJSONError(w, "Failed to list custom events", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"events": events,
		"count":  len(events),
	})
}
