package http

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/http/middleware"
	"github.com/Notifuse/notifuse/pkg/logger"
)

type TemplateBlockHandler struct {
	service      domain.TemplateBlockService
	logger       logger.Logger
	getJWTSecret func() ([]byte, error)
}

func NewTemplateBlockHandler(service domain.TemplateBlockService, getJWTSecret func() ([]byte, error), logger logger.Logger) *TemplateBlockHandler {
	return &TemplateBlockHandler{
		service:      service,
		logger:       logger,
		getJWTSecret: getJWTSecret,
	}
}

func (h *TemplateBlockHandler) RegisterRoutes(mux *http.ServeMux) {
	// Create auth middleware
	authMiddleware := middleware.NewAuthMiddleware(h.getJWTSecret)
	requireAuth := authMiddleware.RequireAuth()

	// Register RPC-style endpoints with dot notation
	mux.Handle("/api/templateBlocks.list", requireAuth(http.HandlerFunc(h.handleList)))
	mux.Handle("/api/templateBlocks.get", requireAuth(http.HandlerFunc(h.handleGet)))
	mux.Handle("/api/templateBlocks.create", requireAuth(http.HandlerFunc(h.handleCreate)))
	mux.Handle("/api/templateBlocks.update", requireAuth(http.HandlerFunc(h.handleUpdate)))
	mux.Handle("/api/templateBlocks.delete", requireAuth(http.HandlerFunc(h.handleDelete)))
}

func (h *TemplateBlockHandler) handleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.ListTemplateBlocksRequest
	if err := req.FromURLParams(r.URL.Query()); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	blocks, err := h.service.ListTemplateBlocks(r.Context(), req.WorkspaceID)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to list template blocks")
		WriteJSONError(w, "Failed to list template blocks", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"blocks": blocks,
	})
}

func (h *TemplateBlockHandler) handleGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.GetTemplateBlockRequest
	if err := req.FromURLParams(r.URL.Query()); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	block, err := h.service.GetTemplateBlock(r.Context(), req.WorkspaceID, req.ID)
	if err != nil {
		if _, ok := err.(*domain.ErrTemplateBlockNotFound); ok {
			WriteJSONError(w, "Template block not found", http.StatusNotFound)
			return
		}
		h.logger.WithField("error", err.Error()).Error("Failed to get template block")
		WriteJSONError(w, "Failed to get template block", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"block": block,
	})
}

func (h *TemplateBlockHandler) handleCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.CreateTemplateBlockRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to decode request body")
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	block, workspaceID, err := req.Validate()
	if err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.service.CreateTemplateBlock(r.Context(), workspaceID, block); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to create template block")

		if strings.Contains(err.Error(), "already exists") {
			WriteJSONError(w, "Template block id already exists", http.StatusBadRequest)
			return
		}

		WriteJSONError(w, "Failed to create template block", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"block": block,
	})
}

func (h *TemplateBlockHandler) handleUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.UpdateTemplateBlockRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to decode request body")
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	block, workspaceID, err := req.Validate()
	if err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.service.UpdateTemplateBlock(r.Context(), workspaceID, block); err != nil {
		if _, ok := err.(*domain.ErrTemplateBlockNotFound); ok {
			WriteJSONError(w, "Template block not found", http.StatusNotFound)
			return
		}
		h.logger.WithField("error", err.Error()).Error("Failed to update template block")
		WriteJSONError(w, "Failed to update template block", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"block": block,
	})
}

func (h *TemplateBlockHandler) handleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.DeleteTemplateBlockRequest
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

	if err := h.service.DeleteTemplateBlock(r.Context(), workspaceID, id); err != nil {
		if _, ok := err.(*domain.ErrTemplateBlockNotFound); ok {
			WriteJSONError(w, "Template block not found", http.StatusNotFound)
			return
		}
		h.logger.WithField("error", err.Error()).Error("Failed to delete template block")
		WriteJSONError(w, "Failed to delete template block", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
	})
}
