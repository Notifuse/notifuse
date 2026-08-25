package http

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/http/middleware"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// ListServiceWithSubscriptionResults is the list service as this handler needs it.
// lists.subscribe answers with the membership each requested list ended up in, and
// domain.ListService.SubscribeToLists returns an error alone. The richer call is
// declared here, in the package that consumes it, rather than widened on the domain
// interface: the service's other callers — the notification center, workspace
// onboarding, the demo seeder — have no use for the statuses, and every mock of
// domain.ListService would have to grow the method to satisfy them.
type ListServiceWithSubscriptionResults interface {
	domain.ListService

	// SubscribeToListsWithResults subscribes and reports one membership per
	// requested list, including the lists it deliberately left untouched.
	SubscribeToListsWithResults(ctx context.Context, payload *domain.SubscribeToListsRequest, hasBearerToken bool) ([]*domain.ContactList, error)
}

type ListHandler struct {
	service      ListServiceWithSubscriptionResults
	logger       logger.Logger
	getJWTSecret func() ([]byte, error)
}

func NewListHandler(service ListServiceWithSubscriptionResults, getJWTSecret func() ([]byte, error), logger logger.Logger) *ListHandler {
	return &ListHandler{
		service:      service,
		logger:       logger,
		getJWTSecret: getJWTSecret,
	}
}

func (h *ListHandler) RegisterRoutes(mux *http.ServeMux) {

	// Create auth middleware
	authMiddleware := middleware.NewAuthMiddleware(h.getJWTSecret)
	requireAuth := authMiddleware.RequireAuth()

	// Register RPC-style endpoints with dot notation
	mux.Handle("/api/lists.list", requireAuth(http.HandlerFunc(h.handleList)))
	mux.Handle("/api/lists.get", requireAuth(http.HandlerFunc(h.handleGet)))
	mux.Handle("/api/lists.create", requireAuth(http.HandlerFunc(h.handleCreate)))
	mux.Handle("/api/lists.update", requireAuth(http.HandlerFunc(h.handleUpdate)))
	mux.Handle("/api/lists.delete", requireAuth(http.HandlerFunc(h.handleDelete)))
	mux.Handle("/api/lists.stats", requireAuth(http.HandlerFunc(h.handleStats)))
	mux.Handle("/api/lists.subscribe", requireAuth(http.HandlerFunc(h.handleSubscribe)))
}

func (h *ListHandler) handleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.GetListsRequest
	if err := req.FromURLParams(r.URL.Query()); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	lists, err := h.service.GetLists(r.Context(), req.WorkspaceID)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to get lists")
		if writeServiceError(w, err, "You do not have access to this workspace") {
			return
		}
		WriteJSONError(w, "Failed to get lists", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"lists": lists,
	})
}

func (h *ListHandler) handleGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.GetListRequest
	if err := req.FromURLParams(r.URL.Query()); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	list, err := h.service.GetListByID(r.Context(), req.WorkspaceID, req.ID)
	if err != nil {
		if _, ok := err.(*domain.ErrListNotFound); ok {
			WriteJSONError(w, "List not found", http.StatusNotFound)
			return
		}
		h.logger.WithField("error", err.Error()).Error("Failed to get list")
		if writeServiceError(w, err, "You do not have access to this workspace") {
			return
		}
		WriteJSONError(w, "Failed to get list", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"list": list,
	})
}

func (h *ListHandler) handleCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.CreateListRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to decode request body")
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	list, workspaceID, err := req.Validate()
	if err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.service.CreateList(r.Context(), workspaceID, list); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to create list")
		if writeServiceError(w, err, "You do not have access to this workspace") {
			return
		}
		WriteJSONError(w, "Failed to create list", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"list": list,
	})
}

func (h *ListHandler) handleUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.UpdateListRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to decode request body")
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	list, workspaceID, err := req.Validate()
	if err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.service.UpdateList(r.Context(), workspaceID, list); err != nil {
		if _, ok := err.(*domain.ErrListNotFound); ok {
			WriteJSONError(w, "List not found", http.StatusNotFound)
			return
		}
		h.logger.WithField("error", err.Error()).Error("Failed to update list")
		if writeServiceError(w, err, "You do not have access to this workspace") {
			return
		}
		WriteJSONError(w, "Failed to update list", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"list": list,
	})
}

func (h *ListHandler) handleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.DeleteListRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to decode request body")
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	workspaceID, err := req.Validate()
	if err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.service.DeleteList(r.Context(), workspaceID, req.ID); err != nil {
		if _, ok := err.(*domain.ErrListNotFound); ok {
			WriteJSONError(w, "List not found", http.StatusNotFound)
			return
		}
		h.logger.WithField("error", err.Error()).Error("Failed to delete list")
		if writeServiceError(w, err, "You do not have access to this workspace") {
			return
		}
		WriteJSONError(w, "Failed to delete list", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
	})
}

func (h *ListHandler) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	listID := r.URL.Query().Get("list_id")
	if listID == "" {
		WriteJSONError(w, "list_id is required", http.StatusBadRequest)
		return
	}

	workspaceID := r.URL.Query().Get("workspace_id")
	if workspaceID == "" {
		WriteJSONError(w, "workspace_id is required", http.StatusBadRequest)
		return
	}

	stats, err := h.service.GetListStats(r.Context(), workspaceID, listID)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to get stats")
		if writeServiceError(w, err, "You do not have access to this workspace") {
			return
		}
		WriteJSONError(w, "Failed to get stats", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"list_id": listID,
		"stats":   stats,
	})
}

func (h *ListHandler) handleSubscribe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.SubscribeToListsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to decode request body")
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := req.Validate(); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to validate request")
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	hasBearerToken := true
	contactLists, err := h.service.SubscribeToListsWithResults(r.Context(), &req, hasBearerToken)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to subscribe to lists")
		if writeServiceError(w, err, "You do not have access to this workspace") {
			return
		}
		WriteJSONError(w, "Failed to subscribe to lists", http.StatusInternalServerError)
		return
	}

	// contact_lists is additive — "success" stays the key an existing client reads.
	// It carries the status each membership ended up in, which the request cannot
	// predict: double opt-in, a prior unsubscribe and a bounced address each decide
	// it server-side.
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":       true,
		"contact_lists": contactLists,
	})
}
