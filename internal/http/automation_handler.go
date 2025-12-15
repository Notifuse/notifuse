package http

import (
	"encoding/json"
	"net/http"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/http/middleware"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// AutomationHandler handles HTTP requests for automation management
type AutomationHandler struct {
	service      domain.AutomationService
	logger       logger.Logger
	getJWTSecret func() ([]byte, error)
}

// NewAutomationHandler creates a new AutomationHandler
func NewAutomationHandler(service domain.AutomationService, getJWTSecret func() ([]byte, error), logger logger.Logger) *AutomationHandler {
	return &AutomationHandler{
		service:      service,
		logger:       logger,
		getJWTSecret: getJWTSecret,
	}
}

// RegisterRoutes registers the automation routes on the given mux
func (h *AutomationHandler) RegisterRoutes(mux *http.ServeMux) {
	authMiddleware := middleware.NewAuthMiddleware(h.getJWTSecret)
	requireAuth := authMiddleware.RequireAuth()

	// Automation CRUD
	mux.Handle("/api/automations.create", requireAuth(http.HandlerFunc(h.handleCreate)))
	mux.Handle("/api/automations.get", requireAuth(http.HandlerFunc(h.handleGet)))
	mux.Handle("/api/automations.list", requireAuth(http.HandlerFunc(h.handleList)))
	mux.Handle("/api/automations.update", requireAuth(http.HandlerFunc(h.handleUpdate)))
	mux.Handle("/api/automations.delete", requireAuth(http.HandlerFunc(h.handleDelete)))

	// Automation status management
	mux.Handle("/api/automations.activate", requireAuth(http.HandlerFunc(h.handleActivate)))
	mux.Handle("/api/automations.pause", requireAuth(http.HandlerFunc(h.handlePause)))

	// Node executions/debugging
	mux.Handle("/api/automations.nodeExecutions", requireAuth(http.HandlerFunc(h.handleGetContactNodeExecutions)))
}

func (h *AutomationHandler) handleCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.CreateAutomationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to decode request body")
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := req.Validate(); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.service.Create(r.Context(), req.WorkspaceID, req.Automation); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to create automation")
		if _, ok := err.(*domain.PermissionError); ok {
			WriteJSONError(w, err.Error(), http.StatusForbidden)
			return
		}
		WriteJSONError(w, "Failed to create automation", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"automation": req.Automation,
	})
}

func (h *AutomationHandler) handleGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.GetAutomationRequest
	if err := req.FromURLParams(r.URL.Query()); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	automation, err := h.service.Get(r.Context(), req.WorkspaceID, req.AutomationID)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to get automation")
		if _, ok := err.(*domain.PermissionError); ok {
			WriteJSONError(w, err.Error(), http.StatusForbidden)
			return
		}
		WriteJSONError(w, "Failed to get automation", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"automation": automation,
	})
}

func (h *AutomationHandler) handleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.ListAutomationsRequest
	if err := req.FromURLParams(r.URL.Query()); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	automations, total, err := h.service.List(r.Context(), req.WorkspaceID, req.ToFilter())
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to list automations")
		if _, ok := err.(*domain.PermissionError); ok {
			WriteJSONError(w, err.Error(), http.StatusForbidden)
			return
		}
		WriteJSONError(w, "Failed to list automations", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"automations": automations,
		"total":       total,
	})
}

func (h *AutomationHandler) handleUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.UpdateAutomationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to decode request body")
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := req.Validate(); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.service.Update(r.Context(), req.WorkspaceID, req.Automation); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to update automation")
		if _, ok := err.(*domain.PermissionError); ok {
			WriteJSONError(w, err.Error(), http.StatusForbidden)
			return
		}
		WriteJSONError(w, "Failed to update automation", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"automation": req.Automation,
	})
}

func (h *AutomationHandler) handleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.DeleteAutomationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to decode request body")
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := req.Validate(); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.service.Delete(r.Context(), req.WorkspaceID, req.AutomationID); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to delete automation")
		if _, ok := err.(*domain.PermissionError); ok {
			WriteJSONError(w, err.Error(), http.StatusForbidden)
			return
		}
		WriteJSONError(w, "Failed to delete automation", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
	})
}

func (h *AutomationHandler) handleActivate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.ActivateAutomationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to decode request body")
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := req.Validate(); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.service.Activate(r.Context(), req.WorkspaceID, req.AutomationID); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to activate automation")
		if _, ok := err.(*domain.PermissionError); ok {
			WriteJSONError(w, err.Error(), http.StatusForbidden)
			return
		}
		WriteJSONError(w, "Failed to activate automation", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
	})
}

func (h *AutomationHandler) handlePause(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.PauseAutomationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to decode request body")
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := req.Validate(); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.service.Pause(r.Context(), req.WorkspaceID, req.AutomationID); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to pause automation")
		if _, ok := err.(*domain.PermissionError); ok {
			WriteJSONError(w, err.Error(), http.StatusForbidden)
			return
		}
		WriteJSONError(w, "Failed to pause automation", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
	})
}

func (h *AutomationHandler) handleGetContactNodeExecutions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.GetContactNodeExecutionsRequest
	if err := req.FromURLParams(r.URL.Query()); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	contactAutomation, nodeExecutions, err := h.service.GetContactNodeExecutions(r.Context(), req.WorkspaceID, req.AutomationID, req.Email)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to get contact node executions")
		if _, ok := err.(*domain.PermissionError); ok {
			WriteJSONError(w, err.Error(), http.StatusForbidden)
			return
		}
		WriteJSONError(w, "Failed to get contact node executions", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"contact_automation": contactAutomation,
		"node_executions":    nodeExecutions,
	})
}
