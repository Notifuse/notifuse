package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/http/middleware"
	"github.com/Notifuse/notifuse/pkg/logger"
)

type BlogHandler struct {
	service      domain.BlogService
	logger       logger.Logger
	getJWTSecret func() ([]byte, error)
	isDemo       bool
}

func NewBlogHandler(service domain.BlogService, getJWTSecret func() ([]byte, error), logger logger.Logger, isDemo bool) *BlogHandler {
	return &BlogHandler{
		service:      service,
		logger:       logger,
		getJWTSecret: getJWTSecret,
		isDemo:       isDemo,
	}
}

func (h *BlogHandler) RegisterRoutes(mux *http.ServeMux) {
	// Create auth middleware
	authMiddleware := middleware.NewAuthMiddleware(h.getJWTSecret)
	requireAuth := authMiddleware.RequireAuth()

	restrictedInDemo := middleware.RestrictedInDemo(h.isDemo)

	// Register RPC-style endpoints with camelCase notation for categories
	mux.Handle("/api/blogCategories.list", requireAuth(http.HandlerFunc(h.HandleListCategories)))
	mux.Handle("/api/blogCategories.get", requireAuth(http.HandlerFunc(h.HandleGetCategory)))
	mux.Handle("/api/blogCategories.create", restrictedInDemo(requireAuth(http.HandlerFunc(h.HandleCreateCategory))))
	mux.Handle("/api/blogCategories.update", restrictedInDemo(requireAuth(http.HandlerFunc(h.HandleUpdateCategory))))
	mux.Handle("/api/blogCategories.delete", restrictedInDemo(requireAuth(http.HandlerFunc(h.HandleDeleteCategory))))

	// Register RPC-style endpoints with camelCase notation for posts
	mux.Handle("/api/blogPosts.list", requireAuth(http.HandlerFunc(h.HandleListPosts)))
	mux.Handle("/api/blogPosts.get", requireAuth(http.HandlerFunc(h.HandleGetPost)))
	mux.Handle("/api/blogPosts.create", restrictedInDemo(requireAuth(http.HandlerFunc(h.HandleCreatePost))))
	mux.Handle("/api/blogPosts.update", restrictedInDemo(requireAuth(http.HandlerFunc(h.HandleUpdatePost))))
	mux.Handle("/api/blogPosts.delete", restrictedInDemo(requireAuth(http.HandlerFunc(h.HandleDeletePost))))
	mux.Handle("/api/blogPosts.publish", restrictedInDemo(requireAuth(http.HandlerFunc(h.HandlePublishPost))))
	mux.Handle("/api/blogPosts.unpublish", restrictedInDemo(requireAuth(http.HandlerFunc(h.HandleUnpublishPost))))
}

// ====================
// Category Handlers
// ====================

// HandleListCategories handles the list categories request (GET)
func (h *BlogHandler) HandleListCategories(w http.ResponseWriter, r *http.Request) {
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

	response, err := h.service.ListCategories(ctx)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to list categories")
		WriteJSONError(w, "Failed to list categories", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"categories":  response.Categories,
		"total_count": response.TotalCount,
	})
}

// HandleGetCategory handles the get category request (GET)
func (h *BlogHandler) HandleGetCategory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	workspace_id := r.URL.Query().Get("workspace_id")
	if workspace_id == "" {
		WriteJSONError(w, "workspace_id is required", http.StatusBadRequest)
		return
	}

	id := r.URL.Query().Get("id")
	slug := r.URL.Query().Get("slug")

	if id == "" && slug == "" {
		WriteJSONError(w, "either id or slug is required", http.StatusBadRequest)
		return
	}

	// Add workspace_id to context
	ctx := context.WithValue(r.Context(), domain.WorkspaceIDKey, workspace_id)

	var category *domain.BlogCategory
	var err error

	if id != "" {
		category, err = h.service.GetCategory(ctx, id)
	} else {
		category, err = h.service.GetCategoryBySlug(ctx, slug)
	}

	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to get category")
		WriteJSONError(w, "Failed to get category", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"category": category,
	})
}

// HandleCreateCategory handles the create category request (POST)
func (h *BlogHandler) HandleCreateCategory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	workspace_id := r.URL.Query().Get("workspace_id")
	if workspace_id == "" {
		WriteJSONError(w, "workspace_id is required", http.StatusBadRequest)
		return
	}

	var req domain.CreateBlogCategoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Add workspace_id to context
	ctx := context.WithValue(r.Context(), domain.WorkspaceIDKey, workspace_id)

	category, err := h.service.CreateCategory(ctx, &req)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to create category")
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"category": category,
	})
}

// HandleUpdateCategory handles the update category request (POST)
func (h *BlogHandler) HandleUpdateCategory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	workspace_id := r.URL.Query().Get("workspace_id")
	if workspace_id == "" {
		WriteJSONError(w, "workspace_id is required", http.StatusBadRequest)
		return
	}

	var req domain.UpdateBlogCategoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Add workspace_id to context
	ctx := context.WithValue(r.Context(), domain.WorkspaceIDKey, workspace_id)

	category, err := h.service.UpdateCategory(ctx, &req)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to update category")
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"category": category,
	})
}

// HandleDeleteCategory handles the delete category request (POST)
func (h *BlogHandler) HandleDeleteCategory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	workspace_id := r.URL.Query().Get("workspace_id")
	if workspace_id == "" {
		WriteJSONError(w, "workspace_id is required", http.StatusBadRequest)
		return
	}

	var req domain.DeleteBlogCategoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Add workspace_id to context
	ctx := context.WithValue(r.Context(), domain.WorkspaceIDKey, workspace_id)

	if err := h.service.DeleteCategory(ctx, &req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to delete category")
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
	})
}

// ====================
// Post Handlers
// ====================

// HandleListPosts handles the list posts request (GET)
func (h *BlogHandler) HandleListPosts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	workspace_id := r.URL.Query().Get("workspace_id")
	if workspace_id == "" {
		WriteJSONError(w, "workspace_id is required", http.StatusBadRequest)
		return
	}

	params, err := parseListBlogPostsParams(r.URL.Query())
	if err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Add workspace_id to context
	ctx := context.WithValue(r.Context(), domain.WorkspaceIDKey, workspace_id)

	response, err := h.service.ListPosts(ctx, &params)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to list posts")
		WriteJSONError(w, "Failed to list posts", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"posts":       response.Posts,
		"total_count": response.TotalCount,
	})
}

// HandleGetPost handles the get post request (GET)
func (h *BlogHandler) HandleGetPost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	workspace_id := r.URL.Query().Get("workspace_id")
	if workspace_id == "" {
		WriteJSONError(w, "workspace_id is required", http.StatusBadRequest)
		return
	}

	id := r.URL.Query().Get("id")
	slug := r.URL.Query().Get("slug")
	categorySlug := r.URL.Query().Get("category_slug")

	if id == "" && slug == "" {
		WriteJSONError(w, "either id or slug is required", http.StatusBadRequest)
		return
	}

	// Add workspace_id to context
	ctx := context.WithValue(r.Context(), domain.WorkspaceIDKey, workspace_id)

	var post *domain.BlogPost
	var err error

	if id != "" {
		post, err = h.service.GetPost(ctx, id)
	} else if categorySlug != "" {
		post, err = h.service.GetPostByCategoryAndSlug(ctx, categorySlug, slug)
	} else {
		post, err = h.service.GetPostBySlug(ctx, slug)
	}

	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to get post")
		WriteJSONError(w, "Failed to get post", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"post": post,
	})
}

// HandleCreatePost handles the create post request (POST)
func (h *BlogHandler) HandleCreatePost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	workspace_id := r.URL.Query().Get("workspace_id")
	if workspace_id == "" {
		WriteJSONError(w, "workspace_id is required", http.StatusBadRequest)
		return
	}

	var req domain.CreateBlogPostRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Add workspace_id to context
	ctx := context.WithValue(r.Context(), domain.WorkspaceIDKey, workspace_id)

	post, err := h.service.CreatePost(ctx, &req)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to create post")
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"post": post,
	})
}

// HandleUpdatePost handles the update post request (POST)
func (h *BlogHandler) HandleUpdatePost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	workspace_id := r.URL.Query().Get("workspace_id")
	if workspace_id == "" {
		WriteJSONError(w, "workspace_id is required", http.StatusBadRequest)
		return
	}

	var req domain.UpdateBlogPostRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Add workspace_id to context
	ctx := context.WithValue(r.Context(), domain.WorkspaceIDKey, workspace_id)

	post, err := h.service.UpdatePost(ctx, &req)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to update post")
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"post": post,
	})
}

// HandleDeletePost handles the delete post request (POST)
func (h *BlogHandler) HandleDeletePost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	workspace_id := r.URL.Query().Get("workspace_id")
	if workspace_id == "" {
		WriteJSONError(w, "workspace_id is required", http.StatusBadRequest)
		return
	}

	var req domain.DeleteBlogPostRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Add workspace_id to context
	ctx := context.WithValue(r.Context(), domain.WorkspaceIDKey, workspace_id)

	if err := h.service.DeletePost(ctx, &req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to delete post")
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
	})
}

// HandlePublishPost handles the publish post request (POST)
func (h *BlogHandler) HandlePublishPost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	workspace_id := r.URL.Query().Get("workspace_id")
	if workspace_id == "" {
		WriteJSONError(w, "workspace_id is required", http.StatusBadRequest)
		return
	}

	var req domain.PublishBlogPostRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Add workspace_id to context
	ctx := context.WithValue(r.Context(), domain.WorkspaceIDKey, workspace_id)

	if err := h.service.PublishPost(ctx, &req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to publish post")
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
	})
}

// HandleUnpublishPost handles the unpublish post request (POST)
func (h *BlogHandler) HandleUnpublishPost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	workspace_id := r.URL.Query().Get("workspace_id")
	if workspace_id == "" {
		WriteJSONError(w, "workspace_id is required", http.StatusBadRequest)
		return
	}

	var req domain.UnpublishBlogPostRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Add workspace_id to context
	ctx := context.WithValue(r.Context(), domain.WorkspaceIDKey, workspace_id)

	if err := h.service.UnpublishPost(ctx, &req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to unpublish post")
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
	})
}

// ====================
// Helper Functions
// ====================

// parseListBlogPostsParams parses URL query parameters into ListBlogPostsRequest
func parseListBlogPostsParams(values url.Values) (domain.ListBlogPostsRequest, error) {
	params := domain.ListBlogPostsRequest{
		CategoryID: values.Get("category_id"),
		Status:     domain.BlogPostStatus(values.Get("status")),
	}

	// Parse limit
	if limitStr := values.Get("limit"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil {
			return params, err
		}
		params.Limit = limit
	}

	// Parse offset
	if offsetStr := values.Get("offset"); offsetStr != "" {
		offset, err := strconv.Atoi(offsetStr)
		if err != nil {
			return params, err
		}
		params.Offset = offset
	}

	return params, nil
}
