package http

import (
	"context"
	"encoding/json"
	"net/http"

	"aidanwoods.dev/go-paseto"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/http/middleware"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// WorkspaceServiceInterface is already defined in workspace_handler.go
// So no need to define it again here

// UserServiceInterface defines the methods required from a user service
type UserServiceInterface interface {
	SignIn(ctx context.Context, input domain.SignInInput) (string, error)
	VerifyCode(ctx context.Context, input domain.VerifyCodeInput) (*domain.AuthResponse, error)
	VerifyUserSession(ctx context.Context, userID string, sessionID string) (*domain.User, error)
	GetUserByID(ctx context.Context, userID string) (*domain.User, error)
}

type UserHandler struct {
	userService      UserServiceInterface
	workspaceService domain.WorkspaceServiceInterface
	config           *config.Config
	publicKey        paseto.V4AsymmetricPublicKey
	logger           logger.Logger
}

func NewUserHandler(userService UserServiceInterface, workspaceService domain.WorkspaceServiceInterface, cfg *config.Config, publicKey paseto.V4AsymmetricPublicKey, logger logger.Logger) *UserHandler {
	return &UserHandler{
		userService:      userService,
		workspaceService: workspaceService,
		config:           cfg,
		publicKey:        publicKey,
		logger:           logger,
	}
}

func (h *UserHandler) SignIn(w http.ResponseWriter, r *http.Request) {
	var input domain.SignInInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		WriteJSONError(w, "Invalid SignIn request body", http.StatusBadRequest)
		return
	}

	code, err := h.userService.SignIn(r.Context(), input)
	if err != nil {
		WriteJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// In development mode, the code will be returned
	// In production, the code will be empty
	response := map[string]string{
		"message": "Magic code sent to your email",
	}

	if code != "" {
		response["code"] = code
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

func (h *UserHandler) VerifyCode(w http.ResponseWriter, r *http.Request) {
	var input domain.VerifyCodeInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		WriteJSONError(w, "Invalid VerifyCode request body", http.StatusBadRequest)
		return
	}

	response, err := h.userService.VerifyCode(r.Context(), input)
	if err != nil {
		WriteJSONError(w, err.Error(), http.StatusUnauthorized)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// GetCurrentUser returns the authenticated user and their workspaces
func (h *UserHandler) GetCurrentUser(w http.ResponseWriter, r *http.Request) {
	// Get authenticated user from context
	userID, ok := r.Context().Value(domain.UserIDKey).(string)
	if !ok || userID == "" {
		WriteJSONError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Get user details
	user, err := h.userService.GetUserByID(r.Context(), userID)
	if err != nil {
		WriteJSONError(w, "User not found", http.StatusNotFound)
		return
	}

	// Get user's workspaces
	workspaces, err := h.workspaceService.ListWorkspaces(r.Context())
	if err != nil {
		WriteJSONError(w, "Failed to retrieve workspaces", http.StatusInternalServerError)
		return
	}

	// Combine user and workspaces in response
	response := map[string]interface{}{
		"user":       user,
		"workspaces": workspaces,
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

func (h *UserHandler) RegisterRoutes(mux *http.ServeMux) {
	// Public routes (no auth required)
	mux.HandleFunc("/api/user.signin", h.SignIn)
	mux.HandleFunc("/api/user.verify", h.VerifyCode)

	// Create auth middleware
	authMiddleware := middleware.NewAuthMiddleware(h.publicKey)
	requireAuth := authMiddleware.RequireAuth()

	// Register protected routes
	mux.Handle("/api/user.me", requireAuth(http.HandlerFunc(h.GetCurrentUser)))
}
