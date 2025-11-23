package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/http/middleware"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// WorkspaceHandler handles HTTP requests for workspace operations
type WorkspaceHandler struct {
	workspaceService domain.WorkspaceServiceInterface
	authService      domain.AuthService
	getJWTSecret     func() ([]byte, error)
	logger           logger.Logger
	secretKey        string
}

// NewWorkspaceHandler creates a new workspace handler
func NewWorkspaceHandler(
	workspaceService domain.WorkspaceServiceInterface,
	authService domain.AuthService,
	getJWTSecret func() ([]byte, error),
	logger logger.Logger,
	secretKey string,
) *WorkspaceHandler {
	return &WorkspaceHandler{
		workspaceService: workspaceService,
		authService:      authService,
		getJWTSecret:     getJWTSecret,
		logger:           logger,
		secretKey:        secretKey,
	}
}

// RegisterRoutes registers all workspace RPC-style routes with authentication middleware
func (h *WorkspaceHandler) RegisterRoutes(mux *http.ServeMux) {
	// Create auth middleware
	authMiddleware := middleware.NewAuthMiddleware(h.getJWTSecret)
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
	mux.Handle("/api/workspaces.deleteInvitation", requireAuth(http.HandlerFunc(h.handleDeleteInvitation)))
	mux.Handle("/api/workspaces.setUserPermissions", requireAuth(http.HandlerFunc(h.handleSetUserPermissions)))

	// Public invitation routes (no authentication required)
	mux.Handle("/api/workspaces.verifyInvitationToken", http.HandlerFunc(h.handleVerifyInvitationToken))
	mux.Handle("/api/workspaces.acceptInvitation", http.HandlerFunc(h.handleAcceptInvitation))

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
		// Check if it's a workspace not found error (using errors.As to handle wrapped errors)
		var workspaceNotFoundErr *domain.ErrWorkspaceNotFound
		if errors.As(err, &workspaceNotFoundErr) {
			WriteJSONError(w, "Workspace not found", http.StatusNotFound)
			return
		}
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
		// Check if it's a workspace not found error (using errors.As to handle wrapped errors)
		var workspaceNotFoundErr *domain.ErrWorkspaceNotFound
		if errors.As(err, &workspaceNotFoundErr) {
			WriteJSONError(w, "Workspace not found", http.StatusNotFound)
			return
		}
		// Check if it's a validation error (e.g., DNS verification failed)
		var validationErr domain.ValidationError
		if errors.As(err, &validationErr) {
			WriteJSONError(w, validationErr.Message, http.StatusBadRequest)
			return
		}
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
		// Check if it's a workspace not found error (using errors.As to handle wrapped errors)
		var workspaceNotFoundErr *domain.ErrWorkspaceNotFound
		if errors.As(err, &workspaceNotFoundErr) {
			WriteJSONError(w, "Workspace not found", http.StatusNotFound)
			return
		}
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
		// Check if it's a workspace not found error (using errors.As to handle wrapped errors)
		var workspaceNotFoundErr *domain.ErrWorkspaceNotFound
		if errors.As(err, &workspaceNotFoundErr) {
			WriteJSONError(w, "Workspace not found", http.StatusNotFound)
			return
		}
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
	invitation, token, err := h.workspaceService.InviteMember(r.Context(), req.WorkspaceID, req.Email, req.Permissions)
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

// handleSetUserPermissions handles the request to set permissions for a user
func (h *WorkspaceHandler) handleSetUserPermissions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.SetUserPermissionsRequest
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
	if req.Permissions == nil {
		WriteJSONError(w, "Missing permissions", http.StatusBadRequest)
		return
	}

	// Call service to set user permissions
	err := h.workspaceService.SetUserPermissions(r.Context(), req.WorkspaceID, req.UserID, req.Permissions)
	if err != nil {
		if _, ok := err.(*domain.ErrUnauthorized); ok {
			WriteJSONError(w, err.Error(), http.StatusForbidden)
			return
		}
		h.logger.WithField("workspace_id", req.WorkspaceID).WithField("user_id", req.UserID).WithField("error", err.Error()).Error("Failed to set user permissions")
		WriteJSONError(w, "Failed to set user permissions", http.StatusInternalServerError)
		return
	}

	// Return success response
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "success",
		"message": "User permissions updated successfully",
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

		WriteJSONError(w, err.Error(), http.StatusInternalServerError)
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

// VerifyInvitationTokenRequest defines the request structure for verifying invitation tokens
type VerifyInvitationTokenRequest struct {
	Token string `json:"token"`
}

// AcceptInvitationRequest defines the request structure for accepting invitations
type AcceptInvitationRequest struct {
	Token string `json:"token"`
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

	integrationID, err := h.workspaceService.CreateIntegration(r.Context(), req)
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

	err := h.workspaceService.UpdateIntegration(r.Context(), req)
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

// handleVerifyInvitationToken verifies an invitation token and returns invitation details
func (h *WorkspaceHandler) handleVerifyInvitationToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req VerifyInvitationTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Token == "" {
		WriteJSONError(w, "Token is required", http.StatusBadRequest)
		return
	}

	// Validate the invitation token
	invitationID, workspaceID, email, err := h.authService.ValidateInvitationToken(req.Token)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to validate invitation token")
		WriteJSONError(w, "Invalid or expired invitation token", http.StatusUnauthorized)
		return
	}

	// Get invitation details from database
	invitation, err := h.workspaceService.GetInvitationByID(r.Context(), invitationID)
	if err != nil {
		h.logger.WithField("invitation_id", invitationID).WithField("error", err.Error()).Error("Failed to get invitation")
		WriteJSONError(w, "Invitation not found", http.StatusNotFound)
		return
	}

	// Verify that the invitation details match the token
	if invitation.WorkspaceID != workspaceID || invitation.Email != email {
		h.logger.WithField("invitation_id", invitationID).Error("Invitation details mismatch")
		WriteJSONError(w, "Invalid invitation token", http.StatusUnauthorized)
		return
	}

	// Get workspace details using system context to bypass authentication for invitation verification
	systemCtx := context.WithValue(r.Context(), domain.SystemCallKey, true)
	workspace, err := h.workspaceService.GetWorkspace(systemCtx, workspaceID)
	if err != nil {
		// Check if it's a workspace not found error
		var workspaceNotFoundErr *domain.ErrWorkspaceNotFound
		if errors.As(err, &workspaceNotFoundErr) {
			h.logger.WithField("workspace_id", workspaceID).WithField("error", err.Error()).Error("Workspace not found for invitation verification")
			WriteJSONError(w, "Workspace not found", http.StatusNotFound)
			return
		}
		h.logger.WithField("workspace_id", workspaceID).WithField("error", err.Error()).Error("Failed to get workspace")
		WriteJSONError(w, "Failed to get workspace", http.StatusInternalServerError)
		return
	}

	// Return invitation and workspace details
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":     "success",
		"invitation": invitation,
		"workspace":  workspace,
		"valid":      true,
	})
}

// handleAcceptInvitation processes an invitation token to create user and add to workspace
func (h *WorkspaceHandler) handleAcceptInvitation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req AcceptInvitationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Token == "" {
		WriteJSONError(w, "Token is required", http.StatusBadRequest)
		return
	}

	// Validate the invitation token
	invitationID, workspaceID, email, err := h.authService.ValidateInvitationToken(req.Token)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to validate invitation token")
		WriteJSONError(w, "Invalid or expired invitation token", http.StatusUnauthorized)
		return
	}

	// Process the invitation acceptance
	authResponse, err := h.workspaceService.AcceptInvitation(r.Context(), invitationID, workspaceID, email)
	if err != nil {
		h.logger.WithField("invitation_id", invitationID).WithField("workspace_id", workspaceID).WithField("email", email).WithField("error", err.Error()).Error("Failed to accept invitation")
		WriteJSONError(w, "Failed to accept invitation", http.StatusInternalServerError)
		return
	}

	// Return success response with auth token
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":       "success",
		"message":      "Invitation accepted successfully",
		"workspace_id": workspaceID,
		"email":        email,
		"token":        authResponse.Token,
		"user":         authResponse.User,
		"expires_at":   authResponse.ExpiresAt,
	})
}

// handleDeleteInvitation processes the deletion of a workspace invitation
func (h *WorkspaceHandler) handleDeleteInvitation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		InvitationID string `json:"invitation_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to parse delete invitation request")
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.InvitationID == "" {
		WriteJSONError(w, "invitation_id is required", http.StatusBadRequest)
		return
	}

	// Delete the invitation
	err := h.workspaceService.DeleteInvitation(r.Context(), req.InvitationID)
	if err != nil {
		h.logger.WithField("invitation_id", req.InvitationID).WithField("error", err.Error()).Error("Failed to delete invitation")
		WriteJSONError(w, "Failed to delete invitation", http.StatusInternalServerError)
		return
	}

	// Return success response
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "success",
		"message": "Invitation deleted successfully",
	})
}
