package http

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/http/middleware"
	"github.com/Notifuse/notifuse/pkg/logger"
)

type ContactHandler struct {
	service      domain.ContactService
	logger       logger.Logger
	getJWTSecret func() ([]byte, error)
}

func NewContactHandler(service domain.ContactService, getJWTSecret func() ([]byte, error), logger logger.Logger) *ContactHandler {
	return &ContactHandler{
		service:      service,
		getJWTSecret: getJWTSecret,
		logger:       logger,
	}
}

func (h *ContactHandler) RegisterRoutes(mux *http.ServeMux) {
	// Create auth middleware
	authMiddleware := middleware.NewAuthMiddleware(h.getJWTSecret)
	requireAuth := authMiddleware.RequireAuth()

	// Register RPC-style endpoints with dot notation
	mux.Handle("/api/contacts.list", requireAuth(http.HandlerFunc(h.handleList)))
	mux.Handle("/api/contacts.count", requireAuth(http.HandlerFunc(h.handleCount)))
	mux.Handle("/api/contacts.getByEmail", requireAuth(http.HandlerFunc(h.handleGetByEmail)))
	mux.Handle("/api/contacts.getByExternalID", requireAuth(http.HandlerFunc(h.handleGetByExternalID)))
	mux.Handle("/api/contacts.delete", requireAuth(http.HandlerFunc(h.handleDelete)))
	mux.Handle("/api/contacts.import", requireAuth(http.HandlerFunc(h.handleImport)))
	mux.Handle("/api/contacts.upsert", requireAuth(http.HandlerFunc(h.handleUpsert)))
}

func (h *ContactHandler) handleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Convert to domain request
	domainReq := &domain.GetContactsRequest{}
	if err := domainReq.FromQueryParams(r.URL.Query()); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	// Validate and set defaults (e.g. default limit)
	if err := domainReq.Validate(); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	// Get contacts from service
	response, err := h.service.GetContacts(r.Context(), domainReq)
	if err != nil {
		h.logger.Error(fmt.Sprintf("Failed to get contacts: %v", err))
		http.Error(w, "Failed to get contacts", http.StatusInternalServerError)
		return
	}

	// Write response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.logger.Error(fmt.Sprintf("Failed to encode response: %v", err))
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

func (h *ContactHandler) handleCount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get workspace_id from query params
	workspaceID := r.URL.Query().Get("workspace_id")
	if workspaceID == "" {
		WriteJSONError(w, "Missing workspace ID", http.StatusBadRequest)
		return
	}

	count, err := h.service.CountContacts(r.Context(), workspaceID)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to count contacts")
		WriteJSONError(w, "Failed to count contacts", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"total_contacts": count,
	})
}

func (h *ContactHandler) handleGetByEmail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get email from query params
	workspaceID := r.URL.Query().Get("workspace_id")
	if workspaceID == "" {
		WriteJSONError(w, "Missing workspace ID", http.StatusBadRequest)
		return
	}
	email := r.URL.Query().Get("email")
	if email == "" {
		WriteJSONError(w, "Missing email", http.StatusBadRequest)
		return
	}

	contact, err := h.service.GetContactByEmail(r.Context(), workspaceID, email)
	if err != nil {
		if strings.Contains(err.Error(), "contact not found") {
			WriteJSONError(w, "Contact not found", http.StatusNotFound)
			return
		}
		h.logger.WithField("error", err.Error()).Error("Failed to get contact by email")
		WriteJSONError(w, "Failed to get contact by email", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"contact": contact,
	})
}

func (h *ContactHandler) handleGetByExternalID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get external ID from query params
	workspaceID := r.URL.Query().Get("workspace_id")
	if workspaceID == "" {
		WriteJSONError(w, "Missing workspace ID", http.StatusBadRequest)
		return
	}
	externalID := r.URL.Query().Get("external_id")
	if externalID == "" {
		WriteJSONError(w, "Missing external ID", http.StatusBadRequest)
		return
	}

	contact, err := h.service.GetContactByExternalID(r.Context(), workspaceID, externalID)
	if err != nil {
		if strings.Contains(err.Error(), "contact not found") {
			WriteJSONError(w, "Contact not found", http.StatusNotFound)
			return
		}
		h.logger.WithField("error", err.Error()).Error("Failed to get contact by external ID")
		WriteJSONError(w, "Failed to get contact by external ID", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"contact": contact,
	})
}

func (h *ContactHandler) handleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.DeleteContactRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to decode request body")
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := req.Validate(); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.service.DeleteContact(r.Context(), req.WorkspaceID, req.Email); err != nil {
		if strings.Contains(err.Error(), "contact not found") {
			WriteJSONError(w, "Contact not found", http.StatusNotFound)
			return
		}
		h.logger.WithField("error", err.Error()).Error("Failed to delete contact")
		WriteJSONError(w, "Failed to delete contact", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
	})
}

func (h *ContactHandler) handleImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read the request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to read request body")
		WriteJSONError(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	var req domain.BatchImportContactsRequest
	if err := json.Unmarshal(body, &req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to decode request body")
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	contacts, workspaceID, err := req.Validate()
	if err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	result := h.service.BatchImportContacts(r.Context(), workspaceID, contacts, req.SubscribeToLists)
	if result.Error != "" {
		h.logger.WithField("error", result.Error).Error("Failed to import contacts")
		WriteJSONError(w, result.Error, http.StatusInternalServerError)
		return
	}

	// Write success response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(result); err != nil {
		WriteJSONError(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

func (h *ContactHandler) handleUpsert(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read the request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		WriteJSONError(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	// Parse the request body to get workspace_id and contact
	var requestData struct {
		WorkspaceID string          `json:"workspace_id"`
		Contact     json.RawMessage `json:"contact"`
	}
	if err := json.Unmarshal(body, &requestData); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Create the request with the parsed data
	req := domain.UpsertContactRequest{
		WorkspaceID: requestData.WorkspaceID,
		Contact:     requestData.Contact,
	}

	contact, workspaceID, err := req.Validate()
	if err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	result := h.service.UpsertContact(r.Context(), workspaceID, contact)
	if result.Action == domain.UpsertContactOperationError {
		h.logger.WithField("error", result.Error).Error("Failed to upsert contact")
		WriteJSONError(w, result.Error, http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusOK, result)
}
