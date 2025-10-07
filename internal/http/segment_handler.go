package http

import (
	"encoding/json"
	"net/http"
	"strconv"

	"aidanwoods.dev/go-paseto"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/http/middleware"
	"github.com/Notifuse/notifuse/pkg/logger"
)

type SegmentHandler struct {
	service   domain.SegmentService
	logger    logger.Logger
	getPublicKey func() (paseto.V4AsymmetricPublicKey, error)
}

func NewSegmentHandler(service domain.SegmentService, getPublicKey func() (paseto.V4AsymmetricPublicKey, error), logger logger.Logger) *SegmentHandler {
	return &SegmentHandler{
		service:   service,
		logger:    logger,
		getPublicKey:        getPublicKey,
	}
}

func (h *SegmentHandler) RegisterRoutes(mux *http.ServeMux) {
	// Create auth middleware
	authMiddleware := middleware.NewAuthMiddleware(h.getPublicKey)
	requireAuth := authMiddleware.RequireAuth()

	// Register RPC-style endpoints with dot notation
	mux.Handle("/api/segments.list", requireAuth(http.HandlerFunc(h.handleList)))
	mux.Handle("/api/segments.get", requireAuth(http.HandlerFunc(h.handleGet)))
	mux.Handle("/api/segments.create", requireAuth(http.HandlerFunc(h.handleCreate)))
	mux.Handle("/api/segments.update", requireAuth(http.HandlerFunc(h.handleUpdate)))
	mux.Handle("/api/segments.delete", requireAuth(http.HandlerFunc(h.handleDelete)))
	mux.Handle("/api/segments.rebuild", requireAuth(http.HandlerFunc(h.handleRebuild)))
	mux.Handle("/api/segments.preview", requireAuth(http.HandlerFunc(h.handlePreview)))
	mux.Handle("/api/segments.contacts", requireAuth(http.HandlerFunc(h.handleGetContacts)))
}

func (h *SegmentHandler) handleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.GetSegmentsRequest
	if err := req.FromURLParams(r.URL.Query()); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	segments, err := h.service.ListSegments(r.Context(), &req)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to get segments")
		WriteJSONError(w, "Failed to get segments", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"segments": segments,
	})
}

func (h *SegmentHandler) handleGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.GetSegmentRequest
	if err := req.FromURLParams(r.URL.Query()); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	segment, err := h.service.GetSegment(r.Context(), &req)
	if err != nil {
		if _, ok := err.(*domain.ErrSegmentNotFound); ok {
			WriteJSONError(w, "Segment not found", http.StatusNotFound)
			return
		}
		h.logger.WithField("error", err.Error()).Error("Failed to get segment")
		WriteJSONError(w, "Failed to get segment", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"segment": segment,
	})
}

func (h *SegmentHandler) handleCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.CreateSegmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to decode request body")
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	segment, err := h.service.CreateSegment(r.Context(), &req)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to create segment")
		WriteJSONError(w, "Failed to create segment", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"segment": segment,
	})
}

func (h *SegmentHandler) handleUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.UpdateSegmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to decode request body")
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	segment, err := h.service.UpdateSegment(r.Context(), &req)
	if err != nil {
		if _, ok := err.(*domain.ErrSegmentNotFound); ok {
			WriteJSONError(w, "Segment not found", http.StatusNotFound)
			return
		}
		h.logger.WithField("error", err.Error()).Error("Failed to update segment")
		WriteJSONError(w, "Failed to update segment", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"segment": segment,
	})
}

func (h *SegmentHandler) handleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.DeleteSegmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to decode request body")
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := h.service.DeleteSegment(r.Context(), &req); err != nil {
		if _, ok := err.(*domain.ErrSegmentNotFound); ok {
			WriteJSONError(w, "Segment not found", http.StatusNotFound)
			return
		}
		h.logger.WithField("error", err.Error()).Error("Failed to delete segment")
		WriteJSONError(w, "Failed to delete segment", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
	})
}

func (h *SegmentHandler) handleRebuild(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		WorkspaceID string `json:"workspace_id"`
		SegmentID   string `json:"segment_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to decode request body")
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.WorkspaceID == "" {
		WriteJSONError(w, "workspace_id is required", http.StatusBadRequest)
		return
	}

	if req.SegmentID == "" {
		WriteJSONError(w, "segment_id is required", http.StatusBadRequest)
		return
	}

	if err := h.service.RebuildSegment(r.Context(), req.WorkspaceID, req.SegmentID); err != nil {
		if _, ok := err.(*domain.ErrSegmentNotFound); ok {
			WriteJSONError(w, "Segment not found", http.StatusNotFound)
			return
		}
		h.logger.WithField("error", err.Error()).Error("Failed to rebuild segment")
		WriteJSONError(w, "Failed to rebuild segment", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Segment rebuild has been queued",
	})
}

func (h *SegmentHandler) handlePreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		WorkspaceID string           `json:"workspace_id"`
		Tree        *domain.TreeNode `json:"tree"`
		Limit       int              `json:"limit,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to decode request body")
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.WorkspaceID == "" {
		WriteJSONError(w, "workspace_id is required", http.StatusBadRequest)
		return
	}

	if req.Tree == nil {
		WriteJSONError(w, "tree is required", http.StatusBadRequest)
		return
	}

	// Default to 10 results
	if req.Limit == 0 {
		req.Limit = 10
	}

	response, err := h.service.PreviewSegment(r.Context(), req.WorkspaceID, req.Tree, req.Limit)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to preview segment")
		WriteJSONError(w, "Failed to preview segment", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, response)
}

func (h *SegmentHandler) handleGetContacts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	workspaceID := r.URL.Query().Get("workspace_id")
	if workspaceID == "" {
		WriteJSONError(w, "workspace_id is required", http.StatusBadRequest)
		return
	}

	segmentID := r.URL.Query().Get("segment_id")
	if segmentID == "" {
		WriteJSONError(w, "segment_id is required", http.StatusBadRequest)
		return
	}

	limit := 50
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}

	offset := 0
	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if parsedOffset, err := strconv.Atoi(offsetStr); err == nil && parsedOffset >= 0 {
			offset = parsedOffset
		}
	}

	emails, err := h.service.GetSegmentContacts(r.Context(), workspaceID, segmentID, limit, offset)
	if err != nil {
		if _, ok := err.(*domain.ErrSegmentNotFound); ok {
			WriteJSONError(w, "Segment not found", http.StatusNotFound)
			return
		}
		h.logger.WithField("error", err.Error()).Error("Failed to get segment contacts")
		WriteJSONError(w, "Failed to get segment contacts", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"emails": emails,
		"limit":  limit,
		"offset": offset,
	})
}
