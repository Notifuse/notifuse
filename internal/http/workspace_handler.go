package http

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/http/middleware"
	"github.com/Notifuse/notifuse/pkg/logger"

	"aidanwoods.dev/go-paseto"
)

// WorkspaceHandler handles HTTP requests for workspace operations
type WorkspaceHandler struct {
	workspaceService domain.WorkspaceServiceInterface
	authService      domain.AuthService
	publicKey        paseto.V4AsymmetricPublicKey
	logger           logger.Logger
	secretKey        string
}

// NewWorkspaceHandler creates a new workspace handler
func NewWorkspaceHandler(
	workspaceService domain.WorkspaceServiceInterface,
	authService domain.AuthService,
	publicKey paseto.V4AsymmetricPublicKey,
	logger logger.Logger,
	secretKey string,
) *WorkspaceHandler {
	return &WorkspaceHandler{
		workspaceService: workspaceService,
		authService:      authService,
		publicKey:        publicKey,
		logger:           logger,
		secretKey:        secretKey,
	}
}

// RegisterRoutes registers all workspace RPC-style routes with authentication middleware
func (h *WorkspaceHandler) RegisterRoutes(mux *http.ServeMux) {
	// Create auth middleware
	authMiddleware := middleware.NewAuthMiddleware(h.publicKey)
	requireAuth := authMiddleware.RequireAuth()

	// Register RPC-style endpoints with dot notation
	mux.Handle("/api/workspaces.list", requireAuth(http.HandlerFunc(h.handleList)))
	mux.Handle("/api/workspaces.get", requireAuth(http.HandlerFunc(h.handleGet)))
	mux.Handle("/api/workspaces.create", requireAuth(http.HandlerFunc(h.handleCreate)))
	mux.Handle("/api/workspaces.update", requireAuth(http.HandlerFunc(h.handleUpdate)))
	mux.Handle("/api/workspaces.delete", requireAuth(http.HandlerFunc(h.handleDelete)))
	mux.Handle("/api/workspaces.members", requireAuth(http.HandlerFunc(h.handleMembers)))
	mux.Handle("/api/workspaces.inviteMember", requireAuth(http.HandlerFunc(h.handleInviteMember)))
	mux.Handle("/api/workspaces.createAPIKey", requireAuth(http.HandlerFunc(h.handleCreateAPIKey)))
	mux.Handle("/api/workspaces.removeMember", requireAuth(http.HandlerFunc(h.handleRemoveMember)))

	// Integration management routes
	mux.Handle("/api/workspaces.createIntegration", requireAuth(http.HandlerFunc(h.handleCreateIntegration)))
	mux.Handle("/api/workspaces.updateIntegration", requireAuth(http.HandlerFunc(h.handleUpdateIntegration)))
	mux.Handle("/api/workspaces.deleteIntegration", requireAuth(http.HandlerFunc(h.handleDeleteIntegration)))
}

func (h *WorkspaceHandler) handleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	workspaces, err := h.workspaceService.ListWorkspaces(r.Context())
	if err != nil {
		WriteJSONError(w, "Failed to list workspaces", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, workspaces)
}

func (h *WorkspaceHandler) handleGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get workspace ID from query params
	workspaceID := r.URL.Query().Get("id")
	if workspaceID == "" {
		WriteJSONError(w, "Missing workspace ID", http.StatusBadRequest)
		return
	}

	workspace, err := h.workspaceService.GetWorkspace(r.Context(), workspaceID)
	if err != nil {
		WriteJSONError(w, "Failed to get workspace", http.StatusInternalServerError)
		return
	}
	if workspace == nil {
		WriteJSONError(w, "Workspace not found", http.StatusNotFound)
		return
	}

	// Wrap the workspace in a response object with a workspace field to match frontend expectations
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"workspace": workspace,
	})
}

func (h *WorkspaceHandler) handleCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.CreateWorkspaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := req.Validate(h.secretKey); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	workspace, err := h.workspaceService.CreateWorkspace(
		r.Context(),
		req.ID,
		req.Name,
		req.Settings.WebsiteURL,
		req.Settings.LogoURL,
		req.Settings.CoverURL,
		req.Settings.Timezone,
		req.Settings.FileManager,
	)
	if err != nil {
		if err.Error() == "workspace already exists" {
			WriteJSONError(w, "Workspace already exists", http.StatusConflict)
		} else {
			WriteJSONError(w, "Failed to create workspace", http.StatusInternalServerError)
		}
		return
	}

	writeJSON(w, http.StatusCreated, workspace)
}

// Helper function to get bytes from request body
func getBytesFromBody(body io.ReadCloser) []byte {
	buf := new(bytes.Buffer)
	buf.ReadFrom(body)
	return buf.Bytes()
}

func (h *WorkspaceHandler) handleUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.UpdateWorkspaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := req.Validate(h.secretKey); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	workspace, err := h.workspaceService.UpdateWorkspace(
		r.Context(),
		req.ID,
		req.Name,
		req.Settings,
	)
	if err != nil {
		WriteJSONError(w, "Failed to update workspace", http.StatusInternalServerError)
		return
	}
	if workspace == nil {
		WriteJSONError(w, "Workspace not found", http.StatusNotFound)
		return
	}

	writeJSON(w, http.StatusOK, workspace)
}

func (h *WorkspaceHandler) handleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.DeleteWorkspaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := req.Validate(); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	err := h.workspaceService.DeleteWorkspace(r.Context(), req.ID)
	if err != nil {
		WriteJSONError(w, "Failed to delete workspace", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "success"})
}

// handleMembers handles the request to get members of a workspace
func (h *WorkspaceHandler) handleMembers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get workspace ID from query params
	workspaceID := r.URL.Query().Get("id")
	if workspaceID == "" {
		WriteJSONError(w, "Missing workspace ID", http.StatusBadRequest)
		return
	}

	// Use the new method that includes emails
	members, err := h.workspaceService.GetWorkspaceMembersWithEmail(r.Context(), workspaceID)
	if err != nil {
		WriteJSONError(w, "Failed to get workspace members", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"members": members,
	})
}

// handleInviteMember handles the request to invite a member to a workspace
func (h *WorkspaceHandler) handleInviteMember(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.InviteMemberRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := req.Validate(); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Create the invitation or add the user directly if they already exist
	invitation, token, err := h.workspaceService.InviteMember(r.Context(), req.WorkspaceID, req.Email)
	if err != nil {
		h.logger.WithField("workspace_id", req.WorkspaceID).WithField("email", req.Email).WithField("error", err.Error()).Error("Failed to invite member")
		WriteJSONError(w, "Failed to invite member", http.StatusInternalServerError)
		return
	}

	// If invitation is nil, it means the user was directly added to the workspace
	if invitation == nil {
		writeJSON(w, http.StatusOK, map[string]string{
			"status":  "success",
			"message": "User added to workspace",
		})
		return
	}

	// Return the invitation details and token
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":     "success",
		"message":    "Invitation sent",
		"invitation": invitation,
		"token":      token,
	})
}

// handleCreateAPIKey handles the request to create an API key for a workspace
func (h *WorkspaceHandler) handleCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.CreateAPIKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := req.Validate(); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Use the workspace service to create the API key
	token, apiEmail, err := h.workspaceService.CreateAPIKey(r.Context(), req.WorkspaceID, req.EmailPrefix)
	if err != nil {
		h.logger.WithField("workspace_id", req.WorkspaceID).WithField("error", err.Error()).Error("Failed to create API key")

		// Check if it's an authorization error
		if _, ok := err.(*domain.ErrUnauthorized); ok {
			WriteJSONError(w, "Only workspace owners can create API keys", http.StatusForbidden)
			return
		}

		WriteJSONError(w, "Failed to create API key", http.StatusInternalServerError)
		return
	}

	// Return the token and API details
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "success",
		"token":  token,
		"email":  apiEmail,
	})
}

// RemoveMemberRequest defines the request structure for removing a member
type RemoveMemberRequest struct {
	WorkspaceID string `json:"workspace_id"`
	UserID      string `json:"user_id"`
}

func (h *WorkspaceHandler) handleRemoveMember(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req RemoveMemberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate request
	if req.WorkspaceID == "" {
		WriteJSONError(w, "Missing workspace_id", http.StatusBadRequest)
		return
	}
	if req.UserID == "" {
		WriteJSONError(w, "Missing user_id", http.StatusBadRequest)
		return
	}

	// Call service to remove the member
	err := h.workspaceService.RemoveMember(r.Context(), req.WorkspaceID, req.UserID)
	if err != nil {
		if _, ok := err.(*domain.ErrUnauthorized); ok {
			WriteJSONError(w, err.Error(), http.StatusForbidden)
			return
		}
		h.logger.WithField("workspace_id", req.WorkspaceID).WithField("user_id", req.UserID).WithField("error", err.Error()).Error("Failed to remove member from workspace")
		WriteJSONError(w, "Failed to remove member from workspace", http.StatusInternalServerError)
		return
	}

	// Return success response
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "success",
		"message": "Member removed successfully",
	})
}

// handleCreateIntegration handles the request to create a new integration
func (h *WorkspaceHandler) handleCreateIntegration(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.CreateIntegrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := req.Validate(h.secretKey); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	integrationID, err := h.workspaceService.CreateIntegration(
		r.Context(),
		req.WorkspaceID,
		req.Name,
		req.Type,
		req.Provider,
	)
	if err != nil {
		h.logger.WithField("workspace_id", req.WorkspaceID).WithField("error", err.Error()).Error("Failed to create integration")

		if _, ok := err.(*domain.ErrUnauthorized); ok {
			WriteJSONError(w, err.Error(), http.StatusForbidden)
			return
		}

		WriteJSONError(w, "Failed to create integration", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"status":         "success",
		"integration_id": integrationID,
	})
}

// handleUpdateIntegration handles the request to update an existing integration
func (h *WorkspaceHandler) handleUpdateIntegration(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.UpdateIntegrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := req.Validate(h.secretKey); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	err := h.workspaceService.UpdateIntegration(
		r.Context(),
		req.WorkspaceID,
		req.IntegrationID,
		req.Name,
		req.Provider,
	)
	if err != nil {
		h.logger.WithField("workspace_id", req.WorkspaceID).WithField("integration_id", req.IntegrationID).WithField("error", err.Error()).Error("Failed to update integration")

		if _, ok := err.(*domain.ErrUnauthorized); ok {
			WriteJSONError(w, err.Error(), http.StatusForbidden)
			return
		}

		WriteJSONError(w, "Failed to update integration", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "success",
		"message": "Integration updated successfully",
	})
}

// handleDeleteIntegration handles the request to delete an integration
func (h *WorkspaceHandler) handleDeleteIntegration(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.DeleteIntegrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := req.Validate(); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	err := h.workspaceService.DeleteIntegration(
		r.Context(),
		req.WorkspaceID,
		req.IntegrationID,
	)
	if err != nil {
		h.logger.WithField("workspace_id", req.WorkspaceID).WithField("integration_id", req.IntegrationID).WithField("error", err.Error()).Error("Failed to delete integration")

		if _, ok := err.(*domain.ErrUnauthorized); ok {
			WriteJSONError(w, err.Error(), http.StatusForbidden)
			return
		}

		WriteJSONError(w, "Failed to delete integration", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "success",
		"message": "Integration deleted successfully",
	})
}
