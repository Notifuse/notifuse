package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/http/middleware"
	"github.com/Notifuse/notifuse/pkg/logger"
)

type AnnotationHandler struct {
	service      domain.AnnotationService
	logger       logger.Logger
	getJWTSecret func() ([]byte, error)
	isDemo       bool
}

func NewAnnotationHandler(service domain.AnnotationService, getJWTSecret func() ([]byte, error), logger logger.Logger, isDemo bool) *AnnotationHandler {
	return &AnnotationHandler{
		service:      service,
		logger:       logger,
		getJWTSecret: getJWTSecret,
		isDemo:       isDemo,
	}
}

// RegisterRoutes registers the annotation HTTP endpoints
func (h *AnnotationHandler) RegisterRoutes(mux *http.ServeMux) {
	// Create auth middleware
	authMiddleware := middleware.NewAuthMiddleware(h.getJWTSecret)
	requireAuth := authMiddleware.RequireAuth()

	// The demo instance is publicly writable, so the mutating endpoints are closed
	// there — annotations are content, like blog posts and broadcasts, which are
	// already restricted. Reads stay open: the demo exists to be browsed.
	restrictedInDemo := middleware.RestrictedInDemo(h.isDemo)

	// Register RPC-style endpoints with dot notation
	mux.Handle("/api/annotations.list", requireAuth(http.HandlerFunc(h.handleList)))
	mux.Handle("/api/annotations.get", requireAuth(http.HandlerFunc(h.handleGet)))
	mux.Handle("/api/annotations.create", restrictedInDemo(requireAuth(http.HandlerFunc(h.handleCreate))))
	mux.Handle("/api/annotations.update", restrictedInDemo(requireAuth(http.HandlerFunc(h.handleUpdate))))
	mux.Handle("/api/annotations.delete", restrictedInDemo(requireAuth(http.HandlerFunc(h.handleDelete))))
}

// GET /api/annotations.list
func (h *AnnotationHandler) handleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.ListAnnotationsRequest
	// FromURLParams rejects a malformed start/end instead of dropping the filter:
	// answering the unfiltered question would be worse than refusing the request.
	if err := req.FromURLParams(r.URL.Query()); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	annotations, err := h.service.ListAnnotations(r.Context(), &req)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to list annotations")
		h.writeServiceError(w, err, "Failed to list annotations")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"annotations": annotations,
	})
}

// GET /api/annotations.get
func (h *AnnotationHandler) handleGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.GetAnnotationRequest
	if err := req.FromURLParams(r.URL.Query()); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	annotation, err := h.service.GetAnnotation(r.Context(), &req)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to get annotation")
		h.writeServiceError(w, err, "Failed to get annotation")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"annotation": annotation,
	})
}

// POST /api/annotations.create
func (h *AnnotationHandler) handleCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.CreateAnnotationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to decode request body")
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	annotation, err := h.service.CreateAnnotation(r.Context(), &req)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to create annotation")
		h.writeServiceError(w, err, "Failed to create annotation")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"annotation": annotation,
	})
}

// POST /api/annotations.update
func (h *AnnotationHandler) handleUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.UpdateAnnotationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to decode request body")
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	annotation, err := h.service.UpdateAnnotation(r.Context(), &req)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to update annotation")
		h.writeServiceError(w, err, "Failed to update annotation")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"annotation": annotation,
	})
}

// POST /api/annotations.delete
func (h *AnnotationHandler) handleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.DeleteAnnotationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to decode request body")
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := h.service.DeleteAnnotation(r.Context(), &req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to delete annotation")
		h.writeServiceError(w, err, "Failed to delete annotation")
		return
	}

	// A body rather than a 204: the console parses every response, and an empty
	// one would make the delete indistinguishable from a proxy swallowing it.
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
	})
}

// writeServiceError maps a service error onto a status code: permission denied
// first, then a missing row, then invalid input, and a generic 500 for everything
// else so an internal failure never surfaces its wording to the caller.
func (h *AnnotationHandler) writeServiceError(w http.ResponseWriter, err error, fallbackMessage string) {
	if writePermissionError(w, err) {
		return
	}

	var notFoundErr *domain.ErrNotFound
	if errors.As(err, &notFoundErr) {
		WriteJSONError(w, notFoundErr.Error(), http.StatusNotFound)
		return
	}

	// errors.As against a value target: ValidationError's Error has a value
	// receiver, so a %w-wrapped one is still matched — a type assertion would not
	// catch it and the caller would get a 500 for their own bad input.
	var validationErr domain.ValidationError
	if errors.As(err, &validationErr) {
		WriteJSONError(w, validationErr.Error(), http.StatusBadRequest)
		return
	}

	WriteJSONError(w, fallbackMessage, http.StatusInternalServerError)
}
