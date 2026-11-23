package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/http/middleware"
	"github.com/Notifuse/notifuse/pkg/logger"
)

type ContactListHandler struct {
	service      domain.ContactListService
	logger       logger.Logger
	getJWTSecret func() ([]byte, error)
}

func NewContactListHandler(service domain.ContactListService, getJWTSecret func() ([]byte, error), logger logger.Logger) *ContactListHandler {
	return &ContactListHandler{
		service:      service,
		getJWTSecret: getJWTSecret,
		logger:       logger,
	}
}

func (h *ContactListHandler) RegisterRoutes(mux *http.ServeMux) {
	// Create auth middleware
	authMiddleware := middleware.NewAuthMiddleware(h.getJWTSecret)
	requireAuth := authMiddleware.RequireAuth()

	// Register RPC-style endpoints with dot notation
	mux.Handle("/api/contactLists.getByIDs", requireAuth(http.HandlerFunc(h.handleGetByIDs)))
	mux.Handle("/api/contactLists.getContactsByList", requireAuth(http.HandlerFunc(h.handleGetContactsByList)))
	mux.Handle("/api/contactLists.getListsByContact", requireAuth(http.HandlerFunc(h.handleGetListsByContact)))
	mux.Handle("/api/contactLists.updateStatus", requireAuth(http.HandlerFunc(h.handleUpdateStatus)))
	mux.Handle("/api/contactLists.removeContact", requireAuth(http.HandlerFunc(h.handleRemoveContact)))
}

func (h *ContactListHandler) handleGetByIDs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.GetContactListRequest
	if err := req.FromURLParams(r.URL.Query()); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to parse request")
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	contactList, err := h.service.GetContactListByIDs(r.Context(), req.WorkspaceID, req.Email, req.ListID)
	if err != nil {
		if writeServiceError(w, err, "You do not have access to this workspace") {
			return
		}
		if _, ok := err.(*domain.ErrContactListNotFound); ok {
			WriteJSONError(w, "Contact list relationship not found", http.StatusNotFound)
			return
		}
		h.logger.WithField("error", err.Error()).Error("Failed to get contact list")
		WriteJSONError(w, "Failed to get contact list", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"contact_list": contactList,
	})
}

func (h *ContactListHandler) handleGetContactsByList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.GetContactsByListRequest
	if err := req.FromURLParams(r.URL.Query()); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to parse request")
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	contactLists, err := h.service.GetContactsByListID(r.Context(), req.WorkspaceID, req.ListID)
	if err != nil {
		if writeServiceError(w, err, "You do not have access to this workspace") {
			return
		}
		h.logger.WithField("error", err.Error()).Error("Failed to get contacts by list")
		WriteJSONError(w, "Failed to get contacts by list", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"contact_lists": contactLists,
	})
}

func (h *ContactListHandler) handleGetListsByContact(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.GetListsByContactRequest
	if err := req.FromURLParams(r.URL.Query()); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to parse request")
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	contactLists, err := h.service.GetListsByEmail(r.Context(), req.WorkspaceID, req.Email)
	if err != nil {
		if writeServiceError(w, err, "You do not have access to this workspace") {
			return
		}
		h.logger.WithField("error", err.Error()).Error("Failed to get lists by contact")
		WriteJSONError(w, "Failed to get lists by contact", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"contact_lists": contactLists,
	})
}

func (h *ContactListHandler) handleUpdateStatus(w http.ResponseWriter, r *http.Request) {
	// Audited only when a contact who had opted out was subscribed again, which the
	// service marks; every other call, refusals and malformed requests included, is
	// data plane and skipped.
	defer skipAuditUnlessResubscribed(r)

	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.UpdateContactListStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to decode request body")
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	workspaceID, contactList, err := req.Validate()
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to validate request")
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	result, err := h.service.UpdateContactListStatus(r.Context(), workspaceID, contactList.Email, contactList.ListID, contactList.Status, req.Resubscribe)
	if err != nil {
		if writeServiceError(w, err, "You do not have access to this workspace") {
			return
		}
		// The contact opted out and the change would undo it: the caller's request, not
		// a fault, and the message says what would be allowed.
		var optedOut *domain.ErrContactListOptedOut
		if errors.As(err, &optedOut) {
			writeJSON(w, http.StatusConflict, map[string]interface{}{
				"error":  optedOut.Error(),
				"status": optedOut.Status,
			})
			return
		}
		if _, ok := err.(*domain.ErrContactListNotFound); ok {
			WriteJSONError(w, err.Error(), http.StatusNotFound)
			return
		}
		h.logger.WithField("error", err.Error()).Error("Failed to update contact list status")
		WriteJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (h *ContactListHandler) handleRemoveContact(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.RemoveContactFromListRequest
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

	err := h.service.RemoveContactFromList(r.Context(), req.WorkspaceID, req.Email, req.ListID)
	if err != nil {
		if writeServiceError(w, err, "You do not have access to this workspace") {
			return
		}
		h.logger.WithField("error", err.Error()).Error("Failed to remove contact from list")
		// Surface "not in list" as 404 with the canonical error text so callers
		// (notably the contacts bulk remove flow) can map it to a "Skipped"
		// result instead of a hard failure.
		var notFound *domain.ErrContactListNotFound
		if errors.As(err, &notFound) {
			WriteJSONError(w, "contact list not found", http.StatusNotFound)
			return
		}
		WriteJSONError(w, "Failed to remove contact from list", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
	})
}

// skipAuditUnlessResubscribed drops the audit record of lists.subscribe and
// contactLists.updateStatus unless the service marked a re-subscription. Deferred at
// the top of both handlers, so it also covers their early 400 and 405 answers.
func skipAuditUnlessResubscribed(r *http.Request) {
	if audit := domain.AuditFromContext(r.Context()); !audit.HasMetadata(domain.AuditResubscribedListIDs) {
		audit.Skip()
	}
}
