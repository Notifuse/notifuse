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

	// webAnalyticsCacheInvalidator, when set, drops the ingest path's cached
	// settings of a workspace after they change.
	webAnalyticsCacheInvalidator func(workspaceID string)

	isDemo bool
}

// WithWebAnalyticsCacheInvalidator wires the ingest settings-cache
// invalidation callback (optional).
func (h *WorkspaceHandler) WithWebAnalyticsCacheInvalidator(fn func(workspaceID string)) *WorkspaceHandler {
	h.webAnalyticsCacheInvalidator = fn
	return h
}

// NewWorkspaceHandler creates a new workspace handler.
// isDemo closes the mutating workspace routes on a demo instance; it is a
// constructor parameter so that forgetting it is a compile error.
func NewWorkspaceHandler(
	workspaceService domain.WorkspaceServiceInterface,
	authService domain.AuthService,
	getJWTSecret func() ([]byte, error),
	logger logger.Logger,
	secretKey string,
	isDemo bool,
) *WorkspaceHandler {
	return &WorkspaceHandler{
		workspaceService: workspaceService,
		authService:      authService,
		getJWTSecret:     getJWTSecret,
		logger:           logger,
		secretKey:        secretKey,
		isDemo:           isDemo,
	}
}

// RegisterRoutes registers all workspace RPC-style routes with authentication middleware
func (h *WorkspaceHandler) RegisterRoutes(mux *http.ServeMux) {
	// Create auth middleware
	authMiddleware := middleware.NewAuthMiddleware(h.getJWTSecret)
	requireAuth := authMiddleware.RequireAuth()

	// The demo instance is publicly writable, so every mutating endpoint is closed
	// there — membership, API keys and integrations hand out durable credentials to
	// whoever asks, and the rest reconfigures the single shared workspace. Reads
	// stay open: the demo exists to be browsed.
	restrictedInDemo := middleware.RestrictedInDemo(h.isDemo)

	// Register RPC-style endpoints with dot notation
	mux.Handle("/api/workspaces.list", requireAuth(http.HandlerFunc(h.handleList)))
	mux.Handle("/api/workspaces.get", requireAuth(http.HandlerFunc(h.handleGet)))
	mux.Handle("/api/workspaces.create", restrictedInDemo(requireAuth(http.HandlerFunc(h.handleCreate))))
	mux.Handle("/api/workspaces.update", restrictedInDemo(requireAuth(http.HandlerFunc(h.handleUpdate))))
	mux.Handle("/api/workspaces.delete", restrictedInDemo(requireAuth(http.HandlerFunc(h.handleDelete))))
	mux.Handle("/api/workspaces.members", requireAuth(http.HandlerFunc(h.handleMembers)))
	mux.Handle("/api/workspaces.inviteMember", restrictedInDemo(requireAuth(http.HandlerFunc(h.handleInviteMember))))
	mux.Handle("/api/workspaces.createAPIKey", restrictedInDemo(requireAuth(http.HandlerFunc(h.handleCreateAPIKey))))
	mux.Handle("/api/workspaces.removeMember", restrictedInDemo(requireAuth(http.HandlerFunc(h.handleRemoveMember))))
	mux.Handle("/api/workspaces.deleteInvitation", restrictedInDemo(requireAuth(http.HandlerFunc(h.handleDeleteInvitation))))
	mux.Handle("/api/workspaces.setUserPermissions", restrictedInDemo(requireAuth(http.HandlerFunc(h.handleSetUserPermissions))))
	mux.Handle("/api/workspaces.setCustomFieldLabels", restrictedInDemo(requireAuth(http.HandlerFunc(h.handleSetCustomFieldLabels))))
	mux.Handle("/api/workspaces.setBlogSettings", restrictedInDemo(requireAuth(http.HandlerFunc(h.handleSetBlogSettings))))
	mux.Handle("/api/workspaces.setWebAnalyticsSettings", restrictedInDemo(requireAuth(http.HandlerFunc(h.handleSetWebAnalyticsSettings))))

	// Public invitation routes (no authentication required)
	mux.Handle("/api/workspaces.verifyInvitationToken", http.HandlerFunc(h.handleVerifyInvitationToken))
	mux.Handle("/api/workspaces.acceptInvitation", http.HandlerFunc(h.handleAcceptInvitation))

	// Integration management routes
	mux.Handle("/api/workspaces.createIntegration", restrictedInDemo(requireAuth(http.HandlerFunc(h.handleCreateIntegration))))
	mux.Handle("/api/workspaces.updateIntegration", restrictedInDemo(requireAuth(http.HandlerFunc(h.handleUpdateIntegration))))
	mux.Handle("/api/workspaces.deleteIntegration", restrictedInDemo(requireAuth(http.HandlerFunc(h.handleDeleteIntegration))))
	mux.Handle("/api/workspaces.connectZapier", restrictedInDemo(requireAuth(http.HandlerFunc(h.handleConnectZapier))))
}

func (h *WorkspaceHandler) handleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	workspaces, err := h.workspaceService.ListWorkspaces(r.Context())
	if err != nil {
		if writeServiceError(w, err, "Failed to list workspaces") {
			return
		}
		WriteJSONError(w, "Failed to list workspaces", http.StatusInternalServerError)
		return
	}

	// Credentials are decrypted on load for the sending path; they must not leave
	// the process. See redactWorkspaceForCaller.
	//
	// This endpoint is the one an integration reaches for first — it is the only
	// workspace endpoint no permission gates, so it is how a key discovers what it
	// is attached to.
	for _, ws := range workspaces {
		redactWorkspaceForCaller(r.Context(), ws)
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
		if writeServiceError(w, err, "You do not have access to this workspace") {
			return
		}
		WriteJSONError(w, "Failed to get workspace", http.StatusInternalServerError)
		return
	}
	if workspace == nil {
		WriteJSONError(w, "Workspace not found", http.StatusNotFound)
		return
	}

	redactWorkspaceForCaller(r.Context(), workspace)

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
		req.Settings.DefaultLanguage,
		req.Settings.Languages,
	)
	if err != nil {
		var limitErr *domain.ErrWorkspaceLimitReached
		if errors.As(err, &limitErr) {
			WriteJSONError(w, limitErr.Error(), http.StatusForbidden)
			return
		}
		if err.Error() == "workspace already exists" {
			WriteJSONError(w, "Workspace already exists", http.StatusConflict)
		} else {
			WriteJSONError(w, "Failed to create workspace", http.StatusInternalServerError)
		}
		return
	}

	redactWorkspaceForCaller(r.Context(), workspace)
	writeJSON(w, http.StatusCreated, workspace)
}

// Helper function to get bytes from request body
func getBytesFromBody(body io.ReadCloser) []byte {
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(body)
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
		if writeServiceError(w, err, "You are not allowed to update this workspace") {
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

	redactWorkspaceForCaller(r.Context(), workspace)
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
		if writeServiceError(w, err, "You are not allowed to delete this workspace") {
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
		if writeServiceError(w, err, "You do not have access to this workspace") {
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
		var limitErr *domain.ErrTeamMemberLimitReached
		if errors.As(err, &limitErr) {
			WriteJSONError(w, limitErr.Error(), http.StatusForbidden)
			return
		}
		if writeServiceError(w, err, "Only workspace owners can invite members") {
			return
		}
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

	// Validate here rather than leaving it to the service: the request carries a
	// permission map, and an unknown resource key is a malformed request (400), not
	// an internal failure.
	if err := req.Validate(); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Call service to set user permissions
	err := h.workspaceService.SetUserPermissions(r.Context(), req.WorkspaceID, req.UserID, req.Permissions)
	if err != nil {
		if writeServiceError(w, err, "Only workspace owners can manage user permissions") {
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

func (h *WorkspaceHandler) handleSetCustomFieldLabels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.SetCustomFieldLabelsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	workspaceID, labels, err := req.Validate()
	if err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.workspaceService.SetCustomFieldLabels(r.Context(), workspaceID, labels); err != nil {
		if writePermissionError(w, err) {
			return
		}
		var unauthorized *domain.ErrUnauthorized
		if errors.As(err, &unauthorized) {
			WriteJSONError(w, unauthorized.Message, http.StatusForbidden)
			return
		}
		h.logger.WithField("workspace_id", workspaceID).WithField("error", err.Error()).Error("Failed to set custom field labels")
		WriteJSONError(w, "Failed to set custom field labels", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "success",
		"message": "Custom field labels updated successfully",
	})
}

// handleSetBlogSettings handles the request to set workspace blog settings (the
// enable flag plus title/SEO/pagination/feed config) via the dedicated, blog:write
// gated endpoint. Unlike workspaces.update (owner-only), this lets a member with
// blog:write manage blog configuration.
func (h *WorkspaceHandler) handleSetBlogSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.SetBlogSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	workspaceID, enabled, settings, settingsSpecified, err := req.Validate()
	if err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// settingsSpecified travels with the settings rather than being resolved here: blog_settings
	// is not part of the replace when the body leaves it out, and the merge belongs to the write,
	// which already holds the workspace it is about to save. Clearing the configuration
	// deliberately stays expressible, as an explicit null.
	if err := h.workspaceService.SetBlogSettings(r.Context(), workspaceID, enabled, settings, settingsSpecified); err != nil {
		if writePermissionError(w, err) {
			return
		}
		var unauthorized *domain.ErrUnauthorized
		if errors.As(err, &unauthorized) {
			WriteJSONError(w, unauthorized.Message, http.StatusForbidden)
			return
		}
		h.logger.WithField("workspace_id", workspaceID).WithField("error", err.Error()).Error("Failed to set blog settings")
		WriteJSONError(w, "Failed to set blog settings", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "success",
		"message": "Blog settings updated successfully",
	})
}

// handleSetWebAnalyticsSettings replaces a workspace's web analytics settings
// via the dedicated, web_analytics:write gated endpoint (mirrors the blog
// settings pattern: members with the feature permission manage it without
// workspace:write).
func (h *WorkspaceHandler) handleSetWebAnalyticsSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.SetWebAnalyticsSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	workspaceID, settings, err := req.Validate()
	if err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.workspaceService.SetWebAnalyticsSettings(r.Context(), workspaceID, settings); err != nil {
		if writePermissionError(w, err) {
			return
		}
		var unauthorized *domain.ErrUnauthorized
		if errors.As(err, &unauthorized) {
			WriteJSONError(w, unauthorized.Message, http.StatusForbidden)
			return
		}
		h.logger.WithField("workspace_id", workspaceID).WithField("error", err.Error()).Error("Failed to set web analytics settings")
		WriteJSONError(w, "Failed to set web analytics settings", http.StatusInternalServerError)
		return
	}

	// The ingest path caches settings for a minute; apply changes promptly.
	if h.webAnalyticsCacheInvalidator != nil {
		h.webAnalyticsCacheInvalidator(workspaceID)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "success",
		"message": "Web analytics settings updated successfully",
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

	// Use the workspace service to create the API key. An absent or null permissions
	// map means full access, which is what the endpoint granted before it took one.
	token, apiEmail, err := h.workspaceService.CreateAPIKey(r.Context(), req.WorkspaceID, req.EmailPrefix, req.Permissions)
	if err != nil {
		h.logger.WithField("workspace_id", req.WorkspaceID).WithField("error", err.Error()).Error("Failed to create API key")

		// Check if it's an authorization error
		var unauthorized *domain.ErrUnauthorized
		if errors.As(err, &unauthorized) {
			WriteJSONError(w, "Only workspace owners can create API keys", http.StatusForbidden)
			return
		}

		// users.email is unique across the deployment, so a prefix can be claimed once.
		var userExists *domain.ErrUserExists
		if errors.As(err, &userExists) {
			WriteJSONError(w, err.Error(), http.StatusConflict)
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
		var unauthorized *domain.ErrUnauthorized
		if errors.As(err, &unauthorized) {
			WriteJSONError(w, unauthorized.Message, http.StatusForbidden)
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

		var unauthorized *domain.ErrUnauthorized
		if errors.As(err, &unauthorized) {
			WriteJSONError(w, unauthorized.Message, http.StatusForbidden)
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

		var unauthorized *domain.ErrUnauthorized
		if errors.As(err, &unauthorized) {
			WriteJSONError(w, unauthorized.Message, http.StatusForbidden)
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

		var unauthorized *domain.ErrUnauthorized
		if errors.As(err, &unauthorized) {
			WriteJSONError(w, unauthorized.Message, http.StatusForbidden)
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

// ConnectZapierRequest defines the request structure for connecting Zapier to a workspace.
//
// The label is all the caller gets to choose: it names the card and seeds the address of the
// key minted for it, which the server derives itself so that no client can claim an address
// belonging to a key it did not create.
type ConnectZapierRequest struct {
	WorkspaceID string `json:"workspace_id"`
	Label       string `json:"label"`
}

// Validate checks the two fields the service cannot supply for itself.
//
// The label check is not cosmetic. ConnectZapier mints the API key before it builds the
// integration, and Integration.Validate rejects a nameless one, so a blank label that got
// this far would cost a key minted and then revoked by the compensation path.
func (r *ConnectZapierRequest) Validate() error {
	if r.WorkspaceID == "" {
		return errors.New("workspace_id is required")
	}

	if r.Label == "" {
		return errors.New("label is required")
	}

	return nil
}

// handleConnectZapier mints an API key for a Zapier connection and records it on the workspace
// as a zapier integration, in one call. The token is in the response once and is unrecoverable
// afterwards — nothing stores it.
//
// This is a control-plane endpoint the console alone calls, so it is deliberately absent from
// openapi/, which documents the customer-facing API only.
//
// It stays in the workspaces.* namespace rather than taking a zapier.* one of its own. The
// vendor precedent — ses.enableTenantIsolation and its siblings on their own handler — is a
// real one, but those proxy an external AWS API. This calls nothing external: it mints a
// workspace API key and writes the workspace row, both WorkspaceService concerns.
func (h *WorkspaceHandler) handleConnectZapier(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ConnectZapierRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := req.Validate(); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	token, apiEmail, integrationID, err := h.workspaceService.ConnectZapier(r.Context(), req.WorkspaceID, req.Label)
	if err != nil {
		h.logger.WithField("workspace_id", req.WorkspaceID).WithField("error", err.Error()).Error("Failed to connect Zapier")

		var unauthorized *domain.ErrUnauthorized
		if errors.As(err, &unauthorized) {
			WriteJSONError(w, unauthorized.Message, http.StatusForbidden)
			return
		}

		// users.email is unique across the deployment, so an address can be claimed once.
		// The service already retried with fresh randomness and still wraps *ErrUserExists
		// when it gives up. Nothing else on this path knows the type — writeServiceError
		// has no case for it either — so without this a conflict reads as a 500.
		var userExists *domain.ErrUserExists
		if errors.As(err, &userExists) {
			WriteJSONError(w, err.Error(), http.StatusConflict)
			return
		}

		WriteJSONError(w, "Failed to connect Zapier", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":         "success",
		"token":          token,
		"email":          apiEmail,
		"integration_id": integrationID,
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

	// This route is public — no authentication at all (see RegisterRoutes) — so it
	// redacts harder than the member-facing ones: no integrations, no credential
	// hints, no S3 secret. The page shows the workspace's name.
	workspace.RedactForPublic()

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
		var limitErr *domain.ErrTeamMemberLimitReached
		if errors.As(err, &limitErr) {
			WriteJSONError(w, limitErr.Error(), http.StatusForbidden)
			return
		}
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
