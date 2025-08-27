package http

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"aidanwoods.dev/go-paseto"
	"go.opencensus.io/trace"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/http/middleware"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/pkg/tracing"
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
	tracer           tracing.Tracer
}

// extractEmailDomain extracts domain part from an email address
// This is used to add context to traces without exposing PII
func extractEmailDomain(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) == 2 {
		return parts[1]
	}
	return ""
}

func NewUserHandler(userService UserServiceInterface, workspaceService domain.WorkspaceServiceInterface, cfg *config.Config, publicKey paseto.V4AsymmetricPublicKey, logger logger.Logger) *UserHandler {
	return &UserHandler{
		userService:      userService,
		workspaceService: workspaceService,
		config:           cfg,
		publicKey:        publicKey,
		logger:           logger,
		tracer:           tracing.GetTracer(),
	}
}

func (h *UserHandler) SignIn(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tracer.StartSpan(r.Context(), "UserHandler.SignIn")
	defer span.End()

	var input domain.SignInInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		WriteJSONError(w, "Invalid SignIn request body", http.StatusBadRequest)
		span.SetStatus(trace.Status{
			Code:    trace.StatusCodeInvalidArgument,
			Message: "Invalid request body",
		})
		return
	}

	// Add email domain to span for context (masking email for privacy)
	span.AddAttributes(trace.StringAttribute("user.email.domain", extractEmailDomain(input.Email)))

	h.tracer.AddAttribute(ctx, "operation", "SignIn")
	code, err := h.userService.SignIn(ctx, input)
	if err != nil {
		// Check if it's a user not found error and return 400
		if _, ok := err.(*domain.ErrUserNotFound); ok {
			WriteJSONError(w, err.Error(), http.StatusBadRequest)
			h.tracer.MarkSpanError(ctx, err)
			return
		}

		// For all other errors, return 500
		WriteJSONError(w, err.Error(), http.StatusInternalServerError)
		h.tracer.MarkSpanError(ctx, err)
		return
	}

	// In development mode, the code will be returned
	// In production, the code will be empty
	response := map[string]string{
		"message": "Magic code sent to your email",
	}

	if code != "" {
		response["code"] = code
		span.AddAttributes(trace.BoolAttribute("dev.code_returned", true))
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

func (h *UserHandler) VerifyCode(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tracer.StartSpan(r.Context(), "UserHandler.VerifyCode")
	defer span.End()

	var input domain.VerifyCodeInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		WriteJSONError(w, "Invalid VerifyCode request body", http.StatusBadRequest)
		span.SetStatus(trace.Status{
			Code:    trace.StatusCodeInvalidArgument,
			Message: "Invalid request body",
		})
		return
	}

	// Add email domain to span for context (masking email for privacy)
	span.AddAttributes(trace.StringAttribute("user.email.domain", extractEmailDomain(input.Email)))

	h.tracer.AddAttribute(ctx, "operation", "VerifyCode")
	response, err := h.userService.VerifyCode(ctx, input)
	if err != nil {
		WriteJSONError(w, err.Error(), http.StatusUnauthorized)
		h.tracer.MarkSpanError(ctx, err)
		return
	}

	// Set user ID in span once we have it
	if response != nil && response.User.ID != "" {
		span.AddAttributes(trace.StringAttribute("user.id", response.User.ID))
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// GetCurrentUser returns the authenticated user and their workspaces
func (h *UserHandler) GetCurrentUser(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tracer.StartSpan(r.Context(), "UserHandler.GetCurrentUser")
	defer span.End()

	// Get authenticated user from context
	userID, ok := ctx.Value(domain.UserIDKey).(string)
	if !ok || userID == "" {
		WriteJSONError(w, "Unauthorized", http.StatusUnauthorized)
		span.SetStatus(trace.Status{
			Code:    trace.StatusCodePermissionDenied,
			Message: "Unauthorized - missing userID",
		})
		return
	}

	// Add user ID to span for context
	span.AddAttributes(trace.StringAttribute("user.id", userID))

	// Get user details
	h.tracer.AddAttribute(ctx, "operation", "GetUserByID")
	user, err := h.userService.GetUserByID(ctx, userID)
	if err != nil {
		WriteJSONError(w, "User not found", http.StatusNotFound)
		span.SetStatus(trace.Status{
			Code:    trace.StatusCodeNotFound,
			Message: "User not found",
		})
		return
	}

	// Add user email domain to span for context (masking email for privacy)
	span.AddAttributes(trace.StringAttribute("user.email.domain", extractEmailDomain(user.Email)))

	// Get user's workspaces
	h.tracer.AddAttribute(ctx, "operation", "ListWorkspaces")
	workspaces, err := h.workspaceService.ListWorkspaces(ctx)
	if err != nil {
		WriteJSONError(w, "Failed to retrieve workspaces", http.StatusInternalServerError)
		span.SetStatus(trace.Status{
			Code:    trace.StatusCodeInternal,
			Message: "Failed to retrieve workspaces",
		})
		return
	}

	// Add workspace count to span for context
	span.AddAttributes(trace.Int64Attribute("workspaces.count", int64(len(workspaces))))

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
