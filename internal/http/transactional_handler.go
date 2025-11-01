package http

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/http/middleware"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// TransactionalNotificationHandler handles HTTP requests for transactional notifications
type TransactionalNotificationHandler struct {
	service   domain.TransactionalNotificationService
	logger    logger.Logger
	getJWTSecret func() ([]byte, error)
	isDemo    bool
}

// NewTransactionalNotificationHandler creates a new instance of TransactionalNotificationHandler
func NewTransactionalNotificationHandler(
	service domain.TransactionalNotificationService,
	getJWTSecret func() ([]byte, error),
	logger logger.Logger,
	isDemo bool,
) *TransactionalNotificationHandler {
	return &TransactionalNotificationHandler{
		service:   service,
		logger:    logger,
		getJWTSecret: getJWTSecret,
		isDemo:    isDemo,
	}
}

// RegisterRoutes registers all routes for transactional notifications
func (h *TransactionalNotificationHandler) RegisterRoutes(mux *http.ServeMux) {
	// Create auth middleware
	authMiddleware := middleware.NewAuthMiddleware(h.getJWTSecret)
	requireAuth := authMiddleware.RequireAuth()

	restrictedInDemo := middleware.RestrictedInDemo(h.isDemo)

	// Register RPC-style endpoints with dot notation
	mux.Handle("/api/transactional.list", requireAuth(http.HandlerFunc(h.handleList)))
	mux.Handle("/api/transactional.get", requireAuth(http.HandlerFunc(h.handleGet)))
	mux.Handle("/api/transactional.create", requireAuth(http.HandlerFunc(h.handleCreate)))
	mux.Handle("/api/transactional.update", requireAuth(http.HandlerFunc(h.handleUpdate)))
	mux.Handle("/api/transactional.delete", requireAuth(http.HandlerFunc(h.handleDelete)))
	mux.Handle("/api/transactional.send", restrictedInDemo(requireAuth(http.HandlerFunc(h.handleSend))))
	mux.Handle("/api/transactional.testTemplate", restrictedInDemo(requireAuth(http.HandlerFunc(h.handleTestTemplate))))
}

// Handler methods
func (h *TransactionalNotificationHandler) handleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.ListTransactionalRequest
	if err := req.FromURLParams(r.URL.Query()); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	notifications, total, err := h.service.ListNotifications(
		r.Context(),
		req.WorkspaceID,
		req.Filter,
		req.Limit,
		req.Offset,
	)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to list transactional notifications")
		WriteJSONError(w, "Failed to list notifications", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"notifications": notifications,
		"total":         total,
	})
}

func (h *TransactionalNotificationHandler) handleGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.GetTransactionalRequest
	if err := req.FromURLParams(r.URL.Query()); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	notification, err := h.service.GetNotification(r.Context(), req.WorkspaceID, req.ID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			WriteJSONError(w, "Notification not found", http.StatusNotFound)
			return
		}
		h.logger.WithField("error", err.Error()).Error("Failed to get transactional notification")
		WriteJSONError(w, "Failed to get notification", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"notification": notification,
	})
}

func (h *TransactionalNotificationHandler) handleCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.CreateTransactionalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to decode request body")
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := req.Validate(); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	notification, err := h.service.CreateNotification(r.Context(), req.WorkspaceID, req.Notification)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to create transactional notification")

		if strings.Contains(err.Error(), "invalid template") {
			WriteJSONError(w, err.Error(), http.StatusBadRequest)
			return
		}

		WriteJSONError(w, "Failed to create notification", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"notification": notification,
	})
}

func (h *TransactionalNotificationHandler) handleUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.UpdateTransactionalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to decode request body")
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := req.Validate(); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	notification, err := h.service.UpdateNotification(r.Context(), req.WorkspaceID, req.ID, req.Updates)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			WriteJSONError(w, "Notification not found", http.StatusNotFound)
			return
		}
		if strings.Contains(err.Error(), "invalid template") {
			WriteJSONError(w, err.Error(), http.StatusBadRequest)
			return
		}
		h.logger.WithField("error", err.Error()).Error("Failed to update transactional notification")
		WriteJSONError(w, "Failed to update notification", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"notification": notification,
	})
}

func (h *TransactionalNotificationHandler) handleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.DeleteTransactionalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to decode request body")
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := req.Validate(); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.service.DeleteNotification(r.Context(), req.WorkspaceID, req.ID); err != nil {
		if strings.Contains(err.Error(), "not found") {
			WriteJSONError(w, "Notification not found", http.StatusNotFound)
			return
		}
		h.logger.WithField("error", err.Error()).Error("Failed to delete transactional notification")
		WriteJSONError(w, "Failed to delete notification", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
	})
}

func (h *TransactionalNotificationHandler) handleSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.SendTransactionalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to decode request body")
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := req.Validate(); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	messageID, err := h.service.SendNotification(r.Context(), req.WorkspaceID, req.Notification)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to send transactional notification")

		if strings.Contains(err.Error(), "not found") ||
			strings.Contains(err.Error(), "not active") ||
			strings.Contains(err.Error(), "no valid channels") {
			WriteJSONError(w, err.Error(), http.StatusBadRequest)
			return
		}

		WriteJSONError(w, "Failed to send notification", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message_id": messageID,
		"success":    true,
	})
}

// handleTestTemplate handles requests to test a template
func (h *TransactionalNotificationHandler) handleTestTemplate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.TestTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to decode request body")
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	err := req.Validate()
	if err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	err = h.service.TestTemplate(r.Context(), req.WorkspaceID, req.TemplateID, req.IntegrationID, req.SenderID, req.RecipientEmail, req.EmailOptions)

	// Create response
	response := domain.TestTemplateResponse{
		Success: err == nil,
	}

	// If there's an error, include it in the response
	if err != nil {
		if _, ok := err.(*domain.ErrTemplateNotFound); ok {
			WriteJSONError(w, "Template not found", http.StatusNotFound)
			return
		}

		h.logger.WithFields(map[string]interface{}{
			"error":        err.Error(),
			"workspace_id": req.WorkspaceID,
			"template_id":  req.TemplateID,
		}).Error("Failed to test template")

		response.Error = err.Error()
	}

	writeJSON(w, http.StatusOK, response)
}
