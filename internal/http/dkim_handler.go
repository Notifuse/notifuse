package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/http/middleware"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// DKIMHandler serves the DKIM keys of a workspace's domains. These are console
// settings, not part of the public API, so they are not in the OpenAPI document:
//
//	GET  /api/workspaces.listDKIMDomains?workspace_id=…  → {"domains": [DKIMDomainView]}
//	POST /api/workspaces.createDKIMDomain   {workspace_id, domain} → 201 {"domain": DKIMDomainView}
//	POST /api/workspaces.regenerateDKIMKey  {workspace_id, domain} → {"domain": DKIMDomainView}
//	POST /api/workspaces.verifyDKIMDomain   {workspace_id, domain} → DKIMVerifyResult
//	POST /api/workspaces.deleteDKIMDomain   {workspace_id, domain} → {"success": true}
//
// Errors: 400 invalid domain (and any change on the demo instance), 403 not an owner (any change) or no workspace read
// permission (list), 404 unknown domain, 409 a domain that already has a key, the cap
// reached, regenerating a published key, or deleting a key senders use ("used_by"
// names their integrations).
type DKIMHandler struct {
	service      domain.DKIMService
	logger       logger.Logger
	getJWTSecret func() ([]byte, error)
	isDemo       bool
}

// NewDKIMHandler returns the handler.
func NewDKIMHandler(service domain.DKIMService, getJWTSecret func() ([]byte, error), logger logger.Logger, isDemo bool) *DKIMHandler {
	return &DKIMHandler{service: service, logger: logger, getJWTSecret: getJWTSecret, isDemo: isDemo}
}

// RegisterRoutes registers the DKIM endpoints. The changes are closed on the demo
// instance, which sends nothing.
func (h *DKIMHandler) RegisterRoutes(mux *http.ServeMux) {
	requireAuth := middleware.NewAuthMiddleware(h.getJWTSecret).RequireAuth()
	restrictedInDemo := middleware.RestrictedInDemo(h.isDemo)

	mux.Handle("/api/workspaces.listDKIMDomains", requireAuth(http.HandlerFunc(h.handleList)))
	mux.Handle("/api/workspaces.createDKIMDomain", restrictedInDemo(requireAuth(http.HandlerFunc(h.handleCreate))))
	mux.Handle("/api/workspaces.regenerateDKIMKey", restrictedInDemo(requireAuth(http.HandlerFunc(h.handleRegenerate))))
	mux.Handle("/api/workspaces.verifyDKIMDomain", restrictedInDemo(requireAuth(http.HandlerFunc(h.handleVerify))))
	mux.Handle("/api/workspaces.deleteDKIMDomain", restrictedInDemo(requireAuth(http.HandlerFunc(h.handleDelete))))
}

func (h *DKIMHandler) handleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	workspaceID := r.URL.Query().Get("workspace_id")
	if workspaceID == "" {
		WriteJSONError(w, "workspace_id is required", http.StatusBadRequest)
		return
	}
	domains, err := h.service.ListDomains(r.Context(), workspaceID)
	if err != nil {
		h.writeError(w, err, "Failed to list DKIM domains")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"domains": domains})
}

// decode reads a POST body naming a domain.
func (h *DKIMHandler) decode(w http.ResponseWriter, r *http.Request) (*domain.DKIMDomainRequest, bool) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return nil, false
	}
	var req domain.DKIMDomainRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return nil, false
	}
	if err := req.Validate(); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return nil, false
	}
	return &req, true
}

func (h *DKIMHandler) handleCreate(w http.ResponseWriter, r *http.Request) {
	req, ok := h.decode(w, r)
	if !ok {
		return
	}
	view, err := h.service.CreateDomain(r.Context(), req.WorkspaceID, req.Domain)
	if err != nil {
		h.writeError(w, err, "Failed to create the DKIM key")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"domain": view})
}

func (h *DKIMHandler) handleRegenerate(w http.ResponseWriter, r *http.Request) {
	req, ok := h.decode(w, r)
	if !ok {
		return
	}
	view, err := h.service.RegenerateKey(r.Context(), req.WorkspaceID, req.Domain)
	if err != nil {
		h.writeError(w, err, "Failed to regenerate the DKIM key")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"domain": view})
}

func (h *DKIMHandler) handleVerify(w http.ResponseWriter, r *http.Request) {
	req, ok := h.decode(w, r)
	if !ok {
		return
	}
	result, err := h.service.VerifyDomain(r.Context(), req.WorkspaceID, req.Domain)
	if err != nil {
		h.writeError(w, err, "Failed to check the DKIM record")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *DKIMHandler) handleDelete(w http.ResponseWriter, r *http.Request) {
	req, ok := h.decode(w, r)
	if !ok {
		return
	}
	if err := h.service.DeleteDomain(r.Context(), req.WorkspaceID, req.Domain); err != nil {
		h.writeError(w, err, "Failed to delete the DKIM key")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true})
}

func (h *DKIMHandler) writeError(w http.ResponseWriter, err error, fallback string) {
	var invalid *domain.ErrDKIMInvalidDomain
	if errors.As(err, &invalid) {
		WriteJSONError(w, invalid.Error(), http.StatusBadRequest)
		return
	}
	var notFound *domain.ErrDKIMDomainNotFound
	if errors.As(err, &notFound) {
		WriteJSONError(w, notFound.Error(), http.StatusNotFound)
		return
	}
	var workspaceNotFound *domain.ErrWorkspaceNotFound
	if errors.As(err, &workspaceNotFound) {
		WriteJSONError(w, "Workspace not found", http.StatusNotFound)
		return
	}
	var conflict *domain.ErrDKIMConflict
	if errors.As(err, &conflict) {
		body := map[string]interface{}{"error": conflict.Error()}
		if len(conflict.UsedBy) > 0 {
			body["used_by"] = conflict.UsedBy
		}
		writeJSON(w, http.StatusConflict, body)
		return
	}
	if writeServiceError(w, err, fallback) {
		return
	}
	h.logger.WithField("error", err.Error()).Error(fallback)
	WriteJSONError(w, fallback, http.StatusInternalServerError)
}
