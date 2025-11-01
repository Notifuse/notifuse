package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/http/middleware"
	"github.com/Notifuse/notifuse/pkg/logger"
	// Import alias needed here too
)

type TemplateHandler struct {
	service   domain.TemplateService
	logger    logger.Logger
	getJWTSecret func() ([]byte, error)
}

func NewTemplateHandler(service domain.TemplateService, getJWTSecret func() ([]byte, error), logger logger.Logger) *TemplateHandler {
	return &TemplateHandler{
		service:   service,
		logger:    logger,
		getJWTSecret: getJWTSecret,
	}
}

func (h *TemplateHandler) RegisterRoutes(mux *http.ServeMux) {
	// Create auth middleware
	authMiddleware := middleware.NewAuthMiddleware(h.getJWTSecret)
	requireAuth := authMiddleware.RequireAuth()

	// Register RPC-style endpoints with dot notation
	mux.Handle("/api/templates.list", requireAuth(http.HandlerFunc(h.handleList)))
	mux.Handle("/api/templates.get", requireAuth(http.HandlerFunc(h.handleGet)))
	mux.Handle("/api/templates.create", requireAuth(http.HandlerFunc(h.handleCreate)))
	mux.Handle("/api/templates.update", requireAuth(http.HandlerFunc(h.handleUpdate)))
	mux.Handle("/api/templates.delete", requireAuth(http.HandlerFunc(h.handleDelete)))
	mux.Handle("/api/templates.compile", requireAuth(http.HandlerFunc(h.handleCompile)))
}

func (h *TemplateHandler) handleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.GetTemplatesRequest
	if err := req.FromURLParams(r.URL.Query()); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	templates, err := h.service.GetTemplates(r.Context(), req.WorkspaceID, req.Category)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to get templates")
		WriteJSONError(w, "Failed to get templates", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"templates": templates,
	})
}

func (h *TemplateHandler) handleGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.GetTemplateRequest
	if err := req.FromURLParams(r.URL.Query()); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	template, err := h.service.GetTemplateByID(r.Context(), req.WorkspaceID, req.ID, req.Version)
	if err != nil {
		if _, ok := err.(*domain.ErrTemplateNotFound); ok {
			WriteJSONError(w, "Template not found", http.StatusNotFound)
			return
		}
		h.logger.WithField("error", err.Error()).Error("Failed to get template")
		WriteJSONError(w, "Failed to get template", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"template": template,
	})
}

func (h *TemplateHandler) handleCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.CreateTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to decode request body")
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	template, workspaceID, err := req.Validate()
	if err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.service.CreateTemplate(r.Context(), workspaceID, template); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to create template")

		if strings.Contains(err.Error(), "duplicate key value violates unique constraint") {
			WriteJSONError(w, "Template id already exists", http.StatusBadRequest)
			return
		}

		WriteJSONError(w, "Failed to create template", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"template": template,
	})
}

func (h *TemplateHandler) handleUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.UpdateTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to decode request body")
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	template, workspaceID, err := req.Validate()
	if err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.service.UpdateTemplate(r.Context(), workspaceID, template); err != nil {
		if _, ok := err.(*domain.ErrTemplateNotFound); ok {
			WriteJSONError(w, "Template not found", http.StatusNotFound)
			return
		}
		h.logger.WithField("error", err.Error()).Error("Failed to update template")
		WriteJSONError(w, "Failed to update template", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"template": template,
	})
}

func (h *TemplateHandler) handleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.DeleteTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to decode request body")
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	workspaceID, id, err := req.Validate()
	if err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.service.DeleteTemplate(r.Context(), workspaceID, id); err != nil {
		if _, ok := err.(*domain.ErrTemplateNotFound); ok {
			WriteJSONError(w, "Template not found", http.StatusNotFound)
			return
		}
		h.logger.WithField("error", err.Error()).Error("Failed to delete template")
		WriteJSONError(w, "Failed to delete template", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
	})
}

func (h *TemplateHandler) handleCompile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.CompileTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to decode compile request body")
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := req.Validate(); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	resp, err := h.service.CompileTemplate(r.Context(), req)
	if err != nil {
		h.logger.WithField("error", err.Error()).Warn("Template compilation failed")
		WriteJSONError(w, fmt.Sprintf("Compilation failed: %s", err.Error()), http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusOK, resp)
}
