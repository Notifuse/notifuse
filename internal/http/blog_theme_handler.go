package http

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/http/middleware"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// BlogThemeHandler handles HTTP requests for blog themes
type BlogThemeHandler struct {
	service      domain.BlogService
	logger       logger.Logger
	getJWTSecret func() ([]byte, error)
}

// NewBlogThemeHandler creates a new blog theme handler
func NewBlogThemeHandler(service domain.BlogService, getJWTSecret func() ([]byte, error), logger logger.Logger) *BlogThemeHandler {
	return &BlogThemeHandler{
		service:      service,
		logger:       logger,
		getJWTSecret: getJWTSecret,
	}
}

// RegisterRoutes registers the blog theme HTTP endpoints
func (h *BlogThemeHandler) RegisterRoutes(mux *http.ServeMux) {
	// Create auth middleware
	authMiddleware := middleware.NewAuthMiddleware(h.getJWTSecret)
	requireAuth := authMiddleware.RequireAuth()

	// Register RPC-style endpoints with dot notation
	mux.Handle("/api/blogThemes.create", requireAuth(http.HandlerFunc(h.HandleCreate)))
	mux.Handle("/api/blogThemes.get", requireAuth(http.HandlerFunc(h.HandleGet)))
	mux.Handle("/api/blogThemes.getPublished", requireAuth(http.HandlerFunc(h.HandleGetPublished)))
	mux.Handle("/api/blogThemes.update", requireAuth(http.HandlerFunc(h.HandleUpdate)))
	mux.Handle("/api/blogThemes.publish", requireAuth(http.HandlerFunc(h.HandlePublish)))
	mux.Handle("/api/blogThemes.list", requireAuth(http.HandlerFunc(h.HandleList)))
}

// HandleCreate creates a new blog theme
func (h *BlogThemeHandler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	workspace_id := r.URL.Query().Get("workspace_id")
	if workspace_id == "" {
		WriteJSONError(w, "workspace_id is required", http.StatusBadRequest)
		return
	}

	// Add workspace_id to context
	ctx := context.WithValue(r.Context(), domain.WorkspaceIDKey, workspace_id)

	var req domain.CreateBlogThemeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to decode request body")
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	theme, err := h.service.CreateTheme(ctx, &req)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to create theme")
		if permErr, ok := err.(*domain.PermissionError); ok {
			WriteJSONError(w, permErr.Error(), http.StatusForbidden)
			return
		}
		WriteJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"theme": theme,
	})
}

// HandleGet retrieves a blog theme by version
func (h *BlogThemeHandler) HandleGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	workspace_id := r.URL.Query().Get("workspace_id")
	if workspace_id == "" {
		WriteJSONError(w, "workspace_id is required", http.StatusBadRequest)
		return
	}

	// Add workspace_id to context
	ctx := context.WithValue(r.Context(), domain.WorkspaceIDKey, workspace_id)

	versionStr := r.URL.Query().Get("version")
	if versionStr == "" {
		WriteJSONError(w, "version parameter is required", http.StatusBadRequest)
		return
	}

	version, err := strconv.Atoi(versionStr)
	if err != nil {
		WriteJSONError(w, "version must be a valid integer", http.StatusBadRequest)
		return
	}

	theme, err := h.service.GetTheme(ctx, version)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to get theme")
		if permErr, ok := err.(*domain.PermissionError); ok {
			WriteJSONError(w, permErr.Error(), http.StatusForbidden)
			return
		}
		WriteJSONError(w, err.Error(), http.StatusNotFound)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"theme": theme,
	})
}

// HandleGetPublished retrieves the currently published blog theme
func (h *BlogThemeHandler) HandleGetPublished(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	workspace_id := r.URL.Query().Get("workspace_id")
	if workspace_id == "" {
		WriteJSONError(w, "workspace_id is required", http.StatusBadRequest)
		return
	}

	// Add workspace_id to context
	ctx := context.WithValue(r.Context(), domain.WorkspaceIDKey, workspace_id)

	theme, err := h.service.GetPublishedTheme(ctx)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to get published theme")
		if permErr, ok := err.(*domain.PermissionError); ok {
			WriteJSONError(w, permErr.Error(), http.StatusForbidden)
			return
		}
		WriteJSONError(w, err.Error(), http.StatusNotFound)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"theme": theme,
	})
}

// HandleUpdate updates an existing blog theme
func (h *BlogThemeHandler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	workspace_id := r.URL.Query().Get("workspace_id")
	if workspace_id == "" {
		WriteJSONError(w, "workspace_id is required", http.StatusBadRequest)
		return
	}

	// Add workspace_id to context
	ctx := context.WithValue(r.Context(), domain.WorkspaceIDKey, workspace_id)

	var req domain.UpdateBlogThemeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to decode request body")
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	theme, err := h.service.UpdateTheme(ctx, &req)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to update theme")
		if permErr, ok := err.(*domain.PermissionError); ok {
			WriteJSONError(w, permErr.Error(), http.StatusForbidden)
			return
		}
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"theme": theme,
	})
}

// HandlePublish publishes a blog theme
func (h *BlogThemeHandler) HandlePublish(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	workspace_id := r.URL.Query().Get("workspace_id")
	if workspace_id == "" {
		WriteJSONError(w, "workspace_id is required", http.StatusBadRequest)
		return
	}

	// Add workspace_id to context
	ctx := context.WithValue(r.Context(), domain.WorkspaceIDKey, workspace_id)

	var req domain.PublishBlogThemeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to decode request body")
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := h.service.PublishTheme(ctx, &req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to publish theme")
		if permErr, ok := err.(*domain.PermissionError); ok {
			WriteJSONError(w, permErr.Error(), http.StatusForbidden)
			return
		}
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Theme published successfully",
	})
}

// HandleList retrieves blog themes with pagination
func (h *BlogThemeHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	workspace_id := r.URL.Query().Get("workspace_id")
	if workspace_id == "" {
		WriteJSONError(w, "workspace_id is required", http.StatusBadRequest)
		return
	}

	// Add workspace_id to context
	ctx := context.WithValue(r.Context(), domain.WorkspaceIDKey, workspace_id)

	var req domain.ListBlogThemesRequest

	// Parse limit
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil {
			WriteJSONError(w, "limit must be a valid integer", http.StatusBadRequest)
			return
		}
		req.Limit = limit
	}

	// Parse offset
	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		offset, err := strconv.Atoi(offsetStr)
		if err != nil {
			WriteJSONError(w, "offset must be a valid integer", http.StatusBadRequest)
			return
		}
		req.Offset = offset
	}

	response, err := h.service.ListThemes(ctx, &req)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to list themes")
		if permErr, ok := err.(*domain.PermissionError); ok {
			WriteJSONError(w, permErr.Error(), http.StatusForbidden)
			return
		}
		WriteJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, response)
}
